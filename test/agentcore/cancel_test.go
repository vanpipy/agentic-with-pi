package agentcore_test

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/tui"
)

type fakeBlockingCore struct{}

func (f *fakeBlockingCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	out := make(chan llm.StreamEvent)
	go func() {
		defer close(out)
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
			return
		}
	}()
	return out, nil
}

func TestAgentRunStreamRespectsContextCancel(t *testing.T) {
	core := &fakeBlockingCore{}
	ag := agentcore.NewAgent(core).WithModel(llm.Model{ID: "test", SupportsStreaming: true})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	events := ag.RunStream(ctx, "test")
	start := time.Now()
	for range events {
	}
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Errorf("RunStream took %v after cancel, expected < 2s", elapsed)
	}
}

func TestEscExitsStreamingState(t *testing.T) {
	m := tui.NewModelForTest()
	out, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = out.(*tui.Model)
	m.SetStateForTest(tui.StateStreamingForTestValue())
	if m.StateForTest() != tui.StateStreaming {
		t.Fatal("setup: SetStateForTest did not set streaming")
	}
	out, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = out.(*tui.Model)
	if m.StateForTest() == tui.StateStreaming {
		t.Fatal("esc should exit streaming state")
	}
}
