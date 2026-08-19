package recordpayload

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type DecisionTicketPayloadData struct {
	Reason string
	State  common.DecisionTicketState
}

type DecisionResolutionPayloadData struct {
	DecisionTicketPayloadData
	ResolutionID       common.ResolutionID
	Type               string
	Payload            workorcheventpayload.Payload
	ContinuationAction string
	ResumeRunID        common.RunID
	SuccessorRunID     common.RunID
}

type ApprovalPayloadData struct {
	Reason            string
	Action            string
	Context           workorcheventpayload.Payload
	State             common.ApprovalState
	ResolutionID      common.ResolutionID
	ResolutionContext workorcheventpayload.Payload
	ResolvedByKind    string
	ResolvedBy        string
}

type AutoGovernancePayloadData struct {
	TraceID            common.AutoGovernanceTraceID
	ItemType           string
	ItemID             string
	Action             string
	RiskLevel          string
	Confidence         float64
	PolicyMatched      string
	PolicyBlocked      string
	BudgetBefore       int
	BudgetAfter        int
	PromptFingerprint  string
	LLMProviderID      string
	LLMModel           string
	LLMDecisionPreview string
	Status             string
	Reason             string
	EscalationReason   string
}

func (data DecisionTicketPayloadData) EventPayload() workorcheventpayload.Payload {
	return workorcheventpayload.New(
		workorcheventpayload.StringField(PayloadKeyReason, data.Reason),
		workorcheventpayload.StringField(PayloadKeyState, string(data.State)),
	)
}

func (data DecisionResolutionPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := data.DecisionTicketPayloadData.EventPayload().Fields
	fields = append(fields,
		workorcheventpayload.StringField(PayloadKeyResolutionID, string(data.ResolutionID)),
		workorcheventpayload.StringField(PayloadKeyType, data.Type),
		workorcheventpayload.ObjectField(PayloadKeyPayload, data.Payload),
	)
	fields = appendOptionalPayloadString(fields, PayloadKeyContinuationAction, data.ContinuationAction)
	fields = appendOptionalPayloadString(fields, PayloadKeyResumeRunID, string(data.ResumeRunID))
	fields = appendOptionalPayloadString(fields, PayloadKeySuccessorRunID, string(data.SuccessorRunID))
	return workorcheventpayload.New(fields...)
}

func (data ApprovalPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyReason, data.Reason),
		workorcheventpayload.StringField(PayloadKeyState, string(data.State)),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyAction, data.Action)
	if len(data.Context.Fields) > 0 {
		fields = append(fields, workorcheventpayload.ObjectField(PayloadKeyContext, data.Context))
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyResolutionID, string(data.ResolutionID))
	if len(data.ResolutionContext.Fields) > 0 {
		fields = append(fields, workorcheventpayload.ObjectField(PayloadKeyResolutionContext, data.ResolutionContext))
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyResolvedByKind, data.ResolvedByKind)
	fields = appendOptionalPayloadString(fields, PayloadKeyResolvedBy, data.ResolvedBy)
	return workorcheventpayload.New(fields...)
}

func DecodeDecisionTicketEventPayload(payload workorcheventpayload.Payload) (DecisionTicketPayloadData, bool) {
	reason, ok := payload.String(PayloadKeyReason)
	if !ok {
		return DecisionTicketPayloadData{}, false
	}
	state, ok := payload.String(PayloadKeyState)
	if !ok {
		return DecisionTicketPayloadData{}, false
	}
	return DecisionTicketPayloadData{Reason: reason, State: common.DecisionTicketState(state)}, true
}

func DecodeDecisionResolutionEventPayload(payload workorcheventpayload.Payload) (DecisionResolutionPayloadData, bool) {
	base, ok := DecodeDecisionTicketEventPayload(payload)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	resolutionID, ok := payload.String(PayloadKeyResolutionID)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	resolutionType, ok := payload.String(PayloadKeyType)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	resolutionPayload, ok := optionalEventPayloadObject(payload, PayloadKeyPayload)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	continuationAction, ok := optionalEventPayloadString(payload, PayloadKeyContinuationAction)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	resumeRunID, ok := optionalEventPayloadString(payload, PayloadKeyResumeRunID)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	successorRunID, ok := optionalEventPayloadString(payload, PayloadKeySuccessorRunID)
	if !ok {
		return DecisionResolutionPayloadData{}, false
	}
	return DecisionResolutionPayloadData{
		DecisionTicketPayloadData: base,
		ResolutionID:              common.ResolutionID(resolutionID),
		Type:                      resolutionType,
		Payload:                   resolutionPayload,
		ContinuationAction:        continuationAction,
		ResumeRunID:               common.RunID(resumeRunID),
		SuccessorRunID:            common.RunID(successorRunID),
	}, true
}

