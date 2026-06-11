package activationports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func WorkspaceGovernanceHistory(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID, limit int) []map[string]any {
	if hub == nil || workspaceID == "" || limit <= 0 {
		return nil
	}
	readCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	workspaceTickets, err := ownerreadports.ListDecisionTicketsByWorkspaceOwnerCommand(readCtx, hub, workspaceID)
	if err != nil {
		return nil
	}
	ret := make([]map[string]any, 0, min(limit, len(workspaceTickets)))
	for _, ticket := range workspaceTickets {
		if ticket == nil || ticket.State != common.DecisionTicketStateResolved {
			continue
		}
		run, err := ownerreadports.ReadRunByOwnerCommand(readCtx, hub, ticket.RunID)
		if err != nil || run == nil || run.WorkspaceID != workspaceID {
			continue
		}
		if goalID != "" && run.GoalID != goalID {
			continue
		}
		if dodID != "" && run.DoDID != dodID {
			continue
		}
		resolution, err := ownerreadports.ReadDecisionResolutionByTicketOwnerCommand(readCtx, hub, ticket.ID)
		if err != nil || resolution == nil {
			continue
		}
		ret = append(ret, common.RedactSensitiveMap(map[string]any{
			"ticketId":          string(ticket.ID),
			"runId":             string(ticket.RunID),
			"stepId":            string(ticket.StepID),
			"decisionKind":      ticket.DecisionKind,
			"reason":            ticket.Reason,
			"question":          ticket.Question,
			"ticketContext":     activationcore.CompactFailureContext(ticket.Context),
			"resolutionId":      string(resolution.ID),
			"resolutionType":    resolution.Type,
			"resolutionPayload": activationcore.CompactDecisionPayload(resolution.Payload),
		}))
		if len(ret) >= limit {
			break
		}
	}
	return ret
}

func RejectedResultContinuation(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID) (map[string]any, error) {
	if hub == nil || workspaceID == "" || goalID == "" || dodID == "" {
		return nil, nil
	}
	items, err := ownerreadports.ListResultsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
	if err != nil {
		return nil, err
	}
	latest := activationcore.LatestTerminalResultForDefinitions(items, goalID, dodID)
	if latest == nil || latest.State != common.ResultStateRejected {
		return nil, nil
	}
	return activationcore.RejectedResultContinuationContext(latest), nil
}
