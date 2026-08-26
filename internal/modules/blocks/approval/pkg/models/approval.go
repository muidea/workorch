package approval

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// Approval is the Approval owner DTO. It remains detached from repository
// implementations while preserving the complete owner result.
type Approval struct {
	ID          common.ApprovalID
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	Reason      string
	Action      string
	Context     workorcheventpayload.Payload
	// ResolutionContext is audit metadata supplied by a trusted resolution
	// ingress. Context above is the immutable authorization target and is never
	// overlaid during approve/reject.
	ResolutionContext                 workorcheventpayload.Payload
	State                             common.ApprovalState
	AutoGovernance                    workorcheventpayload.Payload
	ResolutionID                      common.ResolutionID
	ResolutionPrincipal               ApprovalResolutionPrincipal
	RuntimeGovernance                 *RuntimeGovernanceBinding
	ResolutionProposal                *ResolutionProposal
	EvidenceRecordedAt                string
	ResolutionNotificationPublishedAt string
	ResolvedAt                        string
	ResolvedBy                        string
	CreatedAt                         string
	UpdatedAt                         string
}

func CloneApproval(item *Approval) *Approval {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.Context = item.Context.Clone()
	cloned.ResolutionContext = item.ResolutionContext.Clone()
	cloned.AutoGovernance = item.AutoGovernance.Clone()
	if item.RuntimeGovernance != nil {
		binding := item.RuntimeGovernance.Clone()
		cloned.RuntimeGovernance = &binding
	}
	if item.ResolutionProposal != nil {
		proposal := item.ResolutionProposal.Clone()
		cloned.ResolutionProposal = &proposal
	}
	return &cloned
}

func CloneApprovals(items []*Approval) []*Approval {
	if items == nil {
		return nil
	}
	ret := make([]*Approval, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneApproval(item))
	}
	return ret
}
