package run

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateRunCommand struct {
	RunID            common.RunID
	WorkspaceID      common.WorkspaceID
	GoalID           common.GoalID
	DoDID            common.DoDID
	InitialStepKind  string
	ExecutionContext map[string]any
}

type StartRunCommand struct{ RunID common.RunID }
type ResumeRunCommand struct{ RunID common.RunID }

type BlockOnDecisionCommand struct {
	RunID    common.RunID
	TicketID common.TicketID
}

type BlockOnApprovalCommand struct {
	RunID      common.RunID
	ApprovalID common.ApprovalID
}

type CompleteRunCommand struct{ RunID common.RunID }
type ValidateRunCommand struct{ RunID common.RunID }
type CancelRunCommand struct{ RunID common.RunID }
type SupersedeRunCommand struct{ RunID common.RunID }

type FailRunCommand struct {
	RunID  common.RunID
	Reason string
}

type Service interface {
	CreateRun(context.Context, CreateRunCommand) error
	EnsureRun(context.Context, CreateRunCommand) error
	StartRun(context.Context, StartRunCommand) error
	EnsureStarted(context.Context, StartRunCommand) error
	ResumeRun(context.Context, ResumeRunCommand) error
	EnsureResumed(context.Context, ResumeRunCommand) error
	BlockOnDecision(context.Context, BlockOnDecisionCommand) error
	EnsureBlockedOnDecision(context.Context, BlockOnDecisionCommand) error
	BlockOnApproval(context.Context, BlockOnApprovalCommand) error
	EnsureBlockedOnApproval(context.Context, BlockOnApprovalCommand) error
	ValidateRun(context.Context, ValidateRunCommand) error
	CompleteRun(context.Context, CompleteRunCommand) error
	CancelRun(context.Context, CancelRunCommand) error
	SupersedeRun(context.Context, SupersedeRunCommand) error
	FailRun(context.Context, FailRunCommand) error
}

type Module struct {
	createRun       func(context.Context, CreateRunCommand) error
	ensureRun       func(context.Context, CreateRunCommand) error
	startRun        func(context.Context, StartRunCommand) error
	ensureStarted   func(context.Context, StartRunCommand) error
	resumeRun       func(context.Context, ResumeRunCommand) error
	ensureResumed   func(context.Context, ResumeRunCommand) error
	blockOnDecision func(context.Context, BlockOnDecisionCommand) error
	ensureDecision  func(context.Context, BlockOnDecisionCommand) error
	blockOnApproval func(context.Context, BlockOnApprovalCommand) error
	ensureApproval  func(context.Context, BlockOnApprovalCommand) error
	validateRun     func(context.Context, ValidateRunCommand) error
	completeRun     func(context.Context, CompleteRunCommand) error
	cancelRun       func(context.Context, CancelRunCommand) error
	supersedeRun    func(context.Context, SupersedeRunCommand) error
	failRun         func(context.Context, FailRunCommand) error
}

type Dependencies struct {
	CreateRun       func(context.Context, CreateRunCommand) error
	EnsureRun       func(context.Context, CreateRunCommand) error
	StartRun        func(context.Context, StartRunCommand) error
	EnsureStarted   func(context.Context, StartRunCommand) error
	ResumeRun       func(context.Context, ResumeRunCommand) error
	EnsureResumed   func(context.Context, ResumeRunCommand) error
	BlockOnDecision func(context.Context, BlockOnDecisionCommand) error
	EnsureDecision  func(context.Context, BlockOnDecisionCommand) error
	BlockOnApproval func(context.Context, BlockOnApprovalCommand) error
	EnsureApproval  func(context.Context, BlockOnApprovalCommand) error
	ValidateRun     func(context.Context, ValidateRunCommand) error
	CompleteRun     func(context.Context, CompleteRunCommand) error
	CancelRun       func(context.Context, CancelRunCommand) error
	SupersedeRun    func(context.Context, SupersedeRunCommand) error
	FailRun         func(context.Context, FailRunCommand) error
}

