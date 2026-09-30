package agentcore_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/llm"
)

// splitFakeCore returns split ContentBlocks from CompleteSplit so the
// agent loop exercises the cache-aware code path. StreamChat is a no-op
// terminal stream.
type splitFakeCore struct {
	streamChunks []llm.StreamChunk
	splitBlocks  []llm.ContentBlock
	cacheModel   string
	streamCalls  int
	requests     []llm.ChatRequest
}

func (f *splitFakeCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	f.streamCalls++
	f.requests = append(f.requests, *req)
	ch := make(chan llm.StreamEvent, len(f.streamChunks)+1)
	for _, c := range f.streamChunks {
		if c.Usage != nil {
			ch <- llm.EventUsage{
				InputTokens:         c.Usage.PromptTokens,
				OutputTokens:        c.Usage.CompletionTokens,
				CacheReadTokens:     c.Usage.CacheReadTokens,
				CacheCreationTokens: c.Usage.CacheCreationTokens,
			}
		}
		if len(c.Choices) > 0 {
			choice := c.Choices[0]
			if choice.Delta.Content != "" {
				ch <- llm.EventTextDelta{Text: choice.Delta.Content}
			}
			if choice.Delta.Reasoning != "" {
				ch <- llm.EventThinkingDelta{Text: choice.Delta.Reasoning}
			}
		}
	}
	ch <- llm.EventFinish{Reason: llm.FinishReasonStop}
	close(ch)
	return ch, nil
}

func (f *splitFakeCore) CompleteSplit(systemPrompt string, model string) []llm.ContentBlock {
	if len(f.splitBlocks) > 0 {
		return f.splitBlocks
	}
	return llm.SplitSystemPrompt(systemPrompt, true)
}

func newSplitAgent(core *splitFakeCore, sysPrompt string) *agentcore.Agent {
	a := agentcore.NewAgent(core).
		WithModel(llm.Model{ID: "split-model"}).
		WithSessionID("session-split")
	a.SetSystemPrompts(sysPrompt)
	return a
}

func TestAgentRunStreamEmitsCacheControlOnStaticSystemBlock(t *testing.T) {
	core := &splitFakeCore{
		streamChunks: []llm.StreamChunk{
			textDeltaChunk("ok"),
			messageDeltaStopChunk("end_turn"),
			messageStopChunk(),
		},
	}
	prompt := strings.Repeat("a", 60) + "\n" + strings.Repeat("b", 60)
	ag := newSplitAgent(core, prompt)

	for range ag.RunStream(context.Background(), "user query") {
	}

	if core.streamCalls == 0 {
		t.Fatal("StreamChat was never called")
	}
	req := core.requests[0]

	// Expect at least two system messages (static + dynamic) plus user.
	systemCount := 0
	var firstSys llm.Message
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemCount++
			if firstSys.Role == "" {
				firstSys = m
			}
		}
	}
	if systemCount != 2 {
		t.Fatalf("got %d system messages, want 2 (static + dynamic)", systemCount)
	}
	if firstSys.CacheControl == nil {
		t.Errorf("first system message has no CacheControl; expected ephemeral")
	} else if firstSys.CacheControl.Type != "ephemeral" {
		t.Errorf("first system CacheControl.Type = %q, want %q", firstSys.CacheControl.Type, "ephemeral")
	}
	// The first system message should end with the separator newline,
	// confirming static and dynamic concatenate to the original prompt.
	if !strings.HasSuffix(firstSys.Content, "\n") {
		t.Errorf("first system Content = %q, want trailing newline (static block convention)", firstSys.Content)
	}
}

func TestAgentRunStreamNoMidNewlineFallsBackToSingleSystemMessage(t *testing.T) {
	core := &splitFakeCore{
		streamChunks: []llm.StreamChunk{
			textDeltaChunk("ok"),
			messageDeltaStopChunk("end_turn"),
			messageStopChunk(),
		},
	}
	// No newline anywhere — SplitSystemPrompt should return one block,
	// and the agent should emit a single system message identical to
	// pre-split behaviour.
	prompt := strings.Repeat("x", 80)
	ag := newSplitAgent(core, prompt)

	for range ag.RunStream(context.Background(), "user query") {
	}

	req := core.requests[0]
	systemCount := 0
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemCount++
		}
	}
	if systemCount != 1 {
		t.Fatalf("got %d system messages, want 1 (no mid-newline fallback)", systemCount)
	}
}

func TestAgentSystemPromptMessagesWithEmptyPromptReturnsSingleEmptyMessage(t *testing.T) {
	core := &splitFakeCore{
		streamChunks: []llm.StreamChunk{
			textDeltaChunk("ok"),
			messageDeltaStopChunk("end_turn"),
			messageStopChunk(),
		},
	}
	ag := newSplitAgent(core, "")

	for range ag.RunStream(context.Background(), "user query") {
	}

	req := core.requests[0]
	systemCount := 0
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemCount++
		}
	}
	if systemCount != 1 {
		t.Fatalf("got %d system messages, want 1 (empty prompt fallback)", systemCount)
	}
}

func TestAgentResumedRunAlsoHonoursCacheControl(t *testing.T) {
	core := &splitFakeCore{
		streamChunks: []llm.StreamChunk{
			textDeltaChunk("resumed"),
			messageDeltaStopChunk("end_turn"),
			messageStopChunk(),
		},
	}
	prompt := strings.Repeat("a", 60) + "\n" + strings.Repeat("b", 60)
	ag := newSplitAgent(core, prompt)
	history := []llm.Message{}

	ch, _ := ag.RunStreamResumedWithSnapshot(context.Background(), "next task", history)
	for range ch {
	}

	if core.streamCalls == 0 {
		t.Fatal("StreamChat was never called on resumed run")
	}
	req := core.requests[0]
	var firstSys llm.Message
	for _, m := range req.Messages {
		if m.Role == "system" {
			firstSys = m
			break
		}
	}
	if firstSys.Role != "system" {
		t.Fatalf("no system message in resumed run, got %+v", req.Messages)
	}
	if firstSys.CacheControl == nil {
		t.Errorf("resumed run: first system message lacks CacheControl; want ephemeral on static block")
	}
}
