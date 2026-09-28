package compact

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

type StrategyRunner interface {
	CompactRunner
	LLM() llm.Core
	KeepRecentTurns() int
	ModelID() string
	LogSeq() int
	WithLogLocked(do func())
	WriteCompactionEntry(summary string, before, after, firstKeptSeq int, model string)
	WriteCompactionV3Entry(trigger, detail, strategy string, before, after, firstKeptSeq int, model string, durMS int64)
	FlushLogBuf()
}

func (s CompactionStrategy) ActOn(ctx context.Context, runner StrategyRunner, msgs []llm.Message, observed *int) ([]llm.Message, CompactionAction, error) {
	switch s {
	case StrategyReactive:
		previousSummary := ExtractPreviousSummary(msgs)
		out, err := runReactiveCompaction(ctx, runner, msgs, previousSummary)
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

func runReactiveCompaction(ctx context.Context, runner StrategyRunner, msgs []llm.Message, previousSummary string) ([]llm.Message, error) {
	if len(msgs) == 0 {
		return msgs, nil
	}
	compactionStart := time.Now()
	systemMsg, toSummarize, recent, ok := splitReactiveInputs(msgs, runner.KeepRecentTurns())
	if !ok {
		slog.Debug("agent: nothing to compact")
		return msgs, nil
	}
	slog.Debug("agent: compacting", "messages_to_summarize", len(toSummarize), "messages_to_keep", len(recent))
	summaryText, err := StreamSummary(ctx, runner.LLM(), BuildSummaryRequest(toSummarize, previousSummary, runner.ModelID()))
	if err != nil {
		return nil, err
	}
	out, summaryText := AppendFileOpsSummary(systemMsg, toSummarize, recent, summaryText)
	writeReactiveCompactionLog(runner, compactionStart, msgs, out, summaryText)
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

func writeReactiveCompactionLog(runner StrategyRunner, compactionStart time.Time, beforeMsgs, afterMsgs []llm.Message, summaryText string) {
	tokensBefore := EstimateTotalTokens(beforeMsgs)
	tokensAfter := EstimateTotalTokens(afterMsgs)
	firstKeptSeq := runner.LogSeq()
	runner.WithLogLocked(func() {
		runner.WriteCompactionEntry(summaryText, tokensBefore, tokensAfter, firstKeptSeq, runner.ModelID())
		runner.WriteCompactionV3Entry("reactive_threshold", "", string(StrategyReactive), tokensBefore, tokensAfter, firstKeptSeq, runner.ModelID(), time.Since(compactionStart).Milliseconds())
	})
	runner.FlushLogBuf()
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
