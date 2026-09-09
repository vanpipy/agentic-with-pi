package llm_test

import (
	"errors"
	"fmt"
	"net"
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
