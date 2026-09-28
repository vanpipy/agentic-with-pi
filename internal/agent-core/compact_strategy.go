package agentcore

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
	compactionStart := time.Now()
	systemMsg, toSummarize, recent, ok := splitReactiveInputs(msgs, a.compaction.KeepRecentTurns)
	if !ok {
		slog.Debug("agent: nothing to compact")
		return msgs, nil
	}
	slog.Debug("agent: compacting", "messages_to_summarize", len(toSummarize), "messages_to_keep", len(recent))
	summaryText, err := StreamSummary(ctx, a.core, BuildSummaryRequest(toSummarize, previousSummary, a.Model.ID))
	if err != nil {
		return nil, err
	}
	out, summaryText := AppendFileOpsSummary(systemMsg, toSummarize, recent, summaryText)
	writeReactiveCompactionLog(a, compactionStart, msgs, out, summaryText)
	return out, nil
}

func splitReactiveInputs(msgs []llm.Message, keepRecentTurns int) (systemMsg llm.Message, toSummarize, recent []llm.Message, ok bool) {
	if len(msgs) == 0 {
		return llm.Message{}, nil, nil, false
	}
	systemMsg = msgs[0]
	restMsgs := msgs[1:]
	cutPoint := FindCutPoint(restMsgs, keepRecentTurns)
	if cutPoint >= len(restMsgs) {
		return systemMsg, nil, nil, false
	}
	return systemMsg, restMsgs[:cutPoint], restMsgs[cutPoint:], true
}

func writeReactiveCompactionLog(a *Agent, compactionStart time.Time, beforeMsgs, afterMsgs []llm.Message, summaryText string) {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	tokensBefore := estimateTotalTokens(beforeMsgs)
	tokensAfter := estimateTotalTokens(afterMsgs)
	a.writeCompactionLocked(Event{
		Category:        EventCompaction,
		Summary:         summaryText,
		TokensBefore:    tokensBefore,
		TokensAfter:     tokensAfter,
		FirstKeptSeq:    a.logSeq,
		CompactionModel: a.Model.ID,
	})
	durMS := time.Since(compactionStart).Milliseconds()
	a.writeCompactionV3Locked("reactive_threshold", "", string(StrategyReactive), tokensBefore, tokensAfter, a.logSeq, a.Model.ID, durMS)
	if a.logBuf != nil {
		_ = a.logBuf.Flush()
	}
}

func StreamSummary(ctx context.Context, core llm.Core, req *llm.ChatRequest) (string, error) {
	var summaryText strings.Builder
	var finishReason llm.FinishReason
	events, err := core.StreamChat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}
	for ev := range events {
		if ev.Err != nil {
			return "", fmt.Errorf("summarize stream: %w", ev.Err)
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
		return "", fmt.Errorf("summarize: generation hit the token cap and the summary is incomplete")
	}
	return summaryText.String(), nil
}
