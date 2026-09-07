package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	runtimeCWIReopenReasonValidationFailureRepairRequired = "validation_failure_repair_required"
	runtimeCWIReopenReasonValidationFailureRepairSettled  = "validation_failure_repair_settled"
	runtimeValidationFailureRepairSealRepair              = "repair"
	runtimeValidationFailureRepairSealFreshValidation     = "fresh_validation"
)

// Both pre-dispatch infeasibility and a rejected repair receipt retire the same
// intermediate SM-2 binding. This helper is private to the owner's atomic event
// transaction; it never emits another event or writes a sibling aggregate.
func retireRuntimeValidationRepairBinding(aggregate *RuntimeAggregate, coverageID common.CoverageWorkItemID, binding runtimecontract.RuntimeCWIBindingRef, causationRef, reason string) error {
	if aggregate == nil || coverageID == "" || strings.TrimSpace(causationRef) == "" || strings.TrimSpace(reason) == "" {
		return common.ErrInvalidArgument
	}
	facts := &aggregate.Facts
	index := runtimeCWICoverageIndex(facts.Coverage, coverageID)
	if index < 0 || facts.Coverage[index].CurrentBindingRef == nil || !runtimecontract.SameRuntimeCWIBindingRef(*facts.Coverage[index].CurrentBindingRef, binding) {
		return fmt.Errorf("%w: validation-repair retirement requires its exact current binding", common.ErrConflict)
	}
	item := &facts.Coverage[index]
	if len(item.BindingRetirementLineage) >= runtimecontract.RuntimeCWIBindingRetirementLineageLimit {
		return fmt.Errorf("runtime CWI binding retirement lineage exhausted")
	}
	item.BindingRetirementLineage = append(item.BindingRetirementLineage, runtimecontract.RuntimeCWIBindingRetirementLineage{
		RetiredBindingRef: binding, CausationRef: causationRef, Reason: reason,
	})
	item.CurrentBindingRef = nil
	item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStatePlanning
	updated, err := replaceRuntimeCWIFocusProjection(*facts, []common.CoverageWorkItemID{coverageID})
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
	return nil
}

// runtimeValidationFailureRepairTrigger is a detached view of the typed,
// already-persisted rejection and its immutable Profile declaration. It is
// descriptive only: the SM-2 transition below remains the sole owner of CWI
// reopen, repair budget and next-action authority.
type runtimeValidationFailureRepairTrigger struct {
	coverageID  common.CoverageWorkItemID
	fact        runtimecontract.EvidenceFact
	failure     runtimecontract.FailureFact
	operation   runtimecontract.RuntimeOperation
	convergence runtimecontract.RuntimeValidationFailureRepairConvergence
	route       runtimecontract.RuntimeValidationFailureRepairRoute
}

type runtimeValidationFailureRepairSealContinuation struct {
	kind       string
	coverageID common.CoverageWorkItemID
	repair     runtimecontract.RuntimeRepairLedger
	directive  runtimecontract.RuntimeRepairDirective
	failure    *runtimecontract.FailureFact
}

func (t runtimeValidationFailureRepairTrigger) validate() error {
	if t.coverageID == "" || t.fact.Kind != runtimecontract.EvidenceKindValidation ||
		t.fact.FailureSignature == "" || t.failure.Signature == "" || t.fact.FailureSignature != t.failure.Signature {
		return fmt.Errorf("validation failure repair trigger requires one matching rejected validation fact")
	}
	if err := t.convergence.Validate(); err != nil {
		return err
	}
	if err := t.route.Validate(); err != nil {
		return err
	}
	if t.failure.OriginalDiagnostic == nil || t.failure.ValidationDiagnostic == nil {
		return fmt.Errorf("validation failure repair trigger requires typed owner diagnostics")
	}
	diagnostic := t.failure.ValidationDiagnostic.Normalized()
	original := t.failure.OriginalDiagnostic.Normalized()
	operation := t.operation.Normalized()
	if original == nil || original.OperationID != t.fact.OperationID || operation.ID != t.fact.OperationID ||
		!t.route.MatchesTrigger(original.OwnerModuleID, operation.Kind, diagnostic.Class, diagnostic.RecommendedEvidenceTargetType, diagnostic.OwnerCode) {
		return fmt.Errorf("validation failure repair trigger does not match its Profile convergence")
	}
	matched, found := t.convergence.RouteForTrigger(original.OwnerModuleID, operation.Kind, diagnostic.Class, diagnostic.RecommendedEvidenceTargetType, diagnostic.OwnerCode)
	if !found || runtimecontract.StableRuntimeValidationFailureRepairRouteHash(matched) != runtimecontract.StableRuntimeValidationFailureRepairRouteHash(t.route) {
		return fmt.Errorf("validation failure repair trigger route is not sealed by its Profile convergence")
	}
	// A rejected receipt cannot receive terminal CWI credit, so its derived
	// CoverageWorkItemIDs are normally empty. If a producer does supply them,
	// they still may not escape the current sealed CWI.
	if len(t.fact.CoverageWorkItemIDs) != 0 && !coverageIDsIntersect(t.fact.CoverageWorkItemIDs, []common.CoverageWorkItemID{t.coverageID}) {
		return fmt.Errorf("validation failure repair trigger is outside current CWI coverage")
	}
	return nil
}

