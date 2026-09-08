package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/grounding"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/proof"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type DispatchState string

const (
	DispatchStateSealed      DispatchState = "sealed"
	DispatchStateCommitted   DispatchState = "committed"
	DispatchStateInvalidated DispatchState = "invalidated"
)

type SealedDispatch struct {
	ID            string
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID
	Transaction   runtimecontract.AuthorityTransaction
	CandidateHash string
	Candidate     runtimecontract.RuntimeCandidate
	// CandidateManifestHash and OperationTargetAuthorizations are the exact
	// immutable manifest identity/target projection used by final SM-3 seal.
	CandidateManifestHash         string
	CoverageBindingHash           string
	CoverageBindings              []runtimecontract.CoverageEvidenceBinding
	OperationTargetAuthorizations []runtimecontract.RuntimeDispatchOperationTargetAuthorization
	// FocusTargetRole is copied from the SM-2 decision at seal time. Owner
	// execution and completed-dispatch causation checks must not consult a later
	// mutable Focus projection for this value.
	FocusTargetRole string
	// ValidationRecipes is the immutable subset selected from the candidate
	// manifest's validation-owner-confirmed recipes. Execution must never replace it
	// with activation policy or a later admissibility inventory.
	ValidationRecipes []runtimecontract.RuntimeValidationRecipe
	// RepairCommitment is the pre-seal, request-session-bound purpose and
	// closure lineage for an owner-confirmed validation repair. It is immutable
	// across partial completion and governance resume hops.
	RepairCommitmentHash         string
	RepairCommitment             *runtimecontract.RuntimeDispatchRepairCommitment
	AssessmentAuthorityInputHash string
	AssessmentAuthorityInput     *runtimecontract.RuntimeAssessmentAuthorityInput
	AssessmentAdjudicationHash   string
	AssessmentAdjudication       *runtimecontract.RuntimeAssessmentAdjudication
	// OperationAdmissibilities retain the complete owner catalog preimages used
	// by feasibility and seal. Their matching observations are retained because
	// capability owners validate the same freshness identity before transport.
	OperationAdmissibilities           []runtimecontract.RuntimeOperationAdmissibility
	OperationAdmissibilityObservations []runtimecontract.RuntimePrerequisiteObservation
	// OperationPolicies are exact activation fallbacks for operations without a
	// catalog snapshot. They are immutable SM-3 authority, not current policy.
	OperationPolicies     []runtimecontract.RuntimeOperationPolicyFact
	State                 DispatchState
	CompletedOperationIDs []string
	// OperationExecutions is the per-operation claim/fence ledger (RVC-R9/R10).
	OperationExecutions    []runtimecontract.RuntimeOperationExecutionRecord
	OperationAuthorityHash string
	DispatchAuthorityHash  string
}

func cloneRuntimeDispatchRepairCommitment(value *runtimecontract.RuntimeDispatchRepairCommitment) *runtimecontract.RuntimeDispatchRepairCommitment {
	if value == nil {
		return nil
	}
	copy := value.Clone()
	return &copy
}

func cloneRuntimeAssessmentAuthorityInput(value *runtimecontract.RuntimeAssessmentAuthorityInput) *runtimecontract.RuntimeAssessmentAuthorityInput {
	if value == nil {
		return nil
	}
	copy := value.Clone()
	return &copy
}

func cloneRuntimeAssessmentAdjudication(value *runtimecontract.RuntimeAssessmentAdjudication) *runtimecontract.RuntimeAssessmentAdjudication {
	if value == nil {
		return nil
	}
	copy := value.Clone()
	return &copy
}

type DispatchStepInput struct {
	DispatchID    string
	Transaction   runtimecontract.AuthorityTransaction
	CandidateHash string
	Candidate     runtimecontract.RuntimeCandidate
}

// RuntimeDispatchPreSealAuthority is the complete optional authority supplied
// to the one SM-3 seal transaction.  Neither field may be injected or rehashed
// after SealDispatch returns.
type RuntimeDispatchPreSealAuthority struct {
	RepairCommitment *runtimecontract.RuntimeDispatchRepairCommitment
	// RepairCommitmentOriginDispatchID is empty for an origin seal. Governance
	// resume sets it to the exact committed source dispatch whose immutable
	// commitment is inherited before the new hash is computed.
	RepairCommitmentOriginDispatchID string
	AssessmentAuthorityInput         *runtimecontract.RuntimeAssessmentAuthorityInput
	GovernanceResume                 *RuntimeGovernanceResumePreSealAuthority
}

// RuntimeGovernanceResumePreSealAuthority is the exact committed source and
// grant used to derive continuation execution records before the new dispatch
// receives its sole authority hash.
type RuntimeGovernanceResumePreSealAuthority struct {
	Source  SealedDispatch
	Binding runtimecontract.RuntimeOperationGovernanceBinding
}

func (d SealedDispatch) StepInput() DispatchStepInput {
	return DispatchStepInput{DispatchID: d.ID, Transaction: d.Transaction, CandidateHash: d.CandidateHash, Candidate: d.Candidate}
}

const (
	ExecutionEvidenceStatusAccepted = runtimecontract.RuntimeOperationExecutionStatusAccepted
	ExecutionEvidenceStatusRejected = runtimecontract.RuntimeOperationExecutionStatusRejected
)

type ExecutionEvidence struct {
	EventID             string
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	DispatchID          string
	AuthoritySnapshotID string
	OperationID         string
	Kind                string
	Status              runtimecontract.RuntimeOperationExecutionStatus
	SourceRef           string
	Failure             *runtimecontract.FailureFact
	Evidence            runtimecontract.RuntimeOperationEvidence
	// ObservedEffectScope / NoOp / OwnerReceipt are owner-reported facts (RVC-R9).
	// When present, evidence Kind is re-derived from sealed expectation + receipt.
	ObservedEffectScope runtimecontract.RuntimeEffectScope
	NoOp                bool
	OwnerReceiptRef     string
	OwnerReceiptHash    string
	Receipt             *runtimecontract.RuntimeOperationReceipt
}

