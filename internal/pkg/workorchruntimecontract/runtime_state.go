package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeDispatchState string

const (
	RuntimeDispatchStateSealed    RuntimeDispatchState = "sealed"
	RuntimeDispatchStateCommitted RuntimeDispatchState = "committed"
	// RuntimeDispatchStateInvalidated records a sealed dispatch whose remaining
	// work cannot continue after a prerequisite observation became stale. Any
	// accepted effects before invalidation remain represented by their receipt
	// and evidence; unexecuted operations never become rejected evidence.
	RuntimeDispatchStateInvalidated RuntimeDispatchState = "invalidated"
)

type RuntimeDispatchRecord struct {
	ID            string
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID
	Transaction   AuthorityTransaction
	CandidateHash string
	Candidate     RuntimeCandidate
	// ValidationRecipes is the exact manifest-selected, Shell-owner-confirmed
	// recipe snapshot for this dispatch.  It is a sealed operation contract,
	// not activation policy and not mutable current owner inventory.
	ValidationRecipes     []RuntimeValidationRecipe
	State                 RuntimeDispatchState
	CompletedOperationIDs []string
	// OperationExecutions is the required per-operation claim/fence ledger.
	OperationExecutions []RuntimeOperationExecutionRecord
}

type RuntimeDiagnosticRecord struct {
	EventID             string
	AuthoritySnapshotID string
	CurrentSnapshotID   string
	DispatchID          string
	Reason              string
}

type RuntimeCandidateAttemptState string

const (
	RuntimeCandidateAttemptStatePending  RuntimeCandidateAttemptState = "pending"
	RuntimeCandidateAttemptStateFailed   RuntimeCandidateAttemptState = "failed"
	RuntimeCandidateAttemptStateAccepted RuntimeCandidateAttemptState = "accepted"
	// Invalidated is a freshness transition, not a candidate failure. The next
	// candidate is attempt 0 under a newly sealed authority transaction.
	RuntimeCandidateAttemptStateInvalidated RuntimeCandidateAttemptState = "invalidated"
)

// RuntimeCandidateFeasibilityRecord is the durable, read-only SM-3 admission
// result for one proposed operation.  It is not an execution receipt and can
// never be used as evidence. Persisting it lets restart/duplicate delivery
// resume an attempt without replaying an already completed owner probe.
type RuntimeCandidateFeasibilityRecord struct {
	OperationID             string
	AdmissibilitySnapshotID string
	AcquisitionBinding      string
	// Observation is the sole owner-authored factual result. It is separate from the
	// CandidateAttempt Failure field so an owner infeasibility cannot enter the
	// SM-3 correction matrix.
	Observation *RuntimeOwnerFeasibilityObservation
	// ReplayProof is optional owner preflight output. When present it is sealed
	// into the execution record for same-key/fence receipt recovery; it is never
	// execution evidence and cannot authorize a dispatch by itself.
	ReplayProof *RuntimeOperationReplayProof
}

type RuntimeCandidateAttemptRecord struct {
	AuthorityContractID string
	Transaction         AuthorityTransaction
	Attempt             int
	StablePrefixHash    string
	// ManifestHash and LogicalRequestID bind an admission attempt to the one
	// complete provider document that produced it.  They are SM-3 identity,
	// not a second authority source: the Orchestrator still validates the
	// manifest against the sealed decision before it can admit or seal.
	ManifestHash     string
	LogicalRequestID string
	State            RuntimeCandidateAttemptState
	// Proposal is the schema-valid, unsealed candidate persisted after provider
	// generation. It is replayed after restart into the read-only feasibility
	// gate, never treated as a dispatch or execution record.
	Proposal      *RuntimeCandidate
	Feasibilities []RuntimeCandidateFeasibilityRecord
	Failure       *RuntimeCandidateFailure
	Stale         *RuntimePrerequisiteStale
	// OwnerFeasibility is immutable owner fact lineage for an invalidated
	// proposal. It is never a candidate failure and therefore cannot authorize
	// attempt 1.
	OwnerFeasibility *RuntimeOwnerFeasibilityObservation
	// PrerequisiteRequest records why this unsealed attempt was invalidated for
	// owner fact acquisition, making duplicate delivery/restart deterministic.
	PrerequisiteRequest *RuntimeOperationPrerequisiteRequest
}

