package server

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WorkspaceProjectionWriter is the application-owned derived-write port used
// after an owner command has already succeeded. It cannot issue an owner
// command; all methods only update/rebuild the projection store.
type WorkspaceProjectionWriter interface {
	UpsertWorkspace(appmodelcore.Workspace)
	UpsertSnapshot(readmodelcore.WorkspaceSnapshotState)
	DeleteWorkspace(string)
	RebuildAll(context.Context) error
	RebuildWorkspace(context.Context, string) error
}

// WorkspaceProjectionBatchWriter is an optional optimization implemented by
// the Biz component. Implementations may expose only the single-workspace
// rebuild primitive; the server then uses the same bounded deduplication
// fallback without gaining a second store.
type WorkspaceProjectionBatchWriter interface {
	RebuildWorkspacesForDecisions(context.Context, []projectioncontract.Decision) error
	RebuildWorkspacesForApprovals(context.Context, []projectioncontract.Approval) error
}

// WorkspaceProjection is the transport-facing read port for the one
// Biz-owned projection component. It has no lifecycle, EventHub observation or
// derived-write method, so HTTP cannot start, replay or mutate the projection.
type WorkspaceProjection interface {
	Status() projectioncontract.ProjectionStatus
	Metrics() workspaceviewcore.Metrics
	ReconcileSourceHighWatermarks(context.Context) error
	ListWorkspaces() ([]appmodelcore.Workspace, projectioncontract.ProjectionStatus, error)
	ReadWorkspaceSnapshotWithContext(context.Context, string) (readmodelcore.WorkspaceSnapshotState, projectioncontract.ProjectionStatus, error)
	ReadWorkspaceSummarySnapshot(string) (readmodelcore.WorkspaceSnapshotState, projectioncontract.ProjectionStatus, error)
	ReadWorkspaceActivity(string) (projectioncontract.WorkspaceActivityFeed, bool)
}
