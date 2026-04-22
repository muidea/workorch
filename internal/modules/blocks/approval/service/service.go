package approval

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateApprovalCommand struct {
	ApprovalID  common.ApprovalID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	Reason      string
}

type ResolveApprovalCommand struct {
	ApprovalID common.ApprovalID
}

type Service interface {
	CreateApproval(context.Context, CreateApprovalCommand) error
	EnsureApproval(context.Context, CreateApprovalCommand) error
	Approve(context.Context, ResolveApprovalCommand) error
	EnsureApproved(context.Context, ResolveApprovalCommand) error
	Reject(context.Context, ResolveApprovalCommand) error
	EnsureRejected(context.Context, ResolveApprovalCommand) error
}

type Module struct {
	createApproval func(context.Context, CreateApprovalCommand) error
	ensureApproval func(context.Context, CreateApprovalCommand) error
	approve        func(context.Context, ResolveApprovalCommand) error
	ensureApproved func(context.Context, ResolveApprovalCommand) error
	reject         func(context.Context, ResolveApprovalCommand) error
	ensureRejected func(context.Context, ResolveApprovalCommand) error
}

type Dependencies struct {
	CreateApproval func(context.Context, CreateApprovalCommand) error
	EnsureApproval func(context.Context, CreateApprovalCommand) error
	Approve        func(context.Context, ResolveApprovalCommand) error
	EnsureApproved func(context.Context, ResolveApprovalCommand) error
	Reject         func(context.Context, ResolveApprovalCommand) error
	EnsureRejected func(context.Context, ResolveApprovalCommand) error
}

func New(deps Dependencies) *Module {
	return &Module{
		createApproval: deps.CreateApproval,
		ensureApproval: deps.EnsureApproval,
		approve:        deps.Approve,
		ensureApproved: deps.EnsureApproved,
		reject:         deps.Reject,
		ensureRejected: deps.EnsureRejected,
	}
}

func (m *Module) CreateApproval(ctx context.Context, cmd CreateApprovalCommand) error {
	if m == nil || m.createApproval == nil {
		return common.ErrInvalidArgument
	}
	return m.createApproval(ctx, cmd)
}

func (m *Module) EnsureApproval(ctx context.Context, cmd CreateApprovalCommand) error {
	if m == nil || m.ensureApproval == nil {
		return common.ErrInvalidArgument
	}
	return m.ensureApproval(ctx, cmd)
}

func (m *Module) Approve(ctx context.Context, cmd ResolveApprovalCommand) error {
	if m == nil || m.approve == nil {
		return common.ErrInvalidArgument
	}
	return m.approve(ctx, cmd)
}

func (m *Module) EnsureApproved(ctx context.Context, cmd ResolveApprovalCommand) error {
	if m == nil || m.ensureApproved == nil {
		return common.ErrInvalidArgument
	}
	return m.ensureApproved(ctx, cmd)
}

func (m *Module) Reject(ctx context.Context, cmd ResolveApprovalCommand) error {
	if m == nil || m.reject == nil {
		return common.ErrInvalidArgument
	}
	return m.reject(ctx, cmd)
}

func (m *Module) EnsureRejected(ctx context.Context, cmd ResolveApprovalCommand) error {
	if m == nil || m.ensureRejected == nil {
		return common.ErrInvalidArgument
	}
	return m.ensureRejected(ctx, cmd)
}
