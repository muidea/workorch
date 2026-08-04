package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func RuntimeCapabilityIdentity(toolCall projectioncontract.ToolCall) projectioncontract.RuntimeCapability {
	source := toolCall.Output
	if len(source) == 0 {
		source = toolCall.Arguments
	}
	serverID := readModelStringValue(source["serverId"])
	operation := readModelStringValue(source["operation"])
	kind, name := runtimeCapabilityKindAndName(operation, source)
	if serverID == "" || kind == "" || name == "" {
		return projectioncontract.RuntimeCapability{}
	}
	id := defaultRuntimeCapabilityID(serverID, kind, name)
	return projectioncontract.RuntimeCapability{
		ID:          id,
		ServerID:    serverID,
		Kind:        kind,
		Name:        name,
		DisplayName: name,
	}
}
