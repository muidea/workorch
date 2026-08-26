package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// syncRepairDirective persists the SM-2 repair continuation selected by this
// owner. Runtime contracts carry/validate the directive but never select or
// mutate a canonical convergence state.
//
// A prerequisite acquisition is an interlude in the same repair cycle. It
// must not discard the continuation: once acquisition commits, the reducer
// has to return to the sealed repair action rather than infer a new action
// from raw missing-evidence kinds.
func syncRepairDirective(facts runtimecontract.RuntimeFacts, decision runtimecontract.RuntimeDecision, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) runtimecontract.RuntimeFacts {
	facts = facts.Normalized()
	proofLineage := runtimeCoverageProofLineageArgument(proofLineages)
	decision.RequiredOutput = decision.RequiredOutput.Normalized()
	focusID := strings.TrimSpace(facts.Focus.ID)
	if focusID == "" {
		facts.RepairDirective = nil
		return facts
	}
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID != focusID {
		facts.RepairDirective = nil
	}
	// An accepted state-change repair has precedence over the ordinary
	// assessment boundary. Keep the same closure and require validation before
	// an assessment can clear it; otherwise SM-2 can lose the mandatory
	// state-change -> validation handoff in the same reduction.
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID == focusID &&
		facts.RepairDirective.RequiredState == runtimecontract.ConvergenceStateNeedStateChange &&
		acceptedStateChangeAfterRepairDirective(facts, facts.RepairDirective, proofLineage) {
		directive := facts.RepairDirective.Clone()
		// A repair directive's mutable supersedes revision is the evidence
		// barrier for its *next* required action. The closure keeps its original
		// baseline and identity; after the durable repair succeeds, validation
		// must be newer than that exact repair receipt rather than reusing a
		// validation fact that preceded it.
		directive.SupersedesEvidenceRevision = acceptedRepairStateChangeRevision(facts, &directive, proofLineage)
		directive.RequiredState = runtimecontract.ConvergenceStateNeedValidation
		directive.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession}
		directive.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
		// A directive's allowed kinds are action-scoped authority. Carrying the
		// durable operation inventory into the validation transaction would let
		// an old state-change shape survive the handoff and be rejected only at
		// candidate admission. Rebind it from the persisted focus/capability
		// facts while retaining the same repair closure and target policy.
		directive.AllowedOperationKinds = focusAcceptedKinds(facts.Capabilities.ValidationKinds, facts.Focus)
		// The state-change and validation dispatches are two SM-3 transactions,
		// but remain one SM-2 repair closure. Do not rebuild its target or gap
		// objective from the newly changed workspace.
		directive.RepairAttempt++
		facts.RepairDirective = &directive
		return facts
	}

	switch decision.State {
	case runtimecontract.ConvergenceStateComplete, runtimecontract.ConvergenceStateFailed, runtimecontract.ConvergenceStateNeedAssessment:
		facts.RepairDirective = nil
		return facts
	case runtimecontract.ConvergenceStateNeedPrerequisite, runtimecontract.ConvergenceStateNeedGovernance:
		// The persisted repair continuation is deliberately retained through a
		// prerequisite/governance interlude. No candidate is rendered for these
		// actions, so it cannot leak into the owner command.
		return facts
	}

	switch decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation:
		if facts.RepairDirective != nil || repairDirectiveRequired(facts) {
			directive := buildRepairDirective(facts, decision)
			facts.RepairDirective = &directive
		}
	}
	return facts
}

// repairDirectiveRequired distinguishes a normal evidence-producing action
// from a same-focus repair continuation. A normal state-change must not gain a
// repair directive merely because it is state-changing; otherwise its accepted
// evidence would incorrectly force a validation follow-up.
func repairDirectiveRequired(facts runtimecontract.RuntimeFacts) bool {
	if facts.Repair.ActiveGap != nil || facts.Repair.RepairAttempts > 0 {
		return true
	}
	// A receipt-backed, owner-grounded implementation/effect failure can begin
	// a repair before an assessment creates its gap. Candidate infeasibility is
	// deliberately excluded: it carries no durable repair target authority.
	return needsStateChangeForOwnerGroundedFailure(facts)
}

