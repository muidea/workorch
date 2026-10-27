package workorchmanagementcontract

import "testing"

func TestPreparationAssessmentViewRequiresEveryOutstandingItem(t *testing.T) {
	checks := []PreparationCheckView{{ID: "a", Owner: "file", Required: true, State: "passed", Reason: "observed"}, {ID: "b", Owner: "llm", Required: true, State: "blocked", Reason: "not_observed"}}
	s := PreparationAssessmentView{Version: "workorch-preparation-all-of-assessment/v1", ManifestHash: "manifest", Outcome: "blocked", RequiredCount: 2, PassedRequiredCount: 1, Outstanding: []PreparationOutstandingCheckView{{"b", "llm", "blocked", "not_observed"}}}
	if s.ValidateFor("manifest", checks) != nil {
		t.Fatal("valid historical aggregate rejected")
	}
	for _, mutate := range []func(*PreparationAssessmentView){
		func(s *PreparationAssessmentView) { s.Version = "future" },
		func(s *PreparationAssessmentView) { s.ManifestHash = "other" },
		func(s *PreparationAssessmentView) { s.Outcome = "ready" },
		func(s *PreparationAssessmentView) { s.RequiredCount = 1 },
		func(s *PreparationAssessmentView) { s.PassedRequiredCount = 2 },
		func(s *PreparationAssessmentView) { s.Outstanding = nil },
		func(s *PreparationAssessmentView) { s.Outstanding[0].Owner = "caller" },
		func(s *PreparationAssessmentView) { s.Outstanding[0].Reason = "other" },
	} {
		copy := s
		copy.Outstanding = append([]PreparationOutstandingCheckView(nil), s.Outstanding...)
		mutate(&copy)
		if copy.ValidateFor("manifest", checks) == nil {
			t.Fatal("forged aggregate accepted", copy)
		}
	}
}
