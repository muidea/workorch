package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeOperationRecoveryAuthorizationState is the persisted SM-4 handoff
// state for one bounded replay of an already-claimed SM-3 operation. At most
// one issued or in-flight authorization may exist. A new budget may be issued
// only after the prior call is settled, explicitly uncertain, or its durable
// in-flight lease has expired.
type RuntimeOperationRecoveryAuthorizationState string

const (
	RuntimeOperationRecoveryAuthorizationIssued    RuntimeOperationRecoveryAuthorizationState = "issued"
	RuntimeOperationRecoveryAuthorizationInFlight  RuntimeOperationRecoveryAuthorizationState = "in_flight"
	RuntimeOperationRecoveryAuthorizationUncertain RuntimeOperationRecoveryAuthorizationState = "uncertain"
	RuntimeOperationRecoveryAuthorizationSettled   RuntimeOperationRecoveryAuthorizationState = "settled"
	RuntimeOperationRecoveryAuthorizationRetired   RuntimeOperationRecoveryAuthorizationState = "retired"
)

const RuntimeOperationRecoveryClaimLeaseDuration = 30 * time.Minute

// RuntimeOperationRecoveryAuthorization binds one recovery-budget
// consumption to the exact persisted dispatch execution. It is stored inside
// the opaque Runtime aggregate so a process restart after the budget CAS can
// resume the same replay without reconstructing authority from a counter or a
// process-local value.
type RuntimeOperationRecoveryAuthorization struct {
	AuthorizationID       string
	WorkspaceID           common.WorkspaceID
	RunID                 common.RunID
	DispatchID            string
	OperationID           string
	ExecutionKey          string
	Fence                 uint64
	RecoveryAttempt       int
	IssuedAtStateRevision uint64
	State                 RuntimeOperationRecoveryAuthorizationState
	ClaimID               string
	ClaimGeneration       uint64
	ClaimedAt             string
	LeaseDeadline         string
}

func (a RuntimeOperationRecoveryAuthorization) Normalized() RuntimeOperationRecoveryAuthorization {
	a.AuthorizationID = strings.TrimSpace(a.AuthorizationID)
	a.DispatchID = strings.TrimSpace(a.DispatchID)
	a.OperationID = strings.TrimSpace(a.OperationID)
	a.ExecutionKey = strings.TrimSpace(a.ExecutionKey)
	a.State = RuntimeOperationRecoveryAuthorizationState(strings.TrimSpace(string(a.State)))
	a.ClaimID = strings.TrimSpace(a.ClaimID)
	a.ClaimedAt = strings.TrimSpace(a.ClaimedAt)
	a.LeaseDeadline = strings.TrimSpace(a.LeaseDeadline)
	return a
}

func (a RuntimeOperationRecoveryAuthorization) Clone() RuntimeOperationRecoveryAuthorization {
	return a.Normalized()
}

func (a RuntimeOperationRecoveryAuthorization) Validate() error {
	a = a.Normalized()
	if a.WorkspaceID == "" || a.RunID == "" || a.DispatchID == "" || a.OperationID == "" || a.ExecutionKey == "" ||
		a.Fence == 0 || a.RecoveryAttempt <= 0 || a.IssuedAtStateRevision == 0 {
		return fmt.Errorf("runtime operation recovery authorization is incomplete")
	}
	switch a.State {
	case RuntimeOperationRecoveryAuthorizationIssued:
		if a.ClaimID != "" || a.ClaimGeneration != 0 || a.ClaimedAt != "" || a.LeaseDeadline != "" {
			return fmt.Errorf("issued runtime operation recovery authorization cannot carry claim state")
		}
	case RuntimeOperationRecoveryAuthorizationRetired:
		if a.ClaimID != "" || a.ClaimGeneration != 0 || a.ClaimedAt != "" || a.LeaseDeadline != "" {
			return fmt.Errorf("retired runtime operation recovery authorization cannot carry claim state")
		}
	case RuntimeOperationRecoveryAuthorizationInFlight,
		RuntimeOperationRecoveryAuthorizationUncertain,
		RuntimeOperationRecoveryAuthorizationSettled:
		if a.ClaimID == "" || a.ClaimGeneration != 1 || a.ClaimedAt == "" || a.LeaseDeadline == "" ||
			a.ClaimID != StableRuntimeOperationRecoveryClaimID(a) {
			return fmt.Errorf("runtime operation recovery authorization claim identity is incomplete")
		}
		claimedAt, claimedErr := time.Parse(time.RFC3339Nano, a.ClaimedAt)
		deadline, deadlineErr := time.Parse(time.RFC3339Nano, a.LeaseDeadline)
		if claimedErr != nil || deadlineErr != nil || !deadline.After(claimedAt) {
			return fmt.Errorf("runtime operation recovery authorization claim lease is invalid")
		}
	default:
		return fmt.Errorf("runtime operation recovery authorization has invalid state %q", a.State)
	}
	if a.AuthorizationID != StableRuntimeOperationRecoveryAuthorizationID(a) {
		return fmt.Errorf("runtime operation recovery authorization identity mismatch")
	}
	return nil
}

