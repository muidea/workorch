package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func workspaceProgressMilestones(snapshot readmodelcore.WorkspaceSnapshot, labels WorkspaceTaskProgressLabels) []projectioncontract.ProgressMilestone {
	return []projectioncontract.ProgressMilestone{
		{ID: "workspace_defined", Title: labels.WorkspaceDefined, State: "completed", EvidenceRefs: []string{snapshot.Workspace.ID}},
		{ID: "formal_result", Title: labels.FormalResult, State: ProgressStateFromCount(len(snapshot.Results), false, snapshot.Workspace.State == string(common.WorkspaceStateActive)), EvidenceRefs: IDsFromResults(snapshot.Results)},
	}
}

func workspaceProgressItems(snapshot readmodelcore.WorkspaceSnapshot, labels WorkspaceTaskProgressLabels) []projectioncontract.ProgressItem {
	return []projectioncontract.ProgressItem{
		{ID: "runs", Kind: "run", Title: labels.Runs, State: ProgressStateFromCount(len(snapshot.Runs), false, false)},
		{ID: "results", Kind: "result", Title: labels.Results, State: ProgressStateFromCount(len(snapshot.Results), false, false), EvidenceRefs: IDsFromResults(snapshot.Results)},
		{ID: "artifacts", Kind: "artifact", Title: labels.Artifacts, State: ProgressStateFromCount(len(snapshot.Artifacts), false, false), EvidenceRefs: IDsFromArtifacts(snapshot.Artifacts)},
	}
}
