package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/vanpiyp/awp/internal/agent-core/util"
	"github.com/vanpiyp/awp/internal/llm"
)

type toolCallOutcome int

const (
	toolCallContinue toolCallOutcome = iota
	toolCallStopOk
	toolCallStopFail
)

type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]any
	Invoke(ctx context.Context, argsJSON string) (string, error)
}

type EmitKind int

const (
	EmitKindNone EmitKind = iota
	EmitKindError
	EmitKindObserve
	EmitKindInvalid
	EmitKindTool
)

type EmitEvent struct {
	Kind            EmitKind
	Content         string
	ToolName        string
	ToolArgs        string
	ToolResult      string
	ToolError       string
	ToolIntent      string
	FromCache       bool
	ToolCalls       []llm.ToolCall
	Reasoning       string
	Usage           *llm.Usage
	Summary         string
	TokensBefore    int
	TokensAfter     int
	FirstKeptSeq    int
	CompactionModel string
	SafetyCount     int
}

type TurnState interface {
	IncPreflightFailure(name string)
	ResetPreflightFailure(name string)
}

type Emitter interface {
	Emit(ctx context.Context, ev EmitEvent) bool
}

type Runner interface {
	FindTool(name string) (Tool, bool)
	ToolListNames() []string
	PreflightValidate(tc llm.ToolCall) string
	ToolResultCacheGet(sig string) (string, bool)
	ToolResultCachePut(sig, result string)
	LogLock()
	LogUnlock()
	LogSeq() int
	WriteToolDedupHitLocked(name, sig string, seq int)
	Emit(ctx context.Context, ev EmitEvent) bool
	BuildToolErrorEvent(tc llm.ToolCall, errMsg string) EmitEvent
	ExtractToolIntent(argsJSON string) string
	InvalidToolNameFlag() string
	EnsureTurnState() TurnState
}

func ExecuteTools(ctx context.Context, runner Runner, calls []llm.ToolCall, msgs []llm.Message, ts TurnState) ([]llm.Message, bool) {
	updated := msgs
	for i, tc := range calls {
		next, outcome := tryExecuteToolCall(ctx, runner, tc, i, calls, updated, ts)
		if outcome != toolCallContinue {
			return next, outcome == toolCallStopOk
		}
		updated = next
	}
	return updated, true
}

func tryExecuteToolCall(ctx context.Context, runner Runner, tc llm.ToolCall, idx int, calls []llm.ToolCall, msgs []llm.Message, ts TurnState) ([]llm.Message, toolCallOutcome) {
	if msg := emptyArgsMessage(tc); msg != "" {
		ev := EmitEvent{Kind: EmitKindError, ToolName: tc.Function.Name, ToolError: msg}
		runner.Emit(ctx, ev)
		msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: msg})
		return msgs, toolCallContinue
	}
	tool, err := lookupTool(runner, tc.Function.Name)
	if err != nil {
		runner.Emit(ctx, EmitEvent{Kind: EmitKindError, ToolError: err.Error()})
		msgs = appendSkippedToolResults(msgs, calls, idx, err.Error())
		return msgs, toolCallStopFail
	}
	if msg := preflightAndStreak(runner, tc, ts); msg != "" {
		runner.Emit(ctx, EmitEvent{Kind: EmitKindError, ToolName: tc.Function.Name, ToolError: msg})
		msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: msg})
		msgs = appendSkippedToolResults(msgs, calls, idx+1, fmt.Sprintf("Tool %s skipped: prior tool %s failed preflight validation", tc.Function.Name, tc.Function.Name))
		return msgs, toolCallStopOk
	}
	sig := toolCallDedupKey(tc.Function.Name, tc.Function.Arguments)
	if cached, ok := runner.ToolResultCacheGet(sig); ok {
		next, ok := handleDedupHit(ctx, runner, tc, sig, cached, msgs, ts)
		if !ok {
			return next, toolCallStopFail
		}
		return next, toolCallContinue
	}
	next, ok := invokeAndProcess(ctx, runner, tc, tool, sig, msgs, ts)
	if !ok {
		return next, toolCallStopFail
	}
	return next, toolCallContinue
}

func emptyArgsMessage(tc llm.ToolCall) string {
	if strings.TrimSpace(tc.Function.Arguments) == "" {
		return fmt.Sprintf("Tool %s skipped: empty arguments; tool_call was emitted with no arguments, skipping execution", tc.Function.Name)
	}
	return ""
}

func lookupTool(runner Runner, name string) (Tool, error) {
	tool, ok := runner.FindTool(name)
	if ok {
		return tool, nil
	}
	available := runner.ToolListNames()
	sort.Strings(available)
	return nil, fmt.Errorf("Tool %q is not registered. Available tools: [%s]. Pick one of those and call it again.",
		name, strings.Join(available, ", "))
}

func preflightAndStreak(runner Runner, tc llm.ToolCall, ts TurnState) string {
	msg := runner.PreflightValidate(tc)
	if msg != "" && ts != nil {
		ts.IncPreflightFailure(tc.Function.Name)
	}
	return msg
}

func handleDedupHit(ctx context.Context, runner Runner, tc llm.ToolCall, sig, cached string, msgs []llm.Message, ts TurnState) ([]llm.Message, bool) {
	if ts != nil {
		ts.ResetPreflightFailure(tc.Function.Name)
	}
	intent := runner.ExtractToolIntent(tc.Function.Arguments)
	if !runner.Emit(ctx, EmitEvent{Kind: EmitKindObserve, ToolName: tc.Function.Name, ToolResult: cached, ToolIntent: intent, FromCache: true}) {
		return msgs, false
	}
	runner.LogLock()
	runner.WriteToolDedupHitLocked(tc.Function.Name, sig, runner.LogSeq())
	runner.LogUnlock()
	slog.Debug("agent: tool dedup hit", "tool", tc.Function.Name, "sig", sig[:8])
	msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: applyOversizedGuard(cached, tc.Function.Arguments, tc.Function.Name)})
	return msgs, true
}

