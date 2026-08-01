package runtime

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func ReduceRuntimeState(facts runtimecontract.RuntimeFacts, previous *runtimecontract.AuthoritySnapshot) (runtimecontract.RuntimeDecision, error) {
	facts = facts.Normalized()
	if err := facts.Validate(); err != nil {
		return runtimecontract.RuntimeDecision{}, err
	}
	decision, err := reduce(facts)
	if err != nil {
		return runtimecontract.RuntimeDecision{}, err
	}
	attempt := 0
	if previous != nil && previous.ID != "" &&
		previous.FocusID == facts.Focus.ID &&
		previous.DefinitionRevision == facts.DefinitionRevision &&
		previous.EvidenceRevision == facts.EvidenceRevision &&
		previous.State == decision.State &&
		previous.RequiredOutputHash == runtimecontract.StableRequiredOutputHash(decision.RequiredOutput) {
		attempt = previous.Attempt + 1
	}
	return runtimecontract.SealRuntimeDecision(facts, decision, attempt)
}

// ReduceRuntimeStateWithDirective reduces facts and returns the decision plus facts
// with RepairDirective synchronized to the sealed decision (INV-05).
func ReduceRuntimeStateWithDirective(facts runtimecontract.RuntimeFacts, previous *runtimecontract.AuthoritySnapshot) (runtimecontract.RuntimeFacts, runtimecontract.RuntimeDecision, error) {
	facts = facts.Normalized()
	// A delayed prerequisite result from a completed focus is never current
	// authority. Retire it before ordinary preflight: otherwise the raw resource
	// pointer makes RuntimeFacts invalid and a drive can surface a process error
	// instead of deterministically acquiring the current focus's owner fact.
	// This is an SM-2 aggregate transition, not a pkg fallback or a candidate
	// admission relaxation.
	facts = retireNonCurrentFocusScopedPrerequisites(facts)
	if currentFocusTargetSelection(facts) == nil && facts.Resources.TargetSelection != nil {
		facts.Resources.TargetSelection = nil
		// A selection-derived snapshot without a current selection must not
		// become an implicit repair target. Explicit focus refs and owner
		// failure targets are independently grounded and retain their snapshots.
		if len(facts.Focus.TargetRefs) == 0 && (facts.Resources.FailureTarget == nil || len(facts.Resources.FailureTarget.TargetRefs) == 0) {
			facts.Resources.TargetSnapshots = nil
		}
	}
	// Target grounding is an SM-2 prerequisite, not a candidate-generation
	// hint. Restrict this to the boundary between acquisition batches so an
	// existing persisted owner acquisition can never be erased.
	if len(facts.Prerequisites.Pending) == 0 {
		facts = PreflightRuntimeFacts(facts, runtimeTargetGroundingRequirements(facts))
	}
	decision, err := ReduceRuntimeState(facts, previous)
	if err != nil {
		return facts, runtimecontract.RuntimeDecision{}, err
	}
	facts = syncRepairDirective(facts, decision)
	// Creating a repair closure or advancing an accepted repair state-change to
	// its mandatory validation changes the action-level candidate authority.
	// Re-seal from the persisted directive so neither the first repair dispatch
	// nor the state-change -> validation handoff can retain a weaker ordinary
	// decision produced before syncRepairDirective updated the closure.
	if facts.RepairDirective != nil && facts.RepairDirective.Closure != nil &&
		(facts.RepairDirective.RequiredState == runtimecontract.ConvergenceStateNeedStateChange || facts.RepairDirective.RequiredState == runtimecontract.ConvergenceStateNeedValidation) &&
		(decision.State != facts.RepairDirective.RequiredState || decision.RequiredOutput.RepairClosureID != facts.RepairDirective.Closure.ID) {
		decision, err = ReduceRuntimeState(facts, previous)
		if err != nil {
			return facts, runtimecontract.RuntimeDecision{}, err
		}
		facts = syncRepairDirective(facts, decision)
	}
	return facts, decision, nil
}

