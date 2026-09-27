package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/tools"
)

func TestAftBackendCallMarshalFailureOnChan(t *testing.T) {
	bin := writeFakeAftAlwaysSuccess(t, t.TempDir())
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	ch := make(chan int)
	_, err = backend.Call("read", map[string]any{"path": ch})
	if err == nil {
		t.Fatal("expected marshal error for chan param")
	}
	if !strings.Contains(err.Error(), "aft marshal") {
		t.Errorf("error = %q, want 'aft marshal' prefix", err.Error())
	}
}

func TestAftBackendCallNestedMarshalFailureOnChan(t *testing.T) {
	bin := writeFakeAftAlwaysSuccess(t, t.TempDir())
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	ch := make(chan int)
	_, err = backend.CallNested("bash", map[string]any{"command": ch})
	if err == nil {
		t.Fatal("expected marshal error for chan param")
	}
	if !strings.Contains(err.Error(), "aft marshal") {
		t.Errorf("error = %q, want 'aft marshal' prefix", err.Error())
	}
}

func TestAftBackendCallStripsCommandAndIDFromParams(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "aft")
	capturedReq := filepath.Join(t.TempDir(), "req.json")
	script := "#!/bin/sh\nwhile read -r line; do\n  echo \"$line\" > " + capturedReq + "\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":true,\"output\":\"ok\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	if _, err := backend.Call("read", map[string]any{
		"path":    "/x",
		"command": "should-be-stripped",
		"id":      "should-be-stripped",
	}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(capturedReq)
	wire := string(data)
	if strings.Contains(wire, "should-be-stripped") {
		t.Errorf("params 'command' should be stripped from non-nested call; got %q", wire)
	}
}

func TestSetAftCrashReporterWiresIntoExistingInst(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":true,\"output\":\"ok\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_NO_AFT", "")
	t.Setenv("AWP_TEST_AFT", bin)

	backend, ok := tools.AftBackendForTest()
	if !ok {
		t.Fatal("expected AftBackendForTest to succeed with AWP_TEST_AFT set")
	}
	t.Cleanup(func() { _ = backend.Close() })

	tools.SetAftCrashReporter(func(string) {})
	if backend.OnCrash == nil {
		t.Error("SetAftCrashReporter should wire OnCrash into the cached instance")
	}
}

func TestSetAftCrashReporterOverrideNilAndSet(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	tools.SetAftCrashReporter(nil)
	tools.SetAftCrashReporter(func(string) {})
}

func TestInitAftBackendUsesLookPathWhenEnvEmpty(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	t.Setenv("AWP_NO_AFT", "")
	t.Setenv("AWP_TEST_AFT", "")
	emptyDir := t.TempDir()
	t.Setenv("PATH", emptyDir)

	_, ok := tools.AftBackendForTest()
	if ok {
		t.Error("expected AftBackendForTest to fail when 'aft' is not on PATH")
	}
}

func TestNewAftBackendFailsWithEmptyPath(t *testing.T) {
	if _, err := tools.NewAftBackend(""); err == nil {
		t.Fatal("expected error when binary path is empty")
	}
}

func TestWriteAftWrapperExposesSchema(t *testing.T) {
	tool := tools.WriteAftForTest(nil)
	props, ok := tool.Parameters()["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	if _, ok := props["intent"]; !ok {
		t.Error("write wrapper missing intent field")
	}
	if _, ok := props["accept_large_output"]; !ok {
		t.Error("write wrapper missing accept_large_output field")
	}
	if _, ok := props["content"]; !ok {
		t.Error("write wrapper missing content field")
	}
}

func TestEditAftWrapperExposesSchema(t *testing.T) {
	tool := tools.EditAftForTest(nil)
	props, ok := tool.Parameters()["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	if _, ok := props["old_string"]; !ok {
		t.Error("edit wrapper missing old_string field")
	}
	if _, ok := props["new_string"]; !ok {
		t.Error("edit wrapper missing new_string field")
	}
}

