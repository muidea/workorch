package runtimev2

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// InvalidateCandidateAttemptForPrerequisiteStale abandons an unsealed
// candidate request/proposal after an owner read-only probe proves that one
// prerequisite observation is obsolete. It is deliberately not
// FailCandidateAttempt: no candidate retry, repair debt, provider retry, or
// evidence is created.
func (c *RuntimeController) InvalidateCandidateAttemptForPrerequisiteStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, attempt int, stale runtimecontract.RuntimePrerequisiteStale) (RuntimeAggregate, error) {
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
	ledger, consumed, err := facts.Recovery.Consume(runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, facts.RecoveryPolicy)
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
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// InvalidateActiveDispatchForPrerequisiteStale closes a sealed dispatch after
// an owner has persisted a no-effect stale receipt. The stale receipt remains
// diagnostic lineage only; it cannot create rejected evidence or repair debt.
func (c *RuntimeController) InvalidateActiveDispatchForPrerequisiteStale(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, receipt runtimecontract.RuntimeOperationReceipt) (RuntimeAggregate, error) {
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
	ledger, consumed, err := facts.Recovery.Consume(runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh, facts.RecoveryPolicy)
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
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// applyReceiptResourceChangeInvalidation is called while committing accepted
// evidence. It keeps the accepted receipt/evidence, but atomically prevents
// any remaining sealed operation from using an intersecting old observation.
func applyReceiptResourceChangeInvalidation(aggregate *RuntimeAggregate, dispatchID, operationID string, receipt runtimecontract.RuntimeOperationReceipt) (bool, error) {
	if aggregate == nil {
		return false, common.ErrInvalidArgument
	}
	receipt = receipt.Normalized()
	if len(receipt.ResourceChanges) == 0 {
		return false, nil
	}
	receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	signature := runtimecontract.StableRuntimeResourceChangeInvalidationSignature(receiptHash, receipt.ResourceChanges)
	if prerequisiteInvalidationExists(*aggregate, signature) {
		return false, nil
	}
	facts, prerequisiteIDs, observationIDs, invalidatedDependencies, err := invalidatePrerequisiteObservationsForChanges(aggregate.Facts, receipt.ResourceChanges)
	if err != nil {
		return false, err
	}
	if len(prerequisiteIDs) == 0 {
		return false, nil
	}
	// A resource-changing accepted receipt is forward progress, not a stale
	// recovery. Keep its bounded re-acquisition allowance separate so normal
	// multi-owner workflows cannot exhaust the stale/no-progress budget.
	ledger, consumed, err := facts.Recovery.Consume(runtimecontract.RuntimeRecoveryBudgetPrerequisiteResourceChangeRefresh, facts.RecoveryPolicy)
	if err != nil {
		return false, err
	}
	facts.Recovery = ledger
	aggregate.Facts = facts
	aggregate.PrerequisiteInvalidations = append(aggregate.PrerequisiteInvalidations, runtimecontract.RuntimePrerequisiteInvalidationRecord{
		Signature: signature, PrerequisiteIDs: prerequisiteIDs, ObservationIDs: observationIDs,
		InvalidatedDependencies: invalidatedDependencies,
		DispatchID:              strings.TrimSpace(dispatchID), OperationID: strings.TrimSpace(operationID), ReceiptHash: receiptHash,
		ResourceChanges: receipt.ResourceChanges,
	})
	if !consumed {
		aggregate.Facts.Failure = &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted, Code: "prerequisite_receipt_refresh_exhausted",
			Signature: signature, Message: "prerequisite receipt refresh budget exhausted after owner resource revision",
		}
	}
	return true, nil
}

func prerequisiteInvalidationExists(aggregate RuntimeAggregate, signature string) bool {
	signature = strings.TrimSpace(signature)
	for _, value := range aggregate.PrerequisiteInvalidations {
		if value.Normalized().Signature == signature {
			return true
		}
	}
	return false
}

// validatePrerequisiteResultFreshness prevents an owner response produced for
// a previous instance of a stable prerequisite ID from satisfying the current
// re-acquisition. Prerequisites intentionally retain their semantic ID after
// invalidation so reducer ordering and owner contracts stay stable; the
// persisted owner observation is consequently the generation fence.
//
// This is an Orchestrator admission rule, not a DTO transition: capability
// owners continue to report only their current observation, while SM-2 decides
// whether that observation may re-enter the aggregate.
func validatePrerequisiteResultFreshness(result PrerequisiteResult, invalidations []runtimecontract.RuntimePrerequisiteInvalidationRecord) error {
	prerequisiteID := strings.TrimSpace(result.PrerequisiteID)
	if prerequisiteID == "" {
		return common.ErrInvalidArgument
	}
	for _, invalidation := range invalidations {
		invalidation = invalidation.Normalized()
		if !containsFreshnessString(invalidation.PrerequisiteIDs, prerequisiteID) {
			continue
		}
		if result.Observation == nil {
			return fmt.Errorf("%w: prerequisite %q was invalidated and requires a fresh owner observation", common.ErrConflict, prerequisiteID)
		}
		observation := result.Observation.Normalized()
		for _, dependency := range observation.Dependencies {
			for _, invalidatedDependency := range invalidation.InvalidatedDependencies {
				invalidatedDependency = invalidatedDependency.Normalized()
				if dependency.Domain == invalidatedDependency.Domain && dependency.OwnerModuleID == invalidatedDependency.OwnerModuleID &&
					dependency.SubjectRef == invalidatedDependency.SubjectRef && dependency.Revision == invalidatedDependency.Revision {
					return fmt.Errorf("%w: prerequisite %q result retains an invalidated owner freshness dependency", common.ErrConflict, prerequisiteID)
				}
			}
			if invalidation.Stale != nil {
				stale := invalidation.Stale.Normalized()
				if dependency.Domain == stale.Domain && dependency.OwnerModuleID == stale.OwnerModuleID &&
					dependency.SubjectRef == stale.SubjectRef && dependency.Revision == stale.ExpectedRevision {
					return fmt.Errorf("%w: prerequisite %q result retains superseded %s revision", common.ErrConflict, prerequisiteID, stale.Domain)
				}
			}
			for _, change := range invalidation.ResourceChanges {
				if runtimecontract.RuntimeFreshnessDependencyIntersectsChange(dependency, change) &&
					dependency.Revision == change.BeforeRevision {
					return fmt.Errorf("%w: prerequisite %q result retains resource revision superseded by durable receipt", common.ErrConflict, prerequisiteID)
				}
			}
		}
	}
	return nil
}

func containsFreshnessString(values []string, value string) bool {
	value = strings.TrimSpace(value)
	for _, candidate := range values {
		if strings.TrimSpace(candidate) == value {
			return true
		}
	}
	return false
}

func invalidatePrerequisiteObservationsForStale(facts runtimecontract.RuntimeFacts, stale runtimecontract.RuntimePrerequisiteStale) (runtimecontract.RuntimeFacts, []string, []string, []runtimecontract.RuntimeFreshnessDependency, error) {
	facts = facts.Normalized()
	stale = stale.Normalized()
	observation, found := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, stale.PrerequisiteID)
	if !found || observation.ObservationID != stale.ObservationID {
		return runtimecontract.RuntimeFacts{}, nil, nil, nil, fmt.Errorf("%w: stale prerequisite observation is not current", common.ErrConflict)
	}
	matchedDependency := false
	for _, dependency := range observation.Dependencies {
		if dependency.Domain == stale.Domain && dependency.OwnerModuleID == stale.OwnerModuleID && dependency.SubjectRef == stale.SubjectRef && dependency.Revision == stale.ExpectedRevision {
			matchedDependency = true
			break
		}
	}
	if !matchedDependency {
		return runtimecontract.RuntimeFacts{}, nil, nil, nil, fmt.Errorf("%w: stale prerequisite dependency is not current", common.ErrConflict)
	}
	updated, prerequisiteIDs, observationIDs, dependencies, err := invalidatePrerequisiteObservations(facts, []runtimecontract.RuntimePrerequisiteObservation{observation})
	if err != nil {
		return runtimecontract.RuntimeFacts{}, nil, nil, nil, err
	}
	return updated, prerequisiteIDs, observationIDs, freshnessDependenciesMatchingStale(dependencies, stale), nil
}

