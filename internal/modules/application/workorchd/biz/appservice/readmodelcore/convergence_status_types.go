package readmodelcore

type ConvergenceStatus struct {
	WorkspaceID                   string   `json:"workspaceId,omitempty"`
	RunID                         string   `json:"runId,omitempty"`
	RunState                      string   `json:"runState,omitempty"`
	CurrentSliceID                string   `json:"currentSliceId,omitempty"`
	CurrentFocusID                string   `json:"currentFocusId,omitempty"`
	NextActionKind                string   `json:"nextActionKind,omitempty"`
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
