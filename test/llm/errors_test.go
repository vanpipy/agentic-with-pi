package llm_test

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol"
)

func TestClassifyHTTPStatus(t *testing.T) {
	cases := []struct {
		name string
		code int
		want llm.ErrorKind
	}{
		{"401", 401, llm.ErrorKindAuth},
		{"403", 403, llm.ErrorKindAuth},
		{"429", 429, llm.ErrorKindRateLimit},
		{"400", 400, llm.ErrorKindClient},
		{"404", 404, llm.ErrorKindClient},
		{"500", 500, llm.ErrorKindServer},
		{"502", 502, llm.ErrorKindServer},
		{"503", 503, llm.ErrorKindServer},
		{"200", 200, llm.ErrorKindUnknown},
		{"301", 301, llm.ErrorKindUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := fmt.Errorf("send: %w", &protocol.HTTPError{StatusCode: tc.code, Body: []byte("body")})
			got := llm.Classify(err)
			if got == nil {
				t.Fatal("Classify returned nil")
			}
			if got.Kind != tc.want {
				t.Errorf("Kind = %v, want %v", got.Kind, tc.want)
			}
		})
	}
}

func TestClassifyNetworkError(t *testing.T) {
	netErr := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	got := llm.Classify(netErr)
	if got == nil || got.Kind != llm.ErrorKindNetwork {
		t.Errorf("Kind = %v, want %v", got, llm.ErrorKindNetwork)
	}
}

func TestClassifyUnwrapsLLMError(t *testing.T) {
	orig := &llm.Error{Kind: llm.ErrorKindAuth, Code: 401, Message: "unauthorized"}
	wrapped := fmt.Errorf("outer: %w", orig)
	got := llm.Classify(wrapped)
	if got != orig {
		t.Errorf("Classify did not return original *llm.Error: got %p, want %p", got, orig)
	}
}

func TestClassifyNilAndUnknown(t *testing.T) {
	if got := llm.Classify(nil); got != nil {
		t.Errorf("Classify(nil) = %v, want nil", got)
	}
	if got := llm.Classify(errors.New("random")); got == nil || got.Kind != llm.ErrorKindUnknown {
		t.Errorf("Classify(random) = %v, want Unknown", got)
	}
}

func TestIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"rate_limit", &llm.Error{Kind: llm.ErrorKindRateLimit}, true},
		{"server", &llm.Error{Kind: llm.ErrorKindServer}, true},
		{"network", &llm.Error{Kind: llm.ErrorKindNetwork}, true},
		{"auth", &llm.Error{Kind: llm.ErrorKindAuth}, false},
		{"client", &llm.Error{Kind: llm.ErrorKindClient}, false},
		{"vendor", &llm.Error{Kind: llm.ErrorKindVendor}, false},
		{"unknown", &llm.Error{Kind: llm.ErrorKindUnknown}, false},
		{"plain", errors.New("x"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := llm.IsRetryable(tc.err); got != tc.want {
				t.Errorf("IsRetryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestErrorKindHelpers(t *testing.T) {
	if !llm.IsAuth(&llm.Error{Kind: llm.ErrorKindAuth}) {
		t.Error("IsAuth(Auth) = false")
	}
	if llm.IsAuth(&llm.Error{Kind: llm.ErrorKindRateLimit}) {
		t.Error("IsAuth(RateLimit) = true")
	}
	if !llm.IsRateLimit(&llm.Error{Kind: llm.ErrorKindRateLimit}) {
		t.Error("IsRateLimit(RateLimit) = false")
	}
	if llm.IsRateLimit(&llm.Error{Kind: llm.ErrorKindAuth}) {
		t.Error("IsRateLimit(Auth) = true")
	}
}

func TestErrorKindStringAllKinds(t *testing.T) {
	cases := []struct {
		kind llm.ErrorKind
		want string
	}{
		{llm.ErrorKindUnknown, "unknown"},
		{llm.ErrorKindAuth, "auth"},
		{llm.ErrorKindRateLimit, "rate_limit"},
		{llm.ErrorKindClient, "client"},
		{llm.ErrorKindServer, "server"},
		{llm.ErrorKindNetwork, "network"},
		{llm.ErrorKindVendor, "vendor"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.kind.String(); got != tc.want {
				t.Errorf("ErrorKind(%d).String() = %q, want %q", int(tc.kind), got, tc.want)
			}
		})
	}
}

func TestErrorKindStringOutOfRange(t *testing.T) {
	got := llm.ErrorKind(999).String()
	if got != "unknown" {
		t.Errorf("out-of-range ErrorKind.String() = %q, want %q", got, "unknown")
	}
}

func TestIsAuthNilAndNonError(t *testing.T) {
	if llm.IsAuth(nil) {
		t.Error("IsAuth(nil) = true, want false")
	}
	if llm.IsAuth(errors.New("plain")) {
		t.Error("IsAuth(plain error) = true, want false (Classify yields Unknown)")
	}
	wrapped := fmt.Errorf("outer: %w", errors.New("inner"))
	if llm.IsAuth(wrapped) {
		t.Error("IsAuth(wrapped plain) = true, want false")
	}
}

func TestIsRateLimitNilAndNonError(t *testing.T) {
	if llm.IsRateLimit(nil) {
		t.Error("IsRateLimit(nil) = true, want false")
	}
	if llm.IsRateLimit(errors.New("plain")) {
		t.Error("IsRateLimit(plain error) = true, want false")
	}
	wrapped := fmt.Errorf("outer: %w", errors.New("inner"))
	if llm.IsRateLimit(wrapped) {
		t.Error("IsRateLimit(wrapped plain) = true, want false")
	}
}

func TestErrorErrorMessageShape(t *testing.T) {
	e := &llm.Error{Kind: llm.ErrorKindAuth, Message: "bad key"}
	got := e.Error()
	if !strings.Contains(got, "auth") {
		t.Errorf("Error() = %q, want contains kind", got)
	}
	if !strings.Contains(got, "bad key") {
		t.Errorf("Error() = %q, want contains message", got)
	}
}

func TestErrorUnwrap(t *testing.T) {
	inner := errors.New("inner cause")
	e := &llm.Error{Kind: llm.ErrorKindServer, Cause: inner}
	if got := errors.Unwrap(e); got != inner {
		t.Errorf("Unwrap = %v, want inner cause", got)
	}
}

func TestErrorErrorWithCauseDistinctFromMessage(t *testing.T) {
	inner := errors.New("inner cause distinct from message")
	e := &llm.Error{Kind: llm.ErrorKindServer, Message: "boom", Cause: inner}
	got := e.Error()
	if !strings.Contains(got, "boom") {
		t.Errorf("Error() = %q, want contains Message", got)
	}
	if !strings.Contains(got, "inner cause distinct from message") {
		t.Errorf("Error() = %q, want contains Cause", got)
	}
}

func TestErrorErrorWithCauseEqualMessage(t *testing.T) {
	inner := errors.New("same")
	e := &llm.Error{Kind: llm.ErrorKindServer, Message: "same", Cause: inner}
	got := e.Error()
	if strings.Count(got, "same") != 1 {
		t.Errorf("Error() = %q, want Cause suppressed when equal to Message", got)
	}
}

func TestErrorErrorWithCode(t *testing.T) {
	e := &llm.Error{Kind: llm.ErrorKindServer, Code: 503, Message: "down"}
	got := e.Error()
	if !strings.Contains(got, "(503)") {
		t.Errorf("Error() = %q, want contains (503)", got)
	}
}
