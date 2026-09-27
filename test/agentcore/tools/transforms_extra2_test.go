package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-core/tools"
)

func TestWriteFileSuccess(t *testing.T) {
	dir := t.TempDir()
	tool := tools.WriteFile(dir)

	out, err := tool.Invoke(context.Background(),
		`{"path":"x.txt","content":"hello world","intent":"test write"}`)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "wrote 11 bytes") {
		t.Errorf("out = %q, want 'wrote 11 bytes'", out)
	}

	data, err := os.ReadFile(filepath.Join(dir, "x.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Errorf("data = %q, want 'hello world'", string(data))
	}
}

func TestWriteFileCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	tool := tools.WriteFile(dir)

	_, err := tool.Invoke(context.Background(),
		`{"path":"a/b/c/d.txt","content":"deep","intent":"deep write"}`)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "a", "b", "c", "d.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "deep" {
		t.Errorf("data = %q, want 'deep'", string(data))
	}
}

func TestWriteFileFailureWhenPathIsDir(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	tool := tools.WriteFile(dir)
	_, err := tool.Invoke(context.Background(),
		`{"path":"subdir","content":"data","intent":"dir test"}`)
	if err == nil {
		t.Fatal("expected error when writing to a directory path")
	}
}

func TestWriteFileFailureReadOnlyParent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test cannot simulate read-only filesystem when running as root")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	tool := tools.WriteFile(dir)
	_, err := tool.Invoke(context.Background(),
		`{"path":"foo.txt","content":"data","intent":"read-only parent"}`)
	if err == nil {
		t.Fatal("expected error when parent dir is read-only")
	}
}

func TestWriteFileEmptyArgsJSONError(t *testing.T) {
	dir := t.TempDir()
	tool := tools.WriteFile(dir)
	_, err := tool.Invoke(context.Background(), `{"intent":"only"}`)
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
}

func TestAllWithAFTUnavailableFallsBackToGo(t *testing.T) {
	t.Setenv("AWP_TEST_AFT", "")
	t.Setenv("AWP_NO_AFT", "1")

	all := tools.All(t.TempDir())
	if len(all) == 0 {
		t.Fatal("expected non-empty toolset")
	}
}

func TestAllReturnsToolsInBothModes(t *testing.T) {
	t.Setenv("AWP_NO_AFT", "1")

	goTools := tools.All(t.TempDir())
	if len(goTools) == 0 {
		t.Fatal("expected Go toolset to be non-empty")
	}

	names := make(map[string]bool)
	for _, tl := range goTools {
		names[tl.Name()] = true
	}

	for _, want := range []string{"read", "write", "edit"} {
		if !names[want] {
			t.Errorf("missing tool %q", want)
		}
	}
}

