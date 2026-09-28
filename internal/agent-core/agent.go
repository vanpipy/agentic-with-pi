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

	"github.com/vanpiyp/awp/internal/agent-core/compact"
	"github.com/vanpiyp/awp/internal/agent-core/stream"
	"github.com/vanpiyp/awp/internal/agent-core/util"
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
	EventSafetyNudge
	EventSafetyRepair
	EventSafetyEmptyContinue
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

	SafetyCount int
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
	compaction             compact.CompactionSettings
	repeatedToolErrorLimit int
	strategy               Strategy
	currentParentID        string
	currentStreamBuf       *stream.StreamBuffer
	currentToolCallID      string
	ToolCacheSize          int
	toolResultCache        *util.ToolResultCache
	currentTurn            int
	turnStartAt            time.Time
	turnFirstChunkAt       time.Time
	observedInputTokens    int
	currentMsgs            []llm.Message
	logFileOpened          bool
	v3LogWriter            io.Writer
	v3LogBuf               *bufio.Writer
	activeTurnState        *turnState
	activeEmitCh           chan<- Event
	interrupt              *SoftInterrupt
	interruptInit          sync.Once
}

type turnState struct {
	emptyContinuations         int
	consecutiveSingleToolTurns int
	consecutiveFailedToolTurns int
	preflightFailureStreak     map[string]int
}

