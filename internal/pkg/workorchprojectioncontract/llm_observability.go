package workorchprojectioncontract

import (
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// EventRecord is the immutable, redacted eventstream read projection exposed
// to application and protocol readers. It is not the eventstream owner's
// persisted record and cannot be posted back to EventHub.
type EventRecord struct {
	ID             string                       `json:"id"`
	Version        string                       `json:"version"`
	Topic          string                       `json:"topic"`
	OccurredAt     string                       `json:"occurredAt,omitempty"`
	SourceModule   string                       `json:"sourceModule,omitempty"`
	SubjectType    string                       `json:"subjectType,omitempty"`
	SubjectID      string                       `json:"subjectId,omitempty"`
	WorkspaceID    string                       `json:"workspaceId,omitempty"`
	RunID          string                       `json:"runId,omitempty"`
	StepID         string                       `json:"stepId,omitempty"`
	StreamPosition int64                        `json:"streamPosition,omitempty"`
	Revision       int64                        `json:"revision,omitempty"`
	Payload        workorcheventpayload.Payload `json:"payload,omitempty"`
	PayloadSummary workorcheventpayload.Payload `json:"payloadSummary,omitempty"`
	PayloadRef     string                       `json:"payloadRef,omitempty"`
}

func CloneEventRecords(items []EventRecord) []EventRecord {
	if items == nil {
		return nil
	}
	ret := make([]EventRecord, 0, len(items))
	for _, item := range items {
		item.Payload = item.Payload.Clone()
		item.PayloadSummary = item.PayloadSummary.Clone()
		ret = append(ret, item)
	}
	return ret
}

type LLMContextBudgetTrace struct {
	StepID                string                       `json:"stepId"`
	ProviderID            string                       `json:"providerId,omitempty"`
	Model                 string                       `json:"model,omitempty"`
	CapabilityRevision    string                       `json:"capabilityRevision,omitempty"`
	ConfigurationRevision string                       `json:"configurationRevision,omitempty"`
	MaxBytes              int                          `json:"maxBytes"`
	ActualBytes           int                          `json:"actualBytes"`
	MaxTokens             int                          `json:"maxTokens,omitempty"`
	EstimatedInputTokens  int                          `json:"estimatedInputTokens,omitempty"`
	EstimatedOutputTokens int                          `json:"estimatedOutputTokens,omitempty"`
	ActualTokens          int                          `json:"actualTokens,omitempty"`
	ShrinkLevel           int                          `json:"shrinkLevel,omitempty"`
	Truncated             bool                         `json:"truncated"`
	Rules                 []string                     `json:"rules,omitempty"`
	SourceRefs            []string                     `json:"sourceRefs,omitempty"`
	IncludedSections      []string                     `json:"includedSections,omitempty"`
	NormalizedSections    []string                     `json:"normalizedSections,omitempty"`
	CompressedSections    []string                     `json:"compressedSections,omitempty"`
	OmittedSections       []string                     `json:"omittedSections,omitempty"`
	GoalDoDCompressed     bool                         `json:"goalDoDCompressed"`
	OverBudget            bool                         `json:"overBudget"`
	P0Overflow            bool                         `json:"p0Overflow"`
	ProfileMode           string                       `json:"profileMode,omitempty"`
	CapabilitySource      string                       `json:"capabilitySource,omitempty"`
	ContextUtilization    float64                      `json:"contextUtilizationRatio,omitempty"`
	MaxRequestBytes       int                          `json:"maxRequestBytes,omitempty"`
	StageBudgetRatio      float64                      `json:"stageBudgetRatio,omitempty"`
	StageBudgetSource     string                       `json:"stageBudgetSource,omitempty"`
	TracePolicy           string                       `json:"tracePolicy,omitempty"`
	PromptCaptured        bool                         `json:"promptCaptured"`
	Estimator             string                       `json:"estimator,omitempty"`
	ProviderSubmitted     bool                         `json:"providerSubmitted"`
	ContextGateAction     string                       `json:"contextGateAction,omitempty"`
	ContextGateReason     string                       `json:"contextGateReason,omitempty"`
	BudgetFailureClass    string                       `json:"budgetFailureClass,omitempty"`
	RedactedPaths         []string                     `json:"redactedPaths,omitempty"`
	ProtectedFields       []string                     `json:"protectedFields,omitempty"`
	IntegrityWarnings     []string                     `json:"integrityWarnings,omitempty"`
	SectionHashes         []llmaudit.ContextSection    `json:"sectionHashes,omitempty"`
	Profile               workorcheventpayload.Payload `json:"profile,omitempty"`
	CachePlan             workorcheventpayload.Payload `json:"cachePlan,omitempty"`
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

type LLMUsageTrace struct {
	ID                   string                        `json:"id"`
	LogicalRequestID     string                        `json:"logicalRequestId,omitempty"`
	PhysicalCallID       string                        `json:"physicalCallId,omitempty"`
	WorkspaceID          string                        `json:"workspaceId"`
	RunID                string                        `json:"runId"`
	StepID               string                        `json:"stepId,omitempty"`
	ProviderID           string                        `json:"providerId,omitempty"`
	Provider             string                        `json:"provider,omitempty"`
	APISurface           string                        `json:"apiSurface,omitempty"`
	Model                string                        `json:"model,omitempty"`
	Operation            string                        `json:"operation,omitempty"`
	InputTokens          int                           `json:"inputTokens,omitempty"`
	OutputTokens         int                           `json:"outputTokens,omitempty"`
	TotalTokens          int                           `json:"totalTokens,omitempty"`
	Estimated            bool                          `json:"estimated"`
	UsageSource          string                        `json:"usageSource,omitempty"`
	RequestBytes         int                           `json:"requestBytes,omitempty"`
	ResponseBytes        int                           `json:"responseBytes,omitempty"`
	PhysicalResponseMode llmaudit.PhysicalResponseMode `json:"physicalResponseMode"`
	ProviderSubmitted    bool                          `json:"providerSubmitted"`
	ContextBudget        workorcheventpayload.Payload  `json:"contextBudget,omitempty"`
	CacheUsage           workorcheventpayload.Payload  `json:"cacheUsage,omitempty"`
	CacheMarkers         workorcheventpayload.Payload  `json:"cacheMarkersSent,omitempty"`
	StartedAt            string                        `json:"startedAt,omitempty"`
	CompletedAt          string                        `json:"completedAt,omitempty"`
	DurationMs           int                           `json:"durationMs,omitempty"`
	Status               string                        `json:"status,omitempty"`
	FailureClass         string                        `json:"failureClass,omitempty"`
	OccurredAt           string                        `json:"occurredAt,omitempty"`
	EvidenceRef          string                        `json:"evidenceRef,omitempty"`
}

type LLMUsageSummary struct {
	EntryTotal              int                              `json:"entryTotal"`
	InputTokens             int                              `json:"inputTokens"`
	OutputTokens            int                              `json:"outputTokens"`
	TotalTokens             int                              `json:"totalTokens"`
	EstimatedEntries        int                              `json:"estimatedEntries"`
	ProviderNativeEntries   int                              `json:"providerNativeEntries"`
	FailedEntries           int                              `json:"failedEntries"`
	CacheReadTokens         int                              `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens        int                              `json:"cacheWriteTokens,omitempty"`
	CacheCreationTokens     int                              `json:"cacheCreationTokens,omitempty"`
	CacheableInputTokens    int                              `json:"cacheableInputTokens,omitempty"`
	NonCacheableInputTokens int                              `json:"nonCacheableInputTokens,omitempty"`
	CacheReadInputRatio     float64                          `json:"cacheReadInputRatio,omitempty"`
	CacheableInputRatio     float64                          `json:"cacheableInputRatio,omitempty"`
	NonCacheableInputRatio  float64                          `json:"nonCacheableInputRatio,omitempty"`
	CacheEstimatedEntries   int                              `json:"cacheEstimatedEntries,omitempty"`
	CacheProviderEntries    int                              `json:"cacheProviderEntries,omitempty"`
	ProviderModelBreakdown  []LLMUsageProviderModelBreakdown `json:"providerModelBreakdown,omitempty"`
}

type LLMUsageProviderModelBreakdown struct {
	ProviderID              string  `json:"providerId,omitempty"`
	Provider                string  `json:"provider,omitempty"`
	APISurface              string  `json:"apiSurface,omitempty"`
	Model                   string  `json:"model,omitempty"`
	ProviderMode            string  `json:"providerMode,omitempty"`
	EntryTotal              int     `json:"entryTotal"`
	InputTokens             int     `json:"inputTokens"`
	OutputTokens            int     `json:"outputTokens"`
	TotalTokens             int     `json:"totalTokens"`
	CacheReadTokens         int     `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens        int     `json:"cacheWriteTokens,omitempty"`
	CacheCreationTokens     int     `json:"cacheCreationTokens,omitempty"`
	CacheablePromptCount    int     `json:"cacheablePromptCount,omitempty"`
	ActualCacheHitCount     int     `json:"actualCacheHitCount,omitempty"`
	CacheCreationCount      int     `json:"cacheCreationCount,omitempty"`
	PromptCacheKeySentCount int     `json:"promptCacheKeySentCount,omitempty"`
	CacheControlSentCount   int     `json:"cacheControlSentCount,omitempty"`
	AutoPrefixExpectedCount int     `json:"autoPrefixExpectedCount,omitempty"`
	CacheReadInputRatio     float64 `json:"cacheReadInputRatio,omitempty"`
	EstimatedEntries        int     `json:"estimatedEntries"`
	FailedEntries           int     `json:"failedEntries"`
}

func CloneLLMContextBudgetTraces(items []LLMContextBudgetTrace) []LLMContextBudgetTrace {
	if items == nil {
		return nil
	}
	ret := make([]LLMContextBudgetTrace, 0, len(items))
	for _, item := range items {
		item.Rules = append([]string(nil), item.Rules...)
		item.SourceRefs = append([]string(nil), item.SourceRefs...)
		item.IncludedSections = append([]string(nil), item.IncludedSections...)
		item.NormalizedSections = append([]string(nil), item.NormalizedSections...)
		item.CompressedSections = append([]string(nil), item.CompressedSections...)
		item.OmittedSections = append([]string(nil), item.OmittedSections...)
		item.RedactedPaths = append([]string(nil), item.RedactedPaths...)
		item.ProtectedFields = append([]string(nil), item.ProtectedFields...)
		item.IntegrityWarnings = append([]string(nil), item.IntegrityWarnings...)
		item.SectionHashes = cloneContextSections(item.SectionHashes)
		item.Profile = item.Profile.Clone()
		item.CachePlan = item.CachePlan.Clone()
		ret = append(ret, item)
	}
	return ret
}

func CloneLLMUsageTraces(items []LLMUsageTrace) []LLMUsageTrace {
	if items == nil {
		return nil
	}
	ret := make([]LLMUsageTrace, 0, len(items))
	for _, item := range items {
		item.ContextBudget = item.ContextBudget.Clone()
		item.CacheUsage = item.CacheUsage.Clone()
		item.CacheMarkers = item.CacheMarkers.Clone()
		ret = append(ret, item)
	}
	return ret
}

func CloneLLMUsageSummary(item LLMUsageSummary) LLMUsageSummary {
	item.ProviderModelBreakdown = append([]LLMUsageProviderModelBreakdown(nil), item.ProviderModelBreakdown...)
	return item
}

func cloneContextSections(items []llmaudit.ContextSection) []llmaudit.ContextSection {
	ret := append([]llmaudit.ContextSection(nil), items...)
	for idx := range ret {
		ret[idx].SourceRefs = append([]string(nil), items[idx].SourceRefs...)
	}
	return ret
}
