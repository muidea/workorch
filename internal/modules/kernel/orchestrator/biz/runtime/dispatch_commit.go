package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

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
	ValidationRecipes     []runtimecontract.RuntimeValidationRecipe
	State                 DispatchState
	CompletedOperationIDs []string
	// OperationExecutions is the per-operation claim/fence ledger (RVC-R9/R10).
	OperationExecutions []runtimecontract.RuntimeOperationExecutionRecord
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
	Reconciliation      runtimecontract.RuntimeCWIReconciliationLedger
	Decision            runtimecontract.RuntimeDecision
	ActiveDispatch      *SealedDispatch
	CompletedDispatches []SealedDispatch
	ProcessedEventIDs   []string
	// RepairSurfaceConfirmations is the typed SM-2 replay ledger for File-owner
	// repair-surface facts.  It is separate from generic event IDs so owner
	// confirmation idempotency cannot become a global event index.
	RepairSurfaceConfirmations  []runtimecontract.RuntimeRepairSurfaceConfirmationRecord
	Diagnostics                 []runtimecontract.RuntimeDiagnosticRecord
	CandidateRequestSessions    []runtimecontract.RuntimeCandidateRequestSession
	CandidateAttempts           []runtimecontract.RuntimeCandidateAttemptRecord
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

type DispatchCoordinator struct {
	repository  RuntimeStateRepository
	transitions *RuntimeTransitionCoordinator
}

func NewDispatchCoordinator(repository RuntimeStateRepository) (*DispatchCoordinator, error) {
	transitions, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		return nil, err
	}
	return newDispatchCoordinator(repository, transitions)
}

func newDispatchCoordinator(repository RuntimeStateRepository, transitions *RuntimeTransitionCoordinator) (*DispatchCoordinator, error) {
	if repository == nil || transitions == nil {
		return nil, common.ErrInvalidArgument
	}
	return &DispatchCoordinator{repository: repository, transitions: transitions}, nil
}

func SealDispatch(decision runtimecontract.RuntimeDecision, candidate runtimecontract.RuntimeCandidate) (SealedDispatch, error) {
	return SealDispatchWithActivation(decision, candidate, runtimecontract.RuntimeActivationContext{})
}

// SealDispatchWithActivation seals a candidate using owner-sealed activation capability
// metadata for side-effect gating (MCP RiskLevel, etc.).
func SealDispatchWithActivation(decision runtimecontract.RuntimeDecision, candidate runtimecontract.RuntimeCandidate, activation runtimecontract.RuntimeActivationContext) (SealedDispatch, error) {
	return SealDispatchWithActivationAndFeasibilities(decision, candidate, activation, nil)
}

// SealDispatchWithActivationAndFeasibilities seals the optional, owner-authored
// replay proof from a persisted read-only feasibility ledger into its matching
// execution record. The proof is not inferred from operation kind or provider
// text: absent feasibility proof means absent recovery authority.
func SealDispatchWithActivationAndFeasibilities(decision runtimecontract.RuntimeDecision, candidate runtimecontract.RuntimeCandidate, activation runtimecontract.RuntimeActivationContext, feasibilities []runtimecontract.RuntimeCandidateFeasibilityRecord) (SealedDispatch, error) {
	return sealDispatchWithActivationAndFeasibilities(decision, candidate, activation, nil, feasibilities)
}

// SealDispatchWithManifestAndFeasibilities seals the exact owner-confirmed
// managed-validation recipe selection alongside the candidate.  It is the
// production SM-3 entry after manifest-bound admission; no execution path may
// reconstruct this value from current facts.
func SealDispatchWithManifestAndFeasibilities(decision runtimecontract.RuntimeDecision, candidate runtimecontract.RuntimeCandidate, activation runtimecontract.RuntimeActivationContext, manifest runtimecontract.RuntimeCandidateContractManifest, feasibilities []runtimecontract.RuntimeCandidateFeasibilityRecord) (SealedDispatch, error) {
	recipes, err := runtimecontract.RuntimeCandidateManagedValidationRecipeSnapshot(manifest, candidate)
	if err != nil {
		return SealedDispatch{}, err
	}
	return sealDispatchWithActivationAndFeasibilities(decision, candidate, activation, recipes, feasibilities)
}

func sealDispatchWithActivationAndFeasibilities(decision runtimecontract.RuntimeDecision, candidate runtimecontract.RuntimeCandidate, activation runtimecontract.RuntimeActivationContext, validationRecipes []runtimecontract.RuntimeValidationRecipe, feasibilities []runtimecontract.RuntimeCandidateFeasibilityRecord) (SealedDispatch, error) {
	if err := decision.Validate(); err != nil {
		return SealedDispatch{}, err
	}
	if err := candidate.ValidateWithActivation(decision, activation); err != nil {
		return SealedDispatch{}, err
	}
	if err := runtimecontract.ValidateRuntimeCandidateManagedValidationRecipeSnapshot(candidate, validationRecipes); err != nil {
		return SealedDispatch{}, err
	}
	transaction := decision.AuthorityTransaction()
	candidateHash := runtimecontract.StableRuntimeCandidateHash(candidate)
	digest := sha256.Sum256([]byte(strings.Join([]string{transaction.AuthoritySnapshotID, transaction.RequiredOutputHash, candidateHash}, "|")))
	dispatchID := "dispatch-" + hex.EncodeToString(digest[:])[:20]
	executions := sealOperationExecutionRecords(
		decision.AuthoritySnapshot.WorkspaceID,
		decision.AuthoritySnapshot.RunID,
		dispatchID,
		candidate,
		activation,
		feasibilities,
	)
	return SealedDispatch{
		ID:                  dispatchID,
		WorkspaceID:         decision.AuthoritySnapshot.WorkspaceID,
		RunID:               decision.AuthoritySnapshot.RunID,
		Transaction:         transaction,
		CandidateHash:       candidateHash,
		Candidate:           candidate,
		ValidationRecipes:   runtimecontract.NormalizeRuntimeValidationRecipes(validationRecipes),
		State:               DispatchStateSealed,
		OperationExecutions: executions,
	}, nil
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
		scope := runtimecontract.ResolveExpectedEffectScope(operation, activation.MCPCapabilities)
		execution := runtimecontract.RuntimeOperationExecutionRecord{
			OperationID:         operation.ID,
			ExecutionKey:        runtimecontract.StableRuntimeOperationExecutionKey(workspaceID, runID, dispatchID, operation.ID),
			ExpectedEffectScope: scope,
			State:               runtimecontract.OperationExecutionSealed,
		}
		for _, feasibility := range feasibilities {
			if feasibility.OperationID != operation.ID || feasibility.Observation == nil || feasibility.Observation.Outcome != runtimecontract.RuntimeOwnerFeasibilityFeasible || feasibility.ReplayProof == nil {
				continue
			}
			proof := feasibility.ReplayProof.Clone()
			execution.ReplayProof = &proof
			break
		}
		ret = append(ret, execution)
	}
	return runtimecontract.NormalizeRuntimeOperationExecutionRecords(ret)
}

