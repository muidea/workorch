package readmodelprojection

import (
	"strconv"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func taskProgressSummary(progress projectioncontract.TaskProgress, snapshot *readmodelcore.RunSnapshotState) string {
	if snapshot != nil && snapshot.Run.ID == "" && !isTerminalRunState(snapshot.Run.State) {
		return workspaceWithoutRunSummary(snapshot.Workspace)
	}
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
	if snapshot != nil {
		return workspaceWithoutRunSummary(snapshot.Workspace)
	}
	return "执行未启动；Result 未验收"
}

func workspaceWithoutRunSummary(workspace appmodelcore.Workspace) string {
	summary := "执行未启动；Result 未验收"
	if workspace.AuthoringSummary == nil {
		return summary
	}
	authoring := workspace.AuthoringSummary
	label := "创作状态：" + authoring.State
	switch authoring.State {
	case "failed":
		label = "创作失败"
	case "preparation_failed":
		label = "创作准备失败"
	case "eligible":
		label = "已具备执行准入"
	case "outcome_unresolved":
		label = "创作结果未决"
	}
	if authoring.FailurePhase != "" {
		label += "；失败阶段：" + authoring.FailurePhase
	}
	if authoring.FailureCode != "" {
		label += "；原因代码：" + authoring.FailureCode
	}
	return label + "；" + summary
}