// retireNonCurrentFocusScopedPrerequisites is the defensive half of the
// active/retired lifecycle. Focus advance performs the normal atomic retire;
// this helper handles a delayed result that arrived after that CAS boundary.
// It is intentionally invoked only before an SM-2 reduction, so no provider
// request, candidate attempt, operation, or projection can observe the stale
// fact as current authority.
func retireNonCurrentFocusScopedPrerequisites(facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeFacts {
	facts = facts.Normalized()
	currentFocusID := facts.Focus.ID
	retired := make(map[string]runtimecontract.RuntimePrerequisite)
	filter := func(values []runtimecontract.RuntimePrerequisite) []runtimecontract.RuntimePrerequisite {
		kept := make([]runtimecontract.RuntimePrerequisite, 0, len(values))
		for _, prerequisite := range values {
			if prerequisite.Scope == runtimecontract.RuntimePrerequisiteScopeFocus && prerequisite.FocusID != currentFocusID {
				retired[prerequisite.ID] = prerequisite
				continue
			}
			kept = append(kept, prerequisite)
		}
		return kept
	}
	facts.Prerequisites.Pending = filter(facts.Prerequisites.Pending)
	facts.Prerequisites.Satisfied = filter(facts.Prerequisites.Satisfied)
	if len(retired) == 0 {
		return facts
	}
	knownRetired := make(map[string]struct{}, len(facts.Prerequisites.Retired))
	for _, prerequisite := range facts.Prerequisites.Retired {
		knownRetired[prerequisite.ID] = struct{}{}
	}
	for _, prerequisite := range retired {
		if _, found := knownRetired[prerequisite.ID]; !found {
			facts.Prerequisites.Retired = append(facts.Prerequisites.Retired, prerequisite)
		}
	}
	activeObservations := make([]runtimecontract.RuntimePrerequisiteObservation, 0, len(facts.Resources.PrerequisiteObservations))
	knownRetiredObservations := make(map[string]struct{}, len(facts.Resources.RetiredPrerequisiteObservations))
	for _, observation := range facts.Resources.RetiredPrerequisiteObservations {
		knownRetiredObservations[observation.ObservationID] = struct{}{}
	}
	for _, observation := range facts.Resources.PrerequisiteObservations {
		if _, found := retired[observation.PrerequisiteID]; found {
			if _, exists := knownRetiredObservations[observation.ObservationID]; !exists {
				facts.Resources.RetiredPrerequisiteObservations = append(facts.Resources.RetiredPrerequisiteObservations, observation)
			}
			continue
		}
		activeObservations = append(activeObservations, observation)
	}
	facts.Resources.PrerequisiteObservations = activeObservations
	return facts.Normalized()
}

func reduce(facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeDecision, error) {
	base := runtimecontract.RuntimeDecision{Focus: facts.Focus}
	// A failure whose persisted budget/disposition already determines governance
	// or terminal authority must win before mutable prerequisite bookkeeping.
	// Otherwise an unrelated freshness reacquisition can hide candidate
	// exhaustion or a runtime contract defect and leave SM-3 half closed while
	// SM-4 keeps presenting the Run as resumable.
	if decision, handled, err := reduceDeterminedFailure(base, facts); err != nil {
		return runtimecontract.RuntimeDecision{}, err
	} else if handled {
		return decision, nil
	}
	if len(facts.Prerequisites.Pending) > 0 {
		ids := make([]string, 0, len(facts.Prerequisites.Pending))
		for _, prerequisite := range facts.Prerequisites.Pending {
			ids = append(ids, prerequisite.ID)
		}
		base.State = runtimecontract.ConvergenceStateNeedPrerequisite
		base.Prerequisites = facts.Prerequisites.Pending
		base.RequiredOutput = runtimecontract.RequiredOutputContract{
			PrerequisiteIDs: ids,
		}
		return base, nil
	}
	if len(facts.FreshnessReacquireCoverage) > 0 {
		return stateChangeDecision(base, facts)
	}
	// A validation failure remains a validation gap unless a capability owner
	// has persisted a current, exact repair surface.  Policy text, a command
	// entry, and an infeasible candidate are not implementation authority.
	if needsStateChangeForOwnerGroundedFailure(facts) {
		return stateChangeDecision(base, facts)
	}
	if feasibilityBlocksCurrentAction(facts) {
		code := "capability_unavailable"
		if facts.Feasibility.Category == runtimecontract.RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable {
			code = "validation_recipe_exhausted"
		}
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, code, firstNonEmpty(facts.Feasibility.Reason, "current focus has no feasible operation under sealed capabilities")), nil
	}
	if decision, handled, err := noOpWithoutProvenanceDecision(base, facts); err != nil || handled {
		return decision, err
	}
	// INV-05: persisted RepairDirective is the sole next-action authority for the focus.
	// Raw MissingRequiredEvidenceKinds and anti-replay must not override it.
	if decision, ok, err := applyRepairDirectiveAction(facts, base); err != nil {
		return runtimecontract.RuntimeDecision{}, err
	} else if ok {
		return decision, nil
	}
	// A File-owner create observation is stronger than the ordinary focus
	// evidence ordering: the selected target is absent, so an initial validation
	// cannot truthfully run until one owner-admissible durable create commits.
	// This is not a domain-specific action choice. SM-2 consumes the persisted
	// File observation and descriptor contract only; File remains the path and
	// feasibility authority.
	if hasOwnerGroundedCreationScope(facts) && !hasCurrentFocusTargetBoundStateChangeEvidence(facts) {
		return stateChangeDecision(base, facts)
	}
	missing := missingEvidenceKinds(facts)
	if len(missing) == 0 {
		if !facts.Focus.RequireAssessment {
			return completeOrAdvanceDecision(base, facts, "current coverage focus is satisfied")
		}
		assessment, ok := assessmentForCurrentFocus(facts)
		if !ok {
			return assessmentDecision(base, facts), nil
		}
		// Assessment observation, including its claimed disposition, cannot select
		// an action. Coverage was already classified by owner-confirmed receipts
		// at the internal-event commit boundary; only that persisted result is
		// consulted here.
		if currentFocusCoverageSatisfied(facts) {
			return completeOrAdvanceDecision(base, facts, "current coverage focus is satisfied")
		}
		return repairableAssessmentDecision(base, facts, assessment)
	}
	if contains(missing, runtimecontract.EvidenceKindStateChange) {
		return stateChangeDecision(base, facts)
	}
	if contains(missing, runtimecontract.EvidenceKindValidation) {
		return validationDecision(base, facts)
	}
	return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "unsupported_evidence_gap", fmt.Sprintf("unsupported required evidence kinds: %s", strings.Join(missing, ","))), nil
}