func applyRepairDirectiveAction(facts runtimecontract.RuntimeFacts, base runtimecontract.RuntimeDecision, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) (runtimecontract.RuntimeDecision, bool, error) {
	facts = facts.Normalized()
	proofLineage := runtimeCoverageProofLineageArgument(proofLineages)
	if facts.RepairDirective == nil {
		return base, false, nil
	}
	directive := facts.RepairDirective.Normalized()
	if directive.FocusID != facts.Focus.ID || repairDirectiveEvidenceSatisfied(facts, directive, proofLineage) {
		return base, false, nil
	}
	switch directive.RequiredState {
	case runtimecontract.ConvergenceStateNeedStateChange:
		decision, err := repairStateChangeDecision(base, facts, directive)
		return decision, true, err
	case runtimecontract.ConvergenceStateNeedValidation:
		if validationOnlyRepairBudgetExhausted(facts) {
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, "no_progress_exhausted", "validation-only repair budget exhausted"), true, nil
		}
		decision, err := repairValidationDecision(base, facts, directive)
		return decision, true, err
	default:
		return base, false, fmt.Errorf("repair directive has unsupported required state %q", directive.RequiredState)
	}
}

func repairDirectiveEvidenceSatisfied(facts runtimecontract.RuntimeFacts, directive runtimecontract.RuntimeRepairDirective, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) bool {
	proofLineage := runtimeCoverageProofLineageArgument(proofLineages)
	required := directive.RequiredEvidenceKinds
	if len(required) == 0 {
		switch directive.RequiredState {
		case runtimecontract.ConvergenceStateNeedStateChange:
			required = []string{runtimecontract.EvidenceKindStateChange}
		case runtimecontract.ConvergenceStateNeedValidation:
			required = []string{runtimecontract.EvidenceKindValidation}
		default:
			return false
		}
	}
	accepted := map[string]struct{}{}
	baseline := strings.TrimSpace(directive.SupersedesEvidenceRevision)
	for _, evidence := range facts.Evidence.Accepted {
		if len(evidence.CoverageWorkItemIDs) == 0 {
			if !runtimecontract.EvidenceAppliesToCurrentFocus(facts, evidence) {
				continue
			}
		} else if len(directive.CoverageWorkItemIDs) != 0 {
			if !runtimeEvidenceAppliesToCoverageIDsWithLineage(facts, evidence, directive.CoverageWorkItemIDs, proofLineage) {
				continue
			}
		} else if !runtimeEvidenceAppliesToCurrentCoverageWithLineage(facts, evidence, proofLineage) {
			continue
		}
		// A directive is a same-focus repair continuation. Evidence admitted
		// before its persisted barrier is audit lineage, never proof that the
		// newly selected repair action completed. In particular, the mandatory
		// validation following an accepted repair state-change must be fresh.
		if baseline != "" && !runtimecontract.EvidenceRevisionAfter(evidence.Revision, baseline) {
			continue
		}
		if evidence.Kind == runtimecontract.EvidenceKindStateChange && len(directive.TargetRefs) > 0 && !runtimeTargetRefsIntersect(directive.TargetRefs, evidence.TargetRefs) {
			continue
		}
		accepted[strings.TrimSpace(evidence.Kind)] = struct{}{}
	}
	for _, kind := range required {
		if _, ok := accepted[strings.TrimSpace(kind)]; !ok {
			return false
		}
	}
	return true
}

