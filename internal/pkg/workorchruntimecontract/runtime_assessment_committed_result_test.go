package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func TestCommittedAssessmentReadPreservesHistoricalResultWithoutReclassification(t *testing.T) {
	s := historicalSessionFixture(t)
	if err := auditCommittedAssessmentAdjudications(s.Facts, s.Completed, s.ProofLineage, s); err != nil {
		t.Fatal(err)
	}
	original := cloneRuntimeLineageSourcesRaw(s)
	// Current mutable views need not agree with the old result. These changes
	// require their own current applicability/reconciliation, not a rewrite of
	// the old assessment. The read API deliberately has no proof-graph input.
	s.Facts.Resources.OperationAdmissibilities = nil
	s.Facts.Focus = CoverageFocus{}
	s.Facts.EvidenceRevision += "-later"
	for range 2 {
		if err := ValidateRuntimeAssessmentAdjudications(s.Facts, s.Completed); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(original.Facts.Assessments, s.Facts.Assessments) || !reflect.DeepEqual(original.Completed, s.Completed) {
		t.Fatal("read rewrote the historical result")
	}
	count := 0
	for _, d := range s.Completed {
		if d.Candidate.Assessment == nil || d.State != RuntimeDispatchStateCommitted {
			continue
		}
		for _, f := range s.Facts.Assessments {
			if f.ID != d.ID {
				continue
			}
			count++
			for name, mutate := range map[string]func(*RuntimeDispatchRecord, *RuntimeAssessmentFact, *RuntimeFacts){
				"missing double entry": func(d *RuntimeDispatchRecord, _ *RuntimeAssessmentFact, _ *RuntimeFacts) {
					d.AssessmentAdjudication = nil
				},
				"wrong fact identity": func(_ *RuntimeDispatchRecord, f *RuntimeAssessmentFact, _ *RuntimeFacts) { f.ID += "-other" },
				"wrong focus":         func(_ *RuntimeDispatchRecord, f *RuntimeAssessmentFact, _ *RuntimeFacts) { f.FocusID += "-other" },
				"wrong hash": func(_ *RuntimeDispatchRecord, f *RuntimeAssessmentFact, _ *RuntimeFacts) {
					f.AdjudicationHash = "corrupt"
				},
				"missing evidence": func(_ *RuntimeDispatchRecord, _ *RuntimeAssessmentFact, f *RuntimeFacts) { f.Evidence.Accepted = nil },
				"wrong workspace":  func(_ *RuntimeDispatchRecord, _ *RuntimeAssessmentFact, f *RuntimeFacts) { f.WorkspaceID = "other" },
			} {
				t.Run(name, func(t *testing.T) {
					changed := cloneRuntimeLineageSourcesRaw(s)
					dispatch := cloneRuntimeDispatchRecordRaw(d)
					fact := f
					mutate(&dispatch, &fact, &changed.Facts)
					if err := ValidateRuntimeAssessmentAdjudication(dispatch, fact, changed.Facts); err == nil {
						t.Fatal("missing or corrupt original accepted")
					}
				})
			}
		}
	}
	if count == 0 {
		t.Fatal("fixture did not contain a committed assessment")
	}
}

func TestCommittedAssessmentReadOnlyArchiveAndOfflineAudit(t *testing.T) {
	body, path, runs := runtimeStateConsumerArchive(t)
	count := 0
	for _, run := range runs {
		r := run.RuntimeState
		s := historicalSessionArchiveSources(r)
		if err := auditCommittedAssessmentAdjudications(s.Facts, s.Completed, s.ProofLineage, s); err != nil {
			t.Fatal("original offline audit", err)
		}
		for range 2 {
			if err := ValidateRuntimeAssessmentAdjudications(r.Facts, r.CompletedDispatches); err != nil {
				t.Fatal(err)
			}
		}
		for _, d := range r.CompletedDispatches {
			if d.Candidate.Assessment != nil && d.State == RuntimeDispatchStateCommitted {
				count++
			}
		}
	}
	if count == 0 {
		t.Fatal("archive contained no committed results")
	}
	t.Logf("read and original offline audit agree for %d committed assessments", count)
	assertReadOnlyRuntimeStateConsumerArchive(t, body, path)
}

func BenchmarkCommittedAssessmentReadArchive(b *testing.B) {
	_, _, runs := runtimeStateConsumerArchive(b)
	for _, run := range runs {
		s := historicalSessionArchiveSources(run.RuntimeState)
		for _, mode := range []string{"full_historical_replay", "committed_result_read"} {
			b.Run(mode, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var err error
					if mode == "full_historical_replay" {
						err = auditCommittedAssessmentAdjudications(s.Facts, s.Completed, s.ProofLineage, s)
					} else {
						err = ValidateRuntimeAssessmentAdjudications(s.Facts, s.Completed)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
