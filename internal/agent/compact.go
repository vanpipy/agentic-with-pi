package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/vanpiyp/awp/internal/llm"
)

const SummarizationPrompt = `The messages above are a conversation to summarize. Create a structured context checkpoint summary that another LLM will use to continue the work.

Use this EXACT format:

## Goal
[What is the user trying to accomplish? Can be multiple items if the session covers different tasks.]

## Constraints & Preferences
- [Any constraints, preferences, or requirements mentioned by user]
- [Or "(none)" if none were mentioned]

## Progress
### Done
- [x] [Completed tasks/changes]

### In Progress
- [ ] [Current work]

### Blocked
- [Issues preventing progress, if any]

## Key Decisions
- **[Decision]**: [Brief rationale]

## Next Steps
1. [Ordered list of what should happen next]

## Critical Context
- [Any data, examples, or references needed to continue]
- [Or "(none)" if not applicable]

Keep each section concise. Preserve exact file paths, function names, and error messages. Do NOT continue the conversation. Do NOT respond to any questions in the conversation. Output ONLY the structured summary.`

const UpdateSummarizationPrompt = `Update the existing structured summary with new information. RULES:
- PRESERVE all existing information from the previous summary
- ADD new progress, decisions, and context from the new messages
- UPDATE the Progress section: move items from "In Progress" to "Done" when completed
- UPDATE "Next Steps" based on what was accomplished
- PRESERVE exact file paths, function names, and error messages
- If something is no longer relevant, you may remove it

Use this EXACT format:

## Goal
[Preserve existing goals, add new ones if the task expanded]

## Constraints & Preferences
- [Preserve existing, add new ones discovered]

## Progress
### Done
- [x] [Include previously done items AND newly completed items]

### In Progress
- [ ] [Current work - update based on progress]

### Blocked
- [Current blockers - remove if resolved]

## Key Decisions
- **[Decision]**: [Brief rationale] (preserve all previous, add new)

## Next Steps
1. [Update based on current state]

## Critical Context
- [Preserve important context, add new if needed]

Keep each section concise. Preserve exact file paths, function names, and error messages.`

type FileOperations struct {
	Read    map[string]bool
	Written map[string]bool
	Edited  map[string]bool
}

func ExtractFileOps(msgs []llm.Message) FileOperations {
	ops := FileOperations{
		Read:    map[string]bool{},
		Written: map[string]bool{},
		Edited:  map[string]bool{},
	}
	for _, m := range msgs {
		if m.Role != "assistant" {
			continue
		}
		for _, tc := range m.ToolCalls {
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil || args.Path == "" {
				continue
			}
			switch tc.Function.Name {
			case "read":
				ops.Read[args.Path] = true
			case "write":
				ops.Written[args.Path] = true
			case "edit":
				ops.Edited[args.Path] = true
			}
		}
	}
	return ops
}

func ComputeFileLists(ops FileOperations) (readOnly []string, modified []string) {
	modifiedSet := map[string]bool{}
	for p := range ops.Edited {
		modifiedSet[p] = true
	}
	for p := range ops.Written {
		modifiedSet[p] = true
	}
	for p := range ops.Read {
		if !modifiedSet[p] {
			readOnly = append(readOnly, p)
		}
	}
	for p := range modifiedSet {
		modified = append(modified, p)
	}
	sort.Strings(readOnly)
	sort.Strings(modified)
	return
}

func FormatFileOperations(readFiles, modifiedFiles []string) string {
	var sections []string
	if len(readFiles) > 0 {
		sections = append(sections, "<read-files>\n"+strings.Join(readFiles, "\n")+"\n</read-files>")
	}
	if len(modifiedFiles) > 0 {
		sections = append(sections, "<modified-files>\n"+strings.Join(modifiedFiles, "\n")+"\n</modified-files>")
	}
	if len(sections) == 0 {
		return ""
	}
	return "\n\n" + strings.Join(sections, "\n\n")
}

func ExtractPreviousSummary(msgs []llm.Message) string {
	const prefix = "Previous conversation summary:\n"
	for _, m := range msgs {
		if m.Role == "assistant" && strings.HasPrefix(m.Content, prefix) {
			return strings.TrimPrefix(m.Content, prefix)
		}
	}
	return ""
}

func EstimateTokens(s string) int {
	return (len(s) + 3) / 4
}

const toolResultMaxChars = 2000

func TruncateForSummary(text string, maxChars int) string {
	if text == "" || len(text) <= maxChars {
		return text
	}
	truncated := len(text) - maxChars
	return text[:maxChars] + "\n\n[... " + strconv.Itoa(truncated) + " more characters truncated]"
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

func ShouldCompact(msgs []llm.Message, contextWindow int, settings CompactionSettings) bool {
	if !settings.Enabled {
		return false
	}
	if contextWindow <= 0 {
		return false
	}
	used := estimateTotalTokens(msgs)
	return used > contextWindow-settings.ReserveTokens
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
			fmt.Fprintf(&b, "[tool result %s] %s\n", m.ToolCallID, TruncateForSummary(m.Content, toolResultMaxChars))
		case "system":
			continue
		}
	}
	return b.String()
}

func (a *Agent) compact(ctx context.Context, msgs []llm.Message, previousSummary string) ([]llm.Message, error) {
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
	return out, nil
}
