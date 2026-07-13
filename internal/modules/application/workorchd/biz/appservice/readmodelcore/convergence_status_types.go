package readmodelcore

type ConvergenceStatus struct {
	WorkspaceID               string   `json:"workspaceId,omitempty"`
	RunID                     string   `json:"runId,omitempty"`
	RunState                  string   `json:"runState,omitempty"`
	CurrentFocusID            string   `json:"currentFocusId,omitempty"`
	NextActionKind            string   `json:"nextActionKind,omitempty"`
	CurrentGap                string   `json:"currentGap,omitempty"`
	MissingEvidenceKinds      []string `json:"missingEvidenceKinds,omitempty"`
	AcceptedEvidenceCount     int      `json:"acceptedEvidenceCount,omitempty"`
	RejectedEvidenceCount     int      `json:"rejectedEvidenceCount,omitempty"`
	LatestEffectiveProgressAt string   `json:"latestEffectiveProgressAt,omitempty"`
	DecisionCause             string   `json:"decisionCause,omitempty"`
	DecisionEligibilityDenied bool     `json:"decisionEligibilityDenied,omitempty"`
	DecisionNotEligibleReason string   `json:"decisionNotEligibleReason,omitempty"`
	DecisionNotEligibleStepID string   `json:"decisionNotEligibleStepId,omitempty"`
	Stalled                   bool     `json:"stalled,omitempty"`
	IneffectiveRetry          bool     `json:"ineffectiveRetry,omitempty"`
}
