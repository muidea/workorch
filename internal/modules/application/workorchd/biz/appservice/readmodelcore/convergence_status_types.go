package readmodelcore

type ConvergenceStatus struct {
	WorkspaceID                 string   `json:"workspaceId,omitempty"`
	RunID                       string   `json:"runId,omitempty"`
	RunState                    string   `json:"runState,omitempty"`
	CurrentFocusID              string   `json:"currentFocusId,omitempty"`
	NextActionKind              string   `json:"nextActionKind,omitempty"`
	CurrentGap                  string   `json:"currentGap,omitempty"`
	MissingEvidenceKinds        []string `json:"missingEvidenceKinds,omitempty"`
	AcceptedEvidenceCount       int      `json:"acceptedEvidenceCount,omitempty"`
	RejectedEvidenceCount       int      `json:"rejectedEvidenceCount,omitempty"`
	LatestEffectiveProgressAt   string   `json:"latestEffectiveProgressAt,omitempty"`
	LatestContractViolationRule string   `json:"latestContractViolationRule,omitempty"`
	LatestRecovery              string   `json:"latestRecovery,omitempty"`
	Stalled                     bool     `json:"stalled,omitempty"`
	IneffectiveRetry            bool     `json:"ineffectiveRetry,omitempty"`
}