func (d SealedDispatch) Validate(decision runtimecontract.RuntimeDecision) error {
	return d.ValidateWithActivation(decision, runtimecontract.RuntimeActivationContext{})
}

func (d SealedDispatch) ValidateWithActivation(decision runtimecontract.RuntimeDecision, activation runtimecontract.RuntimeActivationContext) error {
	if strings.TrimSpace(d.ID) == "" || d.WorkspaceID == "" || d.RunID == "" || strings.TrimSpace(d.CandidateHash) == "" {
		return fmt.Errorf("sealed dispatch requires id, scope and candidate hash")
	}
	if err := runtimecontract.ValidateAuthorityTransaction(decision, d.Transaction); err != nil {
		return err
	}
	if d.WorkspaceID != decision.AuthoritySnapshot.WorkspaceID || d.RunID != decision.AuthoritySnapshot.RunID {
		return fmt.Errorf("sealed dispatch scope mismatch")
	}
	if err := d.Candidate.ValidateWithActivation(decision, activation); err != nil {
		return err
	}
	if d.CandidateHash != runtimecontract.StableRuntimeCandidateHash(d.Candidate) {
		return fmt.Errorf("sealed dispatch candidate hash mismatch")
	}
	if d.State != DispatchStateSealed && d.State != DispatchStateCommitted && d.State != DispatchStateInvalidated {
		return fmt.Errorf("sealed dispatch has invalid state %q", d.State)
	}
	for _, operationID := range d.CompletedOperationIDs {
		if !candidateContainsOperation(d.Candidate, operationID) {
			return fmt.Errorf("sealed dispatch completed operation %q is not present in candidate", operationID)
		}
	}
	if err := runtimecontract.ValidateRuntimeOperationExecutionRecords(d.OperationExecutions); err != nil {
		return err
	}
	for _, exec := range runtimecontract.NormalizeRuntimeOperationExecutionRecords(d.OperationExecutions) {
		if !candidateContainsOperation(d.Candidate, exec.OperationID) {
			return fmt.Errorf("sealed dispatch operation execution %q is not present in candidate", exec.OperationID)
		}
	}
	return nil
}

