package runtime

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *candidateDispatchComponent) invalidateCandidateAttemptForPrerequisiteStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stale runtimecontract.RuntimePrerequisiteStale) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	stale = stale.Normalized()
	if err := stale.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	signature := runtimecontract.StableRuntimePrerequisiteStaleSignature(stale)
	if prerequisiteInvalidationExists(aggregate, signature) {
		return aggregate, nil
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, _, found := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !found {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	expectedRevision := aggregate.StateRevision
	facts, affectedPrerequisites, affectedObservations, invalidatedDependencies, err := invalidatePrerequisiteObservationsForStale(aggregate.Facts, stale)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	ledger, consumed, err := consumeRecoveryBudget(facts.Recovery, runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, facts.RecoveryPolicy)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	facts.Recovery = ledger
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateInvalidated
	aggregate.CandidateAttempts[idx].Proposal = nil
	aggregate.CandidateAttempts[idx].Feasibilities = nil
	aggregate.CandidateAttempts[idx].Failure = nil
	staleCopy := stale
	aggregate.CandidateAttempts[idx].Stale = &staleCopy
	aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, runtimecontract.RuntimePrerequisiteInvalidationRecord{
		Signature: signature, PrerequisiteIDs: affectedPrerequisites, ObservationIDs: affectedObservations,
		InvalidatedDependencies: invalidatedDependencies, Stale: &staleCopy,
	})
	if !consumed {
		facts.Failure = &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted, Code: "prerequisite_refresh_exhausted",
			Signature: signature, Message: "prerequisite refresh budget exhausted: " + stale.Reason,
		}
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// invalidateCandidateAttemptForFeasibilityPrerequisite abandons an unsealed
// candidate when its owning capability proves that correcting the operation
// requires a missing current owner fact. Unlike a failed candidate attempt it
// spends neither the 0/1 contract retry nor the focus repair budget: the next
// candidate runs only after a new authority is sealed around the acquired fact.

func (c *candidateDispatchComponent) invalidateCandidateAttemptForFeasibilityPrerequisite(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, request runtimecontract.RuntimeOperationPrerequisiteRequest) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	request = request.Normalized()
	if err := request.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	// Duplicate owner delivery may arrive after the CAS committed a new
	// prerequisite authority. The persisted invalidated attempt is the exact
	// idempotency fence; do not reconstruct or consume any budget.
	for _, record := range aggregate.CandidateAttempts {
		if record.Attempt == attempt && record.State == runtimecontract.RuntimeCandidateAttemptStateInvalidated && record.PrerequisiteRequest != nil && record.PrerequisiteRequest.Same(request) {
			return aggregate, nil
		}
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, record, found := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !found || record.Proposal == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	operation, found := candidateOperationByID(*record.Proposal, request.OperationID)
	if !found || !runtimecontract.RuntimeOperationRequiresCurrentTargetSnapshot(operation) || !runtimeOperationTargetsRef(operation, request.SubjectRef) {
		return RuntimeAggregate{}, fmt.Errorf("%w: prerequisite request is not bound to the pending candidate operation", common.ErrConflict)
	}
	descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !known || descriptor.OwnerModuleID != request.OwnerModuleID {
		return RuntimeAggregate{}, fmt.Errorf("%w: prerequisite request owner does not match candidate operation", common.ErrConflict)
	}
	feasibility, found := candidateAttemptFeasibilityForOperation(record.Feasibilities, request.OperationID)
	if !found || feasibility.Observation == nil || feasibility.Observation.PrerequisiteRequest == nil || !feasibility.Observation.PrerequisiteRequest.Same(request) {
		return RuntimeAggregate{}, fmt.Errorf("%w: prerequisite request has no matching persisted owner feasibility fact", common.ErrConflict)
	}

	expectedRevision := aggregate.StateRevision
	facts := addFeasibilityPrerequisite(factsWithoutTargetSnapshot(aggregate.Facts, request), request)
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateInvalidated
	aggregate.CandidateAttempts[idx].Proposal = nil
	aggregate.CandidateAttempts[idx].Feasibilities = nil
	aggregate.CandidateAttempts[idx].Failure = nil
	aggregate.CandidateAttempts[idx].Stale = nil
	requestCopy := request
	aggregate.CandidateAttempts[idx].PrerequisiteRequest = &requestCopy
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// invalidateCandidateAttemptForOwnerInfeasibility is the SM-2/SM-3 handoff
// for a completed owner feasibility probe. Unlike failCandidateAttempt it
// never seals a CorrectionDisposition and never permits attempt 1. The
// controller preserves the factual observation as immutable lineage, then lets
// the owner reducer choose a fresh authority or a typed terminal. An
// owner-contract-defect is terminal at this boundary: it is evidence that the
// producing capability violated its own total emitted-code registry, not a
// recoverable resource observation.

func (c *candidateDispatchComponent) invalidateCandidateAttemptForOwnerInfeasibility(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, observation runtimecontract.RuntimeOwnerFeasibilityObservation) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	observation = observation.Normalized()
	if err := observation.Validate(); err != nil || (observation.Outcome != runtimecontract.RuntimeOwnerFeasibilityInfeasible && observation.Outcome != runtimecontract.RuntimeOwnerFeasibilityContractDefect) {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, record, found := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !found {
		for _, historical := range aggregate.CandidateAttempts {
			if historical.Attempt == attempt && historical.State == runtimecontract.RuntimeCandidateAttemptStateInvalidated && historical.OwnerFeasibility != nil &&
				(reflect.DeepEqual(historical.OwnerFeasibility.Clone(), observation) || historical.BindingContradiction != nil) {
				return aggregate, nil
			}
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	if record.Proposal == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	matched := false
	var matchedFeasibility runtimecontract.RuntimeCandidateFeasibilityRecord
	for _, feasibility := range record.Feasibilities {
		if feasibility.Observation == nil {
			continue
		}
		if reflect.DeepEqual(feasibility.Observation.Clone(), observation) {
			matched = true
			matchedFeasibility = feasibility
			break
		}
	}
	if !matched {
		return RuntimeAggregate{}, fmt.Errorf("%w: owner infeasibility has no matching persisted feasibility observation", common.ErrConflict)
	}

	expectedRevision := aggregate.StateRevision
	var contradiction *runtimecontract.RuntimeBindingContradictionIdentity
	var contradictionCoverageID common.CoverageWorkItemID
	var contradictionBinding runtimecontract.RuntimeCWIBindingRef
	contradictionUnbound := false
	if observation.SealedTargetAuthorizationContradicted() {
		identity, coverageID, bindingRef, contradictionErr := runtimeBindingContradictionIdentity(aggregate, record, matchedFeasibility, observation)
		if contradictionErr != nil {
			contradictionUnbound = true
		} else {
			contradiction = &identity
			contradictionCoverageID = coverageID
			contradictionBinding = bindingRef
		}
	}
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateInvalidated
	aggregate.CandidateAttempts[idx].Proposal = nil
	aggregate.CandidateAttempts[idx].Feasibilities = nil
	aggregate.CandidateAttempts[idx].Failure = nil
	aggregate.CandidateAttempts[idx].Stale = nil
	aggregate.CandidateAttempts[idx].PrerequisiteRequest = nil
	copyObservation := observation.Clone()
	aggregate.CandidateAttempts[idx].OwnerFeasibility = &copyObservation
	aggregate.CandidateAttempts[idx].BindingContradiction = contradiction
	facts := aggregate.Facts
	if contradiction != nil {
		for itemIndex := range facts.Coverage {
			item := &facts.Coverage[itemIndex]
			if item.ID != contradictionCoverageID || item.CurrentBindingRef == nil || !runtimecontract.SameRuntimeCWIBindingRef(*item.CurrentBindingRef, contradictionBinding) {
				continue
			}
			if len(item.BindingRetirementLineage) >= runtimecontract.RuntimeCWIBindingRetirementLineageLimit {
				return RuntimeAggregate{}, fmt.Errorf("runtime CWI binding retirement lineage exhausted")
			}
			item.BindingRetirementLineage = append(item.BindingRetirementLineage, runtimecontract.RuntimeCWIBindingRetirementLineage{
				RetiredBindingRef: contradictionBinding, CausationRef: contradiction.ObservationHash,
				Reason: "capability owner contradicted the sealed binding before dispatch",
			})
			item.CurrentBindingRef = nil
			item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStatePlanning
		}
		updated, updateErr := replaceRuntimeCWIFocusProjection(facts, []common.CoverageWorkItemID{contradictionCoverageID})
		if updateErr != nil {
			return RuntimeAggregate{}, updateErr
		}
		facts = updated
		contract, contractErr := canonicalRuntimeCWIReconciliationContract(facts)
		if contractErr != nil {
			return RuntimeAggregate{}, contractErr
		}
		facts.ReconciliationContract = contract
		aggregate.Facts = facts
		if _, supersedeErr := supersedeActiveReconciliationForCurrentContract(&aggregate); supersedeErr != nil {
			return RuntimeAggregate{}, supersedeErr
		}
		if retireErr := retireSemanticPlanningGenerationForBindingContradiction(&aggregate, contradictionCoverageID, contradiction.ObservationHash); retireErr != nil {
			return RuntimeAggregate{}, retireErr
		}
		facts = aggregate.Facts
		facts.Failure = nil
		facts, decision, reduceErr := ReduceRuntimeStateWithDirective(facts, nil, aggregate.CoverageProofLineage)
		if reduceErr != nil {
			return RuntimeAggregate{}, reduceErr
		}
		aggregate.Facts = facts
		aggregate.Decision = decision
		aggregate.StateRevision++
		if err := aggregate.Validate(); err != nil {
			return RuntimeAggregate{}, err
		}
		if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
			return RuntimeAggregate{}, err
		}
		return aggregate, nil
	}
	diagnostic := observation.Feedback.OriginalDiagnostic
	failure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassCandidateInfeasible, Code: diagnostic.OriginalCode,
		Signature: diagnostic.Signature, Message: diagnostic.OriginalMessage, TargetRefs: append([]string(nil), diagnostic.TargetRefs...),
	}
	if observation.Outcome == runtimecontract.RuntimeOwnerFeasibilityContractDefect {
		failure = runtimecontract.FailureFact{
			Class:      runtimecontract.FailureClassRuntimeContractMisalignment,
			Code:       "runtime_owner_contract_defect",
			Signature:  "runtime-owner-contract-defect/" + diagnostic.Signature,
			Message:    "capability owner emitted an undeclared feasibility code",
			TargetRefs: append([]string(nil), diagnostic.TargetRefs...),
		}
	}
	if contradictionUnbound {
		failure = runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_contract_defect",
			Signature:  "binding-contradiction-unbound/" + diagnostic.Signature,
			Message:    "owner contradiction cannot be tied to one current binding",
			TargetRefs: append([]string(nil), diagnostic.TargetRefs...),
		}
	}
	// An owner that reports that the sealed validation recipe is unavailable
	// cannot be repaired by asking for another candidate under the same
	// validation authority. A valid alternative must arrive as a new sealed
	// prerequisite/blueprint fact; absent that fact the factual outcome is a
	// deterministic terminal, not a correction or random state change.
	if observation.FailureCategory() == runtimecontract.RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable {
		// The owner rejected the sealed recipe before any repair attempt or
		// dispatch. This is an owner/capability contract gap, not exhausted
		// repair progress; preserving that distinction prevents a zero-attempt
		// prerequisite failure from being reported as no-progress exhaustion.
		failure.Class = runtimecontract.FailureClassCandidateInfeasible
		failure.Code = "validation_recipe_exhausted"
		failure.Signature = "validation-recipe-exhausted/" + diagnostic.Signature
		failure.Message = "sealed validation recipe is unavailable: " + diagnostic.OriginalMessage
	}
	facts.Failure = &failure
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func (c *candidateDispatchComponent) invalidateActiveDispatchForPrerequisiteStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, receipt runtimecontract.RuntimeOperationReceipt) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(dispatchID) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	receipt = receipt.Normalized()
	if err := receipt.Validate(); err != nil || receipt.PrerequisiteStale == nil {
		if err != nil {
			return RuntimeAggregate{}, err
		}
		return RuntimeAggregate{}, fmt.Errorf("active dispatch refresh requires a generic stale receipt")
	}
	stale := receipt.PrerequisiteStale.Normalized()
	signature := runtimecontract.StableRuntimePrerequisiteStaleSignature(stale)
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if prerequisiteInvalidationExists(aggregate, signature) {
		return aggregate, nil
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.ID != strings.TrimSpace(dispatchID) {
		return RuntimeAggregate{}, fmt.Errorf("%w: stale receipt does not match active dispatch", common.ErrConflict)
	}
	if _, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, receipt.ExecutionKey, receipt.Fence); !found {
		return RuntimeAggregate{}, fmt.Errorf("%w: stale receipt is not owner-persisted", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	facts, prerequisiteIDs, observationIDs, invalidatedDependencies, err := invalidatePrerequisiteObservationsForStale(aggregate.Facts, stale)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	ledger, consumed, err := consumeRecoveryBudget(facts.Recovery, runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, facts.RecoveryPolicy)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	facts.Recovery = ledger
	aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, runtimecontract.RuntimePrerequisiteInvalidationRecord{
		Signature: signature, PrerequisiteIDs: prerequisiteIDs, ObservationIDs: observationIDs,
		InvalidatedDependencies: invalidatedDependencies,
		DispatchID:              dispatchID, OperationID: receipt.OperationID, Stale: &stale,
	})
	dispatch := *aggregate.ActiveDispatch
	dispatch.State = DispatchStateInvalidated
	aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
	aggregate.ActiveDispatch = nil
	if !consumed {
		facts.Failure = &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted, Code: "prerequisite_refresh_exhausted",
			Signature: signature, Message: "prerequisite refresh budget exhausted: " + stale.Reason,
		}
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// applyReceiptResourceChangeInvalidation is called while committing accepted
// evidence. It keeps the accepted receipt/evidence and atomically retires any
// intersecting owner observation for the next SM-2 authority. It never makes
// the remaining operations of the current sealed SM-3 transaction stale:
// their identity, targets, dependencies and effect contract were sealed before
// the first owner side effect.
