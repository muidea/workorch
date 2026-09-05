package runtime

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/grounding"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func ReduceRuntimeState(facts runtimecontract.RuntimeFacts, previous *runtimecontract.AuthoritySnapshot, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) (runtimecontract.RuntimeDecision, error) {
	decision, _, err := reduceAndSealRuntimeState(facts, previous, nil, runtimeCoverageProofLineageArgument(proofLineages))
	return decision, err
}

// runtimeOwnerInternalReductionEvent is the typed, invocation-local fact that
// lets the sole SM-2 reducer consume an already-proven owner-private event.
// It is never persisted and carries no requested decision or RequiredOutput:
// the reducer derives those from the immutable source dispatch and the exact
// governance result/binding lineage.
type runtimeOwnerInternalReductionEvent struct {
	Kind                 RuntimeInternalEventKind
	GovernanceResolution runtimecontract.RuntimeGovernanceResolution
	SourceDispatch       SealedDispatch
	OperationBinding     runtimecontract.RuntimeOperationGovernanceBinding
}

func reduceAndSealRuntimeState(
	facts runtimecontract.RuntimeFacts,
	previous *runtimecontract.AuthoritySnapshot,
	ownerEvent *runtimeOwnerInternalReductionEvent,
	proofLineage []runtimecontract.RuntimeCoverageProofLineage,
) (runtimecontract.RuntimeDecision, bool, error) {
	facts = facts.Normalized()
	if err := facts.Validate(); err != nil {
		return runtimecontract.RuntimeDecision{}, false, err
	}
	decision, eventHandled, err := reduceOwnerInternalEvent(facts, ownerEvent)
	if err != nil {
		return runtimecontract.RuntimeDecision{}, false, err
	}
	if !eventHandled {
		decision, err = reduce(facts, proofLineage)
		if err != nil {
			return runtimecontract.RuntimeDecision{}, false, err
		}
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
	sealed, err := runtimecontract.SealRuntimeDecision(facts, decision, attempt)
	return sealed, eventHandled, err
}

// ReduceRuntimeStateWithDirective reduces facts and returns the decision plus facts
// with RepairDirective synchronized to the sealed decision (INV-05).
func ReduceRuntimeStateWithDirective(
	facts runtimecontract.RuntimeFacts,
	previous *runtimecontract.AuthoritySnapshot,
	proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage,
) (runtimecontract.RuntimeFacts, runtimecontract.RuntimeDecision, error) {
	return reduceRuntimeStateWithDirectiveAndOwnerEvent(facts, previous, nil, proofLineages...)
}

// reduceRuntimeStateWithDirectiveAndOwnerEvent is package-private so only an
// Orchestrator owner-private transition can provide an event input. Ordinary
// callers retain the facts-only reducer contract above; both paths share the
// exact same reducer/seal implementation.
func reduceRuntimeStateWithDirectiveAndOwnerEvent(
	facts runtimecontract.RuntimeFacts,
	previous *runtimecontract.AuthoritySnapshot,
	ownerEvent *runtimeOwnerInternalReductionEvent,
	proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage,
) (runtimecontract.RuntimeFacts, runtimecontract.RuntimeDecision, error) {
	facts = facts.Normalized()
	// Every SM-2 reduction is a defensive repair-ledger boundary. Focus changes
	// normally reset this state atomically, while semantic re-seal, retirement,
	// and delayed facts can reach the reducer through a different persisted
	// path. Do not let a prior CWI's gap, budget, or directive choose this
	// focus's next action.
	facts = rebindRepairForCurrentFocus(facts)
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
		facts = preflightRuntimeFactsPreservingPending(facts, runtimeTargetGroundingRequirements(facts, proofLineages...))
	}
	decision, eventHandled, err := reduceAndSealRuntimeState(facts, previous, ownerEvent, runtimeCoverageProofLineageArgument(proofLineages))
	if err != nil {
		return facts, runtimecontract.RuntimeDecision{}, err
	}
	if eventHandled {
		// A sealed-dispatch continuation is authorized by its immutable source
		// transaction. It must not recreate the superseded mutable repair target
		// or directive merely to keep the remaining physical operation runnable.
		return facts, decision, nil
	}
	facts = syncRepairDirective(facts, decision, proofLineages...)
	// Creating a repair closure or advancing an accepted repair state-change to
	// its mandatory validation changes the action-level candidate authority.
	// Re-seal from the persisted directive so neither the first repair dispatch
	// nor the state-change -> validation handoff can retain a weaker ordinary
	// decision produced before syncRepairDirective updated the closure.
	if facts.RepairDirective != nil && facts.RepairDirective.Closure != nil &&
		(facts.RepairDirective.RequiredState == runtimecontract.ConvergenceStateNeedStateChange || facts.RepairDirective.RequiredState == runtimecontract.ConvergenceStateNeedValidation) &&
		(decision.State != facts.RepairDirective.RequiredState || decision.RequiredOutput.RepairClosureID != facts.RepairDirective.Closure.ID ||
			decision.RequiredOutput.RepairPurpose != facts.RepairDirective.Purpose) {
		decision, err = ReduceRuntimeState(facts, previous, proofLineages...)
		if err != nil {
			return facts, runtimecontract.RuntimeDecision{}, err
		}
		facts = syncRepairDirective(facts, decision, proofLineages...)
	}
	return facts, decision, nil
}

