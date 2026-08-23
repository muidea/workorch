package run

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type ReplacementIntentState string

const (
	ReplacementIntentPending                         ReplacementIntentState = "pending"
	ReplacementIntentPendingClarificationResolution  ReplacementIntentState = "pending_clarification_resolution"
	ReplacementIntentClarificationResolutionBound    ReplacementIntentState = "clarification_resolution_bound"
	ReplacementIntentClarificationResolutionRejected ReplacementIntentState = "clarification_resolution_rejected"
	ReplacementIntentTerminationAcknowledged         ReplacementIntentState = "termination_acknowledged"
	ReplacementIntentSuccessorCreated                ReplacementIntentState = "successor_created"
	ReplacementIntentCompleted                       ReplacementIntentState = "completed"
)

type ReplacementRecoveryStage string

const (
	ReplacementRecoveryClarificationResolution   ReplacementRecoveryStage = "clarification_resolution"
	ReplacementRecoveryAdministrativeTermination ReplacementRecoveryStage = "administrative_termination"
	ReplacementRecoverySuccessorMaterialization  ReplacementRecoveryStage = "successor_materialization"
)

type ReplacementRecoveryState string

const (
	ReplacementRecoveryScheduled ReplacementRecoveryState = "scheduled"
	ReplacementRecoveryRunning   ReplacementRecoveryState = "running"
	ReplacementRecoveryExhausted ReplacementRecoveryState = "exhausted"
	ReplacementRecoveryCompleted ReplacementRecoveryState = "completed"
)

// ReplacementRecoveryLease is the Run-owned durable successor for one
// replacement transaction. Process-local tasks may wake its exact generation,
// but only the replacement private reducer may claim, renew, advance or close
// the lease.
type ReplacementRecoveryLease struct {
	RequestID   string
	IntentHash  string
	Stage       ReplacementRecoveryStage
	State       ReplacementRecoveryState
	Generation  uint64
	Attempt     uint64
	MaxAttempts uint64
	DueAt       string
	LastError   string
}

// ReplacementIntent is the predecessor Run's durable restart intent. It
// contains immutable successor activation input and delivery progress, but no
// Workspace target state or Orchestrator reducer instruction.
type ReplacementIntent struct {
	RequestID                          string
	WorkspaceID                        common.WorkspaceID
	GoalID                             common.GoalID
	DoDID                              common.DoDID
	DefinitionRevision                 string
	StartupInputHash                   string
	ReplacementIntentID                string
	ReplacementIntentHash              string
	ClarificationTicketID              common.TicketID
	PredecessorStartupInputHash        string
	SuccessorStartupInputHash          string
	ClarificationResolutionID          common.ResolutionID
	ClarificationResolutionBindingHash string
	ClarificationConflictKind          string
	ConflictingReplacementIntentID     string
	ConflictingReplacementIntentHash   string
	Bootstrap                          *runtimecontract.RuntimeBootstrap
	ReconciliationScope                runtimecontract.RuntimeCWIReconciliationScope
	Kind                               runtimecontract.RuntimeTerminationKind
	Reason                             string
	State                              ReplacementIntentState
	Recovery                           ReplacementRecoveryLease
	TerminalDeliveryID                 string
	RuntimeStateRevision               uint64
	SuccessorRunID                     common.RunID
	AcknowledgedAt                     string
	// legacyIdentityHashSlot is populated only while decoding a persisted v1
	// clarification intent whose immutable hash included the retired caller
	// priority slot. It is never accepted from a command, projected, or used to
	// order execution. Custom JSON round-tripping preserves it solely so an
	// existing Decision binding keeps the same immutable identity across reloads.
	legacyIdentityHashSlot string
}

type replacementIntentJSON ReplacementIntent

