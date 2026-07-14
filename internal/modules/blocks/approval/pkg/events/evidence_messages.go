package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const EvidenceCommandDestination = "kernel/orchestrator"

type ApprovalEvidenceValueKind = workorcheventpayload.ValueKind

const (
	ApprovalEvidenceValueNull   = workorcheventpayload.ValueNull
	ApprovalEvidenceValueString = workorcheventpayload.ValueString
	ApprovalEvidenceValueBool   = workorcheventpayload.ValueBool
	ApprovalEvidenceValueInt    = workorcheventpayload.ValueInt
	ApprovalEvidenceValueUint   = workorcheventpayload.ValueUint
	ApprovalEvidenceValueFloat  = workorcheventpayload.ValueFloat
	ApprovalEvidenceValueObject = workorcheventpayload.ValueObject
	ApprovalEvidenceValueArray  = workorcheventpayload.ValueArray
)

type ApprovalEvidenceField = workorcheventpayload.Field
type ApprovalEvidenceValue = workorcheventpayload.Value

type RecordApprovalEvidenceCommand struct {
	EvidenceRecordID common.EvidenceRecordID
	WorkspaceID      common.WorkspaceID
	RunID            common.RunID
	StepID           common.StepID
	ApprovalID       common.ApprovalID
	ApprovalState    common.ApprovalState
	Reason           string
	Action           string
	Context          []ApprovalEvidenceField
	Timestamp        string
}

type RecordApprovalEvidenceResult struct {
	EvidenceRecordID common.EvidenceRecordID
}