// openRuntimeValidationFailureRepairGeneration is the private SM-2 event
// reducer for a rejected validation which has no exact owner-grounded failure
// target. It retires the current validation binding and opens one bounded
// semantic generation. The Profile selector remains a declaration only; no
// concrete repair target is copied from the validation entry, stdout, focus
// target or target-selection cache.
func openRuntimeValidationFailureRepairGeneration(aggregate *RuntimeAggregate, fact runtimecontract.EvidenceFact, operation runtimecontract.RuntimeOperation) (bool, error) {
	if aggregate == nil {
		return false, common.ErrInvalidArgument
	}
	if aggregate.ActiveDispatch != nil {
		return false, fmt.Errorf("%w: validation failure repair cannot reopen a CWI with an active dispatch", common.ErrConflict)
	}
	trigger, matched, err := matchRuntimeValidationFailureRepairTrigger(*aggregate, fact, operation)
	if err != nil || !matched {
		return false, err
	}
	if trigger.route.RequiresStructuredObservation() {
		// Existing owner resources are never selected by a target-free LLM
		// generation. The validation owner must publish an exact bounded
		// observation and the resource owner must confirm it first.
		return false, nil
	}
	if aggregate.Facts.Repair.Exhausted || aggregate.Facts.Repair.RepairAttempts > trigger.convergence.MaxAttempts {
		if err := markRuntimeValidationFailureRepairExhausted(&aggregate.Facts, trigger); err != nil {
			return false, err
		}
		return false, nil
	}

	facts := aggregate.Facts.Normalized()
	coverageIndex := runtimeCWICoverageIndex(facts.Coverage, trigger.coverageID)
	if coverageIndex < 0 {
		return false, fmt.Errorf("%w: validation failure repair references unknown coverage %q", common.ErrConflict, trigger.coverageID)
	}
	item := &facts.Coverage[coverageIndex]
	if item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound || item.CurrentBindingRef == nil ||
		item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
		return false, fmt.Errorf("%w: validation failure repair requires one current unsatisfied binding", common.ErrConflict)
	}
	lineage := runtimecontract.RuntimeCWIReopenLineage{
		PreviousBindingRef: item.CurrentBindingRef.Clone(),
		TriggerRef:         string(trigger.fact.ID),
		Reason:             runtimeCWIReopenReasonValidationFailureRepairRequired,
	}.Normalized()
	if err := lineage.Validate(); err != nil {
		return false, err
	}
	if len(item.ReopenLineage) >= runtimecontract.RuntimeCWIReopenLineageLimit {
		return false, fmt.Errorf("runtime CWI reopen lineage exceeds bounded limit %d", runtimecontract.RuntimeCWIReopenLineageLimit)
	}
	item.ReopenLineage = append(item.ReopenLineage, lineage)
	item.CurrentBindingRef = nil
	item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateReopened
	item.State = runtimecontract.RuntimeCoverageItemStateOpen

	previousFocusID := facts.Focus.ID
	carriedRepair := facts.Repair.Normalized()
	carriedFailure := trigger.failure.Clone()
	updated, err := replaceRuntimeCWIFocusProjectionForSemanticRebinding(facts, []common.CoverageWorkItemID{trigger.coverageID})
	if err != nil {
		return false, err
	}
	retireFocusScopedPrerequisites(&updated, previousFocusID)
	updated.Resources.TargetSelection = nil
	updated.Resources.TargetSnapshots = nil
	updated.Resources.FailureTarget = nil
	updated.Feasibility = nil
	updated.Failure = &carriedFailure
	carriedRepair.FocusID = updated.Focus.ID
	carriedRepair.SliceID = updated.Focus.SliceID
	if carriedRepair.ActiveGap != nil {
		gap := carriedRepair.ActiveGap.Normalized()
		// The rejected validation surface is observation input, not repair
		// target authority. Only the repair-purpose semantic binding may later
		// install exact prospective targets.
		gap.TargetRefs = nil
		carriedRepair.ActiveGap = &gap
	}
	updated.Repair = carriedRepair.Normalized()
	directive := runtimeValidationFailureRepairPendingDirective(updated, trigger)
	if err := directive.Validate(); err != nil {
		return false, err
	}
	updated.RepairDirective = &directive
	contract, err := canonicalRuntimeCWIReconciliationContract(updated)
	if err != nil {
		return false, err
	}
	updated.ReconciliationContract = contract
	aggregate.Facts = updated.Normalized()
	if _, err := supersedeActiveReconciliationForCurrentContract(aggregate); err != nil {
		return false, err
	}
	if err := retireSemanticPlanningGenerationForCWIReopen(aggregate, []common.CoverageWorkItemID{trigger.coverageID}); err != nil {
		return false, err
	}
	return true, nil
}

func matchRuntimeValidationFailureRepairTrigger(aggregate RuntimeAggregate, fact runtimecontract.EvidenceFact, operation runtimecontract.RuntimeOperation) (runtimeValidationFailureRepairTrigger, bool, error) {
	facts := aggregate.Facts.Normalized()
	operation = operation.Normalized()
	if fact.Kind != runtimecontract.EvidenceKindValidation || facts.Failure == nil ||
		facts.Resources.FailureTarget != nil || needsStateChangeForOwnerGroundedFailure(facts) ||
		len(facts.Focus.CoverageWorkItemIDs) != 1 {
		return runtimeValidationFailureRepairTrigger{}, false, nil
	}
	coverageID := facts.Focus.CoverageWorkItemIDs[0]
	binding, bindingFound := coverageEvidenceBindingForCoverage(facts.Focus, coverageID)
	if !bindingFound || !contains(operation.BindingRefs, binding.ID) || !contains(fact.BindingRefs, binding.ID) {
		// The failed operation must be fenced by the exact current semantic
		// binding. Empty rejected CWI credit is expected; an unrelated binding is
		// never promoted into a repair generation.
		return runtimeValidationFailureRepairTrigger{}, false, nil
	}
	declaration, found, err := runtimeDefinitionFocusForCoverage(facts.DefinitionFocuses, coverageID)
	if err != nil || !found || declaration.ValidationFailureRepairConvergence == nil {
		return runtimeValidationFailureRepairTrigger{}, false, err
	}
	if facts.Failure.ValidationDiagnostic == nil || facts.Failure.OriginalDiagnostic == nil {
		return runtimeValidationFailureRepairTrigger{}, false, nil
	}
	diagnostic := facts.Failure.ValidationDiagnostic.Normalized()
	original := facts.Failure.OriginalDiagnostic.Normalized()
	if original == nil {
		return runtimeValidationFailureRepairTrigger{}, false, nil
	}
	route, routeFound := declaration.ValidationFailureRepairConvergence.RouteForTrigger(
		original.OwnerModuleID, operation.Kind, diagnostic.Class, diagnostic.RecommendedEvidenceTargetType, diagnostic.OwnerCode,
	)
	if !routeFound || (route.RequireInputAddition && !diagnostic.InputAddition.MatchesBinding(binding)) {
		return runtimeValidationFailureRepairTrigger{}, false, nil
	}
	trigger := runtimeValidationFailureRepairTrigger{
		coverageID:  coverageID,
		fact:        fact,
		failure:     facts.Failure.Clone(),
		operation:   operation,
		convergence: declaration.ValidationFailureRepairConvergence.Clone(),
		route:       route,
	}
	if err := trigger.validate(); err != nil {
		// Missing/raw/mismatched typed diagnostics are an ordinary fail-closed
		// non-match. They must never open clarification or prospective repair.
		return runtimeValidationFailureRepairTrigger{}, false, nil
	}
	return trigger, true, nil
}

// markRuntimeValidationFailureRepairSurfaceUnavailable is the private SM-2
// terminal transition for an exact existing-scope Profile route whose owner
// receipt did not contain a structured repair-surface observation. The route
// exists, but its mandatory owner-fact precondition does not; semantic fallback
// is therefore forbidden and cannot create Definition clarification authority.
func markRuntimeValidationFailureRepairSurfaceUnavailable(aggregate *RuntimeAggregate, fact runtimecontract.EvidenceFact, operation runtimecontract.RuntimeOperation) (bool, error) {
	if aggregate == nil {
		return false, common.ErrInvalidArgument
	}
	trigger, matched, err := matchRuntimeValidationFailureRepairTrigger(*aggregate, fact, operation)
	if err != nil || !matched || !trigger.route.RequiresStructuredObservation() {
		return false, err
	}
	if trigger.failure.ValidationFailureObservation != nil {
		return false, nil
	}
	facts := aggregate.Facts.Normalized()
	failure := trigger.failure.Clone()
	failure.Class = runtimecontract.FailureClassCandidateInfeasible
	failure.Code = "repair_surface_unavailable"
	failure.Message = fmt.Sprintf(
		"validation failure repair route %q requires a structured owner target observation for existing scope",
		trigger.route.ID,
	)
	failure.TargetRefs = nil
	failure.TargetObservation = nil
	if err := replaceRuntimeFailureAuthority(&facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
		return false, err
	}
	facts.Resources.TargetSelection = nil
	facts.Resources.TargetSnapshots = nil
	aggregate.Facts = facts.Normalized()
	return true, nil
}

