package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func TestAssessmentEvaluationPreservesSealedVerificationPolicy(t *testing.T) {
	s := historicalAcceptanceProofFixture(t)
	dispatch := s.Completed[len(s.Completed)-1]
	for _, level := range []int{0, 1, 3, 5} {
		t.Run(string(rune('0'+level)), func(t *testing.T) {
			d := cloneRuntimeDispatchRecordRaw(dispatch)
			a := d.AssessmentAuthorityInput
			for i := range a.Focus.CoverageEvidenceBindings {
				binding := &a.Focus.CoverageEvidenceBindings[i]
				binding.VerificationPolicy = &RuntimeVerificationPolicy{Version: RuntimeVerificationPolicyVersion, MinimumEvidenceLevel: level, MinIndependentSources: 1, RequireBoundContext: true}
				binding.Hash = StableCoverageEvidenceBindingHash(*binding)
			}
			d.CoverageBindings = CloneCoverageEvidenceBindings(a.Focus.CoverageEvidenceBindings)
			a.Focus = a.Focus.Normalized()
			a.AuthorityInput.FocusHash = StableRuntimeAuthorityFocusHash(a.Focus)
			d.Transaction.AuthorityInputID = StableRuntimeEquivalentAuthorityInputID(a.AuthorityInput)
			e, err := CompileRuntimeAssessmentEvaluation(d, s.Facts, s.Completed, s.ProofLineage, s)
			if err != nil {
				t.Fatal(err)
			}
			// This owner-backed fixture declares no trusted validation modality.
			// Native check names alone cannot upgrade its level above 1.
			want := level <= 1
			if e.ProofCompleteWithoutOwnerFacts() != want || (e.Adjudication.Disposition == AssessmentDispositionSatisfied) != want {
				t.Fatalf("policy level %d lost by shared classification: %+v", level, e)
			}
			if !reflect.DeepEqual(e.Adjudication.CoverageWorkItemIDs, dispatch.Candidate.Assessment.CoverageWorkItemIDs) {
				t.Fatal("classification changed sealed scope")
			}
		})
	}
}
