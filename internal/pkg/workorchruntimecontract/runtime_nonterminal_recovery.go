package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	RuntimeNonTerminalRecoveryFailureCASConflict    = "runtime-cas-conflict"
	RuntimeNonTerminalRecoveryFailureOwnerTransient = "runtime-owner-transient"

	RuntimeNonTerminalRecoveryBackoffBase = time.Second
	RuntimeNonTerminalRecoveryBackoffCap  = time.Minute
)

// RuntimeNonTerminalRecoveryDisposition is the durable SM-4 budget outcome.
// Both dispositions retain a scheduled successor; exhaustion is deliberately
// not a business-terminal decision.
type RuntimeNonTerminalRecoveryDisposition string

const (
	RuntimeNonTerminalRecoveryDispositionBudgetConsumed  RuntimeNonTerminalRecoveryDisposition = "budget_consumed"
	RuntimeNonTerminalRecoveryDispositionBudgetExhausted RuntimeNonTerminalRecoveryDisposition = "budget_exhausted"
)

// RuntimeNonTerminalRecoveryKind preserves the exact typed persistence
// outcome while both technical outcomes share the owner-local CAS reload
// budget. It is not an operation/provider recovery classification.
type RuntimeNonTerminalRecoveryKind string

const (
	RuntimeNonTerminalRecoveryCASConflict    RuntimeNonTerminalRecoveryKind = "cas_conflict"
	RuntimeNonTerminalRecoveryOwnerTransient RuntimeNonTerminalRecoveryKind = "owner_transient"
)

// RuntimeNonTerminalRecoveryRequest identifies one failed initial drive or
// running lease generation. ObservedAt determines the successor's due time but
// is intentionally excluded from RequestID: redelivery of the same typed owner
// outcome at a later wall-clock instant remains the same recovery request.
type RuntimeNonTerminalRecoveryRequest struct {
	RequestID             string
	WorkspaceID           common.WorkspaceID
	RunID                 common.RunID
	Kind                  RuntimeNonTerminalRecoveryKind
	FailedLeaseGeneration uint64
	// ExpectedRuntimeRevision and OwnerObservedRuntimeRevision are copied from
	// the typed Run-owner persistence outcome. IssuedAtStateRevision is a
	// separate fresh-load admission fence for this recovery transaction.
	ExpectedRuntimeRevision      uint64
	OwnerObservedRuntimeRevision uint64
	IssuedAtStateRevision        uint64
	FailureSignature             string
	OwnerFeedbackHash            string
	ObservedAt                   string
}

func (r RuntimeNonTerminalRecoveryRequest) Normalized() RuntimeNonTerminalRecoveryRequest {
	r.RequestID = strings.TrimSpace(r.RequestID)
	r.FailureSignature = strings.TrimSpace(r.FailureSignature)
	r.OwnerFeedbackHash = strings.TrimSpace(r.OwnerFeedbackHash)
	r.ObservedAt = strings.TrimSpace(r.ObservedAt)
	return r
}

func (r RuntimeNonTerminalRecoveryRequest) Validate() error {
	r = r.Normalized()
	if r.WorkspaceID == "" || r.RunID == "" || r.RequestID == "" || r.ObservedAt == "" {
		return fmt.Errorf("runtime nonterminal recovery request requires scope, identity and observed-at")
	}
	if _, err := time.Parse(time.RFC3339Nano, r.ObservedAt); err != nil {
		return fmt.Errorf("runtime nonterminal recovery observed-at must be RFC3339: %w", err)
	}
	if r.IssuedAtStateRevision == 0 {
		return fmt.Errorf("runtime nonterminal recovery request requires issued state revision")
	}
	switch r.Kind {
	case RuntimeNonTerminalRecoveryCASConflict:
		if r.FailureSignature != RuntimeNonTerminalRecoveryFailureCASConflict || r.OwnerFeedbackHash != "" {
			return fmt.Errorf("runtime CAS recovery request has invalid outcome identity")
		}
	case RuntimeNonTerminalRecoveryOwnerTransient:
		if r.FailureSignature != RuntimeNonTerminalRecoveryFailureOwnerTransient ||
			!runtimeNonTerminalRecoveryHasSHA256(r.OwnerFeedbackHash, "runtime-feedback-") {
			return fmt.Errorf("runtime owner-transient recovery request has invalid outcome identity")
		}
	default:
		return fmt.Errorf("runtime nonterminal recovery request has unsupported kind %q", r.Kind)
	}
	if r.RequestID != StableRuntimeNonTerminalRecoveryRequestID(r) {
		return fmt.Errorf("runtime nonterminal recovery request requires exact stable identity")
	}
	return nil
}

