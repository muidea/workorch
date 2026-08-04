package appservice

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ProjectRunSnapshotDetail(snapshot readmodelcore.RunSnapshot) readmodelcore.RunSnapshot {
	return readmodelports.ProjectRunSnapshotDetail(snapshot)
}

func ProjectWorkspaceSnapshotDetail(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.WorkspaceSnapshot {
	return readmodelports.ProjectWorkspaceSnapshotDetail(snapshot)
}

func ProjectWorkspaceSnapshotSummary(snapshot readmodelcore.WorkspaceSnapshot) readmodelcore.WorkspaceSnapshot {
	return readmodelports.ProjectWorkspaceSnapshotSummary(snapshot)
}

func ProjectWorkspaceRuntimeView(snapshot readmodelcore.WorkspaceSnapshot, activityLimit int, beforeCursor string) workorchprojectioncontract.WorkspaceRuntimeView {
	return readmodelports.ProjectWorkspaceRuntimeView(snapshot, activityLimit, beforeCursor)
}

// ProjectWorkspaceRuntimeViewWithActivity builds RuntimeView using the store-owned
// activity feed so ActivityRevision is never regenerated on the HTTP/read path.
func ProjectWorkspaceRuntimeViewWithActivity(snapshot readmodelcore.WorkspaceSnapshot, storedActivity workorchprojectioncontract.WorkspaceActivityFeed, hasStoredActivity bool, activityLimit int, beforeCursor string) workorchprojectioncontract.WorkspaceRuntimeView {
	return readmodelports.ProjectWorkspaceRuntimeViewWithActivity(snapshot, storedActivity, hasStoredActivity, activityLimit, beforeCursor)
}

func ProjectDecisionView(item workorchprojectioncontract.Decision) workorchprojectioncontract.Decision {
	return readmodelports.ProjectDecisionView(item)
}
