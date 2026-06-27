package workorchport

type ContinuationDiagnosticInfo struct {
	WorkspaceID        string
	RunID              string
	Kind               string
	State              string
	Reason             string
	Detail             string
	Readback           *ContinuationReadbackDiagnosticInfo
	Recoverability     string
	RecommendedActions []string
	DedupeKey          string
	Attempt            int
	OccurredAt         string
	Source             string
}

type ContinuationReadbackDiagnosticInfo struct {
	WorkspaceID             string
	RunID                   string
	Action                  string
	Found                   bool
	OldRunState             string
	NewRunState             string
	OldBlockingTicketID     string
	NewBlockingTicketID     string
	OldBlockingApprovalID   string
	NewBlockingApprovalID   string
	SuccessorRunID          string
	LatestStepID            string
	Terminal                bool
	GovernedWait            bool
	Advanced                bool
	Reason                  string
	ProjectionRevision      int64
	SourceEventRevision     int64
	ProjectedSourceRevision int64
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
