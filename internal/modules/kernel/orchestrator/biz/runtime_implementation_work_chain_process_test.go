package orchestrator

import (
	"context"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	contract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestImplementationWorkFourTargetsFreshProcessMidBatchRecovery(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "workspace")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	runImplementationWorkProcess(t, root, "chain-cut", implementationWorkProcessCutExit)
	raw, err := os.ReadFile(filepath.Join(root, "runs.json"))
	if err != nil {
		t.Fatal(err)
	}
	beforePath := filepath.Join(root, "before-runs.json")
	if err = os.WriteFile(beforePath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	original, err := filerepository.NewRunRepository(beforePath).GetByWorkspaceID(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	before := original.RuntimeState
	if before == nil {
		t.Fatal("no persisted state")
	}
	if err = before.Validate(); err != nil {
		t.Fatal(err)
	}
	if before.ActiveDispatch == nil || len(before.OperationReceipts) != 1 || len(before.CompletedDispatches) != 0 || before.ImplementationWork.Rounds[0].CompletedReceiptSetHash != "" {
		t.Fatal("cut is not the original partially committed batch")
	}
	if before.ActiveDispatch.OperationExecutions[0].State != contract.OperationExecutionEvidenceCommitted || len(before.ActiveDispatch.CompletedOperationIDs) != 1 || before.ActiveDispatch.ImplementationWorkCommitment.WorkingBinding.ReceiptProtocolRef != contract.RuntimeImplementationWorkReceiptChainV1 {
		t.Fatal("first receipt and exact chain protocol were not sealed in original CAS")
	}
	assertImplementationWorkProcessBudget(t, *before, 1)
	assertImplementationWorkProcessNoTerminalCredit(t, *before)
	runImplementationWorkProcess(t, root, "chain-resume", 0)
	restored, err := filerepository.NewRunRepository(filepath.Join(root, "runs.json")).GetByWorkspaceID(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	after := restored.RuntimeState
	if after == nil {
		t.Fatal("no restored state")
	}
	if err = after.Validate(); err != nil {
		t.Fatal(err)
	}
	if after.ActiveDispatch != nil || len(after.OperationReceipts) != 8 || len(after.ImplementationWork.Rounds) != 2 || after.ImplementationWork.Rounds[1].CompletedReceiptSetHash == "" {
		t.Fatal("four-target two-round chain did not complete")
	}
	if !reflect.DeepEqual(before.Definition, after.Definition) || !reflect.DeepEqual(original.RuntimeBootstrap, restored.RuntimeBootstrap) || !reflect.DeepEqual(before.Facts.Recovery, after.Facts.Recovery) {
		t.Fatal("restart rewrote original authority or consumed extra recovery")
	}
	receipt, found := contract.RuntimeOperationReceiptForExecutionKeyFence(after.OperationReceipts, before.OperationReceipts[0].ExecutionKey, before.OperationReceipts[0].Fence)
	if !found || !reflect.DeepEqual(receipt, before.OperationReceipts[0]) {
		t.Fatal("first receipt was replaced or replayed under a new identity")
	}
	if !reflect.DeepEqual(before.ActiveDispatch.ImplementationWorkCommitment, after.CompletedDispatches[0].ImplementationWorkCommitment) {
		t.Fatal("recovery rewrote original sealed batch")
	}
	for _, prior := range before.ExecutionBudget.Consumptions {
		matches := 0
		for _, current := range after.ExecutionBudget.Consumptions {
			if reflect.DeepEqual(prior, current) {
				matches++
			}
		}
		if matches != 1 {
			t.Fatal("original consumption lost or duplicated")
		}
	}
	for _, prior := range before.ExecutionBudget.Allocations {
		matches := 0
		for _, current := range after.ExecutionBudget.Allocations {
			if reflect.DeepEqual(prior, current) {
				matches++
			}
		}
		if matches != 1 {
			t.Fatal("original allocation lost or refunded")
		}
	}
	for _, target := range []string{"resource-a", "resource-b", "resource-c", "resource-d"} {
		data, err := os.ReadFile(filepath.Join(directory, target))
		if err != nil || string(data) != "实际工作内容-2" {
			t.Fatalf("%s: %q / %v", target, data, err)
		}
	}
	assertImplementationWorkProcessBudget(t, *after, 8)
	assertImplementationWorkProcessNoTerminalCredit(t, *after)
}
