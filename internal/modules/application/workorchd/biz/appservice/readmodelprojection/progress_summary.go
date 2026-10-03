package readmodelprojection

import (
	"strconv"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func taskProgressSummary(progress projectioncontract.TaskProgress, snapshot *readmodelcore.RunSnapshotState) string {
	if snapshot != nil && isTerminalRunState(snapshot.Run.State) {
		result := exactOwnerTerminalResult(snapshot.Workspace, snapshot.Run, snapshot.Results)
		if result != nil && ((snapshot.Run.State == "completed" && result.State == "accepted") || result.State == "rejected") {
			if result.Summary != "" {
				return result.Summary
			}
		} else {
			summary := "formal result failed: terminal Run has no exact final Result aligned with the current startup seal"
			if snapshot.Run.FailureReason != "" {
				summary += " | " + snapshot.Run.FailureReason
			}
			if runtimeSummary := runtimeProgressSummary(progress.Runtime); runtimeSummary != "" {
				summary += " | " + runtimeSummary
			}
			return summary
		}
	}
	if runtimeSummary := runtimeProgressSummary(progress.Runtime); runtimeSummary != "" {
		return runtimeSummary
	}
	if snapshot != nil && snapshot.Run.ID != "" {
		results := runScopedResults(*snapshot)
		artifacts := runScopedArtifacts(*snapshot)
		if snapshot.Run.FailureReason != "" {
			return snapshot.Run.FailureReason
		}
		changedFiles := appliedPatchTargetCount(snapshot.Patches)
		summary := "run " + snapshot.Run.State + " with " + strconv.Itoa(len(snapshot.Steps)) + " steps, " + strconv.Itoa(len(results)) + " results, " + strconv.Itoa(len(artifacts)) + " artifacts"
		if changedFiles > 0 {
			summary += ", " + strconv.Itoa(changedFiles) + " changed files"
		}
		return summary
	}
	return "workspace " + progress.State + " with " + strconv.Itoa(len(progress.Blockers)) + " open blockers"
}