func reduceOwnerInternalEvent(
	facts runtimecontract.RuntimeFacts,
	event *runtimeOwnerInternalReductionEvent,
) (runtimecontract.RuntimeDecision, bool, error) {
	if event == nil {
		return runtimecontract.RuntimeDecision{}, false, nil
	}
	if event.Kind != RuntimeInternalEventGovernanceResolutionObserved {
		return runtimecontract.RuntimeDecision{}, false, fmt.Errorf("%w: reducer does not support owner-internal event %q", common.ErrInvalidArgument, event.Kind)
	}
	resolution := event.GovernanceResolution.Normalized()
	if err := resolution.Validate(); err != nil {
		return runtimecontract.RuntimeDecision{}, false, err
	}
	if resolution.Outcome != runtimecontract.RuntimeGovernanceResolutionOutcomeContinue {
		return runtimecontract.RuntimeDecision{}, false, fmt.Errorf("%w: only an accepted governance continuation can carry sealed operation authority", common.ErrConflict)
	}
	binding := event.OperationBinding.Normalized()
	if binding.State != runtimecontract.RuntimeOperationGovernanceBindingGranted || binding.Grant == nil {
		return runtimecontract.RuntimeDecision{}, false, fmt.Errorf("%w: governance continuation reducer input has no exact granted binding", common.ErrConflict)
	}
	grant := binding.Grant.Normalized()
	if grant.ResumeDispatchID != "" || grant.ResolutionID != resolution.ResolutionID ||
		grant.ResolutionHash != runtimecontract.StableRuntimeGovernanceResolutionHash(resolution) ||
		grant.TicketID != resolution.TicketID || grant.ApprovalID != resolution.ApprovalID ||
		binding.GovernanceDispatchID != resolution.DispatchID || binding.GovernanceSnapshotID != resolution.AuthoritySnapshotID ||
		binding.DependencyKind != resolution.DependencyKind || binding.SubjectRef != resolution.SubjectRef {
		return runtimecontract.RuntimeDecision{}, false, fmt.Errorf("%w: governance continuation reducer input does not match its owner result", common.ErrConflict)
	}
	matchingFacts := 0
	for _, fact := range facts.Evidence.Accepted {
		if fact.Revision == facts.EvidenceRevision && runtimecontract.RuntimeGovernanceEvidenceFactMatchesResolution(fact, resolution) {
			matchingFacts++
		}
	}
	if matchingFacts != 1 {
		return runtimecontract.RuntimeDecision{}, false, fmt.Errorf("%w: governance continuation reducer input has no unique current evidence fact", common.ErrConflict)
	}

	source := event.SourceDispatch
	if source.WorkspaceID != facts.WorkspaceID || source.RunID != facts.RunID ||
		source.ID != binding.OperationDispatchID || source.State != DispatchStateCommitted ||
		source.Transaction.FocusID != facts.Focus.ID || source.Transaction.State != runtimecontract.ConvergenceStateNeedStateChange {
		return runtimecontract.RuntimeDecision{}, false, fmt.Errorf("%w: governance continuation source dispatch does not match current authority", common.ErrConflict)
	}
	if err := sealedDispatchToRuntimeRecord(source).ValidateIntrinsic(); err != nil {
		return runtimecontract.RuntimeDecision{}, false, fmt.Errorf("%w: governance continuation source dispatch: %v", common.ErrConflict, err)
	}
	operation, found := candidateOperationByID(source.Candidate, binding.OperationID)
	if !found || containsString(source.CompletedOperationIDs, binding.OperationID) ||
		len(source.CompletedOperationIDs) >= candidateOperationCount(source.Candidate) {
		return runtimecontract.RuntimeDecision{}, false, fmt.Errorf("%w: governance continuation has no pending granted operation", common.ErrConflict)
	}
	execution, found := operationExecutionRecord(source.OperationExecutions, binding.OperationID)
	if !found || execution.OperationHash != runtimecontract.StableRuntimeOperationHash(operation) ||
		execution.ExecutionKey != binding.ExecutionKey || execution.Fence != binding.Fence ||
		grant.ExecutionKey != binding.ExecutionKey || grant.GovernanceFence != binding.Fence || grant.ResumeFence != binding.Fence+1 {
		return runtimecontract.RuntimeDecision{}, false, fmt.Errorf("%w: governance continuation granted operation has no exact sealed execution", common.ErrConflict)
	}

	operations := candidateRuntimeOperations(source.Candidate)
	if len(operations) == 0 || len(operations) != candidateOperationCount(source.Candidate) {
		return runtimecontract.RuntimeDecision{}, false, fmt.Errorf("%w: governance continuation source is not an operation dispatch", common.ErrConflict)
	}
	operationKinds := make([]string, 0, len(operations))
	effectScopes := make([]runtimecontract.RuntimeEffectScope, 0, len(operations))
	targetRefs := make([]string, 0, len(operations))
	for _, rawOperation := range operations {
		operation := rawOperation.Normalized()
		execution, found := operationExecutionRecord(source.OperationExecutions, operation.ID)
		if !found || execution.OperationHash != runtimecontract.StableRuntimeOperationHash(operation) ||
			!runtimecontract.RuntimeEffectIsDurable(execution.ExpectedEffectScope) || len(operation.TargetRefs) == 0 {
			return runtimecontract.RuntimeDecision{}, false, fmt.Errorf("%w: governance continuation source has incomplete sealed operation authority", common.ErrConflict)
		}
		operationKinds = append(operationKinds, operation.Kind)
		effectScopes = append(effectScopes, execution.ExpectedEffectScope)
		targetRefs = append(targetRefs, operation.TargetRefs...)
	}
	targetRefs = normalizeRuntimeTargetRefs(targetRefs)
	requiredSequence, err := sealedDispatchRequiredOperationSequence(source)
	if err != nil {
		return runtimecontract.RuntimeDecision{}, false, err
	}
	closureID := ""
	purpose := runtimecontract.RuntimeDispatchRepairPurpose("")
	if source.RepairCommitment != nil {
		commitment := source.RepairCommitment.Normalized()
		if commitment.FocusID != facts.Focus.ID || !sameCoverageIDs(commitment.Closure.CoverageWorkItemIDs, facts.Focus.CoverageWorkItemIDs) {
			return runtimecontract.RuntimeDecision{}, false, fmt.Errorf("%w: governance continuation repair commitment does not match current focus", common.ErrConflict)
		}
		closureID = commitment.Closure.ID
		purpose = commitment.Purpose
	}
	return runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:             operationKinds,
			AcceptedOperationVariants: acceptedOperationVariantsForKinds(facts.Focus.AcceptedOperationVariants, operationKinds),
			AcceptedEffectScopes:      effectScopes,
			RequiredEffectScopes:      effectScopes,
			RequiredEvidenceKinds:     []string{runtimecontract.EvidenceKindStateChange},
			CoverageWorkItemIDs:       append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...),
			TargetRefs:                targetRefs,
			RepairClosureID:           closureID,
			RepairPurpose:             purpose,
			RequiredOperationSequence: requiredSequence,
		},
	}, true, nil
}

