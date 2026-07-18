package decision

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type Ticket struct {
	ID                        common.TicketID
	WorkspaceID               common.WorkspaceID
	RunID                     common.RunID
	StepID                    common.StepID
	Reason                    string
	DecisionKind              string
	Question                  string
	Context                   workorcheventpayload.Payload
	State                     common.DecisionTicketState
	AutoGovernance            workorcheventpayload.Payload
	FailureClass              string
	FailureSignature          string
	AllowedRecoveryActions    []string
	RuntimeGovernanceProposal *RuntimeGovernanceResolutionProposal
	ResolvedAt                string
	ResolvedBy                string
	CreatedAt                 string
	UpdatedAt                 string
}
