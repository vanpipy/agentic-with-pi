package agent_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent"
	"github.com/vanpiyp/awp/internal/llm"
)

func TestEstimateTokens(t *testing.T) {
	if got := agent.EstimateTokens("hello world"); got != 3 {
		t.Errorf("got %d, want ~3", got)
	}
}

func TestCountUserTurns(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
		{Role: "user", Content: "u3"},
	}
	if got := agent.CountUserTurns(msgs); got != 3 {
		t.Errorf("got %d, want 3", got)
	}
}

func TestFindCutPointKeepsRecent(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system"},
		{Role: "user", Content: "u1"},
		{Role: "assistant"},
		{Role: "user", Content: "u2"},
		{Role: "assistant"},
		{Role: "user", Content: "u3"},
		{Role: "assistant"},
		{Role: "user", Content: "u4"},
		{Role: "assistant"},
	}

	if got := agent.FindCutPoint(msgs, 2); got != 5 {
		t.Errorf("cut at %d, want 5 (u3 boundary, keeping u3+u4 = 2 user turns)", got)
	}
}

func TestFindCutPointKeepZero(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system"},
		{Role: "user", Content: "u1"},
		{Role: "assistant"},
	}
	if got := agent.FindCutPoint(msgs, 0); got != 0 {
		t.Errorf("got %d, want 0 (keep 0 turns = drop everything)", got)
	}
}

func TestShouldCompactTriggersAboveThreshold(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: string(make([]byte, 40000))},
		{Role: "assistant", Content: string(make([]byte, 40000))},
	}
	settings := agent.CompactionSettings{Enabled: true, ReserveTokens: 1024, MinTurnsBetween: 0, FloorPercent: 40}
	if got := agent.ShouldCompact(msgs, 10000, settings, 100); !got {
		t.Errorf("expected shouldCompact=true (20000 tokens > 10000-1024=8976)")
	}
}

func TestShouldCompactDisabledAlwaysFalse(t *testing.T) {
	msgs := []llm.Message{{Role: "user", Content: string(make([]byte, 100000))}}
	settings := agent.CompactionSettings{Enabled: false, ReserveTokens: 0, MinTurnsBetween: 0, FloorPercent: 40}
	if got := agent.ShouldCompact(msgs, 1000, settings, 100); got {
		t.Errorf("disabled should never compact")
	}
}

func TestShouldCompactFloorBlocks(t *testing.T) {
	msgs := []llm.Message{{Role: "user", Content: string(make([]byte, 1000))}}
	settings := agent.CompactionSettings{Enabled: true, ReserveTokens: 0, MinTurnsBetween: 0, FloorPercent: 50}
	if got := agent.ShouldCompact(msgs, 10000, settings, 100); got {
		t.Errorf("expected shouldCompact=false (250 tokens < 5000 floor)")
	}
}

func TestShouldCompactCooldownBlocks(t *testing.T) {
	msgs := []llm.Message{{Role: "user", Content: string(make([]byte, 40000))}}
	settings := agent.CompactionSettings{Enabled: true, ReserveTokens: 1024, MinTurnsBetween: 5, FloorPercent: 40}
	if got := agent.ShouldCompact(msgs, 10000, settings, 2); got {
		t.Errorf("expected shouldCompact=false (cooldown not elapsed)")
	}
}

func TestSerializeForSummary(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "ignore me"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
		{Role: "tool", ToolCallID: "c1", Content: "result1"},
	}
	out := agent.SerializeForSummary(msgs)
	if !strings.Contains(out, "[user] hello") {
		t.Errorf("missing user content in %q", out)
	}
	if !strings.Contains(out, "[assistant] world") {
		t.Errorf("missing assistant content in %q", out)
	}
	if !strings.Contains(out, "[tool result c1] result1") {
		t.Errorf("missing tool result in %q", out)
	}
	if strings.Contains(out, "ignore me") {
		t.Errorf("system content leaked into summary")
	}
}
