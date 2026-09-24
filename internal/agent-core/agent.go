package agentcore

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vanpiyp/awp/internal/llm"
)

const InvalidToolName = "invalid"

type EventCategory int

const (
	EventThoughtStart EventCategory = iota
	EventThoughtChunk
	EventThoughtEnd
	EventTool
	EventObserve
	EventFinalAnswer
	EventError
	EventInvalid
	EventUserMessage
	EventCompaction
)

type Event struct {
	Category   EventCategory
	Content    string
	Reasoning  string
	ToolName   string
	ToolArgs   string
	ToolResult string
	ToolError  string
	ToolIntent string
	ToolCalls  []llm.ToolCall
	Usage      *llm.Usage
	FromCache  bool

	Summary         string
	TokensBefore    int
	TokensAfter     int
	FirstKeptSeq    int
	CompactionModel string
}

type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]any
	Invoke(ctx context.Context, argsJSON string) (string, error)
}

type ToolFunc struct {
	N  string
	D  string
	P  map[string]any
	Fn func(context.Context, string) (string, error)
}

func (t ToolFunc) Name() string               { return t.N }
func (t ToolFunc) Description() string        { return t.D }
func (t ToolFunc) Parameters() map[string]any { return t.P }
func (t ToolFunc) Invoke(ctx context.Context, argsJSON string) (string, error) {
	return t.Fn(ctx, argsJSON)
}

const (
	IntentField       = "intent"
	IntentDescription = "Required short label shown in the UI: why this call is being made."
)

func RequireIntent(argsJSON string) error {
	var a struct {
		Intent string `json:"intent"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return fmt.Errorf("invalid args: %w", err)
	}
	if strings.TrimSpace(a.Intent) == "" {
		return fmt.Errorf("%s is required (%s)", IntentField, IntentDescription)
	}
	return nil
}

func RunTool[T any](ctx context.Context, argsJSON string, fn func(context.Context, T) (string, error)) (string, error) {
	var args T
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	return fn(ctx, args)
}

func extractToolIntent(argsJSON string) string {
	var a struct {
		Intent string `json:"intent"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return ""
	}
	return strings.TrimSpace(a.Intent)
}

type Agent struct {
	core                   llm.Core
	SafetyNet              int
	Model                  llm.Model
	SystemPrompts          string
	toolList               []Tool
	toolsByID              map[string]int
	sessionID              string
	LogWriter              io.Writer
	logMu                  sync.Mutex
	logBuf                 *bufio.Writer
	logSeq                 int
	compaction             CompactionSettings
	repeatedToolErrorLimit int
	strategy               Strategy
	currentParentID        string
	currentStreamBuf       *StreamBuffer
	currentToolCallID      string
	ToolCacheSize          int
	toolResultCache        *ToolResultCache
	currentTurn            int
	turnStartAt            time.Time
	turnFirstChunkAt       time.Time
	observedInputTokens    int
	currentMsgs            []llm.Message
	logFileOpened          bool
	v3LogWriter            io.Writer
	v3LogBuf               *bufio.Writer
}

type Strategy interface {
	Name() string
	Step(ctx context.Context, msgs []llm.Message, emit func(context.Context, Event) bool) (Step, error)
	ShouldAbort(msgs []llm.Message, lastFailedToolError string) error
}

type StepKind int

const (
	StepContinue StepKind = iota
	StepFinal
)

type Step struct {
	Kind         StepKind
	Content      string
	Reasoning    string
	ReasoningSig string
	ToolCalls    []llm.ToolCall
	Usage        *llm.Usage
	FinishReason string
}

type ReActStrategy struct {
	core                   llm.Core
	model                  llm.Model
	toolDefsGetter         func() []llm.ToolDef
	maxToolsPerTurn        int
	repeatedToolErrorLimit int
}

