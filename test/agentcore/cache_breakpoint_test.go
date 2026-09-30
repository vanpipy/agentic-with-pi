package agentcore_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/cache"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol/anthropic"
)

func TestInjectCacheControl_NoSupportReturnsUnchanged(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "you are a coding assistant"},
		{Role: "user", Content: "hi"},
	}
	got := cache.InjectCacheControl(msgs, func(string) bool { return false }, "m")
	if &got[0] != &msgs[0] {
		t.Fatalf("expected same slice when probe returns false, got new backing array")
	}
}

func TestInjectCacheControl_MarksSystemAndLast(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "you are a coding assistant"},
		{Role: "user", Content: "first turn"},
		{Role: "assistant", Content: "answer 1"},
		{Role: "user", Content: "second turn"},
	}
	got := cache.InjectCacheControl(msgs, func(string) bool { return true }, "m")
	if got[0].CacheControl == nil || got[0].CacheControl.Type != "ephemeral" {
		t.Errorf("system message CacheControl = %+v, want ephemeral", got[0].CacheControl)
	}
	for i := 1; i < len(got)-1; i++ {
		if got[i].CacheControl != nil {
			t.Errorf("middle message[%d] CacheControl = %+v, want nil", i, got[i].CacheControl)
		}
	}
	if got[len(got)-1].CacheControl == nil || got[len(got)-1].CacheControl.Type != "ephemeral" {
		t.Errorf("last message CacheControl = %+v, want ephemeral", got[len(got)-1].CacheControl)
	}
	if got[0].Content != msgs[0].Content {
		t.Errorf("system content mutated: got %q, want %q", got[0].Content, msgs[0].Content)
	}
}

func TestInjectCacheControl_NoSystemLeavesFirstUntouched(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "no system here"},
	}
	got := cache.InjectCacheControl(msgs, func(string) bool { return true }, "m")
	if got[0].CacheControl == nil || got[0].CacheControl.Type != "ephemeral" {
		t.Errorf("expected last message (also first) marked, got %+v", got[0].CacheControl)
	}
}

func TestInjectCacheControl_EmptySliceReturnsUnchanged(t *testing.T) {
	got := cache.InjectCacheControl(nil, func(string) bool { return true }, "m")
	if len(got) != 0 {
		t.Errorf("empty slice: got len %d, want 0", len(got))
	}
}

func TestCacheControlFlowsToAnthropicWire(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "you are a coding assistant"},
		{Role: "user", Content: "hi"},
	}
	msgs = cache.InjectCacheControl(msgs, func(string) bool { return true }, "m")
	ar, err := anthropic.MapToAnthropicRequest(llm.ChatRequest{Model: "m", Messages: msgs, Stream: true, MaxTokens: 64})
	if err != nil {
		t.Fatalf("MapToAnthropicRequest: %v", err)
	}
	body, err := anthropic.BuildAnthropicRequest(ar)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	var parsed struct {
		System []map[string]any `json:"system"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(parsed.System) != 1 {
		t.Fatalf("system blocks = %d, want 1", len(parsed.System))
	}
	cc, ok := parsed.System[0]["cache_control"]
	if !ok {
		t.Fatalf("system block missing cache_control: %s", body)
	}
	m, ok := cc.(map[string]any)
	if !ok {
		t.Fatalf("cache_control not object: %T", cc)
	}
	if m["type"] != "ephemeral" {
		t.Errorf("cache_control.type = %v, want ephemeral", m["type"])
	}
}

func TestCacheControlFlowsToAnthropicWire_TextBlock(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "answer"},
		{Role: "user", Content: "tail"},
	}
	msgs = cache.InjectCacheControl(msgs, func(string) bool { return true }, "m")
	ar, err := anthropic.MapToAnthropicRequest(llm.ChatRequest{Model: "m", Messages: msgs, Stream: true, MaxTokens: 64})
	if err != nil {
		t.Fatalf("MapToAnthropicRequest: %v", err)
	}
	body, err := anthropic.BuildAnthropicRequest(ar)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	if !strings.Contains(string(body), `"cache_control":{"type":"ephemeral"}`) {
		t.Errorf("expected cache_control:ephemeral on tail message; body=%s", body)
	}
}

var _ = context.Background
