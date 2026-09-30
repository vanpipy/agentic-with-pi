package sse

import (
	"strings"
	"unicode/utf8"
)

const utf8ReplacementChar = "\uFFFD"

// Utf8StreamDecoder reassembles UTF-8 text from an arbitrarily fragmented
// byte stream. It emits valid runes immediately, holds a truncated trailing
// rune until more bytes arrive, and substitutes U+FFFD for bytes that are
// genuinely invalid (not part of any valid UTF-8 sequence even when
// complete).
type Utf8StreamDecoder struct {
	carry []byte
}

func NewUtf8StreamDecoder() *Utf8StreamDecoder {
	return &Utf8StreamDecoder{}
}

// Decode consumes chunk and returns the longest valid UTF-8 prefix as a
// string. A truncated trailing rune is held internally and retried on the
// next call. Bytes that cannot be part of any valid UTF-8 sequence are
// replaced with U+FFFD.
func (d *Utf8StreamDecoder) Decode(chunk []byte) string {
	bytes := chunk
	if len(d.carry) > 0 {
		d.carry = append(d.carry, chunk...)
		bytes = d.carry
	}

	var out strings.Builder
	i := 0
	for i < len(bytes) {
		b := bytes[i]
		if b < 0x80 {
			out.WriteByte(b)
			i++
			continue
		}
		if isTruncatedStart(bytes[i:]) {
			break
		}
		r, size := utf8.DecodeRune(bytes[i:])
		if r == utf8.RuneError && size == 1 {
			out.WriteString(utf8ReplacementChar)
			i++
			continue
		}
		if size == 0 {
			break
		}
		out.Write(bytes[i : i+size])
		i += size
	}

	if i < len(bytes) {
		d.carry = append(d.carry[:0], bytes[i:]...)
	} else {
		d.carry = d.carry[:0]
	}
	return out.String()
}

// Flush returns any held trailing partial bytes as a single replacement
// character. Call when the stream is known to be terminated without
// further input; never leaves bytes unresolved.
func (d *Utf8StreamDecoder) Flush() string {
	if len(d.carry) == 0 {
		return ""
	}
	d.carry = nil
	return utf8ReplacementChar
}

// HasPendingBytes reports whether a truncated trailing rune is buffered.
func (d *Utf8StreamDecoder) HasPendingBytes() bool {
	return len(d.carry) > 0
}

func isTruncatedStart(rest []byte) bool {
	b := rest[0]
	if b < 0xC2 || b > 0xF4 {
		return false
	}
	var need int
	switch {
	case b < 0xE0:
		need = 2
	case b < 0xF0:
		need = 3
	default:
		need = 4
	}
	return len(rest) < need
}
