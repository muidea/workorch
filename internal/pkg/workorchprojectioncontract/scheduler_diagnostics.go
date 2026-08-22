package workorchprojectioncontract

// ExecutionAdmissionFailureDiagnostic is an immutable observation of one application
// execution-admission failure. Outcome is not a lifecycle state and cannot
// schedule, continue, recover, or transition a Run.
type ExecutionAdmissionFailureDiagnostic struct {
	WorkspaceID        string   `json:"workspaceId,omitempty"`
	RunID              string   `json:"runId,omitempty"`
	Kind               string   `json:"kind"`
	Outcome            string   `json:"outcome"`
	Reason             string   `json:"reason,omitempty"`
	Detail             string   `json:"detail,omitempty"`
	Recoverability     string   `json:"recoverability,omitempty"`
	RecommendedActions []string `json:"recommendedActions,omitempty"`
	DedupeKey          string   `json:"dedupeKey,omitempty"`
	Attempt            int      `json:"attempt,omitempty"`
	OccurredAt         string   `json:"occurredAt,omitempty"`
	Source             string   `json:"source,omitempty"`
}

type ExecutionAdmissionFailureCount struct {
	Kind   string `json:"kind,omitempty"`
	Reason string `json:"reason,omitempty"`
	Count  uint64 `json:"count"`
}

// SchedulerDiagnostics is the complete owner-neutral projection exposed by
// server and client boundaries. It contains transport/admission observations
// only; no scheduler lifecycle, watchdog, or recovery policy is represented.
type SchedulerDiagnostics struct {
	LatestAdmissionFailure  *ExecutionAdmissionFailureDiagnostic  `json:"latestAdmissionFailure,omitempty"`
	RecentAdmissionFailures []ExecutionAdmissionFailureDiagnostic `json:"recentAdmissionFailures,omitempty"`
	AdmissionFailureCounts  []ExecutionAdmissionFailureCount      `json:"admissionFailureCounts,omitempty"`
}

func CloneSchedulerDiagnostics(input *SchedulerDiagnostics) *SchedulerDiagnostics {
	if input == nil {
		return nil
	}
	ret := &SchedulerDiagnostics{
		RecentAdmissionFailures: cloneExecutionAdmissionFailureDiagnostics(input.RecentAdmissionFailures),
		AdmissionFailureCounts:  append([]ExecutionAdmissionFailureCount(nil), input.AdmissionFailureCounts...),
	}
	if input.LatestAdmissionFailure != nil {
		value := cloneExecutionAdmissionFailureDiagnostic(*input.LatestAdmissionFailure)
		ret.LatestAdmissionFailure = &value
	}
	return ret
}

func cloneExecutionAdmissionFailureDiagnostics(input []ExecutionAdmissionFailureDiagnostic) []ExecutionAdmissionFailureDiagnostic {
	if len(input) == 0 {
		return nil
	}
	ret := make([]ExecutionAdmissionFailureDiagnostic, 0, len(input))
	for _, item := range input {
		ret = append(ret, cloneExecutionAdmissionFailureDiagnostic(item))
	}
	return ret
}

func cloneExecutionAdmissionFailureDiagnostic(input ExecutionAdmissionFailureDiagnostic) ExecutionAdmissionFailureDiagnostic {
	ret := input
	ret.RecommendedActions = append([]string(nil), input.RecommendedActions...)
	return ret
}
