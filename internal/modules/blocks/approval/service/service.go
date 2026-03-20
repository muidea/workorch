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
	Approve(context.Context, ResolveApprovalCommand) error
	Reject(context.Context, ResolveApprovalCommand) error
}
