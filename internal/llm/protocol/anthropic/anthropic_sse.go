package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol/sse"
)

const (
	sseInitialBuffer = 64 * 1024
	sseMaxLineBuffer = 4 * 1024 * 1024
	sseChanBuffer    = 64
)

func ParseAnthropicSSE(ctx context.Context, body io.Reader) (<-chan llm.StreamEvent, error) {
	if body == nil {
		return nil, fmt.Errorf("anthropic SSE: nil body reader")
	}
	if ctx == nil {
		return nil, fmt.Errorf("anthropic SSE: nil context")
	}

	state := newStreamState()

	events := make(chan llm.StreamEvent, sseChanBuffer)
	errCh := make(chan error, 1)
	go func() {
		defer close(events)
		scanAnthropicSSE(ctx, body, events, errCh, state)
	}()

	out := make(chan llm.StreamEvent, sseChanBuffer)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					return
				case out <- ev:
				}
			}
		}
	}()

	select {
	case err := <-errCh:
		if err != nil {
			closeErrCh(errCh)
			return nil, err
		}
	default:
	}

	return out, nil
}

func closeErrCh(ch chan error) {
	select {
	case <-ch:
	default:
	}
}

type blockInfo struct {
	kind string
	id   string
}

type streamState struct {
	blocks       map[int]blockInfo
	accInput     int
	accOutput    int
	accCacheRead int
	accCacheCrt  int
	hasInput     bool
	hasOutput    bool
	hasCacheRd   bool
	hasCacheCrt  bool
	usageEmitted bool
	utf8Decoder  *sse.Utf8StreamDecoder
}

func newStreamState() *streamState {
	return &streamState{
		blocks:      make(map[int]blockInfo),
		utf8Decoder: sse.NewUtf8StreamDecoder(),
	}
}

func (s *streamState) setBlock(index int, kind, id string) {
	s.blocks[index] = blockInfo{kind: kind, id: id}
}

func (s *streamState) getBlock(index int) (kind, id string, ok bool) {
	info, ok := s.blocks[index]
	if !ok {
		return "", "", false
	}
	return info.kind, info.id, true
}

func (s *streamState) delBlock(index int) {
	delete(s.blocks, index)
}

func (s *streamState) accumulateUsage(u struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	CacheReadTokens     int `json:"cache_read_input_tokens"`
	CacheCreationTokens int `json:"cache_creation_input_tokens"`
}) {
	if u.InputTokens != 0 {
		s.accInput = u.InputTokens
		s.hasInput = true
	}
	if u.OutputTokens != 0 {
		s.accOutput = u.OutputTokens
		s.hasOutput = true
	}
	if u.CacheReadTokens != 0 {
		s.accCacheRead = u.CacheReadTokens
		s.hasCacheRd = true
	}
	if u.CacheCreationTokens != 0 {
		s.accCacheCrt = u.CacheCreationTokens
		s.hasCacheCrt = true
	}
}

type sseField struct {
	name  string
	value []byte
}

func parseSSEField(line []byte) (sseField, bool) {
	if len(line) == 0 {
		return sseField{}, false
	}
	if line[0] == ':' {
		return sseField{}, false
	}
	idx := bytes.IndexByte(line, ':')
	if idx == -1 {
		return sseField{name: string(bytes.TrimSpace(line))}, true
	}
	name := string(line[:idx])
	val := line[idx+1:]
	val = bytes.TrimPrefix(val, []byte{' '})
	return sseField{name: name, value: val}, true
}

func scanAnthropicSSE(ctx context.Context, body io.Reader, out chan<- llm.StreamEvent, errCh chan<- error, state *streamState) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, sseInitialBuffer), sseMaxLineBuffer)

	var eventName string
	var dataBuf []byte

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			flushPendingEvent(ctx, eventName, dataBuf, out, state)
			eventName = ""
			dataBuf = dataBuf[:0]
			continue
		}
		field, ok := parseSSEField(line)
		if !ok {
			continue
		}
		switch field.name {
		case "event":
			eventName = state.utf8Decoder.Decode(field.value)
		case "data":
			decoded := state.utf8Decoder.Decode(field.value)
			if len(dataBuf) > 0 {
				dataBuf = append(dataBuf, '\n')
			}
			dataBuf = append(dataBuf, decoded...)
		}
	}

	// EOF or scanner error: flush any pending event so the last
	// message_stop / message_delta is not silently dropped. Some
	// Anthropic responses omit the trailing blank line, so the
	// in-loop flush never fires.
	flushPendingEvent(ctx, eventName, dataBuf, out, state)

	if err := scanner.Err(); err != nil {
		if ctx.Err() == nil {
			select {
			case errCh <- fmt.Errorf("anthropic SSE scanner: %w", err):
			default:
			}
		}
	}
}

