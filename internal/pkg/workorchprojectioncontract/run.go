package workorchprojectioncontract

// RunLineage is the owner-neutral management projection of Run lineage.
type RunLineage struct {
	PredecessorRunID    string `json:"predecessorRunId,omitempty"`
	PredecessorResultID string `json:"predecessorResultId,omitempty"`
	SuccessorRunID      string `json:"successorRunId,omitempty"`
	LineageState        string `json:"lineageState,omitempty"`
}

// RunStatus is the owner-neutral status projection assembled from already
// persisted owner state. It cannot transition a Run or schedule Runtime work.
type RunStatus struct {
	ID                 string               `json:"id"`
	WorkspaceID        string               `json:"workspaceId"`
	State              string               `json:"state"`
	CurrentStepID      string               `json:"currentStepId,omitempty"`
	CurrentStepKind    string               `json:"currentStepKind,omitempty"`
	CurrentStepState   string               `json:"currentStepState,omitempty"`
	DiagnosticLineage  string               `json:"diagnosticLineage,omitempty"`
	StepCount          int                  `json:"stepCount"`
	DecisionCount      int                  `json:"decisionCount"`
	ApprovalCount      int                  `json:"approvalCount"`
	BlockingTicketID   string               `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string               `json:"blockingApprovalId,omitempty"`
	FailureReason      string               `json:"failureReason,omitempty"`
	ConvergenceStatus  *ConvergenceStatus   `json:"convergenceStatus,omitempty"`
	Acceptance         AcceptanceProjection `json:"acceptance"`
}

// ConvergenceStatus preserves bounded Runtime convergence observations for
// display. It deliberately contains no transition or next-action suggestion.
type ConvergenceStatus struct {
	WorkspaceID                   string   `json:"workspaceId,omitempty"`
	RunID                         string   `json:"runId,omitempty"`
	RunState                      string   `json:"runState,omitempty"`
	CurrentSliceID                string   `json:"currentSliceId,omitempty"`
	CurrentFocusID                string   `json:"currentFocusId,omitempty"`
	ConvergenceState              string   `json:"convergenceState,omitempty"`
	CurrentGap                    string   `json:"currentGap,omitempty"`
	MissingEvidenceKinds          []string `json:"missingEvidenceKinds,omitempty"`
	AcceptedEvidenceCount         int      `json:"acceptedEvidenceCount,omitempty"`
	RejectedEvidenceCount         int      `json:"rejectedEvidenceCount,omitempty"`
	ClosedCoverageItemCount       int      `json:"closedCoverageItemCount,omitempty"`
	RemainingCoverageItemCount    int      `json:"remainingCoverageItemCount,omitempty"`
	AssessmentDisposition         string   `json:"assessmentDisposition,omitempty"`
	RepairAttempts                int      `json:"repairAttempts,omitempty"`
	MaxRepairAttempts             int      `json:"maxRepairAttempts,omitempty"`
	RepeatedFailureSignatureCount int      `json:"repeatedFailureSignatureCount,omitempty"`
	LatestAcceptedEvidenceRef     string   `json:"latestAcceptedEvidenceRef,omitempty"`
	NextActivationRequired        bool     `json:"nextActivationRequired,omitempty"`
	GapSummary                    string   `json:"gapSummary,omitempty"`
	LatestEffectiveProgressAt     string   `json:"latestEffectiveProgressAt,omitempty"`
	LastProgressAt                string   `json:"lastProgressAt,omitempty"`
	DecisionCause                 string   `json:"decisionCause,omitempty"`
	DecisionEligibilityDenied     bool     `json:"decisionEligibilityDenied,omitempty"`
	DecisionNotEligibleReason     string   `json:"decisionNotEligibleReason,omitempty"`
	DecisionNotEligibleStepID     string   `json:"decisionNotEligibleStepId,omitempty"`
	Stalled                       bool     `json:"stalled,omitempty"`
	IneffectiveRetry              bool     `json:"ineffectiveRetry,omitempty"`
}

func CloneConvergenceStatus(item *ConvergenceStatus) *ConvergenceStatus {
	if item == nil {
		return nil
	}
	ret := *item
	ret.MissingEvidenceKinds = append([]string(nil), item.MissingEvidenceKinds...)
	return &ret
}

func CloneRunStatus(item RunStatus) RunStatus {
	item.ConvergenceStatus = CloneConvergenceStatus(item.ConvergenceStatus)
	item.Acceptance = CloneAcceptanceProjection(item.Acceptance)
	return item
}
