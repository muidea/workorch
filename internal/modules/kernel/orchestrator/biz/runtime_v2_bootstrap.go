package orchestrator

import (
	"context"
	"fmt"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (o *RuntimeOrchestrator) InitializeRuntimeV2FromRun(ctx context.Context, run *runmodel.Run) (runtimev2.RuntimeAggregate, error) {
	if o == nil || o.runtimeV2Controller == nil || run == nil || run.ID == "" || run.WorkspaceID == "" || run.RuntimeBootstrap == nil {
		return runtimev2.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	bootstrap := run.RuntimeBootstrap.Clone()
	if bootstrap.SchemaVersion > runtimecontract.RuntimeStateSchemaVersionCurrent {
		return runtimev2.RuntimeAggregate{}, fmt.Errorf("unsupported future Runtime V2 bootstrap schema %d", bootstrap.SchemaVersion)
	}
	if bootstrap.Definition.GoalID != run.GoalID || bootstrap.Definition.DoDID != run.DoDID || bootstrap.Definition.Revision != run.DefinitionRevision {
		return runtimev2.RuntimeAggregate{}, fmt.Errorf("%w: run identity does not match runtime bootstrap definition", common.ErrConflict)
	}
	facts, err := bootstrap.InitialFacts(run.WorkspaceID, run.ID)
	if err != nil {
		return runtimev2.RuntimeAggregate{}, err
	}
	return o.runtimeV2Controller.Initialize(ctx, facts, bootstrap.Definition, bootstrap.Requirements)
}
