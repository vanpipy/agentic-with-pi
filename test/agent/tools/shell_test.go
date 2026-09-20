package tools_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/agent/tools"
)

func TestBashSimple(t *testing.T) {
	dir := t.TempDir()
	tool := tools.Bash(dir, tools.BashOptions{})

	out, err := tool.Execute(context.Background(), `{"command":"echo hello","intent":"test"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("got %q, want 'hello'", out)
	}
}

func TestBashWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "marker.txt", "found")

	tool := tools.Bash(dir, tools.BashOptions{})
	out, err := tool.Execute(context.Background(), `{"command":"ls marker.txt","intent":"test"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "marker.txt") {
		t.Errorf("got %q, want marker.txt", out)
	}
}

func TestBashTimeout(t *testing.T) {
	dir := t.TempDir()
	tool := tools.Bash(dir, tools.BashOptions{})

	_, err := tool.Execute(context.Background(), `{"command":"sleep 5","timeout":1,"intent":"test"}`)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v, want timeout", err)
	}
}

func TestBashFailure(t *testing.T) {
	dir := t.TempDir()
	tool := tools.Bash(dir, tools.BashOptions{})

	_, err := tool.Execute(context.Background(), `{"command":"exit 1","intent":"test"}`)
	if err == nil {
		t.Fatal("expected error for exit 1")
	}
	if !strings.Contains(err.Error(), "command failed") {
		t.Errorf("err = %v, want 'command failed'", err)
	}
}

func TestBashStderrCaptured(t *testing.T) {
	dir := t.TempDir()
	tool := tools.Bash(dir, tools.BashOptions{})

	out, err := tool.Execute(context.Background(), `{"command":"echo err 1>&2","intent":"test"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "[stderr]") {
		t.Errorf("stderr not in output: %q", out)
	}
	if !strings.Contains(out, "err") {
		t.Errorf("stderr content missing: %q", out)
	}
}

func TestBashContextCancel(t *testing.T) {
	dir := t.TempDir()
	tool := tools.Bash(dir, tools.BashOptions{TimeoutSec: 30})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := tool.Execute(ctx, `{"command":"sleep 10","intent":"test"}`)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
