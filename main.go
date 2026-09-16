package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/vanpiyp/awp/internal/agent"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol"
	"github.com/vanpiyp/awp/internal/llm/providers"
)

func main() {
	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
		log.Fatal("MINIMAX_API_KEY cannot be found")
	}

	registry := llm.NewRegistry()
	registry.MustRegisterDefault(providers.NewMiniMaxProvider(apiKey))

	fmt.Println("registered providers:", registry.Names())
	if m, p, ok := registry.FindModel("MiniMax-M3"); ok {
		fmt.Printf("model lookup: id=%s vendor=%s supports_tool=%v context=%d\n",
			m.ID, p.Name(), m.SupportsTool, m.MaxContextTokens)
	}

	provider, ok := registry.Default()
	if !ok {
		log.Fatal("default provider not registered")
	}
	baseCore := llm.NewCore(provider, protocol.NewHTTPRest())
	core := llm.NewRetryCore(baseCore, llm.DefaultRetryConfig())
	ctx := context.Background()

	fmt.Println("\n=== Test 1: StreamChat, no system ===")
	runStream(ctx, core, "Count from 1 to 3, separated by spaces", nil, nil)

	fmt.Println("\n=== Test 2: StreamChat, with system prompt ===")
	runStream(ctx, core, "Say hello in one short sentence.",
		[]llm.Message{{Role: "system", Content: "You are a pirate. Always speak like one."}}, nil)

	fmt.Println("\n=== Test 3: Chat (non-streaming), simple ===")
	runChat(ctx, core, "What is 2+2? Reply with just the number.", nil)

	fmt.Println("\n=== Test 4: Tool call (non-streaming, single call) ===")
	runToolCall(ctx, core)

	fmt.Println("\n=== Test 5: Tool call (streaming, single call) ===")
	runToolCallStream(ctx, core)

	fmt.Println("\n=== Test 6: Agent loop (full round-trip via internal/agent) ===")
	runAgent(ctx, core)
}

func runStream(ctx context.Context, core llm.Core, prompt string, system []llm.Message, tools []llm.ToolDef) {
	msgs := append(system, llm.Message{Role: "user", Content: prompt})
	req := &llm.ChatRequest{Model: "MiniMax-M3", Messages: msgs}
	if len(tools) > 0 {
		req.Tools = tools
	}
	events, err := core.StreamChat(ctx, req)
	if err != nil {
		log.Fatalf("stream setup failed: %v", err)
	}
	fmt.Print("> ")
	for ev := range events {
		if ev.Err != nil {
			log.Fatalf("stream error: %v", ev.Err)
		}
		for _, c := range ev.Chunk.Choices {
			fmt.Print(c.Delta.Content)
		}
	}
	fmt.Println()
}

func runChat(ctx context.Context, core llm.Core, prompt string, system []llm.Message) {
	msgs := append(system, llm.Message{Role: "user", Content: prompt})
	resp, err := core.Chat(ctx, &llm.ChatRequest{Model: "MiniMax-M3", Messages: msgs})
	if err != nil {
		log.Fatalf("chat failed: %v", err)
	}
	for _, c := range resp.Choices {
		fmt.Printf("> content:    %q\n", c.Message.Content)
		if c.Message.Reasoning != "" {
			fmt.Printf("> reasoning:  %q\n", truncate(c.Message.Reasoning, 80))
		}
		fmt.Printf("> stop:       %s\n", c.FinishReason)
		fmt.Printf("> usage:      %d in / %d out\n",
			resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	}
}

func weatherTool() llm.ToolDef {
	return llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "get_weather",
			Description: "Get the current weather for a given city. Returns temperature in Celsius and a short description.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]any{
						"type":        "string",
						"description": "City name, e.g. 'Beijing' or 'Tokyo'",
					},
					"unit": map[string]any{
						"type":        "string",
						"enum":        []string{"celsius", "fahrenheit"},
						"description": "Temperature unit",
					},
				},
				"required": []string{"location"},
			},
		},
	}
}

