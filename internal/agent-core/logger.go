package agentcore

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/paths"
)

type sessionHeader struct {
	Kind      string   `json:"kind"`
	Version   int      `json:"version"`
	ID        string   `json:"id"`
	Model     string   `json:"model"`
	MaxTurns  int      `json:"max_turns"`
	System    string   `json:"system"`
	Tools     []string `json:"tools"`
	StartedAt string   `json:"started_at"`
}

type sessionCompaction struct {
	Kind         string `json:"kind"`
	At           string `json:"at"`
	Summary      string `json:"summary"`
	TokensBefore int    `json:"tokens_before,omitempty"`
	TokensAfter  int    `json:"tokens_after,omitempty"`
	FirstKeptSeq int    `json:"first_kept_seq,omitempty"`
	Model        string `json:"model,omitempty"`
}

type sessionEvent struct {
	Kind           string `json:"kind"`
	Seq            int    `json:"seq"`
	At             string `json:"at"`
	Category       string `json:"category"`
	Content        string `json:"content,omitempty"`
	Reasoning      string `json:"reasoning,omitempty"`
	ToolName       string `json:"tool_name,omitempty"`
	ToolArgs       string `json:"tool_args,omitempty"`
	UserMessage    string `json:"user_message,omitempty"`
	ToolCallsCount int    `json:"tool_calls_count,omitempty"`
	ToolError      string `json:"tool_error,omitempty"`
}

