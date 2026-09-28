package protocol_test

import (
	"context"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol/anthropic"
)

func drainEvents(t *testing.T, ch <-chan llm.StreamEvent) []llm.StreamEvent {
	t.Helper()
	var events []llm.StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}
	return events
}

func TestParseAnthropicSSE_TextStreamEmitsTypedEvents(t *testing.T) {
	body := `event: message_start
data: {"type":"message_start","message":{"id":"msg_01","usage":{"input_tokens":100}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}

event: message_stop
data: {"type":"message_stop"}

`
	ch, err := anthropic.ParseAnthropicSSE(context.Background(), strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseAnthropicSSE: %v", err)
	}
	got := drainEvents(t, ch)

	wantTypes := []reflect.Type{
		reflect.TypeOf(llm.EventSessionID{}),
		reflect.TypeOf(llm.EventTextDelta{}),
		reflect.TypeOf(llm.EventTextDelta{}),
		reflect.TypeOf(llm.EventUsage{}),
		reflect.TypeOf(llm.EventFinish{}),
	}
	if len(got) != len(wantTypes) {
		t.Fatalf("got %d events %v, want %d %v", len(got), eventTypeNames(got), len(wantTypes), typeNames(wantTypes))
	}
	for i, want := range wantTypes {
		if reflect.TypeOf(got[i]) != want {
			t.Errorf("event[%d] type = %T, want %v", i, got[i], want)
		}
	}

	if s, ok := got[0].(llm.EventSessionID); !ok || s.ID != "msg_01" {
		t.Errorf("event[0] = %+v, want EventSessionID{msg_01}", got[0])
	}
	if d, ok := got[1].(llm.EventTextDelta); !ok || d.Text != "Hello" {
		t.Errorf("event[1] = %+v, want EventTextDelta{Hello}", got[1])
	}
	if d, ok := got[2].(llm.EventTextDelta); !ok || d.Text != " world" {
		t.Errorf("event[2] = %+v, want EventTextDelta{ world}", got[2])
	}
	if u, ok := got[3].(llm.EventUsage); !ok || u.OutputTokens != 2 || u.InputTokens != 100 {
		t.Errorf("event[3] = %+v, want EventUsage{input=100 output=2}", got[3])
	}
	if f, ok := got[4].(llm.EventFinish); !ok || f.Reason != llm.FinishReasonStop {
		t.Errorf("event[4] = %+v, want EventFinish{stop}", got[4])
	}
}

func TestParseAnthropicSSE_ThinkingStreamProducesFullLifecycle(t *testing.T) {
	body := `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"deep thought"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-xyz"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

`
	ch, err := anthropic.ParseAnthropicSSE(context.Background(), strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseAnthropicSSE: %v", err)
	}
	got := drainEvents(t, ch)

	wantTypes := []reflect.Type{
		reflect.TypeOf(llm.EventThinkingStart{}),
		reflect.TypeOf(llm.EventThinkingDelta{}),
		reflect.TypeOf(llm.EventThinkingSignature{}),
		reflect.TypeOf(llm.EventThinkingEnd{}),
	}
	if len(got) != len(wantTypes) {
		t.Fatalf("got %d events %v, want %d", len(got), eventTypeNames(got), len(wantTypes))
	}
	for i, want := range wantTypes {
		if reflect.TypeOf(got[i]) != want {
			t.Errorf("event[%d] type = %T, want %v", i, got[i], want)
		}
	}
	if d, ok := got[1].(llm.EventThinkingDelta); !ok || d.Text != "deep thought" {
		t.Errorf("event[1] = %+v", got[1])
	}
	if s, ok := got[2].(llm.EventThinkingSignature); !ok || s.Signature != "sig-xyz" {
		t.Errorf("event[2] = %+v", got[2])
	}
}

