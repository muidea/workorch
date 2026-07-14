package workorchownercontract

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const TopicApprovalCommandGet = "/approval/command/get"

type Approval struct {
	ID             common.ApprovalID
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