// markRuntimeValidationFailureRepairRouteUnavailable is the private SM-2
// terminal transition for a complete owner diagnostic which is intentionally
// outside the Profile's exact repair table. Once the owner has classified the
// same deterministic validation failure, re-running that validation cannot
// discover a different repair authority. Failing in the receipt CAS prevents
// SM-4 from dispatching an identical validation or asking an LLM to invent a
// cross-owner repair.
func markRuntimeValidationFailureRepairRouteUnavailable(aggregate *RuntimeAggregate, fact runtimecontract.EvidenceFact, operation runtimecontract.RuntimeOperation) (bool, error) {
	if aggregate == nil {
		return false, common.ErrInvalidArgument
	}
	facts := aggregate.Facts.Normalized()
	operation = operation.Normalized()
	if aggregate.ActiveDispatch != nil || fact.Kind != runtimecontract.EvidenceKindValidation || facts.Failure == nil ||
		facts.Resources.FailureTarget != nil || needsStateChangeForOwnerGroundedFailure(facts) || len(facts.Focus.CoverageWorkItemIDs) != 1 {
		return false, nil
	}
	coverageID := facts.Focus.CoverageWorkItemIDs[0]
	binding, bindingFound := coverageEvidenceBindingForCoverage(facts.Focus, coverageID)
	if !bindingFound || !contains(operation.BindingRefs, binding.ID) || !contains(fact.BindingRefs, binding.ID) {
		return false, nil
	}
	declaration, found, err := runtimeDefinitionFocusForCoverage(facts.DefinitionFocuses, coverageID)
	if err != nil || !found || declaration.ValidationFailureRepairConvergence == nil {
		return false, err
	}
	convergence := declaration.ValidationFailureRepairConvergence.Normalized()
	if err := convergence.Validate(); err != nil {
		return false, err
	}
	failure := facts.Failure.Clone()
	if failure.ValidationDiagnostic == nil || failure.OriginalDiagnostic == nil {
		return false, nil
	}
	diagnostic := failure.ValidationDiagnostic.Normalized()
	original := failure.OriginalDiagnostic.Normalized()
	if original == nil || original.OperationID != fact.OperationID || operation.ID != fact.OperationID || fact.FailureSignature == "" ||
		failure.Signature == "" || fact.FailureSignature != failure.Signature {
		return false, nil
	}
	trigger := runtimecontract.RuntimeValidationFailureRepairGenerationInput{
		CoverageWorkItemID:         coverageID,
		FailedFocusID:              declaration.ID,
		FailureSignature:           failure.Signature,
		TriggerOwnerModuleID:       original.OwnerModuleID,
		TriggerOperationKind:       operation.Kind,
		TriggerDiagnosticClass:     diagnostic.Class,
		TriggerEvidenceTargetType:  diagnostic.RecommendedEvidenceTargetType,
		TriggerOwnerDiagnosticCode: diagnostic.OwnerCode,
	}.Normalized()
	if err := trigger.Validate(); err != nil {
		// An incomplete diagnostic is not silently promoted into a different
		// terminal fact; the ordinary bounded failure policy remains responsible.
		return false, nil
	}
	if route, found := convergence.RouteForTrigger(
		trigger.TriggerOwnerModuleID,
		trigger.TriggerOperationKind,
		trigger.TriggerDiagnosticClass,
		trigger.TriggerEvidenceTargetType,
		trigger.TriggerOwnerDiagnosticCode,
	); found && (!route.RequireInputAddition || diagnostic.InputAddition.MatchesBinding(binding)) {
		return false, nil
	}

	repair := facts.Repair.Normalized()
	if repair.FocusID != facts.Focus.ID {
		repair = runtimecontract.RuntimeRepairLedger{FocusID: facts.Focus.ID, SliceID: facts.Focus.SliceID}
	}
	repair.Exhausted = true
	facts.Repair = repair.Normalized()
	failure.Class = runtimecontract.FailureClassNoProgressExhausted
	failure.Code = "validation_failure_repair_route_unavailable"
	failure.Message = fmt.Sprintf(
		"validation failure repair convergence %q has no exact route for owner=%q operation=%q diagnostic=%q targetType=%q ownerCode=%q",
		convergence.ID,
		trigger.TriggerOwnerModuleID,
		trigger.TriggerOperationKind,
		trigger.TriggerDiagnosticClass,
		trigger.TriggerEvidenceTargetType,
		trigger.TriggerOwnerDiagnosticCode,
	)
	if err := replaceRuntimeFailureAuthority(&facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
		return false, err
	}
	aggregate.Facts = facts.Normalized()
	return true, nil
}

func runtimeDefinitionFocusForCoverage(values []runtimecontract.RuntimeDefinitionFocus, coverageID common.CoverageWorkItemID) (runtimecontract.RuntimeDefinitionFocus, bool, error) {
	var ret runtimecontract.RuntimeDefinitionFocus
	found := false
	for _, raw := range values {
		value := raw.Normalized()
		if !coverageIDsIntersect(value.CoverageWorkItemIDs, []common.CoverageWorkItemID{coverageID}) {
			continue
		}
		if found {
			return runtimecontract.RuntimeDefinitionFocus{}, false, fmt.Errorf("%w: coverage %q has multiple Definition focus declarations", common.ErrConflict, coverageID)
		}
		ret, found = value, true
	}
	return ret, found, nil
}

func runtimeValidationFailureRepairPendingDirective(facts runtimecontract.RuntimeFacts, trigger runtimeValidationFailureRepairTrigger) runtimecontract.RuntimeRepairDirective {
	failure := trigger.failure.Clone()
	convergenceHash := runtimecontract.StableRuntimeValidationFailureRepairConvergenceHash(trigger.convergence)
	return runtimecontract.RuntimeRepairDirective{
		ID:                         fmt.Sprintf("validation-failure-repair/%s/%d/%s", facts.Focus.ID, facts.Repair.RepairAttempts, convergenceHash),
		FocusID:                    facts.Focus.ID,
		Trigger:                    runtimecontract.RuntimeRepairTriggerRejectedValidation,
		CurrentFailure:             &failure,
		RequiredState:              runtimecontract.ConvergenceStateNeedStateChange,
		RequiredEffectScopes:       []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		RequiredEvidenceKinds:      []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds:      append([]string(nil), trigger.route.RepairOperationKinds...),
		NoProgressOperationShapes:  append([]runtimecontract.RuntimeRejectedOperationShape(nil), facts.Repair.NoProgressOperationShapes...),
		CoverageWorkItemIDs:        []common.CoverageWorkItemID{trigger.coverageID},
		SupersedesEvidenceRevision: trigger.fact.Revision,
		RepairAttempt:              facts.Repair.RepairAttempts,
	}.Normalized()
}

func runtimeValidationFailureRepairAwaitsSemanticBinding(facts runtimecontract.RuntimeFacts) bool {
	facts = facts.Normalized()
	if facts.RepairDirective == nil || facts.RepairDirective.FocusID != facts.Focus.ID ||
		facts.RepairDirective.Trigger != runtimecontract.RuntimeRepairTriggerRejectedValidation ||
		facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedStateChange ||
		facts.RepairDirective.Closure != nil || len(facts.RepairDirective.TargetRefs) != 0 ||
		len(facts.Focus.CoverageWorkItemIDs) != 1 {
		return false
	}
	index := runtimeCWICoverageIndex(facts.Coverage, facts.Focus.CoverageWorkItemIDs[0])
	return index >= 0 && facts.Coverage[index].CurrentBindingRef == nil &&
		facts.Coverage[index].CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStateReopened &&
		runtimeValidationFailureRepairReopenReason(facts.Coverage[index]) == runtimeCWIReopenReasonValidationFailureRepairRequired
}