const runtimeStateChangeNoOpWithoutProvenanceCode = "state_change_noop_without_provenance"

// noOpWithoutProvenanceDecision is the C8-F convergence branch for a
// state-change operation which the owner proves did nothing.  It deliberately
// does not turn that receipt into accepted state-change evidence.  If the
// focus has a validation capability, SM-2 asks for one owner-observable
// inspection of the current state. An assessment-required focus can then make
// its semantic claim from that validation evidence. Only that structured
// validation plus assessment path may close an already-current workspace; a
// focus without an assessment requirement terminates with a typed provenance
// gap rather than requesting another random write to manufacture a delta.
func noOpWithoutProvenanceDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeDecision, bool, error) {
	if !hasStateChangeNoOpWithoutProvenance(facts) || !contains(missingEvidenceKinds(facts), runtimecontract.EvidenceKindStateChange) {
		return runtimecontract.RuntimeDecision{}, false, nil
	}
	if hasAcceptedCurrentFocusValidationEvidence(facts) {
		if facts.Focus.RequireAssessment {
			if assessment, found := assessmentForCurrentFocus(facts); found {
				if currentFocusCoverageSatisfied(facts) {
					if facts.StateChange.Required && !facts.StateChange.Satisfied {
						return terminalDecision(base, runtimecontract.ConvergenceStateFailed,
							"state_change_provenance_missing", "workspace state was validated and assessed after a no-op, but the run policy requires durable state-change provenance"), true, nil
					}
					decision, err := completeOrAdvanceDecision(base, facts, "current workspace state was validated and assessed after a no-op")
					return decision, true, err
				}
				decision, err := repairableAssessmentDecision(base, facts, assessment)
				return decision, true, err
			}
			return assessmentDecision(base, facts), true, nil
		}
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed,
			"state_change_provenance_missing", "workspace state was validated after a no-op, but this run has no durable state-change provenance"), true, nil
	}
	if len(focusAcceptedKinds(facts.Capabilities.ValidationKinds, facts.Focus)) == 0 {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed,
			"state_change_provenance_missing", "owner reported a no-op and no validation capability can observe the existing workspace state"), true, nil
	}
	decision, err := validationDecision(base, facts)
	return decision, true, err
}

func hasStateChangeNoOpWithoutProvenance(facts runtimecontract.RuntimeFacts) bool {
	for _, shape := range facts.Repair.NoProgressOperationShapes {
		if shape.FailureClass == runtimecontract.FailureClassEffectMismatch && shape.FailureCode == runtimeStateChangeNoOpWithoutProvenanceCode {
			return true
		}
	}
	return false
}

func hasAcceptedCurrentFocusValidationEvidence(facts runtimecontract.RuntimeFacts) bool {
	for _, evidence := range facts.Evidence.Accepted {
		if evidence.Kind == runtimecontract.EvidenceKindValidation && runtimecontract.EvidenceAppliesToCurrentFocus(facts, evidence) && !weakRuntimeEvidence(evidence) {
			return true
		}
	}
	return false
}

// reduceDeterminedFailure owns the SM-2 ordering rule for failure facts whose
// next authority is already determined. It intentionally lives in the
// Orchestrator owner: the shared runtime contract validates persisted shapes but
// does not choose governance or terminal transitions.
func reduceDeterminedFailure(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeDecision, bool, error) {
	if facts.Failure == nil {
		return runtimecontract.RuntimeDecision{}, false, nil
	}
	failure := facts.Failure
	if FailureClassDisposition(failure.Class) == FailureDispositionSameFocusRepair &&
		(facts.Repair.Exhausted || facts.Repair.RepairAttempts >= facts.ConvergencePolicy.MaxRepairAttemptsPerFocus) {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "no_progress_exhausted", failure.Message), true, nil
	}
	switch failure.Class {
	case runtimecontract.FailureClassProviderTransient:
		if facts.Recovery.ProviderTransportAttempts < facts.RecoveryPolicy.MaxProviderTransportAttempts {
			return runtimecontract.RuntimeDecision{}, false, nil
		}
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "provider_transport_exhausted", failure.Message), true, nil
	case runtimecontract.FailureClassOwnerTransientUnavailable:
		if facts.Recovery.OperationRecoveryAttempts < facts.RecoveryPolicy.MaxOperationRecoveryAttempts {
			return runtimecontract.RuntimeDecision{}, false, nil
		}
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "operation_recovery_exhausted", failure.Message), true, nil
	case runtimecontract.FailureClassGovernanceDependency, runtimecontract.FailureClassGovernanceRequired,
		runtimecontract.FailureClassProviderAuthentication, runtimecontract.FailureClassProviderConfiguration:
		decision, err := governanceDecision(base, facts, failure.Message)
		return decision, true, err
	case runtimecontract.FailureClassOperationOutcomeUnknown:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "operation_outcome_unknown", failure.Message), true, nil
	case runtimecontract.FailureClassRuntimeContractMisalignment:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "runtime_contract_defect", failure.Message), true, nil
	case runtimecontract.FailureClassProviderProtocol:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "provider_protocol_failure", failure.Message), true, nil
	case runtimecontract.FailureClassProviderSubmissionUnknown:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "provider_submission_unknown", failure.Message), true, nil
	case runtimecontract.FailureClassCandidateExhausted:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "candidate_retry_exhausted", failure.Message), true, nil
	case runtimecontract.FailureClassCandidateOutputExhausted:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "candidate_output_continuation_exhausted", failure.Message), true, nil
	case runtimecontract.FailureClassPrerequisiteRefreshExhausted:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "prerequisite_refresh_exhausted", failure.Message), true, nil
	case runtimecontract.FailureClassNoProgressExhausted:
		// A sealed owner observation can prove that no admissible recipe exists
		// for the current authority (for example, validation has no recipe).
		// This is a bounded SM-2 terminal, not an invitation to create another
		// same-focus candidate or a generic contract-defect terminal.
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed,
			firstNonEmpty(strings.TrimSpace(failure.Code), "no_progress_exhausted"), failure.Message), true, nil
	case runtimecontract.FailureClassGovernanceRejected:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "governance_rejected", failure.Message), true, nil
	default:
		return runtimecontract.RuntimeDecision{}, false, nil
	}
}

