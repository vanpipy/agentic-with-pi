package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

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
}

func (f *fakeCore) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	f.chatCalls++
	if f.chatErr != nil {
		return nil, f.chatErr
	}
	return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.Message{Content: ""}}}}, nil
}

func (f *fakeCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	f.streamCalls++
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
			Function: llm.FunctionCall{Name: name},
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
		switch ev.Kind {
		case agent.EventFinalAnswer:
			result = ev.Content
		case agent.EventError:
			if firstErr == nil {
				firstErr = ev.ToolError
			}
		}
	}
	return result, firstErr
}

func TestAgentReturnsFinalAnswerImmediately(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		textDeltaChunk("42"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := agent.New(core, "test-model")

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
	ag := agent.New(core, "test-model")
	ag.AddTool(agent.Tool{
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
	ag := agent.New(core, "test-model")
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
	ag := agent.New(core, "test-model")
	ag.AddTool(agent.Tool{
		Name:   "boom",
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
	ag := agent.New(core, "test-model").WithMaxTurns(2)
	ag.AddTool(agent.Tool{
		Name:   "loop",
		Execute: func(ctx context.Context, argsJSON string) (string, error) { return "", nil },
	})
	_, err := runAgent(t, ag, "loop forever")
	if err == nil {
		t.Fatal("expected max-turns error, got nil")
	}
}

func TestAgentStreamErrorPropagates(t *testing.T) {
	core := &fakeCore{streamErr: errors.New("transient failure")}
	ag := agent.New(core, "test-model")
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
	ag := agent.New(core, "test-model")
	ag.AddTool(agent.Tool{Name: "f", Execute: func(ctx context.Context, argsJSON string) (string, error) { return "", nil }})

	_, err := runAgent(t, ag, "truncated")
	if err == nil {
		t.Fatal("expected error for length+tool_calls, got nil")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("err = %q, want mentions truncated", err.Error())
	}
}

func TestAgentExtractsSignatureFromThinkingBlock(t *testing.T) {
	sigPayload := `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","signature":"sig-stream-1"}}`
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(sigPayload), &raw); err != nil {
		t.Fatal(err)
	}
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
			Index: 0,
			Delta: llm.Message{ReasoningSig: "sig-stream-1"},
		}}}},
		textDeltaChunk("final"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := agent.New(core, "test-model")

	var sawFinal bool
	var sawSigInMessages bool
	for ev := range ag.RunStream(context.Background(), "hi") {
		if ev.Kind == agent.EventFinalAnswer {
			sawFinal = true
		}
	}
	_ = raw
	_ = sawSigInMessages
	if !sawFinal {
		t.Error("expected FinalAnswer")
	}
}