func NewReActStrategy(core llm.Core, model llm.Model, toolDefsGetter func() []llm.ToolDef) *ReActStrategy {
	return &ReActStrategy{
		core:                   core,
		model:                  model,
		toolDefsGetter:         toolDefsGetter,
		maxToolsPerTurn:        6,
		repeatedToolErrorLimit: 3,
	}
}

func (r *ReActStrategy) Name() string { return "react" }

func (r *ReActStrategy) Step(ctx context.Context, msgs []llm.Message, emit func(context.Context, Event) bool) (Step, error) {
	if !emit(ctx, Event{Category: EventThoughtStart}) {
		return Step{}, ctx.Err()
	}
	req := &llm.ChatRequest{Model: r.model.ID, Messages: msgs}
	if r.toolDefsGetter != nil {
		req.Tools = r.toolDefsGetter()
	}
	raw, err := r.core.StreamChat(ctx, req)
	if err != nil {
		if isCtxErr(err) {
			return Step{}, err
		}
		emit(ctx, Event{Category: EventError, ToolError: err.Error()})
		emit(ctx, Event{Category: EventThoughtEnd})
		return Step{}, nil
	}
	var result turnResult
	var contentBuf, reasoningBuf strings.Builder
	contentBuf.Grow(2048)
	reasoningBuf.Grow(2048)
	for ev := range raw {
		if ctx.Err() != nil {
			return Step{}, ctx.Err()
		}
		if !r.processStreamEvent(ctx, ev, &result, &contentBuf, &reasoningBuf, emit) {
			return Step{}, ctx.Err()
		}
	}
	result.content = contentBuf.String()
	result.reasoning = reasoningBuf.String()
	if !emit(ctx, Event{
		Category:  EventThoughtEnd,
		Content:   result.content,
		Reasoning: result.reasoning,
		ToolCalls: result.toolCalls,
		Usage:     result.usage,
	}) {
		return Step{}, ctx.Err()
	}
	switch {
	case result.finishReason == llm.FinishReasonLength && len(result.toolCalls) > 0:
		emit(ctx, Event{Category: EventError, ToolError: "response truncated mid tool call, refusing"})
		return Step{}, nil
	case result.finishReason == llm.FinishReasonLength && result.content == "":
		emit(ctx, Event{Category: EventError, ToolError: "response truncated with no content"})
		return Step{}, nil
	case result.finishReason == llm.FinishReasonToolUse && len(result.toolCalls) > 0 && allToolCallsEmpty(result.toolCalls):
		emit(ctx, Event{Category: EventError, ToolError: "empty tool calls, refusing"})
		return Step{}, nil
	case result.finishReason == llm.FinishReasonToolUse && len(result.toolCalls) > 0:
		return Step{
			Kind:         StepContinue,
			Content:      result.content,
			Reasoning:    result.reasoning,
			ReasoningSig: result.reasoningSig,
			ToolCalls:    result.toolCalls,
			Usage:        result.usage,
			FinishReason: result.finishReason.String(),
		}, nil
	case result.finishReason == llm.FinishReasonToolUse && len(result.toolCalls) == 0:
		emit(ctx, Event{Category: EventError, ToolError: "tool_use finish reason with no tool calls"})
		return Step{}, nil
	case (result.finishReason == llm.FinishReasonStop || (result.finishReason == llm.FinishReasonLength && result.content != "")) && len(result.toolCalls) == 0:
		emit(ctx, Event{Category: EventFinalAnswer, Content: result.content, Usage: result.usage})
		return Step{
			Kind:         StepFinal,
			Content:      result.content,
			Usage:        result.usage,
			FinishReason: result.finishReason.String(),
		}, nil
	}
	return Step{}, fmt.Errorf("unreachable: finishReason=%v toolCalls=%d", result.finishReason, len(result.toolCalls))
}