func (c *DispatchCoordinator) commitEvidence(ctx context.Context, evidence ExecutionEvidence) (EvidenceCommitResult, error) {
	if c == nil || c.repository == nil {
		return EvidenceCommitResult{}, common.ErrInvalidArgument
	}
	if err := evidence.Validate(); err != nil {
		return EvidenceCommitResult{}, err
	}
	aggregate, err := c.repository.Load(ctx, evidence.WorkspaceID, evidence.RunID)
	if err != nil {
		return EvidenceCommitResult{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return EvidenceCommitResult{}, err
	}
	// Apply the owner-local receipt transition before validating the complete
	// next aggregate. This permits deterministic recovery of an active dispatch
	// sealed by an older contract whose mixed-effect shape is invalid under the
	// current policy. RuntimeTransitionCoordinator validates the post-transition
	// aggregate before the only CAS write, so partial corrupt state is impossible.
	if containsString(aggregate.ProcessedEventIDs, evidence.EventID) {
		return EvidenceCommitResult{Disposition: CommitDispositionDuplicate, Aggregate: aggregate}, nil
	}
	expectedRevision := aggregate.StateRevision
	currentSnapshotID := aggregate.Decision.AuthoritySnapshot.ID
	if evidence.AuthoritySnapshotID != currentSnapshotID {
		aggregate.ProcessedEventIDs = appendNormalizedString(aggregate.ProcessedEventIDs, evidence.EventID)
		aggregate.Diagnostics = append(aggregate.Diagnostics, runtimecontract.RuntimeDiagnosticRecord{
			EventID: evidence.EventID, AuthoritySnapshotID: evidence.AuthoritySnapshotID, CurrentSnapshotID: currentSnapshotID,
			DispatchID: evidence.DispatchID, Reason: "execution evidence belongs to a stale authority snapshot",
		})
		aggregate.StateRevision++
		if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
			return EvidenceCommitResult{}, err
		}
		return EvidenceCommitResult{Disposition: CommitDispositionStale, Aggregate: aggregate}, nil
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.ID != evidence.DispatchID || aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID != evidence.AuthoritySnapshotID {
		return EvidenceCommitResult{}, fmt.Errorf("execution evidence does not match the active sealed dispatch")
	}
	if !candidateContainsOperation(aggregate.ActiveDispatch.Candidate, evidence.OperationID) {
		return EvidenceCommitResult{}, fmt.Errorf("execution evidence operation %q is not present in the sealed candidate", evidence.OperationID)
	}
	if evidence.Receipt != nil {
		persisted, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, evidence.Receipt.ExecutionKey, evidence.Receipt.Fence)
		if !found || !runtimecontract.SameRuntimeOperationReceipt(persisted, *evidence.Receipt) {
			return EvidenceCommitResult{}, fmt.Errorf("execution evidence owner receipt is not persisted")
		}
		execution, found := operationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, evidence.OperationID)
		if !found || execution.State != runtimecontract.OperationExecutionOwnerCommitted || execution.ExecutionKey != persisted.ExecutionKey {
			return EvidenceCommitResult{}, fmt.Errorf("execution evidence requires owner_committed execution state")
		}
		// The persisted owner receipt is the sole evidence-fact source.  The
		// surrounding internal event carries routing/status information only;
		// it must not be able to substitute a more favourable quality, check,
		// metric or artifact observation after the receipt has been sealed.
		// This is deliberately before both the binding matcher and the SM-2
		// coverage reducer, so all subsequent paths share the same owner fact.
		evidence.Receipt = &persisted
		evidence.Evidence = persisted.Evidence
		evidence.ObservedEffectScope = persisted.ObservedEffectScope
		evidence.NoOp = persisted.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp
	}
	var committedOperation runtimecontract.RuntimeOperation
	for _, operation := range candidateRuntimeOperations(aggregate.ActiveDispatch.Candidate) {
		if operation.Normalized().ID == evidence.OperationID {
			committedOperation = operation.Normalized()
			break
		}
	}
	descriptor, descriptorFound := runtimecontract.RuntimeOperationDescriptorForKind(committedOperation.Kind)
	if !descriptorFound {
		return EvidenceCommitResult{}, fmt.Errorf("execution evidence operation %q has no descriptor", committedOperation.Kind)
	}
	if descriptor.RequiresOwnerReceipt && evidence.Receipt == nil {
		return EvidenceCommitResult{}, fmt.Errorf("execution evidence operation %q requires persisted owner receipt", committedOperation.Kind)
	}
	expectedScope, hasExecution := expectedEffectScopeForOperation(*aggregate.ActiveDispatch, evidence.OperationID, aggregate.Facts.Activation)
	if !hasExecution {
		return EvidenceCommitResult{}, fmt.Errorf("execution evidence operation %q has no sealed execution record", evidence.OperationID)
	}
	evidenceKind, err := adjudicateEvidenceKind(aggregate.Decision, expectedScope, hasExecution, evidence)
	if err != nil {
		return EvidenceCommitResult{}, err
	}
	bindingRefs, coverageIDs, bindingDiagnostic := coverageEvidenceCreditForReceipt(aggregate.Facts, committedOperation, evidenceKind, evidence.Receipt)
	fact := runtimecontract.EvidenceFact{
		ID: common.EvidenceRecordID(evidence.EventID), Kind: evidenceKind, SubjectRef: strings.TrimSpace(evidence.OperationID),
		SourceRef: strings.TrimSpace(evidence.SourceRef), AuthoritySnapshotID: evidence.AuthoritySnapshotID,
		DispatchID: evidence.DispatchID, OperationID: evidence.OperationID, Revision: nextEvidenceRevision(aggregate.EvidenceSequence + 1),
		Summary: evidence.Evidence.Summary, ContentHash: evidence.Evidence.ContentHash,
		ExitCode: evidence.Evidence.ExitCode, EvidenceRefs: append([]string(nil), evidence.Evidence.EvidenceRefs...),
		TargetRefs:          append([]string(nil), committedOperation.TargetRefs...),
		BindingRefs:         bindingRefs,
		CoverageWorkItemIDs: coverageIDs,
		Quality:             evidence.Evidence.Quality, ZeroWork: evidence.Evidence.ZeroWork,
		EvidenceReason: evidence.Evidence.EvidenceReason, EvidenceTags: append([]string(nil), evidence.Evidence.EvidenceTags...),
	}
	if bindingDiagnostic != "" {
		fact.EvidenceReason = strings.TrimSpace(strings.Join([]string{fact.EvidenceReason, bindingDiagnostic}, "; "))
	}
	if evidence.Evidence.Validation != nil {
		observation := evidence.Evidence.Validation.Clone()
		fact.Validation = &observation
	}
	// Mark operation execution as evidence_committed when ledger present.
	aggregate.ActiveDispatch.OperationExecutions = markOperationExecutionEvidenceCommitted(
		aggregate.ActiveDispatch.OperationExecutions, evidence.OperationID, evidence.OwnerReceiptRef, evidence.OwnerReceiptHash,
	)
	switch evidence.Status {
	case ExecutionEvidenceStatusAccepted:
		aggregate.Facts.Evidence.Accepted = append(aggregate.Facts.Evidence.Accepted, fact)
		// An operation that selected one or more sealed bindings was authorized to
		// close coverage only through those exact proofs.  If its persisted owner
		// receipt earns no credit at all, another provider request cannot repair
		// that already-observed receipt: the manifest, recipe, target and effect
		// are immutable for this dispatch.  Record the full mismatch on the
		// evidence lineage and reduce a stable SM-2 terminal instead of allowing
		// SM-3 bookkeeping/evidence revisions to create an unbounded validation
		// loop.  Operations without BindingRefs remain permitted non-credit
		// diagnostics and deliberately do not take this branch.
		if len(fact.BindingRefs) != 0 && len(fact.CoverageWorkItemIDs) == 0 && bindingDiagnostic != "" {
			failure := coverageBindingMismatchFailure(aggregate.Facts, fact, bindingDiagnostic)
			aggregate.Facts.Failure = &failure
		}
		if !aggregate.Facts.Focus.RequireAssessment {
			aggregate.Facts.Coverage = closeCoverageForAcceptedEvidence(aggregate)
		}
		// A profile-declared dependency is the only authority for invalidating a
		// previously accepted validation/assessment coverage item after a source
		// effect changes.  Keep both evidence lineages: a reopen means that the
		// earlier proof is stale for convergence, not that the owner receipt was
		// false.  This happens inside the same aggregate CAS as evidence admission.
		var reopenedCoverage []common.CoverageWorkItemID
		aggregate.Facts.Coverage, reopenedCoverage = reopenCoverageDependenciesForEvidence(aggregate.Facts.Coverage, fact)
		aggregate.Facts = requeueReopenedCoverageFocuses(aggregate.Facts, reopenedCoverage)
		if fact.Kind == runtimecontract.EvidenceKindStateChange {
			currentCoverage := append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...)
			for _, focus := range aggregate.Facts.RemainingFocuses {
				currentCoverage = append(currentCoverage, focus.CoverageWorkItemIDs...)
			}
			aggregate.Facts.StateChange = applyAcceptedStateChangeEvidence(aggregate.Facts.StateChange, fact, currentCoverage)
		}
		// A newly admitted receipt may require a fresh assessment, but admission is
		// not itself convergence progress. In particular, a validation receipt that
		// later recreates the same gap must retain the same-focus repair and
		// validation-only budgets. Only a durable, changed state effect resets those
		// windows here; assessment satisfaction owns the other progress transition.
		if runtimecontract.EvidenceAppliesToCurrentFocus(aggregate.Facts, fact) {
			aggregate.Facts.Assessments = invalidateNonSatisfiedAssessmentsForFocus(aggregate.Facts.Assessments, aggregate.Facts.Focus.ID)
			// Progress must be evaluated against the failure/repair target that was
			// authoritative before this accepted receipt clears the transient failure.
			// Otherwise a repair can change an unrelated validation entry and reset
			// the same-focus budget without shrinking the persisted gap.
			if runtimeEvidenceIsEffectiveProgress(aggregate.Facts, fact, evidence) {
				aggregate.Facts.Repair = noteRepairProgress(aggregate.Facts.Repair, fact.Revision)
			} else if fact.Kind == runtimecontract.EvidenceKindValidation && aggregate.Facts.Repair.ActiveGap != nil {
				aggregate.Facts.Repair = recordNoProgressOperationShape(aggregate.Facts.Repair, runtimecontract.RuntimeRejectedOperationShape{
					Operation: committedOperation, EvidenceID: fact.ID,
					FailureClass:     runtimecontract.FailureClassCoverageAssessmentIncomplete,
					FailureCode:      "accepted_validation_pending_assessment",
					FailureSignature: "accepted-validation-pending-assessment",
					FailureMessage:   "accepted validation has not yet resolved the active assessment gap",
				})
			}
			// A binding mismatch is an owner-fact/authorization mismatch. It is a
			// determined no-progress outcome, not a transient failure that an
			// accepted receipt may clear.
			if aggregate.Facts.Failure == nil || aggregate.Facts.Failure.Code != coverageBindingMismatchFailureCode {
				aggregate.Facts.Failure = nil
			}
		}
	case ExecutionEvidenceStatusRejected:
		if evidence.Failure != nil {
			fact.FailureClass = evidence.Failure.Class
			fact.FailureCode = evidence.Failure.Code
			fact.FailureSignature = evidence.Failure.Signature
			fact.FailureMessage = evidence.Failure.Message
			failure := *evidence.Failure
			// A validation operation's own target is an execution entry, not proof
			// that the target is the object requiring repair. Do not bootstrap
			// FailureTarget authority from that declaration: only a target already
			// grounded by the Focus, a prior owner fact, selected owner target, or
			// sealed directive may be retained. State-changing owner failures remain
			// concrete effect failures and preserve their operation target lineage.
			if fact.Kind == runtimecontract.EvidenceKindValidation && !validationFailureTargetsAreGrounded(aggregate.Facts, failure) {
				failure.TargetRefs = nil
				failure.TargetObservation = nil
			}
			aggregate.Facts.Failure = &failure
			// A receipt is the only owner-authored source for an execution failure.
			// Persist its bounded target set as repair context before the next SM-2
			// reduction so candidate rendering never has to infer targets from text.
			if len(failure.TargetRefs) > 0 {
				aggregate.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{
					Signature:  failure.Signature,
					TargetRefs: append([]string(nil), failure.TargetRefs...),
				}
				if failure.TargetObservation != nil {
					observation := failure.TargetObservation.Normalized()
					aggregate.Facts.Resources.FailureTarget.TargetObservation = &observation
				}
			}
			gap := runtimecontract.RuntimeAssessmentGap{
				CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), fact.CoverageWorkItemIDs...),
				TargetRefs:          append([]string(nil), failure.TargetRefs...), Reason: failure.Message,
				FailureClass: failure.Class, FailureCode: failure.Code, FailureSignature: failure.Signature,
			}
			if len(gap.CoverageWorkItemIDs) == 0 {
				gap.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...)
			}
			if len(gap.TargetRefs) == 0 {
				gap.TargetRefs = append([]string(nil), aggregate.Facts.Focus.TargetRefs...)
			}
			switch fact.Kind {
			case runtimecontract.EvidenceKindValidation:
				gap.MissingEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
			default:
				gap.MissingEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
			}
			signature := strings.TrimSpace(failure.Signature)
			if signature == "" {
				signature = runtimecontract.StableAssessmentFailureSignature(aggregate.Facts.Focus.ID, runtimecontract.AssessmentDispositionRepairableGap, &gap)
				gap.FailureSignature = signature
				aggregate.Facts.Failure.Signature = signature
				fact.FailureSignature = signature
			}
			previousCount := repairFailureSignatureCount(aggregate.Facts.Repair, signature)
			cycleID := runtimecontract.StableRepairCycleID(string(aggregate.Facts.RunID), aggregate.Facts.Focus.ID, signature, evidence.EventID)
			ledger, _, cycleErr := beginRepairCycle(aggregate.Facts.Repair, aggregate.Facts.Focus.ID, aggregate.Facts.Focus.SliceID, cycleID, signature, aggregate.Facts.ConvergencePolicy)
			if cycleErr != nil {
				return EvidenceCommitResult{}, cycleErr
			}
			ledger.ActiveGap = &gap
			if isStateChangeNoOpWithoutProvenanceFailure(failure, evidence.Receipt) {
				// The receipt is durable audit evidence, but cannot become
				// state-change evidence. Persist its exact operation shape so
				// candidate admission and restart take the same no-progress path.
				ledger = recordNoProgressOperationShape(ledger, runtimecontract.RuntimeRejectedOperationShape{
					Operation:        committedOperation,
					EvidenceID:       fact.ID,
					FailureClass:     failure.Class,
					FailureCode:      failure.Code,
					FailureSignature: failure.Signature,
					FailureMessage:   failure.Message,
				})
				ledger = noteRepairNoProgress(ledger)
			} else if previousCount > 0 {
				ledger = noteRepairNoProgress(ledger)
			}
			aggregate.Facts.Repair = ledger
			if fact.Kind == runtimecontract.EvidenceKindStateChange && runtimecontract.RuntimeOperationRequiresCurrentTargetSnapshot(committedOperation) {
				aggregate.Facts = requireFreshTargetSnapshots(aggregate.Facts, committedOperation.TargetRefs)
			}
		}
		aggregate.Facts.Evidence.Rejected = append(aggregate.Facts.Evidence.Rejected, fact)
	default:
		return EvidenceCommitResult{}, fmt.Errorf("unsupported execution evidence status %q", evidence.Status)
	}
	aggregate.EvidenceSequence++
	aggregate.Facts.EvidenceRevision = nextEvidenceRevision(aggregate.EvidenceSequence)
	aggregate.Facts = aggregate.Facts.Normalized()
	aggregate.ProcessedEventIDs = appendNormalizedString(aggregate.ProcessedEventIDs, evidence.EventID)
	dispatch := *aggregate.ActiveDispatch
	dispatch.CompletedOperationIDs = appendNormalizedString(dispatch.CompletedOperationIDs, evidence.OperationID)
	prerequisitesInvalidated := false
	if evidence.Status == ExecutionEvidenceStatusAccepted && evidence.Receipt != nil {
		// A receipt can make owner facts stale for the next SM-2 authority. That
		// does not invalidate the immutable operation set already sealed by this
		// SM-3 transaction: continue until every sealed operation has committed
		// evidence, unless this receipt reports a genuine failure or stale fence.
		prerequisitesInvalidated, err = applyReceiptResourceChangeInvalidation(&aggregate, evidence.DispatchID, evidence.OperationID, *evidence.Receipt)
		if err != nil {
			return EvidenceCommitResult{}, err
		}
	}
	effectUncommittable := evidence.Status == ExecutionEvidenceStatusRejected && evidence.Failure != nil &&
		evidence.Failure.Class == runtimecontract.FailureClassEffectMismatch && evidence.Receipt != nil &&
		(evidence.Receipt.Outcome == runtimecontract.RuntimeOperationOutcomeAccepted || evidence.Receipt.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp)
	// Exhausting the bounded refresh ledger is a determined runtime failure and
	// stops the transaction. Ordinary prerequisite invalidation is not.
	refreshExhausted := prerequisitesInvalidated && aggregate.Facts.Failure != nil &&
		aggregate.Facts.Failure.Class == runtimecontract.FailureClassPrerequisiteRefreshExhausted
	dispatchInvalidated := effectUncommittable || refreshExhausted
	dispatchCompleted := dispatchInvalidated || evidence.Status == ExecutionEvidenceStatusRejected || len(dispatch.CompletedOperationIDs) == candidateOperationCount(dispatch.Candidate)
	if dispatchCompleted {
		if dispatchInvalidated {
			dispatch.State = DispatchStateInvalidated
		} else {
			dispatch.State = DispatchStateCommitted
		}
		aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
		aggregate.ActiveDispatch = nil
		// A completed transaction remains durable audit/provenance evidence, but
		// SM-2 must reacquire changed owner facts before advancing this focus.
		canAdvanceFocus := !dispatchInvalidated && len(aggregate.Facts.Prerequisites.Pending) == 0
		previousFocusID := aggregate.Facts.Focus.ID
		if canAdvanceFocus {
			aggregate.Facts = advanceSatisfiedCoverageFocus(aggregate.Facts)
			if aggregate.Facts.Focus.ID != previousFocusID {
				// Only the Orchestrator-owned evidence commit may advance SM-2.
				// When that reaches an unbound placeholder, reopen the retained
				// global-order ledger for the next focus-local proof transaction.
				if err := prepareSemanticPlanningForAdvancedFocus(&aggregate); err != nil {
					return EvidenceCommitResult{}, err
				}
			}
		}
		var previousSnapshot *runtimecontract.AuthoritySnapshot
		if canAdvanceFocus {
			previousSnapshot = &aggregate.Decision.AuthoritySnapshot
		}
		facts, nextDecision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, previousSnapshot)
		if err != nil {
			return EvidenceCommitResult{}, err
		}
		aggregate.Facts = facts
		aggregate.Decision = nextDecision
	} else {
		aggregate.ActiveDispatch = &dispatch
	}
	aggregate.StateRevision++
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return EvidenceCommitResult{}, err
	}
	return EvidenceCommitResult{Disposition: CommitDispositionCommitted, DispatchCompleted: dispatchCompleted, Aggregate: aggregate}, nil
}

