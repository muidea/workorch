package activationports

import (
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RejectActiveRunProviderTargetOverride preserves the Run's sealed provider
// target. A workspace default is intentionally not an argument: it applies to
// the next activation only and must never turn Resume into a replacement Run.
// Even an equal target is rejected: Resume is scope-only and must not accept a
// provider/model override channel that could later become a mutation path.
func RejectActiveRunProviderTargetOverride(run *runmodel.Run, requested *runtimecontract.ProviderModelTarget) error {
	if run == nil || run.RuntimeBootstrap == nil {
		return common.ErrInvalidArgument
	}
	if requested == nil {
		return nil
	}
	return common.ErrConflict
}
