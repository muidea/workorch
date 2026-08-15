package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// contextProjector is a typed, stateless read-only facade table.
type contextProjector struct {
	RunBundle             func(readmodelcore.RunSnapshotState) projectioncontract.ContextBundle
	WorkspaceBundle       func(readmodelcore.WorkspaceSnapshotState, *readmodelcore.RunSnapshotState) projectioncontract.ContextBundle
	CompressedRunContext  func(readmodelcore.RunSnapshotState) projectioncontract.CompressedContext
	LocalizedTaskProgress func(readmodelcore.RunSnapshotState) projectioncontract.TaskProgress
}

// ContextProjection returns the one typed application-facing context facade.
func ContextProjection() contextProjector {
	return contextProjector{
		RunBundle:             mapRunContextBundle,
		WorkspaceBundle:       workspaceContextBundleFromSnapshots,
		CompressedRunContext:  compressedContextFromLocalizedRunSnapshot,
		LocalizedTaskProgress: mapLocalizedTaskProgress,
	}
}
