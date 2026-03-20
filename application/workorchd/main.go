package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	approvalbiz "github.com/muidea/workorch/internal/modules/blocks/approval/biz"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	legacy "github.com/muidea/workorch/internal/modules/blocks/legacy/biz"
	decisionbiz "github.com/muidea/workorch/internal/modules/kernel/decision/biz"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	orchestrator "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	runsvc "github.com/muidea/workorch/internal/modules/kernel/run/biz"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

type runtime struct {
	workspaceRepo *memory.WorkspaceRepository
	runRepo       *memory.RunRepository
	stepRepo      *memory.StepRepository
	ticketRepo    *memory.DecisionTicketRepository
	approvalRepo  *memory.ApprovalRepository

	workspaceManager    *workspace.GoalDoDManager
	decisionService     *decisionbiz.Service
	approvalService     *approvalbiz.Service
	runtimeOrchestrator *orchestrator.RuntimeOrchestrator
	runService          *runsvc.Service
}

type snapshot struct {
	Slice     string            `json:"slice"`
	Workspace *workspaceView    `json:"workspace"`
	Run       *runView          `json:"run"`
	Step      *stepView         `json:"step,omitempty"`
	Blocking  *blockingView     `json:"blocking,omitempty"`
	Observed  []events.Envelope `json:"observed"`
}

type workspaceView struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	State       string `json:"state"`
	ActiveRunID string `json:"activeRunId"`
}