func DecodeApprovalEventPayload(payload workorcheventpayload.Payload) (ApprovalPayloadData, bool) {
	reason, ok := payload.String(PayloadKeyReason)
	if !ok {
		return ApprovalPayloadData{}, false
	}
	state, ok := payload.String(PayloadKeyState)
	if !ok {
		return ApprovalPayloadData{}, false
	}
	action, ok := optionalEventPayloadString(payload, PayloadKeyAction)
	if !ok {
		return ApprovalPayloadData{}, false
	}
	context, ok := optionalEventPayloadObject(payload, PayloadKeyContext)
	if !ok {
		return ApprovalPayloadData{}, false
	}
	resolutionID, ok := optionalEventPayloadString(payload, PayloadKeyResolutionID)
	if !ok {
		return ApprovalPayloadData{}, false
	}
	resolutionContext, ok := optionalEventPayloadObject(payload, PayloadKeyResolutionContext)
	if !ok {
		return ApprovalPayloadData{}, false
	}
	resolvedByKind, ok := optionalEventPayloadString(payload, PayloadKeyResolvedByKind)
	if !ok {
		return ApprovalPayloadData{}, false
	}
	resolvedBy, ok := optionalEventPayloadString(payload, PayloadKeyResolvedBy)
	if !ok {
		return ApprovalPayloadData{}, false
	}
	return ApprovalPayloadData{
		Reason: reason, Action: action, Context: context, State: common.ApprovalState(state),
		ResolutionID: common.ResolutionID(resolutionID), ResolutionContext: resolutionContext,
		ResolvedByKind: resolvedByKind, ResolvedBy: resolvedBy,
	}, true
}

func optionalEventPayloadString(payload workorcheventpayload.Payload, key string) (string, bool) {
	value, exists := payload.Field(key)
	if !exists || value.Kind == workorcheventpayload.ValueNull {
		return "", true
	}
	if value.Kind != workorcheventpayload.ValueString {
		return "", false
	}
	return value.String, true
}

func optionalEventPayloadInt(payload workorcheventpayload.Payload, key string) (int, bool) {
	value, exists := payload.Field(key)
	if !exists || value.Kind == workorcheventpayload.ValueNull {
		return 0, true
	}
	parsed, ok := payload.Int64(key)
	if !ok {
		return 0, false
	}
	return int(parsed), true
}

func optionalEventPayloadObject(payload workorcheventpayload.Payload, key string) (workorcheventpayload.Payload, bool) {
	value, exists := payload.Field(key)
	if !exists || value.Kind == workorcheventpayload.ValueNull {
		return workorcheventpayload.Payload{}, true
	}
	return payload.Object(key)
}

func (data AutoGovernancePayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyTraceID, string(data.TraceID)),
		workorcheventpayload.StringField(PayloadKeyItemType, data.ItemType),
		workorcheventpayload.StringField(PayloadKeyItemID, data.ItemID),
		workorcheventpayload.StringField(PayloadKeyAction, data.Action),
		workorcheventpayload.StringField(PayloadKeyRisk, data.RiskLevel),
		workorcheventpayload.FloatField(PayloadKeyConfidenceScore, data.Confidence),
		workorcheventpayload.StringField(PayloadKeyPolicyMatched, data.PolicyMatched),
		workorcheventpayload.StringField(PayloadKeyPolicyBlocked, data.PolicyBlocked),
		workorcheventpayload.IntField(PayloadKeyBudgetBefore, int64(data.BudgetBefore)),
		workorcheventpayload.IntField(PayloadKeyBudgetAfter, int64(data.BudgetAfter)),
		workorcheventpayload.StringField(PayloadKeyLLMProviderID, data.LLMProviderID),
		workorcheventpayload.StringField(PayloadKeyLLMModel, data.LLMModel),
		workorcheventpayload.StringField(PayloadKeyLLMDecisionPreview, data.LLMDecisionPreview),
		workorcheventpayload.StringField(PayloadKeyPromptFingerprint, data.PromptFingerprint),
		workorcheventpayload.StringField(PayloadKeyStatus, data.Status),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyReason, data.Reason)
	fields = appendOptionalPayloadString(fields, PayloadKeyEscalationReason, data.EscalationReason)
	return workorcheventpayload.New(fields...)
}
