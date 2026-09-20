package client_sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/vanpiyp/awp/internal/protocol"
	"github.com/vanpiyp/awp/internal/transport"
)

type Client struct {
	conn   net.Conn
	reader *bufio.Reader
}

func Dial(socketPath string) (*Client, error) {
	conn, err := transport.Dial(socketPath)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	return &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

func (c *Client) Close() error {
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
	req, err := protocol.NewRequest("1", protocol.MethodPrompt, protocol.PromptParams{
		SessionID: sessionID,
		Prompt:    prompt,
	})
	if err != nil {
		return nil, err
	}
	if err := protocol.MarshalRequest(c.conn, req); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	events := make(chan Event, 32)
	go func() {
		defer close(events)
		for {
			if ctx.Err() != nil {
				return
			}
			resp, err := protocol.ReadEvent(c.reader)
			if err != nil {
				return
			}
			events <- Event{Kind: resp.Event, Data: resp.Data}
			if resp.Event == protocol.EventFinalAnswer {
				return
			}
		}
	}()
	return events, nil
}

func (c *Client) Ping() error {
	req, _ := protocol.NewRequest("1", protocol.MethodPing, nil)
	if err := protocol.MarshalRequest(c.conn, req); err != nil {
		return err
	}
	resp, err := protocol.ReadEvent(c.reader)
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

func (c *Client) Resume(ctx context.Context, sessionID string) (<-chan Event, error) {
	req, err := protocol.NewRequest("1", protocol.MethodResume, protocol.ResumeParams{
		SessionID: sessionID,
	})
	if err != nil {
		return nil, err
	}
	if err := protocol.MarshalRequest(c.conn, req); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	events := make(chan Event, 32)
	go func() {
		defer close(events)
		for {
			if ctx.Err() != nil {
				return
			}
			resp, err := protocol.ReadEvent(c.reader)
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
