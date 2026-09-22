package tools_test

import (
	"testing"

	"github.com/vanpiyp/awp/internal/agent/tools"
)

func TestAllRegistersEveryBuiltin(t *testing.T) {
	got := tools.All(t.TempDir())
	if len(got) < 7 {
		t.Fatalf("tools.All returned %d tools, want at least 7", len(got))
	}
	want := map[string]bool{
		"read": false, "write": false, "edit": false,
		"bash": false, "grep": false, "find": false, "ls": false,
		"invalid": false,
	}
	for _, t0 := range got {
		if _, ok := want[t0.Name()]; ok {
			want[t0.Name()] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("tools.All missing %q", name)
		}
	}
}

func TestAllTwiceReturnsDistinctToolValues(t *testing.T) {
	a := tools.All(t.TempDir())
	b := tools.All(t.TempDir())
	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Name() != b[i].Name() {
			t.Errorf("position %d: %q != %q", i, a[i].Name(), b[i].Name())
		}
	}
}