func (r *ReActStrategy) processStreamEvent(ctx context.Context, ev llm.StreamEvent, result *turnResult, contentBuf, reasoningBuf *strings.Builder, emit func(context.Context, Event) bool) bool {
	if ev.Err != nil {
		if isCtxErr(ev.Err) {
			return false
		}
		emit(ctx, Event{Category: EventError, ToolError: ev.Err.Error()})
		emit(ctx, Event{Category: EventThoughtEnd})
		return false
	}
	if ev.Chunk == nil {
		return true
	}
	for _, c := range ev.Chunk.Choices {
		if c.Delta.Reasoning != "" {
			reasoningBuf.WriteString(c.Delta.Reasoning)
			if !emit(ctx, Event{Category: EventThoughtChunk, Reasoning: c.Delta.Reasoning}) {
				return false
			}
		}
		if c.Delta.ReasoningSig != "" {
			result.reasoningSig = c.Delta.ReasoningSig
		}
		if c.Delta.Content != "" {
			contentBuf.WriteString(c.Delta.Content)
			if !emit(ctx, Event{Category: EventThoughtChunk, Content: c.Delta.Content}) {
				return false
			}
		}
		if c.FinishReason != llm.FinishReasonUnknown {
			result.finishReason = c.FinishReason
		}
		if len(c.Delta.ToolCalls) > 0 {
			result.toolCalls = append(result.toolCalls, c.Delta.ToolCalls...)
		}
	}
	if ev.Chunk.Usage != nil {
		result.usage = ev.Chunk.Usage
	}
	return true
}

func (r *ReActStrategy) ShouldAbort(msgs []llm.Message, lastFailedToolError string) error {
	const maxConsecutiveRepeats = 3
	recentCalls := []string{}
	for _, m := range msgs {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			sig := toolCallSignature(m.ToolCalls)
			recentCalls = append(recentCalls, sig)
			if len(recentCalls) > maxConsecutiveRepeats {
				recentCalls = recentCalls[1:]
			}
			if len(recentCalls) >= maxConsecutiveRepeats && allEqual(recentCalls) {
				return fmt.Errorf("tool calls repeated %d times, aborting", maxConsecutiveRepeats+1)
			}
		}
	}
	if lastFailedToolError == "" {
		return nil
	}
	consecutiveTurns := 0
	i := len(msgs) - 1
	for i >= 0 {
		m := msgs[i]
		if !(m.Role == "tool" && m.Content == lastFailedToolError) {
			break
		}
		consecutiveTurns++
		i--
		for i >= 0 && msgs[i].Role == "tool" {
			i--
		}
		for i >= 0 && msgs[i].Role != "assistant" {
			i--
		}
		i--
	}
	if consecutiveTurns >= r.repeatedToolErrorLimit {
		return fmt.Errorf("aborting: tool failed %d times in a row with the same error: %s. Stop and report to the user instead of retrying.", consecutiveTurns, lastFailedToolError)
	}
	return nil
}

func NewAgent(llmCore llm.Core) *Agent {
	a := &Agent{
		core:          llmCore,
		SafetyNet:     200,
		SystemPrompts: "You are a helpful coding assistant",
		compaction: CompactionSettings{
			Enabled:         true,
			ReserveTokens:   16384,
			KeepRecentTurns: 5,
		},
		repeatedToolErrorLimit: 3,
		ToolCacheSize:          20,
	}
	a.toolResultCache = NewToolResultCache(a.ToolCacheSize)
	a.strategy = NewReActStrategy(a.core, a.Model, a.toolDefsForStrategy)
	return a
}

func (a *Agent) WithSafetyNet(n int) *Agent {
	if n <= 0 {
		n = 200
	}
	a.SafetyNet = n
	return a
}

func (a *Agent) WithModel(model llm.Model) *Agent {
	a.Model = model
	if rs, ok := a.strategy.(*ReActStrategy); ok && rs != nil {
		rs.model = model
	}
	return a
}

func (a *Agent) WithRepeatedToolErrorLimit(n int) *Agent {
	if n < 1 {
		n = 3
	}
	a.repeatedToolErrorLimit = n
	return a
}