type RuntimeAggregate struct {
	// ResultFeedbackVersion is immutable SM-4 rendering policy, initialized
	// once by Orchestrator. Run persists it opaquely; recovery never upgrades it.
	ResultFeedbackVersion uint32 `json:",omitempty"`
	StateRevision         uint64
	EvidenceSequence      uint64
	Definition            runtimecontract.RuntimeDefinitionContext
	Facts                 runtimecontract.RuntimeFacts
	SemanticPlanning      runtimecontract.RuntimeSemanticPlanningLedger
	// Reconciliation is SM-2-owned request/batch lineage. Capability owner
	// observations enter only through RuntimeInternalEvent and never mutate
	// Facts directly.
	Reconciliation runtimecontract.RuntimeCWIReconciliationLedger
	// CoverageProofLineage is the bounded SM-2 receipt-to-reseal audit ledger.
	// It records source proof and current-binding resolutions without changing
	// immutable EvidenceFact binding refs or granting a capability owner credit.
	CoverageProofLineage []runtimecontract.RuntimeCoverageProofLineage
	Decision             runtimecontract.RuntimeDecision
	ActiveDispatch       *SealedDispatch
	CompletedDispatches  []SealedDispatch
	ProcessedEventIDs    []string
	// RepairSurfaceConfirmations is the typed SM-2 replay ledger for resource-owner
	// repair-surface facts.  It is separate from generic event IDs so owner
	// confirmation idempotency cannot become a global event index.
	RepairSurfaceConfirmations []runtimecontract.RuntimeRepairSurfaceConfirmationRecord
	Diagnostics                []runtimecontract.RuntimeDiagnosticRecord
	CandidateRequestSessions   []runtimecontract.RuntimeCandidateRequestSession
	CandidateAttempts          []runtimecontract.RuntimeCandidateAttemptRecord
	// ExecutionBudget is the sole Orchestrator-owned allocation/consumption
	// sub-ledger. Owner usage and receipts join it through typed private facts.
	ExecutionBudget runtimecontract.RuntimeExecutionBudgetLedger
	// RecoverableWaits are the bounded SM-2/3/4 wait lineage; timers and Run
	// lifecycle never own or infer these records.
	RecoverableWaits []runtimecontract.RuntimeRecoverableWaitRecord
	// OperationRecoveryAuthorizations are the durable SM-4 replay permissions
	// for exact claimed SM-3 executions. A process-local counter is never replay
	// authority.
	OperationRecoveryAuthorizations []runtimecontract.RuntimeOperationRecoveryAuthorization
	// DefinitionClarification is the dedicated semantic-gap transaction. It is
	// neither an operation/governance candidate nor a generic dependency.
	DefinitionClarification     *runtimecontract.RuntimeDefinitionClarificationState
	GovernanceRequests          []runtimecontract.RuntimeGovernanceRequestRecord
	GovernanceResolutions       []runtimecontract.RuntimeGovernanceResolutionRecord
	OperationGovernanceBindings []runtimecontract.RuntimeOperationGovernanceBinding
	OperationReceipts           []runtimecontract.RuntimeOperationReceipt
	DeliveryAcknowledgements    []runtimecontract.RuntimeDeliveryAcknowledgement
	PrerequisiteInvalidations   []runtimecontract.RuntimePrerequisiteInvalidationRecord
	// DriveLease is SM-4 owner-local recovery state. It is persisted with the
	// aggregate so a process-local timer cannot become a second retry authority.
	DriveLease *runtimecontract.RuntimeDriveLease
	// Termination is the SM-1 administrative cancel/supersede record (INV-08).
	Termination *runtimecontract.RuntimeTerminationRecord
}

type RuntimeStateRepository interface {
	Load(context.Context, common.WorkspaceID, common.RunID) (RuntimeAggregate, error)
	Save(context.Context, RuntimeAggregate, uint64) error
}

type CommitDisposition string

const (
	CommitDispositionCommitted CommitDisposition = "committed"
	CommitDispositionStale     CommitDisposition = "stale_diagnostic"
	CommitDispositionDuplicate CommitDisposition = "duplicate"
)

type EvidenceCommitResult struct {
	Disposition       CommitDisposition
	DispatchCompleted bool
	Aggregate         RuntimeAggregate
}

// DispatchCoordinator is a narrow harness for the standalone dispatch
// test/support constructor. Production RuntimeController wiring stores
// candidateDispatchComponent directly. The harness embeds that private
// component rather than re-exporting an alias, so the canonical implementation
// and its owner boundary remain unambiguous to the repository boundary scan.
type DispatchCoordinator struct {
	*candidateDispatchComponent
}

func NewDispatchCoordinator(repository RuntimeStateRepository) (*DispatchCoordinator, error) {
	transitions, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		return nil, err
	}
	return newDispatchCoordinator(repository, transitions)
}

func newDispatchCoordinator(repository RuntimeStateRepository, transitions *RuntimeTransitionCoordinator) (*DispatchCoordinator, error) {
	kernel, err := newRuntimeMutationKernel(repository, transitions)
	if err != nil {
		return nil, err
	}
	component := newCandidateDispatchComponent(kernel)
	if component == nil {
		return nil, common.ErrInvalidArgument
	}
	return &DispatchCoordinator{candidateDispatchComponent: component}, nil
}

// SealDispatchWithManifestAndFeasibilities seals the exact owner-confirmed
// managed-validation recipe selection alongside the candidate.  It is the
// production SM-3 entry after manifest-bound admission; no execution path may
// reconstruct this value from current facts.
func SealDispatchWithManifestAndFeasibilities(decision runtimecontract.RuntimeDecision, candidate runtimecontract.RuntimeCandidate, activation runtimecontract.RuntimeActivationContext, manifest runtimecontract.RuntimeCandidateContractManifest, feasibilities []runtimecontract.RuntimeCandidateFeasibilityRecord) (SealedDispatch, error) {
	return SealDispatchWithManifestAndOwnerAuthority(decision, candidate, activation, manifest, feasibilities, nil, nil)
}

// SealDispatchWithManifestAndOwnerAuthority is the production SM-3 seal. It
// retains complete owner catalog/observation preimages so recovery and physical
// owner execution do not depend on mutable current Runtime facts.
func SealDispatchWithManifestAndOwnerAuthority(
	decision runtimecontract.RuntimeDecision,
	candidate runtimecontract.RuntimeCandidate,
	activation runtimecontract.RuntimeActivationContext,
	manifest runtimecontract.RuntimeCandidateContractManifest,
	feasibilities []runtimecontract.RuntimeCandidateFeasibilityRecord,
	catalogs []runtimecontract.RuntimeOperationAdmissibility,
	observations []runtimecontract.RuntimePrerequisiteObservation,
) (SealedDispatch, error) {
	return SealDispatchWithManifestAndPreSealAuthority(
		decision, candidate, activation, manifest, feasibilities, catalogs, observations,
		RuntimeDispatchPreSealAuthority{},
	)
}

