package llm_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol"
)

func TestValidateToolsViaCoreIndexAboveZero(t *testing.T) {
	fp := &fakeProvider{
		models: []llm.Model{{ID: "fake-1", SupportsTool: true}},
		convertReq: func(r *llm.ChatRequest) ([]byte, error) {
			return []byte("{}"), nil
		},
		convertChunk: func(b []byte) (*llm.StreamChunk, bool, error) {
			return &llm.StreamChunk{}, true, nil
		},
	}
	proto := &fakeProtocol{streamItems: []protocol.StreamItem{}}
	c := llm.NewCore(fp, proto)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := c.StreamChat(ctx, &llm.ChatRequest{
		Model:    "fake-1",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
		Tools: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "ok"}},
			{Type: "function", Function: llm.FunctionDef{Name: ""}},
		},
	})
	if err == nil {
		t.Fatal("expected validation error for invalid tool at index 1")
	}
	if !errors.Is(err, err) {
		t.Fatal("unexpected error type")
	}
}

func TestIfaceLoggerFallsBackOnLogFileFailure(t *testing.T) {
	dir := t.TempDir()
	conflict := filepath.Join(dir, "conflict")
	if err := os.WriteFile(conflict, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_LLM_LOG", filepath.Join(conflict, "log.txt"))
	llm.ResetIfaceLoggerForTest()

	fp := &fakeProvider{
		models:     []llm.Model{{ID: "fake-1"}},
		convertReq: func(r *llm.ChatRequest) ([]byte, error) { return []byte("{}"), nil },
		convertChunk: func(b []byte) (*llm.StreamChunk, bool, error) {
			return &llm.StreamChunk{}, true, nil
		},
	}
	proto := &fakeProtocol{streamItems: []protocol.StreamItem{}}
	c := llm.NewCore(fp, proto)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := c.StreamChat(ctx, &llm.ChatRequest{
		Model:    "fake-1",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	for range ch {
	}
	cancel()
}
