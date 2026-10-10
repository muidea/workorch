package workspace

import (
	"context"
	"reflect"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Current formal definitions are visible only after the current confirmation
// journal materializes. A prepared successor cannot expose its partial rows.
func (m *Biz) authoringFormalDefinitionVisible(ctx context.Context, id common.WorkspaceID) (bool, error) {
	if m == nil || m.manager == nil || m.workspaces == nil || m.goals == nil || m.dods == nil {
		return false, common.ErrInvalidArgument
	}
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	current, err := m.workspaces.Get(ctx, id)
	if err != nil {
		return false, err
	}
	if current.DefinitionAuthoringConfirmation == nil {
		return true, nil
	}
	journal := current.DefinitionAuthoringConfirmation
	if journal.Validate(current.DefinitionAuthoring, id) != nil || journal.Phase != workspacemodel.DefinitionAuthoringConfirmationMaterialized || current.ValidateDefinitionAuthoringHistory() != nil {
		return false, nil
	}
	goals, err := m.goals.ListByWorkspaceID(ctx, id)
	if err != nil {
		return false, err
	}
	dods, err := m.dods.ListByWorkspaceID(ctx, id)
	if err != nil {
		return false, err
	}
	if len(goals) != len(current.DefinitionAuthoringHistory)+1 || len(dods) != len(current.DefinitionAuthoringHistory)+1 {
		return false, common.ErrConflict
	}
	goal, dod := currentGoalLeaf(goals, id), currentDoDLeaf(dods, id)
	if goal == nil || dod == nil || !reflect.DeepEqual(*goal, journal.Goal) || !reflect.DeepEqual(*dod, journal.DoD) {
		return false, common.ErrConflict
	}
	return true, nil
}
