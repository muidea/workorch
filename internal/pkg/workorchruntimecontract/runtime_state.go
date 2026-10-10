package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
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
	// CandidateManifestHash is the exact SM-3 manifest identity whose target
	// authority was projected into OperationTargetAuthorizations. It is retained
	// across recovery/governance continuation and never rebuilt from current facts.
	CandidateManifestHash string
	// ImplementationWorkCommitment is the immutable pre-dispatch round/binding
	// snapshot; dispatch/progress/completion fields inside it remain empty.
	ImplementationWorkCommitment *RuntimeImplementationWorkRound
	// CoverageBindings is the complete immutable binding projection selected by
	// the exact candidate manifest.  Evidence adjudication and recovery consume
	// this dispatch-local snapshot, never mutable current Focus state.
	CoverageBindingHash string
	CoverageBindings    []CoverageEvidenceBinding
	// OperationTargetAuthorizations is the minimal per-operation target-policy
	// snapshot derived at final manifest-bound seal.
	OperationTargetAuthorizations []RuntimeDispatchOperationTargetAuthorization
	// FocusTargetRole is the exact SM-2 focus role used when SM-3 sealed the
	// dispatch. Capability requests and later diagnostic-causation checks use
	// this immutable value rather than the mutable current focus projection.
	FocusTargetRole string
	// ValidationRecipes is the exact manifest-selected, Shell-owner-confirmed
	// recipe snapshot for this dispatch.  It is a sealed operation contract,
	// not activation policy and not mutable current owner inventory.
	ValidationRecipes []RuntimeValidationRecipe
	// RepairCommitment is present only when the accepted candidate was admitted
	// as the exact resource-owner repair of a confirmed rejected-validation surface. It is
	// immutable across execution progress and governance resume.
	RepairCommitmentHash string
	RepairCommitment     *RuntimeDispatchRepairCommitment
	// AssessmentAuthorityInput is the assessment-only pre-seal classifier
	// input. AssessmentAdjudication is its post-seal, commit-time proof and is
	// deliberately excluded from DispatchAuthorityHash.
	AssessmentAuthorityInputHash string
	AssessmentAuthorityInput     *RuntimeAssessmentAuthorityInput
	AssessmentAdjudicationHash   string
	AssessmentAdjudication       *RuntimeAssessmentAdjudication
	// OperationAdmissibilities and OperationAdmissibilityObservations are the
	// complete owner-authored snapshot preimages selected by SM-3. Executions
	// refer to them by SnapshotID/AcquisitionBinding; recovery and owner calls
	// must never replace them with the mutable current Runtime facts.
	OperationAdmissibilities           []RuntimeOperationAdmissibility
	OperationAdmissibilityObservations []RuntimePrerequisiteObservation
	// OperationPolicies contains only exact activation fallback policies used
	// by operations that did not require an owner admissibility snapshot. Static
	// operation-contract resolution is represented by the absence of both.
	OperationPolicies     []RuntimeOperationPolicyFact
	State                 RuntimeDispatchState
	CompletedOperationIDs []string
	// OperationExecutions is the required per-operation claim/fence ledger.
	OperationExecutions    []RuntimeOperationExecutionRecord
	OperationAuthorityHash string
	// DispatchAuthorityHash is the immutable SM-3 seal over the authority
	// transaction, complete candidate, selected recipe bodies and exact
	// per-operation authority set. Mutable execution progress is excluded.
	DispatchAuthorityHash string
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
	// CandidateManifestHash binds the owner observation to the exact sealed
	// proposal vocabulary presented during the side-effect-free probe. It is
	// copied from the owner response and cannot be reconstructed from current
	// catalog state during restart.
	CandidateManifestHash string
	// Observation is the sole owner-authored factual result. It is separate from
	// CandidateAttempt.Failure: only the explicit, manifest-bound
	// candidate_constraint_violated member may be projected by Orchestrator into
	// the bounded SM-3 correction matrix; resource infeasibility never may.
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
	// Accepted attempts retain the exact immutable dispatch seal they produced.
	// Pending/failed/invalidated attempts must not carry these fields.
	CandidateHash string
	DispatchID    string
	// TargetAuthorizationHash is the accepted admission's independent
	// commitment to AdmissionTargetAuthorizations. The projection is derived
	// directly from the exact manifest and candidate before dispatch
	// construction; it is never copied back from a persisted dispatch. This
	// keeps fresh-load admission lineage independent from a coordinated rewrite
	// of the dispatch projection and its self-seal.
	TargetAuthorizationHash       string
	AdmissionTargetAuthorizations []RuntimeDispatchOperationTargetAuthorization
	// AdmissionCoverageBindings is the independently persisted pre-dispatch
	// commitment derived from the exact candidate manifest.  It prevents a
	// coordinated rewrite of a dispatch binding snapshot and its self hash.
	AdmissionCoverageBindingHash string
	AdmissionCoverageBindings    []CoverageEvidenceBinding
	// ValidationRecipeHash is the accepted admission's independent commitment
	// to AdmissionValidationRecipes. Managed-validation recipes are projected
	// directly from the exact manifest and candidate before dispatch
	// construction; a coordinated dispatch recipe/self-seal rewrite therefore
	// cannot rewrite its admission lineage. Non-managed candidates carry neither
	// field.
	ValidationRecipeHash       string
	AdmissionValidationRecipes []RuntimeValidationRecipe
	// AdmissionRepairCommitment is the accepted admission's independent copy of
	// the dispatch repair purpose.  It cannot be reconstructed from a later
	// mutable FailureTarget, RepairDirective or confirmation scan.
	RepairCommitmentHash                  string
	AdmissionRepairCommitment             *RuntimeDispatchRepairCommitment
	AdmissionImplementationWorkCommitment *RuntimeImplementationWorkRound
	DispatchAuthorityHash                 string
	// Proposal is the strictly decoded, unsealed candidate persisted after
	// provider generation. A failed attempt may retain a manifest-rejected value
	// solely as its finite correction frontier; only a pending proposal may be
	// replayed into feasibility, and neither form is dispatch authority.
	Proposal *RuntimeCandidate
	// Conformances are side-effect-free capability-owner facts for sealed
	// candidate parameter constraints. They are independent of ordinary
	// feasibility and cannot authorize dispatch or choose retry policy.
	Conformances  []RuntimeCandidateConformanceRecord
	Feasibilities []RuntimeCandidateFeasibilityRecord
	// FeasibilityCatalogs are the exact owner catalog preimages retained by the
	// Orchestrator when a failed attempt keeps probe facts. They prove historical
	// rejection lineage after current resources change; they are never current
	// feasibility or dispatch authority and cannot be supplied by a probe result.
	FeasibilityCatalogs []RuntimeOperationAdmissibility
	Failure             *RuntimeCandidateFailure
	// Correction is the manifest-bound SM-3 disposition sealed together with a
	// failed attempt. A correctable_once value must carry a concrete semantic
	// witness; attempt 1 may never be inferred again from Failure reason text.
	Correction *RuntimeCandidateCorrectionDirective
	Stale      *RuntimePrerequisiteStale
	// OwnerFeasibility is immutable owner fact lineage for an invalidated
	// proposal. It is never a candidate failure and therefore cannot authorize
	// attempt 1.
	OwnerFeasibility *RuntimeOwnerFeasibilityObservation
	// BindingContradiction is the exact SM-2 retirement fact derived from an
	// owner infeasibility. It carries no retry or next-action choice.
	BindingContradiction *RuntimeBindingContradictionIdentity
	// PrerequisiteRequest records why this unsealed attempt was invalidated for
	// owner fact acquisition, making duplicate delivery/restart deterministic.
	PrerequisiteRequest *RuntimeOperationPrerequisiteRequest
}

// Normalized returns the canonical scalar identity and accepted admission
// target/validation-recipe projections. It deliberately does not reinterpret
// candidate or owner facts: their dedicated clone/validation contracts remain
// authoritative.
func (r RuntimeCandidateAttemptRecord) Normalized() RuntimeCandidateAttemptRecord {
	r.AuthorityContractID = strings.TrimSpace(r.AuthorityContractID)
	r.StablePrefixHash = strings.TrimSpace(r.StablePrefixHash)
	r.ManifestHash = strings.TrimSpace(r.ManifestHash)
	r.LogicalRequestID = strings.TrimSpace(r.LogicalRequestID)
	r.State = RuntimeCandidateAttemptState(strings.TrimSpace(string(r.State)))
	r.CandidateHash = strings.TrimSpace(r.CandidateHash)
	r.DispatchID = strings.TrimSpace(r.DispatchID)
	r.TargetAuthorizationHash = strings.TrimSpace(r.TargetAuthorizationHash)
	r.AdmissionTargetAuthorizations = NormalizeRuntimeDispatchOperationTargetAuthorizations(r.AdmissionTargetAuthorizations)
	r.AdmissionCoverageBindingHash = strings.TrimSpace(r.AdmissionCoverageBindingHash)
	r.AdmissionCoverageBindings = NormalizeCoverageEvidenceBindings(r.AdmissionCoverageBindings)
	r.ValidationRecipeHash = strings.TrimSpace(r.ValidationRecipeHash)
	r.AdmissionValidationRecipes = NormalizeRuntimeValidationRecipes(r.AdmissionValidationRecipes)
	r.RepairCommitmentHash = strings.TrimSpace(r.RepairCommitmentHash)
	if r.AdmissionRepairCommitment != nil {
		value := r.AdmissionRepairCommitment.Normalized()
		r.AdmissionRepairCommitment = &value
	}
	if r.AdmissionImplementationWorkCommitment != nil {
		value := r.AdmissionImplementationWorkCommitment.Clone()
		r.AdmissionImplementationWorkCommitment = &value
	}
	r.DispatchAuthorityHash = strings.TrimSpace(r.DispatchAuthorityHash)
	return r
}

