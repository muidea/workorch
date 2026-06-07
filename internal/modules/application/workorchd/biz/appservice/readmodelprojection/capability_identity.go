package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func RuntimeCapabilityIdentity(toolCall readmodelcore.ToolCall) readmodelcore.RuntimeCapability {
	source := toolCall.Output
	if len(source) == 0 {
		source = toolCall.Arguments
	}
	serverID := readModelStringValue(source["serverId"])
	operation := readModelStringValue(source["operation"])
	kind, name := runtimeCapabilityKindAndName(operation, source)
	if serverID == "" || kind == "" || name == "" {
		return readmodelcore.RuntimeCapability{}
	}
	id := defaultRuntimeCapabilityID(serverID, kind, name)
	return readmodelcore.RuntimeCapability{
		ID:          id,
		ServerID:    serverID,
		Kind:        kind,
		Name:        name,
		DisplayName: name,
	}
}