func sealedDispatchRequiredOperationSequence(dispatch SealedDispatch) ([]runtimecontract.RuntimeRequiredOperationVariant, error) {
	operations, err := orderedCandidateOperations(dispatch.Candidate)
	if err != nil {
		return nil, err
	}
	requirementByOperation := make(map[string]string, len(operations))
	hasRequirement := false
	for _, operation := range operations {
		operation = operation.Normalized()
		if operation.RequiredOperationRef != "" {
			hasRequirement = true
			requirementByOperation[operation.ID] = operation.RequiredOperationRef
		}
	}
	if !hasRequirement {
		return nil, nil
	}
	if len(requirementByOperation) != len(operations) {
		return nil, fmt.Errorf("%w: sealed continuation has a partial required-operation projection", common.ErrConflict)
	}
	ret := make([]runtimecontract.RuntimeRequiredOperationVariant, 0, len(operations))
	for _, operation := range operations {
		operation = operation.Normalized()
		contract, found := runtimecontract.RuntimeOperationContractForKind(operation.Kind)
		if !found {
			return nil, fmt.Errorf("%w: sealed continuation operation %q has no contract", common.ErrConflict, operation.Kind)
		}
		variant, err := runtimecontract.MatchRuntimeOperationVariant(operation, contract)
		if err != nil {
			return nil, err
		}
		dependencies := make([]string, 0, len(operation.DependsOn))
		for _, dependencyID := range operation.DependsOn {
			dependency, found := requirementByOperation[dependencyID]
			if !found {
				return nil, fmt.Errorf("%w: sealed continuation dependency has no requirement identity", common.ErrConflict)
			}
			dependencies = append(dependencies, dependency)
		}
		targetRef := ""
		if len(operation.TargetRefs) == 1 {
			targetRef = operation.TargetRefs[0]
		}
		ret = append(ret, runtimecontract.RuntimeRequiredOperationVariant{
			RequirementID:           operation.RequiredOperationRef,
			Kind:                    operation.Kind,
			VariantID:               variant.ID,
			TargetRef:               targetRef,
			DependsOnRequirementIDs: dependencies,
		})
	}
	return ret, nil
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

func reduce(facts runtimecontract.RuntimeFacts, proofLineage []runtimecontract.RuntimeCoverageProofLineage) (runtimecontract.RuntimeDecision, error) {
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
	// A validation failure remains a validation gap unless a capability owner
	// has persisted a current, exact repair surface.  Policy text, a command
	// entry, and an infeasible candidate are not implementation authority.
	if needsStateChangeForOwnerGroundedFailure(facts) {
		return stateChangeDecision(base, facts)
	}
	if feasibilityBlocksCurrentAction(facts, proofLineage) {
		code := "capability_unavailable"
		if facts.Feasibility.Category == runtimecontract.RuntimeOwnerFeasibilityCategoryValidationRecipeUnavailable {
			code = "validation_recipe_exhausted"
		}
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, code, firstNonEmpty(facts.Feasibility.Reason, "current focus has no feasible operation under sealed capabilities")), nil
	}
	if decision, handled, err := noOpWithoutProvenanceDecision(base, facts, proofLineage); err != nil || handled {
		return decision, err
	}
	// INV-05: persisted RepairDirective is the sole next-action authority for the focus.
	// Raw MissingRequiredEvidenceKinds and anti-replay must not override it.
	if decision, ok, err := applyRepairDirectiveAction(facts, base, proofLineage); err != nil {
		return runtimecontract.RuntimeDecision{}, err
	} else if ok {
		return decision, nil
	}
	// Once assessment has observed the current bound proof, a missing owner
	// postcondition whose single declared acquisition is no longer resolvable
	// or has already been consumed is the authoritative outcome. Do this before
	// the generic prospective-target gate: retrying the acquisition target would
	// cross the Profile's one-shot convergence boundary and mislabel exhaustion
	// as target grounding failure.
	if _, observed := assessmentForCurrentFocus(facts, proofLineage); observed {
		if missing := runtimecontract.RuntimeMissingOwnerFactRequirements(facts.Focus.RequiredOwnerFacts, facts.Resources.OperationAdmissibilities); len(missing) != 0 {
			acquisition, ready := runtimecontract.ResolveRuntimeOwnerFactAcquisition(facts.Focus.RequiredOwnerFacts, facts.Focus.OwnerFactConvergences, facts.Resources.OperationAdmissibilities)
			if !ready || runtimeOwnerFactReplanBudgetExhausted(facts, acquisition) {
				return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "owner_fact_convergence_exhausted", "required capability-owner fact remained unavailable after the bounded semantic replan budget"), nil
			}
		}
	}
	// An owner create observation is stronger than the ordinary focus
	// evidence ordering: the selected target is absent, so an initial validation
	// cannot truthfully run until one owner-admissible durable create commits.
	// This is not a domain-specific action choice. SM-2 consumes the persisted
	// selection and capability-owner catalog only; that owner retains target and
	// feasibility authority.
	if hasOwnerGroundedProspectiveCreationScope(facts) && !hasCurrentFocusTargetBoundStateChangeEvidence(facts, proofLineage) {
		return stateChangeDecision(base, facts)
	}
	missing := missingEvidenceKinds(facts, proofLineage)
	if len(missing) == 0 {
		if !facts.Focus.RequireAssessment {
			return completeOrAdvanceDecision(base, facts, "current coverage focus is satisfied")
		}
		assessment, ok := assessmentForCurrentFocus(facts, proofLineage)
		if !ok {
			return assessmentDecision(base, facts), nil
		}
		// Assessment observation, including its claimed disposition, cannot select
		// an action. Coverage was already classified by owner-confirmed receipts
		// at the internal-event commit boundary; only that persisted result is
		// consulted here.
		if currentFocusCoverageSatisfied(facts, proofLineage) {
			return completeOrAdvanceDecision(base, facts, "current coverage focus is satisfied")
		}
		return repairableAssessmentDecision(base, facts, assessment, proofLineage)
	}
	if contains(missing, runtimecontract.EvidenceKindStateChange) {
		return stateChangeDecision(base, facts)
	}
	if contains(missing, runtimecontract.EvidenceKindValidation) {
		if sameFocusValidationRepairActive(facts) {
			return validationOnlyRepairDecision(base, facts, proofLineage)
		}
		return validationDecision(base, facts, proofLineage)
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
func noOpWithoutProvenanceDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts, proofLineage []runtimecontract.RuntimeCoverageProofLineage) (runtimecontract.RuntimeDecision, bool, error) {
	if !hasStateChangeNoOpWithoutProvenance(facts) || !contains(missingEvidenceKinds(facts, proofLineage), runtimecontract.EvidenceKindStateChange) {
		return runtimecontract.RuntimeDecision{}, false, nil
	}
	if hasAcceptedCurrentFocusValidationEvidence(facts, proofLineage) {
		if facts.Focus.RequireAssessment {
			if assessment, found := assessmentForCurrentFocus(facts, proofLineage); found {
				if currentFocusCoverageSatisfied(facts, proofLineage) {
					if facts.StateChange.Required && !facts.StateChange.Satisfied {
						return terminalDecision(base, runtimecontract.ConvergenceStateFailed,
							"state_change_provenance_missing", "workspace state was validated and assessed after a no-op, but the run policy requires durable state-change provenance"), true, nil
					}
					decision, err := completeOrAdvanceDecision(base, facts, "current workspace state was validated and assessed after a no-op")
					return decision, true, err
				}
				decision, err := repairableAssessmentDecision(base, facts, assessment, proofLineage)
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
	decision, err := validationOnlyRepairDecision(base, facts, proofLineage)
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

func hasAcceptedCurrentFocusValidationEvidence(facts runtimecontract.RuntimeFacts, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) bool {
	for _, evidence := range facts.Evidence.Accepted {
		if evidence.Kind != runtimecontract.EvidenceKindValidation || weakRuntimeEvidence(evidence) {
			continue
		}
		// This predicate is used only by the state-change no-op provenance
		// terminal. An exact FocusID-bound validation observation proves the
		// diagnostic was made for this focus, but does not grant CWI credit.
		if runtimecontract.EvidenceBelongsToCurrentFocusForDiagnosticLineage(facts, evidence) {
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
	// failSemanticContract is the only producer of this code. It has already
	// exhausted (or deliberately declined) the bounded semantic transaction;
	// keep the producer's typed class for audit while preventing generic owner
	// feasibility/governance policy from turning it into an unrelated repair.
	if strings.TrimSpace(failure.Code) == "semantic_contract_failed" {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "semantic_contract_failed", failure.Message), true, nil
	}
	// An exact Profile route miss is already a complete, deterministic SM-2
	// outcome. Preserve that owner-local reason instead of collapsing it into
	// the generic repair-budget terminal used for attempted repairs.
	if strings.TrimSpace(failure.Code) == "validation_failure_repair_route_unavailable" {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "validation_failure_repair_route_unavailable", failure.Message), true, nil
	}
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
		// Persistence-owner availability is SM-4 re-drive state. It never spends
		// or exhausts replay authority for a claimed capability operation, and it
		// cannot select a business terminal here.
		return runtimecontract.RuntimeDecision{}, false, nil
	case runtimecontract.FailureClassGovernanceDependency, runtimecontract.FailureClassGovernanceRequired,
		runtimecontract.FailureClassProviderAuthentication, runtimecontract.FailureClassProviderConfiguration:
		decision, err := governanceDecision(base, facts, failure.Message)
		return decision, true, err
	case runtimecontract.FailureClassOperationOutcomeUnknown:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "operation_outcome_unknown", failure.Message), true, nil
	case runtimecontract.FailureClassExecutionBudgetExhausted:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "execution_budget_exhausted", failure.Message), true, nil
	case runtimecontract.FailureClassRecoverableWaitExpired:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "recoverable_wait_expired", failure.Message), true, nil
	case runtimecontract.FailureClassRecoverableWaitExhausted:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "recoverable_wait_exhausted", failure.Message), true, nil
	case runtimecontract.FailureClassRuntimeContractMisalignment:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "runtime_contract_defect", failure.Message), true, nil
	case runtimecontract.FailureClassProviderProtocol:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "provider_protocol_failure", failure.Message), true, nil
	case runtimecontract.FailureClassProviderSubmissionUnknown:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "provider_submission_unknown", failure.Message), true, nil
	case runtimecontract.FailureClassCandidateExhausted:
		// SM-3 has already sealed the total candidate disposition. Preserve only
		// the two typed exhaustion outcomes; unknown codes fail closed instead of
		// being reinterpreted from diagnostic text as a retry exhaustion.
		switch strings.TrimSpace(failure.Code) {
		case "candidate_authority_exhausted":
			// Backward-compatible replay for aggregates persisted before the
			// authority/retry failure-class split. New producers use the dedicated
			// FailureClassCandidateAuthorityExhausted class below.
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "candidate_authority_exhausted", failure.Message), true, nil
		case "candidate_retry_exhausted":
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "candidate_retry_exhausted", failure.Message), true, nil
		default:
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "runtime_contract_defect", failure.Message), true, nil
		}
	case runtimecontract.FailureClassCandidateAuthorityExhausted:
		if strings.TrimSpace(failure.Code) != "candidate_authority_exhausted" {
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "runtime_contract_defect", failure.Message), true, nil
		}
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "candidate_authority_exhausted", failure.Message), true, nil
	case runtimecontract.FailureClassCandidateDocumentRecoveryExhausted:
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "candidate_document_recovery_exhausted", failure.Message), true, nil
	case runtimecontract.FailureClassCandidateInfeasible:
		// A resource-owner rejection of the exact validation-derived repair surface
		// is a typed infeasibility terminal. It did not consume a repair budget,
		// so it must not be relabeled as no_progress_exhausted or re-enter the
		// same-focus repair loop.
		if strings.TrimSpace(failure.Code) == "repair_surface_unavailable" {
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "repair_surface_unavailable", failure.Message), true, nil
		}
		return runtimecontract.RuntimeDecision{}, false, nil
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

