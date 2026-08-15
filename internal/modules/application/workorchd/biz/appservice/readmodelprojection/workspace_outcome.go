package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	workspacecontracts "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/contracts"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func workspaceOutcomeRunMatchesCurrentDefinitions(snapshot readmodelcore.WorkspaceSnapshotState, run appmodelcore.Run) bool {
	goalID, dodID := latestWorkspaceDefinitionIDs(snapshot)
	if goalID != "" && strings.TrimSpace(run.GoalID) != goalID {
		return false
	}
	if dodID != "" && strings.TrimSpace(run.DoDID) != dodID {
		return false
	}
	definitionRevision := latestWorkspaceDefinitionRevision(snapshot)
	if definitionRevision != "" && strings.TrimSpace(run.DefinitionRevision) != definitionRevision {
		return false
	}
	return true
}

func latestWorkspaceDefinitionRevision(snapshot readmodelcore.WorkspaceSnapshotState) string {
	goalID, dodID := latestWorkspaceDefinitionIDs(snapshot)
	var goalRevision string
	for idx := range snapshot.Goals {
		if strings.TrimSpace(string(snapshot.Goals[idx].ID)) == goalID {
			goalRevision = strings.TrimSpace(snapshot.Goals[idx].DefinitionRevision)
		}
	}
	var dodRevision string
	for idx := range snapshot.DoDs {
		if strings.TrimSpace(string(snapshot.DoDs[idx].ID)) == dodID {
			dodRevision = strings.TrimSpace(snapshot.DoDs[idx].DefinitionRevision)
		}
	}
	return workspacecontracts.ExecutionDefinitionRevision(goalID, goalRevision, dodID, dodRevision)
}

func mapWorkspaceOutcome(snapshot readmodelcore.WorkspaceSnapshotState) projectioncontract.WorkspaceOutcome {
	ret := projectioncontract.WorkspaceOutcome{
		WorkspaceID:       snapshot.Workspace.ID,
		State:             snapshot.Workspace.State,
		RawWorkspaceState: snapshot.Workspace.State,
	}
	selectedRun := selectWorkspaceOutcomeRun(snapshot)
	if selectedRun != nil && workspaceOutcomeRunMatchesCurrentDefinitions(snapshot, *selectedRun) {
		applySelectedWorkspaceRunOutcome(&ret, snapshot, *selectedRun)
		return ret
	}
	if applyWorkspaceDefinitionsChangedOutcome(&ret, snapshot) {
		return ret
	}
	if selectedRun == nil {
		return ret
	}
	applySelectedWorkspaceRunOutcome(&ret, snapshot, *selectedRun)
	return ret
}

func applySelectedWorkspaceRunOutcome(ret *projectioncontract.WorkspaceOutcome, snapshot readmodelcore.WorkspaceSnapshotState, selectedRun appmodelcore.Run) {
	runSnapshot := readmodelcore.RunSnapshotState{
		Workspace: snapshot.Workspace,
		Run:       selectedRun,
		Results:   snapshot.Results,
		Artifacts: snapshot.Artifacts,
	}
	outcome := mapRunOutcome(runSnapshot)
	applyWorkspaceRunOutcome(ret, outcome)
}