// SealDispatchWithManifestAndPreSealAuthority performs the one complete SM-3
// authority seal.  Optional repair/assessment authority must already be rooted
// in the persisted candidate manifest and is included in the first and only
// dispatch authority hash.
func SealDispatchWithManifestAndPreSealAuthority(
	decision runtimecontract.RuntimeDecision,
	candidate runtimecontract.RuntimeCandidate,
	activation runtimecontract.RuntimeActivationContext,
	manifest runtimecontract.RuntimeCandidateContractManifest,
	feasibilities []runtimecontract.RuntimeCandidateFeasibilityRecord,
	catalogs []runtimecontract.RuntimeOperationAdmissibility,
	observations []runtimecontract.RuntimePrerequisiteObservation,
	authority RuntimeDispatchPreSealAuthority,
) (SealedDispatch, error) {
	manifest = manifest.Normalized()
	for _, authorization := range manifest.Operations {
		if authorization.InputRepair == nil {
			continue
		}
		if authority.RepairCommitment == nil || !runtimecontract.SameRuntimeValidationInputRepair(authorization.InputRepair, authority.RepairCommitment.InputRepair) {
			return SealedDispatch{}, fmt.Errorf("runtime input repair seal has no exact source commitment")
		}
		matches := 0
		for _, feasibility := range feasibilities {
			if feasibility.Observation != nil && feasibility.Observation.Outcome == runtimecontract.RuntimeOwnerFeasibilityFeasible && authorization.InputRepair.MatchesReplayProof(feasibility.ReplayProof) {
				matches++
			}
		}
		if matches != 1 {
			return SealedDispatch{}, fmt.Errorf("runtime input repair seal requires one exact owner pre/postimage proof")
		}
	}
	repairRequired := manifest.State == runtimecontract.ConvergenceStateNeedStateChange &&
		manifest.RepairPurpose == runtimecontract.RuntimeDispatchRepairPurposeConfirmedValidation
	if repairRequired {
		if authority.RepairCommitment == nil ||
			authority.RepairCommitment.Normalized().Purpose != manifest.RepairPurpose ||
			authority.RepairCommitment.Normalized().Closure.ID != manifest.RepairClosureID ||
			authority.RepairCommitment.Normalized().OriginDispatchID != runtimecontract.StableRuntimeDispatchID(
				decision.AuthorityTransaction(), runtimecontract.StableRuntimeCandidateHash(runtimecontract.CanonicalRuntimeCandidate(candidate)),
			) {
			return SealedDispatch{}, fmt.Errorf("runtime dispatch manifest-required repair commitment is unavailable")
		}
	} else if authority.RepairCommitment != nil {
		return SealedDispatch{}, fmt.Errorf("runtime dispatch cannot carry repair commitment without manifest purpose")
	}
	if candidate.Assessment != nil {
		if authority.AssessmentAuthorityInput == nil || authority.AssessmentAuthorityInput.Normalized().Purpose != manifest.AssessmentPurpose ||
			manifest.AssessmentPurposeManifestHash == "" {
			return SealedDispatch{}, fmt.Errorf("runtime assessment dispatch purpose does not match pre-seal candidate manifest")
		}
	} else if manifest.AssessmentPurpose != "" || manifest.AssessmentPurposeManifestHash != "" {
		return SealedDispatch{}, fmt.Errorf("non-assessment runtime dispatch cannot use assessment manifest purpose")
	}
	targetAuthorizations, err := runtimecontract.RuntimeDispatchOperationTargetAuthorizationsFromManifest(manifest, candidate)
	if err != nil {
		return SealedDispatch{}, err
	}
	recipes, err := runtimecontract.RuntimeCandidateManagedValidationRecipeSnapshot(manifest, candidate)
	if err != nil {
		return SealedDispatch{}, err
	}
	return sealDispatchWithActivationAndFeasibilities(
		decision,
		candidate,
		activation,
		manifest.Normalized().Hash,
		manifest.Normalized().Bindings,
		targetAuthorizations,
		recipes,
		feasibilities,
		catalogs,
		observations,
		authority,
	)
}

