package decision

import "github.com/muidea/workorch/internal/pkg/common"

type Ticket struct {
	ID                     common.TicketID
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	StepID                 common.StepID
	Reason                 string
	DecisionKind           string
	Question               string
	Context                map[string]any
	State                  common.DecisionTicketState
	AutoGovernance         map[string]any
	FailureClass           string
	FailureSignature       string
	AllowedRecoveryActions []string
	ResolvedAt             string
	ResolvedBy             string
	CreatedAt              string
	UpdatedAt              string
}
