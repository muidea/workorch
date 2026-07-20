package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (d *CLICommandDispatcher) handleTypedCLIGovernance(event magicevent.Event, result magicevent.Result) {
	ctx := ContextWithCommandHub(event.Context(), d.EventHub())
	switch command := event.Data().(type) {
	case workorchdevents.CLIDecisionResolveCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.Request.WorkspaceID)
		item, err := ResolveDecisionWithSession(ctx, d.borrow(), string(command.Request.TicketID), appmodelcore.ResolveDecisionCommand{ResolutionType: command.Request.ResolutionType, ResolutionPayload: command.Request.Payload.Clone()})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIDecisionResolveResult{Decision: readmodelports.PortDecisionInfo(item)}, nil)
	case workorchdevents.CLIDecisionBatchResolveCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		ids := make([]string, 0, len(command.TicketIDs))
		for _, id := range command.TicketIDs {
			ids = append(ids, string(id))
		}
		items, err := BatchResolveDecisionsWithSession(ctx, d.borrow(), appmodelcore.BatchResolveDecisionCommand{TicketIDs: ids, ResolutionType: command.ResolutionType, ResolutionPayload: command.Payload.Clone()})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIDecisionBatchResolveResult{Decisions: readmodelports.PortDecisionInfos(items)}, nil)
	case workorchdevents.CLIDecisionListCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		items, err := ListDecisionsWithSession(ctx, d.borrow(), string(command.WorkspaceID), "")
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIDecisionListResult{Decisions: readmodelports.PortDecisionInfos(items)}, nil)
	case workorchdevents.CLIDecisionGetCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := ReadDecisionWithSession(ctx, d.borrow(), string(command.TicketID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIDecisionGetResult{Decision: readmodelports.PortDecisionInfo(item)}, nil)
	case workorchdevents.CLIApprovalApproveCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := ResolveApprovalWithSession(ctx, d.borrow(), string(command.ApprovalID), appmodelcore.ResolveApprovalCommand{Action: "approve", Payload: command.Payload.Clone()})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIApprovalApproveResult{Approval: readmodelports.PortApprovalInfo(item)}, nil)
	case workorchdevents.CLIApprovalRejectCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := ResolveApprovalWithSession(ctx, d.borrow(), string(command.ApprovalID), appmodelcore.ResolveApprovalCommand{Action: "reject", Payload: command.Payload.Clone()})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIApprovalRejectResult{Approval: readmodelports.PortApprovalInfo(item)}, nil)
	case workorchdevents.CLIApprovalBatchResolveCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		ids := make([]string, 0, len(command.ApprovalIDs))
		for _, id := range command.ApprovalIDs {
			ids = append(ids, string(id))
		}
		items, err := BatchResolveApprovalsWithSession(ctx, d.borrow(), appmodelcore.BatchResolveApprovalCommand{ApprovalIDs: ids, Action: command.Action, Payload: command.Payload.Clone()})
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIApprovalBatchResolveResult{Approvals: readmodelports.PortApprovalInfos(items)}, nil)
	case workorchdevents.CLIApprovalListCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		items, err := ListApprovalsWithSession(ctx, d.borrow(), string(command.WorkspaceID), "")
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIApprovalListResult{Approvals: readmodelports.PortApprovalInfos(items)}, nil)
	case workorchdevents.CLIApprovalGetCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := ReadApprovalWithSession(ctx, d.borrow(), string(command.ApprovalID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIApprovalGetResult{Approval: readmodelports.PortApprovalInfo(item)}, nil)
	default:
		setInvalidTypedCLICommand(result)
	}
}
