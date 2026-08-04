package workorchprojectioncontract

type TaskProgress struct {
	WorkspaceID        string              `json:"workspaceId"`
	RunID              string              `json:"runId,omitempty"`
	State              string              `json:"state"`
	Summary            string              `json:"summary,omitempty"`
	CurrentStepID      string              `json:"currentStepId,omitempty"`
	CurrentStepKind    string              `json:"currentStepKind,omitempty"`
	CurrentStepState   string              `json:"currentStepState,omitempty"`
	CurrentActionLabel string              `json:"currentActionLabel,omitempty"`
	DiagnosticLineage  string              `json:"diagnosticLineage,omitempty"`
	CurrentMilestone   string              `json:"currentMilestone,omitempty"`
	BlockingTicketID   string              `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string              `json:"blockingApprovalId,omitempty"`
	Milestones         []ProgressMilestone `json:"milestones,omitempty"`
	Items              []ProgressItem      `json:"items,omitempty"`
	Blockers           []ProgressBlocker   `json:"blockers,omitempty"`
	Recovery           RecoveryPlan        `json:"recovery"`
	RepairLoop         RepairLoop          `json:"repairLoop"`
	AssessmentOutcome  AssessmentOutcome   `json:"assessmentOutcome,omitempty"`
	EvidenceRefs       []string            `json:"evidenceRefs,omitempty"`
	Runtime            *RuntimeProgress    `json:"runtime,omitempty"`
}

type ProgressItem struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	State        string   `json:"state"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
}

type ProgressBlocker struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	State       string `json:"state"`
	Reason      string `json:"reason,omitempty"`
	EvidenceRef string `json:"evidenceRef,omitempty"`
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
