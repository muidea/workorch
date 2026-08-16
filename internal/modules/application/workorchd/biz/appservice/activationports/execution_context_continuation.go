package activationports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mergeWorkspaceExecutionContinuations(
	definitionChangeImpact map[string]any,
	governanceHistory []runtimecontract.RuntimeGovernanceHistoryEntry,
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
