package agentcore_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol"
	"github.com/vanpiyp/awp/internal/llm/providers"
)

// redirectTransport forwards every outgoing request to a single
// httptest server URL so the production protocol.NewHTTPRest stack
// (http.DefaultTransport + dialer) hits our test fixture.
type redirectTransport struct{ target string }

func (r redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(r.target, "http://")
	return http.DefaultTransport.RoundTrip(clone)
}

// TestAgentCoreAnthropicSelfHealRetryFromAgentContext verifies the
// self-heal path introduced in Sprint 5 closeout is reachable from
// the real agent-core consumer (ReAct strategy), not just the llm
// layer. The fixture mirrors what the Anthropic API would actually
// emit on a transition-window 400: invalid_request_error +
// output_config + not supported, followed by an OK response with the
// thinking envelope dropped. The agent step must end with a
// EventFinalAnswer carrying the recovered content and zero error
// events.
func TestAgentCoreAnthropicSelfHealRetryFromAgentContext(t *testing.T) {
	var calls atomic.Int32
	var bodies [2]string
	var bodiesMu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		bodiesMu.Lock()
		bodies[n-1] = string(body)
		bodiesMu.Unlock()
		if n == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"output_config.effort is not supported on this model"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"recovered"}}
`+"\n\n"+`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":1,"output_tokens":1}}
`+"\n\n"+`data: {"type":"message_stop"}
`+"\n\n")
	}))
	defer srv.Close()

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	core := llm.NewCore(providers.NewAnthropicProvider("tk"), protocol.NewHTTPRestWithClient(client))

	strat := agentcore.NewReActStrategy(core, llm.Model{ID: "claude-opus-5-5", SupportsTool: true}, func() []llm.ToolDef { return nil })

	var (
		mu              sync.Mutex
		finalContent    string
		errCount        int
		eventCategories []agentcore.EventCategory
	)
	capture := func(_ context.Context, e agentcore.Event) bool {
		mu.Lock()
		defer mu.Unlock()
		eventCategories = append(eventCategories, e.Category)
		if e.Category == agentcore.EventFinalAnswer {
			finalContent = e.Content
		}
		if e.Category == agentcore.EventError {
			errCount++
		}
		return true
	}

	_, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "hi"}}, capture)
	if err != nil {
		t.Fatalf("Step returned err: %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Errorf("calls = %d, want 2 (first 400 + self-heal retry)", got)
	}
	if finalContent != "recovered" {
		t.Errorf("final content = %q, want %q (self-heal must surface recovered text to the agent)", finalContent, "recovered")
	}
	if errCount != 0 {
		t.Errorf("error events = %d, want 0 (self-heal must hide the recoverable 400 from the agent)", errCount)
	}

	bodiesMu.Lock()
	defer bodiesMu.Unlock()

	if !strings.Contains(bodies[0], `"output_config"`) {
		t.Errorf("first body missing output_config (must be emitted on first attempt):\n%s", bodies[0])
	}
	if !strings.Contains(bodies[0], `"thinking"`) {
		t.Errorf("first body missing thinking envelope:\n%s", bodies[0])
	}
	if strings.Contains(bodies[1], `"output_config"`) {
		t.Errorf("second body still has output_config (self-heal must drop it):\n%s", bodies[1])
	}
	if !strings.Contains(bodies[1], `"thinking"`) {
		t.Errorf("second body missing thinking envelope (always-on model must keep adaptive thinking):\n%s", bodies[1])
	}
}
