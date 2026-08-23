package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *authorityComponent) reconcileAuthorityBeforeCandidate(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch != nil || isTerminalConvergenceState(aggregate.Decision.State) || len(InspectRuntimeAuthorityInvariants(aggregate).Violations) == 0 {
		return aggregate, nil
	}
	expectedRevision := aggregate.StateRevision
	// A provider ceiling is activation-sealed by bootstrap. If an owner resource
	// was cleared or became insufficient while its prerequisite still appears
	// active, return to the existing typed acquisition rather than allowing the
	// LLM boundary to discover the defect after an SM-3 attempt exists. Do not
	// synthesize a new/default requirement here; activation is the sole producer
	// of the sealed provider contract.
	facts := aggregate.Facts
	if requirements := activeProviderCeilingRequirements(facts); len(requirements) != 0 {
		facts = PreflightRuntimeFacts(facts, requirements)
	}
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

// recordFeasibilityFact CAS-persists the pre-candidate capability gate and
// immediately re-reduces. A missing operation is therefore a formal SM-2 fact,
// never a provider error or consumed candidate attempt.

func (c *authorityComponent) recordFeasibilityFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimecontract.RuntimeFeasibilityFact) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.ActiveDispatch != nil || (aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange && aggregate.Decision.State != runtimecontract.ConvergenceStateNeedValidation) {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not accept feasibility facts", common.ErrConflict)
	}
	fact.FocusID = strings.TrimSpace(fact.FocusID)
	fact.Category = runtimecontract.RuntimeOwnerFeasibilityCategory(strings.TrimSpace(string(fact.Category)))
	if fact.FocusID == "" {
		fact.FocusID = aggregate.Facts.Focus.ID
	}
	if fact.FocusID != aggregate.Facts.Focus.ID || fact.State != aggregate.Decision.State || fact.Reason == "" {
		return RuntimeAggregate{}, fmt.Errorf("%w: feasibility fact does not match current authority", common.ErrConflict)
	}
	if fact.Category != "" && !runtimecontract.ValidRuntimeOwnerFeasibilityCategory(fact.Category) {
		return RuntimeAggregate{}, fmt.Errorf("%w: feasibility fact has an unknown owner category", common.ErrConflict)
	}
	if fact.Category == runtimecontract.RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable &&
		(fact.State != runtimecontract.ConvergenceStateNeedValidation || len(fact.FeasibleOperationKinds) != 0) {
		return RuntimeAggregate{}, fmt.Errorf("%w: validation recipe unavailability must block the current validation authority", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	aggregate.Facts.Feasibility = &fact
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
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

// recordFailureFact persists an SM-2 failure input and lets the reducer choose
// repair, governance, or terminal. Drive/adapter errors must never project a
// Run terminal without this authority transition.

func (c *authorityComponent) recordFailureFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimecontract.FailureFact) (RuntimeAggregate, error) {
	return c.commitFailureFact(ctx, workspaceID, runID, fact, false)
}

// recordDriveFailureFact is the SM-4 error handoff. When a drive defect is
// observed while SM-3 still has an active dispatch, the same aggregate CAS
// invalidates that dispatch before SM-2 reduces the typed failure. This keeps
// generic failure recording narrow while ensuring Resume/restart never sees a
// new authority paired with the former dispatch transaction.

func (c *authorityComponent) recordDriveFailureFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimecontract.FailureFact) (RuntimeAggregate, error) {
	return c.commitFailureFact(ctx, workspaceID, runID, fact, true)
}