func feasibilityBlocksCurrentAction(facts runtimecontract.RuntimeFacts) bool {
	if facts.Feasibility == nil {
		return false
	}
	fact := facts.Feasibility
	if fact.FocusID != facts.Focus.ID || len(fact.FeasibleOperationKinds) != 0 {
		return false
	}
	missing := missingEvidenceKinds(facts)
	return (fact.State == runtimecontract.ConvergenceStateNeedStateChange && contains(missing, runtimecontract.EvidenceKindStateChange)) ||
		(fact.State == runtimecontract.ConvergenceStateNeedValidation && contains(missing, runtimecontract.EvidenceKindValidation))
}

func currentFocusCoverageSatisfied(facts runtimecontract.RuntimeFacts) bool {
	for _, id := range facts.Focus.CoverageWorkItemIDs {
		for _, item := range facts.Coverage {
			if item.ID == id && item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
				goto next
			}
		}
		return false
	next:
	}
	return true
}

func assessmentDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeDecision {
	base.State = runtimecontract.ConvergenceStateNeedAssessment
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs, TargetRefs: GroundedRepairTargets(facts),
	}
	return base
}

func stateChangeDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeDecision, error) {
	if decision, blocked, err := targetGroundingDecision(base, facts, false); err != nil || blocked {
		return decision, err
	}
	acceptedKinds := focusAcceptedKinds(facts.Capabilities.StateChangeKinds, facts.Focus)
	if len(acceptedKinds) == 0 {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "capability_unavailable", "state-change authority has no concrete runtime capability"), nil
	}
	// An empty-workspace create scope is owner-grounded permission to propose a
	// new File target, not a general durable-operation grant.  Fail before the
	// provider when the persisted File admissibility observation cannot accept a
	// new target for any sealed durable File kind; otherwise retries would only
	// cycle through candidates that the File owner must reject.
	if hasOwnerGroundedCreationScope(facts) {
		acceptedKinds = creationScopeAdmissibleFileOperationKinds(facts, acceptedKinds)
		if len(acceptedKinds) == 0 {
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed,
				"creation_scope_file_capability_unavailable", "empty workspace creation scope has no owner-admissible durable File create operation"), nil
		}
	}
	base.State = runtimecontract.ConvergenceStateNeedStateChange
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		AcceptedKinds:             acceptedKinds,
		AcceptedOperationVariants: facts.Focus.AcceptedOperationVariants,
		AcceptedEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
		RequiredEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
		RequiredEvidenceKinds:     []string{runtimecontract.EvidenceKindStateChange},
		CoverageWorkItemIDs:       facts.Focus.CoverageWorkItemIDs,
		TargetRefs:                GroundedRepairTargets(facts),
		RepairClosureID:           currentRepairClosureID(facts),
		RequiredOperationSequence: requiredCapabilityStateSequence(facts, acceptedKinds),
	}
	return base, nil
}

