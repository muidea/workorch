package governance

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

var errGovernanceOwnerContextUnavailable = errors.New("governance owner context unavailable")

type governanceFaultingDefinitionReader struct {
	inner   WorkspaceDefinitionReader
	goalErr error
	dodErr  error
}

func (r governanceFaultingDefinitionReader) GetGoal(ctx context.Context, id common.GoalID) (*workspacemodel.Goal, error) {
	if r.goalErr != nil {
		return nil, r.goalErr
	}
	return r.inner.GetGoal(ctx, id)
}

func (r governanceFaultingDefinitionReader) GetDoD(ctx context.Context, id common.DoDID) (*workspacemodel.DoD, error) {
	if r.dodErr != nil {
		return nil, r.dodErr
	}
	return r.inner.GetDoD(ctx, id)
}

type governanceFaultingMemoryReader struct {
	inner MemoryReader
	err   error
}

func (r governanceFaultingMemoryReader) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.inner.ListByWorkspaceID(ctx, workspaceID)
}

func TestGovernanceTicketOwnerContextReadErrorsFailClosedBeforeTraceOrDecisionMutation(t *testing.T) {
	for _, test := range []struct {
		name      string
		goalErr   error
		dodErr    error
		memoryErr error
	}{
		{name: "goal", goalErr: errGovernanceOwnerContextUnavailable},
		{name: "dod", dodErr: errGovernanceOwnerContextUnavailable},
		{name: "memory", memoryErr: errGovernanceOwnerContextUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			workspaceID := common.WorkspaceID("ws-ticket-owner-read-" + test.name)
			runID := common.RunID("run-ticket-owner-read-" + test.name)
			goalID := common.GoalID("goal-ticket-owner-read-" + test.name)
			dodID := common.DoDID("dod-ticket-owner-read-" + test.name)
			ticketID := common.TicketID("ticket-owner-read-" + test.name)
			scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
			if err != nil {
				t.Fatal(err)
			}

			tickets := memory.NewDecisionTicketRepository()
			runs := memory.NewRunRepository()
			goals := memory.NewGoalRepository()
			dods := memory.NewDoDRepository()
			memoryItems := memory.NewMemoryItemRepository()
			traces := memory.NewAutoGovernanceTraceRepository()
			mustNoErr(t, goals.Create(scopedCtx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "complete the requested outcome"}))
			mustNoErr(t, dods.Create(scopedCtx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"verify the outcome"}, State: common.DoDStateReady}))
			mustNoErr(t, runs.Create(scopedCtx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateBlockedOnDecision, BlockingTicketID: ticketID}))
			mustNoErr(t, tickets.Create(scopedCtx, &decisionmodel.Ticket{
				ID: ticketID, WorkspaceID: workspaceID, RunID: runID, DecisionKind: "provide_missing_info",
				Question: "provide the required input", Reason: "missing input", State: common.DecisionTicketStateOpen,
			}))

			definitions := governanceFaultingDefinitionReader{
				inner: testWorkspaceDefinitions{goals: goals, dods: dods}, goalErr: test.goalErr, dodErr: test.dodErr,
			}
			var decisionMutations atomic.Int32
			var approvalMutations atomic.Int32
			service := NewService(servicePorts{
				Tickets: tickets, Approvals: memory.NewApprovalRepository(), Runs: runs,
				WorkspaceDefinitions: definitions,
				MemoryItems:          governanceFaultingMemoryReader{inner: memoryItems, err: test.memoryErr},
				AutoGovernanceTraces: traces,
				ResolveDecision: func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error {
					decisionMutations.Add(1)
					return nil
				},
				ResolveApproval: func(context.Context, common.ApprovalID, common.RunID) error {
					approvalMutations.Add(1)
					return nil
				},
				LoadConfig: testGovernanceConfig(true, 0.82, 3),
			})

			if err := service.processPendingTicket(scopedCtx, ticketID); !errors.Is(err, errGovernanceOwnerContextUnavailable) {
				t.Fatalf("process ticket error = %v, want owner context error", err)
			}
			if decisionMutations.Load() != 0 || approvalMutations.Load() != 0 {
				t.Fatalf("owner mutations decision=%d approval=%d, want zero", decisionMutations.Load(), approvalMutations.Load())
			}
			items, err := traces.List(scopedCtx, workorchquerycontract.AutoGovernanceTraceQuery{WorkspaceID: workspaceID})
			if err != nil || len(items) != 0 {
				t.Fatalf("traces after failed owner read = %+v, err=%v", items, err)
			}
			current, err := tickets.Get(scopedCtx, ticketID)
			if err != nil || current.State != common.DecisionTicketStateOpen {
				t.Fatalf("ticket after failed owner read = %+v, err=%v", current, err)
			}
		})
	}
}

