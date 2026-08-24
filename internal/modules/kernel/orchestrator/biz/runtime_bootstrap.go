package orchestrator

import (
	"context"
	"fmt"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) initializeRuntimeFromRun(ctx context.Context, run *runmodel.Run) (runtime.RuntimeAggregate, error) {
	if m == nil || m.runtimeController == nil || run == nil || run.ID == "" || run.WorkspaceID == "" || run.RuntimeBootstrap == nil {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	bootstrap := run.RuntimeBootstrap.Clone()
	// A first Runtime aggregate may only be created from an activation that
	// already sealed this binary's executable operation catalog. Do not repair
	// an old bootstrap here: persisted nonterminal state is checked by the
	// controller's typed fence, and a fresh Run must be explicitly activated.
	if run.RuntimeState == nil {
		if err := runtimecontract.ValidateRuntimeOperationContractRevision(bootstrap.Activation.ExecutionContract.OperationContractRevision); err != nil {
			return runtime.RuntimeAggregate{}, err
		}
	}
	if bootstrap.Definition.GoalID != run.GoalID || bootstrap.Definition.DoDID != run.DoDID || bootstrap.Definition.Revision != run.DefinitionRevision {
		return runtime.RuntimeAggregate{}, fmt.Errorf("%w: run identity does not match runtime bootstrap definition", common.ErrConflict)
	}
	facts, err := bootstrap.InitialFacts(run.WorkspaceID, run.ID)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	applied, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventActivationObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: run.WorkspaceID,
			RunID: run.ID}, Activation: &runtime.RuntimeInternalActivationPayload{ActivationFacts: &facts,
			ActivationDefinition:   &bootstrap.Definition,
			ActivationRequirements: append([]runtimecontract.RuntimeRequirement(nil), bootstrap.Requirements...)},
	})
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	return applied.Aggregate, nil
}
