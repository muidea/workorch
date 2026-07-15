package appservice

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"

func ProjectRunSnapshotDetail(snapshot RunSnapshot) RunSnapshot {
	return readmodelports.ProjectRunSnapshotDetail(snapshot)
}

func ProjectWorkspaceSnapshotDetail(snapshot WorkspaceSnapshot) WorkspaceSnapshot {
	return readmodelports.ProjectWorkspaceSnapshotDetail(snapshot)
}

func ProjectWorkspaceSnapshotSummary(snapshot WorkspaceSnapshot) WorkspaceSnapshot {
	return readmodelports.ProjectWorkspaceSnapshotSummary(snapshot)
}

func ProjectWorkspaceRuntimeView(snapshot WorkspaceSnapshot, activityLimit int, beforeCursor string) WorkspaceRuntimeView {
	return readmodelports.ProjectWorkspaceRuntimeView(snapshot, activityLimit, beforeCursor)
}

// ProjectWorkspaceRuntimeViewWithActivity builds RuntimeView using the store-owned
// activity feed so ActivityRevision is never regenerated on the HTTP/read path.
func ProjectWorkspaceRuntimeViewWithActivity(snapshot WorkspaceSnapshot, storedActivity WorkspaceActivityFeed, hasStoredActivity bool, activityLimit int, beforeCursor string) WorkspaceRuntimeView {
	return readmodelports.ProjectWorkspaceRuntimeViewWithActivity(snapshot, storedActivity, hasStoredActivity, activityLimit, beforeCursor)
}

func ProjectDecisionView(item Decision) Decision {
	return readmodelports.ProjectDecisionView(item)
}
