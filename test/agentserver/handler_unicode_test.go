package agentserver_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

func TestMarshalEventFinalAnswerPreservesLiteralAngleBracketsAndAmpersand(t *testing.T) {
	payload := json_rpc.MessageEvent{
		ID:         "0190aaaa-test-aaaa-aaaa-aaaaaaaaaaaa",
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		StopReason: "end_turn",
		Message: json_rpc.Message{
			Role: "assistant",
			Content: []json_rpc.MessageContentPart{
				{Type: "thinking", Thinking: "thinking block"},
				{Type: "text", Text: "use <value> & <tag> in payload"},
			},
		},
	}

	var buf bytes.Buffer
	if err := json_rpc.MarshalEvent(&buf, "req-1", json_rpc.EventMessage, payload); err != nil {
		t.Fatalf("MarshalEvent: %v", err)
	}
	wire := buf.String()

	for _, escaped := range []string{`\u003c`, `\u003e`, `\u0026`} {
		if strings.Contains(wire, escaped) {
			t.Errorf("wire payload contains HTML-escape %q; want literal characters\nwire: %s", escaped, wire)
		}
	}
	for _, want := range []string{"<value>", "<tag>", "&"} {
		if !strings.Contains(wire, want) {
			t.Errorf("wire payload missing literal %q\nwire: %s", want, wire)
		}
	}

	var envelope json_rpc.Response
	if err := json.Unmarshal([]byte(strings.TrimRight(wire, "\n")), &envelope); err != nil {
		t.Fatalf("wire payload is not valid JSON: %v\nwire: %s", err, wire)
	}
	var roundTrip json_rpc.MessageEvent
	if err := json.Unmarshal(envelope.Data, &roundTrip); err != nil {
		t.Fatalf("envelope.Data is not valid JSON for MessageEvent: %v\ndata: %s", err, envelope.Data)
	}
	if len(roundTrip.Message.Content) < 2 {
		t.Fatalf("round-tripped content length=%d, want >=2\nwire: %s", len(roundTrip.Message.Content), wire)
	}
	if got := roundTrip.Message.Content[1].Text; got != "use <value> & <tag> in payload" {
		t.Errorf("round-tripped text=%q", got)
	}
}

func TestMarshalEventUserMessagePreservesLiteralAngleBracketsAndAmpersand(t *testing.T) {
	payload := json_rpc.MessageEvent{
		ID:         "0190bbbb-test-bbbb-bbbb-bbbbbbbbbbbb",
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		StopReason: "end_turn",
		Message: json_rpc.Message{
			Role: "user",
			Content: []json_rpc.MessageContentPart{
				{Type: "text", Text: "search for <div> & <span>"},
			},
		},
	}

	var buf bytes.Buffer
	if err := json_rpc.MarshalEvent(&buf, "req-2", json_rpc.EventMessage, payload); err != nil {
		t.Fatalf("MarshalEvent: %v", err)
	}
	wire := buf.String()

	for _, escaped := range []string{`\u003c`, `\u003e`, `\u0026`} {
		if strings.Contains(wire, escaped) {
			t.Errorf("wire payload contains HTML-escape %q; want literal characters\nwire: %s", escaped, wire)
		}
	}
}

func TestMarshalEventCustomEventPreservesLiteralAngleBracketsAndAmpersand(t *testing.T) {
	payload := json_rpc.CustomEvent{
		ID:         "0190dddd-test-dddd-dddd-dddddddddddd",
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		CustomType: "tool_error",
		Data:       json.RawMessage(`{"message":"line1 <err> & line2"}`),
	}

	var buf bytes.Buffer
	if err := json_rpc.MarshalEvent(&buf, "req-3", json_rpc.EventCustom, payload); err != nil {
		t.Fatalf("MarshalEvent: %v", err)
	}
	wire := buf.String()

	for _, escaped := range []string{`\u003c`, `\u003e`, `\u0026`} {
		if strings.Contains(wire, escaped) {
			t.Errorf("wire payload contains HTML-escape %q; want literal characters\nwire: %s", escaped, wire)
		}
	}
}
