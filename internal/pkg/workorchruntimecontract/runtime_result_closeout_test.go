package workorchruntimecontract

import "testing"

func TestRuntimeResultCloseoutV2ValidatesOperationReceiptProjection(t *testing.T) {
	closeout := RuntimeResultCloseout{
		SchemaVersion: RuntimeResultCloseoutSchemaVersionCurrent,
		OperationOutcomes: []RuntimeOperationCloseout{{
			DispatchID: "dispatch-1", OperationID: "write-1", ExecutionKey: "exec/ws/run/dispatch-1/write-1",
			Fence:               1,
			ExpectedEffectScope: RuntimeEffectWorkspaceDurable, ObservedEffectScope: RuntimeEffectWorkspaceDurable,
			Outcome: RuntimeOperationOutcomeAccepted, ExecutionState: OperationExecutionEvidenceCommitted,
			ReceiptRef: "runtime-receipt/exec/ws/run/dispatch-1/write-1", ReceiptHash: "receipt-hash",
		}},
	}
	if err := closeout.Validate(); err != nil {
		t.Fatalf("closeout validate: %v", err)
	}
	invalid := closeout
	invalid.OperationOutcomes = append([]RuntimeOperationCloseout(nil), closeout.OperationOutcomes...)
	invalid.OperationOutcomes[0].ReceiptHash = ""
	if err := invalid.Validate(); err == nil {
		t.Fatal("owner outcome without receipt hash must fail")
	}
	nonCurrent := closeout
	nonCurrent.SchemaVersion = 1
	if err := nonCurrent.Validate(); err == nil {
		t.Fatal("non-current closeout schema must not be accepted by Runtime")
	}
}
