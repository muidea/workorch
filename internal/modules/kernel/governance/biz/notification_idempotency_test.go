package governance

import (
	"context"
	"sync/atomic"
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestGovernanceDuplicateDecisionAndRunFactsDoNotRepublishResolvedTrace(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-governance-notification-idempotency")
	runID := common.RunID("run-governance-notification-idempotency")
	ticketID := common.TicketID("ticket-governance-notification-idempotency")
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
	tickets := memory.NewDecisionTicketRepository()
	runs := memory.NewRunRepository()
	traces := memory.NewAutoGovernanceTraceRepository()
	mustNoErr(t, tickets.Create(ctx, &decisionmodel.Ticket{
		ID: ticketID, WorkspaceID: workspaceID, RunID: runID, StepID: "step-1",
		DecisionKind: "provide_missing_info", State: common.DecisionTicketStateOpen,
		Context: workorcheventpayload.New(workorcheventpayload.StringField("answer", "bounded-value")),
	}))
	mustNoErr(t, runs.Create(ctx, &runmodel.Run{
		ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision, BlockingTicketID: ticketID,
	}))
	hub := newRecordingGovernanceHub(t)
	var resolveCount atomic.Int32
	svc := NewService(servicePorts{
		Tickets: tickets, Approvals: memory.NewApprovalRepository(), Runs: runs,
		WorkspaceDefinitions: testWorkspaceDefinitions{goals: memory.NewGoalRepository(), dods: memory.NewDoDRepository()},
		MemoryItems:          memory.NewMemoryItemRepository(), AutoGovernanceTraces: traces,
		ResolveDecision: func(context.Context, common.TicketID, string, workorcheventpayload.Payload) error {
			resolveCount.Add(1)
			return nil
		},
		ResolveApproval: func(context.Context, common.ApprovalID, common.RunID) error { return nil },
		LoadConfig:      testGovernanceConfig(true, 0.82, 3), NotificationHub: hub,
	})

	if err := svc.processPendingTicket(ctx, ticketID); err != nil {
		t.Fatalf("first ticket fact: %v", err)
	}
	svc.processTicketCreated(context.Background(), decisionevents.TicketChangedData{
		TicketID: ticketID, WorkspaceID: workspaceID, RunID: runID, State: common.DecisionTicketStateOpen,
	})
	svc.processRunStateChanged(context.Background(), runevents.RunStateChangedData{
		RunID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID, CausationID: "duplicate-run-fact",
	})

	if got := resolveCount.Load(); got != 1 {
		t.Fatalf("owner mutation count = %d, want 1", got)
	}
	counts := map[string]int{}
	for _, event := range hub.Posted() {
		counts[event.ID()]++
	}
	if counts[governanceevents.TopicGovernanceAutoEvaluated] != 1 || counts[governanceevents.TopicGovernanceAutoResolved] != 1 || len(hub.Posted()) != 2 {
		t.Fatalf("published notification counts = %+v (total %d), want one evaluated and one resolved", counts, len(hub.Posted()))
	}
}
