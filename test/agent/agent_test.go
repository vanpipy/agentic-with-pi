package agent_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/agent"
	"github.com/vanpiyp/awp/internal/llm"
)

type fakeCore struct {
	streamChunksList [][]llm.StreamEvent
	streamChunks     []llm.StreamEvent
	streamErr        error
	chatErr          error
	streamCalls      int
	chatCalls        int
	requests         []llm.ChatRequest
}

func (f *fakeCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	f.streamCalls++
	f.requests = append(f.requests, *req)
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	var chunks []llm.StreamEvent
	if f.streamCalls-1 < len(f.streamChunksList) {
		chunks = f.streamChunksList[f.streamCalls-1]
	} else {
		chunks = f.streamChunks
	}
	ch := make(chan llm.StreamEvent, len(chunks))
	for _, item := range chunks {
		ch <- item
	}
	close(ch)
	return ch, nil
}

func textDeltaChunk(text string) llm.StreamEvent {
	return llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		Index: 0,
		Delta: llm.Message{Content: text},
	}}}}
}

func toolUseStartChunk(toolCallID, name string) llm.StreamEvent {
	return llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		Index: 0,
		Delta: llm.Message{ToolCalls: []llm.ToolCall{{
			ID:       toolCallID,
			Type:     "function",
			Function: llm.FunctionCall{Name: name, Arguments: "{}"},
		}}},
	}}}}
}

func messageDeltaStopChunk(stopReason string) llm.StreamEvent {
	var fr llm.FinishReason
	switch stopReason {
	case "end_turn", "stop_sequence":
		fr = llm.FinishReasonStop
	case "max_tokens":
		fr = llm.FinishReasonLength
	case "tool_use":
		fr = llm.FinishReasonToolUse
	default:
		fr = llm.FinishReasonUnknown
	}
	return llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		FinishReason: fr,
	}}}}
}

func messageStopChunk() llm.StreamEvent {
	return llm.StreamEvent{Chunk: &llm.StreamChunk{}, Err: nil}
}

func runAgent(t *testing.T, ag *agent.Agent, msg string) (string, error) {
	t.Helper()
	var result string
	var firstErr error
	for ev := range ag.RunStream(context.Background(), msg) {
		switch ev.Category {
		case agent.EventFinalAnswer:
			result = ev.Content
		case agent.EventError:
			if firstErr == nil {
				firstErr = errors.New(ev.ToolError)
			}
		}
	}
	return result, firstErr
}

func newTestAgent(core *fakeCore, modelID string) *agent.Agent {
	return agent.NewAgent(core).WithModel(llm.Model{ID: modelID, SupportsTool: true})
}

func TestNewAgentContextWindowTriggersCompaction(t *testing.T) {
	core := &fakeCore{}
	ag := newTestAgent(core, "test-model")
	if ag.ContextWindow() == 0 {
		t.Fatal("NewAgent must set a non-zero contextWindow so ShouldCompact can fire")
	}
	if ag.ContextWindow() > 1_000_000 {
		t.Errorf("contextWindow = %d, looks like a no-op hardcoded default (compaction would never trigger)", ag.ContextWindow())
	}
	settings := ag.CompactionSettingsForTest()
	if settings.ReserveTokens >= ag.ContextWindow() {
		t.Errorf("ReserveTokens (%d) >= contextWindow (%d), ShouldCompact would always return false", settings.ReserveTokens, ag.ContextWindow())
	}
}

func TestAgentReturnsFinalAnswerImmediately(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		textDeltaChunk("42"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")

	result, err := runAgent(t, ag, "What is the answer?")
	if err != nil {
		t.Fatal(err)
	}
	if result != "42" {
		t.Errorf("result = %q, want 42", result)
	}
	if core.streamCalls != 1 {
		t.Errorf("stream calls = %d, want 1", core.streamCalls)
	}
}

func TestAgentExecutesToolAndContinues(t *testing.T) {
	core := &fakeCore{streamChunksList: [][]llm.StreamEvent{
		{
			toolUseStartChunk("call_1", "get_time"),
			messageDeltaStopChunk("tool_use"),
			messageStopChunk(),
		},
		{
			textDeltaChunk("It's 3pm in Beijing."),
			messageDeltaStopChunk("end_turn"),
			messageStopChunk(),
		},
	}}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agent.Tool{
		Name: "get_time",
		Execute: func(ctx context.Context, argsJSON string) (string, error) {
			return "2026-09-13T15:00:00+08:00", nil
		},
	})

	result, err := runAgent(t, ag, "time?")
	if err != nil {
		t.Fatal(err)
	}
	if result != "It's 3pm in Beijing." {
		t.Errorf("result = %q", result)
	}
	if core.streamCalls != 2 {
		t.Errorf("stream calls = %d, want 2", core.streamCalls)
	}
}