func StableRuntimeOperationRecoveryClaimID(value RuntimeOperationRecoveryAuthorization) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-operation-recovery-claim")
	writeHashString(h, value.AuthorizationID)
	writeHashString(h, fmt.Sprintf("%d", value.ClaimGeneration))
	writeHashString(h, value.ClaimedAt)
	writeHashString(h, value.LeaseDeadline)
	return "runtime-operation-recovery-claim-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// StableRuntimeOperationRecoveryAuthorizationID excludes mutable state while
// binding the exact recovery-budget attempt to one immutable execution fence.
func StableRuntimeOperationRecoveryAuthorizationID(value RuntimeOperationRecoveryAuthorization) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-operation-recovery-authorization")
	writeHashString(h, string(value.WorkspaceID))
	writeHashString(h, string(value.RunID))
	writeHashString(h, value.DispatchID)
	writeHashString(h, value.OperationID)
	writeHashString(h, value.ExecutionKey)
	writeHashString(h, fmt.Sprintf("%d", value.Fence))
	writeHashString(h, fmt.Sprintf("%d", value.RecoveryAttempt))
	writeHashString(h, fmt.Sprintf("%d", value.IssuedAtStateRevision))
	return "runtime-operation-recovery-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func RuntimeOperationRecoveryAuthorizationForExecution(
	values []RuntimeOperationRecoveryAuthorization,
	dispatchID string,
	execution RuntimeOperationExecutionRecord,
) (RuntimeOperationRecoveryAuthorization, bool) {
	dispatchID = strings.TrimSpace(dispatchID)
	execution = execution.Normalized()
	for idx := len(values) - 1; idx >= 0; idx-- {
		value := values[idx].Normalized()
		if value.State == RuntimeOperationRecoveryAuthorizationIssued && value.DispatchID == dispatchID &&
			value.OperationID == execution.OperationID && value.ExecutionKey == execution.ExecutionKey && value.Fence == execution.Fence {
			return value, true
		}
	}
	return RuntimeOperationRecoveryAuthorization{}, false
}

// RuntimeInFlightOperationRecoveryAuthorizationForExecution resolves the
// exact latest durable claim. The returned record is only an identity check;
// persisted in-flight state is never replay authority by itself. The caller
// must additionally hold the one-shot claim ID returned by the CAS winner.
func RuntimeInFlightOperationRecoveryAuthorizationForExecution(
	values []RuntimeOperationRecoveryAuthorization,
	claimID string,
	now string,
	dispatchID string,
	execution RuntimeOperationExecutionRecord,
) (RuntimeOperationRecoveryAuthorization, bool) {
	claimID = strings.TrimSpace(claimID)
	now = strings.TrimSpace(now)
	dispatchID = strings.TrimSpace(dispatchID)
	execution = execution.Normalized()
	observedAt, err := time.Parse(time.RFC3339Nano, now)
	if claimID == "" || len(values) == 0 || err != nil {
		return RuntimeOperationRecoveryAuthorization{}, false
	}
	value := values[len(values)-1].Normalized()
	deadline, deadlineErr := time.Parse(time.RFC3339Nano, value.LeaseDeadline)
	if value.State != RuntimeOperationRecoveryAuthorizationInFlight || value.ClaimID != claimID || deadlineErr != nil || observedAt.After(deadline) ||
		value.DispatchID != dispatchID || value.OperationID != execution.OperationID ||
		value.ExecutionKey != execution.ExecutionKey || value.Fence != execution.Fence ||
		value.RecoveryAttempt != len(values) {
		return RuntimeOperationRecoveryAuthorization{}, false
	}
	return value, true
}

func RuntimeCurrentOperationRecoveryAuthorizationForExecution(
	values []RuntimeOperationRecoveryAuthorization,
	dispatchID string,
	execution RuntimeOperationExecutionRecord,
) (RuntimeOperationRecoveryAuthorization, bool) {
	dispatchID = strings.TrimSpace(dispatchID)
	execution = execution.Normalized()
	for idx := len(values) - 1; idx >= 0; idx-- {
		value := values[idx].Normalized()
		if value.DispatchID == dispatchID && value.OperationID == execution.OperationID &&
			value.ExecutionKey == execution.ExecutionKey && value.Fence == execution.Fence {
			return value, true
		}
	}
	return RuntimeOperationRecoveryAuthorization{}, false
}