// creationScopeAdmissibleFileOperationKinds checks only the committed owner
// inventory. It does not know a selector name, choose a path, inspect a
// workspace, or call a capability owner: a non-empty File SelectorRefs list
// is the owner's opaque assertion that the listed kind may receive a new
// concrete target. The descriptor's AllowsNewTarget property then narrows the
// candidate contract to operations that can actually create an absent target.
// Keeping that interpretation in SM-2 prevents the LLM from treating an empty
// workspace as authority for Git/Shell/MCP side effects.
func creationScopeAdmissibleFileOperationKinds(facts runtimecontract.RuntimeFacts, acceptedKinds []string) []string {
	accepted := make(map[string]struct{}, len(acceptedKinds))
	for _, kind := range acceptedKinds {
		accepted[runtimecontract.CanonicalRuntimeBlockKind(kind)] = struct{}{}
	}
	admissible := make(map[string]struct{}, len(acceptedKinds))
	for _, inventory := range facts.Resources.OperationAdmissibilities {
		inventory = inventory.Normalized()
		if inventory.OwnerModuleID != runtimecontract.RuntimeModuleFile || len(inventory.SelectorRefs) == 0 {
			continue
		}
		for _, kind := range inventory.OperationKinds {
			kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
			if _, permitted := accepted[kind]; !permitted {
				continue
			}
			descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(kind)
			if known && descriptor.OwnerModuleID == runtimecontract.RuntimeModuleFile && descriptor.AllowsNewTarget && runtimecontract.RuntimeEffectIsDurable(descriptor.DefaultEffectScope) {
				admissible[kind] = struct{}{}
			}
		}
	}
	ret := make([]string, 0, len(admissible))
	for _, kind := range acceptedKinds {
		kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
		if _, ok := admissible[kind]; ok {
			ret = append(ret, kind)
		}
	}
	return ret
}

// requiredCapabilityStateSequence only contributes a capability owner's
// sequence when the current focus admits that owner's operation kind. A
// persisted Git sequence must not make an unrelated File-only focus emit Git
// operations merely because both capabilities were acquired for the Run.
func requiredCapabilityStateSequence(facts runtimecontract.RuntimeFacts, acceptedKinds []string) []runtimecontract.RuntimeRequiredOperationVariant {
	ret := []runtimecontract.RuntimeRequiredOperationVariant{}
	accepted := make(map[string]struct{}, len(acceptedKinds))
	for _, kind := range acceptedKinds {
		accepted[runtimecontract.CanonicalRuntimeBlockKind(kind)] = struct{}{}
	}
	for _, snapshot := range facts.Resources.CapabilityStates {
		if snapshot.Disposition != runtimecontract.RuntimeCapabilityStateSequenceRequired {
			continue
		}
		allAdmitted := len(snapshot.RequiredOperationSequence) != 0
		for _, requirement := range snapshot.RequiredOperationSequence {
			if _, found := accepted[runtimecontract.CanonicalRuntimeBlockKind(requirement.Kind)]; !found {
				allAdmitted = false
				break
			}
		}
		if allAdmitted {
			ret = append(ret, snapshot.RequiredOperationSequence...)
		}
	}
	return ret
}

func validationDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeDecision, error) {
	if decision, blocked, err := targetGroundingDecision(base, facts, true); err != nil || blocked {
		return decision, err
	}
	acceptedKinds := focusAcceptedKinds(facts.Capabilities.ValidationKinds, facts.Focus)
	if len(acceptedKinds) == 0 {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "capability_unavailable", "validation authority has no concrete runtime capability"), nil
	}
	base.State = runtimecontract.ConvergenceStateNeedValidation
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		AcceptedKinds:             acceptedKinds,
		AcceptedOperationVariants: facts.Focus.AcceptedOperationVariants,
		AcceptedEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession},
		RequiredEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
		RequiredEvidenceKinds:     []string{runtimecontract.EvidenceKindValidation},
		CoverageWorkItemIDs:       facts.Focus.CoverageWorkItemIDs,
		TargetRefs:                GroundedRepairTargets(facts),
		RepairClosureID:           currentRepairClosureID(facts),
	}
	return base, nil
}

func currentRepairClosureID(facts runtimecontract.RuntimeFacts) string {
	if facts.RepairDirective == nil || facts.RepairDirective.FocusID != facts.Focus.ID || facts.RepairDirective.Closure == nil {
		return ""
	}
	return facts.RepairDirective.Closure.ID
}

// targetGroundingDecision is the common SM-2 pre-provider fence for executable
// direct-evidence actions. A bounded manifest or selection acquisition
// establishes what the owner observed, but an absent or ambiguous observation
// does not authorize a first validation or a durable candidate. A validation
// following a current-focus accepted target-bound state change may reuse that
// already sealed evidence lineage; it cannot authorize another state change.
//
// An owner-complete target observation is a deterministic feasibility result,
// not an external dependency. In particular, opening a decision ticket cannot
// make an absent or ambiguous target admissible: a generic "continue" would
// only re-enter this same gate and create a ticket loop. The reducer therefore
// always closes this path with a typed terminal. Capability owners and the
// shared contract package only carry observations; SM-2 owns the terminal.
func targetGroundingDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts, allowAcceptedStateChangeLineage bool) (runtimecontract.RuntimeDecision, bool, error) {
	if !facts.Focus.RequireTargetGrounding || len(GroundedRepairTargets(facts)) != 0 || hasOwnerGroundedCreationScope(facts) || hasOwnerGroundedTargetScope(facts) ||
		(allowAcceptedStateChangeLineage && hasCurrentFocusTargetBoundStateChangeEvidence(facts)) {
		return runtimecontract.RuntimeDecision{}, false, nil
	}
	const summary = "direct-evidence focus requires an owner-grounded target; workspace discovery alone did not select one"
	return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "target_grounding_unresolved", summary), true, nil
}

