package tui

import (
	"encoding/json"
	"strconv"

	"github.com/vanpiyp/awp/internal/client-sdk"
)

type serverEventMsg struct {
	Kind string
	Data []byte
}

func parseUsage(data []byte) *msgUsage {
	var d struct {
		PromptTokens     int    `json:"prompt_tokens"`
		CompletionTokens int    `json:"completion_tokens"`
		TotalTokens      int    `json:"total_tokens"`
		PromptTokensStr   string `json:"-"`
		CompletionTokensStr string `json:"-"`
		TotalTokensStr    string `json:"-"`
	}
	_ = json.Unmarshal(data, &d)
	if d.TotalTokens == 0 {
		// try strings fallback
		var s struct {
			PromptTokens     string `json:"prompt_tokens"`
			CompletionTokens string `json:"completion_tokens"`
			TotalTokens      string `json:"total_tokens"`
		}
		_ = json.Unmarshal(data, &s)
		d.PromptTokens, _ = strconv.Atoi(s.PromptTokens)
		d.CompletionTokens, _ = strconv.Atoi(s.CompletionTokens)
		d.TotalTokens, _ = strconv.Atoi(s.TotalTokens)
	}
	if d.TotalTokens == 0 {
		return nil
	}
	return &msgUsage{
		prompt:     d.PromptTokens,
		completion: d.CompletionTokens,
		total:      d.TotalTokens,
	}
}

func handleServerEvent(c *chatModel, sessionID *string, ev client_sdk.Event) {
	switch ev.Kind {
	case "session_started":
		var d struct {
			SessionID string `json:"session_id"`
		}
		json.Unmarshal(ev.Data, &d)
		*sessionID = d.SessionID
	case "thought_chunk":
		var d struct {
			Reasoning string `json:"reasoning"`
			Content   string `json:"content"`
		}
		json.Unmarshal(ev.Data, &d)
		if d.Reasoning != "" {
			c.appendReasoning(d.Reasoning)
		}
		if d.Content != "" {
			c.appendStream(d.Content)
		}
	case "thought_end":
		c.commitStream()
	case "tool":
		var d struct {
			Name   string `json:"name"`
			Args   string `json:"args"`
			Result string `json:"result"`
		}
		json.Unmarshal(ev.Data, &d)
		c.appendTool(d.Name, d.Args, d.Result)
	case "observe":
		var d struct {
			ToolName string `json:"tool_name"`
			Result   string `json:"result"`
			Error    string `json:"error"`
		}
		json.Unmarshal(ev.Data, &d)
		if d.Error != "" {
			c.appendError(d.Error)
		} else {
			c.appendObserve(d.Result)
		}
	case "final_answer":
		c.commitStream()
		usage := parseUsage(ev.Data)
		if usage != nil {
			if len(c.messages) > 0 {
				c.messages[len(c.messages)-1].usage = usage
			}
		}
	case "error":
		var d struct {
			Error string `json:"error"`
		}
		json.Unmarshal(ev.Data, &d)
		c.appendError(d.Error)
	}
}