func TestBashAftNestedWrapperExposesSchema(t *testing.T) {
	tool := tools.BashAftForTest(nil)
	props, ok := tool.Parameters()["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	if _, ok := props["intent"]; !ok {
		t.Error("bash wrapper missing intent field")
	}
	if _, ok := props["accept_large_output"]; !ok {
		t.Error("bash wrapper missing accept_large_output field")
	}
}

func TestAftBackendCallNumberingIsMonotonic(t *testing.T) {
	bin := writeFakeAftAlwaysSuccess(t, t.TempDir())
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	for i := 0; i < 3; i++ {
		if _, err := backend.Call("echo", map[string]any{"i": i}); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
}

func TestAftBackendCallNestedInvokesWithID(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "aft")
	capturedReq := filepath.Join(t.TempDir(), "req.json")
	script := "#!/bin/sh\nwhile read -r line; do\n  echo \"$line\" > " + capturedReq + "\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":true,\"output\":\"ok\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	if _, err := backend.CallNested("bash", map[string]any{"command": "echo"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(capturedReq)
	wire := string(data)
	var req map[string]any
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatalf("invalid request JSON: %v\n%s", err, wire)
	}
	if id, _ := req["id"].(string); !strings.HasPrefix(id, "awp-") {
		t.Errorf("id = %q, want awp- prefix", id)
	}
}

func TestResetAftBackendForTestClearsInst(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\ncat\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_NO_AFT", "")
	t.Setenv("AWP_TEST_AFT", bin)

	backend, ok := tools.AftBackendForTest()
	if !ok {
		t.Fatal("backend should init via AWP_TEST_AFT")
	}

	tools.ResetAftBackendForTest()
	t.Cleanup(func() { _ = backend.Close() })

	_, ok2 := tools.AftBackendForTest()
	if !ok2 {
		t.Error("after reset, AftBackendForTest should re-init and succeed")
	}
}

func TestAftBackendCallPropagatesAnyErrorAsFailure(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":false,\"message\":\"tool error\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	_, err = backend.Call("anyTool", nil)
	if err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(err.Error(), "anyTool") {
		t.Errorf("error = %q, want tool name", err.Error())
	}
}

func TestAftBackendCallPropagatesTransportErrorAfterStdinClose(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "aft")
	script := "#!/bin/sh\nread -r line\nkill -9 $$\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	backend, err := tools.NewAftBackend(bin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })

	_, err = backend.Call("echo", map[string]any{"x": 1})
	if err == nil {
		t.Fatal("expected transport error after AFT crash")
	}
	if !errors.Is(err, os.ErrClosed) && !strings.Contains(err.Error(), "aft read") {
		t.Errorf("error = %q, want read/closed prefix", err.Error())
	}
}

func TestReadAftWrapperExposesRequiredPath(t *testing.T) {
	tool := tools.ReadAftForTest(nil)
	schema := tool.Parameters()
	req, _ := schema["required"].([]string)
	found := false
	for _, r := range req {
		if r == "path" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("read wrapper should require 'path'; got %v", req)
	}
}

func TestAftToolWrapperNamePropagates(t *testing.T) {
	for name, fn := range map[string]func(*tools.AftBackend) interface {
		Name() string
		Invoke(context.Context, string) (string, error)
	}{
		"read": func(b *tools.AftBackend) interface {
			Name() string
			Invoke(context.Context, string) (string, error)
		} {
			return tools.ReadAftForTest(b)
		},
		"write": func(b *tools.AftBackend) interface {
			Name() string
			Invoke(context.Context, string) (string, error)
		} {
			return tools.WriteAftForTest(b)
		},
		"edit_match": func(b *tools.AftBackend) interface {
			Name() string
			Invoke(context.Context, string) (string, error)
		} {
			return tools.EditAftForTest(b)
		},
		"bash": func(b *tools.AftBackend) interface {
			Name() string
			Invoke(context.Context, string) (string, error)
		} {
			return tools.BashAftForTest(b)
		},
	} {
		tool := fn(nil)
		if tool.Name() != name {
			t.Errorf("wrapper name = %q, want %q", tool.Name(), name)
		}
	}
}