func TestParseAnthropicSSE_ToolUseStream(t *testing.T) {
	body := `event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_01","name":"bash","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"cmd\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"ls\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

`
	ch, err := anthropic.ParseAnthropicSSE(context.Background(), strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseAnthropicSSE: %v", err)
	}
	got := drainEvents(t, ch)

	if len(got) != 4 {
		t.Fatalf("got %d events, want 4", len(got))
	}
	start, ok := got[0].(llm.EventToolStart)
	if !ok {
		t.Fatalf("event[0] type = %T, want EventToolStart", got[0])
	}
	if start.ID != "toolu_01" || start.Name != "bash" {
		t.Errorf("start = %+v", start)
	}

	d1, ok := got[1].(llm.EventToolDelta)
	if !ok {
		t.Fatalf("event[1] type = %T, want EventToolDelta", got[1])
	}
	if d1.ID != "toolu_01" || d1.JSON != `{"cmd":` {
		t.Errorf("d1 = %+v", d1)
	}

	d2, ok := got[2].(llm.EventToolDelta)
	if !ok {
		t.Fatalf("event[2] type = %T, want EventToolDelta", got[2])
	}
	if d2.ID != "toolu_01" || d2.JSON != `"ls"}` {
		t.Errorf("d2 = %+v", d2)
	}

	end, ok := got[3].(llm.EventToolEnd)
	if !ok {
		t.Fatalf("event[3] type = %T, want EventToolEnd", got[3])
	}
	if end.ID != "toolu_01" {
		t.Errorf("end = %+v", end)
	}
}

func TestParseAnthropicSSE_MixedTextThenToolUse(t *testing.T) {
	body := `event: message_start
data: {"type":"message_start","message":{"id":"m-1"}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Sure. "}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"bash","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}

event: message_stop
data: {"type":"message_stop"}

`
	ch, err := anthropic.ParseAnthropicSSE(context.Background(), strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseAnthropicSSE: %v", err)
	}
	got := drainEvents(t, ch)

	wantTypes := []reflect.Type{
		reflect.TypeOf(llm.EventSessionID{}),
		reflect.TypeOf(llm.EventTextDelta{}),
		reflect.TypeOf(llm.EventToolStart{}),
		reflect.TypeOf(llm.EventToolDelta{}),
		reflect.TypeOf(llm.EventToolEnd{}),
		reflect.TypeOf(llm.EventUsage{}),
		reflect.TypeOf(llm.EventFinish{}),
	}
	if len(got) != len(wantTypes) {
		t.Fatalf("got %d events %v, want %d %v", len(got), eventTypeNames(got), len(wantTypes), typeNames(wantTypes))
	}
	for i, want := range wantTypes {
		if reflect.TypeOf(got[i]) != want {
			t.Errorf("event[%d] type = %T, want %v", i, got[i], want)
		}
	}
	if u, ok := got[5].(llm.EventUsage); !ok || u.OutputTokens != 7 {
		t.Errorf("event[5] = %+v", got[5])
	}
	if f, ok := got[6].(llm.EventFinish); !ok || f.Reason != llm.FinishReasonToolUse {
		t.Errorf("event[6] = %+v, want FinishReason=tool_use", got[6])
	}
}

func TestParseAnthropicSSE_StopReasonMaxTokensMapsToLength(t *testing.T) {
	body := `event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":4096}}

`
	ch, err := anthropic.ParseAnthropicSSE(context.Background(), strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseAnthropicSSE: %v", err)
	}
	got := drainEvents(t, ch)
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	if u, ok := got[0].(llm.EventUsage); !ok || u.OutputTokens != 4096 {
		t.Errorf("event[0] = %+v", got[0])
	}
	if f, ok := got[1].(llm.EventFinish); !ok || f.Reason != llm.FinishReasonLength {
		t.Errorf("event[1] = %+v, want FinishReason=length", got[1])
	}
}

