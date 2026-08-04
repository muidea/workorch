package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ContextBundleSummary(snapshot readmodelcore.RunSnapshotState, progress projectioncontract.TaskProgress) string {
	if progress.Summary != "" {
		return progress.Summary
	}
	return "run " + snapshot.Run.State + " context bundle for recovery and compression"
}

func CurrentStepID(snapshot readmodelcore.RunSnapshotState) string {
	if result := TerminalTaskProgressResult(snapshot); result != nil {
		return firstNonEmptyTimelineString(result.SourceRef, result.ID)
	}
	if snapshot.Step == nil {
		return ""
	}
	return snapshot.Step.ID
}

func TerminalTaskProgressResult(snapshot readmodelcore.RunSnapshotState) *projectioncontract.Result {
	if !IsTerminalRunState(snapshot.Run.State) {
		return nil
	}
	return PrimaryTaskProgressResult(RunScopedResults(snapshot))
}
