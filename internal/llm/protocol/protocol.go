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
}

func (e *HTTPError) Error() string {
	return "HTTP " + strconv.Itoa(e.StatusCode) + ": " + string(e.Body)
}

type StreamItem struct {
	Data []byte
	Err  error
}

type Protocol interface {
	Send(ctx context.Context, req *Request) ([]byte, error)
	Stream(ctx context.Context, req *Request) (<-chan StreamItem, error)
}
