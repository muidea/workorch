package run

import (
	"context"
	"errors"
	"testing"

	approvalModel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionModel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/contracts"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestService_CreateRunStartRunCompleteRun_Mainline(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		&serviceRuntimeAdapter{output: map[string]any{"status": "ok"}},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		approvalRepo,
		nil,
	)
	service := NewService(runRepo, workspaceRepo, orch)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	if err := service.CreateRun(ctx, contracts.CreateRunCommand{RunID: runID, WorkspaceID: workspaceID}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	created, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get created run: %v", err)
	}
	if created.State != common.RunStateCreated {
		t.Fatalf("created run state = %q, want created", created.State)
	}

	if err := service.StartRun(ctx, contracts.StartRunCommand{RunID: runID}); err != nil {
		t.Fatalf("start run: %v", err)
	}

	completed, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get completed run: %v", err)
	}
	if completed.State != common.RunStateCompleted {
		t.Fatalf("completed run state = %q, want completed", completed.State)
	}

	step, err := stepRepo.Get(ctx, common.StepID(string(runID)+"/step-1"))
	if err != nil {
		t.Fatalf("get completed step: %v", err)
	}
	if step.State != common.StepStateCompleted {
		t.Fatalf("step state = %q, want completed", step.State)
	}
}

func TestService_CreateRun_ValidatesWorkspaceAndArguments(t *testing.T) {
	ctx := context.Background()
	service := NewService(memory.NewRunRepository(), memory.NewWorkspaceRepository(), nil)

	if err := service.CreateRun(ctx, contracts.CreateRunCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty create run err = %v, want invalid argument", err)
	}

	if err := service.CreateRun(ctx, contracts.CreateRunCommand{RunID: common.RunID("run-1"), WorkspaceID: common.WorkspaceID("missing")}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing workspace err = %v, want not found", err)
	}
}

func TestService_ResumeBlockCommands_ValidateArgumentsAndRequireOrchestrator(t *testing.T) {
	ctx := context.Background()
	service := NewService(memory.NewRunRepository(), memory.NewWorkspaceRepository(), nil)

	if err := service.ResumeRun(ctx, contracts.ResumeRunCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("resume empty err = %v, want invalid argument", err)
	}
	if err := service.ResumeRun(ctx, contracts.ResumeRunCommand{RunID: common.RunID("run-1")}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("resume without orchestrator err = %v, want invalid argument", err)
	}

	if err := service.BlockOnDecision(ctx, contracts.BlockOnDecisionCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on decision empty err = %v, want invalid argument", err)
	}
	if err := service.BlockOnDecision(ctx, contracts.BlockOnDecisionCommand{RunID: common.RunID("run-1"), TicketID: common.TicketID("ticket-1")}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on decision without orchestrator err = %v, want invalid argument", err)
	}

	if err := service.BlockOnApproval(ctx, contracts.BlockOnApprovalCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on approval empty err = %v, want invalid argument", err)
	}
	if err := service.BlockOnApproval(ctx, contracts.BlockOnApprovalCommand{RunID: common.RunID("run-1"), ApprovalID: common.ApprovalID("approval-1")}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on approval without orchestrator err = %v, want invalid argument", err)
	}
}

