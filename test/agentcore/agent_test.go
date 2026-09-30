package agentcore_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-core/compact"
	"github.com/vanpiyp/awp/internal/agent-core/stream"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/streamtest"
)

type fakeCore struct {
	mu               sync.Mutex
	streamChunksList [][]llm.StreamChunk
	streamChunks     []llm.StreamChunk
	streamEventsList [][]llm.StreamEvent
	streamEvents     []llm.StreamEvent
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
	f.mu.Unlock()
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	if len(f.streamEventsList) > 0 || f.streamEvents != nil {
		var events []llm.StreamEvent
		if len(f.streamEventsList) > 0 {
			idx := f.streamCalls - 1
			if idx >= 0 && idx < len(f.streamEventsList) {
				events = f.streamEventsList[idx]
			}
		} else {
			events = f.streamEvents
		}
		ch := make(chan llm.StreamEvent, len(events))
		for _, ev := range events {
			ch <- ev
		}
		close(ch)
		return ch, nil
	}
	chunks := f.streamChunks
	if f.streamCalls-1 < len(f.streamChunksList) {
		chunks = f.streamChunksList[f.streamCalls-1]
	}
	return streamtest.Chunks(chunks), nil
}

func textDeltaChunk(text string) llm.StreamChunk {
	return llm.StreamChunk{Choices: []llm.StreamChoice{{
		Index: 0,
		Delta: llm.Message{Content: text},
	}}}
}

func toolUseStartChunk(toolCallID, name string) llm.StreamChunk {
	return llm.StreamChunk{Choices: []llm.StreamChoice{{
		Index: 0,
		Delta: llm.Message{ToolCalls: []llm.ToolCall{{
			ID:       toolCallID,
			Type:     "function",
			Function: llm.FunctionCall{Name: name, Arguments: "{}"},
		}}},
	}}}
}

func messageDeltaStopChunk(stopReason string) llm.StreamChunk {
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
	return llm.StreamChunk{Choices: []llm.StreamChoice{{
		FinishReason: fr,
	}}}
}

func messageStopChunk() llm.StreamChunk {
	return llm.StreamChunk{}
}

func toolUseIDDeltaChunk(id, name, args string) llm.StreamChunk {
	return llm.StreamChunk{Choices: []llm.StreamChoice{{
		Index: 0,
		Delta: llm.Message{ToolCalls: []llm.ToolCall{{
			ID:       id,
			Type:     "function",
			Function: llm.FunctionCall{Name: name, Arguments: args},
		}}},
	}}}
}

func runAgent(t *testing.T, ag *agentcore.Agent, msg string) (string, error) {
	t.Helper()
	var result string
	var firstErr error
	for ev := range ag.RunStream(context.Background(), msg) {
		switch ev.Category {
		case agentcore.EventFinalAnswer:
			result = ev.Content
		case agentcore.EventError:
			if firstErr == nil {
				firstErr = errors.New(ev.ToolError)
			}
		}
	}
	return result, firstErr
}

func runAgentLastError(t *testing.T, ag *agentcore.Agent, msg string) (string, error) {
	t.Helper()
	var result string
	var lastErr error
	for ev := range ag.RunStream(context.Background(), msg) {
		switch ev.Category {
		case agentcore.EventFinalAnswer:
			result = ev.Content
		case agentcore.EventError:
			lastErr = errors.New(ev.ToolError)
		}
	}
	if result != "" {
		return result, nil
	}
	return result, lastErr
}

func newTestAgent(core *fakeCore, modelID string) *agentcore.Agent {
	return agentcore.NewAgent(core).WithModel(llm.Model{ID: modelID, SupportsTool: true})
}

func TestAgentContextWindowLivesOnModelNotAgent(t *testing.T) {
	core := &fakeCore{}
	ag := agentcore.NewAgent(core).WithModel(llm.Model{ID: "m", MaxContextTokens: 128000})
	if ag.Model.MaxContextTokens != 128000 {
		t.Errorf("model.MaxContextTokens = %d, want 128000 (pi pattern: contextWindow lives on model, not on agent)", ag.Model.MaxContextTokens)
	}
}

func TestShouldCompactUsesModelContextWindow(t *testing.T) {
	settings := compact.CompactionSettings{Enabled: true, ReserveTokens: 1024}
	msgs := []llm.Message{
		{Role: "user", Content: string(make([]byte, 40000))},
		{Role: "assistant", Content: string(make([]byte, 40000))},
	}
	model := llm.Model{ID: "m", MaxContextTokens: 10000}
	if !compact.ShouldCompactWithModel(msgs, model, settings) {
		t.Errorf("expected compact: 20000 tokens > 10000 - 1024")
	}
	if compact.ShouldCompactWithModel(msgs, llm.Model{ID: "m", MaxContextTokens: 100000}, settings) {
		t.Errorf("did not expect compact: 20000 < 100000 - 1024")
	}
}

func TestShouldCompactFallsBackWhenModelZero(t *testing.T) {
	settings := compact.CompactionSettings{Enabled: true, ReserveTokens: 1024}
	msgs := []llm.Message{{Role: "user", Content: string(make([]byte, 600000))}}
	if !compact.ShouldCompactWithModel(msgs, llm.Model{ID: "m", MaxContextTokens: 0}, settings) {
		t.Errorf("expected compact: fallback 128000, 150000 tokens > 128000-1024")
	}
}

func TestNewAgentSafetyNetMatchesPiDefault(t *testing.T) {
	core := &fakeCore{}
	ag := newTestAgent(core, "test-model")
	if ag.SafetyNet < 100 {
		t.Errorf("SafetyNet = %d, default should be >= 100 (pi has no turn limit; ours is a safety net only)", ag.SafetyNet)
	}
}

