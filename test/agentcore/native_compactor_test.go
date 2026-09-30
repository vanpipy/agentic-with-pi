package agentcore_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/compact"
	"github.com/vanpiyp/awp/internal/llm"
)

type fakeProvider struct {
	caps    llm.NativeCompactionCapabilities
	result  llm.NativeCompactionResult
	err     error
	called  bool
	gotMsgs []llm.Message
}

func (f *fakeProvider) Name() string               { return "fake" }
func (f *fakeProvider) BaseURL() string            { return "" }
func (f *fakeProvider) Path() string               { return "" }
func (f *fakeProvider) Headers() map[string]string { return nil }
func (f *fakeProvider) ConvertRequest(req *llm.ChatRequest) ([]byte, error) {
	return nil, nil
}
func (f *fakeProvider) ConvertResponse(data []byte) (*llm.StreamChunk, bool, error) {
	return nil, true, io.EOF
}
func (f *fakeProvider) RecoverRequest(req *llm.ChatRequest, err error) bool { return false }
func (f *fakeProvider) Models() []llm.Model                                 { return nil }
func (f *fakeProvider) SupportsCacheControl(model string) bool              { return false }
func (f *fakeProvider) ContextWindow(model string) int                      { return 0 }
func (f *fakeProvider) MaxOutputTokens(model string) int                    { return 0 }
func (f *fakeProvider) AvailableReasoningEfforts(model string) []string     { return nil }
func (f *fakeProvider) AvailableServiceTiers(model string) []string         { return nil }
func (f *fakeProvider) BetaHeaders(model string) []string                   { return nil }
func (f *fakeProvider) ModelCapabilities(model string) llm.ModelCapabilities {
	return llm.ModelCapabilities{}
}
func (f *fakeProvider) NativeCompactMode(model string) string   { return f.caps.Mode }
func (f *fakeProvider) NativeCompactThreshold(model string) int { return f.caps.Threshold }
func (f *fakeProvider) NativeCompactCapabilities(model string) llm.NativeCompactionCapabilities {
	return f.caps
}
func (f *fakeProvider) NativeCompact(ctx context.Context, model string, msgs []llm.Message, summaryText, encryptedContent string) (llm.NativeCompactionResult, error) {
	f.called = true
	f.gotMsgs = msgs
	return f.result, f.err
}

func (f *fakeProvider) CompleteSplit(systemPrompt string, model string) []llm.ContentBlock {
	_ = systemPrompt
	_ = model
	return nil
}

var _ llm.Provider = (*fakeProvider)(nil)

type fakeRunner struct {
	out     []llm.Message
	ok      bool
	called  bool
	gotMsgs []llm.Message
}

func (r *fakeRunner) Apply(ctx context.Context, msgs []llm.Message) ([]llm.Message, bool) {
	r.called = true
	r.gotMsgs = msgs
	return r.out, r.ok
}

func (r *fakeRunner) SetMessages(msgs []llm.Message) {}

var _ compact.CompactRunner = (*fakeRunner)(nil)

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func sampleMessages() []llm.Message {
	return []llm.Message{
		{Role: "system", Content: "you are a helpful assistant"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
}

func TestNativeCompactor_KindUnsupported_RoutesToFallback(t *testing.T) {
	p := &fakeProvider{caps: llm.NativeCompactionCapabilities{Kind: llm.KindUnsupported}}
	runner := &fakeRunner{
		out: []llm.Message{{Role: "system", Content: "summary"}},
		ok:  true,
	}
	n := &compact.NativeCompactor{
		Provider: p,
		Model:    "m",
		Fallback: runner,
		Logger:   newDiscardLogger(),
	}
	out, ok, err := n.Compact(context.Background(), sampleMessages())
	if err != nil {
		t.Fatalf("Compact err = %v", err)
	}
	if !ok {
		t.Errorf("ok = false, want true (fallback should succeed)")
	}
	if len(out) != 1 || out[0].Content != "summary" {
		t.Errorf("out = %+v, want single summary message", out)
	}
	if !runner.called {
		t.Error("fallback runner was not called")
	}
	if p.called {
		t.Error("provider.NativeCompact was called for KindUnsupported, want skipped")
	}
}

func TestNativeCompactor_KindNative_CallsProviderAndReturnsSummary(t *testing.T) {
	p := &fakeProvider{
		caps:   llm.NativeCompactionCapabilities{Kind: llm.KindNative, Mode: "auto", Threshold: 1000},
		result: llm.NativeCompactionResult{Summary: "native-summary"},
	}
	n := &compact.NativeCompactor{
		Provider: p,
		Model:    "m",
		Logger:   newDiscardLogger(),
	}
	out, ok, err := n.Compact(context.Background(), sampleMessages())
	if err != nil {
		t.Fatalf("Compact err = %v", err)
	}
	if !ok {
		t.Error("ok = false, want true")
	}
	if len(out) != 1 {
		t.Fatalf("out len = %d, want 1", len(out))
	}
	if out[0].Role != "system" || out[0].Content != "native-summary" {
		t.Errorf("out[0] = %+v, want system/native-summary", out[0])
	}
	if !p.called {
		t.Error("provider.NativeCompact was not called for KindNative")
	}
}

func TestNativeCompactor_KindNative_ProviderErrorFallsBackToRunner(t *testing.T) {
	p := &fakeProvider{
		caps: llm.NativeCompactionCapabilities{Kind: llm.KindNative},
		err:  errors.New("native failed"),
	}
	runner := &fakeRunner{
		out: []llm.Message{{Role: "system", Content: "client-fallback"}},
		ok:  true,
	}
	n := &compact.NativeCompactor{
		Provider: p,
		Model:    "m",
		Fallback: runner,
		Logger:   newDiscardLogger(),
	}
	out, ok, err := n.Compact(context.Background(), sampleMessages())
	if err != nil {
		t.Fatalf("Compact err = %v", err)
	}
	if !ok || len(out) != 1 || out[0].Content != "client-fallback" {
		t.Errorf("out = %+v, want single client-fallback", out)
	}
	if !runner.called {
		t.Error("fallback runner was not called after provider error")
	}
}

func TestNativeCompactor_NilProviderReturnsInputUnchanged(t *testing.T) {
	var n *compact.NativeCompactor
	msgs := sampleMessages()
	out, ok, err := n.Compact(context.Background(), msgs)
	if err != nil {
		t.Fatalf("Compact err = %v", err)
	}
	if ok {
		t.Error("ok = true, want false for nil receiver")
	}
	if len(out) != len(msgs) {
		t.Errorf("out len = %d, want %d (unchanged)", len(out), len(msgs))
	}
}