// runtimeValidationFailureRepairOwnsSealedDirective recognizes only the
// state-change continuation already sealed by the repair-purpose semantic
// transaction. Generic repair synchronization must not replace this directive
// after a target-snapshot interlude: its binding, exact prospective target and
// ordered fresh-validation closure are the SM-2 authority being resumed.
func runtimeValidationFailureRepairOwnsSealedDirective(facts runtimecontract.RuntimeFacts) bool {
	facts = facts.Normalized()
	if facts.RepairDirective == nil || len(facts.Focus.CoverageWorkItemIDs) != 1 {
		return false
	}
	directive := facts.RepairDirective.Normalized()
	coverageID := facts.Focus.CoverageWorkItemIDs[0]
	if directive.FocusID != facts.Focus.ID || directive.Trigger != runtimecontract.RuntimeRepairTriggerRejectedValidation ||
		directive.RequiredState != runtimecontract.ConvergenceStateNeedStateChange || directive.Closure == nil ||
		len(directive.TargetRefs) == 0 || len(directive.CoverageWorkItemIDs) != 1 || directive.CoverageWorkItemIDs[0] != coverageID {
		return false
	}
	index := runtimeCWICoverageIndex(facts.Coverage, coverageID)
	if index < 0 || facts.Coverage[index].CurrentBindingRef == nil ||
		facts.Coverage[index].CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound ||
		runtimeValidationFailureRepairReopenReason(facts.Coverage[index]) != runtimeCWIReopenReasonValidationFailureRepairRequired {
		return false
	}
	binding, found := coverageEvidenceBindingForCoverage(facts.Focus, coverageID)
	if !found || !sameRuntimeTargetRefs(binding.ProspectiveTargetRefs, directive.TargetRefs) ||
		!contains(binding.PrerequisiteRefs, "semantic-purpose/"+string(runtimecontract.RuntimeSemanticEvidenceObligationValidationFailureRepair)) {
		return false
	}
	closure := directive.Closure.Normalized()
	return closure.RequireValidationAfterStateChange &&
		sameRuntimeTargetRefs(closure.GroundedTargetRefs, binding.ProspectiveTargetRefs) &&
		len(closure.CoverageWorkItemIDs) == 1 && closure.CoverageWorkItemIDs[0] == coverageID &&
		contains(closure.RequiredEvidenceKinds, runtimecontract.EvidenceKindStateChange) &&
		contains(closure.RequiredEvidenceKinds, runtimecontract.EvidenceKindValidation)
}

// runtimeValidationFailureRepairOwnsFreshValidationDirective recognizes the
// Definition-proof successor of the repair binding. Its empty TargetRefs is
// intentional: the repair closure audits the changed targets, while the new
// Definition binding independently authorizes the validation surface. A
// prerequisite interlude must not copy the closure targets back into this
// action-scoped field.
func runtimeValidationFailureRepairOwnsFreshValidationDirective(facts runtimecontract.RuntimeFacts) bool {
	facts = facts.Normalized()
	if facts.RepairDirective == nil || len(facts.Focus.CoverageWorkItemIDs) != 1 {
		return false
	}
	directive := facts.RepairDirective.Normalized()
	coverageID := facts.Focus.CoverageWorkItemIDs[0]
	if directive.FocusID != facts.Focus.ID || directive.Trigger != runtimecontract.RuntimeRepairTriggerRejectedValidation ||
		directive.RequiredState != runtimecontract.ConvergenceStateNeedValidation || directive.Closure == nil ||
		len(directive.TargetRefs) != 0 || len(directive.CoverageWorkItemIDs) != 1 || directive.CoverageWorkItemIDs[0] != coverageID {
		return false
	}
	index := runtimeCWICoverageIndex(facts.Coverage, coverageID)
	if index < 0 || facts.Coverage[index].CurrentBindingRef == nil ||
		facts.Coverage[index].CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound ||
		facts.Coverage[index].State == runtimecontract.RuntimeCoverageItemStateSatisfied ||
		runtimeValidationFailureRepairReopenReason(facts.Coverage[index]) != runtimeCWIReopenReasonValidationFailureRepairSettled {
		return false
	}
	binding, found := coverageEvidenceBindingForCoverage(facts.Focus, coverageID)
	if !found || contains(binding.PrerequisiteRefs, "semantic-purpose/"+string(runtimecontract.RuntimeSemanticEvidenceObligationValidationFailureRepair)) {
		return false
	}
	if _, found := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindValidation); !found {
		return false
	}
	closure := directive.Closure.Normalized()
	return closure.RequireValidationAfterStateChange && len(closure.GroundedTargetRefs) != 0 &&
		len(closure.CoverageWorkItemIDs) == 1 && closure.CoverageWorkItemIDs[0] == coverageID &&
		contains(closure.RequiredEvidenceKinds, runtimecontract.EvidenceKindStateChange) &&
		contains(closure.RequiredEvidenceKinds, runtimecontract.EvidenceKindValidation)
}

