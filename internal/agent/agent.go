package agent

import (
	"context"

	"github.com/vanpiyp/awp/internal/llm"
)

type EventCategory int

const (
	EventThoughtStart EventCategory = iota

	EventThoughtEnd

	EventTool

	EventObserve

	EventFinalAnswer

	EventError
)

type Event struct {
	Category EventCategory

	Content string

	Reasoning string

	ToolName string

	ToolArgs string

	ToolResult string
	
	ToolError string

	ToolCalls []llm.ToolCall
}

type Tool struct {
	Name string

	Description string

	Parameters any

	Execute func(ctx context.Context, argsJSON string) (string, error)
}

type Agent struct {
	core llm.Core

	MaxTurns int

	Model llm.Model

	SystemPrompts string

	Tools []Tool
}

func NewAgent(llmCore llm.Core) *Agent {
	return &Agent{
		core: llmCore,
		MaxTurns: 10,
		SystemPrompts: "You are a helpful coding assistant",
	}
}

func (a *Agent) WithMaxTurns(n int) *Agent {
	a.MaxTurns = n
	return a
}

func (a *Agent) WithModel(model llm.Model) *Agent {
	a.Model = model
	return a
}

func (a *Agent) WithTool(t Tool) *Agent {
	a.Tools = append(a.Tools, t)
	return a
}

func (a *Agent) SetSystemPrompts(p string) {
	a.SystemPrompts = p
}

func (a *Agent) RunStream(ctx context.Context, userMsg string) <-chan Event {
	ch := make(chan Event, 32)
	go func() {
		defer close(ch)
		a.loop(ctx, userMsg, ch)
	}()
	return ch
}

func (a *Agent) loop(ctx content.Context, userMsg string, ch chan<- Event) {
	if a.Model.ID == "" {
		ch <- Event{
			Category: EventError,
			ToolError: "Model not set, call WithModel before RunStream",
		}
		return
	}
}