func invalidatePrerequisiteObservationsForChanges(facts runtimecontract.RuntimeFacts, changes []runtimecontract.RuntimeResourceChange) (runtimecontract.RuntimeFacts, []string, []string, []runtimecontract.RuntimeFreshnessDependency, error) {
	facts = facts.Normalized()
	changes = runtimecontract.NormalizeRuntimeResourceChanges(changes)
	affected := make([]runtimecontract.RuntimePrerequisiteObservation, 0)
	for _, observation := range facts.Resources.PrerequisiteObservations {
		for _, dependency := range observation.Dependencies {
			for _, change := range changes {
				if runtimecontract.RuntimeFreshnessDependencyIntersectsChange(dependency, change) {
					affected = append(affected, observation)
					goto nextObservation
				}
			}
		}
	nextObservation:
	}
	if len(affected) == 0 {
		return facts, nil, nil, nil, nil
	}
	updated, prerequisiteIDs, observationIDs, dependencies, err := invalidatePrerequisiteObservations(facts, affected)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, nil, nil, nil, err
	}
	return updated, prerequisiteIDs, observationIDs, freshnessDependenciesIntersectingChanges(dependencies, changes), nil
}

func freshnessDependenciesMatchingStale(values []runtimecontract.RuntimeFreshnessDependency, stale runtimecontract.RuntimePrerequisiteStale) []runtimecontract.RuntimeFreshnessDependency {
	stale = stale.Normalized()
	ret := make([]runtimecontract.RuntimeFreshnessDependency, 0, 1)
	for _, dependency := range values {
		dependency = dependency.Normalized()
		if dependency.Domain == stale.Domain && dependency.OwnerModuleID == stale.OwnerModuleID &&
			dependency.SubjectRef == stale.SubjectRef && dependency.Revision == stale.ExpectedRevision {
			ret = append(ret, dependency)
		}
	}
	return runtimecontract.NormalizeRuntimeFreshnessDependencies(ret)
}