func buildRepairDirective(facts runtimecontract.RuntimeFacts, decision runtimecontract.RuntimeDecision) runtimecontract.RuntimeRepairDirective {
	trigger := runtimecontract.RuntimeRepairTriggerMissingEvidence
	if needsStateChangeForOwnerGroundedFailure(facts) {
		trigger = runtimecontract.RuntimeRepairTriggerRejectedValidation
	}
	if facts.Failure != nil {
		switch facts.Failure.Class {
		case runtimecontract.FailureClassCandidateInfeasible:
			trigger = runtimecontract.RuntimeRepairTriggerCandidateInfeasible
		case runtimecontract.FailureClassEffectMismatch:
			trigger = runtimecontract.RuntimeRepairTriggerEffectMismatch
		case runtimecontract.FailureClassExecutionRejected:
			trigger = runtimecontract.RuntimeRepairTriggerExecutionRejected
		}
	}
	attempt := 0
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID == facts.Focus.ID {
		attempt = facts.RepairDirective.RepairAttempt
		if facts.RepairDirective.RequiredState != decision.State {
			attempt++
		}
	} else if facts.Repair.RepairAttempts > 0 {
		attempt = facts.Repair.RepairAttempts
	}
	required := decision.RequiredOutput.Normalized()
	directive := runtimecontract.RuntimeRepairDirective{
		ID:                    fmt.Sprintf("repair-%s-%s-%d", facts.Focus.ID, decision.State, attempt),
		FocusID:               facts.Focus.ID,
		Trigger:               trigger,
		RequiredState:         decision.State,
		RequiredEffectScopes:  required.RequiredEffectScopes,
		RequiredEvidenceKinds: required.RequiredEvidenceKinds,
		AllowedOperationKinds: required.AcceptedKinds,
		CoverageWorkItemIDs:   required.CoverageWorkItemIDs,
		TargetRefs:            repairDirectiveTargets(facts, required.TargetRefs),
		RepairAttempt:         attempt,
	}
	if facts.Failure != nil {
		failure := *facts.Failure
		directive.CurrentFailure = &failure
	}
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID == facts.Focus.ID {
		directive.SupersedesEvidenceRevision = facts.RepairDirective.SupersedesEvidenceRevision
		directive.RejectedOperationShapes = append([]runtimecontract.RuntimeRejectedOperationShape(nil), facts.RepairDirective.RejectedOperationShapes...)
	}
	directive.NoProgressOperationShapes = append([]runtimecontract.RuntimeRejectedOperationShape(nil), facts.Repair.NoProgressOperationShapes...)
	if directive.SupersedesEvidenceRevision == "" {
		directive.SupersedesEvidenceRevision = facts.EvidenceRevision
	}
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID == facts.Focus.ID && facts.RepairDirective.Closure != nil {
		// A closure spans the durable repair and its mandatory fresh validation.
		// An active assessment gap remains audit evidence during that handoff; it
		// must not rebuild the closure with a different evidence family/baseline
		// and silently break the already-sealed RequiredOutput.RepairClosureID.
		closure := facts.RepairDirective.Closure.Clone()
		directive.Closure = &closure
		directive.TargetRefs = append([]string(nil), closure.GroundedTargetRefs...)
		directive.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), closure.CoverageWorkItemIDs...)
	} else if facts.Repair.ActiveGap != nil {
		gap := facts.Repair.ActiveGap.Normalized()
		closure := runtimecontract.RuntimeRepairClosure{
			GapSignature:                      firstNonEmpty(gap.FailureSignature, runtimecontract.StableAssessmentFailureSignature(facts.Focus.ID, runtimecontract.AssessmentDispositionRepairableGap, &gap)),
			BaselineEvidenceRevision:          directive.SupersedesEvidenceRevision,
			CoverageWorkItemIDs:               firstRepairCoverage(gap.CoverageWorkItemIDs, directive.CoverageWorkItemIDs),
			RequiredEvidenceKinds:             repairClosureRequiredEvidenceKinds(decision),
			GroundedTargetRefs:                directive.TargetRefs,
			RequireValidationAfterStateChange: decision.State == runtimecontract.ConvergenceStateNeedStateChange,
		}
		closure.ID = runtimecontract.StableRuntimeRepairClosureID(closure)
		directive.Closure = &closure
		// The closure is the same-focus repair authority. Its remaining coverage
		// replaces the broad focus fallback so a partially satisfied assessment
		// cannot dispatch another repair for items that already converged.
		directive.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), closure.CoverageWorkItemIDs...)
	}
	return directive.Normalized()
}

