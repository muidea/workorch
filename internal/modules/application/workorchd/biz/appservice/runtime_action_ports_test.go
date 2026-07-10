package appservice

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimeports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestEventHubActionPortReturnsInvalidArgumentWhenHubOrRequiredInputMissing(t *testing.T) {
	port := runtimeports.NewEventHubActionPort(nil)
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "workspace ensure no hub", run: func() error { return port.EnsureWorkspace(context.Background(), "ws", "title") }},
		{name: "run ensure no hub", run: func() error { return port.EnsureRun(context.Background(), "run", "ws") }},
		{name: "run started no hub", run: func() error { return port.EnsureRunStarted(context.Background(), "run") }},
		{name: "decision ticket no hub", run: func() error {
			return port.EnsureTicket(context.Background(), "ticket", "ws", "run", "run/manual-gate", "reason")
		}},
		{name: "approval ensure no hub", run: func() error {
			return port.EnsureApproval(context.Background(), "approval", "ws", "run", "run/manual-approval", "reason")
		}},
		{name: "workspace ensure missing id", run: func() error {
			return runtimeports.NewEventHubActionPort(events.NewMemoryHub()).EnsureWorkspace(context.Background(), "", "title")
		}},
		{name: "run ensure missing workspace", run: func() error {
			return runtimeports.NewEventHubActionPort(events.NewMemoryHub()).EnsureRun(context.Background(), "run", "")
		}},
		{name: "run blocked decision", run: func() error {
			return runtimeports.NewEventHubActionPort(events.NewMemoryHub()).EnsureRunBlockedOnDecision(context.Background(), "run", "")
		}},
		{name: "run blocked approval", run: func() error {
			return runtimeports.NewEventHubActionPort(events.NewMemoryHub()).EnsureRunBlockedOnApproval(context.Background(), "", "approval")
		}},
		{name: "run resumed missing id", run: func() error {
			return runtimeports.NewEventHubActionPort(events.NewMemoryHub()).EnsureRunResumed(context.Background(), "")
		}},
		{name: "decision resolved missing resolution", run: func() error {
			return runtimeports.NewEventHubActionPort(events.NewMemoryHub()).EnsureResolvedTicket(context.Background(), "", "ticket", "provide_missing_info", nil)
		}},
		{name: "approval approved missing id", run: func() error {
			return runtimeports.NewEventHubActionPort(events.NewMemoryHub()).EnsureApprovalApproved(context.Background(), "")
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("err = %v, want invalid argument", err)
			}
		})
	}
}

