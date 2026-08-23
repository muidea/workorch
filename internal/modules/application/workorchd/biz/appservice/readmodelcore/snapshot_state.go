package readmodelcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// RunSnapshotState is the application-private instantiation of the canonical
// snapshot layout. Runtime aggregate input is projected before this bounded
// application read model is constructed and is never retained here.
type RunSnapshotState projectioncontract.RunSnapshotOf[appmodelcore.Workspace, appmodelcore.Run]