// RuntimeStateRecord 是 Runtime 在 Run owner 中持久化的唯一正式状态。
type RuntimeStateRecord struct {
	// SchemaVersion is the persisted Runtime aggregate schema. Production
	// persistence accepts only SchemaVersionCurrent.
	SchemaVersion    uint32
	StateRevision    uint64
	EvidenceSequence uint64
	Definition       RuntimeDefinitionContext
	Facts            RuntimeFacts
	// SemanticPlanning is the sole persisted semantic-plan request/proposal/
	// feasibility/seal ledger. Run stores it opaquely with the aggregate.
	SemanticPlanning RuntimeSemanticPlanningLedger
	// Reconciliation is the Orchestrator-owned SM-2 request/batch lineage. It
	// is kept separate from Facts so owner observations never become coverage
	// credit by deserialization alone.
	Reconciliation      RuntimeCWIReconciliationLedger
	Decision            RuntimeDecision
	ActiveDispatch      *RuntimeDispatchRecord
	CompletedDispatches []RuntimeDispatchRecord
	ProcessedEventIDs   []string
	Diagnostics         []RuntimeDiagnosticRecord
	// CandidateRequestSessions record provider I/O before a complete candidate
	// exists. They are not candidate attempts and cannot authorize a dispatch.
	CandidateRequestSessions    []RuntimeCandidateRequestSession
	CandidateAttempts           []RuntimeCandidateAttemptRecord
	GovernanceRequests          []RuntimeGovernanceRequestRecord
	GovernanceResolutions       []RuntimeGovernanceResolutionRecord
	OperationGovernanceBindings []RuntimeOperationGovernanceBinding
	// OperationReceipts are owner-authored terminal receipts persisted before
	// capability owners return execution results (introduced in schema v3 / INV-03/INV-04).
	OperationReceipts []RuntimeOperationReceipt
	// DeliveryAcknowledgements are SM-4-only durable receipts for the terminal
	// Result -> Workspace -> Run saga. They never alter the sealed SM-2 decision.
	DeliveryAcknowledgements []RuntimeDeliveryAcknowledgement
	// PrerequisiteInvalidations provide a persisted idempotency fence for
	// freshness refreshes across duplicate events, CAS retries and restart.
	PrerequisiteInvalidations []RuntimePrerequisiteInvalidationRecord
	// DriveLease is the Orchestrator-owned SM-4 recovery permission. Run keeps
	// it opaque together with the rest of the aggregate and never classifies it.
	DriveLease *RuntimeDriveLease
	// Termination is the SM-1 administrative cancel/supersede record.
	// completed/failed do not use this field.
	Termination *RuntimeTerminationRecord
}

// RuntimeStateSchemaVersionCurrent is the only Runtime aggregate schema.
const RuntimeStateSchemaVersionCurrent uint32 = 1

// ErrUnsupportedRuntimeStateSchema identifies a non-current aggregate schema.
var ErrUnsupportedRuntimeStateSchema = errors.New("unsupported runtime state schema")

// RuntimeStateSchemaVersionError retains both schema values so persistence
// adapters can classify this as a typed contract failure rather than parse an
// error string.
type RuntimeStateSchemaVersionError struct {
	Actual    uint32
	Supported uint32
}

func (e *RuntimeStateSchemaVersionError) Error() string {
	if e == nil {
		return ErrUnsupportedRuntimeStateSchema.Error()
	}
	return fmt.Sprintf("%s version %d; supported version is %d", ErrUnsupportedRuntimeStateSchema, e.Actual, e.Supported)
}

func (e *RuntimeStateSchemaVersionError) Unwrap() error {
	return ErrUnsupportedRuntimeStateSchema
}

// ValidateRuntimeStateSchemaVersion is the single schema gate shared by
// bootstrap, persistence and aggregate validation. It is intentionally a pure
// contract helper: it does not migrate, default or select any state transition.
func ValidateRuntimeStateSchemaVersion(version uint32) error {
	if version == RuntimeStateSchemaVersionCurrent {
		return nil
	}
	return &RuntimeStateSchemaVersionError{Actual: version, Supported: RuntimeStateSchemaVersionCurrent}
}

