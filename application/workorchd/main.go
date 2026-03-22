package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/muidea/workorch/application/workorchd/presenter"
	approvalbiz "github.com/muidea/workorch/internal/modules/blocks/approval/biz"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	legacy "github.com/muidea/workorch/internal/modules/blocks/legacy/biz"
	decisionbiz "github.com/muidea/workorch/internal/modules/kernel/decision/biz"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	orchestrator "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	runsvc "github.com/muidea/workorch/internal/modules/kernel/run/biz"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/repository"
	fileRepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

const (
	envBackend  = "WORKORCH_REPOSITORY_BACKEND"
	envDataRoot = "WORKORCH_DATA_DIR"
)

type runtime struct {
	workspaceRepo repository.WorkspaceRepository
	runRepo       repository.RunRepository
	stepRepo      repository.StepRepository
	ticketRepo    repository.DecisionTicketRepository
	approvalRepo  repository.ApprovalRepository

	workspaceManager *workspace.GoalDoDManager
	decisionService  *decisionbiz.Service
	approvalService  *approvalbiz.Service
	runService       *runsvc.Service
}

func main() {
	ctx := context.Background()
	rt := newRuntime()

	presenter.PrintBanner(os.Stdout)

	runnable, err := runRunnableSlice(ctx, rt)
	if err != nil {
		log.Fatal(err)
	}
	if err := presenter.PrintSnapshot(os.Stdout, "phase1 runnable slice", runnable); err != nil {
		log.Fatal(err)
	}

	decisionBlockedResume, err := runDecisionBlockedResumeSlice(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if err := presenter.PrintSnapshot(os.Stdout, "phase1 decision-blocked-resume slice", decisionBlockedResume); err != nil {
		log.Fatal(err)
	}

	approvalBlockedResume, err := runApprovalBlockedResumeSlice(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if err := presenter.PrintSnapshot(os.Stdout, "phase1 approval-blocked-resume slice", approvalBlockedResume); err != nil {
		log.Fatal(err)
	}
}

func newRuntime() *runtime {
	return newRuntimeWithBackend(os.Getenv(envBackend), os.Getenv(envDataRoot))
}

func newRuntimeWithBackend(backend string, dataRoot string) *runtime {
	workspaceRepo, runRepo, stepRepo, ticketRepo, resolutionRepo, approvalRepo := buildRepositories(backend, dataRoot)

	workspaceManager := workspace.NewGoalDoDManager(workspaceRepo)
	decisionService := decisionbiz.NewService(ticketRepo, resolutionRepo)
	approvalService := approvalbiz.NewService(approvalRepo)
	runtimeAdapter := legacy.NewMinimalRuntimeAdapter()
	runtimeOrchestrator := orchestrator.NewRuntimeOrchestrator(runtimeAdapter, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)
	runService := runsvc.NewService(runRepo, workspaceRepo, runtimeOrchestrator)

	return &runtime{
		workspaceRepo:    workspaceRepo,
		runRepo:          runRepo,
		stepRepo:         stepRepo,
		ticketRepo:       ticketRepo,
		approvalRepo:     approvalRepo,
		workspaceManager: workspaceManager,
		decisionService:  decisionService,
		approvalService:  approvalService,
		runService:       runService,
	}
}

func buildRepositories(backend string, dataRoot string) (
	repository.WorkspaceRepository,
	repository.RunRepository,
	repository.StepRepository,
	repository.DecisionTicketRepository,
	repository.DecisionResolutionRepository,
	repository.ApprovalRepository,
) {
	switch backend {
	case "", "memory":
		return memory.NewWorkspaceRepository(),
			memory.NewRunRepository(),
			memory.NewStepRepository(),
			memory.NewDecisionTicketRepository(),
			memory.NewDecisionResolutionRepository(),
			memory.NewApprovalRepository()
	case "file":
		if dataRoot == "" {
			dataRoot = filepath.Join(os.TempDir(), "workorchd-data")
		}
		return fileRepo.NewWorkspaceRepository(filepath.Join(dataRoot, "workspace.json")),
			fileRepo.NewRunRepository(filepath.Join(dataRoot, "run.json")),
			fileRepo.NewStepRepository(filepath.Join(dataRoot, "step.json")),
			fileRepo.NewDecisionTicketRepository(filepath.Join(dataRoot, "ticket.json")),
			fileRepo.NewDecisionResolutionRepository(filepath.Join(dataRoot, "resolution.json")),
			fileRepo.NewApprovalRepository(filepath.Join(dataRoot, "approval.json"))
	default:
		panic(fmt.Sprintf("unsupported repository backend: %s", backend))
	}
}

func runRunnableSlice(ctx context.Context, rt *runtime) (*presenter.Snapshot, error) {
	workspaceID := common.WorkspaceID("ws-phase1-runnable")
	runID := common.RunID("run-phase1-runnable")
	stepID := common.StepID(fmt.Sprintf("%s/step-1", runID))

	if err := rt.workspaceManager.CreateWorkspace(ctx, contracts.CreateWorkspaceCommand{
		WorkspaceID: workspaceID,
		Title:       "phase1-runnable",
	}); err != nil {
		return nil, err
	}

	if err := rt.runService.CreateRun(ctx, contracts.CreateRunCommand{
		RunID:       runID,
		WorkspaceID: workspaceID,
	}); err != nil {
		return nil, err
	}

	if err := rt.runService.StartRun(ctx, contracts.StartRunCommand{RunID: runID}); err != nil {
		return nil, err
	}

	return rt.snapshot(ctx, "runnable", workspaceID, runID, stepID, presenter.BlockingRefs{})
}

func runDecisionBlockedResumeSlice(ctx context.Context) (*presenter.Snapshot, error) {
	rt := newRuntime()

	workspaceID := common.WorkspaceID("ws-phase1-blocked-decision")
	runID := common.RunID("run-phase1-blocked-decision")
	stepID := common.StepID("run-phase1-blocked-decision/manual-gate")
	ticketID := common.TicketID("ticket-phase1-blocked-decision")

	if err := rt.workspaceManager.CreateWorkspace(ctx, contracts.CreateWorkspaceCommand{
		WorkspaceID: workspaceID,
		Title:       "phase1-decision-blocked-resume",
	}); err != nil {
		return nil, err
	}

	if err := rt.runService.CreateRun(ctx, contracts.CreateRunCommand{
		RunID:       runID,
		WorkspaceID: workspaceID,
	}); err != nil {
		return nil, err
	}

	if err := rt.decisionService.CreateTicket(ctx, contracts.CreateTicketCommand{
		TicketID:    ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Reason:      "phase1 manual decision gate",
	}); err != nil {
		return nil, err
	}

	if err := rt.runService.BlockOnDecision(ctx, contracts.BlockOnDecisionCommand{
		RunID:    runID,
		TicketID: ticketID,
	}); err != nil {
		return nil, err
	}

	if err := rt.decisionService.ResolveTicket(ctx, contracts.ResolveTicketCommand{
		ResolutionID: common.ResolutionID("resolution-phase1-blocked-decision"),
		TicketID:     ticketID,
		Type:         "approve_path",
		Payload:      map[string]any{"operator": "phase1-demo"},
	}); err != nil {
		return nil, err
	}

	if err := rt.runService.ResumeRun(ctx, contracts.ResumeRunCommand{RunID: runID}); err != nil {
		return nil, err
	}

	resumedStepID := common.StepID(fmt.Sprintf("%s/step-1", runID))
	return rt.snapshot(ctx, "decision_blocked_resume", workspaceID, runID, resumedStepID, presenter.BlockingRefs{TicketID: ticketID})
}

func runApprovalBlockedResumeSlice(ctx context.Context) (*presenter.Snapshot, error) {
	rt := newRuntime()

	workspaceID := common.WorkspaceID("ws-phase1-blocked-approval")
	runID := common.RunID("run-phase1-blocked-approval")
	stepID := common.StepID("run-phase1-blocked-approval/manual-approval")
	approvalID := common.ApprovalID("approval-phase1-blocked-approval")

	if err := rt.workspaceManager.CreateWorkspace(ctx, contracts.CreateWorkspaceCommand{
		WorkspaceID: workspaceID,
		Title:       "phase1-approval-blocked-resume",
	}); err != nil {
		return nil, err
	}

	if err := rt.runService.CreateRun(ctx, contracts.CreateRunCommand{
		RunID:       runID,
		WorkspaceID: workspaceID,
	}); err != nil {
		return nil, err
	}

	if err := rt.approvalService.CreateApproval(ctx, approvalbiz.CreateApprovalCommand{
		ApprovalID:  approvalID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		Reason:      "phase1 manual approval gate",
	}); err != nil {
		return nil, err
	}

	if err := rt.runService.BlockOnApproval(ctx, contracts.BlockOnApprovalCommand{
		RunID:      runID,
		ApprovalID: approvalID,
	}); err != nil {
		return nil, err
	}

	if err := rt.approvalService.Approve(ctx, approvalbiz.ResolveApprovalCommand{ApprovalID: approvalID}); err != nil {
		return nil, err
	}

	if err := rt.runService.ResumeRun(ctx, contracts.ResumeRunCommand{RunID: runID}); err != nil {
		return nil, err
	}

	resumedStepID := common.StepID(fmt.Sprintf("%s/step-1", runID))
	return rt.snapshot(ctx, "approval_blocked_resume", workspaceID, runID, resumedStepID, presenter.BlockingRefs{ApprovalID: approvalID})
}

func (rt *runtime) snapshot(ctx context.Context, slice string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, refs presenter.BlockingRefs) (*presenter.Snapshot, error) {
	ws, err := rt.workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	r, err := rt.runRepo.Get(ctx, runID)
	if err != nil {
		return nil, err
	}

	var step *runmodel.Step
	if stepID != "" {
		step, err = rt.stepRepo.Get(ctx, stepID)
		if err != nil && err != common.ErrNotFound {
			return nil, err
		}
	}

	var ticket *decisionmodel.Ticket
	if refs.TicketID != "" {
		ticket, err = rt.ticketRepo.Get(ctx, refs.TicketID)
		if err != nil {
			return nil, err
		}
	}

	var approval *approvalmodel.Approval
	if refs.ApprovalID != "" {
		approval, err = rt.approvalRepo.Get(ctx, refs.ApprovalID)
		if err != nil {
			return nil, err
		}
	}

	return presenter.BuildSnapshot(slice, ws, r, step, ticket, approval), nil
}