func TestEventHubActionPortPublishesOwnerCommands(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-action-port")
	rawHub := magicevent.NewHub(16)
	hub := events.NewMagicHubWithHub(rawHub)
	port := runtimeports.NewEventHubActionPort(hub)
	wantTopics := []string{
		workspaceevents.TopicWorkspaceCommandEnsure,
		runevents.TopicRunCommandEnsure,
		orchestratorevents.TopicOrchestratorCommandStartRun,
		orchestratorevents.TopicOrchestratorCommandBlockDecision,
		orchestratorevents.TopicOrchestratorCommandBlockApproval,
		orchestratorevents.TopicOrchestratorCommandResumeRun,
		decisionevents.TopicDecisionCommandEnsureTicket,
		decisionevents.TopicDecisionCommandEnsureResolved,
		approvalevents.TopicApprovalCommandEnsure,
		approvalevents.TopicApprovalCommandEnsureApproved,
		approvalevents.TopicApprovalCommandEnsureRejected,
	}
	captured := map[string]any{}
	subscribeRuntimeActionCommand(t, rawHub, workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandEnsure, captured)
	subscribeRuntimeActionCommand(t, rawHub, runevents.CommandDestination, runevents.TopicRunCommandEnsure, captured)
	subscribeRuntimeActionCommand(t, rawHub, orchestratorevents.CommandDestination, orchestratorevents.TopicOrchestratorCommandStartRun, captured)
	subscribeRuntimeActionCommand(t, rawHub, orchestratorevents.CommandDestination, orchestratorevents.TopicOrchestratorCommandBlockDecision, captured)
	subscribeRuntimeActionCommand(t, rawHub, orchestratorevents.CommandDestination, orchestratorevents.TopicOrchestratorCommandBlockApproval, captured)
	subscribeRuntimeActionCommand(t, rawHub, orchestratorevents.CommandDestination, orchestratorevents.TopicOrchestratorCommandResumeRun, captured)
	subscribeRuntimeActionCommand(t, rawHub, decisionevents.CommandDestination, decisionevents.TopicDecisionCommandEnsureTicket, captured)
	subscribeRuntimeActionCommand(t, rawHub, decisionevents.CommandDestination, decisionevents.TopicDecisionCommandEnsureResolved, captured)
	subscribeRuntimeActionCommand(t, rawHub, approvalevents.CommandDestination, approvalevents.TopicApprovalCommandEnsure, captured)
	subscribeRuntimeActionCommand(t, rawHub, approvalevents.CommandDestination, approvalevents.TopicApprovalCommandEnsureApproved, captured)
	subscribeRuntimeActionCommand(t, rawHub, approvalevents.CommandDestination, approvalevents.TopicApprovalCommandEnsureRejected, captured)

	if err := port.EnsureWorkspace(ctx, "ws-action-port", "workspace action port"); err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	if err := port.EnsureRun(ctx, "run-action-port", "ws-action-port"); err != nil {
		t.Fatalf("EnsureRun: %v", err)
	}
	if err := port.EnsureRunStarted(ctx, "run-action-port"); err != nil {
		t.Fatalf("EnsureRunStarted: %v", err)
	}
	if err := port.EnsureRunBlockedOnDecision(ctx, "run-action-port", "ticket-action-port"); err != nil {
		t.Fatalf("EnsureRunBlockedOnDecision: %v", err)
	}
	if err := port.EnsureRunBlockedOnApproval(ctx, "run-action-port", "approval-action-port"); err != nil {
		t.Fatalf("EnsureRunBlockedOnApproval: %v", err)
	}
	if err := port.EnsureRunResumed(ctx, "run-action-port"); err != nil {
		t.Fatalf("EnsureRunResumed: %v", err)
	}
	payload := map[string]any{"operator": "action-port"}
	if err := port.EnsureTicket(ctx, "ticket-action-port", "ws-action-port", "run-action-port", "run-action-port/manual-gate", "manual decision"); err != nil {
		t.Fatalf("EnsureTicket: %v", err)
	}
	if err := port.EnsureResolvedTicket(ctx, "resolution-action-port", "ticket-action-port", "provide_missing_info", payload); err != nil {
		t.Fatalf("EnsureResolvedTicket: %v", err)
	}
	if err := port.EnsureApproval(ctx, "approval-action-port", "ws-action-port", "run-action-port", "run-action-port/manual-approval", "manual approval"); err != nil {
		t.Fatalf("EnsureApproval: %v", err)
	}
	if err := port.EnsureApprovalApproved(ctx, "approval-action-port"); err != nil {
		t.Fatalf("EnsureApprovalApproved: %v", err)
	}
	if err := port.EnsureApprovalRejected(ctx, "approval-action-port"); err != nil {
		t.Fatalf("EnsureApprovalRejected: %v", err)
	}

	for _, topic := range wantTopics {
		if captured[topic] == nil {
			t.Fatalf("missing captured command for %s", topic)
		}
	}
	workspaceCmd, ok := captured[workspaceevents.TopicWorkspaceCommandEnsure].(workspaceevents.EnsureWorkspaceCommand)
	if !ok || workspaceCmd.WorkspaceID != "ws-action-port" || workspaceCmd.Title != "workspace action port" {
		t.Fatalf("workspace ensure command = %#v", captured[workspaceevents.TopicWorkspaceCommandEnsure])
	}
	runEnsureCmd, ok := captured[runevents.TopicRunCommandEnsure].(runevents.EnsureRunCommand)
	if !ok || runEnsureCmd.RunID != "run-action-port" || runEnsureCmd.WorkspaceID != "ws-action-port" {
		t.Fatalf("run ensure command = %#v", captured[runevents.TopicRunCommandEnsure])
	}
	runStartCmd, ok := captured[orchestratorevents.TopicOrchestratorCommandStartRun].(orchestratorevents.StartRunCommand)
	if !ok || runStartCmd.RunID != "run-action-port" {
		t.Fatalf("run start command = %#v", captured[orchestratorevents.TopicOrchestratorCommandStartRun])
	}
	blockDecisionCmd, ok := captured[orchestratorevents.TopicOrchestratorCommandBlockDecision].(orchestratorevents.BlockDecisionCommand)
	if !ok || blockDecisionCmd.RunID != "run-action-port" || blockDecisionCmd.TicketID != "ticket-action-port" {
		t.Fatalf("block decision command = %#v", captured[orchestratorevents.TopicOrchestratorCommandBlockDecision])
	}
	blockApprovalCmd, ok := captured[orchestratorevents.TopicOrchestratorCommandBlockApproval].(orchestratorevents.BlockApprovalCommand)
	if !ok || blockApprovalCmd.RunID != "run-action-port" || blockApprovalCmd.ApprovalID != "approval-action-port" {
		t.Fatalf("block approval command = %#v", captured[orchestratorevents.TopicOrchestratorCommandBlockApproval])
	}
	resumeCmd, ok := captured[orchestratorevents.TopicOrchestratorCommandResumeRun].(orchestratorevents.ResumeRunCommand)
	if !ok || resumeCmd.RunID != "run-action-port" || resumeCmd.WorkspaceID != "ws-action-port" {
		t.Fatalf("run resume command = %#v", captured[orchestratorevents.TopicOrchestratorCommandResumeRun])
	}
	ticketCmd, ok := captured[decisionevents.TopicDecisionCommandEnsureTicket].(decisionevents.EnsureTicketCommand)
	if !ok || ticketCmd.TicketID != "ticket-action-port" || ticketCmd.WorkspaceID != "ws-action-port" || ticketCmd.RunID != "run-action-port" {
		t.Fatalf("decision ensure command = %#v", captured[decisionevents.TopicDecisionCommandEnsureTicket])
	}
	resolveCmd, ok := captured[decisionevents.TopicDecisionCommandEnsureResolved].(decisionevents.EnsureResolvedCommand)
	if !ok || resolveCmd.ResolutionID != "resolution-action-port" || resolveCmd.TicketID != "ticket-action-port" || resolveCmd.Payload["operator"] != "action-port" {
		t.Fatalf("decision resolve command = %#v", captured[decisionevents.TopicDecisionCommandEnsureResolved])
	}
	approvalCmd, ok := captured[approvalevents.TopicApprovalCommandEnsure].(approvalevents.EnsureApprovalCommand)
	if !ok || approvalCmd.ApprovalID != "approval-action-port" || approvalCmd.WorkspaceID != "ws-action-port" || approvalCmd.RunID != "run-action-port" {
		t.Fatalf("approval ensure command = %#v", captured[approvalevents.TopicApprovalCommandEnsure])
	}
	approvedCmd, ok := captured[approvalevents.TopicApprovalCommandEnsureApproved].(approvalevents.EnsureApprovedCommand)
	if !ok || approvedCmd.ApprovalID != "approval-action-port" || approvedCmd.WorkspaceID != "ws-action-port" {
		t.Fatalf("approval approved command = %#v", captured[approvalevents.TopicApprovalCommandEnsureApproved])
	}
	rejectedCmd, ok := captured[approvalevents.TopicApprovalCommandEnsureRejected].(approvalevents.EnsureRejectedCommand)
	if !ok || rejectedCmd.ApprovalID != "approval-action-port" || rejectedCmd.WorkspaceID != "ws-action-port" {
		t.Fatalf("approval rejected command = %#v", captured[approvalevents.TopicApprovalCommandEnsureRejected])
	}
}

func subscribeRuntimeActionCommand(t *testing.T, rawHub magicevent.Hub, destination string, topic string, captured map[string]any) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(destination, rawHub)
	observer.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
		captured[topic] = ev.Data()
		result.Set(struct{}{}, nil)
	})
}
