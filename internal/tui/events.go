package tui

import (
	"encoding/json"

	agentclient "github.com/vanpiyp/awp/internal/agent-client"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

type serverEventMsg struct {
	Kind string
	Data []byte
}

func handleServerEvent(c *chatModel, sessionID *string, ev agentclient.Event) {
	switch ev.Kind {
	case "session_started":
		var d struct {
			SessionID string `json:"session_id"`
		}
		json.Unmarshal(ev.Data, &d)
		*sessionID = d.SessionID
	case json_rpc.EventMessage:
		handleMessageEvent(c, ev.Data)
	case json_rpc.EventCustom:
		handleCustomEvent(c, ev.Data)
	case json_rpc.EventCustomMessage:
	case "cancelled":
		c.commitStream()
		var d struct {
			Reason string `json:"reason"`
		}
		json.Unmarshal(ev.Data, &d)
		c.appendSystem(systemPrefix.Render(" cancelled") + durationHint.Render(" ("+d.Reason+")"))
	}
}

func handleMessageEvent(c *chatModel, data []byte) {
	var msg json_rpc.MessageEvent
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	for _, part := range msg.Message.Content {
		switch part.Type {
		case "thinking":
			if part.Text != "" {
				c.appendReasoning(part.Text)
			}
		case "text":
			if part.Text != "" {
				c.appendStream(part.Text)
			}
		case "toolCall":
			c.appendTool(part.Name, string(part.Arguments), part.Intent)
		}
	}
	if msg.Details != nil {
		if msg.Details.Error != "" {
			c.appendError(msg.Details.Error)
		} else if msg.Message.Role == "toolResult" {
			text := ""
			if len(msg.Message.Content) > 0 {
				text = msg.Message.Content[0].Text
			}
			c.appendObserve(text, msg.Details.Intent)
		}
	}
	if msg.StopReason == "end_turn" || (msg.Message.Role == "assistant" && msg.StopReason == "") {
		if msg.Message.Role == "assistant" {
			c.commitStream()
		}
		if msg.Usage != nil && len(c.messages) > 0 {
			c.messages[len(c.messages)-1].usage = &msgUsage{
				prompt:     msg.Usage.PromptTokens,
				completion: msg.Usage.CompletionTokens,
				total:      msg.Usage.TotalTokens,
			}
		}
	}
}

func handleCustomEvent(c *chatModel, data []byte) {
	var ev json_rpc.CustomEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return
	}
	switch ev.CustomType {
	case "tool_error":
		var d struct {
			Error string `json:"error"`
		}
		json.Unmarshal(ev.Data, &d)
		if d.Error != "" {
			c.appendError(d.Error)
		}
	case "abort":
		var d struct {
			Reason string `json:"reason"`
		}
		json.Unmarshal(ev.Data, &d)
		c.appendSystem(systemPrefix.Render(" aborted") + durationHint.Render(" ("+d.Reason+")"))
	}
}