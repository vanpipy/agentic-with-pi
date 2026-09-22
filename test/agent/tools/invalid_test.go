package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent/tools"
)

func TestInvalidTool(t *testing.T) {
	tool := tools.InvalidTool()
	out, err := tool.Invoke(context.Background(), `{"tool":"read","reason":"no path argument","intent":"fix my own bad call"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "read") {
		t.Errorf("out = %q, want contains 'read'", out)
	}
	if !strings.Contains(out, "no path argument") {
		t.Errorf("out = %q, want contains 'no path argument'", out)
	}
}

func TestInvalidToolMissingIntent(t *testing.T) {
	tool := tools.InvalidTool()
	out, err := tool.Invoke(context.Background(), `{"tool":"read","reason":"oops"}`)
	if err != nil {
		t.Fatalf("missing intent should NOT block, got err: %v", err)
	}
	if !strings.Contains(out, "oops") {
		t.Errorf("out = %q, want contains 'oops' (the reason)", out)
	}
}

func TestInvalidToolMissingReason(t *testing.T) {
	tool := tools.InvalidTool()
	_, err := tool.Invoke(context.Background(), `{"tool":"read","intent":"x"}`)
	if err == nil {
		t.Fatal("expected error for missing reason")
	}
	if !strings.Contains(err.Error(), "reason") {
		t.Errorf("err = %q, want mentions 'reason'", err.Error())
	}
}
