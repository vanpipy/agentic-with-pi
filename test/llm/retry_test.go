package llm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/llm"
)

type scriptedCore struct {
	scripts     [][]llm.StreamEvent
	connectErrs []error
	calls       int
}

func (s *scriptedCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	idx := s.calls
	s.calls++
	if idx < len(s.connectErrs) && s.connectErrs[idx] != nil {
		return nil, s.connectErrs[idx]
	}
	var events []llm.StreamEvent
	if idx < len(s.scripts) {
		events = s.scripts[idx]
	}
	ch := make(chan llm.StreamEvent, len(events)+1)
	for _, ev := range events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func collectStreamEvents(events <-chan llm.StreamEvent) []llm.StreamEvent {
	var out []llm.StreamEvent
	for ev := range events {
		out = append(out, ev)
	}
	return out
}

func TestRetryCoreMidStreamErrorRollbackAndRetry(t *testing.T) {
	inner := &scriptedCore{
		scripts: [][]llm.StreamEvent{
			{
				{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{Delta: llm.Message{Content: "part1"}}}}},
				{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{Delta: llm.Message{Content: "part2"}}}}},
				{Err: &llm.Error{Kind: llm.ErrorKindServer, Code: 503, Message: "transport blip"}},
			},
			{
				{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{Delta: llm.Message{Content: "fresh1"}}}}},
				{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{Delta: llm.Message{Content: "fresh2"}}}}},
			},
		},
	}
	rc := llm.NewRetryCore(inner, fastConfig(3))
	events, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err != nil {
		t.Fatalf("unexpected connect err: %v", err)
	}

	got := collectStreamEvents(events)
	if inner.calls != 2 {
		t.Errorf("calls = %d, want 2", inner.calls)
	}

	var rollbackCount int
	var firstErr error
	var chunkCount int
	for _, ev := range got {
		if ev.Rollback {
			rollbackCount++
		}
		if ev.Err != nil && firstErr == nil {
			firstErr = ev.Err
		}
		if ev.Chunk != nil {
			chunkCount++
		}
	}
	if firstErr != nil {
		t.Fatalf("unexpected terminal err: %v", firstErr)
	}
	if rollbackCount != 1 {
		t.Errorf("rollbackCount = %d, want 1 (one rollback per mid-stream retry)", rollbackCount)
	}
	if chunkCount != 4 {
		t.Errorf("chunkCount = %d, want 4 (2 partial + 2 fresh)", chunkCount)
	}

	rollbackIdx := -1
	for i, ev := range got {
		if ev.Rollback {
			rollbackIdx = i
			break
		}
	}
	if rollbackIdx < 0 {
		t.Fatal("no Rollback event observed")
	}
	if got[0].Rollback {
		t.Errorf("first event must never be Rollback; got %+v", got[0])
	}
	if rollbackIdx == 0 {
		t.Errorf("Rollback must come after at least one Chunk; got index 0")
	}
}

func TestRetryCoreMidStreamErrorNoRetryIfNotRetryable(t *testing.T) {
	inner := &scriptedCore{
		scripts: [][]llm.StreamEvent{
			{
				{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{Delta: llm.Message{Content: "x"}}}}},
				{Err: &llm.Error{Kind: llm.ErrorKindAuth, Code: 401, Message: "bad key"}},
			},
		},
	}
	rc := llm.NewRetryCore(inner, fastConfig(3))
	events, _ := rc.StreamChat(context.Background(), &llm.ChatRequest{})

	got := collectStreamEvents(events)
	if inner.calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry for non-retryable mid-stream err)", inner.calls)
	}

	var rollbackCount int
	var firstErr error
	for _, ev := range got {
		if ev.Rollback {
			rollbackCount++
		}
		if ev.Err != nil && firstErr == nil {
			firstErr = ev.Err
		}
	}
	if rollbackCount != 0 {
		t.Errorf("rollbackCount = %d, want 0 (non-retryable must not emit Rollback)", rollbackCount)
	}
	if firstErr == nil {
		t.Fatal("expected Err to surface")
	}
}

func TestRetryCoreMidStreamErrorMaxRetriesExceeded(t *testing.T) {
	transient := &llm.Error{Kind: llm.ErrorKindNetwork, Code: 0, Message: "tls bad record"}
	scripts := make([][]llm.StreamEvent, 4)
	for i := range scripts {
		scripts[i] = []llm.StreamEvent{
			{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{Delta: llm.Message{Content: "p"}}}}},
			{Err: transient},
		}
	}
	inner := &scriptedCore{scripts: scripts}
	rc := llm.NewRetryCore(inner, fastConfig(3))
	events, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err != nil {
		t.Fatalf("unexpected connect err: %v", err)
	}

	got := collectStreamEvents(events)
	if inner.calls != 4 {
		t.Errorf("calls = %d, want 4 (1 initial + 3 retries)", inner.calls)
	}

	var rollbackCount int
	var lastEvent llm.StreamEvent
	for _, ev := range got {
		if ev.Rollback {
			rollbackCount++
		}
		lastEvent = ev
	}
	if rollbackCount != 3 {
		t.Errorf("rollbackCount = %d, want 3 (one per failed attempt except final)", rollbackCount)
	}
	if lastEvent.Err == nil {
		t.Errorf("last event must be Err; got %+v", lastEvent)
	}
	if lastEvent.Rollback {
		t.Errorf("last event must not be Rollback (final attempt gave up); got %+v", lastEvent)
	}
}

