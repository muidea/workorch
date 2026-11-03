package workorchruntimecontract

import (
	"encoding/json"
	"testing"
)

func TestImplementationRepetitionUsesFullOriginalStateNotRevisionsOrModelClaims(t *testing.T) {
	m, _ := progressPurposeFixture(t)
	w := m.ProofObligations[0].ImplementationWork
	preview := RuntimeImplementationWorkScopePreview{Request: w.Scope.Request, Observation: &w.Scope}
	round := RuntimeImplementationWorkRound{WorkspaceID: m.WorkspaceID, RunID: m.RunID, CoverageWorkItemID: w.Scope.Request.CoverageWorkItemID, PolicyHash: w.PolicyHash, ScopeRequest: w.Scope.Request, ScopeObservation: &w.Scope, CompletedReceiptSetHash: "trusted-original-set"}
	rounds := []RuntimeImplementationWorkRound{round}
	if got := RuntimeImplementationRepeatedWorkStates(preview, rounds); got != 1 {
		t.Fatal(got)
	}
	changed := preview.Clone()
	changed.Request.AuthoritySnapshotID = "another-authority"
	changed.Request.RequestID = "another-request"
	changed.Observation.Content.ManifestRevision = "another-tree-revision"
	if got := RuntimeImplementationRepeatedWorkStates(changed, rounds); got != 1 {
		t.Fatal("identity refreshed repeat guard", got)
	}
	changed.Request.Policy.MaxInputRevisionsPerCWI = 4
	changed.Request.Policy.MaxTargetsPerRound = 2
	if got := RuntimeImplementationRepeatedWorkStates(changed, rounds); got != 1 {
		t.Fatal("method metadata reset physical guard", got)
	}
	foreign := changed.Clone()
	foreign.Request.Policy.OwnerModuleID = "another-owner"
	if got := RuntimeImplementationRepeatedWorkStates(foreign, rounds); got != 0 {
		t.Fatal("borrowed another owner's state", got)
	}
	foreign = changed.Clone()
	foreign.Request.Policy.TargetSelectorSubjectRef = "another-scope"
	if got := RuntimeImplementationRepeatedWorkStates(foreign, rounds); got != 1 {
		t.Fatal("declaration name reset physical guard", got)
	}
	foreign.Observation.Content.Snapshots[0].TargetRef = "another-physical-target"
	if got := RuntimeImplementationRepeatedWorkStates(foreign, rounds); got != 0 {
		t.Fatal("borrowed another physical scope", got)
	}
	raw, _ := json.Marshal(rounds)
	var restored []RuntimeImplementationWorkRound
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if got := RuntimeImplementationRepeatedWorkStates(changed, restored); got != 1 {
		t.Fatal("restart reset guard", got)
	}
	changed.Observation.Content.Snapshots[0] = workContentSnapshot(changed.Observation.Content.Snapshots[0].TargetRef, "different actual content")
	if got := RuntimeImplementationRepeatedWorkStates(changed, restored); got != 0 {
		t.Fatal("different content consumed repeat guard", got)
	}
	for _, field := range []string{"workspace", "run", "cwi", "failed", "unfinished"} {
		r := round.Clone()
		switch field {
		case "workspace":
			r.WorkspaceID = "foreign"
		case "run":
			r.RunID = "foreign"
		case "cwi":
			r.CoverageWorkItemID = "foreign"
		case "failed":
			r.FailedReceiptHash = "failure"
		case "unfinished":
			r.CompletedReceiptSetHash = ""
		}
		if got := RuntimeImplementationRepeatedWorkStates(preview, []RuntimeImplementationWorkRound{r}); got != 0 {
			t.Fatal("borrowed repeat source", field, got)
		}
	}
}

func TestImplementationProofRequestRetainsSemanticGapsWithoutCredit(t *testing.T) {
	m, d := reviewedImplementationPurposeFixture(t, false)
	d.ImplementationPurpose = RuntimeImplementationWorkPurposeTerminal
	d.ImplementationIntent.SelectedRequirementRefs = nil
	assertImplementationSchemaAndParser(t, m, d, true)
	if !d.ImplementationIntent.RequirementReviews[0].WorkRequired || d.ImplementationIntent.PreviousRoundReview.Outcome != "partial" {
		t.Fatal("proof request rewrote known gap")
	}
	if RuntimeImplementationWorkObservationFailure(RuntimeImplementationWorkPurposeChoice{Purpose: d.ImplementationPurpose}, d.ImplementationIntent) != nil {
		t.Fatal("early check became failure before real validation")
	}
}
