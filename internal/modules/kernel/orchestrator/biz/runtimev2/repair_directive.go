package runtimev2

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// syncRepairDirective persists the SM-2 repair continuation selected by this
// owner. Runtime contracts carry/validate the directive but never select or
// mutate a RuntimeAction.
//
// A prerequisite acquisition is an interlude in the same repair cycle. It
// must not discard the continuation: once acquisition commits, the reducer
// has to return to the sealed repair action rather than infer a new action
// from raw missing-evidence kinds.
func syncRepairDirective(facts runtimecontract.RuntimeFacts, decision runtimecontract.RuntimeDecision) runtimecontract.RuntimeFacts {
	facts = facts.Normalized()
	decision.RequiredOutput = decision.RequiredOutput.Normalized()
	focusID := strings.TrimSpace(facts.Focus.ID)
	if focusID == "" {
		facts.RepairDirective = nil
		return facts
	}
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID != focusID {
		facts.RepairDirective = nil
	}

	switch decision.Action {
	case runtimecontract.RuntimeActionComplete, runtimecontract.RuntimeActionFail, runtimecontract.RuntimeActionProduceAssessment:
		facts.RepairDirective = nil
		return facts
	case runtimecontract.RuntimeActionAcquirePrerequisite, runtimecontract.RuntimeActionRequestGovernance:
		// The persisted repair continuation is deliberately retained through a
		// prerequisite/governance interlude. No candidate is rendered for these
		// actions, so it cannot leak into the owner command.
		return facts
	}

	if facts.RepairDirective != nil && facts.RepairDirective.FocusID == focusID &&
		facts.RepairDirective.RequiredAction == runtimecontract.RuntimeActionProduceStateChange &&
		acceptedStateChangeAfterRepairDirective(facts, facts.RepairDirective) {
		directive := facts.RepairDirective.Clone()
		directive.RequiredAction = runtimecontract.RuntimeActionProduceValidation
		directive.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}
		directive.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
		directive.RepairAttempt++
		facts.RepairDirective = &directive
		return facts
	}

	switch decision.Action {
	case runtimecontract.RuntimeActionProduceStateChange, runtimecontract.RuntimeActionProduceValidation:
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
	// Owner feasibility rejection is the one repairable failure that occurs
	// before an operation receipt can create a repair ledger entry.
	return facts.Failure != nil && facts.Failure.Class == runtimecontract.FailureClassCandidateInfeasible
}

func applyRepairDirectiveAction(facts runtimecontract.RuntimeFacts, base runtimecontract.RuntimeDecision) (runtimecontract.RuntimeDecision, bool, error) {
	facts = facts.Normalized()
	if facts.RepairDirective == nil {
		return base, false, nil
	}
	directive := facts.RepairDirective.Normalized()
	if directive.FocusID != facts.Focus.ID || repairDirectiveEvidenceSatisfied(facts, directive) {
		return base, false, nil
	}
	switch directive.RequiredAction {
	case runtimecontract.RuntimeActionProduceStateChange:
		decision, err := repairStateChangeDecision(base, facts, directive)
		return decision, true, err
	case runtimecontract.RuntimeActionProduceValidation:
		decision, err := repairValidationDecision(base, facts, directive)
		return decision, true, err
	default:
		return base, false, fmt.Errorf("repair directive has unsupported required action %q", directive.RequiredAction)
	}
}

