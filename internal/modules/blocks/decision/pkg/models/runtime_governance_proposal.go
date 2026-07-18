package decision

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// RuntimeGovernanceResolutionProposal is Decision-owner durable state for a
// Runtime V2 resolution which has not yet become a formal Decision resolution.
// It is persisted with its ticket so a process failure cannot make a resolved
// ticket precede the authoritative Orchestrator aggregate commit.
type RuntimeGovernanceResolutionProposal struct {
	ResolutionID       common.ResolutionID
	ResolutionType     string
	Payload            workorcheventpayload.Payload
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
	PayloadHash        string
	State              RuntimeGovernanceResolutionProposalState
	FailureClass       string
	FailureReason      string
	CreatedAt          string
	UpdatedAt          string
}

type RuntimeGovernanceResolutionProposalState string

const (
	RuntimeGovernanceResolutionProposalPending   RuntimeGovernanceResolutionProposalState = "pending"
	RuntimeGovernanceResolutionProposalCommitted RuntimeGovernanceResolutionProposalState = "committed"
	RuntimeGovernanceResolutionProposalRejected  RuntimeGovernanceResolutionProposalState = "rejected"
)

func (p RuntimeGovernanceResolutionProposal) Clone() RuntimeGovernanceResolutionProposal {
	p.Payload = p.Payload.Clone()
	return p
}