// isStateChangeNoOpWithoutProvenanceFailure identifies the one bounded C8-F
// branch.  Receipt outcome is checked as well as the typed failure code so a
// provider- or adapter-supplied failure string can never enter it.
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
	targets := failure.TargetRefs
	if len(targets) == 0 {
		return false
	}
	grounded := append([]string(nil), facts.Focus.TargetRefs...)
	if facts.Resources.FailureTarget != nil {
		grounded = append(grounded, facts.Resources.FailureTarget.TargetRefs...)
	}
	// A File owner scope is a bounded feasibility inventory, not a selected
	// repair surface.  A validation command that happens to name a member of
	// that inventory cannot turn it into FailureTarget authority.  Only a
	// selected owner target was already concrete before this receipt.
	if selection := currentFocusTargetSelection(facts); selection != nil &&
		selection.Disposition == runtimecontract.RuntimeTargetSelectionSelected && len(selection.TargetRefs) > 0 {
		grounded = append(grounded, selection.TargetRefs...)
	}
	if facts.RepairDirective != nil {
		grounded = append(grounded, facts.RepairDirective.TargetRefs...)
	}
	if runtimeTargetRefsIntersect(grounded, targets) {
		return true
	}
	// A current validation receipt may introduce a new target only when the
	// capability owner explicitly observed it and the observation is bound to
	// this Focus's declared role. This is deliberately stricter than a command
	// target or a diagnostic string: no owner/revision/role binding means the
	// receipt remains ordinary failure lineage and cannot create repair authority.
	observation := failure.TargetObservation
	if observation == nil {
		return false
	}
	value := observation.Normalized()
	if err := value.Validate(); err != nil || !runtimeFailureTargetObservationOwnerAllowed(value, failure) ||
		value.TargetRole != facts.Focus.TargetRole || !sameRuntimeTargetRefs(value.TargetRefs, targets) ||
		!failureTargetObservationHasCurrentFreshness(facts, value) {
		return false
	}
	return facts.Focus.RequireTargetGrounding
}

