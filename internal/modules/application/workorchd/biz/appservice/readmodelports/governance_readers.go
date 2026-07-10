package readmodelports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type AutoGovernanceTraceReader struct {
	Hub magicevent.Hub
}

func (r AutoGovernanceTraceReader) List(ctx context.Context, query repository.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error) {
	return ownerreadports.ListAutoGovernanceTracesByOwnerCommand(ctx, r.Hub, query)
}

type DecisionTicketReader struct {
	Hub magicevent.Hub
}

func (r DecisionTicketReader) Get(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Ticket, error) {
	return ownerreadports.ReadDecisionTicketByOwnerCommand(ctx, r.Hub, ticketID)
}

func (r DecisionTicketReader) List(ctx context.Context) ([]*decisionmodel.Ticket, error) {
	return ownerreadports.ListDecisionTicketsByOwnerCommand(ctx, r.Hub)
}

func (r DecisionTicketReader) ListByRunID(ctx context.Context, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	return ownerreadports.ListDecisionTicketsByRunOwnerCommand(ctx, r.Hub, runID)
}

func (r DecisionTicketReader) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	return ownerreadports.ListDecisionTicketsByWorkspaceOwnerCommand(ctx, r.Hub, workspaceID)
}

type DecisionResolutionReader struct {
	Hub magicevent.Hub
}

func (r DecisionResolutionReader) GetByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	return ownerreadports.ReadDecisionResolutionByTicketOwnerCommand(ctx, r.Hub, ticketID)
}

type ApprovalReader struct {
	Hub magicevent.Hub
}

func (r ApprovalReader) Get(ctx context.Context, approvalID common.ApprovalID) (*approvalmodel.Approval, error) {
	return ownerreadports.ReadApprovalByOwnerCommand(ctx, r.Hub, approvalID)
}

func (r ApprovalReader) List(ctx context.Context) ([]*approvalmodel.Approval, error) {
	return ownerreadports.ListApprovalsByOwnerCommand(ctx, r.Hub)
}

func (r ApprovalReader) ListByRunID(ctx context.Context, runID common.RunID) ([]*approvalmodel.Approval, error) {
	return ownerreadports.ListApprovalsByRunOwnerCommand(ctx, r.Hub, runID)
}

func (r ApprovalReader) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*approvalmodel.Approval, error) {
	return ownerreadports.ListApprovalsByWorkspaceOwnerCommand(ctx, r.Hub, workspaceID)
}
