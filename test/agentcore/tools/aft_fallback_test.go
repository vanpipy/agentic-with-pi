package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/tools"
)

func writeFakeAftAlwaysSuccess(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  cmd=$(printf '%s' \"$line\" | sed -n 's/.*\"command\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":true,\"command\":\"%s\",\"output\":\"ok\"}\\n' \"$id\" \"$cmd\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func writeFakeAftAlwaysFail(t *testing.T, dir, message string) string {
	t.Helper()
	bin := filepath.Join(dir, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":false,\"message\":\"%s\"}\\n' \"$id\" \"" + message + "\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestAftFallbackFiresWhenBackendReturnsFailure(t *testing.T) {
	tmp := t.TempDir()
	bin := writeFakeAftAlwaysFail(t, tmp, "unknown command: ls")
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	var fallbackCalls atomic.Int32
	var fallbackArgs string
	fallback := func(_ context.Context, argsJSON string) (string, error) {
		fallbackCalls.Add(1)
		fallbackArgs = argsJSON
		return "from-go", nil
	}

	tool := tools.NewAftToolWithFallbackForTest(backend, "ls", "ls desc",
		map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
		fallback)
	out, err := tool.Invoke(context.Background(), `{"path":"/tmp","intent":"foo"}`)
	if err != nil {
		t.Fatalf("expected fallback to recover: %v", err)
	}
	if out != "from-go" {
		t.Errorf("got %q, want fallback output", out)
	}
	if fallbackCalls.Load() != 1 {
		t.Errorf("fallback called %d times, want 1", fallbackCalls.Load())
	}
	if !strings.Contains(fallbackArgs, `"path":"/tmp"`) {
		t.Errorf("fallback did not receive original argsJSON: %q", fallbackArgs)
	}
}

func TestAftFallbackSkippedWhenBackendSucceeds(t *testing.T) {
	tmp := t.TempDir()
	bin := writeFakeAftAlwaysSuccess(t, tmp)
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	var fallbackCalls atomic.Int32
	fallback := func(_ context.Context, argsJSON string) (string, error) {
		fallbackCalls.Add(1)
		return "from-go", nil
	}

	tool := tools.NewAftToolWithFallbackForTest(backend, "read", "read desc",
		map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
		fallback)
	out, err := tool.Invoke(context.Background(), `{"path":"/tmp","intent":"foo"}`)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(out, `"success":true`) {
		t.Errorf("expected AFT output, got %q", out)
	}
	if fallbackCalls.Load() != 0 {
		t.Errorf("fallback should not fire on success; got %d calls", fallbackCalls.Load())
	}
}

func TestAftFallbackSkippedOnTransportCrash(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nkill -9 $$\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}

	var crashCount atomic.Int32
	backend.OnCrash = func(_ string) { crashCount.Add(1) }

	var fallbackCalls atomic.Int32
	fallback := func(_ context.Context, argsJSON string) (string, error) {
		fallbackCalls.Add(1)
		return "from-go", nil
	}

	tool := tools.NewAftToolWithFallbackForTest(backend, "read", "read desc",
		map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
		fallback)
	_, err = tool.Invoke(context.Background(), `{"path":"/tmp","intent":"foo"}`)
	if err == nil {
		t.Fatal("expected transport error after AFT crash")
	}
	if !strings.Contains(err.Error(), "aft ") {
		t.Errorf("expected AFT transport error prefix, got %q", err.Error())
	}
	if fallbackCalls.Load() != 0 {
		t.Errorf("fallback must NOT fire on transport crash; got %d calls", fallbackCalls.Load())
	}
	if crashCount.Load() == 0 {
		t.Errorf("OnCrash callback should have fired")
	}
	_ = backend.Close()
}

func TestAftFallbackNilMeansOriginalErrorReturned(t *testing.T) {
	tmp := t.TempDir()
	bin := writeFakeAftAlwaysFail(t, tmp, "tool failed for reasons")
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	tool := tools.NewAftToolWithFallbackForTest(backend, "read", "read desc",
		map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
		nil)
	_, err = tool.Invoke(context.Background(), `{"path":"/tmp","intent":"foo"}`)
	if err == nil {
		t.Fatal("expected error when AFT fails and no fallback")
	}
	if !strings.Contains(err.Error(), "tool failed for reasons") {
		t.Errorf("got %q, want original AFT error", err.Error())
	}
}

func TestAftFallbackPropagatesGoError(t *testing.T) {
	tmp := t.TempDir()
	bin := writeFakeAftAlwaysFail(t, tmp, "aft-side failure")
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	goErr := errors.New("go-side failure")
	fallback := func(_ context.Context, argsJSON string) (string, error) {
		return "", goErr
	}

	tool := tools.NewAftToolWithFallbackForTest(backend, "read", "read desc",
		map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
		fallback)
	_, err = tool.Invoke(context.Background(), `{"path":"/tmp","intent":"foo"}`)
	if err == nil {
		t.Fatal("expected Go error to propagate")
	}
	if !strings.Contains(err.Error(), "go-side failure") {
		t.Errorf("got %q, want Go error to be returned", err.Error())
	}
}

func TestAftFallbackNestedFiresForBash(t *testing.T) {
	tmp := t.TempDir()
	bin := writeFakeAftAlwaysFail(t, tmp, "bash rejected")
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	var fallbackCalls atomic.Int32
	fallback := func(_ context.Context, argsJSON string) (string, error) {
		fallbackCalls.Add(1)
		var a map[string]any
		_ = json.Unmarshal([]byte(argsJSON), &a)
		cmd, _ := a["command"].(string)
		return "shell-output:" + cmd, nil
	}

	tool := tools.NewAftNestedToolWithFallbackForTest(backend, "bash", "bash desc",
		map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}},
		fallback)
	out, err := tool.Invoke(context.Background(), `{"command":"echo hi","intent":"foo"}`)
	if err != nil {
		t.Fatalf("nested fallback: %v", err)
	}
	if !strings.Contains(out, "shell-output:echo hi") {
		t.Errorf("got %q, want nested fallback output", out)
	}
	if fallbackCalls.Load() != 1 {
		t.Errorf("nested fallback called %d times, want 1", fallbackCalls.Load())
	}
}

func TestAftFallbackNoGoFallbackConfiguredStillReturnsError(t *testing.T) {
	tmp := t.TempDir()
	bin := writeFakeAftAlwaysFail(t, tmp, "no fallback available")
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	readTool := tools.ReadAftForTest(backend)
	_, err = readTool.Invoke(context.Background(), `{"path":"/tmp","intent":"foo"}`)
	if err == nil {
		t.Fatal("expected error when fallback is nil and AFT fails")
	}
	if !strings.Contains(err.Error(), "no fallback available") {
		t.Errorf("got %q, want original error", err.Error())
	}
}
