package events

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// GovernanceEvidencePrincipal is the Orchestrator-owned wire value recorded
// with immutable approval evidence. Approval maps its owner-local principal to
// this value before submitting the command.
type GovernanceEvidencePrincipal struct {
	Kind      string
	SubjectID string
}

func (p GovernanceEvidencePrincipal) Normalized() GovernanceEvidencePrincipal {
	p.Kind = strings.TrimSpace(p.Kind)
	p.SubjectID = strings.TrimSpace(p.SubjectID)
	return p
}

func (p GovernanceEvidencePrincipal) Validate() error {
	p = p.Normalized()
	if p.SubjectID == "" {
		return fmt.Errorf("%w: governance evidence principal requires subject", common.ErrInvalidArgument)
	}
	switch p.Kind {
	case "human_operator", "auto_governance":
		return nil
	default:
		return fmt.Errorf("%w: invalid governance evidence principal kind %q", common.ErrInvalidArgument, p.Kind)
	}
}

type RecordDecisionEvidenceCommand struct {
	EvidenceRecordID   common.EvidenceRecordID
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	StepID             common.StepID
	TicketID           common.TicketID
	ResolutionID       common.ResolutionID
	ResolutionType     string
	DecisionKind       string
	Reason             string
	Question           string
	ResolutionPayload  []workorcheventpayload.Field
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
	Timestamp          string
}

type RecordDecisionEvidenceResult struct {
	EvidenceRecordID common.EvidenceRecordID
}

type RecordApprovalEvidenceCommand struct {
	EvidenceRecordID  common.EvidenceRecordID
	WorkspaceID       common.WorkspaceID
	RunID             common.RunID
	StepID            common.StepID
	ApprovalID        common.ApprovalID
	ApprovalState     common.ApprovalState
	ResolutionID      common.ResolutionID
	Principal         GovernanceEvidencePrincipal
	Reason            string
	Action            string
	Context           []workorcheventpayload.Field
	ResolutionContext []workorcheventpayload.Field
	Timestamp         string
}

type RecordApprovalEvidenceResult struct {
	EvidenceRecordID common.EvidenceRecordID
	WorkspaceID      common.WorkspaceID
	RunID            common.RunID
	ApprovalID       common.ApprovalID
	ResolutionID     common.ResolutionID
	ApprovalState    common.ApprovalState
}