// hasOwnerGroundedCreationScope is the empty-workspace counterpart to an
// existing target snapshot. It is true only for the File owner's persisted
// creation observation; the reducer never derives a file path from Goal, DoD,
// manifest ordering, or provider text.
func hasOwnerGroundedCreationScope(facts runtimecontract.RuntimeFacts) bool {
	selection := currentFocusTargetSelection(facts)
	if selection == nil {
		return false
	}
	value := selection.Normalized()
	focus := facts.Focus.Normalized()
	return value.Disposition == runtimecontract.RuntimeTargetSelectionCreate &&
		value.OwnerModuleID == runtimecontract.RuntimeModuleFile &&
		value.TargetRole == focus.TargetRole && value.SelectorRef == focus.TargetSelectorRef &&
		value.SelectorOwnerRef == focus.TargetSelectorOwnerRef && value.SelectorSubjectRef == focus.TargetSelectorSubjectRef &&
		runtimecontract.SameRuntimeTargetSelectorCandidateRefs(value.SelectorCandidateRefs, focus.TargetSelectorCandidateRefs) &&
		value.ProspectiveTargetRef != "" && value.ProspectiveTargetRef == focus.ProspectiveTargetRef
}

func hasOwnerGroundedTargetScope(facts runtimecontract.RuntimeFacts) bool {
	selection := currentFocusTargetSelection(facts)
	if selection == nil || selection.Disposition != runtimecontract.RuntimeTargetSelectionScope || selection.OwnerModuleID != runtimecontract.RuntimeModuleFile {
		return false
	}
	// A complete existing-workspace scope has the bounded target set. An empty
	// scope is legal only when activation sealed an owning profile/subject pair;
	// File never obtains that authority from Goal/DoD prose or directory names.
	return len(selection.TargetRefs) != 0 ||
		(selection.CandidateCount == 0 && len(selection.CandidateTargetRefs) == 0 && selection.ProspectiveTargetRef == "" &&
			selection.SelectorOwnerRef != "" && selection.SelectorSubjectRef != "")
}

// targetSelectionHasConcreteTarget is intentionally narrower than an owner
// scope. A selected value is an owner-grounded concrete repair surface; a
// scope is only a bounded File feasibility inventory and must not enter facts
// that are interpreted as a repair target.
func targetSelectionHasConcreteTarget(selection runtimecontract.RuntimeTargetSelection) bool {
	return selection.Disposition == runtimecontract.RuntimeTargetSelectionSelected && len(selection.TargetRefs) != 0
}

func hasCurrentFocusTargetBoundStateChangeEvidence(facts runtimecontract.RuntimeFacts) bool {
	for _, evidence := range facts.Evidence.Accepted {
		if evidence.Kind == runtimecontract.EvidenceKindStateChange && len(evidence.TargetRefs) != 0 && runtimecontract.EvidenceAppliesToCurrentFocus(facts, evidence) {
			return true
		}
	}
	return false
}

func focusAcceptedKinds(capabilityKinds []string, focus runtimecontract.CoverageFocus) []string {
	allowed := make(map[string]struct{}, len(focus.AcceptedStepKinds))
	for _, kind := range focus.AcceptedStepKinds {
		allowed[kind] = struct{}{}
	}
	rejected := make(map[string]struct{}, len(focus.RejectedStepKinds))
	for _, kind := range focus.RejectedStepKinds {
		rejected[kind] = struct{}{}
	}
	ret := make([]string, 0, len(capabilityKinds))
	for _, kind := range capabilityKinds {
		if _, denied := rejected[kind]; denied {
			continue
		}
		if len(allowed) > 0 {
			if _, accepted := allowed[kind]; !accepted {
				continue
			}
		}
		ret = append(ret, kind)
	}
	return ret
}

func governanceDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts, summary string) (runtimecontract.RuntimeDecision, error) {
	if _, found := governanceDependencyForFacts(facts); !found {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "governance_dependency_missing", "runtime governance requires a sealed external dependency"), nil
	}
	if len(facts.Capabilities.GovernanceKinds) == 0 {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "governance_capability_unavailable", "runtime governance dependency has no concrete runtime capability"), nil
	}
	base.State = runtimecontract.ConvergenceStateNeedGovernance
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		AcceptedKinds: facts.Capabilities.GovernanceKinds,
	}
	return base, nil
}