// runtimeFailureTargetObservationOwnerAllowed is the narrow owner-protocol
// fence for target discovery. MCP stays limited to its explicit response
// field. Shell must return the owner-parsed, content-bound stdout record;
// command targets, stderr and validation diagnostics remain non-authoritative.
func runtimeFailureTargetObservationOwnerAllowed(value runtimecontract.RuntimeFailureTargetObservation, failure runtimecontract.FailureFact) bool {
	switch value.OwnerModuleID {
	case runtimecontract.RuntimeModuleMCP:
		return true
	case runtimecontract.RuntimeModuleShell:
		return value.SubjectRef == "shell_runtime_policy" && value.DiagnosticClass != "" && value.ContentHash != "" && len(value.EvidenceRefs) > 0 &&
			failure.ValidationDiagnostic != nil && value.DiagnosticClass == failure.ValidationDiagnostic.Normalized().Class
	default:
		return false
	}
}

func failureTargetObservationHasCurrentFreshness(facts runtimecontract.RuntimeFacts, target runtimecontract.RuntimeFailureTargetObservation) bool {
	target = target.Normalized()
	for _, observation := range facts.Resources.PrerequisiteObservations {
		for _, dependency := range observation.Normalized().Dependencies {
			if dependency.Domain == runtimecontract.RuntimeResourceOwnerCatalog && dependency.OwnerModuleID == target.OwnerModuleID && dependency.Revision == target.ObservedRevision {
				return true
			}
		}
	}
	return false
}