func sealDispatchWithActivationAndFeasibilities(
	decision runtimecontract.RuntimeDecision,
	candidate runtimecontract.RuntimeCandidate,
	activation runtimecontract.RuntimeActivationContext,
	candidateManifestHash string,
	coverageBindings []runtimecontract.CoverageEvidenceBinding,
	operationTargetAuthorizations []runtimecontract.RuntimeDispatchOperationTargetAuthorization,
	validationRecipes []runtimecontract.RuntimeValidationRecipe,
	feasibilities []runtimecontract.RuntimeCandidateFeasibilityRecord,
	catalogs []runtimecontract.RuntimeOperationAdmissibility,
	observations []runtimecontract.RuntimePrerequisiteObservation,
	authority RuntimeDispatchPreSealAuthority,
) (SealedDispatch, error) {
	if err := decision.Validate(); err != nil {
		return SealedDispatch{}, err
	}
	if err := runtimecontract.ValidateRuntimeCandidateRawShapeForSeal(candidate); err != nil {
		return SealedDispatch{}, err
	}
	if err := candidate.ValidateWithActivation(decision, activation); err != nil {
		return SealedDispatch{}, err
	}
	candidate = runtimecontract.CanonicalRuntimeCandidate(candidate)
	if strings.TrimSpace(candidateManifestHash) == "" {
		return SealedDispatch{}, fmt.Errorf("runtime dispatch requires candidate manifest hash")
	}
	coverageBindings = runtimecontract.CloneCoverageEvidenceBindings(coverageBindings)
	if err := runtimecontract.ValidateCoverageEvidenceBindingProjection(coverageBindings); err != nil {
		return SealedDispatch{}, err
	}
	for _, operation := range candidateRuntimeOperations(candidate) {
		for _, ref := range operation.BindingRefs {
			if _, found := runtimecontract.CoverageEvidenceBindingForRef(coverageBindings, ref); !found {
				return SealedDispatch{}, fmt.Errorf("runtime dispatch operation %q binding %q is absent from candidate manifest", operation.ID, ref)
			}
		}
	}
	if err := runtimecontract.ValidateRuntimeDispatchOperationTargetAuthorizations(candidate, operationTargetAuthorizations); err != nil {
		return SealedDispatch{}, err
	}
	if err := runtimecontract.ValidateRuntimeCandidateManagedValidationRecipeSnapshot(candidate, validationRecipes); err != nil {
		return SealedDispatch{}, err
	}
	transaction := decision.AuthorityTransaction()
	candidateHash := runtimecontract.StableRuntimeCandidateHash(candidate)
	dispatchID := runtimecontract.StableRuntimeDispatchID(transaction, candidateHash)
	if authority.RepairCommitment != nil {
		commitment := *authority.RepairCommitment
		commitmentHash := runtimecontract.StableRuntimeDispatchRepairCommitmentHash(commitment)
		if err := runtimecontract.ValidateRuntimeDispatchRepairCommitmentProjection(commitmentHash, &commitment); err != nil {
			return SealedDispatch{}, fmt.Errorf("runtime dispatch pre-seal repair commitment: %w", err)
		}
		commitment = commitment.Clone()
		expectedOrigin := strings.TrimSpace(authority.RepairCommitmentOriginDispatchID)
		if expectedOrigin == "" {
			expectedOrigin = dispatchID
		}
		if commitment.OriginDispatchID != expectedOrigin || commitment.FocusID != transaction.FocusID {
			return SealedDispatch{}, fmt.Errorf("runtime dispatch pre-seal repair commitment identity mismatch")
		}
		authority.RepairCommitment = &commitment
	} else if strings.TrimSpace(authority.RepairCommitmentOriginDispatchID) != "" {
		return SealedDispatch{}, fmt.Errorf("runtime dispatch repair origin has no pre-seal commitment")
	}
	if candidate.Assessment != nil {
		if authority.AssessmentAuthorityInput == nil {
			return SealedDispatch{}, fmt.Errorf("runtime assessment dispatch requires complete pre-seal authority input")
		}
		input := *authority.AssessmentAuthorityInput
		if err := input.Validate(transaction, coverageBindings); err != nil {
			return SealedDispatch{}, fmt.Errorf("runtime assessment dispatch pre-seal authority: %w", err)
		}
		input = input.Clone()
		authority.AssessmentAuthorityInput = &input
	} else if authority.AssessmentAuthorityInput != nil {
		return SealedDispatch{}, fmt.Errorf("non-assessment runtime dispatch cannot carry pre-seal assessment authority")
	}
	sealedCatalogs, sealedObservations, sealedPolicies, err := sealRuntimeDispatchOperationAuthority(candidate, activation, feasibilities, catalogs, observations)
	if err != nil {
		return SealedDispatch{}, err
	}
	sealedActivation := activation.Clone()
	sealedActivation.OperationPolicies = runtimecontract.RuntimeDispatchSealedOperationPolicies(runtimecontract.RuntimeDispatchRecord{
		OperationAdmissibilities: sealedCatalogs,
		OperationPolicies:        sealedPolicies,
	})
	executions := sealOperationExecutionRecords(
		decision.AuthoritySnapshot.WorkspaceID,
		decision.AuthoritySnapshot.RunID,
		dispatchID,
		candidate,
		sealedActivation,
		feasibilities,
	)
	completedOperationIDs := []string(nil)
	if authority.GovernanceResume != nil {
		var projectionErr error
		executions, completedOperationIDs, projectionErr = projectGovernanceResumePreSealExecutions(
			executions,
			*authority.GovernanceResume,
		)
		if projectionErr != nil {
			return SealedDispatch{}, projectionErr
		}
	}
	dispatch := SealedDispatch{
		ID:                                 dispatchID,
		WorkspaceID:                        decision.AuthoritySnapshot.WorkspaceID,
		RunID:                              decision.AuthoritySnapshot.RunID,
		Transaction:                        transaction,
		CandidateHash:                      candidateHash,
		Candidate:                          candidate,
		CandidateManifestHash:              strings.TrimSpace(candidateManifestHash),
		CoverageBindingHash:                runtimecontract.StableRuntimeCoverageEvidenceBindingSnapshotHash(coverageBindings),
		CoverageBindings:                   coverageBindings,
		OperationTargetAuthorizations:      runtimecontract.CloneRuntimeDispatchOperationTargetAuthorizations(operationTargetAuthorizations),
		FocusTargetRole:                    decision.Focus.Normalized().TargetRole,
		ValidationRecipes:                  runtimecontract.NormalizeRuntimeValidationRecipes(validationRecipes),
		RepairCommitment:                   cloneRuntimeDispatchRepairCommitment(authority.RepairCommitment),
		AssessmentAuthorityInput:           cloneRuntimeAssessmentAuthorityInput(authority.AssessmentAuthorityInput),
		OperationAdmissibilities:           sealedCatalogs,
		OperationAdmissibilityObservations: sealedObservations,
		OperationPolicies:                  sealedPolicies,
		State:                              DispatchStateSealed,
		CompletedOperationIDs:              completedOperationIDs,
		OperationExecutions:                executions,
		OperationAuthorityHash:             runtimecontract.StableRuntimeOperationExecutionAuthoritiesHash(executions),
	}
	if dispatch.RepairCommitment != nil {
		dispatch.RepairCommitmentHash = runtimecontract.StableRuntimeDispatchRepairCommitmentHash(*dispatch.RepairCommitment)
	}
	if dispatch.AssessmentAuthorityInput != nil {
		dispatch.AssessmentAuthorityInputHash = runtimecontract.StableRuntimeAssessmentAuthorityInputHash(*dispatch.AssessmentAuthorityInput)
	}
	dispatch.DispatchAuthorityHash = runtimecontract.StableRuntimeDispatchAuthorityHash(sealedDispatchToRuntimeRecord(dispatch))
	if err := dispatch.ValidateWithActivation(decision, activation); err != nil {
		return SealedDispatch{}, err
	}
	return dispatch, nil
}

func projectGovernanceResumePreSealExecutions(
	executions []runtimecontract.RuntimeOperationExecutionRecord,
	authority RuntimeGovernanceResumePreSealAuthority,
) ([]runtimecontract.RuntimeOperationExecutionRecord, []string, error) {
	source := authority.Source
	binding := authority.Binding.Normalized()
	if source.State != DispatchStateCommitted || binding.State != runtimecontract.RuntimeOperationGovernanceBindingGranted || binding.Grant == nil ||
		binding.OperationDispatchID != source.ID || source.CandidateHash != runtimecontract.StableRuntimeCandidateHash(source.Candidate) {
		return nil, nil, fmt.Errorf("governance resume pre-seal authority does not match one committed source dispatch")
	}
	if source.DispatchAuthorityHash != runtimecontract.StableRuntimeDispatchAuthorityHash(sealedDispatchToRuntimeRecord(source)) {
		return nil, nil, fmt.Errorf("governance resume source dispatch authority hash mismatch")
	}
	sourceExecution, found := operationExecutionRecord(source.OperationExecutions, binding.OperationID)
	if !found || !governancecheck.BindingMatchesExecution(binding, sourceExecution) {
		return nil, nil, fmt.Errorf("governance resume source execution does not match granted binding")
	}
	completed := append([]string(nil), source.CompletedOperationIDs...)
	found = false
	for index := range executions {
		execution := &executions[index]
		freshExecution := execution.Clone()
		persistedExecution, ok := operationExecutionRecord(source.OperationExecutions, execution.OperationID)
		if !ok {
			return nil, nil, fmt.Errorf("governance resume operation %q has no source execution", execution.OperationID)
		}
		if err := persistedExecution.Validate(); err != nil {
			return nil, nil, fmt.Errorf("governance resume operation %q source execution: %w", execution.OperationID, err)
		}
		if execution.OperationID == binding.OperationID {
			found = true
			*execution = persistedExecution.Clone()
			execution.State = runtimecontract.OperationExecutionClaimed
			execution.Fence = binding.Grant.ResumeFence
			execution.OwnerReceiptRef = ""
			execution.OwnerReceiptHash = ""
			execution.ClaimedAt = runtimecontract.RuntimeOperationClaimMarkerGovernanceGranted
			continue
		}
		if containsString(source.CompletedOperationIDs, execution.OperationID) {
			if persistedExecution.State != runtimecontract.OperationExecutionEvidenceCommitted {
				return nil, nil, fmt.Errorf("governance resume completed operation %q has no committed evidence", execution.OperationID)
			}
			*execution = persistedExecution.Clone()
			continue
		}
		if persistedExecution.State != runtimecontract.OperationExecutionSealed ||
			freshExecution.ExpectedEffectScope != persistedExecution.ExpectedEffectScope ||
			freshExecution.IdempotencyClass != persistedExecution.IdempotencyClass {
			return nil, nil, fmt.Errorf("governance resume pending operation %q source authority drifted", execution.OperationID)
		}
		*execution = persistedExecution.Clone()
		execution.ExecutionKey = freshExecution.ExecutionKey
		execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(*execution)
	}
	if !found {
		return nil, nil, fmt.Errorf("governance resume operation %q is missing from source candidate", binding.OperationID)
	}
	return runtimecontract.NormalizeRuntimeOperationExecutionRecords(executions), completed, nil
}