func TestReadFileEmptyContentReportsZero(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(target, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.ReadFile(dir, tools.FileOptions{MaxBytes: 1024})
	out, err := tool.Invoke(context.Background(), `{"path":"empty.txt","intent":"empty"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("out = %q, want empty string", out)
	}
}

func TestReadFileInvalidArgsError(t *testing.T) {
	dir := t.TempDir()
	tool := tools.ReadFile(dir, tools.FileOptions{MaxBytes: 1024})
	_, err := tool.Invoke(context.Background(), `not json at all`)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestReadFileMissingIntentStillSucceeds(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(target, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.ReadFile(dir, tools.FileOptions{MaxBytes: 1024})
	out, err := tool.Invoke(context.Background(), `{"path":"x.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("out = %q, want 'hi'", out)
	}
}

func TestReadFileLineRangeOutOfBounds(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(target, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.ReadFile(dir, tools.FileOptions{MaxBytes: 1024})
	out, err := tool.Invoke(context.Background(),
		`{"path":"x.txt","offset":100,"limit":10,"intent":"out of bounds"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Errorf("out = empty, want non-empty response")
	}
}

func TestReadFileOffsetZeroIsTreatedAsOne(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(target, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.ReadFile(dir, tools.FileOptions{MaxBytes: 1024})
	out, err := tool.Invoke(context.Background(),
		`{"path":"x.txt","offset":0,"intent":"offset 0"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "a") {
		t.Errorf("out = %q, want 'a' (offset 0 should be treated as 1)", out)
	}
}

func TestAllToolsImplementAgentcoreTool(t *testing.T) {
	t.Setenv("AWP_NO_AFT", "1")

	all := tools.All(t.TempDir())
	for _, tool := range all {
		var _ agentcore.Tool = tool
	}
}

func TestAllToolsHaveName(t *testing.T) {
	t.Setenv("AWP_NO_AFT", "1")

	all := tools.All(t.TempDir())
	for _, tool := range all {
		if tool.Name() == "" {
			t.Errorf("tool has empty name: %+v", tool)
		}
	}
}

func TestAllToolsHaveDescription(t *testing.T) {
	t.Setenv("AWP_NO_AFT", "1")

	all := tools.All(t.TempDir())
	for _, tool := range all {
		if tool.Description() == "" {
			t.Errorf("tool %q has empty description", tool.Name())
		}
	}
}

func TestAllToolsHaveParameters(t *testing.T) {
	t.Setenv("AWP_NO_AFT", "1")

	all := tools.All(t.TempDir())
	for _, tool := range all {
		params := tool.Parameters()
		if params == nil {
			t.Errorf("tool %q has nil parameters", tool.Name())
		}
	}
}

func TestBashEmptyCommand(t *testing.T) {
	dir := t.TempDir()
	tool := tools.Bash(dir, tools.BashOptions{Shell: "/bin/sh"})

	_, err := tool.Invoke(context.Background(), `{"command":"   ","intent":"empty cmd"}`)
	if err == nil {
		t.Fatal("expected error for empty/whitespace command")
	}
}

func TestBashShellOverride(t *testing.T) {
	dir := t.TempDir()
	tool := tools.Bash(dir, tools.BashOptions{Shell: "/bin/sh"})

	out, err := tool.Invoke(context.Background(), `{"command":"echo override-test","shell":"/bin/sh","intent":"shell override"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "override-test") {
		t.Errorf("out = %q, want 'override-test'", out)
	}
}

func TestBashZeroTimeoutMeansNoTimeout(t *testing.T) {
	dir := t.TempDir()
	tool := tools.Bash(dir, tools.BashOptions{Shell: "/bin/sh"})

	out, err := tool.Invoke(context.Background(), `{"command":"echo no-timeout","timeout":0,"intent":"no timeout"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no-timeout") {
		t.Errorf("out = %q, want 'no-timeout'", out)
	}
}

func TestInvalidToolWithEmptyToolName(t *testing.T) {
	tool := tools.InvalidTool()
	_, err := tool.Invoke(context.Background(), `{"tool":"","data":{"x":1},"intent":"empty"}`)
	if err == nil {
		t.Fatal("expected error for empty tool name")
	}
}

func TestInvalidToolRejectsInvalidJSON(t *testing.T) {
	tool := tools.InvalidTool()
	_, err := tool.Invoke(context.Background(), `not json`)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestToolOutputLooksFailedDetectsFailed(t *testing.T) {
	if !tools.ToolOutputLooksFailed("error: something bad") {
		t.Error("expected detection of 'error:' prefix")
	}
}

func TestToolOutputLooksFailedDetectsFailedColon(t *testing.T) {
	if !tools.ToolOutputLooksFailed("failed: action") {
		t.Error("expected detection of 'failed:' prefix")
	}
}

func TestToolOutputLooksFailedDetectsExitCodeExtra(t *testing.T) {
	if !tools.ToolOutputLooksFailed("--- Command finished with exit code: 1 ---") {
		t.Error("expected detection of nonzero exit code")
	}
}

func TestToolOutputLooksFailedEmptyContent(t *testing.T) {
	if tools.ToolOutputLooksFailed("") {
		t.Error("empty content should not be flagged")
	}
	if tools.ToolOutputLooksFailed("   \n  ") {
		t.Error("whitespace-only content should not be flagged")
	}
}

func TestToolOutputLooksFailedDetectsPrefix(t *testing.T) {
	if !tools.ToolOutputLooksFailed("\u2717 bad outcome") {
		t.Error("expected detection of \u2717 prefix")
	}
}

func TestToolOutputLooksFailedDetectsStatusExtra(t *testing.T) {
	if !tools.ToolOutputLooksFailed("Status: failed") {
		t.Error("expected detection of 'Status: failed'")
	}
}