// CurrentRuntimeValidationFailureRepairGenerationInput returns the one
// persisted repair-purpose semantic input selected by SM-2. The SM-4 drive may
// use this detached value only to call the pure shared compiler; it cannot
// mutate the CWI, choose an operation or infer a target from diagnostics.
func CurrentRuntimeValidationFailureRepairGenerationInput(aggregate RuntimeAggregate) (runtimecontract.RuntimeValidationFailureRepairGenerationInput, bool, error) {
	facts := aggregate.Facts.Normalized()
	if len(facts.Focus.CoverageWorkItemIDs) != 1 || facts.RepairDirective == nil ||
		facts.RepairDirective.Trigger != runtimecontract.RuntimeRepairTriggerRejectedValidation ||
		facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedStateChange ||
		facts.RepairDirective.Closure != nil || len(facts.RepairDirective.TargetRefs) != 0 ||
		facts.Resources.FailureTarget != nil || facts.Failure == nil {
		return runtimecontract.RuntimeValidationFailureRepairGenerationInput{}, false, nil
	}
	coverageID := facts.Focus.CoverageWorkItemIDs[0]
	index := runtimeCWICoverageIndex(facts.Coverage, coverageID)
	if index < 0 {
		return runtimecontract.RuntimeValidationFailureRepairGenerationInput{}, false, fmt.Errorf("%w: validation failure repair generation references unknown CWI", common.ErrConflict)
	}
	item := facts.Coverage[index]
	if item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateReopened || item.CurrentBindingRef != nil ||
		runtimeValidationFailureRepairReopenReason(item) != runtimeCWIReopenReasonValidationFailureRepairRequired {
		return runtimecontract.RuntimeValidationFailureRepairGenerationInput{}, false, nil
	}
	lineage := item.ReopenLineage[len(item.ReopenLineage)-1].Normalized()
	var rejected *runtimecontract.EvidenceFact
	for idx := range facts.Evidence.Rejected {
		if string(facts.Evidence.Rejected[idx].ID) != lineage.TriggerRef {
			continue
		}
		value := facts.Evidence.Rejected[idx]
		if rejected != nil {
			return runtimecontract.RuntimeValidationFailureRepairGenerationInput{}, false, fmt.Errorf("%w: validation failure repair trigger fact is duplicated", common.ErrConflict)
		}
		rejected = &value
	}
	if rejected == nil {
		return runtimecontract.RuntimeValidationFailureRepairGenerationInput{}, false, fmt.Errorf("%w: validation failure repair trigger fact is absent", common.ErrConflict)
	}
	operation, found := runtimeOperationForEvidence(aggregate, *rejected)
	if !found {
		return runtimecontract.RuntimeValidationFailureRepairGenerationInput{}, false, fmt.Errorf("%w: validation failure repair trigger operation is absent from dispatch lineage", common.ErrConflict)
	}
	declaration, found, err := runtimeDefinitionFocusForCoverage(facts.DefinitionFocuses, coverageID)
	if err != nil || !found || declaration.ValidationFailureRepairConvergence == nil {
		return runtimecontract.RuntimeValidationFailureRepairGenerationInput{}, false, err
	}
	trigger := runtimeValidationFailureRepairTrigger{
		coverageID: coverageID, fact: *rejected, failure: facts.Failure.Clone(), operation: operation,
		convergence: declaration.ValidationFailureRepairConvergence.Clone(),
	}
	diagnostic := trigger.failure.ValidationDiagnostic.Normalized()
	original := trigger.failure.OriginalDiagnostic.Normalized()
	if original == nil {
		return runtimecontract.RuntimeValidationFailureRepairGenerationInput{}, false, fmt.Errorf("%w: persisted validation failure repair trigger lacks owner diagnostic", common.ErrConflict)
	}
	var binding runtimecontract.CoverageEvidenceBinding
	for _, dispatch := range aggregate.CompletedDispatches {
		if dispatch.ID == rejected.DispatchID && len(rejected.BindingRefs) == 1 {
			binding, _ = runtimecontract.CoverageEvidenceBindingForRef(dispatch.CoverageBindings, rejected.BindingRefs[0])
		}
	}
	route, routeFound := trigger.convergence.RouteForTrigger(
		original.OwnerModuleID, operation.Kind, diagnostic.Class, diagnostic.RecommendedEvidenceTargetType, diagnostic.OwnerCode,
	)
	if !routeFound || (route.RequireInputAddition && !diagnostic.InputAddition.MatchesBinding(binding)) {
		return runtimecontract.RuntimeValidationFailureRepairGenerationInput{}, false, fmt.Errorf("%w: persisted validation failure repair trigger has no exact Profile route", common.ErrConflict)
	}
	trigger.route = route
	if err := trigger.validate(); err != nil {
		return runtimecontract.RuntimeValidationFailureRepairGenerationInput{}, false, fmt.Errorf("%w: persisted validation failure repair trigger is invalid: %v", common.ErrConflict, err)
	}
	if facts.Repair.Exhausted || facts.Repair.RepairAttempts < 1 || facts.Repair.RepairAttempts > trigger.convergence.MaxAttempts ||
		facts.RepairDirective.SupersedesEvidenceRevision != rejected.Revision ||
		!sameRuntimeStringSet(facts.RepairDirective.AllowedOperationKinds, trigger.route.RepairOperationKinds) {
		return runtimecontract.RuntimeValidationFailureRepairGenerationInput{}, false, fmt.Errorf("%w: validation failure repair generation exceeds or conflicts with its sealed budget", common.ErrConflict)
	}
	input := runtimecontract.RuntimeValidationFailureRepairGenerationInput{
		CoverageWorkItemID: coverageID,
		// The shared compiler intentionally binds this identity to the immutable
		// Definition declaration, not the retired generation-specific focus ID.
		FailedFocusID:              declaration.ID,
		FailureSignature:           trigger.failure.Signature,
		TriggerOwnerModuleID:       original.OwnerModuleID,
		TriggerOperationKind:       operation.Kind,
		TriggerDiagnosticClass:     diagnostic.Class,
		TriggerEvidenceTargetType:  diagnostic.RecommendedEvidenceTargetType,
		TriggerOwnerDiagnosticCode: diagnostic.OwnerCode,
	}.Normalized()
	if trigger.route.RequireInputAddition {
		input.InputAddition = diagnostic.InputAddition
	}
	if err := input.Validate(); err != nil {
		return runtimecontract.RuntimeValidationFailureRepairGenerationInput{}, false, err
	}
	return input, true, nil
}

func runtimeValidationFailureRepairContinuationForSemanticSeal(aggregate RuntimeAggregate, manifest runtimecontract.RuntimeSemanticPlanManifest) (runtimeValidationFailureRepairSealContinuation, bool, error) {
	facts := aggregate.Facts.Normalized()
	manifest = manifest.Normalized()
	if len(facts.Focus.CoverageWorkItemIDs) != 1 || facts.RepairDirective == nil {
		return runtimeValidationFailureRepairSealContinuation{}, false, nil
	}
	coverageID := facts.Focus.CoverageWorkItemIDs[0]
	index := runtimeCWICoverageIndex(facts.Coverage, coverageID)
	if index < 0 || facts.Coverage[index].CurrentBindingRef != nil || facts.Coverage[index].CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateReopened {
		return runtimeValidationFailureRepairSealContinuation{}, false, nil
	}
	reason := runtimeValidationFailureRepairReopenReason(facts.Coverage[index])
	kind := ""
	switch reason {
	case runtimeCWIReopenReasonValidationFailureRepairRequired:
		kind = runtimeValidationFailureRepairSealRepair
	case runtimeCWIReopenReasonValidationFailureRepairSettled:
		kind = runtimeValidationFailureRepairSealFreshValidation
	default:
		return runtimeValidationFailureRepairSealContinuation{}, false, nil
	}
	obligation, found, err := runtimeSemanticObligationForCoverage(manifest, coverageID)
	if err != nil || !found {
		return runtimeValidationFailureRepairSealContinuation{}, false, err
	}
	purpose, err := runtimecontract.ResolveRuntimeSemanticEvidenceObligation(obligation)
	if err != nil {
		return runtimeValidationFailureRepairSealContinuation{}, false, err
	}
	if kind == runtimeValidationFailureRepairSealRepair {
		if purpose.Purpose != runtimecontract.RuntimeSemanticEvidenceObligationValidationFailureRepair || obligation.ValidationFailureRepair == nil ||
			facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedStateChange || facts.RepairDirective.Closure != nil {
			return runtimeValidationFailureRepairSealContinuation{}, false, fmt.Errorf("%w: repair semantic seal does not match its persisted pending directive", common.ErrConflict)
		}
	} else if purpose.Purpose != runtimecontract.RuntimeSemanticEvidenceObligationDefinitionProof ||
		facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedValidation || facts.RepairDirective.Closure == nil {
		return runtimeValidationFailureRepairSealContinuation{}, false, fmt.Errorf("%w: fresh-validation semantic seal does not restore Definition proof", common.ErrConflict)
	}
	continuation := runtimeValidationFailureRepairSealContinuation{
		kind: kind, coverageID: coverageID, repair: facts.Repair.Normalized(), directive: facts.RepairDirective.Clone(),
	}
	if facts.Failure != nil {
		failure := facts.Failure.Clone()
		continuation.failure = &failure
	} else if continuation.directive.CurrentFailure != nil {
		failure := continuation.directive.CurrentFailure.Clone()
		continuation.failure = &failure
	}
	return continuation, true, nil
}

