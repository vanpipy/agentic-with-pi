package sse

// SplitJSON is the result of SplitConcatenatedJSON.
type SplitJSON struct {
	// Objects holds complete brace-balanced JSON objects in stream order.
	Objects []string
	// TrailingPartial holds a final object whose braces never closed. The
	// caller should prepend it to the next event's payload rather than
	// dropping it.
	TrailingPartial string
}

// SplitConcatenatedJSON splits a payload that contains several JSON
// objects concatenated without a separator. Some proxies drop the SSE
// event separator, producing payloads like `{"id":"a"}{"id":"b"}` or
// `{"id":"a"}data: {"id":"b"}`; a normal json.Unmarshal rejects the
// entire blob with "trailing characters" and the caller would lose
// every object.
//
// Callers should attempt a normal parse first and only fall back to
// this on failure, so well-formed payloads keep their existing
// behavior.
//
// Returns nil when the payload is not a recoverable concatenation
// (zero or one complete object and no truncated trailing object),
// leaving the caller's original error intact.
func SplitConcatenatedJSON(payload string) *SplitJSON {
	var objects []string
	depth := 0
	start := -1
	inString := false
	escaped := false

	for i := 0; i < len(payload); i++ {
		b := payload[i]
		if inString {
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
			if depth == 0 && start >= 0 {
				objects = append(objects, payload[start:i+1])
				start = -1
			}
		}
	}

	var trailingPartial string
	if start >= 0 {
		trailingPartial = payload[start:]
	}

	if len(objects) < 2 && trailingPartial == "" {
		return nil
	}
	return &SplitJSON{Objects: objects, TrailingPartial: trailingPartial}
}
