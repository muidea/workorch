package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func RunScopedResults(snapshot readmodelcore.RunSnapshotState) []projectioncontract.Result {
	return runScopedResults(snapshot)
}
