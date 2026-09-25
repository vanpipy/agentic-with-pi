package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/tools"
)

func TestRegistryWiresGoFallbackForAftLSTools(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":false,\"message\":\"unknown command: ls\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	cwd := t.TempDir()
	got := tools.All(cwd)
	var lsTool interface {
		Invoke(context.Context, string) (string, error)
	}
	for _, t0 := range got {
		if t0.Name() == "ls" {
			if inv, ok := t0.(interface {
				Invoke(context.Context, string) (string, error)
			}); ok {
				lsTool = inv
			}
		}
	}
	if lsTool == nil {
		t.Fatal("ls tool not registered")
	}

	dirPath := filepath.Join(cwd, "fallbackdir")
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := lsTool.Invoke(context.Background(), `{"path":"`+dirPath+`","intent":"check fallback wiring"}`)
	if err != nil {
		t.Fatalf("ls fallback should succeed when AFT rejects: %v", err)
	}
	_ = out
}

func TestRegistryAFTReadFallsBackToGoOnAFTRejection(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":false,\"message\":\"read blocked\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	cwd := t.TempDir()
	target := filepath.Join(cwd, "r.txt")
	if err := os.WriteFile(target, []byte("hello from go"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := tools.All(cwd)
	for _, t0 := range got {
		if t0.Name() == "read" {
			out, err := t0.Invoke(context.Background(), `{"file":"`+target+`","intent":"check read fallback"}`)
			if err != nil {
				t.Fatalf("read fallback should succeed when AFT rejects: %v", err)
			}
			if !strings.Contains(out, "hello from go") {
				t.Errorf("read fallback did not return file contents: %q", out)
			}
			return
		}
	}
	t.Fatal("read tool not registered")
}

func TestRegistryAFTBashFallsBackToGoOnAFTRejection(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":false,\"message\":\"bash denied\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	cwd := t.TempDir()
	got := tools.All(cwd)
	for _, t0 := range got {
		if t0.Name() == "bash" {
			out, err := t0.Invoke(context.Background(), `{"command":"echo bash-fallback","intent":"check bash fallback"}`)
			if err != nil {
				t.Fatalf("bash fallback should succeed when AFT rejects: %v", err)
			}
			if !strings.Contains(out, "bash-fallback") {
				t.Errorf("bash fallback output missing echo result: %q", out)
			}
			return
		}
	}
	t.Fatal("bash tool not registered")
}

func TestRegistryAFTEditMatchFallsBackToGoOnAFTRejection(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":false,\"message\":\"edit_match unavailable\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	cwd := t.TempDir()
	target := filepath.Join(cwd, "edit.txt")
	if err := os.WriteFile(target, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := tools.All(cwd)
	for _, t0 := range got {
		if t0.Name() == "edit_match" {
			args := `{"path":"` + target + `","old_string":"alpha","new_string":"ALPHA","intent":"check edit fallback"}`
			out, err := t0.Invoke(context.Background(), args)
			if err != nil {
				t.Fatalf("edit_match fallback should succeed when AFT rejects: %v", err)
			}
			if !strings.Contains(out, "1 edits") && !strings.Contains(out, "ALPHA") {
				t.Errorf("edit_match fallback output unexpected: %q", out)
			}
			updated, _ := os.ReadFile(target)
			if !strings.Contains(string(updated), "ALPHA") {
				t.Errorf("edit_match fallback did not actually edit file: %q", string(updated))
			}
			return
		}
	}
	t.Fatal("edit_match tool not registered")
}

func TestRegistryFallbackNotFiredWhenAFTSucceeds(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  cmd=$(printf '%s' \"$line\" | sed -n 's/.*\"command\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":true,\"command\":\"%s\",\"output\":\"AFT-OK\"}\\n' \"$id\" \"$cmd\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	cwd := t.TempDir()
	got := tools.All(cwd)
	var found int
	for _, t0 := range got {
		if t0.Name() == "ls" {
			found++
			out, err := t0.Invoke(context.Background(), `{"path":"`+cwd+`","intent":"check fallback suppression"}`)
			if err != nil {
				t.Fatalf("ls via AFT failed: %v", err)
			}
			if !strings.Contains(out, "AFT-OK") {
				t.Errorf("expected AFT output, got %q — fallback may have leaked", out)
			}
		}
	}
	if found == 0 {
		t.Fatal("ls tool not registered")
	}
}

func TestRegistryFallbackUsedForAgentGrep(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":false,\"message\":\"agentgrep broken\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	cwd := t.TempDir()
	target := filepath.Join(cwd, "log.txt")
	if err := os.WriteFile(target, []byte("warning: disk almost full\nall good here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := tools.All(cwd)
	for _, t0 := range got {
		if t0.Name() == "agentgrep" {
			args := `{"query":"warning","path":"` + cwd + `","intent":"check grep fallback"}`
			out, err := t0.Invoke(context.Background(), args)
			if err != nil {
				t.Fatalf("agentgrep fallback failed: %v", err)
			}
			if !strings.Contains(out, "warning") {
				t.Errorf("agentgrep fallback did not return match: %q", out)
			}
			return
		}
	}
	t.Fatal("agentgrep tool not registered")
}

func TestRegistryFallbackUsedForGlob(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":false,\"message\":\"glob broken\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "alpha.go"), []byte("package x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "beta.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := tools.All(cwd)
	for _, t0 := range got {
		if t0.Name() == "glob" {
			args := `{"pattern":"*.go","path":"` + cwd + `","intent":"check glob fallback"}`
			out, err := t0.Invoke(context.Background(), args)
			if err != nil {
				t.Fatalf("glob fallback failed: %v", err)
			}
			if !strings.Contains(out, "alpha.go") {
				t.Errorf("glob fallback did not return .go match: %q", out)
			}
			return
		}
	}
	t.Fatal("glob tool not registered")
}

func TestRegistryFallbackUsedForWrite(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":false,\"message\":\"write denied\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	cwd := t.TempDir()
	target := filepath.Join(cwd, "subdir", "w.txt")
	got := tools.All(cwd)
	for _, t0 := range got {
		if t0.Name() == "write" {
			args := `{"file":"` + target + `","content":"written by go","intent":"check write fallback"}`
			out, err := t0.Invoke(context.Background(), args)
			if err != nil {
				t.Fatalf("write fallback failed: %v", err)
			}
			if !strings.Contains(out, "wrote") {
				t.Errorf("write fallback output unexpected: %q", out)
			}
			data, _ := os.ReadFile(target)
			if !strings.Contains(string(data), "written by go") {
				t.Errorf("write fallback did not create file: %q", string(data))
			}
			return
		}
	}
	t.Fatal("write tool not registered")
}

func TestRegistryFallbackIsIdempotentAcrossCalls(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":false,\"message\":\"intermittent\"}\\n' \"$id\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	cwd := t.TempDir()
	target := filepath.Join(cwd, "multi.txt")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := tools.All(cwd)
	var calls atomic.Int32
	for _, t0 := range got {
		if t0.Name() == "read" {
			for i := 0; i < 3; i++ {
				out, err := t0.Invoke(context.Background(), `{"file":"`+target+`","intent":"check repeated fallback"}`)
				if err != nil {
					t.Fatalf("read fallback #%d failed: %v", i, err)
				}
				if !strings.Contains(out, "data") {
					t.Errorf("read fallback #%d output unexpected: %q", i, out)
				}
				calls.Add(1)
			}
			if calls.Load() != 3 {
				t.Errorf("expected 3 fallback calls, got %d", calls.Load())
			}
			return
		}
	}
	t.Fatal("read tool not registered")
}
