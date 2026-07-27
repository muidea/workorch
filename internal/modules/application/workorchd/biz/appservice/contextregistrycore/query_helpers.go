package contextregistrycore

import (
	"strings"

	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
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
		ret[events.PayloadKeyServerID] = value
	}
	if value := strings.TrimSpace(query.Kind); value != "" {
		ret[events.PayloadKeyKind] = value
	}
	if value := strings.TrimSpace(query.Category); value != "" {
		ret[events.PayloadKeyCategory] = value
	}
	if value := strings.TrimSpace(query.Stage); value != "" {
		ret[events.PayloadKeyStage] = value
	}
	if value := strings.TrimSpace(query.RiskLevel); value != "" {
		ret[events.PayloadKeyRisk] = value
	}
	if query.Enabled != nil {
		ret[events.PayloadKeyEnabled] = *query.Enabled
	}
	if value := strings.TrimSpace(query.Query); value != "" {
		ret[events.PayloadKeyQuery] = value
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
