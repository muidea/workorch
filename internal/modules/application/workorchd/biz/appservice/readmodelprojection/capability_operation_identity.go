package readmodelprojection

import mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"

func runtimeCapabilityKindAndName(operation string, source map[string]any) (string, string) {
	switch operation {
	case "mcp/tool_call":
		return mcpmodel.CapabilityKindTool, readModelStringValue(source["tool"])
	case "mcp/resource_read":
		return mcpmodel.CapabilityKindResource, readModelStringValue(source["uri"])
	case "mcp/prompt_get":
		return mcpmodel.CapabilityKindPrompt, readModelStringValue(source["prompt"])
	default:
		return "", ""
	}
}