func feasibilityBlocksCurrentAction(facts runtimecontract.RuntimeFacts, proofLineage []runtimecontract.RuntimeCoverageProofLineage) bool {
	if facts.Feasibility == nil {
		return false
	}
	fact := facts.Feasibility
	if fact.FocusID != facts.Focus.ID || len(fact.FeasibleOperationKinds) != 0 {
		return false
	}
	missing := missingEvidenceKinds(facts, proofLineage)
	return (fact.State == runtimecontract.ConvergenceStateNeedStateChange && contains(missing, runtimecontract.EvidenceKindStateChange)) ||
		(fact.State == runtimecontract.ConvergenceStateNeedValidation && contains(missing, runtimecontract.EvidenceKindValidation))
}

func currentFocusCoverageSatisfied(facts runtimecontract.RuntimeFacts, _ ...[]runtimecontract.RuntimeCoverageProofLineage) bool {
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
	// A semantic validation binding deliberately grants CWI credit only to its
	// validation operation. When that owner returns a receipt-backed failure
	// target, a durable repair is a non-credit continuation and therefore cannot
	// be selected from the binding's proof kinds. Derive it from the already
	// persisted activation capability plus the current capability-owner catalog;
	// candidate feasibility, receipt and evidence fences still apply afterward.
	if len(acceptedKinds) == 0 && needsStateChangeForOwnerGroundedFailure(facts) {
		acceptedKinds = ownerGroundedFailureRepairKinds(facts)
	}
	if len(acceptedKinds) == 0 {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "capability_unavailable", "state-change authority has no concrete runtime capability"), nil
	}
	// An empty-scope creation fact is owner-grounded permission to propose one
	// prospective target, not a general durable-operation grant. Fail before the
	// provider when the matching owner catalog cannot accept a new target for
	// any sealed durable kind; otherwise retries can only cycle through owner
	// rejections.
	if hasOwnerGroundedProspectiveCreationScope(facts) {
		acceptedKinds = creationScopeAdmissibleOperationKinds(facts, acceptedKinds)
		if len(acceptedKinds) == 0 {
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed,
				"creation_scope_capability_unavailable", "prospective creation scope has no owner-admissible durable operation"), nil
		}
	}
	base.State = runtimecontract.ConvergenceStateNeedStateChange
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		AcceptedKinds:             acceptedKinds,
		AcceptedOperationVariants: acceptedOperationVariantsForKinds(facts.Focus.AcceptedOperationVariants, acceptedKinds),
		AcceptedEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
		RequiredEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
		RequiredEvidenceKinds:     []string{runtimecontract.EvidenceKindStateChange},
		CoverageWorkItemIDs:       facts.Focus.CoverageWorkItemIDs,
		TargetRefs:                GroundedRepairTargets(facts),
		RepairClosureID:           currentRepairClosureID(facts),
		RepairPurpose:             currentRepairPurpose(facts),
		RequiredOperationSequence: requiredStateChangeOperationSequence(facts, acceptedKinds),
	}
	return base, nil
}

func ownerGroundedFailureRepairKinds(facts runtimecontract.RuntimeFacts) []string {
	if !needsStateChangeForOwnerGroundedFailure(facts) || facts.Resources.FailureTarget == nil ||
		facts.Resources.FailureTarget.TargetObservation == nil {
		return nil
	}
	target := facts.Resources.FailureTarget
	repairOwner := ownerGroundedFailureRepairOwner(facts, *target)
	if repairOwner == "" {
		return nil
	}
	targetRefs := target.TargetRefs
	ret := make([]string, 0, len(facts.Capabilities.StateChangeKinds))
	for _, rawKind := range facts.Capabilities.StateChangeKinds {
		kind := runtimecontract.CanonicalRuntimeBlockKind(rawKind)
		if kind == "" || runtimeStringIncluded(facts.Focus.RejectedStepKinds, kind) {
			continue
		}
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(kind)
		if !found || descriptor.OwnerModuleID != repairOwner || !runtimecontract.RuntimeEffectIsDurable(descriptor.DefaultEffectScope) {
			continue
		}
		inventory, found := runtimeOperationAdmissibilityForKind(facts.Resources.OperationAdmissibilities, descriptor.OwnerModuleID, kind)
		if !found || !runtimePrerequisiteSatisfied(facts.Prerequisites.Satisfied, inventory.PrerequisiteID) {
			continue
		}
		if _, found := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, inventory.PrerequisiteID); !found {
			continue
		}
		if !runtimeTargetsIncluded(inventory.TargetRefs, targetRefs) || !runtimeInventoryAllowsExactTarget(inventory, kind) {
			continue
		}
		ret = append(ret, kind)
	}
	return ret
}

