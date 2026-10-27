package workorchruntimecontract

import (
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeExecutionIntentKind identifies the bounded user activation scope. It
// is an immutable intent value, never a requested SM-1 state.
type RuntimeExecutionIntentKind string

const (
	RuntimeExecutionIntentResume  RuntimeExecutionIntentKind = "resume"
	RuntimeExecutionIntentRestart RuntimeExecutionIntentKind = "restart"
)

// RuntimeExecutionIntentOutcome is the factual admission outcome returned by
// Workspace and Run owners for a resume/restart intent. It is not a requested
// SM-1 state and carries no retry or transition instruction.
type RuntimeExecutionIntentOutcome string

const (
	RuntimeExecutionIntentReconciliationRequested RuntimeExecutionIntentOutcome = "reconciliation_requested"
	RuntimeExecutionIntentReplacementPending      RuntimeExecutionIntentOutcome = "replacement_pending"
	RuntimeExecutionIntentRestartRequired         RuntimeExecutionIntentOutcome = "restart_required"
	RuntimeExecutionIntentFreshActivationRequired RuntimeExecutionIntentOutcome = "fresh_activation_required"
	RuntimeExecutionIntentDefinitionChanged       RuntimeExecutionIntentOutcome = "definition_changed_requires_restart"
	RuntimeExecutionIntentGovernanceRequired      RuntimeExecutionIntentOutcome = "governance_required"
	RuntimeExecutionIntentFreshUnresolved         RuntimeExecutionIntentOutcome = "fresh_intent_unresolved"
)

// RuntimeExecutionIntentReason preserves the exact owner-authored factual
// cause without transporting a second free-form diagnostic authority.
type RuntimeExecutionIntentReason string

const (
	RuntimeExecutionIntentReasonWorkspaceActiveRunAbsent              RuntimeExecutionIntentReason = "workspace_active_run_absent"
	RuntimeExecutionIntentReasonSelectedRunAbsent                     RuntimeExecutionIntentReason = "selected_run_absent"
	RuntimeExecutionIntentReasonSelectedRunTerminal                   RuntimeExecutionIntentReason = "selected_run_terminal"
	RuntimeExecutionIntentReasonDefinitionChanged                     RuntimeExecutionIntentReason = "definition_changed"
	RuntimeExecutionIntentReasonRuntimeBootstrapMissing               RuntimeExecutionIntentReason = "runtime_bootstrap_missing"
	RuntimeExecutionIntentReasonCurrentRunContinuationAccepted        RuntimeExecutionIntentReason = "current_run_continuation_accepted"
	RuntimeExecutionIntentReasonPendingRunActivated                   RuntimeExecutionIntentReason = "pending_run_activated"
	RuntimeExecutionIntentReasonGovernancePending                     RuntimeExecutionIntentReason = "governance_pending"
	RuntimeExecutionIntentReasonRunNotResumable                       RuntimeExecutionIntentReason = "run_not_resumable"
	RuntimeExecutionIntentReasonDuplicateRestart                      RuntimeExecutionIntentReason = "duplicate_restart"
	RuntimeExecutionIntentReasonNewRunActivated                       RuntimeExecutionIntentReason = "new_run_activated"
	RuntimeExecutionIntentReasonPredecessorSupersedeRequested         RuntimeExecutionIntentReason = "predecessor_supersede_requested"
	RuntimeExecutionIntentReasonTerminalPredecessorSuccessorRequested RuntimeExecutionIntentReason = "terminal_predecessor_successor_requested"
	RuntimeExecutionIntentReasonDefinitionAdmissionMissing            RuntimeExecutionIntentReason = "definition_admission_missing"
	RuntimeExecutionIntentReasonDefinitionAdmissionMismatch           RuntimeExecutionIntentReason = "definition_admission_mismatch"
	RuntimeExecutionIntentReasonDefinitionClarificationPending        RuntimeExecutionIntentReason = "definition_clarification_pending"
	RuntimeExecutionIntentReasonFreshResultUnknown                    RuntimeExecutionIntentReason = "fresh_intent_result_unknown"
	RuntimeExecutionIntentReasonCreationFenceMissing                  RuntimeExecutionIntentReason = "preparation_creation_fence_missing"
)

// RuntimeExecutionIntentAdmission is the immutable, owner-neutral admission
// fact carried through the Run, Workspace, application and process boundaries.
// It reports what the state owner admitted; it never requests an SM-1
// transition or selects a subsequent Runtime action.
type RuntimeExecutionIntentAdmission struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	RequestID   string
	Kind        RuntimeExecutionIntentKind
	Outcome     RuntimeExecutionIntentOutcome
	Reason      RuntimeExecutionIntentReason
	// An unresolved original is not an accepted replacement or a rejection.
	// RequestID remains the current caller identity; these fields preserve the
	// blocking original even when a different fresh request was submitted.
	PendingRequestID        string
	PendingStartupInputHash string
	PendingPayloadHash      string
}

