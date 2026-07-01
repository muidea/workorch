package workorchownercontract

import "github.com/muidea/workorch/internal/pkg/common"

const TopicApprovalCommandGet = "/approval/command/get"

type Approval struct {
	ID          common.ApprovalID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	Reason      string
	Action      string
	Context     map[string]any
	State       common.ApprovalState
	CreatedAt   string
	UpdatedAt   string
}
