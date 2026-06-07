package readmodelcore

import (
	"context"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type AutoGovernanceTraceReader interface {
	List(context.Context, repository.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error)
}

type DecisionTicketReader interface {
	Get(context.Context, common.TicketID) (*decisionmodel.Ticket, error)
	List(context.Context) ([]*decisionmodel.Ticket, error)
	ListByRunID(context.Context, common.RunID) ([]*decisionmodel.Ticket, error)
	ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*decisionmodel.Ticket, error)
}

type DecisionResolutionReader interface {
	GetByTicketID(context.Context, common.TicketID) (*decisionmodel.Resolution, error)
}

type ApprovalReader interface {
	Get(context.Context, common.ApprovalID) (*approvalmodel.Approval, error)
	List(context.Context) ([]*approvalmodel.Approval, error)
	ListByRunID(context.Context, common.RunID) ([]*approvalmodel.Approval, error)
	ListByWorkspaceID(context.Context, common.WorkspaceID) ([]*approvalmodel.Approval, error)
}