func (r RuntimeStateRecord) Validate() error {
	if err := ValidateRuntimeStateSchemaVersion(r.SchemaVersion); err != nil {
		return err
	}
	if r.Facts.WorkspaceID == "" || r.Facts.RunID == "" {
		return fmt.Errorf("runtime state requires workspace and run scope")
	}
	if err := r.Facts.Validate(); err != nil {
		return err
	}
	if err := r.SemanticPlanning.Validate(); err != nil {
		return fmt.Errorf("runtime semantic planning ledger: %w", err)
	}
	if r.SemanticPlanning.DefinitionSeed.Hash != r.Facts.DefinitionSeed.Hash {
		return fmt.Errorf("runtime semantic planning seed does not match facts")
	}
	if err := r.Reconciliation.Validate(); err != nil {
		return fmt.Errorf("runtime reconciliation ledger: %w", err)
	}
	for _, focus := range append(append([]CoverageFocus{r.Facts.Focus}, r.Facts.RemainingFocuses...), r.Facts.CompletedFocuses...) {
		if err := ValidateCoverageEvidenceBindingsForFocus(focus, r.Facts.DefinitionRevision); err != nil {
			return fmt.Errorf("runtime state focus %s: %w", focus.ID, err)
		}
	}
	if err := r.Definition.Validate(); err != nil {
		return err
	}
	if r.Definition.Normalized().Revision != r.Facts.Normalized().DefinitionRevision {
		return fmt.Errorf("runtime state definition revision mismatch")
	}
	if err := r.Decision.Validate(); err != nil {
		return err
	}
	if r.Decision.AuthoritySnapshot.WorkspaceID != r.Facts.WorkspaceID || r.Decision.AuthoritySnapshot.RunID != r.Facts.RunID {
		return fmt.Errorf("runtime state decision scope mismatch")
	}
	if r.ActiveDispatch != nil {
		if err := r.ActiveDispatch.ValidateWithActivation(r.Decision, r.Facts.Activation); err != nil {
			return fmt.Errorf("runtime state active dispatch: %w", err)
		}
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(r.Facts.WorkspaceID, r.Facts.RunID, r.CandidateAttempts); err != nil {
		return err
	}
	if err := ValidateRuntimeCandidateRequestSessions(r.CandidateRequestSessions); err != nil {
		return err
	}
	if isTerminalRuntimeDecision(r.Decision) && hasPendingRuntimeCandidateAttempt(r.CandidateAttempts) {
		return fmt.Errorf("terminal runtime state cannot retain a pending candidate attempt")
	}
	if hasUnreducedCurrentCandidateRetryFailure(r.Decision, r.CandidateAttempts) {
		return fmt.Errorf("current runtime authority retains a failed candidate attempt 1 without reducer transition")
	}
	for _, attempt := range r.CandidateAttempts {
		// A pending proposal is the exact candidate that can be resumed into the
		// feasibility gate. Validate it again at aggregate load so corrupted
		// persistence cannot cause owner probes for a candidate that was never
		// schema-valid under the still-current authority contract.
		if attempt.State != RuntimeCandidateAttemptStatePending || attempt.Proposal == nil {
			continue
		}
		if !SameAuthorityContract(attempt.Transaction, r.Decision.AuthorityTransaction()) {
			return fmt.Errorf("pending runtime candidate proposal does not match current authority contract")
		}
		if err := attempt.Proposal.ValidateWithActivation(r.Decision, r.Facts.Activation); err != nil {
			return fmt.Errorf("pending runtime candidate proposal is invalid: %w", err)
		}
	}
	if err := ValidateRuntimeGovernanceResolutionRecords(r.GovernanceResolutions); err != nil {
		return err
	}
	if err := ValidateRuntimeGovernanceRequestRecords(r.GovernanceRequests); err != nil {
		return err
	}
	if err := ValidateRuntimeOperationGovernanceBindings(r.OperationGovernanceBindings); err != nil {
		return err
	}
	if err := ValidateRuntimeOperationReceipts(r.OperationReceipts); err != nil {
		return err
	}
	for _, invalidation := range r.PrerequisiteInvalidations {
		if err := invalidation.Validate(); err != nil {
			return fmt.Errorf("runtime prerequisite invalidation: %w", err)
		}
	}
	if r.DriveLease != nil {
		if err := r.DriveLease.Validate(); err != nil {
			return fmt.Errorf("runtime state drive lease: %w", err)
		}
		if r.DriveLease.WorkspaceID != r.Facts.WorkspaceID || r.DriveLease.RunID != r.Facts.RunID {
			return fmt.Errorf("runtime state drive lease scope mismatch")
		}
	}
	for _, request := range r.GovernanceRequests {
		matched := r.ActiveDispatch != nil && r.ActiveDispatch.ID == request.DispatchID &&
			r.ActiveDispatch.Transaction.AuthoritySnapshotID == request.AuthoritySnapshotID && r.ActiveDispatch.Candidate.Governance != nil
		if !matched {
			for _, dispatch := range r.CompletedDispatches {
				if dispatch.ID == request.DispatchID && dispatch.Transaction.AuthoritySnapshotID == request.AuthoritySnapshotID && dispatch.Candidate.Governance != nil {
					matched = true
					break
				}
			}
		}
		if !matched {
			return fmt.Errorf("runtime governance request has no governed dispatch")
		}
	}
	for _, rawBinding := range r.OperationGovernanceBindings {
		binding := rawBinding.Normalized()
		var source *RuntimeDispatchRecord
		for idx := range r.CompletedDispatches {
			if r.CompletedDispatches[idx].ID == binding.OperationDispatchID {
				value := r.CompletedDispatches[idx]
				source = &value
				break
			}
		}
		if source == nil {
			return fmt.Errorf("operation governance binding source dispatch %q is not completed", binding.OperationDispatchID)
		}
		var execution *RuntimeOperationExecutionRecord
		for idx := range source.OperationExecutions {
			if source.OperationExecutions[idx].OperationID == binding.OperationID {
				value := source.OperationExecutions[idx].Normalized()
				execution = &value
				break
			}
		}
		if execution == nil || execution.ExecutionKey != binding.ExecutionKey || execution.Fence != binding.Fence ||
			execution.OwnerReceiptHash != binding.GovernanceReceiptHash || execution.State != OperationExecutionOwnerCommitted {
			return fmt.Errorf("operation governance binding source execution does not match sealed receipt")
		}
		receipt, found := RuntimeOperationReceiptForExecutionKeyFence(r.OperationReceipts, binding.ExecutionKey, binding.Fence)
		if !found || receipt.Outcome != RuntimeOperationOutcomeGovernanceRequired || StableRuntimeOperationReceiptHash(receipt) != binding.GovernanceReceiptHash {
			return fmt.Errorf("operation governance binding has no matching governance receipt")
		}
		if binding.State == RuntimeOperationGovernanceBindingSuspended {
			continue
		}
		bound := false
		for _, request := range r.GovernanceRequests {
			if request.TicketID == binding.TicketID && request.DispatchID == binding.GovernanceDispatchID &&
				request.AuthoritySnapshotID == binding.GovernanceSnapshotID && request.DependencyKind == binding.DependencyKind && request.SubjectRef == binding.SubjectRef {
				bound = true
				break
			}
		}
		if !bound {
			return fmt.Errorf("operation governance binding has no matching ticket request")
		}
	}
	for _, attempt := range r.CandidateAttempts {
		if attempt.State != RuntimeCandidateAttemptStateAccepted {
			continue
		}
		matched := r.ActiveDispatch != nil && r.ActiveDispatch.Transaction == attempt.Transaction
		if !matched {
			for _, dispatch := range r.CompletedDispatches {
				if dispatch.Transaction == attempt.Transaction {
					matched = true
					break
				}
			}
		}
		if !matched {
			return fmt.Errorf("accepted runtime candidate attempt has no sealed dispatch")
		}
	}
	for _, resolution := range r.GovernanceResolutions {
		matched := false
		bound := false
		for _, request := range r.GovernanceRequests {
			if request.TicketID == resolution.TicketID && request.DispatchID == resolution.DispatchID && request.AuthoritySnapshotID == resolution.AuthoritySnapshotID {
				bound = true
				break
			}
		}
		for _, dispatch := range r.CompletedDispatches {
			if dispatch.ID == resolution.DispatchID && dispatch.Transaction.AuthoritySnapshotID == resolution.AuthoritySnapshotID &&
				dispatch.State == RuntimeDispatchStateCommitted && dispatch.Candidate.Governance != nil {
				matched = true
				break
			}
		}
		if !matched || !bound {
			return fmt.Errorf("runtime governance resolution has no completed governed dispatch")
		}
	}
	if r.Termination != nil {
		if err := r.Termination.Validate(); err != nil {
			return fmt.Errorf("runtime state termination: %w", err)
		}
	}
	if r.ActiveDispatch != nil {
		if err := ValidateRuntimeOperationExecutionRecords(r.ActiveDispatch.OperationExecutions); err != nil {
			return fmt.Errorf("runtime state active dispatch executions: %w", err)
		}
	}
	for idx, dispatch := range r.CompletedDispatches {
		if err := ValidateRuntimeOperationExecutionRecords(dispatch.OperationExecutions); err != nil {
			return fmt.Errorf("runtime state completed dispatch[%d] executions: %w", idx, err)
		}
	}
	return nil
}

func isTerminalRuntimeDecision(decision RuntimeDecision) bool {
	return decision.State == ConvergenceStateComplete || decision.State == ConvergenceStateFailed || decision.Terminal != nil
}

func hasPendingRuntimeCandidateAttempt(values []RuntimeCandidateAttemptRecord) bool {
	for _, value := range values {
		if value.State == RuntimeCandidateAttemptStatePending {
			return true
		}
	}
	return false
}

// hasUnreducedCurrentCandidateRetryFailure identifies the only illegal
// half-closed retry state: attempt 1 failed while the same authority still
// requests its original candidate. A valid exhaustion or repair transition
// must re-reduce facts and therefore seal a different authority decision.
func hasUnreducedCurrentCandidateRetryFailure(decision RuntimeDecision, values []RuntimeCandidateAttemptRecord) bool {
	current := decision.AuthorityTransaction()
	for _, value := range values {
		if value.Attempt == 1 && value.State == RuntimeCandidateAttemptStateFailed && SameAuthorityContract(value.Transaction, current) {
			return true
		}
	}
	return false
}

func ValidateRuntimeCandidateAttemptRecords(values []RuntimeCandidateAttemptRecord) error {
	seen := map[string]struct{}{}
	bySnapshot := map[string][]RuntimeCandidateAttemptRecord{}
	for _, value := range values {
		if value.Attempt < 0 || value.Attempt > 1 || strings.TrimSpace(value.Transaction.AuthoritySnapshotID) == "" || strings.TrimSpace(value.StablePrefixHash) == "" || strings.TrimSpace(value.ManifestHash) == "" || strings.TrimSpace(value.LogicalRequestID) == "" {
			return fmt.Errorf("runtime candidate attempt requires sealed transaction, stable prefix, manifest, request session and attempt 0 or 1")
		}
		key := value.Transaction.AuthoritySnapshotID + "|" + fmt.Sprintf("%d", value.Attempt)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("runtime candidate attempt %s is duplicated", key)
		}
		seen[key] = struct{}{}
		bySnapshot[value.Transaction.AuthoritySnapshotID] = append(bySnapshot[value.Transaction.AuthoritySnapshotID], value)
		switch value.State {
		case RuntimeCandidateAttemptStatePending, RuntimeCandidateAttemptStateAccepted:
			if value.Failure != nil || value.Stale != nil || value.PrerequisiteRequest != nil || value.OwnerFeasibility != nil {
				return fmt.Errorf("runtime candidate attempt %s state %s cannot carry failure", key, value.State)
			}
			if value.State == RuntimeCandidateAttemptStateAccepted && value.Proposal != nil {
				return fmt.Errorf("accepted runtime candidate attempt %s cannot retain an unsealed proposal", key)
			}
			if value.State == RuntimeCandidateAttemptStateAccepted && len(value.Feasibilities) > 0 {
				return fmt.Errorf("accepted runtime candidate attempt %s cannot retain unsealed feasibility records", key)
			}
		case RuntimeCandidateAttemptStateFailed:
			if value.Failure == nil || value.Failure.Class == "" || strings.TrimSpace(value.Failure.Reason) == "" {
				return fmt.Errorf("failed runtime candidate attempt %s requires typed failure", key)
			}
			if value.Stale != nil || value.PrerequisiteRequest != nil || value.OwnerFeasibility != nil {
				return fmt.Errorf("failed runtime candidate attempt %s cannot carry freshness stale", key)
			}
		case RuntimeCandidateAttemptStateInvalidated:
			reasonCount := 0
			if value.Stale != nil {
				reasonCount++
			}
			if value.PrerequisiteRequest != nil {
				reasonCount++
			}
			if value.OwnerFeasibility != nil {
				reasonCount++
			}
			if value.Failure != nil || value.Proposal != nil || len(value.Feasibilities) != 0 || reasonCount != 1 {
				return fmt.Errorf("invalidated runtime candidate attempt %s requires exactly one typed invalidation reason", key)
			}
			if value.Stale != nil {
				if err := value.Stale.Validate(); err != nil {
					return fmt.Errorf("invalidated runtime candidate attempt %s stale: %w", key, err)
				}
			}
			if value.PrerequisiteRequest != nil {
				if err := value.PrerequisiteRequest.Validate(); err != nil {
					return fmt.Errorf("invalidated runtime candidate attempt %s prerequisite request: %w", key, err)
				}
			}
			if value.OwnerFeasibility != nil {
				if err := value.OwnerFeasibility.Validate(); err != nil {
					return fmt.Errorf("invalidated runtime candidate attempt %s owner feasibility: %w", key, err)
				}
				if value.OwnerFeasibility.Outcome != RuntimeOwnerFeasibilityInfeasible && value.OwnerFeasibility.Outcome != RuntimeOwnerFeasibilityContractDefect {
					return fmt.Errorf("invalidated runtime candidate attempt %s owner feasibility must be infeasible or owner contract defect", key)
				}
			}
		default:
			return fmt.Errorf("runtime candidate attempt %s has invalid state %q", key, value.State)
		}
		if err := ValidateRuntimeCandidateFeasibilityRecords(value.Feasibilities); err != nil {
			return fmt.Errorf("runtime candidate attempt %s feasibility records: %w", key, err)
		}
	}
	for snapshotID, attempts := range bySnapshot {
		first, hasFirst := runtimeCandidateAttemptByNumber(attempts, 0)
		second, hasSecond := runtimeCandidateAttemptByNumber(attempts, 1)
		if hasSecond && hasFirst && (first.State != RuntimeCandidateAttemptStateFailed || first.Transaction != second.Transaction || first.StablePrefixHash != second.StablePrefixHash || first.ManifestHash != second.ManifestHash) {
			return fmt.Errorf("runtime candidate snapshot %s attempt 1 is not a same-contract retry", snapshotID)
		}
		if hasFirst && first.State == RuntimeCandidateAttemptStateAccepted && hasSecond {
			return fmt.Errorf("runtime candidate snapshot %s retried after accepted attempt 0", snapshotID)
		}
	}
	return nil
}

