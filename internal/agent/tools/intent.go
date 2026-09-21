package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

const IntentField = "intent"
const IntentDescription = "Required short label shown in the UI: why this call is being made."
const AcceptLargeOutputField = "accept_large_output"
const AcceptLargeOutputDescription = "Default false; set true only when accepting the token cost of a withheld result."

type intentArgs struct {
	Intent string `json:"intent"`
}

func requireIntentSchema(name string, schema map[string]any) map[string]any {
	if schema == nil {
		schema = map[string]any{}
	}
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
		schema["properties"] = props
	}
	if _, ok := props[IntentField]; !ok {
		props[IntentField] = map[string]any{
			"type":        "string",
			"description": IntentDescription,
		}
	}
	if _, ok := props[AcceptLargeOutputField]; !ok {
		props[AcceptLargeOutputField] = map[string]any{
			"type":        "boolean",
			"default":     false,
			"description": AcceptLargeOutputDescription,
		}
	}
	if _, ok := schema["type"]; !ok {
		schema["type"] = "object"
	}
	requiredRaw, _ := schema["required"].([]string)
	required := make([]string, 0, len(requiredRaw)+1)
	required = append(required, requiredRaw...)
	hasIntent := false
	for _, r := range required {
		if r == IntentField {
			hasIntent = true
			break
		}
	}
	if !hasIntent {
		required = append(required, IntentField)
	}
	schema["required"] = required
	return schema
}

func requireIntentOrError(argsJSON string) error {
	var args intentArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Errorf("invalid args: %w", err)
	}
	if strings.TrimSpace(args.Intent) == "" {
		return fmt.Errorf("%s is required (%s)", IntentField, IntentDescription)
	}
	return nil
}
