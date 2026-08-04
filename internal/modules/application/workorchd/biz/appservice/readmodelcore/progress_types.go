package readmodelcore

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

type TaskProgress struct {
	WorkspaceID        string                                 `json:"workspaceId"`
	RunID              string                                 `json:"runId,omitempty"`
	State              string                                 `json:"state"`
	Summary            string                                 `json:"summary,omitempty"`
	CurrentStepID      string                                 `json:"currentStepId,omitempty"`
	CurrentStepKind    string                                 `json:"currentStepKind,omitempty"`
	CurrentStepState   string                                 `json:"currentStepState,omitempty"`
	CurrentActionLabel string                                 `json:"currentActionLabel,omitempty"`
	DiagnosticLineage  string                                 `json:"diagnosticLineage,omitempty"`
	CurrentMilestone   string                                 `json:"currentMilestone,omitempty"`
	BlockingTicketID   string                                 `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string                                 `json:"blockingApprovalId,omitempty"`
	Milestones         []projectioncontract.ProgressMilestone `json:"milestones,omitempty"`
	Items              []ProgressItem                         `json:"items,omitempty"`
	Blockers           []ProgressBlocker                      `json:"blockers,omitempty"`
	Recovery           projectioncontract.RecoveryPlan        `json:"recovery"`
	RepairLoop         RepairLoop                             `json:"repairLoop"`
	AssessmentOutcome  projectioncontract.AssessmentOutcome   `json:"assessmentOutcome,omitempty"`
	EvidenceRefs       []string                               `json:"evidenceRefs,omitempty"`
	Runtime            *projectioncontract.RuntimeProgress    `json:"runtime,omitempty"`
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