// ValidateRuntimeCandidateFeasibilityRecords enforces the durable admission
// ledger's one-result-per-operation rule. It intentionally validates only
// read-only probe facts; candidate/static validation remains the seal path's
// responsibility.
func ValidateRuntimeCandidateFeasibilityRecords(values []RuntimeCandidateFeasibilityRecord) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		operationID := strings.TrimSpace(value.OperationID)
		if operationID == "" || strings.TrimSpace(value.AdmissibilitySnapshotID) == "" {
			return fmt.Errorf("runtime candidate feasibility record requires operation and snapshot identity")
		}
		if _, exists := seen[operationID]; exists {
			return fmt.Errorf("runtime candidate feasibility operation %q is duplicated", operationID)
		}
		seen[operationID] = struct{}{}
		if value.Observation == nil {
			return fmt.Errorf("runtime candidate operation %q requires an owner feasibility observation", operationID)
		}
		if err := value.Observation.Validate(); err != nil {
			return fmt.Errorf("runtime candidate operation %q owner feasibility observation: %w", operationID, err)
		}
		if value.Observation.Outcome == RuntimeOwnerFeasibilityFeasible {
			if value.ReplayProof != nil {
				proof := value.ReplayProof.Normalized()
				if err := proof.Validate(); err != nil {
					return fmt.Errorf("feasible runtime candidate operation %q replay proof: %w", operationID, err)
				}
				if proof.OperationID != operationID || proof.AdmissibilitySnapshotID != strings.TrimSpace(value.AdmissibilitySnapshotID) || proof.AcquisitionBinding != strings.TrimSpace(value.AcquisitionBinding) {
					return fmt.Errorf("feasible runtime candidate operation %q replay proof identity mismatch", operationID)
				}
			}
			continue
		}
		if value.ReplayProof != nil {
			return fmt.Errorf("infeasible runtime candidate operation %q cannot carry replay proof", operationID)
		}
	}
	return nil
}