func TestRetryCoreCleanStreamNoRollback(t *testing.T) {
	inner := &recordingCore{streamEvents: okStream("clean")}
	rc := llm.NewRetryCore(inner, fastConfig(3))
	events, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var rollbackCount int
	for ev := range events {
		if ev.Rollback {
			rollbackCount++
		}
	}
	if rollbackCount != 0 {
		t.Errorf("rollbackCount = %d, want 0 on clean stream", rollbackCount)
	}
	if inner.streamCalls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on clean stream)", inner.streamCalls)
	}
}

func TestRetryCoreRollbackOnlyAfterFirstEvent(t *testing.T) {
	inner := &scriptedCore{
		scripts: [][]llm.StreamEvent{
			{
				{Err: &llm.Error{Kind: llm.ErrorKindServer, Code: 503, Message: "boom on first event"}},
			},
		},
	}
	rc := llm.NewRetryCore(inner, fastConfig(3))
	events, _ := rc.StreamChat(context.Background(), &llm.ChatRequest{})

	got := collectStreamEvents(events)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1 (only the Err; first-event-err must not rollback)", len(got))
	}
	if got[0].Rollback {
		t.Errorf("first event was Rollback; should be the Err (count=1, nothing to discard)")
	}
	if got[0].Err == nil {
		t.Errorf("expected Err; got %+v", got[0])
	}
	if inner.calls != 1 {
		t.Errorf("calls = %d, want 1 (no rollback when nothing to discard)", inner.calls)
	}
}

type recordingCore struct {
	streamCalls  int
	streamEvents []llm.StreamEvent
	streamErr    error
}

func (r *recordingCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	r.streamCalls++
	if r.streamErr != nil {
		return nil, r.streamErr
	}
	ch := make(chan llm.StreamEvent, len(r.streamEvents))
	for _, ev := range r.streamEvents {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func fastConfig(maxRetries int) llm.RetryConfig {
	return llm.RetryConfig{
		MaxRetries:     maxRetries,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Multiplier:     2.0,
		Jitter:         0,
	}
}

func okStream(content string) []llm.StreamEvent {
	return []llm.StreamEvent{
		{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
			Delta: llm.Message{Content: content},
		}}}},
	}
}

func drain(events <-chan llm.StreamEvent) (firstErr error, totalContent string) {
	for ev := range events {
		if ev.Err != nil && firstErr == nil {
			firstErr = ev.Err
		}
		if ev.Chunk != nil {
			for _, c := range ev.Chunk.Choices {
				totalContent += c.Delta.Content
			}
		}
	}
	return
}

func TestRetryStreamForwardsEvents(t *testing.T) {
	inner := &recordingCore{streamEvents: okStream("hello")}
	rc := llm.NewRetryCore(inner, fastConfig(3))
	events, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	_, content := drain(events)
	if content != "hello" {
		t.Errorf("content = %q, want hello", content)
	}
	if inner.streamCalls != 1 {
		t.Errorf("calls = %d, want 1", inner.streamCalls)
	}
}

func TestRetryStreamPropagatesStreamError(t *testing.T) {
	inner := &recordingCore{
		streamEvents: []llm.StreamEvent{
			{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{Delta: llm.Message{Content: "x"}}}}},
			{Err: errors.New("mid-stream fail")},
		},
	}
	rc := llm.NewRetryCore(inner, fastConfig(3))
	events, _ := rc.StreamChat(context.Background(), &llm.ChatRequest{})

	firstErr, _ := drain(events)
	if firstErr == nil {
		t.Fatal("expected stream error")
	}
	if firstErr.Error() != "mid-stream fail" {
		t.Errorf("err = %v", firstErr)
	}
}

func TestDefaultRetryConfigHasThreeRetries(t *testing.T) {
	cfg := llm.DefaultRetryConfig()
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.InitialBackoff <= 0 {
		t.Errorf("InitialBackoff must be > 0")
	}
	if cfg.MaxBackoff < cfg.InitialBackoff {
		t.Errorf("MaxBackoff < InitialBackoff")
	}
}

func TestRetryCoreConstructorClampsNegative(t *testing.T) {
	cfg := fastConfig(0)
	cfg.MaxRetries = -1
	rc := llm.NewRetryCore(&recordingCore{}, cfg)
	if rc == nil {
		t.Fatal("constructor returned nil")
	}
}