// governanceDependencyForFacts recognizes only a dependency explicitly sealed
// into a failure fact or the provider configuration dependency whose subject
// is derived from the sealed provider ceiling. It never turns a summary,
// focus, target, feasibility result, or retry exhaustion into a ticket.
func governanceDependencyForFacts(facts runtimecontract.RuntimeFacts) (runtimecontract.ExternalDependency, bool) {
	facts = facts.Normalized()
	if facts.Failure != nil && facts.Failure.GovernanceDependency != nil {
		dependency := facts.Failure.GovernanceDependency.Normalized()
		if dependency.Validate() == nil {
			return dependency, true
		}
		return runtimecontract.ExternalDependency{}, false
	}
	if facts.Failure == nil || (facts.Failure.Class != runtimecontract.FailureClassProviderAuthentication && facts.Failure.Class != runtimecontract.FailureClassProviderConfiguration) || facts.Resources.ProviderCeiling == nil {
		return runtimecontract.ExternalDependency{}, false
	}
	target := facts.Resources.ProviderCeiling.Target.Normalized()
	if target.ProviderID == "" || target.ModelID == "" {
		return runtimecontract.ExternalDependency{}, false
	}
	reason := "provider configuration requires attention"
	if facts.Failure.Class == runtimecontract.FailureClassProviderAuthentication {
		reason = "provider authentication requires configuration"
	}
	return runtimecontract.ExternalDependency{
		Kind: string(runtimecontract.GovernanceConfiguration), SubjectRef: target.ProviderID + "/" + target.ModelID, Reason: reason,
	}, true
}

// completeOrAdvanceDecision completes the current focus only when remaining focuses are empty
// and the Run-level state-change ledger satisfies the sealed activation policy.
func completeOrAdvanceDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts, summary string) (runtimecontract.RuntimeDecision, error) {
	if len(facts.RemainingFocuses) > 0 {
		// Focus-level satisfaction alone does not terminal the Run while more focuses remain.
		// Drive/commit paths advance the focus queue; reducer reports complete for the current focus.
		return terminalDecision(base, runtimecontract.ConvergenceStateComplete, "coverage_satisfied", summary), nil
	}
	if !facts.StateChange.SatisfiesRequirement() {
		// Run policy still needs coverage-bound primary state-change evidence.
		return stateChangeDecision(base, facts)
	}
	return terminalDecision(base, runtimecontract.ConvergenceStateComplete, "coverage_satisfied", summary), nil
}

func terminalDecision(base runtimecontract.RuntimeDecision, state runtimecontract.ConvergenceState, code, summary string) runtimecontract.RuntimeDecision {
	base.State = state
	base.Terminal = &runtimecontract.RuntimeTerminalDecision{Code: code, Summary: summary}
	return base
}

func missingEvidenceKinds(facts runtimecontract.RuntimeFacts) []string {
	return runtimecontract.MissingRequiredEvidenceKinds(facts)
}

// needsStateChangeForOwnerGroundedFailure is the only SM-2 bridge from a
// rejected validation/effect fact to a durable repair.  A raw failure target
// is intentionally insufficient: the target must have been retained as the
// current owner-grounded FailureTarget by the receipt/prerequisite path.
func needsStateChangeForOwnerGroundedFailure(facts runtimecontract.RuntimeFacts) bool {
	failure := facts.Failure
	if failure == nil || facts.Resources.FailureTarget == nil {
		return false
	}
	switch failure.Class {
	case runtimecontract.FailureClassExecutionRejected, runtimecontract.FailureClassExecutionFailure,
		runtimecontract.FailureClassBuildFailure, runtimecontract.FailureClassEffectMismatch:
	default:
		return false
	}
	target := facts.Resources.FailureTarget
	return strings.TrimSpace(target.Signature) != "" && target.Signature == failure.Signature &&
		len(target.TargetRefs) > 0 && sameRuntimeTargetRefs(target.TargetRefs, failure.TargetRefs)
}

// evidenceRevisionAfter is an owner-local convenience shared by candidate
// admission and SM-2 reduction. The contract package supplies the pure value
// comparison; neither path uses it to choose a state transition on its own.
func evidenceRevisionAfter(left string, right string) bool {
	return runtimecontract.EvidenceRevisionAfter(left, right)
}