// ValidateRuntimeCandidateAttemptRecordsForScope validates every persisted record
// against its explicit stable authority contract identity.
func ValidateRuntimeCandidateAttemptRecordsForScope(workspaceID common.WorkspaceID, runID common.RunID, values []RuntimeCandidateAttemptRecord) error {
	if err := ValidateRuntimeCandidateAttemptRecords(values); err != nil {
		return err
	}
	byContract := map[string][]RuntimeCandidateAttemptRecord{}
	for _, value := range values {
		if strings.TrimSpace(value.AuthorityContractID) == "" {
			return fmt.Errorf("runtime candidate attempt requires authority contract id")
		}
		contractID := CandidateAttemptAuthorityContractID(workspaceID, runID, value)
		byContract[contractID] = append(byContract[contractID], value)
		if value.AuthorityContractID != contractID || value.AuthorityContractID != StableAuthorityContractID(workspaceID, runID, value.Transaction) {
			return fmt.Errorf("runtime candidate authority contract id mismatch")
		}
	}
	for contractID, attempts := range byContract {
		attemptCounts := [2]int{}
		for _, attempt := range attempts {
			attemptCounts[attempt.Attempt]++
		}
		first, hasFirst := runtimeCandidateAttemptByNumber(attempts, 0)
		second, hasSecond := runtimeCandidateAttemptByNumber(attempts, 1)
		if len(attempts) > 2 || attemptCounts[0] > 1 || attemptCounts[1] > 1 || (hasSecond && (!hasFirst || first.State != RuntimeCandidateAttemptStateFailed || !SameAuthorityContract(first.Transaction, second.Transaction) || first.StablePrefixHash != second.StablePrefixHash || first.ManifestHash != second.ManifestHash)) {
			return fmt.Errorf("runtime candidate authority contract %s exceeds the sealed attempt 0/1 transaction", contractID)
		}
		if hasFirst && first.State == RuntimeCandidateAttemptStateAccepted && hasSecond {
			return fmt.Errorf("runtime candidate authority contract %s retried after accepted attempt 0", contractID)
		}
	}
	return nil
}

