package sse_test

import (
	"testing"

	"github.com/vanpiyp/awp/internal/llm/protocol/sse"
)

func TestSplitConcatenatedJSON_SingleObjectReturnsNil(t *testing.T) {
	payload := `{"id":"a","value":1}`
	got := sse.SplitConcatenatedJSON(payload)
	if got != nil {
		t.Errorf("SplitConcatenatedJSON on single object = %+v, want nil", got)
	}
}

func TestSplitConcatenatedJSON_TwoAdjacentObjects(t *testing.T) {
	payload := `{"id":"a"}{"id":"b"}`
	got := sse.SplitConcatenatedJSON(payload)
	if got == nil {
		t.Fatal("got nil, want non-nil split")
	}
	if len(got.Objects) != 2 {
		t.Fatalf("Objects len = %d, want 2", len(got.Objects))
	}
	if got.Objects[0] != `{"id":"a"}` {
		t.Errorf("Objects[0] = %q, want %q", got.Objects[0], `{"id":"a"}`)
	}
	if got.Objects[1] != `{"id":"b"}` {
		t.Errorf("Objects[1] = %q, want %q", got.Objects[1], `{"id":"b"}`)
	}
	if got.TrailingPartial != "" {
		t.Errorf("TrailingPartial = %q, want empty", got.TrailingPartial)
	}
}

func TestSplitConcatenatedJSON_EmbeddedDataPrefix(t *testing.T) {
	// A proxy that drops the SSE event separator may emit
	// `{"id":"a"}data: {"id":"b"}`. Both brace-balanced objects are
	// recoverable; the literal `data: ` between them is dropped because
	// it sits outside either object.
	payload := `{"id":"a"}data: {"id":"b"}`
	got := sse.SplitConcatenatedJSON(payload)
	if got == nil {
		t.Fatal("got nil, want non-nil split")
	}
	if len(got.Objects) != 2 {
		t.Fatalf("Objects len = %d, want 2", len(got.Objects))
	}
	if got.Objects[0] != `{"id":"a"}` {
		t.Errorf("Objects[0] = %q, want %q", got.Objects[0], `{"id":"a"}`)
	}
	if got.Objects[1] != `{"id":"b"}` {
		t.Errorf("Objects[1] = %q, want %q", got.Objects[1], `{"id":"b"}`)
	}
}

func TestSplitConcatenatedJSON_TruncatedTrailingObject(t *testing.T) {
	payload := `{"id":"a"}{"id":`
	got := sse.SplitConcatenatedJSON(payload)
	if got == nil {
		t.Fatal("got nil, want non-nil split (recoverable trailing partial)")
	}
	if len(got.Objects) != 1 {
		t.Fatalf("Objects len = %d, want 1", len(got.Objects))
	}
	if got.Objects[0] != `{"id":"a"}` {
		t.Errorf("Objects[0] = %q, want %q", got.Objects[0], `{"id":"a"}`)
	}
	if got.TrailingPartial != `{"id":` {
		t.Errorf("TrailingPartial = %q, want %q", got.TrailingPartial, `{"id":`)
	}
}

func TestSplitConcatenatedJSON_StringWithBracesDoesNotConfuseSplitter(t *testing.T) {
	payload := `{"text":"hello } world","id":"a"}{"id":"b"}`
	got := sse.SplitConcatenatedJSON(payload)
	if got == nil {
		t.Fatal("got nil, want non-nil split")
	}
	if len(got.Objects) != 2 {
		t.Fatalf("Objects len = %d, want 2", len(got.Objects))
	}
	if got.Objects[0] != payload[:len(payload)-len(`{"id":"b"}`)] {
		t.Errorf("Objects[0] = %q", got.Objects[0])
	}
	if got.Objects[1] != `{"id":"b"}` {
		t.Errorf("Objects[1] = %q, want %q", got.Objects[1], `{"id":"b"}`)
	}
}

func TestSplitConcatenatedJSON_EscapedQuoteInString(t *testing.T) {
	payload := `{"text":"a\"b","id":"a"}{"id":"b"}`
	got := sse.SplitConcatenatedJSON(payload)
	if got == nil {
		t.Fatal("got nil, want non-nil split")
	}
	if len(got.Objects) != 2 {
		t.Fatalf("Objects len = %d, want 2", len(got.Objects))
	}
}

func TestSplitConcatenatedJSON_EmptyReturnsNil(t *testing.T) {
	if got := sse.SplitConcatenatedJSON(""); got != nil {
		t.Errorf("SplitConcatenatedJSON(\"\") = %+v, want nil", got)
	}
}