func (a *Agent) WithToolCacheSize(n int) *Agent {
	if n <= 0 {
		n = 20
	}
	a.ToolCacheSize = n
	a.toolResultCache = NewToolResultCache(n)
	return a
}

func AgentRepeatedToolErrorLimitForTest(a *Agent) int {
	return a.repeatedToolErrorLimit
}
func (a *Agent) WithTool(t Tool) *Agent {
	if a.toolsByID == nil {
		a.toolsByID = make(map[string]int)
	}
	if i, exists := a.toolsByID[t.Name()]; exists {
		a.toolList[i] = t
		return a
	}
	a.toolsByID[t.Name()] = len(a.toolList)
	a.toolList = append(a.toolList, t)
	return a
}
func (a *Agent) WithLogWriter(w io.Writer) *Agent { a.LogWriter = w; return a }
func (a *Agent) WithSessionID(id string) *Agent   { a.sessionID = id; return a }
func (a *Agent) SessionIDForTest() string         { return a.sessionID }
func (a *Agent) SetSystemPrompts(p string)        { a.SystemPrompts = p }

func (a *Agent) WithCompaction(s CompactionSettings) *Agent {
	a.compaction = s
	return a
}

func (a *Agent) CompactionSettingsForTest() CompactionSettings {
	return a.compaction
}

func (a *Agent) RunStream(ctx context.Context, userMsg string) <-chan Event {
	ch := make(chan Event, 32)
	go func() {
		defer close(ch)
		defer a.flushLog()
		a.openLogLocked()
		if a.Model.ID == "" {
			a.emit(ctx, ch, Event{Category: EventError, ToolError: "Model not set, call WithModel before RunStream"})
			return
		}
		msgs := preSizedHistory(a, userMsg)
		a.loopWithMsgs(ctx, msgs, ch)
	}()
	return ch
}

func (a *Agent) RunStreamResumedWithSnapshot(ctx context.Context, userMsg string, history []llm.Message) (<-chan Event, <-chan []llm.Message) {
	done := make(chan []llm.Message, 1)
	ch := a.runStreamResumedImpl(ctx, userMsg, history, done)
	return ch, done
}

func (a *Agent) runStreamResumedImpl(ctx context.Context, userMsg string, history []llm.Message, sink chan<- []llm.Message) <-chan Event {
	ch := make(chan Event, 32)
	go func() {
		defer close(ch)
		defer a.flushLog()
		a.openLogLocked()
		if sink != nil {
			defer close(sink)
		}
		if a.Model.ID == "" {
			a.emit(ctx, ch, Event{Category: EventError, ToolError: "Model not set, call WithModel before RunStream"})
			return
		}
		msgs := make([]llm.Message, 0, len(history)+2)
		if len(history) == 0 || history[0].Role != "system" {
			msgs = append(msgs, llm.Message{Role: "system", Content: a.SystemPrompts})
		}
		msgs = append(msgs, history...)
		msgs = append(msgs, llm.Message{Role: "user", Content: userMsg})
		a.loopWithMsgs(ctx, msgs, ch)
		if sink != nil {
			sink <- append([]llm.Message{}, msgs...)
		}
	}()
	return ch
}

