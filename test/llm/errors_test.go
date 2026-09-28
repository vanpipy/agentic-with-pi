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

func TestIsTransientTransportError(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		// connection-level
		{"connection_reset_by_peer", "read tcp 127.0.0.1:443: connection reset by peer", true},
		{"connection_reset_short", "connection reset", true},
		{"connection_closed", "connection closed before message length could be read", true},
		{"connection_refused", "dial tcp 1.2.3.4:443: connect: connection refused", true},
		{"connection_aborted", "wsasend: An existing connection was forcibly closed by the remote host. (connection aborted)", true},
		{"broken_pipe", "write tcp 127.0.0.1:443: broken pipe", true},
		// timeout
		{"timed_out", "Post https://api.example.com/v1: net/http: request timed out", true},
		{"timeout", "i/o timeout", true},
		{"operation_timed_out", "operation timed out after 30s", true},
		// read/decoding
		{"error_decoding_message", "http: response.header: error decoding message", true},
		{"error_reading", "http: error reading response body", true},
		{"unexpected_eof", "Post https://api.example.com: unexpected EOF", true},
		// TLS
		{"close_notify", "tls: received unexpected close_notify alert prior to receiving peer's close_notify", true},
		{"peer_closed_connection_no_tls_close_notify", "peer closed connection without sending TLS close_notify", true},
		{"tls_handshake_eof", "tls handshake eof", true},
		{"badrecordmac_low", "badrecordmac", true},
		{"badrecordmac_mixed", "BadRecordMac", true},
		{"bad_record_mac_underscore", "bad_record_mac", true},
		{"fatal_alert_badrecordmac", "tls: received fatal alert: badrecordmac", true},
		{"fatal_alert_bad_record_mac", "tls: received fatal alert: bad_record_mac", true},
		{"received_fatal_alert_badrecordmac", "remote error: received fatal alert: badrecordmac", true},
		{"decryption_failed_or_bad_record_mac", "tls: decryption failed or bad record mac", true},
		// reqwest/hyper
		{"client_error_connect", "error trying to connect: client error (Connect)", true},
		{"client_error_connect_full", "request error: client error (Connect)", true},
		{"connection_error_grpc", "rpc error: code = Unavailable desc = connection error: desc = \"transport: error while dialing: dial tcp 1.2.3.4:443: connect: connection refused\"", true},
		{"connection_error_short", "connection error", true},
		{"incomplete_message", "http: response body: unexpected EOF — incomplete message", true},
		{"request_or_response_body_error", "request or response body error", true},
		// DNS
		{"temporary_failure_in_name_resolution", "dial tcp: lookup api.example.com: temporary failure in name resolution", true},
		{"failed_to_lookup_address_information", "getaddrinfo: failed to lookup address information", true},
		{"dns_error", "dns error: no such host", true},
		{"name_or_service_not_known", "getaddrinfo WSAERROR: Name or service not known", true},
		// routing
		{"no_route_to_host", "dial tcp 10.0.0.1:443: connect: no route to host", true},
		{"network_is_unreachable", "dial tcp 192.168.0.1:80: connect: network is unreachable", true},
		{"host_is_unreachable", "dial tcp 192.168.0.1:80: connect: host is unreachable", true},
		// HTTP/2
		{"http2_error", "http2 error: stream closed", true},
		{"stream_error", "http2: received unexpected RST_STREAM frame: stream error received: unspecific protocol error detected", true},
		{"stream_read_error", "http2: stream_read_error: PROTOCOL_ERROR", true},
		{"protocol_error", "http2: protocol error: frame size exceeded", true},
		{"refused_stream", "http2: RST_STREAM frame received: REFUSED_STREAM", true},
		{"refused_stream_underscore", "refused_stream", true},
		{"refused_stream_space", "refused stream", true},
		{"enhance_your_calm", "http2: received GOAWAY frame with error code ENHANCE_YOUR_CALM", true},
		{"enhance_your_calm_low", "enhance_your_calm", true},
		{"goaway", "http2: received GOAWAY frame", true},
		{"goaway_low", "server sent goaway", true},
		{"go_away_space", "the server told us to go away", true},
		{"sendrequest", "http2: SendRequest error: stream error", true},
		// case insensitivity
		{"uppercase_connection_reset", "CONNECTION RESET BY PEER", true},
		{"uppercase_broken_pipe", "BROKEN PIPE", true},
		{"mixed_case_timeout", "Operation Timed Out After 5 Seconds", true},
		{"wrapped_connection_reset", "Post https://api.x.example.com: read tcp 10.0.0.1:443: read: connection reset by peer", true},
		{"http2_goaway_proto_error", "http2: received GOAWAY frame with error code PROTOCOL_ERROR", true},
		{"http2_internal_error", "http2: received GOAWAY frame with error code INTERNAL_ERROR", true},
		{"client_error_connect_uppercase", "Client error (Connect)", true},
		{"client_error_connecting", "error trying to connect: error connecting: connection refused", true},
		{"context_deadline_with_transport", "context deadline exceeded by caller", false},
		{"read_eof_lower", "read eof", false},
		// negative — auth
		{"invalid_api_key", "invalid api key", false},
		{"authentication_failed", "authentication failed", false},
		// negative — context cancellation
		{"context_canceled", "context canceled", false},
		{"context_deadline_exceeded", "context deadline exceeded", false},
		// negative — generic errors that should NOT match
		{"empty_string", "", false},
		{"plain_hello", "hello world", false},
		{"json_parse_error", "json: cannot unmarshal string into int", false},
		{"nil_underlying_message", "operation failed", false},
		{"status_404_message_only", "Not Found", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var in error
			if tc.msg != "" {
				in = errors.New(tc.msg)
			}
			if got := llm.IsTransientTransportError(in); got != tc.want {
				t.Errorf("IsTransientTransportError(%q) = %v, want %v", tc.msg, got, tc.want)
			}
		})
	}
}

