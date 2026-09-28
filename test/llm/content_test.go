package llm_test

import (
	"encoding/json"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
)

func TestRoleConstants(t *testing.T) {
	if llm.RoleUser != "user" {
		t.Fatalf("RoleUser = %q, want %q", llm.RoleUser, "user")
	}
	if llm.RoleAssistant != "assistant" {
		t.Fatalf("RoleAssistant = %q, want %q", llm.RoleAssistant, "assistant")
	}
	if llm.RoleTool != "tool" {
		t.Fatalf("RoleTool = %q, want %q", llm.RoleTool, "tool")
	}
	if llm.RoleSystem != "system" {
		t.Fatalf("RoleSystem = %q, want %q", llm.RoleSystem, "system")
	}
}

func TestContentBlockSealedInterface(t *testing.T) {
	var b llm.ContentBlock
	b = llm.ContentText{Text: "hello"}
	b = llm.ContentImage{MediaType: "image/png", Data: "base64data"}
	b = llm.ContentToolUse{ID: "1", Name: "bash", Input: []byte(`{}`)}
	b = llm.ContentToolResult{ToolUseID: "1", Content: nil, IsError: false}
	b = llm.ContentThinking{Text: "thinking", Signature: "sig"}
	_ = b
}

func TestContentTextPayload(t *testing.T) {
	cc := llm.CacheEphemeral()
	cb := llm.ContentText{Text: "hi", CacheControl: cc}
	if cb.Text != "hi" {
		t.Fatalf("Text = %q, want %q", cb.Text, "hi")
	}
	if cb.CacheControl != cc {
		t.Fatalf("CacheControl = %v, want %v", cb.CacheControl, cc)
	}
	if !cb.CacheEligible() {
		t.Fatalf("CacheEligible() = false, want true")
	}
}

func TestContentImagePayload(t *testing.T) {
	cb := llm.ContentImage{MediaType: "image/jpeg", Data: "abc"}
	if cb.MediaType != "image/jpeg" {
		t.Fatalf("MediaType = %q, want %q", cb.MediaType, "image/jpeg")
	}
	if cb.Data != "abc" {
		t.Fatalf("Data = %q, want %q", cb.Data, "abc")
	}
	if cb.CacheEligible() {
		t.Fatalf("CacheEligible() = true, want false")
	}
}

func TestContentToolUsePayload(t *testing.T) {
	cb := llm.ContentToolUse{ID: "tool-1", Name: "bash", Input: []byte(`{"cmd":"ls"}`), Signature: "gsig"}
	if cb.ID != "tool-1" {
		t.Fatalf("ID = %q, want %q", cb.ID, "tool-1")
	}
	if cb.Name != "bash" {
		t.Fatalf("Name = %q, want %q", cb.Name, "bash")
	}
	if string(cb.Input) != `{"cmd":"ls"}` {
		t.Fatalf("Input = %q, want %q", string(cb.Input), `{"cmd":"ls"}`)
	}
	if cb.Signature != "gsig" {
		t.Fatalf("Signature = %q, want %q", cb.Signature, "gsig")
	}
	if cb.CacheEligible() {
		t.Fatalf("CacheEligible() = true, want false")
	}
}

func TestContentToolResultPayload(t *testing.T) {
	cb := llm.ContentToolResult{
		ToolUseID: "tool-1",
		Content:   []llm.ContentBlock{llm.ContentText{Text: "out"}},
		IsError:   true,
	}
	if cb.ToolUseID != "tool-1" {
		t.Fatalf("ToolUseID = %q, want %q", cb.ToolUseID, "tool-1")
	}
	if len(cb.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(cb.Content))
	}
	if !cb.IsError {
		t.Fatalf("IsError = false, want true")
	}
	if cb.CacheEligible() {
		t.Fatalf("CacheEligible() = true, want false")
	}
}

func TestContentThinkingPayload(t *testing.T) {
	cb := llm.ContentThinking{Text: "deep thought", Signature: "tsig"}
	if cb.Text != "deep thought" {
		t.Fatalf("Text = %q, want %q", cb.Text, "deep thought")
	}
	if cb.Signature != "tsig" {
		t.Fatalf("Signature = %q, want %q", cb.Signature, "tsig")
	}
	if cb.CacheEligible() {
		t.Fatalf("CacheEligible() = true, want false")
	}
}

func TestContentBlockCacheEligibleMatrix(t *testing.T) {
	cases := []struct {
		name string
		cb   llm.ContentBlock
		want bool
	}{
		{"text", llm.ContentText{Text: "x"}, true},
		{"image", llm.ContentImage{MediaType: "image/png"}, false},
		{"tool_use", llm.ContentToolUse{ID: "1"}, false},
		{"tool_result", llm.ContentToolResult{ToolUseID: "1"}, false},
		{"thinking", llm.ContentThinking{Text: "t"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cb.CacheEligible(); got != tc.want {
				t.Fatalf("CacheEligible() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestContentTextJSONRoundTrip(t *testing.T) {
	cb := llm.ContentText{Text: "hello"}
	data, err := json.Marshal(cb)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	want := `{"Text":"hello","CacheControl":null}`
	if string(data) != want {
		t.Fatalf("Marshal = %s, want %s", string(data), want)
	}
}
