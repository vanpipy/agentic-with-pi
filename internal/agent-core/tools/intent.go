package tools

const AcceptLargeOutputField = "accept_large_output"
const AcceptLargeOutputDescription = "Default false; set true only when accepting the token cost of a withheld result."

func requireIntentSchema(name string, schema map[string]any) map[string]any {
	const intentField = "intent"
	const intentDescription = "Short label shown in the UI alongside the call. Strongly recommended: explains why this call is being made. If omitted, the UI falls back to '<tool_name> <args>'."
	if schema == nil {
		schema = map[string]any{}
	}
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
		schema["properties"] = props
	}
	if _, ok := props[intentField]; !ok {
		props[intentField] = map[string]any{
			"type":        "string",
			"description": intentDescription,
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
	return schema
}
