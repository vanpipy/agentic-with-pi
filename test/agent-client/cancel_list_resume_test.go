package agentclient_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	agentclient "github.com/vanpiyp/awp/internal/agent-client"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

func startSequenceServer(t *testing.T, handler func(conn net.Conn, req *json_rpc.Request)) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "seq.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()
				reader := bufio.NewReader(c)
				_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
				req, rerr := json_rpc.ReadRequest(reader)
				if rerr != nil {
					return
				}
				handler(c, req)
			}(conn)
		}
	}()

	cleanup := func() {
		close(stop)
		listener.Close()
		wg.Wait()
	}
	return socketPath, cleanup
}

func TestCancelSendsCancelMethod(t *testing.T) {
	var (
		mu        sync.Mutex
		gotMethod string
		gotParams json.RawMessage
		readDone  = make(chan struct{})
	)
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {
		mu.Lock()
		gotMethod = req.Method
		gotParams = append(json.RawMessage(nil), req.Params...)
		mu.Unlock()
		close(readDone)
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Cancel(context.Background()); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	<-readDone

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != json_rpc.MethodCancel {
		t.Errorf("method = %q, want %q", gotMethod, json_rpc.MethodCancel)
	}
	if len(gotParams) != 0 {
		t.Errorf("params = %q, want empty (CancelParams{})", gotParams)
	}
	c.Close()
}

func TestCancelAfterCloseReturnsError(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.Cancel(context.Background()); err == nil {
		t.Error("expected error from Cancel after Close, got nil")
	}
}

func TestListSessionsParsesResult(t *testing.T) {
	wantSessions := []json_rpc.SessionSummary{
		{SessionID: "s1", Model: "m1", StartedAt: "2025-01-01T00:00:00Z", Events: 10},
		{SessionID: "s2", Model: "m2", StartedAt: "2025-01-02T00:00:00Z", Events: 20},
	}

	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {
		if req.Method != json_rpc.MethodListSessions {
			t.Errorf("server: method = %q, want %q", req.Method, json_rpc.MethodListSessions)
		}
		if err := json_rpc.MarshalEvent(conn, req.ID, "sessions_list", json_rpc.ListSessionsResult{
			Sessions: wantSessions,
		}); err != nil {
			t.Errorf("server: marshal: %v", err)
		}
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	got, err := c.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].SessionID != "s1" || got[1].SessionID != "s2" {
		t.Errorf("got %+v, want session ids s1, s2", got)
	}
	if got[0].Events != 10 || got[1].Events != 20 {
		t.Errorf("events wrong: %+v", got)
	}
}

func TestListSessionsServerErrorPropagates(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {
		if err := json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventError, map[string]string{
			"error": "permission denied",
		}); err != nil {
			t.Errorf("server: marshal: %v", err)
		}
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.ListSessions(context.Background())
	if err == nil {
		t.Fatal("expected error from ListSessions when server returns error event")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("err = %q, want it to mention 'permission denied'", err)
	}
}

func TestListSessionsUnexpectedEvent(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {
		if err := json_rpc.MarshalEvent(conn, req.ID, "wrong_event", []byte(`{}`)); err != nil {
			t.Errorf("server: marshal: %v", err)
		}
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.ListSessions(context.Background())
	if err == nil {
		t.Fatal("expected error from ListSessions when event is unexpected")
	}
	if !strings.Contains(err.Error(), "wrong_event") {
		t.Errorf("err = %q, want it to mention the unexpected event", err)
	}
}

func TestListSessionsInvalidJSON(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {
		if err := json_rpc.MarshalEvent(conn, req.ID, "sessions_list", []byte(`not json`)); err != nil {
			t.Errorf("server: marshal: %v", err)
		}
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.ListSessions(context.Background())
	if err == nil {
		t.Fatal("expected error from ListSessions on invalid JSON body")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("err = %q, want it to mention decode", err)
	}
}

func TestListSessionsServerClosesConnection(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {
		_ = conn.Close()
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.ListSessions(context.Background())
	if err == nil {
		t.Fatal("expected error when server closes mid-read")
	}
}

func TestListSessionsAfterCloseReturnsError(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err = c.ListSessions(context.Background())
	if err == nil {
		t.Error("expected error from ListSessions after Close")
	}
}

func TestResumeEmitsEventsUntilSessionResumed(t *testing.T) {
	var (
		mu        sync.Mutex
		gotMethod string
		gotParams json.RawMessage
	)
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {
		mu.Lock()
		gotMethod = req.Method
		gotParams = append(json.RawMessage(nil), req.Params...)
		mu.Unlock()

		_ = json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventMessage, []byte(`{"session_id":"abc"}`))
		_ = json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventThoughtStart, []byte(`{"session_id":"abc"}`))
		_ = json_rpc.MarshalEvent(conn, req.ID, "session_resumed", []byte(`{"session_id":"abc","status":"ok"}`))
		_ = json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventMessage, []byte(`{"session_id":"abc"}`))
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	events, err := c.Resume(context.Background(), "abc")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}

	var kinds []string
	for ev := range events {
		kinds = append(kinds, ev.Kind)
	}

	if len(kinds) != 3 {
		t.Errorf("kinds = %v, want 3 (stop at first session_resumed)", kinds)
	}
	if kinds[len(kinds)-1] != "session_resumed" {
		t.Errorf("last kind = %q, want session_resumed", kinds[len(kinds)-1])
	}

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != json_rpc.MethodResume {
		t.Errorf("method = %q, want %q", gotMethod, json_rpc.MethodResume)
	}
	if !strings.Contains(string(gotParams), `"session_id":"abc"`) {
		t.Errorf("params = %q, want session_id=abc", gotParams)
	}
}

func TestResumeServerClosesConnection(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {
		_ = conn.Close()
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	events, err := c.Resume(context.Background(), "s1")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	for range events {
	}
}

func TestResumeContextCancel(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {
		time.Sleep(3 * time.Second)
		_ = json_rpc.MarshalEvent(conn, req.ID, "session_resumed", []byte(`{}`))
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	events, err := c.Resume(ctx, "s1")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	count := 0
	for range events {
		count++
	}
}

func TestResumeAfterCloseReturnsError(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err = c.Resume(context.Background(), "s1")
	if err == nil {
		t.Error("expected error from Resume after Close")
	}
}

func TestPromptStreamEndsOnCancelAck(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {
		if req.Method != json_rpc.MethodPrompt {
			t.Errorf("server: method = %q, want %q", req.Method, json_rpc.MethodPrompt)
		}
		_ = json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventMessage, []byte(`{"session_id":"x","message":{"role":"user","content":[{"type":"text","text":"hi"}]},"stop_reason":"toolUse"}`))
		_ = json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventCancelAck, []byte(`{"reason":"user"}`))
		_ = json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventMessage, []byte(`{}`))
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	events, err := c.Prompt(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}

	var count int
	for range events {
		count++
	}
	if count != 2 {
		t.Errorf("events count = %d, want 2 (stop at cancel_ack)", count)
	}
}

func TestPromptStreamEndsOnMalformedMessageBody(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {
		_ = json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventMessage, []byte(`not json`))
		_ = json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventMessage, []byte(`{}`))
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	events, err := c.Prompt(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}

	var count int
	for range events {
		count++
	}
	if count != 2 {
		t.Errorf("events count = %d, want 2 (malformed body is non-terminal, final assistant ends)", count)
	}
}

