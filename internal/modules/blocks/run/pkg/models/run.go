package run

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type ReplacementTerminationIntentState string

const (
	ReplacementTerminationIntentPending      ReplacementTerminationIntentState = "pending"
	ReplacementTerminationIntentAcknowledged ReplacementTerminationIntentState = "acknowledged"
)

// ReplacementTerminationIntent is Run-owned durable delivery state for the
// supersede request that must leave the Workspace -> Run synchronous command
// lane before it can enter Orchestrator SM-4. It records delivery progress but
// never selects an Orchestrator or predecessor lifecycle state.
type ReplacementTerminationIntent struct {
	RequestID            string
	PredecessorRunID     common.RunID
	Kind                 runtimecontract.RuntimeTerminationKind
	Reason               string
	State                ReplacementTerminationIntentState
	AttemptCount         uint64
	LastAttemptAt        string
	LastError            string
	TerminalDeliveryID   string
	RuntimeStateRevision uint64
	AcknowledgedAt       string
}

func (i ReplacementTerminationIntent) ValidFor(successor *Run) bool {
	if successor == nil || strings.TrimSpace(i.RequestID) == "" || i.PredecessorRunID == "" || i.PredecessorRunID == successor.ID || !runtimecontract.ValidRuntimeTerminationKind(i.Kind) || strings.TrimSpace(i.Reason) == "" {
		return false
	}
	switch i.State {
	case ReplacementTerminationIntentPending:
		return strings.TrimSpace(i.TerminalDeliveryID) == "" && i.RuntimeStateRevision == 0 && strings.TrimSpace(i.AcknowledgedAt) == ""
	case ReplacementTerminationIntentAcknowledged:
		return strings.TrimSpace(i.TerminalDeliveryID) != "" && i.RuntimeStateRevision > 0 && strings.TrimSpace(i.AcknowledgedAt) != ""
	default:
		return false
	}
}

type Run struct {
	// Revision is the owner-local persistence revision. Lifecycle and Runtime
	// aggregate writes compare it before replacing the record.
	Revision           uint64
	ID                 common.RunID
	WorkspaceID        common.WorkspaceID
	GoalID             common.GoalID
	DoDID              common.DoDID
	Lineage            RunLineage
	DefinitionRevision string
	InitialStepKind    string
	RuntimeBootstrap   *runtimecontract.RuntimeBootstrap
	// ActivationRequestID is the Workspace-scoped idempotency identity for a
	// Runtime replacement activation. It is owned by Run and is never used
	// as an authority/coverage/dispatch value.
	ActivationRequestID string
	// ReplacementTerminationIntent is persisted atomically with a restart
	// successor. A Run-owned background sender changes only this delivery
	// ledger after Orchestrator returns a concrete SM-4 acknowledgement.
	ReplacementTerminationIntent *ReplacementTerminationIntent
	// ReconciliationRequestID is a Run-owner continuation identity.  It is
	// emitted with the pending->active state fact so Orchestrator cannot lose a
	// resume reconciliation when the ordinary state notification races it.
	ReconciliationRequestID string
	// ReconciliationScope is immutable activation metadata owned by Run. Empty
	// means current_run for legacy/in-process fixtures; restart-created Runs
	// seal workspace_full so the first Orchestrator drive cannot inherit
	// predecessor coverage credit.
	ReconciliationScope runtimecontract.RuntimeCWIReconciliationScope
	RuntimeState        *runtimecontract.RuntimeStateRecord
	// RuntimeActivationStatus is the Run owner's administrative resume status.
	// It never contains decoded aggregate authority or a requested target state.
	RuntimeActivationStatus *RuntimeActivationStatus
	State                   common.RunState
	Priority                int
	BlockingTicketID        common.TicketID
	BlockingApprovalID      common.ApprovalID
	FailureReason           string
	CreatedAt               string
	StartedAt               string
	EndedAt                 string
	// TerminalDelivery is the Run owner's immutable SM-1 terminal fence.
	TerminalDelivery *runtimecontract.RuntimeTerminalDeliveryIdentity
}

type RunLineage struct {
	PredecessorRunID    common.RunID
	PredecessorResultID common.ResultID
	SuccessorRunID      common.RunID
	LineageState        string
}
