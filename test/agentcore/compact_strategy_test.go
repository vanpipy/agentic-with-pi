package agentcore_test

import (
	"context"
	"strings"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-core/compact"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/streamtest"
)

func TestSelectStrategyTableDriven(t *testing.T) {
	big := func(s string) *int { v := len(s) * 200; return &v }
	small := func(s string) *int { v := 10; return &v }
	none := func() *int { return nil }

	cases := []struct {
		name     string
		msgs     []llm.Message
		settings compact.CompactionSettings
		observed *int
		want     compact.CompactionStrategy
	}{
		{
			name:     "disabled_falls_to_emergency",
			msgs:     []llm.Message{{Role: "user", Content: "x"}},
			settings: compact.CompactionSettings{Enabled: false, ReserveTokens: 100, MaxContextTokens: 1000},
			observed: none(),
			want:     compact.StrategyEmergency,
		},
		{
			name:     "should_compact_returns_reactive",
			msgs:     []llm.Message{{Role: "user", Content: strings.Repeat("a", 4000)}},
			settings: compact.CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 1000},
			observed: big(strings.Repeat("a", 4000)),
			want:     compact.StrategyReactive,
		},
		{
			name:     "below_threshold_with_proactive_returns_proactive",
			msgs:     []llm.Message{{Role: "user", Content: "short"}},
			settings: compact.CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 100000, Proactive: true},
			observed: small("short"),
			want:     compact.StrategyProactive,
		},
		{
			name:     "below_threshold_with_semantic_returns_semantic",
			msgs:     []llm.Message{{Role: "user", Content: "short"}},
			settings: compact.CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 100000, Semantic: true},
			observed: small("short"),
			want:     compact.StrategySemantic,
		},
		{
			name:     "proactive_beats_semantic_in_precedence",
			msgs:     []llm.Message{{Role: "user", Content: "short"}},
			settings: compact.CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 100000, Proactive: true, Semantic: true},
			observed: small("short"),
			want:     compact.StrategyProactive,
		},
		{
			name:     "reactive_beats_proactive_in_precedence",
			msgs:     []llm.Message{{Role: "user", Content: strings.Repeat("a", 4000)}},
			settings: compact.CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 1000, Proactive: true},
			observed: big(strings.Repeat("a", 4000)),
			want:     compact.StrategyReactive,
		},
		{
			name:     "no_flags_falls_to_emergency",
			msgs:     []llm.Message{{Role: "user", Content: "short"}},
			settings: compact.CompactionSettings{Enabled: true, ReserveTokens: 100, MaxContextTokens: 100000},
			observed: small("short"),
			want:     compact.StrategyEmergency,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := compact.SelectStrategy(tc.msgs, tc.settings, tc.observed)
			if got != tc.want {
				t.Errorf("compact.SelectStrategy = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCompactionStrategyConstants(t *testing.T) {
	if string(compact.StrategyReactive) != "reactive" {
		t.Errorf("compact.StrategyReactive = %q, want reactive", compact.StrategyReactive)
	}
	if string(compact.StrategyProactive) != "proactive" {
		t.Errorf("compact.StrategyProactive = %q, want proactive", compact.StrategyProactive)
	}
	if string(compact.StrategySemantic) != "semantic" {
		t.Errorf("compact.StrategySemantic = %q, want semantic", compact.StrategySemantic)
	}
	if string(compact.StrategyEmergency) != "emergency" {
		t.Errorf("compact.StrategyEmergency = %q, want emergency", compact.StrategyEmergency)
	}
}

func TestCompactionActionConstants(t *testing.T) {
	if string(compact.ActionNone) != "none" {
		t.Errorf("compact.ActionNone = %q, want none", compact.ActionNone)
	}
	if string(compact.ActionBackgroundStarted) != "background_started" {
		t.Errorf("compact.ActionBackgroundStarted = %q, want background_started", compact.ActionBackgroundStarted)
	}
	if string(compact.ActionFullCompacted) != "full_compacted" {
		t.Errorf("compact.ActionFullCompacted = %q, want full_compacted", compact.ActionFullCompacted)
	}
	if string(compact.ActionEmergencyHard) != "emergency_hard" {
		t.Errorf("compact.ActionEmergencyHard = %q, want emergency_hard", compact.ActionEmergencyHard)
	}
	if string(compact.ActionIncrementalRecovered) != "incremental_recovered" {
		t.Errorf("compact.ActionIncrementalRecovered = %q, want incremental_recovered", compact.ActionIncrementalRecovered)
	}
}

func TestCompactionStatsFields(t *testing.T) {
	obs := 1234
	s := compact.CompactionStats{
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
	out, action, err := compact.StrategyProactive.ActOn(context.Background(), a, msgs, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != compact.ActionNone {
		t.Errorf("action = %q, want %q", action, compact.ActionNone)
	}
	if len(out) != 1 || out[0].Content != "hi" {
		t.Errorf("msgs changed unexpectedly: %+v", out)
	}
}

func TestSemanticActOnReturnsActionNone(t *testing.T) {
	a := &agentcore.Agent{}
	msgs := []llm.Message{{Role: "user", Content: "hi"}}
	out, action, err := compact.StrategySemantic.ActOn(context.Background(), a, msgs, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != compact.ActionNone {
		t.Errorf("action = %q, want %q", action, compact.ActionNone)
	}
	if len(out) != 1 || out[0].Content != "hi" {
		t.Errorf("msgs changed unexpectedly: %+v", out)
	}
}

func TestEmergencyActOnReturnsActionNone(t *testing.T) {
	a := &agentcore.Agent{}
	msgs := []llm.Message{{Role: "user", Content: "hi"}}
	out, action, err := compact.StrategyEmergency.ActOn(context.Background(), a, msgs, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != compact.ActionNone {
		t.Errorf("action = %q, want %q", action, compact.ActionNone)
	}
	if len(out) != 1 || out[0].Content != "hi" {
		t.Errorf("msgs changed unexpectedly: %+v", out)
	}
}

func TestReactiveActOnEmptyMsgsReturnsInput(t *testing.T) {
	a := &agentcore.Agent{}
	out, action, err := compact.StrategyReactive.ActOn(context.Background(), a, nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != compact.ActionFullCompacted {
		t.Errorf("action = %q, want %q (Reactive returns full_compacted)", action, compact.ActionFullCompacted)
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
	out, action, err := compact.StrategyReactive.ActOn(context.Background(), a, msgs, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != compact.ActionFullCompacted {
		t.Errorf("action = %q, want %q", action, compact.ActionFullCompacted)
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
	ch <- streamtest.Text("summary text")
	ch <- streamtest.Finish(llm.FinishReasonStop)
	ch <- streamtest.NoOp()
	close(ch)
	return ch, nil
}

func TestStrategyEnumsAreDistinct(t *testing.T) {
	all := []compact.CompactionStrategy{compact.StrategyReactive, compact.StrategyProactive, compact.StrategySemantic, compact.StrategyEmergency}
	seen := map[compact.CompactionStrategy]bool{}
	for _, s := range all {
		if seen[s] {
			t.Errorf("duplicate strategy value: %q", s)
		}
		seen[s] = true
	}
}

func TestActionEnumsAreDistinct(t *testing.T) {
	all := []compact.CompactionAction{compact.ActionNone, compact.ActionBackgroundStarted, compact.ActionFullCompacted, compact.ActionEmergencyHard, compact.ActionIncrementalRecovered}
	seen := map[compact.CompactionAction]bool{}
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
	_, _, err := compact.StrategyProactive.ActOn(ctx, a, nil, nil)
	if err != nil {
		t.Fatalf("proactive should not error on cancelled ctx for stub: %v", err)
	}
}

func TestBuildSummaryRequestUsesFreshPromptWhenNoPrev(t *testing.T) {
	msgs := []llm.Message{{Role: "user", Content: "u1"}}
	req := compact.BuildSummaryRequest(msgs, "", "model-x")
	if req == nil {
		t.Fatal("BuildSummaryRequest returned nil")
	}
	if req.Model != "model-x" {
		t.Errorf("Model = %q, want model-x", req.Model)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2 (system + user)", len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		t.Errorf("Messages[0].Role = %q, want system", req.Messages[0].Role)
	}
	if req.Messages[0].Content != compact.SummarizationPrompt {
		t.Errorf("first system prompt should be SummarizationPrompt when no previous summary")
	}
	if req.Messages[1].Role != "user" {
		t.Errorf("Messages[1].Role = %q, want user", req.Messages[1].Role)
	}
	if !strings.Contains(req.Messages[1].Content, "u1") {
		t.Errorf("user message content missing serialized data: %q", req.Messages[1].Content)
	}
}

func TestBuildSummaryRequestUsesUpdatePromptWhenPrev(t *testing.T) {
	msgs := []llm.Message{{Role: "user", Content: "u1"}}
	prev := "previous summary content"
	req := compact.BuildSummaryRequest(msgs, prev, "model-x")
	if req.Messages[0].Content != compact.UpdateSummarizationPrompt {
		t.Errorf("expected UpdateSummarizationPrompt when previous summary is non-empty")
	}
	if !strings.Contains(req.Messages[1].Content, prev) {
		t.Errorf("user content missing previous-summary block: %q", req.Messages[1].Content)
	}
	if !strings.Contains(req.Messages[1].Content, "<previous-summary>") {
		t.Errorf("user content missing <previous-summary> wrapper")
	}
	if !strings.Contains(req.Messages[1].Content, "u1") {
		t.Errorf("user content missing serialized messages: %q", req.Messages[1].Content)
	}
}

func TestStreamSummaryReturnsSummaryOnSuccess(t *testing.T) {
	core := streamSummaryStubCore{chunks: []string{"hello ", "world"}, finish: llm.FinishReasonStop}
	req := &llm.ChatRequest{Model: "m", Messages: []llm.Message{{Role: "user", Content: "x"}}}
	got, err := compact.StreamSummary(context.Background(), core, req)
	if err != nil {
		t.Fatalf("StreamSummary err: %v", err)
	}
	if got != "hello world" {
		t.Errorf("StreamSummary = %q, want %q", got, "hello world")
	}
}

func TestStreamSummaryReturnsErrorOnLLMFailure(t *testing.T) {
	core := streamSummaryStubCore{returnErr: errFakeLLM}
	req := &llm.ChatRequest{Model: "m", Messages: []llm.Message{{Role: "user", Content: "x"}}}
	_, err := compact.StreamSummary(context.Background(), core, req)
	if err == nil {
		t.Fatal("expected error from failing core, got nil")
	}
	if !strings.Contains(err.Error(), "summarize") {
		t.Errorf("error should mention summarize: %v", err)
	}
}

func TestStreamSummaryReturnsErrorOnLengthFinish(t *testing.T) {
	core := streamSummaryStubCore{chunks: []string{"partial"}, finish: llm.FinishReasonLength}
	req := &llm.ChatRequest{Model: "m", Messages: []llm.Message{{Role: "user", Content: "x"}}}
	_, err := compact.StreamSummary(context.Background(), core, req)
	if err == nil {
		t.Fatal("expected error on FinishReasonLength, got nil")
	}
	if !strings.Contains(err.Error(), "token cap") {
		t.Errorf("error should mention token cap on length: %v", err)
	}
}

func TestAppendFileOpsSummaryAppendsFromExtractedOps(t *testing.T) {
	systemMsg := llm.Message{Role: "system", Content: "sys"}
	recent := []llm.Message{{Role: "user", Content: "u2"}}
	toSummarize := []llm.Message{
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1", ToolCalls: []llm.ToolCall{{
			ID:   "t1",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "write",
				Arguments: `{"path":"foo.go"}`,
			},
		}}},
		{Role: "assistant", Content: "a2", ToolCalls: []llm.ToolCall{{
			ID:   "t2",
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "read",
				Arguments: `{"path":"bar.go"}`,
			},
		}}},
	}
	out, finalText := compact.AppendFileOpsSummary(systemMsg, toSummarize, recent, "summary body")
	if !strings.Contains(finalText, "<modified-files>") {
		t.Errorf("finalText missing <modified-files>: %q", finalText)
	}
	if !strings.Contains(finalText, "foo.go") {
		t.Errorf("finalText missing modified file foo.go: %q", finalText)
	}
	if !strings.Contains(finalText, "<read-files>") {
		t.Errorf("finalText missing <read-files>: %q", finalText)
	}
	if !strings.Contains(finalText, "bar.go") {
		t.Errorf("finalText missing read file bar.go: %q", finalText)
	}
	if !strings.Contains(finalText, "summary body") {
		t.Errorf("finalText missing summary body: %q", finalText)
	}
	if len(out) != 3 {
		t.Fatalf("len(out) = %d, want 3 (system + summary + 1 recent)", len(out))
	}
	if out[0].Role != "system" || out[0].Content != "sys" {
		t.Errorf("out[0] = %+v, want system msg", out[0])
	}
	if out[1].Role != "assistant" {
		t.Errorf("out[1].Role = %q, want assistant", out[1].Role)
	}
	if !strings.HasPrefix(out[1].Content, "Previous conversation summary:\n") {
		t.Errorf("out[1] missing summary prefix: %q", out[1].Content)
	}
	if !strings.Contains(out[1].Content, "summary body") {
		t.Errorf("out[1] missing summary body: %q", out[1].Content)
	}
	if !strings.Contains(out[1].Content, "<modified-files>") {
		t.Errorf("out[1] missing <modified-files> tag: %q", out[1].Content)
	}
	if !strings.Contains(out[1].Content, "foo.go") {
		t.Errorf("out[1] missing modified file foo.go: %q", out[1].Content)
	}
	if !strings.Contains(out[1].Content, "<read-files>") {
		t.Errorf("out[1] missing <read-files> tag: %q", out[1].Content)
	}
	if !strings.Contains(out[1].Content, "bar.go") {
		t.Errorf("out[1] missing read file bar.go: %q", out[1].Content)
	}
	if out[2].Role != "user" || out[2].Content != "u2" {
		t.Errorf("out[2] = %+v, want recent msg", out[2])
	}
}

func TestAppendFileOpsSummaryNoFileOpsHasNoSections(t *testing.T) {
	systemMsg := llm.Message{Role: "system", Content: "sys"}
	recent := []llm.Message{{Role: "user", Content: "u2"}}
	toSummarize := []llm.Message{
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
	}
	out, finalText := compact.AppendFileOpsSummary(systemMsg, toSummarize, recent, "summary body")
	if len(out) != 3 {
		t.Fatalf("len(out) = %d, want 3", len(out))
	}
	if strings.Contains(out[1].Content, "<modified-files>") {
		t.Errorf("out[1] should not contain <modified-files> when no tools: %q", out[1].Content)
	}
	if strings.Contains(out[1].Content, "<read-files>") {
		t.Errorf("out[1] should not contain <read-files> when no tools: %q", out[1].Content)
	}
	if !strings.Contains(out[1].Content, "summary body") {
		t.Errorf("out[1] missing summary body: %q", out[1].Content)
	}
	if strings.Contains(finalText, "<modified-files>") {
		t.Errorf("finalText should not contain <modified-files> when no tools: %q", finalText)
	}
	if strings.Contains(finalText, "<read-files>") {
		t.Errorf("finalText should not contain <read-files> when no tools: %q", finalText)
	}
	if finalText != "summary body" {
		t.Errorf("finalText = %q, want summary body (no file ops to append)", finalText)
	}
}

type streamSummaryStubCore struct {
	chunks    []string
	finish    llm.FinishReason
	returnErr error
}

func (s streamSummaryStubCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	if s.returnErr != nil {
		return nil, s.returnErr
	}
	ch := make(chan llm.StreamEvent, len(s.chunks)+1)
	for _, c := range s.chunks {
		ch <- streamtest.Text(c)
	}
	ch <- streamtest.Finish(s.finish)
	close(ch)
	return ch, nil
}

var errFakeLLM = &llm.Error{Kind: llm.ErrorKindServer, Message: "stub failure"}
