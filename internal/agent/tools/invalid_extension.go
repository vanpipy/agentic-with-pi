package tools

import (
	"context"
	"fmt"

	"github.com/vanpiyp/awp/internal/agent"
)

type invalidArgs struct {
	Tool   string `json:"tool"`
	Reason string `json:"reason"`
}

func InvalidTool() agent.Tool {
	return agent.ToolFunc{
		N: agent.InvalidToolName,
		D: "Report an invalid tool invocation. Use only when a tool call is malformed (missing argument, wrong type, semantic error). The agent records the report and continues instead of aborting.",
		P: invalidSchema(),
		Fn: func(_ context.Context, argsJSON string) (string, error) {
			return agent.RunTool(context.TODO(), argsJSON, func(_ context.Context, args invalidArgs) (string, error) {
				if args.Tool == "" {
					return "", fmt.Errorf("tool name is required")
				}
				if args.Reason == "" {
					return "", fmt.Errorf("reason is required")
				}
				return fmt.Sprintf("Recorded invalid invocation of %q: %s", args.Tool, args.Reason), nil
			})
		},
	}
}

func invalidSchema() map[string]any {
	return requireIntentSchema(agent.InvalidToolName, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tool":   map[string]any{"type": "string", "description": "REQUIRED. Name of the tool that was called incorrectly."},
			"reason": map[string]any{"type": "string", "description": "REQUIRED. What was wrong with the call (missing argument, wrong type, semantic error, etc.)."},
		},
		"required": []string{"tool", "reason"},
	})
}
