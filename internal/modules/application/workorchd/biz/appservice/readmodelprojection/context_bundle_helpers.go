package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func ContextBundleSummary(snapshot readmodelcore.RunSnapshot, progress readmodelcore.TaskProgress) string {
	if progress.Summary != "" {
		return progress.Summary
	}
	return "run " + snapshot.Run.State + " context bundle for recovery and compression"
}

func CurrentStepID(snapshot readmodelcore.RunSnapshot) string {
	if result := TerminalTaskProgressResult(snapshot); result != nil {
		return firstNonEmptyTimelineString(result.SourceRef, result.ID)
	}
	if snapshot.Step == nil {
		return ""
	}
	return snapshot.Step.ID
}

func TerminalTaskProgressResult(snapshot readmodelcore.RunSnapshot) *readmodelcore.Result {
	if !IsTerminalRunState(snapshot.Run.State) {
		return nil
	}
	return PrimaryTaskProgressResult(RunScopedResults(snapshot))
}
