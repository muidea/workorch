package activationports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mergeWorkspaceExecutionContinuations(
	ctx context.Context,
	workspaceID common.WorkspaceID,
	goalID common.GoalID,
	dodID common.DoDID,
	definitionChangeImpact map[string]any,
	governanceHistory []runtimecontract.RuntimeGovernanceHistoryEntry,
	request WorkspaceRuntimeActivationRequest,
) (runtimecontract.RuntimeContinuationContext, error) {
	changed := activationcore.DefinitionChangeImpactChanged(definitionChangeImpact)
	ret := runtimecontract.RuntimeContinuationContext{
		DefinitionChangeSupersedesPrior: changed,
		GovernanceHistory:               governanceHistory,
	}
	ret = ret.Normalized()
	if err := ret.Validate(); err != nil {
		return runtimecontract.RuntimeContinuationContext{}, err
	}
	return ret, nil
}