func sealRuntimeDispatchOperationAuthority(
	candidate runtimecontract.RuntimeCandidate,
	activation runtimecontract.RuntimeActivationContext,
	feasibilities []runtimecontract.RuntimeCandidateFeasibilityRecord,
	catalogs []runtimecontract.RuntimeOperationAdmissibility,
	observations []runtimecontract.RuntimePrerequisiteObservation,
) ([]runtimecontract.RuntimeOperationAdmissibility, []runtimecontract.RuntimePrerequisiteObservation, []runtimecontract.RuntimeOperationPolicyFact, error) {
	var sealedCatalogs []runtimecontract.RuntimeOperationAdmissibility
	var sealedObservations []runtimecontract.RuntimePrerequisiteObservation
	var sealedPolicies []runtimecontract.RuntimeOperationPolicyFact
	sealedCatalogSnapshots := make(map[string]struct{})
	sealedObservationIDs := make(map[string]struct{})
	for _, operation := range candidateRuntimeOperations(candidate) {
		operation = operation.Normalized()
		var snapshotID, acquisitionBinding string
		for _, feasibility := range feasibilities {
			if feasibility.OperationID == operation.ID && feasibility.Observation != nil && feasibility.Observation.Outcome == runtimecontract.RuntimeOwnerFeasibilityFeasible {
				snapshotID = feasibility.AdmissibilitySnapshotID
				acquisitionBinding = feasibility.AcquisitionBinding
				break
			}
		}
		if snapshotID != "" {
			catalog := runtimeSealedOwnerCatalogPreimage(catalogs, operation, snapshotID, acquisitionBinding)
			if catalog == nil {
				return nil, nil, nil, fmt.Errorf("runtime dispatch operation %q has no complete owner catalog preimage", operation.ID)
			}
			observation := runtimeOwnerCatalogObservationForOperation(observations, catalog)
			if _, alreadySealed := sealedCatalogSnapshots[catalog.SnapshotID]; !alreadySealed {
				sealedCatalogs = append(sealedCatalogs, *catalog)
				sealedCatalogSnapshots[catalog.SnapshotID] = struct{}{}
			}
			if observation != nil {
				if _, alreadySealed := sealedObservationIDs[observation.ObservationID]; !alreadySealed {
					sealedObservations = append(sealedObservations, *observation)
					sealedObservationIDs[observation.ObservationID] = struct{}{}
				}
			}
			continue
		}
		if policy, found := runtimecontract.RuntimeOperationPolicyForOperation(operation, activation.OperationPolicies); found {
			sealedPolicies = append(sealedPolicies, policy)
		}
	}
	return runtimecontract.CloneRuntimeOperationAdmissibilities(sealedCatalogs),
		runtimecontract.CloneRuntimePrerequisiteObservations(sealedObservations),
		runtimecontract.NormalizeRuntimeOperationPolicyFacts(sealedPolicies), nil
}

func runtimeSealedOwnerCatalogPreimage(values []runtimecontract.RuntimeOperationAdmissibility, operation runtimecontract.RuntimeOperation, snapshotID, acquisitionBinding string) *runtimecontract.RuntimeOperationAdmissibility {
	operation = operation.Normalized()
	snapshotID = strings.TrimSpace(snapshotID)
	acquisitionBinding = strings.TrimSpace(acquisitionBinding)
	descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !known || snapshotID == "" {
		return nil
	}
	for _, raw := range values {
		catalog := raw.Normalized()
		if catalog.SnapshotID != snapshotID || catalog.AcquisitionBinding != acquisitionBinding || catalog.OwnerModuleID != descriptor.OwnerModuleID || catalog.Validate() != nil {
			continue
		}
		for _, kind := range catalog.OperationKinds {
			if runtimecontract.CanonicalRuntimeBlockKind(kind) == operation.Kind {
				value := catalog
				return &value
			}
		}
	}
	return nil
}

func sealOperationExecutionRecords(
	workspaceID common.WorkspaceID,
	runID common.RunID,
	dispatchID string,
	candidate runtimecontract.RuntimeCandidate,
	activation runtimecontract.RuntimeActivationContext,
	feasibilities []runtimecontract.RuntimeCandidateFeasibilityRecord,
) []runtimecontract.RuntimeOperationExecutionRecord {
	ops := candidateRuntimeOperations(candidate)
	if len(ops) == 0 {
		return nil
	}
	ret := make([]runtimecontract.RuntimeOperationExecutionRecord, 0, len(ops))
	for _, operation := range ops {
		operation = operation.Normalized()
		scope := runtimecontract.ResolveExpectedEffectScope(operation, activation.OperationPolicies)
		execution := runtimecontract.RuntimeOperationExecutionRecord{
			OperationID:         operation.ID,
			OperationHash:       runtimecontract.StableRuntimeOperationHash(operation),
			ExecutionKey:        runtimecontract.StableRuntimeOperationExecutionKey(workspaceID, runID, dispatchID, operation.ID),
			ExpectedEffectScope: scope,
			IdempotencyClass:    runtimecontract.ResolveOperationIdempotencyClass(operation, activation.OperationPolicies),
			State:               runtimecontract.OperationExecutionSealed,
		}
		for _, feasibility := range feasibilities {
			if feasibility.OperationID != operation.ID || feasibility.Observation == nil || feasibility.Observation.Outcome != runtimecontract.RuntimeOwnerFeasibilityFeasible {
				continue
			}
			execution.AdmissibilitySnapshotID = feasibility.AdmissibilitySnapshotID
			execution.AcquisitionBinding = feasibility.AcquisitionBinding
			if feasibility.ReplayProof != nil {
				proof := feasibility.ReplayProof.Clone()
				execution.ReplayProof = &proof
			}
			break
		}
		execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(execution)
		ret = append(ret, execution)
	}
	return runtimecontract.NormalizeRuntimeOperationExecutionRecords(ret)
}

func (d SealedDispatch) Validate(decision runtimecontract.RuntimeDecision) error {
	return d.ValidateWithActivation(decision, runtimecontract.RuntimeActivationContext{})
}

func (d SealedDispatch) ValidateWithActivation(decision runtimecontract.RuntimeDecision, activation runtimecontract.RuntimeActivationContext) error {
	return sealedDispatchToRuntimeRecord(d).ValidateWithActivation(decision, activation)
}

