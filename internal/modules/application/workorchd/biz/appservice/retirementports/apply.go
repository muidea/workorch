package retirementports

import (
	"context"
	"fmt"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/retirementcore"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventvalue"
)

func ApplyRuntimeV1RetirementPlan(ctx context.Context, hub magicevent.Hub, inventory retirementcore.RuntimeV1Inventory, plan retirementcore.RuntimeV1RetirementPlan) error {
	if hub == nil {
		return common.ErrInvalidArgument
	}
	runs := runtimeV1RunsByID(inventory)
	workspaces := runtimeV1WorkspacesByID(inventory)
	tickets := runtimeV1TicketsByID(inventory)
	approvals := runtimeV1ApprovalsByID(inventory)

	for _, action := range plan.Tickets {
		item := tickets[action.TicketID]
		if item == nil {
			return fmt.Errorf("%w: retirement ticket %s is missing from inventory", common.ErrNotFound, action.TicketID)
		}
		contextPayload, err := eventvalue.FromMap(item.Context)
		if err != nil {
			return err
		}
		autoGovernance, err := eventvalue.FromMap(item.AutoGovernance)
		if err != nil {
			return err
		}
		_, err = sendRetirementCommand[decisionevents.UpdateTicketCommand, decisionevents.DecisionCommandResult](ctx, hub, decisionevents.TopicDecisionCommandUpdateTicket, decisionevents.CommandDestination, "decision_ticket", decisionevents.UpdateTicketCommand{
			TicketID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, StepID: item.StepID,
			Reason: item.Reason, DecisionKind: item.DecisionKind, Question: item.Question, Context: contextPayload,
			State: common.DecisionTicketStateSuperseded, AutoGovernance: autoGovernance,
			FailureClass: item.FailureClass, FailureSignature: item.FailureSignature, AllowedRecoveryActions: item.AllowedRecoveryActions,
			ResolvedAt: item.ResolvedAt, ResolvedBy: item.ResolvedBy, CreatedAt: item.CreatedAt, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			return err
		}
	}
	for _, action := range plan.Approvals {
		item := approvals[action.ApprovalID]
		if item == nil {
			return fmt.Errorf("%w: retirement approval %s is missing from inventory", common.ErrNotFound, action.ApprovalID)
		}
		contextPayload, err := eventvalue.FromMap(item.Context)
		if err != nil {
			return err
		}
		autoGovernance, err := eventvalue.FromMap(item.AutoGovernance)
		if err != nil {
			return err
		}
		_, err = sendRetirementCommand[approvalevents.UpdateApprovalCommand, approvalevents.ApprovalCommandResult](ctx, hub, approvalevents.TopicApprovalCommandUpdate, approvalevents.CommandDestination, "approval", approvalevents.UpdateApprovalCommand{
			ApprovalID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, StepID: item.StepID,
			Reason: item.Reason, Action: item.Action, Context: contextPayload, State: common.ApprovalStateSuperseded,
			AutoGovernance: autoGovernance, ResolvedAt: item.ResolvedAt, ResolvedBy: item.ResolvedBy, CreatedAt: item.CreatedAt, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			return err
		}
	}
	for _, action := range plan.Runs {
		item := runs[action.RunID]
		if item == nil {
			return fmt.Errorf("%w: retirement run %s is missing from inventory", common.ErrNotFound, action.RunID)
		}
		endedAt := item.EndedAt
		if endedAt == "" {
			endedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		lineage := item.Lineage
		if lineage.LineageState == "" || lineage.LineageState == "active" {
			lineage.LineageState = "superseded"
		}
		_, err := sendRetirementCommand[runevents.UpdateRunCommand, runevents.RunCommandResult](ctx, hub, runevents.TopicRunCommandUpdate, runevents.CommandDestination, "run", runevents.UpdateRunCommand{
			RunID: item.ID, WorkspaceID: item.WorkspaceID, GoalID: item.GoalID, DoDID: item.DoDID, Lineage: lineage,
			DefinitionRevision: item.DefinitionRevision, InitialStepKind: item.InitialStepKind,
			RuntimeBootstrap: item.RuntimeBootstrap, RuntimeContractVersion: item.RuntimeContractVersion, RuntimeState: item.RuntimeState,
			State: common.RunStateSuperseded, Priority: item.Priority, FailureReason: action.Reason,
			CreatedAt: item.CreatedAt, StartedAt: item.StartedAt, EndedAt: endedAt,
		})
		if err != nil {
			return err
		}
	}
	for _, action := range plan.Workspaces {
		item := workspaces[action.WorkspaceID]
		if item == nil {
			return fmt.Errorf("%w: retirement workspace %s is missing from inventory", common.ErrNotFound, action.WorkspaceID)
		}
		contextHints, err := eventvalue.FromMap(item.ContextHints)
		if err != nil {
			return err
		}
		evidencePolicies, err := eventvalue.FromMapSlice(item.EvidencePolicies)
		if err != nil {
			return err
		}
		_, err = sendRetirementCommand[workspaceevents.UpdateWorkspaceCommand, workspaceevents.WorkspaceCommandResult](ctx, hub, workspaceevents.TopicWorkspaceCommandUpdate, workspaceevents.CommandDestination, "workspace", workspaceevents.UpdateWorkspaceCommand{
			WorkspaceID: item.ID, Title: item.Title, Description: item.Description,
			DeliverableDirectory: item.DeliverableDirectory, InternalDirectory: item.InternalDirectory,
			State: common.WorkspaceStateActive, TemplateID: item.TemplateID, ProfileID: item.ProfileID,
			DefaultSoulID: item.DefaultSoulID, SoulBindings: item.SoulBindings, ContextHints: contextHints, EvidencePolicies: evidencePolicies,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func runtimeV1RunsByID(inventory retirementcore.RuntimeV1Inventory) map[common.RunID]*runmodel.Run {
	ret := make(map[common.RunID]*runmodel.Run, len(inventory.Runs))
	for _, item := range inventory.Runs {
		if item != nil && item.ID != "" {
			ret[item.ID] = item
		}
	}
	return ret
}

func runtimeV1WorkspacesByID(inventory retirementcore.RuntimeV1Inventory) map[common.WorkspaceID]*workspacemodel.Workspace {
	ret := make(map[common.WorkspaceID]*workspacemodel.Workspace, len(inventory.Workspaces))
	for _, item := range inventory.Workspaces {
		if item != nil && item.ID != "" {
			ret[item.ID] = item
		}
	}
	return ret
}

func runtimeV1TicketsByID(inventory retirementcore.RuntimeV1Inventory) map[common.TicketID]*decisionmodel.Ticket {
	ret := make(map[common.TicketID]*decisionmodel.Ticket, len(inventory.Tickets))
	for _, item := range inventory.Tickets {
		if item != nil && item.ID != "" {
			ret[item.ID] = item
		}
	}
	return ret
}

func runtimeV1ApprovalsByID(inventory retirementcore.RuntimeV1Inventory) map[common.ApprovalID]*approvalmodel.Approval {
	ret := make(map[common.ApprovalID]*approvalmodel.Approval, len(inventory.Approvals))
	for _, item := range inventory.Approvals {
		if item != nil && item.ID != "" {
			ret[item.ID] = item
		}
	}
	return ret
}