func runtimeNonTerminalRecoveryHasSHA256(value, prefix string) bool {
	encoded := strings.TrimPrefix(value, prefix)
	if encoded == value || len(encoded) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(encoded)
	return err == nil
}

// StableRuntimeNonTerminalRecoveryRequestID binds the typed persistence
// outcome and failed lease generation without binding redelivery time.
func StableRuntimeNonTerminalRecoveryRequestID(request RuntimeNonTerminalRecoveryRequest) string {
	r := request.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-nonterminal-recovery-request")
	writeHashString(h, string(r.WorkspaceID))
	writeHashString(h, string(r.RunID))
	writeHashString(h, string(r.Kind))
	writeHashString(h, strconv.FormatUint(r.FailedLeaseGeneration, 10))
	writeHashString(h, strconv.FormatUint(r.ExpectedRuntimeRevision, 10))
	writeHashString(h, strconv.FormatUint(r.OwnerObservedRuntimeRevision, 10))
	writeHashString(h, strconv.FormatUint(r.IssuedAtStateRevision, 10))
	writeHashString(h, r.FailureSignature)
	writeHashString(h, r.OwnerFeedbackHash)
	return "runtime-nonterminal-recovery-" + hex.EncodeToString(h.Sum(nil))
}

// SameRuntimeNonTerminalRecoveryRequestIdentity compares the stable request
// preimage. ObservedAt is excluded for the same reason it is excluded from ID.
func SameRuntimeNonTerminalRecoveryRequestIdentity(left, right RuntimeNonTerminalRecoveryRequest) bool {
	l, r := left.Normalized(), right.Normalized()
	return l.RequestID == r.RequestID &&
		l.WorkspaceID == r.WorkspaceID && l.RunID == r.RunID && l.Kind == r.Kind &&
		l.FailedLeaseGeneration == r.FailedLeaseGeneration && l.ExpectedRuntimeRevision == r.ExpectedRuntimeRevision &&
		l.OwnerObservedRuntimeRevision == r.OwnerObservedRuntimeRevision && l.IssuedAtStateRevision == r.IssuedAtStateRevision &&
		l.FailureSignature == r.FailureSignature &&
		l.OwnerFeedbackHash == r.OwnerFeedbackHash
}

func RuntimeNonTerminalRecoveryLeaseReason(kind RuntimeNonTerminalRecoveryKind) (RuntimeDriveLeaseReason, bool) {
	switch kind {
	case RuntimeNonTerminalRecoveryCASConflict:
		return RuntimeDriveLeaseReasonCASReload, true
	case RuntimeNonTerminalRecoveryOwnerTransient:
		return RuntimeDriveLeaseReasonOwnerTransient, true
	default:
		return "", false
	}
}

// RuntimeNonTerminalRecoveryBackoff is exponential and bounded. Once the
// attempt counter is exhausted, every later successor uses the same capped
// delay instead of inventing an unbounded counter.
func RuntimeNonTerminalRecoveryBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return RuntimeNonTerminalRecoveryBackoffBase
	}
	delay := RuntimeNonTerminalRecoveryBackoffBase
	for current := 1; current < attempt && delay < RuntimeNonTerminalRecoveryBackoffCap; current++ {
		delay *= 2
		if delay > RuntimeNonTerminalRecoveryBackoffCap {
			delay = RuntimeNonTerminalRecoveryBackoffCap
		}
	}
	return delay
}

