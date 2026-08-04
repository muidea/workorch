package readmodelprojection

import (
	"strconv"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TaskProgressSummary(progress projectioncontract.TaskProgress, snapshot *readmodelcore.RunSnapshot) string {
	if runtimeSummary := RuntimeProgressSummary(progress.Runtime); runtimeSummary != "" {
		return runtimeSummary
	}
	if snapshot != nil {
		results := RunScopedResults(*snapshot)
		artifacts := RunScopedArtifacts(*snapshot)
		if result := PrimaryTaskProgressResult(results); result != nil && result.Summary != "" {
			return result.Summary
		}
		if snapshot.Run.FailureReason != "" {
			return snapshot.Run.FailureReason
		}
		changedFiles := AppliedPatchTargetCount(snapshot.Patches)
		summary := "run " + snapshot.Run.State + " with " + strconv.Itoa(len(snapshot.Steps)) + " steps, " + strconv.Itoa(len(results)) + " results, " + strconv.Itoa(len(artifacts)) + " artifacts"
		if changedFiles > 0 {
			summary += ", " + strconv.Itoa(changedFiles) + " changed files"
		}
		return summary
	}
	return "workspace " + progress.State + " with " + strconv.Itoa(len(progress.Blockers)) + " open blockers"
}
