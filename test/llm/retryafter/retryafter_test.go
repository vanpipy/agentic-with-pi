package retryafter_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/llm/retryafter"
)

func TestParseRetryAfterDeltaSeconds(t *testing.T) {
	got := retryafter.ParseRetryAfter("7", time.Unix(1_000_000, 0))
	if got != 7*time.Second {
		t.Fatalf("ParseRetryAfter(7) = %v, want 7s", got)
	}
}

func TestParseRetryAfterDeltaSecondsIsTrimmed(t *testing.T) {
	got := retryafter.ParseRetryAfter("  42  ", time.Unix(1_000_000, 0))
	if got != 42*time.Second {
		t.Fatalf("ParseRetryAfter trimmed = %v, want 42s", got)
	}
}

func TestParseRetryAfterFutureHTTPDateReturnsDelta(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	retryAt := now.Add(12 * time.Second)
	value := retryAt.Format(time.RFC1123)
	got := retryafter.ParseRetryAfter(value, now)
	if got < 11*time.Second || got > 13*time.Second {
		t.Fatalf("ParseRetryAfter(date+12s) = %v, want ~12s", got)
	}
}

func TestParseRetryAfterFutureHTTPDateGMTFormat(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	retryAt := now.Add(30 * time.Second)
	value := retryAt.Format("Mon, 02 Jan 2006 15:04:05 GMT")
	got := retryafter.ParseRetryAfter(value, now)
	if got < 29*time.Second || got > 31*time.Second {
		t.Fatalf("ParseRetryAfter(GMT+30s) = %v, want ~30s", got)
	}
}

func TestParseRetryAfterPastHTTPDateReturnsZero(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	past := now.Add(-30 * time.Second)
	value := past.Format(time.RFC1123)
	got := retryafter.ParseRetryAfter(value, now)
	if got != 0 {
		t.Fatalf("ParseRetryAfter(past) = %v, want 0", got)
	}
}

func TestParseRetryAfterFarFutureHTTPDateIsCapped(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	far := now.Add(3 * time.Hour)
	value := far.Format(time.RFC1123)
	got := retryafter.ParseRetryAfter(value, now)
	if got != retryafter.MaxRetryAfter {
		t.Fatalf("ParseRetryAfter(far future) = %v, want %v", got, retryafter.MaxRetryAfter)
	}
}

func TestParseRetryAfterLargeSecondsIsCapped(t *testing.T) {
	got := retryafter.ParseRetryAfter("9999999999", time.Unix(1_000_000, 0))
	if got != retryafter.MaxRetryAfter {
		t.Fatalf("ParseRetryAfter(9999999999) = %v, want %v", got, retryafter.MaxRetryAfter)
	}
}

func TestParseRetryAfterOverflowingDigitsAreCapped(t *testing.T) {
	value := strings.Repeat("9", 200)
	got := retryafter.ParseRetryAfter(value, time.Unix(1_000_000, 0))
	if got != retryafter.MaxRetryAfter {
		t.Fatalf("ParseRetryAfter(%d nines) = %v, want %v", len(value), got, retryafter.MaxRetryAfter)
	}
}

func TestParseRetryAfterMalformedReturnsZero(t *testing.T) {
	got := retryafter.ParseRetryAfter("not-a-delay", time.Unix(1_000_000, 0))
	if got != 0 {
		t.Fatalf("ParseRetryAfter(malformed) = %v, want 0", got)
	}
}

func TestParseRetryAfterEmptyReturnsZero(t *testing.T) {
	got := retryafter.ParseRetryAfter("", time.Unix(1_000_000, 0))
	if got != 0 {
		t.Fatalf("ParseRetryAfter(empty) = %v, want 0", got)
	}
}

func TestParseRetryAfterMixedAlphaIsDigitTimesout(t *testing.T) {
	got := retryafter.ParseRetryAfter("12abc", time.Unix(1_000_000, 0))
	if got != 0 {
		t.Fatalf("ParseRetryAfter(mixed) = %v, want 0", got)
	}
}

func TestParseRetryAfterFromHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "5")
	got := retryafter.ParseRetryAfterFromHeaders(h)
	if got != 5*time.Second {
		t.Fatalf("ParseRetryAfterFromHeaders(5) = %v, want 5s", got)
	}
}

func TestParseRetryAfterFromHeadersMissing(t *testing.T) {
	got := retryafter.ParseRetryAfterFromHeaders(http.Header{})
	if got != 0 {
		t.Fatalf("ParseRetryAfterFromHeaders(missing) = %v, want 0", got)
	}
}

func TestParseRetryAfterFromHeadersNil(t *testing.T) {
	got := retryafter.ParseRetryAfterFromHeaders(nil)
	if got != 0 {
		t.Fatalf("ParseRetryAfterFromHeaders(nil) = %v, want 0", got)
	}
}

func TestRetryAfterRemainingWithinBound(t *testing.T) {
	ra := retryafter.NewRetryAfter(5 * time.Second)
	got := ra.Remaining()
	if got < 4*time.Second || got > 5*time.Second {
		t.Fatalf("Remaining() = %v, want ~5s", got)
	}
}

func TestRetryAfterRemainingElapsedIsZero(t *testing.T) {
	ra := retryafter.NewRetryAfter(-time.Second)
	if got := ra.Remaining(); got != 0 {
		t.Fatalf("Remaining() on elapsed = %v, want 0", got)
	}
}

func TestWithRetryAfterPreservesMessage(t *testing.T) {
	base := errors.New("rate limited")
	wrapped := retryafter.WithRetryAfter(base, 9*time.Second)
	if wrapped.Error() != "rate limited" {
		t.Fatalf("Error() = %q, want %q", wrapped.Error(), "rate limited")
	}
	got := retryafter.RetryAfterFromError(wrapped)
	if got < 8*time.Second || got > 9*time.Second {
		t.Fatalf("RetryAfterFromError = %v, want ~9s", got)
	}
}

