package appservice

import (
	"context"
	"errors"
	"reflect"
	"testing"

	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	approvalservice "github.com/muidea/workorch/internal/modules/blocks/approval/service"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	decisionservice "github.com/muidea/workorch/internal/modules/kernel/decision/service"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runservice "github.com/muidea/workorch/internal/modules/kernel/run/service"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	workspaceservice "github.com/muidea/workorch/internal/modules/kernel/workspace/service"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeActionPortsReturnInvalidArgumentWhenServiceMissing(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "workspace ensure", run: func() error { return newWorkspaceActionPort(nil).EnsureWorkspace(context.Background(), "ws", "title") }},
		{name: "run ensure", run: func() error { return newRunActionPort(nil).EnsureRun(context.Background(), "run", "ws") }},
		{name: "run started", run: func() error { return newRunActionPort(nil).EnsureRunStarted(context.Background(), "run") }},
		{name: "run blocked decision", run: func() error {
			return newRunActionPort(nil).EnsureRunBlockedOnDecision(context.Background(), "run", "ticket")
		}},
		{name: "run blocked approval", run: func() error {
			return newRunActionPort(nil).EnsureRunBlockedOnApproval(context.Background(), "run", "approval")
		}},
		{name: "run resumed", run: func() error { return newRunActionPort(nil).EnsureRunResumed(context.Background(), "run") }},
		{name: "decision ticket", run: func() error {
			return newDecisionActionPort(nil).EnsureTicket(context.Background(), "ticket", "ws", "run", "run/manual-gate", "reason")
		}},
		{name: "decision resolved", run: func() error {
			return newDecisionActionPort(nil).EnsureResolvedTicket(context.Background(), "resolution", "ticket", "approve_path", nil)
		}},
		{name: "approval ensure", run: func() error {
			return newApprovalActionPort(nil).EnsureApproval(context.Background(), "approval", "ws", "run", "run/manual-approval", "reason")
		}},
		{name: "approval approved", run: func() error {
			return newApprovalActionPort(nil).EnsureApprovalApproved(context.Background(), "approval")
		}},
		{name: "approval rejected", run: func() error {
			return newApprovalActionPort(nil).EnsureApprovalRejected(context.Background(), "approval")
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

func TestWorkspaceActionPortMapsEnsureWorkspaceCommand(t *testing.T) {
	var got workspacemodule.EnsureWorkspaceCommand
	port := newWorkspaceActionPort(workspaceservice.New(workspaceservice.Dependencies{
		EnsureWorkspace: func(_ context.Context, cmd workspacemodule.EnsureWorkspaceCommand) error {
			got = cmd
			return nil
		},
	}))

	if err := port.EnsureWorkspace(context.Background(), "ws-action-port", "workspace action port"); err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	if got.WorkspaceID != "ws-action-port" || got.Title != "workspace action port" {
		t.Fatalf("workspace command = %+v", got)
	}
}

func TestRunActionPortMapsCommands(t *testing.T) {
	var ensured runmodule.CreateRunCommand
	var started runmodule.StartRunCommand
	var blockedDecision runmodule.BlockOnDecisionCommand
	var blockedApproval runmodule.BlockOnApprovalCommand
	var resumed runmodule.ResumeRunCommand
	port := newRunActionPort(runservice.New(runservice.Dependencies{
		EnsureRun: func(_ context.Context, cmd runmodule.CreateRunCommand) error {
			ensured = cmd
			return nil
		},
		EnsureStarted: func(_ context.Context, cmd runmodule.StartRunCommand) error {
			started = cmd
			return nil
		},
		EnsureDecision: func(_ context.Context, cmd runmodule.BlockOnDecisionCommand) error {
			blockedDecision = cmd
			return nil
		},
		EnsureApproval: func(_ context.Context, cmd runmodule.BlockOnApprovalCommand) error {
			blockedApproval = cmd
			return nil
		},
		EnsureResumed: func(_ context.Context, cmd runmodule.ResumeRunCommand) error {
			resumed = cmd
			return nil
		},
	}))

	if err := port.EnsureRun(context.Background(), "run-action-port", "ws-action-port"); err != nil {
		t.Fatalf("EnsureRun: %v", err)
	}
	if err := port.EnsureRunStarted(context.Background(), "run-action-port"); err != nil {
		t.Fatalf("EnsureRunStarted: %v", err)
	}
	if err := port.EnsureRunBlockedOnDecision(context.Background(), "run-action-port", "ticket-action-port"); err != nil {
		t.Fatalf("EnsureRunBlockedOnDecision: %v", err)
	}
	if err := port.EnsureRunBlockedOnApproval(context.Background(), "run-action-port", "approval-action-port"); err != nil {
		t.Fatalf("EnsureRunBlockedOnApproval: %v", err)
	}
	if err := port.EnsureRunResumed(context.Background(), "run-action-port"); err != nil {
		t.Fatalf("EnsureRunResumed: %v", err)
	}

	if ensured.RunID != "run-action-port" || ensured.WorkspaceID != "ws-action-port" || ensured.GoalID != "" || ensured.DoDID != "" {
		t.Fatalf("ensure run command = %+v", ensured)
	}
	if started.RunID != "run-action-port" {
		t.Fatalf("start run command = %+v", started)
	}
	if blockedDecision.RunID != "run-action-port" || blockedDecision.TicketID != "ticket-action-port" {
		t.Fatalf("block decision command = %+v", blockedDecision)
	}
	if blockedApproval.RunID != "run-action-port" || blockedApproval.ApprovalID != "approval-action-port" {
		t.Fatalf("block approval command = %+v", blockedApproval)
	}
	if resumed.RunID != "run-action-port" {
		t.Fatalf("resume run command = %+v", resumed)
	}
}

func TestDecisionActionPortMapsCommands(t *testing.T) {
	var ticket decisionmodule.CreateTicketCommand
	var resolution decisionmodule.ResolveTicketCommand
	port := newDecisionActionPort(decisionservice.New(decisionservice.Dependencies{
		EnsureTicket: func(_ context.Context, cmd decisionmodule.CreateTicketCommand) error {
			ticket = cmd
			return nil
		},
		EnsureResolvedTicket: func(_ context.Context, cmd decisionmodule.ResolveTicketCommand) error {
			resolution = cmd
			return nil
		},
	}))

	payload := map[string]any{"operator": "action-port"}
	if err := port.EnsureTicket(context.Background(), "ticket-action-port", "ws-action-port", "run-action-port", "run-action-port/manual-gate", "manual decision"); err != nil {
		t.Fatalf("EnsureTicket: %v", err)
	}
	if err := port.EnsureResolvedTicket(context.Background(), "resolution-action-port", "ticket-action-port", "approve_path", payload); err != nil {
		t.Fatalf("EnsureResolvedTicket: %v", err)
	}

	if ticket.TicketID != "ticket-action-port" || ticket.WorkspaceID != "ws-action-port" || ticket.RunID != "run-action-port" || ticket.StepID != "run-action-port/manual-gate" || ticket.Reason != "manual decision" {
		t.Fatalf("ticket command = %+v", ticket)
	}
	if resolution.ResolutionID != "resolution-action-port" || resolution.TicketID != "ticket-action-port" || resolution.Type != "approve_path" {
		t.Fatalf("resolution command = %+v", resolution)
	}
	if !reflect.DeepEqual(resolution.Payload, payload) {
		t.Fatalf("resolution payload = %+v, want %+v", resolution.Payload, payload)
	}
}

func TestApprovalActionPortMapsCommands(t *testing.T) {
	var approval approvalmodule.CreateApprovalCommand
	var approved approvalmodule.ResolveApprovalCommand
	var rejected approvalmodule.ResolveApprovalCommand
	port := newApprovalActionPort(approvalservice.New(approvalservice.Dependencies{
		EnsureApproval: func(_ context.Context, cmd approvalmodule.CreateApprovalCommand) error {
			approval = cmd
			return nil
		},
		EnsureApproved: func(_ context.Context, cmd approvalmodule.ResolveApprovalCommand) error {
			approved = cmd
			return nil
		},
		EnsureRejected: func(_ context.Context, cmd approvalmodule.ResolveApprovalCommand) error {
			rejected = cmd
			return nil
		},
	}))

	if err := port.EnsureApproval(context.Background(), "approval-action-port", "ws-action-port", "run-action-port", "run-action-port/manual-approval", "manual approval"); err != nil {
		t.Fatalf("EnsureApproval: %v", err)
	}
	if err := port.EnsureApprovalApproved(context.Background(), "approval-action-port"); err != nil {
		t.Fatalf("EnsureApprovalApproved: %v", err)
	}
	if err := port.EnsureApprovalRejected(context.Background(), "approval-action-port"); err != nil {
		t.Fatalf("EnsureApprovalRejected: %v", err)
	}

	if approval.ApprovalID != "approval-action-port" || approval.WorkspaceID != "ws-action-port" || approval.RunID != "run-action-port" || approval.StepID != "run-action-port/manual-approval" || approval.Reason != "manual approval" {
		t.Fatalf("approval command = %+v", approval)
	}
	if approved.ApprovalID != "approval-action-port" {
		t.Fatalf("approved command = %+v", approved)
	}
	if rejected.ApprovalID != "approval-action-port" {
		t.Fatalf("rejected command = %+v", rejected)
	}
}
