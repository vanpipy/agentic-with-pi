package agentcore

import (
	"bufio"
	"bytes"
	"encoding/json"
	"sync"
	"testing"
)

func newV3TestAgent() (*Agent, *bytes.Buffer) {
	var buf bytes.Buffer
	a := &Agent{
		LogWriter:     &buf,
		logMu:         sync.Mutex{},
		logBuf:        bufio.NewWriterSize(&buf, 4096),
		logFileOpened: true,
	}
	return a, &buf
}

func parseV3Line(t *testing.T, line []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(line, &m); err != nil {
		t.Fatalf("unmarshal: %v\nline: %s", err, line)
	}
	return m
}

func TestRoundtripTurnStartEntry(t *testing.T) {
	obs := 24512
	entry := turnStartEntry{
		Kind:                "turn_start",
		Version:             3,
		At:                  "2026-09-24T03:00:00Z",
		Turn:                3,
		UserMessageID:       "msg-abc",
		ContextWindow:       128000,
		EstimateTokens:      24000,
		ObservedInputTokens: &obs,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back turnStartEntry
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Kind != "turn_start" || back.Version != 3 || back.Turn != 3 || back.UserMessageID != "msg-abc" {
		t.Errorf("scalar mismatch: %+v", back)
	}
	if back.ContextWindow != 128000 || back.EstimateTokens != 24000 {
		t.Errorf("token counts mismatch: %+v", back)
	}
	if back.ObservedInputTokens == nil || *back.ObservedInputTokens != 24512 {
		t.Errorf("observed mismatch: %+v", back.ObservedInputTokens)
	}
	m := parseV3Line(t, raw)
	if m["kind"].(string) != "turn_start" || m["version"].(float64) != 3 {
		t.Errorf("wire format mismatch: %v", m)
	}
	if m["observed_input_tokens"].(float64) != 24512 {
		t.Errorf("observed wire mismatch: %v", m["observed_input_tokens"])
	}
}

func TestRoundtripTurnStartEntryOmitsObservedWhenNil(t *testing.T) {
	entry := turnStartEntry{
		Kind:           "turn_start",
		Version:        3,
		At:             "2026-09-24T03:00:00Z",
		Turn:           1,
		UserMessageID:  "msg-x",
		ContextWindow:  128000,
		EstimateTokens: 100,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(raw, []byte("observed_input_tokens")) {
		t.Errorf("observed_input_tokens should be omitted when nil, got %s", raw)
	}
}

func TestRoundtripTurnResponseEntry(t *testing.T) {
	ttft := int64(312)
	entry := turnResponseEntry{
		Kind:             "turn_response",
		Version:          3,
		At:               "2026-09-24T03:00:01Z",
		Turn:             3,
		Model:            "claude-opus-4-5",
		Vendor:           "anthropic",
		FinishReason:     "end_turn",
		DurationMS:       943,
		TTFTMS:           &ttft,
		PromptTokens:     24512,
		CompletionTokens: 812,
		TotalTokens:      25324,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back turnResponseEntry
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Model != "claude-opus-4-5" || back.Vendor != "anthropic" {
		t.Errorf("model/vendor mismatch: %+v", back)
	}
	if back.FinishReason != "end_turn" || back.DurationMS != 943 || back.TTFTMS == nil || *back.TTFTMS != 312 {
		t.Errorf("duration/ttft mismatch: %+v", back)
	}
	if back.PromptTokens != 24512 || back.CompletionTokens != 812 || back.TotalTokens != 25324 {
		t.Errorf("tokens mismatch: %+v", back)
	}
	m := parseV3Line(t, raw)
	if m["ttft_ms"].(float64) != 312 {
		t.Errorf("ttft_ms wire mismatch: %v", m["ttft_ms"])
	}
	if m["finish_reason"].(string) != "end_turn" {
		t.Errorf("finish_reason wire mismatch: %v", m["finish_reason"])
	}
}

func TestRoundtripTurnResponseEntryOmitsTTFTWhenNil(t *testing.T) {
	entry := turnResponseEntry{
		Kind:         "turn_response",
		Version:      3,
		At:           "2026-09-24T03:00:01Z",
		Turn:         1,
		Model:        "x",
		Vendor:       "y",
		FinishReason: "stop",
		DurationMS:   100,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(raw, []byte("ttft_ms")) {
		t.Errorf("ttft_ms should be omitted when nil, got %s", raw)
	}
}

func TestRoundtripToolDedupHitEntry(t *testing.T) {
	entry := toolDedupHitEntry{
		Kind:            "tool_dedup_hit",
		Version:         3,
		At:              "2026-09-24T03:00:02Z",
		ToolName:        "bash",
		SignatureSHA256: "abcdef0123456789",
		ReusedFromSeq:   280320106,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back toolDedupHitEntry
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.ToolName != "bash" || back.SignatureSHA256 != "abcdef0123456789" || back.ReusedFromSeq != 280320106 {
		t.Errorf("mismatch: %+v", back)
	}
}

func TestRoundtripCompactionV3Entry(t *testing.T) {
	entry := compactionV3Entry{
		Kind:          "compaction_v3",
		Version:       3,
		At:            "2026-09-24T03:00:03Z",
		Trigger:       "manual_command",
		TriggerDetail: "/compact --force",
		Strategy:      "reactive",
		TokensBefore:  75200,
		TokensAfter:   12400,
		Model:         "claude-opus-4-5",
		FirstKeptSeq:  280320106,
		DurationMS:    4821,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back compactionV3Entry
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Trigger != "manual_command" || back.TriggerDetail != "/compact --force" || back.Strategy != "reactive" {
		t.Errorf("trigger/strategy mismatch: %+v", back)
	}
	if back.TokensBefore != 75200 || back.TokensAfter != 12400 || back.FirstKeptSeq != 280320106 || back.DurationMS != 4821 {
		t.Errorf("count/duration mismatch: %+v", back)
	}
}

func TestRoundtripCompactionV3EntryOmitsTriggerDetail(t *testing.T) {
	entry := compactionV3Entry{
		Kind:         "compaction_v3",
		Version:      3,
		At:           "2026-09-24T03:00:03Z",
		Trigger:      "reactive_threshold",
		Strategy:     "reactive",
		TokensBefore: 75200,
		TokensAfter:  12400,
		Model:        "m",
		FirstKeptSeq: 1,
		DurationMS:   100,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(raw, []byte("trigger_detail")) {
		t.Errorf("trigger_detail should be omitted when empty, got %s", raw)
	}
}

func TestRoundtripErrorV3Entry(t *testing.T) {
	entry := errorV3Entry{
		Kind:       "error",
		Version:    3,
		At:         "2026-09-24T03:00:04Z",
		Scope:      "llm_stream",
		Code:       "request_too_large",
		Message:    "request exceeded 200k",
		Stack:      "goroutine 1 [running]:\nmain.foo()",
		RetryCount: 1,
		Recovered:  false,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back errorV3Entry
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Scope != "llm_stream" || back.Code != "request_too_large" || back.Message != "request exceeded 200k" {
		t.Errorf("scope/code/message mismatch: %+v", back)
	}
	if back.Stack == "" || back.RetryCount != 1 || back.Recovered != false {
		t.Errorf("stack/retry/recovered mismatch: %+v", back)
	}
}

func TestRoundtripErrorV3EntryOmitsEmptyOptionalFields(t *testing.T) {
	entry := errorV3Entry{
		Kind:       "error",
		Version:    3,
		At:         "2026-09-24T03:00:04Z",
		Scope:      "tool_invoke",
		Message:    "boom",
		RetryCount: 0,
		Recovered:  false,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(raw, []byte("code")) {
		t.Errorf("code should be omitted when empty, got %s", raw)
	}
	if bytes.Contains(raw, []byte("stack")) {
		t.Errorf("stack should be omitted when empty, got %s", raw)
	}
}

func TestAgentExposesV3WriteMethods(t *testing.T) {
	a, _ := newV3TestAgent()
	a.logMu.Lock()
	a.writeTurnStartLocked("msg-1")
	a.writeTurnResponseLocked("model", "vendor", "end_turn", 100, nil, 0, 0, 0)
	a.writeToolDedupHitLocked("bash", "deadbeef", 7)
	a.writeCompactionV3Locked("reactive_threshold", "", "reactive", 1000, 200, 5, "m", 50)
	a.writeErrorV3Locked("tool_invoke", "", "boom", 0, false)
	a.logMu.Unlock()
}

func TestAgentWriteMethodsAreNoopWithoutLogWriter(t *testing.T) {
	a := &Agent{}
	a.writeTurnStartLocked("msg-1")
	a.writeTurnResponseLocked("m", "v", "stop", 1, nil, 0, 0, 0)
	a.writeToolDedupHitLocked("bash", "x", 1)
	a.writeCompactionV3Locked("manual_command", "", "reactive", 1, 1, 1, "m", 1)
	a.writeErrorV3Locked("aft_worker", "crash", "boom", 1, false)
}