func TestIsTransientTransportErrorNil(t *testing.T) {
	if llm.IsTransientTransportError(nil) {
		t.Error("IsTransientTransportError(nil) = true, want false")
	}
}

func TestIsTransientTransportErrorWrapped(t *testing.T) {
	inner := errors.New("connection reset by peer")
	wrapped := fmt.Errorf("transport: %w", inner)
	if !llm.IsTransientTransportError(wrapped) {
		t.Error("IsTransientTransportError(wrapped) = false, want true (transient message bubbles up)")
	}
}

func TestClassifyTransientMessageUpgradesToNetwork(t *testing.T) {
	cases := []string{
		"connection reset by peer",
		"broken pipe",
		"tls: received fatal alert: badrecordmac",
		"http2: stream error received: unspecific protocol error detected",
		"peer closed connection without sending TLS close_notify",
		"temporary failure in name resolution",
		"enhance_your_calm",
		"operation timed out after 30s",
	}
	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			err := errors.New(msg)
			got := llm.Classify(err)
			if got == nil {
				t.Fatalf("Classify(%q) = nil", msg)
			}
			if got.Kind != llm.ErrorKindNetwork {
				t.Errorf("Classify(%q).Kind = %v, want %v", msg, got.Kind, llm.ErrorKindNetwork)
			}
		})
	}
}

func TestClassifyPlainErrorStillUnknownWhenNonTransient(t *testing.T) {
	if got := llm.Classify(errors.New("invalid api key")); got == nil || got.Kind != llm.ErrorKindUnknown {
		t.Errorf("non-transient plain error should remain Unknown, got %+v", got)
	}
	if got := llm.Classify(errors.New("context canceled")); got == nil || got.Kind != llm.ErrorKindUnknown {
		t.Errorf("context canceled should remain Unknown, got %+v", got)
	}
}

func TestClassifyTransientDoesNotOverrideTyped(t *testing.T) {
	typed := &llm.Error{Kind: llm.ErrorKindAuth, Code: 401, Message: "unauthorized"}
	if got := llm.Classify(typed); got != typed {
		t.Errorf("typed *llm.Error should pass through unchanged, got %+v", got)
	}
	httpErr := &protocol.HTTPError{StatusCode: 500, Body: []byte("boom")}
	got := llm.Classify(httpErr)
	if got == nil || got.Kind != llm.ErrorKindServer {
		t.Errorf("HTTPError should classify by status, got %+v", got)
	}
}

func TestIsRetryableTransientMessage(t *testing.T) {
	cases := []string{
		"connection reset by peer",
		"broken pipe",
		"http2 error: stream closed",
		"temporary failure in name resolution",
	}
	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			if !llm.IsRetryable(errors.New(msg)) {
				t.Errorf("IsRetryable(%q) = false, want true", msg)
			}
		})
	}
}
