package events

import "github.com/muidea/workorch/internal/pkg/common"

const EvidenceCommandDestination = "kernel/orchestrator"

type DecisionEvidenceValueKind string

const (
	DecisionEvidenceValueNull   DecisionEvidenceValueKind = "null"
	DecisionEvidenceValueString DecisionEvidenceValueKind = "string"
	DecisionEvidenceValueBool   DecisionEvidenceValueKind = "bool"
	DecisionEvidenceValueInt    DecisionEvidenceValueKind = "int"
	DecisionEvidenceValueUint   DecisionEvidenceValueKind = "uint"
	DecisionEvidenceValueFloat  DecisionEvidenceValueKind = "float"
	DecisionEvidenceValueObject DecisionEvidenceValueKind = "object"
	DecisionEvidenceValueArray  DecisionEvidenceValueKind = "array"
)

type DecisionEvidenceField struct {
	Name  string
	Value DecisionEvidenceValue
}

type DecisionEvidenceValue struct {
	Kind   DecisionEvidenceValueKind
	String string
	Bool   bool
	Int    int64
	Uint   uint64
	Float  float64
	Object []DecisionEvidenceField
	Array  []DecisionEvidenceValue
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
	ResolutionPayload  []DecisionEvidenceField
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
	Timestamp          string
}

type RecordDecisionEvidenceResult struct {
	EvidenceRecordID common.EvidenceRecordID
}
