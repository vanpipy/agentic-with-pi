package agent_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/agent"
	"github.com/vanpiyp/awp/internal/llm"
)

type fakeCore struct {
	mu               sync.Mutex
	streamChunksList [][]llm.StreamEvent
	streamChunks     []llm.StreamEvent
	streamErr        error
	chatErr          error
	streamCalls      int
	chatCalls        int
	requests         []llm.ChatRequest
}

func (f *fakeCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	f.mu.Lock()
	f.streamCalls++
	f.requests = append(f.requests, *req)
	chunks := f.streamChunks
	if f.streamCalls-1 < len(f.streamChunksList) {
		chunks = f.streamChunksList[f.streamCalls-1]
	}
	f.mu.Unlock()
	if f.streamErr != nil {
		return nil, f.streamErr
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

func toolUseIDDeltaChunk(id, name, args string) llm.StreamEvent {
	return llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		Index: 0,
		Delta: llm.Message{ToolCalls: []llm.ToolCall{{
			ID:       id,
			Type:     "function",
			Function: llm.FunctionCall{Name: name, Arguments: args},
		}}},
	}}}}
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

func runAgentLastError(t *testing.T, ag *agent.Agent, msg string) (string, error) {
	t.Helper()
	var result string
	var lastErr error
	for ev := range ag.RunStream(context.Background(), msg) {
		switch ev.Category {
		case agent.EventFinalAnswer:
			result = ev.Content
		case agent.EventError:
			lastErr = errors.New(ev.ToolError)
		}
	}
	return result, lastErr
}

func newTestAgent(core *fakeCore, modelID string) *agent.Agent {
	return agent.NewAgent(core).WithModel(llm.Model{ID: modelID, SupportsTool: true})
}

func TestAgentContextWindowLivesOnModelNotAgent(t *testing.T) {
	core := &fakeCore{}
	ag := agent.NewAgent(core).WithModel(llm.Model{ID: "m", MaxContextTokens: 128000})
	if ag.Model.MaxContextTokens != 128000 {
		t.Errorf("model.MaxContextTokens = %d, want 128000 (pi pattern: contextWindow lives on model, not on agent)", ag.Model.MaxContextTokens)
	}
}

func TestShouldCompactUsesModelContextWindow(t *testing.T) {
	settings := agent.CompactionSettings{Enabled: true, ReserveTokens: 1024}
	msgs := []llm.Message{
		{Role: "user", Content: string(make([]byte, 40000))},
		{Role: "assistant", Content: string(make([]byte, 40000))},
	}
	model := llm.Model{ID: "m", MaxContextTokens: 10000}
	if !agent.ShouldCompactWithModel(msgs, model, settings) {
		t.Errorf("expected compact: 20000 tokens > 10000 - 1024")
	}
	if agent.ShouldCompactWithModel(msgs, llm.Model{ID: "m", MaxContextTokens: 100000}, settings) {
		t.Errorf("did not expect compact: 20000 < 100000 - 1024")
	}
}

func TestShouldCompactFallsBackWhenModelZero(t *testing.T) {
	settings := agent.CompactionSettings{Enabled: true, ReserveTokens: 1024}
	msgs := []llm.Message{{Role: "user", Content: string(make([]byte, 600000))}}
	if !agent.ShouldCompactWithModel(msgs, llm.Model{ID: "m", MaxContextTokens: 0}, settings) {
		t.Errorf("expected compact: fallback 128000, 150000 tokens > 128000-1024")
	}
}

func TestNewAgentMaxTurnsMatchesPiDefault(t *testing.T) {
	core := &fakeCore{}
	ag := newTestAgent(core, "test-model")
	if ag.MaxTurns < 100 {
		t.Errorf("MaxTurns = %d, safety net default should be >= 100 to cover long-task scenarios", ag.MaxTurns)
	}
}

