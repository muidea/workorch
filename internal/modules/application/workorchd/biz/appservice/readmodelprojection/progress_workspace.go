package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func MapWorkspaceTaskProgress(snapshot readmodelcore.WorkspaceSnapshot, labels WorkspaceTaskProgressLabels) readmodelcore.TaskProgress {
	if selectedRun := selectWorkspaceOutcomeRun(snapshot); selectedRun != nil && IsTerminalRunState(selectedRun.State) {
		runSnapshot := readmodelcore.RunSnapshot{
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