type runView struct {
	ID                 string `json:"id"`
	WorkspaceID        string `json:"workspaceId"`
	State              string `json:"state"`
	BlockingTicketID   string `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string `json:"blockingApprovalId,omitempty"`
}

type stepView struct {
	ID     string         `json:"id"`
	Kind   string         `json:"kind"`
	State  string         `json:"state"`
	Input  map[string]any `json:"input,omitempty"`
	Output map[string]any `json:"output,omitempty"`
}

type blockingView struct {
	DecisionTicket *decisionTicketView `json:"decisionTicket,omitempty"`
	Approval       *approvalView       `json:"approval,omitempty"`
}

type decisionTicketView struct {
	ID     string `json:"id"`
	RunID  string `json:"runId"`
	StepID string `json:"stepId"`
	Reason string `json:"reason"`
	State  string `json:"state"`
}

type approvalView struct {
	ID     string `json:"id"`
	RunID  string `json:"runId"`
	StepID string `json:"stepId"`
	Reason string `json:"reason"`
	State  string `json:"state"`
}

func main() {
	ctx := context.Background()
	rt := newRuntime()

	printBanner()

	runnable, err := runRunnableSlice(ctx, rt)
	if err != nil {
		log.Fatal(err)
	}
	printSnapshot("phase1 runnable slice", runnable)

	decisionBlockedResume, err := runDecisionBlockedResumeSlice(ctx)
	if err != nil {
		log.Fatal(err)
	}
	printSnapshot("phase1 decision-blocked-resume slice", decisionBlockedResume)

	approvalBlockedResume, err := runApprovalBlockedResumeSlice(ctx)
	if err != nil {
		log.Fatal(err)
	}
	printSnapshot("phase1 approval-blocked-resume slice", approvalBlockedResume)
}

func newRuntime() *runtime {
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()

	workspaceManager := workspace.NewGoalDoDManager(workspaceRepo)
	decisionService := decisionbiz.NewService(ticketRepo, resolutionRepo)
	approvalService := approvalbiz.NewService(approvalRepo)
	runtimeAdapter := legacy.NewMinimalRuntimeAdapter()
	runtimeOrchestrator := orchestrator.NewRuntimeOrchestrator(runtimeAdapter, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo)
	runService := runsvc.NewService(runRepo, workspaceRepo, runtimeOrchestrator)

	return &runtime{
		workspaceRepo:       workspaceRepo,
		runRepo:             runRepo,
		stepRepo:            stepRepo,
		ticketRepo:          ticketRepo,
		approvalRepo:        approvalRepo,
		workspaceManager:    workspaceManager,
		decisionService:     decisionService,
		approvalService:     approvalService,
		runtimeOrchestrator: runtimeOrchestrator,
		runService:          runService,
	}
}

func runRunnableSlice(ctx context.Context, rt *runtime) (*snapshot, error) {
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

	return rt.snapshot(ctx, "runnable", workspaceID, runID, stepID, nil)
}

func runDecisionBlockedResumeSlice(ctx context.Context) (*snapshot, error) {
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
		Payload: map[string]any{
			"operator": "phase1-demo",
		},
	}); err != nil {
		return nil, err
	}

	if err := rt.runService.ResumeRun(ctx, contracts.ResumeRunCommand{RunID: runID}); err != nil {
		return nil, err
	}

	resumedStepID := common.StepID(fmt.Sprintf("%s/step-1", runID))
	return rt.snapshot(ctx, "decision_blocked_resume", workspaceID, runID, resumedStepID, &blockingRefs{ticketID: ticketID})
}

func runApprovalBlockedResumeSlice(ctx context.Context) (*snapshot, error) {
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
	return rt.snapshot(ctx, "approval_blocked_resume", workspaceID, runID, resumedStepID, &blockingRefs{approvalID: approvalID})
}

func (rt *runtime) snapshot(ctx context.Context, slice string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, refs *blockingRefs) (*snapshot, error) {
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

	blocking := &blockingView{}
	if refs != nil && refs.ticketID != "" {
		ticket, err := rt.ticketRepo.Get(ctx, refs.ticketID)
		if err != nil {
			return nil, err
		}
		blocking.DecisionTicket = mapTicket(ticket)
	}
	if refs != nil && refs.approvalID != "" {
		approval, err := rt.approvalRepo.Get(ctx, refs.approvalID)
		if err != nil {
			return nil, err
		}
		blocking.Approval = mapApproval(approval)
	}
	if blocking.DecisionTicket == nil && blocking.Approval == nil {
		blocking = nil
	}

	observed := []events.Envelope{
		newEnvelope("workspace.snapshot", "workspace", string(ws.ID), ws.ID, runID, "", map[string]any{
			"state":       ws.State,
			"activeRunId": ws.ActiveRunID,
		}),
		newEnvelope("run.snapshot", "run", string(r.ID), workspaceID, r.ID, "", map[string]any{
			"state":              r.State,
			"blockingTicketId":   r.BlockingTicketID,
			"blockingApprovalId": r.BlockingApprovalID,
		}),
	}
	if step != nil {
		observed = append(observed, newEnvelope("step.snapshot", "step", string(step.ID), workspaceID, runID, step.ID, map[string]any{
			"kind":   step.Kind,
			"state":  step.State,
			"input":  step.Input,
			"output": step.Output,
		}))
	}
	if blocking != nil && blocking.DecisionTicket != nil {
		observed = append(observed, newEnvelope("decision.ticket.snapshot", "decision_ticket", blocking.DecisionTicket.ID, workspaceID, runID, common.StepID(blocking.DecisionTicket.StepID), map[string]any{
			"reason": blocking.DecisionTicket.Reason,
			"state":  blocking.DecisionTicket.State,
		}))
	}
	if blocking != nil && blocking.Approval != nil {
		observed = append(observed, newEnvelope("approval.snapshot", "approval", blocking.Approval.ID, workspaceID, runID, common.StepID(blocking.Approval.StepID), map[string]any{
			"reason": blocking.Approval.Reason,
			"state":  blocking.Approval.State,
		}))
	}

	return &snapshot{
		Slice:     slice,
		Workspace: mapWorkspace(ws),
		Run:       mapRun(r),
		Step:      mapStep(step),
		Blocking:  blocking,
		Observed:  observed,
	}, nil
}

type blockingRefs struct {
	ticketID   common.TicketID
	approvalID common.ApprovalID
}

func printBanner() {
	fmt.Fprintln(os.Stdout, "== workorch phase1 observable demo ==")
	fmt.Fprintln(os.Stdout, "acceptance target: runnable slice + decision/approval blocked->resume slices")
}

func printSnapshot(title string, data *snapshot) {
	fmt.Fprintf(os.Stdout, "\n## %s\n", title)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		log.Fatalf("encode snapshot: %v", err)
	}
}

func mapWorkspace(item *workspacemodel.Workspace) *workspaceView {
	if item == nil {
		return nil
	}
	return &workspaceView{
		ID:          string(item.ID),
		Title:       item.Title,
		State:       string(item.State),
		ActiveRunID: string(item.ActiveRunID),
	}
}

func mapRun(item *runmodel.Run) *runView {
	if item == nil {
		return nil
	}
	return &runView{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		State:              string(item.State),
		BlockingTicketID:   string(item.BlockingTicketID),
		BlockingApprovalID: string(item.BlockingApprovalID),
	}
}

func mapStep(item *runmodel.Step) *stepView {
	if item == nil {
		return nil
	}
	return &stepView{
		ID:     string(item.ID),
		Kind:   item.Kind,
		State:  string(item.State),
		Input:  item.Input,
		Output: item.Output,
	}
}

func mapTicket(item *decisionmodel.Ticket) *decisionTicketView {
	if item == nil {
		return nil
	}
	return &decisionTicketView{
		ID:     string(item.ID),
		RunID:  string(item.RunID),
		StepID: string(item.StepID),
		Reason: item.Reason,
		State:  string(item.State),
	}
}

func mapApproval(item *approvalmodel.Approval) *approvalView {
	if item == nil {
		return nil
	}
	return &approvalView{
		ID:     string(item.ID),
		RunID:  string(item.RunID),
		StepID: string(item.StepID),
		Reason: item.Reason,
		State:  string(item.State),
	}
}

func newEnvelope(topic, subjectType, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, payload map[string]any) events.Envelope {
	return events.Envelope{
		ID:           fmt.Sprintf("%s:%s", topic, subjectID),
		Version:      "phase1-demo.v1",
		Topic:        topic,
		OccurredAt:   time.Now().UTC().Format(time.RFC3339),
		SourceModule: "application/workorchd",
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		StepID:       string(stepID),
		Payload:      payload,
	}
}
