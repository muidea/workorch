package recordpayload

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
)

type MemoryPayloadData struct {
	MemoryType string
	Summary    string
	Content    string
	SourceType string
	SourceRef  string
	Tags       []string
	Importance int
	Confidence int
	Visibility string
}

type MemoryRecallPayloadData struct {
	TraceID            common.MemoryRecallTraceID
	Stage              string
	Source             string
	Policy             string
	Status             string
	ProviderID         string
	Operation          string
	RetrievedMemoryIDs []common.MemoryID
	SelectionEvidence  *workorchmemorycontract.RecallSelectionEvidence
	Reason             string
}

func (data MemoryPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyMemoryType, data.MemoryType),
		workorcheventpayload.StringField(PayloadKeySummary, data.Summary),
		workorcheventpayload.StringField(PayloadKeySourceType, data.SourceType),
		workorcheventpayload.StringArrayField(PayloadKeyTags, data.Tags),
		workorcheventpayload.IntField(PayloadKeyImportance, int64(data.Importance)),
		workorcheventpayload.IntField(PayloadKeyConfidence, int64(data.Confidence)),
		workorcheventpayload.StringField(PayloadKeyVisibility, data.Visibility),
	}
	if data.Content != "" {
		fields = append(fields,
			workorcheventpayload.StringField(PayloadKeyContentSHA256, SHA256String(data.Content)),
			workorcheventpayload.IntField(PayloadKeyContentBytes, int64(len(data.Content))),
		)
	}
	fields = appendOptionalPayloadString(fields, PayloadKeySourceRef, data.SourceRef)
	return workorcheventpayload.New(fields...)
}

func (data MemoryRecallPayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyTraceID, string(data.TraceID)),
		workorcheventpayload.StringField(PayloadKeyStage, data.Stage),
		workorcheventpayload.StringField(PayloadKeySourceType, data.Source),
		workorcheventpayload.StringField(PayloadKeyPolicy, data.Policy),
		workorcheventpayload.StringField(PayloadKeyStatus, data.Status),
		workorcheventpayload.StringArrayField(PayloadKeyRetrievedMemoryIDs, memoryIDsToStrings(data.RetrievedMemoryIDs)),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyProviderID, data.ProviderID)
	fields = appendOptionalPayloadString(fields, PayloadKeyOperation, data.Operation)
	fields = appendOptionalPayloadString(fields, PayloadKeyReason, data.Reason)
	if data.SelectionEvidence != nil {
		fields = append(fields, workorcheventpayload.ObjectField(PayloadKeySelectionEvidence, recallSelectionEvidencePayload(*data.SelectionEvidence)))
	}
	return workorcheventpayload.New(fields...)
}

