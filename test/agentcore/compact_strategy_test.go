package agentcore_test

import (
	"context"
	"strings"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/llm"
)

func TestSelectStrategyTableDriven(t *testing.T) {
	big := func(s string) *int { v := len(s) * 200; return &v }
	small := func(s string) *int { v := 10; return &v }
	none := func() *int { return nil }

	cases := []struct {
		name     string
		msgs     []llm.Message
		settings agentcore.CompactionSettings
		observed *int
		want     agentcore.CompactionStrategy
	}{
		{
			name:     "disabled_falls_to_emergency",
			msgs:     []llm.Message{{Role: "user", Content: "x"}},
			settings: agentcore.CompactionSettings{Enabled: false, ReserveTokens: 100, MaxContextTokens: 1000},
			observed: none(),
			want:     agentcore.StrategyEmergency,
		},
		{
			name:     "should_compact_returns_reactive",
			msgs:     []llm.Message{{Role: "user", Content: strings.Repeat("a", 4000)}},
			settings: agentcore.CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 1000},
			observed: big(strings.Repeat("a", 4000)),
			want:     agentcore.StrategyReactive,
		},
		{
			name:     "below_threshold_with_proactive_returns_proactive",
			msgs:     []llm.Message{{Role: "user", Content: "short"}},
			settings: agentcore.CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 100000, Proactive: true},
			observed: small("short"),
			want:     agentcore.StrategyProactive,
		},
		{
			name:     "below_threshold_with_semantic_returns_semantic",
			msgs:     []llm.Message{{Role: "user", Content: "short"}},
			settings: agentcore.CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 100000, Semantic: true},
			observed: small("short"),
			want:     agentcore.StrategySemantic,
		},
		{
			name:     "proactive_beats_semantic_in_precedence",
			msgs:     []llm.Message{{Role: "user", Content: "short"}},
			settings: agentcore.CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 100000, Proactive: true, Semantic: true},
			observed: small("short"),
			want:     agentcore.StrategyProactive,
		},
		{
			name:     "reactive_beats_proactive_in_precedence",
			msgs:     []llm.Message{{Role: "user", Content: strings.Repeat("a", 4000)}},
			settings: agentcore.CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 1000, Proactive: true},
			observed: big(strings.Repeat("a", 4000)),
			want:     agentcore.StrategyReactive,
		},
		{
			name:     "no_flags_falls_to_emergency",
			msgs:     []llm.Message{{Role: "user", Content: "short"}},
			settings: agentcore.CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 100000},
			observed: small("short"),
			want:     agentcore.StrategyEmergency,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := agentcore.SelectStrategy(tc.msgs, tc.settings, tc.observed)
			if got != tc.want {
				t.Errorf("agentcore.SelectStrategy = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCompactionStrategyConstants(t *testing.T) {
	if string(agentcore.StrategyReactive) != "reactive" {
		t.Errorf("agentcore.StrategyReactive = %q, want reactive", agentcore.StrategyReactive)
	}
	if string(agentcore.StrategyProactive) != "proactive" {
		t.Errorf("agentcore.StrategyProactive = %q, want proactive", agentcore.StrategyProactive)
	}
	if string(agentcore.StrategySemantic) != "semantic" {
		t.Errorf("agentcore.StrategySemantic = %q, want semantic", agentcore.StrategySemantic)
	}
	if string(agentcore.StrategyEmergency) != "emergency" {
		t.Errorf("agentcore.StrategyEmergency = %q, want emergency", agentcore.StrategyEmergency)
	}
}

func TestCompactionActionConstants(t *testing.T) {
	if string(agentcore.ActionNone) != "none" {
		t.Errorf("agentcore.ActionNone = %q, want none", agentcore.ActionNone)
	}
	if string(agentcore.ActionBackgroundStarted) != "background_started" {
		t.Errorf("agentcore.ActionBackgroundStarted = %q, want background_started", agentcore.ActionBackgroundStarted)
	}
	if string(agentcore.ActionFullCompacted) != "full_compacted" {
		t.Errorf("agentcore.ActionFullCompacted = %q, want full_compacted", agentcore.ActionFullCompacted)
	}
	if string(agentcore.ActionEmergencyHard) != "emergency_hard" {
		t.Errorf("agentcore.ActionEmergencyHard = %q, want emergency_hard", agentcore.ActionEmergencyHard)
	}
	if string(agentcore.ActionIncrementalRecovered) != "incremental_recovered" {
		t.Errorf("agentcore.ActionIncrementalRecovered = %q, want incremental_recovered", agentcore.ActionIncrementalRecovered)
	}
}

func TestCompactionStatsFields(t *testing.T) {
	obs := 1234
	s := agentcore.CompactionStats{
		TotalTurns:          10,
		ActiveMessages:      20,
		HasSummary:          true,
		IsCompacting:        false,
		TokenEstimate:       5000,
		EffectiveTokens:     4500,
		ObservedInputTokens: &obs,
		ContextUsage:        0.45,
	}
	if s.TotalTurns != 10 {
		t.Errorf("TotalTurns = %d", s.TotalTurns)
	}
	if s.ObservedInputTokens == nil || *s.ObservedInputTokens != 1234 {
		t.Errorf("ObservedInputTokens = %v", s.ObservedInputTokens)
	}
	if s.ContextUsage != 0.45 {
		t.Errorf("ContextUsage = %f", s.ContextUsage)
	}
}

func TestProactiveActOnReturnsActionNone(t *testing.T) {
	a := &agentcore.Agent{}
	msgs := []llm.Message{{Role: "user", Content: "hi"}}
	out, action, err := agentcore.StrategyProactive.ActOn(context.Background(), a, msgs, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != agentcore.ActionNone {
		t.Errorf("action = %q, want %q", action, agentcore.ActionNone)
	}
	if len(out) != 1 || out[0].Content != "hi" {
		t.Errorf("msgs changed unexpectedly: %+v", out)
	}
}

func TestSemanticActOnReturnsActionNone(t *testing.T) {
	a := &agentcore.Agent{}
	msgs := []llm.Message{{Role: "user", Content: "hi"}}
	out, action, err := agentcore.StrategySemantic.ActOn(context.Background(), a, msgs, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != agentcore.ActionNone {
		t.Errorf("action = %q, want %q", action, agentcore.ActionNone)
	}
	if len(out) != 1 || out[0].Content != "hi" {
		t.Errorf("msgs changed unexpectedly: %+v", out)
	}
}

func TestEmergencyActOnReturnsActionNone(t *testing.T) {
	a := &agentcore.Agent{}
	msgs := []llm.Message{{Role: "user", Content: "hi"}}
	out, action, err := agentcore.StrategyEmergency.ActOn(context.Background(), a, msgs, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != agentcore.ActionNone {
		t.Errorf("action = %q, want %q", action, agentcore.ActionNone)
	}
	if len(out) != 1 || out[0].Content != "hi" {
		t.Errorf("msgs changed unexpectedly: %+v", out)
	}
}

func TestReactiveActOnEmptyMsgsReturnsInput(t *testing.T) {
	a := &agentcore.Agent{}
	out, action, err := agentcore.StrategyReactive.ActOn(context.Background(), a, nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != agentcore.ActionFullCompacted {
		t.Errorf("action = %q, want %q (Reactive returns full_compacted)", action, agentcore.ActionFullCompacted)
	}
	if out != nil {
		t.Errorf("out = %+v, want nil", out)
	}
}

func TestReactiveActOnDelegatesToRunReactive(t *testing.T) {
	a := &agentcore.Agent{}
	a.SetCoreForTest(&strategyTestFakeCore{})
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
	}
	out, action, err := agentcore.StrategyReactive.ActOn(context.Background(), a, msgs, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != agentcore.ActionFullCompacted {
		t.Errorf("action = %q, want %q", action, agentcore.ActionFullCompacted)
	}
	if len(out) < 2 {
		t.Fatalf("expected system + summary + recent, got len=%d", len(out))
	}
	if out[0].Role != "system" {
		t.Errorf("first msg role = %q, want system", out[0].Role)
	}
	if !strings.Contains(out[1].Content, "Previous conversation summary:") {
		t.Errorf("missing summary marker in %q", out[1].Content)
	}
}

type strategyTestFakeCore struct{}

func (strategyTestFakeCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 4)
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		Index: 0,
		Delta: llm.Message{Content: "summary text"},
	}}}}
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{FinishReason: llm.FinishReasonStop}}}}
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{}}
	close(ch)
	return ch, nil
}

func TestStrategyEnumsAreDistinct(t *testing.T) {
	all := []agentcore.CompactionStrategy{agentcore.StrategyReactive, agentcore.StrategyProactive, agentcore.StrategySemantic, agentcore.StrategyEmergency}
	seen := map[agentcore.CompactionStrategy]bool{}
	for _, s := range all {
		if seen[s] {
			t.Errorf("duplicate strategy value: %q", s)
		}
		seen[s] = true
	}
}

func TestActionEnumsAreDistinct(t *testing.T) {
	all := []agentcore.CompactionAction{agentcore.ActionNone, agentcore.ActionBackgroundStarted, agentcore.ActionFullCompacted, agentcore.ActionEmergencyHard, agentcore.ActionIncrementalRecovered}
	seen := map[agentcore.CompactionAction]bool{}
	for _, s := range all {
		if seen[s] {
			t.Errorf("duplicate action value: %q", s)
		}
		seen[s] = true
	}
}

func TestStrategyActOnCtxCancelled(t *testing.T) {
	a := &agentcore.Agent{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := agentcore.StrategyProactive.ActOn(ctx, a, nil, nil)
	if err != nil {
		t.Fatalf("proactive should not error on cancelled ctx for stub: %v", err)
	}
}