// ownerGroundedFailureRepairOwner keeps diagnostic provenance separate from
// effect ownership. A directly confirmed observation names its resource owner.
// A receipt-evidence observation instead names the capability that produced
// the diagnostic; its repair owner is the unique owner of the exact, current
// target snapshots already sealed in the aggregate. The reducer never selects
// an owner from a capability-specific constant.
func ownerGroundedFailureRepairOwner(facts runtimecontract.RuntimeFacts, target runtimecontract.FailureTarget) string {
	if target.TargetObservation == nil {
		return ""
	}
	observation := target.TargetObservation.Normalized()
	if observation.OwnerModuleID == "" {
		return ""
	}
	if observation.CatalogDependency == nil {
		return observation.OwnerModuleID
	}
	repairOwner := ""
	for _, targetRef := range target.TargetRefs {
		found := false
		for _, snapshot := range facts.Resources.TargetSnapshots {
			if snapshot.TargetRef == targetRef && snapshot.Complete() {
				found = true
				break
			}
		}
		if !found {
			return ""
		}
		owner := runtimeTargetSnapshotOwner(facts, targetRef)
		if owner == "" || repairOwner != "" && owner != repairOwner {
			return ""
		}
		repairOwner = owner
	}
	if len(target.TargetRefs) == 0 || repairOwner == "" {
		return ""
	}
	return repairOwner
}

func runtimeTargetSnapshotOwner(facts runtimecontract.RuntimeFacts, targetRef string) string {
	targetRef = strings.TrimSpace(targetRef)
	owner := ""
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		prerequisite.ID = strings.TrimSpace(prerequisite.ID)
		prerequisite.OwnerModuleID = strings.TrimSpace(prerequisite.OwnerModuleID)
		prerequisite.SubjectRef = strings.TrimSpace(prerequisite.SubjectRef)
		if prerequisite.Kind != runtimecontract.PrerequisiteKindTargetSnapshot || prerequisite.SubjectRef != targetRef || prerequisite.OwnerModuleID == "" {
			continue
		}
		observation, found := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisite.ID)
		if !found || observation.Kind != runtimecontract.PrerequisiteKindTargetSnapshot || observation.OwnerModuleID != prerequisite.OwnerModuleID {
			continue
		}
		if owner != "" && owner != prerequisite.OwnerModuleID {
			return ""
		}
		owner = prerequisite.OwnerModuleID
	}
	return owner
}

func runtimeInventoryAllowsExactTarget(inventory runtimecontract.RuntimeOperationAdmissibility, kind string) bool {
	inventory = inventory.Normalized()
	if len(inventory.OperationVariants) == 0 {
		allowed, _, _, _, err := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicy(
			inventory, kind, "", runtimecontract.RuntimeOperationTargetPolicyExactTarget,
		)
		return err == nil && allowed
	}
	for _, variant := range inventory.OperationVariants {
		if variant.Kind != runtimecontract.CanonicalRuntimeBlockKind(kind) {
			continue
		}
		allowed, _, _, found, err := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicy(
			inventory, kind, variant.VariantID, runtimecontract.RuntimeOperationTargetPolicyExactTarget,
		)
		if err == nil && found && allowed {
			return true
		}
	}
	return false
}

func runtimePrerequisiteSatisfied(values []runtimecontract.RuntimePrerequisite, prerequisiteID string) bool {
	for _, value := range values {
		if value.ID == prerequisiteID {
			return true
		}
	}
	return false
}

func runtimeTargetsIncluded(available, required []string) bool {
	for _, target := range required {
		if !runtimeStringIncluded(available, target) {
			return false
		}
	}
	return len(required) > 0
}