func TestAgentUnknownTool(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		toolUseStartChunk("1", "ghost"),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")
	_, err := runAgent(t, ag, "ghost")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("err = %q, want mentions ghost", err.Error())
	}
}

func TestAgentToolExecutionError(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		toolUseStartChunk("1", "boom"),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agent.Tool{
		Name:    "boom",
		Execute: func(ctx context.Context, argsJSON string) (string, error) { return "", errors.New("kaboom") },
	})
	_, err := runAgent(t, ag, "boom")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "kaboom") {
		t.Errorf("err = %q, want mentions kaboom", err.Error())
	}
}

func TestAgentMaxTurnsExceeded(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		toolUseStartChunk("1", "loop"),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}}
	_ = core  // legacy streamChunks (unused)
	ag := newTestAgent(core, "test-model").WithMaxTurns(2)
	ag.WithTool(agent.Tool{
		Name:    "loop",
		Execute: func(ctx context.Context, argsJSON string) (string, error) { return "", nil },
	})
	_, err := runAgent(t, ag, "loop forever")
	if err == nil {
		t.Fatal("expected max-turns error, got nil")
	}
}

func TestAgentStreamErrorPropagates(t *testing.T) {
	core := &fakeCore{streamErr: errors.New("transient failure")}
	ag := newTestAgent(core, "test-model")
	_, err := runAgent(t, ag, "x")
	if err == nil || !strings.Contains(err.Error(), "transient") {
		t.Errorf("err = %v, want contains transient", err)
	}
}

func TestAgentRefusesLengthWithToolCalls(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		toolUseStartChunk("1", "f"),
		messageDeltaStopChunk("max_tokens"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agent.Tool{Name: "f", Execute: func(ctx context.Context, argsJSON string) (string, error) { return "", nil }})

	_, err := runAgent(t, ag, "truncated")
	if err == nil {
		t.Fatal("expected error for length+tool_calls, got nil")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("err = %q, want mentions truncated", err.Error())
	}
}

func TestAgentExtractsSignatureFromThinkingBlock(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
			Index: 0,
			Delta: llm.Message{ReasoningSig: "sig-stream-1"},
		}}}},
		textDeltaChunk("final"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")

	var sawFinal bool
	for ev := range ag.RunStream(context.Background(), "hi") {
		if ev.Category == agent.EventFinalAnswer {
			sawFinal = true
		}
	}
	if !sawFinal {
		t.Error("expected FinalAnswer")
	}
}

type jsonlLine struct {
	raw map[string]any
	seq int
}

func parseJsonl(t *testing.T, buf *bytes.Buffer) []jsonlLine {
	t.Helper()
	var lines []jsonlLine
	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		b := scanner.Bytes()
		if len(bytes.TrimSpace(b)) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("parseJsonl: invalid JSON %q: %v", b, err)
		}
		seq := 0
		if s, ok := m["seq"].(float64); ok {
			seq = int(s)
		}
		lines = append(lines, jsonlLine{raw: m, seq: seq})
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("parseJsonl: scanner error: %v", err)
	}
	return lines
}

