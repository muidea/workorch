package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const EvidenceCommandDestination = "kernel/orchestrator"

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
