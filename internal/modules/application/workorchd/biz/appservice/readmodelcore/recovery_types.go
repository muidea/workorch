package readmodelcore

type RecoveryPlan struct {
	Mode               string `json:"mode"`
	NextAction         string `json:"nextAction,omitempty"`
	RunID              string `json:"runId,omitempty"`
	StepID             string `json:"stepId,omitempty"`
	BlockingTicketID   string `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string `json:"blockingApprovalId,omitempty"`
	CheckpointID       string `json:"checkpointId,omitempty"`
	FailureClass       string `json:"failureClass,omitempty"`
	FailureSignature   string `json:"failureSignature,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

type RepairLoop struct {
	State                    string           `json:"state"`
	AttemptCount             int              `json:"attemptCount"`
	LastAttempt              int              `json:"lastAttempt,omitempty"`
	LastRepairStepID         string           `json:"lastRepairStepId,omitempty"`
	LastFailureStepID        string           `json:"lastFailureStepId,omitempty"`
	LastFailureClass         string           `json:"lastFailureClass,omitempty"`
	LastFailureSignature     string           `json:"lastFailureSignature,omitempty"`
	LastFailureTargetScope   string           `json:"lastFailureTargetScope,omitempty"`
	LastFailureAt            string           `json:"lastFailureAt,omitempty"`
	StructuredFailureSignals map[string]any   `json:"structuredFailureSignals,omitempty"`
	RepairInputCompaction    map[string]any   `json:"repairInputCompaction,omitempty"`
	LastRepairInputBytes     int              `json:"lastRepairInputBytes,omitempty"`
	RepeatedFailureReason    string           `json:"repeatedFailureReason,omitempty"`
	MissingDoDItems          []map[string]any `json:"missingDoDItems,omitempty"`
	RecentAttempts           []map[string]any `json:"recentAttempts,omitempty"`
	RecommendedNextSteps     []string         `json:"recommendedNextSteps,omitempty"`
	NextAction               string           `json:"nextAction,omitempty"`
	Escalated                bool             `json:"escalated"`
	EscalationReason         string           `json:"escalationReason,omitempty"`
	EscalationTicketID       string           `json:"escalationTicketId,omitempty"`
	EvidenceRefs             []string         `json:"evidenceRefs,omitempty"`
}
