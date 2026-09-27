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

func TestRetryableNetworkErrorKind(t *testing.T) {
	e := &llm.Error{Kind: llm.ErrorKindNetwork}
	if !e.Retryable() {
		t.Error("Network should be retryable")
	}
	if llm.IsRetryable(e) != true {
		t.Error("IsRetryable(Network) = false")
	}
}

func TestRetryableAllKinds(t *testing.T) {
	cases := []struct {
		kind llm.ErrorKind
		want bool
	}{
		{llm.ErrorKindUnknown, false},
		{llm.ErrorKindAuth, false},
		{llm.ErrorKindRateLimit, true},
		{llm.ErrorKindClient, false},
		{llm.ErrorKindServer, true},
		{llm.ErrorKindNetwork, true},
		{llm.ErrorKindVendor, false},
	}
	for _, tc := range cases {
		got := (&llm.Error{Kind: tc.kind}).Retryable()
		if got != tc.want {
			t.Errorf("ErrorKind(%v).Retryable() = %v, want %v", tc.kind, got, tc.want)
		}
	}
}

func TestErrorMessageWithoutCode(t *testing.T) {
	e := &llm.Error{Kind: llm.ErrorKindServer, Message: "down"}
	got := e.Error()
	if strings.Contains(got, "(0)") {
		t.Errorf("Error() = %q, should not contain (0) for Code=0", got)
	}
	if !strings.Contains(got, "down") {
		t.Errorf("Error() = %q, want it to mention Message", got)
	}
}

func TestErrorMessageWithoutMessageOrCause(t *testing.T) {
	e := &llm.Error{Kind: llm.ErrorKindServer}
	got := e.Error()
	if !strings.Contains(got, "server error") {
		t.Errorf("Error() = %q, want it to contain 'server error'", got)
	}
}

func TestErrorNilCause(t *testing.T) {
	e := &llm.Error{Kind: llm.ErrorKindServer}
	if err := e.Unwrap(); err != nil {
		t.Errorf("Unwrap() = %v, want nil", err)
	}
}

func TestClassifyEmptyErrorMessage(t *testing.T) {
	err := &protocol.HTTPError{StatusCode: 500, Body: nil}
	got := llm.Classify(err)
	if got == nil {
		t.Fatal("Classify returned nil")
	}
	if got.Kind != llm.ErrorKindServer {
		t.Errorf("Kind = %v, want Server", got.Kind)
	}
	if got.Message != "" {
		t.Errorf("Message = %q, want empty", got.Message)
	}
}

func TestClassifyStatus304Unknown(t *testing.T) {
	err := fmt.Errorf("upstream: %w", &protocol.HTTPError{StatusCode: 304, Body: []byte("not modified")})
	got := llm.Classify(err)
	if got == nil || got.Kind != llm.ErrorKindUnknown {
		t.Errorf("304 should be Unknown, got %+v", got)
	}
}

func TestClassifyNetErrorWrapped(t *testing.T) {
	netErr := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	wrapped := fmt.Errorf("transport: %w", netErr)
	got := llm.Classify(wrapped)
	if got == nil || got.Kind != llm.ErrorKindNetwork {
		t.Errorf("wrapped net error should yield Network, got %+v", got)
	}
	if !errors.Is(got.Cause, netErr) {
		t.Errorf("Cause should preserve original net error, got %v", got.Cause)
	}
}
