package protocol

import (
	"context"
	"strconv"
	"time"
)

type Request struct {
	URL     string
	Method  string
	Headers map[string]string
	Body    []byte

	Timeout time.Duration
}

type HTTPError struct {
	StatusCode int
	Body       []byte
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	msg := "HTTP " + strconv.Itoa(e.StatusCode) + ": " + string(e.Body)
	if e.RetryAfter > 0 {
		msg += " (retry-after: " + e.RetryAfter.String() + ")"
	}
	return msg
}

type StreamItem struct {
	Data []byte
	Err  error
}

type Protocol interface {
	Send(ctx context.Context, req *Request) ([]byte, error)
	Stream(ctx context.Context, req *Request) (<-chan StreamItem, error)
}
