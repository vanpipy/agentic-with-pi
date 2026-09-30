package sse_test

import (
	"testing"

	"github.com/vanpiyp/awp/internal/llm/protocol/sse"
)

func TestUtf8StreamDecoder_ASCIIPassesThrough(t *testing.T) {
	d := sse.NewUtf8StreamDecoder()
	got := d.Decode([]byte("hello"))
	if got != "hello" {
		t.Errorf("Decode = %q, want %q", got, "hello")
	}
}

func TestUtf8StreamDecoder_MultibyteRuneInOneChunk(t *testing.T) {
	d := sse.NewUtf8StreamDecoder()
	got := d.Decode([]byte("hello \xe4\xb8\x96\xe7\x95\x8c"))
	if got != "hello 世界" {
		t.Errorf("Decode = %q, want %q", got, "hello 世界")
	}
}

func TestUtf8StreamDecoder_RuneSplitAcrossChunksReassembles(t *testing.T) {
	d := sse.NewUtf8StreamDecoder()
	first := d.Decode([]byte("a\xe4\xb8"))
	if first != "a" {
		t.Errorf("first chunk = %q, want %q (truncated rune held)", first, "a")
	}
	if !d.HasPendingBytes() {
		t.Fatal("HasPendingBytes = false after truncated rune, want true")
	}
	second := d.Decode([]byte("\x96"))
	if first+second != "a世" {
		t.Errorf("after second chunk, concatenated = %q, want %q", first+second, "a世")
	}
	if d.HasPendingBytes() {
		t.Error("HasPendingBytes = true after rune complete, want false")
	}
}

func TestUtf8StreamDecoder_RuneSplitByteByByteReassembles(t *testing.T) {
	d := sse.NewUtf8StreamDecoder()
	want := "世"
	bytes := []byte(want)
	for _, b := range bytes[:len(bytes)-1] {
		out := d.Decode([]byte{b})
		if out != "" {
			t.Errorf("intermediate chunk returned %q, want empty (truncated rune held)", out)
		}
	}
	if !d.HasPendingBytes() {
		t.Fatal("HasPendingBytes = false after partial rune, want true")
	}
	final := d.Decode([]byte{bytes[len(bytes)-1]})
	if final != want {
		t.Errorf("final = %q, want %q", final, want)
	}
	if d.HasPendingBytes() {
		t.Error("HasPendingBytes = true after rune complete, want false")
	}
}

func TestUtf8StreamDecoder_InvalidByteReplacedWithReplacementChar(t *testing.T) {
	d := sse.NewUtf8StreamDecoder()
	got := d.Decode([]byte{0x80, 'a'})
	if got != "\uFFFDa" {
		t.Errorf("Decode = %q, want %q", got, "\uFFFDa")
	}
}

func TestUtf8StreamDecoder_FlushEmptiesCarry(t *testing.T) {
	d := sse.NewUtf8StreamDecoder()
	d.Decode([]byte{0xe4, 0xb8})
	if !d.HasPendingBytes() {
		t.Fatal("carry should be set")
	}
	got := d.Flush()
	if got != "\uFFFD" {
		t.Errorf("Flush = %q, want %q", got, "\uFFFD")
	}
	if d.HasPendingBytes() {
		t.Error("HasPendingBytes = true after Flush, want false")
	}
}

func TestUtf8StreamDecoder_FlushEmptyReturnsEmpty(t *testing.T) {
	d := sse.NewUtf8StreamDecoder()
	if got := d.Flush(); got != "" {
		t.Errorf("Flush on empty = %q, want empty", got)
	}
}
