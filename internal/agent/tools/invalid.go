package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/vanpiyp/awp/internal/agent"
)

type invalidArgs struct {
	Tool   string `json:"tool"`
	Reason string `json:"reason"`
	Intent string `json:"intent"`
}

func InvalidTool() agent.Tool {
	return agent.Tool{
		Name:        agent.InvalidToolName,
		Description: "Report an invalid tool invocation. Use only when a tool call is malformed (missing argument, wrong type, semantic error). The agent records the report and continues instead of aborting.",
		Parameters:  invalidSchema(),
		Execute:     invalidExecute,
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

func invalidExecute(_ context.Context, argsJSON string) (string, error) {
	if err := requireIntentOrError(argsJSON); err != nil {
		return "", err
	}
	var args invalidArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if args.Tool == "" {
		return "", fmt.Errorf("tool name is required")
	}
	if args.Reason == "" {
		return "", fmt.Errorf("reason is required")
	}
	return fmt.Sprintf("Recorded invalid invocation of %q: %s", args.Tool, args.Reason), nil
}
