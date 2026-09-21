package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	case protocol.MethodListSessions:
		s.handleListSessions(conn, req)
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

func (s *Server) handleListSessions(conn io.Writer, req *protocol.Request) {
	summaries, err := s.collectSessionSummaries()
	if err != nil {
		if mErr := protocol.MarshalEvent(conn, req.ID, protocol.EventError, map[string]string{
			"error": "list sessions: " + err.Error(),
		}); mErr != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "list_sessions_err", "err", mErr)
		}
		return
	}
	if err := protocol.MarshalEvent(conn, req.ID, "sessions_list", protocol.ListSessionsResult{
		Sessions: summaries,
	}); err != nil {
		slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "list_sessions", "err", err)
	}
}

func (s *Server) collectSessionSummaries() ([]protocol.SessionSummary, error) {
	entries, err := os.ReadDir(s.sessionsDir)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.SessionSummary, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		sessionID := strings.TrimSuffix(name, ".jsonl")
		path := filepath.Join(s.sessionsDir, name)
		loaded, err := Load(path)
		if err != nil || loaded == nil {
			continue
		}
		out = append(out, protocol.SessionSummary{
			SessionID: sessionID,
			Model:     loaded.Meta.Model,
			StartedAt: loaded.Meta.StartedAt,
			Events:    len(loaded.Events),
		})
	}
	sortSummaries(out)
	return out, nil
}

func sortSummaries(s []protocol.SessionSummary) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].StartedAt > s[j-1].StartedAt; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
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

	var events <-chan agent.Event
	var snapshotCh <-chan []llm.Message
	var seed []llm.Message
	if history, ok := s.loadResumeHistory(sessionID); ok && len(history) > 0 {
		seed = history
	}
	if msgs, ok := s.sessionStates.snapshot(sessionID); ok {
		seed = append(seed, msgs...)
	}
	if len(seed) > 0 {
		events, snapshotCh = s.agent.RunStreamResumedWithSnapshot(runCtx, params.Prompt, seed)
	} else {
		events = s.agent.RunStream(runCtx, params.Prompt)
	}
	defer func() {
		if !s.sessionsHasHeader(store, sessionID) {
			_ = os.Remove(store.Path())
		}
	}()

	headerWritten := false
	cancelled := false
	for ev := range events {
		if !headerWritten {
			meta := SessionMeta{
				SessionID: sessionID,
				Model:     s.agent.Model.ID,
				MaxTurns:  s.agent.SafetyNet,
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

		if runCtx.Err() != nil {
			cancelled = true
			break
		}
	}

	if cancelled {
		if err := protocol.MarshalEvent(conn, req.ID, protocol.EventCancelAck, map[string]string{
			"reason": "user_cancelled",
		}); err != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "cancel_ack_stream", "err", err)
		}
		if writeErr := store.WriteEvent(protocol.EventCancelAck, map[string]string{"reason": "user_cancelled"}); writeErr != nil {
			slog.Debug("server: cancel session write failed", "err", writeErr)
		}
	}

	if snapshotCh != nil {
		select {
		case msgs, ok := <-snapshotCh:
			if ok && len(msgs) > 0 {
				s.sessionStates.update(sessionID, msgs)
			}
		default:
		}
	}
}

func (s *Server) loadResumeHistory(sessionID string) ([]llm.Message, bool) {
	path := DefaultPath(s.sessionsDir, sessionID)
	loaded, err := Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DEBUG loadResumeHistory no file path=%s err=%v\n", path, err)
		return nil, false
	}
	fmt.Fprintf(os.Stderr, "DEBUG loadResumeHistory loaded path=%s compactions=%d events=%d\n", path, len(loaded.Compactions), len(loaded.Events))
	if len(loaded.Compactions) == 0 {
		return nil, false
	}
	last := loaded.Compactions[len(loaded.Compactions)-1]
	return []llm.Message{
		{Role: "assistant", Content: "Previous conversation summary:\n" + last.Summary},
	}, true
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
