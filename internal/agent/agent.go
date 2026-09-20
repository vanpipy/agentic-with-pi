package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/vanpiyp/awp/internal/llm"
)

type EventCategory int

const (
	EventThoughtStart EventCategory = iota
	EventThoughtChunk
	EventThoughtEnd
	EventTool
	EventObserve
	EventFinalAnswer
	EventError
)

type Event struct {
	Category   EventCategory
	Content    string
	Reasoning  string
	ToolName   string
	ToolArgs   string
	ToolResult string
	ToolError  string
	ToolCalls  []llm.ToolCall
	Usage      *llm.Usage
}

type Tool struct {
	Name        string
	Description string
	Parameters  any
	Execute     func(ctx context.Context, argsJSON string) (string, error)
}

type Agent struct {
	core          llm.Core
	MaxTurns      int
	Model         llm.Model
	SystemPrompts string
	Tools         []Tool
	LogWriter     io.Writer
	logMu         sync.Mutex
	logBuf        *bufio.Writer
	logSeq        int
	compaction    CompactionSettings
	contextWindow int
}

func NewAgent(llmCore llm.Core) *Agent {
	return &Agent{
		core:          llmCore,
		MaxTurns:      20,
		SystemPrompts: "You are a helpful coding assistant",
		compaction: CompactionSettings{
			Enabled:         true,
			ReserveTokens:   16384,
			KeepRecentTurns: 5,
		},
		contextWindow: 200000,
	}
}

func (a *Agent) WithMaxTurns(n int) *Agent {
	if n <= 0 {
		n = 10
	}
	a.MaxTurns = n
	return a
}

func (a *Agent) WithModel(model llm.Model) *Agent { a.Model = model; return a }
func (a *Agent) WithTool(t Tool) *Agent           { a.Tools = append(a.Tools, t); return a }
func (a *Agent) WithLogWriter(w io.Writer) *Agent { a.LogWriter = w; return a }
func (a *Agent) SetSystemPrompts(p string)        { a.SystemPrompts = p }

func (a *Agent) WithCompaction(s CompactionSettings) *Agent {
	a.compaction = s
	return a
}

func (a *Agent) WithContextWindow(tokens int) *Agent {
	a.contextWindow = tokens
	return a
}

func (a *Agent) ContextWindow() int {
	return a.contextWindow
}

func (a *Agent) CompactionSettingsForTest() CompactionSettings {
	return a.compaction
}