// flushPendingEvent emits one parsed event from the buffered name + data.
// When the data field is several JSON objects concatenated without a
// separator (proxy stripped the SSE event terminator), each object is
// emitted individually; concatenated JSON that fails to parse emits an
// EventErr so the caller sees the corruption instead of silently dropping
// content.
func flushPendingEvent(ctx context.Context, eventName string, dataBuf []byte, out chan<- llm.StreamEvent, state *streamState) {
	if eventName == "" || len(dataBuf) == 0 {
		return
	}
	emit := func(ev llm.StreamEvent) bool {
		select {
		case <-ctx.Done():
			return false
		case out <- ev:
			return true
		}
	}
	payload := append([]byte(nil), dataBuf...)
	evs, parseErr := decodeAnthropicEvent(eventName, payload, state)
	if parseErr == nil {
		for _, ev := range evs {
			if ev == nil {
				continue
			}
			if !emit(ev) {
				return
			}
		}
		return
	}
	split := sse.SplitConcatenatedJSON(string(payload))
	if split == nil {
		if !emit(llm.EventErr{Err: fmt.Errorf("anthropic SSE decode %q: %w", eventName, parseErr)}) {
			return
		}
		return
	}
	for _, obj := range split.Objects {
		objEvs, objErr := decodeAnthropicEvent(eventName, []byte(obj), state)
		if objErr != nil {
			if !emit(llm.EventErr{Err: fmt.Errorf("anthropic SSE decode %q (split): %w", eventName, objErr)}) {
				return
			}
			continue
		}
		for _, ev := range objEvs {
			if ev == nil {
				continue
			}
			if !emit(ev) {
				return
			}
		}
	}
}

type anthropicBaseEvent struct {
	Type         string          `json:"type"`
	Index        int             `json:"index"`
	ContentBlock json.RawMessage `json:"content_block"`
	Delta        json.RawMessage `json:"delta"`
	Message      json.RawMessage `json:"message"`
	Usage        json.RawMessage `json:"usage"`
}

func decodeAnthropicEvent(eventName string, payload []byte, state *streamState) ([]llm.StreamEvent, error) {
	if eventName == "ping" {
		return nil, nil
	}

	var base anthropicBaseEvent
	if err := json.Unmarshal(payload, &base); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	switch eventName {
	case "message_start":
		return decodeMessageStart(payload, state)
	case "content_block_start":
		ev, err := decodeContentBlockStart(base, state)
		return wrapSingle(ev, err)
	case "content_block_delta":
		ev, err := decodeContentBlockDelta(base, state)
		return wrapSingle(ev, err)
	case "content_block_stop":
		ev, err := decodeContentBlockStop(base, state)
		return wrapSingle(ev, err)
	case "message_delta":
		return decodeMessageDelta(payload, state)
	case "message_stop":
		return nil, nil
	case "error":
		ev, err := decodeError(payload)
		return wrapSingle(ev, err)
	default:
		return nil, nil
	}
}

func wrapSingle(ev llm.StreamEvent, err error) ([]llm.StreamEvent, error) {
	if err != nil {
		return nil, err
	}
	if ev == nil {
		return nil, nil
	}
	return []llm.StreamEvent{ev}, nil
}

type antrhopicUsageFields struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	CacheReadTokens     int `json:"cache_read_input_tokens"`
	CacheCreationTokens int `json:"cache_creation_input_tokens"`
}

func decodeMessageStart(payload []byte, state *streamState) ([]llm.StreamEvent, error) {
	var top struct {
		Message struct {
			ID    string               `json:"id"`
			Usage antrhopicUsageFields `json:"usage"`
		} `json:"message"`
	}
	if err := json.Unmarshal(payload, &top); err != nil {
		return nil, err
	}
	state.accumulateUsage(top.Message.Usage)
	if top.Message.ID == "" {
		return nil, nil
	}
	return []llm.StreamEvent{llm.EventSessionID{ID: top.Message.ID}}, nil
}

