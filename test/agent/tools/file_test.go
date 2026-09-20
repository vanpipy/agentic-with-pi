package tools_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent"
	"github.com/vanpiyp/awp/internal/agent/tools"
)

func TestReadFileMissingIntent(t *testing.T) {
	dir := t.TempDir()
	tool := tools.ReadFile(dir, tools.FileOptions{})
	_, err := tool.Execute(context.Background(), `{"path":"hello.txt"}`)
	if err == nil {
		t.Fatal("expected error for missing intent")
	}
	if !strings.Contains(err.Error(), "intent") {
		t.Errorf("err = %q, want mentions 'intent'", err.Error())
	}
}

func TestReadFileWithIntent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := tools.ReadFile(dir, tools.FileOptions{})
	out, err := tool.Execute(context.Background(), `{"path":"hello.txt","intent":"read hello for greeting"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("out = %q, want contains 'hi'", out)
	}
}

func TestAllToolsRequireIntentInSchema(t *testing.T) {
	cwd := t.TempDir()
	allTools := []agent.Tool{
		tools.ReadFile(cwd, tools.FileOptions{}),
		tools.WriteFile(cwd),
		tools.EditFile(cwd),
		tools.Bash(cwd, tools.BashOptions{}),
		tools.Grep(cwd, tools.FileOptions{}),
		tools.Find(cwd, tools.FileOptions{}),
		tools.Ls(cwd, tools.FileOptions{}),
	}
	for _, tool := range allTools {
		schema, ok := tool.Parameters.(map[string]any)
		if !ok {
			t.Errorf("tool %q: parameters not a map", tool.Name)
			continue
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Errorf("tool %q: schema has no properties", tool.Name)
			continue
		}
		if _, ok := props["intent"]; !ok {
			t.Errorf("tool %q: schema missing intent property", tool.Name)
		}
		if _, ok := props["accept_large_output"]; !ok {
			t.Errorf("tool %q: schema missing accept_large_output property", tool.Name)
		}
		required, ok := schema["required"].([]string)
		if !ok {
			t.Errorf("tool %q: schema required not []string", tool.Name)
			continue
		}
		hasIntent := false
		for _, r := range required {
			if r == "intent" {
				hasIntent = true
				break
			}
		}
		if !hasIntent {
			t.Errorf("tool %q: required does not contain 'intent'", tool.Name)
		}
		for _, r := range required {
			if r == "accept_large_output" {
				t.Errorf("tool %q: accept_large_output must NOT be required", tool.Name)
			}
		}
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.ReadFile(dir, tools.FileOptions{})

	out, err := tool.Execute(context.Background(), `{"path":"hello.txt","intent":"test"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line3") {
		t.Errorf("got %q, expected all lines", out)
	}
}

func TestReadFileWithOffset(t *testing.T) {
	dir := t.TempDir()
	lines := []string{"a", "b", "c", "d", "e"}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.ReadFile(dir, tools.FileOptions{})

	out, err := tool.Execute(context.Background(), `{"path":"f.txt","offset":2,"limit":2,"intent":"test"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "b\nc" {
		t.Errorf("got %q, want %q", out, "b\nc")
	}
}

func TestReadFileMissing(t *testing.T) {
	dir := t.TempDir()
	tool := tools.ReadFile(dir, tools.FileOptions{})
	_, err := tool.Execute(context.Background(), `{"path":"nonexistent.txt","intent":"test"}`)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadFileEmptyPath(t *testing.T) {
	dir := t.TempDir()
	tool := tools.ReadFile(dir, tools.FileOptions{})
	out, err := tool.Execute(context.Background(), `{"intent":"test"}`)
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if !strings.Contains(err.Error(), "path is required") {
		t.Errorf("err = %q, want mentions 'path is required'", err.Error())
	}
	_ = out
}

func TestReadFileDirectoryReturnsHelpfulError(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := tools.ReadFile(dir, tools.FileOptions{})
	_, err := tool.Execute(context.Background(), fmt.Sprintf(`{"path":%q,"intent":"test"}`, subdir))
	if err == nil {
		t.Fatal("expected error when reading a directory")
	}
	if !strings.Contains(err.Error(), "is a directory") || !strings.Contains(err.Error(), "ls") {
		t.Errorf("err = %q, want hint to use ls", err.Error())
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	tool := tools.WriteFile(dir)

	out, err := tool.Execute(context.Background(), `{"path":"sub/dir/out.txt","content":"written content","intent":"test"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "wrote") {
		t.Errorf("got %q", out)
	}

	data, err := os.ReadFile(filepath.Join(dir, "sub/dir/out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "written content" {
		t.Errorf("file content = %q, want %q", string(data), "written content")
	}
}

func TestWriteFileOverwrite(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.WriteFile(dir)
	if _, err := tool.Execute(context.Background(), `{"path":"f.txt","content":"new","intent":"test"}`); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(target)
	if string(data) != "new" {
		t.Errorf("file content = %q, want %q", string(data), "new")
	}
}

func TestEditFileSingleEdit(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	original := "hello world\nfoo bar\nhello world\n"
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.EditFile(dir)
	_, err := tool.Execute(context.Background(), `{"path":"f.txt","edits":[{"old_text":"foo bar","new_text":"BAZ"}],"intent":"test"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	data, _ := os.ReadFile(target)
	got := string(data)
	if strings.Count(got, "BAZ") != 1 {
		t.Errorf("expected exactly 1 'BAZ', got %q", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Errorf("untouched content lost: %q", got)
	}
}

func TestEditFileReplaceAll(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(target, []byte("foo foo foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.EditFile(dir)
	_, err := tool.Execute(context.Background(), `{"path":"f.txt","edits":[{"old_text":"foo","new_text":"bar","replace_all":true}],"intent":"test"}`)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(target)
	if string(data) != "bar bar bar\n" {
		t.Errorf("got %q, want %q", string(data), "bar bar bar\n")
	}
}

func TestEditFileAmbiguousMatchFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(target, []byte("foo foo foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.EditFile(dir)
	_, err := tool.Execute(context.Background(), `{"path":"f.txt","edits":[{"old_text":"foo","new_text":"bar"}],"intent":"test"}`)
	if err == nil {
		t.Fatal("expected error for ambiguous match (3 matches without replace_all)")
	}
	if !strings.Contains(err.Error(), "match exactly once") {
		t.Errorf("err = %v, want ambiguity message", err)
	}
}

func TestEditFileMultipleEdits(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(target, []byte("a b c d e\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.EditFile(dir)
	_, err := tool.Execute(context.Background(),
		`{"path":"f.txt","edits":[{"old_text":"a","new_text":"A"},{"old_text":"e","new_text":"E"}],"intent":"test"}`)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(target)
	if string(data) != "A b c d E\n" {
		t.Errorf("got %q, want %q", string(data), "A b c d E\n")
	}
}
