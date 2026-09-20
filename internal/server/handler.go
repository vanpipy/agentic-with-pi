package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/vanpiyp/awp/internal/agent"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/protocol"
)

func (s *Server) dispatch(conn io.Writer, connCtx context.Context, req *protocol.Request) {
	switch req.Method {
	case protocol.MethodPing:
		s.handlePing(conn, req)
	case protocol.MethodPrompt:
		s.handlePrompt(conn, connCtx, req)
	case protocol.MethodResume:
		s.handleResume(conn, req)
	case protocol.MethodCancel:
		s.handleCancel(conn, req)
	default:
		if err := protocol.MarshalEvent(conn, req.ID, protocol.EventError, map[string]string{
			"error": "unknown method: " + req.Method,
		}); err != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "default_error", "err", err)
		}
	}
}

func (s *Server) handlePing(conn io.Writer, req *protocol.Request) {
	if err := protocol.MarshalEvent(conn, req.ID, "pong", nil); err != nil {
		slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "ping_pong", "err", err)
	}
}

func (s *Server) handlePrompt(conn io.Writer, connCtx context.Context, req *protocol.Request) {
	var params protocol.PromptParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		if mErr := protocol.MarshalEvent(conn, req.ID, protocol.EventError, map[string]string{
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

	store := s.getOrCreateStore(sessionID)
	if err := protocol.MarshalEvent(conn, req.ID, "session_started", map[string]string{
		"session_id": sessionID,
	}); err != nil {
		slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "session_started", "session_id", sessionID, "err", err)
		return
	}

	runCtx, runCancel := context.WithCancel(connCtx)
	s.registerConnCancel(req.ID, runCancel)
	defer s.popConnCancel(req.ID)
	defer runCancel()

	events := s.agent.RunStream(runCtx, params.Prompt)
	defer func() {
		if !s.sessionsHasHeader(store, sessionID) {
			_ = os.Remove(store.Path())
		}
	}()

	headerWritten := false
	for ev := range events {
		if !headerWritten {
			meta := SessionMeta{
				SessionID: sessionID,
				Model:     s.agent.Model.ID,
				MaxTurns:  s.agent.MaxTurns,
				System:    s.agent.SystemPrompts,
				StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
			}
			if err := store.WriteHeader(meta); err != nil {
				slog.Debug("server: header write failed", "err", err)
			}
			headerWritten = true
		}

		eventName, data := mapAgentEvent(ev)

		if err := protocol.MarshalEvent(conn, req.ID, eventName, data); err != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "prompt_stream", "session_id", sessionID, "event", eventName, "err", err)
			return
		}

		if writeErr := store.WriteEvent(eventName, data); writeErr != nil {
			slog.Debug("server: session write failed", "err", writeErr)
		}
	}
}

func (s *Server) handleResume(conn io.Writer, req *protocol.Request) {
	var params protocol.ResumeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		if mErr := protocol.MarshalEvent(conn, req.ID, protocol.EventError, map[string]string{
			"error": "invalid params",
		}); mErr != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "resume_invalid_params", "err", mErr)
		}
		return
	}

	loaded, err := Load(DefaultPath(s.sessionsDir, params.SessionID))
	if err != nil {
		if mErr := protocol.MarshalEvent(conn, req.ID, protocol.EventError, map[string]string{
			"error": "session not found: " + params.SessionID,
		}); mErr != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "resume_not_found", "err", mErr)
		}
		return
	}

	for _, ev := range loaded.Events {
		if err := protocol.MarshalEvent(conn, req.ID, ev.Kind, json.RawMessage(ev.Data)); err != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "resume_stream", "session_id", params.SessionID, "err", err)
			return
		}
	}

	if err := protocol.MarshalEvent(conn, req.ID, "session_resumed", map[string]int{
		"event_count": len(loaded.Events),
	}); err != nil {
		slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "resume_done", "session_id", params.SessionID, "err", err)
	}
}

func (s *Server) handleCancel(conn io.Writer, req *protocol.Request) {
	cancel := s.popConnCancel(req.ID)
	if cancel == nil {
		if err := protocol.MarshalEvent(conn, req.ID, protocol.EventError, map[string]string{
			"error": "no active prompt to cancel for id " + req.ID,
		}); err != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "cancel_no_active", "err", err)
		}
		return
	}
	cancel()
	if err := protocol.MarshalEvent(conn, req.ID, protocol.EventCancelAck, nil); err != nil {
		slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "cancel_ack", "err", err)
	}
}

func (s *Server) sessionsHasHeader(store *Store, sessionID string) bool {
	loaded, err := Load(store.Path())
	if err != nil {
		return false
	}
	return loaded.Meta.SessionID == sessionID
}

func mapAgentEvent(ev agent.Event) (string, any) {
	switch ev.Category {
	case agent.EventThoughtStart:
		return protocol.EventThoughtStart, nil
	case agent.EventThoughtChunk:
		return protocol.EventThoughtChunk, map[string]string{
			"reasoning": ev.Reasoning,
			"content":   ev.Content,
		}
	case agent.EventThoughtEnd:
		data := map[string]string{
			"reasoning": ev.Reasoning,
			"content":   ev.Content,
		}
		if u := usageToMap(ev.Usage); u != nil {
			for k, v := range u {
				data[k] = v
			}
		}
		return protocol.EventThoughtEnd, data
	case agent.EventTool:
		return protocol.EventTool, map[string]string{
			"name": ev.ToolName,
			"args": ev.ToolArgs,
		}
	case agent.EventObserve:
		return protocol.EventObserve, map[string]string{
			"tool_name": ev.ToolName,
			"result":    ev.ToolResult,
			"error":     ev.ToolError,
		}
	case agent.EventFinalAnswer:
		data := map[string]string{"content": ev.Content}
		if u := usageToMap(ev.Usage); u != nil {
			for k, v := range u {
				data[k] = v
			}
		}
		return protocol.EventFinalAnswer, data
	case agent.EventError:
		return protocol.EventError, map[string]string{
			"error": ev.ToolError,
		}
	default:
		return "unknown", nil
	}
}

func usageToMap(u *llm.Usage) map[string]string {
	if u == nil {
		return nil
	}
	return map[string]string{
		"prompt_tokens":     strconv.Itoa(u.PromptTokens),
		"completion_tokens": strconv.Itoa(u.CompletionTokens),
		"total_tokens":      strconv.Itoa(u.TotalTokens),
	}
}
