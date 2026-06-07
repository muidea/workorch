package readmodelprojection

import "strings"

func ProjectStepOutput(kind string, output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}
	ret := map[string]any{}
	for key, value := range output {
		if !isStepOutputAllowedKey(key) {
			continue
		}
		ret[key] = value
	}
	if strings.TrimSpace(kind) == "mcp/tool_call" {
		if result := projectMCPToolCallResult(output["result"]); len(result) > 0 {
			ret["result"] = result
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
