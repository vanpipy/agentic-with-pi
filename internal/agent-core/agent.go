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
	"strings"
	"sync"
	"time"

	"github.com/vanpiyp/awp/internal/llm"
)

const InvalidToolName = "invalid"

const PreflightAbortThreshold = 2

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
	preflightFailureStreak map[string]int
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
			for _, tc := range c.Delta.ToolCalls {
				if tc.ID != "" {
					idx := -1
					for i := range result.toolCalls {
						if result.toolCalls[i].ID == tc.ID {
							idx = i
							break
						}
					}
					if idx >= 0 {
						mergeToolCallDelta(&result.toolCalls[idx], tc)
					} else {
						result.toolCalls = append(result.toolCalls, tc)
					}
				} else if n := len(result.toolCalls); n > 0 {
					mergeToolCallDelta(&result.toolCalls[n-1], tc)
				} else {
					result.toolCalls = append(result.toolCalls, tc)
				}
			}
		}
	}
	if ev.Chunk.Usage != nil {
		result.usage = ev.Chunk.Usage
	}
	return true
}

func mergeToolCallDelta(existing *llm.ToolCall, delta llm.ToolCall) {
	if delta.Function.Arguments != "" {
		if isPlaceholderToolArgs(existing.Function.Arguments) {
			existing.Function.Arguments = delta.Function.Arguments
		} else {
			existing.Function.Arguments += delta.Function.Arguments
		}
	}
	if delta.Function.Name != "" && existing.Function.Name == "" {
		existing.Function.Name = delta.Function.Name
	}
}

func isPlaceholderToolArgs(s string) bool {
	trimmed := strings.TrimSpace(s)
	return trimmed == "" || trimmed == "{}"
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
	normalised := NormalizeToolError(lastFailedToolError)
	consecutiveTurns := 0
	i := len(msgs) - 1
	for i >= 0 {
		m := msgs[i]
		if !(m.Role == "tool" && NormalizeToolError(m.Content) == normalised) {
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
		preflightFailureStreak: make(map[string]int),
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

func (a *Agent) ResetForRun() {
	if a.preflightFailureStreak == nil {
		a.preflightFailureStreak = make(map[string]int)
		return
	}
	for k := range a.preflightFailureStreak {
		delete(a.preflightFailureStreak, k)
	}
}

func (a *Agent) PreflightFailureStreakForTest() map[string]int {
	return a.preflightFailureStreak
}

func (a *Agent) ShouldAbort(msgs []llm.Message, lastFailedToolError string) error {
	if err := a.checkPreflightStreak(); err != nil {
		return err
	}
	return a.strategy.ShouldAbort(msgs, lastFailedToolError)
}

func (a *Agent) CompactionSettingsForTest() CompactionSettings {
	return a.compaction
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

func AgentCurrentTurnForTest(a *Agent) int           { return a.currentTurn }
func AgentTurnStartAtForTest(a *Agent) time.Time     { return a.turnStartAt }
func AgentCurrentMsgsForTest(a *Agent) []llm.Message { return a.currentMsgs }

func AccumulateStreamToolCallsForTest(events []llm.StreamEvent) []llm.ToolCall {
	rs := &ReActStrategy{}
	var result turnResult
	var contentBuf, reasoningBuf strings.Builder
	emitNoop := func(_ context.Context, _ Event) bool { return true }
	for _, ev := range events {
		if !rs.processStreamEvent(context.Background(), ev, &result, &contentBuf, &reasoningBuf, emitNoop) {
			break
		}
	}
	return result.toolCalls
}

func NewAgentWithLogBufForTest(w io.Writer) *Agent {
	return &Agent{
		LogWriter: w,
		logBuf:    bufio.NewWriterSize(w, 4096),
	}
}

func NewAgentWithLogWriterForTest(w io.Writer) *Agent {
	return &Agent{
		LogWriter: w,
		logBuf:    bufio.NewWriterSize(w, 4096),
		logMu:     sync.Mutex{},
	}
}

func (a *Agent) SetCoreForTest(c llm.Core) {
	a.core = c
}

func (a *Agent) FlushLogForTest() {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	if a.logBuf != nil {
		_ = a.logBuf.Flush()
	}
}

func (a *Agent) LockLogForTest() {
	a.logMu.Lock()
}

func (a *Agent) UnlockLogForTest() {
	a.logMu.Unlock()
}

func (a *Agent) SetLogFileOpenedForTest(v bool) {
	a.logFileOpened = v
}

func (a *Agent) CurrentParentIDForTest() string {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	return a.currentParentID
}

func (a *Agent) WriteLegacyEventForTest(ev Event) {
	a.logMu.Lock()
	a.logSeq++
	seq := a.logSeq
	a.logMu.Unlock()
	if a.LogWriter == nil {
		return
	}
	a.writeEvent(seq, ev)
	a.FlushLogForTest()
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
