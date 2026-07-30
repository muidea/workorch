package orchestrator

import (
	"context"
	"fmt"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (o *RuntimeOrchestrator) InitializeRuntimeV2FromRun(ctx context.Context, run *runmodel.Run) (runtimev2.RuntimeAggregate, error) {
	if o == nil || o.runtimeV2Controller == nil || run == nil || run.ID == "" || run.WorkspaceID == "" || run.RuntimeBootstrap == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	bootstrap := run.RuntimeBootstrap.Clone()
	if err := runtimecontract.ValidateRuntimeStateSchemaVersion(bootstrap.SchemaVersion); err != nil {
		return runtimev2.RuntimeAggregate{}, fmt.Errorf("unsupported Runtime V2 bootstrap schema: %w", err)
	}
	// A first Runtime V2 aggregate may only be created from an activation that
	// already sealed this binary's executable operation catalog. Do not repair
	// an old bootstrap here: persisted nonterminal state is checked by the
	// controller's typed fence, and a fresh Run must be explicitly activated.
	if run.RuntimeState == nil {
		if err := runtimecontract.ValidateRuntimeOperationContractRevision(bootstrap.Activation.ExecutionContract.OperationContractRevision); err != nil {
			return runtimev2.RuntimeAggregate{}, err
		}
	}
	if bootstrap.Definition.GoalID != run.GoalID || bootstrap.Definition.DoDID != run.DoDID || bootstrap.Definition.Revision != run.DefinitionRevision {
		return runtimev2.RuntimeAggregate{}, fmt.Errorf("%w: run identity does not match runtime bootstrap definition", common.ErrConflict)
	}
	facts, err := bootstrap.InitialFacts(run.WorkspaceID, run.ID)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	applied, err := o.ApplyRuntimeV2InternalEvent(ctx, runtimev2.RuntimeInternalEvent{
		Kind:                   runtimev2.RuntimeInternalEventActivationObserved,
		WorkspaceID:            run.WorkspaceID,
		RunID:                  run.ID,
		ActivationFacts:        &facts,
		ActivationDefinition:   &bootstrap.Definition,
		ActivationRequirements: append([]runtimev2.RuntimeRequirement(nil), bootstrap.Requirements...),
	})
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	return applied.Aggregate, nil
}
