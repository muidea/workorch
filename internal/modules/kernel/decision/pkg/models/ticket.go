package decision

import "github.com/muidea/workorch/internal/pkg/common"

type Ticket struct {
	ID          common.TicketID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	Reason      string
	State       common.DecisionTicketState
}