func TestGovernanceApprovalDefinitionReadErrorsFailClosedBeforeTraceOrApprovalMutation(t *testing.T) {
	for _, test := range []struct {
		name    string
		goalErr error
		dodErr  error
	}{
		{name: "goal", goalErr: errGovernanceOwnerContextUnavailable},
		{name: "dod", dodErr: errGovernanceOwnerContextUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			workspaceID := common.WorkspaceID("ws-approval-owner-read-" + test.name)
			runID := common.RunID("run-approval-owner-read-" + test.name)
			goalID := common.GoalID("goal-approval-owner-read-" + test.name)
			dodID := common.DoDID("dod-approval-owner-read-" + test.name)
			approvalID := common.ApprovalID("approval-owner-read-" + test.name)
			scopedCtx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
			if err != nil {
				t.Fatal(err)
			}

			approvals := memory.NewApprovalRepository()
			runs := memory.NewRunRepository()
			goals := memory.NewGoalRepository()
			dods := memory.NewDoDRepository()
			traces := memory.NewAutoGovernanceTraceRepository()
			mustNoErr(t, goals.Create(scopedCtx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "inspect configuration"}))
			mustNoErr(t, dods.Create(scopedCtx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"return configuration"}, State: common.DoDStateReady}))
			mustNoErr(t, runs.Create(scopedCtx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, GoalID: goalID, DoDID: dodID, State: common.RunStateBlockedOnApproval, BlockingApprovalID: approvalID}))
			mustNoErr(t, approvals.Create(scopedCtx, &approvalmodel.Approval{
				ID: approvalID, WorkspaceID: workspaceID, RunID: runID, StepID: common.StepID(string(runID) + "/step-1"),
				Reason: "read configuration", Action: "read configuration", State: common.ApprovalStatePending,
				Context: workorcheventpayload.New(workorcheventpayload.StringField("risk", "read_only")),
			}))

			var decisionMutations atomic.Int32
			var approvalMutations atomic.Int32
			service := NewService(servicePorts{
				Tickets: memory.NewDecisionTicketRepository(), Approvals: approvals, Runs: runs,
				WorkspaceDefinitions: governanceFaultingDefinitionReader{
					inner: testWorkspaceDefinitions{goals: goals, dods: dods}, goalErr: test.goalErr, dodErr: test.dodErr,
				},
				MemoryItems: memory.NewMemoryItemRepository(), AutoGovernanceTraces: traces,
				ResolveDecision: func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error {
					decisionMutations.Add(1)
					return nil
				},
				ResolveApproval: func(context.Context, common.ApprovalID, common.RunID) error {
					approvalMutations.Add(1)
					return nil
				},
				LoadConfig: testGovernanceConfig(true, 0.82, 3),
			})

			if err := service.processPendingApproval(scopedCtx, runID, approvalID); !errors.Is(err, errGovernanceOwnerContextUnavailable) {
				t.Fatalf("process approval error = %v, want owner context error", err)
			}
			if decisionMutations.Load() != 0 || approvalMutations.Load() != 0 {
				t.Fatalf("owner mutations decision=%d approval=%d, want zero", decisionMutations.Load(), approvalMutations.Load())
			}
			items, err := traces.List(scopedCtx, workorchquerycontract.AutoGovernanceTraceQuery{WorkspaceID: workspaceID})
			if err != nil || len(items) != 0 {
				t.Fatalf("traces after failed owner read = %+v, err=%v", items, err)
			}
			current, err := approvals.GetScoped(scopedCtx, workspaceID, runID, approvalID)
			if err != nil || current.State != common.ApprovalStatePending {
				t.Fatalf("approval after failed owner read = %+v, err=%v", current, err)
			}
		})
	}
}
