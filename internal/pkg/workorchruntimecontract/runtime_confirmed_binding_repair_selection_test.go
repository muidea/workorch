package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"
)

func compareConfirmedRepairSelection(t testing.TB, binding CoverageEvidenceBinding, s RuntimeLineageAssertionSources, cutoff string) bool {
	t.Helper()
	before := cloneRuntimeLineageSourcesRaw(s)
	beforeBinding := cloneCoverageEvidenceBindingRaw(binding)
	want := referenceConfirmedRepairBeforeCandidateSelection(binding, s, cutoff, &runtimeHistoricalPreimageSession{})
	for _, session := range []*runtimeHistoricalPreimageSession{nil, {}, {semantic: newRuntimeSemanticValidationSession(), sourceCompiler: &runtimeHistoricalSourceCompiler{}}} {
		for range 2 {
			if got := runtimeConfirmedBindingRepairInSession(binding, s, cutoff, session); got != want {
				t.Fatalf("confirmed repair selection differs from original at %q: %v != %v", cutoff, got, want)
			}
		}
	}
	if !reflect.DeepEqual(before, s) || !reflect.DeepEqual(beforeBinding, binding) {
		t.Fatal("repair query modified original source or binding")
	}
	return want
}

func TestConfirmedRepairSelectionNegativeDoesNotCompileUnrelatedSources(t *testing.T) {
	s := historicalSessionFixture(t)
	for _, d := range s.Completed {
		for _, binding := range d.CoverageBindings {
			for _, scenario := range []string{"absent", "not-committed", "no-state-change", "validation", "work", "other-binding"} {
				t.Run(scenario, func(t *testing.T) {
					value := cloneRuntimeLineageSourcesRaw(s)
					// Keep a raw current row even for a historical binding so the
					// first currentness check cannot explain the negative shortcut.
					for i := range value.Facts.Coverage {
						if value.Facts.Coverage[i].ID == binding.CoverageWorkItemID {
							ref := RuntimeCWIBindingRefForCoverageBinding(binding, "manifest")
							value.Facts.Coverage[i].CurrentBindingRef = &ref
						}
					}
					for i := range value.Completed {
						repair := &value.Completed[i]
						switch scenario {
						case "absent":
							repair.RepairCommitment = nil
						case "not-committed":
							repair.State = RuntimeDispatchStateInvalidated
						case "no-state-change":
							repair.Candidate.StateChange = nil
						case "validation":
							repair.Candidate.Validation = &ValidationCandidate{}
						case "work":
							repair.ImplementationWorkCommitment = &RuntimeImplementationWorkRound{}
						case "other-binding":
							repair.CoverageBindings = nil
						}
					}
					before := cloneRuntimeLineageSourcesRaw(value)
					session := &runtimeHistoricalPreimageSession{semantic: newRuntimeSemanticValidationSession()}
					if runtimeConfirmedBindingRepairInSession(binding, value, value.Facts.EvidenceRevision, session) || session.ledger != nil || len(session.lineageOperations) != 0 || len(session.semantic.workStates) != 0 {
						t.Fatal("missing repair dispatch rebuilt unrelated sources or received credit")
					}
					if !reflect.DeepEqual(before, value) {
						t.Fatal("negative selection modified input")
					}
					compareConfirmedRepairSelection(t, binding, value, value.Facts.EvidenceRevision)
				})
			}
			return
		}
	}
	t.Fatal("fixture lacks binding")
}

