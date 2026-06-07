package readmodelcore

type LLMUsageTrace struct {
	ID            string         `json:"id"`
	WorkspaceID   string         `json:"workspaceId"`
	RunID         string         `json:"runId"`
	StepID        string         `json:"stepId,omitempty"`
	ProviderID    string         `json:"providerId,omitempty"`
	Provider      string         `json:"provider,omitempty"`
	Model         string         `json:"model,omitempty"`
	Operation     string         `json:"operation,omitempty"`
	InputTokens   int            `json:"inputTokens,omitempty"`
	OutputTokens  int            `json:"outputTokens,omitempty"`
	TotalTokens   int            `json:"totalTokens,omitempty"`
	Estimated     bool           `json:"estimated"`
	UsageSource   string         `json:"usageSource,omitempty"`
	RequestBytes  int            `json:"requestBytes,omitempty"`
	ResponseBytes int            `json:"responseBytes,omitempty"`
	Streaming     bool           `json:"streaming"`
	ContextBudget map[string]any `json:"contextBudget,omitempty"`
	CacheUsage    map[string]any `json:"cacheUsage,omitempty"`
	StartedAt     string         `json:"startedAt,omitempty"`
	CompletedAt   string         `json:"completedAt,omitempty"`
	DurationMs    int            `json:"durationMs,omitempty"`
	Status        string         `json:"status,omitempty"`
	FailureClass  string         `json:"failureClass,omitempty"`
	OccurredAt    string         `json:"occurredAt,omitempty"`
	EvidenceRef   string         `json:"evidenceRef,omitempty"`
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
	ProviderID          string  `json:"providerId,omitempty"`
	Provider            string  `json:"provider,omitempty"`
	Model               string  `json:"model,omitempty"`
	EntryTotal          int     `json:"entryTotal"`
	InputTokens         int     `json:"inputTokens"`
	OutputTokens        int     `json:"outputTokens"`
	TotalTokens         int     `json:"totalTokens"`
	CacheReadTokens     int     `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens    int     `json:"cacheWriteTokens,omitempty"`
	CacheReadInputRatio float64 `json:"cacheReadInputRatio,omitempty"`
	EstimatedEntries    int     `json:"estimatedEntries"`
	FailedEntries       int     `json:"failedEntries"`
}