func RuntimeNonTerminalRecoveryDueAt(observedAt string, attempt int, dispositions ...RuntimeNonTerminalRecoveryDisposition) (string, error) {
	observed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(observedAt))
	if err != nil || attempt < 1 || len(dispositions) > 1 {
		return "", fmt.Errorf("runtime nonterminal recovery due-at input is invalid")
	}
	delay := RuntimeNonTerminalRecoveryBackoff(attempt)
	if len(dispositions) == 1 {
		switch dispositions[0] {
		case RuntimeNonTerminalRecoveryDispositionBudgetConsumed:
		case RuntimeNonTerminalRecoveryDispositionBudgetExhausted:
			delay = RuntimeNonTerminalRecoveryBackoffCap
		default:
			return "", fmt.Errorf("runtime nonterminal recovery due-at disposition is invalid")
		}
	}
	return observed.Add(delay).UTC().Format(time.RFC3339Nano), nil
}

// RuntimeNonTerminalRecoveryRecord is the latest atomic budget/lease
// linearization point. Keeping the complete scheduled successor makes a lost
// response recognizable after reload without creating a second retry authority.
type RuntimeNonTerminalRecoveryRecord struct {
	Request                 RuntimeNonTerminalRecoveryRequest
	Disposition             RuntimeNonTerminalRecoveryDisposition
	Attempt                 int
	RecordedAtStateRevision uint64
	SuccessorLease          RuntimeDriveLease
}

func (r RuntimeNonTerminalRecoveryRecord) Normalized() RuntimeNonTerminalRecoveryRecord {
	r.Request = r.Request.Normalized()
	r.Disposition = RuntimeNonTerminalRecoveryDisposition(strings.TrimSpace(string(r.Disposition)))
	r.SuccessorLease = r.SuccessorLease.Normalized()
	return r
}

func (r RuntimeNonTerminalRecoveryRecord) Validate(policy RuntimeRecoveryPolicy) error {
	r = r.Normalized()
	if err := r.Request.Validate(); err != nil {
		return err
	}
	limit := policy.Normalized().MaxCASReloadAttempts
	if limit < 1 || r.Attempt < 1 || r.Attempt > limit || r.RecordedAtStateRevision == 0 ||
		r.Request.IssuedAtStateRevision == ^uint64(0) || r.RecordedAtStateRevision != r.Request.IssuedAtStateRevision+1 {
		return fmt.Errorf("runtime nonterminal recovery record has invalid budget position")
	}
	switch r.Disposition {
	case RuntimeNonTerminalRecoveryDispositionBudgetConsumed:
	case RuntimeNonTerminalRecoveryDispositionBudgetExhausted:
		if r.Attempt != limit {
			return fmt.Errorf("runtime nonterminal recovery exhaustion does not match policy limit")
		}
	default:
		return fmt.Errorf("runtime nonterminal recovery record has invalid disposition %q", r.Disposition)
	}
	if r.Request.FailedLeaseGeneration == ^uint64(0) {
		return fmt.Errorf("runtime nonterminal recovery lease generation exhausted")
	}
	if err := r.SuccessorLease.Validate(); err != nil {
		return fmt.Errorf("runtime nonterminal recovery successor: %w", err)
	}
	reason, _ := RuntimeNonTerminalRecoveryLeaseReason(r.Request.Kind)
	dueAt, err := RuntimeNonTerminalRecoveryDueAt(r.Request.ObservedAt, r.Attempt, r.Disposition)
	if err != nil || r.SuccessorLease.WorkspaceID != r.Request.WorkspaceID || r.SuccessorLease.RunID != r.Request.RunID ||
		r.SuccessorLease.AggregateRevision != r.RecordedAtStateRevision ||
		r.SuccessorLease.RecoveryGeneration != r.Request.FailedLeaseGeneration+1 ||
		r.SuccessorLease.Reason != reason || r.SuccessorLease.FailureSignature != r.Request.FailureSignature ||
		r.SuccessorLease.RecoveryRequestID != r.Request.RequestID || r.SuccessorLease.DueAt != dueAt ||
		r.SuccessorLease.State != RuntimeDriveLeaseScheduled {
		return fmt.Errorf("runtime nonterminal recovery successor does not match its request")
	}
	return nil
}