func (c *authorityComponent) commitFailureFact(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, fact runtimecontract.FailureFact, closeActiveDispatch bool) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || fact.Class == "" || strings.TrimSpace(fact.Message) == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if isTerminalConvergenceState(aggregate.Decision.State) {
		return aggregate, nil
	}
	expectedRevision := aggregate.StateRevision
	aggregate, err = applyFailureFactToAggregate(aggregate, fact, closeActiveDispatch)
	if err != nil {
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

// applyFailureFactToAggregate is the pure SM-2 half of failure persistence.
// Keeping repository/CAS work outside lets the owner-private provider-terminal
// coordinator apply this successor together with the SM-3 request close in one
// aggregate CAS without introducing a second mutation entry point.
func applyFailureFactToAggregate(aggregate RuntimeAggregate, fact runtimecontract.FailureFact, closeActiveDispatch bool) (RuntimeAggregate, error) {
	if isTerminalConvergenceState(aggregate.Decision.State) {
		return RuntimeAggregate{}, fmt.Errorf("%w: terminal runtime cannot accept another failure fact", common.ErrConflict)
	}
	if aggregate.ActiveDispatch != nil && !closeActiveDispatch {
		return RuntimeAggregate{}, fmt.Errorf("%w: active dispatch failures require a dispatch/receipt-specific transition", common.ErrConflict)
	}
	fact.Code = strings.TrimSpace(fact.Code)
	fact.Signature = strings.TrimSpace(fact.Signature)
	fact.Message = strings.TrimSpace(fact.Message)
	if aggregate.ActiveDispatch != nil {
		dispatch := *aggregate.ActiveDispatch
		dispatch.State = DispatchStateInvalidated
		aggregate.CompletedDispatches = append(aggregate.CompletedDispatches, dispatch)
		aggregate.ActiveDispatch = nil
	}
	aggregate.Facts.Failure = &fact
	// A persisted FailureTarget is scoped to the current typed failure identity.
	// Re-recording a failure with a different signature/target set must return to
	// the existing local prerequisite, never leave the former mapping as an
	// implicit repair surface until a later drive happens to preflight it.
	facts := aggregate.Facts
	if requirements := activeFailureTargetRequirements(facts); len(requirements) != 0 {
		facts = PreflightRuntimeFacts(facts, requirements)
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts = facts
	aggregate.Decision = decision
	closePendingCandidateAttemptsForTerminal(&aggregate, fact)
	return aggregate, nil
}

func activeFailureTargetRequirements(facts runtimecontract.RuntimeFacts) []runtimecontract.RuntimeRequirement {
	facts = facts.Normalized()
	values := make([]runtimecontract.RuntimeRequirement, 0, 1)
	seen := make(map[string]struct{})
	for _, prerequisite := range append(append([]runtimecontract.RuntimePrerequisite(nil), facts.Prerequisites.Pending...), facts.Prerequisites.Satisfied...) {
		if prerequisite.Kind != runtimecontract.PrerequisiteKindFailureTarget {
			continue
		}
		requirement := runtimecontract.RuntimeRequirement{
			Kind: prerequisite.Kind, Scope: prerequisite.Scope, FocusID: prerequisite.FocusID,
			OwnerModuleID: prerequisite.OwnerModuleID, SubjectRef: prerequisite.SubjectRef,
			Reason: prerequisite.Reason,
		}
		key := runtimePrerequisiteID(requirement)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		values = append(values, requirement)
	}
	return normalizeRuntimeRequirements(values)
}

// requestTermination CAS-persists an administrative termination record on an active V2
// aggregate (INV-08). SM-1 may project cancelled/superseded only after this record exists.

func (c *authorityComponent) commitPrerequisite(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, result PrerequisiteResult, recoverableWaitProbe ...*runtimecontract.RuntimeRecoverableWaitProbeObservation) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		return RuntimeAggregate{}, fmt.Errorf("%w: current runtime action does not accept prerequisite results", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	probeObservation := firstRuntimeRecoverableWaitProbeObservation(recoverableWaitProbe)
	if _, _, waiting := currentRuntimeRecoverableWait(aggregate.RecoverableWaits); waiting && probeObservation == nil {
		return RuntimeAggregate{}, fmt.Errorf("%w: active recoverable wait requires a matching probe observation", common.ErrConflict)
	}
	previousFocusID := aggregate.Facts.Focus.ID
	semanticCatalogRefreshPending := semanticCatalogPrerequisitePending(aggregate, result.PrerequisiteID)
	// Validate the concrete owner fact before consuming a recovery probe. A
	// stale/mismatched result cannot spend the current wait allocation, even if
	// its observation timestamp is already beyond the deadline.
	if err := validatePrerequisiteResultFreshness(result, aggregate.PrerequisiteInvalidations); err != nil {
		return RuntimeAggregate{}, err
	}
	facts, err := CommitPrerequisiteResult(aggregate.Facts, result)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if probeObservation != nil {
		if result.Observation == nil || result.Observation.ObservationID != probeObservation.OwnerResultHash {
			return RuntimeAggregate{}, fmt.Errorf("%w: recoverable wait probe observation does not bind the prerequisite result", common.ErrConflict)
		}
		expired, confirmErr := confirmRecoverableWaitProbe(&aggregate, *probeObservation)
		if confirmErr != nil {
			return RuntimeAggregate{}, confirmErr
		}
		if expired {
			aggregate.StateRevision++
			if err := aggregate.Validate(); err != nil {
				return RuntimeAggregate{}, err
			}
			if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
				return RuntimeAggregate{}, err
			}
			return aggregate, nil
		}
	}
	// A prerequisite ID is stable across re-acquisition. It is therefore not
	// sufficient to match only the pending ID here: a response started before a
	// durable resource change can otherwise satisfy the newly-pending instance
	// after that change invalidated the original observation. The persisted
	// invalidation records are the authoritative freshness fence; never accept
	// a result whose owner observation still reports the superseded revision.
	aggregate.Facts = facts
	semanticGenerationRetired := false
	// A receipt-driven owner catalog change can invalidate the sealed semantic
	// generation even when the current focus ID remains unchanged (for example
	// a partial dispatch or a target refresh). Reopen semantic planning before
	// any candidate path can consume the reacquired facts; an active dispatch is
	// fenced by resetSemanticPlanningForInvalidatedCatalog itself.
	if aggregate.ActiveDispatch == nil && semanticCatalogRefreshPending {
		retired, resetErr := resetSemanticPlanningForInvalidatedCatalog(&aggregate, runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated)
		if resetErr != nil {
			return RuntimeAggregate{}, resetErr
		}
		if !retired {
			// A prerequisite can be invalidated and reacquired without changing
			// the owner catalog identity. In that case the sealed generation is
			// still current and must remain reusable; only a remaining freshness
			// gap is a contract conflict.
			if aggregate.SemanticPlanning.Manifest != nil {
				if gaps, current := semanticPlanningCatalogsCurrent(aggregate, *aggregate.SemanticPlanning.Manifest); !current {
					return RuntimeAggregate{}, fmt.Errorf("%w: semantic catalog prerequisite was reacquired without retiring the stale generation (gaps=%d)", common.ErrConflict, len(gaps))
				}
			}
		}
		semanticGenerationRetired = retired
	}
	// A single-operation durable dispatch may have completed its current
	// coverage before its receipt invalidated mutable prerequisites. A changed
	// catalog must first re-seal the current binding and let SM-2 persist its
	// proof-lineage resolution; advancing under the retired binding would skip
	// the equivalent/incompatible gate. When re-acquisition retained the same
	// catalog identity, the existing exact binding remains current and ordinary
	// focus advancement is still valid.
	facts = aggregate.Facts
	if !semanticGenerationRetired && len(facts.Prerequisites.Pending) == 0 && len(facts.FreshnessReacquireCoverage) == 0 {
		advanced, advanceErr := advanceRuntimeCWIFocus(facts, aggregate.CoverageProofLineage)
		if advanceErr != nil {
			return RuntimeAggregate{}, advanceErr
		}
		facts = advanced
		aggregate.Facts = facts
	}
	if aggregate.Facts.Focus.ID != previousFocusID {
		if err := prepareSemanticPlanningForAdvancedFocus(&aggregate); err != nil {
			return RuntimeAggregate{}, err
		}
		facts = aggregate.Facts
	}
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

func firstRuntimeRecoverableWaitProbeObservation(values []*runtimecontract.RuntimeRecoverableWaitProbeObservation) *runtimecontract.RuntimeRecoverableWaitProbeObservation {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}
