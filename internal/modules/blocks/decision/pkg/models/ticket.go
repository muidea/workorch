package decision

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type Ticket struct {
	ID                                common.TicketID
	WorkspaceID                       common.WorkspaceID
	RunID                             common.RunID
	StepID                            common.StepID
	Reason                            string
	DecisionKind                      string
	Question                          string
	Context                           workorcheventpayload.Payload
	State                             common.DecisionTicketState
	AutoGovernance                    workorcheventpayload.Payload
	FailureClass                      string
	FailureSignature                  string
	AllowedRecoveryActions            []string
	DefinitionClarification           *DefinitionClarificationBinding
	RuntimeGovernanceProposal         *RuntimeGovernanceResolutionProposal
	ResolvedAt                        string
	ResolvedBy                        string
	CreatedAt                         string
	UpdatedAt                         string
	CreatedNotificationPublishedAt    string
	ResolutionNotificationPublishedAt string
}

func CloneTicket(item *Ticket) *Ticket {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.Context = item.Context.Clone()
	cloned.AutoGovernance = item.AutoGovernance.Clone()
	cloned.AllowedRecoveryActions = append([]string(nil), item.AllowedRecoveryActions...)
	if item.DefinitionClarification != nil {
		cloned.DefinitionClarification = item.DefinitionClarification.Clone()
	}
	if item.RuntimeGovernanceProposal != nil {
		proposal := item.RuntimeGovernanceProposal.Clone()
		cloned.RuntimeGovernanceProposal = &proposal
	}
	return &cloned
}

func CloneTickets(items []*Ticket) []*Ticket {
	if items == nil {
		return nil
	}
	ret := make([]*Ticket, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneTicket(item))
	}
	return ret
}