func recallSelectionEvidencePayload(input workorchmemorycontract.RecallSelectionEvidence) workorcheventpayload.Payload {
	value := input.Normalized()
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyPolicy, value.Policy),
		workorcheventpayload.StringField(PayloadKeyStatus, value.Status),
		workorcheventpayload.FloatField(PayloadKeyMinScore, value.MinScore),
		workorcheventpayload.IntField(PayloadKeyLimit, int64(value.Limit)),
		workorcheventpayload.IntField(PayloadKeyVectorDimension, int64(value.VectorDimension)),
		workorcheventpayload.StringArrayField(PayloadKeyCandidateMemoryIDs, memoryIDsToStrings(value.CandidateMemoryIDs)),
		workorcheventpayload.StringArrayField(PayloadKeySelectedMemoryIDs, memoryIDsToStrings(value.SelectedMemoryIDs)),
		workorcheventpayload.StringArrayField(PayloadKeyExcludedMemoryIDs, memoryIDsToStrings(value.ExcludedMemoryIDs)),
		workorcheventpayload.StringArrayField(PayloadKeyRetrievedMemoryIDs, memoryIDsToStrings(value.RetrievedMemoryIDs)),
		workorcheventpayload.ObjectField(PayloadKeyContextBudget, workorcheventpayload.New(
			workorcheventpayload.IntField("maxItems", int64(value.ContextBudget.MaxItems)),
			workorcheventpayload.IntField("maxBytes", int64(value.ContextBudget.MaxBytes)),
			workorcheventpayload.IntField("maxEstimatedTokens", int64(value.ContextBudget.MaxEstimatedTokens)),
			workorcheventpayload.IntField("usedItems", int64(value.ContextBudget.UsedItems)),
			workorcheventpayload.IntField("usedBytes", int64(value.ContextBudget.UsedBytes)),
			workorcheventpayload.IntField("estimatedTokens", int64(value.ContextBudget.EstimatedTokens)),
		)),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyReason, value.Reason)
	fields = appendOptionalPayloadString(fields, PayloadKeyProviderID, value.ProviderID)
	fields = appendOptionalPayloadString(fields, PayloadKeyModelID, value.ModelID)
	fields = appendOptionalPayloadString(fields, PayloadKeyOperation, value.Operation)
	fields = appendOptionalPayloadString(fields, PayloadKeyQueryTextSHA256, value.QueryTextSHA256)
	fields = appendOptionalPayloadString(fields, PayloadKeyEmbeddingModel, value.EmbeddingModel)
	fields = appendOptionalPayloadString(fields, PayloadKeyRefreshSource, value.RefreshSource)
	fields = appendOptionalPayloadString(fields, PayloadKeySelectionPolicyVersion, value.SelectionPolicyVersion)
	fields = appendOptionalPayloadString(fields, PayloadKeyFallbackFrom, value.FallbackFrom)
	fields = append(fields,
		objectArrayField(PayloadKeyTypeQuota, typeQuotaPayloads(value.TypeQuota)),
		objectArrayField(PayloadKeySelectionScores, selectionScorePayloads(value.SelectionScores)),
		objectArrayField(PayloadKeyRetrievedMemoryHits, recallHitPayloads(value.RetrievedMemoryHits)),
	)
	if value.SemanticRecall != nil {
		fields = append(fields, workorcheventpayload.ObjectField(PayloadKeySemanticRecall, recallSelectionEvidencePayload(*value.SemanticRecall)))
	}
	return workorcheventpayload.New(fields...)
}

func objectArrayField(name string, payloads []workorcheventpayload.Payload) workorcheventpayload.Field {
	values := make([]workorcheventpayload.Value, 0, len(payloads))
	for _, payload := range payloads {
		values = append(values, workorcheventpayload.Value{Kind: workorcheventpayload.ValueObject, Object: payload.Fields})
	}
	return workorcheventpayload.Field{Name: name, Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: values}}
}

func typeQuotaPayloads(values []workorchmemorycontract.TypeQuotaEntry) []workorcheventpayload.Payload {
	ret := make([]workorcheventpayload.Payload, 0, len(values))
	for _, value := range values {
		ret = append(ret, workorcheventpayload.New(workorcheventpayload.StringField("type", value.Type), workorcheventpayload.IntField("quota", int64(value.Quota))))
	}
	return ret
}

func selectionScorePayloads(values []workorchmemorycontract.RecallSelectionScore) []workorcheventpayload.Payload {
	ret := make([]workorcheventpayload.Payload, 0, len(values))
	for _, value := range values {
		ret = append(ret, workorcheventpayload.New(workorcheventpayload.StringField("id", string(value.ID)), workorcheventpayload.IntField("score", int64(value.Score)), workorcheventpayload.StringField(PayloadKeyReason, value.Reason)))
	}
	return ret
}

func recallHitPayloads(values []workorchmemorycontract.RecallHit) []workorcheventpayload.Payload {
	ret := make([]workorcheventpayload.Payload, 0, len(values))
	for _, value := range values {
		ret = append(ret, workorcheventpayload.New(
			workorcheventpayload.StringField("id", string(value.ID)),
			workorcheventpayload.FloatField("vectorScore", value.VectorScore), workorcheventpayload.IntField("vectorDimension", int64(value.VectorDimension)),
		))
	}
	return ret
}
