package file

import (
	"os"
	"path/filepath"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestReviewedFileEditPreflightIsOptInChangedAndSideEffectFree(t *testing.T) {
	directory := t.TempDir()
	before, after := "original input\n", "corrected input\n"
	path := filepath.Join(directory, "input")
	if err := os.WriteFile(path, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(directory, "input-catalog", []string{runtimecontract.StepKindFileEdit}, []string{"input"}, 4)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256Hex([]byte(before))
	operation := runtimecontract.RuntimeOperation{ID: "edit", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"input"}, Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &before}, {Name: "newText", String: &after}, {Name: "baseHash", String: &hash}}}
	failure, stale, proof := adapter.ProbeRuntimeOperationFeasibilityWithReplayProof(directory, inventory, operation, nil)
	if failure != nil || stale != nil || proof != nil {
		t.Fatal("ordinary edit changed its original preflight behavior")
	}
	failure, stale, proof = adapter.probeRuntimeOperationFeasibilityWithReplayProof(directory, inventory, operation, nil, true)
	if failure != nil || stale != nil || proof == nil || proof.BeforeHash != hash || proof.AfterHash != sha256Hex([]byte(after)) || proof.Validate() != nil {
		t.Fatalf("reviewed edit lacks exact before/after proof: %+v %+v %+v", failure, stale, proof)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != before {
		t.Fatal("feasibility changed actual input")
	}
	operation.Parameters[1].String = &before
	failure, _, proof = adapter.probeRuntimeOperationFeasibilityWithReplayProof(directory, inventory, operation, nil, true)
	if failure == nil || proof != nil {
		t.Fatal("no-op opened independent re-review route")
	}
}
