package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func TestCompleteAdmissionHasNoHistoricalReplayInputs(t *testing.T) {
	// Historical result/receipt/governance integrity belongs to canonical owner
	// admission. The final seal cannot acquire a second copy of those ledgers.
	typ := reflect.TypeFor[RuntimeCompleteAdmissionInput]()
	for _, name := range []string{"Definition", "ProcessedEventIDs", "Reconciliation", "CompletedDispatches", "OperationReceipts", "ProofLineage", "Governance", "Budget", "EvidenceSequence", "Resolutions", "Attempts", "Sessions", "Confirmations"} {
		if _, found := typ.FieldByName(name); found {
			t.Fatalf("final seal reintroduced historical input %s", name)
		}
	}
}
