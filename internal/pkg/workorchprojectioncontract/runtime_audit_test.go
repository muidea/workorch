package workorchprojectioncontract

import "testing"

func TestCloneRuntimeAuditEvidenceDetachesCollections(t *testing.T) {
	input := []RuntimeAuditEvidence{{
		ID: "evidence-1", TargetRefs: []string{"a.go"}, BindingRefs: []string{"binding-1"},
		CoverageWorkItemIDs: []string{"cwi-1"}, EvidenceRefs: []string{"ref-1"}, ExecutedChecks: []string{"test-a"},
	}}
	cloned := CloneRuntimeAuditEvidence(input)
	cloned[0].TargetRefs[0] = "b.go"
	cloned[0].BindingRefs[0] = "binding-2"
	cloned[0].CoverageWorkItemIDs[0] = "cwi-2"
	cloned[0].EvidenceRefs[0] = "ref-2"
	cloned[0].ExecutedChecks[0] = "test-b"
	if input[0].TargetRefs[0] != "a.go" || input[0].BindingRefs[0] != "binding-1" || input[0].CoverageWorkItemIDs[0] != "cwi-1" || input[0].EvidenceRefs[0] != "ref-1" || input[0].ExecutedChecks[0] != "test-a" {
		t.Fatalf("clone mutated source: %+v", input)
	}
}
