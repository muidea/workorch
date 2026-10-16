package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func compareLineagePreimageSession(t *testing.T, s RuntimeLineageAssertionSources) {
	t.Helper()
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	session := &runtimeHistoricalPreimageSession{}
	for _, dispatch := range s.Completed {
		for _, binding := range dispatch.CoverageBindings {
			want, wantErr := referenceLineagePreimageCompileRuntimeBindingValidationInputReview(binding, s.Semantic)
			for range 2 {
				got, gotErr := compileRuntimeBindingValidationInputReviewInSession(binding, s.Semantic, session)
				if cwiFocusBindingsError(gotErr) != cwiFocusBindingsError(wantErr) || !reflect.DeepEqual(got, want) {
					t.Fatal("exact input review differs from original")
				}
				if got.InputReview != nil && len(got.InputReview.Files) > 0 {
					got.InputReview.Files[0].Content = "caller copy"
				}
			}
			cutoff := s.Facts.EvidenceRevision
			if runtimeExecutionLineageAssertionsInSession(binding, s, cutoff, session) != referenceLineagePreimageRuntimeExecutionLineageAssertionsSatisfied(binding, s, cutoff) {
				t.Fatal("complete lineage AND changed")
			}
			if runtimeConfirmedBindingRepairInSession(binding, s, cutoff, session) != referenceLineagePreimageruntimeConfirmedBindingRepairSatisfied(binding, s, cutoff) {
				t.Fatal("confirmed repair source/cutoff changed")
			}
			if runtimeRepairThenFreshValidationInSession(binding, s, cutoff, session) != referenceLineagePreimageruntimeRepairThenFreshValidationSatisfied(binding, s, cutoff) {
				t.Fatal("retired repair source/cutoff changed")
			}
		}
		if dispatch.Candidate.Assessment != nil {
			got, gotErr := deriveRuntimeAssessmentAdjudicationInSession(dispatch, s.Facts, s.Completed, s.ProofLineage, s, session)
			want, wantErr := referenceLineagePreimageDeriveRuntimeAssessmentAdjudication(dispatch, s.Facts, s.Completed, s.ProofLineage, s)
			if cwiFocusBindingsError(gotErr) != cwiFocusBindingsError(wantErr) || !reflect.DeepEqual(got, want) {
				t.Fatal("assessment complete result/first error changed")
			}
		}
	}
	got := ValidateRuntimeAssessmentAdjudications(s.Facts, s.Completed, s.ProofLineage, s)
	want := referenceLineagePreimageValidateRuntimeAssessmentAdjudications(s.Facts, s.Completed, s.ProofLineage, s)
	if cwiFocusBindingsError(got) != cwiFocusBindingsError(want) {
		t.Fatal("complete assessment history first error changed")
	}
	after, _ := json.Marshal(s)
	if !bytes.Equal(before, after) {
		t.Fatal("lineage reuse changed raw source")
	}
}

func TestLineagePreimageSessionCompleteAssessmentOracle(t *testing.T) {
	for arm := uint8(0); arm < 12; arm++ {
		s := historicalSessionFixture(t)
		mutateHistoricalSession(&s, arm, "same hash raw drift")
		compareLineagePreimageSession(t, s)
		body, _ := json.Marshal(s)
		var loaded RuntimeLineageAssertionSources
		if err := json.Unmarshal(body, &loaded); err != nil {
			t.Fatal(err)
		}
		compareLineagePreimageSession(t, loaded)
	}
}

func TestLineagePreimageSessionSourceAndLaterRawMutation(t *testing.T) {
	s, assertion, expected := executionProofSourceFixture(t)
	session := &runtimeHistoricalPreimageSession{}
	for range 2 {
		got, err := runtimeExecutionProofSourceBindingInSession(assertion, s, session)
		want, wantErr := referenceLineagePreimageruntimeExecutionProofSourceBinding(assertion, s)
		if err != nil || wantErr != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(got, expected) {
			t.Fatal("source resolver differs", err, wantErr)
		}
		got.Proofs[0].RequiredCheckRefs = []string{"caller"}
	}
	s.Semantic.Manifest.OwnerCatalogs[0].SnapshotID = "same-hash raw change"
	got, gotErr := runtimeExecutionProofSourceBindingInSession(assertion, s, session)
	want, wantErr := referenceLineagePreimageruntimeExecutionProofSourceBinding(assertion, s)
	if gotErr == nil || cwiFocusBindingsError(gotErr) != cwiFocusBindingsError(wantErr) || !reflect.DeepEqual(got, want) {
		t.Fatal("session accepted later source drift")
	}
}

func FuzzLineagePreimageSessionCompleteOracle(f *testing.F) {
	for arm := uint8(0); arm < 12; arm++ {
		f.Add(arm, "原文\x00")
	}
	f.Fuzz(func(t *testing.T, arm uint8, text string) {
		if len(text) > 128 {
			t.Skip()
		}
		s := historicalSessionFixture(t)
		mutateHistoricalSession(&s, arm, text)
		compareLineagePreimageSession(t, s)
	})
}

func TestLineagePreimageSessionExplicitArchiveOracle(t *testing.T) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		t.Skip("explicit read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState RuntimeStateRecord }
	if err := json.Unmarshal(body, &runs); err != nil {
		t.Fatal(err)
	}
	for _, r := range runs {
		compareLineagePreimageSession(t, historicalSessionArchiveSources(r.RuntimeState))
		for _, mutate := range []func(*RuntimeStateRecord){
			func(r *RuntimeStateRecord) {
				r.SemanticPlanning.History[0].Snapshot.Manifest.OwnerCatalogs[0].SnapshotID = "same-hash raw drift"
			},
			func(r *RuntimeStateRecord) { r.OperationReceipts[0].Fence++ },
			func(r *RuntimeStateRecord) { r.RepairSurfaceConfirmations = nil },
			func(r *RuntimeStateRecord) { r.Facts.EvidenceRevision = InitialRuntimeEvidenceRevision },
		} {
			raw := r.RuntimeState.Clone()
			mutate(&raw)
			compareLineagePreimageSession(t, historicalSessionArchiveSources(raw))
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("read-only oracle altered archive")
	}
}

func BenchmarkLineagePreimageSessionPublicArchiveAssessmentHistory(b *testing.B) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		b.Skip("explicit read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState RuntimeStateRecord }
	if err := json.Unmarshal(body, &runs); err != nil {
		b.Fatal(err)
	}
	for _, r := range runs {
		s := historicalSessionArchiveSources(r.RuntimeState)
		for _, original := range []bool{true, false} {
			name := "session"
			if original {
				name = "original"
			}
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					var err error
					if original {
						err = referenceLineagePreimageValidateRuntimeAssessmentAdjudications(s.Facts, s.Completed, s.ProofLineage, s)
					} else {
						err = ValidateRuntimeAssessmentAdjudications(s.Facts, s.Completed, s.ProofLineage, s)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