func (a *Agent) writeHeaderLocked() {
	system := a.SystemPrompts
	if len(system) > 200 {
		system = system[:200] + "..."
	}
	tools := make([]string, 0, len(a.toolList))
	for _, t := range a.toolList {
		tools = append(tools, t.Name())
	}
	header := sessionHeader{
		Kind:      "session",
		Version:   1,
		ID:        a.sessionID,
		Model:     a.Model.ID,
		MaxTurns:  a.SafetyNet,
		System:    system,
		Tools:     tools,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := writeJSONLine(a.logBuf, header); err != nil {
		slog.Debug("agent: header write failed", "err", err)
	}
}

func (a *Agent) writeEvent(seq int, ev Event) {
	if a.LogWriter == nil {
		return
	}
	a.logMu.Lock()
	defer a.logMu.Unlock()
	if a.logBuf == nil {
		return
	}
	entry := sessionEvent{Kind: "event", Seq: seq, At: time.Now().UTC().Format(time.RFC3339Nano), Category: categoryName(ev.Category)}
	switch ev.Category {
	case EventThoughtChunk:
		if ev.Reasoning != "" {
			entry.Reasoning = ev.Reasoning
		}
		if ev.Content != "" {
			entry.Content = ev.Content
		}
	case EventThoughtEnd:
		if ev.Reasoning != "" {
			entry.Reasoning = ev.Reasoning
		}
		if ev.Content != "" {
			entry.Content = ev.Content
		}
		if len(ev.ToolCalls) > 0 {
			entry.ToolCallsCount = len(ev.ToolCalls)
		}
	case EventObserve:
		if ev.ToolName != "" {
			entry.ToolName = ev.ToolName
		}
		if ev.ToolResult != "" {
			entry.Content = ev.ToolResult
		}
		if ev.ToolError != "" {
			entry.ToolError = ev.ToolError
		}
	case EventFinalAnswer:
		entry.Content = ev.Content
	case EventError:
		entry.ToolError = ev.ToolError
	case EventTool:
		entry.ToolName = ev.ToolName
		entry.ToolArgs = ev.ToolArgs
		entry.ToolCallsCount = len(ev.ToolCalls)
	case EventUserMessage:
		entry.UserMessage = ev.Content
	}
	if err := writeJSONLine(a.logBuf, entry); err != nil {
		slog.Debug("agent: event write failed", "err", err)
	}
}

func (a *Agent) writeCompactionLocked(ev Event) {
	if a.LogWriter == nil {
		return
	}
	if a.logBuf == nil {
		return
	}
	entry := sessionCompaction{
		Kind:         "compaction",
		At:           time.Now().UTC().Format(time.RFC3339Nano),
		Summary:      ev.Summary,
		TokensBefore: ev.TokensBefore,
		TokensAfter:  ev.TokensAfter,
		FirstKeptSeq: ev.FirstKeptSeq,
		Model:        ev.CompactionModel,
	}
	if err := writeJSONLine(a.logBuf, entry); err != nil {
		slog.Debug("agent: compaction write failed", "err", err)
	}
}

func writeJSONLine(w *bufio.Writer, v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	_, err = w.Write(line)
	return err
}

type alignedMessageEntry struct {
	Kind    string          `json:"kind"`
	Version int             `json:"version"`
	Entry   json.RawMessage `json:"entry"`
}

type alignedCustomEntry struct {
	Kind    string          `json:"kind"`
	Version int             `json:"version"`
	Entry   json.RawMessage `json:"entry"`
}

type alignedCustomMessageEntry struct {
	Kind    string          `json:"kind"`
	Version int             `json:"version"`
	Entry   json.RawMessage `json:"entry"`
}

func (a *Agent) WriteMessage(msg json_rpc.MessageEvent) error {
	if a.LogWriter == nil || a.logBuf == nil {
		return fmt.Errorf("log writer not initialized")
	}
	a.logMu.Lock()
	defer a.logMu.Unlock()
	if msg.ID == "" {
		msg.ID = json_rpc.NewV7()
	}
	if msg.Timestamp == "" {
		msg.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	entry := alignedMessageEntry{Kind: "message", Version: 2, Entry: raw}
	if err := writeJSONLine(a.logBuf, entry); err != nil {
		return err
	}
	a.currentParentID = msg.ID
	return nil
}

func (a *Agent) WriteCustom(parentID, customType string, data json.RawMessage) error {
	if a.LogWriter == nil || a.logBuf == nil {
		return fmt.Errorf("log writer not initialized")
	}
	a.logMu.Lock()
	defer a.logMu.Unlock()
	custom := json_rpc.CustomEvent{
		ID:         json_rpc.NewV7(),
		ParentID:   parentID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		CustomType: customType,
		Data:       data,
	}
	raw, err := json.Marshal(custom)
	if err != nil {
		return err
	}
	entry := alignedCustomEntry{Kind: "custom", Version: 2, Entry: raw}
	return writeJSONLine(a.logBuf, entry)
}

func (a *Agent) WriteCustomMessage(parentID, customType, content string, details json.RawMessage) error {
	if a.LogWriter == nil || a.logBuf == nil {
		return fmt.Errorf("log writer not initialized")
	}
	a.logMu.Lock()
	defer a.logMu.Unlock()
	custom := json_rpc.CustomMessageEvent{
		ID:         json_rpc.NewV7(),
		ParentID:   parentID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		CustomType: customType,
		Content:    content,
		Details:    details,
	}
	raw, err := json.Marshal(custom)
	if err != nil {
		return err
	}
	entry := alignedCustomMessageEntry{Kind: "custom_message", Version: 2, Entry: raw}
	return writeJSONLine(a.logBuf, entry)
}

func (a *Agent) writeAlignedEvent(ev Event) *StreamBuffer {
	a.logMu.Lock()
	parentID := a.currentParentID
	buf := a.currentStreamBuf
	a.logMu.Unlock()

	switch ev.Category {
	case EventUserMessage:
		userBuf := NewStreamBuffer(parentID)
		userBuf.SetRole("user")
		userBuf.AppendText(ev.Content)
		userBuf.SetStopReason("end_turn")
		userMsg := userBuf.Finalize()
		if err := a.WriteMessage(userMsg); err != nil {
			slog.Debug("agent: aligned write failed", "err", err)
			return nil
		}
		return nil

	case EventThoughtStart:
		newBuf := NewStreamBuffer(parentID)
		newBuf.SetRole("assistant")
		a.logMu.Lock()
		a.currentStreamBuf = newBuf
		a.logMu.Unlock()
		return newBuf

	case EventThoughtChunk:
		if buf == nil {
			return nil
		}
		buf.AppendThinking(ev.Reasoning, "")
		return buf

	case EventThoughtEnd:
		if buf == nil {
			return nil
		}
		if ev.Usage != nil {
			buf.SetUsage(json_rpc.UsageStats{
				PromptTokens:     ev.Usage.PromptTokens,
				CompletionTokens: ev.Usage.CompletionTokens,
				TotalTokens:      ev.Usage.TotalTokens,
			})
		}
		return buf

	case EventTool:
		if buf == nil {
			return nil
		}
		toolCallID := json_rpc.NewV7()
		intent := ev.ToolIntent
		if intent == "" {
			intent = extractToolIntent(ev.ToolArgs)
		}
		var args json.RawMessage
		if ev.ToolArgs != "" {
			args = json.RawMessage(ev.ToolArgs)
		} else {
			args = json.RawMessage("{}")
		}
		buf.AppendToolCall(toolCallID, ev.ToolName, intent, args)
		a.logMu.Lock()
		a.currentToolCallID = toolCallID
		a.logMu.Unlock()
		return buf

	case EventObserve:
		a.logMu.Lock()
		toolCallID := a.currentToolCallID
		a.logMu.Unlock()
		if buf != nil {
			buf.SetStopReason("toolUse")
			assistantMsg := buf.Finalize()
			if err := a.WriteMessage(assistantMsg); err != nil {
				slog.Debug("agent: aligned write failed", "err", err)
			} else {
				a.logMu.Lock()
				a.currentParentID = assistantMsg.ID
				a.logMu.Unlock()
			}
		}
		toolResultBuf := NewStreamBuffer(toolCallID)
		toolResultBuf.SetRole("toolResult")
		if ev.ToolError != "" {
			toolResultBuf.AppendText(ev.ToolError)
		} else if ev.ToolResult != "" {
			toolResultBuf.AppendText(ev.ToolResult)
		}
		toolResultBuf.SetDetails(json_rpc.MessageDetails{
			ToolName: ev.ToolName,
			Intent:   ev.ToolIntent,
			Error:    ev.ToolError,
		})
		toolResultBuf.SetStopReason("toolUse")
		toolResultMsg := toolResultBuf.Finalize()
		if err := a.WriteMessage(toolResultMsg); err != nil {
			slog.Debug("agent: aligned write failed", "err", err)
			return nil
		}
		a.logMu.Lock()
		a.currentParentID = toolResultMsg.ID
		nextBuf := NewStreamBuffer(toolResultMsg.ID)
		nextBuf.SetRole("assistant")
		a.currentStreamBuf = nextBuf
		a.logMu.Unlock()
		return a.currentStreamBuf

	case EventFinalAnswer:
		if buf == nil {
			return nil
		}
		buf.AppendText(ev.Content)
		if ev.Usage != nil {
			buf.SetUsage(json_rpc.UsageStats{
				PromptTokens:     ev.Usage.PromptTokens,
				CompletionTokens: ev.Usage.CompletionTokens,
				TotalTokens:      ev.Usage.TotalTokens,
			})
		}
		buf.SetStopReason("end_turn")
		msg := buf.Finalize()
		if err := a.WriteMessage(msg); err != nil {
			slog.Debug("agent: aligned write failed", "err", err)
			return nil
		}
		a.logMu.Lock()
		a.currentParentID = msg.ID
		a.currentStreamBuf = nil
		a.logMu.Unlock()
		return nil

	case EventError:
		var anchorID string
		if buf != nil {
			buf.AppendText(ev.ToolError)
			buf.SetStopReason("end_turn")
			msg := buf.Finalize()
			if err := a.WriteMessage(msg); err != nil {
				slog.Debug("agent: aligned write failed", "err", err)
				return nil
			}
			a.logMu.Lock()
			a.currentParentID = msg.ID
			a.currentStreamBuf = nil
			a.logMu.Unlock()
			anchorID = msg.ID
		} else {
			a.logMu.Lock()
			anchorID = a.currentParentID
			a.logMu.Unlock()
		}
		if err := a.WriteCustom(anchorID, "tool_error", json.RawMessage(fmt.Sprintf(`{"error":%q}`, ev.ToolError))); err != nil {
			slog.Debug("agent: aligned custom write failed", "err", err)
		}
		return nil
	}
	return nil
}

func (a *Agent) flushLog() {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	if a.logBuf == nil {
		return
	}
	if err := a.logBuf.Flush(); err != nil {
		slog.Debug("agent: log flush failed", "err", err)
	}
	a.logBuf = nil
	if f, ok := a.LogWriter.(*os.File); ok {
		f.Close()
		a.LogWriter = nil
	}
	if a.v3LogBuf != nil {
		if err := a.v3LogBuf.Flush(); err != nil {
			slog.Debug("agent: v3 log flush failed", "err", err)
		}
		a.v3LogBuf = nil
	}
	if f, ok := a.v3LogWriter.(*os.File); ok {
		f.Close()
		a.v3LogWriter = nil
	}
}

func categoryName(c EventCategory) string {
	switch c {
	case EventThoughtStart:
		return "thought_start"
	case EventThoughtChunk:
		return "thought_chunk"
	case EventThoughtEnd:
		return "thought_end"
	case EventTool:
		return "tool"
	case EventObserve:
		return "observe"
	case EventFinalAnswer:
		return "final_answer"
	case EventError:
		return "error"
	case EventInvalid:
		return "invalid"
	case EventUserMessage:
		return "user_message"
	case EventCompaction:
		return "compaction"
	}
	return "unknown"
}

func defaultSessionLogPath(sessionID string) (string, error) {
	if env := os.Getenv("AWP_NO_SESSION_LOG"); env != "" {
		return "", nil
	}
	path := os.Getenv("AWP_SESSION_LOG_PATH")
	if path != "" {
		return path, nil
	}
	home := paths.Home()
	dir := filepath.Join(home, "logs", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir sessions dir: %w", err)
	}
	if sessionID == "" {
		sessionID = "orphan-" + time.Now().UTC().Format("20060102T150405")
	}
	return filepath.Join(dir, sessionID+".jsonl"), nil
}
