package readmodelcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// RunObservabilityState retains the application-private Run while sharing the
// canonical observability field layout.
type RunObservabilityState projectioncontract.RunObservabilityOf[appmodelcore.Run]