// Clone detaches every nested candidate-attempt value, including the
// accepted-only target and validation-recipe admission projections used during
// fresh load.
func (r RuntimeCandidateAttemptRecord) Clone() RuntimeCandidateAttemptRecord {
	r = r.Normalized()
	r.AdmissionTargetAuthorizations = CloneRuntimeDispatchOperationTargetAuthorizations(r.AdmissionTargetAuthorizations)
	r.AdmissionCoverageBindings = CloneCoverageEvidenceBindings(r.AdmissionCoverageBindings)
	r.AdmissionValidationRecipes = CloneRuntimeValidationRecipes(r.AdmissionValidationRecipes)
	if r.AdmissionRepairCommitment != nil {
		value := r.AdmissionRepairCommitment.Clone()
		r.AdmissionRepairCommitment = &value
	}
	if r.Proposal != nil {
		proposal := cloneRuntimeCandidate(*r.Proposal)
		r.Proposal = &proposal
	}
	r.Feasibilities = cloneRuntimeCandidateFeasibilities(r.Feasibilities)
	r.FeasibilityCatalogs = cloneRuntimeStateRawSlice(r.FeasibilityCatalogs, cloneRuntimeOperationAdmissibilityRaw)
	conformances := make([]RuntimeCandidateConformanceRecord, 0, len(r.Conformances))
	for _, value := range r.Conformances {
		conformances = append(conformances, value.Clone())
	}
	r.Conformances = conformances
	if r.Failure != nil {
		failure := *r.Failure
		failure.TargetRefs = append([]string(nil), r.Failure.TargetRefs...)
		failure.Diagnostic = r.Failure.Diagnostic.Clone()
		if r.Failure.ValidationDiagnostic != nil {
			diagnostic := r.Failure.ValidationDiagnostic.Clone()
			failure.ValidationDiagnostic = &diagnostic
		}
		r.Failure = &failure
	}
	if r.Correction != nil {
		r.Correction = r.Correction.Clone()
	}
	if r.Stale != nil {
		stale := r.Stale.Normalized()
		r.Stale = &stale
	}
	if r.PrerequisiteRequest != nil {
		request := r.PrerequisiteRequest.Normalized()
		r.PrerequisiteRequest = &request
	}
	if r.OwnerFeasibility != nil {
		observation := r.OwnerFeasibility.Clone()
		r.OwnerFeasibility = &observation
	}
	if r.BindingContradiction != nil {
		identity := r.BindingContradiction.Normalized()
		r.BindingContradiction = &identity
	}
	return r
}

