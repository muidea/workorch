package orchestrator

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type Service interface {
	BlockOnDecision(context.Context, common.RunID, common.TicketID) error
	BlockOnApproval(context.Context, common.RunID, common.ApprovalID) error
	CancelRun(context.Context, common.RunID) error
	SupersedeRun(context.Context, common.RunID) error
	FailRun(context.Context, common.RunID, string) error
}

type Module struct {
	blockOnDecision func(context.Context, common.RunID, common.TicketID) error
	blockOnApproval func(context.Context, common.RunID, common.ApprovalID) error
	cancelRun       func(context.Context, common.RunID) error
	supersedeRun    func(context.Context, common.RunID) error
	failRun         func(context.Context, common.RunID, string) error
}

type Dependencies struct {
	BlockOnDecision func(context.Context, common.RunID, common.TicketID) error
	BlockOnApproval func(context.Context, common.RunID, common.ApprovalID) error
	CancelRun       func(context.Context, common.RunID) error
	SupersedeRun    func(context.Context, common.RunID) error
	FailRun         func(context.Context, common.RunID, string) error
}

func New(deps Dependencies) *Module {
	return &Module{
		blockOnDecision: deps.BlockOnDecision,
		blockOnApproval: deps.BlockOnApproval,
		cancelRun:       deps.CancelRun,
		supersedeRun:    deps.SupersedeRun,
		failRun:         deps.FailRun,
	}
}

func (m *Module) BlockOnDecision(ctx context.Context, runID common.RunID, ticketID common.TicketID) error {
	if m == nil || m.blockOnDecision == nil {
		return common.ErrInvalidArgument
	}
	return m.blockOnDecision(ctx, runID, ticketID)
}

func (m *Module) BlockOnApproval(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) error {
	if m == nil || m.blockOnApproval == nil {
		return common.ErrInvalidArgument
	}
	return m.blockOnApproval(ctx, runID, approvalID)
}

func (m *Module) CancelRun(ctx context.Context, runID common.RunID) error {
	if m == nil || m.cancelRun == nil {
		return common.ErrInvalidArgument
	}
	return m.cancelRun(ctx, runID)
}

func (m *Module) SupersedeRun(ctx context.Context, runID common.RunID) error {
	if m == nil || m.supersedeRun == nil {
		return common.ErrInvalidArgument
	}
	return m.supersedeRun(ctx, runID)
}

func (m *Module) FailRun(ctx context.Context, runID common.RunID, reason string) error {
	if m == nil || m.failRun == nil {
		return common.ErrInvalidArgument
	}
	return m.failRun(ctx, runID, reason)
}
