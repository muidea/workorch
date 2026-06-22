package workorchport

type ContinuationDiagnosticInfo struct {
	WorkspaceID        string
	RunID              string
	Kind               string
	State              string
	Reason             string
	Detail             string
	Recoverability     string
	RecommendedActions []string
	DedupeKey          string
	Attempt            int
	OccurredAt         string
	Source             string
}

type ContinuationDiagnosticCountInfo struct {
	Kind   string
	Reason string
	Count  uint64
}

type WatchdogDiagnosticsInfo struct {
	TimerRegistrations uint64
	LatestTickAt       string
	LatestDurationMs   int64
	LatestError        string
	StaleRecoveryRuns  uint64
}

type SchedulerDiagnosticsInfo struct {
	Watchdog                    WatchdogDiagnosticsInfo
	LatestContinuation          *ContinuationDiagnosticInfo
	RecentContinuations         []ContinuationDiagnosticInfo
	ContinuationFailureCounts   []ContinuationDiagnosticCountInfo
	ContinuationNoActionCounts  []ContinuationDiagnosticCountInfo
	ContinuationCompletedCounts []ContinuationDiagnosticCountInfo
}