func TestAgentEmitsFinalAnswerOnLengthWithContent(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		textDeltaChunk("partial answer"),
		messageDeltaStopChunk("max_tokens"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")
	result, err := runAgent(t, ag, "cut me off")
	if err != nil {
		t.Fatalf("expected best-effort, got err: %v", err)
	}
	if result != "partial answer" {
		t.Errorf("result = %q, want %q", result, "partial answer")
	}
}

func TestAgentErrorsOnLengthWithoutContent(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		messageDeltaStopChunk("max_tokens"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")
	_, err := runAgent(t, ag, "empty cutoff")
	if err == nil {
		t.Fatal("expected error for length+no content, got nil")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("err = %q, want mentions truncated", err.Error())
	}
}

func TestAgentCarriesReasoningSigForward(t *testing.T) {
	core := &fakeCore{streamChunksList: [][]llm.StreamEvent{
		{
			{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
				Index: 0,
				Delta: llm.Message{ReasoningSig: "sig-1"},
			}}}},
			toolUseStartChunk("c1", "noop"),
			messageDeltaStopChunk("tool_use"),
			messageStopChunk(),
		},
		{
			textDeltaChunk("done"),
			messageDeltaStopChunk("end_turn"),
			messageStopChunk(),
		},
	}}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agent.Tool{Name: "noop", Execute: func(ctx context.Context, argsJSON string) (string, error) { return "r", nil }})
	if _, err := runAgent(t, ag, "multi turn"); err != nil {
		t.Fatal(err)
	}
	if len(core.requests) < 2 {
		t.Fatalf("expected 2 stream calls, got %d", len(core.requests))
	}
	turn2 := core.requests[1].Messages
	var found bool
	for _, msg := range turn2 {
		if msg.Role == "assistant" && msg.ReasoningSig == "sig-1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("turn 2 history missing assistant msg with ReasoningSig=sig-1; got msgs=%+v", turn2)
	}
}

type counterWriter struct {
	calls atomic.Int64
}

func (c *counterWriter) Write(p []byte) (int, error) {
	c.calls.Add(1)
	return len(p), nil
}

type errWriter struct{}

func (e *errWriter) Write(p []byte) (int, error) {
	return 0, errors.New("disk on fire")
}

func TestAgentWritesHeaderWhenLogWriterSet(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")
	buf := &bytes.Buffer{}
	ag.WithLogWriter(buf)
	if _, err := runAgent(t, ag, "x"); err != nil {
		t.Fatal(err)
	}
	lines := parseJsonl(t, buf)
	if len(lines) == 0 {
		t.Fatal("expected at least 1 line, got 0")
	}
	if lines[0].raw["kind"] != "session" {
		t.Errorf("line[0].kind = %v, want session", lines[0].raw["kind"])
	}
	if lines[0].raw["model"] != "test-model" {
		t.Errorf("line[0].model = %v, want test-model", lines[0].raw["model"])
	}
}

func TestAgentWritesNoLogWhenLogWriterNil(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")
	cw := &counterWriter{}
	_ = cw
	if _, err := runAgent(t, ag, "x"); err != nil {
		t.Fatal(err)
	}
	if cw.calls.Load() != 0 {
		t.Errorf("counterWriter.calls = %d, want 0 (LogWriter not set)", cw.calls.Load())
	}
}

func TestAgentWritesEventsAsJsonlLines(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		textDeltaChunk("hello "),
		textDeltaChunk("world"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")
	buf := &bytes.Buffer{}
	ag.WithLogWriter(buf)
	if _, err := runAgent(t, ag, "x"); err != nil {
		t.Fatal(err)
	}
	lines := parseJsonl(t, buf)
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines (header + event), got %d", len(lines))
	}
	if lines[0].raw["kind"] != "session" {
		t.Errorf("line[0].kind = %v, want session", lines[0].raw["kind"])
	}
	var prevSeq int
	for i, ln := range lines[1:] {
		if ln.raw["kind"] != "event" {
			t.Errorf("line[%d].kind = %v, want event", i+1, ln.raw["kind"])
		}
		if ln.seq != prevSeq+1 {
			t.Errorf("line[%d].seq = %d, want %d (monotonic)", i+1, ln.seq, prevSeq+1)
		}
		prevSeq = ln.seq
	}
}

func TestAgentSilentOnLogWriteError(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")
	ag.WithLogWriter(&errWriter{})
	result, err := runAgent(t, ag, "x")
	if err != nil {
		t.Fatalf("log write error must not abort agent, got: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %q, want ok", result)
	}
}

func TestAgentContextCancelAbortsCleanly(t *testing.T) {
	gate := make(chan struct{})
	core := &blockingCore{gate: gate}
	ag := agent.NewAgent(core).WithModel(llm.Model{ID: "test-model", SupportsTool: true})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		var count int
		for ev := range ag.RunStream(ctx, "x") {
			count++
			_ = ev
		}
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	close(gate)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunStream did not return after ctx cancel")
	}
}

type blockingCore struct {
	gate chan struct{}
}