func invokeAndProcess(ctx context.Context, runner Runner, tc llm.ToolCall, tool Tool, sig string, msgs []llm.Message, ts TurnState) ([]llm.Message, bool) {
	intent := runner.ExtractToolIntent(tc.Function.Arguments)
	if !runner.Emit(ctx, EmitEvent{Kind: EmitKindTool, ToolName: tc.Function.Name, ToolArgs: tc.Function.Arguments, ToolIntent: intent}) {
		return msgs, false
	}
	result, err := tool.Invoke(ctx, tc.Function.Arguments)
	observe := EmitEvent{Kind: EmitKindObserve, ToolName: tc.Function.Name, ToolResult: result, ToolIntent: intent}
	if err != nil {
		observe.ToolError = err.Error()
		if tc.Function.Name == runner.InvalidToolNameFlag() {
			observe.Kind = EmitKindInvalid
		}
	}
	if !runner.Emit(ctx, observe) {
		return msgs, false
	}
	if err != nil {
		msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: fmt.Sprintf("Tool %s failed: %s", tc.Function.Name, err.Error())})
		runner.Emit(ctx, runner.BuildToolErrorEvent(tc, err.Error()))
		return msgs, true
	}
	runner.ToolResultCachePut(sig, result)
	if ts != nil {
		ts.ResetPreflightFailure(tc.Function.Name)
	}
	msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: applyOversizedGuard(result, tc.Function.Arguments, tc.Function.Name)})
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

func AgentExecuteToolsForTest(a Runner, calls []llm.ToolCall, msgs []llm.Message) ([]llm.Message, bool) {
	ts := a.EnsureTurnState()
	return ExecuteTools(context.Background(), a, calls, msgs, ts)
}

func AgentExecuteToolsWithChanForTest(runner Runner, calls []llm.ToolCall, msgs []llm.Message) ([]EmitEvent, []llm.Message, bool) {
	ts := runner.EnsureTurnState()
	captured := &captureEmitter{}
	type execResult struct {
		msgs []llm.Message
		ok   bool
	}
	resCh := make(chan execResult, 1)
	captured.binding = func(_ context.Context, ev EmitEvent) bool {
		captured.events = append(captured.events, ev)
		return true
	}
	updated, ok := ExecuteTools(context.Background(), &forwardingRunner{inner: runner, emitter: captured}, calls, msgs, ts)
	_ = resCh
	return captured.events, updated, ok
}

func AgentTryExecuteToolCallForTest(runner Runner, ctx context.Context, tc llm.ToolCall, idx int, calls []llm.ToolCall, msgs []llm.Message) ([]EmitEvent, []llm.Message, bool) {
	ts := runner.EnsureTurnState()
	captured := &captureEmitter{}
	captured.binding = func(_ context.Context, ev EmitEvent) bool {
		captured.events = append(captured.events, ev)
		return true
	}
	updated, outcome := tryExecuteToolCall(ctx, &forwardingRunner{inner: runner, emitter: captured}, tc, idx, calls, msgs, ts)
	return captured.events, updated, outcome != toolCallStopFail
}

type captureEmitter struct {
	events  []EmitEvent
	binding func(context.Context, EmitEvent) bool
}

func (c *captureEmitter) Emit(ctx context.Context, ev EmitEvent) bool {
	if c.binding != nil {
		return c.binding(ctx, ev)
	}
	c.events = append(c.events, ev)
	return true
}

type forwardingRunner struct {
	inner   Runner
	emitter Emitter
}

func (f *forwardingRunner) FindTool(name string) (Tool, bool) {
	return f.inner.FindTool(name)
}
func (f *forwardingRunner) ToolListNames() []string {
	return f.inner.ToolListNames()
}
func (f *forwardingRunner) PreflightValidate(tc llm.ToolCall) string {
	return f.inner.PreflightValidate(tc)
}
func (f *forwardingRunner) ToolResultCacheGet(sig string) (string, bool) {
	return f.inner.ToolResultCacheGet(sig)
}
func (f *forwardingRunner) ToolResultCachePut(sig, result string) {
	f.inner.ToolResultCachePut(sig, result)
}
func (f *forwardingRunner) LogLock()    { f.inner.LogLock() }
func (f *forwardingRunner) LogUnlock()  { f.inner.LogUnlock() }
func (f *forwardingRunner) LogSeq() int { return f.inner.LogSeq() }
func (f *forwardingRunner) WriteToolDedupHitLocked(name, sig string, seq int) {
	f.inner.WriteToolDedupHitLocked(name, sig, seq)
}
func (f *forwardingRunner) Emit(ctx context.Context, ev EmitEvent) bool {
	return f.emitter.Emit(ctx, ev)
}
func (f *forwardingRunner) BuildToolErrorEvent(tc llm.ToolCall, errMsg string) EmitEvent {
	return f.inner.BuildToolErrorEvent(tc, errMsg)
}
func (f *forwardingRunner) ExtractToolIntent(argsJSON string) string {
	return f.inner.ExtractToolIntent(argsJSON)
}
func (f *forwardingRunner) InvalidToolNameFlag() string {
	return f.inner.InvalidToolNameFlag()
}
func (f *forwardingRunner) EnsureTurnState() TurnState {
	return f.inner.EnsureTurnState()
}

func toolCallDedupKey(name, argsJSON string) string {
	h := util.ToolCallDedupKeyForTest(name, argsJSON)
	return h
}
