package util

import "github.com/vanpiyp/awp/internal/llm"

func RepairMissingToolOutputs(msgs []llm.Message) ([]llm.Message, int) {
	if len(msgs) == 0 {
		return msgs, 0
	}
	out := make([]llm.Message, 0, len(msgs)+4)
	repaired := 0
	i := 0
	for i < len(msgs) {
		m := msgs[i]
		out = append(out, m)
		i++
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			continue
		}
		blockEnd := i
		for j := i; j < len(msgs); j++ {
			if msgs[j].Role != "tool" {
				break
			}
			out = append(out, msgs[j])
			blockEnd = j + 1
		}
		seen := make(map[string]bool)
		for j := i; j < blockEnd; j++ {
			if msgs[j].ToolCallID != "" {
				seen[msgs[j].ToolCallID] = true
			}
		}
		for _, tc := range m.ToolCalls {
			if seen[tc.ID] {
				continue
			}
			out = append(out, interruptedToolMessage(tc.Function.Name, tc.ID))
			repaired++
		}
		i = blockEnd
	}
	return out, repaired
}

func interruptedToolMessage(name, toolCallID string) llm.Message {
	return llm.Message{
		Role:       "tool",
		ToolCallID: toolCallID,
		Content:    "[Tool '" + name + "' interrupted by server reload before its result was recorded. Resume by rerunning the same tool call.]",
	}
}
