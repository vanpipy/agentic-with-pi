package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/tools"
)

func TestEditFileEmptyEditsArray(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(target, []byte("data\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.EditFile(dir)
	_, err := tool.Invoke(context.Background(),
		`{"path":"f.txt","edits":[],"intent":"check empty edits"}`)
	if err == nil {
		t.Fatal("expected error for empty edits array")
	}
	if !strings.Contains(err.Error(), "edits array must not be empty") {
		t.Errorf("err = %q, want 'edits array must not be empty'", err.Error())
	}
}

func TestEditFileReadMissingFile(t *testing.T) {
	dir := t.TempDir()
	tool := tools.EditFile(dir)
	_, err := tool.Invoke(context.Background(),
		`{"path":"nope.txt","edits":[{"old_text":"a","new_text":"b"}],"intent":"check missing file"}`)
	if err == nil {
		t.Fatal("expected error when target file does not exist")
	}
}

func TestEditFileReadDirectoryFails(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	tool := tools.EditFile(dir)
	_, err := tool.Invoke(context.Background(),
		`{"path":"subdir","edits":[{"old_text":"a","new_text":"b"}],"intent":"read directory"}`)
	if err == nil {
		t.Fatal("expected error when target is a directory")
	}
}

func TestEditFileReplaceAllOnEmptyFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(target, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.EditFile(dir)
	out, err := tool.Invoke(context.Background(),
		`{"path":"empty.txt","edits":[{"old_text":"foo","new_text":"bar","replace_all":true}],"intent":"empty file"}`)
	if err != nil {
		t.Fatalf("replace_all on empty file should succeed: %v", err)
	}
	if !strings.Contains(out, "applied") {
		t.Errorf("out = %q, want 'applied'", out)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "" {
		t.Errorf("empty file should stay empty; got %q", string(data))
	}
}

func TestEditFileUniqueMatchSingleOccurrence(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(target, []byte("alpha beta gamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.EditFile(dir)
	_, err := tool.Invoke(context.Background(),
		`{"path":"f.txt","edits":[{"old_text":"alpha","new_text":"ALPHA"}],"intent":"single match"}`)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	data, _ := os.ReadFile(target)
	if !strings.Contains(string(data), "ALPHA beta gamma") {
		t.Errorf("data = %q, want 'ALPHA beta gamma'", string(data))
	}
}

func TestEditFileReplaceAllReplacesAllOccurrences(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(target, []byte("x x x x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.EditFile(dir)
	_, err := tool.Invoke(context.Background(),
		`{"path":"f.txt","edits":[{"old_text":"x","new_text":"y","replace_all":true}],"intent":"all replace"}`)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "y y y y\n" {
		t.Errorf("data = %q, want 'y y y y\\n'", string(data))
	}
}

func TestEditFileSequentialEdits(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(target, []byte("hello world\nfoo bar\nbaz qux\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.EditFile(dir)
	_, err := tool.Invoke(context.Background(),
		`{"path":"f.txt","edits":[{"old_text":"hello","new_text":"HI"},{"old_text":"foo","new_text":"FOO"},{"old_text":"baz","new_text":"BAZ"}],"intent":"sequential"}`)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	data, _ := os.ReadFile(target)
	want := "HI world\nFOO bar\nBAZ qux\n"
	if string(data) != want {
		t.Errorf("data = %q, want %q", string(data), want)
	}
}

func TestEditFileNoMatchReturnsError(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(target, []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.EditFile(dir)
	_, err := tool.Invoke(context.Background(),
		`{"path":"f.txt","edits":[{"old_text":"absent","new_text":"X"}],"intent":"no match"}`)
	if err == nil {
		t.Fatal("expected error when old_text is absent")
	}
	if !strings.Contains(err.Error(), "match exactly once") {
		t.Errorf("err = %q, want 'match exactly once'", err.Error())
	}
}

func TestEditFilePathResolutionRelativeToCWD(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(target, []byte("alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.EditFile(dir)
	_, err := tool.Invoke(context.Background(),
		`{"path":"f.txt","edits":[{"old_text":"alpha","new_text":"ALPHA"}],"intent":"cwd-relative"}`)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	data, _ := os.ReadFile(target)
	if !strings.Contains(string(data), "ALPHA") {
		t.Errorf("data = %q, want ALPHA", string(data))
	}
}

func TestEditFileAppliesEditsMessageIncludesCount(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(target, []byte("a b c\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := tools.EditFile(dir)
	out, err := tool.Invoke(context.Background(),
		`{"path":"f.txt","edits":[{"old_text":"a","new_text":"A"},{"old_text":"c","new_text":"C"}],"intent":"count message"}`)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !strings.Contains(out, "applied 2 edits") {
		t.Errorf("out = %q, want 'applied 2 edits'", out)
	}
	if !strings.Contains(out, "f.txt") {
		t.Errorf("out = %q, want to mention target path", out)
	}
}