func New(deps Dependencies) *Module {
	return &Module{
		createRun:       deps.CreateRun,
		ensureRun:       deps.EnsureRun,
		startRun:        deps.StartRun,
		ensureStarted:   deps.EnsureStarted,
		resumeRun:       deps.ResumeRun,
		ensureResumed:   deps.EnsureResumed,
		blockOnDecision: deps.BlockOnDecision,
		ensureDecision:  deps.EnsureDecision,
		blockOnApproval: deps.BlockOnApproval,
		ensureApproval:  deps.EnsureApproval,
		validateRun:     deps.ValidateRun,
		completeRun:     deps.CompleteRun,
		cancelRun:       deps.CancelRun,
		supersedeRun:    deps.SupersedeRun,
		failRun:         deps.FailRun,
	}
}

func (m *Module) CreateRun(ctx context.Context, cmd CreateRunCommand) error {
	if m == nil || m.createRun == nil {
		return common.ErrInvalidArgument
	}
	return m.createRun(ctx, cmd)
}

func (m *Module) EnsureRun(ctx context.Context, cmd CreateRunCommand) error {
	if m == nil || m.ensureRun == nil {
		return common.ErrInvalidArgument
	}
	return m.ensureRun(ctx, cmd)
}

func (m *Module) StartRun(ctx context.Context, cmd StartRunCommand) error {
	if m == nil || m.startRun == nil {
		return common.ErrInvalidArgument
	}
	return m.startRun(ctx, cmd)
}

func (m *Module) EnsureStarted(ctx context.Context, cmd StartRunCommand) error {
	if m == nil || m.ensureStarted == nil {
		return common.ErrInvalidArgument
	}
	return m.ensureStarted(ctx, cmd)
}

func (m *Module) ResumeRun(ctx context.Context, cmd ResumeRunCommand) error {
	if m == nil || m.resumeRun == nil {
		return common.ErrInvalidArgument
	}
	return m.resumeRun(ctx, cmd)
}

func (m *Module) EnsureResumed(ctx context.Context, cmd ResumeRunCommand) error {
	if m == nil || m.ensureResumed == nil {
		return common.ErrInvalidArgument
	}
	return m.ensureResumed(ctx, cmd)
}

func (m *Module) BlockOnDecision(ctx context.Context, cmd BlockOnDecisionCommand) error {
	if m == nil || m.blockOnDecision == nil {
		return common.ErrInvalidArgument
	}
	return m.blockOnDecision(ctx, cmd)
}

func (m *Module) EnsureBlockedOnDecision(ctx context.Context, cmd BlockOnDecisionCommand) error {
	if m == nil || m.ensureDecision == nil {
		return common.ErrInvalidArgument
	}
	return m.ensureDecision(ctx, cmd)
}

func (m *Module) BlockOnApproval(ctx context.Context, cmd BlockOnApprovalCommand) error {
	if m == nil || m.blockOnApproval == nil {
		return common.ErrInvalidArgument
	}
	return m.blockOnApproval(ctx, cmd)
}

func (m *Module) EnsureBlockedOnApproval(ctx context.Context, cmd BlockOnApprovalCommand) error {
	if m == nil || m.ensureApproval == nil {
		return common.ErrInvalidArgument
	}
	return m.ensureApproval(ctx, cmd)
}

func (m *Module) CompleteRun(ctx context.Context, cmd CompleteRunCommand) error {
	if m == nil || m.completeRun == nil {
		return common.ErrInvalidArgument
	}
	return m.completeRun(ctx, cmd)
}

func (m *Module) ValidateRun(ctx context.Context, cmd ValidateRunCommand) error {
	if m == nil || m.validateRun == nil {
		return common.ErrInvalidArgument
	}
	return m.validateRun(ctx, cmd)
}

func (m *Module) CancelRun(ctx context.Context, cmd CancelRunCommand) error {
	if m == nil || m.cancelRun == nil {
		return common.ErrInvalidArgument
	}
	return m.cancelRun(ctx, cmd)
}

func (m *Module) SupersedeRun(ctx context.Context, cmd SupersedeRunCommand) error {
	if m == nil || m.supersedeRun == nil {
		return common.ErrInvalidArgument
	}
	return m.supersedeRun(ctx, cmd)
}

func (m *Module) FailRun(ctx context.Context, cmd FailRunCommand) error {
	if m == nil || m.failRun == nil {
		return common.ErrInvalidArgument
	}
	return m.failRun(ctx, cmd)
}