func runtimeStringIncluded(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// creationScopeAdmissibleOperationKinds checks only the committed target-owner
// inventory. It never chooses a target or infers a create grant from an
// owner identity, descriptor effect, or broad selector union. For a static
// operation kind such as patch/apply, every currently sealed variant must have
// the same explicit prospective-target fact; a mixed variant set must be
// narrowed by semantic binding before it can reach candidate rendering.
//
// Keeping that interpretation in SM-2 prevents an empty workspace from
// becoming authority for unrelated owner side effects, while still allowing
// the target owner to publish one concrete creation operation variant.
func creationScopeAdmissibleOperationKinds(facts runtimecontract.RuntimeFacts, acceptedKinds []string) []string {
	selectionOwnerModuleID := ""
	if selection := currentFocusTargetSelection(facts); selection != nil && selection.OwnerModuleID != "" {
		selectionOwnerModuleID = selection.OwnerModuleID
	}
	semanticScope := selectionOwnerModuleID == "" && hasSemanticOwnerGroundedTargetScope(facts)
	if selectionOwnerModuleID == "" && !semanticScope {
		return nil
	}
	accepted := make(map[string]struct{}, len(acceptedKinds))
	for _, kind := range acceptedKinds {
		accepted[runtimecontract.CanonicalRuntimeBlockKind(kind)] = struct{}{}
	}
	admissible := make(map[string]struct{}, len(acceptedKinds))
	for _, inventory := range facts.Resources.OperationAdmissibilities {
		inventory = inventory.Normalized()
		if selectionOwnerModuleID != "" && inventory.OwnerModuleID != selectionOwnerModuleID {
			continue
		}
		for _, kind := range inventory.OperationKinds {
			kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
			if _, permitted := accepted[kind]; !permitted {
				continue
			}
			descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(kind)
			if !known || descriptor.OwnerModuleID != inventory.OwnerModuleID || !runtimecontract.RuntimeEffectIsDurable(descriptor.DefaultEffectScope) {
				continue
			}
			variantIDs := creationScopeOperationVariantIDs(facts, kind)
			if !creationScopeOperationVariantsAreDurable(kind, variantIDs) {
				continue
			}
			if semanticScope && !runtimeSemanticBindingAllowsProspectiveOperation(facts, inventory, kind, variantIDs) {
				continue
			}
			allowed, _, _, _, err := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicyForVariants(
				inventory, kind, variantIDs, runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			)
			if err == nil && allowed {
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

func acceptedOperationVariantsForKinds(values []runtimecontract.RuntimeOperationVariantRef, kinds []string) []runtimecontract.RuntimeOperationVariantRef {
	accepted := make(map[string]struct{}, len(kinds))
	for _, kind := range kinds {
		if canonical := runtimecontract.CanonicalRuntimeBlockKind(kind); canonical != "" {
			accepted[canonical] = struct{}{}
		}
	}
	ret := make([]runtimecontract.RuntimeOperationVariantRef, 0, len(values))
	for _, value := range runtimecontract.NormalizeRuntimeOperationVariantRefs(values) {
		if _, found := accepted[runtimecontract.CanonicalRuntimeBlockKind(value.Kind)]; found {
			ret = append(ret, value)
		}
	}
	return ret
}

func creationScopeOperationVariantIDs(facts runtimecontract.RuntimeFacts, kind string) []string {
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	focus := facts.Focus.Normalized()
	seen := make(map[string]struct{})
	ret := make([]string, 0)
	for _, value := range focus.AcceptedOperationVariants {
		if runtimecontract.CanonicalRuntimeBlockKind(value.Kind) != kind || strings.TrimSpace(value.VariantID) == "" {
			continue
		}
		if _, duplicate := seen[value.VariantID]; duplicate {
			continue
		}
		seen[value.VariantID] = struct{}{}
		ret = append(ret, strings.TrimSpace(value.VariantID))
	}
	for _, snapshot := range facts.Resources.CapabilityStates {
		if snapshot.Disposition != runtimecontract.RuntimeCapabilityStateSequenceRequired {
			continue
		}
		for _, value := range snapshot.RequiredOperationSequence {
			if runtimecontract.CanonicalRuntimeBlockKind(value.Kind) != kind || strings.TrimSpace(value.VariantID) == "" {
				continue
			}
			variantID := strings.TrimSpace(value.VariantID)
			if _, duplicate := seen[variantID]; duplicate {
				continue
			}
			seen[variantID] = struct{}{}
			ret = append(ret, variantID)
		}
	}
	sort.Strings(ret)
	return ret
}

func creationScopeOperationVariantsAreDurable(kind string, variantIDs []string) bool {
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(kind)
	if !found || !runtimecontract.RuntimeEffectIsDurable(descriptor.DefaultEffectScope) {
		return false
	}
	contract, hasContract := runtimecontract.RuntimeOperationContractForKind(kind)
	if !hasContract || len(contract.Variants) == 0 {
		return len(variantIDs) == 0
	}
	if len(variantIDs) == 0 {
		return false
	}
	for _, variantID := range variantIDs {
		matched := false
		for _, variant := range contract.Variants {
			if variant.ID != variantID {
				continue
			}
			matched = true
			if !runtimecontract.RuntimeEffectIsDurable(variant.EffectScope) {
				return false
			}
			break
		}
		if !matched {
			return false
		}
	}
	return true
}

// requiredCapabilityStateSequence only contributes a capability owner's
// sequence when the current focus admits that owner's operation kind. A
// persisted owner sequence must not make an unrelated focus emit that owner's
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

// requiredStateChangeOperationSequence combines independent capability-owner
// sequences with the target-complete sequence sealed in a semantic prospective
// binding. The latter turns one owner-fact acquisition attempt into one bounded
// candidate transaction: every target must have exactly one operation and the
// deterministic dependency chain prevents partial parallel dispatch.
func requiredStateChangeOperationSequence(facts runtimecontract.RuntimeFacts, acceptedKinds []string) []runtimecontract.RuntimeRequiredOperationVariant {
	ret := requiredCapabilityStateSequence(facts, acceptedKinds)
	accepted := make(map[string]struct{}, len(acceptedKinds))
	for _, kind := range acceptedKinds {
		accepted[runtimecontract.CanonicalRuntimeBlockKind(kind)] = struct{}{}
	}
	previousProspectiveRequirement := ""
	for _, binding := range facts.Focus.Normalized().CoverageEvidenceBindings {
		binding = binding.Normalized()
		if len(binding.ProspectiveTargetRefs) == 0 {
			continue
		}
		proof, found := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindStateChange)
		if !found || len(proof.AllowedOperationKinds) != 1 {
			continue
		}
		kind := runtimecontract.CanonicalRuntimeBlockKind(proof.AllowedOperationKinds[0])
		if _, found := accepted[kind]; !found {
			continue
		}
		variantID := ""
		if len(proof.AllowedOperationVariants) == 1 && runtimecontract.CanonicalRuntimeBlockKind(proof.AllowedOperationVariants[0].Kind) == kind {
			variantID = proof.AllowedOperationVariants[0].VariantID
		}
		bindingHash := binding.Hash
		if bindingHash == "" {
			bindingHash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
		}
		if len(bindingHash) > 12 {
			bindingHash = bindingHash[:12]
		}
		for index, targetRef := range binding.ProspectiveTargetRefs {
			requirementID := fmt.Sprintf("prospective-%s-target-%02d", bindingHash, index+1)
			dependencies := []string(nil)
			if previousProspectiveRequirement != "" {
				dependencies = []string{previousProspectiveRequirement}
			}
			ret = append(ret, runtimecontract.RuntimeRequiredOperationVariant{
				RequirementID: requirementID, Kind: kind, VariantID: variantID, TargetRef: targetRef,
				DependsOnRequirementIDs: dependencies,
				Reason:                  "complete the sealed prospective all-of target transaction",
			})
			previousProspectiveRequirement = requirementID
		}
	}
	return ret
}

func validationDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) (runtimecontract.RuntimeDecision, error) {
	if decision, blocked, err := targetGroundingDecision(base, facts, true, proofLineages...); err != nil || blocked {
		return decision, err
	}
	acceptedKinds := focusAcceptedKinds(facts.Capabilities.ValidationKinds, facts.Focus)
	if len(acceptedKinds) == 0 {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "capability_unavailable", "validation authority has no concrete runtime capability"), nil
	}
	base.State = runtimecontract.ConvergenceStateNeedValidation
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		AcceptedKinds:             acceptedKinds,
		AcceptedOperationVariants: acceptedOperationVariantsForKinds(facts.Focus.AcceptedOperationVariants, acceptedKinds),
		AcceptedEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession},
		RequiredEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession},
		RequiredEvidenceKinds:     []string{runtimecontract.EvidenceKindValidation},
		CoverageWorkItemIDs:       facts.Focus.CoverageWorkItemIDs,
		TargetRefs:                GroundedRepairTargets(facts),
		RepairClosureID:           currentRepairClosureID(facts),
		RepairPurpose:             currentRepairPurpose(facts),
	}
	return base, nil
}

func currentRepairClosureID(facts runtimecontract.RuntimeFacts) string {
	if facts.RepairDirective == nil || facts.RepairDirective.FocusID != facts.Focus.ID || facts.RepairDirective.Closure == nil {
		return ""
	}
	return facts.RepairDirective.Closure.ID
}

func currentRepairPurpose(facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeDispatchRepairPurpose {
	if facts.RepairDirective == nil || facts.RepairDirective.FocusID != facts.Focus.ID || facts.RepairDirective.Closure == nil {
		return ""
	}
	return facts.RepairDirective.Normalized().Purpose
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
func targetGroundingDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts, allowAcceptedStateChangeLineage bool, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) (runtimecontract.RuntimeDecision, bool, error) {
	_, groundingRequired := runtimeCurrentTargetGroundingFocus(facts)
	if !groundingRequired || runtimeTargetAuthorityPlanForFacts(facts).grounded() ||
		(allowAcceptedStateChangeLineage && hasCurrentFocusTargetBoundStateChangeEvidence(facts, proofLineages...)) {
		return runtimecontract.RuntimeDecision{}, false, nil
	}
	const summary = "direct-evidence focus requires an owner-grounded target; workspace discovery alone did not select one"
	return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "target_grounding_unresolved", summary), true, nil
}

// hasSemanticOwnerGroundedTargetScope recognizes a capability scope only when
// the current SM-2-sealed binding carries the exact owner catalog snapshot,
// operation selector and semantic-feasibility lineage. TargetSelectorOwnerRef
// is the Profile/Definition declaration owner and is deliberately not used as
// the capability owner; the latter is derived from each sealed proof operation
// kind and must match the catalog owner. The provider-authored selector alone
// is insufficient: the matching persisted owner inventory is the grounding
// fact.
//
// Prospective targets are deliberately stricter than existing owner scopes.
// An empty scope is grounded only when the sealed binding and current owner
// catalog agree on one concrete prospective target and the catalog explicitly
// advertises its bounded selector/constraint fact.
func hasSemanticOwnerGroundedTargetScope(facts runtimecontract.RuntimeFacts) bool {
	focus := facts.Focus.Normalized()
	if focus.TargetSelectorOwnerRef == "" || focus.TargetSelectorRef == "" {
		return false
	}
	for _, binding := range focus.CoverageEvidenceBindings {
		binding = binding.Normalized()
		if binding.TargetSelectorOwnerRef != focus.TargetSelectorOwnerRef || binding.TargetSelectorRef != focus.TargetSelectorRef {
			continue
		}
		for _, catalog := range facts.Resources.OperationAdmissibilities {
			catalog = catalog.Normalized()
			if !runtimeStringsContain(catalog.SelectorRefs, focus.TargetSelectorRef) {
				continue
			}
			if !runtimeBindingSealsSemanticCatalog(binding, catalog) || !runtimeBindingHasCatalogOwnedOperation(binding, catalog) {
				continue
			}
			if len(focus.ProspectiveTargetRefs) == 0 {
				return true
			}
			if len(focus.TargetRefs) != 0 ||
				binding.TargetSelectorSubjectRef == "" || binding.TargetSelectorSubjectRef != focus.TargetSelectorSubjectRef ||
				len(binding.ProspectiveTargetRefs) == 0 || !runtimecontract.SameRuntimeTargetSelectorCandidateRefs(binding.ProspectiveTargetRefs, focus.ProspectiveTargetRefs) || len(binding.TargetRefs) != 0 ||
				!runtimeBindingProspectiveOperationAllowed(catalog, binding) {
				continue
			}
			return true
		}
	}
	return false
}

