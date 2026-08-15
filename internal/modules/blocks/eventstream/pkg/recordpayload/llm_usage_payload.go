package recordpayload

import (
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// LLMUsagePayloadData is the durable event payload for /run/llm_usage/recorded.
// It carries sealed provider/model target, revisions, budget and usage for one request.
type LLMUsagePayloadData struct {
	RequestID             string
	WorkspaceID           string
	RunID                 string
	StepID                string
	ProviderID            string
	Provider              string
	APISurface            string
	Model                 string
	Operation             string
	CapabilityRevision    string
	ConfigurationRevision string
	StablePrefixHash      string
	Kind                  string
	InputTokens           int
	OutputTokens          int
	TotalTokens           int
	Estimated             bool
	UsageSource           string
	RequestBytes          int
	ResponseBytes         int
	Streaming             bool
	ContextBudget         llmaudit.ContextBudget
	CacheUsage            llmaudit.CacheUsage
	StartedAt             string
	CompletedAt           string
	DurationMs            int
	Status                string
	FailureClass          string
	ProviderSubmitted     bool
}

func (data LLMUsagePayloadData) EventPayload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
		workorcheventpayload.StringField(PayloadKeyWorkspaceID, data.WorkspaceID),
		workorcheventpayload.StringField(PayloadKeyRunID, data.RunID),
		workorcheventpayload.StringField(PayloadKeyStatus, data.Status),
		workorcheventpayload.BoolField(PayloadKeyEstimated, data.Estimated),
		workorcheventpayload.BoolField(PayloadKeyStreaming, data.Streaming),
		workorcheventpayload.BoolField("providerSubmitted", data.ProviderSubmitted),
		workorcheventpayload.IntField(PayloadKeyInputTokens, int64(data.InputTokens)),
		workorcheventpayload.IntField(PayloadKeyOutputTokens, int64(data.OutputTokens)),
		workorcheventpayload.IntField(PayloadKeyTotalTokens, int64(data.TotalTokens)),
		workorcheventpayload.IntField(PayloadKeyRequestBytes, int64(data.RequestBytes)),
		workorcheventpayload.IntField(PayloadKeyResponseBytes, int64(data.ResponseBytes)),
		workorcheventpayload.IntField(PayloadKeyDurationMs, int64(data.DurationMs)),
	}
	fields = appendOptionalPayloadString(fields, PayloadKeyRequestID, data.RequestID)
	fields = appendOptionalPayloadString(fields, PayloadKeyStepID, data.StepID)
	fields = appendOptionalPayloadString(fields, PayloadKeyProviderID, data.ProviderID)
	fields = appendOptionalPayloadString(fields, PayloadKeyProvider, data.Provider)
	fields = appendOptionalPayloadString(fields, PayloadKeyAPISurface, data.APISurface)
	fields = appendOptionalPayloadString(fields, PayloadKeyModel, data.Model)
	fields = appendOptionalPayloadString(fields, PayloadKeyOperation, data.Operation)
	fields = appendOptionalPayloadString(fields, "capabilityRevision", data.CapabilityRevision)
	fields = appendOptionalPayloadString(fields, "configurationRevision", data.ConfigurationRevision)
	fields = appendOptionalPayloadString(fields, "stablePrefixHash", data.StablePrefixHash)
	fields = appendOptionalPayloadString(fields, PayloadKeyKind, data.Kind)
	fields = appendOptionalPayloadString(fields, PayloadKeyUsageSource, data.UsageSource)
	fields = appendOptionalPayloadString(fields, PayloadKeyStartedAt, data.StartedAt)
	fields = appendOptionalPayloadString(fields, PayloadKeyCompletedAt, data.CompletedAt)
	fields = appendOptionalPayloadString(fields, PayloadKeyFailureClass, data.FailureClass)
	if !data.ContextBudget.Empty() {
		fields = append(fields, workorcheventpayload.ObjectField(PayloadKeyContextBudget, data.ContextBudget.Payload()))
	}
	if !data.CacheUsage.Empty() {
		fields = append(fields, workorcheventpayload.ObjectField(PayloadKeyCacheUsage, data.CacheUsage.Payload()))
	}
	return workorcheventpayload.New(fields...)
}