func TestService_ResumeRun_FromBlockedStates_ReachesMinimalExecutionChain(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name       string
		blocked    common.RunState
		setupBlock func(context.Context, *Service, common.RunID) error
	}{
		{
			name:    "decision",
			blocked: common.RunStateBlockedOnDecision,
			setupBlock: func(ctx context.Context, service *Service, runID common.RunID) error {
				return service.BlockOnDecision(ctx, contracts.BlockOnDecisionCommand{RunID: runID, TicketID: common.TicketID("ticket-1")})
			},
		},
		{
			name:    "approval",
			blocked: common.RunStateBlockedOnApproval,
			setupBlock: func(ctx context.Context, service *Service, runID common.RunID) error {
				return service.BlockOnApproval(ctx, contracts.BlockOnApprovalCommand{RunID: runID, ApprovalID: common.ApprovalID("approval-1")})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspaceRepo := memory.NewWorkspaceRepository()
			runRepo := memory.NewRunRepository()
			stepRepo := memory.NewStepRepository()
			ticketRepo := memory.NewDecisionTicketRepository()
			approvalRepo := memory.NewApprovalRepository()
			orch := orchestratorbiz.NewRuntimeOrchestrator(
				&serviceRuntimeAdapter{output: map[string]any{"status": tc.name}},
				runRepo,
				stepRepo,
				workspaceRepo,
				ticketRepo,
				approvalRepo,
				nil,
			)
			service := NewService(runRepo, workspaceRepo, orch)

			workspaceID := common.WorkspaceID("ws-" + tc.name)
			runID := common.RunID("run-" + tc.name)
			if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
				t.Fatalf("seed workspace: %v", err)
			}
			if err := service.CreateRun(ctx, contracts.CreateRunCommand{RunID: runID, WorkspaceID: workspaceID}); err != nil {
				t.Fatalf("create run: %v", err)
			}
			if err := service.StartRun(ctx, contracts.StartRunCommand{RunID: runID}); err != nil {
				t.Fatalf("start run: %v", err)
			}

			stepID := common.StepID(string(runID) + "/step-1")
			step, err := stepRepo.Get(ctx, stepID)
			if err != nil {
				t.Fatalf("get initial step: %v", err)
			}
			step.State = common.StepStateFailed
			step.Output = map[string]any{"stale": true}
			if err := stepRepo.Update(ctx, step); err != nil {
				t.Fatalf("prepare reusable step: %v", err)
			}

			r, err := runRepo.Get(ctx, runID)
			if err != nil {
				t.Fatalf("get completed run: %v", err)
			}
			r.State = common.RunStateRunning
			if err := runRepo.Update(ctx, r); err != nil {
				t.Fatalf("set run back to running: %v", err)
			}
			ws, err := workspaceRepo.Get(ctx, workspaceID)
			if err != nil {
				t.Fatalf("get workspace: %v", err)
			}
			ws.ActiveRunID = runID
			if err := workspaceRepo.Update(ctx, ws); err != nil {
				t.Fatalf("set active run: %v", err)
			}

			if err := tc.setupBlock(ctx, service, runID); err != nil {
				t.Fatalf("setup block state: %v", err)
			}

			blockedRun, err := runRepo.Get(ctx, runID)
			if err != nil {
				t.Fatalf("get blocked run: %v", err)
			}
			if blockedRun.State != tc.blocked {
				t.Fatalf("blocked run state = %q, want %q", blockedRun.State, tc.blocked)
			}
			switch tc.blocked {
			case common.RunStateBlockedOnDecision:
				if err := ticketRepo.Create(ctx, &decisionModel.Ticket{
					ID:          blockedRun.BlockingTicketID,
					WorkspaceID: workspaceID,
					RunID:       runID,
					StepID:      stepID,
					Reason:      "resume",
					State:       common.DecisionTicketStateResolved,
				}); err != nil {
					t.Fatalf("seed resolved ticket: %v", err)
				}
			case common.RunStateBlockedOnApproval:
				if err := approvalRepo.Create(ctx, &approvalModel.Approval{
					ID:          blockedRun.BlockingApprovalID,
					WorkspaceID: workspaceID,
					RunID:       runID,
					StepID:      stepID,
					Reason:      "resume",
					State:       common.ApprovalStateApproved,
				}); err != nil {
					t.Fatalf("seed resolved approval: %v", err)
				}
			}

			if err := service.ResumeRun(ctx, contracts.ResumeRunCommand{RunID: runID}); err != nil {
				t.Fatalf("resume run: %v", err)
			}

			resumedRun, err := runRepo.Get(ctx, runID)
			if err != nil {
				t.Fatalf("get resumed run: %v", err)
			}
			if resumedRun.State != common.RunStateCompleted {
				t.Fatalf("resumed run state = %q, want completed", resumedRun.State)
			}

			resumedStep, err := stepRepo.Get(ctx, stepID)
			if err != nil {
				t.Fatalf("get resumed step: %v", err)
			}
			if resumedStep.State != common.StepStateCompleted {
				t.Fatalf("resumed step state = %q, want completed", resumedStep.State)
			}
			if resumedStep.Output["status"] != tc.name {
				t.Fatalf("resumed step output status = %v, want %s", resumedStep.Output["status"], tc.name)
			}
		})
	}
}

func TestService_ResumeRun_RejectsNonBlockedStates(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := orchestratorbiz.NewRuntimeOrchestrator(
		&serviceRuntimeAdapter{output: map[string]any{"status": "ok"}},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		approvalRepo,
		nil,
	)
	service := NewService(runRepo, workspaceRepo, orch)

	workspaceID := common.WorkspaceID("ws-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name  string
		state common.RunState
	}{
		{name: "created", state: common.RunStateCreated},
		{name: "running", state: common.RunStateRunning},
		{name: "completed", state: common.RunStateCompleted},
		{name: "failed", state: common.RunStateFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runID := common.RunID("run-" + tc.name)
			if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: tc.state}); err != nil {
				t.Fatalf("seed run: %v", err)
			}

			if err := service.ResumeRun(ctx, contracts.ResumeRunCommand{RunID: runID}); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("resume err = %v, want conflict", err)
			}
		})
	}
}

func TestService_CompleteRun_WithoutOrchestrator(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	service := NewService(runRepo, workspaceRepo, nil)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := service.CreateRun(ctx, contracts.CreateRunCommand{RunID: runID, WorkspaceID: workspaceID}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	if err := service.CompleteRun(ctx, contracts.CompleteRunCommand{RunID: runID}); err != nil {
		t.Fatalf("complete run: %v", err)
	}

	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateCompleted {
		t.Fatalf("run state = %q, want completed", r.State)
	}
}

type serviceRuntimeAdapter struct {
	output map[string]any
	err    error
}

func (s *serviceRuntimeAdapter) ExecuteStep(_ context.Context, _ common.RunID, _ common.StepID) (map[string]any, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.output, nil
}
