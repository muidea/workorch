package approval

import "github.com/muidea/workorch/internal/pkg/common"

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
