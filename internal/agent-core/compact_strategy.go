package agentcore

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/vanpiyp/awp/internal/llm"
)

type CompactionStrategy string

const (
	StrategyReactive  CompactionStrategy = "reactive"
	StrategyProactive CompactionStrategy = "proactive"
	StrategySemantic  CompactionStrategy = "semantic"
	StrategyEmergency CompactionStrategy = "emergency"
)

type CompactionAction string

const (
	ActionNone                 CompactionAction = "none"
	ActionBackgroundStarted    CompactionAction = "background_started"
	ActionFullCompacted        CompactionAction = "full_compacted"
	ActionEmergencyHard        CompactionAction = "emergency_hard"
	ActionIncrementalRecovered CompactionAction = "incremental_recovered"
)

type CompactionStats struct {
	TotalTurns          int
	ActiveMessages      int
	HasSummary          bool
	IsCompacting        bool
	TokenEstimate       int
	EffectiveTokens     int
	ObservedInputTokens *int
	ContextUsage        float32
}

func SelectStrategy(msgs []llm.Message, settings CompactionSettings, observedInput *int) CompactionStrategy {
	model := llm.Model{MaxContextTokens: settings.MaxContextTokens}
	if settings.Enabled && ShouldCompactWithModel(msgs, model, settings) {
		return StrategyReactive
	}
	if settings.Enabled && settings.Proactive {
		return StrategyProactive
	}
	if settings.Enabled && settings.Semantic {
		return StrategySemantic
	}
	return StrategyEmergency
}

func (s CompactionStrategy) ActOn(ctx context.Context, a *Agent, msgs []llm.Message, observed *int) ([]llm.Message, CompactionAction, error) {
	switch s {
	case StrategyReactive:
		previousSummary := ExtractPreviousSummary(msgs)
		out, err := runReactiveCompaction(ctx, a, msgs, previousSummary)
		if err != nil {
			return msgs, ActionNone, err
		}
		return out, ActionFullCompacted, nil
	case StrategyProactive, StrategySemantic, StrategyEmergency:
		return msgs, ActionNone, nil
	default:
		return msgs, ActionNone, fmt.Errorf("unknown compaction strategy: %q", s)
	}
}

func runReactiveCompaction(ctx context.Context, a *Agent, msgs []llm.Message, previousSummary string) ([]llm.Message, error) {
	if len(msgs) == 0 {
		return msgs, nil
	}

	systemMsg := msgs[0]
	restMsgs := msgs[1:]
	cutPoint := FindCutPoint(restMsgs, a.compaction.KeepRecentTurns)

	if cutPoint >= len(restMsgs) {
		slog.Debug("agent: nothing to compact")
		return msgs, nil
	}

	toSummarize := restMsgs[:cutPoint]
	recent := restMsgs[cutPoint:]

	slog.Debug("agent: compacting", "messages_to_summarize", len(toSummarize), "messages_to_keep", len(recent))

	systemContent := SummarizationPrompt
	if previousSummary != "" {
		systemContent = UpdateSummarizationPrompt
	}

	userContent := SerializeForSummary(toSummarize)
	if previousSummary != "" {
		userContent = "<previous-summary>\n" + previousSummary + "\n</previous-summary>\n\n" + userContent
	}

	summaryReq := &llm.ChatRequest{
		Model: a.Model.ID,
		Messages: []llm.Message{
			{Role: "system", Content: systemContent},
			{Role: "user", Content: userContent},
		},
	}

	var summaryText strings.Builder
	var finishReason llm.FinishReason
	events, err := a.core.StreamChat(ctx, summaryReq)
	if err != nil {
		return nil, fmt.Errorf("summarize: %w", err)
	}
	for ev := range events {
		if ev.Err != nil {
			return nil, fmt.Errorf("summarize stream: %w", ev.Err)
		}
		if ev.Chunk == nil {
			continue
		}
		for _, c := range ev.Chunk.Choices {
			summaryText.WriteString(c.Delta.Content)
			if c.FinishReason != llm.FinishReasonUnknown {
				finishReason = c.FinishReason
			}
		}
	}

	if finishReason == llm.FinishReasonLength {
		return nil, fmt.Errorf("summarize: generation hit the token cap and the summary is incomplete")
	}

	fileOps := ExtractFileOps(toSummarize)
	readOnly, modified := ComputeFileLists(fileOps)
	summaryText.WriteString(FormatFileOperations(readOnly, modified))

	summaryMsg := llm.Message{
		Role:    "assistant",
		Content: "Previous conversation summary:\n" + summaryText.String(),
	}

	out := make([]llm.Message, 0, 2+len(recent))
	out = append(out, systemMsg, summaryMsg)
	out = append(out, recent...)

	a.logMu.Lock()
	defer a.logMu.Unlock()
	tokensBefore := estimateTotalTokens(msgs)
	tokensAfter := estimateTotalTokens(out)
	a.writeCompactionLocked(Event{
		Category:        EventCompaction,
		Summary:         summaryText.String(),
		TokensBefore:    tokensBefore,
		TokensAfter:     tokensAfter,
		FirstKeptSeq:    a.logSeq,
		CompactionModel: a.Model.ID,
	})
	if a.logBuf != nil {
		_ = a.logBuf.Flush()
	}
	return out, nil
}
