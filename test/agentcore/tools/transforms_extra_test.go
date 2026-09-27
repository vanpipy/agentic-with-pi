package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-core/tools"
)

func setupAllWithAlwaysFailAFT(t *testing.T, msg string) {
	t.Helper()
	tools.ResetAftBackendForTest()
	t.Cleanup(tools.ResetAftBackendForTest)

	bin := filepath.Join(t.TempDir(), "aft")
	script := "#!/bin/sh\nwhile read -r line; do\n  id=$(printf '%s' \"$line\" | sed -n 's/.*\"id\":\"\\([^\"]*\\)\".*/\\1/p')\n  cmd=$(printf '%s' \"$line\" | sed -n 's/.*\"command\":\"\\([^\"]*\\)\".*/\\1/p')\n  printf '{\"id\":\"%s\",\"success\":false,\"command\":\"%s\",\"message\":\"%s\"}\\n' \"$id\" \"$cmd\" \"" + msg + "\"\ndone\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_TEST_AFT", bin)
	t.Setenv("AWP_NO_AFT", "")
}

func findTool(t *testing.T, allTools []agentcore.Tool, name string) agentcore.Tool {
	t.Helper()
	for _, tl := range allTools {
		if tl.Name() == name {
			return tl
		}
	}
	t.Fatalf("tool %q not registered", name)
	return nil
}

func TestBashTransformExactMsNoRounding(t *testing.T) {
	setupAllWithAlwaysFailAFT(t, "bash denied")

	cwd := t.TempDir()
	all := tools.All(cwd)
	tool := findTool(t, all, "bash")

	out, err := tool.Invoke(context.Background(),
		`{"command":"echo hi","timeout":2000,"intent":"check exact ms"}`)
	if err != nil {
		t.Fatalf("bash fallback: %v", err)
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("expected fallback to run; got %q", out)
	}
}

func TestBashTransformNoTimeout(t *testing.T) {
	setupAllWithAlwaysFailAFT(t, "bash denied")

	cwd := t.TempDir()
	all := tools.All(cwd)
	tool := findTool(t, all, "bash")

	out, err := tool.Invoke(context.Background(),
		`{"command":"echo no-timeout","intent":"check no timeout"}`)
	if err != nil {
		t.Fatalf("bash fallback: %v", err)
	}
	if !strings.Contains(out, "no-timeout") {
		t.Errorf("expected fallback to run; got %q", out)
	}
}

func TestLsTransformDepthPositiveDoesNotDropLimit(t *testing.T) {
	setupAllWithAlwaysFailAFT(t, "ls denied")

	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	all := tools.All(cwd)
	tool := findTool(t, all, "ls")

	out, err := tool.Invoke(context.Background(),
		`{"path":"`+cwd+`","depth":3,"intent":"check depth positive"}`)
	if err != nil {
		t.Fatalf("ls fallback: %v", err)
	}
	if !strings.Contains(out, "sub") {
		t.Errorf("ls fallback should list 'sub'; got %q", out)
	}
}

func TestLsTransformDepthZeroWithExistingLimit(t *testing.T) {
	setupAllWithAlwaysFailAFT(t, "ls denied")

	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	all := tools.All(cwd)
	tool := findTool(t, all, "ls")

	out, err := tool.Invoke(context.Background(),
		`{"path":"`+cwd+`","depth":0,"limit":42,"intent":"check depth zero with limit"}`)
	if err != nil {
		t.Fatalf("ls fallback: %v", err)
	}
	if !strings.Contains(out, "sub") {
		t.Errorf("ls fallback should list 'sub'; got %q", out)
	}
}

func TestLsTransformNoDepth(t *testing.T) {
	setupAllWithAlwaysFailAFT(t, "ls denied")

	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	all := tools.All(cwd)
	tool := findTool(t, all, "ls")

	out, err := tool.Invoke(context.Background(),
		`{"path":"`+cwd+`","intent":"check no depth"}`)
	if err != nil {
		t.Fatalf("ls fallback: %v", err)
	}
	if !strings.Contains(out, "sub") {
		t.Errorf("ls fallback should list 'sub'; got %q", out)
	}
}

func TestAgentGrepTransformNoFields(t *testing.T) {
	setupAllWithAlwaysFailAFT(t, "grep denied")

	cwd := t.TempDir()
	target := filepath.Join(cwd, "log.txt")
	if err := os.WriteFile(target, []byte("alpha beta gamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	all := tools.All(cwd)
	tool := findTool(t, all, "agentgrep")

	out, err := tool.Invoke(context.Background(),
		`{"path":"`+cwd+`","intent":"check no query/no ignore"}`)
	if err != nil {
		t.Fatalf("grep fallback: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty fallback output")
	}
}

func TestEditMatchTransformEmptyArgsJSON(t *testing.T) {
	setupAllWithAlwaysFailAFT(t, "edit_match denied")

	cwd := t.TempDir()
	all := tools.All(cwd)
	tool := findTool(t, all, "edit_match")

	_, err := tool.Invoke(context.Background(), `{"intent":"check empty args"}`)
	if err == nil {
		t.Fatal("expected error when argsJSON lacks required fields and AFT rejects")
	}
}

func TestReadTransformEmptyArgsJSON(t *testing.T) {
	setupAllWithAlwaysFailAFT(t, "read denied")

	cwd := t.TempDir()
	all := tools.All(cwd)
	tool := findTool(t, all, "read")

	_, err := tool.Invoke(context.Background(), `{"intent":"check empty args read"}`)
	if err == nil {
		t.Fatal("expected read fallback to error on empty path")
	}
}

func TestWriteTransformEmptyArgsJSON(t *testing.T) {
	setupAllWithAlwaysFailAFT(t, "write denied")

	cwd := t.TempDir()
	all := tools.All(cwd)
	tool := findTool(t, all, "write")

	_, err := tool.Invoke(context.Background(), `{"intent":"check empty args write"}`)
	if err == nil {
		t.Fatal("expected write fallback to error when no path given")
	}
}

func TestGlobTransformPassesThroughEmpty(t *testing.T) {
	setupAllWithAlwaysFailAFT(t, "glob denied")

	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "x.go"), []byte("package x"), 0o644); err != nil {
		t.Fatal(err)
	}
	all := tools.All(cwd)
	tool := findTool(t, all, "glob")

	out, err := tool.Invoke(context.Background(),
		`{"pattern":"*.go","path":"`+cwd+`","intent":"check glob fallback"}`)
	if err != nil {
		t.Fatalf("glob fallback: %v", err)
	}
	if !strings.Contains(out, "x.go") {
		t.Errorf("glob fallback did not list x.go; got %q", out)
	}
}
