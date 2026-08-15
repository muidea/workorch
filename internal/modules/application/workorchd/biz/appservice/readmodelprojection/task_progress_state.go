package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: progress_state.go

func stepProgressState(snapshot readmodelcore.RunSnapshotState) string {
	if hasAcceptedRunResult(snapshot) && snapshot.Run.State == string(common.RunStateCompleted) {
		return "completed"
	}
	// Terminal run state always wins over partial evidence counters.
	if snapshot.Run.State == string(common.RunStateFailed) {
		return "failed"
	}
	if snapshot.Run.State == string(common.RunStateCompleted) {
		return "completed"
	}
	if len(snapshot.Steps) == 0 {
		if snapshot.TaskProgress.Runtime != nil {
			summary := snapshot.TaskProgress.Runtime
			if summary.CompletedDispatchCount > 0 || summary.AcceptedEvidenceCount > 0 || summary.CandidateAttemptCount > 0 || summary.ActiveDispatchID != "" {
				return "running"
			}
		}
		return "pending"
	}
	for _, item := range snapshot.Steps {
		if item.State == string(common.StepStateFailed) {
			return "failed"
		}
	}
	if mapRunProgress(snapshot).StepCompleted == len(snapshot.Steps) && len(snapshot.Steps) > 0 {
		return "completed"
	}
	return "running"
}

// source: progress_validation_state.go

func validationProgressState(snapshot readmodelcore.RunSnapshotState) string {
	if len(snapshot.ValidationResults) == 0 {
		if snapshot.Run.State == string(common.RunStateActive) {
			return "running"
		}
		return "pending"
	}
	if hasAcceptedRunResult(snapshot) && snapshot.Run.State == string(common.RunStateCompleted) {
		return "completed"
	}
	for _, item := range snapshot.ValidationResults {
		if item.State == string(common.ValidationResultStateFailed) {
			return "failed"
		}
	}
	return "completed"
}
