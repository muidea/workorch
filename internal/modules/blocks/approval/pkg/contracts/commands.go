package contracts

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type CreateApprovalCommand struct {
	ApprovalID     common.ApprovalID
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	StepID         common.StepID
	Reason         string
	Action         string
	Context        workorcheventpayload.Payload
	AutoGovernance workorcheventpayload.Payload
}

type UpdateApprovalCommand struct {
	ApprovalID     common.ApprovalID
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	StepID         common.StepID
	Reason         string
	Action         string
	Context        workorcheventpayload.Payload
	State          common.ApprovalState
	AutoGovernance workorcheventpayload.Payload
	ResolvedAt     string
	ResolvedBy     string
	CreatedAt      string
	UpdatedAt      string
}

type ResolveApprovalCommand struct {
	ApprovalID  common.ApprovalID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Context     workorcheventpayload.Payload
}

type DeleteApprovalCommand struct {
	ApprovalID  common.ApprovalID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
}