func advanceSatisfiedCoverageFocus(facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeFacts {
	facts = facts.Normalized()
	if len(missingEvidenceKinds(facts)) != 0 || !currentFocusCoverageSatisfied(facts) || len(facts.RemainingFocuses) == 0 {
		return facts
	}
	completedFocusID := facts.Focus.ID
	facts.CompletedFocuses = append(facts.CompletedFocuses, facts.Focus)
	facts.Focus = facts.RemainingFocuses[0]
	facts.RemainingFocuses = append([]runtimecontract.CoverageFocus(nil), facts.RemainingFocuses[1:]...)
	retireFocusScopedPrerequisites(&facts, completedFocusID)
	facts.Failure = nil
	facts.Feasibility = nil
	// Target selections and snapshots are scoped to the completed Focus. A
	// profile bootstrap recipe intentionally advances through several sealed
	// artifact targets; retaining the prior File observation would make the
	// next Focus reuse its predecessor's path and violate its own selector
	// binding. Manifest and capability inventories remain valid owner facts and
	// are refreshed normally when their revisions change.
	facts.Resources.TargetSelection = nil
	facts.Resources.TargetSnapshots = nil
	facts.Resources.FailureTarget = nil
	facts.Repair = resetRepairForFocus(facts.Focus.ID, facts.Focus.SliceID, facts.EvidenceRevision)
	facts.RepairDirective = nil
	return facts.Normalized()
}

// retireFocusScopedPrerequisites moves the completed Focus's owner facts out
// of the active prerequisite graph in the same SM-2 transition that advances
// Focus.  Retained values are audit lineage only: freshness never sees them,
// so a later workspace-tree change cannot recreate an old target authority.
func retireFocusScopedPrerequisites(facts *runtimecontract.RuntimeFacts, focusID string) {
	if facts == nil {
		return
	}
	focusID = strings.TrimSpace(focusID)
	if focusID == "" {
		return
	}
	retired := make(map[string]runtimecontract.RuntimePrerequisite)
	retire := func(values []runtimecontract.RuntimePrerequisite) []runtimecontract.RuntimePrerequisite {
		kept := make([]runtimecontract.RuntimePrerequisite, 0, len(values))
		for _, prerequisite := range values {
			if prerequisite.Scope == runtimecontract.RuntimePrerequisiteScopeFocus && prerequisite.FocusID == focusID {
				retired[prerequisite.ID] = prerequisite
				continue
			}
			kept = append(kept, prerequisite)
		}
		return kept
	}
	facts.Prerequisites.Pending = retire(facts.Prerequisites.Pending)
	facts.Prerequisites.Satisfied = retire(facts.Prerequisites.Satisfied)
	if len(retired) == 0 {
		return
	}
	for _, prerequisite := range retired {
		facts.Prerequisites.Retired = append(facts.Prerequisites.Retired, prerequisite)
	}
	activeObservations := make([]runtimecontract.RuntimePrerequisiteObservation, 0, len(facts.Resources.PrerequisiteObservations))
	for _, observation := range facts.Resources.PrerequisiteObservations {
		if _, found := retired[observation.PrerequisiteID]; found {
			facts.Resources.RetiredPrerequisiteObservations = append(facts.Resources.RetiredPrerequisiteObservations, observation)
			continue
		}
		activeObservations = append(activeObservations, observation)
	}
	facts.Resources.PrerequisiteObservations = activeObservations
}

// repairableAssessmentDecision keeps the Run active and selects a repair
// branch exclusively from owner-confirmed persisted facts. Assessment gap
// fields remain audit observations: an LLM must not choose a repair action,
// target, validation-only budget or terminal by naming a missing evidence kind.
func repairableAssessmentDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts, assessment runtimecontract.RuntimeAssessmentFact) (runtimecontract.RuntimeDecision, error) {
	policy := facts.ConvergencePolicy.Normalized()
	ledger := facts.Repair.Normalized()
	if ledger.Exhausted || ledger.RepairAttempts >= policy.MaxRepairAttemptsPerFocus {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "no_progress_exhausted", firstNonEmptyAssessmentSummary(assessment, "repair budget exhausted")), nil
	}
	if !hasAcceptedCurrentFocusValidationEvidence(facts) {
		return validationOnlyRepairDecision(base, facts, policy, ledger)
	}
	if focusRequiresDurableStateChangeRepair(facts) {
		return stateChangeDecision(base, facts)
	}
	return validationOnlyRepairDecision(base, facts, policy, ledger)
}

func validationOnlyRepairDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts, policy runtimecontract.RuntimeConvergencePolicy, ledger runtimecontract.RuntimeRepairLedger) (runtimecontract.RuntimeDecision, error) {
	if policy.MaxValidationOnlyRetriesPerFocus > 0 && ledger.ValidationOnlyRetries >= policy.MaxValidationOnlyRetriesPerFocus {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "no_progress_exhausted", "validation-only repair budget exhausted"), nil
	}
	return validationDecision(base, facts)
}

// focusRequiresDurableStateChangeRepair is a neutral policy read: it does not
// inspect a file name, summary or assessment candidate. A repairable coverage
// gap after usable validation can require a durable repair only when the
// sealed focus or Run-level requirement says state-change evidence matters.
func focusRequiresDurableStateChangeRepair(facts runtimecontract.RuntimeFacts) bool {
	if contains(facts.Focus.RequiredEvidenceKinds, runtimecontract.EvidenceKindStateChange) {
		return true
	}
	return facts.StateChange.Required && !facts.StateChange.SatisfiesRequirement()
}

func firstNonEmptyAssessmentSummary(assessment runtimecontract.RuntimeAssessmentFact, fallback string) string {
	if summary := strings.TrimSpace(assessment.Summary); summary != "" {
		return summary
	}
	if assessment.Gap != nil {
		if reason := strings.TrimSpace(assessment.Gap.Reason); reason != "" {
			return reason
		}
	}
	return fallback
}

func assessmentForCurrentFocus(facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeAssessmentFact, bool) {
	// Prefer the latest assessment for the current focus so repair cycles can supersede older facts.
	found := false
	var latest runtimecontract.RuntimeAssessmentFact
	for _, assessment := range facts.Assessments {
		if assessment.FocusID != facts.Focus.ID {
			continue
		}
		latest = assessment
		found = true
	}
	if !found {
		return runtimecontract.RuntimeAssessmentFact{}, false
	}
	return latest, true
}

func evidenceAppliesToCurrentFocus(facts runtimecontract.RuntimeFacts, evidence runtimecontract.EvidenceFact) bool {
	return runtimecontract.EvidenceAppliesToCurrentFocus(facts, evidence)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