func (a RuntimeExecutionIntentAdmission) Normalized() RuntimeExecutionIntentAdmission {
	a.RequestID = strings.TrimSpace(a.RequestID)
	a.Kind = RuntimeExecutionIntentKind(strings.TrimSpace(string(a.Kind)))
	a.Outcome = RuntimeExecutionIntentOutcome(strings.TrimSpace(string(a.Outcome)))
	a.Reason = RuntimeExecutionIntentReason(strings.TrimSpace(string(a.Reason)))
	return a
}

func (a RuntimeExecutionIntentAdmission) Validate() error {
	a = a.Normalized()
	if a.WorkspaceID == "" || a.RequestID == "" {
		return fmt.Errorf("runtime execution intent admission requires workspace and request identity")
	}
	if a.Kind != RuntimeExecutionIntentResume && a.Kind != RuntimeExecutionIntentRestart {
		return fmt.Errorf("runtime execution intent admission has invalid kind %q", a.Kind)
	}
	if a.Outcome == RuntimeExecutionIntentFreshUnresolved {
		decoded, err := hex.DecodeString(a.PendingPayloadHash)
		if a.Kind != RuntimeExecutionIntentRestart || a.RunID != "" || a.PendingRequestID == "" || !utf8.ValidString(a.PendingRequestID) || len(a.PendingRequestID) > 256 || strings.TrimSpace(a.PendingRequestID) != a.PendingRequestID || strings.ContainsAny(a.PendingRequestID, "\x00\r\n") || a.PendingStartupInputHash == "" || !utf8.ValidString(a.PendingStartupInputHash) || len(a.PendingStartupInputHash) > 256 || strings.TrimSpace(a.PendingStartupInputHash) != a.PendingStartupInputHash || err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != a.PendingPayloadHash {
			return fmt.Errorf("unresolved fresh intent requires exact pending identity and no asserted Run")
		}
	} else if a.PendingRequestID != "" || a.PendingStartupInputHash != "" || a.PendingPayloadHash != "" {
		return fmt.Errorf("resolved execution outcome cannot carry an unresolved identity")
	}
	return ValidateRuntimeExecutionIntentAdmission(a.Outcome, a.Reason)
}

