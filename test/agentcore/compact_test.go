package agentcore_test

import (
	"strings"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/llm"
)

func TestSummarizationPromptSections(t *testing.T) {
	prompt := agentcore.SummarizationPrompt
	requiredSections := []string{
		"## Goal",
		"## Constraints & Preferences",
		"## Progress",
		"### Done",
		"### In Progress",
		"### Blocked",
		"## Key Decisions",
		"## Next Steps",
		"## Critical Context",
	}
	for _, section := range requiredSections {
		if !strings.Contains(prompt, section) {
			t.Errorf("SummarizationPrompt missing section %q", section)
		}
	}
}

func TestSummarizationPromptInstructsNotToContinue(t *testing.T) {
	prompt := agentcore.SummarizationPrompt
	lowered := strings.ToLower(prompt)
	wantPhrases := []string{"do not continue", "do not respond", "only"}
	for _, want := range wantPhrases {
		if !strings.Contains(lowered, want) {
			t.Errorf("SummarizationPrompt missing %q (must instruct model to summarize only)", want)
		}
	}
}

func TestUpdateSummarizationPromptPreservesRules(t *testing.T) {
	prompt := agentcore.UpdateSummarizationPrompt
	requiredSections := []string{
		"## Goal",
		"## Constraints & Preferences",
		"## Progress",
		"### Done",
		"### In Progress",
		"### Blocked",
		"## Key Decisions",
		"## Next Steps",
		"## Critical Context",
	}
	for _, section := range requiredSections {
		if !strings.Contains(prompt, section) {
			t.Errorf("UpdateSummarizationPrompt missing section %q", section)
		}
	}
	lowered := strings.ToLower(prompt)
	for _, phrase := range []string{"preserve", "update", "previous"} {
		if !strings.Contains(lowered, phrase) {
			t.Errorf("UpdateSummarizationPrompt must mention %q (iterative update semantics)", phrase)
		}
	}
}

func TestShouldCompactTriggersAboveReserve(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: string(make([]byte, 40000))},
		{Role: "assistant", Content: string(make([]byte, 40000))},
	}
	settings := agentcore.CompactionSettings{Enabled: true, ReserveTokens: 1024}
	if got := agentcore.ShouldCompactWithModel(msgs, llm.Model{ID: "m", MaxContextTokens: 10000}, settings); !got {
		t.Errorf("expected shouldCompact=true (20000 tokens > 10000-1024=8976)")
	}
}

func TestShouldCompactDisabledAlwaysFalse(t *testing.T) {
	msgs := []llm.Message{{Role: "user", Content: string(make([]byte, 100000))}}
	settings := agentcore.CompactionSettings{Enabled: false, ReserveTokens: 0}
	if got := agentcore.ShouldCompactWithModel(msgs, llm.Model{ID: "m", MaxContextTokens: 1000}, settings); got {
		t.Errorf("disabled should never compact")
	}
}

func TestShouldCompactBelowReserveStays(t *testing.T) {
	msgs := []llm.Message{{Role: "user", Content: string(make([]byte, 1000))}}
	settings := agentcore.CompactionSettings{Enabled: true, ReserveTokens: 5000}
	if got := agentcore.ShouldCompactWithModel(msgs, llm.Model{ID: "m", MaxContextTokens: 10000}, settings); got {
		t.Errorf("expected shouldCompact=false (250 tokens < 10000-5000=5000)")
	}
}

func TestShouldCompactFallsBackToDefaultWhenModelZero(t *testing.T) {
	msgs := []llm.Message{{Role: "user", Content: string(make([]byte, 600000))}}
	settings := agentcore.CompactionSettings{Enabled: true, ReserveTokens: 100}
	if got := agentcore.ShouldCompactWithModel(msgs, llm.Model{ID: "m", MaxContextTokens: 0}, settings); !got {
		t.Errorf("expected compact: model has no window, fallback 128000, 150000 tokens > 128000-100")
	}
}
