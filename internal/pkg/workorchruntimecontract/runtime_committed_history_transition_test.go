package workorchruntimecontract

import (
	"errors"
	"github.com/muidea/workorch/internal/pkg/common"
	"reflect"
	"testing"
)

func TestCommittedHistoryPreservesOriginalsAndAllowsPackingAndCurrentChanges(t *testing.T) {
	before := RuntimeStateRecord{SemanticPlanning: RuntimeSemanticPlanningLedger{History: []RuntimeSemanticPlanningGeneration{{RetiredAtStateRevision: 8, Reason: RuntimeSemanticPlanningRetirementResourceInvalidated}}},
		ImplementationWork: RuntimeImplementationWorkLedger{Rounds: []RuntimeImplementationWorkRound{{CompletedReceiptSetHash: "collective", Hash: "original"}}},
		Facts:              RuntimeFacts{Evidence: EvidenceLedger{Accepted: []EvidenceFact{{ID: "z", Summary: "原始结果"}}}}}
	// Packing validation remains a separate current-schema gate. The transition
	// invariant preserves bodies across storage containers without rereading the
	// semantic inputs that originally produced them.
	after := before.Clone()
	after.SemanticPlanning.HistoryArchive = &RuntimeSemanticHistoryArchive{Segments: [][]RuntimeSemanticPlanningGeneration{after.SemanticPlanning.History}}
	after.SemanticPlanning.History = nil
	after.Facts.Evidence.Accepted = append([]EvidenceFact{{ID: "a", Summary: "新事实"}}, after.Facts.Evidence.Accepted...)
	if err := ValidateRuntimeCommittedHistoryTransition(before, after); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RuntimeStateRecord){
		"removed semantic history": func(r *RuntimeStateRecord) { r.SemanticPlanning.HistoryArchive = nil },
		"rewritten semantic history": func(r *RuntimeStateRecord) {
			r.SemanticPlanning.HistoryArchive.Segments[0][0].Reason = RuntimeSemanticPlanningRetirementInputRevision
		},
		"rehash completed round":      func(r *RuntimeStateRecord) { r.ImplementationWork.Rounds[0].Hash = "new self hash" },
		"rewritten original evidence": func(r *RuntimeStateRecord) { r.Facts.Evidence.Accepted[1].Summary = "replacement" },
		"cross workspace":             func(r *RuntimeStateRecord) { r.Facts.WorkspaceID = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := after.Clone()
			mutate(&changed)
			if err := ValidateRuntimeCommittedHistoryTransition(before, changed); !errors.Is(err, common.ErrConflict) {
				t.Fatal("original rewrite admitted", err)
			}
		})
	}
}

func TestCommittedTransitionFixedSnapshotDoesNotReadmitHistoryOrHideNewMissingInput(t *testing.T) {
	_, _, runs := runtimeStateConsumerArchive(t)
	for _, run := range runs {
		before := run.RuntimeState
		if err := before.Validate(); err != nil {
			t.Fatal("source must first pass independent cold audit", err)
		}
		after := before.Clone()
		after.StateRevision++
		original := before.Clone()
		image := RuntimeOwnerStateSourceImage{Converted: &after, AggregateJoins: func(*RuntimeOwnerInvariantSourceGate) error { return nil }}
		if err := CompileRuntimeOwnerStateTransition(before, image); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, original) {
			t.Fatal("transition mutated formal preimage")
		}
		// This is a new changed input. Its self hash cannot borrow any previous
		// successful catalog/semantic source result.
		after.Facts.DefinitionRevision = ""
		if err := CompileRuntimeOwnerStateTransition(before, image); err == nil {
			t.Fatal("new missing required source admitted")
		}
	}
}

func BenchmarkRuntimeCommittedStateTransition(b *testing.B) {
	_, _, runs := runtimeStateConsumerArchive(b)
	for _, run := range runs {
		before := run.RuntimeState
		if err := before.Validate(); err != nil {
			b.Fatal(err)
		}
		after := before.Clone()
		after.StateRevision++
		image := RuntimeOwnerStateSourceImage{Converted: &after, AggregateJoins: func(*RuntimeOwnerInvariantSourceGate) error { return nil }}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := CompileRuntimeOwnerStateTransition(before, image); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func TestCommittedDispatchHistorySeparatesLateProgressFromSettledResults(t *testing.T) {
	for _, state := range []RuntimeDispatchState{RuntimeDispatchStateInvalidated, RuntimeDispatchStateCommitted} {
		t.Run(string(state), func(t *testing.T) {
			dispatch := RuntimeDispatchRecord{ID: "original", State: state,
				OperationExecutions: []RuntimeOperationExecutionRecord{{State: OperationExecutionClaimed}}}
			dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(dispatch)
			before := RuntimeStateRecord{CompletedDispatches: []RuntimeDispatchRecord{dispatch}}
			after := before.Clone()
			after.CompletedDispatches[0].OperationExecutions[0].State = OperationExecutionEvidenceCommitted
			if err := ValidateRuntimeCommittedHistoryTransition(before, after); err != nil {
				t.Fatal("late original execution progress rejected", err)
			}
			changed := after.Clone()
			changed.CompletedDispatches[0].FocusTargetRole = "replacement authority"
			changed.CompletedDispatches[0].DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(changed.CompletedDispatches[0])
			if err := ValidateRuntimeCommittedHistoryTransition(before, changed); !errors.Is(err, common.ErrConflict) {
				t.Fatal("late execution changed original authority", err)
			}
			if state == RuntimeDispatchStateCommitted {
				changed = after.Clone()
				changed.CompletedDispatches[0].OperationExecutions[0].State = OperationExecutionClaimed
				if err := ValidateRuntimeCommittedHistoryTransition(after, changed); !errors.Is(err, common.ErrConflict) {
					t.Fatal("settled result rewound", err)
				}
			}
		})
	}
}

func TestCommittedTransitionRejectsReloadImage(t *testing.T) {
	before := RuntimeStateRecord{}
	image := RuntimeOwnerStateSourceImage{Original: &before, Converted: &before,
		AggregateJoins: func(*RuntimeOwnerInvariantSourceGate) error {
			t.Fatal("reload image reached postimage compiler")
			return nil
		}}
	if err := CompileRuntimeOwnerStateTransition(before, image); err == nil {
		t.Fatal("reload was accepted as a postimage")
	}
}
