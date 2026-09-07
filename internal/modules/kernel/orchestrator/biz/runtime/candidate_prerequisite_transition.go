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
	if err := applyCandidatePrerequisiteStale(&aggregate, idx, stale); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// applyCandidatePrerequisiteStale is the mutation half shared by the legacy
// focused transition tests and CandidateFeasibilityObserved. Its caller owns
// the single aggregate CAS.
func applyCandidatePrerequisiteStale(aggregate *RuntimeAggregate, idx int, stale runtimecontract.RuntimePrerequisiteStale) error {
	if aggregate == nil || idx < 0 || idx >= len(aggregate.CandidateAttempts) {
		return common.ErrInvalidArgument
	}
	signature := runtimecontract.StableRuntimePrerequisiteStaleSignature(stale)
	facts, affectedPrerequisites, affectedObservations, invalidatedDependencies, err := invalidatePrerequisiteObservationsForStale(aggregate.Facts, stale)
	if err != nil {
		return err
	}
	ledger, consumed, err := consumeRecoveryBudget(facts.Recovery, runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, facts.RecoveryPolicy)
	if err != nil {
		return err
	}
	facts.Recovery = ledger
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateInvalidated
	aggregate.CandidateAttempts[idx].Proposal = nil
	aggregate.CandidateAttempts[idx].Conformances = nil
	aggregate.CandidateAttempts[idx].Feasibilities = nil
	aggregate.CandidateAttempts[idx].Failure = nil
	staleCopy := stale
	aggregate.CandidateAttempts[idx].Stale = &staleCopy
	aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, runtimecontract.RuntimePrerequisiteInvalidationRecord{
		Signature: signature, PrerequisiteIDs: affectedPrerequisites, ObservationIDs: affectedObservations,
		InvalidatedDependencies: invalidatedDependencies, Stale: &staleCopy,
	})
	if !consumed {
		failure := runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted, Code: "prerequisite_refresh_exhausted",
			Signature: signature, Message: "prerequisite refresh budget exhausted: " + stale.Reason,
		}
		if err := replaceRuntimeFailureAuthority(&facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
			return err
		}
	}
	facts, decision, err := reduceRuntimeAggregateWithDirective(*aggregate, facts, nil)
	if err != nil {
		return err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	return nil
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
	if err := applyCandidateFeasibilityPrerequisite(&aggregate, idx, request); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func applyCandidateFeasibilityPrerequisite(aggregate *RuntimeAggregate, idx int, request runtimecontract.RuntimeOperationPrerequisiteRequest) error {
	if aggregate == nil || idx < 0 || idx >= len(aggregate.CandidateAttempts) {
		return common.ErrInvalidArgument
	}
	facts := addFeasibilityPrerequisite(factsWithoutTargetSnapshot(aggregate.Facts, request), request)
	aggregate.CandidateAttempts[idx].State = runtimecontract.RuntimeCandidateAttemptStateInvalidated
	aggregate.CandidateAttempts[idx].Proposal = nil
	aggregate.CandidateAttempts[idx].Conformances = nil
	aggregate.CandidateAttempts[idx].Feasibilities = nil
	aggregate.CandidateAttempts[idx].Failure = nil
	aggregate.CandidateAttempts[idx].Stale = nil
	requestCopy := request
	aggregate.CandidateAttempts[idx].PrerequisiteRequest = &requestCopy
	facts, decision, err := reduceRuntimeAggregateWithDirective(*aggregate, facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	return nil
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
	observation, err := normalizeOwnerInfeasibilityInput(c, workspaceID, runID, attempt, observation)
	if err != nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	contractID := runtimecontract.StableAuthorityContractID(workspaceID, runID, aggregate.Decision.AuthorityTransaction())
	idx, record, found := pendingCandidateAttempt(aggregate.CandidateAttempts, workspaceID, runID, contractID, attempt)
	if !found {
		if duplicateOwnerInfeasibilityInvalidation(aggregate.CandidateAttempts, attempt, observation) {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	if record.Proposal == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: candidate attempt %d is not pending", common.ErrConflict, attempt)
	}
	matchedFeasibility, matched := matchingOwnerInfeasibility(record.Feasibilities, observation)
	if !matched {
		return RuntimeAggregate{}, fmt.Errorf("%w: owner infeasibility has no matching persisted feasibility observation", common.ErrConflict)
	}

	expectedRevision := aggregate.StateRevision
	if err := applyCandidateOwnerInfeasibility(&aggregate, idx, record, matchedFeasibility, observation); err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func applyCandidateOwnerInfeasibility(aggregate *RuntimeAggregate, idx int, record runtimecontract.RuntimeCandidateAttemptRecord, matchedFeasibility runtimecontract.RuntimeCandidateFeasibilityRecord, observation runtimecontract.RuntimeOwnerFeasibilityObservation) error {
	if aggregate == nil || idx < 0 || idx >= len(aggregate.CandidateAttempts) {
		return common.ErrInvalidArgument
	}
	contradiction := resolveOwnerBindingContradiction(*aggregate, record, matchedFeasibility, observation)
	repairInfeasibility, repairBound, repairErr := resolveOwnerValidationRepairInfeasibility(*aggregate, record, matchedFeasibility, observation)
	if repairErr != nil {
		return repairErr
	}
	invalidateCandidateAttemptForOwnerObservation(aggregate, idx, observation, contradiction.identity)
	var err error
	if contradiction.identity != nil {
		err = applyOwnerBindingContradiction(aggregate, contradiction)
	} else if repairBound {
		err = applyOwnerValidationRepairInfeasibility(aggregate, observation, repairInfeasibility)
	} else {
		err = applyOwnerInfeasibilityFailure(aggregate, observation, contradiction.unbound)
	}
	return err
}

type ownerBindingContradiction struct {
	identity   *runtimecontract.RuntimeBindingContradictionIdentity
	coverageID common.CoverageWorkItemID
	binding    runtimecontract.RuntimeCWIBindingRef
	unbound    bool
}

// ownerValidationRepairInfeasibility scopes an ordinary owner rejection to
// the exact intermediate validation-repair binding.  It is Orchestrator-local
// transition input, not a capability instruction or a second owner fact.
type ownerValidationRepairInfeasibility struct {
	coverageID     common.CoverageWorkItemID
	binding        runtimecontract.RuntimeCWIBindingRef
	causationRef   string
	contractDefect bool
}

func normalizeOwnerInfeasibilityInput(c *candidateDispatchComponent, workspaceID common.WorkspaceID, runID common.RunID, attempt int, observation runtimecontract.RuntimeOwnerFeasibilityObservation) (runtimecontract.RuntimeOwnerFeasibilityObservation, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || (attempt != 0 && attempt != 1) {
		return runtimecontract.RuntimeOwnerFeasibilityObservation{}, common.ErrInvalidArgument
	}
	observation = observation.Normalized()
	if err := observation.Validate(); err != nil || (observation.Outcome != runtimecontract.RuntimeOwnerFeasibilityInfeasible && observation.Outcome != runtimecontract.RuntimeOwnerFeasibilityContractDefect) {
		return runtimecontract.RuntimeOwnerFeasibilityObservation{}, common.ErrInvalidArgument
	}
	return observation, nil
}

func duplicateOwnerInfeasibilityInvalidation(attempts []runtimecontract.RuntimeCandidateAttemptRecord, attempt int, observation runtimecontract.RuntimeOwnerFeasibilityObservation) bool {
	for _, historical := range attempts {
		if historical.Attempt == attempt && historical.State == runtimecontract.RuntimeCandidateAttemptStateInvalidated && historical.OwnerFeasibility != nil &&
			(reflect.DeepEqual(historical.OwnerFeasibility.Clone(), observation) || historical.BindingContradiction != nil) {
			return true
		}
	}
	return false
}

func matchingOwnerInfeasibility(feasibilities []runtimecontract.RuntimeCandidateFeasibilityRecord, observation runtimecontract.RuntimeOwnerFeasibilityObservation) (runtimecontract.RuntimeCandidateFeasibilityRecord, bool) {
	for _, feasibility := range feasibilities {
		if feasibility.Observation != nil && reflect.DeepEqual(feasibility.Observation.Clone(), observation) {
			return feasibility, true
		}
	}
	return runtimecontract.RuntimeCandidateFeasibilityRecord{}, false
}

func resolveOwnerBindingContradiction(aggregate RuntimeAggregate, record runtimecontract.RuntimeCandidateAttemptRecord, feasibility runtimecontract.RuntimeCandidateFeasibilityRecord, observation runtimecontract.RuntimeOwnerFeasibilityObservation) ownerBindingContradiction {
	if !observation.SealedTargetAuthorizationContradicted() {
		return ownerBindingContradiction{}
	}
	identity, coverageID, binding, err := runtimeBindingContradictionIdentity(aggregate, record, feasibility, observation)
	if err != nil {
		return ownerBindingContradiction{unbound: true}
	}
	return ownerBindingContradiction{identity: &identity, coverageID: coverageID, binding: binding}
}

// resolveOwnerValidationRepairInfeasibility prevents the generic candidate
// infeasibility path from tearing down an exact validation-repair directive
// while leaving its intermediate semantic binding current.  The capability
// still reports only feasibility; Orchestrator classifies that fact against
// its own current SM-2 purpose lineage.
func resolveOwnerValidationRepairInfeasibility(aggregate RuntimeAggregate, record runtimecontract.RuntimeCandidateAttemptRecord, feasibility runtimecontract.RuntimeCandidateFeasibilityRecord, observation runtimecontract.RuntimeOwnerFeasibilityObservation) (ownerValidationRepairInfeasibility, bool, error) {
	if observation.SealedTargetAuthorizationContradicted() {
		return ownerValidationRepairInfeasibility{}, false, nil
	}
	_, repairPurpose, err := currentRuntimeSemanticValidationFailureRepair(aggregate)
	if err != nil || !repairPurpose {
		return ownerValidationRepairInfeasibility{}, false, err
	}
	focus := aggregate.Facts.Focus.Normalized()
	if len(focus.CoverageWorkItemIDs) != 1 {
		return ownerValidationRepairInfeasibility{}, false, fmt.Errorf("%w: validation repair owner infeasibility has no unique current CWI", common.ErrConflict)
	}
	coverageID := focus.CoverageWorkItemIDs[0]
	index := runtimeCWICoverageIndex(aggregate.Facts.Coverage, coverageID)
	if index < 0 || aggregate.Facts.Coverage[index].CurrentBindingRef == nil {
		return ownerValidationRepairInfeasibility{}, false, fmt.Errorf("%w: validation repair owner infeasibility has no current binding", common.ErrConflict)
	}
	result := ownerValidationRepairInfeasibility{
		coverageID: coverageID,
		binding:    aggregate.Facts.Coverage[index].CurrentBindingRef.Clone(),
	}
	diagnostic := observation.Feedback.Normalized().OriginalDiagnostic
	if diagnostic == nil || strings.TrimSpace(diagnostic.Signature) == "" {
		return ownerValidationRepairInfeasibility{}, false, fmt.Errorf("%w: validation repair owner infeasibility has no diagnostic identity", common.ErrConflict)
	}
	result.causationRef = "validation-repair-owner-infeasible/" + diagnostic.Signature
	operation, operationFound := candidateOperationByID(*record.Proposal, feasibility.OperationID)
	descriptor, descriptorFound := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	result.contractDefect = !operationFound || !descriptorFound || descriptor.OwnerModuleID != diagnostic.OwnerModuleID ||
		!contains(operation.BindingRefs, result.binding.BindingID) ||
		strings.TrimSpace(feasibility.CandidateManifestHash) == "" || feasibility.CandidateManifestHash != record.ManifestHash
	return result, true, nil
}

func invalidateCandidateAttemptForOwnerObservation(aggregate *RuntimeAggregate, idx int, observation runtimecontract.RuntimeOwnerFeasibilityObservation, contradiction *runtimecontract.RuntimeBindingContradictionIdentity) {
	record := &aggregate.CandidateAttempts[idx]
	record.State = runtimecontract.RuntimeCandidateAttemptStateInvalidated
	record.Proposal = nil
	record.Conformances = nil
	record.Feasibilities = nil
	record.Failure = nil
	record.Stale = nil
	record.PrerequisiteRequest = nil
	copyObservation := observation.Clone()
	record.OwnerFeasibility = &copyObservation
	record.BindingContradiction = contradiction
}

func applyOwnerBindingContradiction(aggregate *RuntimeAggregate, contradiction ownerBindingContradiction) error {
	facts := aggregate.Facts
	if err := retireContradictedRuntimeCWIBinding(&facts, contradiction); err != nil {
		return err
	}
	updated, err := replaceRuntimeCWIFocusProjection(facts, []common.CoverageWorkItemID{contradiction.coverageID})
	if err != nil {
		return err
	}
	contract, err := canonicalRuntimeCWIReconciliationContract(updated)
	if err != nil {
		return err
	}
	updated.ReconciliationContract = contract
	aggregate.Facts = updated
	if _, err := supersedeActiveReconciliationForCurrentContract(aggregate); err != nil {
		return err
	}
	if err := retireSemanticPlanningGenerationForBindingContradiction(aggregate, contradiction.coverageID, contradiction.identity.ObservationHash); err != nil {
		return err
	}
	facts = aggregate.Facts
	facts.Failure = nil
	facts, decision, err := reduceRuntimeAggregateWithDirective(*aggregate, facts, nil)
	if err != nil {
		return err
	}
	aggregate.Facts, aggregate.Decision = facts, decision
	return nil
}

// applyOwnerValidationRepairInfeasibility is one private SM-2/SM-3 CAS.  A
// repair-purpose binding is intermediate authority and cannot survive after
// its exact operation set is owner-infeasible; retaining it without its repair
// directive is invalid, while silently opening another generation would loop.
// Retire the binding and terminate boundedly without dispatch or effect.
func applyOwnerValidationRepairInfeasibility(aggregate *RuntimeAggregate, observation runtimecontract.RuntimeOwnerFeasibilityObservation, infeasibility ownerValidationRepairInfeasibility) error {
	if aggregate == nil || infeasibility.coverageID == "" || strings.TrimSpace(infeasibility.causationRef) == "" {
		return common.ErrInvalidArgument
	}
	if err := retireRuntimeValidationRepairBinding(aggregate, infeasibility.coverageID, infeasibility.binding, infeasibility.causationRef,
		"validation-repair operation set was rejected by its capability owner before dispatch"); err != nil {
		return err
	}
	failure := ownerInfeasibilityFailure(observation, false)
	if infeasibility.contractDefect {
		failure.Class = runtimecontract.FailureClassRuntimeContractMisalignment
		failure.Code = "runtime_owner_contract_defect"
		failure.Signature = "validation-repair-owner-contract-defect/" + failure.Signature
		failure.Message = "validation-repair owner infeasibility is not bound to its sealed operation authority"
	} else {
		failure.Class = runtimecontract.FailureClassCandidateInfeasible
		failure.Code = "repair_surface_unavailable"
		failure.Signature = "repair-surface-unavailable/" + failure.Signature
		failure.Message = "sealed validation-repair operation set is not feasible: " + failure.Message
	}
	if err := replaceRuntimeFailureAuthority(&aggregate.Facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
		return err
	}
	resolvedFacts, decision, err := reduceRuntimeAggregateWithDirective(*aggregate, aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return err
	}
	aggregate.Facts, aggregate.Decision = resolvedFacts, decision
	return nil
}

func retireContradictedRuntimeCWIBinding(facts *runtimecontract.RuntimeFacts, contradiction ownerBindingContradiction) error {
	for idx := range facts.Coverage {
		item := &facts.Coverage[idx]
		if item.ID != contradiction.coverageID || item.CurrentBindingRef == nil || !runtimecontract.SameRuntimeCWIBindingRef(*item.CurrentBindingRef, contradiction.binding) {
			continue
		}
		if len(item.BindingRetirementLineage) >= runtimecontract.RuntimeCWIBindingRetirementLineageLimit {
			return fmt.Errorf("runtime CWI binding retirement lineage exhausted")
		}
		item.BindingRetirementLineage = append(item.BindingRetirementLineage, runtimecontract.RuntimeCWIBindingRetirementLineage{
			RetiredBindingRef: contradiction.binding, CausationRef: contradiction.identity.ObservationHash,
			Reason: "capability owner contradicted the sealed binding before dispatch",
		})
		item.CurrentBindingRef = nil
		item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStatePlanning
	}
	return nil
}

func applyOwnerInfeasibilityFailure(aggregate *RuntimeAggregate, observation runtimecontract.RuntimeOwnerFeasibilityObservation, contradictionUnbound bool) error {
	failure := ownerInfeasibilityFailure(observation, contradictionUnbound)
	if err := replaceRuntimeFailureAuthority(&aggregate.Facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
		return err
	}
	facts := aggregate.Facts
	facts, decision, err := reduceRuntimeAggregateWithDirective(*aggregate, facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		return err
	}
	aggregate.Facts, aggregate.Decision = facts, decision
	return nil
}

func ownerInfeasibilityFailure(observation runtimecontract.RuntimeOwnerFeasibilityObservation, contradictionUnbound bool) runtimecontract.FailureFact {
	diagnostic := observation.Feedback.OriginalDiagnostic
	failure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassCandidateInfeasible, Code: diagnostic.OriginalCode,
		Signature: diagnostic.Signature, Message: diagnostic.OriginalMessage, TargetRefs: append([]string(nil), diagnostic.TargetRefs...),
	}
	if observation.Outcome == runtimecontract.RuntimeOwnerFeasibilityContractDefect {
		failure = runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_owner_contract_defect",
			Signature: "runtime-owner-contract-defect/" + diagnostic.Signature, Message: "capability owner emitted an undeclared feasibility code",
			TargetRefs: append([]string(nil), diagnostic.TargetRefs...),
		}
	}
	if contradictionUnbound {
		failure = runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_contract_defect",
			Signature: "binding-contradiction-unbound/" + diagnostic.Signature, Message: "owner contradiction cannot be tied to one current binding",
			TargetRefs: append([]string(nil), diagnostic.TargetRefs...),
		}
	}
	if observation.FailureCategory() == runtimecontract.RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable {
		failure.Class = runtimecontract.FailureClassCandidateInfeasible
		failure.Code = "validation_recipe_exhausted"
		failure.Signature = "validation-recipe-exhausted/" + diagnostic.Signature
		failure.Message = "sealed validation recipe is unavailable: " + diagnostic.OriginalMessage
	}
	failure.OriginalDiagnostic = diagnostic.CloneOriginal()
	return failure
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
		failure := runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted, Code: "prerequisite_refresh_exhausted",
			Signature: signature, Message: "prerequisite refresh budget exhausted: " + stale.Reason,
		}
		if err := replaceRuntimeFailureAuthority(&facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
			return RuntimeAggregate{}, err
		}
	}
	facts, decision, err := reduceRuntimeAggregateWithDirective(aggregate, facts, nil)
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