// StableAuthorityContractID identifies one sealed candidate retry contract.
// AuthoritySnapshotID is intentionally part of the identity: an SM-2 owner
// fact (including an infeasible capability observation) may produce a fresh
// authority with otherwise identical focus/action/output hashes. Reusing the
// former contract would either overwrite its immutable invalidated lineage or
// incorrectly deny the new authority its own attempt 0.
func StableAuthorityContractID(workspaceID common.WorkspaceID, runID common.RunID, transaction AuthorityTransaction) string {
	h := sha256.New()
	writeHashString(h, string(workspaceID))
	writeHashString(h, string(runID))
	writeHashString(h, transaction.AuthoritySnapshotID)
	writeHashString(h, transaction.FocusID)
	writeHashString(h, string(transaction.State))
	writeHashString(h, transaction.RequiredOutputHash)
	writeHashString(h, transaction.DefinitionRevision)
	writeHashString(h, transaction.EvidenceRevision)
	writeHashString(h, transaction.CapabilityStateHash)
	if transaction.PrerequisiteFreshnessHash != "" {
		writeHashString(h, transaction.PrerequisiteFreshnessHash)
	}
	writeHashString(h, transaction.OperationContractRevision)
	return "authority-contract-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func SameAuthorityContract(left AuthorityTransaction, right AuthorityTransaction) bool {
	return left.AuthoritySnapshotID == right.AuthoritySnapshotID &&
		left.FocusID == right.FocusID &&
		left.State == right.State &&
		left.RequiredOutputHash == right.RequiredOutputHash &&
		left.DefinitionRevision == right.DefinitionRevision &&
		left.EvidenceRevision == right.EvidenceRevision &&
		left.CapabilityStateHash == right.CapabilityStateHash &&
		left.PrerequisiteFreshnessHash == right.PrerequisiteFreshnessHash &&
		left.OperationContractRevision == right.OperationContractRevision
}

func CandidateAttemptAuthorityContractID(workspaceID common.WorkspaceID, runID common.RunID, record RuntimeCandidateAttemptRecord) string {
	return strings.TrimSpace(record.AuthorityContractID)
}

func runtimeCandidateAttemptByNumber(values []RuntimeCandidateAttemptRecord, attempt int) (RuntimeCandidateAttemptRecord, bool) {
	for _, value := range values {
		if value.Attempt == attempt {
			return value, true
		}
	}
	return RuntimeCandidateAttemptRecord{}, false
}

