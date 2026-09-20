package llm

import (
	"errors"
	"net"
	"strconv"

	"github.com/vanpiyp/awp/internal/llm/protocol"
)

type ErrorKind int

const (
	ErrorKindUnknown ErrorKind = iota
	ErrorKindAuth
	ErrorKindRateLimit
	ErrorKindClient
	ErrorKindServer
	ErrorKindNetwork
	ErrorKindVendor
)

func (k ErrorKind) String() string {
	switch k {
	case ErrorKindAuth:
		return "auth"
	case ErrorKindRateLimit:
		return "rate_limit"
	case ErrorKindClient:
		return "client"
	case ErrorKindServer:
		return "server"
	case ErrorKindNetwork:
		return "network"
	case ErrorKindVendor:
		return "vendor"
	default:
		return "unknown"
	}
}

type Error struct {
	Kind    ErrorKind
	Code    int
	Message string
	Cause   error
}

func (e *Error) Error() string {
	s := "llm " + e.Kind.String() + " error"
	if e.Code != 0 {
		s += " (" + strconv.Itoa(e.Code) + ")"
	}
	if e.Message != "" {
		s += ": " + e.Message
	}
	if e.Cause != nil && e.Cause.Error() != e.Message {
		s += ": " + e.Cause.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.Cause }

func (e *Error) Retryable() bool {
	switch e.Kind {
	case ErrorKindRateLimit, ErrorKindServer, ErrorKindNetwork:
		return true
	default:
		return false
	}
}

func Classify(err error) *Error {
	if err == nil {
		return nil
	}
	var llmErr *Error
	if errors.As(err, &llmErr) {
		return llmErr
	}
	var httpErr *protocol.HTTPError
	if errors.As(err, &httpErr) {
		return &Error{
			Kind:    classifyHTTPStatus(httpErr.StatusCode),
			Code:    httpErr.StatusCode,
			Message: string(httpErr.Body),
			Cause:   err,
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return &Error{Kind: ErrorKindNetwork, Cause: err}
	}
	return &Error{Kind: ErrorKindUnknown, Cause: err}
}

func classifyHTTPStatus(code int) ErrorKind {
	switch {
	case code == 401 || code == 403:
		return ErrorKindAuth
	case code == 429:
		return ErrorKindRateLimit
	case code >= 400 && code < 500:
		return ErrorKindClient
	case code >= 500:
		return ErrorKindServer
	default:
		return ErrorKindUnknown
	}
}

func IsRetryable(err error) bool {
	if e := Classify(err); e != nil {
		return e.Retryable()
	}
	return false
}

func IsAuth(err error) bool {
	if e := Classify(err); e != nil {
		return e.Kind == ErrorKindAuth
	}
	return false
}

func IsRateLimit(err error) bool {
	if e := Classify(err); e != nil {
		return e.Kind == ErrorKindRateLimit
	}
	return false
}
