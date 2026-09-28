package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/vanpiyp/awp/internal/llm"
)

func (a *Agent) executeTools(ctx context.Context, calls []llm.ToolCall, msgs []llm.Message, ch chan<- Event, ts *turnState) ([]llm.Message, bool) {
	updated := msgs
	for i, tc := range calls {
		next, outcome := a.tryExecuteToolCall(ctx, tc, i, calls, updated, ch, ts)
		if outcome != toolCallContinue {
			return next, outcome == toolCallStopOk
		}
		updated = next
	}
	return updated, true
}

type toolCallOutcome int

const (
	toolCallContinue toolCallOutcome = iota
	toolCallStopOk
	toolCallStopFail
)

func (a *Agent) tryExecuteToolCall(ctx context.Context, tc llm.ToolCall, idx int, calls []llm.ToolCall, msgs []llm.Message, ch chan<- Event, ts *turnState) ([]llm.Message, toolCallOutcome) {
	if msg := emptyArgsMessage(tc); msg != "" {
		a.emit(ctx, ch, Event{Category: EventError, ToolName: tc.Function.Name, ToolError: msg})
		msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: msg})
		return msgs, toolCallContinue
	}
	tool, err := a.lookupTool(tc.Function.Name)
	if err != nil {
		a.emit(ctx, ch, Event{Category: EventError, ToolError: err.Error()})
		msgs = appendSkippedToolResults(msgs, calls, idx, err.Error())
		return msgs, toolCallStopFail
	}
	if msg := a.preflightAndStreak(tc, ts); msg != "" {
		a.emit(ctx, ch, Event{Category: EventError, ToolName: tc.Function.Name, ToolError: msg})
		msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: msg})
		msgs = appendSkippedToolResults(msgs, calls, idx+1, fmt.Sprintf("Tool %s skipped: prior tool %s failed preflight validation", tc.Function.Name, tc.Function.Name))
		return msgs, toolCallStopOk
	}
	sig := toolCallDedupKey(tc.Function.Name, tc.Function.Arguments)
	if cached, ok := a.toolResultCache.Get(sig); ok {
		next, ok := a.handleDedupHit(ctx, tc, sig, cached, msgs, ch, ts)
		if !ok {
			return next, toolCallStopFail
		}
		return next, toolCallContinue
	}
	next, ok := a.invokeAndProcess(ctx, tc, tool, sig, msgs, ch, ts)
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

func (a *Agent) lookupTool(name string) (Tool, error) {
	tool, ok := a.findTool(name)
	if ok {
		return tool, nil
	}
	available := a.toolNamesForError()
	sort.Strings(available)
	return nil, fmt.Errorf("Tool %q is not registered. Available tools: [%s]. Pick one of those and call it again.",
		name, strings.Join(available, ", "))
}

func (a *Agent) preflightAndStreak(tc llm.ToolCall, ts *turnState) string {
	msg := a.preflightValidate(tc)
	if msg != "" && ts != nil {
		ts.preflightFailureStreak[tc.Function.Name]++
	}
	return msg
}

func (a *Agent) handleDedupHit(ctx context.Context, tc llm.ToolCall, sig, cached string, msgs []llm.Message, ch chan<- Event, ts *turnState) ([]llm.Message, bool) {
	if ts != nil {
		delete(ts.preflightFailureStreak, tc.Function.Name)
	}
	if !a.emit(ctx, ch, Event{Category: EventObserve, ToolName: tc.Function.Name, ToolResult: cached, ToolIntent: extractToolIntent(tc.Function.Arguments), FromCache: true}) {
		return msgs, false
	}
	a.logMu.Lock()
	a.writeToolDedupHitLocked(tc.Function.Name, sig, a.logSeq)
	a.logMu.Unlock()
	slog.Debug("agent: tool dedup hit", "tool", tc.Function.Name, "sig", sig[:8])
	msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: applyOversizedGuard(cached, tc.Function.Arguments, tc.Function.Name)})
	return msgs, true
}

func (a *Agent) toolErrorEvent(tc llm.ToolCall, errMsg string) Event {
	if tc.Function.Name == InvalidToolName {
		return Event{Category: EventInvalid, ToolName: tc.Function.Name, ToolError: errMsg, ToolArgs: tc.Function.Arguments}
	}
	return Event{Category: EventError, ToolError: errMsg}
}

func (a *Agent) invokeAndProcess(ctx context.Context, tc llm.ToolCall, tool Tool, sig string, msgs []llm.Message, ch chan<- Event, ts *turnState) ([]llm.Message, bool) {
	if !a.emit(ctx, ch, Event{Category: EventTool, ToolName: tc.Function.Name, ToolArgs: tc.Function.Arguments, ToolIntent: extractToolIntent(tc.Function.Arguments)}) {
		return msgs, false
	}
	intent := extractToolIntent(tc.Function.Arguments)
	result, err := tool.Invoke(ctx, tc.Function.Arguments)
	observe := Event{Category: EventObserve, ToolName: tc.Function.Name, ToolResult: result, ToolIntent: intent}
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
		msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: fmt.Sprintf("Tool %s failed: %s", tc.Function.Name, err.Error())})
		a.emit(ctx, ch, a.toolErrorEvent(tc, err.Error()))
		return msgs, true
	}
	a.toolResultCache.Put(sig, result)
	if ts != nil {
		delete(ts.preflightFailureStreak, tc.Function.Name)
	}
	msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: applyOversizedGuard(result, tc.Function.Arguments, tc.Function.Name)})
	return msgs, true
}

func (a *Agent) toolNamesForError() []string {
	names := make([]string, 0, len(a.toolList))
	for _, t := range a.toolList {
		names = append(names, t.Name())
	}
	return names
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
	ts := a.ensureActiveTurnStateForTest()
	return a.executeTools(context.Background(), calls, msgs, nil, ts)
}

func AgentExecuteToolsWithChanForTest(a *Agent, calls []llm.ToolCall, msgs []llm.Message) ([]Event, []llm.Message, bool) {
	ts := a.ensureActiveTurnStateForTest()
	ch := make(chan Event, len(calls)*4+4)
	updated, ok := a.executeTools(context.Background(), calls, msgs, ch, ts)
	close(ch)
	var events []Event
	for ev := range ch {
		events = append(events, ev)
	}
	return events, updated, ok
}

func AgentTryExecuteToolCallForTest(a *Agent, ctx context.Context, tc llm.ToolCall, idx int, calls []llm.ToolCall, msgs []llm.Message) ([]Event, []llm.Message, bool) {
	ts := a.ensureActiveTurnStateForTest()
	ch := make(chan Event, 8)
	updated, outcome := a.tryExecuteToolCall(ctx, tc, idx, calls, msgs, ch, ts)
	close(ch)
	var events []Event
	for ev := range ch {
		events = append(events, ev)
	}
	return events, updated, outcome != toolCallStopFail
}
