package orchestrator

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type Service interface {
	StartRun(context.Context, common.RunID) error
	DispatchNextStep(context.Context, common.RunID) error
	BlockOnDecision(context.Context, common.RunID, common.TicketID) error
	BlockOnApproval(context.Context, common.RunID, common.ApprovalID) error
	ResumeRun(context.Context, common.RunID) error
	ValidateRun(context.Context, common.RunID) error
	CompleteRun(context.Context, common.RunID) error
	CancelRun(context.Context, common.RunID) error
	SupersedeRun(context.Context, common.RunID) error
	FailRun(context.Context, common.RunID, string) error
}

type Module struct {
	startRun         func(context.Context, common.RunID) error
	dispatchNextStep func(context.Context, common.RunID) error
	blockOnDecision  func(context.Context, common.RunID, common.TicketID) error
	blockOnApproval  func(context.Context, common.RunID, common.ApprovalID) error
	resumeRun        func(context.Context, common.RunID) error
	validateRun      func(context.Context, common.RunID) error
	completeRun      func(context.Context, common.RunID) error
	cancelRun        func(context.Context, common.RunID) error
	supersedeRun     func(context.Context, common.RunID) error
	failRun          func(context.Context, common.RunID, string) error
}

type Dependencies struct {
	StartRun         func(context.Context, common.RunID) error
	DispatchNextStep func(context.Context, common.RunID) error
	BlockOnDecision  func(context.Context, common.RunID, common.TicketID) error
	BlockOnApproval  func(context.Context, common.RunID, common.ApprovalID) error
	ResumeRun        func(context.Context, common.RunID) error
	ValidateRun      func(context.Context, common.RunID) error
	CompleteRun      func(context.Context, common.RunID) error
	CancelRun        func(context.Context, common.RunID) error
	SupersedeRun     func(context.Context, common.RunID) error
	FailRun          func(context.Context, common.RunID, string) error
}

func New(deps Dependencies) *Module {
	return &Module{
		startRun:         deps.StartRun,
		dispatchNextStep: deps.DispatchNextStep,
		blockOnDecision:  deps.BlockOnDecision,
		blockOnApproval:  deps.BlockOnApproval,
		resumeRun:        deps.ResumeRun,
		validateRun:      deps.ValidateRun,
		completeRun:      deps.CompleteRun,
		cancelRun:        deps.CancelRun,
		supersedeRun:     deps.SupersedeRun,
		failRun:          deps.FailRun,
	}
}

func (m *Module) StartRun(ctx context.Context, runID common.RunID) error {
	if m == nil || m.startRun == nil {
		return common.ErrInvalidArgument
	}
	return m.startRun(ctx, runID)
}

func (m *Module) DispatchNextStep(ctx context.Context, runID common.RunID) error {
	if m == nil || m.dispatchNextStep == nil {
		return common.ErrInvalidArgument
	}
	return m.dispatchNextStep(ctx, runID)
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

func (m *Module) ResumeRun(ctx context.Context, runID common.RunID) error {
	if m == nil || m.resumeRun == nil {
		return common.ErrInvalidArgument
	}
	return m.resumeRun(ctx, runID)
}

func (m *Module) CompleteRun(ctx context.Context, runID common.RunID) error {
	if m == nil || m.completeRun == nil {
		return common.ErrInvalidArgument
	}
	return m.completeRun(ctx, runID)
}

func (m *Module) ValidateRun(ctx context.Context, runID common.RunID) error {
	if m == nil || m.validateRun == nil {
		return common.ErrInvalidArgument
	}
	return m.validateRun(ctx, runID)
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
