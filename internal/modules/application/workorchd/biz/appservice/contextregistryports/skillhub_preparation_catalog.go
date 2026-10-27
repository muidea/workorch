package contextregistryports

import (
	"context"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// SkillHubPreparationCatalog keeps the whole observation, even on a metadata
// failure. It is not a readiness receipt or proof of daemon/local agreement.
type SkillHubPreparationCatalog struct {
	Observation    SkillHubPreparationInstructions
	Metadata       []skillhubpreparation.SkillHubPreparationMetadata
	Skills         []workorchmanagementcontract.Skill
	FailureCode    string
	FailureSkillID string
}

// This explicit operation obtains its own inventory and installed bytes; it
// never accepts a caller-supplied "verified" inventory or retries status.
func ObserveSkillHubPreparationCatalog(ctx context.Context, runner SkillHubPreparationRunner, workspaceID common.WorkspaceID, directory string, global bool) (SkillHubPreparationCatalog, error) {
	value := SkillHubPreparationCatalog{}
	var err error
	value.Observation, err = ObserveSkillHubPreparationInstructions(ctx, runner, workspaceID, directory, global)
	if err != nil {
		value.FailureCode = "installed_observation_failed"
		return value, err
	}
	index := 0
	scope, materialWorkspace := contextregistrycore.SkillScopeWorkspace, string(workspaceID)
	if global {
		scope, materialWorkspace = contextregistrycore.SkillScopeGlobal, ""
	}
	for _, item := range value.Observation.Inventory.Status.Items {
		if global && item.Agent != "codex" {
			continue
		}
		if err := ctx.Err(); err != nil {
			value.FailureCode = "cancelled"
			return value, err
		}
		if index >= len(value.Observation.Instructions) || value.Observation.Instructions[index].SkillID != item.SkillID {
			value.FailureCode = "instruction_mapping_conflict"
			return value, common.ErrConflict
		}
		instruction := value.Observation.Instructions[index]
		metadata, err := skillhubpreparation.ParsePreparationSkillHubMetadata(instruction.Raw)
		if err != nil {
			value.FailureCode, value.FailureSkillID = "invalid_metadata", item.SkillID
			return value, err
		}
		value.Metadata = append(value.Metadata, metadata)
		value.Skills = append(value.Skills, contextregistrycore.MapPreparationSkillHubItem(item, scope, materialWorkspace, value.Observation.Inventory.Status, instruction.Raw, metadata))
		index++
	}
	if index != len(value.Observation.Instructions) {
		value.FailureCode = "instruction_mapping_conflict"
		return value, common.ErrConflict
	}
	return value, nil
}
