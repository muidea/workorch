package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func MapWorkspaceTaskProgress(snapshot readmodelcore.WorkspaceSnapshot, labels WorkspaceTaskProgressLabels) readmodelcore.TaskProgress {
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