func decodeContentBlockStart(base anthropicBaseEvent, state *streamState) (llm.StreamEvent, error) {
	if len(base.ContentBlock) == 0 {
		return nil, nil
	}
	var block struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(base.ContentBlock, &block); err != nil {
		return nil, err
	}
	switch block.Type {
	case "text":
		state.setBlock(base.Index, "text", "")
		return nil, nil
	case "thinking":
		state.setBlock(base.Index, "thinking", "")
		return llm.EventThinkingStart{}, nil
	case "tool_use":
		state.setBlock(base.Index, "tool_use", block.ID)
		return llm.EventToolStart{ID: block.ID, Name: block.Name}, nil
	default:
		return nil, nil
	}
}

func decodeContentBlockDelta(base anthropicBaseEvent, state *streamState) (llm.StreamEvent, error) {
	kind, id, ok := state.getBlock(base.Index)
	if !ok {
		return nil, nil
	}
	if len(base.Delta) == 0 {
		return nil, nil
	}
	var delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		Signature   string `json:"signature"`
		PartialJSON string `json:"partial_json"`
	}
	if err := json.Unmarshal(base.Delta, &delta); err != nil {
		return nil, err
	}
	switch kind {
	case "text":
		if delta.Type == "text_delta" {
			return llm.EventTextDelta{Text: delta.Text}, nil
		}
	case "thinking":
		switch delta.Type {
		case "thinking_delta":
			return llm.EventThinkingDelta{Text: delta.Thinking}, nil
		case "signature_delta":
			return llm.EventThinkingSignature{Signature: delta.Signature}, nil
		}
	case "tool_use":
		if delta.Type == "input_json_delta" {
			return llm.EventToolDelta{ID: id, JSON: delta.PartialJSON}, nil
		}
	}
	return nil, nil
}

func decodeContentBlockStop(base anthropicBaseEvent, state *streamState) (llm.StreamEvent, error) {
	kind, id, ok := state.getBlock(base.Index)
	state.delBlock(base.Index)
	if !ok {
		return nil, nil
	}
	switch kind {
	case "thinking":
		return llm.EventThinkingEnd{}, nil
	case "tool_use":
		return llm.EventToolEnd{ID: id}, nil
	}
	return nil, nil
}

func decodeMessageDelta(payload []byte, state *streamState) ([]llm.StreamEvent, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, err
	}
	out := make([]llm.StreamEvent, 0, 2)

	if usageRaw, ok := raw["usage"]; ok && len(usageRaw) > 0 {
		var usage antrhopicUsageFields
		if err := json.Unmarshal(usageRaw, &usage); err != nil {
			return nil, err
		}
		state.accumulateUsage(usage)
		if !state.usageEmitted && (state.hasInput || state.hasOutput || state.hasCacheRd || state.hasCacheCrt) {
			out = append(out, llm.EventUsage{
				InputTokens:         state.accInput,
				OutputTokens:        state.accOutput,
				CacheReadTokens:     state.accCacheRead,
				CacheCreationTokens: state.accCacheCrt,
			})
			state.usageEmitted = true
		}
	}

	if deltaRaw, ok := raw["delta"]; ok && len(deltaRaw) > 0 {
		var delta struct {
			StopReason string `json:"stop_reason"`
		}
		if err := json.Unmarshal(deltaRaw, &delta); err != nil {
			return nil, err
		}
		if delta.StopReason != "" {
			out = append(out, llm.EventFinish{Reason: mapStopReason(delta.StopReason)})
		}
	}
	return out, nil
}

func decodeError(payload []byte) (llm.StreamEvent, error) {
	var top struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &top); err != nil {
		return nil, err
	}
	return llm.EventErr{Err: fmt.Errorf("anthropic server error %s: %s", top.Error.Type, top.Error.Message)}, nil
}

func mapStopReason(s string) llm.FinishReason {
	switch s {
	case "end_turn", "stop_sequence":
		return llm.FinishReasonStop
	case "max_tokens":
		return llm.FinishReasonLength
	case "tool_use":
		return llm.FinishReasonToolUse
	case "refusal":
		return llm.FinishReasonContentFilter
	default:
		return llm.FinishReasonUnknown
	}
}
