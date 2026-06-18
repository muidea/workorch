package readmodelcore

type ContinuationDiagnostic struct {
	WorkspaceID        string   `json:"workspaceId,omitempty"`
	RunID              string   `json:"runId,omitempty"`
	Kind               string   `json:"kind"`
	State              string   `json:"state"`
	Reason             string   `json:"reason,omitempty"`
	Detail             string   `json:"detail,omitempty"`
	Recoverability     string   `json:"recoverability,omitempty"`
	RecommendedActions []string `json:"recommendedActions,omitempty"`
	DedupeKey          string   `json:"dedupeKey,omitempty"`
	Attempt            int      `json:"attempt,omitempty"`
	OccurredAt         string   `json:"occurredAt,omitempty"`
	Source             string   `json:"source,omitempty"`
}

type ContinuationDiagnosticCount struct {
	Kind   string `json:"kind,omitempty"`
	Reason string `json:"reason,omitempty"`
	Count  uint64 `json:"count"`
}

type WatchdogDiagnostics struct {
	LatestTickAt      string `json:"latestTickAt,omitempty"`
	LatestDurationMs  int64  `json:"latestDurationMs,omitempty"`
	LatestError       string `json:"latestError,omitempty"`
	StaleRecoveryRuns uint64 `json:"staleRecoveryRuns,omitempty"`
}

type SchedulerDiagnostics struct {
	Watchdog                    WatchdogDiagnostics           `json:"watchdog,omitempty"`
	LatestContinuation          *ContinuationDiagnostic       `json:"latestContinuation,omitempty"`
	RecentContinuations         []ContinuationDiagnostic      `json:"recentContinuations,omitempty"`
	ContinuationFailureCounts   []ContinuationDiagnosticCount `json:"continuationFailureCounts,omitempty"`
	ContinuationNoActionCounts  []ContinuationDiagnosticCount `json:"continuationNoActionCounts,omitempty"`
	ContinuationCompletedCounts []ContinuationDiagnosticCount `json:"continuationCompletedCounts,omitempty"`
}
