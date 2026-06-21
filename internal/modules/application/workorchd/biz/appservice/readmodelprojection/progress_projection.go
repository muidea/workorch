package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func MapTaskProgress(snapshot readmodelcore.RunSnapshot, labels RunTaskProgressLabels) readmodelcore.TaskProgress {
	latestCheckpoint := LatestRuntimeCheckpoint(snapshot.RuntimeCheckpoints)
	results := RunScopedResults(snapshot)
	artifacts := RunScopedArtifacts(snapshot)
	ret := readmodelcore.TaskProgress{
		WorkspaceID: snapshot.Run.WorkspaceID,
		RunID:       snapshot.Run.ID,
		State:       snapshot.Run.State,
		Recovery: readmodelcore.RecoveryPlan{
			Mode:       "resume",
			NextAction: labels.RecoveryNextAction,
			RunID:      snapshot.Run.ID,
		},
	}
	applyTaskProgressCurrentStep(&ret, snapshot)
	if latestCheckpoint != nil {
		ret.Recovery.CheckpointID = latestCheckpoint.ID
	}

	ret.Blockers = TaskProgressBlockers(snapshot)
	ret.Milestones = TaskProgressMilestones(snapshot, labels, results, artifacts)
	ret.Items = TaskProgressItems(snapshot)
	ret.CurrentMilestone = CurrentProgressMilestone(ret.Milestones)
	ret.EvidenceRefs = TaskProgressEvidenceRefs(snapshot)
	ret.Recovery = TaskProgressRecovery(snapshot, latestCheckpoint, ret.Blockers, ret.Recovery)
	ret.BlockingTicketID = ret.Recovery.BlockingTicketID
	ret.BlockingApprovalID = ret.Recovery.BlockingApprovalID
	ret.RepairLoop = MapTaskProgressRepairLoop(snapshot, ret.Recovery)
	ret.Summary = TaskProgressSummary(ret, &snapshot)
	return ret
}