func runtimeBindingSealsSemanticCatalog(binding runtimecontract.CoverageEvidenceBinding, catalog runtimecontract.RuntimeOperationAdmissibility) bool {
	hasCatalogSnapshot := false
	hasSemanticFeasibility := false
	for _, ref := range binding.Normalized().PrerequisiteRefs {
		if ref == catalog.SnapshotID {
			hasCatalogSnapshot = true
		}
		if strings.HasPrefix(ref, "semantic-feasibility/") && strings.TrimPrefix(ref, "semantic-feasibility/") != "" {
			hasSemanticFeasibility = true
		}
	}
	return hasCatalogSnapshot && hasSemanticFeasibility
}

func runtimeBindingHasCatalogOwnedOperation(binding runtimecontract.CoverageEvidenceBinding, catalog runtimecontract.RuntimeOperationAdmissibility) bool {
	catalog = catalog.Normalized()
	for _, proof := range binding.Normalized().Proofs {
		for _, kind := range proof.AllowedOperationKinds {
			kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
			descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(kind)
			if found && descriptor.OwnerModuleID == catalog.OwnerModuleID && runtimeStringsContain(catalog.OperationKinds, kind) {
				return true
			}
		}
	}
	return false
}

func runtimeBindingProspectiveOperationAllowed(catalog runtimecontract.RuntimeOperationAdmissibility, binding runtimecontract.CoverageEvidenceBinding) bool {
	for _, proof := range binding.Proofs {
		proof = proof.Normalized()
		if proof.EvidenceKind != runtimecontract.EvidenceKindStateChange {
			continue
		}
		for _, kind := range proof.AllowedOperationKinds {
			kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
			descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(kind)
			if !found || descriptor.OwnerModuleID != catalog.OwnerModuleID {
				continue
			}
			variantIDs := make([]string, 0)
			for _, ref := range proof.AllowedOperationVariants {
				if runtimecontract.CanonicalRuntimeBlockKind(ref.Kind) == kind && strings.TrimSpace(ref.VariantID) != "" {
					variantIDs = append(variantIDs, strings.TrimSpace(ref.VariantID))
				}
			}
			if runtimeBindingProofAllowsProspectiveOperation(catalog, binding, kind, variantIDs) {
				return true
			}
		}
	}
	return false
}

func runtimeSemanticBindingAllowsProspectiveOperation(facts runtimecontract.RuntimeFacts, catalog runtimecontract.RuntimeOperationAdmissibility, kind string, variantIDs []string) bool {
	focus := facts.Focus.Normalized()
	for _, binding := range focus.CoverageEvidenceBindings {
		binding = binding.Normalized()
		if binding.TargetSelectorOwnerRef != focus.TargetSelectorOwnerRef || binding.TargetSelectorRef != focus.TargetSelectorRef ||
			binding.TargetSelectorSubjectRef == "" || binding.TargetSelectorSubjectRef != focus.TargetSelectorSubjectRef ||
			len(binding.TargetRefs) != 0 || len(binding.ProspectiveTargetRefs) == 0 ||
			!runtimecontract.SameRuntimeTargetSelectorCandidateRefs(binding.ProspectiveTargetRefs, focus.ProspectiveTargetRefs) ||
			!runtimeBindingSealsSemanticCatalog(binding, catalog) {
			continue
		}
		for _, proof := range binding.Proofs {
			proof = proof.Normalized()
			if proof.EvidenceKind != runtimecontract.EvidenceKindStateChange || !runtimeStringsContain(proof.AllowedOperationKinds, kind) ||
				!runtimeProofAllowsOperationVariants(proof, kind, variantIDs) {
				continue
			}
			if runtimeBindingProofAllowsProspectiveOperation(catalog, binding, kind, variantIDs) {
				return true
			}
		}
	}
	return false
}

func runtimeBindingProofAllowsProspectiveOperation(catalog runtimecontract.RuntimeOperationAdmissibility, binding runtimecontract.CoverageEvidenceBinding, kind string, variantIDs []string) bool {
	kind = runtimecontract.CanonicalRuntimeBlockKind(kind)
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(kind)
	if !found || descriptor.OwnerModuleID != catalog.OwnerModuleID || !runtimeStringsContain(catalog.OperationKinds, kind) {
		return false
	}
	allowed, selectors, constraints, _, err := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicyForVariants(
		catalog, kind, variantIDs, runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
	)
	return err == nil && allowed && runtimeStringsContain(selectors, binding.TargetSelectorRef) && len(constraints) != 0
}

func runtimeProofAllowsOperationVariants(proof runtimecontract.CoverageEvidenceProof, kind string, variantIDs []string) bool {
	allowedVariants := make(map[string]struct{})
	for _, ref := range proof.Normalized().AllowedOperationVariants {
		if runtimecontract.CanonicalRuntimeBlockKind(ref.Kind) == kind {
			allowedVariants[ref.VariantID] = struct{}{}
		}
	}
	if len(allowedVariants) == 0 {
		return true
	}
	for _, variantID := range variantIDs {
		if _, allowed := allowedVariants[strings.TrimSpace(variantID)]; !allowed {
			return false
		}
	}
	return len(variantIDs) != 0
}

func hasOwnerGroundedProspectiveCreationScope(facts runtimecontract.RuntimeFacts) bool {
	return runtimeTargetAuthorityPlanForFacts(facts).hasProspectiveTargets()
}

