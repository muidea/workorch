package run

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

type runBizTestBoundRuns struct {
	repository.RunRepository
	binding repository.RunOwnerRepositoryBinding
}

func (r runBizTestBoundRuns) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	return r.binding
}

func (r runBizTestBoundRuns) ReadWorkspaceExecutionIntentReceipt(ctx context.Context, workspaceID common.WorkspaceID, requestID string) (*runmodel.WorkspaceExecutionIntentReceipt, error) {
	reader, ok := r.RunRepository.(repository.RunExecutionIntentReceiptReader)
	if !ok {
		return nil, common.ErrConflict
	}
	return reader.ReadWorkspaceExecutionIntentReceipt(ctx, workspaceID, requestID)
}

type runBizTestBoundSteps struct {
	repository.StepRepository
	binding repository.RunOwnerRepositoryBinding
}

func (r runBizTestBoundSteps) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	return r.binding
}

type runBizTestBoundCheckpoints struct {
	repository.RuntimeCheckpointRepository
	binding repository.RunOwnerRepositoryBinding
}

func (r runBizTestBoundCheckpoints) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	return r.binding
}

type runBizTestPurger struct {
	binding repository.RunOwnerRepositoryBinding
}

func (p runBizTestPurger) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	return p.binding
}

func (runBizTestPurger) PurgeWorkspaceResources(context.Context, runmodel.WorkspaceRetirementFence) (repository.RunWorkspaceResourcePurgeReceipt, error) {
	return repository.RunWorkspaceResourcePurgeReceipt{}, common.ErrInvalidArgument
}

func runBizTestDependencies(
	runs repository.RunRepository,
	steps repository.StepRepository,
	checkpoints repository.RuntimeCheckpointRepository,
	hub magicevent.Hub,
	background magictask.BackgroundRoutine,
) Dependencies {
	if steps == nil {
		steps = memoryrepo.NewStepRepository()
	}
	if checkpoints == nil {
		checkpoints = memoryrepo.NewRuntimeCheckpointRepository()
	}
	binding := repository.NewRunOwnerRepositoryBinding()
	owner, err := repository.NewRunOwnerRepositories(
		runBizTestBoundRuns{RunRepository: runs, binding: binding},
		runBizTestBoundSteps{StepRepository: steps, binding: binding},
		runBizTestBoundCheckpoints{RuntimeCheckpointRepository: checkpoints, binding: binding},
		runBizTestPurger{binding: binding},
	)
	if err != nil {
		panic(err)
	}
	return Dependencies{OwnerRepositories: owner, EventHub: hub, BackgroundRoutine: background}
}
