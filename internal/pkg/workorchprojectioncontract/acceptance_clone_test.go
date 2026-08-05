package workorchprojectioncontract

import "testing"

func TestCloneAcceptanceProjectionDetachesRuntimeAudit(t *testing.T) {
	input := AcceptanceProjection{Audit: &RuntimeAudit{
		RemainingFocusIDs: []string{"focus-1"},
		Focus:             RuntimeAuditFocus{TargetRefs: []string{"main.go"}},
		Grounding: RuntimeAuditGrounding{
			TargetSelection:          &RuntimeAuditTargetSelection{TargetRefs: []string{"main.go"}},
			PrerequisiteObservations: []RuntimeAuditPrerequisiteObservation{{Dependencies: []RuntimeAuditFreshnessDependency{{SubjectRef: "workspace"}}}},
		},
		Dispatches:  []RuntimeAuditDispatch{{Operations: []RuntimeAuditSealedOperation{{TargetRefs: []string{"main.go"}}}}},
		Assessments: []RuntimeAuditAssessment{{Items: []RuntimeAuditAssessmentItem{{EvidenceRefs: []string{"evidence-1"}}}}},
	}}
	got := CloneAcceptanceProjection(input)
	input.Audit.RemainingFocusIDs[0] = "mutated"
	input.Audit.Focus.TargetRefs[0] = "mutated.go"
	input.Audit.Grounding.TargetSelection.TargetRefs[0] = "mutated.go"
	input.Audit.Grounding.PrerequisiteObservations[0].Dependencies[0].SubjectRef = "mutated"
	input.Audit.Dispatches[0].Operations[0].TargetRefs[0] = "mutated.go"
	input.Audit.Assessments[0].Items[0].EvidenceRefs[0] = "mutated"

	if got.Audit.RemainingFocusIDs[0] != "focus-1" || got.Audit.Focus.TargetRefs[0] != "main.go" ||
		got.Audit.Grounding.TargetSelection.TargetRefs[0] != "main.go" ||
		got.Audit.Grounding.PrerequisiteObservations[0].Dependencies[0].SubjectRef != "workspace" ||
		got.Audit.Dispatches[0].Operations[0].TargetRefs[0] != "main.go" ||
		got.Audit.Assessments[0].Items[0].EvidenceRefs[0] != "evidence-1" {
		t.Fatalf("acceptance audit clone shares source storage: %+v", got.Audit)
	}
}
