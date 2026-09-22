package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/vanpiyp/awp/internal/storage"
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

func writeJSONLine(w *bufio.Writer, v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	_, err = w.Write(line)
	return err
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
	home := storage.Home()
	dir := filepath.Join(home, "logs", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir sessions dir: %w", err)
	}
	if sessionID == "" {
		sessionID = "orphan-" + time.Now().UTC().Format("20060102T150405")
	}
	return filepath.Join(dir, sessionID+".jsonl"), nil
}
