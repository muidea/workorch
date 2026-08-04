package appservice

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ProjectRunSnapshotDetail(snapshot readmodelcore.RunSnapshotState) readmodelcore.RunSnapshotState {
	return readmodelports.ProjectRunSnapshotDetail(snapshot)
}

func PublicRunSnapshot(snapshot readmodelcore.RunSnapshotState) workorchprojectioncontract.RunSnapshot {
	return readmodelprojection.PublicRunSnapshot(snapshot)
}

func PublicWorkspaceSnapshot(snapshot readmodelcore.WorkspaceSnapshotState) workorchprojectioncontract.WorkspaceSnapshot {
	return readmodelprojection.PublicWorkspaceSnapshot(snapshot)
}

func PublicRunObservability(observability readmodelcore.RunObservabilityState) workorchprojectioncontract.RunObservability {
	return readmodelprojection.PublicRunObservability(observability)
}

func PublicWorkspaceExecutionActivation(activation readmodelcore.WorkspaceExecutionActivationState) workorchprojectioncontract.WorkspaceExecutionActivation {
	return readmodelprojection.PublicWorkspaceExecutionActivation(activation)
}

func ProjectWorkspaceSnapshotDetail(snapshot readmodelcore.WorkspaceSnapshotState) readmodelcore.WorkspaceSnapshotState {
	return readmodelports.ProjectWorkspaceSnapshotDetail(snapshot)
}

func ProjectWorkspaceSnapshotSummary(snapshot readmodelcore.WorkspaceSnapshotState) readmodelcore.WorkspaceSnapshotState {
	return readmodelports.ProjectWorkspaceSnapshotSummary(snapshot)
}

func ProjectWorkspaceRuntimeView(snapshot readmodelcore.WorkspaceSnapshotState, activityLimit int, beforeCursor string) workorchprojectioncontract.WorkspaceRuntimeView {
	return readmodelports.ProjectWorkspaceRuntimeView(snapshot, activityLimit, beforeCursor)
}

// ProjectWorkspaceRuntimeViewWithActivity builds RuntimeView using the store-owned
// activity feed so ActivityRevision is never regenerated on the HTTP/read path.
func ProjectWorkspaceRuntimeViewWithActivity(snapshot readmodelcore.WorkspaceSnapshotState, storedActivity workorchprojectioncontract.WorkspaceActivityFeed, hasStoredActivity bool, activityLimit int, beforeCursor string) workorchprojectioncontract.WorkspaceRuntimeView {
	return readmodelports.ProjectWorkspaceRuntimeViewWithActivity(snapshot, storedActivity, hasStoredActivity, activityLimit, beforeCursor)
}

func ProjectDecisionView(item workorchprojectioncontract.Decision) workorchprojectioncontract.Decision {
	return readmodelports.ProjectDecisionView(item)
}
