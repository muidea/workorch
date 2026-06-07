package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func MapWorkspaceOutcome(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.WorkspaceOutcome {
	ret := readmodelcore.WorkspaceOutcome{
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
	runSnapshot := readmodelcore.RunSnapshot{
		Workspace: snapshot.Workspace,
		Run:       *selectedRun,
		Results:   snapshot.Results,
		Artifacts: snapshot.Artifacts,
	}
	outcome := MapRunOutcome(runSnapshot)
	applyWorkspaceRunOutcome(&ret, outcome)
	return ret
}
