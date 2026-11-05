package workorchruntimecontract

import (
	"fmt"
	"testing"
)

func TestConfirmedRepairHistoricalCutoffDoesNotExpandFutureSources(t *testing.T) {
	_, _, runs := applicabilityScanArchive(t)
	count := 0
	for _, run := range runs {
		s := historicalSessionArchiveSources(run.RuntimeState)
		for _, repair := range s.Completed {
			if repair.RepairCommitment == nil || len(repair.CoverageBindings) != 1 {
				continue
			}
			binding := repair.CoverageBindings[0]
			// Replay every original evidence fence, including before/equal/after the
			// rejected baseline. The full original consumer is the independent oracle.
			for seq := uint64(0); seq <= s.EvidenceSequence; seq++ {
				cutoff := fmt.Sprintf("runtime-evidence-%020d", seq)
				want := referenceConfirmedRepairBeforeCandidateSelection(binding, s, cutoff, &runtimeHistoricalPreimageSession{})
				session := &runtimeHistoricalPreimageSession{semantic: newRuntimeSemanticValidationSession(), sourceCompiler: &runtimeHistoricalSourceCompiler{}}
				if before := historyCutoffReferenceRepair(binding, s, cutoff, &runtimeHistoricalPreimageSession{}); before != want {
					t.Fatalf("same-turn baseline differs at %s", cutoff)
				}
				if got := runtimeConfirmedBindingRepairInSession(binding, s, cutoff, session); got != want {
					t.Fatalf("cutoff %s differs: %v != %v", cutoff, got, want)
				}
				if !EvidenceRevisionAfter(cutoff, repair.Transaction.EvidenceRevision) && (session.ledger != nil || len(session.lineageOperations) != 0) {
					t.Fatalf("future repair expanded at %s", cutoff)
				}
			}
			if !compareConfirmedRepairSelection(t, binding, s, s.Facts.EvidenceRevision) {
				t.Fatal("original actual repair positive lost")
			}
			count++
		}
	}
	if count == 0 {
		t.Fatal("no original confirmed repair")
	}
}

func BenchmarkConfirmedRepairHistoryCutoff(b *testing.B) {
	_, _, runs := applicabilityScanArchive(b)
	for _, run := range runs {
		s := historicalSessionArchiveSources(run.RuntimeState)
		for _, repair := range s.Completed {
			if repair.RepairCommitment == nil || len(repair.CoverageBindings) != 1 {
				continue
			}
			binding := repair.CoverageBindings[0]
			for _, name := range []string{"before", "current"} {
				b.Run(name, func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						session := &runtimeHistoricalPreimageSession{semantic: newRuntimeSemanticValidationSession(), sourceCompiler: &runtimeHistoricalSourceCompiler{}}
						for _, d := range s.Completed {
							if d.Candidate.Assessment == nil {
								continue
							}
							if name == "before" {
								historyCutoffReferenceRepair(binding, s, d.Transaction.EvidenceRevision, session)
							} else {
								runtimeConfirmedBindingRepairInSession(binding, s, d.Transaction.EvidenceRevision, session)
							}
						}
					}
				})
			}
		}
	}
}

func TestConfirmedRepairFutureCutoffRejectsBeforeHistoricalCompilation(t *testing.T) {
	s := historicalSessionFixture(t)
	for _, d := range s.Completed {
		for _, binding := range d.CoverageBindings {
			value := cloneRuntimeLineageSourcesRaw(s)
			for i := range value.Facts.Coverage {
				if value.Facts.Coverage[i].ID == binding.CoverageWorkItemID {
					ref := RuntimeCWIBindingRefForCoverageBinding(binding, "original-manifest")
					value.Facts.Coverage[i].CurrentBindingRef = &ref
				}
			}
			repair := RuntimeDispatchRecord{State: RuntimeDispatchStateCommitted, Candidate: RuntimeCandidate{StateChange: &StateChangeCandidate{}}, CoverageBindings: []CoverageEvidenceBinding{binding}, RepairCommitment: &RuntimeDispatchRepairCommitment{}}
			repair.Transaction.EvidenceRevision = "runtime-evidence-00000000000000000005"
			value.Completed = []RuntimeDispatchRecord{repair}
			for _, cutoff := range []string{InitialRuntimeEvidenceRevision, repair.Transaction.EvidenceRevision} {
				session := &runtimeHistoricalPreimageSession{semantic: newRuntimeSemanticValidationSession(), sourceCompiler: &runtimeHistoricalSourceCompiler{}}
				if runtimeConfirmedBindingRepairInSession(binding, value, cutoff, session) || session.ledger != nil || len(session.lineageOperations) != 0 {
					t.Fatal("future repair compiled history or gained credit")
				}
				if historyCutoffReferenceRepair(binding, value, cutoff, &runtimeHistoricalPreimageSession{}) {
					t.Fatal("original negative differs")
				}
			}
			return
		}
	}
	t.Fatal("fixture lacks original binding")
}
