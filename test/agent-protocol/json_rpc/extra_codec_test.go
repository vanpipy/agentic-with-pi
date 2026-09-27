package json_rpc_test

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

type errWriter struct {
	err error
}

func (e errWriter) Write([]byte) (int, error) {
	return 0, e.err
}

func TestMarshalRequestWriterError(t *testing.T) {
	req, err := json_rpc.NewRequest("id-1", "method", map[string]any{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}

	w := errWriter{err: errors.New("write failed")}
	if err := json_rpc.MarshalRequest(w, req); err == nil {
		t.Fatal("expected write error to propagate")
	}
}

func TestMarshalRequestMarshalError(t *testing.T) {
	req := &json_rpc.Request{
		JSONRPC: "2.0",
		ID:      "id-1",
		Method:  "method",
		Params:  []byte(`{"k":`),
	}

	if err := json_rpc.MarshalRequest(&bytes.Buffer{}, req); err == nil {
		t.Fatal("expected marshal error to propagate")
	}
}

func TestMarshalRequestTrailingNewline(t *testing.T) {
	req, _ := json_rpc.NewRequest("id", "m", nil)
	var buf bytes.Buffer
	if err := json_rpc.MarshalRequest(&buf, req); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()
	if len(out) == 0 || out[len(out)-1] != '\n' {
		t.Errorf("expected trailing newline; got %q", out)
	}
}

func TestMarshalEventWriterError(t *testing.T) {
	w := errWriter{err: errors.New("write failed")}
	if err := json_rpc.MarshalEvent(w, "id", "event", map[string]any{"k": "v"}); err == nil {
		t.Fatal("expected write error to propagate")
	}
}

func TestMarshalEventDataMarshalError(t *testing.T) {
	w := errWriter{err: errors.New("never reached")}
	data := func() {} // functions cannot be marshaled
	if err := json_rpc.MarshalEvent(w, "id", "event", data); err == nil {
		t.Fatal("expected marshal error on unserializable data")
	}
}

func TestMarshalEventResponseMarshalError(t *testing.T) {
	w := errWriter{err: errors.New("never reached")}

	bad := map[string]any{
		"chan": make(chan int),
	}
	if err := json_rpc.MarshalEvent(w, "id", "event", bad); err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestMarshalEventTrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	if err := json_rpc.MarshalEvent(&buf, "id", "event", nil); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()
	if len(out) == 0 || out[len(out)-1] != '\n' {
		t.Errorf("expected trailing newline; got %q", out)
	}
}

func TestNewRequestParamsMarshalError(t *testing.T) {
	_, err := json_rpc.NewRequest("id", "m", func() {})
	if err == nil {
		t.Fatal("expected marshal error for unserializable params")
	}
}

func TestNewRequestNilParams(t *testing.T) {
	req, err := json_rpc.NewRequest("id", "m", nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.Params != nil {
		t.Errorf("expected nil Params when params arg is nil; got %q", req.Params)
	}
}

func TestReadRequestUnsupportedVersionExtra(t *testing.T) {
	line := []byte(`{"jsonrpc":"1.0","id":"x","method":"m"}` + "\n")
	r := newBufioReader(line)
	_, err := json_rpc.ReadRequest(r)
	if err == nil {
		t.Fatal("expected unsupported version error")
	}
	if !strings.Contains(err.Error(), "unsupported jsonrpc") {
		t.Errorf("err = %v, want 'unsupported jsonrpc'", err)
	}
}

func TestReadRequestMalformedJSONExtra(t *testing.T) {
	line := []byte(`{not valid json` + "\n")
	r := newBufioReader(line)
	_, err := json_rpc.ReadRequest(r)
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("err = %v, want 'unmarshal'", err)
	}
}

func TestReadRequestEOFExtra(t *testing.T) {
	r := newBufioReader([]byte{})
	_, err := json_rpc.ReadRequest(r)
	if err == nil {
		t.Fatal("expected EOF error")
	}
}

func TestReadEventMalformedJSON(t *testing.T) {
	line := []byte(`{still-broken` + "\n")
	r := newBufioReader(line)
	_, err := json_rpc.ReadEvent(r)
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestReadEventEOFExtra(t *testing.T) {
	r := newBufioReader([]byte{})
	_, err := json_rpc.ReadEvent(r)
	if err == nil {
		t.Fatal("expected EOF error")
	}
}

func TestReadRequestIgnoresTrailingNewline(t *testing.T) {
	line := []byte(`{"jsonrpc":"2.0","id":"x","method":"m"}` + "\n\n")
	r := newBufioReader(line)
	req, err := json_rpc.ReadRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	if req.ID != "x" {
		t.Errorf("ID = %q, want x", req.ID)
	}
}

func TestMarshalRequestRoundTrip(t *testing.T) {
	req, _ := json_rpc.NewRequest("id", "m", map[string]any{"k": "v"})
	var buf bytes.Buffer
	if err := json_rpc.MarshalRequest(&buf, req); err != nil {
		t.Fatal(err)
	}

	r := newBufioReader(buf.Bytes())
	got, err := json_rpc.ReadRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != req.ID || got.Method != req.Method {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", got, req)
	}
}

func TestMarshalEventRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := json_rpc.MarshalEvent(&buf, "id", "ev", map[string]any{"k": "v"}); err != nil {
		t.Fatal(err)
	}

	r := newBufioReader(buf.Bytes())
	got, err := json_rpc.ReadEvent(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "id" || got.Event != "ev" {
		t.Errorf("roundtrip mismatch: got %+v", got)
	}
}

func TestMarshalRequestNoHTMLEscape(t *testing.T) {
	req, _ := json_rpc.NewRequest("id", "m", map[string]any{"html": "<script>"})
	var buf bytes.Buffer
	if err := json_rpc.MarshalRequest(&buf, req); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `\u003c`) {
		t.Errorf("expected unescaped <; got %q", buf.String())
	}
	if !strings.Contains(buf.String(), `<script>`) {
		t.Errorf("expected literal <script> in output; got %q", buf.String())
	}
}

func TestNewV7UniqueIDs(t *testing.T) {
	const N = 1000
	seen := make(map[string]bool, N)
	for i := 0; i < N; i++ {
		id := json_rpc.NewV7()
		if seen[id] {
			t.Errorf("duplicate id at i=%d: %s", i, id)
		}
		seen[id] = true
	}
}

func TestNewV7Format(t *testing.T) {
	id := json_rpc.NewV7()
	if len(id) != 36 {
		t.Errorf("len = %d, want 36", len(id))
	}
	for i, c := range id {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				t.Errorf("position %d: want '-', got %q", i, c)
			}
		default:
			if !isHex(byte(c)) {
				t.Errorf("position %d: want hex, got %q", i, c)
			}
		}
	}
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
}

func newBufioReader(b []byte) *bufio.Reader {
	return bufio.NewReader(bytes.NewReader(b))
}
