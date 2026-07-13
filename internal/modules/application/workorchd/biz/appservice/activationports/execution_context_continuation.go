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
	if !changed {
		var err error
		ret.RejectedResult, err = RejectedResultContinuation(ctx, request.Hub, workspaceID, goalID, dodID)
		if err != nil {
			return runtimecontract.RuntimeContinuationContext{}, err
		}
		ret.Replan = runtimeV2ReplanContinuation(governanceHistory)
	}
	ret = ret.Normalized()
	if err := ret.Validate(); err != nil {
		return runtimecontract.RuntimeContinuationContext{}, err
	}
	return ret, nil
}

func runtimeV2ReplanContinuation(history []runtimecontract.RuntimeGovernanceHistoryEntry) *runtimecontract.RuntimeReplanContinuation {
	for idx := len(history) - 1; idx >= 0; idx-- {
		item := history[idx].Normalized()
		if item.ResolutionType != "replan" && item.RecoveryAction != "replan" {
			continue
		}
		ret := runtimecontract.RuntimeReplanContinuation{
			PredecessorRunID: item.RunID, TicketID: item.TicketID, TicketStepID: item.StepID,
			ResolutionID: item.ResolutionID, ResolutionType: item.ResolutionType, RecoveryAction: item.RecoveryAction,
			Reason: "resolved_governance_replan", ResolutionSummary: item.ResolutionSummary,
		}
		if item.Failure != nil {
			value := item.Failure.Clone()
			ret.Failure = &value
		}
		ret = ret.Normalized()
		return &ret
	}
	return nil
}
