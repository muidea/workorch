package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func TestSemanticLedgerClonePreservesCommittedHistoryOriginals(t *testing.T) {
	for _, segmented := range []bool{false, true} {
		name := "tail"
		ledger := assessmentPreimageSessionFixture(t)
		if segmented {
			name = "segments"
			ledger = semanticSegmentedHistoryFixture(t, 33)
		}
		t.Run(name, func(t *testing.T) {
			before := cloneRuntimeSemanticPlanningLedgerRaw(ledger)
			copy := ledger.CloneRaw()
			if !reflect.DeepEqual(copy.History, before.History) || !reflect.DeepEqual(copy.HistoryArchive, before.HistoryArchive) {
				t.Fatal("clone normalized committed generation originals")
			}
			if err := ValidateRuntimeCommittedHistoryTransition(RuntimeStateRecord{SemanticPlanning: ledger}, RuntimeStateRecord{SemanticPlanning: copy}); err != nil {
				t.Fatal("clone alone invalidated committed lineage", err)
			}
			for _, generation := range copy.AllHistory() {
				generation.Snapshot.Manifest.Hash = "caller mutation"
				if len(generation.PrerequisiteIDs) > 0 {
					generation.PrerequisiteIDs[0] = "caller mutation"
				}
			}
			if !reflect.DeepEqual(ledger, before) {
				t.Fatal("clone aliases an original generation")
			}
			if err := ValidateRuntimeCommittedHistoryTransition(RuntimeStateRecord{SemanticPlanning: ledger}, RuntimeStateRecord{SemanticPlanning: copy}); err == nil {
				t.Fatal("mutation of cloned history bypassed immutable-result gate")
			}
		})
	}
}
