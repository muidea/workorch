package approval

import "github.com/muidea/workorch/internal/pkg/common"

type Approval struct {
	ID          common.ApprovalID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	Reason      string
	State       common.ApprovalState
}
