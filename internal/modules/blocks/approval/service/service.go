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

type Service interface {
	CreateApproval(context.Context, CreateApprovalCommand) error
}