func TestParseAnthropicSSE_StopReasonRefusalMapsToContentFilter(t *testing.T) {
	body := `event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"refusal"}}

`
	ch, err := anthropic.ParseAnthropicSSE(context.Background(), strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	got := drainEvents(t, ch)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if f, ok := got[0].(llm.EventFinish); !ok || f.Reason != llm.FinishReasonContentFilter {
		t.Errorf("event[0] = %+v, want FinishReason=content_filter", got[0])
	}
}

func TestParseAnthropicSSE_CacheFieldsOnUsage(t *testing.T) {
	body := `event: message_start
data: {"type":"message_start","message":{"id":"m-1","usage":{"input_tokens":100,"cache_read_input_tokens":80,"cache_creation_input_tokens":20}}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5,"cache_read_input_tokens":80,"cache_creation_input_tokens":20}}

event: message_stop
data: {"type":"message_stop"}

`
	ch, err := anthropic.ParseAnthropicSSE(context.Background(), strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	got := drainEvents(t, ch)
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	if u, ok := got[1].(llm.EventUsage); !ok {
		t.Fatalf("event[1] type = %T", got[1])
	} else if u.CacheReadTokens != 80 || u.CacheCreationTokens != 20 || u.OutputTokens != 5 {
		t.Errorf("event[1] = %+v", u)
	}
}

func TestParseAnthropicSSE_MalformedJSONEmitsEventErrThenCloses(t *testing.T) {
	body := `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {this is not json}

event: message_stop
data: {"type":"message_stop"}

`
	ch, err := anthropic.ParseAnthropicSSE(context.Background(), strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var events []llm.StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}
	sawErr := false
	for _, ev := range events {
		if _, ok := ev.(llm.EventErr); ok {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatalf("expected EventErr in %v", eventTypeNames(events))
	}
}

func TestParseAnthropicSSE_EOFClosesCleanly(t *testing.T) {
	body := `event: message_stop
data: {"type":"message_stop"}

`
	ch, err := anthropic.ParseAnthropicSSE(context.Background(), strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not close within 2s of EOF")
	}
}

func TestParseAnthropicSSE_CtxCancelClosesChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := newBlockingPipe()

	ch, err := anthropic.ParseAnthropicSSE(ctx, pr)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not close after ctx cancel")
	}
	pw.Close()
}

type blockingPipe struct {
	mu   sync.Mutex
	buf  []byte
	cond *sync.Cond
	eof  bool
}

func newBlockingPipe() (*blockingPipeReader, *blockingPipeWriter) {
	p := &blockingPipe{}
	p.cond = sync.NewCond(&p.mu)
	return &blockingPipeReader{p: p}, &blockingPipeWriter{p: p}
}

type blockingPipeReader struct {
	p *blockingPipe
}

func (r *blockingPipeReader) Read(p []byte) (int, error) {
	r.p.mu.Lock()
	defer r.p.mu.Unlock()
	for len(r.p.buf) == 0 && !r.p.eof {
		r.p.cond.Wait()
	}
	if len(r.p.buf) == 0 && r.p.eof {
		return 0, io.EOF
	}
	n := copy(p, r.p.buf)
	r.p.buf = r.p.buf[n:]
	r.p.cond.Broadcast()
	return n, nil
}

type blockingPipeWriter struct {
	p *blockingPipe
}

func (w *blockingPipeWriter) Write(p []byte) (int, error) {
	w.p.mu.Lock()
	defer w.p.mu.Unlock()
	w.p.buf = append(w.p.buf, p...)
	w.p.cond.Broadcast()
	return len(p), nil
}

func (w *blockingPipeWriter) Close() error {
	w.p.mu.Lock()
	w.p.eof = true
	w.p.mu.Unlock()
	w.p.cond.Broadcast()
	return nil
}

func eventTypeNames(evs []llm.StreamEvent) []string {
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = reflect.TypeOf(ev).Name()
	}
	return out
}

func typeNames(types []reflect.Type) []string {
	out := make([]string, len(types))
	for i, ty := range types {
		out[i] = ty.Name()
	}
	return out
}
