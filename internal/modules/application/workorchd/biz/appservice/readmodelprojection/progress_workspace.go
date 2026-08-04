package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MapWorkspaceTaskProgress(snapshot readmodelcore.WorkspaceSnapshotState, labels WorkspaceTaskProgressLabels) projectioncontract.TaskProgress {
	if selectedRun := selectWorkspaceOutcomeRun(snapshot); selectedRun != nil && IsTerminalRunState(selectedRun.State) {
		runSnapshot := readmodelcore.RunSnapshotState{
			Workspace:             snapshot.Workspace,
			Run:                   *selectedRun,
			Results:               snapshot.Results,
			Artifacts:             snapshot.Artifacts,
			ArtifactContinuations: snapshot.ArtifactContinuations,
			Patches:               snapshot.Patches,
			WebSources:            snapshot.WebSources,
			Decisions:             snapshot.Decisions,
			Approvals:             snapshot.Approvals,
			TaskDecompositions:    snapshot.TaskDecompositions,
			TaskPlans:             snapshot.TaskPlans,
			ExecutionSections:     snapshot.ExecutionSections,
			Acceptance:            snapshot.Acceptance,
		}
		return MapLocalizedTaskProgress(runSnapshot)
	}
	ret := newWorkspaceTaskProgressBase(snapshot, labels)
	applyWorkspaceProgressCurrentStep(&ret, snapshot)
	if ret.CurrentActionLabel == "" {
		ret.CurrentActionLabel = currentActionLabelFromWorkspaceSnapshot(snapshot)
	}
	applyWorkspaceProgressBlockers(&ret, snapshot)
	ret.Milestones = workspaceProgressMilestones(snapshot, labels)
	ret.Items = workspaceProgressItems(snapshot, labels)
	ret.CurrentMilestone = CurrentProgressMilestone(ret.Milestones)
	ret.EvidenceRefs = compactUniqueStrings(append(IDsFromResults(snapshot.Results), IDsFromArtifacts(snapshot.Artifacts)...))
	ret.AssessmentOutcome = WorkspaceTaskProgressAssessmentOutcome(snapshot)
	ret.Summary = TaskProgressSummary(ret, nil)
	return ret
}
