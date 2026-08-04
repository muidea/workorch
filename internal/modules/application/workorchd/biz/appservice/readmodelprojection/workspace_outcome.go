package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MapWorkspaceOutcome(snapshot readmodelcore.WorkspaceSnapshotState) projectioncontract.WorkspaceOutcome {
	ret := projectioncontract.WorkspaceOutcome{
		WorkspaceID:       snapshot.Workspace.ID,
		State:             snapshot.Workspace.State,
		RawWorkspaceState: snapshot.Workspace.State,
	}
	if applyWorkspaceDefinitionsChangedOutcome(&ret, snapshot) {
		return ret
	}
	selectedRun := selectWorkspaceOutcomeRun(snapshot)
	if selectedRun == nil {
		return ret
	}
	runSnapshot := readmodelcore.RunSnapshotState{
		Workspace: snapshot.Workspace,
		Run:       *selectedRun,
		Results:   snapshot.Results,
		Artifacts: snapshot.Artifacts,
	}
	outcome := MapRunOutcome(runSnapshot)
	applyWorkspaceRunOutcome(&ret, outcome)
	return ret
}
