package tools_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/tools"
)

func writeFakeAftEchoOne(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "aft")
	script := "#!/bin/sh\nread -r line\nid=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\nprintf '{\"id\":\"%s\",\"success\":true,\"output\":\"ok\"}\\n' \"$id\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestSetAftCrashReporterNilIsAccepted(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	tools.SetAftCrashReporter(nil)
	tools.SetAftCrashReporter(func(string) {})
}

func TestSetAftCrashReporterAfterBackendCreated(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":true,\"output\":\"ok\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	var calls atomic.Int32
	tools.SetAftCrashReporter(func(string) { calls.Add(1) })

	if _, err := backend.Call("echo", map[string]any{"x": 1}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Errorf("OnCrash should not fire on success; got %d", calls.Load())
	}
}

func TestInitAftBackendDisabledByEnv(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	t.Setenv("AWP_NO_AFT", "1")
	t.Setenv("AWP_TEST_AFT", "")

	_, ok := tools.AftBackendForTest()
	if ok {
		t.Error("expected AftBackendForTest to fail when AWP_NO_AFT=1")
	}
}

func TestInitAftBackendUsesAWP_TEST_AFT(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	tmp := t.TempDir()
	bin := writeFakeAftEchoOne(t, tmp)
	t.Setenv("AWP_NO_AFT", "")
	t.Setenv("AWP_TEST_AFT", bin)

	backend, ok := tools.AftBackendForTest()
	if !ok {
		t.Fatal("expected AftBackendForTest to succeed with AWP_TEST_AFT set")
	}
	t.Cleanup(func() { _ = backend.Close() })

	out, err := backend.Call("echo", map[string]any{"x": 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"success":true`) {
		t.Errorf("got %q, want success:true", out)
	}
}

func TestInitAftBackendFailsWhenBinaryMissing(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	t.Setenv("AWP_NO_AFT", "")
	t.Setenv("AWP_TEST_AFT", "/nonexistent/aft-binary-xyz")

	_, ok := tools.AftBackendForTest()
	if ok {
		t.Error("expected AftBackendForTest to fail when AWP_TEST_AFT points at missing binary")
	}
}

func TestNewAftBackendFailsOnBadBinary(t *testing.T) {
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "aft")
	if err := os.WriteFile(bad, []byte("not a real binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := tools.NewAftBackend(bad); err == nil {
		t.Fatal("expected NewAftBackend to fail when binary cannot start")
	}
}

func TestNewAftBackendFailsOnMissingBinary(t *testing.T) {
	if _, err := tools.NewAftBackend("/nonexistent/aft-binary-xyz"); err == nil {
		t.Fatal("expected NewAftBackend to fail when binary path is missing")
	}
}

func TestAftBackendCallAfterCloseErrors(t *testing.T) {
	tmp := t.TempDir()
	bin := writeFakeAftEchoOne(t, tmp)
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}

	if err := backend.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}

	_, err = backend.Call("echo", nil)
	if err == nil {
		t.Fatal("expected error when calling closed backend")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("error = %q, want to mention 'closed'", err.Error())
	}
}

func TestAftBackendCloseIdempotentWithRace(t *testing.T) {
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

	done := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() { done <- backend.Close() }()
	}
	for i := 0; i < 4; i++ {
		if err := <-done; err != nil {
			t.Logf("concurrent Close returned (acceptable): %v", err)
		}
	}
}

func TestAftBackendCallNestedPreservesParamsField(t *testing.T) {
	tmp := t.TempDir()
	capturedReq := filepath.Join(tmp, "request.json")
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  echo \"$line\" > " + capturedReq + "\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":true,\"output\":\"ok\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	if _, err := backend.CallNested("bash", map[string]any{"command": "echo hi"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(capturedReq)
	if err != nil {
		t.Fatal(err)
	}
	wire := string(data)
	if !strings.Contains(wire, `"params":`) {
		t.Errorf("CallNested should use nested params; got %q", wire)
	}
	if !strings.Contains(wire, `"command":"echo hi"`) {
		t.Errorf("params.command missing; got %q", wire)
	}
}

func TestAftBackendCallFiltersResponsesByID(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"stale\",\"success\":true,\"output\":\"old\"}\\n'\n  printf '{\"id\":\"%s\",\"success\":true,\"output\":\"new\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	out, err := backend.Call("echo", map[string]any{"x": 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"output":"new"`) {
		t.Errorf("Call should skip stale responses; got %q", out)
	}
	if strings.Contains(out, `"output":"old"`) {
		t.Errorf("Call returned stale payload: %q", out)
	}
}

func TestAftBackendCallSkipsUnparsableLines(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf 'this is not json\\n'\n  printf '{\"id\":\"%s\",\"success\":true,\"output\":\"ok\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	out, err := backend.Call("echo", map[string]any{"x": 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"success":true`) {
		t.Errorf("got %q, want success response after skipping unparsable line", out)
	}
}

func TestAftBackendCallReadErrorInvokesOnCrash(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nread -r line\nkill -9 $$\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	var calls atomic.Int32
	backend.OnCrash = func(string) { calls.Add(1) }

	_, err = backend.Call("echo", map[string]any{"x": 1})
	if err == nil {
		t.Fatal("expected error when worker dies")
	}
	if !strings.Contains(err.Error(), "aft read") {
		t.Errorf("error = %q, want aft read prefix", err.Error())
	}
	if calls.Load() == 0 {
		t.Errorf("OnCrash should fire when read fails; got %d calls", calls.Load())
	}
}
