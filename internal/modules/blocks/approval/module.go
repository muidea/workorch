package approval

import (
	approvalbiz "github.com/muidea/workorch/internal/modules/blocks/approval/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = approvalbiz.Service
type CreateApprovalCommand = approvalbiz.CreateApprovalCommand
type ResolveApprovalCommand = approvalbiz.ResolveApprovalCommand

func NewService(approvals repository.ApprovalRepository) *approvalbiz.Service {
	return approvalbiz.NewService(approvals)
}