func sameRuntimeTargetRefs(left, right []string) bool {
	left = normalizeRuntimeTargetRefs(left)
	right = normalizeRuntimeTargetRefs(right)
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

func normalizeRuntimeTargetRefs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		seen[value] = struct{}{}
	}
	for value := range seen {
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
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
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		if value = strings.TrimSpace(value); value != "" {
			seen[value] = struct{}{}
		}
	}
	for _, value := range right {
		if value = strings.TrimSpace(value); value != "" {
			if _, ok := seen[value]; ok {
				return true
			}
		}
	}
	return false
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
	return PreflightRuntimeFacts(facts, requirements)
}

// coverageEvidenceCreditForReceipt is the SM-2 proof bridge. The operation's
// BindingRefs were sealed by SM-3, but only this reducer-owned matcher can
// derive CWI IDs after comparing those refs with owner receipt facts.
func coverageEvidenceCreditForReceipt(facts runtimecontract.RuntimeFacts, operation runtimecontract.RuntimeOperation, evidenceKind string, receipt *runtimecontract.RuntimeOperationReceipt) ([]string, []common.CoverageWorkItemID, string) {
	operation = operation.Normalized()
	bindings := facts.Focus.Normalized().CoverageEvidenceBindings
	if len(bindings) == 0 {
		// A CWI can only receive credit through a binding compiled at activation
		// and sealed by SM-3. In particular, do not fall back to the operation's
		// deprecated raw CoverageWorkItemIDs: that would let an unsealed
		// candidate-shaped value drive an SM-2 coverage transition.
		return append([]string(nil), operation.BindingRefs...), nil, "coverage_binding_mismatch: focus has no sealed coverage evidence bindings"
	}
	refs := append([]string(nil), operation.BindingRefs...)
	// Candidate admission rejects this shape for current dispatches. Retain the
	// same fence here because an aggregate may be restored after a crash or be
	// supplied by a corrupted/stale producer: one owner receipt has one exact
	// CWI credit path and must never fan out across bindings.
	if len(refs) > 1 {
		return refs, nil, "coverage_binding_mismatch: an operation may select exactly one coverage evidence binding"
	}
	if receipt == nil {
		return refs, nil, "coverage_binding_mismatch: missing persisted owner receipt"
	}
	credit := make([]common.CoverageWorkItemID, 0, len(refs))
	diagnostics := make([]string, 0)
	for _, ref := range refs {
		binding, found := runtimecontract.CoverageEvidenceBindingForRef(bindings, ref)
		if !found {
			diagnostics = append(diagnostics, "unknown binding "+ref)
			continue
		}
		if _, requiresKind := runtimecontract.CoverageEvidenceProofForKind(binding, evidenceKind); !requiresKind {
			diagnostics = append(diagnostics, "binding "+ref+" does not require evidence kind "+evidenceKind)
			continue
		}
		if err := runtimecontract.MatchCoverageEvidenceBindingReceipt(binding, evidenceKind, operation, *receipt); err != nil {
			diagnostics = append(diagnostics, err.Error())
			continue
		}
		credit = append(credit, binding.CoverageWorkItemID)
	}
	credit = normalizeCoverageIDs(credit)
	if len(diagnostics) != 0 {
		sort.Strings(diagnostics)
		return refs, credit, "coverage_binding_mismatch: " + strings.Join(diagnostics, " | ")
	}
	return refs, credit, ""
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

// reopenCoverageDependenciesForEvidence applies compiled profile dependency
// edges to the coverage ledger.  The edges and trigger kinds were sealed at
// activation, so this reducer helper remains neutral to target paths, owner
// names, languages and command semantics.  It preserves all accepted evidence
// references and records why the dependent was reopened for restart/audit.
func reopenCoverageDependenciesForEvidence(coverage []runtimecontract.RuntimeCoverageItemFact, evidence runtimecontract.EvidenceFact) ([]runtimecontract.RuntimeCoverageItemFact, []common.CoverageWorkItemID) {
	sourceIDs := map[common.CoverageWorkItemID]struct{}{}
	for _, id := range evidence.CoverageWorkItemIDs {
		if id != "" {
			sourceIDs[id] = struct{}{}
		}
	}
	if len(sourceIDs) == 0 || strings.TrimSpace(evidence.Kind) == "" {
		return runtimecontract.NormalizeRuntimeCoverageFacts(coverage), nil
	}
	reopen := map[common.CoverageWorkItemID]struct{}{}
	for _, source := range coverage {
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
		return runtimecontract.NormalizeRuntimeCoverageFacts(coverage), nil
	}
	reopened := make([]common.CoverageWorkItemID, 0, len(reopen))
	for idx := range coverage {
		item := &coverage[idx]
		if _, matches := reopen[item.ID]; !matches || item.State == runtimecontract.RuntimeCoverageItemStateOpen {
			continue
		}
		item.State = runtimecontract.RuntimeCoverageItemStateOpen
		item.LastAssessmentID = ""
		item.ReopenedByEvidenceRefs = append(item.ReopenedByEvidenceRefs, string(evidence.ID))
		reopened = append(reopened, item.ID)
	}
	coverage = runtimecontract.NormalizeRuntimeCoverageFacts(coverage)
	if len(reopened) == 0 {
		return coverage, nil
	}
	// Normalization orders ledger facts, not this return value.  Preserve a
	// deterministic coverage-ID result for diagnostics and focused tests.
	sort.Slice(reopened, func(i, j int) bool { return string(reopened[i]) < string(reopened[j]) })
	return coverage, reopened
}

// requeueReopenedCoverageFocuses restores completed focus contracts before the
// still-pending queue when a dependency reopened one of their coverage items.
// It uses only persisted focus/coverage identities and therefore introduces no
// action choice or domain interpretation into the reducer.
func requeueReopenedCoverageFocuses(facts runtimecontract.RuntimeFacts, reopened []common.CoverageWorkItemID) runtimecontract.RuntimeFacts {
	if len(reopened) == 0 || len(facts.CompletedFocuses) == 0 {
		return facts
	}
	needed := map[common.CoverageWorkItemID]struct{}{}
	for _, id := range reopened {
		if id != "" {
			needed[id] = struct{}{}
		}
	}
	if len(needed) == 0 {
		return facts
	}
	requeued := make([]runtimecontract.CoverageFocus, 0, len(facts.CompletedFocuses))
	completed := make([]runtimecontract.CoverageFocus, 0, len(facts.CompletedFocuses))
	for _, focus := range facts.CompletedFocuses {
		match := false
		for _, id := range focus.CoverageWorkItemIDs {
			if _, ok := needed[id]; ok {
				match = true
				break
			}
		}
		if match {
			requeued = append(requeued, focus)
			continue
		}
		completed = append(completed, focus)
	}
	if len(requeued) == 0 {
		return facts
	}
	facts.CompletedFocuses = completed
	facts.RemainingFocuses = append(requeued, facts.RemainingFocuses...)
	return facts.Normalized()
}

func (a RuntimeAggregate) Validate() error {
	if a.Facts.WorkspaceID == "" || a.Facts.RunID == "" {
		return fmt.Errorf("runtime aggregate requires workspace and run scope")
	}
	if err := a.Facts.Validate(); err != nil {
		return err
	}
	if err := a.SemanticPlanning.Validate(); err != nil {
		return fmt.Errorf("runtime aggregate semantic planning ledger: %w", err)
	}
	if a.SemanticPlanning.DefinitionSeed.Hash != a.Facts.DefinitionSeed.Hash {
		return fmt.Errorf("runtime aggregate semantic planning seed does not match facts")
	}
	if err := a.Reconciliation.Validate(); err != nil {
		return fmt.Errorf("runtime aggregate reconciliation ledger: %w", err)
	}
	if err := a.Definition.Validate(); err != nil {
		return err
	}
	if a.Definition.Normalized().Revision != a.Facts.Normalized().DefinitionRevision {
		return fmt.Errorf("runtime aggregate definition revision mismatch")
	}
	if err := a.Decision.Validate(); err != nil {
		return err
	}
	if a.Decision.AuthoritySnapshot.WorkspaceID != a.Facts.WorkspaceID || a.Decision.AuthoritySnapshot.RunID != a.Facts.RunID {
		return fmt.Errorf("runtime aggregate decision scope mismatch")
	}
	if a.ActiveDispatch != nil {
		if err := a.ActiveDispatch.ValidateWithActivation(a.Decision, a.Facts.Activation); err != nil {
			return fmt.Errorf("runtime aggregate active dispatch: %w", err)
		}
	}
	if a.Termination != nil {
		if err := a.Termination.Validate(); err != nil {
			return fmt.Errorf("runtime aggregate termination: %w", err)
		}
	}
	if err := runtimecontract.ValidateRuntimeCandidateAttemptRecordsForScope(a.Facts.WorkspaceID, a.Facts.RunID, a.CandidateAttempts); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeCandidateRequestSessions(a.CandidateRequestSessions); err != nil {
		return err
	}
	for _, attempt := range a.CandidateAttempts {
		matched := false
		for _, session := range a.CandidateRequestSessions {
			if session.LogicalRequestID != attempt.LogicalRequestID {
				continue
			}
			if session.Transaction != attempt.Transaction || session.StablePrefixHash != attempt.StablePrefixHash || session.ManifestHash != attempt.ManifestHash || session.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
				return fmt.Errorf("runtime candidate attempt request-session binding does not match its complete document")
			}
			matched = true
			break
		}
		if !matched {
			return fmt.Errorf("runtime candidate attempt has no complete request-session binding")
		}
	}
	if (isTerminalConvergenceState(a.Decision.State) || a.Decision.Terminal != nil) && hasPendingCandidateAttempt(a.CandidateAttempts) {
		return fmt.Errorf("terminal runtime aggregate cannot retain a pending candidate attempt")
	}
	if hasUnreducedCurrentCandidateRetryFailure(a.Decision, a.CandidateAttempts) {
		return fmt.Errorf("current runtime authority retains a failed candidate attempt 1 without reducer transition")
	}
	for _, attempt := range a.CandidateAttempts {
		if attempt.State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
			continue
		}
		matched := a.ActiveDispatch != nil && a.ActiveDispatch.Transaction == attempt.Transaction
		if !matched {
			for _, dispatch := range a.CompletedDispatches {
				if dispatch.Transaction == attempt.Transaction {
					matched = true
					break
				}
			}
		}
		if !matched {
			return fmt.Errorf("accepted candidate attempt has no sealed dispatch")
		}
	}
	if err := runtimecontract.ValidateRuntimeGovernanceResolutionRecords(a.GovernanceResolutions); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeGovernanceRequestRecords(a.GovernanceRequests); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeOperationGovernanceBindings(a.OperationGovernanceBindings); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeOperationReceipts(a.OperationReceipts); err != nil {
		return err
	}
	for _, acknowledgement := range a.DeliveryAcknowledgements {
		if err := acknowledgement.Validate(); err != nil {
			return err
		}
		if acknowledgement.WorkspaceID != a.Facts.WorkspaceID || acknowledgement.RunID != a.Facts.RunID {
			return fmt.Errorf("delivery acknowledgement scope mismatch")
		}
	}
	for _, invalidation := range a.PrerequisiteInvalidations {
		if err := invalidation.Validate(); err != nil {
			return fmt.Errorf("runtime aggregate prerequisite invalidation: %w", err)
		}
	}
	if err := runtimecontract.ValidateRuntimeRepairSurfaceConfirmationRecords(a.RepairSurfaceConfirmations); err != nil {
		return fmt.Errorf("runtime aggregate repair surface confirmations: %w", err)
	}
	for _, confirmation := range a.RepairSurfaceConfirmations {
		if confirmation.Confirmation.Observation.WorkspaceID != a.Facts.WorkspaceID || confirmation.Confirmation.Observation.RunID != a.Facts.RunID {
			return fmt.Errorf("runtime aggregate repair surface confirmation scope mismatch")
		}
	}
	if a.DriveLease != nil {
		if err := a.DriveLease.Validate(); err != nil {
			return fmt.Errorf("runtime aggregate drive lease: %w", err)
		}
		if a.DriveLease.WorkspaceID != a.Facts.WorkspaceID || a.DriveLease.RunID != a.Facts.RunID {
			return fmt.Errorf("runtime aggregate drive lease scope mismatch")
		}
	}
	for _, request := range a.GovernanceRequests {
		matched := a.ActiveDispatch != nil && a.ActiveDispatch.ID == request.DispatchID &&
			a.ActiveDispatch.Transaction.AuthoritySnapshotID == request.AuthoritySnapshotID && a.ActiveDispatch.Candidate.Governance != nil
		if !matched {
			for _, dispatch := range a.CompletedDispatches {
				if dispatch.ID == request.DispatchID && dispatch.Transaction.AuthoritySnapshotID == request.AuthoritySnapshotID && dispatch.Candidate.Governance != nil {
					matched = true
					break
				}
			}
		}
		if !matched {
			return fmt.Errorf("runtime aggregate governance request has no governed dispatch")
		}
	}
	for _, rawBinding := range a.OperationGovernanceBindings {
		binding := rawBinding.Normalized()
		var source *SealedDispatch
		for idx := range a.CompletedDispatches {
			if a.CompletedDispatches[idx].ID == binding.OperationDispatchID {
				value := a.CompletedDispatches[idx]
				source = &value
				break
			}
		}
		if source == nil {
			return fmt.Errorf("operation governance binding source dispatch %q is not completed", binding.OperationDispatchID)
		}
		execution, found := operationExecutionRecord(source.OperationExecutions, binding.OperationID)
		if !found || execution.ExecutionKey != binding.ExecutionKey || execution.Fence != binding.Fence ||
			execution.OwnerReceiptHash != binding.GovernanceReceiptHash || execution.State != runtimecontract.OperationExecutionOwnerCommitted {
			return fmt.Errorf("operation governance binding source execution does not match sealed receipt")
		}
		receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(a.OperationReceipts, binding.ExecutionKey, binding.Fence)
		if !found || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeGovernanceRequired || runtimecontract.StableRuntimeOperationReceiptHash(receipt) != binding.GovernanceReceiptHash {
			return fmt.Errorf("operation governance binding has no matching governance receipt")
		}
		if binding.State == runtimecontract.RuntimeOperationGovernanceBindingSuspended {
			continue
		}
		bound := false
		for _, request := range a.GovernanceRequests {
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
	for _, resolution := range a.GovernanceResolutions {
		matched := false
		bound := false
		for _, request := range a.GovernanceRequests {
			if request.TicketID == resolution.TicketID && request.DispatchID == resolution.DispatchID && request.AuthoritySnapshotID == resolution.AuthoritySnapshotID {
				bound = true
				break
			}
		}
		for _, dispatch := range a.CompletedDispatches {
			if dispatch.ID == resolution.DispatchID && dispatch.Transaction.AuthoritySnapshotID == resolution.AuthoritySnapshotID &&
				dispatch.State == DispatchStateCommitted && dispatch.Candidate.Governance != nil {
				matched = true
				break
			}
		}
		if !matched || !bound {
			return fmt.Errorf("runtime aggregate governance resolution has no completed governed dispatch")
		}
	}
	return nil
}

func hasPendingCandidateAttempt(values []runtimecontract.RuntimeCandidateAttemptRecord) bool {
	for _, value := range values {
		if value.State == runtimecontract.RuntimeCandidateAttemptStatePending {
			return true
		}
	}
	return false
}

func hasUnreducedCurrentCandidateRetryFailure(decision runtimecontract.RuntimeDecision, values []runtimecontract.RuntimeCandidateAttemptRecord) bool {
	current := decision.AuthorityTransaction()
	for _, value := range values {
		if value.Attempt == 1 && value.State == runtimecontract.RuntimeCandidateAttemptStateFailed && runtimecontract.SameAuthorityContract(value.Transaction, current) {
			return true
		}
	}
	return false
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
	return fmt.Sprintf("evidence-v1-%020d", sequence)
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
