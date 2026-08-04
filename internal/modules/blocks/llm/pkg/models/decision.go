package models

import "github.com/muidea/workorch/internal/pkg/llmaudit"

type ProviderRequest struct {
	WorkspaceID    string `json:"workspaceId"`
	RunID          string `json:"runId"`
	StepID         string `json:"stepId"`
	Kind           string `json:"kind"`
	Model          string `json:"model"`
	SystemLocale   string `json:"systemLocale,omitempty"`
	DialogLanguage string `json:"dialogLanguage,omitempty"`
	Schema         string `json:"schema"`
	// RawTextOutput disables provider JSON-object mode for an explicitly typed
	// continuation suffix. All ordinary Runtime candidate requests remain
	// schema-constrained JSON objects.
	RawTextOutput bool                   `json:"rawTextOutput,omitempty"`
	TraceRefs     map[string]string      `json:"traceRefs,omitempty"`
	Config        RedactedProviderConfig `json:"config"`
	PromptContext *PromptContext         `json:"promptContext,omitempty"`
	ContextBudget *ProviderContextBudget `json:"contextBudget,omitempty"`
}

type PromptContext struct {
	SchemaVersion string                 `json:"schemaVersion,omitempty"`
	Sections      []PromptSection        `json:"sections,omitempty"`
	Turns         []PromptTurn           `json:"turns,omitempty"`
	Normalization *NormalizationManifest `json:"normalization,omitempty"`
}

type PromptSection struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name,omitempty"`
	Role               string   `json:"role,omitempty"`
	Policy             string   `json:"policy,omitempty"`
	Cacheability       string   `json:"cacheability,omitempty"`
	Hash               string   `json:"hash,omitempty"`
	SourceRefs         []string `json:"sourceRefs,omitempty"`
	DefinitionRevision string   `json:"definitionRevision,omitempty"`
	DisplayOrder       int      `json:"displayOrder,omitempty"`
	Content            string   `json:"content,omitempty"`
	Normalized         bool     `json:"normalized,omitempty"`
	Protected          bool     `json:"protected,omitempty"`
}

type PromptTurn struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	SectionID string `json:"sectionId,omitempty"`
}

type NormalizationManifest struct {
	NormalizedSections   []string `json:"normalizedSections,omitempty"`
	ProtectedSections    []string `json:"protectedSections,omitempty"`
	EstimatedTokensSaved int      `json:"estimatedTokensSaved,omitempty"`
	SemanticLoss         bool     `json:"semanticLoss"`
}

type ProviderContextBudget struct {
	// Sealed provider/model identity and revisions for request audit.
	ProviderID               string `json:"providerId,omitempty"`
	ModelID                  string `json:"modelId,omitempty"`
	CapabilityRevision       string `json:"capabilityRevision,omitempty"`
	ConfigurationRevision    string `json:"configurationRevision,omitempty"`
	ContextWindowTokens      int    `json:"contextWindowTokens,omitempty"`
	ProviderMaxOutputTokens  int    `json:"providerMaxOutputTokens,omitempty"`
	EffectiveMaxOutputTokens int    `json:"effectiveMaxOutputTokens,omitempty"`

	MaxBytes              int                    `json:"maxBytes"`
	ActualBytes           int                    `json:"actualBytes"`
	MaxTokens             int                    `json:"maxTokens,omitempty"`
	EstimatedInputTokens  int                    `json:"estimatedInputTokens,omitempty"`
	EstimatedOutputTokens int                    `json:"estimatedOutputTokens,omitempty"`
	ActualTokens          int                    `json:"actualTokens,omitempty"`
	MinimumProtectedBytes int                    `json:"minimumProtectedRequestBytes,omitempty"`
	ShrinkLevel           int                    `json:"shrinkLevel,omitempty"`
	Truncated             bool                   `json:"truncated"`
	Rules                 []string               `json:"rules,omitempty"`
	SourceRefs            []string               `json:"sourceRefs,omitempty"`
	IncludedSections      []string               `json:"includedSections,omitempty"`
	NormalizedSections    []string               `json:"normalizedSections,omitempty"`
	CompressedSections    []string               `json:"compressedSections,omitempty"`
	OmittedSections       []string               `json:"omittedSections,omitempty"`
	GoalDoDCompressed     bool                   `json:"goalDoDCompressed,omitempty"`
	OverBudget            bool                   `json:"overBudget,omitempty"`
	P0Overflow            bool                   `json:"p0Overflow,omitempty"`
	ProfileMode           string                 `json:"profileMode,omitempty"`
	CapabilitySource      string                 `json:"capabilitySource,omitempty"`
	ContextUtilization    float64                `json:"contextUtilizationRatio,omitempty"`
	MaxRequestBytes       int                    `json:"maxRequestBytes,omitempty"`
	StageBudgetRatio      float64                `json:"stageBudgetRatio,omitempty"`
	StageBudgetSource     string                 `json:"stageBudgetSource,omitempty"`
	TracePolicy           string                 `json:"tracePolicy,omitempty"`
	PromptCaptured        bool                   `json:"promptCaptured"`
	Estimator             string                 `json:"estimator,omitempty"`
	RedactionApplied      bool                   `json:"redactionApplied,omitempty"`
	RedactedPaths         []string               `json:"redactedPaths,omitempty"`
	ProtectedFields       []string               `json:"protectedFields,omitempty"`
	IntegrityWarnings     []string               `json:"integrityWarnings,omitempty"`
	ContextGateAction     string                 `json:"contextGateAction,omitempty"`
	ContextGateReason     string                 `json:"contextGateReason,omitempty"`
	BudgetFailureClass    string                 `json:"budgetFailureClass,omitempty"`
	ProviderSubmitted     bool                   `json:"providerSubmitted"`
	CachePlan             *ProviderCachePlan     `json:"cachePlan,omitempty"`
	Profile               ProviderContextProfile `json:"profile,omitempty"`
}

