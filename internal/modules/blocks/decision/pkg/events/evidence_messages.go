package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const EvidenceCommandDestination = "kernel/orchestrator"

type DecisionEvidenceValueKind = workorcheventpayload.ValueKind

const (
	DecisionEvidenceValueNull   = workorcheventpayload.ValueNull
	DecisionEvidenceValueString = workorcheventpayload.ValueString
	DecisionEvidenceValueBool   = workorcheventpayload.ValueBool
	DecisionEvidenceValueInt    = workorcheventpayload.ValueInt
	DecisionEvidenceValueUint   = workorcheventpayload.ValueUint
	DecisionEvidenceValueFloat  = workorcheventpayload.ValueFloat
	DecisionEvidenceValueObject = workorcheventpayload.ValueObject
	DecisionEvidenceValueArray  = workorcheventpayload.ValueArray
)

type DecisionEvidenceField = workorcheventpayload.Field
type DecisionEvidenceValue = workorcheventpayload.Value

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
	ResolutionPayload  []DecisionEvidenceField
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
	Timestamp          string
}

type RecordDecisionEvidenceResult struct {
	EvidenceRecordID common.EvidenceRecordID
}