func applyRuntimeValidationFailureRepairSemanticSeal(facts runtimecontract.RuntimeFacts, manifest runtimecontract.RuntimeSemanticPlanManifest, continuation runtimeValidationFailureRepairSealContinuation) (runtimecontract.RuntimeFacts, error) {
	facts = facts.Normalized()
	manifest = manifest.Normalized()
	if continuation.coverageID == "" || len(facts.Focus.CoverageWorkItemIDs) != 1 || facts.Focus.CoverageWorkItemIDs[0] != continuation.coverageID {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("%w: validation failure repair semantic seal did not retain current CWI", common.ErrConflict)
	}
	index := runtimeCWICoverageIndex(facts.Coverage, continuation.coverageID)
	if index < 0 || facts.Coverage[index].CurrentBindingRef == nil || facts.Coverage[index].CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("%w: validation failure repair semantic seal has no current binding", common.ErrConflict)
	}
	binding, found := coverageEvidenceBindingForCoverage(facts.Focus, continuation.coverageID)
	if !found {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("%w: validation failure repair semantic seal omitted its focus binding", common.ErrConflict)
	}
	obligation, found, err := runtimeSemanticObligationForCoverage(manifest, continuation.coverageID)
	if err != nil || !found {
		return runtimecontract.RuntimeFacts{}, err
	}
	purpose, err := runtimecontract.ResolveRuntimeSemanticEvidenceObligation(obligation)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	directive := continuation.directive.Clone()
	directive.FocusID = facts.Focus.ID
	directive.CoverageWorkItemIDs = []common.CoverageWorkItemID{continuation.coverageID}
	directive.CurrentFailure = continuation.failure
	continuation.repair.FocusID = facts.Focus.ID
	continuation.repair.SliceID = facts.Focus.SliceID
	facts.Repair = continuation.repair.Normalized()
	if continuation.failure != nil {
		if err := replaceRuntimeFailureAuthority(&facts, *continuation.failure, runtimeFailureAuthorityRetainInterlude, nil); err != nil {
			return runtimecontract.RuntimeFacts{}, err
		}
	}
	facts.Feasibility = nil

	switch continuation.kind {
	case runtimeValidationFailureRepairSealRepair:
		if purpose.Purpose != runtimecontract.RuntimeSemanticEvidenceObligationValidationFailureRepair || obligation.ValidationFailureRepair == nil {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("%w: validation failure repair binding has wrong semantic purpose", common.ErrConflict)
		}
		repair := obligation.ValidationFailureRepair.Normalized()
		declaration, definitionFound, definitionErr := runtimeDefinitionFocusForCoverage(facts.DefinitionFocuses, continuation.coverageID)
		if definitionErr != nil || !definitionFound || declaration.ValidationFailureRepairConvergence == nil {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("%w: validation failure repair semantic seal has no Definition convergence", common.ErrConflict)
		}
		convergence := declaration.ValidationFailureRepairConvergence.Normalized()
		if runtimecontract.StableRuntimeValidationFailureRepairConvergenceHash(convergence) != repair.ConvergenceHash {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("%w: validation failure repair semantic seal convergence drifted from Definition", common.ErrConflict)
		}
		routeMatched := false
		for _, route := range convergence.Routes {
			if route.ID == repair.RouteID && runtimecontract.StableRuntimeValidationFailureRepairRouteHash(route) == repair.RouteHash {
				routeMatched = true
				break
			}
		}
		if !routeMatched {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("%w: validation failure repair semantic seal route drifted from Definition", common.ErrConflict)
		}
		if facts.Repair.Exhausted || facts.Repair.RepairAttempts < 1 || facts.Repair.RepairAttempts > repair.MaxAttempts ||
			len(binding.ProspectiveTargetRefs) == 0 {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("%w: validation failure repair binding exceeds budget or lacks exact prospective targets", common.ErrConflict)
		}
		proof, found := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindStateChange)
		if !found || !runtimeStringsWithin(proof.AllowedOperationKinds, repair.RepairOperationKinds) {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("%w: validation failure repair binding operations exceed sealed convergence", common.ErrConflict)
		}
		directive.ID = fmt.Sprintf("validation-failure-repair/%s/%d/%s", facts.Focus.ID, facts.Repair.RepairAttempts, repair.ConvergenceHash)
		directive.RequiredState = runtimecontract.ConvergenceStateNeedStateChange
		directive.RequiredEffectScopes = append([]runtimecontract.RuntimeEffectScope(nil), proof.RequiredEffectScopes...)
		directive.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
		directive.AllowedOperationKinds = append([]string(nil), proof.AllowedOperationKinds...)
		directive.TargetRefs = append([]string(nil), binding.ProspectiveTargetRefs...)
		closure := runtimecontract.RuntimeRepairClosure{
			GapSignature:                      repair.FailureSignature,
			BaselineEvidenceRevision:          directive.SupersedesEvidenceRevision,
			CoverageWorkItemIDs:               []common.CoverageWorkItemID{continuation.coverageID},
			RequiredEvidenceKinds:             []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
			GroundedTargetRefs:                append([]string(nil), binding.ProspectiveTargetRefs...),
			RequireValidationAfterStateChange: repair.RequireFreshValidation,
		}
		closure.ID = runtimecontract.StableRuntimeRepairClosureID(closure)
		directive.Closure = &closure
	case runtimeValidationFailureRepairSealFreshValidation:
		if purpose.Purpose != runtimecontract.RuntimeSemanticEvidenceObligationDefinitionProof {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("%w: repair successor binding is not Definition proof", common.ErrConflict)
		}
		proof, found := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindValidation)
		if !found {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("%w: repair successor Definition proof has no fresh validation", common.ErrConflict)
		}
		directive.ID = fmt.Sprintf("validation-failure-repair/%s/%d/fresh-validation", facts.Focus.ID, facts.Repair.RepairAttempts)
		// The repair-purpose commitment applies only to the durable state-change
		// effect. Fresh validation preserves the closure lineage but is an
		// ordinary Definition-proof operation.
		directive.Purpose = ""
		directive.RequiredState = runtimecontract.ConvergenceStateNeedValidation
		directive.RequiredEffectScopes = append([]runtimecontract.RuntimeEffectScope(nil), proof.RequiredEffectScopes...)
		directive.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
		directive.AllowedOperationKinds = append([]string(nil), proof.AllowedOperationKinds...)
		// Repair and terminal validation surfaces are independently sealed. The
		// immutable closure retains the repaired targets for audit, while the
		// Definition binding supplies the validation target authority.
		directive.TargetRefs = nil
	default:
		return runtimecontract.RuntimeFacts{}, common.ErrInvalidArgument
	}
	directive = directive.Normalized()
	if err := directive.Validate(); err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	facts.RepairDirective = &directive
	return facts.Normalized(), nil
}

func runtimeSemanticObligationForCoverage(manifest runtimecontract.RuntimeSemanticPlanManifest, coverageID common.CoverageWorkItemID) (runtimecontract.RuntimeSemanticProofObligation, bool, error) {
	var ret runtimecontract.RuntimeSemanticProofObligation
	found := false
	for _, raw := range manifest.Normalized().ProofObligations {
		if raw.CoverageWorkItemID != coverageID {
			continue
		}
		if found {
			return runtimecontract.RuntimeSemanticProofObligation{}, false, fmt.Errorf("%w: semantic manifest duplicates coverage %q", common.ErrConflict, coverageID)
		}
		ret, found = raw.Clone(), true
	}
	return ret, found, nil
}

