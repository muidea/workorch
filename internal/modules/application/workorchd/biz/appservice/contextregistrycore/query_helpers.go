package contextregistrycore

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func FindSkillByRef(items []workorchmanagementcontract.Skill, id string, sourceSkillID string) (workorchmanagementcontract.Skill, error) {
	id = strings.TrimSpace(id)
	sourceSkillID = strings.TrimSpace(sourceSkillID)
	for _, item := range items {
		if item.ID == id || (sourceSkillID != "" && item.SourceSkillID == sourceSkillID) {
			return item, nil
		}
	}
	return workorchmanagementcontract.Skill{}, common.ErrNotFound
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

func MemorySearchOwnerQuery(query workorchmanagementcontract.MemorySearchQuery) (workorchquerycontract.MemorySearchQuery, error) {
	workspaceID := strings.TrimSpace(query.WorkspaceID)
	if workspaceID == "" {
		return workorchquerycontract.MemorySearchQuery{}, common.ErrInvalidArgument
	}
	return workorchquerycontract.MemorySearchQuery{
		WorkspaceID: common.WorkspaceID(workspaceID),
		Query:       strings.TrimSpace(query.Query),
		Tags:        CloneStrings(query.Tags),
		Types:       CloneStrings(query.Types),
		Limit:       query.Limit,
	}, nil
}