func (r RuntimeStateRecord) Clone() RuntimeStateRecord {
	ret := r
	ret.Definition = r.Definition.Clone()
	ret.Facts = r.Facts.Normalized()
	ret.SemanticPlanning = r.SemanticPlanning.Clone()
	ret.Reconciliation = r.Reconciliation.Clone()
	ret.Decision = cloneRuntimeDecision(r.Decision)
	if r.ActiveDispatch != nil {
		value := r.ActiveDispatch.Clone()
		ret.ActiveDispatch = &value
	}
	ret.CompletedDispatches = make([]RuntimeDispatchRecord, len(r.CompletedDispatches))
	for idx := range r.CompletedDispatches {
		ret.CompletedDispatches[idx] = r.CompletedDispatches[idx].Clone()
	}
	ret.ProcessedEventIDs = append([]string(nil), r.ProcessedEventIDs...)
	ret.Diagnostics = append([]RuntimeDiagnosticRecord(nil), r.Diagnostics...)
	ret.CandidateRequestSessions = make([]RuntimeCandidateRequestSession, len(r.CandidateRequestSessions))
	for idx := range r.CandidateRequestSessions {
		ret.CandidateRequestSessions[idx] = r.CandidateRequestSessions[idx].Clone()
	}
	ret.CandidateAttempts = make([]RuntimeCandidateAttemptRecord, len(r.CandidateAttempts))
	for idx, attempt := range r.CandidateAttempts {
		ret.CandidateAttempts[idx] = attempt
		if attempt.Proposal != nil {
			proposal := cloneRuntimeCandidate(*attempt.Proposal)
			ret.CandidateAttempts[idx].Proposal = &proposal
		}
		ret.CandidateAttempts[idx].Feasibilities = cloneRuntimeCandidateFeasibilities(attempt.Feasibilities)
		if attempt.Failure != nil {
			failure := *attempt.Failure
			failure.TargetRefs = append([]string(nil), attempt.Failure.TargetRefs...)
			failure.Diagnostic = attempt.Failure.Diagnostic.Clone()
			if attempt.Failure.ValidationDiagnostic != nil {
				diagnostic := attempt.Failure.ValidationDiagnostic.Clone()
				failure.ValidationDiagnostic = &diagnostic
			}
			ret.CandidateAttempts[idx].Failure = &failure
		}
		if attempt.Stale != nil {
			stale := *attempt.Stale
			ret.CandidateAttempts[idx].Stale = &stale
		}
		if attempt.PrerequisiteRequest != nil {
			request := attempt.PrerequisiteRequest.Normalized()
			ret.CandidateAttempts[idx].PrerequisiteRequest = &request
		}
		if attempt.OwnerFeasibility != nil {
			observation := attempt.OwnerFeasibility.Clone()
			ret.CandidateAttempts[idx].OwnerFeasibility = &observation
		}
	}
	ret.GovernanceRequests = append([]RuntimeGovernanceRequestRecord(nil), r.GovernanceRequests...)
	ret.GovernanceResolutions = append([]RuntimeGovernanceResolutionRecord(nil), r.GovernanceResolutions...)
	ret.OperationGovernanceBindings = make([]RuntimeOperationGovernanceBinding, len(r.OperationGovernanceBindings))
	for idx := range r.OperationGovernanceBindings {
		ret.OperationGovernanceBindings[idx] = r.OperationGovernanceBindings[idx].Clone()
	}
	ret.OperationReceipts = NormalizeRuntimeOperationReceipts(r.OperationReceipts)
	ret.DeliveryAcknowledgements = append([]RuntimeDeliveryAcknowledgement(nil), r.DeliveryAcknowledgements...)
	ret.PrerequisiteInvalidations = NormalizeRuntimePrerequisiteInvalidations(r.PrerequisiteInvalidations)
	if r.DriveLease != nil {
		lease := r.DriveLease.Clone()
		ret.DriveLease = &lease
	}
	if r.Termination != nil {
		term := r.Termination.Clone()
		ret.Termination = &term
	}
	return ret
}

func cloneRuntimeCandidateFeasibilities(values []RuntimeCandidateFeasibilityRecord) []RuntimeCandidateFeasibilityRecord {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeCandidateFeasibilityRecord, len(values))
	for idx, value := range values {
		ret[idx] = value
		if value.Observation != nil {
			observation := value.Observation.Clone()
			ret[idx].Observation = &observation
		}
		if value.ReplayProof != nil {
			proof := value.ReplayProof.Clone()
			ret[idx].ReplayProof = &proof
		}
	}
	return ret
}

func cloneRuntimeDecision(decision RuntimeDecision) RuntimeDecision {
	ret := decision
	ret.Focus = decision.Focus.Normalized()
	ret.RequiredOutput = decision.RequiredOutput.Normalized()
	ret.Prerequisites = append([]RuntimePrerequisite(nil), decision.Prerequisites...)
	for idx := range ret.Prerequisites {
		ret.Prerequisites[idx].RequiredCapabilities = append([]string(nil), decision.Prerequisites[idx].RequiredCapabilities...)
	}
	if decision.Terminal != nil {
		value := *decision.Terminal
		ret.Terminal = &value
	}
	return ret
}

func (d RuntimeDispatchRecord) Validate(decision RuntimeDecision) error {
	return d.ValidateWithActivation(decision, RuntimeActivationContext{})
}

func (d RuntimeDispatchRecord) ValidateWithActivation(decision RuntimeDecision, activation RuntimeActivationContext) error {
	if strings.TrimSpace(d.ID) == "" || d.WorkspaceID == "" || d.RunID == "" || strings.TrimSpace(d.CandidateHash) == "" {
		return fmt.Errorf("runtime dispatch requires id, scope and candidate hash")
	}
	if err := ValidateAuthorityTransaction(decision, d.Transaction); err != nil {
		return err
	}
	if d.WorkspaceID != decision.AuthoritySnapshot.WorkspaceID || d.RunID != decision.AuthoritySnapshot.RunID {
		return fmt.Errorf("runtime dispatch scope mismatch")
	}
	if err := d.Candidate.ValidateWithActivation(decision, activation); err != nil {
		return err
	}
	if d.CandidateHash != StableRuntimeCandidateHash(d.Candidate) {
		return fmt.Errorf("runtime dispatch candidate hash mismatch")
	}
	if err := ValidateRuntimeCandidateManagedValidationRecipeSnapshot(d.Candidate, d.ValidationRecipes); err != nil {
		return fmt.Errorf("runtime dispatch validation recipe snapshot: %w", err)
	}
	if d.State != RuntimeDispatchStateSealed && d.State != RuntimeDispatchStateCommitted && d.State != RuntimeDispatchStateInvalidated {
		return fmt.Errorf("runtime dispatch has invalid state %q", d.State)
	}
	available := runtimeCandidateOperationIDs(d.Candidate)
	seen := map[string]struct{}{}
	for _, operationID := range d.CompletedOperationIDs {
		operationID = strings.TrimSpace(operationID)
		if _, ok := available[operationID]; !ok {
			return fmt.Errorf("runtime dispatch completed operation %q is not present in candidate", operationID)
		}
		if _, ok := seen[operationID]; ok {
			return fmt.Errorf("runtime dispatch completed operation %q is duplicated", operationID)
		}
		seen[operationID] = struct{}{}
	}
	if err := ValidateRuntimeOperationExecutionRecords(d.OperationExecutions); err != nil {
		return err
	}
	for _, exec := range NormalizeRuntimeOperationExecutionRecords(d.OperationExecutions) {
		if _, ok := available[exec.OperationID]; !ok {
			return fmt.Errorf("runtime dispatch operation execution %q is not present in candidate", exec.OperationID)
		}
		if exec.ReplayProof != nil {
			operation, found := runtimeCandidateOperationForID(d.Candidate, exec.OperationID)
			if !found {
				return fmt.Errorf("runtime dispatch replay proof operation %q is not present in candidate", exec.OperationID)
			}
			descriptor, known := RuntimeOperationDescriptorForKind(operation.Kind)
			proof := exec.ReplayProof.Normalized()
			if !known || proof.OwnerModuleID != descriptor.OwnerModuleID || !sameRuntimeStringSet(proof.TargetRefs, operation.TargetRefs) {
				return fmt.Errorf("runtime dispatch replay proof does not match sealed operation %q", exec.OperationID)
			}
		}
	}
	return nil
}