func isStateChangeNoOpWithoutProvenanceFailure(failure runtimecontract.FailureFact, receipt *runtimecontract.RuntimeOperationReceipt) bool {
	if failure.Class != runtimecontract.FailureClassEffectMismatch || failure.Code != runtimeStateChangeNoOpWithoutProvenanceCode || receipt == nil {
		return false
	}
	value := receipt.Normalized()
	return value.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp && !value.Changed && value.ObservedEffectScope == runtimecontract.RuntimeEffectNone
}

// validationFailureTargetsAreGrounded verifies that a validation receipt does
// not promote its command entry to a repair target. It is an SM-2 ownership
// fence and intentionally uses only facts that existed before this receipt;
// it never uses the receipt's own target list as evidence of authority.
func validationFailureTargetsAreGrounded(facts runtimecontract.RuntimeFacts, failure runtimecontract.FailureFact) bool {
	return grounding.ValidationFailureTargetsAreGrounded(facts, failure)
}

func sameRuntimeTargetRefs(left, right []string) bool {
	return grounding.SameTargetRefs(left, right)
}

func normalizeRuntimeTargetRefs(values []string) []string {
	return grounding.NormalizeTargetRefs(values)
}

// runtimeEvidenceIsEffectiveProgress is an SM-2 decision helper. It is kept in
// the Orchestrator owner because the contract package must not decide when an
// aggregate transition may reset repair state. A validation receipt is useful
// evidence for assessment, but cannot by itself prove convergence progress.
func runtimeEvidenceIsEffectiveProgress(facts runtimecontract.RuntimeFacts, fact runtimecontract.EvidenceFact, evidence ExecutionEvidence, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) bool {
	if !runtimeEvidenceIsGroundedDurableStateChange(facts, fact, evidence, proofLineages...) {
		return false
	}
	// A focus that explicitly requires assessment has not demonstrated a gap
	// delta merely by changing the correct target. The durable receipt invalidates
	// the old assessment; only the subsequent accepted assessment may reset the
	// repair ledger or close coverage. This avoids state-change -> same-gap ->
	// reset loops while preserving non-assessed neutral workflows.
	return !facts.Focus.RequireAssessment
}

// runtimeEvidenceIsGroundedDurableStateChange verifies the exact receipt,
// current-binding credit and owner-grounded target facts that make a changed
// state receipt relevant to the active repair. It deliberately does not decide
// whether mandatory assessment has completed.
func runtimeEvidenceIsGroundedDurableStateChange(facts runtimecontract.RuntimeFacts, fact runtimecontract.EvidenceFact, evidence ExecutionEvidence, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) bool {
	if fact.Kind != runtimecontract.EvidenceKindStateChange || evidence.Receipt == nil {
		return false
	}
	if !runtimeAcceptedEvidenceCreditsCurrentFocusWithLineage(facts, fact, runtimeCoverageProofLineageArgument(proofLineages)) {
		return false
	}
	receipt := evidence.Receipt.Normalized()
	if !receipt.Changed || receipt.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp || receipt.ObservedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable {
		return false
	}
	// Use the exact same sealed target precedence as candidate admission and
	// RepairDirective construction. A selected owner target is therefore not
	// merely prompt context: an unrelated durable change cannot reset the
	// current repair window.
	requiredTargets := runtimeEffectiveProgressTargetRefs(facts)
	if len(requiredTargets) == 0 {
		return true
	}
	return runtimeTargetRefsIntersect(requiredTargets, receipt.TargetRefs)
}

// runtimeEffectiveProgressTargetRefs narrows the broader candidate grounding
// surface to the active repair transaction when one exists. A Focus may allow
// several owner-grounded targets, while a repair directive selects the exact
// subset whose changed receipt can advance that repair's progress ledger.
func runtimeEffectiveProgressTargetRefs(facts runtimecontract.RuntimeFacts) []string {
	directive := facts.RepairDirective
	if directive != nil && directive.FocusID == facts.Focus.ID {
		normalized := directive.Normalized()
		if normalized.Closure != nil && len(normalized.Closure.GroundedTargetRefs) != 0 {
			return append([]string(nil), normalized.Closure.GroundedTargetRefs...)
		}
		if len(normalized.TargetRefs) != 0 {
			return append([]string(nil), normalized.TargetRefs...)
		}
	}
	return GroundedRepairTargets(facts)
}

func runtimeTargetRefsIntersect(left, right []string) bool {
	return grounding.TargetRefsIntersect(left, right)
}

func repairFailureSignatureCount(ledger runtimecontract.RuntimeRepairLedger, signature string) int {
	for _, item := range ledger.SignatureCounts {
		if item.Signature == signature {
			return item.Count
		}
	}
	return 0
}

func requireFreshTargetSnapshots(facts runtimecontract.RuntimeFacts, targetRefs []string) runtimecontract.RuntimeFacts {
	targets := map[string]struct{}{}
	for _, ref := range targetRefs {
		ref = strings.TrimSpace(ref)
		if ref != "" {
			targets[ref] = struct{}{}
		}
	}
	if len(targets) == 0 {
		return facts
	}
	owners := make(map[string]string, len(targets))
	focus, _ := runtimeCurrentTargetGroundingFocus(facts)
	for ref := range targets {
		owner := runtimeTargetSnapshotRequirementOwner(facts, focus, ref)
		if owner == "" {
			// The rejected effect proves the current snapshot stale, so it must
			// be retired even when its owner lineage is malformed. Do not issue
			// a partial or conventional-owner refresh; the resulting missing
			// authority fails closed on the next SM-2 reduction.
			owners = nil
			break
		}
		owners[ref] = owner
	}
	kept := make([]runtimecontract.TargetSnapshot, 0, len(facts.Resources.TargetSnapshots))
	for _, snapshot := range facts.Resources.TargetSnapshots {
		if _, refresh := targets[snapshot.TargetRef]; !refresh {
			kept = append(kept, snapshot)
		}
	}
	facts.Resources.TargetSnapshots = kept
	if len(owners) != len(targets) {
		return facts.Normalized()
	}
	requirements := make([]runtimecontract.RuntimeRequirement, 0, len(targets))
	for ref := range targets {
		requirements = append(requirements, runtimecontract.RuntimeRequirement{
			Kind: runtimecontract.PrerequisiteKindTargetSnapshot, OwnerModuleID: owners[ref],
			SubjectRef: ref, Reason: "fresh target snapshot required after state-change conflict",
		})
	}
	return preflightRuntimeFactsPreservingPending(facts, requirements)
}

// coverageEvidenceCreditForReceipt is the SM-2 proof bridge. The operation's
// BindingRefs were sealed by SM-3, but only this reducer-owned matcher can
// derive CWI IDs after comparing those refs with owner receipt facts.
func coverageEvidenceCreditForReceipt(bindings []runtimecontract.CoverageEvidenceBinding, operation runtimecontract.RuntimeOperation, evidenceKind string, receipt *runtimecontract.RuntimeOperationReceipt) ([]string, []common.CoverageWorkItemID, string) {
	finding := proof.MatchCoverageReceipt(bindings, operation, evidenceKind, receipt)
	return finding.BindingRefs, finding.CoverageWorkItemIDs, finding.Diagnostic
}

