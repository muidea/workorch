package run

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type ReplacementIntentState string

const (
	ReplacementIntentPending          ReplacementIntentState = "pending"
	ReplacementIntentSuccessorCreated ReplacementIntentState = "successor_created"
	ReplacementIntentCompleted        ReplacementIntentState = "completed"
)

// ReplacementIntent is the predecessor Run's durable restart intent. It
// contains immutable successor activation input and delivery progress, but no
// Workspace target state or Orchestrator reducer instruction.
type ReplacementIntent struct {
	RequestID            string
	WorkspaceID          common.WorkspaceID
	GoalID               common.GoalID
	DoDID                common.DoDID
	DefinitionRevision   string
	Bootstrap            *runtimecontract.RuntimeBootstrap
	Priority             int
	ReconciliationScope  runtimecontract.RuntimeCWIReconciliationScope
	Kind                 runtimecontract.RuntimeTerminationKind
	Reason               string
	State                ReplacementIntentState
	AttemptCount         uint64
	LastAttemptAt        string
	LastError            string
	TerminalDeliveryID   string
	RuntimeStateRevision uint64
	SuccessorRunID       common.RunID
	AcknowledgedAt       string
}

func (i ReplacementIntent) ValidFor(predecessor *Run) bool {
	if predecessor == nil || strings.TrimSpace(i.RequestID) == "" || i.WorkspaceID == "" || i.WorkspaceID != predecessor.WorkspaceID || i.GoalID == "" || i.DoDID == "" || strings.TrimSpace(i.DefinitionRevision) == "" || i.Bootstrap == nil || !runtimecontract.ValidRuntimeTerminationKind(i.Kind) || strings.TrimSpace(i.Reason) == "" {
		return false
	}
	if i.Kind != runtimecontract.RuntimeTerminationSuperseded || i.ReconciliationScope != runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull {
		return false
	}
	definition := i.Bootstrap.Definition.Normalized()
	if definition.GoalID != i.GoalID || definition.DoDID != i.DoDID || definition.Revision != i.DefinitionRevision {
		return false
	}
	switch i.State {
	case ReplacementIntentPending:
		return i.SuccessorRunID == "" && strings.TrimSpace(i.TerminalDeliveryID) == "" && i.RuntimeStateRevision == 0 && strings.TrimSpace(i.AcknowledgedAt) == ""
	case ReplacementIntentSuccessorCreated:
		return i.SuccessorRunID != "" && strings.TrimSpace(i.TerminalDeliveryID) != "" && i.RuntimeStateRevision == 0 && strings.TrimSpace(i.AcknowledgedAt) == ""
	case ReplacementIntentCompleted:
		return i.SuccessorRunID != "" && strings.TrimSpace(i.TerminalDeliveryID) != "" && i.RuntimeStateRevision > 0 && strings.TrimSpace(i.AcknowledgedAt) != ""
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
	// ReplacementIntent is persisted on the predecessor before SM-4 starts.
	// The successor is materialized only after the final Run delivery proves
	// that Result and Workspace acknowledgements were already persisted.
	ReplacementIntent *ReplacementIntent
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
	// RuntimeLifecycleFence is Orchestrator-authored opaque identity persisted
	// beside RuntimeState. Run compares it without decoding aggregate content.
	RuntimeLifecycleFence *runtimecontract.RuntimeOpaqueLifecycleFence
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

func CloneRun(item *Run) *Run {
	if item == nil {
		return nil
	}
	cloned := *item
	if item.RuntimeBootstrap != nil {
		bootstrap := item.RuntimeBootstrap.Clone()
		cloned.RuntimeBootstrap = &bootstrap
	}
	if item.RuntimeState != nil {
		state := item.RuntimeState.Clone()
		cloned.RuntimeState = &state
	}
	if item.RuntimeLifecycleFence != nil {
		fence := item.RuntimeLifecycleFence.Normalized()
		cloned.RuntimeLifecycleFence = &fence
	}
	if item.ReplacementIntent != nil {
		intent := *item.ReplacementIntent
		if item.ReplacementIntent.Bootstrap != nil {
			bootstrap := item.ReplacementIntent.Bootstrap.Clone()
			intent.Bootstrap = &bootstrap
		}
		cloned.ReplacementIntent = &intent
	}
	cloned.RuntimeActivationStatus = item.RuntimeActivationStatus.Clone()
	if item.TerminalDelivery != nil {
		identity := *item.TerminalDelivery
		cloned.TerminalDelivery = &identity
	}
	return &cloned
}

func CloneRuns(items []*Run) []*Run {
	if items == nil {
		return nil
	}
	ret := make([]*Run, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneRun(item))
	}
	return ret
}
