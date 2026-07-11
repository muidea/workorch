package events

import "github.com/muidea/workorch/internal/pkg/common"

const EvidenceCommandDestination = "kernel/orchestrator"

type ApprovalEvidenceValueKind string

const (
	ApprovalEvidenceValueNull   ApprovalEvidenceValueKind = "null"
	ApprovalEvidenceValueString ApprovalEvidenceValueKind = "string"
	ApprovalEvidenceValueBool   ApprovalEvidenceValueKind = "bool"
	ApprovalEvidenceValueInt    ApprovalEvidenceValueKind = "int"
	ApprovalEvidenceValueUint   ApprovalEvidenceValueKind = "uint"
	ApprovalEvidenceValueFloat  ApprovalEvidenceValueKind = "float"
	ApprovalEvidenceValueObject ApprovalEvidenceValueKind = "object"
	ApprovalEvidenceValueArray  ApprovalEvidenceValueKind = "array"
)

type ApprovalEvidenceField struct {
	Name  string
	Value ApprovalEvidenceValue
}

type ApprovalEvidenceValue struct {
	Kind   ApprovalEvidenceValueKind
	String string
	Bool   bool
	Int    int64
	Uint   uint64
	Float  float64
	Object []ApprovalEvidenceField
	Array  []ApprovalEvidenceValue
}

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