func runtimeStringsWithin(values, allowed []string) bool {
	for _, value := range values {
		if !contains(allowed, value) {
			return false
		}
	}
	return true
}

func markRuntimeValidationFailureRepairExhausted(facts *runtimecontract.RuntimeFacts, trigger runtimeValidationFailureRepairTrigger) error {
	if facts == nil {
		return common.ErrInvalidArgument
	}
	facts.Repair.Exhausted = true
	failure := trigger.failure.Clone()
	failure.Class = runtimecontract.FailureClassNoProgressExhausted
	failure.Code = "validation_failure_repair_exhausted"
	failure.Message = fmt.Sprintf("validation failure repair convergence %q exhausted its bounded %d attempts", trigger.convergence.ID, trigger.convergence.MaxAttempts)
	return replaceRuntimeFailureAuthority(facts, failure, runtimeFailureAuthorityRetire, nil)
}

// currentRuntimeSemanticValidationFailureRepair resolves purpose exclusively
// from the current CWI binding's manifest hash. Active and immutable retired
// semantic generations are lineage sources; neither is reconstructed from the
// mutable focus projection or Profile text.
func currentRuntimeSemanticValidationFailureRepair(aggregate RuntimeAggregate) (runtimecontract.RuntimeSemanticValidationFailureRepair, bool, error) {
	focus := aggregate.Facts.Focus.Normalized()
	if len(focus.CoverageWorkItemIDs) != 1 {
		return runtimecontract.RuntimeSemanticValidationFailureRepair{}, false, nil
	}
	coverageID := focus.CoverageWorkItemIDs[0]
	index := runtimeCWICoverageIndex(aggregate.Facts.Coverage, coverageID)
	if index < 0 || aggregate.Facts.Coverage[index].CurrentBindingRef == nil {
		return runtimecontract.RuntimeSemanticValidationFailureRepair{}, false, nil
	}
	ref := aggregate.Facts.Coverage[index].CurrentBindingRef.Clone()
	if err := ref.Validate(); err != nil {
		return runtimecontract.RuntimeSemanticValidationFailureRepair{}, false, fmt.Errorf("%w: validation failure repair binding: %v", common.ErrConflict, err)
	}
	if ref.FocusID != focus.ID {
		return runtimecontract.RuntimeSemanticValidationFailureRepair{}, false, fmt.Errorf("%w: validation failure repair binding focus is not current", common.ErrConflict)
	}
	binding, found := coverageEvidenceBindingForCoverage(focus, coverageID)
	if !found || binding.ID != ref.BindingID || binding.Hash != ref.BindingHash {
		return runtimecontract.RuntimeSemanticValidationFailureRepair{}, false, fmt.Errorf("%w: validation failure repair CWI binding conflicts with focus", common.ErrConflict)
	}

	manifestMatches := 0
	var purpose runtimecontract.RuntimeSemanticEvidenceObligationPurpose
	var repair runtimecontract.RuntimeSemanticValidationFailureRepair
	resolve := func(manifest runtimecontract.RuntimeSemanticPlanManifest) error {
		manifest = manifest.Normalized()
		if manifest.Hash != ref.SemanticManifestHash {
			return nil
		}
		if err := manifest.Validate(); err != nil {
			return fmt.Errorf("%w: validation failure repair manifest: %v", common.ErrConflict, err)
		}
		var matched *runtimecontract.RuntimeSemanticProofObligation
		for idx := range manifest.ProofObligations {
			candidate := manifest.ProofObligations[idx].Clone()
			if candidate.CoverageWorkItemID != coverageID {
				continue
			}
			if matched != nil {
				return fmt.Errorf("%w: validation failure repair manifest duplicates current CWI", common.ErrConflict)
			}
			matched = &candidate
		}
		if matched == nil {
			return fmt.Errorf("%w: validation failure repair manifest omits current CWI", common.ErrConflict)
		}
		resolved, err := runtimecontract.ResolveRuntimeSemanticEvidenceObligation(*matched)
		if err != nil {
			return err
		}
		if manifestMatches != 0 && purpose != resolved.Purpose {
			return fmt.Errorf("%w: validation failure repair purpose conflicts across semantic lineage", common.ErrConflict)
		}
		manifestMatches++
		purpose = resolved.Purpose
		if resolved.Purpose == runtimecontract.RuntimeSemanticEvidenceObligationValidationFailureRepair {
			if matched.ValidationFailureRepair == nil {
				return fmt.Errorf("%w: validation failure repair purpose has no typed contract", common.ErrConflict)
			}
			repair = matched.ValidationFailureRepair.Clone()
		}
		return nil
	}
	ledger := aggregate.SemanticPlanning.Normalized()
	if ledger.Manifest != nil {
		if err := resolve(*ledger.Manifest); err != nil {
			return runtimecontract.RuntimeSemanticValidationFailureRepair{}, false, err
		}
	}
	for _, generation := range ledger.History {
		if generation.Snapshot.Manifest == nil || generation.Snapshot.Manifest.Hash != ref.SemanticManifestHash {
			continue
		}
		if err := generation.Validate(); err != nil {
			return runtimecontract.RuntimeSemanticValidationFailureRepair{}, false, fmt.Errorf("%w: validation failure repair retired generation: %v", common.ErrConflict, err)
		}
		if err := resolve(*generation.Snapshot.Manifest); err != nil {
			return runtimecontract.RuntimeSemanticValidationFailureRepair{}, false, err
		}
	}
	if manifestMatches == 0 {
		return runtimecontract.RuntimeSemanticValidationFailureRepair{}, false, fmt.Errorf("%w: current CWI binding has no semantic purpose lineage", common.ErrConflict)
	}
	if purpose != runtimecontract.RuntimeSemanticEvidenceObligationValidationFailureRepair {
		return runtimecontract.RuntimeSemanticValidationFailureRepair{}, false, nil
	}
	if err := repair.Validate(); err != nil {
		return runtimecontract.RuntimeSemanticValidationFailureRepair{}, false, err
	}
	return repair, true, nil
}

