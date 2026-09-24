package agentcore

import (
	"context"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
)

func TestSelectStrategyTableDriven(t *testing.T) {
	big := func(s string) *int { v := len(s) * 200; return &v }
	small := func(s string) *int { v := 10; return &v }
	none := func() *int { return nil }

	cases := []struct {
		name     string
		msgs     []llm.Message
		settings CompactionSettings
		observed *int
		want     CompactionStrategy
	}{
		{
			name:     "disabled_falls_to_emergency",
			msgs:     []llm.Message{{Role: "user", Content: "x"}},
			settings: CompactionSettings{Enabled: false, ReserveTokens: 100, MaxContextTokens: 1000},
			observed: none(),
			want:     StrategyEmergency,
		},
		{
			name:     "should_compact_returns_reactive",
			msgs:     []llm.Message{{Role: "user", Content: strings.Repeat("a", 4000)}},
			settings: CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 1000},
			observed: big(strings.Repeat("a", 4000)),
			want:     StrategyReactive,
		},
		{
			name:     "below_threshold_with_proactive_returns_proactive",
			msgs:     []llm.Message{{Role: "user", Content: "short"}},
			settings: CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 100000, Proactive: true},
			observed: small("short"),
			want:     StrategyProactive,
		},
		{
			name:     "below_threshold_with_semantic_returns_semantic",
			msgs:     []llm.Message{{Role: "user", Content: "short"}},
			settings: CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 100000, Semantic: true},
			observed: small("short"),
			want:     StrategySemantic,
		},
		{
			name:     "proactive_beats_semantic_in_precedence",
			msgs:     []llm.Message{{Role: "user", Content: "short"}},
			settings: CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 100000, Proactive: true, Semantic: true},
			observed: small("short"),
			want:     StrategyProactive,
		},
		{
			name:     "reactive_beats_proactive_in_precedence",
			msgs:     []llm.Message{{Role: "user", Content: strings.Repeat("a", 4000)}},
			settings: CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 1000, Proactive: true},
			observed: big(strings.Repeat("a", 4000)),
			want:     StrategyReactive,
		},
		{
			name:     "no_flags_falls_to_emergency",
			msgs:     []llm.Message{{Role: "user", Content: "short"}},
			settings: CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 100000},
			observed: small("short"),
			want:     StrategyEmergency,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SelectStrategy(tc.msgs, tc.settings, tc.observed)
			if got != tc.want {
				t.Errorf("SelectStrategy = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCompactionStrategyConstants(t *testing.T) {
	if string(StrategyReactive) != "reactive" {
		t.Errorf("StrategyReactive = %q, want reactive", StrategyReactive)
	}
	if string(StrategyProactive) != "proactive" {
		t.Errorf("StrategyProactive = %q, want proactive", StrategyProactive)
	}
	if string(StrategySemantic) != "semantic" {
		t.Errorf("StrategySemantic = %q, want semantic", StrategySemantic)
	}
	if string(StrategyEmergency) != "emergency" {
		t.Errorf("StrategyEmergency = %q, want emergency", StrategyEmergency)
	}
}

func TestCompactionActionConstants(t *testing.T) {
	if string(ActionNone) != "none" {
		t.Errorf("ActionNone = %q, want none", ActionNone)
	}
	if string(ActionBackgroundStarted) != "background_started" {
		t.Errorf("ActionBackgroundStarted = %q, want background_started", ActionBackgroundStarted)
	}
	if string(ActionFullCompacted) != "full_compacted" {
		t.Errorf("ActionFullCompacted = %q, want full_compacted", ActionFullCompacted)
	}
	if string(ActionEmergencyHard) != "emergency_hard" {
		t.Errorf("ActionEmergencyHard = %q, want emergency_hard", ActionEmergencyHard)
	}
	if string(ActionIncrementalRecovered) != "incremental_recovered" {
		t.Errorf("ActionIncrementalRecovered = %q, want incremental_recovered", ActionIncrementalRecovered)
	}
}

func TestCompactionStatsFields(t *testing.T) {
	obs := 1234
	s := CompactionStats{
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
	a := &Agent{}
	msgs := []llm.Message{{Role: "user", Content: "hi"}}
	out, action, err := StrategyProactive.ActOn(context.Background(), a, msgs, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != ActionNone {
		t.Errorf("action = %q, want %q", action, ActionNone)
	}
	if len(out) != 1 || out[0].Content != "hi" {
		t.Errorf("msgs changed unexpectedly: %+v", out)
	}
}

func TestSemanticActOnReturnsActionNone(t *testing.T) {
	a := &Agent{}
	msgs := []llm.Message{{Role: "user", Content: "hi"}}
	out, action, err := StrategySemantic.ActOn(context.Background(), a, msgs, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != ActionNone {
		t.Errorf("action = %q, want %q", action, ActionNone)
	}
	if len(out) != 1 || out[0].Content != "hi" {
		t.Errorf("msgs changed unexpectedly: %+v", out)
	}
}

func TestEmergencyActOnReturnsActionNone(t *testing.T) {
	a := &Agent{}
	msgs := []llm.Message{{Role: "user", Content: "hi"}}
	out, action, err := StrategyEmergency.ActOn(context.Background(), a, msgs, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != ActionNone {
		t.Errorf("action = %q, want %q", action, ActionNone)
	}
	if len(out) != 1 || out[0].Content != "hi" {
		t.Errorf("msgs changed unexpectedly: %+v", out)
	}
}

func TestReactiveActOnEmptyMsgsReturnsInput(t *testing.T) {
	a := &Agent{}
	out, action, err := StrategyReactive.ActOn(context.Background(), a, nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != ActionFullCompacted {
		t.Errorf("action = %q, want %q (Reactive returns full_compacted)", action, ActionFullCompacted)
	}
	if out != nil {
		t.Errorf("out = %+v, want nil", out)
	}
}

func TestReactiveActOnDelegatesToRunReactive(t *testing.T) {
	a := &Agent{core: &strategyTestFakeCore{}}
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
	}
	out, action, err := StrategyReactive.ActOn(context.Background(), a, msgs, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != ActionFullCompacted {
		t.Errorf("action = %q, want %q", action, ActionFullCompacted)
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
	all := []CompactionStrategy{StrategyReactive, StrategyProactive, StrategySemantic, StrategyEmergency}
	seen := map[CompactionStrategy]bool{}
	for _, s := range all {
		if seen[s] {
			t.Errorf("duplicate strategy value: %q", s)
		}
		seen[s] = true
	}
}

func TestActionEnumsAreDistinct(t *testing.T) {
	all := []CompactionAction{ActionNone, ActionBackgroundStarted, ActionFullCompacted, ActionEmergencyHard, ActionIncrementalRecovered}
	seen := map[CompactionAction]bool{}
	for _, s := range all {
		if seen[s] {
			t.Errorf("duplicate action value: %q", s)
		}
		seen[s] = true
	}
}

func TestStrategyActOnCtxCancelled(t *testing.T) {
	a := &Agent{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := StrategyProactive.ActOn(ctx, a, nil, nil)
	if err != nil {
		t.Fatalf("proactive should not error on cancelled ctx for stub: %v", err)
	}
}
