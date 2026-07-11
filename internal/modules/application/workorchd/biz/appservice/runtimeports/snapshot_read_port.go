package runtimeports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type EventHubSnapshotReadPort struct {
	hub magicevent.Hub
}

func NewEventHubSnapshotReadPort(hub magicevent.Hub) runtimecore.SnapshotReadRuntime {
	if hub == nil {
		return nil
	}
	return EventHubSnapshotReadPort{hub: hub}
}

func (p EventHubSnapshotReadPort) GetWorkspaceByID(ctx context.Context, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	return ownerreadports.ReadWorkspaceByOwnerCommand(ctx, p.hub, workspaceID)
}

func (p EventHubSnapshotReadPort) GetRun(ctx context.Context, runID common.RunID) (*runmodel.Run, error) {
	return ownerreadports.ReadRunByOwnerCommand(ctx, p.hub, runID)
}

func (p EventHubSnapshotReadPort) GetStep(ctx context.Context, stepID common.StepID) (*runmodel.Step, error) {
	return ownerreadports.ReadStepByOwnerCommand(ctx, p.hub, stepID)
}

func (p EventHubSnapshotReadPort) GetTicket(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Ticket, error) {
	return ownerreadports.ReadDecisionTicketByOwnerCommand(ctx, p.hub, ticketID)
}

func (p EventHubSnapshotReadPort) GetResolutionByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	return ownerreadports.ReadDecisionResolutionByTicketOwnerCommand(ctx, p.hub, ticketID)
}

func (p EventHubSnapshotReadPort) GetApproval(ctx context.Context, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	return ownerreadports.ReadApprovalByOwnerCommand(ctx, p.hub, approvalID)
}
