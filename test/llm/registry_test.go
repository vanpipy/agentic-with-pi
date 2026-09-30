package llm_test

import (
	"context"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
)

type stubProvider struct {
	name   string
	models []llm.Model
}

func (s *stubProvider) Name() string                                    { return s.name }
func (s *stubProvider) BaseURL() string                                 { return "" }
func (s *stubProvider) Path() string                                    { return "" }
func (s *stubProvider) Headers() map[string]string                      { return nil }
func (s *stubProvider) ConvertRequest(*llm.ChatRequest) ([]byte, error) { return nil, nil }
func (s *stubProvider) ConvertResponse([]byte) (*llm.StreamChunk, bool, error) {
	return nil, false, nil
}
func (s *stubProvider) RecoverRequest(*llm.ChatRequest, error) bool {
	return false
}
func (s *stubProvider) Models() []llm.Model { return s.models }
func (s *stubProvider) SupportsCacheControl(model string) bool {
	return false
}
func (s *stubProvider) ContextWindow(model string) int   { return 0 }
func (s *stubProvider) MaxOutputTokens(model string) int { return 0 }
func (s *stubProvider) AvailableReasoningEfforts(model string) []string {
	return nil
}
func (s *stubProvider) AvailableServiceTiers(model string) []string {
	return nil
}
func (s *stubProvider) BetaHeaders(model string) []string { return nil }
func (s *stubProvider) ModelCapabilities(model string) llm.ModelCapabilities {
	return llm.ModelCapabilities{ID: model}
}
func (s *stubProvider) NativeCompactMode(model string) string   { return "" }
func (s *stubProvider) NativeCompactThreshold(model string) int { return 0 }
func (s *stubProvider) NativeCompactCapabilities(model string) llm.NativeCompactionCapabilities {
	return llm.NativeCompactionCapabilities{Kind: llm.KindUnsupported}
}
func (s *stubProvider) NativeCompact(ctx context.Context, model string, msgs []llm.Message, summaryText, encryptedContent string) (llm.NativeCompactionResult, error) {
	return llm.NativeCompactionResult{}, llm.ErrNativeCompactionUnsupported
}

func (s *stubProvider) CompleteSplit(systemPrompt string, model string) []llm.ContentBlock {
	_ = systemPrompt
	_ = model
	return nil
}

func TestRegisterAndGet(t *testing.T) {
	r := llm.NewRegistry()
	p := &stubProvider{name: "foo"}
	if err := r.Register("foo", p); err != nil {
		t.Fatal(err)
	}
	got, ok := r.Get("foo")
	if !ok {
		t.Fatal("Get(foo) = false")
	}
	if got != p {
		t.Errorf("Get returned different provider")
	}
}

func TestRegisterRejectsEmptyID(t *testing.T) {
	r := llm.NewRegistry()
	if err := r.Register("", &stubProvider{}); err == nil {
		t.Error("expected error for empty id")
	}
}

func TestRegisterRejectsNil(t *testing.T) {
	r := llm.NewRegistry()
	if err := r.Register("x", nil); err == nil {
		t.Error("expected error for nil provider")
	}
}

func TestRegisterRejectsDuplicate(t *testing.T) {
	r := llm.NewRegistry()
	_ = r.Register("x", &stubProvider{})
	if err := r.Register("x", &stubProvider{}); err == nil {
		t.Error("expected error for duplicate")
	}
}

func TestMustRegisterDefault(t *testing.T) {
	r := llm.NewRegistry()
	p := &stubProvider{}
	r.MustRegisterDefault(p)
	got, ok := r.Default()
	if !ok {
		t.Fatal("Default() = false")
	}
	if got != p {
		t.Error("Default() returned different provider")
	}
}

func TestDefaultKeyIsMinimax(t *testing.T) {
	r := llm.NewRegistry()
	_, ok := r.Default()
	if ok {
		t.Error("empty registry should not have default")
	}
}

func TestNamesReturnsSorted(t *testing.T) {
	r := llm.NewRegistry()
	_ = r.Register("c", &stubProvider{})
	_ = r.Register("a", &stubProvider{})
	_ = r.Register("b", &stubProvider{})
	names := r.Names()
	want := []string{"a", "b", "c"}
	if len(names) != 3 {
		t.Fatalf("len = %d, want 3", len(names))
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, n, want[i])
		}
	}
}

func TestFindModel(t *testing.T) {
	r := llm.NewRegistry()
	_ = r.Register("a", &stubProvider{name: "a", models: []llm.Model{{ID: "a-m", Vendor: "a"}}})
	_ = r.Register("b", &stubProvider{name: "b", models: []llm.Model{{ID: "b-m", Vendor: "b"}}})

	m, p, ok := r.FindModel("b-m")
	if !ok {
		t.Fatal("FindModel(b-m) = false")
	}
	if m.Vendor != "b" {
		t.Errorf("Vendor = %q, want b", m.Vendor)
	}
	if p.Name() != "b" {
		t.Errorf("provider name = %q, want b", p.Name())
	}

	if _, _, ok := r.FindModel("missing"); ok {
		t.Error("FindModel(missing) = true, want false")
	}
}

func TestAllModels(t *testing.T) {
	r := llm.NewRegistry()
	_ = r.Register("a", &stubProvider{models: []llm.Model{{ID: "a1"}, {ID: "a2"}}})
	_ = r.Register("b", &stubProvider{models: []llm.Model{{ID: "b1"}}})
	all := r.AllModels()
	if len(all) != 3 {
		t.Errorf("AllModels = %d, want 3", len(all))
	}
}
