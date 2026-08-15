package recordpayload

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type ResultPayloadData struct {
	GoalID       common.GoalID
	DoDID        common.DoDID
	State        common.ResultState
	Title        string
	Summary      string
	SourceType   string
	SourceRef    string
	EvidenceRefs []string
	ArtifactRefs []string
}

type ArtifactPayloadData struct {
	ResultID           common.ResultID
	StepID             common.StepID
	ToolCallID         common.ToolCallID
	PatchID            common.PatchID
	ValidationResultID common.ValidationResultID
	ArtifactType       string
	Name               string
	Summary            string
	URI                string
	ContentType        string
	State              common.ArtifactState
	EvidenceRefs       []string
}

type PatchPayloadData struct {
	PatchID      common.PatchID
	StepID       common.StepID
	ToolCallID   common.ToolCallID
	PatchType    string
	TargetRef    string
	Summary      string
	State        common.PatchState
	AddedLines   int
	DeletedLines int
	ContentHash  string
	DiffHash     string
	ApprovalID   common.ApprovalID
	Risk         string
	Error        string
}

type ArtifactContinuationPayloadData struct {
	ContinuationID    common.ArtifactContinuationID
	RunID             common.RunID
	ArtifactID        string
	TargetRef         string
	State             common.ArtifactContinuationState
	ExpectedSequence  int
	FinalReceived     bool
	BytesWritten      int64
	RollingHash       string
	LastChunkHash     string
	ReceivedSequences []int
	CoverageTargets   []string
	EvidenceRefs      []string
	FailureReason     string
}

func (data ResultPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyState, string(data.State)),
		workorcheventpayload.StringArrayField(PayloadKeyEvidenceRefs, data.EvidenceRefs),
		workorcheventpayload.StringArrayField(PayloadKeyArtifactRefs, data.ArtifactRefs),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyGoalID, string(data.GoalID))
	fields = appendOptionalPayloadString(fields, PayloadKeyDoDID, string(data.DoDID))
	fields = appendOptionalPayloadString(fields, PayloadKeyTitle, data.Title)
	fields = appendOptionalPayloadString(fields, PayloadKeySummary, data.Summary)
	fields = appendOptionalPayloadString(fields, PayloadKeySourceType, data.SourceType)
	fields = appendOptionalPayloadString(fields, PayloadKeySourceRef, data.SourceRef)
	return workorcheventpayload.New(fields...)
}

func (data ArtifactPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyArtifactType, data.ArtifactType),
		workorcheventpayload.StringField(PayloadKeyState, string(data.State)),
		workorcheventpayload.StringArrayField(PayloadKeyEvidenceRefs, data.EvidenceRefs),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyResultID, string(data.ResultID))
	fields = appendOptionalPayloadString(fields, PayloadKeyStepID, string(data.StepID))
	fields = appendOptionalPayloadString(fields, PayloadKeyToolCallID, string(data.ToolCallID))
	fields = appendOptionalPayloadString(fields, PayloadKeyPatchID, string(data.PatchID))
	fields = appendOptionalPayloadString(fields, "validationResultId", string(data.ValidationResultID))
	fields = appendOptionalPayloadString(fields, PayloadKeyName, data.Name)
	fields = appendOptionalPayloadString(fields, PayloadKeySummary, data.Summary)
	fields = appendOptionalPayloadString(fields, PayloadKeyURI, data.URI)
	fields = appendOptionalPayloadString(fields, PayloadKeyContentType, data.ContentType)
	return workorcheventpayload.New(fields...)
}

func (data PatchPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyState, string(data.State)),
		workorcheventpayload.StringField(PayloadKeyPatchType, data.PatchType),
		workorcheventpayload.StringField(PayloadKeyTargetRef, data.TargetRef),
		workorcheventpayload.IntField(PayloadKeyAddedLines, int64(data.AddedLines)),
		workorcheventpayload.IntField(PayloadKeyDeletedLines, int64(data.DeletedLines)),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyPatchID, string(data.PatchID))
	fields = appendOptionalPayloadString(fields, PayloadKeyStepID, string(data.StepID))
	fields = appendOptionalPayloadString(fields, PayloadKeyToolCallID, string(data.ToolCallID))
	fields = appendOptionalPayloadString(fields, PayloadKeySummary, data.Summary)
	fields = appendOptionalPayloadString(fields, PayloadKeyContentHash, data.ContentHash)
	fields = appendOptionalPayloadString(fields, PayloadKeyDiffHash, data.DiffHash)
	fields = appendOptionalPayloadString(fields, PayloadKeyApprovalID, string(data.ApprovalID))
	fields = appendOptionalPayloadString(fields, PayloadKeyRisk, data.Risk)
	fields = appendOptionalPayloadString(fields, PayloadKeyError, data.Error)
	return workorcheventpayload.New(fields...)
}

func (data ArtifactContinuationPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField("id", string(data.ContinuationID)),
		workorcheventpayload.StringField(PayloadKeyRunID, string(data.RunID)),
		workorcheventpayload.StringField(PayloadKeyArtifactID, data.ArtifactID),
		workorcheventpayload.StringField(PayloadKeyTargetRef, data.TargetRef),
		workorcheventpayload.StringField(PayloadKeyState, string(data.State)),
		workorcheventpayload.IntField("expectedSequence", int64(data.ExpectedSequence)),
		workorcheventpayload.BoolField("finalReceived", data.FinalReceived),
		workorcheventpayload.IntField("bytesWritten", data.BytesWritten),
	}
	fields = appendOptionalPayloadString(fields, "rollingHash", data.RollingHash)
	fields = appendOptionalPayloadString(fields, "lastChunkHash", data.LastChunkHash)
	if len(data.ReceivedSequences) > 0 {
		values := make([]workorcheventpayload.Value, 0, len(data.ReceivedSequences))
		for _, value := range data.ReceivedSequences {
			values = append(values, workorcheventpayload.Value{Kind: workorcheventpayload.ValueInt, Int: int64(value)})
		}
		fields = append(fields, workorcheventpayload.Field{Name: "receivedSequences", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: values}})
	}
	if len(data.CoverageTargets) > 0 {
		fields = append(fields, workorcheventpayload.StringArrayField("coverageTargets", data.CoverageTargets))
	}
	if len(data.EvidenceRefs) > 0 {
		fields = append(fields, workorcheventpayload.StringArrayField(PayloadKeyEvidenceRefs, data.EvidenceRefs))
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyFailureReason, data.FailureReason)
	return workorcheventpayload.New(fields...)
}
