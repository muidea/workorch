package readmodelcore

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

type LLMUsageTrace struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspaceId"`
	RunID         string `json:"runId"`
	StepID        string `json:"stepId,omitempty"`
	ProviderID    string `json:"providerId,omitempty"`
	Provider      string `json:"provider,omitempty"`
	APISurface    string `json:"apiSurface,omitempty"`
	Model         string `json:"model,omitempty"`
	Operation     string `json:"operation,omitempty"`
	InputTokens   int    `json:"inputTokens,omitempty"`
	OutputTokens  int    `json:"outputTokens,omitempty"`
	TotalTokens   int    `json:"totalTokens,omitempty"`
	Estimated     bool   `json:"estimated"`
	UsageSource   string `json:"usageSource,omitempty"`
	RequestBytes  int    `json:"requestBytes,omitempty"`
	ResponseBytes int    `json:"responseBytes,omitempty"`
	Streaming     bool   `json:"streaming"`
	// These values originate from the durable EventRecord and retain the
	// concrete Payload representation through projection and port mapping.
	ContextBudget workorcheventpayload.Payload `json:"contextBudget,omitempty"`
	CacheUsage    workorcheventpayload.Payload `json:"cacheUsage,omitempty"`
	CacheMarkers  workorcheventpayload.Payload `json:"cacheMarkersSent,omitempty"`
	StartedAt     string                       `json:"startedAt,omitempty"`
	CompletedAt   string                       `json:"completedAt,omitempty"`
	DurationMs    int                          `json:"durationMs,omitempty"`
	Status        string                       `json:"status,omitempty"`
	FailureClass  string                       `json:"failureClass,omitempty"`
	OccurredAt    string                       `json:"occurredAt,omitempty"`
	EvidenceRef   string                       `json:"evidenceRef,omitempty"`
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