func (a *Agent) RunStream(ctx context.Context, userMsg string) <-chan Event {
	ch := make(chan Event, 32)
	a.openLogLocked()
	go func() {
		defer close(ch)
		defer a.flushLog()
		a.loop(ctx, userMsg, ch)
	}()
	return ch
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
	path, err := defaultSessionLogPath()
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
	_ = a.logBuf.Flush()
	slog.Debug("agent: session log opened", "path", path)
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

func (a *Agent) loop(ctx context.Context, userMsg string, ch chan<- Event) {
	if a.Model.ID == "" {
		a.emit(ctx, ch, Event{Category: EventError, ToolError: "Model not set, call WithModel before RunStream"})
		return
	}
	msgs := preSizedHistory(a, userMsg)
	const maxConsecutiveRepeats = 3
	const maxToolsPerTurn = 6
	recentCalls := []string{}
	for turn := 0; turn < a.MaxTurns; turn++ {
		if ShouldCompact(msgs, a.contextWindow, a.compaction) {
			previousSummary := ExtractPreviousSummary(msgs)
			compacted, err := a.compact(ctx, msgs, previousSummary)
			if err != nil {
				a.emit(ctx, ch, Event{Category: EventError, ToolError: "compact: " + err.Error()})
				return
			}
			msgs = compacted
		}
		result, ok := a.runTurn(ctx, msgs, ch)
		if !ok {
			return
		}
		msgs = append(msgs, result.toAssistantMessage())
		if len(result.toolCalls) == 0 {
			return
		}

		toolCalls := result.toolCalls
		if len(toolCalls) > maxToolsPerTurn {
			a.emit(ctx, ch, Event{Category: EventError, ToolError: fmt.Sprintf("too many tool calls in one turn (%d > %d), truncating", len(toolCalls), maxToolsPerTurn)})
			toolCalls = toolCalls[:maxToolsPerTurn]
		}

		signature := toolCallSignature(toolCalls)
		remaining := a.MaxTurns - turn - 1
		if remaining > maxConsecutiveRepeats*2 && len(recentCalls) >= maxConsecutiveRepeats &&
			allEqual(append(recentCalls, signature)) {
			a.emit(ctx, ch, Event{Category: EventError, ToolError: fmt.Sprintf("tool calls repeated %d times, aborting", maxConsecutiveRepeats+1)})
			return
		}
		_ = maxConsecutiveRepeats
		recentCalls = append(recentCalls, signature)
		if len(recentCalls) > maxConsecutiveRepeats {
			recentCalls = recentCalls[1:]
		}

		msgs, ok = a.executeTools(ctx, toolCalls, msgs, ch)
		if !ok {
			return
		}
	}
	a.emit(ctx, ch, Event{Category: EventError, ToolError: fmt.Sprintf("max turns exceeded (%d)", a.MaxTurns)})
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
	msgs := make([]llm.Message, 2, 2+a.MaxTurns*4)
	msgs[0] = llm.Message{Role: "system", Content: a.SystemPrompts}
	msgs[1] = llm.Message{Role: "user", Content: userMsg}
	return msgs
}

func buildRequest(a *Agent, msgs []llm.Message) *llm.ChatRequest {
	req := &llm.ChatRequest{Model: a.Model.ID, Messages: msgs}
	if len(a.Tools) > 0 {
		req.Tools = make([]llm.ToolDef, 0, len(a.Tools))
		for _, t := range a.Tools {
			req.Tools = append(req.Tools, llm.ToolDef{Type: "function", Function: llm.FunctionDef{Name: t.Name, Description: t.Description, Parameters: t.Parameters}})
		}
	}
	return req
}

func (a *Agent) runTurn(ctx context.Context, msgs []llm.Message, ch chan<- Event) (turnResult, bool) {
	if !a.emit(ctx, ch, Event{Category: EventThoughtStart}) {
		return turnResult{}, false
	}
	raw, err := a.core.StreamChat(ctx, buildRequest(a, msgs))
	if err != nil {
		if isCtxErr(err) {
			return turnResult{}, false
		}
		a.emit(ctx, ch, Event{Category: EventError, ToolError: err.Error()})
		a.emit(ctx, ch, Event{Category: EventThoughtEnd})
		return turnResult{}, false
	}
	var result turnResult
	var contentBuf, reasoningBuf strings.Builder
	contentBuf.Grow(2048)
	reasoningBuf.Grow(2048)
	for ev := range raw {
		if ctx.Err() != nil {
			return turnResult{}, false
		}
		if !a.processStreamEvent(ctx, ch, ev, &result, &contentBuf, &reasoningBuf) {
			return turnResult{}, false
		}
	}
	result.content = contentBuf.String()
	result.reasoning = reasoningBuf.String()
	if !a.emit(ctx, ch, Event{
		Category:  EventThoughtEnd,
		Content:   result.content,
		Reasoning: result.reasoning,
		ToolCalls: result.toolCalls,
		Usage:     result.usage,
	}) {
		return turnResult{}, false
	}
	switch {
	case result.finishReason == llm.FinishReasonLength && len(result.toolCalls) > 0:
		a.emit(ctx, ch, Event{Category: EventError, ToolError: "response truncated mid tool call, refusing"})
		return turnResult{}, false
	case result.finishReason == llm.FinishReasonLength && result.content == "":
		a.emit(ctx, ch, Event{Category: EventError, ToolError: "response truncated with no content"})
		return turnResult{}, false
	case result.finishReason == llm.FinishReasonToolUse && len(result.toolCalls) > 0 && allToolCallsEmpty(result.toolCalls):
		a.emit(ctx, ch, Event{Category: EventError, ToolError: "empty tool calls, refusing"})
		return turnResult{}, false

	case result.finishReason == llm.FinishReasonToolUse && len(result.toolCalls) > 0:
	case result.finishReason == llm.FinishReasonToolUse && len(result.toolCalls) == 0:
		a.emit(ctx, ch, Event{Category: EventError, ToolError: "tool_use finish reason with no tool calls"})
		return turnResult{}, false
	case (result.finishReason == llm.FinishReasonStop || (result.finishReason == llm.FinishReasonLength && result.content != "")) && len(result.toolCalls) == 0:
		a.emit(ctx, ch, Event{Category: EventFinalAnswer, Content: result.content, Usage: result.usage})
		return turnResult{}, false
	}
	return result, true
}

func (a *Agent) processStreamEvent(ctx context.Context, ch chan<- Event, ev llm.StreamEvent, result *turnResult, contentBuf, reasoningBuf *strings.Builder) bool {
	if ev.Err != nil {
		if isCtxErr(ev.Err) {
			return false
		}
		a.emit(ctx, ch, Event{Category: EventError, ToolError: ev.Err.Error()})
		a.emit(ctx, ch, Event{Category: EventThoughtEnd})
		return false
	}
	if ev.Chunk == nil {
		return true
	}
	for _, c := range ev.Chunk.Choices {
		if c.Delta.Reasoning != "" {
			reasoningBuf.WriteString(c.Delta.Reasoning)
			if !a.emit(ctx, ch, Event{Category: EventThoughtChunk, Reasoning: c.Delta.Reasoning}) {
				return false
			}
		}
		if c.Delta.ReasoningSig != "" {
			result.reasoningSig = c.Delta.ReasoningSig
		}
		if c.Delta.Content != "" {
			contentBuf.WriteString(c.Delta.Content)
			if !a.emit(ctx, ch, Event{Category: EventThoughtChunk, Content: c.Delta.Content}) {
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

func (a *Agent) executeTools(ctx context.Context, calls []llm.ToolCall, msgs []llm.Message, ch chan<- Event) ([]llm.Message, bool) {
	for _, tc := range calls {
		tool, ok := a.findTool(tc.Function.Name)
		if !ok {
			a.emit(ctx, ch, Event{Category: EventError, ToolError: fmt.Sprintf("unknown tool: %s", tc.Function.Name)})
			return msgs, false
		}
		if !a.emit(ctx, ch, Event{Category: EventTool, ToolName: tc.Function.Name, ToolArgs: tc.Function.Arguments}) {
			return msgs, false
		}
		result, err := tool.Execute(ctx, tc.Function.Arguments)
		observe := Event{Category: EventObserve, ToolName: tc.Function.Name, ToolResult: result}
		if err != nil {
			observe.ToolError = err.Error()
		}
		if !a.emit(ctx, ch, observe) {
			return msgs, false
		}
		if err != nil {
			content := fmt.Sprintf("Tool %s failed: %s", tc.Function.Name, err.Error())
			msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: content})
			a.emit(ctx, ch, Event{Category: EventError, ToolError: err.Error()})
			return msgs, true
		}
		msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: result})
	}
	return msgs, true
}

func (a *Agent) findTool(name string) (Tool, bool) {
	for _, t := range a.Tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

func (a *Agent) emit(ctx context.Context, ch chan<- Event, ev Event) bool {
	if a.LogWriter != nil {
		a.logSeq++
		a.writeEvent(a.logSeq, ev)
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
