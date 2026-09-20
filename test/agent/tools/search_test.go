package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent/tools"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLsSimple(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "")
	writeFile(t, dir, "b.txt", "")
	os.Mkdir(filepath.Join(dir, "subdir"), 0o755)

	tool := tools.Ls(dir, tools.FileOptions{})
	out, err := tool.Execute(context.Background(), `{"intent":"test"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "a.txt") {
		t.Errorf("missing a.txt in: %q", out)
	}
	if !strings.Contains(out, "b.txt") {
		t.Errorf("missing b.txt in: %q", out)
	}
	if !strings.Contains(out, "subdir/") {
		t.Errorf("missing subdir/ in: %q", out)
	}
}

func TestLsLimit(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		writeFile(t, dir, n+".txt", "")
	}

	tool := tools.Ls(dir, tools.FileOptions{})
	out, err := tool.Execute(context.Background(), `{"limit":2,"intent":"test"}`)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Errorf("got %d lines, want 2: %q", len(lines), out)
	}
}

func TestLsMissing(t *testing.T) {
	dir := t.TempDir()
	tool := tools.Ls(dir, tools.FileOptions{})
	_, err := tool.Execute(context.Background(), `{"path":"nonexistent","intent":"test"}`)
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestFindGlob(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "foo.go", "package main")
	writeFile(t, dir, "bar.go", "package main")
	writeFile(t, dir, "baz.txt", "")

	tool := tools.Find(dir, tools.FileOptions{})
	out, err := tool.Execute(context.Background(), `{"pattern":"*.go","intent":"test"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "foo.go") {
		t.Errorf("missing foo.go: %q", out)
	}
	if !strings.Contains(out, "bar.go") {
		t.Errorf("missing bar.go: %q", out)
	}
	if strings.Contains(out, "baz.txt") {
		t.Errorf("baz.txt should not match *.go: %q", out)
	}
}

func TestFindRecursive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "sub/deep/file.go", "")
	writeFile(t, dir, "top.go", "")

	tool := tools.Find(dir, tools.FileOptions{})
	out, err := tool.Execute(context.Background(), `{"pattern":"*.go","intent":"test"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "sub/deep/file.go") {
		t.Errorf("recursive missing: %q", out)
	}
	if !strings.Contains(out, "top.go") {
		t.Errorf("top-level missing: %q", out)
	}
}

func TestFindLimit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		writeFile(t, dir, "f"+string(rune('a'+i))+".txt", "")
	}
	tool := tools.Find(dir, tools.FileOptions{})
	out, err := tool.Execute(context.Background(), `{"pattern":"*.txt","limit":3,"intent":"test"}`)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Errorf("got %d, want 3: %q", len(lines), out)
	}
}

func TestFindNoMatch(t *testing.T) {
	dir := t.TempDir()
	tool := tools.Find(dir, tools.FileOptions{})
	out, err := tool.Execute(context.Background(), `{"pattern":"*.xyz","intent":"test"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no matches") {
		t.Errorf("got %q, want 'no matches'", out)
	}
}

func TestGrepPattern(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello world\nfoo bar\nhello again\n")
	writeFile(t, dir, "b.txt", "hello world\n")

	tool := tools.Grep(dir, tools.FileOptions{})
	out, err := tool.Execute(context.Background(), `{"pattern":"hello","intent":"test"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "a.txt:1:hello world") {
		t.Errorf("missing a.txt:1 line: %q", out)
	}
	if !strings.Contains(out, "a.txt:3:hello again") {
		t.Errorf("missing a.txt:3 line: %q", out)
	}
	if !strings.Contains(out, "b.txt:1:hello world") {
		t.Errorf("missing b.txt:1 line: %q", out)
	}
}

func TestGrepIgnoreCase(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "Hello World\nHELLO WORLD\n")

	tool := tools.Grep(dir, tools.FileOptions{})
	out, err := tool.Execute(context.Background(), `{"pattern":"hello","ignoreCase":true,"intent":"test"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Hello") {
		t.Errorf("case-insensitive miss: %q", out)
	}
}

func TestGrepLiteral(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "a.b.c\naXbXc\n")

	tool := tools.Grep(dir, tools.FileOptions{})
	out, err := tool.Execute(context.Background(), `{"pattern":"a.b.c","literal":true,"intent":"test"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "a.b.c") {
		t.Errorf("literal pattern miss: %q", out)
	}
	if strings.Contains(out, "aXbXc") {
		t.Errorf("regex false-positive in literal mode: %q", out)
	}
}

func TestGrepGlob(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "match here\n")
	writeFile(t, dir, "a.txt", "match here too\n")

	tool := tools.Grep(dir, tools.FileOptions{})
	out, err := tool.Execute(context.Background(), `{"pattern":"match","glob":"*.go","intent":"test"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "a.go") {
		t.Errorf("missing a.go: %q", out)
	}
	if strings.Contains(out, "a.txt") {
		t.Errorf("glob failed to filter a.txt: %q", out)
	}
}

func TestGrepNoMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello\n")

	tool := tools.Grep(dir, tools.FileOptions{})
	out, err := tool.Execute(context.Background(), `{"pattern":"xyz","intent":"test"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no matches") {
		t.Errorf("got %q, want 'no matches'", out)
	}
}
