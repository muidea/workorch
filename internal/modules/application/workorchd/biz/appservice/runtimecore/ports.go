package runtimecore

import (
	"context"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type SnapshotReadRuntime interface {
	GetWorkspaceByID(context.Context, common.WorkspaceID) (*workspacemodel.Workspace, error)
	GetRun(context.Context, common.RunID) (*runmodel.Run, error)
	GetStep(context.Context, common.StepID) (*runmodel.Step, error)
	GetTicket(context.Context, common.TicketID) (*decisionmodel.Ticket, error)
	GetResolutionByTicketID(context.Context, common.TicketID) (*decisionmodel.Resolution, error)
	GetApproval(context.Context, common.ApprovalID) (*approvalmodel.Approval, error)
}

type WorkspaceActionRuntime interface {
	EnsureWorkspace(context.Context, common.WorkspaceID, string) error
}

type RunActionRuntime interface {
	EnsureRun(context.Context, common.RunID, common.WorkspaceID) error
	EnsureRunStarted(context.Context, common.RunID) error
	EnsureRunBlockedOnDecision(context.Context, common.RunID, common.TicketID) error
	EnsureRunBlockedOnApproval(context.Context, common.RunID, common.ApprovalID) error
	EnsureRunResumed(context.Context, common.RunID) error
}

type DecisionActionRuntime interface {
	EnsureTicket(context.Context, common.TicketID, common.WorkspaceID, common.RunID, common.StepID, string) error
	EnsureResolvedTicket(context.Context, common.ResolutionID, common.TicketID, string, map[string]any) error
}

type ApprovalActionRuntime interface {
	EnsureApproval(context.Context, common.ApprovalID, common.WorkspaceID, common.RunID, common.StepID, string) error
	EnsureApprovalApproved(context.Context, common.ApprovalID) error
	EnsureApprovalRejected(context.Context, common.ApprovalID) error
}

type ActionRuntime interface {
	WorkspaceActionRuntime
	RunActionRuntime
	DecisionActionRuntime
	ApprovalActionRuntime
}

type Runtime interface {
	SnapshotReadRuntime
	ActionRuntime
}
