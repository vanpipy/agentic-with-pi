package protocol

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"
)

const maxResponseBodyBytes = 10 * 1024 * 1024

type HTTPRest struct {
	client *http.Client
}

func NewHTTPRest() *HTTPRest {
	return &HTTPRest{
		client: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,

				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,

				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,

				ForceAttemptHTTP2: true,
			},
		},
	}
}

func applyRequestTimeout(parent context.Context, req *Request) (context.Context, context.CancelFunc) {
	if req.Timeout > 0 {
		return context.WithTimeout(parent, req.Timeout)
	}
	return parent, func() {}
}

func logPath(s string) string {
	if u, err := url.Parse(s); err == nil {
		return u.Path
	}
	return s
}

func (p *HTTPRest) Send(ctx context.Context, req *Request) ([]byte, error) {
	ctx, cancel := applyRequestTimeout(ctx, req)
	defer cancel()

	start := time.Now()
	status := 0
	defer func() {
		slog.Debug("http_send",
			"method", req.Method,
			"path", logPath(req.URL),
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}()

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	status = resp.StatusCode

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if len(data) > maxResponseBodyBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", maxResponseBodyBytes)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       data,
		}
	}

	return data, nil
}

func (p *HTTPRest) Stream(ctx context.Context, req *Request) (<-chan StreamItem, error) {
	ctx, cancel := applyRequestTimeout(ctx, req)
	defer cancel()

	start := time.Now()
	status := 0
	defer func() {
		slog.Debug("http_stream",
			"method", req.Method,
			"path", logPath(req.URL),
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}()

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
		resp.Body.Close()
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       body,
		}
	}
	status = resp.StatusCode

	out := make(chan StreamItem, 64)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Bytes()
			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			payload := bytes.TrimSpace(line[5:])
			if len(payload) == 0 {
				continue
			}
			cp := append([]byte(nil), payload...)
			select {
			case out <- StreamItem{Data: cp}:
			case <-ctx.Done():
				return
			}
		}

		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			select {
			case out <- StreamItem{Err: fmt.Errorf("stream read error: %w", err)}:
			case <-ctx.Done():
			}
		}
	}()

	return out, nil
}