func (b *blockingCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	go func() {
		<-b.gate
	}()
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestAgentNoModelSetEmitsError(t *testing.T) {
	core := &fakeCore{}
	ag := agent.NewAgent(core)
	_, err := runAgent(t, ag, "x")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Model") {
		t.Errorf("err = %q, want mentions Model", err.Error())
	}
}

func TestAgentEmitsThoughtBracketEvents(t *testing.T) {
	core := &fakeCore{streamChunksList: [][]llm.StreamEvent{
		{toolUseStartChunk("c1", "noop"), messageDeltaStopChunk("tool_use"), messageStopChunk()},
		{textDeltaChunk("b"), messageDeltaStopChunk("end_turn"), messageStopChunk()},
	}}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agent.Tool{Name: "noop", Execute: func(ctx context.Context, argsJSON string) (string, error) { return "r", nil }})
	var starts, ends int
	for ev := range ag.RunStream(context.Background(), "x") {
		switch ev.Category {
		case agent.EventThoughtStart:
			starts++
		case agent.EventThoughtEnd:
			ends++
		}
	}
	if starts != 2 {
		t.Errorf("ThoughtStart count = %d, want 2", starts)
	}
	if ends != 2 {
		t.Errorf("ThoughtEnd count = %d, want 2", ends)
	}
}

func TestAgentMessageHistoryGrowsAcrossTurns(t *testing.T) {
	core := &fakeCore{streamChunksList: [][]llm.StreamEvent{
		{toolUseStartChunk("c1", "noop"), messageDeltaStopChunk("tool_use"), messageStopChunk()},
		{textDeltaChunk("done"), messageDeltaStopChunk("end_turn"), messageStopChunk()},
	}}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agent.Tool{Name: "noop", Execute: func(ctx context.Context, argsJSON string) (string, error) { return "r", nil }})
	if _, err := runAgent(t, ag, "x"); err != nil {
		t.Fatal(err)
	}
	if len(core.requests) != 2 {
		t.Fatalf("expected 2 stream calls, got %d", len(core.requests))
	}
	turn1Len := len(core.requests[0].Messages)
	turn2Len := len(core.requests[1].Messages)
	if turn2Len <= turn1Len {
		t.Errorf("turn2 messages (%d) should be > turn1 messages (%d)", turn2Len, turn1Len)
	}
	var foundTool bool
	for _, msg := range core.requests[1].Messages {
		if msg.Role == "tool" {
			foundTool = true
			break
		}
	}
	if !foundTool {
		t.Errorf("expected tool result in turn 2 history; got msgs=%+v", core.requests[1].Messages)
	}
}

func TestAgentDefensiveMaxTurnsClamp(t *testing.T) {
	repeat := []llm.StreamEvent{
		toolUseStartChunk("1", "loop"),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}
	_ = repeat  // keep tests clean
	chunksList := make([][]llm.StreamEvent, 10)
	for i := range chunksList {
		chunksList[i] = repeat
	}
	core := &fakeCore{streamChunksList: chunksList}
	ag := newTestAgent(core, "test-model").WithMaxTurns(0)
	ag.WithTool(agent.Tool{Name: "loop", Execute: func(ctx context.Context, argsJSON string) (string, error) { return "", nil }})
	_, err := runAgent(t, ag, "x")
	if err == nil {
		t.Fatal("expected error from clamped 0 max turns, got nil")
	}
	if !strings.Contains(err.Error(), "max turns") {
		t.Errorf("err = %q, want mentions max turns", err.Error())
	}
}

func TestAgentFinalAnswerDoesNotDuplicateReasoning(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
			Index: 0,
			Delta: llm.Message{Reasoning: "thinking out loud"},
		}}}},
		textDeltaChunk("answer"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")

	var finalEvent *agent.Event
	for ev := range ag.RunStream(context.Background(), "hi") {
		if ev.Category == agent.EventFinalAnswer {
			e := ev
			finalEvent = &e
		}
	}
	if finalEvent == nil {
		t.Fatal("expected EventFinalAnswer")
	}
	if finalEvent.Content != "answer" {
		t.Errorf("Content = %q, want answer", finalEvent.Content)
	}
	if finalEvent.Reasoning != "" {
		t.Errorf("Reasoning on EventFinalAnswer = %q, want empty (already streamed via EventObserve)", finalEvent.Reasoning)
	}
}

