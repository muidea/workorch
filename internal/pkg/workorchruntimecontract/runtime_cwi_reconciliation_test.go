package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCompileRuntimeCWIReconciliationContractBuildsScopedOwnerRefs(t *testing.T) {
	contract, err := CompileRuntimeCWIReconciliationContract("definition-1", []CoverageFocus{runtimeCWIReconciliationFocus()})
	if err != nil {
		t.Fatalf("CompileRuntimeCWIReconciliationContract() error = %v", err)
	}
	if err := contract.Validate(); err != nil {
		t.Fatalf("contract.Validate() error = %v", err)
	}
	if len(contract.Bindings) != 1 {
		t.Fatalf("bindings = %#v, want exactly one", contract.Bindings)
	}
	binding := contract.Bindings[0]
	if binding.CoverageWorkItemID != "cwi-1" || binding.BindingID != "binding-1" ||
		len(binding.OwnerModuleIDs) != 1 || binding.OwnerModuleIDs[0] != RuntimeModuleShell {
		t.Fatalf("binding ref = %#v, want shell-owned sealed binding", binding)
	}
	if contract.BindingSetHash == "" || contract.BindingSetHash != StableRuntimeCWIReconciliationContractHash(contract) {
		t.Fatalf("binding set hash = %q, want stable hash", contract.BindingSetHash)
	}
}

func TestRuntimeCWIReconciliationRequestAndBatchRejectScopeAndCompletenessDrift(t *testing.T) {
	contract, err := CompileRuntimeCWIReconciliationContract("definition-1", []CoverageFocus{runtimeCWIReconciliationFocus()})
	if err != nil {
		t.Fatal(err)
	}
	request := RuntimeCWIReconciliationRequest{
		RequestID:                 "reconcile-1",
		WorkspaceID:               "ws-1",
		RunID:                     "run-1",
		Scope:                     RuntimeCWIReconciliationScopeCurrentRun,
		DefinitionRevision:        contract.DefinitionRevision,
		BindingSetHash:            contract.BindingSetHash,
		Bindings:                  contract.Bindings,
		WorkspaceBaselineRevision: "tree-1",
		WorkspaceBaselineRef:      "workspace-tree/ws-1/tree-1",
		Generation:                1,
	}
	if err := request.ValidateAgainstContract(contract); err != nil {
		t.Fatalf("request.ValidateAgainstContract() error = %v", err)
	}

	observation := RuntimeCWIReconciliationObservation{
		RequestID:          request.RequestID,
		WorkspaceID:        request.WorkspaceID,
		RunID:              request.RunID,
		Generation:         request.Generation,
		CoverageWorkItemID: "cwi-1",
		BindingID:          "binding-1",
		BindingHash:        contract.Bindings[0].BindingHash,
		OwnerModuleID:      RuntimeModuleShell,
		ObservedRevision:   "tree-1",
		Status:             RuntimeCWIReconciliationObservationSatisfied,
		TargetRefs:         []string{"main.go"},
		CheckRefs:          []string{"check/main"},
	}
	batch := RuntimeCWIReconciliationBatch{
		RequestID: request.RequestID, WorkspaceID: request.WorkspaceID, RunID: request.RunID,
		Generation: request.Generation, BindingSetHash: request.BindingSetHash,
		Observations: []RuntimeCWIReconciliationObservation{observation},
	}
	if ref := StableRuntimeCWIReconciliationObservationRef(observation); ref == "" {
		t.Fatal("canonical observation did not produce stable lineage ref")
	}
	batch.Hash = StableRuntimeCWIReconciliationBatchHash(batch)
	if err := batch.ValidateForRequest(request); err != nil {
		t.Fatalf("batch.ValidateForRequest() error = %v", err)
	}

	wrongWorkspace := observation
	wrongWorkspace.WorkspaceID = "ws-2"
	badScope := batch
	badScope.Observations = []RuntimeCWIReconciliationObservation{wrongWorkspace}
	badScope.Hash = StableRuntimeCWIReconciliationBatchHash(badScope)
	if err := badScope.ValidateForRequest(request); err == nil {
		t.Fatal("batch with cross-workspace observation unexpectedly validated")
	}

	duplicate := batch
	duplicate.Observations = append(duplicate.Observations, observation)
	duplicate.Hash = StableRuntimeCWIReconciliationBatchHash(duplicate)
	if err := duplicate.ValidateForRequest(request); err == nil {
		t.Fatal("batch with duplicate binding-owner observation unexpectedly validated")
	}
}

func TestRuntimeCWIReconciliationBatchHashExcludesOriginalDiagnosticText(t *testing.T) {
	contract, err := CompileRuntimeCWIReconciliationContract("definition-1", []CoverageFocus{runtimeCWIReconciliationFocus()})
	if err != nil {
		t.Fatal(err)
	}
	observation := RuntimeCWIReconciliationObservation{
		RequestID: "reconcile-1", WorkspaceID: "ws-1", RunID: "run-1", Generation: 1,
		CoverageWorkItemID: "cwi-1", BindingID: "binding-1", BindingHash: contract.Bindings[0].BindingHash,
		OwnerModuleID: RuntimeModuleShell, ObservedRevision: "tree-1", Status: RuntimeCWIReconciliationObservationUnverifiable,
		OriginalDiagnostic: &RuntimeOriginalDiagnostic{
			SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: RuntimeModuleShell,
			OriginalClass: "validation", OriginalCode: "missing_check", OriginalMessage: "first safe message", Signature: "missing-check",
		},
	}
	batch := RuntimeCWIReconciliationBatch{
		RequestID: "reconcile-1", WorkspaceID: "ws-1", RunID: "run-1", Generation: 1,
		BindingSetHash: contract.BindingSetHash, Observations: []RuntimeCWIReconciliationObservation{observation},
	}
	first := StableRuntimeCWIReconciliationBatchHash(batch)
	batch.Observations[0].OriginalDiagnostic.OriginalMessage = "second safe message"
	if second := StableRuntimeCWIReconciliationBatchHash(batch); second != first {
		t.Fatalf("batch hash changed with original diagnostic text: %q -> %q", first, second)
	}
}

func runtimeCWIReconciliationFocus() CoverageFocus {
	binding := CoverageEvidenceBinding{
		ID: "binding-1", CoverageWorkItemID: common.CoverageWorkItemID("cwi-1"), FocusID: "focus-1", DefinitionRevision: "definition-1",
		Proofs: []CoverageEvidenceProof{{
			EvidenceKind: EvidenceKindValidation, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone},
			AllowedOperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "recipe/main", RequiredCheckRefs: []string{"check/main"},
		}},
		TargetRefs: []string{"main.go"}, DeclarationRef: "profile/test",
	}
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	return CoverageFocus{
		ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindValidation},
		CoverageEvidenceBindings: []CoverageEvidenceBinding{binding},
	}
}
