package contracts

import "github.com/muidea/workorch/internal/pkg/common"

type CreateApprovalCommand struct {
	ApprovalID     common.ApprovalID
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	StepID         common.StepID
	Reason         string
	Action         string
	Context        map[string]any
	AutoGovernance map[string]any
}

type UpdateApprovalCommand struct {
	ApprovalID     common.ApprovalID
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	StepID         common.StepID
	Reason         string
	Action         string
	Context        map[string]any
	State          common.ApprovalState
	AutoGovernance map[string]any
	ResolvedAt     string
	ResolvedBy     string
	CreatedAt      string
	UpdatedAt      string
}

type ResolveApprovalCommand struct {
	ApprovalID  common.ApprovalID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Context     map[string]any
}

type DeleteApprovalCommand struct {
	ApprovalID common.ApprovalID
}