func (a *Agent) loopWithMsgs(ctx context.Context, msgs []llm.Message, ch chan<- Event) {
	const maxToolsPerTurn = 6
	emit := a.bindEmit(ch)
	for turn := 0; turn < a.SafetyNet; turn++ {
		a.currentTurn = turn + 1
		a.turnStartAt = time.Now()
		a.turnFirstChunkAt = time.Time{}
		a.currentMsgs = msgs
		userMsgID := fmt.Sprintf("turn-%d", turn+1)
		if last := lastUserMessageID(msgs); last != "" {
			userMsgID = last
		}
		a.logMu.Lock()
		a.writeTurnStartLocked(userMsgID)
		a.logMu.Unlock()

		settings := a.compaction
		settings.MaxContextTokens = a.Model.MaxContextTokens
		compacted, action, err := SelectStrategy(msgs, settings, nil).ActOn(ctx, a, msgs, nil)
		if err != nil {
			a.logMu.Lock()
			a.writeErrorV3Locked("compaction", "", "compact: "+err.Error(), 0, false)
			a.logMu.Unlock()
			a.emit(ctx, ch, Event{Category: EventError, ToolError: "compact: " + err.Error()})
			return
		}
		if action != ActionNone {
			msgs = compacted
			a.currentMsgs = msgs
		}
		step, err := a.strategy.Step(ctx, msgs, emit)
		a.logMu.Lock()
		a.writeTurnResponseEnded(step, a.turnStartAt, a.turnFirstChunkAt)
		a.logMu.Unlock()
		if err != nil {
			return
		}
		if step.Kind == StepFinal {
			return
		}
		toolCalls := step.ToolCalls
		if len(toolCalls) > maxToolsPerTurn {
			a.logMu.Lock()
			a.writeErrorV3Locked("tool_invoke", "", fmt.Sprintf("too many tool calls in one turn (%d > %d), truncating", len(toolCalls), maxToolsPerTurn), 0, false)
			a.logMu.Unlock()
			a.emit(ctx, ch, Event{Category: EventError, ToolError: fmt.Sprintf("too many tool calls in one turn (%d > %d), truncating", len(toolCalls), maxToolsPerTurn)})
			toolCalls = toolCalls[:maxToolsPerTurn]
		}
		msgs = append(msgs, llm.Message{Role: "assistant", Content: step.Content, Reasoning: step.Reasoning, ReasoningSig: step.ReasoningSig, ToolCalls: toolCalls})
		a.currentMsgs = msgs
		var ok bool
		msgs, ok = a.executeTools(ctx, toolCalls, msgs, ch)
		if !ok {
			return
		}
		a.currentMsgs = msgs
		lastFailed := lastFailedToolError(msgs)
		if abortErr := a.strategy.ShouldAbort(msgs, lastFailed); abortErr != nil {
			a.logMu.Lock()
			a.writeErrorV3Locked("tool_invoke", "", abortErr.Error(), 0, false)
			a.logMu.Unlock()
			a.emit(ctx, ch, Event{Category: EventError, ToolError: abortErr.Error()})
			return
		}
	}
	a.logMu.Lock()
	a.writeErrorV3Locked("safety_net", "", fmt.Sprintf("safety net reached (%d turns); agent aborted to prevent infinite loop. Compact or raise the safety net via WithSafetyNet.", a.SafetyNet), 0, false)
	a.logMu.Unlock()
	a.emit(ctx, ch, Event{Category: EventError, ToolError: fmt.Sprintf("safety net reached (%d turns); agent aborted to prevent infinite loop. Compact or raise the safety net via WithSafetyNet.", a.SafetyNet)})
}

func lastUserMessageID(msgs []llm.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].ToolCallID
		}
	}
	return ""
}

func (a *Agent) writeTurnResponseEnded(step Step, startAt, firstChunkAt time.Time) {
	finishReason := step.FinishReason
	if finishReason == "" {
		finishReason = "stop"
	}
	durMS := time.Since(startAt).Milliseconds()
	var ttft *int64
	if !firstChunkAt.IsZero() {
		v := firstChunkAt.Sub(startAt).Milliseconds()
		ttft = &v
	}
	var prompt, completion, total int
	if step.Usage != nil {
		prompt = step.Usage.PromptTokens
		completion = step.Usage.CompletionTokens
		total = step.Usage.TotalTokens
	}
	a.writeTurnResponseLocked(a.Model.ID, a.Model.Vendor, finishReason, durMS, ttft, prompt, completion, total)
	if prompt > 0 {
		a.observedInputTokens = prompt
	}
}

func (a *Agent) bindEmit(ch chan<- Event) func(context.Context, Event) bool {
	return func(ctx context.Context, ev Event) bool {
		if ev.Category == EventThoughtChunk && a.turnFirstChunkAt.IsZero() {
			a.turnFirstChunkAt = time.Now()
		}
		return a.emit(ctx, ch, ev)
	}
}