func freshnessDependenciesIntersectingChanges(values []runtimecontract.RuntimeFreshnessDependency, changes []runtimecontract.RuntimeResourceChange) []runtimecontract.RuntimeFreshnessDependency {
	changes = runtimecontract.NormalizeRuntimeResourceChanges(changes)
	ret := make([]runtimecontract.RuntimeFreshnessDependency, 0, len(values))
	for _, dependency := range values {
		for _, change := range changes {
			if runtimecontract.RuntimeFreshnessDependencyIntersectsChange(dependency, change) {
				ret = append(ret, dependency)
				break
			}
		}
	}
	return runtimecontract.NormalizeRuntimeFreshnessDependencies(ret)
}

func invalidatePrerequisiteObservations(facts runtimecontract.RuntimeFacts, observations []runtimecontract.RuntimePrerequisiteObservation) (runtimecontract.RuntimeFacts, []string, []string, []runtimecontract.RuntimeFreshnessDependency, error) {
	facts = facts.Normalized()
	byID := map[string]runtimecontract.RuntimePrerequisiteObservation{}
	for _, observation := range observations {
		observation = observation.Normalized()
		byID[observation.PrerequisiteID] = observation
	}
	if len(byID) == 0 {
		return facts, nil, nil, nil, nil
	}
	satisfied := make(map[string]runtimecontract.RuntimePrerequisite, len(facts.Prerequisites.Satisfied))
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		satisfied[prerequisite.ID] = prerequisite
	}
	prerequisiteIDs := make([]string, 0, len(byID))
	observationIDs := make([]string, 0, len(byID))
	invalidatedDependencies := make([]runtimecontract.RuntimeFreshnessDependency, 0)
	for prerequisiteID, observation := range byID {
		prerequisite, found := satisfied[prerequisiteID]
		if !found || prerequisite.Kind != observation.Kind || prerequisite.OwnerModuleID != observation.OwnerModuleID {
			return runtimecontract.RuntimeFacts{}, nil, nil, nil, fmt.Errorf("%w: observation %q has no current satisfied prerequisite", common.ErrConflict, prerequisiteID)
		}
		if err := removePrerequisiteResourceFact(&facts, prerequisite); err != nil {
			return runtimecontract.RuntimeFacts{}, nil, nil, nil, err
		}
		prerequisiteIDs = append(prerequisiteIDs, prerequisiteID)
		observationIDs = append(observationIDs, observation.ObservationID)
		invalidatedDependencies = append(invalidatedDependencies, observation.Dependencies...)
	}
	// A failure target observed by a capability is only authoritative while the
	// owner catalog revision that produced it remains fresh. The invalidated
	// prerequisite already carries that owner-neutral dependency; do not retain
	// its target as a second, stale source of repair authority.
	invalidateFailureTargetObservations(&facts, byID)
	keptSatisfied := make([]runtimecontract.RuntimePrerequisite, 0, len(facts.Prerequisites.Satisfied))
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		if _, invalidated := byID[prerequisite.ID]; !invalidated {
			keptSatisfied = append(keptSatisfied, prerequisite)
		}
	}
	facts.Prerequisites.Satisfied = keptSatisfied
	for _, prerequisiteID := range prerequisiteIDs {
		facts.Prerequisites.Pending = append(facts.Prerequisites.Pending, satisfied[prerequisiteID])
	}
	keptObservations := make([]runtimecontract.RuntimePrerequisiteObservation, 0, len(facts.Resources.PrerequisiteObservations))
	for _, observation := range facts.Resources.PrerequisiteObservations {
		if _, invalidated := byID[observation.PrerequisiteID]; !invalidated {
			keptObservations = append(keptObservations, observation)
		}
	}
	facts.Resources.PrerequisiteObservations = keptObservations
	facts.Feasibility = nil
	return facts.Normalized(), normalizedFreshnessStrings(prerequisiteIDs), normalizedFreshnessStrings(observationIDs), runtimecontract.NormalizeRuntimeFreshnessDependencies(invalidatedDependencies), nil
}

