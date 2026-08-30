package run

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

type runTestBoundRuns struct {
	repository.RunRepository
	binding repository.RunOwnerRepositoryBinding
}

func (r runTestBoundRuns) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	return r.binding
}

type runTestBoundSteps struct {
	repository.StepRepository
	binding repository.RunOwnerRepositoryBinding
}

func (r runTestBoundSteps) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	return r.binding
}

type runTestBoundCheckpoints struct {
	repository.RuntimeCheckpointRepository
	binding repository.RunOwnerRepositoryBinding
}

func (r runTestBoundCheckpoints) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	return r.binding
}

type runTestPurger struct {
	binding repository.RunOwnerRepositoryBinding
}

func (p runTestPurger) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	return p.binding
}

func (runTestPurger) PurgeWorkspaceResources(context.Context, runmodel.WorkspaceRetirementFence) (repository.RunWorkspaceResourcePurgeReceipt, error) {
	return repository.RunWorkspaceResourcePurgeReceipt{}, common.ErrInvalidArgument
}

func runTestDependencies(
	runs repository.RunRepository,
	steps repository.StepRepository,
	checkpoints repository.RuntimeCheckpointRepository,
	hub magicevent.Hub,
	background magictask.BackgroundRoutine,
) runbiz.Dependencies {
	if steps == nil {
		steps = memoryrepo.NewStepRepository()
	}
	if checkpoints == nil {
		checkpoints = memoryrepo.NewRuntimeCheckpointRepository()
	}
	binding := repository.NewRunOwnerRepositoryBinding()
	owner, err := repository.NewRunOwnerRepositories(
		runTestBoundRuns{RunRepository: runs, binding: binding},
		runTestBoundSteps{StepRepository: steps, binding: binding},
		runTestBoundCheckpoints{RuntimeCheckpointRepository: checkpoints, binding: binding},
		runTestPurger{binding: binding},
	)
	if err != nil {
		panic(err)
	}
	return runbiz.Dependencies{OwnerRepositories: owner, EventHub: hub, BackgroundRoutine: background}
}
