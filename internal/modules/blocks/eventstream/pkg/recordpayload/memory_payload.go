package recordpayload

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
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
	fields = appendOptionalPayloadString(fields, PayloadKeyContent, data.Content)
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
	return workorcheventpayload.New(fields...)
}