// settleRuntimeValidationFailureRepair runs after the immutable repair
// dispatch is complete but inside the same evidence CAS. It withholds terminal
// credit, retires the repair binding and opens the original Definition proof
// for a fresh validation. The accepted receipt remains audit/proof lineage.
func settleRuntimeValidationFailureRepair(
	aggregate *RuntimeAggregate,
	fact runtimecontract.EvidenceFact,
	repair runtimecontract.RuntimeSemanticValidationFailureRepair,
	sourceDirectives ...*runtimecontract.RuntimeRepairDirective,
) (bool, error) {
	if aggregate == nil || aggregate.ActiveDispatch != nil {
		return false, common.ErrInvalidArgument
	}
	repair = repair.Normalized()
	if err := repair.Validate(); err != nil {
		return false, err
	}
	if fact.Kind != runtimecontract.EvidenceKindStateChange || weakRuntimeEvidence(fact) || len(fact.CoverageWorkItemIDs) != 1 {
		// A binding mismatch remains a deterministic receipt failure handled by
		// the ordinary evidence gate; it must not partially reopen this CWI.
		return false, nil
	}
	facts := aggregate.Facts.Normalized()
	if len(facts.Focus.CoverageWorkItemIDs) != 1 || facts.Focus.CoverageWorkItemIDs[0] != fact.CoverageWorkItemIDs[0] {
		return false, nil
	}
	coverageID := facts.Focus.CoverageWorkItemIDs[0]
	index := runtimeCWICoverageIndex(facts.Coverage, coverageID)
	if index < 0 || facts.Coverage[index].CurrentBindingRef == nil || facts.Coverage[index].CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound ||
		facts.Coverage[index].State == runtimecontract.RuntimeCoverageItemStateSatisfied {
		return false, fmt.Errorf("%w: validation failure repair receipt has no current open binding", common.ErrConflict)
	}
	binding, found := coverageEvidenceBindingForCoverage(facts.Focus, coverageID)
	if !found || len(binding.ProspectiveTargetRefs) == 0 {
		return false, fmt.Errorf("%w: validation failure repair receipt has no exact prospective target binding", common.ErrConflict)
	}
	if len(binding.ProspectiveTargetRefs) == 1 {
		if !sameRuntimeTargetRefs(binding.ProspectiveTargetRefs, fact.TargetRefs) {
			return false, fmt.Errorf("%w: validation failure repair receipt does not match its prospective target", common.ErrConflict)
		}
	} else {
		dispatchID, complete := runtimeProspectiveCoverageKindComplete(
			*aggregate,
			binding,
			coverageID,
			runtimecontract.EvidenceKindStateChange,
			aggregate.Facts.Evidence.Accepted,
		)
		if !complete || dispatchID != fact.DispatchID {
			return false, fmt.Errorf("%w: validation failure repair dispatch lacks its complete prospective all-of receipt set", common.ErrConflict)
		}
	}
	operation, found := runtimeOperationForEvidence(*aggregate, fact)
	if !found || !contains(repair.RepairOperationKinds, runtimecontract.CanonicalRuntimeBlockKind(operation.Kind)) {
		return false, fmt.Errorf("%w: validation failure repair receipt operation is outside sealed repair kinds", common.ErrConflict)
	}
	if len(sourceDirectives) > 1 {
		return false, common.ErrInvalidArgument
	}
	sourceDirective := facts.RepairDirective
	if len(sourceDirectives) == 1 && sourceDirectives[0] != nil {
		value := sourceDirectives[0].Clone()
		sourceDirective = &value
	}
	if sourceDirective == nil || sourceDirective.Trigger != runtimecontract.RuntimeRepairTriggerRejectedValidation ||
		sourceDirective.RequiredState != runtimecontract.ConvergenceStateNeedStateChange || sourceDirective.Closure == nil ||
		!sameRuntimeTargetRefs(sourceDirective.TargetRefs, binding.ProspectiveTargetRefs) {
		return false, fmt.Errorf("%w: validation failure repair receipt has no exact persisted directive", common.ErrConflict)
	}
	if err := sourceDirective.Validate(); err != nil {
		return false, fmt.Errorf("%w: validation failure repair source directive: %v", common.ErrConflict, err)
	}

	item := &facts.Coverage[index]
	lineage := runtimecontract.RuntimeCWIReopenLineage{
		PreviousBindingRef: item.CurrentBindingRef.Clone(), TriggerRef: string(fact.ID),
		Reason: runtimeCWIReopenReasonValidationFailureRepairSettled,
	}.Normalized()
	if err := lineage.Validate(); err != nil {
		return false, err
	}
	if len(item.ReopenLineage) >= runtimecontract.RuntimeCWIReopenLineageLimit {
		return false, fmt.Errorf("runtime CWI reopen lineage exceeds bounded limit %d", runtimecontract.RuntimeCWIReopenLineageLimit)
	}
	item.ReopenLineage = append(item.ReopenLineage, lineage)
	item.CurrentBindingRef = nil
	item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateReopened
	item.State = runtimecontract.RuntimeCoverageItemStateOpen

	previousFocusID := facts.Focus.ID
	carriedRepair := facts.Repair.Normalized()
	carriedDirective := sourceDirective.Clone()
	var carriedFailure *runtimecontract.FailureFact
	if facts.Failure != nil {
		value := facts.Failure.Clone()
		carriedFailure = &value
	} else if carriedDirective.CurrentFailure != nil {
		value := carriedDirective.CurrentFailure.Clone()
		carriedFailure = &value
	}
	updated, err := replaceRuntimeCWIFocusProjectionForSemanticRebinding(facts, []common.CoverageWorkItemID{coverageID})
	if err != nil {
		return false, err
	}
	retireFocusScopedPrerequisites(&updated, previousFocusID)
	updated.Resources.TargetSelection = nil
	updated.Resources.TargetSnapshots = nil
	updated.Resources.FailureTarget = nil
	updated.Feasibility = nil
	updated.Failure = carriedFailure
	carriedRepair.FocusID = updated.Focus.ID
	carriedRepair.SliceID = updated.Focus.SliceID
	// The repair effect has closed the retired semantic generation, but it has
	// not yet proved the successor validation contract.  Do not carry the old
	// generation's progress watermark into that fresh-validation boundary: it
	// would let restart/accounting treat pre-repair evidence as successor
	// progress.  Repair attempts and no-progress history remain bounded on the
	// same cycle; only the generation-local progress marker is reset.
	carriedRepair.LastProgressRevision = ""
	carriedRepair.LastProgressAt = ""
	updated.Repair = carriedRepair.Normalized()
	carriedDirective.ID = fmt.Sprintf("validation-failure-repair/%s/%d/fresh-validation", updated.Focus.ID, updated.Repair.RepairAttempts)
	carriedDirective.FocusID = updated.Focus.ID
	// Retain the repair closure across phases, but never carry the state-change
	// repair-purpose commitment into its fresh-validation successor.
	carriedDirective.Purpose = ""
	carriedDirective.RequiredState = runtimecontract.ConvergenceStateNeedValidation
	carriedDirective.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession}
	carriedDirective.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	// This activation-sealed inventory is provisional while semantic authority
	// is pending. SM-3 cannot dispatch without a current binding; Definition
	// re-seal below narrows it to the exact proof kinds.
	carriedDirective.AllowedOperationKinds = append([]string(nil), updated.Capabilities.ValidationKinds...)
	carriedDirective.TargetRefs = nil
	carriedDirective.SupersedesEvidenceRevision = fact.Revision
	carriedDirective.RepairAttempt++
	updated.RepairDirective = &carriedDirective
	contract, err := canonicalRuntimeCWIReconciliationContract(updated)
	if err != nil {
		return false, err
	}
	updated.ReconciliationContract = contract
	aggregate.Facts = updated.Normalized()
	if _, err := supersedeActiveReconciliationForCurrentContract(aggregate); err != nil {
		return false, err
	}
	if err := retireSemanticPlanningGenerationForCWIReopen(aggregate, []common.CoverageWorkItemID{coverageID}); err != nil {
		return false, err
	}
	return true, nil
}

func runtimeValidationFailureRepairReopenReason(item runtimecontract.RuntimeCoverageItemFact) string {
	if len(item.ReopenLineage) == 0 {
		return ""
	}
	return strings.TrimSpace(item.ReopenLineage[len(item.ReopenLineage)-1].Normalized().Reason)
}
