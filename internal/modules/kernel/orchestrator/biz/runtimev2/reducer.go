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

func reduce(facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeDecision, error) {
	base := runtimecontract.RuntimeDecision{Focus: facts.Focus}
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
	if facts.Failure != nil {
		if repeatedFailureCount(facts, facts.Failure.Signature) >= 3 {
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "no_progress_exhausted", facts.Failure.Message), nil
		}
		switch facts.Failure.Class {
		case runtimecontract.FailureClassGovernanceDependency:
			return governanceDecision(base, facts, facts.Failure.Message)
		case runtimecontract.FailureClassRuntimeContractMisalignment:
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "runtime_contract_defect", facts.Failure.Message), nil
		case runtimecontract.FailureClassCandidateExhausted:
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "candidate_retry_exhausted", facts.Failure.Message), nil
		case runtimecontract.FailureClassGovernanceRejected:
			return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "governance_rejected", facts.Failure.Message), nil
		}
	}
	if needsStateChangeAfterRejectedValidation(facts) {
		return stateChangeDecision(base, facts)
	}
	missing := missingEvidenceKinds(facts)
	if len(missing) == 0 {
		if !facts.Focus.RequireAssessment {
			return terminalDecision(base, runtimecontract.ConvergenceStateComplete, runtimecontract.RuntimeActionComplete, "coverage_satisfied", "current coverage focus is satisfied"), nil
		}
		assessment, ok := assessmentForCurrentFocus(facts)
		if !ok {
			return assessmentDecision(base, facts), nil
		}
		if assessment.Satisfied {
			return terminalDecision(base, runtimecontract.ConvergenceStateComplete, runtimecontract.RuntimeActionComplete, "coverage_satisfied", "current coverage focus is satisfied"), nil
		}
		return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "assessment_rejected", assessment.Summary), nil
	}
	if contains(missing, runtimecontract.EvidenceKindStateChange) {
		return stateChangeDecision(base, facts)
	}
	if contains(missing, runtimecontract.EvidenceKindValidation) {
		return validationDecision(base, facts)
	}
	return terminalDecision(base, runtimecontract.ConvergenceStateFailed, runtimecontract.RuntimeActionFail, "unsupported_evidence_gap", fmt.Sprintf("unsupported required evidence kinds: %s", strings.Join(missing, ","))), nil
}

func repeatedFailureCount(facts runtimecontract.RuntimeFacts, signature string) int {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return 0
	}
	latestAcceptedRevision := ""
	for _, evidence := range facts.Evidence.Accepted {
		if evidenceAppliesToCurrentFocus(facts, evidence) && evidence.Revision > latestAcceptedRevision {
			latestAcceptedRevision = evidence.Revision
		}
	}
	count := 0
	for _, evidence := range facts.Evidence.Rejected {
		if evidenceAppliesToCurrentFocus(facts, evidence) && strings.TrimSpace(evidence.FailureSignature) == signature &&
			(latestAcceptedRevision == "" || evidence.Revision == "" || evidence.Revision > latestAcceptedRevision) {
			count++
		}
	}
	return count
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
		return runtimecontract.RuntimeDecision{}, fmt.Errorf("state-change authority requires a concrete runtime capability")
	}
	base.State = runtimecontract.ConvergenceStateNeedStateChange
	base.Action = runtimecontract.RuntimeActionProduceStateChange
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		Kind:                  runtimecontract.RequiredOutputKindStateChange,
		AcceptedKinds:         acceptedKinds,
		AcceptedOperations:    facts.Focus.AcceptedOperations,
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		CoverageWorkItemIDs:   facts.Focus.CoverageWorkItemIDs,
		TargetRefs:            facts.Focus.TargetRefs,
	}
	return base, nil
}

func validationDecision(base runtimecontract.RuntimeDecision, facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeDecision, error) {
	acceptedKinds := focusAcceptedKinds(facts.Capabilities.ValidationKinds, facts.Focus)
	if len(acceptedKinds) == 0 {
		return runtimecontract.RuntimeDecision{}, fmt.Errorf("validation authority requires a concrete runtime capability")
	}
	base.State = runtimecontract.ConvergenceStateNeedValidation
	base.Action = runtimecontract.RuntimeActionProduceValidation
	base.RequiredOutput = runtimecontract.RequiredOutputContract{
		Kind:                  runtimecontract.RequiredOutputKindValidation,
		AcceptedKinds:         acceptedKinds,
		AcceptedOperations:    facts.Focus.AcceptedOperations,
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		CoverageWorkItemIDs:   facts.Focus.CoverageWorkItemIDs,
		TargetRefs:            facts.Focus.TargetRefs,
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
	assessment, assessed := assessmentForCurrentFocus(facts)
	assessmentSatisfied := !facts.Focus.RequireAssessment || (assessed && assessment.Satisfied)
	if len(missingEvidenceKinds(facts)) != 0 || !assessmentSatisfied || len(facts.RemainingFocuses) == 0 {
		return facts
	}
	facts.Focus = facts.RemainingFocuses[0]
	facts.RemainingFocuses = append([]runtimecontract.CoverageFocus(nil), facts.RemainingFocuses[1:]...)
	facts.Failure = nil
	return facts.Normalized()
}

func assessmentForCurrentFocus(facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeAssessmentFact, bool) {
	for _, assessment := range facts.Assessments {
		if assessment.FocusID == facts.Focus.ID {
			return assessment, true
		}
	}
	return runtimecontract.RuntimeAssessmentFact{}, false
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