func TestAgentToolCallTruncationKeepsAssistantAndResultsAligned(t *testing.T) {
	callIDs := make([]string, 7)
	for i := range callIDs {
		callIDs[i] = fmt.Sprintf("call_%d", i)
	}
	chunks := []llm.StreamChunk{toolUseStartChunk(callIDs[0], "ls")}
	for _, id := range callIDs {
		chunks = append(chunks, toolUseIDDeltaChunk(id, "ls", `{"path":"."}`))
	}
	chunks = append(chunks, messageDeltaStopChunk("tool_use"), messageStopChunk())
	core := &fakeCore{streamChunks: chunks}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agentcore.ToolFunc{N: "ls", Fn: func(_ context.Context, _ string) (string, error) {
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
	core := &fakeCore{streamChunks: []llm.StreamChunk{
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
	core := &fakeCore{streamChunksList: [][]llm.StreamChunk{
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
	ag.WithTool(agentcore.ToolFunc{
		N: "get_time",
		Fn: func(ctx context.Context, argsJSON string) (string, error) {
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
	core := &fakeCore{streamChunks: []llm.StreamChunk{
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
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		toolUseStartChunk("1", "boom"),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agentcore.ToolFunc{
		N:  "boom",
		Fn: func(ctx context.Context, argsJSON string) (string, error) { return "", errors.New("kaboom") },
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
	ag := agentcore.NewAgent(core).WithModel(llm.Model{ID: "m", SupportsTool: true})
	ag.WithTool(agentcore.ToolFunc{N: "ok", Fn: func(ctx context.Context, argsJSON string) (string, error) { return "ok", nil }})
	ag.WithTool(agentcore.ToolFunc{N: "boom", Fn: func(ctx context.Context, argsJSON string) (string, error) {
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
	updated, ok := stream.AgentExecuteToolsForTest(ag, calls, msgs)
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

func TestAgentSafetyNetReached(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		toolUseStartChunk("1", "loop"),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}}
	_ = core // legacy streamChunks (unused)
	ag := newTestAgent(core, "test-model").WithSafetyNet(2)
	ag.WithTool(agentcore.ToolFunc{
		N:  "loop",
		Fn: func(ctx context.Context, argsJSON string) (string, error) { return "", nil },
	})
	_, err := runAgent(t, ag, "loop forever")
	if err == nil {
		t.Fatal("expected safety-net error, got nil")
	}
	if !strings.Contains(err.Error(), "safety net") {
		t.Errorf("err = %q, want mentions safety net", err.Error())
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
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		toolUseStartChunk("1", "f"),
		messageDeltaStopChunk("max_tokens"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agentcore.ToolFunc{N: "f", Fn: func(ctx context.Context, argsJSON string) (string, error) { return "", nil }})

	_, err := runAgent(t, ag, "truncated")
	if err == nil {
		t.Fatal("expected error for length+tool_calls, got nil")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("err = %q, want mentions truncated", err.Error())
	}
}

func TestAgentExtractsSignatureFromThinkingBlock(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		{Choices: []llm.StreamChoice{{
			Index: 0,
			Delta: llm.Message{ReasoningSig: "sig-stream-1"},
		}}},
		textDeltaChunk("final"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")

	var sawFinal bool
	for ev := range ag.RunStream(context.Background(), "hi") {
		if ev.Category == agentcore.EventFinalAnswer {
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
	core := &fakeCore{streamChunks: []llm.StreamChunk{
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
	core := &fakeCore{streamChunks: []llm.StreamChunk{
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
	core := &fakeCore{streamChunksList: [][]llm.StreamChunk{
		{
			{Choices: []llm.StreamChoice{{
				Index: 0,
				Delta: llm.Message{ReasoningSig: "sig-1"},
			}}},
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
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(ctx context.Context, argsJSON string) (string, error) { return "r", nil }})
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
	core := &fakeCore{streamChunks: []llm.StreamChunk{
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
	core := &fakeCore{streamChunks: []llm.StreamChunk{
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
	core := &fakeCore{streamChunks: []llm.StreamChunk{
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
	core := &fakeCore{streamChunks: []llm.StreamChunk{
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
	ag := agentcore.NewAgent(core).WithModel(llm.Model{ID: "test-model", SupportsTool: true})
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
	ag := agentcore.NewAgent(core)
	_, err := runAgent(t, ag, "x")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Model") {
		t.Errorf("err = %q, want mentions Model", err.Error())
	}
}

func TestAgentEmitsThoughtBracketEvents(t *testing.T) {
	core := &fakeCore{streamChunksList: [][]llm.StreamChunk{
		{toolUseStartChunk("c1", "noop"), messageDeltaStopChunk("tool_use"), messageStopChunk()},
		{textDeltaChunk("b"), messageDeltaStopChunk("end_turn"), messageStopChunk()},
	}}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(ctx context.Context, argsJSON string) (string, error) { return "r", nil }})
	var starts, ends int
	for ev := range ag.RunStream(context.Background(), "x") {
		switch ev.Category {
		case agentcore.EventThoughtStart:
			starts++
		case agentcore.EventThoughtEnd:
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
	core := &fakeCore{streamChunksList: [][]llm.StreamChunk{
		{toolUseStartChunk("c1", "noop"), messageDeltaStopChunk("tool_use"), messageStopChunk()},
		{textDeltaChunk("done"), messageDeltaStopChunk("end_turn"), messageStopChunk()},
	}}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(ctx context.Context, argsJSON string) (string, error) { return "r", nil }})
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
	repeat := []llm.StreamChunk{
		toolUseStartChunk("1", "loop"),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}
	_ = repeat // keep tests clean
	chunksList := make([][]llm.StreamChunk, 10)
	for i := range chunksList {
		chunksList[i] = repeat
	}
	core := &fakeCore{streamChunksList: chunksList}
	ag := newTestAgent(core, "test-model").WithSafetyNet(0)
	ag.WithTool(agentcore.ToolFunc{N: "loop", Fn: func(ctx context.Context, argsJSON string) (string, error) { return "", nil }})
	_, err := runAgentLastError(t, ag, "x")
	if err == nil {
		t.Fatal("expected error from clamped 0 max turns, got nil")
	}
	if !strings.Contains(err.Error(), "aborting") && !strings.Contains(err.Error(), "max turns") {
		t.Errorf("err = %q, want mentions aborting or max turns (WithMaxTurns(0) clamps to 200 safety net; here loop repeats cause repeated-tool-error abort)", err.Error())
	}
}

func TestAgentSafetyNetStopsLongLoop(t *testing.T) {
	repeat := []llm.StreamChunk{
		toolUseStartChunk("loop", "noop"),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}
	chunksList := make([][]llm.StreamChunk, 250)
	for i := range chunksList {
		chunksList[i] = repeat
	}
	core := &fakeCore{streamChunksList: chunksList}
	ag := newTestAgent(core, "test-model").WithSafetyNet(250)
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(ctx context.Context, argsJSON string) (string, error) { return "", nil }})

	_, err := runAgentLastError(t, ag, "x")
	if err == nil {
		t.Fatal("expected error from long loop, got nil")
	}
	if !strings.Contains(err.Error(), "aborting") && !strings.Contains(err.Error(), "max turns") {
		t.Errorf("err = %q, want safety-net abort (either 'aborting' for repeated calls or 'max turns')", err.Error())
	}
}

func TestAgentDoesNotAbortOnSingleTransientToolError(t *testing.T) {
	chunks := [][]llm.StreamChunk{
		{toolUseStartChunk("c1", "read"), toolUseIDDeltaChunk("c1", "read", `{}`),
			messageDeltaStopChunk("tool_use"), messageStopChunk()},
		{toolUseStartChunk("c2", "read"), toolUseIDDeltaChunk("c2", "read", `{"path":"AGENTS.md"}`),
			messageDeltaStopChunk("tool_use"), messageStopChunk()},
		{toolUseStartChunk("c3", "read"), toolUseIDDeltaChunk("c3", "read", `{"path":"README.md"}`),
			messageDeltaStopChunk("tool_use"), messageStopChunk()},
		{textDeltaChunk("all done"),
			messageDeltaStopChunk("end_turn"), messageStopChunk()},
	}
	core := &fakeCore{streamChunksList: chunks}
	ag := newTestAgent(core, "test-model").WithSafetyNet(50)
	ag.WithTool(agentcore.ToolFunc{N: "read", Fn: func(ctx context.Context, argsJSON string) (string, error) {
		if argsJSON == "{}" {
			return "", errors.New("path is required")
		}
		return "ok", nil
	}})

	result, err := runAgentLastError(t, ag, "explore")
	if err != nil {
		t.Fatalf("expected recovery after one transient error, got %q", err.Error())
	}
	if result != "all done" {
		t.Errorf("result = %q, want 'all done'", result)
	}
}

func TestAgentWithholdsOversizedToolResult(t *testing.T) {
	huge := strings.Repeat("x", 200_000)
	chunks := [][]llm.StreamChunk{
		{toolUseStartChunk("c1", "dump"), toolUseIDDeltaChunk("c1", "dump", `{"intent":"test"}`),
			messageDeltaStopChunk("tool_use"), messageStopChunk()},
		{textDeltaChunk("read refusal"),
			messageDeltaStopChunk("end_turn"), messageStopChunk()},
	}
	core := &fakeCore{streamChunksList: chunks}
	ag := newTestAgent(core, "test-model").WithSafetyNet(50)
	ag.WithTool(agentcore.ToolFunc{N: "dump", Fn: func(_ context.Context, _ string) (string, error) {
		return huge, nil
	}})

	result, err := runAgentLastError(t, ag, "give me everything")
	if err != nil {
		t.Fatalf("expected final answer after withheld result, got %q", err.Error())
	}
	if result != "read refusal" {
		t.Errorf("result = %q, want 'read refusal'", result)
	}

	msgs := core.requests[len(core.requests)-1].Messages
	var sawRefusal bool
	for _, m := range msgs {
		if m.Role == "tool" && strings.Contains(m.Content, "OUTPUT WITHHELD") {
			sawRefusal = true
			if len(m.Content) >= len(huge) {
				t.Errorf("refusal should be much smaller than raw result; refusal=%d bytes, raw=%d", len(m.Content), len(huge))
			}
		}
	}
	if !sawRefusal {
		t.Errorf("expected tool result to be replaced with WITHHELD refusal")
	}
}

func TestAgentAcceptLargeOutputOverridesWithhold(t *testing.T) {
	huge := strings.Repeat("y", 200_000)
	chunks := [][]llm.StreamChunk{
		{toolUseStartChunk("c1", "dump"), toolUseIDDeltaChunk("c1", "dump", `{"accept_large_output":true,"intent":"test"}`),
			messageDeltaStopChunk("tool_use"), messageStopChunk()},
		{textDeltaChunk("got it"),
			messageDeltaStopChunk("end_turn"), messageStopChunk()},
	}
	core := &fakeCore{streamChunksList: chunks}
	ag := newTestAgent(core, "test-model").WithSafetyNet(50)
	ag.WithTool(agentcore.ToolFunc{N: "dump", Fn: func(_ context.Context, _ string) (string, error) {
		return huge, nil
	}})

	result, err := runAgentLastError(t, ag, "give me everything")
	if err != nil {
		t.Fatalf("expected final answer, got %q", err.Error())
	}
	if result != "got it" {
		t.Errorf("result = %q, want 'got it'", result)
	}

	msgs := core.requests[len(core.requests)-1].Messages
	var sawRaw bool
	for _, m := range msgs {
		if m.Role == "tool" && strings.Contains(m.Content, huge[:500]) {
			sawRaw = true
		}
	}
	if !sawRaw {
		t.Errorf("expected raw oversized result to pass through when accept_large_output=true")
	}
}

func TestAgentAbortsOnRepeatedToolError(t *testing.T) {
	repeat := []llm.StreamChunk{
		toolUseStartChunk("1", "read"),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}
	chunksList := make([][]llm.StreamChunk, 5)
	for i := range chunksList {
		chunksList[i] = repeat
	}
	core := &fakeCore{streamChunksList: chunksList}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agentcore.ToolFunc{
		N: "read",
		Fn: func(ctx context.Context, argsJSON string) (string, error) {
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
	if got := agentcore.AgentRepeatedToolErrorLimitForTest(ag); got != 2 {
		t.Errorf("limit = %d, want 2", got)
	}
}

func TestAgentRunStreamResumedStartsWithHistory(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
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
	ch, _ := ag.RunStreamResumedWithSnapshot(context.Background(), "next task", history)
	for ev := range ch {
		if ev.Category == agentcore.EventFinalAnswer {
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
	if len(req.Messages) != len(history)+2 {
		t.Errorf("messages = %d, want %d (system + history + new prompt)", len(req.Messages), len(history)+2)
	}
	lastMsg := req.Messages[len(req.Messages)-1]
	if lastMsg.Role != "user" || lastMsg.Content != "next task" {
		t.Errorf("last msg = %+v, want user/next task", lastMsg)
	}
}

func TestAgentFinalAnswerDoesNotDuplicateReasoning(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		{Choices: []llm.StreamChoice{{
			Index: 0,
			Delta: llm.Message{Reasoning: "thinking out loud"},
		}}},
		textDeltaChunk("answer"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")

	var finalEvent *agentcore.Event
	for ev := range ag.RunStream(context.Background(), "hi") {
		if ev.Category == agentcore.EventFinalAnswer {
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

func TestAgentFindToolByName(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m", SupportsTool: true})
	ag.WithTool(agentcore.ToolFunc{N: "alpha", Fn: func(context.Context, string) (string, error) { return "a", nil }})
	ag.WithTool(agentcore.ToolFunc{N: "beta", Fn: func(context.Context, string) (string, error) { return "b", nil }})

	got, ok := agentcore.AgentFindToolForTest(ag, "beta")
	if !ok {
		t.Fatal("findTool(beta) = !ok, want true")
	}
	if got.Name() != "beta" {
		t.Errorf("got Name = %q, want beta", got.Name())
	}

	if _, ok := agentcore.AgentFindToolForTest(ag, "missing"); ok {
		t.Error("findTool(missing) = ok, want false")
	}
}

func TestAgentFindTool_DoubleRegisterOverwrites(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m", SupportsTool: true})
	ag.WithTool(agentcore.ToolFunc{N: "alpha", Fn: func(context.Context, string) (string, error) { return "first", nil }})
	ag.WithTool(agentcore.ToolFunc{N: "alpha", Fn: func(context.Context, string) (string, error) { return "second", nil }})

	got, ok := agentcore.AgentFindToolForTest(ag, "alpha")
	if !ok {
		t.Fatal("findTool(alpha) = !ok, want true")
	}
	out, err := got.Invoke(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "second" {
		t.Errorf("Execute output = %q, want second (overwrite)", out)
	}
}

func TestReActStrategyStep_ContinuesOnToolCalls(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		toolUseStartChunk("c1", "ls"),
		textDeltaChunk("thinking..."),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}}
	strat := agentcore.NewReActStrategy(core, llm.Model{ID: "m", SupportsTool: true}, func() []llm.ToolDef { return nil }, nil)
	noop := func(context.Context, agentcore.Event) bool { return true }

	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "explore"}}, noop)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if step.Kind != agentcore.StepContinue {
		t.Errorf("Kind = %v, want StepContinue", step.Kind)
	}
	if len(step.ToolCalls) != 1 {
		t.Errorf("len(ToolCalls) = %d, want 1", len(step.ToolCalls))
	}
	if step.ToolCalls[0].Function.Name != "ls" {
		t.Errorf("ToolCalls[0].Name = %q, want ls", step.ToolCalls[0].Function.Name)
	}
	if step.Content != "thinking..." {
		t.Errorf("Content = %q, want %q", step.Content, "thinking...")
	}
}

func TestReActStrategyStep_FinalAnswer(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("the answer"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	strat := agentcore.NewReActStrategy(core, llm.Model{ID: "m", SupportsTool: true}, func() []llm.ToolDef { return nil }, nil)
	noop := func(context.Context, agentcore.Event) bool { return true }

	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "?"}}, noop)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if step.Kind != agentcore.StepFinal {
		t.Errorf("Kind = %v, want StepFinal", step.Kind)
	}
	if step.Content != "the answer" {
		t.Errorf("Content = %q, want %q", step.Content, "the answer")
	}
	if len(step.ToolCalls) != 0 {
		t.Errorf("len(ToolCalls) = %d, want 0", len(step.ToolCalls))
	}
}

func TestReActStrategyShouldAbort_RepeatedCalls(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m", SupportsTool: true}, func() []llm.ToolDef { return nil }, nil)
	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"."}`}}
	msgs := []llm.Message{
		{Role: "user", Content: "explore"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
	}
	if err := strat.ShouldAbort(msgs, ""); err == nil {
		t.Error("ShouldAbort returned nil, want non-nil for repeated identical tool calls")
	}
}

func TestReActStrategyShouldAbort_RepeatedErrors(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m", SupportsTool: true}, func() []llm.ToolDef { return nil }, nil)
	errMsg := "Tool ls failed: kaboom"
	msgs := []llm.Message{
		{Role: "user", Content: "ls"},
		{Role: "tool", Content: errMsg},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Function: llm.FunctionCall{Name: "ls", Arguments: `{}`}}}},
		{Role: "tool", Content: errMsg},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c2", Function: llm.FunctionCall{Name: "ls", Arguments: `{}`}}}},
		{Role: "tool", Content: errMsg},
	}
	if err := strat.ShouldAbort(msgs, errMsg); err == nil {
		t.Error("ShouldAbort returned nil, want non-nil for repeated identical errors")
	}
}

func TestReActStrategyShouldAbort_NormalHistory(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m", SupportsTool: true}, func() []llm.ToolDef { return nil }, nil)
	msgs := []llm.Message{
		{Role: "user", Content: "explore"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"."}`}}}},
		{Role: "tool", ToolCallID: "c1", Content: "a.txt\nb.txt"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c2", Function: llm.FunctionCall{Name: "grep", Arguments: `{"pattern":"x"}`}}}},
		{Role: "tool", ToolCallID: "c2", Content: "match"},
	}
	if err := strat.ShouldAbort(msgs, ""); err != nil {
		t.Errorf("ShouldAbort returned %v, want nil for varied history", err)
	}
}

type runToolArgs struct {
	Name string `json:"name"`
}

func TestRunTool_Success(t *testing.T) {
	handler := func(_ context.Context, a runToolArgs) (string, error) {
		return "hi " + a.Name, nil
	}
	out, err := agentcore.RunTool(context.Background(), `{"name":"alice","intent":"x"}`, handler)
	if err != nil {
		t.Fatalf("RunTool: %v", err)
	}
	if out != "hi alice" {
		t.Errorf("out = %q, want %q", out, "hi alice")
	}
}

func TestRunTool_MissingIntent(t *testing.T) {
	handler := func(_ context.Context, a runToolArgs) (string, error) { return "ok", nil }
	out, err := agentcore.RunTool(context.Background(), `{"name":"alice"}`, handler)
	if err != nil {
		t.Fatalf("missing intent should NOT block the call, got err: %v", err)
	}
	if out != "ok" {
		t.Errorf("out = %q, want ok", out)
	}
}

func TestRunTool_BadJSON(t *testing.T) {
	handler := func(_ context.Context, a runToolArgs) (string, error) { return "", nil }
	_, err := agentcore.RunTool(context.Background(), `not json`, handler)
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}

func TestToolFunc_Adapter(t *testing.T) {
	tf := agentcore.ToolFunc{
		N:  "test",
		D:  "test tool",
		P:  map[string]any{"type": "object"},
		Fn: func(_ context.Context, _ string) (string, error) { return "result", nil },
	}
	if tf.Name() != "test" {
		t.Errorf("Name() = %q, want test", tf.Name())
	}
	if tf.Description() != "test tool" {
		t.Errorf("Description() = %q, want test tool", tf.Description())
	}
	if tf.Parameters()["type"] != "object" {
		t.Errorf("Parameters() type = %v, want object", tf.Parameters()["type"])
	}
	out, err := tf.Invoke(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out != "result" {
		t.Errorf("out = %q, want result", out)
	}
}

var _ agentcore.Tool = agentcore.ToolFunc{N: "x", Fn: func(context.Context, string) (string, error) { return "", nil }}

func TestAgentCompactionPersistsToSessionLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "session.jsonl")

	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("the user's session log file was getting silently deleted by an os.Remove defer; fix: remove the defer; also stop server-side double writes that caused byte-level race"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model").
		WithModel(llm.Model{ID: "test-model", MaxContextTokens: 10}).
		WithCompaction(compact.CompactionSettings{
			Enabled:         true,
			ReserveTokens:   1,
			KeepRecentTurns: 0,
		})
	ag.LogWriter = mustOpenWriter(t, logPath)
	defer ag.LogWriter.(*os.File).Close()

	prompt := "this is a long-running session and the user has been complaining about os.Remove silently deleting their JSONL files. The fix is to remove the defer. Also remove server-side session writes to fix byte-level race"
	if _, err := runAgent(t, ag, prompt); err != nil {
		t.Fatal(err)
	}

	lines := readJSONLFile(t, logPath)
	var compaction map[string]any
	for _, ln := range lines {
		if ln["kind"] == "compaction" {
			compaction = ln
			break
		}
	}
	if compaction == nil {
		t.Fatalf("no compaction entry written to session log; lines:\n%s", mustDumpLines(lines))
	}
	if s, _ := compaction["summary"].(string); s == "" {
		t.Errorf("compaction.summary is empty, got: %v", compaction)
	}
	tb, _ := compaction["tokens_before"].(float64)
	ta, _ := compaction["tokens_after"].(float64)
	if tb <= 0 {
		t.Errorf("compaction.tokens_before = %v, want > 0", compaction["tokens_before"])
	}
	if ta <= 0 {
		t.Errorf("compaction.tokens_after = %v, want > 0", compaction["tokens_after"])
	}
	if ta >= tb*2 {
		t.Errorf("compaction.tokens_after (%v) much larger than tokens_before (%v); compact should not balloon", ta, tb)
	}
	if model, _ := compaction["model"].(string); model != "test-model" {
		t.Errorf("compaction.model = %q, want test-model", model)
	}
}

func mustOpenWriter(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func readJSONLFile(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid JSON %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func mustDumpLines(lines []map[string]any) string {
	var b strings.Builder
	for i, l := range lines {
		b.WriteString(fmt.Sprintf("L%d: kind=%v seq=%v\n", i, l["kind"], l["seq"]))
	}
	return b.String()
}

func TestRunOneTurnInitializesState(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("done"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	ch := make(chan agentcore.Event, 32)
	msgs := []llm.Message{{Role: "user", Content: "hi"}}

	before := time.Now()
	msgsOut, ok := agentcore.AgentRunOneTurnForTest(ag, context.Background(), msgs, ch, 0)
	after := time.Now()

	if ok {
		t.Errorf("runOneTurn ok = true, want false (StepFinal exits the loop)")
	}
	if len(msgsOut) != 1 {
		t.Fatalf("msgs len = %d, want 1 (StepFinal returns before the assistant message is appended)", len(msgsOut))
	}
	if got := agentcore.AgentCurrentTurnForTest(ag); got != 1 {
		t.Errorf("a.currentTurn = %d, want 1 (turn 0 → +1)", got)
	}
	ts := agentcore.AgentTurnStartAtForTest(ag)
	if ts.IsZero() {
		t.Fatalf("a.turnStartAt is zero; initTurnState did not set it")
	}
	if ts.Before(before) || ts.After(after) {
		t.Errorf("a.turnStartAt = %v, want in [%v, %v]", ts, before, after)
	}
}

func TestRunOneTurnAppliesCompaction(t *testing.T) {
	core := &fakeCore{streamChunksList: [][]llm.StreamChunk{
		{textDeltaChunk("summary"), messageDeltaStopChunk("end_turn"), messageStopChunk()},
		{textDeltaChunk("done"), messageDeltaStopChunk("end_turn"), messageStopChunk()},
	}}
	// MaxContextTokens=10, used ~12 tokens -> 90% threshold fires compaction.
	ag := newTestAgent(core, "m").WithModel(llm.Model{ID: "m", MaxContextTokens: 10})
	ch := make(chan agentcore.Event, 32)
	msgs := []llm.Message{{Role: "system", Content: "sys"}}
	for i := 0; i < 7; i++ {
		msgs = append(msgs, llm.Message{Role: "user", Content: fmt.Sprintf("u%d-padding", i)})
	}

	_, _ = agentcore.AgentRunOneTurnForTest(ag, context.Background(), msgs, ch, 0)

	if core.streamCalls < 2 {
		t.Errorf("StreamChat calls = %d, want >=2 (compaction summary + strategy step)", core.streamCalls)
	}
	finalMsgs := agentcore.AgentCurrentMsgsForTest(ag)
	if len(finalMsgs) == 0 {
		t.Fatal("a.currentMsgs is empty after runOneTurn")
	}
}

func TestRunOneTurnExecutesStrategyStep(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("done"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	ch := make(chan agentcore.Event, 32)
	msgs := []llm.Message{{Role: "user", Content: "hi"}}

	msgsOut, ok := agentcore.AgentRunOneTurnForTest(ag, context.Background(), msgs, ch, 0)
	if ok {
		t.Errorf("runOneTurn ok = true, want false (StepFinal exits the loop)")
	}
	if core.streamCalls != 1 {
		t.Errorf("StreamChat calls = %d, want 1 (default settings skip compaction; only strategy step runs)", core.streamCalls)
	}
	if len(msgsOut) != 1 {
		t.Errorf("msgs len = %d, want 1 (StepFinal returns before the assistant message is appended)", len(msgsOut))
	}
	if got := agentcore.AgentCurrentMsgsForTest(ag); len(got) != 1 {
		t.Errorf("a.currentMsgs len = %d, want 1 (StepFinal path)", len(got))
	}
}

func TestRunOneTurnExecutesTools(t *testing.T) {
	var toolCalls atomic.Int32
	core := &fakeCore{streamChunksList: [][]llm.StreamChunk{
		{
			toolUseStartChunk("c1", "noop"),
			toolUseIDDeltaChunk("c1", "noop", `{"intent":"x"}`),
			messageDeltaStopChunk("tool_use"),
			messageStopChunk(),
		},
		{
			textDeltaChunk("done"),
			messageDeltaStopChunk("end_turn"),
			messageStopChunk(),
		},
	}}
	ag := newTestAgent(core, "m")
	ag.WithTool(agentcore.ToolFunc{
		N: "noop",
		Fn: func(_ context.Context, _ string) (string, error) {
			toolCalls.Add(1)
			return "ok", nil
		},
	})
	ch := make(chan agentcore.Event, 32)
	msgs := []llm.Message{{Role: "user", Content: "go"}}

	_, ok := agentcore.AgentRunOneTurnForTest(ag, context.Background(), msgs, ch, 0)
	if !ok {
		t.Fatal("runOneTurn ok = false (StepContinue with tool should keep the loop alive)")
	}
	if got := toolCalls.Load(); got != 1 {
		t.Errorf("tool invoked %d times, want 1", got)
	}
	finalMsgs := agentcore.AgentCurrentMsgsForTest(ag)
	var sawToolResult bool
	for _, m := range finalMsgs {
		if m.Role == "tool" && m.ToolCallID == "c1" && strings.Contains(m.Content, "ok") {
			sawToolResult = true
		}
	}
	if !sawToolResult {
		t.Errorf("expected tool result message in currentMsgs; got %+v", finalMsgs)
	}
}

func TestLoopWithMsgsSafetyNetHalts(t *testing.T) {
	toolChunks := []llm.StreamChunk{
		toolUseStartChunk("c1", "noop"),
		toolUseIDDeltaChunk("c1", "noop", `{"intent":"x"}`),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}
	core := &fakeCore{streamChunksList: [][]llm.StreamChunk{toolChunks, toolChunks, toolChunks}}
	ag := newTestAgent(core, "m").WithSafetyNet(2)
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(_ context.Context, _ string) (string, error) { return "ok", nil }})
	ch := make(chan agentcore.Event, 32)
	msgs := []llm.Message{{Role: "user", Content: "go"}}

	agentcore.AgentLoopWithMsgsForTest(ag, context.Background(), msgs, ch)
	close(ch)

	var sawSafetyNet bool
	for ev := range ch {
		if ev.Category == agentcore.EventError && strings.Contains(ev.ToolError, "safety net") {
			sawSafetyNet = true
		}
	}
	if !sawSafetyNet {
		t.Errorf("expected safety_net error event on the channel, did not see one")
	}
	if core.streamCalls != 2 {
		t.Errorf("StreamChat calls = %d, want 2 (SafetyNet=2 must halt the loop after the second turn)", core.streamCalls)
	}
}

func TestRunOneTurnEmptyStepFinalWithToolMessageRetries(t *testing.T) {
	emptyStop := []llm.StreamChunk{
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}
	core := &fakeCore{streamChunksList: [][]llm.StreamChunk{
		emptyStop, emptyStop, emptyStop, emptyStop, emptyStop, emptyStop,
	}}
	ag := newTestAgent(core, "m")
	ch := make(chan agentcore.Event, 64)
	msgs := []llm.Message{
		{Role: "user", Content: "go"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "c1", Function: llm.FunctionCall{Name: "noop", Arguments: `{}`}}}},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
	}

	counter := 0
	const wantMax = 5
	for attempt := 1; attempt <= wantMax; attempt++ {
		msgsOut, ok := agentcore.AgentRunOneTurnWithEmptyContinuationCounterForTest(ag, context.Background(), msgs, ch, 0, &counter)
		if !ok {
			t.Fatalf("attempt %d: ok=false, want true (should continue after empty post-tool hiccup)", attempt)
		}
		if counter != attempt {
			t.Errorf("attempt %d: counter = %d, want %d", attempt, counter, attempt)
		}
		last := msgsOut[len(msgsOut)-1]
		if last.Role != "user" || last.Content != "Please continue." {
			t.Errorf("attempt %d: last message = %+v, want user/Please continue.", attempt, last)
		}
		var sawContinuation bool
	loop:
		for {
			select {
			case ev := <-ch:
				if ev.Category == agentcore.EventUserMessage && ev.Content == "Please continue." {
					sawContinuation = true
				}
			default:
				break loop
			}
		}
		if !sawContinuation {
			t.Errorf("attempt %d: did not see EventUserMessage on the channel", attempt)
		}
		msgs = msgsOut
	}

	_, ok := agentcore.AgentRunOneTurnWithEmptyContinuationCounterForTest(ag, context.Background(), msgs, ch, 0, &counter)
	if ok {
		t.Errorf("attempt %d (after %d retries): ok=true, want false (safety net must give up)", wantMax+1, wantMax)
	}
	if counter != wantMax {
		t.Errorf("counter at give-up = %d, want %d", counter, wantMax)
	}
}

func TestRunOneTurnEmptyStepFinalWithoutToolMessageTerminates(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	ch := make(chan agentcore.Event, 32)
	msgs := []llm.Message{{Role: "user", Content: "go"}}

	msgsOut, ok := agentcore.AgentRunOneTurnForTest(ag, context.Background(), msgs, ch, 0)
	if ok {
		t.Errorf("ok=true, want false (empty StepFinal without preceding tool message is a genuine finish)")
	}
	if len(msgsOut) != 1 {
		t.Errorf("msgs len = %d, want 1 (no continuation injected without preceding tool message)", len(msgsOut))
	}
}

func TestRunOneTurnNonEmptyStepFinalTerminates(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("final answer"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	ch := make(chan agentcore.Event, 32)
	msgs := []llm.Message{
		{Role: "user", Content: "go"},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
	}

	msgsOut, ok := agentcore.AgentRunOneTurnForTest(ag, context.Background(), msgs, ch, 0)
	if ok {
		t.Errorf("ok=true, want false (non-empty StepFinal must terminate regardless of preceding tool messages)")
	}
	if len(msgsOut) != 2 {
		t.Errorf("msgs len = %d, want 2 (no continuation injected for non-empty finish)", len(msgsOut))
	}
}

func TestRunOneTurnCounterResetsOnToolCall(t *testing.T) {
	emptyStop := []llm.StreamChunk{
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}
	toolCall := []llm.StreamChunk{
		toolUseStartChunk("c2", "noop"),
		toolUseIDDeltaChunk("c2", "noop", `{"intent":"x"}`),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}
	core := &fakeCore{streamChunksList: [][]llm.StreamChunk{emptyStop, toolCall}}
	ag := newTestAgent(core, "m")
	ag.WithTool(agentcore.ToolFunc{
		N:  "noop",
		Fn: func(_ context.Context, _ string) (string, error) { return "ok", nil },
	})
	ch := make(chan agentcore.Event, 64)
	msgs := []llm.Message{
		{Role: "user", Content: "go"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "c1", Function: llm.FunctionCall{Name: "noop", Arguments: `{"intent":"x"}`}}}},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
	}

	counter := 0
	msgsOut, ok := agentcore.AgentRunOneTurnWithEmptyContinuationCounterForTest(ag, context.Background(), msgs, ch, 0, &counter)
	if !ok {
		t.Fatalf("turn 1: ok=false, want true (empty hiccup after tool result must continue)")
	}
	if counter != 1 {
		t.Errorf("after turn 1 (empty hiccup): counter = %d, want 1", counter)
	}
	msgs = msgsOut

	msgsOut, ok = agentcore.AgentRunOneTurnWithEmptyContinuationCounterForTest(ag, context.Background(), msgs, ch, 1, &counter)
	if !ok {
		t.Fatalf("turn 2: ok=false, want true (tool call keeps loop alive)")
	}
	if counter != 0 {
		t.Errorf("after turn 2 (tool call): counter = %d, want 0 (reset on real tool call)", counter)
	}
}

func TestRunOneTurnCounterResetsOnNonEmptyContent(t *testing.T) {
	core := &fakeCore{streamChunksList: [][]llm.StreamChunk{
		{messageDeltaStopChunk("end_turn"), messageStopChunk()},
		{textDeltaChunk("answer"), messageDeltaStopChunk("end_turn"), messageStopChunk()},
	}}
	ag := newTestAgent(core, "m")
	ch := make(chan agentcore.Event, 64)
	msgs := []llm.Message{
		{Role: "user", Content: "go"},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
	}

	counter := 0
	_, ok := agentcore.AgentRunOneTurnWithEmptyContinuationCounterForTest(ag, context.Background(), msgs, ch, 0, &counter)
	if !ok {
		t.Fatal("turn 1: ok=false, want true (empty hiccup must continue)")
	}
	if counter != 1 {
		t.Errorf("after turn 1: counter = %d, want 1", counter)
	}

	msgsWithContinuation := []llm.Message{
		{Role: "user", Content: "go"},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
		{Role: "user", Content: "Please continue."},
	}
	_, ok = agentcore.AgentRunOneTurnWithEmptyContinuationCounterForTest(ag, context.Background(), msgsWithContinuation, ch, 1, &counter)
	if ok {
		t.Error("turn 2: ok=true, want false (non-empty StepFinal terminates)")
	}
	if counter != 0 {
		t.Errorf("after turn 2 (non-empty finish): counter = %d, want 0 (reset on non-empty content)", counter)
	}
}

func singleToolChunks(name, args string) []llm.StreamChunk {
	return []llm.StreamChunk{
		toolUseStartChunk("c", name),
		toolUseIDDeltaChunk("c", name, args),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}
}

func multiToolChunks(n int, name string) []llm.StreamChunk {
	if n <= 0 {
		return nil
	}
	chunks := make([]llm.StreamChunk, 0, n+2)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("c%d", i+1)
		if i == 0 {
			chunks = append(chunks, toolUseStartChunk(id, name))
		} else {
			chunks = append(chunks, toolUseIDDeltaChunk(id, name, `{"intent":"x"}`))
		}
	}
	chunks = append(chunks, messageDeltaStopChunk("tool_use"), messageStopChunk())
	return chunks
}

const systemReminderBatchNudgeMarker = "<system-reminder>"

func requestContainsNudge(req llm.ChatRequest) bool {
	for _, m := range req.Messages {
		if m.Role == "user" && strings.Contains(m.Content, systemReminderBatchNudgeMarker) {
			return true
		}
	}
	return false
}

func requestHasNudgeAsLastMessage(req llm.ChatRequest) bool {
	if len(req.Messages) == 0 {
		return false
	}
	last := req.Messages[len(req.Messages)-1]
	return last.Role == "user" && strings.Contains(last.Content, systemReminderBatchNudgeMarker)
}

func TestLoopWithMsgsNoNudgeForMultiToolCalls(t *testing.T) {
	multiTool := multiToolChunks(3, "noop")
	finalAnswer := []llm.StreamChunk{
		textDeltaChunk("done"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}
	core := &fakeCore{streamChunksList: [][]llm.StreamChunk{
		multiTool, multiTool, multiTool, finalAnswer,
	}}
	ag := newTestAgent(core, "test-model").WithSafetyNet(8)
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(_ context.Context, _ string) (string, error) { return "ok", nil }})
	ag.WithTool(agentcore.ToolFunc{N: "noop2", Fn: func(_ context.Context, _ string) (string, error) { return "ok2", nil }})

	ch := make(chan agentcore.Event, 256)
	msgs := []llm.Message{{Role: "user", Content: "go"}}
	agentcore.AgentLoopWithMsgsForTest(ag, context.Background(), msgs, ch)
	close(ch)
	for range ch {
	}

	for i, req := range core.requests {
		if requestContainsNudge(req) {
			t.Errorf("request[%d] contained unexpected system-reminder nudge; multi-tool turns must never trigger nudge", i)
		}
	}
}

func TestLoopWithMsgsNoNudgeForZeroToolCalls(t *testing.T) {
	finalAnswer := []llm.StreamChunk{
		textDeltaChunk("done"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}
	core := &fakeCore{streamChunks: finalAnswer}
	ag := newTestAgent(core, "test-model").WithSafetyNet(5)
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(_ context.Context, _ string) (string, error) { return "ok", nil }})
	ag.WithTool(agentcore.ToolFunc{N: "noop2", Fn: func(_ context.Context, _ string) (string, error) { return "ok2", nil }})

	ch := make(chan agentcore.Event, 64)
	msgs := []llm.Message{{Role: "user", Content: "go"}}
	agentcore.AgentLoopWithMsgsForTest(ag, context.Background(), msgs, ch)
	close(ch)
	for range ch {
	}

	if core.streamCalls != 1 {
		t.Errorf("stream calls = %d, want 1 (final answer terminates loop on first turn)", core.streamCalls)
	}
	if len(core.requests) == 0 {
		t.Fatal("expected at least 1 request")
	}
	if requestContainsNudge(core.requests[0]) {
		t.Errorf("expected no nudge on final-answer path; got msgs=%+v", core.requests[0].Messages)
	}
}

func TestLoopWithMsgsNudgeAfterThreeSingleToolTurns(t *testing.T) {
	chunksList := make([][]llm.StreamChunk, 5)
	for i := range chunksList {
		chunksList[i] = singleToolChunks("noop", fmt.Sprintf(`{"intent":"x","t":%d}`, i))
	}
	core := &fakeCore{streamChunksList: chunksList}
	ag := newTestAgent(core, "test-model").WithSafetyNet(5)
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(_ context.Context, _ string) (string, error) { return "ok", nil }})
	ag.WithTool(agentcore.ToolFunc{N: "noop2", Fn: func(_ context.Context, _ string) (string, error) { return "ok2", nil }})

	ch := make(chan agentcore.Event, 256)
	msgs := []llm.Message{{Role: "user", Content: "go"}}
	agentcore.AgentLoopWithMsgsForTest(ag, context.Background(), msgs, ch)
	close(ch)
	for range ch {
	}

	if core.streamCalls != 5 {
		t.Fatalf("stream calls = %d, want 5 (SafetyNet=5)", core.streamCalls)
	}
	if len(core.requests) < 4 {
		t.Fatalf("requests = %d, want >= 4", len(core.requests))
	}
	if !requestContainsNudge(core.requests[3]) {
		t.Errorf("expected batch nudge injected before turn 3 (0-indexed); got request[3].Messages = %+v", core.requests[3].Messages)
	}
	for i := 0; i < 3; i++ {
		if requestContainsNudge(core.requests[i]) {
			t.Errorf("request[%d] contained unexpected nudge; first nudge must wait until turn 3", i)
		}
	}
}

func TestLoopWithMsgsNudgeSingleShot(t *testing.T) {
	chunksList := make([][]llm.StreamChunk, 6)
	for i := range chunksList {
		chunksList[i] = singleToolChunks("noop", fmt.Sprintf(`{"intent":"x","t":%d}`, i))
	}
	core := &fakeCore{streamChunksList: chunksList}
	ag := newTestAgent(core, "test-model").WithSafetyNet(6)
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(_ context.Context, _ string) (string, error) { return "ok", nil }})
	ag.WithTool(agentcore.ToolFunc{N: "noop2", Fn: func(_ context.Context, _ string) (string, error) { return "ok2", nil }})

	ch := make(chan agentcore.Event, 256)
	msgs := []llm.Message{{Role: "user", Content: "go"}}
	agentcore.AgentLoopWithMsgsForTest(ag, context.Background(), msgs, ch)
	close(ch)
	for range ch {
	}

	var injectedAt []int
	for i, req := range core.requests {
		if requestHasNudgeAsLastMessage(req) {
			injectedAt = append(injectedAt, i)
		}
	}
	if len(injectedAt) != 1 {
		t.Errorf("nudge injection count = %d (turns=%v), want exactly 1 (counter resets after nudge; next streak at turn 6 doesn't trigger because SafetyNet=6 halts at turn 5)", len(injectedAt), injectedAt)
	}
	if len(injectedAt) >= 1 && injectedAt[0] != 3 {
		t.Errorf("first nudge injection at turn %d, want 3", injectedAt[0])
	}
}

func TestLoopWithMsgsCounterResetsOnMultiToolCall(t *testing.T) {
	singleTurn0 := singleToolChunks("noop", `{"intent":"x","t":0}`)
	singleTurn3 := singleToolChunks("noop", `{"intent":"x","t":3}`)
	multiTool := multiToolChunks(3, "noop")
	finalAnswer := []llm.StreamChunk{
		textDeltaChunk("done"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}
	core := &fakeCore{streamChunksList: [][]llm.StreamChunk{
		singleTurn0, singleTurn0, multiTool, singleTurn3, finalAnswer,
	}}
	ag := newTestAgent(core, "test-model").WithSafetyNet(8)
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(_ context.Context, _ string) (string, error) { return "ok", nil }})
	ag.WithTool(agentcore.ToolFunc{N: "noop2", Fn: func(_ context.Context, _ string) (string, error) { return "ok2", nil }})

	ch := make(chan agentcore.Event, 256)
	msgs := []llm.Message{{Role: "user", Content: "go"}}
	agentcore.AgentLoopWithMsgsForTest(ag, context.Background(), msgs, ch)
	close(ch)
	for range ch {
	}

	for i, req := range core.requests {
		if requestContainsNudge(req) {
			t.Errorf("request[%d] contained unexpected nudge; multi-tool turn at turn 2 must reset the counter before threshold", i)
		}
	}
}

func rollbackEvent() llm.StreamEvent {
	return streamtest.Rollback(1, 3)
}

func TestProcessStreamEventRollbackResetsContentBuf(t *testing.T) {
	events := []llm.StreamEvent{
		streamtest.Text("hello"),
		rollbackEvent(),
		streamtest.Text("world"),
	}
	got := agentcore.ProcessStreamEventsForTest(events)
	if got.Content != "world" {
		t.Errorf("after Rollback, Content = %q, want %q (failed-attempt partial must be discarded)", got.Content, "world")
	}
	if !got.Continue {
		t.Errorf("Rollback must return continue=true; got continue=false (stream would terminate)")
	}
}

func TestProcessStreamEventRollbackResetsReasoningBuf(t *testing.T) {
	reasoningEvent := streamtest.Reasoning("thinking...")
	events := []llm.StreamEvent{
		reasoningEvent,
		rollbackEvent(),
		reasoningEvent,
	}
	got := agentcore.ProcessStreamEventsForTest(events)
	if got.Reasoning != "thinking..." {
		t.Errorf("after Rollback, Reasoning = %q, want %q (reasoning buffer must reset then accumulate only post-rollback)", got.Reasoning, "thinking...")
	}
}

func TestProcessStreamEventRollbackResetsResultFields(t *testing.T) {
	reasoningEvent := streamtest.Reasoning("thinking")
	sigEvent := streamtest.ReasoningSignature("sig123")
	events := []llm.StreamEvent{
		reasoningEvent,
		sigEvent,
		streamtest.ToolStartDelta("call_1", "read", "")[0],
		rollbackEvent(),
		streamtest.Text("after"),
	}
	got := agentcore.ProcessStreamEventsForTest(events)
	if got.Content != "after" {
		t.Errorf("after Rollback, Content = %q, want %q", got.Content, "after")
	}
	if got.Reasoning != "" {
		t.Errorf("after Rollback, Reasoning = %q, want \"\"", got.Reasoning)
	}
	if got.ReasoningSig != "" {
		t.Errorf("after Rollback, ReasoningSig = %q, want \"\"", got.ReasoningSig)
	}
	if got.ToolCalls != nil {
		t.Errorf("after Rollback, ToolCalls = %+v, want nil", got.ToolCalls)
	}
}

func TestProcessStreamEventNoRollbackAccumulates(t *testing.T) {
	events := []llm.StreamEvent{
		streamtest.Text("foo"),
		streamtest.Text("bar"),
		streamtest.Text("baz"),
	}
	got := agentcore.ProcessStreamEventsForTest(events)
	if got.Content != "foobarbaz" {
		t.Errorf("without Rollback, Content = %q, want %q (control: accumulate behavior preserved)", got.Content, "foobarbaz")
	}
}

func TestProcessStreamEventRollbackDoesNotEmit(t *testing.T) {
	events := []llm.StreamEvent{
		rollbackEvent(),
	}
	got := agentcore.ProcessStreamEventsForTest(events)
	if len(got.Emitted) != 0 {
		t.Errorf("Rollback event alone must emit 0 events, got %d: %+v", len(got.Emitted), got.Emitted)
	}
}

func TestRunOneTurnEmitsSafetyRepairWhenToolOutputsMissing(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("done"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	ch := make(chan agentcore.Event, 32)
	msgs := []llm.Message{
		{Role: "user", Content: "go"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "c1", Function: llm.FunctionCall{Name: "noop", Arguments: `{}`}},
			{ID: "c2", Function: llm.FunctionCall{Name: "noop", Arguments: `{}`}},
		}},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
	}

	_, _ = agentcore.AgentRunOneTurnForTest(ag, context.Background(), msgs, ch, 0)

	var safetyRepairCount int
	var sawRepair bool
	for {
		select {
		case ev := <-ch:
			if ev.Category == agentcore.EventSafetyRepair {
				sawRepair = true
				safetyRepairCount = ev.SafetyCount
			}
		default:
			goto done
		}
	}
done:
	if !sawRepair {
		t.Fatalf("expected EventSafetyRepair emit when RepairMissingToolOutputs fills 1 gap, got none")
	}
	if safetyRepairCount != 1 {
		t.Errorf("EventSafetyRepair.SafetyCount = %d, want 1 (one missing tool output recovered)", safetyRepairCount)
	}
}

func TestLoopWithMsgsEmitsSafetyNudgeAfterThreshold(t *testing.T) {
	chunksList := make([][]llm.StreamChunk, 5)
	for i := range chunksList {
		chunksList[i] = singleToolChunks("noop", fmt.Sprintf(`{"intent":"x","t":%d}`, i))
	}
	core := &fakeCore{streamChunksList: chunksList}
	ag := newTestAgent(core, "test-model").WithSafetyNet(5)
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(_ context.Context, _ string) (string, error) { return "ok", nil }})
	ag.WithTool(agentcore.ToolFunc{N: "noop2", Fn: func(_ context.Context, _ string) (string, error) { return "ok2", nil }})

	ch := make(chan agentcore.Event, 256)
	msgs := []llm.Message{{Role: "user", Content: "go"}}
	agentcore.AgentLoopWithMsgsForTest(ag, context.Background(), msgs, ch)
	close(ch)

	sawNudge := false
	sawUserMessage := false
	for ev := range ch {
		if ev.Category == agentcore.EventSafetyNudge {
			sawNudge = true
		}
		if ev.Category == agentcore.EventUserMessage && strings.Contains(ev.Content, systemReminderBatchNudgeMarker) {
			sawUserMessage = true
		}
	}
	if !sawNudge {
		t.Errorf("expected EventSafetyNudge emit when BATCH_NUDGE fires after 3 single-tool turns; got none")
	}
	if !sawUserMessage {
		t.Errorf("expected EventUserMessage containing system-reminder nudge text on wire; got none (TUI relies on this signal)")
	}
}

func TestHandleEmptyPostToolContinuationEmitsSafetyEvent(t *testing.T) {
	emptyStop := []llm.StreamChunk{
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}
	core := &fakeCore{streamChunks: emptyStop}
	ag := newTestAgent(core, "m")
	ch := make(chan agentcore.Event, 32)
	msgs := []llm.Message{
		{Role: "user", Content: "go"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "c1", Function: llm.FunctionCall{Name: "noop", Arguments: `{}`}}}},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
	}

	counter := 0
	_, ok := agentcore.AgentRunOneTurnWithEmptyContinuationCounterForTest(ag, context.Background(), msgs, ch, 0, &counter)
	if !ok {
		t.Fatal("expected empty post-tool continuation to keep loop alive, got ok=false")
	}

	var sawSafetyEmptyContinue bool
	var safetyContent string
	var sawUserContinue bool
	for {
		select {
		case ev := <-ch:
			if ev.Category == agentcore.EventSafetyEmptyContinue {
				sawSafetyEmptyContinue = true
				safetyContent = ev.Content
			}
			if ev.Category == agentcore.EventUserMessage && ev.Content == "Please continue." {
				sawUserContinue = true
			}
		default:
			goto done
		}
	}
done:
	if !sawSafetyEmptyContinue {
		t.Fatalf("expected EventSafetyEmptyContinue emit when handleEmptyPostToolContinuation fires, got none")
	}
	if !strings.Contains(safetyContent, "EMPTY_CONTINUE") {
		t.Errorf("EventSafetyEmptyContinue.Content = %q, want contains EMPTY_CONTINUE marker", safetyContent)
	}
	if !sawUserContinue {
		t.Errorf("expected EventUserMessage(\"Please continue.\") on wire so TUI can pattern-detect; got none")
	}
}

func TestTurnStateFreshAllocation(t *testing.T) {
	ts := agentcore.TurnStateForTest()
	if ts == nil {
		t.Fatal("TurnStateForTest() returned nil")
	}
	if got := agentcore.TurnStateEmptyContinuationsForTest(ts); got != 0 {
		t.Errorf("fresh emptyContinuations = %d, want 0", got)
	}
	if got := agentcore.TurnStateConsecutiveSingleToolTurnsForTest(ts); got != 0 {
		t.Errorf("fresh consecutiveSingleToolTurns = %d, want 0", got)
	}
	if got := agentcore.TurnStateConsecutiveFailedToolTurnsForTest(ts); got != 0 {
		t.Errorf("fresh consecutiveFailedToolTurns = %d, want 0", got)
	}
	if got := agentcore.TurnStatePreflightStreakMapForTest(ts); got == nil {
		t.Errorf("fresh preflightFailureStreak map is nil, want non-nil empty map")
	} else if len(got) != 0 {
		t.Errorf("fresh preflightFailureStreak len = %d, want 0", len(got))
	}
}

func TestCheckPreflightStreakEmpty(t *testing.T) {
	if err := agentcore.CheckPreflightStreakForTest(agentcore.TurnStateForTest()); err != nil {
		t.Errorf("checkPreflightStreak on empty turnState returned %v, want nil", err)
	}
}

func TestCheckPreflightStreakUnderThreshold(t *testing.T) {
	ts := agentcore.TurnStateForTest()
	m := agentcore.TurnStatePreflightStreakMapForTest(ts)
	under := agentcore.PreflightAbortThreshold - 1
	m["bash"] = under
	if err := agentcore.CheckPreflightStreakForTest(ts); err != nil {
		t.Errorf("checkPreflightStreak at streak %d (below threshold %d) returned %v, want nil", under, agentcore.PreflightAbortThreshold, err)
	}
}

func TestCheckPreflightStreakAtThreshold(t *testing.T) {
	ts := agentcore.TurnStateForTest()
	m := agentcore.TurnStatePreflightStreakMapForTest(ts)
	m["bash"] = agentcore.PreflightAbortThreshold
	err := agentcore.CheckPreflightStreakForTest(ts)
	if err == nil {
		t.Fatalf("checkPreflightStreak at threshold %d returned nil, want error", agentcore.PreflightAbortThreshold)
	}
	if !strings.Contains(err.Error(), "bash") {
		t.Errorf("error should name the failing tool 'bash': %q", err.Error())
	}
	if !strings.Contains(err.Error(), "invalid arguments") {
		t.Errorf("error should mention 'invalid arguments': %q", err.Error())
	}
}

func TestTurnStateEmptyContinuationsTrackable(t *testing.T) {
	ts := agentcore.TurnStateForTest()
	if got := agentcore.TurnStateEmptyContinuationsForTest(ts); got != 0 {
		t.Fatalf("baseline emptyContinuations = %d, want 0", got)
	}
	ts.IncrementEmptyContinuationsForTest()
	if got := agentcore.TurnStateEmptyContinuationsForTest(ts); got != 1 {
		t.Errorf("after one increment emptyContinuations = %d, want 1", got)
	}
	ts.IncrementEmptyContinuationsForTest()
	if got := agentcore.TurnStateEmptyContinuationsForTest(ts); got != 2 {
		t.Errorf("after two increments emptyContinuations = %d, want 2", got)
	}
}

func TestTurnStateConsecutiveSingleToolTurnsTrackable(t *testing.T) {
	ts := agentcore.TurnStateForTest()
	if got := agentcore.TurnStateConsecutiveSingleToolTurnsForTest(ts); got != 0 {
		t.Fatalf("baseline consecutiveSingleToolTurns = %d, want 0", got)
	}
	ts.IncrementConsecutiveSingleToolTurnsForTest()
	if got := agentcore.TurnStateConsecutiveSingleToolTurnsForTest(ts); got != 1 {
		t.Errorf("after one increment consecutiveSingleToolTurns = %d, want 1", got)
	}
}

func TestTurnStateConsecutiveFailedToolTurnsTrackable(t *testing.T) {
	ts := agentcore.TurnStateForTest()
	if got := agentcore.TurnStateConsecutiveFailedToolTurnsForTest(ts); got != 0 {
		t.Fatalf("baseline consecutiveFailedToolTurns = %d, want 0", got)
	}
	ts.SetConsecutiveFailedToolTurnsForTest(5)
	if got := agentcore.TurnStateConsecutiveFailedToolTurnsForTest(ts); got != 5 {
		t.Errorf("after set consecutiveFailedToolTurns = %d, want 5", got)
	}
}

func TestTurnStatePreflightStreakIsolatedPerLoop(t *testing.T) {
	ts1 := agentcore.TurnStateForTest()
	ts2 := agentcore.TurnStateForTest()
	m1 := agentcore.TurnStatePreflightStreakMapForTest(ts1)
	m1["alpha"] = 7
	m1["beta"] = 3
	m2 := agentcore.TurnStatePreflightStreakMapForTest(ts2)
	if len(m2) != 0 {
		t.Errorf("ts2 map has %d entries after ts1 mutations, want 0 (independent maps)", len(m2))
	}
	if got := agentcore.TurnStatePreflightStreakForTest(ts1, "alpha"); got != 7 {
		t.Errorf("ts1[alpha] = %d, want 7", got)
	}
	if got := agentcore.TurnStatePreflightStreakForTest(ts2, "alpha"); got != 0 {
		t.Errorf("ts2[alpha] = %d, want 0 (cross-loop isolation)", got)
	}
}

func TestRunOneTurnUsesTurnStateEmptyContinuations(t *testing.T) {
	emptyStop := []llm.StreamChunk{
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}
	core := &fakeCore{streamChunks: emptyStop}
	ag := newTestAgent(core, "m")
	ch := make(chan agentcore.Event, 32)
	msgs := []llm.Message{
		{Role: "user", Content: "go"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "c1", Function: llm.FunctionCall{Name: "noop", Arguments: `{}`}}}},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
	}

	_, _, _, ts := agentcore.AgentRunOneTurnWithTurnStateForTest(ag, context.Background(), msgs, ch, 0)

	if got := agentcore.TurnStateEmptyContinuationsForTest(ts); got != 1 {
		t.Errorf("after empty-post-tool continuation: emptyContinuations = %d, want 1 (runOneTurn should increment via handleEmptyPostToolContinuation)", got)
	}
}

func TestCompactorAsyncCancelsWithinBudget(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	fn := func(ctx context.Context) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c := compact.NewCompactor(fn)
	ctx, cancel := context.WithCancel(context.Background())
	if err := c.CompactAsync(ctx); err != nil {
		t.Fatalf("CompactAsync returned err: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("compactor goroutine did not start within 1s")
	}
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer waitCancel()
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- c.WaitIdle(waitCtx)
	}()
	select {
	case err := <-waitDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WaitIdle err = %v, want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("WaitIdle did not return within 500ms after cancel")
	}
}

func TestCompactorWaitIdleReturnsSuccessAfterFnCompletes(t *testing.T) {
	c := compact.NewCompactor(func(ctx context.Context) error { return nil })
	if err := c.CompactAsync(context.Background()); err != nil {
		t.Fatalf("CompactAsync err: %v", err)
	}
	if err := c.WaitIdle(context.Background()); err != nil {
		t.Fatalf("WaitIdle err = %v, want nil", err)
	}
}

func TestCompactorCompactAsyncIsIdempotent(t *testing.T) {
	var calls int32
	c := compact.NewCompactor(func(ctx context.Context) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	for i := 0; i < 3; i++ {
		if err := c.CompactAsync(context.Background()); err != nil {
			t.Fatalf("CompactAsync call #%d: %v", i, err)
		}
	}
	if err := c.WaitIdle(context.Background()); err != nil {
		t.Fatalf("WaitIdle err: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("compactFn calls = %d, want 1 (CompactAsync must be idempotent)", got)
	}
}

type slowStreamCore struct {
	inner *fakeCore
	delay time.Duration
}

func (s *slowStreamCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	raw, err := s.inner.StreamChat(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make(chan llm.StreamEvent, 32)
	go func() {
		defer close(out)
		for ev := range raw {
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
			select {
			case <-time.After(s.delay):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func TestSoftInterruptHaltsRunWithinBudget(t *testing.T) {
	chunks := []llm.StreamChunk{
		textDeltaChunk("hello"),
		textDeltaChunk(" world"),
		textDeltaChunk(" foo"),
		textDeltaChunk(" bar"),
		textDeltaChunk(" baz"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}
	inner := &fakeCore{streamChunks: chunks}
	slow := &slowStreamCore{inner: inner, delay: 5 * time.Second}
	ag := agentcore.NewAgent(slow).WithModel(llm.Model{ID: "test-model", SupportsTool: true})
	intr := ag.Interrupt()
	if intr == nil {
		t.Fatal("Interrupt() returned nil")
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		intr.Raise(agentcore.SignalUser)
	}()

	start := time.Now()
	for range ag.RunStream(context.Background(), "test") {
	}
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Fatalf("interrupt took %v, expected <500ms", elapsed)
	}
	if intr.Last() != agentcore.SignalUser {
		t.Fatalf("expected last signal SignalUser, got %v", intr.Last())
	}
}
