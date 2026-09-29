package retryafter

import (
	"errors"
	"math"
	"net/http"
	"strings"
	"time"
)

const MaxRetryAfter = 60 * time.Second

var httpDateLayouts = [...]string{
	time.RFC1123,
	time.RFC850,
}

func ParseRetryAfter(value string, now time.Time) time.Duration {
	v := strings.TrimSpace(value)
	if v == "" {
		return 0
	}
	if isAllDigits(v) {
		return clampSeconds(parseDigitSeconds(v))
	}
	t, ok := parseHTTPDate(v)
	if !ok {
		return 0
	}
	delta := t.Sub(now)
	if delta <= 0 {
		return 0
	}
	return clampDuration(delta)
}

func ParseRetryAfterFromHeaders(h http.Header) time.Duration {
	if h == nil {
		return 0
	}
	return ParseRetryAfter(h.Get("Retry-After"), time.Now())
}

func isAllDigits(v string) bool {
	if v == "" {
		return false
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func parseDigitSeconds(v string) time.Duration {
	maxSecs := int64(MaxRetryAfter / time.Second)
	var secs int64
	for i := 0; i < len(v); i++ {
		secs = secs*10 + int64(v[i]-'0')
		if secs > maxSecs {
			return MaxRetryAfter
		}
	}
	return time.Duration(secs) * time.Second
}

func clampSeconds(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	if d > MaxRetryAfter {
		return MaxRetryAfter
	}
	return d
}

func clampDuration(d time.Duration) time.Duration {
	if d > MaxRetryAfter {
		return MaxRetryAfter
	}
	return d
}

func parseHTTPDate(value string) (time.Time, bool) {
	for _, layout := range httpDateLayouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

type RetryAfter struct {
	deadline time.Time
}

func NewRetryAfter(delay time.Duration) RetryAfter {
	return RetryAfter{deadline: time.Now().Add(delay)}
}

func (r RetryAfter) Remaining() time.Duration {
	d := r.deadline.Sub(time.Now())
	return max(d, 0)
}

type RetryAfterError struct {
	base     error
	deadline time.Time
}

func (e *RetryAfterError) Error() string {
	return e.base.Error()
}

func (e *RetryAfterError) Unwrap() error {
	return e.base
}

func WithRetryAfter(base error, hint time.Duration) error {
	if hint <= 0 || base == nil {
		return base
	}
	if hint > MaxRetryAfter {
		hint = MaxRetryAfter
	}
	return &RetryAfterError{base: base, deadline: time.Now().Add(hint)}
}

func WithRetryAfterDeadline(base error, deadline time.Time) error {
	if base == nil {
		return nil
	}
	return &RetryAfterError{base: base, deadline: deadline}
}

func RetryAfterFromError(err error) time.Duration {
	if err == nil {
		return 0
	}
	var rae *RetryAfterError
	if errors.As(err, &rae) {
		d := rae.deadline.Sub(time.Now())
		if d <= 0 {
			return 0
		}
		return d
	}
	return 0
}

func RetryDelay(attempt int, baseMs int64, hint time.Duration) time.Duration {
	if hint > 0 {
		if hint > MaxRetryAfter {
			return MaxRetryAfter
		}
		return hint
	}
	if baseMs <= 0 {
		return 0
	}
	if attempt < 1 {
		attempt = 1
	}
	shift := attempt - 1
	if shift > 30 {
		shift = 30
	}
	mult := int64(1) << uint(shift)
	if baseMs > math.MaxInt64/mult {
		return MaxRetryAfter
	}
	d := time.Duration(baseMs*mult) * time.Millisecond
	if d > MaxRetryAfter {
		return MaxRetryAfter
	}
	return d
}
