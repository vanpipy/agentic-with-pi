package agent_test

import (
	"testing"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestParseCommandQuit(t *testing.T) {
	parsed, ok := tui.ParseCommandForTest("/quit")
	if !ok || parsed.Name != "quit" {
		t.Fatalf("parseCommand(/quit) = %+v ok=%v", parsed, ok)
	}
}

func TestParseCommandNewIgnoresExtraArgs(t *testing.T) {
	parsed, _ := tui.ParseCommandForTest("/new")
	if parsed.Name != "new" {
		t.Fatalf("parseCommand(/new) name = %q, want new", parsed.Name)
	}
	if parsed.Arg != "" {
		t.Errorf("parseCommand(/new) arg = %q, want empty", parsed.Arg)
	}
}

func TestParseCommandResume(t *testing.T) {
	parsed, _ := tui.ParseCommandForTest("/resume sess-1")
	if parsed.Name != "resume" || parsed.Arg != "sess-1" {
		t.Fatalf("parseCommand(/resume sess-1) = %+v", parsed)
	}
}

func TestParseCommandRejectsNonSlash(t *testing.T) {
	if _, ok := tui.ParseCommandForTest("hello world"); ok {
		t.Error("parseCommand(hello world) should reject non-slash")
	}
}

func TestRegistryHasEveryBuiltin(t *testing.T) {
	specs := tui.AllCommandSpecsForTest()
	want := []string{"quit", "help", "new", "clear", "tools", "resume"}
	have := map[string]bool{}
	for _, s := range specs {
		have[s.Name] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("registry missing %q", w)
		}
	}
}

func TestQuitHasNoArg(t *testing.T) {
	for _, s := range tui.AllCommandSpecsForTest() {
		if s.Name == "quit" {
			if s.HasArg {
				t.Errorf("quit should not require an arg")
			}
		}
	}
}