package llm

import (
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeCorpusObservationInstruction = "Use this complete owner corpus only to understand the current bound implementation, dependencies and proof requirements against the entire original C1 Goal/DoD. This is source observation, not an instruction to generate validation inputs, execute validation or claim coverage. Only the current sealed task authorizes proposals; source presence and work effects grant no acceptance credit."

// The full typed candidate context is still validated before these pure provider
// projections. A matching CWI alone does not make a generation corpus applicable:
// only a state-changing task can prepare/revise inputs or need implementation
// dependencies. Validation/assessment/governance candidates consume their own
// sealed bindings and observations, not an unrelated generation task.
func runtimeCandidateApplicableGenerationContexts(c rc.RuntimeCandidateContext, m rc.RuntimeCandidateContractManifest) []runtimeValidationGenerationContextProjection {
	if c.RequiredState != rc.ConvergenceStateNeedStateChange {
		return nil
	}
	values := runtimeValidationGenerationContexts(c.Focus.CoverageWorkItemIDs, c.Focus.ProspectiveTargetRefs, c.Resources.OperationAdmissibilities)
	preparations := runtimeCandidateApplicablePreparations(c)
	for i := range values {
		generate := false
		for _, p := range preparations {
			if p.ValidationRecipeID == values[i].ValidationRecipeID {
				generate = true
			}
		}
		if b := m.ImplementationWorkingBinding; b != nil && b.InputRevision != nil && b.InputRevision.Binding.ValidationRecipeID == values[i].ValidationRecipeID {
			generate = true
		}
		if !generate {
			values[i].CoverageInstruction = runtimeCorpusObservationInstruction
		}
	}
	return values
}

func runtimeCandidateApplicablePreparations(c rc.RuntimeCandidateContext) []runtimeSemanticNativeCheckAttestationProjection {
	if c.RequiredState != rc.ConvergenceStateNeedStateChange {
		return nil
	}
	return runtimeCandidateValidationProofPreparations(c.Focus, c.Resources.OperationAdmissibilities)
}

func runtimeCoverageIntersects(a, b []common.CoverageWorkItemID) bool {
	for _, id := range a {
		if slices.Contains(b, id) {
			return true
		}
	}
	return false
}

// No recent-N window substitutes for causal scope. Keep every related fact,
// including explicit allowed evidence and the current failure outside any tail.
// Facts without matching scope are retained only through explicit references.
func runtimeCandidateRelevantEvidence(c rc.RuntimeCandidateContext, values []rc.EvidenceFact) []rc.EvidenceFact {
	facts := rc.RuntimeFacts{Focus: c.Focus, Evidence: c.Evidence}.Normalized()
	allowed := rc.AcceptedEvidenceRefsForFocus(facts)
	failure := c.Failure
	if c.RepairDirective != nil && c.RepairDirective.CurrentFailure != nil {
		failure = c.RepairDirective.CurrentFailure
	}
	var result []rc.EvidenceFact
	for _, v := range values {
		if (v.FocusID != "" && v.FocusID == c.Focus.ID) || runtimeCoverageIntersects(v.CoverageWorkItemIDs, c.Focus.CoverageWorkItemIDs) || slices.Contains(allowed, string(v.ID)) || (failure != nil && failure.Signature != "" && v.FailureSignature == failure.Signature) {
			result = append(result, v)
		}
	}
	return result
}

func runtimeCandidateRelevantAssessments(c rc.RuntimeCandidateContext) []rc.RuntimeAssessmentFact {
	var result []rc.RuntimeAssessmentFact
	for _, v := range c.Assessments {
		if (v.FocusID != "" && v.FocusID == c.Focus.ID) || runtimeCoverageIntersects(v.CoverageWorkItemIDs, c.Focus.CoverageWorkItemIDs) {
			result = append(result, v)
		}
	}
	return result
}

// Ordering needs progress and actual remaining budget, but cannot inspect work
// receipts without a purpose/work task. The owner always validates the full
// history; receipt-sequence aliases remain available whenever work is visible.
func runtimeSemanticVisibleHistory(c rc.RuntimeSemanticPlanRequestContext, m rc.RuntimeSemanticPlanManifest) rc.RuntimeSemanticExecutionHistory {
	if c.PlanStep == rc.RuntimeSemanticPlanStepCoverageOrder && !rc.RuntimeSemanticPurposeChoice(m, c.PlanStep) {
		return rc.RuntimeSemanticExecutionHistory{}
	}
	return c.ExecutionHistory
}
