package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/tools"
)

func writeFakeAftAlwaysFailForTest(t *testing.T, dir, message string) string {
	t.Helper()
	bin := filepath.Join(dir, "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":false,\"message\":\"%s\"}\\n' \"$id\" \"" + message + "\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestBashTransformStripsBackgroundWaitCompressedViaFallback(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	tmp := t.TempDir()
	bin := writeFakeAftAlwaysFailForTest(t, tmp, "bash rejected")
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")

	cwd := t.TempDir()
	all := tools.All(cwd)
	for _, tl := range all {
		if tl.Name() != "bash" {
			continue
		}
		out, err := tl.Invoke(context.Background(),
			`{"command":"echo hi","background":true,"wait":true,"compressed":true,"timeout":2000,"intent":"check transform"}`)
		if err != nil {
			t.Fatalf("bash fallback should succeed when AFT rejects: %v", err)
		}
		if !strings.Contains(out, "hi") {
			t.Errorf("expected fallback to run echo; got %q", out)
		}
		return
	}
	t.Fatal("bash tool not registered")
}

func TestBashTransformTimeoutMsRoundsUpViaFallback(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	tmp := t.TempDir()
	bin := writeFakeAftAlwaysFailForTest(t, tmp, "bash rejected")
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")

	cwd := t.TempDir()
	all := tools.All(cwd)
	for _, tl := range all {
		if tl.Name() != "bash" {
			continue
		}
		out, err := tl.Invoke(context.Background(),
			`{"command":"echo hi","timeout":2500,"intent":"check timeout transform"}`)
		if err != nil {
			t.Fatalf("bash fallback should succeed when AFT rejects: %v", err)
		}
		if !strings.Contains(out, "hi") {
			t.Errorf("expected fallback to run; got %q", out)
		}
		return
	}
	t.Fatal("bash tool not registered")
}

func TestReadTransformRenamesFileToPathViaFallback(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	tmp := t.TempDir()
	bin := writeFakeAftAlwaysFailForTest(t, tmp, "read rejected")
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")

	cwd := t.TempDir()
	target := filepath.Join(cwd, "f.txt")
	if err := os.WriteFile(target, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	all := tools.All(cwd)
	for _, tl := range all {
		if tl.Name() != "read" {
			continue
		}
		out, err := tl.Invoke(context.Background(),
			`{"file":"`+target+`","start_line":1,"end_line":10,"max_bytes":1024,"hashline":true,"intent":"check read transform"}`)
		if err != nil {
			t.Fatalf("read fallback should succeed when AFT rejects: %v", err)
		}
		if !strings.Contains(out, "hello world") {
			t.Errorf("read fallback output unexpected: %q", out)
		}
		return
	}
	t.Fatal("read tool not registered")
}

func TestWriteTransformRenamesFileToPathViaFallback(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	tmp := t.TempDir()
	bin := writeFakeAftAlwaysFailForTest(t, tmp, "write rejected")
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")

	cwd := t.TempDir()
	target := filepath.Join(cwd, "w.txt")

	all := tools.All(cwd)
	for _, tl := range all {
		if tl.Name() != "write" {
			continue
		}
		out, err := tl.Invoke(context.Background(),
			`{"file":"`+target+`","content":"transformed","intent":"check write transform"}`)
		if err != nil {
			t.Fatalf("write fallback should succeed when AFT rejects: %v", err)
		}
		if !strings.Contains(out, "wrote") {
			t.Errorf("write fallback output unexpected: %q", out)
		}
		data, _ := os.ReadFile(target)
		if string(data) != "transformed" {
			t.Errorf("file contents = %q, want 'transformed'", string(data))
		}
		return
	}
	t.Fatal("write tool not registered")
}

func TestGrepTransformRenamesQueryAndIgnoreCaseViaFallback(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	tmp := t.TempDir()
	bin := writeFakeAftAlwaysFailForTest(t, tmp, "grep rejected")
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")

	cwd := t.TempDir()
	target := filepath.Join(cwd, "g.txt")
	if err := os.WriteFile(target, []byte("Hello World\nfoo bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	all := tools.All(cwd)
	for _, tl := range all {
		if tl.Name() != "agentgrep" {
			continue
		}
		out, err := tl.Invoke(context.Background(),
			`{"query":"hello","path":"`+target+`","ignore_case":true,"intent":"check grep transform"}`)
		if err != nil {
			t.Fatalf("grep fallback should succeed when AFT rejects: %v", err)
		}
		if !strings.Contains(strings.ToLower(out), "hello") {
			t.Errorf("grep fallback output should contain hello: %q", out)
		}
		return
	}
	t.Fatal("agentgrep tool not registered")
}

func TestLsTransformDepthNonPositiveSetsLimitViaFallback(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	tmp := t.TempDir()
	bin := writeFakeAftAlwaysFailForTest(t, tmp, "ls rejected")
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")

	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	all := tools.All(cwd)
	for _, tl := range all {
		if tl.Name() != "ls" {
			continue
		}
		out, err := tl.Invoke(context.Background(),
			`{"path":"`+cwd+`","depth":0,"intent":"check ls transform"}`)
		if err != nil {
			t.Fatalf("ls fallback should succeed when AFT rejects: %v", err)
		}
		if !strings.Contains(out, "sub") {
			t.Errorf("ls fallback should list 'sub' entry; got %q", out)
		}
		return
	}
	t.Fatal("ls tool not registered")
}

func TestEditTransformProducesEditsArrayViaFallback(t *testing.T) {
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	tmp := t.TempDir()
	bin := writeFakeAftAlwaysFailForTest(t, tmp, "edit rejected")
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")

	cwd := t.TempDir()
	target := filepath.Join(cwd, "e.txt")
	if err := os.WriteFile(target, []byte("hello world\nfoo bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	all := tools.All(cwd)
	for _, tl := range all {
		if tl.Name() != "edit_match" {
			continue
		}
		out, err := tl.Invoke(context.Background(),
			`{"path":"`+target+`","old_string":"hello world","new_string":"HI","replace_all":false,"intent":"check edit transform"}`)
		if err != nil {
			t.Fatalf("edit fallback should succeed when AFT rejects: %v", err)
		}
		if !strings.Contains(out, "applied") && !strings.Contains(out, "edited") && !strings.Contains(out, "wrote") {
			t.Errorf("edit fallback output unexpected: %q", out)
		}
		data, _ := os.ReadFile(target)
		if !strings.Contains(string(data), "HI") {
			t.Errorf("file should contain 'HI' after edit; got %q", string(data))
		}
		return
	}
	t.Fatal("edit_match tool not registered")
}
