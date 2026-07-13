package retirementports

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/retirementcore"
	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	workspacemodule "github.com/muidea/workorch/internal/modules/blocks/workspace"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestApplyRuntimeV1RetirementPlanSendsTypedOwnerUpdates(t *testing.T) {
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	var runUpdate runevents.UpdateRunCommand
	var workspaceUpdate workspaceevents.UpdateWorkspaceCommand
	var ticketUpdate decisionevents.UpdateTicketCommand
	var approvalUpdate approvalevents.UpdateApprovalCommand
	registerRetirementApplyOwner(t, hub, runevents.CommandDestination, runevents.TopicRunCommandUpdate, func(event magicevent.Event) any {
		runUpdate = event.Data().(runevents.UpdateRunCommand)
		return runevents.RunCommandResult{}
	})
	registerRetirementApplyOwner(t, hub, workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandUpdate, func(event magicevent.Event) any {
		workspaceUpdate = event.Data().(workspaceevents.UpdateWorkspaceCommand)
		return workspaceevents.WorkspaceCommandResult{}
	})
	registerRetirementApplyOwner(t, hub, decisionevents.CommandDestination, decisionevents.TopicDecisionCommandUpdateTicket, func(event magicevent.Event) any {
		ticketUpdate = event.Data().(decisionevents.UpdateTicketCommand)
		return decisionevents.DecisionCommandResult{}
	})
	registerRetirementApplyOwner(t, hub, approvalevents.CommandDestination, approvalevents.TopicApprovalCommandUpdate, func(event magicevent.Event) any {
		approvalUpdate = event.Data().(approvalevents.UpdateApprovalCommand)
		return approvalevents.ApprovalCommandResult{}
	})

	inventory := retirementcore.RuntimeV1Inventory{
		Runs: []*runmodel.Run{{
			ID: "run-v1", WorkspaceID: "ws-v1", State: common.RunStateBlockedOnDecision,
			Lineage: runmodel.RunLineage{LineageState: "active"}, CreatedAt: "created", StartedAt: "started",
		}},
		Workspaces: []*workspacemodel.Workspace{{ID: "ws-v1", Title: "workspace", State: common.WorkspaceStateBlockedOnApproval, ActiveRunID: "run-v1"}},
		Tickets:    []*decisionmodel.Ticket{{ID: "ticket-v1", WorkspaceID: "ws-v1", RunID: "run-v1", State: common.DecisionTicketStateOpen}},
		Approvals:  []*approvalmodel.Approval{{ID: "approval-v1", WorkspaceID: "ws-v1", RunID: "run-v1", State: common.ApprovalStatePending}},
	}
	plan := retirementcore.BuildRuntimeV1RetirementPlan(inventory)
	if err := ApplyRuntimeV1RetirementPlan(context.Background(), hub, inventory, plan); err != nil {
		t.Fatalf("ApplyRuntimeV1RetirementPlan: %v", err)
	}
	if runUpdate.State != common.RunStateSuperseded || runUpdate.FailureReason != retirementcore.RuntimeV1RetirementReason || runUpdate.BlockingTicketID != "" || runUpdate.BlockingApprovalID != "" || runUpdate.Lineage.LineageState != "superseded" || runUpdate.CreatedAt != "created" || runUpdate.StartedAt != "started" || runUpdate.EndedAt == "" {
		t.Fatalf("run update = %+v", runUpdate)
	}
	if workspaceUpdate.State != common.WorkspaceStateActive || workspaceUpdate.ActiveRunID != "" || workspaceUpdate.Title != "workspace" {
		t.Fatalf("workspace update = %+v", workspaceUpdate)
	}
	if ticketUpdate.State != common.DecisionTicketStateSuperseded || ticketUpdate.TicketID != "ticket-v1" {
		t.Fatalf("ticket update = %+v", ticketUpdate)
	}
	if approvalUpdate.State != common.ApprovalStateSuperseded || approvalUpdate.ApprovalID != "approval-v1" {
		t.Fatalf("approval update = %+v", approvalUpdate)
	}
}