func ValidateRuntimeExecutionIntentAdmission(outcome RuntimeExecutionIntentOutcome, reason RuntimeExecutionIntentReason) error {
	outcome = RuntimeExecutionIntentOutcome(strings.TrimSpace(string(outcome)))
	reason = RuntimeExecutionIntentReason(strings.TrimSpace(string(reason)))
	valid := false
	switch outcome {
	case RuntimeExecutionIntentReconciliationRequested:
		switch reason {
		case RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
			RuntimeExecutionIntentReasonPendingRunActivated,
			RuntimeExecutionIntentReasonNewRunActivated:
			valid = true
		}
	case RuntimeExecutionIntentReplacementPending:
		valid = reason == RuntimeExecutionIntentReasonDuplicateRestart || reason == RuntimeExecutionIntentReasonPredecessorSupersedeRequested || reason == RuntimeExecutionIntentReasonTerminalPredecessorSuccessorRequested
	case RuntimeExecutionIntentRestartRequired:
		switch reason {
		case RuntimeExecutionIntentReasonWorkspaceActiveRunAbsent,
			RuntimeExecutionIntentReasonSelectedRunAbsent,
			RuntimeExecutionIntentReasonSelectedRunTerminal,
			RuntimeExecutionIntentReasonRunNotResumable:
			valid = true
		}
	case RuntimeExecutionIntentFreshActivationRequired:
		valid = reason == RuntimeExecutionIntentReasonRuntimeBootstrapMissing || reason == RuntimeExecutionIntentReasonDefinitionAdmissionMissing || reason == RuntimeExecutionIntentReasonDefinitionAdmissionMismatch || reason == RuntimeExecutionIntentReasonCreationFenceMissing
	case RuntimeExecutionIntentFreshUnresolved:
		valid = reason == RuntimeExecutionIntentReasonFreshResultUnknown
	case RuntimeExecutionIntentDefinitionChanged:
		valid = reason == RuntimeExecutionIntentReasonDefinitionChanged
	case RuntimeExecutionIntentGovernanceRequired:
		valid = reason == RuntimeExecutionIntentReasonGovernancePending || reason == RuntimeExecutionIntentReasonDefinitionClarificationPending
	}
	if !valid {
		return fmt.Errorf("invalid runtime execution intent admission outcome %q reason %q", outcome, reason)
	}
	return nil
}

// RuntimeExecutionIntentReasonSummary is a presentation-only pure helper.
// State owners persist and compare Reason, never this derived text.
func RuntimeExecutionIntentReasonSummary(reason RuntimeExecutionIntentReason) string {
	switch RuntimeExecutionIntentReason(strings.TrimSpace(string(reason))) {
	case RuntimeExecutionIntentReasonWorkspaceActiveRunAbsent:
		return "workspace has no active run"
	case RuntimeExecutionIntentReasonSelectedRunAbsent:
		return "Workspace selected Run is absent"
	case RuntimeExecutionIntentReasonSelectedRunTerminal:
		return "Workspace selected Run is terminal"
	case RuntimeExecutionIntentReasonDefinitionChanged:
		return "Workspace Goal/DoD definition differs from sealed Run"
	case RuntimeExecutionIntentReasonRuntimeBootstrapMissing:
		return "Run lacks a matching sealed Runtime bootstrap"
	case RuntimeExecutionIntentReasonCurrentRunContinuationAccepted:
		return "current Run continuation accepted"
	case RuntimeExecutionIntentReasonPendingRunActivated:
		return "pending Run activated for continuation"
	case RuntimeExecutionIntentReasonGovernancePending:
		return "Run remains blocked until matching governance resolution"
	case RuntimeExecutionIntentReasonRunNotResumable:
		return "Run is not resumable"
	case RuntimeExecutionIntentReasonDuplicateRestart:
		return "duplicate restart request"
	case RuntimeExecutionIntentReasonNewRunActivated:
		return "new Workspace Run activated"
	case RuntimeExecutionIntentReasonPredecessorSupersedeRequested:
		return "predecessor supersede requested"
	case RuntimeExecutionIntentReasonTerminalPredecessorSuccessorRequested:
		return "terminal predecessor successor requested"
	case RuntimeExecutionIntentReasonDefinitionAdmissionMissing:
		return "Workspace definition has not completed pre-seal admission"
	case RuntimeExecutionIntentReasonDefinitionAdmissionMismatch:
		return "restart bootstrap does not match the admitted Workspace startup input"
	case RuntimeExecutionIntentReasonDefinitionClarificationPending:
		return "revise Goal/DoD to produce a newly admitted startup input before restarting"
	case RuntimeExecutionIntentReasonFreshResultUnknown:
		return "original fresh intent result remains unresolved; no new execution was forwarded"
	case RuntimeExecutionIntentReasonCreationFenceMissing:
		return "Workspace has no preparation creation identity; fresh execution is blocked"
	default:
		return ""
	}
}