// ValidateRuntimeOperationRecoveryAuthorizations is the fresh-load fence for
// the SM-4 authorization ledger. Every recovery attempt must be present once,
// ordered by the global bounded counter, and reverse-bind to an exact sealed
// execution in current or immutable dispatch history.
func ValidateRuntimeOperationRecoveryAuthorizations(
	workspaceID common.WorkspaceID,
	runID common.RunID,
	active *RuntimeDispatchRecord,
	completed []RuntimeDispatchRecord,
	receipts []RuntimeOperationReceipt,
	values []RuntimeOperationRecoveryAuthorization,
	recovery RuntimeRecoveryLedger,
	stateRevision uint64,
) error {
	if recovery.OperationRecoveryAttempts != len(values) {
		return fmt.Errorf("runtime operation recovery counter/authorization ledger mismatch")
	}
	seen := make(map[string]struct{}, len(values))
	var previousIssuedRevision uint64
	actionableCount := 0
	for idx, raw := range values {
		value := raw.Normalized()
		if err := value.Validate(); err != nil {
			return err
		}
		if value.WorkspaceID != workspaceID || value.RunID != runID || value.RecoveryAttempt != idx+1 {
			return fmt.Errorf("runtime operation recovery authorization scope or attempt sequence mismatch")
		}
		if value.IssuedAtStateRevision > stateRevision || value.IssuedAtStateRevision <= previousIssuedRevision {
			return fmt.Errorf("runtime operation recovery authorization revision sequence mismatch")
		}
		previousIssuedRevision = value.IssuedAtStateRevision
		if _, duplicate := seen[value.AuthorizationID]; duplicate {
			return fmt.Errorf("runtime operation recovery authorization %q is duplicated", value.AuthorizationID)
		}
		seen[value.AuthorizationID] = struct{}{}
		execution, activeExecution, found := runtimeOperationRecoveryAuthorizationExecution(value, active, completed)
		if !found {
			return fmt.Errorf("runtime operation recovery authorization has no exact dispatch execution")
		}
		if value.State == RuntimeOperationRecoveryAuthorizationIssued || value.State == RuntimeOperationRecoveryAuthorizationInFlight {
			actionableCount++
			if actionableCount != 1 || idx != len(values)-1 {
				return fmt.Errorf("runtime operation recovery authorization ledger has multiple or non-current actionable grants")
			}
			if !activeExecution || execution.State != OperationExecutionClaimed {
				return fmt.Errorf("actionable runtime operation recovery authorization requires an active claimed execution")
			}
			if _, receiptFound := RuntimeOperationReceiptForExecutionKeyFence(receipts, execution.ExecutionKey, execution.Fence); receiptFound {
				return fmt.Errorf("actionable runtime operation recovery authorization cannot retain a terminal receipt")
			}
		}
		if value.State == RuntimeOperationRecoveryAuthorizationSettled {
			if _, receiptFound := RuntimeOperationReceiptForExecutionKeyFence(receipts, execution.ExecutionKey, execution.Fence); !receiptFound {
				return fmt.Errorf("settled runtime operation recovery authorization requires a terminal receipt")
			}
		}
	}
	return nil
}

func runtimeOperationRecoveryAuthorizationExecution(value RuntimeOperationRecoveryAuthorization, active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord) (RuntimeOperationExecutionRecord, bool, bool) {
	matches := func(dispatch RuntimeDispatchRecord) (RuntimeOperationExecutionRecord, bool) {
		if dispatch.ID != value.DispatchID || dispatch.WorkspaceID != value.WorkspaceID || dispatch.RunID != value.RunID {
			return RuntimeOperationExecutionRecord{}, false
		}
		for _, execution := range dispatch.OperationExecutions {
			execution = execution.Normalized()
			if execution.OperationID == value.OperationID && execution.ExecutionKey == value.ExecutionKey && execution.Fence == value.Fence {
				return execution, true
			}
		}
		return RuntimeOperationExecutionRecord{}, false
	}
	if active != nil {
		if execution, found := matches(*active); found {
			return execution, true, true
		}
	}
	for _, dispatch := range completed {
		if execution, found := matches(dispatch); found {
			return execution, false, true
		}
	}
	return RuntimeOperationExecutionRecord{}, false, false
}