func repairDirectiveEvidenceSatisfied(facts runtimecontract.RuntimeFacts, directive runtimecontract.RuntimeRepairDirective) bool {
	required := directive.RequiredEvidenceKinds
	if len(required) == 0 {
		switch directive.RequiredAction {
		case runtimecontract.RuntimeActionProduceStateChange:
			required = []string{runtimecontract.EvidenceKindStateChange}
		case runtimecontract.RuntimeActionProduceValidation:
			required = []string{runtimecontract.EvidenceKindValidation}
		default:
			return false
		}
	}
	accepted := map[string]struct{}{}
	for _, evidence := range facts.Evidence.Accepted {
		if !runtimecontract.EvidenceAppliesToCurrentFocus(facts, evidence) {
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
	if needsStateChangeAfterRejectedValidation(facts) {
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
		if facts.RepairDirective.RequiredAction != decision.Action {
			attempt++
		}
	} else if facts.Repair.RepairAttempts > 0 {
		attempt = facts.Repair.RepairAttempts
	}
	required := decision.RequiredOutput.Normalized()
	directive := runtimecontract.RuntimeRepairDirective{
		ID:                    fmt.Sprintf("repair-%s-%s-%d", facts.Focus.ID, decision.Action, attempt),
		FocusID:               facts.Focus.ID,
		Trigger:               trigger,
		RequiredAction:        decision.Action,
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
	return directive.Normalized()
}

// repairDirectiveTargets is an SM-2 ownership rule: an explicit Focus target
// is immutable authority; otherwise a persisted owner failure target grounds
// the next repair. The runtime core does not infer source/test roles from path
// names or provider text.
func repairDirectiveTargets(facts runtimecontract.RuntimeFacts, fallback []string) []string {
	if len(facts.Focus.TargetRefs) > 0 {
		return append([]string(nil), facts.Focus.TargetRefs...)
	}
	if facts.Resources.FailureTarget != nil && len(facts.Resources.FailureTarget.TargetRefs) > 0 {
		return append([]string(nil), facts.Resources.FailureTarget.TargetRefs...)
	}
	if facts.Failure != nil && len(facts.Failure.TargetRefs) > 0 {
		return append([]string(nil), facts.Failure.TargetRefs...)
	}
	if selection := facts.Resources.TargetSelection; selection != nil && selection.Disposition == runtimecontract.RuntimeTargetSelectionSelected && len(selection.TargetRefs) > 0 {
		return append([]string(nil), selection.TargetRefs...)
	}
	return append([]string(nil), fallback...)
}

func acceptedStateChangeAfterRepairDirective(facts runtimecontract.RuntimeFacts, directive *runtimecontract.RuntimeRepairDirective) bool {
	if directive == nil {
		return false
	}
	// Assessment-required focuses need a persisted post-change assessment before
	// SM-2 can claim that the repair progressed or move on to validation.
	if facts.Focus.RequireAssessment {
		return false
	}
	baseline := strings.TrimSpace(directive.SupersedesEvidenceRevision)
	requiredTargets := directive.TargetRefs
	for _, evidence := range facts.Evidence.Accepted {
		if runtimecontract.EvidenceAppliesToCurrentFocus(facts, evidence) && evidence.Kind == runtimecontract.EvidenceKindStateChange &&
			(baseline == "" || runtimecontract.EvidenceRevisionAfter(evidence.Revision, baseline)) {
			if len(requiredTargets) > 0 && !runtimeTargetRefsIntersect(requiredTargets, evidence.TargetRefs) {
				continue
			}
			return true
		}
	}
	return false
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
	base.Action = runtimecontract.RuntimeActionProduceStateChange
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		Kind: runtimecontract.RequiredOutputKindStateChange, AcceptedKinds: acceptedKinds,
		AcceptedOperationVariants: facts.Focus.AcceptedOperationVariants,
		AcceptedEffectScopes:      directive.RequiredEffectScopes, RequiredEffectScopes: directive.RequiredEffectScopes,
		RequiredEvidenceKinds:     []string{runtimecontract.EvidenceKindStateChange},
		CoverageWorkItemIDs:       firstRepairCoverage(directive.CoverageWorkItemIDs, facts.Focus.CoverageWorkItemIDs),
		TargetRefs:                firstRepairTargets(directive.TargetRefs, facts.Focus.TargetRefs),
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
		return runtimecontract.RuntimeDecision{}, fmt.Errorf("repair directive produce_validation has no accepted operation kinds")
	}
	base.State = runtimecontract.ConvergenceStateNeedValidation
	base.Action = runtimecontract.RuntimeActionProduceValidation
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		Kind: runtimecontract.RequiredOutputKindValidation, AcceptedKinds: acceptedKinds,
		AcceptedOperationVariants: facts.Focus.AcceptedOperationVariants,
		AcceptedEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession},
		RequiredEffectScopes:      directive.RequiredEffectScopes, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		CoverageWorkItemIDs: firstRepairCoverage(directive.CoverageWorkItemIDs, facts.Focus.CoverageWorkItemIDs),
		TargetRefs:          firstRepairTargets(directive.TargetRefs, facts.Focus.TargetRefs),
	}
	return base, nil
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