func TestRuntimeV1RetirementApplyIsIdempotentAcrossRealOwners(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memoryrepo.NewRunRepository()
	workspaces := memoryrepo.NewWorkspaceRepository()
	tickets := memoryrepo.NewDecisionTicketRepository()
	resolutions := memoryrepo.NewDecisionResolutionRepository()
	approvals := memoryrepo.NewApprovalRepository()

	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, Steps: memoryrepo.NewStepRepository(), Checkpoints: memoryrepo.NewRuntimeCheckpointRepository(), EventHub: hub}); err != nil {
		t.Fatalf("configure run owner: %v", err)
	}
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspacemodule.Dependencies{Workspaces: workspaces, Goals: memoryrepo.NewGoalRepository(), DoDs: memoryrepo.NewDoDRepository(), Templates: memoryrepo.NewWorkspaceTemplateRepository(), EventHub: hub}); err != nil {
		t.Fatalf("configure workspace owner: %v", err)
	}
	decisionOwner := decisionmodule.New()
	if err := decisionOwner.Configure(decisionmodule.Dependencies{Tickets: tickets, Resolutions: resolutions, EventHub: hub}); err != nil {
		t.Fatalf("configure decision owner: %v", err)
	}
	approvalOwner := approvalmodule.New()
	if err := approvalOwner.Configure(approvalmodule.Dependencies{Approvals: approvals, EventHub: hub}); err != nil {
		t.Fatalf("configure approval owner: %v", err)
	}
	for name, start := range map[string]func() error{"run": runOwner.Start, "workspace": workspaceOwner.Start, "decision": decisionOwner.Start, "approval": approvalOwner.Start} {
		if err := start(); err != nil {
			t.Fatalf("start %s owner: %v", name, err)
		}
	}
	t.Cleanup(func() {
		_ = approvalOwner.Stop(ctx)
		_ = decisionOwner.Stop(ctx)
		_ = workspaceOwner.Stop(ctx)
		_ = runOwner.Stop(ctx)
	})

	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v1-real", WorkspaceID: "ws-v1-real", State: common.RunStateBlockedOnDecision, BlockingTicketID: "ticket-v1-real", BlockingApprovalID: "approval-v1-real"}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v1-real", State: common.WorkspaceStateBlockedOnApproval, ActiveRunID: "run-v1-real"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := tickets.Create(ctx, &decisionmodel.Ticket{ID: "ticket-v1-real", WorkspaceID: "ws-v1-real", RunID: "run-v1-real", State: common.DecisionTicketStateOpen}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := approvals.Create(ctx, &approvalmodel.Approval{ID: "approval-v1-real", WorkspaceID: "ws-v1-real", RunID: "run-v1-real", State: common.ApprovalStatePending}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}

	inventory, err := LoadRuntimeV1Inventory(ctx, hub)
	if err != nil {
		t.Fatalf("load inventory: %v", err)
	}
	plan := retirementcore.BuildRuntimeV1RetirementPlan(inventory)
	if err := ApplyRuntimeV1RetirementPlan(ctx, hub, inventory, plan); err != nil {
		t.Fatalf("apply retirement: %v", err)
	}
	after, err := LoadRuntimeV1Inventory(ctx, hub)
	if err != nil {
		t.Fatalf("reload inventory: %v", err)
	}
	second := retirementcore.BuildRuntimeV1RetirementPlan(after)
	if len(second.Runs) != 0 || len(second.Workspaces) != 0 || len(second.Tickets) != 0 || len(second.Approvals) != 0 {
		t.Fatalf("second retirement plan = %+v", second)
	}
	if after.Runs[0].State != common.RunStateSuperseded || after.Runs[0].FailureReason != retirementcore.RuntimeV1RetirementReason || after.Workspaces[0].ActiveRunID != "" || after.Workspaces[0].State != common.WorkspaceStateActive || after.Tickets[0].State != common.DecisionTicketStateSuperseded || after.Approvals[0].State != common.ApprovalStateSuperseded {
		t.Fatalf("retired inventory = %+v", after)
	}
}

func registerRetirementApplyOwner(t *testing.T, hub magicevent.Hub, destination string, topic string, handle func(magicevent.Event) any) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(destination, hub)
	observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
		result.Set(handle(event), nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(topic) })
}