func RuntimeNonTerminalRecoveryRecordPresent(record RuntimeNonTerminalRecoveryRecord) bool {
	return record.Normalized() != (RuntimeNonTerminalRecoveryRecord{})
}

func RuntimeDriveLeaseBindsNonTerminalRecovery(lease RuntimeDriveLease) bool {
	lease = lease.Normalized()
	return lease.RecoveryRequestID != "" &&
		(lease.Reason == RuntimeDriveLeaseReasonCASReload || lease.Reason == RuntimeDriveLeaseReasonOwnerTransient)
}

// ValidateRuntimeNonTerminalRecoveryLeaseBinding proves that a current generic
// recovery lease descends from the latest atomic ledger record. Claim and
// restart recovery may advance revision/state/generation, but cannot change the
// request, reason or failure identity that authorized the chain.
func ValidateRuntimeNonTerminalRecoveryLeaseBinding(ledger RuntimeRecoveryLedger, lease RuntimeDriveLease) error {
	lease = lease.Normalized()
	if !RuntimeDriveLeaseBindsNonTerminalRecovery(lease) {
		return nil
	}
	record := ledger.Normalized().NonTerminalRecovery
	if !RuntimeNonTerminalRecoveryRecordPresent(record) ||
		lease.RecoveryRequestID != record.Request.RequestID ||
		lease.WorkspaceID != record.Request.WorkspaceID || lease.RunID != record.Request.RunID ||
		lease.Reason != record.SuccessorLease.Reason || lease.FailureSignature != record.SuccessorLease.FailureSignature ||
		lease.RecoveryGeneration < record.SuccessorLease.RecoveryGeneration ||
		lease.AggregateRevision < record.RecordedAtStateRevision {
		return fmt.Errorf("runtime nonterminal recovery lease has no matching persisted record")
	}
	if lease.RecoveryGeneration == record.SuccessorLease.RecoveryGeneration {
		if lease.DueAt != record.SuccessorLease.DueAt ||
			(lease.State == RuntimeDriveLeaseScheduled && lease.AggregateRevision != record.RecordedAtStateRevision) {
			return fmt.Errorf("runtime nonterminal recovery lease does not match its scheduled successor")
		}
	}
	return nil
}

// ValidateRuntimeNonTerminalRecoveryLinearization proves the reverse side of
// the atomic recovery transaction. At the revision that records a request and
// budget disposition, its exact scheduled successor must be the current lease;
// only a later owner transition may claim, replace or retire that authority.
func ValidateRuntimeNonTerminalRecoveryLinearization(ledger RuntimeRecoveryLedger, stateRevision uint64, current *RuntimeDriveLease) error {
	record := ledger.Normalized().NonTerminalRecovery.Normalized()
	if !RuntimeNonTerminalRecoveryRecordPresent(record) {
		return nil
	}
	if record.RecordedAtStateRevision > stateRevision {
		return fmt.Errorf("runtime nonterminal recovery record is ahead of current state revision")
	}
	if record.RecordedAtStateRevision != stateRevision {
		return nil
	}
	if current == nil || current.Normalized() != record.SuccessorLease.Normalized() {
		return fmt.Errorf("runtime nonterminal recovery record requires its exact atomic successor lease")
	}
	return nil
}
