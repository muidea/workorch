package workorchruntimecontract

import "testing"

func TestRuntimeAssessmentAdjudicationMissingAuthorityFailsClosed(t *testing.T) {
	s := historicalSessionFixture(t)
	session := &runtimeHistoricalPreimageSession{sourceCompiler: &runtimeHistoricalSourceCompiler{}}
	if err := validateRuntimeAssessmentAdjudicationsInSession(s.Facts, s.Completed, s.ProofLineage, s, session); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, dispatch := range s.Completed {
		for _, fact := range s.Facts.Assessments {
			if dispatch.Candidate.Assessment == nil || fact.ID != dispatch.ID {
				continue
			}
			checked++
			dispatch.AssessmentAuthorityInput = nil
			for range 2 {
				for _, err := range []error{
					ValidateRuntimeAssessmentAdjudication(dispatch, fact, s.Facts, s.Completed, s.ProofLineage, s),
					validateRuntimeAssessmentAdjudicationInSession(dispatch, fact, s.Facts, s.Completed, s.ProofLineage, s, session),
				} {
					if err == nil || err.Error() != "runtime assessment adjudication requires assessment dispatch authority" {
						t.Fatalf("missing authority did not fail closed: %v", err)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("fixture did not exercise an assessment")
	}
}

func TestRuntimeAssessmentAdjudicationMissingAuthorityPreservesPriorErrors(t *testing.T) {
	s := historicalSessionFixture(t)
	checked := 0
	for _, dispatch := range s.Completed {
		for _, fact := range s.Facts.Assessments {
			if dispatch.Candidate.Assessment == nil || fact.ID != dispatch.ID {
				continue
			}
			checked++
			dispatch.AssessmentAuthorityInput = nil
			for arm := range 3 {
				d := cloneRuntimeDispatchRecordRaw(dispatch)
				switch arm {
				case 0:
					d.AssessmentAdjudication = nil
				case 1:
					d.AssessmentAdjudicationHash = "wrong hash before missing authority"
				case 2:
					d.AssessmentAdjudication.Disposition = "invalid adjudication before missing authority"
				}
				// These prior violations return before the old nil dereference.
				want := referenceLineagePreimageValidateRuntimeAssessmentAdjudication(d, fact, s.Facts, s.Completed, s.ProofLineage, s)
				if want == nil {
					t.Fatal("fixture omitted prior malformed-adjudication gate")
				}
				for range 2 {
					got := ValidateRuntimeAssessmentAdjudication(d, fact, s.Facts, s.Completed, s.ProofLineage, s)
					if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
						t.Fatalf("missing authority overtook prior gate %d: %v != %v", arm, got, want)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("fixture did not exercise an assessment")
	}
}
