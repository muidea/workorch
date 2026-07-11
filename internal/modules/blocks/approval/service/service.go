package approval

import (
	"context"

	approvalcontracts "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/contracts"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateApprovalCommand = approvalcontracts.CreateApprovalCommand
type ResolveApprovalCommand = approvalcontracts.ResolveApprovalCommand

type Service interface {
	CreateApproval(context.Context, CreateApprovalCommand) error
	EnsureApproval(context.Context, CreateApprovalCommand) error
	Approve(context.Context, ResolveApprovalCommand) error
	EnsureApproved(context.Context, ResolveApprovalCommand) error
	EnsureApprovedAndReturn(context.Context, ResolveApprovalCommand) (*approvalmodel.Approval, error)
	Reject(context.Context, ResolveApprovalCommand) error
	EnsureRejected(context.Context, ResolveApprovalCommand) error
	EnsureRejectedAndReturn(context.Context, ResolveApprovalCommand) (*approvalmodel.Approval, error)
}

type Module struct {
	createApproval          func(context.Context, CreateApprovalCommand) error
	ensureApproval          func(context.Context, CreateApprovalCommand) error
	approve                 func(context.Context, ResolveApprovalCommand) error
	ensureApproved          func(context.Context, ResolveApprovalCommand) error
	ensureApprovedAndReturn func(context.Context, ResolveApprovalCommand) (*approvalmodel.Approval, error)
	reject                  func(context.Context, ResolveApprovalCommand) error
	ensureRejected          func(context.Context, ResolveApprovalCommand) error
	ensureRejectedAndReturn func(context.Context, ResolveApprovalCommand) (*approvalmodel.Approval, error)
}

type Dependencies struct {
	CreateApproval          func(context.Context, CreateApprovalCommand) error
	EnsureApproval          func(context.Context, CreateApprovalCommand) error
	Approve                 func(context.Context, ResolveApprovalCommand) error
	EnsureApproved          func(context.Context, ResolveApprovalCommand) error
	EnsureApprovedAndReturn func(context.Context, ResolveApprovalCommand) (*approvalmodel.Approval, error)
	Reject                  func(context.Context, ResolveApprovalCommand) error
	EnsureRejected          func(context.Context, ResolveApprovalCommand) error
	EnsureRejectedAndReturn func(context.Context, ResolveApprovalCommand) (*approvalmodel.Approval, error)
}

func New(deps Dependencies) *Module {
	return &Module{
		createApproval:          deps.CreateApproval,
		ensureApproval:          deps.EnsureApproval,
		approve:                 deps.Approve,
		ensureApproved:          deps.EnsureApproved,
		ensureApprovedAndReturn: deps.EnsureApprovedAndReturn,
		reject:                  deps.Reject,
		ensureRejected:          deps.EnsureRejected,
		ensureRejectedAndReturn: deps.EnsureRejectedAndReturn,
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

func (m *Module) EnsureApprovedAndReturn(ctx context.Context, cmd ResolveApprovalCommand) (*approvalmodel.Approval, error) {
	if m == nil || m.ensureApprovedAndReturn == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.ensureApprovedAndReturn(ctx, cmd)
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

func (m *Module) EnsureRejectedAndReturn(ctx context.Context, cmd ResolveApprovalCommand) (*approvalmodel.Approval, error) {
	if m == nil || m.ensureRejectedAndReturn == nil {
		return nil, common.ErrInvalidArgument
	}
	return m.ensureRejectedAndReturn(ctx, cmd)
}
