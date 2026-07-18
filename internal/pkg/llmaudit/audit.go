package llmaudit

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

// ContextBudget is the concrete cross-owner audit DTO for a sealed provider
// request budget. It carries audit-safe scalar values only.
type ContextBudget struct {
	ProviderID               string
	ModelID                  string
	CapabilityRevision       string
	ConfigurationRevision    string
	ContextWindowTokens      int
	ProviderMaxOutputTokens  int
	EffectiveMaxOutputTokens int
	MaxBytes                 int
	ActualBytes              int
	MaxTokens                int
	EstimatedInputTokens     int
	EstimatedOutputTokens    int
	ActualTokens             int
	MaxRequestBytes          int
	ProfileMode              string
	CapabilitySource         string
	ProviderSubmitted        bool
	OverBudget               bool
	Truncated                bool
	StablePrefixHash         string
	RenderedStablePrefixHash string
	CacheStrategy            string
	ContextGateAction        string
	ContextGateReason        string
	BudgetFailureClass       string
}

func (v ContextBudget) Empty() bool { return v == ContextBudget{} }

func (v ContextBudget) Payload() workorcheventpayload.Payload {
	return workorcheventpayload.New(
		workorcheventpayload.StringField("providerId", v.ProviderID),
		workorcheventpayload.StringField("modelId", v.ModelID),
		workorcheventpayload.StringField("capabilityRevision", v.CapabilityRevision),
		workorcheventpayload.StringField("configurationRevision", v.ConfigurationRevision),
		workorcheventpayload.IntField("contextWindowTokens", int64(v.ContextWindowTokens)),
		workorcheventpayload.IntField("providerMaxOutputTokens", int64(v.ProviderMaxOutputTokens)),
		workorcheventpayload.IntField("effectiveMaxOutputTokens", int64(v.EffectiveMaxOutputTokens)),
		workorcheventpayload.IntField("maxBytes", int64(v.MaxBytes)),
		workorcheventpayload.IntField("actualBytes", int64(v.ActualBytes)),
		workorcheventpayload.IntField("maxTokens", int64(v.MaxTokens)),
		workorcheventpayload.IntField("estimatedInputTokens", int64(v.EstimatedInputTokens)),
		workorcheventpayload.IntField("estimatedOutputTokens", int64(v.EstimatedOutputTokens)),
		workorcheventpayload.IntField("actualTokens", int64(v.ActualTokens)),
		workorcheventpayload.IntField("maxRequestBytes", int64(v.MaxRequestBytes)),
		workorcheventpayload.StringField("profileMode", v.ProfileMode),
		workorcheventpayload.StringField("capabilitySource", v.CapabilitySource),
		workorcheventpayload.BoolField("providerSubmitted", v.ProviderSubmitted),
		workorcheventpayload.BoolField("overBudget", v.OverBudget),
		workorcheventpayload.BoolField("truncated", v.Truncated),
		workorcheventpayload.StringField("stablePrefixHash", v.StablePrefixHash),
		workorcheventpayload.StringField("renderedStablePrefixHash", v.RenderedStablePrefixHash),
		workorcheventpayload.StringField("cacheStrategy", v.CacheStrategy),
		workorcheventpayload.StringField("contextGateAction", v.ContextGateAction),
		workorcheventpayload.StringField("contextGateReason", v.ContextGateReason),
		workorcheventpayload.StringField("budgetFailureClass", v.BudgetFailureClass),
	)
}

// CacheUsage is the concrete cross-owner representation of provider cache
// usage. Native and estimated values remain explicit in the contract.
type CacheUsage struct {
	CacheReadTokens         int
	CacheWriteTokens        int
	CacheCreationTokens     int
	CacheableInputTokens    int
	NonCacheableInputTokens int
	Estimated               bool
	Source                  string
}

func (v CacheUsage) Empty() bool { return v == CacheUsage{} }

func (v CacheUsage) Payload() workorcheventpayload.Payload {
	return workorcheventpayload.New(
		workorcheventpayload.IntField("cacheReadTokens", int64(v.CacheReadTokens)),
		workorcheventpayload.IntField("cacheWriteTokens", int64(v.CacheWriteTokens)),
		workorcheventpayload.IntField("cacheCreationTokens", int64(v.CacheCreationTokens)),
		workorcheventpayload.IntField("cacheableInputTokens", int64(v.CacheableInputTokens)),
		workorcheventpayload.IntField("nonCacheableInputTokens", int64(v.NonCacheableInputTokens)),
		workorcheventpayload.BoolField("estimated", v.Estimated),
		workorcheventpayload.StringField("source", v.Source),
	)
}
