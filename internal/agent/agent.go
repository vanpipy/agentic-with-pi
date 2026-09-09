package agent

import (
	"context"
	"fmt"

	"github.com/vanpiyp/awp/internal/llm"
)

type Tool struct {
	Def  llm.ToolDef
	Func func(argsJSON string) (string, error)
}

type Agent struct {
	core         llm.Core
	model        string
	systemPrompt string
	tools        []Tool
	maxTurns     int
}

func New(core llm.Core, model string) *Agent {
	return &Agent{
		core:     core,
		model:    model,
		maxTurns: 10,
	}
}

func (a *Agent) WithSystemPrompt(prompt string) *Agent {
	a.systemPrompt = prompt
	return a
}

func (a *Agent) WithMaxTurns(n int) *Agent {
	if n > 0 {
		a.maxTurns = n
	}
	return a
}

func (a *Agent) RegisterTool(tool Tool) {
	a.tools = append(a.tools, tool)
}

func (a *Agent) Run(ctx context.Context, userMessage string) (string, error) {
	messages := []llm.Message{}
	if a.systemPrompt != "" {
		messages = append(messages, llm.Message{Role: "system", Content: a.systemPrompt})
	}
	messages = append(messages, llm.Message{Role: "user", Content: userMessage})

	toolDefs := make([]llm.ToolDef, 0, len(a.tools))
	for _, t := range a.tools {
		toolDefs = append(toolDefs, t.Def)
	}

	for turn := 0; turn < a.maxTurns; turn++ {
		resp, err := a.core.Chat(ctx, &llm.ChatRequest{
			Model:    a.model,
			Messages: messages,
			Tools:    toolDefs,
		})
		if err != nil {
			return "", fmt.Errorf("turn %d: chat failed: %w", turn, err)
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("turn %d: no choices in response", turn)
		}
		choice := resp.Choices[0]

		if len(choice.Message.ToolCalls) == 0 {
			return choice.Message.Content, nil
		}

		messages = append(messages, choice.Message)

		for _, tc := range choice.Message.ToolCalls {
			tool, ok := a.findTool(tc.Function.Name)
			if !ok {
				return "", fmt.Errorf("turn %d: unknown tool %q", turn, tc.Function.Name)
			}
			result, err := tool.Func(tc.Function.Arguments)
			if err != nil {
				return "", fmt.Errorf("turn %d: tool %q failed: %w", turn, tc.Function.Name, err)
			}
			messages = append(messages, llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    result,
			})
		}
	}
	return "", fmt.Errorf("max turns (%d) reached without final answer", a.maxTurns)
}

func (a *Agent) findTool(name string) (*Tool, bool) {
	for i := range a.tools {
		if a.tools[i].Def.Function.Name == name {
			return &a.tools[i], true
		}
	}
	return nil, false
}
