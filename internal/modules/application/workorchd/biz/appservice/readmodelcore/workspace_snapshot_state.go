package readmodelcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WorkspaceSnapshotState is the application-private instantiation of the
// canonical workspace snapshot layout. Owner models and Runtime aggregates are
// projected into bounded immutable values before this cacheable state is built.
type WorkspaceSnapshotState projectioncontract.WorkspaceSnapshotOf[appmodelcore.Workspace, appmodelcore.Run]

type WorkspaceExecutionActivationState struct {
	Workspace appmodelcore.Workspace `json:"workspace"`
	Run       appmodelcore.Run       `json:"run"`
	Snapshot  WorkspaceSnapshotState `json:"snapshot"`
}
