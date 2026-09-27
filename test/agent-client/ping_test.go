package agentclient_test

import (
	"bufio"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	agentclient "github.com/vanpiyp/awp/internal/agent-client"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

func startFakeServer(t *testing.T, handler func(conn net.Conn, req *json_rpc.Request)) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "ping.sock")
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
				_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
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

func TestPingServerReturnsUnexpectedEvent(t *testing.T) {
	socketPath, cleanup := startFakeServer(t, func(conn net.Conn, req *json_rpc.Request) {
		if err := json_rpc.MarshalEvent(conn, req.ID, "not_pong", map[string]string{"hi": "there"}); err != nil {
			t.Errorf("server: marshal event: %v", err)
		}
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	err = c.Ping()
	if err == nil {
		t.Fatal("expected error from Ping when server returns unexpected event")
	}
	if !strings.Contains(err.Error(), "not_pong") {
		t.Errorf("err = %q, want it to mention the unexpected event", err)
	}
}

func TestPingServerClosesConnection(t *testing.T) {
	socketPath, cleanup := startFakeServer(t, func(conn net.Conn, req *json_rpc.Request) {
		_ = conn.Close()
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	err = c.Ping()
	if err == nil {
		t.Fatal("expected error from Ping when server closes connection")
	}
}

func TestPingServerReturnsEmptyPayload(t *testing.T) {
	socketPath, cleanup := startFakeServer(t, func(conn net.Conn, req *json_rpc.Request) {
		_, _ = conn.Write([]byte("\n"))
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.Ping(); err == nil {
		t.Fatal("expected error from Ping when server returns empty payload")
	}
}

func TestPingAfterCloseReturnsError(t *testing.T) {
	socketPath, cleanup := startFakeServer(t, func(conn net.Conn, req *json_rpc.Request) {
		if err := json_rpc.MarshalEvent(conn, req.ID, "pong", nil); err != nil {
			t.Errorf("server: marshal: %v", err)
		}
	})
	defer cleanup()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.Ping(); err == nil {
		t.Fatal("expected error from Ping after Close")
	}
}
