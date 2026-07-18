package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// SyncRepairDirective derives or updates the single persisted repair directive for the
// current focus (INV-05). It does not choose RuntimeAction for the reducer; it records
// the authority the reducer already decided, or materializes a directive from rejected
// validation / assessment gap when none exists.
//
// Rules:
//  1. focus mismatch or empty focus → clear directive
//  2. existing directive for current focus is kept when its RequiredAction still matches
//     the sealed decision action (or decision is not yet a produce_* action)
//  3. rejected validation with implementation-repair policy → produce_state_change
//  4. accepted durable state-change after repair → produce_validation
//  5. raw MissingRequiredEvidenceKinds never overrides an active repair directive action
func SyncRepairDirective(facts RuntimeFacts, decision RuntimeDecision) RuntimeFacts {
	facts = facts.Normalized()
	decision.RequiredOutput = decision.RequiredOutput.Normalized()
	focusID := strings.TrimSpace(facts.Focus.ID)
	if focusID == "" {
		facts.RepairDirective = nil
		return facts
	}

	// Clear directive on terminal / non-repair actions.
	switch decision.Action {
	case RuntimeActionComplete, RuntimeActionFail, RuntimeActionAcquirePrerequisite:
		facts.RepairDirective = nil
		return facts
	case RuntimeActionProduceAssessment:
		// Assessment begins only after repair evidence is satisfied; clear repair authority.
		facts.RepairDirective = nil
		return facts
	}

	// If durable state-change was accepted after a repair directive, flip to validation.
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID == focusID &&
		facts.RepairDirective.RequiredAction == RuntimeActionProduceStateChange {
		if acceptedStateChangeAfterDirective(facts, facts.RepairDirective) {
			directive := facts.RepairDirective.Clone()
			directive.RequiredAction = RuntimeActionProduceValidation
			directive.RequiredEffectScopes = []RuntimeEffectScope{RuntimeEffectNone}
			if len(directive.RequiredEvidenceKinds) == 0 || !containsStringValue(directive.RequiredEvidenceKinds, EvidenceKindValidation) {
				directive.RequiredEvidenceKinds = normalizeStrings(append(directive.RequiredEvidenceKinds, EvidenceKindValidation))
			}
			// Drop pure state_change requirement from the flipped directive.
			filtered := make([]string, 0, len(directive.RequiredEvidenceKinds))
			for _, kind := range directive.RequiredEvidenceKinds {
				if kind == EvidenceKindStateChange {
					continue
				}
				filtered = append(filtered, kind)
			}
			if len(filtered) == 0 {
				filtered = []string{EvidenceKindValidation}
			}
			directive.RequiredEvidenceKinds = filtered
			directive.RepairAttempt++
			facts.RepairDirective = &directive
			return facts
		}
	}

	// Materialize only repairable produce_* actions. request_governance is a wait state,
	// not a durable repair directive — clear any stale governance directive.
	switch decision.Action {
	case RuntimeActionProduceStateChange, RuntimeActionProduceValidation:
		directive := buildRepairDirectiveFromDecision(facts, decision)
		facts.RepairDirective = &directive
		return facts
	case RuntimeActionRequestGovernance:
		facts.RepairDirective = nil
		return facts
	}

	// Keep existing same-focus directive when decision is not a produce_* action yet.
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID == focusID {
		return facts
	}
	facts.RepairDirective = nil
	return facts
}

// ApplyRepairDirectiveAction forces the reducer decision to obey the persisted directive
// RequiredAction when present for the current focus. Returns false when no directive applies.
func ApplyRepairDirectiveAction(facts RuntimeFacts, base RuntimeDecision) (RuntimeDecision, bool, error) {
	facts = facts.Normalized()
	if facts.RepairDirective == nil {
		return base, false, nil
	}
	directive := facts.RepairDirective.Normalized()
	if directive.FocusID != facts.Focus.ID {
		return base, false, nil
	}
	// request_governance is a transient wait, not a repair authority that must be re-applied
	// after resolution. Governance continue must re-enter normal missing-evidence reduce.
	if directive.RequiredAction == RuntimeActionRequestGovernance {
		return base, false, nil
	}
	// If the directive's required evidence is already satisfied on this focus, do not
	// re-force the action — let normal reduce complete, assess, or advance.
	if repairDirectiveEvidenceSatisfied(facts, directive) {
		return base, false, nil
	}
	switch directive.RequiredAction {
	case RuntimeActionProduceStateChange:
		decision, err := stateChangeDecisionFromFacts(base, facts, directive)
		return decision, true, err
	case RuntimeActionProduceValidation:
		decision, err := validationDecisionFromFacts(base, facts, directive)
		return decision, true, err
	default:
		return base, false, fmt.Errorf("repair directive has unsupported required action %q", directive.RequiredAction)
	}
}

