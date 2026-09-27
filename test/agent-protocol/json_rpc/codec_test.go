package json_rpc_test

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

func TestNewRequestWithNilParams(t *testing.T) {
	req, err := json_rpc.NewRequest("1", "ping", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if req.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q", req.JSONRPC)
	}
	if req.ID != "1" {
		t.Errorf("ID = %q", req.ID)
	}
	if req.Method != "ping" {
		t.Errorf("Method = %q", req.Method)
	}
	if len(req.Params) != 0 {
		t.Errorf("Params should be empty for nil input; got %q", string(req.Params))
	}
}

func TestNewRequestWithParams(t *testing.T) {
	req, err := json_rpc.NewRequest("2", "echo", map[string]any{"hello": "world"})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if !bytes.Contains(req.Params, []byte("hello")) {
		t.Errorf("Params missing 'hello': %q", string(req.Params))
	}
	if !bytes.Contains(req.Params, []byte("world")) {
		t.Errorf("Params missing 'world': %q", string(req.Params))
	}
}

func TestNewRequestMarshalFailure(t *testing.T) {
	ch := make(chan int)
	if _, err := json_rpc.NewRequest("1", "x", ch); err == nil {
		t.Fatal("expected marshal error for chan parameter")
	}
}

func TestNewRequestDoesNotEscapeHTML(t *testing.T) {
	req, err := json_rpc.NewRequest("1", "x", map[string]any{"a": "<b>&"})
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if bytes.Contains(req.Params, []byte(`\u003c`)) {
		t.Errorf("Params should not HTML-escape: %q", string(req.Params))
	}
	if !bytes.Contains(req.Params, []byte("<b>&")) {
		t.Errorf("Params missing raw chars: %q", string(req.Params))
	}
}

func TestMarshalRequestHappyPath(t *testing.T) {
	req, err := json_rpc.NewRequest("1", "ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := json_rpc.MarshalRequest(&buf, req); err != nil {
		t.Fatalf("MarshalRequest: %v", err)
	}
	if !bytes.HasSuffix(buf.Bytes(), []byte{'\n'}) {
		t.Errorf("MarshalRequest missing trailing newline: %q", buf.String())
	}
	if !strings.Contains(buf.String(), `"method":"ping"`) {
		t.Errorf("missing method: %q", buf.String())
	}
}

type failWriter struct {
	fail bool
}

func (f *failWriter) Write(p []byte) (int, error) {
	if f.fail {
		return 0, errors.New("write-fail")
	}
	return len(p), nil
}

func TestMarshalRequestWriterFails(t *testing.T) {
	req, _ := json_rpc.NewRequest("1", "ping", nil)
	if err := json_rpc.MarshalRequest(&failWriter{fail: true}, req); err == nil {
		t.Fatal("expected write error")
	}
}

func TestReadRequestHappyPath(t *testing.T) {
	reqIn, _ := json_rpc.NewRequest("42", "echo", map[string]any{"k": "v"})
	var buf bytes.Buffer
	_ = json_rpc.MarshalRequest(&buf, reqIn)

	r := bufio.NewReader(&buf)
	reqOut, err := json_rpc.ReadRequest(r)
	if err != nil {
		t.Fatalf("ReadRequest: %v", err)
	}
	if reqOut.ID != "42" || reqOut.Method != "echo" {
		t.Errorf("unexpected request: %+v", reqOut)
	}
}

func TestReadRequestInvalidJSON(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader([]byte("not json\n")))
	if _, err := json_rpc.ReadRequest(r); err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestReadRequestUnsupportedVersion(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader([]byte(`{"jsonrpc":"1.0","id":"1","method":"x"}` + "\n")))
	_, err := json_rpc.ReadRequest(r)
	if err == nil {
		t.Fatal("expected unsupported version error")
	}
	if !strings.Contains(err.Error(), "unsupported jsonrpc") {
		t.Errorf("error = %q, want 'unsupported jsonrpc'", err.Error())
	}
}

func TestReadRequestEOF(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader([]byte("")))
	if _, err := json_rpc.ReadRequest(r); err == nil {
		t.Fatal("expected error on empty input")
	}
}

func TestMarshalEventHappyPath(t *testing.T) {
	var buf bytes.Buffer
	if err := json_rpc.MarshalEvent(&buf, "1", "evt", map[string]any{"a": 1}); err != nil {
		t.Fatalf("MarshalEvent: %v", err)
	}
	if !bytes.HasSuffix(buf.Bytes(), []byte{'\n'}) {
		t.Errorf("missing newline: %q", buf.String())
	}
	if !strings.Contains(buf.String(), `"event":"evt"`) {
		t.Errorf("missing event: %q", buf.String())
	}
}

func TestMarshalEventNilData(t *testing.T) {
	var buf bytes.Buffer
	if err := json_rpc.MarshalEvent(&buf, "1", "evt", nil); err != nil {
		t.Fatalf("MarshalEvent: %v", err)
	}
	if !strings.Contains(buf.String(), `"event":"evt"`) {
		t.Errorf("missing event: %q", buf.String())
	}
}

func TestMarshalEventWriterFails(t *testing.T) {
	if err := json_rpc.MarshalEvent(&failWriter{fail: true}, "1", "evt", map[string]any{"a": 1}); err == nil {
		t.Fatal("expected write error")
	}
}

func TestMarshalEventMarshalFails(t *testing.T) {
	ch := make(chan int)
	if err := json_rpc.MarshalEvent(&bytes.Buffer{}, "1", "evt", ch); err == nil {
		t.Fatal("expected marshal error for chan data")
	}
}

func TestReadEventHappyPath(t *testing.T) {
	var buf bytes.Buffer
	_ = json_rpc.MarshalEvent(&buf, "42", "evt", map[string]any{"k": "v"})

	r := bufio.NewReader(&buf)
	resp, err := json_rpc.ReadEvent(r)
	if err != nil {
		t.Fatalf("ReadEvent: %v", err)
	}
	if resp.ID != "42" || resp.Event != "evt" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestReadEventInvalidJSON(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader([]byte("garbage\n")))
	if _, err := json_rpc.ReadEvent(r); err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestReadEventEOF(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader([]byte("")))
	if _, err := json_rpc.ReadEvent(r); err == nil {
		t.Fatal("expected error on empty input")
	}
}
