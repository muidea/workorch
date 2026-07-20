package runtimev2

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
		previous.Action == decision.Action &&
		previous.RequiredOutputHash == runtimecontract.StableRequiredOutputHash(decision.RequiredOutput) {
		attempt = previous.Attempt + 1
	}
	return runtimecontract.SealRuntimeDecision(facts, decision, attempt)
}

// ReduceRuntimeStateWithDirective reduces facts and returns the decision plus facts
// with RepairDirective synchronized to the sealed decision (INV-05).
func ReduceRuntimeStateWithDirective(facts runtimecontract.RuntimeFacts, previous *runtimecontract.AuthoritySnapshot) (runtimecontract.RuntimeFacts, runtimecontract.RuntimeDecision, error) {
	decision, err := ReduceRuntimeState(facts, previous)
	if err != nil {
		return facts, runtimecontract.RuntimeDecision{}, err
	}
	facts = runtimecontract.SyncRepairDirective(facts, decision)
	return facts, decision, nil
}

func reduce(facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeDecision, error) {
	base := runtimecontract.RuntimeDecision{Focus: facts.Focus}
	// A refresh budget is an authority terminal, even though the change that
	// exhausted it has already reopened its mutable prerequisites. Letting the
	// generic pending-prerequisite branch win here would request a new candidate
	// from the old facts after the budget is spent, disguising exhaustion as
	// retry bookkeeping. SM-2 must instead seal the explicit terminal and leave
	// SM-4 to deliver it idempotently.
	if facts.Failure != nil && facts.Failure.Class == runtimecontract.FailureClassPrerequisiteRefreshExhausted {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "prerequisite_refresh_exhausted", facts.Failure.Message), nil
	}
	if len(facts.Prerequisites.Pending) > 0 {
		ids := make([]string, 0, len(facts.Prerequisites.Pending))
		for _, prerequisite := range facts.Prerequisites.Pending {
			ids = append(ids, prerequisite.ID)
		}
		base.State = runtimecontract.ConvergenceStateNeedPrerequisite
		base.Action = runtimecontract.RuntimeActionAcquirePrerequisite
		base.Prerequisites = facts.Prerequisites.Pending
		base.RequiredOutput = runtimecontract.RequiredOutputContract{
			Kind:            runtimecontract.RequiredOutputKindPrerequisite,
			PrerequisiteIDs: ids,
		}
		return base, nil
	}
	if len(facts.FreshnessReacquireCoverage) > 0 {
		return stateChangeDecision(base, facts)
	}
	if facts.Failure != nil {
		if facts.Failure.Class == runtimecontract.FailureClassExecutionFailure &&
			(facts.Repair.Exhausted || facts.Repair.RepairAttempts >= facts.ConvergencePolicy.MaxRepairAttemptsPerFocus) {
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "no_progress_exhausted", facts.Failure.Message), nil
		}
		switch facts.Failure.Class {
		case runtimecontract.FailureClassProviderTransient:
			if facts.Recovery.ProviderTransportAttempts < facts.RecoveryPolicy.MaxProviderTransportAttempts {
				break
			}
			if len(facts.Capabilities.GovernanceKinds) > 0 {
				return governanceDecision(base, facts, facts.Failure.Message)
			}
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "provider_transport_exhausted", facts.Failure.Message), nil
		case runtimecontract.FailureClassOwnerTransientUnavailable:
			if facts.Recovery.OperationRecoveryAttempts < facts.RecoveryPolicy.MaxOperationRecoveryAttempts {
				break
			}
			if len(facts.Capabilities.GovernanceKinds) > 0 {
				return governanceDecision(base, facts, facts.Failure.Message)
			}
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "operation_recovery_exhausted", facts.Failure.Message), nil
		case runtimecontract.FailureClassGovernanceDependency, runtimecontract.FailureClassGovernanceRequired,
			runtimecontract.FailureClassProviderAuthentication, runtimecontract.FailureClassProviderConfiguration,
			runtimecontract.FailureClassOperationOutcomeUnknown:
			return governanceDecision(base, facts, facts.Failure.Message)
		case runtimecontract.FailureClassRuntimeContractMisalignment:
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "runtime_contract_defect", facts.Failure.Message), nil
		case runtimecontract.FailureClassCandidateExhausted:
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "candidate_retry_exhausted", facts.Failure.Message), nil
		case runtimecontract.FailureClassPrerequisiteRefreshExhausted:
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "prerequisite_refresh_exhausted", facts.Failure.Message), nil
		case runtimecontract.FailureClassGovernanceRejected:
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "governance_rejected", facts.Failure.Message), nil
		}
	}
	if feasibilityBlocksCurrentAction(facts) {
		if len(facts.Capabilities.GovernanceKinds) == 0 {
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "capability_unavailable", firstNonEmpty(facts.Feasibility.Reason, "current focus has no feasible operation under sealed capabilities")), nil
		}
		return governanceDecision(base, facts, firstNonEmpty(facts.Feasibility.Reason, "current focus has no feasible operation under sealed capabilities"))
	}
	// INV-05: persisted RepairDirective is the sole next-action authority for the focus.
	// Raw MissingRequiredEvidenceKinds and anti-replay must not override it.
	if decision, ok, err := runtimecontract.ApplyRepairDirectiveAction(facts, base); err != nil {
		return runtimecontract.RuntimeDecision{}, err
	} else if ok {
		return decision, nil
	}
	if needsStateChangeAfterRejectedValidation(facts) {
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
		switch assessment.Disposition {
		case runtimecontract.AssessmentDispositionSatisfied:
			if currentFocusCoverageSatisfied(facts) {
				return completeOrAdvanceDecision(base, facts, "current coverage focus is satisfied")
			}
			return repairableAssessmentDecision(base, facts, assessment)
		case runtimecontract.AssessmentDispositionRepairableGap:
			return repairableAssessmentDecision(base, facts, assessment)
		case runtimecontract.AssessmentDispositionGovernanceRequired:
			return governanceDecision(base, facts, firstNonEmptyAssessmentSummary(assessment, "assessment requires governance"))
		case runtimecontract.AssessmentDispositionTerminalRejection:
			// The candidate can report irrecoverability, but terminal authority stays
			// in the reducer. Only an already-persisted governance rejection is an
			// irrecoverable state; all other negative assessment facts are repaired.
			if assessment.Gap != nil && assessment.Gap.FailureClass == runtimecontract.FailureClassGovernanceRejected {
				return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "governance_rejected", firstNonEmptyAssessmentSummary(assessment, "governance rejected")), nil
			}
			return repairableAssessmentDecision(base, facts, assessment)
		default:
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "assessment_rejected", firstNonEmptyAssessmentSummary(assessment, "unknown assessment disposition")), nil
		}
	}
	if contains(missing, runtimecontract.EvidenceKindStateChange) {
		return stateChangeDecision(base, facts)
	}
	if contains(missing, runtimecontract.EvidenceKindValidation) {
		return validationDecision(base, facts)
	}
	return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "unsupported_evidence_gap", fmt.Sprintf("unsupported required evidence kinds: %s", strings.Join(missing, ","))), nil
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
	return (fact.Action == runtimecontract.RuntimeActionProduceStateChange && contains(missing, runtimecontract.EvidenceKindStateChange)) ||
		(fact.Action == runtimecontract.RuntimeActionProduceValidation && contains(missing, runtimecontract.EvidenceKindValidation))
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
	base.Action = runtimecontract.RuntimeActionProduceAssessment
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		Kind: runtimecontract.RequiredOutputKindAssessment, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs, TargetRefs: facts.Focus.TargetRefs,
	}
	return base
}

func stateChangeDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeDecision, error) {
	acceptedKinds := focusAcceptedKinds(facts.Capabilities.StateChangeKinds, facts.Focus)
	if len(acceptedKinds) == 0 {
		if len(facts.Capabilities.GovernanceKinds) > 0 {
			return governanceDecision(base, facts, "state-change authority has no concrete runtime capability")
		}
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "capability_unavailable", "state-change authority has no concrete runtime capability"), nil
	}
	base.State = runtimecontract.ConvergenceStateNeedStateChange
	base.Action = runtimecontract.RuntimeActionProduceStateChange
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		Kind:                      runtimecontract.RequiredOutputKindStateChange,
		AcceptedKinds:             acceptedKinds,
		AcceptedOperationVariants: facts.Focus.AcceptedOperationVariants,
		AcceptedEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
		RequiredEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
		RequiredEvidenceKinds:     []string{runtimecontract.EvidenceKindStateChange},
		CoverageWorkItemIDs:       facts.Focus.CoverageWorkItemIDs,
		TargetRefs:                facts.Focus.TargetRefs,
		RequiredOperationSequence: requiredCapabilityStateSequence(facts, acceptedKinds),
	}
	return base, nil
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
	acceptedKinds := focusAcceptedKinds(facts.Capabilities.ValidationKinds, facts.Focus)
	if len(acceptedKinds) == 0 {
		if len(facts.Capabilities.GovernanceKinds) > 0 {
			return governanceDecision(base, facts, "validation authority has no concrete runtime capability")
		}
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "capability_unavailable", "validation authority has no concrete runtime capability"), nil
	}
	base.State = runtimecontract.ConvergenceStateNeedValidation
	base.Action = runtimecontract.RuntimeActionProduceValidation
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		Kind:                      runtimecontract.RequiredOutputKindValidation,
		AcceptedKinds:             acceptedKinds,
		AcceptedOperationVariants: facts.Focus.AcceptedOperationVariants,
		AcceptedEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession},
		RequiredEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
		RequiredEvidenceKinds:     []string{runtimecontract.EvidenceKindValidation},
		CoverageWorkItemIDs:       facts.Focus.CoverageWorkItemIDs,
		TargetRefs:                facts.Focus.TargetRefs,
	}
	return base, nil
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
	if len(facts.Capabilities.GovernanceKinds) == 0 {
		return runtimecontract.RuntimeDecision{}, fmt.Errorf("governance authority requires a concrete runtime capability")
	}
	base.State = runtimecontract.ConvergenceStateNeedGovernance
	base.Action = runtimecontract.RuntimeActionRequestGovernance
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		Kind:          runtimecontract.RequiredOutputKindGovernance,
		AcceptedKinds: facts.Capabilities.GovernanceKinds,
	}
	return base, nil
}

