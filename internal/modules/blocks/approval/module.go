package approval

import (
	"context"

	approvalbiz "github.com/muidea/workorch/internal/modules/blocks/approval/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	approvalservice "github.com/muidea/workorch/internal/modules/blocks/approval/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = approvalservice.Service
type CreateApprovalCommand = approvalservice.CreateApprovalCommand
type ResolveApprovalCommand = approvalservice.ResolveApprovalCommand

type Dependencies struct {
	Approvals repository.ApprovalRepository
	EventHub  events.Hub
}

type Module struct {
	bizPtr     *approvalbiz.Service
	servicePtr *approvalservice.Module
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Setup(deps Dependencies) error {
	if deps.Approvals == nil {
		return common.ErrInvalidArgument
	}
	m.bizPtr = approvalbiz.NewService(deps.Approvals, deps.EventHub)
	m.servicePtr = approvalservice.New(approvalservice.Dependencies{
		CreateApproval: m.bizPtr.CreateApproval,
		EnsureApproval: m.bizPtr.EnsureApproval,
		Approve:        m.bizPtr.Approve,
		EnsureApproved: m.bizPtr.EnsureApproved,
		Reject:         m.bizPtr.Reject,
		EnsureRejected: m.bizPtr.EnsureRejected,
	})
	return nil
}

func (m *Module) Run() error { return nil }

func (m *Module) Teardown(ctx context.Context) error {
	_ = ctx
	m.servicePtr = nil
	m.bizPtr = nil
	return nil
}

func (m *Module) Service() Service {
	return m.servicePtr
}
