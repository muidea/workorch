package events

import (
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// CLIRepairLoopInfo is the map-free EventHub representation of a projected
// repair loop. Payload values preserve the complete structured diagnostics.
type CLIRepairLoopInfo struct {
	State                    string
	AttemptCount             int
	LastAttempt              int
	LastRepairStepID         string
	LastFailureStepID        string
	LastFailureClass         string
	LastFailureSignature     string
	LastFailureTargetScope   string
	LastFailureAt            string
	StructuredFailureSignals workorcheventpayload.Payload
	RepairInputCompaction    workorcheventpayload.Payload
	LastRepairInputBytes     int
	RepeatedFailureReason    string
	MissingDoDItems          []workorcheventpayload.Payload
	RecentAttempts           []workorcheventpayload.Payload
	Escalated                bool
	EscalationReason         string
	EscalationTicketID       string
	EvidenceRefs             []string
}

type CLITaskProgressInfo struct {
	WorkspaceID        string
	RunID              string
	State              string
	Summary            string
	CurrentStepID      string
	CurrentStepKind    string
	CurrentStepState   string
	DiagnosticLineage  string
	CurrentMilestone   string
	BlockingTicketID   string
	BlockingApprovalID string
	Milestones         []projectioncontract.ProgressMilestone
	Items              []projectioncontract.ProgressItem
	Blockers           []projectioncontract.ProgressBlocker
	Recovery           projectioncontract.RecoveryPlan
	RepairLoop         CLIRepairLoopInfo
	AssessmentOutcome  projectioncontract.AssessmentOutcome
	EvidenceRefs       []string
	Runtime            *projectioncontract.RuntimeProgress
}
