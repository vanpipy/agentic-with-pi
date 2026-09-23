package agentcore_test

import (
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/skills"
)

func makeReg(t *testing.T, names ...string) *skills.Registry {
	t.Helper()
	r := &skills.Registry{Skills: map[string]*skills.Skill{}}
	for _, n := range names {
		r.Skills[n] = &skills.Skill{Name: n, Description: "d"}
	}
	return r
}

func TestResolveLongestPrefixWins(t *testing.T) {
	r := makeReg(t, "optimize", "optimize deep")

	name, prompt, ok := r.ResolveInvocation("/optimize deep my code please")
	if !ok {
		t.Fatalf("expected match")
	}
	if name != "optimize deep" {
		t.Errorf("name = %q, want %q", name, "optimize deep")
	}
	if prompt != "my code please" {
		t.Errorf("prompt = %q, want %q", prompt, "my code please")
	}
}

func TestResolveSingleMatch(t *testing.T) {
	r := makeReg(t, "optimize")

	name, prompt, ok := r.ResolveInvocation("/optimize hello world")
	if !ok {
		t.Fatalf("expected match")
	}
	if name != "optimize" {
		t.Errorf("name = %q, want optimize", name)
	}
	if prompt != "hello world" {
		t.Errorf("prompt = %q, want %q", prompt, "hello world")
	}
}

func TestResolveDoubleQuotedPromptUnquoted(t *testing.T) {
	r := makeReg(t, "foo")

	name, prompt, ok := r.ResolveInvocation(`/foo "quoted prompt"`)
	if !ok {
		t.Fatalf("expected match")
	}
	if name != "foo" {
		t.Errorf("name = %q, want foo", name)
	}
	if prompt != "quoted prompt" {
		t.Errorf("prompt = %q, want %q (quotes should be stripped)", prompt, "quoted prompt")
	}
}

func TestResolveSingleQuotedPromptUnquoted(t *testing.T) {
	r := makeReg(t, "foo")

	_, prompt, ok := r.ResolveInvocation(`/foo 'quoted prompt'`)
	if !ok {
		t.Fatalf("expected match")
	}
	if prompt != "quoted prompt" {
		t.Errorf("prompt = %q, want %q", prompt, "quoted prompt")
	}
}

func TestResolveNoPrompt(t *testing.T) {
	r := makeReg(t, "foo")

	name, prompt, ok := r.ResolveInvocation("/foo")
	if !ok {
		t.Fatalf("expected match")
	}
	if name != "foo" {
		t.Errorf("name = %q, want foo", name)
	}
	if prompt != "" {
		t.Errorf("prompt = %q, want empty", prompt)
	}
}

func TestResolveTrailingSpace(t *testing.T) {
	r := makeReg(t, "foo")

	name, prompt, ok := r.ResolveInvocation("/foo   ")
	if !ok {
		t.Fatalf("expected match")
	}
	if name != "foo" {
		t.Errorf("name = %q, want foo", name)
	}
	if prompt != "" {
		t.Errorf("prompt = %q, want empty", prompt)
	}
}

func TestResolveUnknownNameFails(t *testing.T) {
	r := makeReg(t, "foo")

	_, _, ok := r.ResolveInvocation("/unknown stuff")
	if ok {
		t.Errorf("expected ok=false for unknown name")
	}
}

func TestResolveLeadingSlashOnly(t *testing.T) {
	r := makeReg(t, "foo")

	if _, _, ok := r.ResolveInvocation("/"); ok {
		t.Errorf("input / should not match")
	}
	if _, _, ok := r.ResolveInvocation("/ "); ok {
		t.Errorf("input '/ ' should not match")
	}
}

func TestResolveInputNotSlash(t *testing.T) {
	r := makeReg(t, "foo")

	if _, _, ok := r.ResolveInvocation("foo bar"); ok {
		t.Errorf("plain text should not match slash invocation")
	}
}

func TestResolveLongerNameSubstringDoesNotMatch(t *testing.T) {
	r := makeReg(t, "foo")

	// "foo bar" might or might not match — it's clearly longer than "foo".
	// But registry only has "foo", so "foo bar" is not in registry.
	_, _, ok := r.ResolveInvocation("/foobar baz")
	if ok {
		t.Errorf("registries without 'foobar' should not match 'foobar baz'")
	}
}

func TestResolveEmpty(t *testing.T) {
	r := makeReg(t, "foo")

	if _, _, ok := r.ResolveInvocation(""); ok {
		t.Errorf("empty input should not match")
	}
}

func TestResolveNilRegistry(t *testing.T) {
	var r *skills.Registry

	if _, _, ok := r.ResolveInvocation("/foo bar"); ok {
		t.Errorf("nil registry should not match")
	}
}

func TestResolveLeavesPromptUnchangedWhenNoQuotes(t *testing.T) {
	r := makeReg(t, "foo")

	_, prompt, ok := r.ResolveInvocation("/foo  spaces  preserved ")
	if !ok {
		t.Fatalf("expected match")
	}
	if prompt != "spaces  preserved" {
		t.Errorf("prompt = %q, want %q (only edges should trim, internal whitespace preserved)", prompt, "spaces  preserved")
	}
}
