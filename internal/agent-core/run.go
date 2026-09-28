package agentcore

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/vanpiyp/awp/internal/llm"
)

const emptyPostToolContinuationPrompt = "Please continue."

// MaxEmptyPostToolContinuations mirrors jcode's MAX_EMPTY_POST_TOOL_CONTINUATION_ATTEMPTS
// = 5. The counter is per turn-loop, so genuinely-finished agents still exit promptly.
// Without this, a single empty response can silently end a long-running task.
const MaxEmptyPostToolContinuations = 5

const MaxSingleToolTurnsBeforeNudge = 3

const systemReminderBatchNudge = "<system-reminder>You have made several consecutive single-tool-call turns. If the upcoming tool calls are independent (no data dependency between them), call them in the same turn to save round-trips. Only batch when truly independent.</system-reminder>"

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
	emit := a.bindEmit(ch)
	ts := a.ensureActiveTurnStateForTest()
	for turn := 0; turn < a.SafetyNet; turn++ {
		if ts.consecutiveSingleToolTurns >= MaxSingleToolTurnsBeforeNudge && len(a.toolList) >= 2 {
			slog.Debug("agent: batch nudge injected", "consecutive", ts.consecutiveSingleToolTurns)
			ts.consecutiveSingleToolTurns = 0
			msgs = append(msgs, llm.Message{Role: "user", Content: systemReminderBatchNudge})
			a.emit(ctx, ch, Event{Category: EventUserMessage, Content: systemReminderBatchNudge})
			a.emit(ctx, ch, Event{Category: EventSafetyNudge, Content: "BATCH_NUDGE: consider batching independent tool calls"})
		}
		var ok bool
		var step Step
		msgs, ok, step = a.runOneTurn(ctx, msgs, ch, emit, turn, ts)
		if !ok {
			return
		}
		if step.Kind == StepContinue && len(step.ToolCalls) == 1 {
			ts.consecutiveSingleToolTurns++
		} else {
			ts.consecutiveSingleToolTurns = 0
		}
	}
	a.emitSafetyNetHalt(ctx, ch)
}

func (a *Agent) runOneTurn(ctx context.Context, msgs []llm.Message, ch chan<- Event, emit func(context.Context, Event) bool, turn int, ts *turnState) ([]llm.Message, bool, Step) {
	if repaired, count := RepairMissingToolOutputs(msgs); count > 0 {
		slog.Warn("agent: repaired missing tool outputs before next turn", "count", count)
		msgs = repaired
		a.emit(ctx, ch, Event{Category: EventSafetyRepair, Content: fmt.Sprintf("REPAIR: recovered %d interrupted tool outputs", count), SafetyCount: count})
	}
	msgs = a.initTurnState(msgs, turn)
	a.logTurnStartLocked(msgs)
	var ok bool
	if msgs, ok = a.applyCompaction(ctx, msgs, ch); !ok {
		return msgs, false, Step{}
	}
	var step Step
	step, ok = a.runStrategyStep(ctx, msgs, emit)
	if !ok {
		return msgs, false, step
	}
	emptyPostTool := step.Kind == StepFinal && step.Content == "" && (lastMessageRoleIsTool(msgs) || ts.emptyContinuations > 0)
	if emptyPostTool {
		msgs, ok = a.handleEmptyPostToolContinuation(ctx, msgs, ch, step, ts)
		return msgs, ok, step
	}
	ts.emptyContinuations = 0
	if step.Kind == StepFinal {
		return msgs, false, step
	}
	msgs, ok = a.appendAssistantToolsAndAbort(ctx, msgs, ch, step, ts)
	return msgs, ok, step
}

func (a *Agent) handleEmptyPostToolContinuation(ctx context.Context, msgs []llm.Message, ch chan<- Event, step Step, ts *turnState) ([]llm.Message, bool) {
	if ts.emptyContinuations >= MaxEmptyPostToolContinuations {
		slog.Warn("agent: safety-net reached for empty post-tool continuations", "max", MaxEmptyPostToolContinuations)
		return msgs, false
	}
	ts.emptyContinuations++
	slog.Debug("agent: empty post-tool continuation attempt", "attempt", ts.emptyContinuations, "max", MaxEmptyPostToolContinuations)
	a.emit(ctx, ch, Event{Category: EventSafetyEmptyContinue, Content: fmt.Sprintf("EMPTY_CONTINUE: attempt %d/%d", ts.emptyContinuations, MaxEmptyPostToolContinuations)})
	msgs = append(msgs, llm.Message{Role: "assistant", Content: "", Reasoning: step.Reasoning, ReasoningSig: step.ReasoningSig})
	msgs = append(msgs, llm.Message{Role: "user", Content: emptyPostToolContinuationPrompt})
	a.currentMsgs = msgs
	a.emit(ctx, ch, Event{Category: EventUserMessage, Content: emptyPostToolContinuationPrompt})
	return msgs, true
}

func lastMessageRoleIsTool(msgs []llm.Message) bool {
	return len(msgs) > 0 && msgs[len(msgs)-1].Role == "tool"
}

func (a *Agent) initTurnState(msgs []llm.Message, turn int) []llm.Message {
	a.currentTurn = turn + 1
	a.turnStartAt = time.Now()
	a.turnFirstChunkAt = time.Time{}
	a.currentMsgs = msgs
	return msgs
}