func TestConfirmedRepairSelectionCompleteArchive(t *testing.T) {
	body, path, runs := applicabilityScanArchive(t)
	positives := 0
	for _, run := range runs {
		s := historicalSessionArchiveSources(run.RuntimeState)
		for _, d := range s.Completed {
			for _, binding := range d.CoverageBindings {
				if compareConfirmedRepairSelection(t, binding, s, s.Facts.EvidenceRevision) {
					positives++
				}
			}
		}
		// Actual confirmed repair gives a positive path with exact original
		// source/receipt/fence/confirmation and a later ordinary validation.
		for _, dispatch := range s.Completed {
			if dispatch.RepairCommitment == nil || len(dispatch.CoverageBindings) == 0 {
				continue
			}
			binding := dispatch.CoverageBindings[0]
			for _, invalidate := range []func(*RuntimeDispatchRecord){
				func(d *RuntimeDispatchRecord) { d.RepairCommitment = nil },
				func(d *RuntimeDispatchRecord) { d.State = RuntimeDispatchStateInvalidated },
				func(d *RuntimeDispatchRecord) { d.Candidate.StateChange = nil },
				func(d *RuntimeDispatchRecord) { d.Candidate.Validation = &ValidationCandidate{} },
				func(d *RuntimeDispatchRecord) { d.ImplementationWorkCommitment = &RuntimeImplementationWorkRound{} },
				func(d *RuntimeDispatchRecord) { d.CoverageBindings = nil },
			} {
				value := cloneRuntimeLineageSourcesRaw(s)
				for i := range value.Completed {
					if value.Completed[i].RepairCommitment != nil {
						invalidate(&value.Completed[i])
					}
				}
				session := &runtimeHistoricalPreimageSession{semantic: newRuntimeSemanticValidationSession()}
				if runtimeConfirmedBindingRepairInSession(binding, value, value.Facts.EvidenceRevision, session) || session.ledger != nil || len(session.lineageOperations) != 0 {
					t.Fatal("ineligible actual repair compiled unrelated sources or gained credit")
				}
				compareConfirmedRepairSelection(t, binding, value, value.Facts.EvidenceRevision)
			}
			for _, mutate := range []func(*RuntimeLineageAssertionSources){
				func(v *RuntimeLineageAssertionSources) { v.Completed = nil },
				func(v *RuntimeLineageAssertionSources) { slices.Reverse(v.Completed) },
				func(v *RuntimeLineageAssertionSources) { v.Completed = append(v.Completed, v.Completed[0]) },
				func(v *RuntimeLineageAssertionSources) { v.Facts.WorkspaceID += "-other" },
				func(v *RuntimeLineageAssertionSources) { v.Facts.RunID += "-other" },
				func(v *RuntimeLineageAssertionSources) { v.Confirmations = nil },
				func(v *RuntimeLineageAssertionSources) { v.Receipts[0].Fence++ },
				func(v *RuntimeLineageAssertionSources) { v.Budget.Allocations = nil },
				func(v *RuntimeLineageAssertionSources) {
					v.Semantic.History = append(v.Semantic.History, RuntimeSemanticPlanningGeneration{})
				},
				func(v *RuntimeLineageAssertionSources) {
					for i := range v.Completed {
						if v.Completed[i].RepairCommitment != nil {
							v.Completed[i].RepairCommitmentHash += "-drift"
						}
					}
				},
			} {
				value := cloneRuntimeLineageSourcesRaw(s)
				mutate(&value)
				compareConfirmedRepairSelection(t, binding, value, value.Facts.EvidenceRevision)
			}
			for _, cutoff := range []string{"", "wrong-cutoff", InitialRuntimeEvidenceRevision, dispatch.Transaction.EvidenceRevision, s.Facts.EvidenceRevision} {
				compareConfirmedRepairSelection(t, binding, s, cutoff)
			}
			encoded, err := json.Marshal(s)
			var reloaded RuntimeLineageAssertionSources
			if err != nil || json.Unmarshal(encoded, &reloaded) != nil {
				t.Fatal("source reload failed")
			}
			compareConfirmedRepairSelection(t, binding, reloaded, reloaded.Facts.EvidenceRevision)
		}
	}
	if positives == 0 {
		t.Fatal("archive replay has no actual positive confirmed repair")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(body, after) {
		t.Fatal("read-only repair replay changed archive")
	}
}

func BenchmarkConfirmedRepairSelectionCompleteArchive(b *testing.B) {
	_, _, runs := applicabilityScanArchive(b)
	for _, run := range runs {
		s := historicalSessionArchiveSources(run.RuntimeState)
		var bindings []CoverageEvidenceBinding
		for _, d := range s.Completed {
			bindings = append(bindings, d.CoverageBindings...)
		}
		for _, original := range []bool{true, false} {
			name := "current"
			if original {
				name = "reference"
			}
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					session := &runtimeHistoricalPreimageSession{semantic: newRuntimeSemanticValidationSession(), sourceCompiler: &runtimeHistoricalSourceCompiler{}}
					for _, binding := range bindings {
						if original {
							referenceConfirmedRepairBeforeCandidateSelection(binding, s, s.Facts.EvidenceRevision, session)
						} else {
							runtimeConfirmedBindingRepairInSession(binding, s, s.Facts.EvidenceRevision, session)
						}
					}
				}
			})
		}
	}
}