func runToolCall(ctx context.Context, core llm.Core) {
	resp, err := core.Chat(ctx, &llm.ChatRequest{
		Model: "MiniMax-M3",
		Tools: []llm.ToolDef{weatherTool()},
		Messages: []llm.Message{
			{Role: "user", Content: "What's the weather like in Beijing right now?"},
		},
	})
	if err != nil {
		log.Fatalf("tool-call chat failed: %v", err)
	}
	for _, c := range resp.Choices {
		fmt.Printf("> content:    %q\n", c.Message.Content)
		if c.Message.Reasoning != "" {
			fmt.Printf("> reasoning:  %q\n", truncate(c.Message.Reasoning, 80))
		}
		fmt.Printf("> stop:       %s\n", c.FinishReason)
		fmt.Printf("> tool_calls: %d\n", len(c.Message.ToolCalls))
		for i, tc := range c.Message.ToolCalls {
			fmt.Printf("  [%d] id=%s name=%s args=%s\n", i, tc.ID, tc.Function.Name, tc.Function.Arguments)
			var parsed map[string]any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &parsed); err != nil {
				log.Printf("  WARN: args not valid JSON: %v", err)
			} else {
				fmt.Printf("      parsed: %v\n", parsed)
			}
		}
	}
}

func runToolCallStream(ctx context.Context, core llm.Core) {
	events, err := core.StreamChat(ctx, &llm.ChatRequest{
		Model: "MiniMax-M3",
		Tools: []llm.ToolDef{weatherTool()},
		Messages: []llm.Message{
			{Role: "user", Content: "Get the Tokyo weather in celsius, please."},
		},
	})
	if err != nil {
		log.Fatalf("stream setup failed: %v", err)
	}

	fmt.Print("> stream: ")
	var reasoningBuf strings.Builder
	var assembledCalls []llm.ToolCall
	for ev := range events {
		if ev.Err != nil {
			log.Fatalf("stream error: %v", ev.Err)
		}
		for _, c := range ev.Chunk.Choices {
			fmt.Print(c.Delta.Content)
			reasoningBuf.WriteString(c.Delta.Reasoning)
			if len(c.Delta.ToolCalls) > 0 {
				assembledCalls = append(assembledCalls, c.Delta.ToolCalls...)
			}
		}
	}
	fmt.Println()
	if r := reasoningBuf.String(); r != "" {
		fmt.Printf("  [reasoning: %d chars]\n", len(r))
	}
	if len(assembledCalls) > 0 {
		fmt.Printf("  [assembled tool calls: %d]\n", len(assembledCalls))
		for i, tc := range assembledCalls {
			fmt.Printf("  [%d] id=%s name=%s args=%s\n", i, tc.ID, tc.Function.Name, tc.Function.Arguments)
			var parsed map[string]any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &parsed); err != nil {
				log.Printf("  WARN: args not valid JSON: %v", err)
			} else {
				fmt.Printf("      parsed: %v\n", parsed)
			}
		}
	}
}

func runAgent(ctx context.Context, core llm.Core) {
	ag := agent.New(core, "MiniMax-M3")
	ag.SetSystemPrompt("You are a helpful assistant. Use the available tools to answer user questions. Be concise.")

	ag.AddTool(agent.Tool{
		Name:        "get_weather",
		Description: "Get the current weather for a given city. Returns temperature in Celsius and a short description.",
		Parameters: weatherTool().Function.Parameters,
		Execute: func(_ context.Context, argsJSON string) (string, error) {
			var args struct {
				Location string `json:"location"`
				Unit     string `json:"unit"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}
			unit := args.Unit
			if unit == "" {
				unit = "celsius"
			}
			return fmt.Sprintf("Weather in %s: 22°%s, sunny, light wind from the south.", args.Location, degree(unit)), nil
		},
	})

	ag.AddTool(agent.Tool{
		Name:        "get_time",
		Description: "Get the current local time for a city. Returns ISO 8601 format.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"location": map[string]any{
					"type":        "string",
					"description": "City name",
				},
			},
			"required": []string{"location"},
		},
		Execute: func(_ context.Context, argsJSON string) (string, error) {
			var args struct {
				Location string `json:"location"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}
			return fmt.Sprintf("Current time in %s: 2026-09-13T20:55:00+08:00", args.Location), nil
		},
	})

	fmt.Print("> ")
	result := ""
	var firstError error
	for ev := range ag.RunStream(ctx, "What's the weather in Beijing and what time is it there?") {
		switch ev.Kind {
		case agent.EventFinalAnswer:
			result = ev.Content
		case agent.EventError:
			if firstError == nil {
				firstError = ev.ToolError
			}
		}
	}
	if firstError != nil {
		log.Fatalf("agent run failed: %v", firstError)
	}
	fmt.Println(result)
}

func degree(unit string) string {
	if unit == "fahrenheit" {
		return "F"
	}
	return "C"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