func runtimeStringsContain(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// hasOwnerGroundedCreationScope is the empty-scope counterpart to an existing
// target snapshot. It is true only for a persisted owner creation observation;
// the reducer never derives a target from Goal, DoD, inventory ordering, or
// provider text.
func hasOwnerGroundedCreationScope(facts runtimecontract.RuntimeFacts) bool {
	return grounding.HasOwnerGroundedCreationScope(facts)
}

func hasOwnerGroundedTargetScope(facts runtimecontract.RuntimeFacts) bool {
	return grounding.HasOwnerGroundedTargetScope(facts)
}

// targetSelectionHasConcreteTarget is intentionally narrower than an owner
// scope. A selected value is an owner-grounded concrete repair surface; a
// scope is only a bounded owner feasibility inventory and must not enter facts
// that are interpreted as a repair target.
func targetSelectionHasConcreteTarget(selection runtimecontract.RuntimeTargetSelection) bool {
	return grounding.TargetSelectionHasConcreteTarget(selection)
}

func hasCurrentFocusTargetBoundStateChangeEvidence(facts runtimecontract.RuntimeFacts, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) bool {
	proofLineage := runtimeCoverageProofLineageArgument(proofLineages)
	for _, evidence := range facts.Evidence.Accepted {
		if evidence.Kind == runtimecontract.EvidenceKindStateChange && len(evidence.TargetRefs) != 0 && !weakRuntimeEvidence(evidence) &&
			runtimeEvidenceAppliesToCurrentCoverageWithLineage(facts, evidence, proofLineage) {
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
	target := facts.Resources.ProviderCeiling.Selection.Target.Normalized()
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

func missingEvidenceKinds(facts runtimecontract.RuntimeFacts, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) []string {
	proofLineage := runtimeCoverageProofLineageArgument(proofLineages)
	if len(proofLineage) == 0 {
		return runtimecontract.MissingRequiredEvidenceKinds(facts)
	}
	facts = facts.Normalized()
	missing := make([]string, 0, len(facts.Focus.RequiredEvidenceKinds))
	for _, evidenceKind := range facts.Focus.RequiredEvidenceKinds {
		evidenceKind = strings.TrimSpace(evidenceKind)
		if evidenceKind == "" || runtimeCurrentFocusHasEvidenceKindWithLineage(facts, evidenceKind, proofLineage) {
			continue
		}
		missing = append(missing, evidenceKind)
	}
	return missing
}

func runtimeCurrentFocusHasEvidenceKindWithLineage(facts runtimecontract.RuntimeFacts, evidenceKind string, proofLineage []runtimecontract.RuntimeCoverageProofLineage) bool {
	if len(facts.Focus.CoverageWorkItemIDs) == 0 {
		return false
	}
	for _, evidence := range facts.Evidence.Accepted {
		if strings.TrimSpace(evidence.Kind) != evidenceKind {
			continue
		}
		if runtimeAcceptedEvidenceCreditsCurrentFocusWithLineage(facts, evidence, proofLineage) {
			return true
		}
	}
	return false
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

func advanceSatisfiedCoverageFocus(facts runtimecontract.RuntimeFacts, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) (runtimecontract.RuntimeFacts, error) {
	facts = facts.Normalized()
	if len(missingEvidenceKinds(facts, proofLineages...)) != 0 || !currentFocusCoverageSatisfied(facts, proofLineages...) || len(facts.RemainingFocuses) == 0 {
		return facts, nil
	}
	// A settled current CWI may leave every remaining row dependency-blocked.
	// Preserve that settled cursor for the typed dependency/terminal decision;
	// do not duplicate it in CompletedFocuses or fabricate an unbound focus.
	if len(runtimecontract.RuntimeCWIsEligibleForPlanning(facts)) == 0 {
		return facts, nil
	}
	completedFocusID := facts.Focus.ID
	completedFocus := facts.Focus.Normalized()
	retireFocusScopedPrerequisites(&facts, completedFocusID)
	facts.Failure = nil
	facts.Feasibility = nil
	// Target selections and snapshots are scoped to the completed Focus. A
	// profile bootstrap recipe intentionally advances through several sealed
	// artifact targets; retaining the prior owner observation would make the
	// next Focus reuse its predecessor's path and violate its own selector
	// binding. Manifest and capability inventories remain valid owner facts and
	// are refreshed normally when their revisions change.
	facts.Resources.TargetSelection = nil
	facts.Resources.TargetSnapshots = nil
	facts.Resources.FailureTarget = nil
	facts.RepairDirective = nil
	return projectRuntimeCWIExecutionFocus(facts, nil, &completedFocus)
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
func repairableAssessmentDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts, assessment runtimecontract.RuntimeAssessmentFact, proofLineage []runtimecontract.RuntimeCoverageProofLineage) (runtimecontract.RuntimeDecision, error) {
	policy := facts.ConvergencePolicy.Normalized()
	ledger := facts.Repair.Normalized()
	if len(runtimecontract.RuntimeMissingOwnerFactRequirements(facts.Focus.RequiredOwnerFacts, facts.Resources.OperationAdmissibilities)) != 0 {
		acquisition, acquisitionReady := runtimecontract.ResolveRuntimeOwnerFactAcquisition(facts.Focus.RequiredOwnerFacts, facts.Focus.OwnerFactConvergences, facts.Resources.OperationAdmissibilities)
		if !acquisitionReady || runtimeOwnerFactReplanBudgetExhausted(facts, acquisition) {
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "owner_fact_convergence_exhausted", "required capability-owner fact remained unavailable after the bounded semantic replan budget"), nil
		}
		// Production assessment commit normally reopens the CWI and retires its
		// semantic generation in the same CAS before reaching this reducer. This
		// fallback keeps direct reducer fixtures fail-closed and chooses only the
		// durable branch already declared by the current focus.
		return stateChangeDecision(base, facts)
	}
	if ledger.Exhausted || ledger.RepairAttempts >= policy.MaxRepairAttemptsPerFocus {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "no_progress_exhausted", firstNonEmptyAssessmentSummary(assessment, "repair budget exhausted")), nil
	}
	if !hasAcceptedCurrentFocusValidationEvidence(facts, proofLineage) {
		return validationOnlyRepairDecision(base, facts, proofLineage)
	}
	if focusRequiresDurableStateChangeRepair(facts) {
		return stateChangeDecision(base, facts)
	}
	return validationOnlyRepairDecision(base, facts, proofLineage)
}

func validationOnlyRepairDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) (runtimecontract.RuntimeDecision, error) {
	if validationOnlyRepairBudgetExhausted(facts) {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "no_progress_exhausted", "validation-only repair budget exhausted"), nil
	}
	return validationDecision(base, facts, proofLineages...)
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

func assessmentForCurrentFocus(facts runtimecontract.RuntimeFacts, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) (runtimecontract.RuntimeAssessmentFact, bool) {
	// Assessments are immutable audit facts. A later accepted ordinary owner
	// receipt for this focus makes every assessment at or before its revision
	// stale, while a subsequent assessment supersedes earlier assessments by the
	// same global evidence sequence used across operation and governance facts.
	proofLineage := runtimeCoverageProofLineageArgument(proofLineages)
	latestAcceptedRevision := ""
	for _, evidence := range facts.Evidence.Accepted {
		if evidence.Kind == runtimecontract.EvidenceKindGovernance ||
			!runtimeAcceptedEvidenceCreditsCurrentFocusWithLineage(facts, evidence, proofLineage) {
			continue
		}
		if latestAcceptedRevision == "" || runtimecontract.EvidenceRevisionAfter(evidence.Revision, latestAcceptedRevision) {
			latestAcceptedRevision = evidence.Revision
		}
	}
	found := false
	var latest runtimecontract.RuntimeAssessmentFact
	for _, assessment := range facts.Assessments {
		if assessment.FocusID != facts.Focus.ID {
			continue
		}
		if latestAcceptedRevision != "" && !runtimecontract.EvidenceRevisionAfter(assessment.Revision, latestAcceptedRevision) {
			continue
		}
		if !found || runtimecontract.EvidenceRevisionAfter(assessment.Revision, latest.Revision) ||
			(assessment.Revision == latest.Revision && assessment.ID > latest.ID) {
			latest = assessment
			found = true
		}
	}
	if !found {
		return runtimecontract.RuntimeAssessmentFact{}, false
	}
	return latest, true
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