// RuntimeStateRecord 是 Runtime 在 Run owner 中持久化的唯一正式状态。
type RuntimeStateRecord struct {
	ResultFeedbackVersion uint32 `json:",omitempty"`
	StateRevision         uint64
	EvidenceSequence      uint64
	Definition            RuntimeDefinitionContext
	Facts                 RuntimeFacts
	// SemanticPlanning is the sole persisted semantic-plan request/proposal/
	// feasibility/seal ledger. Run stores it opaquely with the aggregate.
	SemanticPlanning RuntimeSemanticPlanningLedger
	// ImplementationWork is the bounded Orchestrator-owned work transaction
	// lineage. Run stores it opaquely with the one Runtime aggregate.
	ImplementationWork RuntimeImplementationWorkLedger
	// Reconciliation is the Orchestrator-owned SM-2 request/batch lineage. It
	// is kept separate from Facts so owner observations never become coverage
	// credit by deserialization alone.
	Reconciliation RuntimeCWIReconciliationLedger
	// CoverageProofLineage is the aggregate-local SM-2 receipt-to-reseal audit
	// ledger. Run persists it opaquely; only Orchestrator may resolve an old
	// accepted proof against a newly sealed binding.
	CoverageProofLineage []RuntimeCoverageProofLineage
	Decision             RuntimeDecision
	ActiveDispatch       *RuntimeDispatchRecord
	CompletedDispatches  []RuntimeDispatchRecord
	ProcessedEventIDs    []string
	// RepairSurfaceConfirmations is the typed SM-2 replay ledger for owner
	// confirmation facts; it is intentionally not folded into ProcessedEventIDs.
	RepairSurfaceConfirmations []RuntimeRepairSurfaceConfirmationRecord
	Diagnostics                []RuntimeDiagnosticRecord
	// CandidateRequestSessions record provider I/O before a complete candidate
	// exists. They are not candidate attempts and cannot authorize a dispatch.
	CandidateRequestSessions []RuntimeCandidateRequestSession
	CandidateAttempts        []RuntimeCandidateAttemptRecord
	// ExecutionBudget is persisted opaquely by Run; only Orchestrator reducers
	// allocate, consume, reconcile or derive its read-only view.
	ExecutionBudget RuntimeExecutionBudgetLedger
	// RecoverableWaits are Orchestrator-owned bounded wait records. Run stores
	// them opaquely; only one record may be active for a Runtime aggregate.
	RecoverableWaits []RuntimeRecoverableWaitRecord
	// OperationRecoveryAuthorizations are exact SM-4 replay permissions. Run
	// persists them opaquely and never interprets an operation recovery counter
	// as execution authority.
	OperationRecoveryAuthorizations []RuntimeOperationRecoveryAuthorization
	// DefinitionClarification is the dedicated persisted gap observation,
	// sealed dispatch and Decision-owner request binding.
	DefinitionClarification     *RuntimeDefinitionClarificationState
	GovernanceRequests          []RuntimeGovernanceRequestRecord
	GovernanceResolutions       []RuntimeGovernanceResolutionRecord
	OperationGovernanceBindings []RuntimeOperationGovernanceBinding
	// OperationReceipts are owner-authored terminal receipts persisted before
	// capability owners return execution results (INV-03/INV-04).
	OperationReceipts []RuntimeOperationReceipt
	// DeliveryAcknowledgements are SM-4-only durable receipts for the terminal
	// Result -> Workspace -> Run saga. They never alter the sealed SM-2 decision.
	DeliveryAcknowledgements []RuntimeDeliveryAcknowledgement
	// TerminalMaterialSeal 是 SM-4 首次交付材料的不可变前像，Run 仅 opaque 保存。
	TerminalMaterialSeal *RuntimeTerminalMaterialSeal
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

func (r RuntimeStateRecord) Validate() error {
	if err := ValidateRuntimeResultFeedbackVersion(r.ResultFeedbackVersion); err != nil {
		return err
	}
	if r.Facts.WorkspaceID == "" || r.Facts.RunID == "" {
		return fmt.Errorf("runtime state requires workspace and run scope")
	}
	if r.Decision.RequiredOutput.ImplementationWorkBindingHash != "" {
		matches := 0
		for _, round := range r.ImplementationWork.Rounds {
			if round.WorkingBinding != nil && round.WorkingBinding.Hash == r.Decision.RequiredOutput.ImplementationWorkBindingHash && round.CompletedReceiptSetHash == "" && round.FailedReceiptHash == "" && round.GovernanceReceiptHash == "" && len(r.Facts.Focus.CoverageWorkItemIDs) == 1 && r.Facts.Focus.CoverageWorkItemIDs[0] == round.CoverageWorkItemID && r.Facts.Failure == nil && r.Facts.RepairDirective == nil {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("runtime state working output has no unique live original semantic work commitment")
		}
	}
	if err := ValidateRuntimeImplementationWorkState(r.Definition, r.Facts, r.ImplementationWork, r.SemanticPlanning, r.ActiveDispatch, r.CompletedDispatches, r.OperationReceipts, r.OperationGovernanceBindings, r.ExecutionBudget); err != nil {
		return fmt.Errorf("runtime state implementation work: %w", err)
	}
	if err := ValidateRuntimeDispatchHistoryShape(r.Facts.WorkspaceID, r.Facts.RunID, r.ActiveDispatch, r.CompletedDispatches); err != nil {
		return fmt.Errorf("runtime state dispatch history: %w", err)
	}
	if err := ValidateRuntimeEvidenceLedgerSequence(r.EvidenceSequence, r.Facts, r.ActiveDispatch, r.CompletedDispatches, r.GovernanceResolutions); err != nil {
		return fmt.Errorf("runtime state evidence ledger: %w", err)
	}
	if err := ValidateRuntimeAssessmentAuthoritySources(r.Facts, r.ActiveDispatch, r.CompletedDispatches); err != nil {
		return fmt.Errorf("runtime state assessment authority: %w", err)
	}
	if err := ValidateRuntimeAssessmentAdjudications(r.Facts, r.CompletedDispatches, r.CoverageProofLineage, RuntimeLineageAssertionSources{Definition: r.Definition, Facts: r.Facts, Work: r.ImplementationWork, Semantic: r.SemanticPlanning, Active: r.ActiveDispatch, Completed: r.CompletedDispatches, Receipts: r.OperationReceipts, Governance: r.OperationGovernanceBindings, Budget: r.ExecutionBudget, EvidenceSequence: r.EvidenceSequence, Resolutions: r.GovernanceResolutions, Attempts: r.CandidateAttempts, Sessions: r.CandidateRequestSessions, ProcessedEventIDs: r.ProcessedEventIDs, ProofLineage: r.CoverageProofLineage, Confirmations: r.RepairSurfaceConfirmations}); err != nil {
		return fmt.Errorf("runtime state assessment adjudication: %w", err)
	}
	if err := ValidateRuntimeDefinitionFocuses(r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses); err != nil {
		return fmt.Errorf("runtime state definition focuses: %w", err)
	}
	if err := ValidateRuntimeStateCWILedger(r.Facts, r.Reconciliation); err != nil {
		return err
	}
	if err := ValidateRuntimeCWIBindingRetirementsAgainstExecution(r.Facts, r.ActiveDispatch, r.CompletedDispatches, r.OperationReceipts); err != nil {
		return err
	}
	if err := r.Facts.Validate(); err != nil {
		return err
	}
	if err := r.SemanticPlanning.Validate(); err != nil {
		return fmt.Errorf("runtime semantic planning ledger: %w", err)
	}
	if err := ValidateRuntimeSemanticProofBatchLimits(r.SemanticPlanning, r.Facts.Activation.ExecutionContract.ExecutionLimits); err != nil {
		return err
	}
	if r.SemanticPlanning.DefinitionSeed.Hash != r.Facts.DefinitionSeed.Hash {
		return fmt.Errorf("runtime semantic planning seed does not match facts")
	}
	if err := r.Reconciliation.Validate(); err != nil {
		return fmt.Errorf("runtime reconciliation ledger: %w", err)
	}
	if err := ValidateRuntimeCoverageProofLineage(r.CoverageProofLineage); err != nil {
		return fmt.Errorf("runtime coverage proof lineage: %w", err)
	}
	if err := r.Definition.Validate(); err != nil {
		return err
	}
	if r.Definition.Normalized().Revision != r.Facts.Normalized().DefinitionRevision {
		return fmt.Errorf("runtime state definition revision mismatch")
	}
	if !SameRuntimeSourceObligationInventory(r.Definition.SourceObligations, r.Facts.DefinitionSeed.SourceObligations) {
		return fmt.Errorf("runtime state Definition and seed source obligations mismatch")
	}
	if r.Facts.DefinitionSeed.SourceObligations != nil && r.Facts.DefinitionSeed.SourceObligations.StartupInputHash != r.Definition.StartupInputHash {
		return fmt.Errorf("runtime state source obligations startup input mismatch")
	}
	if err := r.Decision.Validate(); err != nil {
		return err
	}
	if r.Decision.AuthoritySnapshot.WorkspaceID != r.Facts.WorkspaceID || r.Decision.AuthoritySnapshot.RunID != r.Facts.RunID {
		return fmt.Errorf("runtime state decision scope mismatch")
	}
	if r.ActiveDispatch != nil {
		if err := r.ActiveDispatch.ValidateWithActivation(r.Decision, RuntimeActivationForFacts(r.Facts)); err != nil {
			return fmt.Errorf("runtime state active dispatch: %w", err)
		}
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(r.Facts.WorkspaceID, r.Facts.RunID, r.CandidateAttempts); err != nil {
		return err
	}
	if err := ValidateRuntimeCandidateRequestSessions(r.CandidateRequestSessions); err != nil {
		return err
	}
	if err := r.ExecutionBudget.Validate(); err != nil {
		return fmt.Errorf("runtime execution budget: %w", err)
	}
	if !SameRuntimeExecutionBudgetPolicy(r.ExecutionBudget.Policy, r.Facts.ExecutionBudgetPolicy) {
		return fmt.Errorf("runtime execution budget policy does not match activation facts")
	}
	if r.DefinitionClarification != nil {
		if err := r.DefinitionClarification.Validate(); err != nil {
			return fmt.Errorf("runtime Definition clarification: %w", err)
		}
		clarification := r.DefinitionClarification.Clone()
		if clarification.Observation.WorkspaceID != r.Facts.WorkspaceID || clarification.Observation.RunID != r.Facts.RunID ||
			clarification.Observation.DefinitionRevision != r.Definition.Normalized().Revision || clarification.Observation.StartupInputHash != r.Definition.Normalized().StartupInputHash ||
			r.Decision.State != ConvergenceStateNeedGovernance || !sameStrings(r.Decision.RequiredOutput.Normalized().AcceptedKinds, []string{RuntimeDefinitionClarificationRequiredOutputKind}) || r.ActiveDispatch != nil {
			return fmt.Errorf("runtime Definition clarification aggregate binding mismatch")
		}
		if clarification.Dispatch != nil && clarification.Dispatch.AuthoritySnapshotID != r.Decision.AuthoritySnapshot.ID {
			return fmt.Errorf("runtime Definition clarification dispatch authority mismatch")
		}
		matchedSession := false
		for _, session := range r.CandidateRequestSessions {
			if session.LogicalRequestID == clarification.Observation.LogicalRequestID && session.State == RuntimeCandidateRequestSessionDocumentComplete &&
				session.Transaction.AuthoritySnapshotID == clarification.Observation.AuthoritySnapshotID {
				matchedSession = true
				break
			}
		}
		if !matchedSession {
			return fmt.Errorf("runtime Definition clarification has no complete semantic request session")
		}
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
		if err := attempt.Proposal.ValidateWithActivation(r.Decision, RuntimeActivationForFacts(r.Facts)); err != nil {
			return fmt.Errorf("pending runtime candidate proposal is invalid: %w", err)
		}
	}
	if err := ValidateRuntimeGovernanceResolutionRecords(r.GovernanceResolutions); err != nil {
		return err
	}
	if err := ValidateRuntimeGovernanceRequestRecords(r.GovernanceRequests); err != nil {
		return err
	}
	if err := ValidateRuntimeGovernanceRequestDispatchLineage(r.GovernanceRequests, r.ActiveDispatch, r.CompletedDispatches); err != nil {
		return err
	}
	if err := ValidateRuntimeOperationGovernanceBindings(r.OperationGovernanceBindings); err != nil {
		return err
	}
	if err := ValidateRuntimeDispatchExecutionKeyLineage(r.Facts.WorkspaceID, r.Facts.RunID, r.ActiveDispatch, r.CompletedDispatches, r.OperationGovernanceBindings); err != nil {
		return err
	}
	if err := ValidateRuntimeDispatchAdmissionLineage(r.ActiveDispatch, r.CompletedDispatches, r.CandidateAttempts, r.OperationGovernanceBindings); err != nil {
		return err
	}
	if err := ValidateRuntimeAcceptedCandidateManifestLineage(r.ActiveDispatch, r.CompletedDispatches, r.CandidateAttempts, r.CandidateRequestSessions); err != nil {
		return err
	}
	if err := ValidateRuntimeDurableDispatchLimits(r.Facts.Activation.ExecutionContract.ExecutionLimits, r.ActiveDispatch, r.CompletedDispatches, r.CandidateAttempts); err != nil {
		return err
	}
	if err := ValidateRuntimeAcceptedAssessmentPurposePreimagesWithValidatedSemanticLedger(r.SemanticPlanning, r.ActiveDispatch, r.CompletedDispatches, r.CandidateAttempts, r.CandidateRequestSessions); err != nil {
		return err
	}
	if err := ValidateRuntimeOperationReceipts(r.OperationReceipts); err != nil {
		return err
	}
	if err := ValidateRuntimeOperationReceiptExecutionLineage(r.ActiveDispatch, r.CompletedDispatches, r.OperationReceipts); err != nil {
		return err
	}
	for _, invalidation := range r.PrerequisiteInvalidations {
		if err := invalidation.Validate(); err != nil {
			return fmt.Errorf("runtime prerequisite invalidation: %w", err)
		}
	}
	if err := ValidateRuntimeRepairSurfaceConfirmationRecords(r.RepairSurfaceConfirmations); err != nil {
		return fmt.Errorf("runtime repair surface confirmations: %w", err)
	}
	for _, confirmation := range r.RepairSurfaceConfirmations {
		if confirmation.Confirmation.Observation.WorkspaceID != r.Facts.WorkspaceID || confirmation.Confirmation.Observation.RunID != r.Facts.RunID {
			return fmt.Errorf("runtime repair surface confirmation scope mismatch")
		}
	}
	recovery := r.Facts.Recovery.Normalized()
	if err := ValidateRuntimeNonTerminalRecoveryLinearization(recovery, r.StateRevision, r.DriveLease); err != nil {
		return fmt.Errorf("runtime state recovery: %w", err)
	}
	if r.DriveLease != nil {
		if err := r.DriveLease.Validate(); err != nil {
			return fmt.Errorf("runtime state drive lease: %w", err)
		}
		if r.DriveLease.WorkspaceID != r.Facts.WorkspaceID || r.DriveLease.RunID != r.Facts.RunID {
			return fmt.Errorf("runtime state drive lease scope mismatch")
		}
		canonicalTerminalDelivery := r.DriveLease.Reason == RuntimeDriveLeaseReasonDeliveryRetry && r.DriveLease.FailureSignature == "terminal-delivery"
		if canonicalTerminalDelivery {
			authorization := r.Facts.Recovery.Normalized().TerminalDeliveryRequest
			outcome, terminal, outcomeErr := DeriveRuntimeTerminalOutcome(r.Decision, r.Termination)
			if outcomeErr != nil || !terminal {
				return fmt.Errorf("runtime state terminal-delivery lease has no sealed terminal")
			}
			deliveryID := StableTerminalDeliveryID(r.Facts.WorkspaceID, r.Facts.RunID, r.Decision.AuthoritySnapshot.ID, outcome)
			creatorGeneration := r.DriveLease.RecoveryGeneration - 1
			creatorRequestID := StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, creatorGeneration)
			phaseMatches := authorization.Validate() == nil && authorization.TerminalDeliveryID == deliveryID &&
				((authorization.FailedLeaseGeneration == creatorGeneration && authorization.RequestID == creatorRequestID) ||
					(r.DriveLease.State == RuntimeDriveLeaseRunning && authorization.FailedLeaseGeneration == r.DriveLease.RecoveryGeneration &&
						authorization.RequestID == StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, r.DriveLease.RecoveryGeneration)))
			if r.DriveLease.RecoveryRequestID != creatorRequestID || !phaseMatches {
				return fmt.Errorf("runtime state terminal-delivery lease does not match persisted recovery request")
			}
		} else if RuntimeDriveLeaseBindsNonTerminalRecovery(*r.DriveLease) {
			if err := ValidateRuntimeNonTerminalRecoveryLeaseBinding(r.Facts.Recovery, *r.DriveLease); err != nil {
				return fmt.Errorf("runtime state drive lease: %w", err)
			}
			// A newly sealed convergence terminal may retain only the exact
			// request-bound running worker until SM-4 exit settlement. Scheduled
			// authority and the administrative termination lane remain disjoint.
			if r.Termination != nil || (isTerminalRuntimeDecision(r.Decision) && r.DriveLease.State != RuntimeDriveLeaseRunning) {
				return fmt.Errorf("runtime state terminal cannot retain nonterminal recovery authority outside an exact running convergence handoff")
			}
		} else if r.DriveLease.RecoveryRequestID != "" {
			return fmt.Errorf("runtime state non-terminal lease carries terminal recovery request")
		}
	}
	if recovery.TerminalDeliveryIntervention.Validate() == nil && r.DriveLease != nil {
		return fmt.Errorf("runtime state terminal delivery intervention cannot retain automatic drive authority")
	}
	retiredLease := recovery.RetiredTerminalDeliveryLease
	retiredLeasePresent := RuntimeDriveLeasePresent(retiredLease)
	if retiredLeasePresent {
		requestBoundNonterminal := RuntimeDriveLeaseBindsNonTerminalRecovery(retiredLease) &&
			ValidateRuntimeNonTerminalRecoveryLeaseBinding(recovery, retiredLease) == nil
		if r.Termination == nil || retiredLease.Validate() != nil || retiredLease.State != RuntimeDriveLeaseRunning ||
			retiredLease.WorkspaceID != r.Facts.WorkspaceID || retiredLease.RunID != r.Facts.RunID ||
			retiredLease.AggregateRevision >= r.Termination.Revision || (retiredLease.RecoveryRequestID != "" && !requestBoundNonterminal) ||
			(retiredLease.Reason == RuntimeDriveLeaseReasonDeliveryRetry && retiredLease.FailureSignature == "terminal-delivery") {
			return fmt.Errorf("runtime state retired terminal-delivery lease does not match administrative termination")
		}
		if recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != "" {
			outcome, terminal, err := DeriveRuntimeTerminalOutcome(r.Decision, r.Termination)
			if err != nil || !terminal {
				return fmt.Errorf("runtime state retired terminal-delivery lease has no terminal authority")
			}
			deliveryID := StableTerminalDeliveryID(r.Facts.WorkspaceID, r.Facts.RunID, r.Decision.AuthoritySnapshot.ID, outcome)
			expectedRequestID := StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, retiredLease.RecoveryGeneration)
			if recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != expectedRequestID {
				return fmt.Errorf("runtime state retired terminal-delivery lease handoff request mismatch")
			}
		}
		if r.DriveLease != nil && r.DriveLease.Reason == RuntimeDriveLeaseReasonDeliveryRetry && r.DriveLease.FailureSignature == "terminal-delivery" &&
			r.DriveLease.RecoveryGeneration == retiredLease.RecoveryGeneration+1 &&
			recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != r.DriveLease.RecoveryRequestID {
			return fmt.Errorf("runtime state retired terminal-delivery successor is not bound to its handoff request")
		}
	} else if recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != "" {
		return fmt.Errorf("runtime state retired terminal-delivery handoff request has no exact lease")
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
		if source == nil || source.State != RuntimeDispatchStateCommitted {
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
			execution.ExpectedEffectScope != binding.ExpectedEffectScope ||
			execution.IdempotencyClass != binding.IdempotencyClass || execution.AuthorityHash != binding.ExecutionAuthorityHash ||
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
			if request.TicketID == binding.TicketID && request.ApprovalID == binding.ApprovalID && request.DispatchID == binding.GovernanceDispatchID &&
				request.AuthoritySnapshotID == binding.GovernanceSnapshotID && request.DependencyKind == binding.DependencyKind && request.SubjectRef == binding.SubjectRef &&
				request.RequestedAction == binding.RequestedAction && request.RiskLevel == binding.RiskLevel && request.AuthorizationHash == binding.AuthorizationHash {
				bound = true
				break
			}
		}
		if !bound {
			return fmt.Errorf("operation governance binding has no matching owner request")
		}
	}
	governedExecutions := make([]RuntimeOperationExecutionRecord, 0)
	if r.ActiveDispatch != nil {
		governedExecutions = append(governedExecutions, r.ActiveDispatch.OperationExecutions...)
	}
	for _, dispatch := range r.CompletedDispatches {
		governedExecutions = append(governedExecutions, dispatch.OperationExecutions...)
	}
	if err := ValidateRuntimeOperationGovernanceExecutionBindings(r.OperationGovernanceBindings, governedExecutions); err != nil {
		return err
	}
	if err := ValidateRuntimeOperationGovernanceGrantLineage(r.OperationGovernanceBindings, r.GovernanceResolutions, r.Facts.Evidence.Accepted, r.Facts.Evidence.Rejected); err != nil {
		return err
	}
	for _, attempt := range r.CandidateAttempts {
		if attempt.State != RuntimeCandidateAttemptStateAccepted {
			continue
		}
		matched := r.ActiveDispatch != nil && RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, *r.ActiveDispatch)
		if !matched {
			for _, dispatch := range r.CompletedDispatches {
				if RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, dispatch) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return fmt.Errorf("accepted runtime candidate attempt has no exact sealed dispatch target/validation-recipe authority projection")
		}
	}
	for _, resolution := range r.GovernanceResolutions {
		resolution = resolution.Normalized()
		if resolution.WorkspaceID != r.Facts.WorkspaceID || resolution.RunID != r.Facts.RunID {
			return fmt.Errorf("runtime governance resolution scope mismatch")
		}
		matched := false
		bound := false
		for _, request := range r.GovernanceRequests {
			if request.TicketID == resolution.TicketID && request.ApprovalID == resolution.ApprovalID && request.DispatchID == resolution.DispatchID && request.AuthoritySnapshotID == resolution.AuthoritySnapshotID &&
				strings.TrimSpace(request.DependencyKind) == resolution.DependencyKind && strings.TrimSpace(request.SubjectRef) == resolution.SubjectRef {
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
		if r.Termination.Revision > r.StateRevision {
			return fmt.Errorf("runtime state termination revision exceeds state revision")
		}
	}
	if err := ValidateRuntimeTerminalMaterialSeal(r.TerminalMaterialSeal, r); err != nil {
		return err
	}
	if err := ValidateRuntimeOperationRecoveryAuthorizations(r.Facts.WorkspaceID, r.Facts.RunID, r.ActiveDispatch, r.CompletedDispatches, r.OperationReceipts, r.OperationRecoveryAuthorizations, r.Facts.Recovery, r.StateRevision); err != nil {
		return err
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

// ValidateRuntimeStateTerminalDeliveryLedger binds terminal-delivery recovery
// facts to an already sealed convergence terminal or administrative
// termination. A non-terminal aggregate cannot carry delivery attempts or
// acknowledgements because those fields have no current non-terminal meaning.
func ValidateRuntimeStateTerminalDeliveryLedger(
	decision RuntimeDecision,
	termination *RuntimeTerminationRecord,
	acknowledgements []RuntimeDeliveryAcknowledgement,
	recovery RuntimeRecoveryLedger,
) error {
	convergenceTerminal := isTerminalRuntimeDecision(decision)
	if convergenceTerminal && termination != nil {
		return fmt.Errorf("runtime state cannot carry both convergence and administrative terminals")
	}
	terminal := convergenceTerminal || termination != nil
	normalizedRecovery := recovery.Normalized()
	retiredLease := normalizedRecovery.RetiredTerminalDeliveryLease
	retiredLeasePresent := RuntimeDriveLeasePresent(retiredLease)
	if !terminal {
		if normalizedRecovery.TerminalDeliveryAttempts != 0 {
			return fmt.Errorf("non-terminal runtime state cannot consume terminal delivery budget")
		}
		if retiredLeasePresent || normalizedRecovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != "" {
			return fmt.Errorf("non-terminal runtime state cannot retain an administrative terminal lease handoff")
		}
		if len(acknowledgements) != 0 {
			return fmt.Errorf("non-terminal runtime state cannot retain terminal delivery acknowledgements")
		}
		return nil
	}
	if convergenceTerminal && (retiredLeasePresent || normalizedRecovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != "") {
		return fmt.Errorf("convergence terminal cannot retain an administrative terminal lease handoff")
	}
	outcome, terminalOutcome, err := DeriveRuntimeTerminalOutcome(decision, termination)
	if err != nil || !terminalOutcome {
		if err == nil {
			err = fmt.Errorf("runtime terminal outcome is absent")
		}
		return err
	}
	workspaceID := decision.AuthoritySnapshot.WorkspaceID
	runID := decision.AuthoritySnapshot.RunID
	expectedDeliveryID := StableTerminalDeliveryID(workspaceID, runID, decision.AuthoritySnapshot.ID, outcome)
	recoveryRequest := normalizedRecovery.TerminalDeliveryRequest
	if recoveryRequest.RequestID != "" {
		if err := recoveryRequest.Validate(); err != nil {
			return fmt.Errorf("runtime state terminal delivery recovery request: %w", err)
		}
		if recoveryRequest.TerminalDeliveryID != expectedDeliveryID {
			return fmt.Errorf("runtime state terminal delivery recovery request does not match sealed terminal")
		}
	}
	intervention := normalizedRecovery.TerminalDeliveryIntervention
	if intervention.RequestID != "" {
		if err := intervention.Validate(); err != nil {
			return fmt.Errorf("runtime state terminal delivery intervention: %w", err)
		}
		if intervention.TerminalDeliveryID != expectedDeliveryID {
			return fmt.Errorf("runtime state terminal delivery intervention does not match sealed terminal")
		}
	}
	expectedResultID := RuntimeTerminalResultID(workspaceID, runID)
	expectedOwners := []RuntimeDeliveryOwner{RuntimeDeliveryOwnerResult, RuntimeDeliveryOwnerWorkspace, RuntimeDeliveryOwnerRun}
	if len(acknowledgements) > len(expectedOwners) {
		return fmt.Errorf("runtime state terminal delivery acknowledgement ledger is over-complete")
	}
	for idx, acknowledgement := range acknowledgements {
		if err := acknowledgement.ValidateRaw(); err != nil {
			return fmt.Errorf("terminal delivery acknowledgement: %w", err)
		}
		if acknowledgement.WorkspaceID != workspaceID || acknowledgement.RunID != runID ||
			acknowledgement.TerminalDeliveryID != expectedDeliveryID || acknowledgement.Owner != expectedOwners[idx] ||
			acknowledgement.AuthoritySnapshotID != decision.AuthoritySnapshot.ID ||
			acknowledgement.CausationID != "terminal-delivery/"+expectedDeliveryID || acknowledgement.Outcome != outcome {
			return fmt.Errorf("runtime state terminal delivery acknowledgement does not match exact terminal order and identity")
		}
		switch acknowledgement.Owner {
		case RuntimeDeliveryOwnerResult:
			if acknowledgement.ResultID != expectedResultID ||
				(acknowledgement.AcknowledgedState != RuntimeDeliveryAcknowledgedResultRecorded && acknowledgement.AcknowledgedState != RuntimeDeliveryAcknowledgedNoop) {
				return fmt.Errorf("runtime state Result acknowledgement has invalid owner-local state")
			}
		case RuntimeDeliveryOwnerWorkspace:
			if acknowledgement.AcknowledgedState != RuntimeDeliveryAcknowledgedWorkspaceReleased && acknowledgement.AcknowledgedState != RuntimeDeliveryAcknowledgedNoop {
				return fmt.Errorf("runtime state Workspace acknowledgement has invalid owner-local state")
			}
		case RuntimeDeliveryOwnerRun:
			if acknowledgement.AcknowledgedState != RuntimeDeliveryAcknowledgedRunTerminal && acknowledgement.AcknowledgedState != RuntimeDeliveryAcknowledgedNoop {
				return fmt.Errorf("runtime state Run acknowledgement has invalid owner-local state")
			}
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
			if value.Failure != nil || value.Correction != nil || value.Stale != nil || value.PrerequisiteRequest != nil || value.OwnerFeasibility != nil {
				return fmt.Errorf("runtime candidate attempt %s state %s cannot carry failure", key, value.State)
			}
			if value.State == RuntimeCandidateAttemptStateAccepted && value.Proposal != nil {
				return fmt.Errorf("accepted runtime candidate attempt %s cannot retain an unsealed proposal", key)
			}
			if value.State == RuntimeCandidateAttemptStateAccepted && len(value.Feasibilities) > 0 {
				return fmt.Errorf("accepted runtime candidate attempt %s cannot retain unsealed feasibility records", key)
			}
			if value.State == RuntimeCandidateAttemptStateAccepted && len(value.Conformances) > 0 {
				return fmt.Errorf("accepted runtime candidate attempt %s cannot retain candidate conformance records", key)
			}
			if value.State == RuntimeCandidateAttemptStatePending {
				for _, conformance := range value.Conformances {
					if conformance.Observation.Normalized().Outcome != RuntimeOperationCandidateConformant {
						return fmt.Errorf("pending runtime candidate attempt %s cannot retain a rejected conformance fact", key)
					}
				}
				for _, feasibility := range value.Feasibilities {
					if feasibility.Observation == nil || feasibility.Observation.Normalized().Outcome != RuntimeOwnerFeasibilityFeasible {
						return fmt.Errorf("pending runtime candidate attempt %s cannot retain an undecided owner feasibility fact", key)
					}
				}
			}
			if value.State == RuntimeCandidateAttemptStateAccepted {
				if strings.TrimSpace(value.CandidateHash) == "" || strings.TrimSpace(value.DispatchID) == "" ||
					strings.TrimSpace(value.TargetAuthorizationHash) == "" || strings.TrimSpace(value.AdmissionCoverageBindingHash) == "" || strings.TrimSpace(value.DispatchAuthorityHash) == "" {
					return fmt.Errorf("accepted runtime candidate attempt %s requires candidate, dispatch, target-authorization, coverage-binding and authority hashes", key)
				}
				if err := ValidateRuntimeDispatchOperationTargetAuthorizationProjection(value.AdmissionTargetAuthorizations); err != nil {
					return fmt.Errorf("accepted runtime candidate attempt %s admission target projection: %w", key, err)
				}
				if value.TargetAuthorizationHash != StableRuntimeDispatchOperationTargetAuthorizationsHash(value.AdmissionTargetAuthorizations) {
					return fmt.Errorf("accepted runtime candidate attempt %s target-authorization hash does not match admission projection", key)
				}
				if err := ValidateCoverageEvidenceBindingProjection(value.AdmissionCoverageBindings); err != nil {
					return fmt.Errorf("accepted runtime candidate attempt %s admission coverage-binding projection: %w", key, err)
				}
				if value.AdmissionCoverageBindingHash != StableRuntimeCoverageEvidenceBindingSnapshotHash(value.AdmissionCoverageBindings) {
					return fmt.Errorf("accepted runtime candidate attempt %s coverage-binding hash does not match admission projection", key)
				}
				if len(value.AdmissionValidationRecipes) == 0 {
					if strings.TrimSpace(value.ValidationRecipeHash) != "" {
						return fmt.Errorf("accepted runtime candidate attempt %s validation-recipe hash requires an admission projection", key)
					}
				} else {
					if err := ValidateRuntimeValidationRecipeProjection(value.AdmissionValidationRecipes); err != nil {
						return fmt.Errorf("accepted runtime candidate attempt %s admission validation-recipe projection: %w", key, err)
					}
					if value.ValidationRecipeHash != StableRuntimeValidationRecipeSnapshotHash(value.AdmissionValidationRecipes) {
						return fmt.Errorf("accepted runtime candidate attempt %s validation-recipe hash does not match admission projection", key)
					}
				}
				if value.AdmissionImplementationWorkCommitment != nil {
					work := value.AdmissionImplementationWorkCommitment
					if work.Validate() != nil || work.WorkingBinding == nil || work.DispatchID != "" || work.CompletedReceiptSetHash != "" || value.AdmissionRepairCommitment != nil || len(value.AdmissionCoverageBindings) != 0 || len(value.AdmissionValidationRecipes) != 0 || value.Transaction.State != ConvergenceStateNeedStateChange {
						return fmt.Errorf("accepted runtime candidate attempt %s has invalid working commitment", key)
					}
				}
				if err := ValidateRuntimeDispatchRepairCommitmentProjection(value.RepairCommitmentHash, value.AdmissionRepairCommitment); err != nil {
					return fmt.Errorf("accepted runtime candidate attempt %s repair commitment: %w", key, err)
				}
			} else if value.CandidateHash != "" || value.DispatchID != "" || value.TargetAuthorizationHash != "" || len(value.AdmissionTargetAuthorizations) != 0 || value.AdmissionCoverageBindingHash != "" || len(value.AdmissionCoverageBindings) != 0 || value.ValidationRecipeHash != "" || len(value.AdmissionValidationRecipes) != 0 || value.RepairCommitmentHash != "" || value.AdmissionRepairCommitment != nil || value.AdmissionImplementationWorkCommitment != nil || value.DispatchAuthorityHash != "" {
				return fmt.Errorf("pending runtime candidate attempt %s cannot carry dispatch authority", key)
			}
		case RuntimeCandidateAttemptStateFailed:
			if value.Failure == nil || value.Failure.Class == "" || strings.TrimSpace(value.Failure.Reason) == "" || value.Correction == nil {
				return fmt.Errorf("failed runtime candidate attempt %s requires typed failure and total correction disposition", key)
			}
			if value.Stale != nil || value.PrerequisiteRequest != nil || value.OwnerFeasibility != nil {
				return fmt.Errorf("failed runtime candidate attempt %s cannot carry freshness stale", key)
			}
			if value.CandidateHash != "" || value.DispatchID != "" || value.TargetAuthorizationHash != "" || len(value.AdmissionTargetAuthorizations) != 0 || value.AdmissionCoverageBindingHash != "" || len(value.AdmissionCoverageBindings) != 0 || value.ValidationRecipeHash != "" || len(value.AdmissionValidationRecipes) != 0 || value.RepairCommitmentHash != "" || value.AdmissionRepairCommitment != nil || value.AdmissionImplementationWorkCommitment != nil || value.DispatchAuthorityHash != "" {
				return fmt.Errorf("failed runtime candidate attempt %s cannot carry dispatch authority", key)
			}
			if err := value.Correction.Validate(); err != nil {
				return fmt.Errorf("failed runtime candidate attempt %s correction: %w", key, err)
			}
			if err := ValidateRuntimeCandidateFailureCorrectionPair(value.Failure, value.Correction); err != nil {
				return fmt.Errorf("failed runtime candidate attempt %s failure/correction lineage: %w", key, err)
			}
			if value.Correction.ManifestHash != value.ManifestHash {
				return fmt.Errorf("failed runtime candidate attempt %s correction manifest mismatch", key)
			}
			if value.Attempt == 1 && value.Correction.Disposition == RuntimeCandidateCorrectionCorrectableOnce {
				return fmt.Errorf("runtime candidate attempt %s cannot authorize a third candidate attempt", key)
			}
			for _, conformance := range value.Conformances {
				outcome := conformance.Observation.Normalized().Outcome
				if outcome == RuntimeOperationCandidateNonconformant && value.Failure.Code != "candidate_owner_constraint_nonconformant" {
					return fmt.Errorf("failed runtime candidate attempt %s has an unbound owner candidate conformance rejection", key)
				}
				if outcome == RuntimeOperationCandidateContractDefect && value.Failure.Code != "runtime_contract_defect" {
					return fmt.Errorf("failed runtime candidate attempt %s has an unbound owner candidate contract defect", key)
				}
			}
			for _, feasibility := range value.Feasibilities {
				if feasibility.Observation != nil && feasibility.Observation.Normalized().Outcome == RuntimeOwnerFeasibilityCandidateConstraintViolated && value.Failure.Code != "candidate_owner_constraint_nonconformant" {
					return fmt.Errorf("failed runtime candidate attempt %s has an unbound owner candidate applicability rejection", key)
				}
			}
		case RuntimeCandidateAttemptStateInvalidated:
			if value.CandidateHash != "" || value.DispatchID != "" || value.TargetAuthorizationHash != "" || len(value.AdmissionTargetAuthorizations) != 0 || value.AdmissionCoverageBindingHash != "" || len(value.AdmissionCoverageBindings) != 0 || value.ValidationRecipeHash != "" || len(value.AdmissionValidationRecipes) != 0 || value.RepairCommitmentHash != "" || value.AdmissionRepairCommitment != nil || value.AdmissionImplementationWorkCommitment != nil || value.DispatchAuthorityHash != "" {
				return fmt.Errorf("invalidated runtime candidate attempt %s cannot carry dispatch authority", key)
			}
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
			if value.Failure != nil || value.Correction != nil || value.Proposal != nil || len(value.Conformances) != 0 || len(value.Feasibilities) != 0 || reasonCount != 1 {
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
		if value.State != RuntimeCandidateAttemptStateFailed && len(value.FeasibilityCatalogs) != 0 {
			return fmt.Errorf("runtime candidate attempt %s only failed attempts may retain historical feasibility catalogs", key)
		}
		catalogRefs := make(map[string]bool, len(value.FeasibilityCatalogs))
		for _, catalog := range value.FeasibilityCatalogs {
			if err := catalog.Validate(); err != nil {
				return fmt.Errorf("runtime candidate attempt %s historical feasibility catalog: %w", key, err)
			}
			if _, duplicate := catalogRefs[catalog.SnapshotID]; duplicate {
				return fmt.Errorf("runtime candidate attempt %s duplicates historical feasibility catalog", key)
			}
			catalogRefs[catalog.SnapshotID] = false
		}
		if value.State == RuntimeCandidateAttemptStateFailed {
			for _, feasibility := range value.Feasibilities {
				if _, found := catalogRefs[feasibility.AdmissibilitySnapshotID]; !found {
					return fmt.Errorf("runtime candidate attempt %s missing historical feasibility catalog", key)
				}
				catalogRefs[feasibility.AdmissibilitySnapshotID] = true
			}
		}
		for _, used := range catalogRefs {
			if !used {
				return fmt.Errorf("runtime candidate attempt %s has an unreferenced historical feasibility catalog", key)
			}
		}
		if err := ValidateRuntimeCandidateConformanceRecords(value.Conformances); err != nil {
			return fmt.Errorf("runtime candidate attempt %s conformance records: %w", key, err)
		}
		for _, conformance := range value.Conformances {
			if conformance.LogicalRequestID != value.LogicalRequestID || conformance.CandidateManifestHash != value.ManifestHash {
				return fmt.Errorf("runtime candidate attempt %s conformance request-session binding mismatch", key)
			}
		}
	}
	for snapshotID, attempts := range bySnapshot {
		first, hasFirst := runtimeCandidateAttemptByNumber(attempts, 0)
		second, hasSecond := runtimeCandidateAttemptByNumber(attempts, 1)
		if hasSecond && hasFirst && (first.State != RuntimeCandidateAttemptStateFailed || first.Correction == nil || first.Correction.Disposition != RuntimeCandidateCorrectionCorrectableOnce || first.Transaction != second.Transaction || first.StablePrefixHash != second.StablePrefixHash || first.ManifestHash != second.ManifestHash) {
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
		if operationID == "" || strings.TrimSpace(value.AdmissibilitySnapshotID) == "" || strings.TrimSpace(value.CandidateManifestHash) == "" {
			return fmt.Errorf("runtime candidate feasibility record requires operation, snapshot and candidate manifest identity")
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
		if value.BindingContradiction != nil {
			identity := value.BindingContradiction.Normalized()
			if err := identity.Validate(); err != nil || identity.WorkspaceID != workspaceID || identity.RunID != runID || identity.AuthorityInputID != value.Transaction.AuthorityInputID || value.State != RuntimeCandidateAttemptStateInvalidated || value.OwnerFeasibility == nil || !value.OwnerFeasibility.SealedTargetAuthorizationContradicted() {
				return fmt.Errorf("runtime candidate binding contradiction identity mismatch")
			}
		}
	}
	for contractID, attempts := range byContract {
		attemptCounts := [2]int{}
		for _, attempt := range attempts {
			attemptCounts[attempt.Attempt]++
		}
		first, hasFirst := runtimeCandidateAttemptByNumber(attempts, 0)
		second, hasSecond := runtimeCandidateAttemptByNumber(attempts, 1)
		if len(attempts) > 2 || attemptCounts[0] > 1 || attemptCounts[1] > 1 || (hasSecond && (!hasFirst || first.State != RuntimeCandidateAttemptStateFailed || first.Correction == nil || first.Correction.Disposition != RuntimeCandidateCorrectionCorrectableOnce || !SameAuthorityContract(first.Transaction, second.Transaction) || first.StablePrefixHash != second.StablePrefixHash || first.ManifestHash != second.ManifestHash)) {
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
	writeHashString(h, transaction.AuthorityInputID)
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
		left.OperationContractRevision == right.OperationContractRevision &&
		left.AuthorityInputID == right.AuthorityInputID
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
	return cloneRuntimeStateRecordRaw(r)
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
		value.OriginalDiagnostic = value.OriginalDiagnostic.CloneOriginal()
		ret.Terminal = &value
	}
	return ret
}

func (d RuntimeDispatchRecord) Validate(decision RuntimeDecision) error {
	return d.ValidateWithActivation(decision, RuntimeActivationContext{})
}

func (d RuntimeDispatchRecord) ValidateWithActivation(decision RuntimeDecision, activation RuntimeActivationContext) error {
	if err := ValidateAuthorityTransaction(decision, d.Transaction); err != nil {
		return err
	}
	if d.WorkspaceID != decision.AuthoritySnapshot.WorkspaceID || d.RunID != decision.AuthoritySnapshot.RunID {
		return fmt.Errorf("runtime dispatch scope mismatch")
	}
	if d.FocusTargetRole != decision.Focus.Normalized().TargetRole {
		return fmt.Errorf("runtime dispatch focus target role mismatch")
	}
	// Only non-policy startup activation fields are read from the current
	// aggregate. Effect/replay semantics are immutable dispatch authority.
	activation = activation.Clone()
	activation.OperationPolicies = RuntimeDispatchSealedOperationPolicies(d)
	if err := d.Candidate.ValidateWithActivation(decision, activation); err != nil {
		return err
	}
	return d.ValidateIntrinsic()
}

// ValidateIntrinsic verifies the self-contained, immutable dispatch shape. It
// is used for historical dispatches whose original RuntimeDecision is no longer
// current; it deliberately does not attempt to rebuild that authority from
// present facts.
func (d RuntimeDispatchRecord) ValidateIntrinsic() error {
	if strings.TrimSpace(d.ID) == "" || d.WorkspaceID == "" || d.RunID == "" || strings.TrimSpace(d.CandidateHash) == "" ||
		strings.TrimSpace(d.CandidateManifestHash) == "" ||
		strings.TrimSpace(d.Transaction.AuthoritySnapshotID) == "" || strings.TrimSpace(d.Transaction.FocusID) == "" ||
		strings.TrimSpace(d.Transaction.RequiredOutputHash) == "" || strings.TrimSpace(d.Transaction.DefinitionRevision) == "" ||
		strings.TrimSpace(d.Transaction.EvidenceRevision) == "" || strings.TrimSpace(d.Transaction.CapabilityStateHash) == "" ||
		strings.TrimSpace(d.Transaction.PrerequisiteFreshnessHash) == "" || strings.TrimSpace(d.Transaction.OperationContractRevision) == "" ||
		strings.TrimSpace(d.Transaction.AuthorityInputID) == "" || d.Transaction.Attempt < 0 {
		return fmt.Errorf("runtime dispatch requires id, scope, authority and candidate hash")
	}
	if err := ValidateRuntimeOperationContractRevision(d.Transaction.OperationContractRevision); err != nil {
		return fmt.Errorf("runtime dispatch operation contract revision: %w", err)
	}
	if d.FocusTargetRole != strings.TrimSpace(d.FocusTargetRole) {
		return fmt.Errorf("runtime dispatch focus target role must be canonical")
	}
	if d.Transaction.AuthoritySnapshotID != StableAuthoritySnapshotIDForTransaction(d.WorkspaceID, d.RunID, d.Transaction) {
		return fmt.Errorf("runtime dispatch authority snapshot identity mismatch")
	}
	if err := ValidateSealedRuntimeCandidateShape(d.Candidate); err != nil {
		return err
	}
	if err := validateRuntimeDispatchCandidateIntrinsic(d.Candidate, d.Transaction); err != nil {
		return err
	}
	if d.CandidateHash != StableRuntimeCandidateHash(d.Candidate) {
		return fmt.Errorf("runtime dispatch candidate hash mismatch")
	}
	if strings.TrimSpace(d.CoverageBindingHash) == "" {
		return fmt.Errorf("runtime dispatch requires an immutable coverage-binding hash")
	}
	if err := ValidateCoverageEvidenceBindingProjection(d.CoverageBindings); err != nil {
		return fmt.Errorf("runtime dispatch coverage-binding projection: %w", err)
	}
	if d.CoverageBindingHash != StableRuntimeCoverageEvidenceBindingSnapshotHash(d.CoverageBindings) {
		return fmt.Errorf("runtime dispatch coverage-binding hash mismatch")
	}
	for _, operation := range runtimeCandidateAdmissionOperations(d.Candidate) {
		for _, ref := range operation.BindingRefs {
			if _, found := CoverageEvidenceBindingForRef(d.CoverageBindings, ref); !found {
				return fmt.Errorf("runtime dispatch operation %q binding %q is absent from its immutable snapshot", operation.ID, ref)
			}
		}
	}
	if err := ValidateRuntimeDispatchOperationTargetAuthorizations(d.Candidate, d.OperationTargetAuthorizations); err != nil {
		return err
	}
	if d.ID != StableRuntimeDispatchID(d.Transaction, d.CandidateHash) {
		return fmt.Errorf("runtime dispatch identity mismatch")
	}
	if err := ValidateRuntimeCandidateManagedValidationRecipeSnapshot(d.Candidate, d.ValidationRecipes); err != nil {
		return fmt.Errorf("runtime dispatch validation recipe snapshot: %w", err)
	}
	if err := ValidateRuntimeImplementationWorkDispatchCommitment(d); err != nil {
		return fmt.Errorf("runtime implementation dispatch commitment: %w", err)
	}
	if err := ValidateRuntimeDispatchRepairCommitmentProjection(d.RepairCommitmentHash, d.RepairCommitment); err != nil {
		return fmt.Errorf("runtime dispatch repair commitment: %w", err)
	}
	if d.Candidate.Assessment != nil {
		if d.AssessmentAuthorityInput == nil || d.AssessmentAuthorityInputHash == "" ||
			d.AssessmentAuthorityInputHash != StableRuntimeAssessmentAuthorityInputHash(*d.AssessmentAuthorityInput) {
			return fmt.Errorf("runtime assessment dispatch requires immutable authority input")
		}
		if err := d.AssessmentAuthorityInput.Validate(d.Transaction, d.CoverageBindings); err != nil {
			return err
		}
	} else if d.AssessmentAuthorityInput != nil || d.AssessmentAuthorityInputHash != "" || d.AssessmentAdjudication != nil || d.AssessmentAdjudicationHash != "" {
		return fmt.Errorf("non-assessment runtime dispatch cannot carry assessment authority or adjudication")
	}
	if d.State == RuntimeDispatchStateCommitted && d.Candidate.Assessment != nil {
		if d.AssessmentAdjudication == nil || d.AssessmentAdjudicationHash == "" {
			return fmt.Errorf("committed runtime assessment dispatch requires adjudication")
		}
		if err := d.AssessmentAdjudication.ValidateRaw(); err != nil {
			return err
		}
	} else if d.AssessmentAdjudication != nil || d.AssessmentAdjudicationHash != "" {
		return fmt.Errorf("non-committed runtime assessment dispatch cannot carry adjudication")
	}
	if d.State != RuntimeDispatchStateSealed && d.State != RuntimeDispatchStateCommitted && d.State != RuntimeDispatchStateInvalidated {
		return fmt.Errorf("runtime dispatch has invalid state %q", d.State)
	}
	available := runtimeCandidateOperationIDs(d.Candidate)
	completed := map[string]struct{}{}
	for _, rawOperationID := range d.CompletedOperationIDs {
		operationID := strings.TrimSpace(rawOperationID)
		if operationID == "" || operationID != rawOperationID {
			return fmt.Errorf("runtime dispatch completed operation id must be canonical")
		}
		if _, ok := available[operationID]; !ok {
			return fmt.Errorf("runtime dispatch completed operation %q is not present in candidate", operationID)
		}
		if _, ok := completed[operationID]; ok {
			return fmt.Errorf("runtime dispatch completed operation %q is duplicated", operationID)
		}
		completed[operationID] = struct{}{}
	}
	if err := ValidateRuntimeOperationExecutionRecords(d.OperationExecutions); err != nil {
		return err
	}
	if len(available) != len(d.OperationExecutions) {
		return fmt.Errorf("runtime dispatch requires exactly one operation execution per candidate operation")
	}
	wantOperationAuthorityHash := StableRuntimeOperationExecutionAuthoritiesHash(d.OperationExecutions)
	if d.OperationAuthorityHash != wantOperationAuthorityHash {
		return fmt.Errorf("runtime dispatch operation authority hash mismatch")
	}
	for _, exec := range NormalizeRuntimeOperationExecutionRecords(d.OperationExecutions) {
		if _, ok := available[exec.OperationID]; !ok {
			return fmt.Errorf("runtime dispatch operation execution %q is not present in candidate", exec.OperationID)
		}
		originID, err := RuntimeOperationExecutionKeyOrigin(d.WorkspaceID, d.RunID, exec.ExecutionKey, exec.OperationID)
		if err != nil {
			return fmt.Errorf("runtime dispatch operation execution %q: %w", exec.OperationID, err)
		}
		if originID != d.ID && !RuntimeOperationClaimMarkerIsGovernance(exec.ClaimedAt) && exec.State != OperationExecutionEvidenceCommitted {
			return fmt.Errorf("runtime dispatch operation execution %q has non-current execution-key origin without governance or completed-clone state", exec.OperationID)
		}
		operation, found := runtimeCandidateOperationForID(d.Candidate, exec.OperationID)
		if !found || exec.OperationHash != StableRuntimeOperationHash(operation) {
			return fmt.Errorf("runtime dispatch operation execution %q operation hash mismatch", exec.OperationID)
		}
		_, markedCompleted := completed[exec.OperationID]
		if markedCompleted != (exec.State == OperationExecutionEvidenceCommitted) {
			return fmt.Errorf("runtime dispatch operation %q completed marker/evidence state mismatch", exec.OperationID)
		}
		if exec.ReplayProof != nil {
			descriptor, known := RuntimeOperationDescriptorForKind(operation.Kind)
			proof := exec.ReplayProof.Normalized()
			if !known || proof.OwnerModuleID != descriptor.OwnerModuleID || !sameRuntimeStringSet(proof.TargetRefs, operation.TargetRefs) {
				return fmt.Errorf("runtime dispatch replay proof does not match sealed operation %q", exec.OperationID)
			}
		}
	}
	if err := ValidateRuntimeDispatchOperationAuthority(d); err != nil {
		return err
	}
	if strings.TrimSpace(d.DispatchAuthorityHash) == "" || d.DispatchAuthorityHash != StableRuntimeDispatchAuthorityHash(d) {
		return fmt.Errorf("runtime dispatch authority hash mismatch")
	}
	return nil
}

func StableRuntimeDispatchID(transaction AuthorityTransaction, candidateHash string) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{transaction.AuthoritySnapshotID, transaction.RequiredOutputHash, strings.TrimSpace(candidateHash)}, "|")))
	return "dispatch-" + hex.EncodeToString(digest[:])[:20]
}

// StableRuntimeDispatchAuthorityHash is the single immutable SM-3 authority
// seal. It binds fields that must move together across persistence and
// governance resume without including mutable dispatch/execution progress.
func StableRuntimeDispatchAuthorityHash(dispatch RuntimeDispatchRecord) string {
	h := sha256.New()
	writeHashString(h, "runtime-dispatch-authority/v5")
	writeHashString(h, string(dispatch.WorkspaceID))
	writeHashString(h, string(dispatch.RunID))
	writeHashString(h, strings.TrimSpace(dispatch.FocusTargetRole))
	transaction := dispatch.Transaction
	writeHashString(h, transaction.AuthoritySnapshotID)
	writeHashString(h, transaction.FocusID)
	writeHashString(h, string(transaction.State))
	writeHashString(h, transaction.RequiredOutputHash)
	writeHashString(h, transaction.DefinitionRevision)
	writeHashString(h, transaction.EvidenceRevision)
	writeHashString(h, transaction.CapabilityStateHash)
	writeHashString(h, transaction.PrerequisiteFreshnessHash)
	writeHashString(h, transaction.OperationContractRevision)
	writeHashString(h, transaction.AuthorityInputID)
	writeHashString(h, fmt.Sprintf("%d", transaction.Attempt))
	writeHashString(h, StableRuntimeCandidateHash(dispatch.Candidate))
	writeHashString(h, strings.TrimSpace(dispatch.CandidateManifestHash))
	if dispatch.ImplementationWorkCommitment != nil {
		writeHashString(h, dispatch.ImplementationWorkCommitment.Hash)
	}
	writeHashString(h, StableRuntimeCoverageEvidenceBindingSnapshotHash(dispatch.CoverageBindings))
	writeHashString(h, StableRuntimeDispatchOperationTargetAuthorizationsHash(dispatch.OperationTargetAuthorizations))
	writeHashString(h, StableRuntimeValidationRecipeSnapshotHash(dispatch.ValidationRecipes))
	writeHashString(h, strings.TrimSpace(dispatch.RepairCommitmentHash))
	writeHashString(h, strings.TrimSpace(dispatch.AssessmentAuthorityInputHash))
	writeHashString(h, StableRuntimeDispatchOperationAuthorityHash(dispatch))
	writeHashString(h, StableRuntimeOperationExecutionAuthoritiesHash(dispatch.OperationExecutions))
	return hex.EncodeToString(h.Sum(nil))
}

// StableRuntimeValidationRecipeSnapshotHash commits the complete canonical
// managed-validation recipe projection, including ordered arguments.
func StableRuntimeValidationRecipeSnapshotHash(values []RuntimeValidationRecipe) string {
	recipes := NormalizeRuntimeValidationRecipes(values)
	h := sha256.New()
	writeHashString(h, "runtime-validation-recipe-snapshot/v1")
	writeHashString(h, fmt.Sprintf("%d", len(recipes)))
	for _, recipe := range recipes {
		writeHashString(h, recipe.ID)
		writeHashString(h, recipe.AdapterRef)
		writeRuntimeValidationRecipeInputReviewHash(h, recipe)
		writeHashString(h, recipe.Command)
		writeHashStrings(h, recipe.Args)
		writeHashString(h, fmt.Sprintf("%d", recipe.TimeoutMillis))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func validateRuntimeDispatchCandidateIntrinsic(candidate RuntimeCandidate, transaction AuthorityTransaction) error {
	var operations []RuntimeOperation
	switch transaction.State {
	case ConvergenceStateNeedStateChange:
		if candidate.StateChange == nil || candidate.Validation != nil || candidate.Assessment != nil || candidate.Governance != nil {
			return fmt.Errorf("runtime dispatch state-change authority requires exactly one state-change candidate")
		}
		operations = stateChangingRuntimeOperations(candidate.StateChange.Operations)
	case ConvergenceStateNeedValidation:
		if candidate.Validation == nil || candidate.StateChange != nil || candidate.Assessment != nil || candidate.Governance != nil {
			return fmt.Errorf("runtime dispatch validation authority requires exactly one validation candidate")
		}
		operations = validationRuntimeOperations(candidate.Validation.Operations)
	case ConvergenceStateNeedAssessment:
		if candidate.Assessment == nil || candidate.StateChange != nil || candidate.Validation != nil || candidate.Governance != nil {
			return fmt.Errorf("runtime dispatch assessment authority requires exactly one assessment candidate")
		}
		assessment := candidate.Assessment.Normalized()
		if assessment.FocusID != transaction.FocusID || assessment.Summary == "" || len(assessment.CoverageWorkItemIDs) == 0 {
			return fmt.Errorf("runtime dispatch assessment candidate requires authority focus, coverage and summary")
		}
		if _, err := ValidateAssessmentDisposition(assessment.Disposition, assessment.Items, assessment.Gap); err != nil {
			return err
		}
		return nil
	case ConvergenceStateNeedGovernance:
		if candidate.Governance == nil || candidate.StateChange != nil || candidate.Validation != nil || candidate.Assessment != nil {
			return fmt.Errorf("runtime dispatch governance authority requires exactly one governance candidate")
		}
		return candidate.Governance.ExternalDependency.Validate()
	default:
		return fmt.Errorf("runtime dispatch transaction state %q cannot own a candidate", transaction.State)
	}
	if len(operations) == 0 {
		return fmt.Errorf("runtime dispatch operation candidate requires at least one operation")
	}
	ids := make(map[string]struct{}, len(operations))
	for _, raw := range operations {
		operation := raw.Normalized()
		if operation.ID == "" || operation.Kind == "" {
			return fmt.Errorf("runtime dispatch operation requires id and kind")
		}
		if len(operation.CoverageWorkItemIDs) != 0 {
			return fmt.Errorf("runtime dispatch operation %q cannot carry coverage work item ids", operation.ID)
		}
		if _, duplicate := ids[operation.ID]; duplicate {
			return fmt.Errorf("runtime dispatch operation id %q is duplicated", operation.ID)
		}
		ids[operation.ID] = struct{}{}
		descriptor, known := RuntimeOperationDescriptorForKind(operation.Kind)
		if !known {
			return fmt.Errorf("runtime dispatch operation %q has unknown kind %q", operation.ID, operation.Kind)
		}
		if !runtimeDescriptorAllowsState(descriptor, transaction.State) {
			return fmt.Errorf("runtime dispatch operation %q kind %q is not allowed in convergence state %q", operation.ID, operation.Kind, transaction.State)
		}
		if err := validateOperationParameters(operation.Parameters); err != nil {
			return fmt.Errorf("runtime dispatch operation %q: %w", operation.ID, err)
		}
		if err := ValidateRuntimeOperationContract(operation); err != nil {
			return fmt.Errorf("runtime dispatch operation %q: %w", operation.ID, err)
		}
	}
	dependencies := make(map[string][]string, len(operations))
	for _, raw := range operations {
		operation := raw.Normalized()
		dependencies[operation.ID] = append([]string(nil), operation.DependsOn...)
		for _, dependency := range operation.DependsOn {
			if dependency == operation.ID {
				return fmt.Errorf("runtime dispatch operation %q cannot depend on itself", operation.ID)
			}
			if _, found := ids[dependency]; !found {
				return fmt.Errorf("runtime dispatch operation %q depends on unknown operation %q", operation.ID, dependency)
			}
		}
	}
	visitState := make(map[string]uint8, len(dependencies))
	var visit func(string) error
	visit = func(operationID string) error {
		switch visitState[operationID] {
		case 1:
			return fmt.Errorf("runtime dispatch operation dependency cycle at %q", operationID)
		case 2:
			return nil
		}
		visitState[operationID] = 1
		for _, dependency := range dependencies[operationID] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visitState[operationID] = 2
		return nil
	}
	for operationID := range dependencies {
		if err := visit(operationID); err != nil {
			return err
		}
	}
	return nil
}

func (d RuntimeDispatchRecord) Clone() RuntimeDispatchRecord {
	ret := d
	ret.Candidate = cloneRuntimeCandidate(d.Candidate)
	ret.CoverageBindings = CloneCoverageEvidenceBindings(d.CoverageBindings)
	ret.OperationTargetAuthorizations = CloneRuntimeDispatchOperationTargetAuthorizations(d.OperationTargetAuthorizations)
	ret.ValidationRecipes = CloneRuntimeValidationRecipes(d.ValidationRecipes)
	if d.RepairCommitment != nil {
		value := d.RepairCommitment.Clone()
		ret.RepairCommitment = &value
	}
	if d.ImplementationWorkCommitment != nil {
		value := d.ImplementationWorkCommitment.Clone()
		ret.ImplementationWorkCommitment = &value
	}
	if d.AssessmentAuthorityInput != nil {
		value := d.AssessmentAuthorityInput.Clone()
		ret.AssessmentAuthorityInput = &value
	}
	if d.AssessmentAdjudication != nil {
		value := d.AssessmentAdjudication.Clone()
		ret.AssessmentAdjudication = &value
	}
	ret.OperationAdmissibilities = CloneRuntimeOperationAdmissibilities(d.OperationAdmissibilities)
	ret.OperationAdmissibilityObservations = CloneRuntimePrerequisiteObservations(d.OperationAdmissibilityObservations)
	ret.OperationPolicies = NormalizeRuntimeOperationPolicyFacts(d.OperationPolicies)
	ret.CompletedOperationIDs = append([]string(nil), d.CompletedOperationIDs...)
	if d.OperationExecutions != nil {
		ret.OperationExecutions = make([]RuntimeOperationExecutionRecord, len(d.OperationExecutions))
		for idx := range d.OperationExecutions {
			ret.OperationExecutions[idx] = d.OperationExecutions[idx].Clone()
		}
	}
	return ret
}

// ValidateRuntimeDispatchHistoryShape enforces the aggregate-level dispatch
// partition without reconstructing historical decisions. Active contains only
// the current sealed transaction; completed contains only closed transactions,
// and identities cannot overlap or repeat.
func ValidateRuntimeDispatchHistoryShape(workspaceID common.WorkspaceID, runID common.RunID, active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord) error {
	seen := make(map[string]struct{}, len(completed)+1)
	if active != nil {
		if active.WorkspaceID != workspaceID || active.RunID != runID {
			return fmt.Errorf("active runtime dispatch scope mismatch")
		}
		if active.State != RuntimeDispatchStateSealed {
			return fmt.Errorf("active runtime dispatch must be sealed")
		}
		if err := active.ValidateIntrinsic(); err != nil {
			return fmt.Errorf("active runtime dispatch: %w", err)
		}
		seen[active.ID] = struct{}{}
	}
	for idx := range completed {
		dispatch := completed[idx]
		if dispatch.WorkspaceID != workspaceID || dispatch.RunID != runID {
			return fmt.Errorf("completed runtime dispatch[%d] scope mismatch", idx)
		}
		if dispatch.State != RuntimeDispatchStateCommitted && dispatch.State != RuntimeDispatchStateInvalidated {
			return fmt.Errorf("completed runtime dispatch[%d] must be committed or invalidated", idx)
		}
		if _, duplicate := seen[dispatch.ID]; duplicate {
			return fmt.Errorf("runtime dispatch id %q is duplicated across active/completed history", dispatch.ID)
		}
		if err := dispatch.ValidateIntrinsic(); err != nil {
			return fmt.Errorf("completed runtime dispatch[%d]: %w", idx, err)
		}
		seen[dispatch.ID] = struct{}{}
	}
	return nil
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
		value := GovernanceCandidate{ExternalDependency: candidate.Governance.ExternalDependency.Normalized()}
		ret.Governance = &value
	}
	return ret
}

func cloneRuntimeOperation(operation RuntimeOperation) RuntimeOperation {
	ret := operation.Normalized()
	parameters := append([]OperationParameter(nil), ret.Parameters...)
	ret.Parameters = make([]OperationParameter, len(parameters))
	for idx, parameter := range parameters {
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