func NewAgent(llmCore llm.Core) *Agent {
	a := &Agent{
		core:          llmCore,
		SafetyNet:     200,
		SystemPrompts: "You are a helpful coding assistant",
		compaction: compact.CompactionSettings{
			Enabled:         true,
			ReserveTokens:   16384,
			KeepRecentTurns: 5,
		},
		repeatedToolErrorLimit: 3,
		ToolCacheSize:          20,
	}
	a.toolResultCache = util.NewToolResultCache(a.ToolCacheSize)
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
	a.toolResultCache = util.NewToolResultCache(n)
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

func (a *Agent) WithCompaction(s compact.CompactionSettings) *Agent {
	a.compaction = s
	return a
}

func (a *Agent) Apply(ctx context.Context, msgs []llm.Message) ([]llm.Message, bool) {
	if msgs == nil {
		msgs = a.currentMsgs
	}
	return a.applyCompaction(ctx, msgs, nil)
}

func (a *Agent) SetMessages(msgs []llm.Message) { a.currentMsgs = msgs }

func (a *Agent) LLM() llm.Core { return a.core }

func (a *Agent) KeepRecentTurns() int { return a.compaction.KeepRecentTurns }

func (a *Agent) ModelID() string { return a.Model.ID }

func (a *Agent) LogSeq() int { return a.logSeq }

func (a *Agent) WithLogLocked(do func()) {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	do()
}

func (a *Agent) WriteCompactionEntry(summary string, before, after, firstKeptSeq int, model string) {
	a.writeCompactionLocked(Event{
		Category:        EventCompaction,
		Summary:         summary,
		TokensBefore:    before,
		TokensAfter:     after,
		FirstKeptSeq:    firstKeptSeq,
		CompactionModel: model,
	})
}

func (a *Agent) WriteCompactionV3Entry(trigger, detail, strategy string, before, after, firstKeptSeq int, model string, durMS int64) {
	a.writeCompactionV3Locked(trigger, detail, strategy, before, after, firstKeptSeq, model, durMS)
}

func (a *Agent) FlushLogBuf() {
	if a.logBuf != nil {
		_ = a.logBuf.Flush()
	}
}

func (a *Agent) FindTool(name string) (stream.Tool, bool) {
	tool, ok := a.findTool(name)
	if !ok {
		return nil, false
	}
	return tool, true
}

func (a *Agent) ToolListNames() []string {
	names := make([]string, 0, len(a.toolList))
	for _, t := range a.toolList {
		names = append(names, t.Name())
	}
	return names
}

func (a *Agent) PreflightValidate(tc llm.ToolCall) string {
	return a.preflightValidate(tc)
}

func (a *Agent) ToolResultCacheGet(sig string) (string, bool) {
	if a.toolResultCache == nil {
		return "", false
	}
	return a.toolResultCache.Get(sig)
}

func (a *Agent) ToolResultCachePut(sig, result string) {
	if a.toolResultCache == nil {
		return
	}
	a.toolResultCache.Put(sig, result)
}

func (a *Agent) LogLock()   { a.logMu.Lock() }
func (a *Agent) LogUnlock() { a.logMu.Unlock() }

func (a *Agent) WriteToolDedupHitLocked(name, sig string, seq int) {
	a.writeToolDedupHitLocked(name, sig, seq)
}

func (a *Agent) Emit(ctx context.Context, ev stream.EmitEvent) bool {
	return a.emit(ctx, a.activeEmitCh, streamEmitToAgentEvent(ev))
}

func streamEmitToAgentEvent(ev stream.EmitEvent) Event {
	out := Event{
		Category:   emitKindToCategory(ev.Kind),
		Content:    ev.Content,
		ToolName:   ev.ToolName,
		ToolArgs:   ev.ToolArgs,
		ToolResult: ev.ToolResult,
		ToolError:  ev.ToolError,
		ToolIntent: ev.ToolIntent,
		FromCache:  ev.FromCache,
	}
	if len(ev.ToolCalls) > 0 {
		out.ToolCalls = ev.ToolCalls
	}
	if ev.Usage != nil {
		out.Usage = ev.Usage
	}
	if ev.Reasoning != "" {
		out.Reasoning = ev.Reasoning
	}
	if ev.Summary != "" || ev.TokensAfter != 0 || ev.TokensBefore != 0 {
		out.Summary = ev.Summary
		out.TokensBefore = ev.TokensBefore
		out.TokensAfter = ev.TokensAfter
		out.FirstKeptSeq = ev.FirstKeptSeq
		out.CompactionModel = ev.CompactionModel
	}
	if ev.SafetyCount != 0 {
		out.SafetyCount = ev.SafetyCount
	}
	return out
}

func emitKindToCategory(k stream.EmitKind) EventCategory {
	switch k {
	case stream.EmitKindTool:
		return EventTool
	case stream.EmitKindError:
		return EventError
	case stream.EmitKindObserve:
		return EventObserve
	case stream.EmitKindInvalid:
		return EventInvalid
	}
	return EventInvalid
}

func (a *Agent) BuildToolErrorEvent(tc llm.ToolCall, errMsg string) stream.EmitEvent {
	if tc.Function.Name == InvalidToolName {
		return stream.EmitEvent{Kind: stream.EmitKindInvalid, ToolName: tc.Function.Name, ToolError: errMsg, ToolArgs: tc.Function.Arguments}
	}
	return stream.EmitEvent{Kind: stream.EmitKindError, ToolError: errMsg}
}

func (a *Agent) ExtractToolIntent(argsJSON string) string {
	return extractToolIntent(argsJSON)
}

func (a *Agent) InvalidToolNameFlag() string { return InvalidToolName }

func (a *Agent) EnsureTurnState() stream.TurnState {
	ts := a.ensureActiveTurnStateForTest()
	return &turnStateAdapter{ts: ts}
}

type turnStateAdapter struct {
	ts *turnState
}

func (a *turnStateAdapter) IncPreflightFailure(name string) {
	if a.ts == nil {
		return
	}
	if a.ts.preflightFailureStreak == nil {
		a.ts.preflightFailureStreak = make(map[string]int)
	}
	a.ts.preflightFailureStreak[name]++
}

func (a *turnStateAdapter) ResetPreflightFailure(name string) {
	if a.ts == nil {
		return
	}
	delete(a.ts.preflightFailureStreak, name)
}

func (a *Agent) Interrupt() *SoftInterrupt {
	a.interruptInit.Do(func() {
		a.interrupt = NewSoftInterrupt()
		a.core = &interruptAwareCore{inner: a.core, intr: a.interrupt}
		if rs, ok := a.strategy.(*ReActStrategy); ok {
			rs.core = a.core
		}
	})
	return a.interrupt
}

func (a *Agent) ResetForRun() {
	if a.activeTurnState == nil {
		a.activeTurnState = &turnState{preflightFailureStreak: make(map[string]int)}
		return
	}
	for k := range a.activeTurnState.preflightFailureStreak {
		delete(a.activeTurnState.preflightFailureStreak, k)
	}
}

func (a *Agent) PreflightFailureStreakForTest() map[string]int {
	return a.ensureActiveTurnStateForTest().preflightFailureStreak
}

func (a *Agent) ensureActiveTurnStateForTest() *turnState {
	if a.activeTurnState == nil {
		a.activeTurnState = &turnState{preflightFailureStreak: make(map[string]int)}
	}
	return a.activeTurnState
}

func (a *Agent) ShouldAbort(msgs []llm.Message, lastFailedToolError string) error {
	ts := a.ensureActiveTurnStateForTest()
	if err := checkPreflightStreak(ts); err != nil {
		return err
	}
	return a.strategy.ShouldAbort(msgs, lastFailedToolError)
}

func (a *Agent) CompactionSettingsForTest() compact.CompactionSettings {
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

type ProcessStreamEventsForTestResult struct {
	Content      string
	Reasoning    string
	ReasoningSig string
	ToolCalls    []llm.ToolCall
	Emitted      []Event
	Continue     bool
}

func ProcessStreamEventsForTest(events []llm.StreamEvent) ProcessStreamEventsForTestResult {
	rs := &ReActStrategy{}
	var result turnResult
	var contentBuf, reasoningBuf strings.Builder
	var emitted []Event
	emit := func(_ context.Context, ev Event) bool {
		emitted = append(emitted, ev)
		return true
	}
	ok := true
	for _, ev := range events {
		if !rs.processStreamEvent(context.Background(), ev, &result, &contentBuf, &reasoningBuf, emit) {
			ok = false
			break
		}
	}
	return ProcessStreamEventsForTestResult{
		Content:      contentBuf.String(),
		Reasoning:    reasoningBuf.String(),
		ReasoningSig: result.reasoningSig,
		ToolCalls:    result.toolCalls,
		Emitted:      emitted,
		Continue:     ok,
	}
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

func TurnStateForTest() *turnState {
	return &turnState{preflightFailureStreak: make(map[string]int)}
}

func TurnStateEmptyContinuationsForTest(ts *turnState) int {
	return ts.emptyContinuations
}

func TurnStateConsecutiveSingleToolTurnsForTest(ts *turnState) int {
	return ts.consecutiveSingleToolTurns
}

func TurnStateConsecutiveFailedToolTurnsForTest(ts *turnState) int {
	return ts.consecutiveFailedToolTurns
}

func TurnStatePreflightStreakForTest(ts *turnState, name string) int {
	return ts.preflightFailureStreak[name]
}

func TurnStatePreflightStreakMapForTest(ts *turnState) map[string]int {
	return ts.preflightFailureStreak
}

func (ts *turnState) IncrementEmptyContinuationsForTest() {
	ts.emptyContinuations++
}

func (ts *turnState) IncrementConsecutiveSingleToolTurnsForTest() {
	ts.consecutiveSingleToolTurns++
}

func (ts *turnState) SetConsecutiveFailedToolTurnsForTest(v int) {
	ts.consecutiveFailedToolTurns = v
}