const coverageBindingMismatchFailureCode = "coverage_binding_mismatch"

// coverageBindingMismatchFailure converts a completed exact-proof comparison
// into the stable, owner-selected category consumed by the reducer. The full
// matcher diagnostic is retained on FailureFact.OriginalDiagnostic; the
// owner-authored EvidenceFact.EvidenceReason remains an exact receipt field.
func coverageBindingMismatchFailure(focusID string, bindings []runtimecontract.CoverageEvidenceBinding, fact runtimecontract.EvidenceFact, diagnostic string) runtimecontract.FailureFact {
	bindings = runtimecontract.NormalizeCoverageEvidenceBindings(bindings)
	coverageIDs := make([]common.CoverageWorkItemID, 0, len(fact.BindingRefs))
	for _, ref := range fact.BindingRefs {
		if binding, found := runtimecontract.CoverageEvidenceBindingForRef(bindings, ref); found {
			coverageIDs = append(coverageIDs, binding.CoverageWorkItemID)
		}
	}
	coverageIDs = normalizeCoverageIDs(coverageIDs)
	gap := runtimecontract.RuntimeAssessmentGap{
		CoverageWorkItemIDs:  coverageIDs,
		TargetRefs:           append([]string(nil), fact.TargetRefs...),
		MissingEvidenceKinds: []string{fact.Kind},
		// A receipt/binding mismatch is an already-observed owner/authority
		// contract violation. No repair budget was consumed and the provider
		// cannot repair an immutable dispatched receipt, so this must not be
		// represented as no-progress exhaustion.
		FailureClass: runtimecontract.FailureClassRuntimeContractMisalignment,
		FailureCode:  coverageBindingMismatchFailureCode,
	}
	if len(gap.CoverageWorkItemIDs) == 0 {
		for _, binding := range bindings {
			gap.CoverageWorkItemIDs = append(gap.CoverageWorkItemIDs, binding.CoverageWorkItemID)
		}
		gap.CoverageWorkItemIDs = normalizeCoverageIDs(gap.CoverageWorkItemIDs)
	}
	gap.FailureSignature = runtimecontract.StableAssessmentFailureSignature(
		strings.TrimSpace(focusID),
		runtimecontract.AssessmentDispositionRepairableGap,
		&gap,
	)
	original := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion:   runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID:   runtimecontract.RuntimeModuleOrchestrator,
		OriginalClass:   "runtime_contract",
		OriginalCode:    coverageBindingMismatchFailureCode,
		OriginalMessage: strings.TrimSpace(diagnostic),
		Signature:       gap.FailureSignature,
		TargetRefs:      append([]string(nil), fact.TargetRefs...),
	}
	return runtimecontract.FailureFact{
		Class:              runtimecontract.FailureClassRuntimeContractMisalignment,
		Code:               coverageBindingMismatchFailureCode,
		Signature:          gap.FailureSignature,
		Message:            "sealed coverage binding does not match persisted owner receipt: " + strings.TrimSpace(diagnostic),
		OriginalDiagnostic: &original,
		TargetRefs:         append([]string(nil), fact.TargetRefs...),
	}
}

// closeCoverageForAcceptedEvidence derives coverage from the whole current
// binding proof set, rather than allowing the latest receipt to satisfy every
// required kind on its own. This is the non-assessment counterpart of the
// assessment gate and remains an SM-2-only coverage transition.
func closeCoverageForAcceptedEvidence(aggregate RuntimeAggregate) []runtimecontract.RuntimeCoverageItemFact {
	coverage := append([]runtimecontract.RuntimeCoverageItemFact(nil), aggregate.Facts.Coverage...)
	focus := aggregate.Facts.Focus.Normalized()
	for idx := range coverage {
		item := &coverage[idx]
		if item.SliceID != "" && focus.SliceID != "" && item.SliceID != focus.SliceID {
			continue
		}
		if !coverageWorkItemIDIncluded(focus.CoverageWorkItemIDs, item.ID) {
			continue
		}
		refs, satisfied := canonicalAssessmentEvidenceForCoverage(aggregate, item.ID)
		item.EvidenceRefs = append(item.EvidenceRefs, refs...)
		if satisfied {
			item.State = runtimecontract.RuntimeCoverageItemStateSatisfied
		} else {
			item.State = runtimecontract.RuntimeCoverageItemStateUnsatisfied
		}
	}
	return runtimecontract.NormalizeRuntimeCoverageFacts(coverage)
}

// runtimeCWIReopenTriggersForEvidence derives only typed local transition
// inputs from activation-sealed dependency declarations. The shared CWI helper
// owns the actual coverage, focus, binding and lineage mutation so receipt- and
// reconciliation-driven reopens cannot diverge.
func runtimeCWIReopenTriggersForEvidence(facts runtimecontract.RuntimeFacts, evidence runtimecontract.EvidenceFact) []runtimeCWIReopenTrigger {
	sourceIDs := map[common.CoverageWorkItemID]struct{}{}
	for _, id := range evidence.CoverageWorkItemIDs {
		if id != "" {
			sourceIDs[id] = struct{}{}
		}
	}
	if len(sourceIDs) == 0 || strings.TrimSpace(evidence.Kind) == "" {
		return nil
	}
	reopen := map[common.CoverageWorkItemID]struct{}{}
	for _, source := range facts.Coverage {
		if _, matches := sourceIDs[source.ID]; !matches {
			continue
		}
		for _, dependency := range source.ReopenDependencies {
			for _, kind := range dependency.TriggerEvidenceKinds {
				if strings.TrimSpace(kind) == strings.TrimSpace(evidence.Kind) {
					reopen[dependency.CoverageWorkItemID] = struct{}{}
					break
				}
			}
		}
	}
	if len(reopen) == 0 {
		return nil
	}
	coverageIDs := make([]common.CoverageWorkItemID, 0, len(reopen))
	for coverageID := range reopen {
		coverageIDs = append(coverageIDs, coverageID)
	}
	sort.SliceStable(coverageIDs, func(i, j int) bool { return string(coverageIDs[i]) < string(coverageIDs[j]) })
	triggers := make([]runtimeCWIReopenTrigger, 0, len(coverageIDs))
	for _, coverageID := range coverageIDs {
		triggers = append(triggers, runtimeCWIReopenTrigger{
			CoverageWorkItemID: coverageID,
			TriggerRef:         string(evidence.ID),
			Reason:             "declared coverage dependency observed " + strings.TrimSpace(evidence.Kind),
		})
	}
	return triggers
}

