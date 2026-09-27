package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/tools"
)

type ctxPropagationKey struct{}

func TestReadFilePropagatesOuterContext(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	tools.ReadCtxProbe = nil
	t.Cleanup(func() { tools.ReadCtxProbe = nil })

	sentinel := "ctx-propagation-marker"
	ctx := context.WithValue(context.Background(), ctxPropagationKey{}, sentinel)

	tool := tools.ReadFile(dir, tools.FileOptions{})
	if _, err := tool.Invoke(ctx, `{"path":"hello.txt","intent":"test"}`); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	if tools.ReadCtxProbe == nil {
		t.Fatal("ReadCtxProbe is nil: inner closure never received a ctx")
	}
	if got := tools.ReadCtxProbe.Value(ctxPropagationKey{}); got != sentinel {
		t.Fatalf("inner ctx did not carry outer sentinel: got=%v want=%v", got, sentinel)
	}
}