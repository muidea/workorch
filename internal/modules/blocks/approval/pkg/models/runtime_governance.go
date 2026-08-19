package approval

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type ApprovalResolutionPrincipalKind string

const (
	ApprovalResolutionPrincipalHumanOperator  ApprovalResolutionPrincipalKind = "human_operator"
	ApprovalResolutionPrincipalAutoGovernance ApprovalResolutionPrincipalKind = "auto_governance"
)

type ApprovalResolutionPrincipal struct {
	Kind      ApprovalResolutionPrincipalKind
	SubjectID string
}

func (p ApprovalResolutionPrincipal) Normalized() ApprovalResolutionPrincipal {
	p.Kind = ApprovalResolutionPrincipalKind(strings.TrimSpace(string(p.Kind)))
	p.SubjectID = strings.TrimSpace(p.SubjectID)
	return p
}

func (p ApprovalResolutionPrincipal) Validate() error {
	p = p.Normalized()
	if p.SubjectID == "" {
		return fmt.Errorf("approval resolution principal requires subject")
	}
	switch p.Kind {
	case ApprovalResolutionPrincipalHumanOperator, ApprovalResolutionPrincipalAutoGovernance:
		return nil
	default:
		return fmt.Errorf("approval resolution principal kind %q is invalid", p.Kind)
	}
}

type RuntimeGovernanceBinding struct {
	AuthoritySnapshotID string
	DispatchID          string
	DependencyKind      string
	SubjectRef          string
	RequestedAction     string
	RiskLevel           string
	AuthorizationHash   string
}

func (b RuntimeGovernanceBinding) Clone() RuntimeGovernanceBinding {
	b.AuthoritySnapshotID = strings.TrimSpace(b.AuthoritySnapshotID)
	b.DispatchID = strings.TrimSpace(b.DispatchID)
	b.DependencyKind = strings.TrimSpace(b.DependencyKind)
	b.SubjectRef = strings.TrimSpace(b.SubjectRef)
	b.RequestedAction = strings.TrimSpace(b.RequestedAction)
	b.RiskLevel = strings.TrimSpace(b.RiskLevel)
	b.AuthorizationHash = strings.TrimSpace(b.AuthorizationHash)
	return b
}

type ResolutionProposalState string

const (
	ResolutionProposalPending   ResolutionProposalState = "pending"
	ResolutionProposalCommitted ResolutionProposalState = "committed"
	ResolutionProposalRejected  ResolutionProposalState = "rejected"
)

type ResolutionProposal struct {
	ResolutionID                      common.ResolutionID
	TargetState                       common.ApprovalState
	Principal                         ApprovalResolutionPrincipal
	Context                           workorcheventpayload.Payload
	State                             ResolutionProposalState
	FailureClass                      string
	FailureReason                     string
	ProposedAt                        string
	CommittedAt                       string
	EvidenceRecordedAt                string
	ResolutionNotificationPublishedAt string
}

func (p ResolutionProposal) Clone() ResolutionProposal {
	p.Principal = p.Principal.Normalized()
	p.Context = p.Context.Clone()
	p.FailureClass = strings.TrimSpace(p.FailureClass)
	p.FailureReason = strings.TrimSpace(p.FailureReason)
	return p
}
