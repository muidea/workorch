package activationports

import (
	"context"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ActiveRunProviderTargetStale reports whether a workspace-scoped resume must
// create a new Runtime V2 activation because the desired provider/model target
// differs from the target sealed into the existing Run.
func ActiveRunProviderTargetStale(
	ctx context.Context,
	run *runmodel.Run,
	requested *runtimecontract.ProviderModelTarget,
	workspaceDefault *runtimecontract.ProviderModelTarget,
) (bool, error) {
	if run == nil || run.RuntimeBootstrap == nil {
		return true, nil
	}
	desired, err := configuredActivationProviderTarget(ctx, requested, workspaceDefault)
	if err != nil {
		return false, err
	}
	return !run.RuntimeBootstrap.Activation.ProviderTarget.Same(desired), nil
}
