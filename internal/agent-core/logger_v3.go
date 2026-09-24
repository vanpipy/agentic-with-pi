package agentcore

import (
	"log/slog"
	"time"
)

type turnStartEntry struct {
	Kind                string `json:"kind"`
	Version             int    `json:"version"`
	At                  string `json:"at"`
	Turn                int    `json:"turn"`
	UserMessageID       string `json:"user_message_id"`
	ContextWindow       int    `json:"context_window"`
	EstimateTokens      int    `json:"estimate_tokens"`
	ObservedInputTokens *int   `json:"observed_input_tokens,omitempty"`
}

type turnResponseEntry struct {
	Kind             string `json:"kind"`
	Version          int    `json:"version"`
	At               string `json:"at"`
	Turn             int    `json:"turn"`
	Model            string `json:"model"`
	Vendor           string `json:"vendor"`
	FinishReason     string `json:"finish_reason"`
	DurationMS       int64  `json:"duration_ms"`
	TTFTMS           *int64 `json:"ttft_ms,omitempty"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
}

type toolDedupHitEntry struct {
	Kind            string `json:"kind"`
	Version         int    `json:"version"`
	At              string `json:"at"`
	ToolName        string `json:"tool_name"`
	SignatureSHA256 string `json:"signature_sha256"`
	ReusedFromSeq   int    `json:"reused_from_seq"`
}

type compactionV3Entry struct {
	Kind          string `json:"kind"`
	Version       int    `json:"version"`
	At            string `json:"at"`
	Trigger       string `json:"trigger"`
	TriggerDetail string `json:"trigger_detail,omitempty"`
	Strategy      string `json:"strategy"`
	TokensBefore  int    `json:"tokens_before"`
	TokensAfter   int    `json:"tokens_after"`
	Model         string `json:"model"`
	FirstKeptSeq  int    `json:"first_kept_seq"`
	DurationMS    int64  `json:"duration_ms"`
}

type errorV3Entry struct {
	Kind       string `json:"kind"`
	Version    int    `json:"version"`
	At         string `json:"at"`
	Scope      string `json:"scope"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message"`
	Stack      string `json:"stack,omitempty"`
	RetryCount int    `json:"retry_count"`
	Recovered  bool   `json:"recovered"`
}

func (a *Agent) writeTurnStartLocked(userMsgID string) {
	if a.v3LogBuf == nil {
		return
	}
	ctxWindow := a.Model.MaxContextTokens
	if ctxWindow <= 0 {
		ctxWindow = DefaultContextWindow
	}
	var observed *int
	if a.observedInputTokens > 0 {
		v := a.observedInputTokens
		observed = &v
	}
	entry := turnStartEntry{
		Kind:                "turn_start",
		Version:             3,
		At:                  time.Now().UTC().Format(time.RFC3339Nano),
		Turn:                a.currentTurn,
		UserMessageID:       userMsgID,
		ContextWindow:       ctxWindow,
		EstimateTokens:      estimateTotalTokens(a.currentMsgs),
		ObservedInputTokens: observed,
	}
	if err := writeJSONLine(a.v3LogBuf, entry); err != nil {
		slog.Debug("agent: turn_start write failed", "err", err)
	}
}

func (a *Agent) writeTurnResponseLocked(model, vendor, finishReason string, durMS int64, ttftMS *int64, prompt, completion, total int) {
	if a.v3LogBuf == nil {
		return
	}
	entry := turnResponseEntry{
		Kind:             "turn_response",
		Version:          3,
		At:               time.Now().UTC().Format(time.RFC3339Nano),
		Turn:             a.currentTurn,
		Model:            model,
		Vendor:           vendor,
		FinishReason:     finishReason,
		DurationMS:       durMS,
		TTFTMS:           ttftMS,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      total,
	}
	if err := writeJSONLine(a.v3LogBuf, entry); err != nil {
		slog.Debug("agent: turn_response write failed", "err", err)
	}
}

func (a *Agent) writeToolDedupHitLocked(toolName, sig string, reusedFromSeq int) {
	if a.v3LogBuf == nil {
		return
	}
	entry := toolDedupHitEntry{
		Kind:            "tool_dedup_hit",
		Version:         3,
		At:              time.Now().UTC().Format(time.RFC3339Nano),
		ToolName:        toolName,
		SignatureSHA256: sig,
		ReusedFromSeq:   reusedFromSeq,
	}
	if err := writeJSONLine(a.v3LogBuf, entry); err != nil {
		slog.Debug("agent: tool_dedup_hit write failed", "err", err)
	}
}

func (a *Agent) writeCompactionV3Locked(trigger, detail, strategy string, tokensBefore, tokensAfter, firstKeptSeq int, model string, durMS int64) {
	if a.v3LogBuf == nil {
		return
	}
	entry := compactionV3Entry{
		Kind:          "compaction_v3",
		Version:       3,
		At:            time.Now().UTC().Format(time.RFC3339Nano),
		Trigger:       trigger,
		TriggerDetail: detail,
		Strategy:      strategy,
		TokensBefore:  tokensBefore,
		TokensAfter:   tokensAfter,
		Model:         model,
		FirstKeptSeq:  firstKeptSeq,
		DurationMS:    durMS,
	}
	if err := writeJSONLine(a.v3LogBuf, entry); err != nil {
		slog.Debug("agent: compaction_v3 write failed", "err", err)
	}
}

func (a *Agent) writeErrorV3Locked(scope, code, msg string, retry int, recoveredFlag bool) {
	if a.v3LogBuf == nil {
		return
	}
	entry := errorV3Entry{
		Kind:       "error",
		Version:    3,
		At:         time.Now().UTC().Format(time.RFC3339Nano),
		Scope:      scope,
		Code:       code,
		Message:    msg,
		RetryCount: retry,
		Recovered:  recoveredFlag,
	}
	if err := writeJSONLine(a.v3LogBuf, entry); err != nil {
		slog.Debug("agent: error_v3 write failed", "err", err)
	}
}
