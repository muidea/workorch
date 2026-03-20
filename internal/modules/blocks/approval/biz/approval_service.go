package approval

import (
	"context"

	approvalModel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type CreateApprovalCommand struct {
	ApprovalID  common.ApprovalID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	Reason      string
}

type Service struct {
	approvals repository.ApprovalRepository
}

func NewService(approvals repository.ApprovalRepository) *Service {
	return &Service{approvals: approvals}
}

func (s *Service) CreateApproval(ctx context.Context, cmd CreateApprovalCommand) error {
	if cmd.ApprovalID == "" || cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}

	item := &approvalModel.Approval{
		ID:          cmd.ApprovalID,
		WorkspaceID: cmd.WorkspaceID,
		RunID:       cmd.RunID,
		StepID:      cmd.StepID,
		Reason:      cmd.Reason,
		State:       common.ApprovalStatePending,
	}
	return s.approvals.Create(ctx, item)
}