type flakyCore struct {
	failures int
	calls    int
}

func (f *flakyCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	f.calls++
	if f.calls <= f.failures {
		return nil, &llm.Error{
			Kind:    llm.ErrorKindServer,
			Code:    529,
			Message: "overloaded_error",
		}
	}
	return okStreamChannel("recovered"), nil
}

func okStreamChannel(content string) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent, 1)
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		Delta: llm.Message{Content: content},
	}}}}
	close(ch)
	return ch
}

func TestRetryCoreRetriesOnTransientFailure(t *testing.T) {
	inner := &flakyCore{failures: 2}
	rc := llm.NewRetryCore(inner, fastConfig(3))
	events, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	_, content := drain(events)
	if content != "recovered" {
		t.Errorf("content = %q, want recovered", content)
	}
	if inner.calls != 3 {
		t.Errorf("calls = %d, want 3 (2 failures + 1 success)", inner.calls)
	}
}

func TestRetryCoreGivesUpAfterMaxRetries(t *testing.T) {
	inner := &flakyCore{failures: 10}
	rc := llm.NewRetryCore(inner, fastConfig(3))
	_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	if inner.calls != 4 {
		t.Errorf("calls = %d, want 4 (1 initial + 3 retries)", inner.calls)
	}
}

func TestRetryCoreDoesNotRetryNonRetryableError(t *testing.T) {
	inner := &recordingCore{streamErr: &llm.Error{
		Kind:    llm.ErrorKindAuth,
		Code:    401,
		Message: "invalid api key",
	}}
	rc := llm.NewRetryCore(inner, fastConfig(3))
	_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if inner.streamCalls != 1 {
		t.Errorf("calls = %d, want 1 (no retry for auth error)", inner.streamCalls)
	}
}

type timingCore struct {
	calls []time.Time
}

func (t *timingCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	t.calls = append(t.calls, time.Now())
	return okStreamChannel("ok"), nil
}

func TestRateLimitedCoreThrottlesBursts(t *testing.T) {
	inner := &timingCore{}
	cfg := llm.RateLimitConfig{RatePerSec: 5, Burst: 2}
	rc := llm.NewRateLimitedCore(inner, cfg)

	start := time.Now()
	for i := 0; i < 5; i++ {
		_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
		if err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)

	if len(inner.calls) != 5 {
		t.Fatalf("calls = %d, want 5", len(inner.calls))
	}
	if elapsed < 500*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 500ms (5 calls at 5/sec means burst 2 then 3 paced at 200ms each)", elapsed)
	}
}

func TestRateLimitedCoreBurstAllowed(t *testing.T) {
	inner := &timingCore{}
	cfg := llm.RateLimitConfig{RatePerSec: 10, Burst: 3}
	rc := llm.NewRateLimitedCore(inner, cfg)

	start := time.Now()
	for i := 0; i < 3; i++ {
		_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
		if err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("burst of 3 should be fast (took %v)", elapsed)
	}
}

func TestRateLimitedCoreRespectsContextCancel(t *testing.T) {
	inner := &timingCore{}
	cfg := llm.RateLimitConfig{RatePerSec: 1, Burst: 1}
	rc := llm.NewRateLimitedCore(inner, cfg)

	if _, err := rc.StreamChat(context.Background(), &llm.ChatRequest{}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := rc.StreamChat(ctx, &llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

type probeCore struct {
	calls int
}

func (p *probeCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	p.calls++
	return okStreamChannel("ok"), nil
}

func TestRateLimitedCoreZeroConfigUsesDefaults(t *testing.T) {
	inner := &probeCore{}
	rc := llm.NewRateLimitedCore(inner, llm.RateLimitConfig{})

	for i := 0; i < 3; i++ {
		_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}
	if inner.calls != 3 {
		t.Errorf("inner.calls = %d, want 3", inner.calls)
	}
}

func TestRateLimitedCoreNegativeConfigUsesDefaults(t *testing.T) {
	inner := &probeCore{}
	cfg := llm.RateLimitConfig{RatePerSec: -5, Burst: -10}
	rc := llm.NewRateLimitedCore(inner, cfg)

	for i := 0; i < 2; i++ {
		_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}
	if inner.calls != 2 {
		t.Errorf("inner.calls = %d, want 2", inner.calls)
	}
}

func TestRateLimitedCoreOnlyNegativeRateUsesDefaultBurst(t *testing.T) {
	inner := &probeCore{}
	cfg := llm.RateLimitConfig{RatePerSec: 0, Burst: 4}
	rc := llm.NewRateLimitedCore(inner, cfg)

	start := time.Now()
	for i := 0; i < 4; i++ {
		_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("burst of 4 with default rate should be fast (took %v)", elapsed)
	}
}