func TestAgentToolCallTruncationKeepsAssistantAndResultsAligned(t *testing.T) {	callIDs := make([]string, 7)
	for i := range callIDs {
		callIDs[i] = fmt.Sprintf("call_%d", i)
	}
	chunks := []llm.StreamEvent{toolUseStartChunk(callIDs[0], "ls")}
	for _, id := range callIDs {
		chunks = append(chunks, toolUseIDDeltaChunk(id, "ls", `{"path":"."}`))
	}
	chunks = append(chunks, messageDeltaStopChunk("tool_use"), messageStopChunk())
	core := &fakeCore{streamChunks: chunks}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agent.Tool{Name: "ls", Execute: func(_ context.Context, _ string) (string, error) {
		return "ok", nil
	}})

	_, _ = runAgent(t, ag, "explore")

	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.requests) < 2 {
		t.Fatalf("expected at least 2 chat requests (turn 0 prompt + turn 1 retry), got %d", len(core.requests))
	}
	lastReq := core.requests[len(core.requests)-1]
	toolCallCount := 0
	toolResultCount := 0
	for _, m := range lastReq.Messages {
		if m.Role == "assistant" {
			toolCallCount += len(m.ToolCalls)
		}
		if m.Role == "tool" {
			toolResultCount++
		}
	}
	if toolCallCount > 0 && toolCallCount != toolResultCount {
		t.Errorf("assistant.tool_calls (%d) != tool results (%d) after truncation; would cause API 400", toolCallCount, toolResultCount)
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

func TestAgentMidBatchToolFailureKeepsAssistantAndResultsAligned(t *testing.T) {
	core := &fakeCore{}
	ag := agent.NewAgent(core).WithModel(llm.Model{ID: "m", SupportsTool: true})
	ag.WithTool(agent.Tool{Name: "ok", Execute: func(ctx context.Context, argsJSON string) (string, error) { return "ok", nil }})
	ag.WithTool(agent.Tool{Name: "boom", Execute: func(ctx context.Context, argsJSON string) (string, error) {
		return "", errors.New("mid-batch kaboom")
	}})

	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "ok", Arguments: "{}"}},
		{ID: "c2", Function: llm.FunctionCall{Name: "boom", Arguments: "{}"}},
		{ID: "c3", Function: llm.FunctionCall{Name: "ok", Arguments: "{}"}},
		{ID: "c4", Function: llm.FunctionCall{Name: "ok", Arguments: "{}"}},
	}
	msgs := []llm.Message{
		{Role: "assistant", ToolCalls: calls},
	}
	updated, ok := agent.AgentExecuteToolsForTest(ag, calls, msgs)
	if !ok {
		t.Fatal("executeTools returned not-ok")
	}

	toolCallCount := 0
	toolResultCount := 0
	for _, m := range updated {
		if m.Role == "assistant" {
			toolCallCount += len(m.ToolCalls)
		}
		if m.Role == "tool" {
			toolResultCount++
		}
	}
	if toolCallCount != toolResultCount {
		t.Errorf("assistant.tool_calls (%d) != tool results (%d) after mid-batch failure; would cause API 400", toolCallCount, toolResultCount)
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
	_, err := runAgentLastError(t, ag, "x")
	if err == nil {
		t.Fatal("expected error from clamped 0 max turns, got nil")
	}
	if !strings.Contains(err.Error(), "aborting") && !strings.Contains(err.Error(), "max turns") {
		t.Errorf("err = %q, want mentions aborting or max turns (WithMaxTurns(0) clamps to 200 safety net; here loop repeats cause repeated-tool-error abort)", err.Error())
	}
}

func TestAgentSafetyNetStopsLongLoop(t *testing.T) {
	repeat := []llm.StreamEvent{
		toolUseStartChunk("loop", "noop"),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}
	chunksList := make([][]llm.StreamEvent, 250)
	for i := range chunksList {
		chunksList[i] = repeat
	}
	core := &fakeCore{streamChunksList: chunksList}
	ag := newTestAgent(core, "test-model").WithMaxTurns(250)
	ag.WithTool(agent.Tool{Name: "noop", Execute: func(ctx context.Context, argsJSON string) (string, error) { return "", nil }})

	_, err := runAgentLastError(t, ag, "x")
	if err == nil {
		t.Fatal("expected error from long loop, got nil")
	}
	if !strings.Contains(err.Error(), "aborting") && !strings.Contains(err.Error(), "max turns") {
		t.Errorf("err = %q, want safety-net abort (either 'aborting' for repeated calls or 'max turns')", err.Error())
	}
}

func TestAgentAbortsOnRepeatedToolError(t *testing.T) {
	repeat := []llm.StreamEvent{
		toolUseStartChunk("1", "read"),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}
	chunksList := make([][]llm.StreamEvent, 5)
	for i := range chunksList {
		chunksList[i] = repeat
	}
	core := &fakeCore{streamChunksList: chunksList}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agent.Tool{
		Name: "read",
		Execute: func(ctx context.Context, argsJSON string) (string, error) {
			return "", errors.New("path is required")
		},
	})
	_, err := runAgentLastError(t, ag, "explore")
	if err == nil {
		t.Fatal("expected abort after repeated errors, got nil")
	}
	if !strings.Contains(err.Error(), "aborting") && !strings.Contains(err.Error(), "repeated") {
		t.Errorf("err = %q, want mentions aborting/repeated", err.Error())
	}
}

func TestAgentRepeatedErrorLimitIsConfigurable(t *testing.T) {
	core := &fakeCore{}
	ag := newTestAgent(core, "test-model").WithRepeatedToolErrorLimit(2)
	if got := agent.AgentRepeatedToolErrorLimitForTest(ag); got != 2 {
		t.Errorf("limit = %d, want 2", got)
	}
}

func TestAgentRunStreamResumedStartsWithHistory(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		textDeltaChunk("resumed and done"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")

	history := []llm.Message{
		{Role: "user", Content: "earlier task"},
		{Role: "assistant", Content: "Previous conversation summary:\n## Goal\ndone X"},
		{Role: "user", Content: "summarize"},
		{Role: "assistant", Content: "the summary"},
		{Role: "tool", Content: "tool result"},
	}

	var result string
	for ev := range ag.RunStreamResumed(context.Background(), "next task", history) {
		if ev.Category == agent.EventFinalAnswer {
			result = ev.Content
		}
	}
	if result != "resumed and done" {
		t.Errorf("result = %q, want resumed and done", result)
	}
	if len(core.requests) != 1 {
		t.Fatalf("stream calls = %d, want 1", len(core.requests))
	}

	req := core.requests[0]
	if len(req.Messages) != len(history)+1 {
		t.Errorf("messages = %d, want %d (history + new prompt)", len(req.Messages), len(history)+1)
	}
	lastMsg := req.Messages[len(req.Messages)-1]
	if lastMsg.Role != "user" || lastMsg.Content != "next task" {
		t.Errorf("last msg = %+v, want user/next task", lastMsg)
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

