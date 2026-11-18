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
	current, err := m.getWorkspaceReadOnly(ctx, id)
	if err != nil {
		return false, err
	}
	_, _, visible, err := m.formalDefinitionVisibilityAndRowsLocked(ctx, id, current, false, false)
	return visible, err
}

func (m *Biz) readVisibleWorkspaceFormalDefinitions(ctx context.Context, id common.WorkspaceID, needGoals, needDoDs bool) ([]*workspacemodel.Goal, []*workspacemodel.DoD, bool, error) {
	if m == nil || m.manager == nil || m.workspaces == nil || m.goals == nil || m.dods == nil || id == "" {
		return nil, nil, false, common.ErrInvalidArgument
	}
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	current, err := m.getWorkspaceReadOnly(ctx, id)
	if err != nil {
		return nil, nil, false, err
	}
	if current.Deletion != nil {
		if err := current.Deletion.Validate(current.ID); err != nil {
			return nil, nil, false, common.ErrConflict
		}
		if workspaceDeletionCompleted(current) {
			return nil, nil, false, nil
		}
	}
	return m.formalDefinitionVisibilityAndRowsLocked(ctx, id, current, needGoals, needDoDs)
}

func (m *Biz) formalDefinitionVisibilityAndRowsLocked(ctx context.Context, id common.WorkspaceID, current *workspacemodel.Workspace, needGoals, needDoDs bool) ([]*workspacemodel.Goal, []*workspacemodel.DoD, bool, error) {
	if current.DefinitionAuthoringConfirmation == nil {
		var goals []*workspacemodel.Goal
		var dods []*workspacemodel.DoD
		var err error
		if needGoals {
			goals, err = m.goals.ListByWorkspaceID(ctx, id)
			if err != nil {
				return nil, nil, false, err
			}
		}
		if needDoDs {
			dods, err = m.dods.ListByWorkspaceID(ctx, id)
			if err != nil {
				return nil, nil, false, err
			}
		}
		return goals, dods, true, nil
	}
	journal := current.DefinitionAuthoringConfirmation
	if current.ID != id || journal.Phase != workspacemodel.DefinitionAuthoringConfirmationMaterialized {
		return nil, nil, false, nil
	}
	if current.Revision > 0 && m.manager.formalReadCache != nil {
		if cached, ok := m.manager.formalReadCache[id]; ok &&
			cached.workspaceRevision == current.Revision &&
			cached.historyLen == len(current.DefinitionAuthoringHistory) &&
			reflect.DeepEqual(cached.goal, journal.Goal) &&
			reflect.DeepEqual(cached.dod, journal.DoD) {
			return cached.goals, cached.dods, true, nil
		}
	}
	if current.ReadCommittedAuthoring() != nil {
		return nil, nil, false, nil
	}
	goals, err := m.goals.ListByWorkspaceID(ctx, id)
	if err != nil {
		return nil, nil, false, err
	}
	dods, err := m.dods.ListByWorkspaceID(ctx, id)
	if err != nil {
		return nil, nil, false, err
	}
	if len(goals) != len(current.DefinitionAuthoringHistory)+1 || len(dods) != len(current.DefinitionAuthoringHistory)+1 {
		return nil, nil, false, common.ErrConflict
	}
	goal, dod := currentGoalLeaf(goals, id), currentDoDLeaf(dods, id)
	if goal == nil || dod == nil || !reflect.DeepEqual(*goal, journal.Goal) || !reflect.DeepEqual(*dod, journal.DoD) {
		return nil, nil, false, common.ErrConflict
	}
	if current.Revision > 0 {
		if m.manager.formalReadCache == nil {
			m.manager.formalReadCache = map[common.WorkspaceID]formalDefinitionReadCacheEntry{}
		}
		m.manager.formalReadCache[id] = formalDefinitionReadCacheEntry{
			workspaceRevision: current.Revision,
			historyLen:        len(current.DefinitionAuthoringHistory),
			goal:              *workspacemodel.CloneGoal(&journal.Goal),
			dod:               *workspacemodel.CloneDoD(&journal.DoD),
			goals:             goals,
			dods:              dods,
		}
	}
	return goals, dods, true, nil
}
