package readmodelports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// ProjectWorkspaceRuntimeView builds the internal RuntimeView from a snapshot.
func ProjectWorkspaceRuntimeView(snapshot readmodelcore.WorkspaceSnapshotState, activityLimit int, beforeCursor string) projectioncontract.WorkspaceRuntimeView {
	return readmodelprojection.ProjectWorkspaceRuntimeView(snapshot, activityLimit, beforeCursor)
}

// ProjectWorkspaceRuntimeViewWithActivity builds RuntimeView using store-owned activity.
func ProjectWorkspaceRuntimeViewWithActivity(snapshot readmodelcore.WorkspaceSnapshotState, storedActivity projectioncontract.WorkspaceActivityFeed, hasStoredActivity bool, activityLimit int, beforeCursor string) projectioncontract.WorkspaceRuntimeView {
	return readmodelprojection.ProjectWorkspaceRuntimeViewWithActivity(snapshot, storedActivity, hasStoredActivity, activityLimit, beforeCursor)
}

// ProjectAndPortWorkspaceRuntimeView builds RuntimeView from a snapshot and maps it to port.
func ProjectAndPortWorkspaceRuntimeView(snapshot readmodelcore.WorkspaceSnapshotState, activityLimit int, beforeCursor string) port.WorkspaceRuntimeViewInfo {
	view := readmodelprojection.ProjectWorkspaceRuntimeView(snapshot, activityLimit, beforeCursor)
	return port.WorkspaceRuntimeViewInfoFromProjection(view)
}

// ProjectAndPortWorkspaceRuntimeViewWithActivity uses store-owned activity authority.
func ProjectAndPortWorkspaceRuntimeViewWithActivity(snapshot readmodelcore.WorkspaceSnapshotState, storedActivity projectioncontract.WorkspaceActivityFeed, hasStoredActivity bool, activityLimit int, beforeCursor string) port.WorkspaceRuntimeViewInfo {
	view := readmodelprojection.ProjectWorkspaceRuntimeViewWithActivity(snapshot, storedActivity, hasStoredActivity, activityLimit, beforeCursor)
	return port.WorkspaceRuntimeViewInfoFromProjection(view)
}