type ProviderCachePlan struct {
	SchemaVersion            string                       `json:"schemaVersion,omitempty"`
	Strategy                 string                       `json:"strategy,omitempty"`
	CacheKey                 string                       `json:"cacheKey,omitempty"`
	StablePrefixHash         string                       `json:"stablePrefixHash,omitempty"`
	RenderedStablePrefixHash string                       `json:"renderedStablePrefixHash,omitempty"`
	WorkspaceDefinitionHash  string                       `json:"workspaceDefinitionHash,omitempty"`
	CapabilityContractHash   string                       `json:"capabilityContractHash,omitempty"`
	StageBundleHash          string                       `json:"stageBundleHash,omitempty"`
	VolatileTailHash         string                       `json:"volatileTailHash,omitempty"`
	LargeRefsHash            string                       `json:"largeRefsHash,omitempty"`
	SectionHashes            []ProviderCacheSectionHash   `json:"sectionHashes,omitempty"`
	CacheBreakers            []ProviderCacheBreaker       `json:"cacheBreakers,omitempty"`
	Canonicalization         ProviderCanonicalizationPlan `json:"canonicalization,omitempty"`
	ObservedRequestShape     ProviderObservedRequestShape `json:"observedRequestShape,omitempty"`
	AdapterHint              ProviderAdapterCacheHint     `json:"adapterHint,omitempty"`
}

type ProviderCacheSectionHash struct {
	SectionID          string   `json:"sectionId"`
	Name               string   `json:"name,omitempty"`
	Cacheability       string   `json:"cacheability,omitempty"`
	Hash               string   `json:"hash,omitempty"`
	SizeBytes          int      `json:"sizeBytes,omitempty"`
	SourceRefs         []string `json:"sourceRefs,omitempty"`
	DefinitionRevision string   `json:"definitionRevision,omitempty"`
}

type ProviderCacheBreaker struct {
	SectionID string `json:"sectionId,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type ProviderCanonicalizationPlan struct {
	SectionOrderPolicy string   `json:"sectionOrderPolicy,omitempty"`
	SectionOrder       []string `json:"sectionOrder,omitempty"`
	OrderedDataPolicy  string   `json:"orderedDataPolicy,omitempty"`
	MapKeyPolicy       string   `json:"mapKeyPolicy,omitempty"`
	ProviderMapping    string   `json:"providerMapping,omitempty"`
}

type ProviderObservedRequestShape struct {
	ProviderFacingMode  string `json:"providerFacingMode,omitempty"`
	TurnsHaveSectionIDs bool   `json:"turnsHaveSectionIds,omitempty"`
	TypedEventTape      bool   `json:"typedEventTape,omitempty"`
}

type ProviderAdapterCacheHint struct {
	Enabled          bool     `json:"enabled"`
	Mode             string   `json:"mode,omitempty"`
	ProviderMapping  string   `json:"providerMapping,omitempty"`
	CacheablePrefix  []string `json:"cacheablePrefix,omitempty"`
	VolatileSections []string `json:"volatileSections,omitempty"`
	CacheBreakers    []string `json:"cacheBreakers,omitempty"`
}

type ProviderContextProfile struct {
	ProfileMode             string  `json:"profileMode,omitempty"`
	CapabilitySource        string  `json:"capabilitySource,omitempty"`
	ContextWindowTokens     int     `json:"contextWindowTokens,omitempty"`
	MaxInputTokens          int     `json:"maxInputTokens,omitempty"`
	ReservedOutputTokens    int     `json:"reservedOutputTokens,omitempty"`
	SafetyRatio             float64 `json:"safetyRatio,omitempty"`
	ContextUtilizationRatio float64 `json:"contextUtilizationRatio,omitempty"`
	BytesHardLimit          int     `json:"bytesHardLimit,omitempty"`
	MaxRequestBytes         int     `json:"maxRequestBytes,omitempty"`
	EstimatedTokensPerByte  float64 `json:"estimatedTokensPerByte,omitempty"`
}

type ProviderCompletionResult struct {
	Payload []byte                  `json:"payload,omitempty"`
	Usage   *llmaudit.ProviderUsage `json:"usage,omitempty"`
}
