package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent"
	"github.com/vanpiyp/awp/internal/llm"
)

type fakeCore struct {
	responses []*llm.ChatResponse
	errs      []error
	calls     int
}

func (f *fakeCore) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	idx := f.calls
	f.calls++
	if idx < len(f.errs) && f.errs[idx] != nil {
		return nil, f.errs[idx]
	}
	if idx < len(f.responses) {
		return f.responses[idx], nil
	}
	return nil, errors.New("no more responses queued")
}

func (f *fakeCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, errors.New("not used")
}

func toolCallResp(id, name, args string) *llm.ChatResponse {
	return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.Message{
		ToolCalls: []llm.ToolCall{{
			ID:       id,
			Type:     "function",
			Function: llm.FunctionCall{Name: name, Arguments: args},
		}},
	}}}}
}

func textResp(content string) *llm.ChatResponse {
	return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.Message{Content: content}}}}
}

func TestAgentReturnsFinalAnswerImmediately(t *testing.T) {
	core := &fakeCore{responses: []*llm.ChatResponse{textResp("42")}}
	ag := agent.New(core, "test-model")

	result, err := ag.Run(context.Background(), "What is the answer?")
	if err != nil {
		t.Fatal(err)
	}
	if result != "42" {
		t.Errorf("result = %q, want 42", result)
	}
	if core.calls != 1 {
		t.Errorf("calls = %d, want 1", core.calls)
	}
}

func TestAgentExecutesToolAndContinues(t *testing.T) {
	core := &fakeCore{
		responses: []*llm.ChatResponse{
			toolCallResp("call_1", "get_time", `{"location":"Beijing"}`),
			textResp("It's 3pm in Beijing."),
		},
	}
	ag := agent.New(core, "test-model")
	ag.RegisterTool(agent.Tool{
		Def:  llm.ToolDef{Type: "function", Function: llm.FunctionDef{Name: "get_time"}},
		Func: func(argsJSON string) (string, error) { return "2026-09-13T15:00:00+08:00", nil },
	})

	result, err := ag.Run(context.Background(), "time?")
	if err != nil {
		t.Fatal(err)
	}
	if result != "It's 3pm in Beijing." {
		t.Errorf("result = %q", result)
	}
	if core.calls != 2 {
		t.Errorf("calls = %d, want 2", core.calls)
	}
}

func TestAgentCallsAllToolCallsInOneTurn(t *testing.T) {
	t1 := llm.ToolCall{ID: "a", Type: "function", Function: llm.FunctionCall{Name: "t1", Arguments: "{}"}}
	t2 := llm.ToolCall{ID: "b", Type: "function", Function: llm.FunctionCall{Name: "t2", Arguments: "{}"}}
	resp := &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.Message{ToolCalls: []llm.ToolCall{t1, t2}}}}}
	core := &fakeCore{responses: []*llm.ChatResponse{resp, textResp("done")}}

	ag := agent.New(core, "test-model")
	var t1Calls, t2Calls int
	ag.RegisterTool(agent.Tool{
		Def:  llm.ToolDef{Type: "function", Function: llm.FunctionDef{Name: "t1"}},
		Func: func(argsJSON string) (string, error) { t1Calls++; return "1", nil },
	})
	ag.RegisterTool(agent.Tool{
		Def:  llm.ToolDef{Type: "function", Function: llm.FunctionDef{Name: "t2"}},
		Func: func(argsJSON string) (string, error) { t2Calls++; return "2", nil },
	})

	result, err := ag.Run(context.Background(), "both")
	if err != nil {
		t.Fatal(err)
	}
	if result != "done" {
		t.Errorf("result = %q", result)
	}
	if t1Calls != 1 || t2Calls != 1 {
		t.Errorf("t1=%d t2=%d, want 1 each", t1Calls, t2Calls)
	}
}

func TestAgentUnknownTool(t *testing.T) {
	core := &fakeCore{responses: []*llm.ChatResponse{toolCallResp("1", "ghost", "{}")}}
	ag := agent.New(core, "test-model")
	_, err := ag.Run(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("err = %q, want mentions ghost", err.Error())
	}
}

func TestAgentToolExecutionError(t *testing.T) {
	core := &fakeCore{responses: []*llm.ChatResponse{toolCallResp("1", "boom", "{}")}}
	ag := agent.New(core, "test-model")
	ag.RegisterTool(agent.Tool{
		Def:  llm.ToolDef{Type: "function", Function: llm.FunctionDef{Name: "boom"}},
		Func: func(argsJSON string) (string, error) { return "", errors.New("kaboom") },
	})
	_, err := ag.Run(context.Background(), "boom")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "kaboom") {
		t.Errorf("err = %q, want mentions kaboom", err.Error())
	}
}

func TestAgentMaxTurnsExceeded(t *testing.T) {
	core := &fakeCore{responses: []*llm.ChatResponse{
		toolCallResp("1", "loop", "{}"),
		toolCallResp("2", "loop", "{}"),
	}}
	ag := agent.New(core, "test-model").WithMaxTurns(2)
	ag.RegisterTool(agent.Tool{
		Def:  llm.ToolDef{Type: "function", Function: llm.FunctionDef{Name: "loop"}},
		Func: func(argsJSON string) (string, error) { return "", nil },
	})
	_, err := ag.Run(context.Background(), "loop forever")
	if err == nil {
		t.Fatal("expected max-turns error, got nil")
	}
}

func TestAgentChatErrorPropagates(t *testing.T) {
	core := &fakeCore{errs: []error{errors.New("transient failure")}}
	ag := agent.New(core, "test-model")
	_, err := ag.Run(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "transient") {
		t.Errorf("err = %v, want contains transient", err)
	}
}