// repairClosureRequiredEvidenceKinds is canonical SM-2 repair sequencing. It
// never consumes an assessment candidate's claimed missing kinds as action
// authority. A durable repair always closes through validation in the same
// repair closure.
func repairClosureRequiredEvidenceKinds(decision runtimecontract.RuntimeDecision) []string {
	switch decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange:
		return []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	case runtimecontract.ConvergenceStateNeedValidation:
		return []string{runtimecontract.EvidenceKindValidation}
	default:
		return nil
	}
}

// repairDirectiveTargets is an SM-2 ownership rule: an explicit Focus target
// remains immutable focus declaration, while a persisted owner failure target
// may supersede it as the current repair surface. A File owner scope is
// deliberately excluded: it is a bounded feasibility inventory, not a
// concrete repair target, and must remain available for the next candidate to
// propose one concrete File target. The runtime core does not infer
// source/test roles from path names or provider text.
func repairDirectiveTargets(facts runtimecontract.RuntimeFacts, fallback []string) []string {
	if facts.RepairDirective != nil && facts.RepairDirective.Closure != nil && len(facts.RepairDirective.Closure.GroundedTargetRefs) > 0 {
		return append([]string(nil), facts.RepairDirective.Closure.GroundedTargetRefs...)
	}
	if facts.Resources.FailureTarget != nil && len(facts.Resources.FailureTarget.TargetRefs) > 0 {
		return append([]string(nil), facts.Resources.FailureTarget.TargetRefs...)
	}
	if len(facts.Focus.TargetRefs) > 0 {
		return append([]string(nil), facts.Focus.TargetRefs...)
	}
	if selection := currentFocusTargetSelection(facts); selection != nil && selection.Disposition == runtimecontract.RuntimeTargetSelectionSelected && len(selection.TargetRefs) > 0 {
		return append([]string(nil), selection.TargetRefs...)
	}
	if hasOwnerGroundedTargetScope(facts) {
		// The state-change decision retains the bounded scope only in Resources
		// for candidate context. It must not turn into directive authority through
		// this fallback; SM-3 still asks File to adjudicate the candidate's next
		// concrete target.
		return nil
	}
	return append([]string(nil), fallback...)
}

func acceptedStateChangeAfterRepairDirective(facts runtimecontract.RuntimeFacts, directive *runtimecontract.RuntimeRepairDirective, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) bool {
	return acceptedRepairStateChangeRevision(facts, directive, proofLineages...) != ""
}

// acceptedRepairStateChangeRevision returns the latest accepted durable repair
// effect that is newer than the directive's barrier and matches its sealed
// target set. It is an owner-local fact query; the returned revision advances
// the follow-up validation barrier without changing the immutable closure.
func acceptedRepairStateChangeRevision(facts runtimecontract.RuntimeFacts, directive *runtimecontract.RuntimeRepairDirective, proofLineages ...[]runtimecontract.RuntimeCoverageProofLineage) string {
	if directive == nil {
		return ""
	}
	proofLineage := runtimeCoverageProofLineageArgument(proofLineages)
	// Assessment is the final evidence-quality gate, after every required
	// evidence kind is present.  It must not keep a state-change repair
	// directive active when that repair has already produced a later accepted
	// durable effect and the focus still lacks validation: doing so would force
	// another produce_state_change candidate instead of returning to the
	// reducer's missing-validation branch.  Assessment-required focuses still
	// cannot close coverage until the normal assessment decision is reached.
	baseline := strings.TrimSpace(directive.SupersedesEvidenceRevision)
	requiredTargets := directive.TargetRefs
	latest := ""
	for _, evidence := range facts.Evidence.Accepted {
		if evidence.Kind != runtimecontract.EvidenceKindStateChange || weakRuntimeEvidence(evidence) ||
			(baseline != "" && !runtimecontract.EvidenceRevisionAfter(evidence.Revision, baseline)) {
			continue
		}
		if len(evidence.CoverageWorkItemIDs) == 0 {
			continue
		}
		if len(directive.CoverageWorkItemIDs) != 0 {
			if !runtimeEvidenceAppliesToCoverageIDsWithLineage(facts, evidence, directive.CoverageWorkItemIDs, proofLineage) {
				continue
			}
		} else if !runtimeEvidenceAppliesToCurrentCoverageWithLineage(facts, evidence, proofLineage) {
			continue
		}
		if len(requiredTargets) > 0 && !runtimeTargetRefsIntersect(requiredTargets, evidence.TargetRefs) {
			continue
		}
		if latest == "" || runtimecontract.EvidenceRevisionAfter(evidence.Revision, latest) {
			latest = evidence.Revision
		}
	}
	return latest
}

func repairStateChangeDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts, directive runtimecontract.RuntimeRepairDirective) (runtimecontract.RuntimeDecision, error) {
	acceptedKinds := directive.AllowedOperationKinds
	if len(acceptedKinds) == 0 {
		acceptedKinds = focusAcceptedKinds(facts.Capabilities.StateChangeKinds, facts.Focus)
	}
	if len(acceptedKinds) == 0 {
		return runtimecontract.RuntimeDecision{}, fmt.Errorf("repair directive produce_state_change has no accepted operation kinds")
	}
	base.State = runtimecontract.ConvergenceStateNeedStateChange
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		AcceptedKinds:             acceptedKinds,
		AcceptedOperationVariants: acceptedOperationVariantsForKinds(facts.Focus.AcceptedOperationVariants, acceptedKinds),
		AcceptedEffectScopes:      directive.RequiredEffectScopes, RequiredEffectScopes: directive.RequiredEffectScopes,
		RequiredEvidenceKinds:     []string{runtimecontract.EvidenceKindStateChange},
		CoverageWorkItemIDs:       firstRepairCoverage(directive.CoverageWorkItemIDs, facts.Focus.CoverageWorkItemIDs),
		TargetRefs:                firstRepairTargets(directive.TargetRefs, facts.Focus.TargetRefs),
		RepairClosureID:           repairClosureID(directive),
		RequiredOperationSequence: requiredCapabilityStateSequence(facts, acceptedKinds),
	}
	return base, nil
}

func repairValidationDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts, directive runtimecontract.RuntimeRepairDirective) (runtimecontract.RuntimeDecision, error) {
	acceptedKinds := directive.AllowedOperationKinds
	if len(acceptedKinds) == 0 {
		acceptedKinds = focusAcceptedKinds(facts.Capabilities.ValidationKinds, facts.Focus)
	}
	if len(acceptedKinds) == 0 {
		// The state-change -> validation handoff is a persisted SM-2 repair
		// continuation.  Re-entering it without a sealed validation recipe is
		// not a Go/process error and cannot be repaired by another candidate,
		// governance ticket, or retry of the same durable operation.  Close it
		// as a typed owner terminal so recovery can persist and deliver one
		// canonical result rather than repeatedly re-driving an invalid action.
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed,
			"validation_recipe_exhausted", "repair validation has no concrete runtime capability"), nil
	}
	base.State = runtimecontract.ConvergenceStateNeedValidation
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		AcceptedKinds:             acceptedKinds,
		AcceptedOperationVariants: acceptedOperationVariantsForKinds(facts.Focus.AcceptedOperationVariants, acceptedKinds),
		AcceptedEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession},
		RequiredEffectScopes:      directive.RequiredEffectScopes, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		CoverageWorkItemIDs: firstRepairCoverage(directive.CoverageWorkItemIDs, facts.Focus.CoverageWorkItemIDs),
		TargetRefs:          firstRepairTargets(directive.TargetRefs, facts.Focus.TargetRefs),
		RepairClosureID:     repairClosureID(directive),
	}
	return base, nil
}

func repairClosureID(directive runtimecontract.RuntimeRepairDirective) string {
	if directive.Closure == nil {
		return ""
	}
	return directive.Closure.ID
}

func firstRepairCoverage(primary, fallback []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func firstRepairTargets(primary, fallback []string) []string {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}
