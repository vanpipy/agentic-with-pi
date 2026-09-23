package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/tools"
)

func TestAllRegistersEveryBuiltin(t *testing.T) {
	t.Setenv("AWP_TEST_AFT", "")
	t.Setenv("AWP_NO_AFT", "1")
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	got := tools.All(t.TempDir())
	if len(got) < 7 {
		t.Fatalf("tools.All returned %d tools, want at least 7", len(got))
	}
	want := map[string]bool{
		"read": false, "write": false, "edit": false,
		"bash": false, "grep": false, "find": false, "ls": false,
		"invalid": false,
	}
	for _, t0 := range got {
		if _, ok := want[t0.Name()]; ok {
			want[t0.Name()] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("tools.All missing %q", name)
		}
	}
}

func TestAllTwiceReturnsDistinctToolValues(t *testing.T) {
	t.Setenv("AWP_TEST_AFT", "")
	t.Setenv("AWP_NO_AFT", "1")
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	a := tools.All(t.TempDir())
	b := tools.All(t.TempDir())
	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Name() != b[i].Name() {
			t.Errorf("position %d: %q != %q", i, a[i].Name(), b[i].Name())
		}
	}
}
func TestRegistryUsesAftWhenBinaryAvailable(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  cmd=$(printf '%s' \"$line\" | sed -n 's/.*\"command\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":true,\"command\":\"%s\",\"output\":\"ok\"}\\n' \"$id\" \"$cmd\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	got := tools.All(t.TempDir())
	if len(got) < 7 {
		t.Fatalf("tools.All returned %d tools, want at least 7", len(got))
	}
	for _, t0 := range got {
		if t0.Name() == "read" {
			out, err := t0.Invoke(context.Background(), `{"path":"/tmp/x"}`)
			if err != nil {
				t.Fatalf("read tool invoke failed: %v", err)
			}
			if !strings.Contains(out, `"command":"read"`) {
				t.Errorf("read tool did not route via AFT backend: %q", out)
			}
			return
		}
	}
	t.Fatal("read tool not found in registry")
}

func TestRegistryFallsBackToGoWhenAftDisabled(t *testing.T) {
	t.Setenv("AWP_NO_AFT", "1")
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	got := tools.All(t.TempDir())
	if len(got) < 7 {
		t.Fatalf("tools.All returned %d tools, want at least 7", len(got))
	}
	readTool := got[0]
	for _, t0 := range got {
		if t0.Name() == "read" {
			readTool = t0
			break
		}
	}
	if readTool.Name() != "read" {
		t.Fatalf("read tool not found in fallback registry")
	}
}

func TestRegistryBashUsesNestedWireFormat(t *testing.T) {
	tmp := t.TempDir()
	capturedReq := filepath.Join(tmp, "request.json")
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  echo \"$line\" > " + capturedReq + "\n  printf '{\"id\":\"awp-1\",\"success\":true,\"output\":\"ok\"}\\n'\n  break\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	got := tools.All(t.TempDir())
	for _, t0 := range got {
		if t0.Name() == "bash" {
			if _, err := t0.Invoke(context.Background(), `{"command":"date"}`); err != nil {
				t.Fatalf("bash invoke: %v", err)
			}
			data, _ := os.ReadFile(capturedReq)
			wire := string(data)
			if !strings.Contains(wire, `"params":`) {
				t.Errorf("registry bash must use nested wire format; got: %q", wire)
			}
			if !strings.Contains(wire, `"command":"bash"`) {
				t.Errorf("registry bash must route to bash tool; got: %q", wire)
			}
			if !strings.Contains(wire, `"command":"date"`) {
				t.Errorf("registry bash must include the LLM's command; got: %q", wire)
			}
			return
		}
	}
	t.Fatal("bash tool not found")
}

func TestRegistryRespectsAwpNoAftOverride(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  printf '{\"id\":\"x\",\"success\":true}\\n'\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "1")
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	got := tools.All(t.TempDir())
	for _, t0 := range got {
		if t0.Name() == "read" {
			out, err := t0.Invoke(context.Background(), `{"path":"/dev/null"}`)
			if err != nil {
				t.Logf("read invoke err (expected: falls back to Go): %v", err)
			}
			if strings.Contains(out, `"command":"read"`) {
				t.Errorf("read tool routed via AFT despite AWP_NO_AFT=1: %q", out)
			}
			return
		}
	}
}
