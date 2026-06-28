package readmodelcore

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
}

type AssessmentOutcome struct {
	State                       string `json:"state,omitempty"`
	AcceptedSlices              int    `json:"acceptedSlices,omitempty"`
	AcceptedResultCount         int    `json:"acceptedResultCount,omitempty"`
	RejectedGlobalAssessments   int    `json:"rejectedGlobalAssessments,omitempty"`
	SatisfiedCoverageCount      int    `json:"satisfiedCoverageCount,omitempty"`
	RemainingCoverageCount      int    `json:"remainingCoverageCount,omitempty"`
	NewlySatisfiedCoverageCount int    `json:"newlySatisfiedCoverageCount,omitempty"`
	LatestAcceptedResultID      string `json:"latestAcceptedResultId,omitempty"`
	LatestRejectedResultID      string `json:"latestRejectedResultId,omitempty"`
	LatestRejectedReason        string `json:"latestRejectedReason,omitempty"`
}

type ProgressMilestone struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	State        string   `json:"state"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
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
