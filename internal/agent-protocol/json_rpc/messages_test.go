package json_rpc

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestMessageContentPartTextRoundtrip(t *testing.T) {
	part := MessageContentPart{Type: "text", Text: "hello world"}
	data, err := json.Marshal(part)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `"type":"text"`) {
		t.Fatalf("expected type=text in %q", got)
	}
	if !strings.Contains(got, `"text":"hello world"`) {
		t.Fatalf("expected text field in %q", got)
	}
	for _, hidden := range []string{"thinking", "thinkingSignature", "id", "name", "intent", "arguments"} {
		if strings.Contains(got, `"`+hidden+`":`) {
			t.Fatalf("unexpected %q field in %q", hidden, got)
		}
	}
	var back MessageContentPart
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(back, part) {
		t.Fatalf("roundtrip mismatch: want %+v got %+v", part, back)
	}
}

func TestMessageContentPartThinkingRoundtrip(t *testing.T) {
	part := MessageContentPart{
		Type:              "thinking",
		Thinking:          "the user asked for x",
		ThinkingSignature: "deadbeef",
	}
	data, err := json.Marshal(part)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"type":"thinking"`) {
		t.Fatalf("missing type:thinking in %q", data)
	}
	if !strings.Contains(string(data), `"thinkingSignature":"deadbeef"`) {
		t.Fatalf("missing thinkingSignature in %q", data)
	}
	var back MessageContentPart
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(back, part) {
		t.Fatalf("roundtrip mismatch: want %+v got %+v", part, back)
	}
}

func TestMessageContentPartToolCallRoundtrip(t *testing.T) {
	args := json.RawMessage(`{"pattern":"jcode"}`)
	part := MessageContentPart{
		Type:      "toolCall",
		ID:        "call_abc",
		Name:      "agentgrep",
		Intent:    "find jcode",
		Arguments: args,
	}
	data, err := json.Marshal(part)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"arguments":{"pattern":"jcode"}`) {
		t.Fatalf("arguments not preserved as nested JSON: %q", data)
	}
	var back MessageContentPart
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.ID != part.ID || back.Name != part.Name || back.Intent != part.Intent {
		t.Fatalf("id/name/intent mismatch: %+v vs %+v", part, back)
	}
	if string(back.Arguments) != string(part.Arguments) {
		t.Fatalf("arguments mismatch: %q vs %q", back.Arguments, part.Arguments)
	}
}

func TestMessageRoundtrip(t *testing.T) {
	msg := Message{
		Role: "assistant",
		Content: []MessageContentPart{
			{Type: "thinking", Thinking: "I should call bash", ThinkingSignature: "sig1"},
			{Type: "text", Text: "Let me check the date."},
			{Type: "toolCall", ID: "call_001", Name: "bash", Arguments: json.RawMessage(`{"command":"date"}`)},
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Message
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Role != msg.Role {
		t.Fatalf("role mismatch: %q vs %q", back.Role, msg.Role)
	}
	if len(back.Content) != len(msg.Content) {
		t.Fatalf("content length mismatch: %d vs %d", len(back.Content), len(msg.Content))
	}
	for i, p := range msg.Content {
		if !reflect.DeepEqual(back.Content[i], p) {
			t.Fatalf("content[%d] mismatch: %+v vs %+v", i, p, back.Content[i])
		}
	}
}

func TestUsageStatsRoundtrip(t *testing.T) {
	stats := UsageStats{
		PromptTokens:     1024,
		CompletionTokens: 171,
		TotalTokens:      1195,
		CacheReadTokens:  128,
		CacheWriteTokens: 0,
	}
	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), `"cacheWriteTokens":0`) {
		t.Fatalf("expected cacheWriteTokens=0 omitted, got %q", data)
	}
	var back UsageStats
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back != stats {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", stats, back)
	}
}

func TestUsageStatsMinimalOmitted(t *testing.T) {
	stats := UsageStats{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}
	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, hidden := range []string{"cacheRead", "cacheWrite"} {
		if strings.Contains(string(data), `"`+hidden) {
			t.Fatalf("expected %q omitted, got %q", hidden, data)
		}
	}
}

func TestMessageDetailsRoundtrip(t *testing.T) {
	det := MessageDetails{
		ToolName: "agentgrep",
		Intent:   "find jcode",
		Error:    "timeout",
		Args:     `{"pattern":"jcode"}`,
	}
	data, err := json.Marshal(det)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back MessageDetails
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back != det {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", det, back)
	}
}

