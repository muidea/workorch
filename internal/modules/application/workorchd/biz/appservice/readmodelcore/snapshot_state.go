package readmodelcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// RunSnapshotState is the application-private instantiation of the canonical
// snapshot layout. appmodelcore.Run may retain private RuntimeState and must be
// projected before crossing a protocol boundary.
type RunSnapshotState projectioncontract.RunSnapshotOf[appmodelcore.Workspace, appmodelcore.Run]
