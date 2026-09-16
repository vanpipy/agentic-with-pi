package protocol_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/llm/protocol"
)

func TestSendSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "hello" {
			t.Errorf("body = %q, want hello", body)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	p := protocol.NewHTTPRest()
	resp, err := p.Send(context.Background(), &protocol.Request{
		URL:    srv.URL,
		Method: "POST",
		Body:   []byte("hello"),
		Headers: map[string]string{"X-Test": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp) != "ok" {
		t.Errorf("body = %q, want ok", resp)
	}
}

func TestSendNon200ReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("auth failed"))
	}))
	defer srv.Close()

	p := protocol.NewHTTPRest()
	_, err := p.Send(context.Background(), &protocol.Request{URL: srv.URL, Method: "GET"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	httpErr, ok := err.(*protocol.HTTPError)
	if !ok {
		t.Fatalf("err is not *HTTPError: %T", err)
	}
	if httpErr.StatusCode != 401 {
		t.Errorf("status = %d, want 401", httpErr.StatusCode)
	}
	if string(httpErr.Body) != "auth failed" {
		t.Errorf("body = %q, want auth failed", httpErr.Body)
	}
}

func TestSendBodySizeLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("x"), 11*1024*1024))
	}))
	defer srv.Close()

	p := protocol.NewHTTPRest()
	_, err := p.Send(context.Background(), &protocol.Request{URL: srv.URL, Method: "GET"})
	if err == nil {
		t.Fatal("expected error for oversized body, got nil")
	}
}

func TestSendTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Write([]byte("slow"))
	}))
	defer srv.Close()

	p := protocol.NewHTTPRest()
	_, err := p.Send(context.Background(), &protocol.Request{
		URL:     srv.URL,
		Method:  "GET",
		Timeout: 50 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestStreamSSEReadsDataLines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: hello\n\n"))
		flusher.Flush()
		_, _ = w.Write([]byte("data: world\n\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	p := protocol.NewHTTPRest()
	ch, err := p.Stream(context.Background(), &protocol.Request{URL: srv.URL, Method: "GET"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for item := range ch {
		if item.Err != nil {
			t.Fatal(item.Err)
		}
		got = append(got, string(item.Data))
	}
	if len(got) != 2 || got[0] != "hello" || got[1] != "world" {
		t.Errorf("got %v, want [hello world]", got)
	}
}

func TestStreamNon200ReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("rate limited"))
	}))
	defer srv.Close()

	p := protocol.NewHTTPRest()
	_, err := p.Stream(context.Background(), &protocol.Request{URL: srv.URL, Method: "GET"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	httpErr, ok := err.(*protocol.HTTPError)
	if !ok {
		t.Fatalf("err is not *HTTPError: %T", err)
	}
	if httpErr.StatusCode != 429 {
		t.Errorf("status = %d, want 429", httpErr.StatusCode)
	}
}

func TestStreamMidStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: ok\n\n"))
		flusher.Flush()
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer srv.Close()

	p := protocol.NewHTTPRest()
	ch, err := p.Stream(context.Background(), &protocol.Request{URL: srv.URL, Method: "GET"})
	if err != nil {
		t.Fatal(err)
	}
	var sawErr bool
	for item := range ch {
		if item.Err != nil {
			sawErr = true
		}
	}
	if !sawErr {
		t.Error("expected mid-stream error, got clean close")
	}
}
