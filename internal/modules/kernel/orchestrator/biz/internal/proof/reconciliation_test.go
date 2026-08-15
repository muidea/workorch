package proof

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestReconciliationFactsPreserveDuplicateAndStaleOwnerFences(t *testing.T) {
	binding := runtimecontract.RuntimeCWIReconciliationBindingRef{
		CoverageWorkItemID: "cwi-1", BindingID: "binding-1", BindingHash: "binding-hash-1",
		OwnerModuleIDs: []string{runtimecontract.RuntimeModuleFile}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
	}
	contract := runtimecontract.RuntimeCWIReconciliationContract{
		SchemaVersion: runtimecontract.RuntimeCWIReconciliationContractSchemaCurrent, DefinitionRevision: "definition-1", Bindings: []runtimecontract.RuntimeCWIReconciliationBindingRef{binding},
	}
	contract.BindingSetHash = runtimecontract.StableRuntimeCWIReconciliationContractHash(contract)
	request := runtimecontract.RuntimeCWIReconciliationRequest{
		RequestID: "request-1", WorkspaceID: "workspace-1", RunID: "run-1", Scope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun,
		DefinitionRevision: contract.DefinitionRevision, BindingSetHash: contract.BindingSetHash, Bindings: contract.Bindings,
		WorkspaceBaselineRevision: "tree-1", WorkspaceBaselineRef: "workspace-tree/tree-1", Generation: 1,
	}
	observation := runtimecontract.RuntimeCWIReconciliationObservation{
		RequestID: request.RequestID, WorkspaceID: request.WorkspaceID, RunID: request.RunID, Generation: request.Generation,
		CoverageWorkItemID: binding.CoverageWorkItemID, BindingID: binding.BindingID, BindingHash: binding.BindingHash,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, ObservedRevision: "tree-1", Status: runtimecontract.RuntimeCWIReconciliationObservationStale,
	}
	batch := runtimecontract.RuntimeCWIReconciliationBatch{
		RequestID: request.RequestID, WorkspaceID: request.WorkspaceID, RunID: request.RunID, Generation: request.Generation,
		BindingSetHash: request.BindingSetHash, Observations: []runtimecontract.RuntimeCWIReconciliationObservation{observation},
	}
	batch.Hash = runtimecontract.StableRuntimeCWIReconciliationBatchHash(batch)
	if err := batch.ValidateForRequest(request); err != nil {
		t.Fatalf("fixture batch: %v", err)
	}

	facts := CompareReconciliationBinding(binding, batch)
	if facts.AllOwnersSatisfied || len(facts.Observations) != 1 || facts.ObservationRef == "" || facts.UnsatisfiedReason != "current-run owner observation is stale" {
		t.Fatalf("stale comparison = %+v", facts)
	}
	generation := runtimecontract.RuntimeCWIReconciliationGeneration{
		Superseded: true, Request: request, RequestHash: runtimecontract.StableRuntimeCWIReconciliationRequestHash(request), Batch: &batch,
	}
	if !ReconciliationHistoryContainsExactBatch([]runtimecontract.RuntimeCWIReconciliationGeneration{generation}, batch) {
		t.Fatal("exact historical batch was not recognized")
	}
	delayed := batch.Clone()
	delayed.RequestID = "request-delayed"
	delayed.Hash = runtimecontract.StableRuntimeCWIReconciliationBatchHash(delayed)
	if ReconciliationHistoryContainsExactBatch([]runtimecontract.RuntimeCWIReconciliationGeneration{generation}, delayed) {
		t.Fatal("different request identity was treated as an exact historical batch")
	}

	// The comparison must not mutate the owner batch supplied by the caller.
	if batch.Observations[0].Status != runtimecontract.RuntimeCWIReconciliationObservationStale || batch.Observations[0].CoverageWorkItemID != common.CoverageWorkItemID("cwi-1") {
		t.Fatalf("comparison mutated input batch: %+v", batch)
	}
}
