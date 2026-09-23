package tools_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/tools"
)

func writeFakeAftSuccess(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  cmd=$(printf '%s' \"$line\" | sed -n 's/.*\"command\":\"\\([^\"]*\\)\".*/\\1/p')\n  rest=$(printf '%s' \"$line\" | sed 's/.*\"command\":\"[^\"]*\",\\(.*\\)}$/\\1/')\n  printf '{\"id\":\"%s\",\"success\":true,\"command\":\"%s\",%s}\\n' \"$id\" \"$cmd\" \"$rest\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestAftBackendNDJSONRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	bin := writeFakeAftSuccess(t, tmp)

	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	out, err := backend.Call("echo", map[string]any{"message": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"success":true`) {
		t.Errorf("response missing success:true: %q", out)
	}
	if !strings.Contains(out, `"hi"`) {
		t.Errorf("response missing payload: %q", out)
	}
}

func TestAftBackendCallPropagatesFailure(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nread -r line\necho '{\"id\":\"awp-1\",\"success\":false,\"message\":\"nope\"}'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	_, err = backend.Call("anything", map[string]any{"x": 1})
	if err == nil {
		t.Fatal("expected error when aft returns success:false")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error = %q, want to contain 'nope'", err.Error())
	}
}

func TestAftBackendConcurrentCallsAreSerialized(t *testing.T) {
	tmp := t.TempDir()
	bin := writeFakeAftSuccess(t, tmp)

	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, err := backend.Call("echo", map[string]any{"i": i})
			if err != nil {
				t.Errorf("call %d: %v", i, err)
				return
			}
			if !strings.Contains(out, `"success":true`) {
				t.Errorf("call %d bad response: %q", i, out)
			}
		}(i)
	}
	wg.Wait()
}

func TestAftBackendCloseIsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\ncat\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := backend.Close(); err != nil && !errors.Is(err, context.Canceled) {
		t.Logf("second Close returned (acceptable): %v", err)
	}
}