// completeOrAdvanceDecision completes the current focus only when remaining focuses are empty
// and the Run-level state-change ledger satisfies the sealed activation policy.
func completeOrAdvanceDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts, summary string) (runtimecontract.RuntimeDecision, error) {
	if len(facts.RemainingFocuses) > 0 {
		// Focus-level satisfaction alone does not terminal the Run while more focuses remain.
		// Drive/commit paths advance the focus queue; reducer reports complete for the current focus.
		return terminalDecision(base, runtimecontract.ConvergenceStateComplete, runtimecontract.RuntimeActionComplete, "coverage_satisfied", summary), nil
	}
	if !facts.StateChange.SatisfiesRequirement() {
		// Run policy still needs coverage-bound primary state-change evidence.
		return stateChangeDecision(base, facts)
	}
	return terminalDecision(base, runtimecontract.ConvergenceStateComplete, runtimecontract.RuntimeActionComplete, "coverage_satisfied", summary), nil
}

func terminalDecision(base runtimecontract.RuntimeDecision, state runtimecontract.ConvergenceState, action runtimecontract.RuntimeAction, code, summary string) runtimecontract.RuntimeDecision {
	base.State = state
	base.Action = action
	base.Terminal = &runtimecontract.TerminalOutcome{Code: code, Summary: summary}
	return base
}

func missingEvidenceKinds(facts runtimecontract.RuntimeFacts) []string {
	return runtimecontract.MissingRequiredEvidenceKinds(facts)
}

// needsStateChangeAfterRejectedValidation forces repair state-change after a focus-local
// validation rejection until a later state-change is accepted. Rejected repair attempts do
// not re-open produce_validation: replaying the same failed validation would be blocked by
// anti-replay and only waste candidate budget. Re-validation is allowed only after accepted
// state-change progress supersedes the failed validation lineage.
func needsStateChangeAfterRejectedValidation(facts runtimecontract.RuntimeFacts) bool {
	if !runtimecontract.RuntimeValidationFailureRequiresStateChange(facts.Activation.ExecutionContract.ValidationPolicies, runtimecontract.EvidenceKindValidation) {
		return false
	}
	validationRevision, ok := latestRejectedValidationRevision(facts)
	if !ok {
		return false
	}
	for _, evidence := range facts.Evidence.Accepted {
		if !runtimecontract.EvidenceAppliesToCurrentFocus(facts, evidence) || evidence.Kind != runtimecontract.EvidenceKindStateChange {
			continue
		}
		if evidenceRevisionAfter(evidence.Revision, validationRevision) {
			return false
		}
	}
	return true
}

