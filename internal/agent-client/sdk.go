package agentclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/ipc"
)

type Client struct {
	conn    net.Conn
	reader  *bufio.Reader
	closeMu sync.Mutex
	closed  bool
}

func Dial(socketPath string) (*Client, error) {
	conn, err := ipc.Dial(socketPath)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	return &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

func SendPrompt(ctx context.Context, socketPath, sessionID, prompt string) (<-chan Event, error) {
	c, err := Dial(socketPath)
	if err != nil {
		return nil, err
	}
	events, err := c.PromptWithSessionID(ctx, prompt, sessionID)
	if err != nil {
		c.Close()
		return nil, err
	}
	wrapped := make(chan Event, 32)
	go func() {
		defer close(wrapped)
		defer c.Close()
		for ev := range events {
			select {
			case wrapped <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return wrapped, nil
}

func (c *Client) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}

type Event struct {
	Kind string
	Data []byte
}

func (c *Client) Prompt(ctx context.Context, prompt string) (<-chan Event, error) {
	return c.PromptWithSessionID(ctx, prompt, "")
}

func (c *Client) PromptWithSessionID(ctx context.Context, prompt, sessionID string) (<-chan Event, error) {
	req, err := json_rpc.NewRequest("1", json_rpc.MethodPrompt, json_rpc.PromptParams{
		SessionID: sessionID,
		Prompt:    prompt,
	})
	if err != nil {
		return nil, err
	}
	if err := json_rpc.MarshalRequest(c.conn, req); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	events := make(chan Event, 32)
	go func() {
		defer close(events)
		for {
			if ctx.Err() != nil {
				return
			}
			resp, err := json_rpc.ReadEvent(c.reader)
			if err != nil {
				return
			}
			events <- Event{Kind: resp.Event, Data: resp.Data}
			if resp.Event == json_rpc.EventFinalAnswer {
				return
			}
		}
	}()
	return events, nil
}

func (c *Client) Ping() error {
	req, _ := json_rpc.NewRequest("1", json_rpc.MethodPing, nil)
	if err := json_rpc.MarshalRequest(c.conn, req); err != nil {
		return err
	}
	resp, err := json_rpc.ReadEvent(c.reader)
	if err != nil {
		return err
	}
	if resp.Event != "pong" {
		return fmt.Errorf("unexpected event: %s", resp.Event)
	}
	return nil
}

func ParseEventData(data []byte, dst any) error {
	return json.Unmarshal(data, dst)
}

func (c *Client) Cancel(ctx context.Context) error {
	req, _ := json_rpc.NewRequest("1", json_rpc.MethodCancel, nil)
	if err := json_rpc.MarshalRequest(c.conn, req); err != nil {
		return fmt.Errorf("send cancel: %w", err)
	}
	return nil
}

func (c *Client) ListSessions(ctx context.Context) ([]json_rpc.SessionSummary, error) {
	req, _ := json_rpc.NewRequest("1", json_rpc.MethodListSessions, json_rpc.ListSessionsParams{})
	if err := json_rpc.MarshalRequest(c.conn, req); err != nil {
		return nil, fmt.Errorf("send list_sessions: %w", err)
	}
	resp, err := json_rpc.ReadEvent(c.reader)
	if err != nil {
		return nil, fmt.Errorf("read list_sessions: %w", err)
	}
	if resp.Event == json_rpc.EventError {
		return nil, fmt.Errorf("server error: %s", string(resp.Data))
	}
	if resp.Event != "sessions_list" {
		return nil, fmt.Errorf("unexpected event: %s", resp.Event)
	}
	var out json_rpc.ListSessionsResult
	if err := json.Unmarshal(resp.Data, &out); err != nil {
		return nil, fmt.Errorf("decode list_sessions: %w", err)
	}
	return out.Sessions, nil
}

func (c *Client) Resume(ctx context.Context, sessionID string) (<-chan Event, error) {
	req, err := json_rpc.NewRequest("1", json_rpc.MethodResume, json_rpc.ResumeParams{
		SessionID: sessionID,
	})
	if err != nil {
		return nil, err
	}
	if err := json_rpc.MarshalRequest(c.conn, req); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	events := make(chan Event, 32)
	go func() {
		defer close(events)
		for {
			if ctx.Err() != nil {
				return
			}
			resp, err := json_rpc.ReadEvent(c.reader)
			if err != nil {
				return
			}
			events <- Event{Kind: resp.Event, Data: resp.Data}
			if resp.Event == "session_resumed" {
				return
			}
		}
	}()
	return events, nil
}