func (a *Agent) LogEventForTest(ev Event) {
	if a.LogWriter == nil {
		return
	}
	a.logMu.Lock()
	defer a.logMu.Unlock()
	a.logSeq++
	a.writeEvent(a.logSeq, ev)
}

func lastFailedToolError(msgs []llm.Message) string {
	for _, m := range msgs {
		if m.Role == "tool" && strings.HasPrefix(m.Content, "Tool ") && strings.Contains(m.Content, " failed: ") {
			return m.Content
		}
	}
	return ""
}

func (a *Agent) openLogLocked() {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	if a.LogWriter != nil {
		if a.logBuf == nil {
			a.logBuf = bufio.NewWriterSize(a.LogWriter, 4096)
			a.writeHeaderLocked()
			_ = a.logBuf.Flush()
		}
		return
	}
	path, err := defaultSessionLogPath(a.sessionID)
	if err != nil {
		slog.Warn("agent: default session log path failed", "err", err)
		return
	}
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		slog.Warn("agent: session log open failed", "path", path, "err", err)
		return
	}
	a.LogWriter = f
	a.logBuf = bufio.NewWriterSize(f, 4096)
	a.writeHeaderLocked()
	a.logFileOpened = true
	_ = a.logBuf.Flush()
	slog.Debug("agent: session log opened", "path", path)

	v3Path := strings.TrimSuffix(path, ".jsonl") + ".v3.ndjson"
	v3f, v3err := os.OpenFile(v3Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if v3err != nil {
		slog.Debug("agent: v3 log open skipped", "path", v3Path, "err", v3err)
	} else {
		a.v3LogWriter = v3f
		a.v3LogBuf = bufio.NewWriterSize(v3f, 4096)
		slog.Debug("agent: v3 log opened", "path", v3Path)
	}
}

type turnResult struct {
	finishReason llm.FinishReason
	content      string
	reasoning    string
	reasoningSig string
	toolCalls    []llm.ToolCall
	aborted      error
	usage        *llm.Usage
}

func (r turnResult) toAssistantMessage() llm.Message {
	return llm.Message{Role: "assistant", Content: r.content, Reasoning: r.reasoning, ReasoningSig: r.reasoningSig, ToolCalls: r.toolCalls}
}

func toolCallSignature(calls []llm.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	parts := make([]string, len(calls))
	for i, c := range calls {
		parts[i] = c.Function.Name + ":" + c.Function.Arguments
	}
	return strings.Join(parts, "|")
}

func allEqual(sigs []string) bool {
	if len(sigs) == 0 {
		return false
	}
	first := sigs[0]
	for _, s := range sigs[1:] {
		if s != first {
			return false
		}
	}
	return true
}

func allToolCallsEmpty(calls []llm.ToolCall) bool {
	if len(calls) == 0 {
		return true
	}
	for _, c := range calls {
		trimmed := strings.TrimSpace(c.Function.Arguments)
		if trimmed != "" {
			return false
		}
	}
	return true
}

func preSizedHistory(a *Agent, userMsg string) []llm.Message {
	msgs := make([]llm.Message, 2, 2+a.SafetyNet*4)
	msgs[0] = llm.Message{Role: "system", Content: a.SystemPrompts}
	msgs[1] = llm.Message{Role: "user", Content: userMsg}
	return msgs
}

func buildRequest(a *Agent, msgs []llm.Message) *llm.ChatRequest {
	req := &llm.ChatRequest{Model: a.Model.ID, Messages: msgs}
	if len(a.toolList) > 0 {
		req.Tools = make([]llm.ToolDef, 0, len(a.toolList))
		for _, t := range a.toolList {
			req.Tools = append(req.Tools, llm.ToolDef{Type: "function", Function: llm.FunctionDef{Name: t.Name(), Description: t.Description(), Parameters: t.Parameters()}})
		}
	}
	return req
}

