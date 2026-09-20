package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/vanpiyp/awp/internal/llm"
)

const summarizationPrompt = `You are a context summarization assistant. Read the conversation below and produce a concise structured summary that preserves all critical information needed to continue the task: decisions made, files modified, tool results, errors, and the current state. Do NOT respond to any questions in the conversation. Output ONLY the structured summary.`

func EstimateTokens(s string) int {
	return (len(s) + 3) / 4
}

func estimateMessageTokens(m llm.Message) int {
	n := EstimateTokens(m.Content) + EstimateTokens(m.Reasoning) + EstimateTokens(m.ReasoningSig)
	for _, tc := range m.ToolCalls {
		n += EstimateTokens(tc.ID) + EstimateTokens(tc.Function.Name) + EstimateTokens(tc.Function.Arguments)
	}
	if n < 1 {
		return 1
	}
	return n
}

func estimateTotalTokens(msgs []llm.Message) int {
	total := 0
	for _, m := range msgs {
		total += estimateMessageTokens(m)
	}
	return total
}

func CountUserTurns(msgs []llm.Message) int {
	n := 0
	for _, m := range msgs {
		if m.Role == "user" {
			n++
		}
	}
	return n
}

func FindCutPoint(msgs []llm.Message, keepRecentTurns int) int {
	if keepRecentTurns <= 0 || len(msgs) == 0 {
		return 0
	}

	userCount := 0
	targetUsers := CountUserTurns(msgs) - keepRecentTurns

	if targetUsers <= 0 {
		return 0
	}

	for i, m := range msgs {
		if m.Role == "user" {
			if userCount == targetUsers {
				return i
			}
			userCount++
		}
	}
	return 0
}

func ShouldCompact(msgs []llm.Message, contextWindow int, settings CompactionSettings, turnsSinceLastCompact int) bool {
	if !settings.Enabled {
		return false
	}
	if turnsSinceLastCompact < settings.MinTurnsBetween {
		return false
	}
	if contextWindow > 0 {
		used := estimateTotalTokens(msgs)
		threshold := contextWindow * settings.FloorPercent / 100
		if used < threshold {
			return false
		}
		if used > contextWindow-settings.ReserveTokens {
			return true
		}
	}
	if settings.CompactEveryTurns > 0 {
		userTurns := CountUserTurns(msgs)
		if userTurns > settings.KeepRecentTurns &&
			(userTurns-settings.KeepRecentTurns)%settings.CompactEveryTurns == 0 {
			return true
		}
	}
	return false
}

func SerializeForSummary(msgs []llm.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case "user":
			fmt.Fprintf(&b, "[user] %s\n\n", m.Content)
		case "assistant":
			if m.Reasoning != "" {
				fmt.Fprintf(&b, "[assistant reasoning] %s\n", m.Reasoning)
			}
			if m.Content != "" {
				fmt.Fprintf(&b, "[assistant] %s\n", m.Content)
			}
			for _, tc := range m.ToolCalls {
				fmt.Fprintf(&b, "[assistant tool call] %s(%s)\n", tc.Function.Name, tc.Function.Arguments)
			}
		case "tool":
			fmt.Fprintf(&b, "[tool result %s] %s\n", m.ToolCallID, m.Content)
		case "system":
			continue
		}
	}
	return b.String()
}

func (a *Agent) compact(ctx context.Context, msgs []llm.Message) ([]llm.Message, error) {
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

	summaryReq := &llm.ChatRequest{
		Model: a.Model.ID,
		Messages: []llm.Message{
			{Role: "system", Content: summarizationPrompt},
			{Role: "user", Content: SerializeForSummary(toSummarize)},
		},
	}

	var summaryText strings.Builder
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
		}
	}

	summaryMsg := llm.Message{
		Role:    "assistant",
		Content: "Previous conversation summary:\n" + summaryText.String(),
	}

	out := make([]llm.Message, 0, 2+len(recent))
	out = append(out, systemMsg, summaryMsg)
	out = append(out, recent...)
	return out, nil
}
