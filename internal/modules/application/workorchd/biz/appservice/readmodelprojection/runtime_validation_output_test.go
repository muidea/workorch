package readmodelprojection

import (
	"bytes"
	"encoding/json"
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAuditValidationOutputLinksBindExactReceiptAndPreserveOmissions(t *testing.T) {
	workspaceID, runID := common.WorkspaceID("ws-output"), common.RunID("run-output")
	receipt := runtimecontract.RuntimeOperationReceipt{
		OperationID: "operation-output", Fence: 3,
		ExecutionKey: runtimecontract.StableRuntimeOperationExecutionKey(workspaceID, runID, "dispatch-output", "operation-output"),
		Evidence: runtimecontract.RuntimeOperationEvidence{Validation: &runtimecontract.RuntimeValidationObservation{
			ExecutionConjunction: &runtimecontract.RuntimeValidationExecutionConjunction{Stages: []runtimecontract.RuntimeValidationExecutionStage{{
				Identity: runtimecontract.RuntimeValidationExecutionStageIdentity{ID: "test"},
				Stdout:   runtimecontract.RuntimeValidationOutputIdentity{Bytes: 9, SavedBytes: 5, Complete: false, OmittedReason: "output_capacity_exceeded", Storage: &runtimecontract.RuntimeValidationOutputStorage{Reference: "private-storage"}},
				Stderr:   runtimecontract.RuntimeValidationOutputIdentity{Storage: &runtimecontract.RuntimeValidationOutputStorage{OmittedReason: "storage_unavailable"}},
			}}},
		}},
	}
	before, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	items := runtimeAuditOperationReceipts([]runtimecontract.RuntimeOperationReceipt{receipt}, workspaceID, runID)
	if len(items) != 1 || len(items[0].ValidationOutputs) != 2 {
		t.Fatalf("output inventory missing: %+v", items)
	}
	outputs := items[0].ValidationOutputs
	command := orchestratorevents.QueryRuntimeValidationOutputCommand{WorkspaceID: workspaceID, RunID: runID, DispatchID: "dispatch-output", OperationID: receipt.OperationID, ExecutionKey: receipt.ExecutionKey, Fence: receipt.Fence, SourceReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(receipt), StageID: "test", Stream: "stdout"}
	ref, err := command.ContentRef()
	if err != nil || outputs[0].Ref != ref || outputs[0].Complete || outputs[0].Bytes != 9 || outputs[0].SavedBytes != 5 || outputs[0].OmittedReason != "output_capacity_exceeded" {
		t.Fatalf("partial output link or identity lost: %+v err=%v", outputs[0], err)
	}
	if outputs[1].Ref != "" || outputs[1].StorageOmittedReason != "storage_unavailable" {
		t.Fatal("unsaved stream gained read link or lost reason")
	}
	clone := projectioncontract.CloneRuntimeAudit(&projectioncontract.RuntimeAudit{OperationReceipts: items})
	clone.OperationReceipts[0].ValidationOutputs[0].Ref = "caller"
	after, err := json.Marshal(receipt)
	if err != nil || items[0].ValidationOutputs[0].Ref != ref || !bytes.Equal(before, after) {
		t.Fatal("read projection aliases output or changed persisted receipt")
	}
}