func (a *Agent) toolDefsForStrategy() []llm.ToolDef {
	if len(a.toolList) == 0 {
		return nil
	}
	defs := make([]llm.ToolDef, 0, len(a.toolList))
	for _, t := range a.toolList {
		defs = append(defs, llm.ToolDef{Type: "function", Function: llm.FunctionDef{Name: t.Name(), Description: t.Description(), Parameters: t.Parameters()}})
	}
	return defs
}

func (a *Agent) toolNamesForError() []string {
	names := make([]string, 0, len(a.toolList))
	for _, t := range a.toolList {
		names = append(names, t.Name())
	}
	return names
}

func (a *Agent) preflightValidate(tc llm.ToolCall) string {
	tool, ok := a.findTool(tc.Function.Name)
	if !ok {
		return ""
	}
	schema := tool.Parameters()
	if schema == nil {
		return ""
	}
	required, _ := schema["required"].([]string)
	if len(required) == 0 {
		return ""
	}
	var got map[string]any
	if tc.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &got); err != nil {
			return ""
		}
	}
	missing := []string{}
	for _, name := range required {
		v, present := got[name]
		if !present {
			missing = append(missing, name)
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	parts := []string{}
	for _, name := range missing {
		props, _ := schema["properties"].(map[string]any)
		if prop, ok := props[name].(map[string]any); ok {
			if desc, ok := prop["description"].(string); ok {
				parts = append(parts, fmt.Sprintf("%s: %s", name, desc))
				continue
			}
		}
		parts = append(parts, name)
	}
	return fmt.Sprintf("Tool %s: missing required field(s) [%s]. Pass them as a JSON object argument. Example: {\"%s\": \"<value>\"}. Field descriptions: %s",
		tc.Function.Name, strings.Join(missing, ", "), missing[0], strings.Join(parts, "; "))
}

func (a *Agent) executeTools(ctx context.Context, calls []llm.ToolCall, msgs []llm.Message, ch chan<- Event) ([]llm.Message, bool) {
	for i, tc := range calls {
		tool, ok := a.findTool(tc.Function.Name)
		if !ok {
			available := a.toolNamesForError()
			sort.Strings(available)
			errMsg := fmt.Sprintf("Tool %q is not registered. Available tools: [%s]. Pick one of those and call it again.",
				tc.Function.Name, strings.Join(available, ", "))
			a.emit(ctx, ch, Event{Category: EventError, ToolError: errMsg})
			msgs = appendSkippedToolResults(msgs, calls, i, errMsg)
			return msgs, false
		}
		if preflight := a.preflightValidate(tc); preflight != "" {
			a.emit(ctx, ch, Event{Category: EventError, ToolName: tc.Function.Name, ToolError: preflight})
			msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: preflight})
			msgs = appendSkippedToolResults(msgs, calls, i+1, fmt.Sprintf("Tool %s skipped: prior tool %s failed preflight validation", tc.Function.Name, tc.Function.Name))
			return msgs, true
		}
		sig := toolCallDedupKey(tc.Function.Name, tc.Function.Arguments)
		if cached, ok := a.toolResultCache.Get(sig); ok {
			if !a.emit(ctx, ch, Event{Category: EventObserve, ToolName: tc.Function.Name, ToolResult: cached, ToolIntent: extractToolIntent(tc.Function.Arguments), FromCache: true}) {
				return msgs, false
			}
			a.logMu.Lock()
			a.writeToolDedupHitLocked(tc.Function.Name, sig, a.logSeq)
			a.logMu.Unlock()
			slog.Debug("agent: tool dedup hit", "tool", tc.Function.Name, "sig", sig[:8])
			msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: applyOversizedGuard(cached, tc.Function.Arguments, tc.Function.Name)})
			continue
		}
		if !a.emit(ctx, ch, Event{Category: EventTool, ToolName: tc.Function.Name, ToolArgs: tc.Function.Arguments, ToolIntent: extractToolIntent(tc.Function.Arguments)}) {
			return msgs, false
		}
		result, err := tool.Invoke(ctx, tc.Function.Arguments)
		observe := Event{Category: EventObserve, ToolName: tc.Function.Name, ToolResult: result, ToolIntent: extractToolIntent(tc.Function.Arguments)}
		if err != nil {
			observe.ToolError = err.Error()
			if tc.Function.Name == InvalidToolName {
				observe.Category = EventInvalid
			}
		}
		if !a.emit(ctx, ch, observe) {
			return msgs, false
		}
		if err != nil {
			content := fmt.Sprintf("Tool %s failed: %s", tc.Function.Name, err.Error())
			msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: content})
			if tc.Function.Name == InvalidToolName {
				a.emit(ctx, ch, Event{Category: EventInvalid, ToolName: tc.Function.Name, ToolError: err.Error(), ToolArgs: tc.Function.Arguments})
			} else {
				a.emit(ctx, ch, Event{Category: EventError, ToolError: err.Error()})
			}
			continue
		}
		a.toolResultCache.Put(sig, result)
		msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: applyOversizedGuard(result, tc.Function.Arguments, tc.Function.Name)})
	}
	return msgs, true
}

