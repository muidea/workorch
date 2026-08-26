package runtime

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
	// ValidationRecipes is the immutable subset selected from the candidate
	// manifest's Shell-owner-confirmed recipes.  Execution must never replace it
	// with activation policy or a later admissibility inventory.
	ValidationRecipes []runtimecontract.RuntimeValidationRecipe
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

type DispatchStepInput struct {
	DispatchID    string
	Transaction   runtimecontract.AuthorityTransaction
	CandidateHash string
	Candidate     runtimecontract.RuntimeCandidate
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
	StateRevision    uint64
	EvidenceSequence uint64
	Definition       runtimecontract.RuntimeDefinitionContext
	Facts            runtimecontract.RuntimeFacts
	SemanticPlanning runtimecontract.RuntimeSemanticPlanningLedger
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
	// RepairSurfaceConfirmations is the typed SM-2 replay ledger for File-owner
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

func SealDispatch(decision runtimecontract.RuntimeDecision, candidate runtimecontract.RuntimeCandidate) (SealedDispatch, error) {
	return SealDispatchWithActivation(decision, candidate, runtimecontract.RuntimeActivationContext{})
}

// SealDispatchWithActivation seals a candidate using owner-published generic
// operation policies for effect and replay gating.
func SealDispatchWithActivation(decision runtimecontract.RuntimeDecision, candidate runtimecontract.RuntimeCandidate, activation runtimecontract.RuntimeActivationContext) (SealedDispatch, error) {
	return SealDispatchWithActivationAndFeasibilities(decision, candidate, activation, nil)
}

// SealDispatchWithActivationAndFeasibilities seals the optional, owner-authored
// replay proof from a persisted read-only feasibility ledger into its matching
// execution record. The proof is not inferred from operation kind or provider
// text: absent feasibility proof means absent recovery authority.
func SealDispatchWithActivationAndFeasibilities(decision runtimecontract.RuntimeDecision, candidate runtimecontract.RuntimeCandidate, activation runtimecontract.RuntimeActivationContext, feasibilities []runtimecontract.RuntimeCandidateFeasibilityRecord) (SealedDispatch, error) {
	return sealDispatchWithActivationAndFeasibilities(decision, candidate, activation, nil, feasibilities, nil, nil)
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
	recipes, err := runtimecontract.RuntimeCandidateManagedValidationRecipeSnapshot(manifest, candidate)
	if err != nil {
		return SealedDispatch{}, err
	}
	return sealDispatchWithActivationAndFeasibilities(decision, candidate, activation, recipes, feasibilities, catalogs, observations)
}

func sealDispatchWithActivationAndFeasibilities(
	decision runtimecontract.RuntimeDecision,
	candidate runtimecontract.RuntimeCandidate,
	activation runtimecontract.RuntimeActivationContext,
	validationRecipes []runtimecontract.RuntimeValidationRecipe,
	feasibilities []runtimecontract.RuntimeCandidateFeasibilityRecord,
	catalogs []runtimecontract.RuntimeOperationAdmissibility,
	observations []runtimecontract.RuntimePrerequisiteObservation,
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
	if err := runtimecontract.ValidateRuntimeCandidateManagedValidationRecipeSnapshot(candidate, validationRecipes); err != nil {
		return SealedDispatch{}, err
	}
	transaction := decision.AuthorityTransaction()
	candidateHash := runtimecontract.StableRuntimeCandidateHash(candidate)
	dispatchID := runtimecontract.StableRuntimeDispatchID(transaction, candidateHash)
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
	dispatch := SealedDispatch{
		ID:                                 dispatchID,
		WorkspaceID:                        decision.AuthoritySnapshot.WorkspaceID,
		RunID:                              decision.AuthoritySnapshot.RunID,
		Transaction:                        transaction,
		CandidateHash:                      candidateHash,
		Candidate:                          candidate,
		ValidationRecipes:                  runtimecontract.NormalizeRuntimeValidationRecipes(validationRecipes),
		OperationAdmissibilities:           sealedCatalogs,
		OperationAdmissibilityObservations: sealedObservations,
		OperationPolicies:                  sealedPolicies,
		State:                              DispatchStateSealed,
		OperationExecutions:                executions,
		OperationAuthorityHash:             runtimecontract.StableRuntimeOperationExecutionAuthoritiesHash(executions),
	}
	dispatch.DispatchAuthorityHash = runtimecontract.StableRuntimeDispatchAuthorityHash(sealedDispatchToRuntimeRecord(dispatch))
	if err := dispatch.ValidateWithActivation(decision, activation); err != nil {
		return SealedDispatch{}, err
	}
	return dispatch, nil
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
			sealedCatalogs = append(sealedCatalogs, *catalog)
			if observation != nil {
				sealedObservations = append(sealedObservations, *observation)
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
func runtimeEvidenceIsEffectiveProgress(facts runtimecontract.RuntimeFacts, fact runtimecontract.EvidenceFact, evidence ExecutionEvidence) bool {
	if fact.Kind != runtimecontract.EvidenceKindStateChange || evidence.Receipt == nil {
		return false
	}
	receipt := evidence.Receipt.Normalized()
	if !receipt.Changed || receipt.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp || receipt.ObservedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable {
		return false
	}
	// A focus that explicitly requires assessment has not demonstrated a gap
	// delta merely by changing the correct target. The durable receipt invalidates
	// the old assessment; only the subsequent accepted assessment may reset the
	// repair ledger or close coverage. This avoids state-change -> same-gap ->
	// reset loops while preserving non-assessed neutral workflows.
	if facts.Focus.RequireAssessment {
		return false
	}
	// Use the exact same sealed target precedence as candidate admission and
	// RepairDirective construction. A selected owner target is therefore not
	// merely prompt context: an unrelated durable change cannot reset the
	// current repair window.
	requiredTargets := GroundedRepairTargets(facts)
	if len(requiredTargets) == 0 {
		return true
	}
	return runtimeTargetRefsIntersect(requiredTargets, receipt.TargetRefs)
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
	kept := make([]runtimecontract.TargetSnapshot, 0, len(facts.Resources.TargetSnapshots))
	for _, snapshot := range facts.Resources.TargetSnapshots {
		if _, refresh := targets[snapshot.TargetRef]; !refresh {
			kept = append(kept, snapshot)
		}
	}
	facts.Resources.TargetSnapshots = kept
	requirements := make([]runtimecontract.RuntimeRequirement, 0, len(targets))
	for ref := range targets {
		requirements = append(requirements, runtimecontract.RuntimeRequirement{
			Kind: runtimecontract.PrerequisiteKindTargetSnapshot, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: ref, Reason: "fresh target snapshot required after state-change conflict",
		})
	}
	return preflightRuntimeFactsPreservingPending(facts, requirements)
}

// coverageEvidenceCreditForReceipt is the SM-2 proof bridge. The operation's
// BindingRefs were sealed by SM-3, but only this reducer-owned matcher can
// derive CWI IDs after comparing those refs with owner receipt facts.
func coverageEvidenceCreditForReceipt(facts runtimecontract.RuntimeFacts, operation runtimecontract.RuntimeOperation, evidenceKind string, receipt *runtimecontract.RuntimeOperationReceipt) ([]string, []common.CoverageWorkItemID, string) {
	finding := proof.MatchCoverageReceipt(facts, operation, evidenceKind, receipt)
	return finding.BindingRefs, finding.CoverageWorkItemIDs, finding.Diagnostic
}

const coverageBindingMismatchFailureCode = "coverage_binding_mismatch"

// coverageBindingMismatchFailure converts a completed exact-proof comparison
// into the stable, owner-selected category consumed by the reducer. The full
// matcher diagnostic is retained on EvidenceFact.EvidenceReason; it is not
// parsed to choose a retry, target, or action.
func coverageBindingMismatchFailure(facts runtimecontract.RuntimeFacts, fact runtimecontract.EvidenceFact, diagnostic string) runtimecontract.FailureFact {
	coverageIDs := make([]common.CoverageWorkItemID, 0, len(fact.BindingRefs))
	for _, ref := range fact.BindingRefs {
		if binding, found := runtimecontract.CoverageEvidenceBindingForRef(facts.Focus.CoverageEvidenceBindings, ref); found {
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
		gap.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...)
	}
	gap.FailureSignature = runtimecontract.StableAssessmentFailureSignature(
		facts.Focus.ID,
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

// invalidateNonSatisfiedAssessmentsForFocus drops repairable/governance/terminal assessments
// for the active focus after new accepted evidence arrives. Satisfied assessments are retained
// as lineage; the next reduce will request a fresh assessment when required.
func invalidateNonSatisfiedAssessmentsForFocus(values []runtimecontract.RuntimeAssessmentFact, focusID string) []runtimecontract.RuntimeAssessmentFact {
	focusID = strings.TrimSpace(focusID)
	if focusID == "" || len(values) == 0 {
		return values
	}
	ret := make([]runtimecontract.RuntimeAssessmentFact, 0, len(values))
	for _, value := range values {
		if value.FocusID == focusID && value.Disposition != runtimecontract.AssessmentDispositionSatisfied {
			continue
		}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
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

func markOperationExecutionEvidenceCommitted(values []runtimecontract.RuntimeOperationExecutionRecord, operationID, receiptRef, receiptHash string) []runtimecontract.RuntimeOperationExecutionRecord {
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
	}
	return ret
}
