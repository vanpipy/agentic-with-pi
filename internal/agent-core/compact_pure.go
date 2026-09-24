package agentcore

import (
	"encoding/json"
	"fmt"
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