func latestRejectedValidationRevision(facts runtimecontract.RuntimeFacts) (string, bool) {
	found := false
	revision := ""
	for _, evidence := range facts.Evidence.Rejected {
		if !runtimecontract.EvidenceAppliesToCurrentFocus(facts, evidence) || evidence.Kind != runtimecontract.EvidenceKindValidation {
			continue
		}
		if evidence.FailureClass == "" && strings.TrimSpace(evidence.FailureMessage) == "" {
			continue
		}
		if !found || evidenceRevisionAfter(evidence.Revision, revision) || (evidence.Revision == "" && revision == "") {
			found = true
			revision = evidence.Revision
		}
	}
	return revision, found
}

func evidenceRevisionAfter(left string, right string) bool {
	return runtimecontract.EvidenceRevisionAfter(left, right)
}

func advanceSatisfiedCoverageFocus(facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeFacts {
	facts = facts.Normalized()
	if len(missingEvidenceKinds(facts)) != 0 || !currentFocusCoverageSatisfied(facts) || len(facts.RemainingFocuses) == 0 {
		return facts
	}
	facts.Focus = facts.RemainingFocuses[0]
	facts.RemainingFocuses = append([]runtimecontract.CoverageFocus(nil), facts.RemainingFocuses[1:]...)
	facts.Failure = nil
	facts.Feasibility = nil
	facts.Repair = facts.Repair.ResetForFocus(facts.Focus.ID, facts.Focus.SliceID, facts.EvidenceRevision)
	facts = runtimecontract.ClearRepairDirectiveForFocusAdvance(facts)
	return facts.Normalized()
}

// repairableAssessmentDecision keeps the Run active and re-opens state-change or validation
// based on the structured gap. Repair budget is independent from candidate attempt 0/1.
func repairableAssessmentDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts, assessment runtimecontract.RuntimeAssessmentFact) (runtimecontract.RuntimeDecision, error) {
	policy := facts.ConvergencePolicy.Normalized()
	ledger := facts.Repair.Normalized()
	if ledger.Exhausted || ledger.RepairAttempts >= policy.MaxRepairAttemptsPerFocus {
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "no_progress_exhausted", firstNonEmptyAssessmentSummary(assessment, "repair budget exhausted")), nil
	}

	wantValidationOnly := false
	if assessment.Gap != nil {
		for _, kind := range assessment.Gap.MissingEvidenceKinds {
			if kind == runtimecontract.EvidenceKindStateChange {
				return stateChangeDecision(base, facts)
			}
		}
		for _, kind := range assessment.Gap.MissingEvidenceKinds {
			if kind == runtimecontract.EvidenceKindValidation {
				wantValidationOnly = true
				break
			}
		}
	}
	if !wantValidationOnly {
		for _, evidence := range facts.Evidence.Accepted {
			if evidenceAppliesToCurrentFocus(facts, evidence) && evidence.Kind == runtimecontract.EvidenceKindStateChange {
				wantValidationOnly = true
				break
			}
		}
	}
	if wantValidationOnly {
		if ledger.ValidationOnlyRetries >= policy.MaxValidationOnlyRetriesPerFocus {
			if runtimecontract.RuntimeValidationFailureRequiresStateChange(facts.Activation.ExecutionContract.ValidationPolicies, runtimecontract.EvidenceKindValidation) && len(facts.Capabilities.StateChangeKinds) > 0 {
				return stateChangeDecision(base, facts)
			}
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "no_progress_exhausted", "validation-only repair budget exhausted"), nil
		}
		return validationDecision(base, facts)
	}
	if len(facts.Capabilities.StateChangeKinds) > 0 {
		return stateChangeDecision(base, facts)
	}
	return validationDecision(base, facts)
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
