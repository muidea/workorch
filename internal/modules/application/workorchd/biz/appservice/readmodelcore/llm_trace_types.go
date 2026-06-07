package readmodelcore

type LLMContextBudgetTrace struct {
	StepID                string         `json:"stepId"`
	ProviderID            string         `json:"providerId,omitempty"`
	Model                 string         `json:"model,omitempty"`
	MaxBytes              int            `json:"maxBytes"`
	ActualBytes           int            `json:"actualBytes"`
	MaxTokens             int            `json:"maxTokens,omitempty"`
	EstimatedInputTokens  int            `json:"estimatedInputTokens,omitempty"`
	EstimatedOutputTokens int            `json:"estimatedOutputTokens,omitempty"`
	ActualTokens          int            `json:"actualTokens,omitempty"`
	ShrinkLevel           int            `json:"shrinkLevel,omitempty"`
	Truncated             bool           `json:"truncated"`
	Rules                 []string       `json:"rules,omitempty"`
	SourceRefs            []string       `json:"sourceRefs,omitempty"`
	IncludedSections      []string       `json:"includedSections,omitempty"`
	NormalizedSections    []string       `json:"normalizedSections,omitempty"`
	CompressedSections    []string       `json:"compressedSections,omitempty"`
	OmittedSections       []string       `json:"omittedSections,omitempty"`
	GoalDoDCompressed     bool           `json:"goalDoDCompressed"`
	OverBudget            bool           `json:"overBudget"`
	P0Overflow            bool           `json:"p0Overflow"`
	ProfileMode           string         `json:"profileMode,omitempty"`
	CapabilitySource      string         `json:"capabilitySource,omitempty"`
	ContextUtilization    float64        `json:"contextUtilizationRatio,omitempty"`
	MaxRequestBytes       int            `json:"maxRequestBytes,omitempty"`
	StageBudgetRatio      float64        `json:"stageBudgetRatio,omitempty"`
	StageBudgetSource     string         `json:"stageBudgetSource,omitempty"`
	TracePolicy           string         `json:"tracePolicy,omitempty"`
	PromptCaptured        bool           `json:"promptCaptured"`
	Estimator             string         `json:"estimator,omitempty"`
	Profile               map[string]any `json:"profile,omitempty"`
	CachePlan             map[string]any `json:"cachePlan,omitempty"`
}