func TestMessageEventRoundtrip(t *testing.T) {
	ev := MessageEvent{
		ID:        "0190a3b7-0002-7c8a-9000-000000000002",
		ParentID:  "0190a3b7-0001-7c8a-9000-000000000001",
		Timestamp: "2026-09-23T13:00:01.234Z",
		Message: Message{
			Role: "assistant",
			Content: []MessageContentPart{
				{Type: "text", Text: "I'll search for that."},
			},
		},
		StopReason: "toolUse",
		Usage:      &UsageStats{PromptTokens: 1024, CompletionTokens: 171, TotalTokens: 1195},
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"id":"0190a3b7-0002-7c8a-9000-000000000002","parentId":"0190a3b7-0001-7c8a-9000-000000000001","timestamp":"2026-09-23T13:00:01.234Z","message":{"role":"assistant","content":[{"type":"text","text":"I'll search for that."}]},"stopReason":"toolUse","usage":{"promptTokens":1024,"completionTokens":171,"totalTokens":1195}}`
	if string(data) != want {
		t.Fatalf("unexpected JSON:\nwant %q\ngot  %q", want, data)
	}
	var back MessageEvent
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.ID != ev.ID || back.ParentID != ev.ParentID || back.Timestamp != ev.Timestamp {
		t.Fatalf("id/parent/timestamp mismatch")
	}
	if back.StopReason != "toolUse" {
		t.Fatalf("stopReason lost: %q", back.StopReason)
	}
	if back.Usage == nil || *back.Usage != *ev.Usage {
		t.Fatalf("usage lost")
	}
}

func TestMessageEventOmittedFields(t *testing.T) {
	ev := MessageEvent{
		ID:        "msg-1",
		Timestamp: "2026-09-23T13:00:01.234Z",
		Message: Message{
			Role:    "user",
			Content: []MessageContentPart{{Type: "text", Text: "hi"}},
		},
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, hidden := range []string{"parentId", "stopReason", "usage", "details"} {
		if strings.Contains(string(data), `"`+hidden+`":`) {
			t.Fatalf("unexpected %q in %q", hidden, data)
		}
	}
}

func TestCustomEventRoundtrip(t *testing.T) {
	ev := CustomEvent{
		ID:         "custom-1",
		ParentID:   "msg-2",
		Timestamp:  "2026-09-23T13:00:30.000Z",
		CustomType: "abort",
		Data:       json.RawMessage(`{"reason":"tool failed 3 times"}`),
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"id":"custom-1","parentId":"msg-2","timestamp":"2026-09-23T13:00:30.000Z","customType":"abort","data":{"reason":"tool failed 3 times"}}`
	if string(data) != want {
		t.Fatalf("unexpected JSON:\nwant %q\ngot  %q", want, data)
	}
	var back CustomEvent
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.ID != ev.ID || back.ParentID != ev.ParentID || back.CustomType != ev.CustomType {
		t.Fatalf("field mismatch")
	}
	if string(back.Data) != string(ev.Data) {
		t.Fatalf("data mismatch: %q vs %q", back.Data, ev.Data)
	}
}

func TestCustomMessageEventRoundtrip(t *testing.T) {
	ev := CustomMessageEvent{
		ID:         "cm-1",
		ParentID:   "msg-3",
		Timestamp:  "2026-09-23T13:00:35.000Z",
		CustomType: "subagent_notification",
		Content:    "subagent task T-42 finished",
		Display:    true,
		Details:    json.RawMessage(`{}`),
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back CustomMessageEvent
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Content != ev.Content || back.CustomType != ev.CustomType || back.Display != ev.Display {
		t.Fatalf("content/type/display mismatch")
	}
}

func TestCompactParamsRoundtrip(t *testing.T) {
	params := CompactParams{SessionID: "abc-123", Force: true}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"session_id":"abc-123","force":true}`
	if string(data) != want {
		t.Fatalf("CompactParams marshal mismatch:\nwant %q\ngot  %q", want, data)
	}
	var back CompactParams
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.SessionID != params.SessionID || back.Force != params.Force {
		t.Fatalf("roundtrip mismatch: want %+v got %+v", params, back)
	}
}

func TestCompactParamsForceOmittedWhenFalse(t *testing.T) {
	params := CompactParams{SessionID: "sess"}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), `"force":`) {
		t.Fatalf("expected force omitted when false, got %q", data)
	}
	want := `{"session_id":"sess"}`
	if string(data) != want {
		t.Fatalf("marshal mismatch:\nwant %q\ngot  %q", want, data)
	}
}

func TestCompactResultRoundtrip(t *testing.T) {
	res := CompactResult{
		Triggered:    true,
		Strategy:     "forced",
		TokensBefore: 12000,
		TokensAfter:  4200,
		DurationMS:   1500,
	}
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"triggered":true,"strategy":"forced","tokens_before":12000,"tokens_after":4200,"duration_ms":1500}`
	if string(data) != want {
		t.Fatalf("CompactResult marshal mismatch:\nwant %q\ngot  %q", want, data)
	}
	var back CompactResult
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back != res {
		t.Fatalf("roundtrip mismatch: want %+v got %+v", res, back)
	}
}

func TestCompactResultStrategyOmittedWhenEmpty(t *testing.T) {
	res := CompactResult{Triggered: false, TokensBefore: 1, TokensAfter: 1, DurationMS: 0}
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), `"strategy":`) {
		t.Fatalf("expected strategy omitted when empty, got %q", data)
	}
}

func TestCustomMessageEventDisplayFalseOmitted(t *testing.T) {
	ev := CustomMessageEvent{
		ID:         "cm-1",
		Timestamp:  "2026-09-23T13:00:35.000Z",
		CustomType: "log_only",
		Content:    "noise",
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), `"display":`) {
		t.Fatalf("expected display omitted when false, got %q", data)
	}
}
