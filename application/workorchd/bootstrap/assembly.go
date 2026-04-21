package bootstrap

import (
	"os"

	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	legacymodule "github.com/muidea/workorch/internal/modules/blocks/legacy"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	orchestratormodule "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/events"
)

func NewFromEnv() *Runtime {
	return NewWithBackend(os.Getenv(envBackend), os.Getenv(envDataRoot))
}

func NewWithBackend(backend string, dataRoot string) *Runtime {
	repos := buildRepositories(backend, dataRoot)

	workspaceManager := workspacemodule.NewGoalDoDManager(repos.workspace)
	decisionService := decisionmodule.NewService(repos.ticket, repos.resolution)
	approvalService := approvalmodule.NewService(repos.approval)
	runtimeAdapter := legacymodule.NewMinimalRuntimeAdapter()
	eventHub := events.NewMemoryHub()
	runtimeOrchestrator := orchestratormodule.NewRuntimeOrchestrator(
		runtimeAdapter,
		repos.run,
		repos.step,
		repos.workspace,
		repos.ticket,
		repos.approval,
		eventHub,
	)
	runService := runmodule.NewService(repos.run, repos.workspace, runtimeOrchestrator)

	return &Runtime{
		workspaceRepo:    repos.workspace,
		runRepo:          repos.run,
		stepRepo:         repos.step,
		ticketRepo:       repos.ticket,
		approvalRepo:     repos.approval,
		workspaceManager: workspaceManager,
		decisionService:  decisionService,
		approvalService:  approvalService,
		runService:       runService,
	}
}
