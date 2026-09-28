package agentserver

import "github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"

func legacyWireName(category string) string {
	switch category {
	case "thought_start":
		return json_rpc.EventThoughtStart
	case "thought_chunk":
		return json_rpc.EventThoughtChunk
	case "thought_end":
		return json_rpc.EventThoughtEnd
	case "tool":
		return json_rpc.EventTool
	case "observe":
		return json_rpc.EventObserve
	case "final_answer":
		return json_rpc.EventFinalAnswer
	case "error":
		return json_rpc.EventError
	}
	return ""
}

func marshalLegacyPayload(legacy LegacyEvent) any {
	switch legacy.Category {
	case "thought_chunk":
		return map[string]string{
			"reasoning": legacy.Reasoning,
			"content":   legacy.Content,
		}
	case "thought_start":
		return nil
	case "thought_end":
		return map[string]string{
			"reasoning": legacy.Reasoning,
			"content":   legacy.Content,
		}
	case "tool":
		payload := map[string]string{
			"name": legacy.ToolName,
			"args": legacy.ToolArgs,
		}
		if intent := extractIntent(legacy.ToolArgs); intent != "" {
			payload["intent"] = intent
		}
		return payload
	case "observe":
		observe := map[string]string{
			"tool_name": legacy.ToolName,
			"result":    legacy.ToolResult,
			"error":     legacy.ToolError,
		}
		if legacy.ToolName != "" {
			if intent := extractIntent(legacy.ToolArgs); intent != "" {
				observe["intent"] = intent
			}
		}
		return observe
	case "final_answer":
		return map[string]string{"content": legacy.Content}
	case "error":
		return map[string]string{"error": legacy.ToolError}
	}
	return nil
}
