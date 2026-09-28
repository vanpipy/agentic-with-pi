package agentcore

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/vanpiyp/awp/internal/llm"
)

func (a *Agent) preflightValidate(tc llm.ToolCall) string {
	tool, ok := a.findTool(tc.Function.Name)
	if !ok {
		return ""
	}
	schema := tool.Parameters()
	if schema == nil {
		return ""
	}
	required, _ := schema["required"].([]string)
	if len(required) == 0 {
		return ""
	}
	var got map[string]any
	if tc.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &got); err != nil {
			return ""
		}
	}
	missing := []string{}
	for _, name := range required {
		v, present := got[name]
		if !present {
			missing = append(missing, name)
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	parts := []string{}
	for _, name := range missing {
		props, _ := schema["properties"].(map[string]any)
		if prop, ok := props[name].(map[string]any); ok {
			if desc, ok := prop["description"].(string); ok {
				parts = append(parts, fmt.Sprintf("%s: %s", name, desc))
				continue
			}
		}
		parts = append(parts, name)
	}
	return fmt.Sprintf("Tool %s: missing required field(s) [%s]. Pass them as a JSON object argument. Example: {\"%s\": \"<value>\"}. Field descriptions: %s",
		tc.Function.Name, strings.Join(missing, ", "), missing[0], strings.Join(parts, "; "))
}

func (a *Agent) checkPreflightStreak() error {
	if len(a.preflightFailureStreak) == 0 {
		return nil
	}
	names := make([]string, 0, len(a.preflightFailureStreak))
	for name := range a.preflightFailureStreak {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		streak := a.preflightFailureStreak[name]
		if streak >= PreflightAbortThreshold {
			return fmt.Errorf("Tool %s has produced invalid arguments %d turns in a row. The model cannot generate valid args for this tool. Stop calling it and either pick a different tool or ask the user for guidance.", name, streak)
		}
	}
	return nil
}
