package readmodelprojection

func taskNodeTitle(raw map[string]any) string {
	if title := readModelStringValue(raw["summary"]); title != "" {
		return title
	}
	input, _ := raw["input"].(map[string]any)
	if command := readModelStringValue(input["command"]); command != "" {
		return command
	}
	if tool := readModelStringValue(input["tool"]); tool != "" {
		return tool
	}
	if operation := readModelStringValue(input["operation"]); operation != "" {
		return operation
	}
	return readModelStringValue(raw["kind"])
}
