package readmodelports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// ProjectWorkspaceRuntimeView builds the internal RuntimeView from a snapshot.
func ProjectWorkspaceRuntimeView(snapshot readmodelcore.WorkspaceSnapshotState, activityLimit int, beforeCursor string) projectioncontract.WorkspaceRuntimeView {
	return readmodelprojection.WorkspaceProjection().RuntimeView(snapshot, activityLimit, beforeCursor)
}

// ProjectWorkspaceRuntimeViewWithActivity builds RuntimeView using store-owned activity.
func ProjectWorkspaceRuntimeViewWithActivity(snapshot readmodelcore.WorkspaceSnapshotState, storedActivity projectioncontract.WorkspaceActivityFeed, hasStoredActivity bool, activityLimit int, beforeCursor string) projectioncontract.WorkspaceRuntimeView {
	return readmodelprojection.WorkspaceProjection().RuntimeViewWithActivity(snapshot, storedActivity, hasStoredActivity, activityLimit, beforeCursor)
}
