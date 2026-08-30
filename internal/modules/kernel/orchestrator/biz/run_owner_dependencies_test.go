package orchestrator

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

type orchestratorTestBoundRuns struct {
	repository.RunRepository
	binding repository.RunOwnerRepositoryBinding
}

func (r orchestratorTestBoundRuns) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	return r.binding
}

type orchestratorTestBoundSteps struct {
	repository.StepRepository
	binding repository.RunOwnerRepositoryBinding
}

func (r orchestratorTestBoundSteps) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	return r.binding
}

type orchestratorTestBoundCheckpoints struct {
	repository.RuntimeCheckpointRepository
	binding repository.RunOwnerRepositoryBinding
}

func (r orchestratorTestBoundCheckpoints) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	return r.binding
}

type orchestratorTestRunPurger struct {
	binding repository.RunOwnerRepositoryBinding
}

func (p orchestratorTestRunPurger) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	return p.binding
}

func (orchestratorTestRunPurger) PurgeWorkspaceResources(context.Context, runmodel.WorkspaceRetirementFence) (repository.RunWorkspaceResourcePurgeReceipt, error) {
	return repository.RunWorkspaceResourcePurgeReceipt{}, common.ErrInvalidArgument
}

func runOwnerTestDependencies(
	runs repository.RunRepository,
	steps repository.StepRepository,
	checkpoints repository.RuntimeCheckpointRepository,
	hub magicevent.Hub,
) runbiz.Dependencies {
	if steps == nil {
		steps = memoryrepo.NewStepRepository()
	}
	if checkpoints == nil {
		checkpoints = memoryrepo.NewRuntimeCheckpointRepository()
	}
	binding := repository.NewRunOwnerRepositoryBinding()
	owner, err := repository.NewRunOwnerRepositories(
		orchestratorTestBoundRuns{RunRepository: runs, binding: binding},
		orchestratorTestBoundSteps{StepRepository: steps, binding: binding},
		orchestratorTestBoundCheckpoints{RuntimeCheckpointRepository: checkpoints, binding: binding},
		orchestratorTestRunPurger{binding: binding},
	)
	if err != nil {
		panic(err)
	}
	return runbiz.Dependencies{OwnerRepositories: owner, EventHub: hub}
}