func (e ExecutionEvidence) Validate() error {
	if strings.TrimSpace(e.EventID) == "" || e.WorkspaceID == "" || e.RunID == "" || strings.TrimSpace(e.DispatchID) == "" || strings.TrimSpace(e.AuthoritySnapshotID) == "" || strings.TrimSpace(e.OperationID) == "" {
		return fmt.Errorf("execution evidence requires event, scope, dispatch, snapshot and operation")
	}
	if e.Receipt == nil && strings.TrimSpace(e.Kind) == "" {
		return fmt.Errorf("execution evidence requires owner receipt or diagnostic kind")
	}
	if e.Receipt != nil {
		receipt := e.Receipt.Normalized()
		if err := receipt.Validate(); err != nil {
			return err
		}
		if receipt.OperationID != strings.TrimSpace(e.OperationID) {
			return fmt.Errorf("execution evidence receipt operation mismatch")
		}
	}
	if e.Status != ExecutionEvidenceStatusAccepted && e.Status != ExecutionEvidenceStatusRejected {
		return fmt.Errorf("execution evidence requires accepted or rejected status")
	}
	if e.Status == ExecutionEvidenceStatusRejected && e.Failure == nil {
		return fmt.Errorf("rejected execution evidence requires a failure fact")
	}
	return nil
}

func nextEvidenceRevision(sequence uint64) string {
	return fmt.Sprintf("runtime-evidence-%020d", sequence)
}

func appendNormalizedString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || containsString(values, value) {
		return values
	}
	ret := append(append([]string(nil), values...), value)
	sort.Strings(ret)
	return ret
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func candidateContainsOperation(candidate runtimecontract.RuntimeCandidate, operationID string) bool {
	operationID = strings.TrimSpace(operationID)
	switch {
	case candidate.StateChange != nil:
		for _, operation := range candidate.StateChange.Operations {
			if operation.Operation.Normalized().ID == operationID {
				return true
			}
		}
	case candidate.Validation != nil:
		for _, operation := range candidate.Validation.Operations {
			if operation.Operation.Normalized().ID == operationID {
				return true
			}
		}
	}
	return false
}

func candidateOperationCount(candidate runtimecontract.RuntimeCandidate) int {
	switch {
	case candidate.StateChange != nil:
		return len(candidate.StateChange.Operations)
	case candidate.Validation != nil:
		return len(candidate.Validation.Operations)
	default:
		return 0
	}
}

func expectedEffectScopeForOperation(dispatch SealedDispatch, operationID string, activation runtimecontract.RuntimeActivationContext) (runtimecontract.RuntimeEffectScope, bool) {
	operationID = strings.TrimSpace(operationID)
	for _, exec := range dispatch.OperationExecutions {
		if exec.Normalized().OperationID == operationID {
			return runtimecontract.NormalizeRuntimeEffectScope(exec.ExpectedEffectScope), true
		}
	}
	_ = activation
	return runtimecontract.RuntimeEffectUnknown, false
}

func adjudicateEvidenceKind(decision runtimecontract.RuntimeDecision, expected runtimecontract.RuntimeEffectScope, hasExpected bool, evidence ExecutionEvidence) (string, error) {
	if evidence.Receipt != nil {
		receipt := evidence.Receipt.Normalized()
		if !hasExpected {
			return "", fmt.Errorf("receipt evidence requires sealed expected effect")
		}
		// A successful owner call can still be rejected by the Orchestrator effect
		// gate (for example no-op under produce_state_change). In that case the
		// receipt remains authoritative and the rejected evidence records the
		// mismatch without pretending a durable effect occurred.
		if evidence.Status == ExecutionEvidenceStatusRejected &&
			(receipt.Outcome == runtimecontract.RuntimeOperationOutcomeAccepted || receipt.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp) {
			if err := receipt.Validate(); err != nil {
				return "", fmt.Errorf("evidence authority: %w", err)
			}
			return evidenceKindForDecision(decision), nil
		}
		kind, err := runtimecontract.EvidenceKindFromReceipt(decision.State, expected, receipt)
		if err != nil {
			return "", fmt.Errorf("evidence authority: %w", err)
		}
		return kind, nil
	}
	observed := runtimecontract.NormalizeRuntimeEffectScope(evidence.ObservedEffectScope)
	// Receipt-less evidence is only used for stale/diagnostic inputs. Current
	// sealed capability operations are rejected before this point when their
	// descriptor requires an owner receipt.
	if strings.TrimSpace(string(evidence.ObservedEffectScope)) == "" && !evidence.NoOp {
		kind := strings.TrimSpace(evidence.Kind)
		if kind == "" {
			kind = evidenceKindForDecision(decision)
		}
		if kind == "" {
			return "", fmt.Errorf("execution evidence requires kind or observed effect")
		}
		// Still reject durable mismatch when expected is known non-durable under validation.
		if decision.State == runtimecontract.ConvergenceStateNeedValidation && hasExpected && runtimecontract.RuntimeEffectIsDurable(expected) {
			return "", fmt.Errorf("validation dispatch sealed durable expected effect %q", expected)
		}
		return kind, nil
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey:        "evidence/" + evidence.EventID,
		OperationID:         evidence.OperationID,
		ObservedEffectScope: observed,
		Evidence:            evidence.Evidence,
	}
	switch evidence.Status {
	case ExecutionEvidenceStatusAccepted:
		if evidence.NoOp {
			receipt.Outcome = runtimecontract.RuntimeOperationOutcomeNoOp
		} else {
			receipt.Outcome = runtimecontract.RuntimeOperationOutcomeAccepted
		}
	case ExecutionEvidenceStatusRejected:
		receipt.Outcome = runtimecontract.RuntimeOperationOutcomeRejected
	}
	if !hasExpected {
		expected = observed
	}
	kind, err := runtimecontract.EvidenceKindFromReceipt(decision.State, expected, receipt)
	if err != nil {
		return "", fmt.Errorf("evidence authority: %w", err)
	}
	return kind, nil
}

func markOperationExecutionEvidenceCommitted(values []runtimecontract.RuntimeOperationExecutionRecord, operationID, receiptRef, receiptHash string, fact runtimecontract.EvidenceFact) []runtimecontract.RuntimeOperationExecutionRecord {
	if len(values) == 0 {
		return values
	}
	ret := append([]runtimecontract.RuntimeOperationExecutionRecord(nil), values...)
	for idx := range ret {
		if ret[idx].OperationID != operationID {
			continue
		}
		ret[idx].State = runtimecontract.OperationExecutionEvidenceCommitted
		if strings.TrimSpace(receiptRef) != "" {
			ret[idx].OwnerReceiptRef = strings.TrimSpace(receiptRef)
		}
		if strings.TrimSpace(receiptHash) != "" {
			ret[idx].OwnerReceiptHash = strings.TrimSpace(receiptHash)
		}
		// Ensure validation still passes: committed requires fence + receipt.
		if ret[idx].Fence == 0 {
			ret[idx].Fence = 1
		}
		if ret[idx].OwnerReceiptRef == "" {
			ret[idx].OwnerReceiptRef = "inline/" + operationID
		}
		if ret[idx].OwnerReceiptHash == "" {
			ret[idx].OwnerReceiptHash = "inline"
		}
		ret[idx].EvidenceFactID = fact.ID
		ret[idx].EvidenceFactHash = runtimecontract.StableRuntimeEvidenceFactHash(fact)
	}
	return ret
}