func (a *Agent) logTurnStartLocked(msgs []llm.Message) {
	userMsgID := fmt.Sprintf("turn-%d", a.currentTurn)
	if last := lastUserMessageID(msgs); last != "" {
		userMsgID = last
	}
	a.logMu.Lock()
	a.writeTurnStartLocked(userMsgID)
	a.logMu.Unlock()
}

func (a *Agent) applyCompaction(ctx context.Context, msgs []llm.Message, ch chan<- Event) ([]llm.Message, bool) {
	settings := a.compaction
	settings.MaxContextTokens = a.Model.MaxContextTokens
	compacted, action, err := SelectStrategy(msgs, settings, nil).ActOn(ctx, a, msgs, nil)
	if err != nil {
		a.logMu.Lock()
		a.writeErrorV3Locked("compaction", "", "compact: "+err.Error(), 0, false)
		a.logMu.Unlock()
		a.emit(ctx, ch, Event{Category: EventError, ToolError: "compact: " + err.Error()})
		return msgs, false
	}
	if action != ActionNone {
		msgs = compacted
		a.currentMsgs = msgs
	}
	return msgs, true
}

func (a *Agent) runStrategyStep(ctx context.Context, msgs []llm.Message, emit func(context.Context, Event) bool) (Step, bool) {
	step, err := a.strategy.Step(ctx, msgs, emit)
	a.logMu.Lock()
	a.writeTurnResponseEnded(step, a.turnStartAt, a.turnFirstChunkAt)
	a.logMu.Unlock()
	if err != nil {
		return step, false
	}
	return step, true
}

func (a *Agent) appendAssistantToolsAndAbort(ctx context.Context, msgs []llm.Message, ch chan<- Event, step Step, ts *turnState) ([]llm.Message, bool) {
	const maxToolsPerTurn = 6
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
	msgs, ok = a.executeTools(ctx, toolCalls, msgs, ch, ts)
	if !ok {
		return msgs, false
	}
	a.currentMsgs = msgs
	lastFailed := lastFailedToolError(msgs)
	if abortErr := a.ShouldAbort(msgs, lastFailed); abortErr != nil {
		a.logMu.Lock()
		a.writeErrorV3Locked("tool_invoke", "", abortErr.Error(), 0, false)
		a.logMu.Unlock()
		a.emit(ctx, ch, Event{Category: EventError, ToolError: abortErr.Error()})
		return msgs, false
	}
	return msgs, true
}

func (a *Agent) emitSafetyNetHalt(ctx context.Context, ch chan<- Event) {
	msg := fmt.Sprintf("safety net reached (%d turns); agent aborted to prevent infinite loop. Compact or raise the safety net via WithSafetyNet.", a.SafetyNet)
	a.logMu.Lock()
	a.writeErrorV3Locked("safety_net", "", msg, 0, false)
	a.logMu.Unlock()
	a.emit(ctx, ch, Event{Category: EventError, ToolError: msg})
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

func lastFailedToolError(msgs []llm.Message) string {
	for _, m := range msgs {
		if m.Role != "tool" || !strings.HasPrefix(m.Content, "Tool ") {
			continue
		}
		if strings.Contains(m.Content, " failed: ") ||
			strings.Contains(m.Content, ": missing required field") ||
			strings.Contains(m.Content, " is not registered") {
			return m.Content
		}
	}
	return ""
}

func preSizedHistory(a *Agent, userMsg string) []llm.Message {
	msgs := make([]llm.Message, 2, 2+a.SafetyNet*4)
	msgs[0] = llm.Message{Role: "system", Content: a.SystemPrompts}
	msgs[1] = llm.Message{Role: "user", Content: userMsg}
	return msgs
}

func AgentRunOneTurnForTest(a *Agent, ctx context.Context, msgs []llm.Message, ch chan<- Event, turn int) ([]llm.Message, bool) {
	counter := 0
	return AgentRunOneTurnWithEmptyContinuationCounterForTest(a, ctx, msgs, ch, turn, &counter)
}

func AgentRunOneTurnWithEmptyContinuationCounterForTest(a *Agent, ctx context.Context, msgs []llm.Message, ch chan<- Event, turn int, counter *int) ([]llm.Message, bool) {
	ts := a.ensureActiveTurnStateForTest()
	emit := a.bindEmit(ch)
	msgsOut, ok, _ := a.runOneTurn(ctx, msgs, ch, emit, turn, ts)
	if counter != nil {
		*counter = ts.emptyContinuations
	}
	return msgsOut, ok
}

func AgentRunOneTurnWithStepForTest(a *Agent, ctx context.Context, msgs []llm.Message, ch chan<- Event, turn int) ([]llm.Message, bool, Step) {
	ts := a.ensureActiveTurnStateForTest()
	emit := a.bindEmit(ch)
	return a.runOneTurn(ctx, msgs, ch, emit, turn, ts)
}

func AgentRunOneTurnWithTurnStateForTest(a *Agent, ctx context.Context, msgs []llm.Message, ch chan<- Event, turn int) ([]llm.Message, bool, Step, *turnState) {
	ts := a.ensureActiveTurnStateForTest()
	emit := a.bindEmit(ch)
	msgsOut, ok, step := a.runOneTurn(ctx, msgs, ch, emit, turn, ts)
	return msgsOut, ok, step, ts
}

func AgentLoopWithMsgsForTest(a *Agent, ctx context.Context, msgs []llm.Message, ch chan<- Event) {
	a.loopWithMsgs(ctx, msgs, ch)
}
