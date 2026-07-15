package readmodelcore

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

type LLMContextBudgetTrace struct {
	StepID                string   `json:"stepId"`
	ProviderID            string   `json:"providerId,omitempty"`
	Model                 string   `json:"model,omitempty"`
	CapabilityRevision    string   `json:"capabilityRevision,omitempty"`
	ConfigurationRevision string   `json:"configurationRevision,omitempty"`
	MaxBytes              int      `json:"maxBytes"`
	ActualBytes           int      `json:"actualBytes"`
	MaxTokens             int      `json:"maxTokens,omitempty"`
	EstimatedInputTokens  int      `json:"estimatedInputTokens,omitempty"`
	EstimatedOutputTokens int      `json:"estimatedOutputTokens,omitempty"`
	ActualTokens          int      `json:"actualTokens,omitempty"`
	ShrinkLevel           int      `json:"shrinkLevel,omitempty"`
	Truncated             bool     `json:"truncated"`
	Rules                 []string `json:"rules,omitempty"`
	SourceRefs            []string `json:"sourceRefs,omitempty"`
	IncludedSections      []string `json:"includedSections,omitempty"`
	NormalizedSections    []string `json:"normalizedSections,omitempty"`
	CompressedSections    []string `json:"compressedSections,omitempty"`
	OmittedSections       []string `json:"omittedSections,omitempty"`
	GoalDoDCompressed     bool     `json:"goalDoDCompressed"`
	OverBudget            bool     `json:"overBudget"`
	P0Overflow            bool     `json:"p0Overflow"`
	ProfileMode           string   `json:"profileMode,omitempty"`
	CapabilitySource      string   `json:"capabilitySource,omitempty"`
	ContextUtilization    float64  `json:"contextUtilizationRatio,omitempty"`
	MaxRequestBytes       int      `json:"maxRequestBytes,omitempty"`
	StageBudgetRatio      float64  `json:"stageBudgetRatio,omitempty"`
	StageBudgetSource     string   `json:"stageBudgetSource,omitempty"`
	TracePolicy           string   `json:"tracePolicy,omitempty"`
	PromptCaptured        bool     `json:"promptCaptured"`
	Estimator             string   `json:"estimator,omitempty"`
	// Profile and CachePlan remain structured event payload values at this
	// read-model boundary; they are not map authority reconstructed by callers.
	Profile   workorcheventpayload.Payload `json:"profile,omitempty"`
	CachePlan workorcheventpayload.Payload `json:"cachePlan,omitempty"`
}

type LLMContextBudgetSummary struct {
	TraceTotal             int     `json:"traceTotal"`
	TruncatedCount         int     `json:"truncatedCount,omitempty"`
	OverBudgetCount        int     `json:"overBudgetCount,omitempty"`
	P0OverflowCount        int     `json:"p0OverflowCount,omitempty"`
	GoalDoDCompressedCount int     `json:"goalDoDCompressedCount,omitempty"`
	PromptCapturedCount    int     `json:"promptCapturedCount,omitempty"`
	MaxShrinkLevel         int     `json:"maxShrinkLevel,omitempty"`
	MaxContextUtilization  float64 `json:"maxContextUtilizationRatio,omitempty"`
	AvgContextUtilization  float64 `json:"avgContextUtilizationRatio,omitempty"`
	MaxInputTokens         int     `json:"maxInputTokens,omitempty"`
	MaxRequestBytes        int     `json:"maxRequestBytes,omitempty"`
	RecentWindowSize       int     `json:"recentWindowSize,omitempty"`
	RecentTraceTotal       int     `json:"recentTraceTotal,omitempty"`
	RecentTruncatedCount   int     `json:"recentTruncatedCount,omitempty"`
	RecentOverBudgetCount  int     `json:"recentOverBudgetCount,omitempty"`
	RecentMaxUtilization   float64 `json:"recentMaxContextUtilizationRatio,omitempty"`
	RecentAvgUtilization   float64 `json:"recentAvgContextUtilizationRatio,omitempty"`
	RecentMaxInputTokens   int     `json:"recentMaxInputTokens,omitempty"`
	UtilizationTrend       string  `json:"contextUtilizationTrend,omitempty"`
}
