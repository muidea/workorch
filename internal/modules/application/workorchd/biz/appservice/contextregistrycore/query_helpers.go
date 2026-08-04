package contextregistrycore

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func FindSkillByRef(items []Skill, id string, sourceSkillID string) (Skill, error) {
	id = strings.TrimSpace(id)
	sourceSkillID = strings.TrimSpace(sourceSkillID)
	for _, item := range items {
		if item.ID == id || (sourceSkillID != "" && item.SourceSkillID == sourceSkillID) {
			return item, nil
		}
	}
	return Skill{}, common.ErrNotFound
}

func MCPCapabilityQueryPayload(query MCPCapabilityQuery) map[string]any {
	ret := map[string]any{}
	if value := strings.TrimSpace(query.ServerID); value != "" {
		ret[recordpayload.PayloadKeyServerID] = value
	}
	if value := strings.TrimSpace(query.Kind); value != "" {
		ret[recordpayload.PayloadKeyKind] = value
	}
	if value := strings.TrimSpace(query.Category); value != "" {
		ret[recordpayload.PayloadKeyCategory] = value
	}
	if value := strings.TrimSpace(query.Stage); value != "" {
		ret[recordpayload.PayloadKeyStage] = value
	}
	if value := strings.TrimSpace(query.RiskLevel); value != "" {
		ret[recordpayload.PayloadKeyRisk] = value
	}
	if query.Enabled != nil {
		ret[recordpayload.PayloadKeyEnabled] = *query.Enabled
	}
	if value := strings.TrimSpace(query.Query); value != "" {
		ret[recordpayload.PayloadKeyQuery] = value
	}
	return ret
}

func MemorySearchOwnerQuery(query MemoryQuery) (memoryevents.MemorySearchQuery, error) {
	workspaceID := strings.TrimSpace(query.WorkspaceID)
	if workspaceID == "" {
		return memoryevents.MemorySearchQuery{}, common.ErrInvalidArgument
	}
	return memoryevents.MemorySearchQuery{
		WorkspaceID: common.WorkspaceID(workspaceID),
		Query:       strings.TrimSpace(query.Query),
		Tags:        CloneStrings(query.Tags),
		Types:       CloneStrings(query.Types),
		Limit:       query.Limit,
	}, nil
}