func (i *ReplacementIntent) UnmarshalJSON(data []byte) error {
	if i == nil {
		return fmt.Errorf("replacement intent decode target is nil")
	}
	var wire struct {
		replacementIntentJSON
		Priority *int `json:"Priority"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	decoded := ReplacementIntent(wire.replacementIntentJSON)
	if wire.Priority != nil && *wire.Priority != 0 && strings.TrimSpace(decoded.ReplacementIntentHash) != "" {
		decoded.legacyIdentityHashSlot = strconv.Itoa(*wire.Priority)
		if decoded.ReplacementIntentHash != stableReplacementIntentHashWithSlot(decoded, decoded.legacyIdentityHashSlot) {
			return fmt.Errorf("persisted replacement intent legacy identity hash does not match its sealed payload")
		}
	}
	*i = decoded
	return nil
}

func (i ReplacementIntent) MarshalJSON() ([]byte, error) {
	wire := struct {
		replacementIntentJSON
		Priority *int `json:"Priority,omitempty"`
	}{replacementIntentJSON: replacementIntentJSON(i)}
	if i.legacyIdentityHashSlot != "" {
		legacyValue, err := strconv.Atoi(i.legacyIdentityHashSlot)
		if err != nil || legacyValue == 0 {
			return nil, fmt.Errorf("replacement intent legacy identity hash slot is invalid")
		}
		if strings.TrimSpace(i.ReplacementIntentHash) == "" || i.ReplacementIntentHash != stableReplacementIntentHashWithSlot(i, i.legacyIdentityHashSlot) {
			return nil, fmt.Errorf("replacement intent legacy identity hash does not match its sealed payload")
		}
		wire.Priority = &legacyValue
	}
	return json.Marshal(wire)
}

func (i ReplacementIntent) ValidFor(predecessor *Run) bool {
	if predecessor == nil || strings.TrimSpace(i.RequestID) == "" || i.WorkspaceID == "" || i.WorkspaceID != predecessor.WorkspaceID || i.GoalID == "" || i.DoDID == "" || strings.TrimSpace(i.DefinitionRevision) == "" || strings.TrimSpace(i.StartupInputHash) == "" || i.Bootstrap == nil || !runtimecontract.ValidRuntimeTerminationKind(i.Kind) || strings.TrimSpace(i.Reason) == "" {
		return false
	}
	if i.Kind != runtimecontract.RuntimeTerminationSuperseded || i.ReconciliationScope != runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull {
		return false
	}
	definition := i.Bootstrap.Definition.Normalized()
	if definition.GoalID != i.GoalID || definition.DoDID != i.DoDID || definition.Revision != i.DefinitionRevision || definition.StartupInputHash != i.StartupInputHash {
		return false
	}
	clarification := i.ClarificationTicketID != ""
	clarificationBound := !clarification || (i.ClarificationResolutionID != "" && i.ClarificationResolutionBindingHash != "" && i.ClarificationConflictKind == "")
	if clarification {
		if strings.TrimSpace(i.ReplacementIntentID) == "" || strings.TrimSpace(i.ReplacementIntentHash) == "" || i.ReplacementIntentHash != StableReplacementIntentHash(i) || strings.TrimSpace(i.PredecessorStartupInputHash) == "" || strings.TrimSpace(i.SuccessorStartupInputHash) == "" || i.SuccessorStartupInputHash != i.StartupInputHash || i.PredecessorStartupInputHash == i.SuccessorStartupInputHash || strings.TrimSpace(predecessor.DefinitionRevision) == "" || strings.TrimSpace(i.DefinitionRevision) == strings.TrimSpace(predecessor.DefinitionRevision) {
			return false
		}
	} else if i.ReplacementIntentID != "" || i.ReplacementIntentHash != "" || i.PredecessorStartupInputHash != "" || i.SuccessorStartupInputHash != "" || i.ClarificationResolutionID != "" || i.ClarificationResolutionBindingHash != "" || i.ClarificationConflictKind != "" || i.ConflictingReplacementIntentID != "" || i.ConflictingReplacementIntentHash != "" {
		return false
	}
	if !i.Recovery.validFor(i) {
		return false
	}
	switch i.State {
	case ReplacementIntentPending:
		return !clarification && i.Recovery.Stage == ReplacementRecoveryAdministrativeTermination && replacementRecoveryOpen(i.Recovery.State) && i.SuccessorRunID == "" && strings.TrimSpace(i.TerminalDeliveryID) == "" && i.RuntimeStateRevision == 0 && strings.TrimSpace(i.AcknowledgedAt) == ""
	case ReplacementIntentPendingClarificationResolution:
		return clarification && i.Recovery.Stage == ReplacementRecoveryClarificationResolution && replacementRecoveryOpen(i.Recovery.State) && predecessor.State == common.RunStateBlockedOnDecision && predecessor.BlockingTicketID == i.ClarificationTicketID && i.ClarificationResolutionID == "" && i.ClarificationResolutionBindingHash == "" && i.ClarificationConflictKind == "" && i.ConflictingReplacementIntentID == "" && i.ConflictingReplacementIntentHash == "" && i.SuccessorRunID == "" && strings.TrimSpace(i.TerminalDeliveryID) == "" && i.RuntimeStateRevision == 0 && strings.TrimSpace(i.AcknowledgedAt) == ""
	case ReplacementIntentClarificationResolutionBound:
		return clarificationBound && clarification && i.Recovery.Stage == ReplacementRecoveryAdministrativeTermination && replacementRecoveryOpen(i.Recovery.State) && i.ConflictingReplacementIntentID == "" && i.ConflictingReplacementIntentHash == "" && i.SuccessorRunID == "" && strings.TrimSpace(i.TerminalDeliveryID) == "" && i.RuntimeStateRevision == 0 && strings.TrimSpace(i.AcknowledgedAt) == ""
	case ReplacementIntentClarificationResolutionRejected:
		if !clarification || i.ClarificationResolutionID != "" || i.ClarificationResolutionBindingHash == "" || i.SuccessorRunID != "" || strings.TrimSpace(i.TerminalDeliveryID) != "" || i.RuntimeStateRevision != 0 || strings.TrimSpace(i.AcknowledgedAt) != "" {
			return false
		}
		if i.Recovery.Stage != ReplacementRecoveryClarificationResolution || i.Recovery.State != ReplacementRecoveryCompleted {
			return false
		}
		switch i.ClarificationConflictKind {
		case "scope_mismatch":
			return i.ConflictingReplacementIntentID == "" && i.ConflictingReplacementIntentHash == ""
		case "ticket_already_bound":
			return i.ConflictingReplacementIntentID != "" && i.ConflictingReplacementIntentHash != ""
		default:
			return false
		}
	case ReplacementIntentTerminationAcknowledged:
		return clarificationBound && predecessor.State == common.RunStateSuperseded && predecessor.TerminalDelivery != nil && predecessor.TerminalDelivery.TerminalDeliveryID == i.TerminalDeliveryID && i.Recovery.Stage == ReplacementRecoverySuccessorMaterialization && replacementRecoveryOpen(i.Recovery.State) && i.SuccessorRunID == "" && strings.TrimSpace(i.TerminalDeliveryID) != "" && i.RuntimeStateRevision > 0 && strings.TrimSpace(i.AcknowledgedAt) == ""
	case ReplacementIntentSuccessorCreated:
		return clarificationBound && predecessor.State == common.RunStateSuperseded && predecessor.TerminalDelivery != nil && predecessor.TerminalDelivery.TerminalDeliveryID == i.TerminalDeliveryID && i.Recovery.Stage == ReplacementRecoverySuccessorMaterialization && replacementRecoveryOpen(i.Recovery.State) && i.SuccessorRunID != "" && strings.TrimSpace(i.TerminalDeliveryID) != "" && i.RuntimeStateRevision > 0 && strings.TrimSpace(i.AcknowledgedAt) == ""
	case ReplacementIntentCompleted:
		return clarificationBound && predecessor.State == common.RunStateSuperseded && predecessor.TerminalDelivery != nil && predecessor.TerminalDelivery.TerminalDeliveryID == i.TerminalDeliveryID && i.Recovery.Stage == ReplacementRecoverySuccessorMaterialization && i.Recovery.State == ReplacementRecoveryCompleted && i.SuccessorRunID != "" && strings.TrimSpace(i.TerminalDeliveryID) != "" && i.RuntimeStateRevision > 0 && strings.TrimSpace(i.AcknowledgedAt) != "" && predecessor.Lineage.SuccessorRunID == i.SuccessorRunID
	default:
		return false
	}
}

func replacementRecoveryOpen(state ReplacementRecoveryState) bool {
	return state == ReplacementRecoveryScheduled || state == ReplacementRecoveryRunning || state == ReplacementRecoveryExhausted
}

func (r ReplacementRecoveryLease) validFor(intent ReplacementIntent) bool {
	r.RequestID = strings.TrimSpace(r.RequestID)
	r.IntentHash = strings.TrimSpace(r.IntentHash)
	r.DueAt = strings.TrimSpace(r.DueAt)
	r.LastError = strings.TrimSpace(r.LastError)
	if r.RequestID == "" || r.RequestID != intent.RequestID || r.IntentHash == "" || r.IntentHash != StableReplacementIntentHash(intent) || r.Generation == 0 || r.MaxAttempts == 0 || r.Attempt > r.MaxAttempts {
		return false
	}
	switch r.Stage {
	case ReplacementRecoveryClarificationResolution, ReplacementRecoveryAdministrativeTermination, ReplacementRecoverySuccessorMaterialization:
	default:
		return false
	}
	switch r.State {
	case ReplacementRecoveryScheduled:
		return r.DueAt != "" && r.Attempt < r.MaxAttempts
	case ReplacementRecoveryRunning:
		return r.DueAt == "" && r.Attempt < r.MaxAttempts && r.LastError == ""
	case ReplacementRecoveryExhausted:
		return r.DueAt == "" && r.Attempt == r.MaxAttempts && r.LastError != ""
	case ReplacementRecoveryCompleted:
		return r.DueAt == "" && r.LastError == ""
	default:
		return false
	}
}

// StableReplacementIntentHash seals immutable restart identity only. Delivery,
// acknowledgement and recovery progress deliberately do not participate.
func StableReplacementIntentHash(i ReplacementIntent) string {
	legacySlot := "0"
	if i.legacyIdentityHashSlot != "" {
		legacySlot = i.legacyIdentityHashSlot
	}
	return stableReplacementIntentHashWithSlot(i, legacySlot)
}

func stableReplacementIntentHashWithSlot(i ReplacementIntent, legacySlot string) string {
	if i.Bootstrap == nil {
		return ""
	}
	h := sha256.New()
	for _, value := range []string{
		"workorch-replacement-intent-v1", i.RequestID, string(i.WorkspaceID), string(i.GoalID), string(i.DoDID),
		strings.TrimSpace(i.DefinitionRevision), strings.TrimSpace(i.StartupInputHash), strings.TrimSpace(i.ReplacementIntentID),
		string(i.ClarificationTicketID), strings.TrimSpace(i.PredecessorStartupInputHash), strings.TrimSpace(i.SuccessorStartupInputHash),
		legacySlot, // persisted v1 identity slot; new intents always seal the neutral value "0"
		string(i.ReconciliationScope), string(i.Kind), strings.TrimSpace(i.Reason),
		runtimecontract.StableRuntimeBootstrapHash(*i.Bootstrap),
	} {
		_, _ = fmt.Fprintf(h, "%08x%s", len(value), value)
	}
	return "replacement-intent-sha256:" + hex.EncodeToString(h.Sum(nil))
}

// EquivalentReplacementIntentHash accepts an exact current hash or a
// canonical neutral-slot hash for an already validated persisted v1 intent.
// This keeps duplicate Restart idempotent after removing caller priority
// without allowing that retired value to influence any new intent.
func EquivalentReplacementIntentHash(current, requested ReplacementIntent) bool {
	currentHash := strings.TrimSpace(current.ReplacementIntentHash)
	requestedHash := strings.TrimSpace(requested.ReplacementIntentHash)
	if currentHash == "" || requestedHash == "" {
		return currentHash == requestedHash
	}
	if currentHash != StableReplacementIntentHash(current) || requestedHash != StableReplacementIntentHash(requested) {
		return false
	}
	if currentHash == requestedHash {
		return true
	}
	if current.legacyIdentityHashSlot == "" {
		return false
	}
	current.legacyIdentityHashSlot = ""
	return requestedHash == StableReplacementIntentHash(current)
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
	StartupInputHash   string
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
	// RetiredReplacementIntents preserves rejected clarification attempts when
	// a user submits a new explicit Restart. Entries are immutable audit
	// lineage; recovery and delivery only inspect ReplacementIntent.
	RetiredReplacementIntents []ReplacementIntent
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
		intent := CloneReplacementIntent(*item.ReplacementIntent)
		cloned.ReplacementIntent = &intent
	}
	if item.RetiredReplacementIntents != nil {
		cloned.RetiredReplacementIntents = make([]ReplacementIntent, len(item.RetiredReplacementIntents))
		for idx := range item.RetiredReplacementIntents {
			cloned.RetiredReplacementIntents[idx] = CloneReplacementIntent(item.RetiredReplacementIntents[idx])
		}
	}
	cloned.RuntimeActivationStatus = item.RuntimeActivationStatus.Clone()
	if item.TerminalDelivery != nil {
		identity := *item.TerminalDelivery
		cloned.TerminalDelivery = &identity
	}
	return &cloned
}

func CloneReplacementIntent(intent ReplacementIntent) ReplacementIntent {
	cloned := intent
	if intent.Bootstrap != nil {
		bootstrap := intent.Bootstrap.Clone()
		cloned.Bootstrap = &bootstrap
	}
	return cloned
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
