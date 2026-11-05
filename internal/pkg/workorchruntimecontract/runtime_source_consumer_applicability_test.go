package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestCandidateAttemptIntrinsicAndScopeShareSingleRawRule(t *testing.T) {
	r := candidateSourceCompilerFixture(t, 2)
	s := newCandidateSourceTestSession()
	for _, intrinsicFirst := range []bool{true, false} {
		if intrinsicFirst {
			if err := validateRuntimeCandidateAttemptRecordsInSession(r.CandidateAttempts, s); err != nil {
				t.Fatal(err)
			}
		}
		compare := func(image RuntimeStateRecord) {
			t.Helper()
			got := validateRuntimeCandidateAttemptSourcesInSession(image.Facts.WorkspaceID, image.Facts.RunID, image.CandidateAttempts, s)
			want := referenceAttemptScopeBeforeIntrinsicSplit(image.Facts.WorkspaceID, image.Facts.RunID, image.CandidateAttempts)
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("raw/scope first error: %v != %v", got, want)
			}
			if got, want := validateRuntimeCandidateAttemptRecordsInSession(image.CandidateAttempts, s), ValidateRuntimeCandidateAttemptRecords(image.CandidateAttempts); fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("scope result changed intrinsic rule: %v != %v", got, want)
			}
		}
		compare(r)
		for _, mutate := range []func(*RuntimeStateRecord){
			func(r *RuntimeStateRecord) { r.Facts.WorkspaceID = "other-workspace" },
			func(r *RuntimeStateRecord) { r.Facts.RunID = "other-run" },
			func(r *RuntimeStateRecord) { r.CandidateAttempts[0].AuthorityContractID = "wrong scope" },
			func(r *RuntimeStateRecord) { r.CandidateAttempts[0].TargetAuthorizationHash = "raw drift" },
			func(r *RuntimeStateRecord) { r.CandidateAttempts = append(r.CandidateAttempts, r.CandidateAttempts[0]) },
			func(r *RuntimeStateRecord) { r.CandidateAttempts = nil },
			func(r *RuntimeStateRecord) { r.CandidateAttempts = []RuntimeCandidateAttemptRecord{} },
		} {
			changed := r.Clone()
			mutate(&changed)
			compare(changed)
			compare(r)
		}
		body, _ := json.Marshal(r)
		var loaded RuntimeStateRecord
		if err := json.Unmarshal(body, &loaded); err != nil {
			t.Fatal(err)
		}
		compare(loaded)
		assertRuntimeStateRawCloneDetached(t, reflect.ValueOf(r.CandidateAttempts), reflect.ValueOf(s.candidateSources.attempts.values), "intrinsic attempts")
		s = newCandidateSourceTestSession()
	}
}

func TestConfirmedRepairRejectsNoncurrentBindingBeforeUnrelatedCompilation(t *testing.T) {
	s := historicalSessionFixture(t)
	var binding CoverageEvidenceBinding
	for _, d := range s.Completed {
		if len(d.CoverageBindings) != 0 {
			binding = d.CoverageBindings[0]
			break
		}
	}
	if binding.ID == "" {
		t.Fatal("fixture lacks original binding")
	}
	// Removing the current row cannot produce repair credit, even when the
	// unrelated history is invalid. No whole-source validation is needed to
	// answer this negative query; owner admission remains independently strict.
	s.Facts.Coverage = nil
	s.Semantic.History = append(s.Semantic.History, RuntimeSemanticPlanningGeneration{})
	before := cloneRuntimeLineageSourcesRaw(s)
	for range 2 {
		session := &runtimeHistoricalPreimageSession{semantic: newRuntimeSemanticValidationSession()}
		got := runtimeConfirmedBindingRepairInSession(binding, s, s.Facts.EvidenceRevision, session)
		want := referenceLineagePreimageruntimeConfirmedBindingRepairSatisfied(binding, s, s.Facts.EvidenceRevision)
		if got || got != want || session.ledger != nil || len(session.lineageOperations) != 0 || len(session.semantic.workStates) != 0 {
			t.Fatal("noncurrent binding received credit or rebuilt unrelated history")
		}
	}
	if !reflect.DeepEqual(before, s) {
		t.Fatal("negative repair query changed original source")
	}
}