func TestPromptContextCancelBeforeSend(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	events, err := c.Prompt(ctx, "hi")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	for range events {
	}
}

func TestPromptWithSessionIDSendError(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err = c.Prompt(context.Background(), "hi")
	if err == nil {
		t.Error("expected error from Prompt when conn is closed before send")
	}
}

func TestCompactUnexpectedEvent(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {
		_ = json_rpc.MarshalEvent(conn, req.ID, "wrong_compact_event", []byte(`{}`))
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Compact(context.Background(), "s1", false)
	if err == nil {
		t.Fatal("expected error from Compact when event is unexpected")
	}
	if !strings.Contains(err.Error(), "unexpected") {
		t.Errorf("err = %q, want it to mention unexpected", err)
	}
}

func TestCompactServerClosesConnection(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {
		_ = conn.Close()
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Compact(context.Background(), "s1", false)
	if err == nil {
		t.Fatal("expected error from Compact when server closes mid-read")
	}
}

func TestSendPromptDialFailure(t *testing.T) {
	_, err := agentclient.SendPrompt(context.Background(), "/nonexistent.sock", "", "hi")
	if err == nil {
		t.Fatal("expected error from SendPrompt when Dial fails")
	}
}

func TestSendPromptContextCancelMidStream(t *testing.T) {
	socketPath, cleanup := startSequenceServer(t, func(conn net.Conn, req *json_rpc.Request) {
		_ = json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventMessage, []byte(`{"session_id":"x","message":{"role":"user","content":[{"type":"text","text":"hi"}]},"stop_reason":"toolUse"}`))
	})
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	events, err := agentclient.SendPrompt(ctx, socketPath, "", "hi")
	if err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	count := 0
	for range events {
		count++
	}
}
