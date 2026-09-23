package protocol_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

func TestNewRequestWithParams(t *testing.T) {
	req, err := json_rpc.NewRequest("1", json_rpc.MethodPrompt, json_rpc.PromptParams{
		SessionID: "abc",
		Prompt:    "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q", req.JSONRPC)
	}
	if req.ID != "1" {
		t.Errorf("id = %q", req.ID)
	}
	if req.Method != json_rpc.MethodPrompt {
		t.Errorf("method = %q", req.Method)
	}
	if len(req.Params) == 0 {
		t.Error("params should be set")
	}
}

func TestNewRequestNoParams(t *testing.T) {
	req, err := json_rpc.NewRequest("1", json_rpc.MethodPing, nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != json_rpc.MethodPing {
		t.Errorf("method = %q", req.Method)
	}
	if len(req.Params) > 0 {
		t.Errorf("params should be empty, got %q", req.Params)
	}
}

func TestMarshalRequestSingleLine(t *testing.T) {
	req, _ := json_rpc.NewRequest("1", json_rpc.MethodPrompt, json_rpc.PromptParams{
		Prompt: "line1\nline2",
	})

	var buf bytes.Buffer
	if err := json_rpc.MarshalRequest(&buf, req); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line, got %d", len(lines))
	}
}

func TestReadRequestRoundTrip(t *testing.T) {
	req, _ := json_rpc.NewRequest("42", json_rpc.MethodPing, nil)

	var buf bytes.Buffer
	if err := json_rpc.MarshalRequest(&buf, req); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(&buf)
	parsed, err := json_rpc.ReadRequest(reader)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ID != "42" {
		t.Errorf("id = %q", parsed.ID)
	}
	if parsed.Method != json_rpc.MethodPing {
		t.Errorf("method = %q", parsed.Method)
	}
}

func TestReadRequestInvalidJSON(t *testing.T) {
	buf := bytes.NewBufferString("not json\n")
	reader := bufio.NewReader(buf)
	if _, err := json_rpc.ReadRequest(reader); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestReadRequestWrongJSONRPC(t *testing.T) {
	buf := bytes.NewBufferString(`{"jsonrpc":"1.0","id":"1","method":"x"}` + "\n")
	reader := bufio.NewReader(buf)
	if _, err := json_rpc.ReadRequest(reader); err == nil {
		t.Error("expected error for unsupported jsonrpc")
	}
}

func TestMarshalEventWithData(t *testing.T) {
	var buf bytes.Buffer
	if err := json_rpc.MarshalEvent(&buf, "1", json_rpc.EventFinalAnswer, map[string]string{"content": "ok"}); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(&buf)
	resp, err := json_rpc.ReadEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "1" {
		t.Errorf("id = %q", resp.ID)
	}
	if resp.Event != json_rpc.EventFinalAnswer {
		t.Errorf("event = %q", resp.Event)
	}
	if len(resp.Data) == 0 {
		t.Error("data should be set")
	}

	var data struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Content != "ok" {
		t.Errorf("content = %q", data.Content)
	}
}

func TestMarshalEventNoData(t *testing.T) {
	var buf bytes.Buffer
	if err := json_rpc.MarshalEvent(&buf, "1", json_rpc.EventThoughtStart, nil); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(&buf)
	resp, err := json_rpc.ReadEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Event != json_rpc.EventThoughtStart {
		t.Errorf("event = %q", resp.Event)
	}
}

func TestMethodConstants(t *testing.T) {
	methods := []string{
		json_rpc.MethodPrompt,
		json_rpc.MethodResume,
		json_rpc.MethodCancel,
		json_rpc.MethodPing,
	}
	if len(methods) != 4 {
		t.Errorf("expected 4 methods, got %d", len(methods))
	}
}

func TestEventConstants(t *testing.T) {
	events := []string{
		json_rpc.EventThoughtStart,
		json_rpc.EventThoughtChunk,
		json_rpc.EventThoughtEnd,
		json_rpc.EventTool,
		json_rpc.EventObserve,
		json_rpc.EventFinalAnswer,
		json_rpc.EventError,
	}
	if len(events) != 7 {
		t.Errorf("expected 7 events, got %d", len(events))
	}
}

func TestPromptParamsRoundTrip(t *testing.T) {
	params := json_rpc.PromptParams{
		SessionID: "session-xyz",
		Prompt:    "hello world",
	}
	req, err := json_rpc.NewRequest("1", json_rpc.MethodPrompt, params)
	if err != nil {
		t.Fatal(err)
	}

	var parsed json_rpc.PromptParams
	if err := json.Unmarshal(req.Params, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.SessionID != "session-xyz" {
		t.Errorf("session_id = %q", parsed.SessionID)
	}
	if parsed.Prompt != "hello world" {
		t.Errorf("prompt = %q", parsed.Prompt)
	}
}

func TestReadMultipleEvents(t *testing.T) {
	var buf bytes.Buffer
	for _, ev := range []string{
		json_rpc.EventThoughtStart,
		json_rpc.EventThoughtChunk,
		json_rpc.EventThoughtEnd,
	} {
		json_rpc.MarshalEvent(&buf, "1", ev, nil)
	}

	reader := bufio.NewReader(&buf)
	for _, want := range []string{
		json_rpc.EventThoughtStart,
		json_rpc.EventThoughtChunk,
		json_rpc.EventThoughtEnd,
	} {
		resp, err := json_rpc.ReadEvent(reader)
		if err != nil {
			t.Fatalf("read event: %v", err)
		}
		if resp.Event != want {
			t.Errorf("event = %q, want %q", resp.Event, want)
		}
	}
}