func repairDirectiveEvidenceSatisfied(facts RuntimeFacts, directive RuntimeRepairDirective) bool {
	required := directive.RequiredEvidenceKinds
	if len(required) == 0 {
		switch directive.RequiredAction {
		case RuntimeActionProduceStateChange:
			required = []string{EvidenceKindStateChange}
		case RuntimeActionProduceValidation:
			required = []string{EvidenceKindValidation}
		default:
			return false
		}
	}
	accepted := map[string]struct{}{}
	for _, evidence := range facts.Evidence.Accepted {
		if EvidenceAppliesToCurrentFocus(facts, evidence) {
			accepted[strings.TrimSpace(evidence.Kind)] = struct{}{}
		}
	}
	for _, kind := range required {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		if _, ok := accepted[kind]; !ok {
			return false
		}
	}
	return true
}

func buildRepairDirectiveFromDecision(facts RuntimeFacts, decision RuntimeDecision) RuntimeRepairDirective {
	trigger := RuntimeRepairTriggerMissingEvidence
	if needsStateChangeRepairFromRejectedValidation(facts) {
		trigger = RuntimeRepairTriggerRejectedValidation
	}
	if facts.Failure != nil && facts.Failure.Class == FailureClassExecutionRejected {
		trigger = RuntimeRepairTriggerExecutionRejected
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
	directive := RuntimeRepairDirective{
		ID:                    stableRepairDirectiveID(facts, decision.Action, attempt),
		FocusID:               facts.Focus.ID,
		Trigger:               trigger,
		RequiredAction:        decision.Action,
		RequiredEffectScopes:  required.RequiredEffectScopes,
		RequiredEvidenceKinds: required.RequiredEvidenceKinds,
		AllowedOperationKinds: required.AcceptedKinds,
		CoverageWorkItemIDs:   required.CoverageWorkItemIDs,
		TargetRefs:            required.TargetRefs,
		RepairAttempt:         attempt,
	}
	if facts.Failure != nil {
		failure := *facts.Failure
		directive.CurrentFailure = &failure
	}
	if facts.RepairDirective != nil && facts.RepairDirective.FocusID == facts.Focus.ID {
		directive.SupersedesEvidenceRevision = facts.RepairDirective.SupersedesEvidenceRevision
		if len(facts.RepairDirective.RejectedOperationShapes) > 0 {
			directive.RejectedOperationShapes = append([]RuntimeRejectedOperationShape(nil), facts.RepairDirective.RejectedOperationShapes...)
		}
	}
	if directive.SupersedesEvidenceRevision == "" {
		directive.SupersedesEvidenceRevision = facts.EvidenceRevision
	}
	return directive.Normalized()
}

func stableRepairDirectiveID(facts RuntimeFacts, action RuntimeAction, attempt int) string {
	return fmt.Sprintf("repair-%s-%s-%d", strings.TrimSpace(facts.Focus.ID), strings.TrimSpace(string(action)), attempt)
}

func acceptedStateChangeAfterDirective(facts RuntimeFacts, directive *RuntimeRepairDirective) bool {
	if directive == nil {
		return false
	}
	baseline := strings.TrimSpace(directive.SupersedesEvidenceRevision)
	for _, evidence := range facts.Evidence.Accepted {
		if !EvidenceAppliesToCurrentFocus(facts, evidence) || evidence.Kind != EvidenceKindStateChange {
			continue
		}
		if baseline == "" || EvidenceRevisionAfter(evidence.Revision, baseline) || evidence.Revision == baseline {
			// accepted state-change at or after the directive baseline counts as repair progress
			if baseline == "" || EvidenceRevisionAfter(evidence.Revision, baseline) {
				return true
			}
		}
	}
	return false
}

// needsStateChangeRepairFromRejectedValidation mirrors reducer policy without importing
// orchestrator packages. Used only for trigger classification.
func needsStateChangeRepairFromRejectedValidation(facts RuntimeFacts) bool {
	if !RuntimeValidationFailureRequiresStateChange(facts.Activation.ExecutionContract.ValidationPolicies, EvidenceKindValidation) {
		return false
	}
	hasRejectedValidation := false
	latestValidation := ""
	for _, evidence := range facts.Evidence.Rejected {
		if !EvidenceAppliesToCurrentFocus(facts, evidence) || evidence.Kind != EvidenceKindValidation {
			continue
		}
		if evidence.FailureClass == "" && strings.TrimSpace(evidence.FailureMessage) == "" {
			continue
		}
		hasRejectedValidation = true
		if latestValidation == "" || EvidenceRevisionAfter(evidence.Revision, latestValidation) {
			latestValidation = evidence.Revision
		}
	}
	if !hasRejectedValidation {
		return false
	}
	for _, evidence := range facts.Evidence.Accepted {
		if !EvidenceAppliesToCurrentFocus(facts, evidence) || evidence.Kind != EvidenceKindStateChange {
			continue
		}
		if latestValidation == "" || EvidenceRevisionAfter(evidence.Revision, latestValidation) {
			return false
		}
	}
	return true
}

func stateChangeDecisionFromFacts(base RuntimeDecision, facts RuntimeFacts, directive RuntimeRepairDirective) (RuntimeDecision, error) {
	acceptedKinds := directive.AllowedOperationKinds
	if len(acceptedKinds) == 0 {
		acceptedKinds = focusAcceptedKindsForDirective(facts.Capabilities.StateChangeKinds, facts.Focus)
	}
	if len(acceptedKinds) == 0 {
		return RuntimeDecision{}, fmt.Errorf("repair directive produce_state_change has no accepted operation kinds")
	}
	base.State = ConvergenceStateNeedStateChange
	base.Action = RuntimeActionProduceStateChange
	base.RequiredOutput = RequiredOutputContract{
		Kind:                  RequiredOutputKindStateChange,
		AcceptedKinds:         acceptedKinds,
		AcceptedOperations:    facts.Focus.AcceptedOperations,
		AcceptedEffectScopes:  directive.RequiredEffectScopes,
		RequiredEffectScopes:  directive.RequiredEffectScopes,
		RequiredEvidenceKinds: []string{EvidenceKindStateChange},
		CoverageWorkItemIDs:   firstNonEmptyCoverage(directive.CoverageWorkItemIDs, facts.Focus.CoverageWorkItemIDs),
		TargetRefs:            firstNonEmptyStrings(directive.TargetRefs, facts.Focus.TargetRefs),
	}
	return base, nil
}

func validationDecisionFromFacts(base RuntimeDecision, facts RuntimeFacts, directive RuntimeRepairDirective) (RuntimeDecision, error) {
	acceptedKinds := directive.AllowedOperationKinds
	if len(acceptedKinds) == 0 {
		acceptedKinds = focusAcceptedKindsForDirective(facts.Capabilities.ValidationKinds, facts.Focus)
	}
	if len(acceptedKinds) == 0 {
		return RuntimeDecision{}, fmt.Errorf("repair directive produce_validation has no accepted operation kinds")
	}
	base.State = ConvergenceStateNeedValidation
	base.Action = RuntimeActionProduceValidation
	base.RequiredOutput = RequiredOutputContract{
		Kind:                  RequiredOutputKindValidation,
		AcceptedKinds:         acceptedKinds,
		AcceptedOperations:    facts.Focus.AcceptedOperations,
		AcceptedEffectScopes:  NormalizeRuntimeEffectScopes(append([]RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession}, directive.RequiredEffectScopes...)),
		RequiredEffectScopes:  directive.RequiredEffectScopes,
		RequiredEvidenceKinds: []string{EvidenceKindValidation},
		CoverageWorkItemIDs:   firstNonEmptyCoverage(directive.CoverageWorkItemIDs, facts.Focus.CoverageWorkItemIDs),
		TargetRefs:            firstNonEmptyStrings(directive.TargetRefs, facts.Focus.TargetRefs),
	}
	return base, nil
}

func focusAcceptedKindsForDirective(capabilityKinds []string, focus CoverageFocus) []string {
	capabilityKinds = canonicalizeOperationKinds(capabilityKinds)
	if len(focus.AcceptedStepKinds) == 0 {
		return capabilityKinds
	}
	allowed := stringSet(canonicalizeOperationKinds(focus.AcceptedStepKinds))
	ret := make([]string, 0, len(capabilityKinds))
	for _, kind := range capabilityKinds {
		if _, ok := allowed[kind]; ok {
			ret = append(ret, kind)
		}
	}
	return ret
}

func firstNonEmptyCoverage(primary []common.CoverageWorkItemID, fallback []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

func firstNonEmptyStrings(primary []string, fallback []string) []string {
	if len(primary) > 0 {
		return primary
	}
	return fallback
}

// ClearRepairDirectiveForFocusAdvance drops the active directive when focus advances.
func ClearRepairDirectiveForFocusAdvance(facts RuntimeFacts) RuntimeFacts {
	facts = facts.Normalized()
	facts.RepairDirective = nil
	return facts
}