func TestWithRetryAfterZeroHintReturnsBase(t *testing.T) {
	base := errors.New("transient")
	got := retryafter.WithRetryAfter(base, 0)
	if got != base {
		t.Fatalf("WithRetryAfter(base, 0) = %v, want base unchanged", got)
	}
}

func TestWithRetryAfterNilBaseReturnsNil(t *testing.T) {
	if got := retryafter.WithRetryAfter(nil, 5*time.Second); got != nil {
		t.Fatalf("WithRetryAfter(nil, 5s) = %v, want nil", got)
	}
}

func TestWithRetryAfterOversizedHintIsCapped(t *testing.T) {
	base := errors.New("rate limited")
	wrapped := retryafter.WithRetryAfter(base, 999*time.Hour)
	got := retryafter.RetryAfterFromError(wrapped)
	if got > retryafter.MaxRetryAfter {
		t.Fatalf("RetryAfterFromError with oversized hint = %v, want <= %v", got, retryafter.MaxRetryAfter)
	}
}

func TestRetryAfterErrorElapsedReturnsZero(t *testing.T) {
	wrapped := retryafter.WithRetryAfterDeadline(errors.New("rate limited"), time.Now().Add(-time.Second))
	got := retryafter.RetryAfterFromError(wrapped)
	if got != 0 {
		t.Fatalf("RetryAfterFromError(elapsed) = %v, want 0", got)
	}
}

func TestWithRetryAfterDeadlineNilBaseReturnsNil(t *testing.T) {
	if got := retryafter.WithRetryAfterDeadline(nil, time.Now()); got != nil {
		t.Fatalf("WithRetryAfterDeadline(nil) = %v, want nil", got)
	}
}

func TestRetryAfterFromErrorNoHint(t *testing.T) {
	err := errors.New("plain error")
	if got := retryafter.RetryAfterFromError(err); got != 0 {
		t.Fatalf("RetryAfterFromError(plain) = %v, want 0", got)
	}
}

func TestRetryAfterFromErrorNil(t *testing.T) {
	if got := retryafter.RetryAfterFromError(nil); got != 0 {
		t.Fatalf("RetryAfterFromError(nil) = %v, want 0", got)
	}
}

func TestRetryAfterFromErrorUnwrapsThroughCause(t *testing.T) {
	wrapped := retryafter.WithRetryAfter(fmt.Errorf("inner: %w", errors.New("core")), 7*time.Second)
	got := retryafter.RetryAfterFromError(wrapped)
	if got < 6*time.Second || got > 7*time.Second {
		t.Fatalf("RetryAfterFromError(double-wrapped) = %v, want ~7s", got)
	}
}

func TestRetryDelayServerHintWins(t *testing.T) {
	got := retryafter.RetryDelay(3, 1000, 4*time.Second)
	if got != 4*time.Second {
		t.Fatalf("RetryDelay with hint = %v, want 4s", got)
	}
}

func TestRetryDelayHintCappedAtMax(t *testing.T) {
	got := retryafter.RetryDelay(1, 1000, 2*time.Hour)
	if got != retryafter.MaxRetryAfter {
		t.Fatalf("RetryDelay oversized hint = %v, want %v", got, retryafter.MaxRetryAfter)
	}
}

func TestRetryDelayBackoffDoublesPerAttempt(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{5, 1600 * time.Millisecond},
	}
	for _, c := range cases {
		got := retryafter.RetryDelay(c.attempt, 100, 0)
		if got != c.want {
			t.Errorf("RetryDelay(%d) = %v, want %v", c.attempt, got, c.want)
		}
	}
}

func TestRetryDelayBackoffCappedAtMax(t *testing.T) {
	got := retryafter.RetryDelay(60, 1000, 0)
	if got != retryafter.MaxRetryAfter {
		t.Fatalf("RetryDelay(60, 1000) = %v, want %v", got, retryafter.MaxRetryAfter)
	}
}

func TestRetryDelayBackoffOverflowCappedAtMax(t *testing.T) {
	got := retryafter.RetryDelay(31, 10_000_000_000, 0)
	if got != retryafter.MaxRetryAfter {
		t.Fatalf("RetryDelay(31, overflow) = %v, want %v", got, retryafter.MaxRetryAfter)
	}
}

func TestRetryDelayZeroBaseReturnsZero(t *testing.T) {
	got := retryafter.RetryDelay(3, 0, 0)
	if got != 0 {
		t.Fatalf("RetryDelay(0 base) = %v, want 0", got)
	}
}

func TestRetryDelayNegativeAttemptClamps(t *testing.T) {
	got := retryafter.RetryDelay(-5, 100, 0)
	if got != 100*time.Millisecond {
		t.Fatalf("RetryDelay(-5) = %v, want 100ms", got)
	}
}

func TestRetryAfterErrorUnwrapsToBase(t *testing.T) {
	base := errors.New("inner")
	wrapped := retryafter.WithRetryAfter(base, 5*time.Second)
	if !errors.Is(wrapped, base) {
		t.Fatalf("errors.Is(wrapped, base) = false, want true (chain should reach base)")
	}
}

func TestRetryAfterErrorUnwrap(t *testing.T) {
	base := errors.New("inner")
	wrapped := retryafter.WithRetryAfter(base, 5*time.Second)
	rae, ok := wrapped.(interface{ Unwrap() error })
	if !ok {
		t.Fatalf("wrapped does not implement Unwrap")
	}
	if rae.Unwrap() != base {
		t.Fatalf("Unwrap() = %v, want base", rae.Unwrap())
	}
}
