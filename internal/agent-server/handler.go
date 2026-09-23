package agentserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/paths"
)

func extractIntent(argsJSON string) string {
	var args struct {
		Intent string `json:"intent"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ""
	}
	return strings.TrimSpace(args.Intent)
}

func (s *Server) dispatch(conn io.Writer, connCtx context.Context, req *json_rpc.Request) {
	switch req.Method {
	case json_rpc.MethodPing:
		s.handlePing(conn, req)
	case json_rpc.MethodPrompt:
		s.handlePrompt(conn, connCtx, req)
	case json_rpc.MethodResume:
		s.handleResume(conn, req)
	case json_rpc.MethodCancel:
		s.handleCancel(conn, req)
	case json_rpc.MethodListSessions:
		s.handleListSessions(conn, req)
	default:
		if err := json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventError, map[string]string{
			"error": "unknown method: " + req.Method,
		}); err != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "default_error", "err", err)
		}
	}
}

func (s *Server) handlePing(conn io.Writer, req *json_rpc.Request) {
	if err := json_rpc.MarshalEvent(conn, req.ID, "pong", nil); err != nil {
		slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "ping_pong", "err", err)
	}
}

func (s *Server) handleListSessions(conn io.Writer, req *json_rpc.Request) {
	summaries, err := s.collectSessionSummaries()
	if err != nil {
		if mErr := json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventError, map[string]string{
			"error": "list sessions: " + err.Error(),
		}); mErr != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "list_sessions_err", "err", mErr)
		}
		return
	}
	if err := json_rpc.MarshalEvent(conn, req.ID, "sessions_list", json_rpc.ListSessionsResult{
		Sessions: summaries,
	}); err != nil {
		slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "list_sessions", "err", err)
	}
}

func (s *Server) collectSessionSummaries() ([]json_rpc.SessionSummary, error) {
	sessionsDir := paths.SessionsDir()
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, err
	}
	out := make([]json_rpc.SessionSummary, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		sessionID := strings.TrimSuffix(name, ".jsonl")
		path := filepath.Join(sessionsDir, name)
		loaded, err := Load(path)
		if err != nil || loaded == nil {
			continue
		}
		out = append(out, json_rpc.SessionSummary{
			SessionID: sessionID,
			Model:     loaded.Meta.Model,
			StartedAt: loaded.Meta.StartedAt,
			Events:    len(loaded.Events),
		})
	}
	sortSummaries(out)
	return out, nil
}

func sortSummaries(s []json_rpc.SessionSummary) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].StartedAt > s[j-1].StartedAt; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func (s *Server) handlePrompt(conn io.Writer, connCtx context.Context, req *json_rpc.Request) {
	var params json_rpc.PromptParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		if mErr := json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventError, map[string]string{
			"error": "invalid params",
		}); mErr != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "invalid_params", "err", mErr)
		}
		return
	}

	sessionID := params.SessionID
	if sessionID == "" {
		sessionID = NewID()
	}

	s.agent.WithSessionID(sessionID)
	if err := json_rpc.MarshalEvent(conn, req.ID, "session_started", map[string]string{
		"session_id": sessionID,
	}); err != nil {
		slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "session_started", "session_id", sessionID, "err", err)
		return
	}

	runCtx, runCancel := context.WithCancel(connCtx)
	s.registerConnCancel(req.ID, runCancel)
	defer s.popConnCancel(req.ID)
	defer runCancel()

	var events <-chan agentcore.Event
	var snapshotCh <-chan []llm.Message
	var seed []llm.Message
	if history, ok := s.loadResumeHistory(sessionID); ok && len(history) > 0 {
		seed = history
	}
	if msgs, ok := s.sessionStates.snapshot(sessionID); ok {
		seed = append(seed, msgs...)
	}
	s.agent.LogEventForTest(agentcore.Event{Category: agentcore.EventUserMessage, Content: params.Prompt})
	events, snapshotCh = s.agent.RunStreamResumedWithSnapshot(runCtx, params.Prompt, seed)

	var parentID string
	var streamBuf *agentcore.StreamBuffer
	cancelled := false
	for ev := range events {
		emits := marshalAgentEventForWire(ev, &parentID, &streamBuf)
		for _, em := range emits {
			if err := json_rpc.MarshalEvent(conn, req.ID, em.eventName, em.payload); err != nil {
				slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "prompt_stream", "session_id", sessionID, "event", em.eventName, "err", err)
				return
			}
		}
		if runCtx.Err() != nil {
			cancelled = true
			break
		}
	}

	if cancelled {
		if err := json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventCancelAck, map[string]string{
			"reason": "user_cancelled",
		}); err != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "cancel_ack_stream", "err", err)
		}
	}

	if snapshotCh != nil {
		select {
		case msgs, ok := <-snapshotCh:
			if ok && len(msgs) > 0 {
				s.sessionStates.update(sessionID, msgs)
			}
		}
	}
}

func (s *Server) loadResumeHistory(sessionID string) ([]llm.Message, bool) {
	path := DefaultPath(paths.SessionsDir(), sessionID)
	loaded, err := Load(path)
	if err != nil {
		return nil, false
	}
	if len(loaded.Compactions) == 0 {
		return nil, false
	}
	last := loaded.Compactions[len(loaded.Compactions)-1]
	return []llm.Message{
		{Role: "assistant", Content: "Previous conversation summary:\n" + last.Summary},
	}, true
}

func (s *Server) handleResume(conn io.Writer, req *json_rpc.Request) {
	var params json_rpc.ResumeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		if mErr := json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventError, map[string]string{
			"error": "invalid params",
		}); mErr != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "resume_invalid_params", "err", mErr)
		}
		return
	}

	entries, err := LoadEntries(DefaultPath(paths.SessionsDir(), params.SessionID))
	if err != nil {
		if mErr := json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventError, map[string]string{
			"error": "session not found: " + params.SessionID,
		}); mErr != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "resume_not_found", "err", mErr)
		}
		return
	}

	replayed := 0
	for _, e := range entries {
		switch e.Kind {
		case "session", "compaction":
			continue
		case "event":
			legacy, ok := e.Parsed.(LegacyEvent)
			if !ok {
				continue
			}
			eventName := legacyWireName(legacy.Category)
			if eventName == "" {
				eventName = legacy.Category
			}
			payload := marshalLegacyPayload(legacy)
			if err := json_rpc.MarshalEvent(conn, req.ID, eventName, payload); err != nil {
				slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "resume_stream", "session_id", params.SessionID, "err", err)
				return
			}
			replayed++
		case "message":
			msg, ok := e.Parsed.(json_rpc.MessageEvent)
			if !ok {
				continue
			}
			if err := json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventMessage, msg); err != nil {
				slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "resume_stream", "session_id", params.SessionID, "err", err)
				return
			}
			replayed++
		case "custom":
			evt, ok := e.Parsed.(json_rpc.CustomEvent)
			if !ok {
				continue
			}
			if err := json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventCustom, evt); err != nil {
				slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "resume_stream", "session_id", params.SessionID, "err", err)
				return
			}
			replayed++
		case "custom_message":
			cm, ok := e.Parsed.(json_rpc.CustomMessageEvent)
			if !ok {
				continue
			}
			if err := json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventCustomMessage, cm); err != nil {
				slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "resume_stream", "session_id", params.SessionID, "err", err)
				return
			}
			replayed++
		}
	}

	if err := json_rpc.MarshalEvent(conn, req.ID, "session_resumed", map[string]int{
		"event_count":    replayed,
		"schema_version": 2,
	}); err != nil {
		slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "resume_done", "session_id", params.SessionID, "err", err)
	}
}

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

func (s *Server) handleCancel(conn io.Writer, req *json_rpc.Request) {
	cancel := s.popConnCancel(req.ID)
	if cancel == nil {
		if err := json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventError, map[string]string{
			"error": "no active prompt to cancel for id " + req.ID,
		}); err != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "cancel_no_active", "err", err)
		}
		return
	}
	cancel()
	if err := json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventCancelAck, nil); err != nil {
		slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "cancel_ack", "err", err)
	}
}

type wireEmit struct {
	eventName string
	payload   any
}

func marshalAgentEventForWire(ev agentcore.Event, parentID *string, streamBuf **agentcore.StreamBuffer) []wireEmit {
	switch ev.Category {
	case agentcore.EventUserMessage:
		buf := agentcore.NewStreamBuffer(*parentID)
		buf.SetRole("user")
		buf.AppendText(ev.Content)
		buf.SetStopReason("end_turn")
		msg := buf.Finalize()
		*parentID = msg.ID
		return []wireEmit{{eventName: json_rpc.EventMessage, payload: msg}}

	case agentcore.EventThoughtStart:
		*streamBuf = agentcore.NewStreamBuffer(*parentID)
		return nil

	case agentcore.EventThoughtChunk:
		if *streamBuf != nil {
			(*streamBuf).AppendThinking(ev.Reasoning, "")
		}
		return nil

	case agentcore.EventThoughtEnd:
		if *streamBuf != nil && ev.Usage != nil {
			(*streamBuf).SetUsage(json_rpc.UsageStats{
				PromptTokens:     ev.Usage.PromptTokens,
				CompletionTokens: ev.Usage.CompletionTokens,
				TotalTokens:      ev.Usage.TotalTokens,
			})
		}
		return nil

	case agentcore.EventTool:
		if *streamBuf == nil {
			return nil
		}
		toolCallID := json_rpc.NewV7()
		intent := ev.ToolIntent
		if intent == "" {
			intent = extractIntent(ev.ToolArgs)
		}
		args := json.RawMessage(ev.ToolArgs)
		if len(args) == 0 {
			args = json.RawMessage("{}")
		}
		(*streamBuf).SetRole("assistant")
		(*streamBuf).AppendToolCall(toolCallID, ev.ToolName, intent, args)
		(*streamBuf).SetStopReason("toolUse")
		assistantMsg := (*streamBuf).Finalize()
		*parentID = assistantMsg.ID
		emits := []wireEmit{{eventName: json_rpc.EventMessage, payload: assistantMsg}}
		toolResultBuf := agentcore.NewStreamBuffer(toolCallID)
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
		*parentID = toolResultMsg.ID
		emits = append(emits, wireEmit{eventName: json_rpc.EventMessage, payload: toolResultMsg})
		*streamBuf = agentcore.NewStreamBuffer(toolResultMsg.ID)
		return emits

	case agentcore.EventObserve:
		if *streamBuf == nil {
			return nil
		}
		(*streamBuf).SetStopReason("toolUse")
		msg := (*streamBuf).Finalize()
		*parentID = msg.ID
		*streamBuf = agentcore.NewStreamBuffer(msg.ID)
		return []wireEmit{{eventName: json_rpc.EventMessage, payload: msg}}

	case agentcore.EventFinalAnswer:
		if *streamBuf == nil {
			return nil
		}
		(*streamBuf).SetRole("assistant")
		(*streamBuf).AppendText(ev.Content)
		if ev.Usage != nil {
			(*streamBuf).SetUsage(json_rpc.UsageStats{
				PromptTokens:     ev.Usage.PromptTokens,
				CompletionTokens: ev.Usage.CompletionTokens,
				TotalTokens:      ev.Usage.TotalTokens,
			})
		}
		(*streamBuf).SetStopReason("end_turn")
		msg := (*streamBuf).Finalize()
		*parentID = msg.ID
		*streamBuf = nil
		return []wireEmit{{eventName: json_rpc.EventMessage, payload: msg}}

	case agentcore.EventError:
		if *streamBuf == nil {
			return nil
		}
		(*streamBuf).AppendText(ev.ToolError)
		(*streamBuf).SetStopReason("end_turn")
		msg := (*streamBuf).Finalize()
		*parentID = msg.ID
		*streamBuf = nil
		customEv := json_rpc.CustomEvent{
			ID:         json_rpc.NewV7(),
			ParentID:   *parentID,
			Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
			CustomType: "tool_error",
			Data:       json.RawMessage(fmt.Sprintf(`{"error":%q}`, ev.ToolError)),
		}
		return []wireEmit{
			{eventName: json_rpc.EventMessage, payload: msg},
			{eventName: json_rpc.EventCustom, payload: customEv},
		}
	}
	return nil
}

func usageToMap(u *llm.Usage) map[string]string {
	return nil
}
