package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent/tools"
)

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.ReadFile(dir, tools.FileOptions{})

	out, err := tool.Execute(context.Background(), `{"path":"hello.txt"}`)
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

	out, err := tool.Execute(context.Background(), `{"path":"f.txt","offset":2,"limit":2}`)
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
	_, err := tool.Execute(context.Background(), `{"path":"nonexistent.txt"}`)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	tool := tools.WriteFile(dir)

	out, err := tool.Execute(context.Background(), `{"path":"sub/dir/out.txt","content":"written content"}`)
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
	if _, err := tool.Execute(context.Background(), `{"path":"f.txt","content":"new"}`); err != nil {
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
	_, err := tool.Execute(context.Background(), `{"path":"f.txt","edits":[{"old_text":"foo bar","new_text":"BAZ"}]}`)
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
	_, err := tool.Execute(context.Background(), `{"path":"f.txt","edits":[{"old_text":"foo","new_text":"bar","replace_all":true}]}`)
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
	_, err := tool.Execute(context.Background(), `{"path":"f.txt","edits":[{"old_text":"foo","new_text":"bar"}]}`)
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
		`{"path":"f.txt","edits":[{"old_text":"a","new_text":"A"},{"old_text":"e","new_text":"E"}]}`)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(target)
	if string(data) != "A b c d E\n" {
		t.Errorf("got %q, want %q", string(data), "A b c d E\n")
	}
}
