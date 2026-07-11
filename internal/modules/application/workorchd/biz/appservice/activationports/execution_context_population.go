package activationports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
)

func appendSoulRoutingContext(contextData map[string]any, defaultSoul *soulmodel.Soul, soulRouting appmodelcore.SoulRouting) {
	if defaultSoul != nil {
		contextData["soulId"] = string(defaultSoul.ID)
		contextData["soul"] = activationcore.SoulContext(defaultSoul)
	}
	contextData["soulRouting"] = soulRouting
}

func appendSkillSelectionContext(contextData map[string]any, skillResult WorkspaceSkillSelectionResult) {
	contextData["skillSelection"] = skillResult.Evidence
	if len(skillResult.Skills) > 0 {
		contextData["skillIds"] = skillResult.IDs
		contextData["skills"] = activationcore.SkillContexts(skillResult.Skills)
	}
}

func appendMCPSelectionContext(contextData map[string]any, mcpSelection WorkspaceMCPSelectionResult) {
	contextData["mcpSelection"] = mcpSelection.Context()
	if len(mcpSelection.Servers) > 0 {
		contextData["mcpServerIds"] = mcpSelection.MCPServerIDs
		contextData["mcpServers"] = activationcore.MCPServerContexts(mcpSelection.Servers)
	}
	if len(mcpSelection.Capabilities) > 0 {
		contextData["capabilityIds"] = mcpSelection.CapabilityIDs
		contextData["mcpCapabilities"] = activationcore.MCPCapabilityContexts(mcpSelection.Capabilities)
	}
}

func appendMemorySelectionContext(contextData map[string]any, memories []*memorymodel.MemoryItem, retrievalEvidence map[string]any) {
	if retrievalEvidence != nil {
		contextData["memoryRetrieval"] = retrievalEvidence
	}
	if len(memories) > 0 {
		contextData["memoryIds"] = activationcore.MemoryItemIDs(memories)
		contextData["memories"] = activationcore.MemoryContexts(memories)
	}
}