func (d RuntimeDispatchRecord) Clone() RuntimeDispatchRecord {
	ret := d
	ret.Candidate = cloneRuntimeCandidate(d.Candidate)
	ret.ValidationRecipes = NormalizeRuntimeValidationRecipes(d.ValidationRecipes)
	ret.CompletedOperationIDs = append([]string(nil), d.CompletedOperationIDs...)
	ret.OperationExecutions = NormalizeRuntimeOperationExecutionRecords(d.OperationExecutions)
	if len(ret.OperationExecutions) > 0 {
		execs := make([]RuntimeOperationExecutionRecord, len(ret.OperationExecutions))
		for idx := range ret.OperationExecutions {
			execs[idx] = ret.OperationExecutions[idx].Clone()
		}
		ret.OperationExecutions = execs
	}
	return ret
}

func cloneRuntimeCandidate(candidate RuntimeCandidate) RuntimeCandidate {
	ret := RuntimeCandidate{}
	if candidate.StateChange != nil {
		value := StateChangeCandidate{Operations: make([]StateChangingOperation, len(candidate.StateChange.Operations))}
		for idx, operation := range candidate.StateChange.Operations {
			value.Operations[idx] = StateChangingOperation{Operation: cloneRuntimeOperation(operation.Operation)}
		}
		ret.StateChange = &value
	}
	if candidate.Validation != nil {
		value := ValidationCandidate{Operations: make([]ValidationOperation, len(candidate.Validation.Operations))}
		for idx, operation := range candidate.Validation.Operations {
			value.Operations[idx] = ValidationOperation{Operation: cloneRuntimeOperation(operation.Operation)}
		}
		ret.Validation = &value
	}
	if candidate.Assessment != nil {
		value := candidate.Assessment.Normalized()
		ret.Assessment = &value
	}
	if candidate.Governance != nil {
		value := *candidate.Governance
		ret.Governance = &value
	}
	return ret
}

func cloneRuntimeOperation(operation RuntimeOperation) RuntimeOperation {
	ret := operation.Normalized()
	ret.Parameters = make([]OperationParameter, len(operation.Parameters))
	for idx, parameter := range operation.Parameters {
		ret.Parameters[idx] = parameter
		ret.Parameters[idx].StringList = append([]string(nil), parameter.StringList...)
		if parameter.String != nil {
			value := *parameter.String
			ret.Parameters[idx].String = &value
		}
		if parameter.Integer != nil {
			value := *parameter.Integer
			ret.Parameters[idx].Integer = &value
		}
		if parameter.Boolean != nil {
			value := *parameter.Boolean
			ret.Parameters[idx].Boolean = &value
		}
	}
	return ret
}

func runtimeCandidateOperationIDs(candidate RuntimeCandidate) map[string]struct{} {
	ret := map[string]struct{}{}
	switch {
	case candidate.StateChange != nil:
		for _, operation := range candidate.StateChange.Operations {
			ret[operation.Operation.Normalized().ID] = struct{}{}
		}
	case candidate.Validation != nil:
		for _, operation := range candidate.Validation.Operations {
			ret[operation.Operation.Normalized().ID] = struct{}{}
		}
	}
	return ret
}

func runtimeCandidateOperationForID(candidate RuntimeCandidate, wanted string) (RuntimeOperation, bool) {
	wanted = strings.TrimSpace(wanted)
	if candidate.StateChange != nil {
		for _, item := range candidate.StateChange.Operations {
			operation := item.Operation.Normalized()
			if operation.ID == wanted {
				return operation, true
			}
		}
	}
	if candidate.Validation != nil {
		for _, item := range candidate.Validation.Operations {
			operation := item.Operation.Normalized()
			if operation.ID == wanted {
				return operation, true
			}
		}
	}
	return RuntimeOperation{}, false
}

func sameRuntimeStringSet(left, right []string) bool {
	left = normalizeStrings(left)
	right = normalizeStrings(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}