func appendSkippedToolResults(msgs []llm.Message, calls []llm.ToolCall, startIndex int, reason string) []llm.Message {
	for i := startIndex; i < len(calls); i++ {
		tc := calls[i]
		msgs = append(msgs, llm.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    reason,
		})
	}
	return msgs
}

func allSameRecent(s []string) bool {
	if len(s) == 0 {
		return false
	}
	for _, v := range s[1:] {
		if v != s[0] {
			return false
		}
	}
	return true
}

const OversizedResultThreshold = 100_000

func applyOversizedGuard(result, argsJSON, toolName string) string {
	if len(result) <= OversizedResultThreshold {
		return result
	}
	if acceptsLargeOutput(argsJSON) {
		return result
	}
	return fmt.Sprintf(
		"⚠️ OUTPUT WITHHELD: this tool returned %d bytes (~%dk tokens), exceeding the %d-byte threshold. "+
			"Narrow the request: add or tighten 'path', 'glob', 'pattern', or set a limit. "+
			"To accept this cost and see the full output, repeat the same call with `\"accept_large_output\": true`. "+
			"That returns the full result and permanently spends the context budget on it.",
		len(result), len(result)/4000, OversizedResultThreshold)
}

func acceptsLargeOutput(argsJSON string) bool {
	var args struct {
		AcceptLargeOutput *bool `json:"accept_large_output"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return false
	}
	return args.AcceptLargeOutput != nil && *args.AcceptLargeOutput
}

func AgentExecuteToolsForTest(a *Agent, calls []llm.ToolCall, msgs []llm.Message) ([]llm.Message, bool) {
	return a.executeTools(context.Background(), calls, msgs, nil)
}

func AgentExecuteToolsWithChanForTest(a *Agent, calls []llm.ToolCall, msgs []llm.Message) ([]Event, []llm.Message, bool) {
	ch := make(chan Event, len(calls)*4+4)
	updated, ok := a.executeTools(context.Background(), calls, msgs, ch)
	close(ch)
	var events []Event
	for ev := range ch {
		events = append(events, ev)
	}
	return events, updated, ok
}

func (a *Agent) findTool(name string) (Tool, bool) {
	i, ok := a.toolsByID[name]
	if !ok {
		return nil, false
	}
	return a.toolList[i], true
}

func AgentFindToolForTest(a *Agent, name string) (Tool, bool) {
	return a.findTool(name)
}

func (a *Agent) emit(ctx context.Context, ch chan<- Event, ev Event) bool {
	if a.LogWriter != nil {
		a.logSeq++
		a.writeEvent(a.logSeq, ev)
	}
	if ch == nil {
		return ctx.Err() == nil
	}
	select {
	case ch <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

func isCtxErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