func invalidateFailureTargetObservations(facts *runtimecontract.RuntimeFacts, invalidated map[string]runtimecontract.RuntimePrerequisiteObservation) {
	if facts == nil || facts.Resources.FailureTarget == nil || facts.Resources.FailureTarget.TargetObservation == nil {
		return
	}
	targetObservation := facts.Resources.FailureTarget.TargetObservation.Normalized()
	if !failureTargetObservationDependsOnInvalidatedOwnerRevision(targetObservation, invalidated) {
		return
	}
	facts.Resources.FailureTarget = nil
	if facts.Failure != nil && sameFailureTargetObservationBinding(facts.Failure.TargetObservation, &targetObservation) {
		facts.Failure.TargetRefs = nil
		facts.Failure.TargetObservation = nil
	}
	// The directive is a sealed SM-2 decision derived from the same stale
	// failure fact. ReduceRuntimeStateWithDirective will rebuild it from the
	// remaining fresh facts after re-acquisition; retaining it would make its
	// target refs an unauthorized fallback.
	if facts.RepairDirective != nil && facts.RepairDirective.CurrentFailure != nil &&
		sameFailureTargetObservationBinding(facts.RepairDirective.CurrentFailure.TargetObservation, &targetObservation) {
		facts.RepairDirective = nil
	}
}

func failureTargetObservationDependsOnInvalidatedOwnerRevision(target runtimecontract.RuntimeFailureTargetObservation, invalidated map[string]runtimecontract.RuntimePrerequisiteObservation) bool {
	target = target.Normalized()
	for _, observation := range invalidated {
		for _, dependency := range observation.Normalized().Dependencies {
			if dependency.OwnerModuleID == target.OwnerModuleID && dependency.Revision == target.ObservedRevision {
				return true
			}
		}
	}
	return false
}

func sameFailureTargetObservationBinding(left *runtimecontract.RuntimeFailureTargetObservation, right *runtimecontract.RuntimeFailureTargetObservation) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	l := left.Normalized()
	r := right.Normalized()
	if l.OwnerModuleID != r.OwnerModuleID || l.SubjectRef != r.SubjectRef || l.TargetRole != r.TargetRole || l.ObservedRevision != r.ObservedRevision || len(l.TargetRefs) != len(r.TargetRefs) {
		return false
	}
	for idx := range l.TargetRefs {
		if l.TargetRefs[idx] != r.TargetRefs[idx] {
			return false
		}
	}
	return true
}

func normalizedFreshnessStrings(values []string) []string {
	ret := []string{}
	for _, value := range values {
		ret = appendNormalizedString(ret, value)
	}
	return ret
}

func removePrerequisiteResourceFact(facts *runtimecontract.RuntimeFacts, prerequisite runtimecontract.RuntimePrerequisite) error {
	if facts == nil {
		return common.ErrInvalidArgument
	}
	switch prerequisite.Kind {
	case runtimecontract.PrerequisiteKindManifest:
		facts.Resources.Manifest = nil
	case runtimecontract.PrerequisiteKindTargetSnapshot:
		kept := make([]runtimecontract.TargetSnapshot, 0, len(facts.Resources.TargetSnapshots))
		for _, snapshot := range facts.Resources.TargetSnapshots {
			if snapshot.TargetRef != prerequisite.SubjectRef {
				kept = append(kept, snapshot)
			}
		}
		facts.Resources.TargetSnapshots = kept
	case runtimecontract.PrerequisiteKindTargetSelection:
		facts.Resources.TargetSelection = nil
	case runtimecontract.PrerequisiteKindCapabilityState:
		kept := make([]runtimecontract.RuntimeCapabilityStateSnapshot, 0, len(facts.Resources.CapabilityStates))
		for _, snapshot := range facts.Resources.CapabilityStates {
			if snapshot.RequirementID != prerequisite.ID {
				kept = append(kept, snapshot)
			}
		}
		facts.Resources.CapabilityStates = kept
	case runtimecontract.PrerequisiteKindValidationScratch:
		facts.Resources.ValidationScratch = nil
	case runtimecontract.PrerequisiteKindProviderCeiling:
		facts.Resources.ProviderCeiling = nil
	case runtimecontract.PrerequisiteKindOperationAdmissibility:
		kept := make([]runtimecontract.RuntimeOperationAdmissibility, 0, len(facts.Resources.OperationAdmissibilities))
		for _, inventory := range facts.Resources.OperationAdmissibilities {
			if inventory.PrerequisiteID != prerequisite.ID {
				kept = append(kept, inventory)
			}
		}
		facts.Resources.OperationAdmissibilities = kept
	default:
		return fmt.Errorf("freshness invalidation does not support prerequisite kind %q", prerequisite.Kind)
	}
	return nil
}
