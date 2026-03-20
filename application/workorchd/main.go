package main

import (
	"context"
	"log"

	legacy "github.com/muidea/workorch/internal/modules/blocks/legacy/biz"
	orchestrator "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	runsvc "github.com/muidea/workorch/internal/modules/kernel/run/biz"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/contracts"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func main() {
	ctx := context.Background()

	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()

	workspaceManager := workspace.NewGoalDoDManager(workspaceRepo)
	runtimeAdapter := legacy.NewMinimalRuntimeAdapter()
	runtimeOrchestrator := orchestrator.NewRuntimeOrchestrator(runtimeAdapter, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)
	runService := runsvc.NewService(runRepo, workspaceRepo, runtimeOrchestrator)

	workspaceID := common.WorkspaceID("ws-phase1")
	runID := common.RunID("run-phase1")

	if err := workspaceManager.CreateWorkspace(ctx, contracts.CreateWorkspaceCommand{
		WorkspaceID: workspaceID,
		Title:       "phase1-minimal",
	}); err != nil {
		log.Fatal(err)
	}

	if err := runService.CreateRun(ctx, contracts.CreateRunCommand{
		RunID:       runID,
		WorkspaceID: workspaceID,
	}); err != nil {
		log.Fatal(err)
	}

	if err := runService.StartRun(ctx, contracts.StartRunCommand{RunID: runID}); err != nil {
		log.Fatal(err)
	}
}
