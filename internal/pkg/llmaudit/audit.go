package llmaudit

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

// ProviderUsage is the canonical, secret-free token usage value shared by LLM
// provider adapters, typed EventHub results and Runtime request audit. It is an
// observation only and carries no provider or Runtime state transition.
type ProviderUsage struct {
	InputTokens             int  `json:"inputTokens,omitempty"`
	OutputTokens            int  `json:"outputTokens,omitempty"`
	TotalTokens             int  `json:"totalTokens,omitempty"`
	CacheReadTokensKnown    bool `json:"cacheReadTokensKnown,omitempty"`
	CacheWriteTokensKnown   bool `json:"cacheWriteTokensKnown,omitempty"`
	CacheReadTokens         int  `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens        int  `json:"cacheWriteTokens,omitempty"`
	CacheCreationTokens     int  `json:"cacheCreationTokens,omitempty"`
	CacheableInputTokens    int  `json:"cacheableInputTokens,omitempty"`
	NonCacheableInputTokens int  `json:"nonCacheableInputTokens,omitempty"`
}

// ContextBudget is the concrete cross-owner audit DTO for a sealed provider
// request budget. It carries only bounded, secret-free metadata; it must never
// carry prompt text, snapshot content, credentials, endpoints or owner-private
// provider payloads.
type ContextBudget struct {
	CacheObservation         CacheObservation
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
	IncludedSections         []string
	NormalizedSections       []string
	CompressedSections       []string
	OmittedSections          []string
	RedactedPaths            []string
	ProtectedFields          []string
	IntegrityWarnings        []string
	SourceRefs               []string
	SectionHashes            []ContextSection
}

// ContextSection is the audit-safe identity of one rendered prompt section.
// SourceRefs are opaque identifiers only; they must not contain source content.
type ContextSection struct {
	ID         string   `json:"id,omitempty"`
	Hash       string   `json:"hash,omitempty"`
	SizeBytes  int      `json:"sizeBytes,omitempty"`
	SourceRefs []string `json:"sourceRefs,omitempty"`
}

func (v ContextBudget) Empty() bool {
	return v.ProviderID == "" && v.ModelID == "" && v.CapabilityRevision == "" && v.ConfigurationRevision == "" &&
		v.ContextWindowTokens == 0 && v.ProviderMaxOutputTokens == 0 && v.EffectiveMaxOutputTokens == 0 &&
		v.MaxBytes == 0 && v.ActualBytes == 0 && v.MaxTokens == 0 && v.EstimatedInputTokens == 0 &&
		v.EstimatedOutputTokens == 0 && v.ActualTokens == 0 && v.MaxRequestBytes == 0 && v.ProfileMode == "" &&
		v.CapabilitySource == "" && !v.ProviderSubmitted && !v.OverBudget && !v.Truncated &&
		v.StablePrefixHash == "" && v.RenderedStablePrefixHash == "" && v.CacheStrategy == "" &&
		v.ContextGateAction == "" && v.ContextGateReason == "" && v.BudgetFailureClass == "" &&
		len(v.IncludedSections) == 0 && len(v.NormalizedSections) == 0 && len(v.CompressedSections) == 0 &&
		len(v.OmittedSections) == 0 && len(v.RedactedPaths) == 0 && len(v.ProtectedFields) == 0 &&
		len(v.IntegrityWarnings) == 0 && len(v.SourceRefs) == 0 && len(v.SectionHashes) == 0 && v.CacheObservation == (CacheObservation{})
}

func (v ContextBudget) Payload() workorcheventpayload.Payload {
	fields := []workorcheventpayload.Field{
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
		// Versioned grouping keeps all metadata within Eventstream's 32-field
		// guard. It does not increase that guard or truncate the provider prompt.
		workorcheventpayload.StringField("schemaVersion", "workorch.context_budget_audit/v2"),
		workorcheventpayload.ObjectField("processing", workorcheventpayload.New(
			workorcheventpayload.StringArrayField("includedSections", v.IncludedSections),
			workorcheventpayload.StringArrayField("normalizedSections", v.NormalizedSections),
			workorcheventpayload.StringArrayField("compressedSections", v.CompressedSections),
			workorcheventpayload.StringArrayField("omittedSections", v.OmittedSections),
			workorcheventpayload.StringArrayField("redactedPaths", v.RedactedPaths),
			workorcheventpayload.StringArrayField("protectedFields", v.ProtectedFields),
			workorcheventpayload.StringArrayField("integrityWarnings", v.IntegrityWarnings),
			workorcheventpayload.StringArrayField("sourceRefs", v.SourceRefs),
		)),
		workorcheventpayload.ObjectField("cacheObservation", v.CacheObservation.Payload()),
	}
	if len(v.SectionHashes) > 0 {
		sections := make([]workorcheventpayload.Value, 0, len(v.SectionHashes))
		for _, section := range v.SectionHashes {
			sections = append(sections, workorcheventpayload.Value{Kind: workorcheventpayload.ValueObject, Object: workorcheventpayload.New(
				workorcheventpayload.StringField("id", section.ID),
				workorcheventpayload.StringField("hash", section.Hash),
				workorcheventpayload.IntField("sizeBytes", int64(section.SizeBytes)),
				workorcheventpayload.StringArrayField("sourceRefs", section.SourceRefs),
			).Fields})
		}
		fields = append(fields, workorcheventpayload.Field{Name: "sectionHashes", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: sections}})
	}
	return workorcheventpayload.New(fields...)
}

// CacheUsage is the concrete cross-owner representation of provider cache
// usage. Native and estimated values remain explicit in the contract.
type CacheUsage struct {
	CacheReadTokensKnown    bool
	CacheWriteTokensKnown   bool
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
	fields := []workorcheventpayload.Field{
		workorcheventpayload.BoolField("cacheReadTokensKnown", v.CacheReadTokensKnown || v.CacheReadTokens != 0),
		workorcheventpayload.BoolField("cacheWriteTokensKnown", v.CacheWriteTokensKnown || v.CacheWriteTokens != 0),
		workorcheventpayload.IntField("cacheCreationTokens", int64(v.CacheCreationTokens)),
		workorcheventpayload.IntField("cacheableInputTokens", int64(v.CacheableInputTokens)),
		workorcheventpayload.IntField("nonCacheableInputTokens", int64(v.NonCacheableInputTokens)),
		workorcheventpayload.BoolField("estimated", v.Estimated),
		workorcheventpayload.StringField("source", v.Source),
	}
	if v.CacheReadTokensKnown || v.CacheReadTokens != 0 {
		fields = append(fields, workorcheventpayload.IntField("cacheReadTokens", int64(v.CacheReadTokens)))
	}
	if v.CacheWriteTokensKnown || v.CacheWriteTokens != 0 {
		fields = append(fields, workorcheventpayload.IntField("cacheWriteTokens", int64(v.CacheWriteTokens)))
	}
	return workorcheventpayload.New(fields...)
}

// PromptStage is an immutable observation of the owner-selected prompt purpose.
// It never selects a Runtime transition or grants execution authority.
type PromptStage string

// CacheObservation describes the actual owner rendering, never cache support
// inferred from a model name or endpoint. Empty metadata remains unavailable.
type CacheObservation struct {
	RenderContractRef    string
	Stage                PromptStage
	Renderer             string
	MessageLayout        string
	MetadataState        string
	SourceSectionCount   int
	RecordedSectionCount int
	RequestedHints       string
}

func (v CacheObservation) Payload() workorcheventpayload.Payload {
	state := v.MetadataState
	if state == "" {
		state = "unavailable"
	}
	return workorcheventpayload.New(
		workorcheventpayload.StringField("schemaVersion", "workorch.cache_observation/v1"),
		workorcheventpayload.StringField("stage", string(v.Stage)),
		workorcheventpayload.StringField("renderer", v.Renderer),
		workorcheventpayload.StringField("renderContractRef", v.RenderContractRef),
		workorcheventpayload.StringField("messageLayout", v.MessageLayout),
		workorcheventpayload.StringField("metadataState", state),
		workorcheventpayload.IntField("sourceSectionCount", int64(v.SourceSectionCount)),
		workorcheventpayload.IntField("recordedSectionCount", int64(v.RecordedSectionCount)),
		workorcheventpayload.StringField("requestedHints", v.RequestedHints),
		workorcheventpayload.StringField("effectiveHints", "unknown"),
	)
}
