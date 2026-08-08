package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeExpectedDocumentContractValidatesOperationsAndSemanticBatch(t *testing.T) {
	operations := RuntimeExpectedDocumentContract{
		SchemaVersion: RuntimeExpectedDocumentSchemaCurrent, DocumentKind: RuntimeProviderDocumentOperations,
		ManifestHash: "manifest-1", LogicalRequestID: "request-1", StablePrefixHash: "prefix-1",
		MaxOutputTokens: 8192, MaxRecoveryAttempts: 3,
	}
	operations.Hash = StableRuntimeExpectedDocumentHash(operations)
	if err := operations.Validate(); err != nil {
		t.Fatalf("operations contract: %v", err)
	}
	batch := RuntimeExpectedDocumentContract{
		SchemaVersion: RuntimeExpectedDocumentSchemaCurrent, DocumentKind: RuntimeProviderDocumentSemanticPlan, PlanStep: RuntimeSemanticPlanStepProofBatch,
		ManifestHash: "manifest-2", DefinitionSeedHash: "seed-1", LogicalRequestID: "request-2", StablePrefixHash: "prefix-1",
		BatchID: "batch-2", BatchSequence: 2, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2", "cwi-3"}, CoverageOrderHash: "order-1",
		CorrectionAttempt: 1, MaxOutputTokens: 4096, MaxRecoveryAttempts: 3,
	}
	batch.Hash = StableRuntimeExpectedDocumentHash(batch)
	if err := batch.Validate(); err != nil {
		t.Fatalf("semantic batch contract: %v", err)
	}
	wrongStep := batch
	wrongStep.PlanStep = RuntimeSemanticPlanStepCoverageOrder
	wrongStep.Hash = StableRuntimeExpectedDocumentHash(wrongStep)
	if err := wrongStep.Validate(); err == nil {
		t.Fatal("coverage-order contract accepted proof-batch identity")
	}
	missingBatch := batch
	missingBatch.BatchID = ""
	missingBatch.Hash = StableRuntimeExpectedDocumentHash(missingBatch)
	if err := missingBatch.Validate(); err == nil {
		t.Fatal("proof-batch contract accepted missing batch identity")
	}
}

func TestRuntimeExpectedDocumentHashIsOrderSensitiveAndCloneDetached(t *testing.T) {
	contract := RuntimeExpectedDocumentContract{
		SchemaVersion: RuntimeExpectedDocumentSchemaCurrent, DocumentKind: RuntimeProviderDocumentSemanticPlan, PlanStep: RuntimeSemanticPlanStepProofBatch,
		ManifestHash: "manifest-1", DefinitionSeedHash: "seed-1", LogicalRequestID: "request-1", StablePrefixHash: "prefix-1",
		BatchID: "batch-1", BatchSequence: 1, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"}, CoverageOrderHash: "order-1",
		MaxOutputTokens: 8192, MaxRecoveryAttempts: 3,
	}
	contract.Hash = StableRuntimeExpectedDocumentHash(contract)
	cloned := contract.Clone()
	cloned.BatchCoverageWorkItemIDs[0] = "mutated"
	if contract.BatchCoverageWorkItemIDs[0] != "cwi-1" {
		t.Fatal("expected-document clone leaked batch mutation")
	}
	reordered := contract
	reordered.BatchCoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-2", "cwi-1"}
	if StableRuntimeExpectedDocumentHash(reordered) == contract.Hash {
		t.Fatal("expected-document hash lost semantic batch order")
	}
}

func TestRuntimeProviderDocumentOutcomeIsFactualOnly(t *testing.T) {
	cases := []RuntimeProviderDocumentOutcome{
		{Disposition: RuntimeProviderDocumentComplete, ExpectedDocumentHash: "expected-1", ResponseHash: "response-1", ResponseBytes: 32},
		{Disposition: RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: "expected-1", ResponseHash: "response-2", ResponseBytes: 16, Code: "max_output_tokens"},
		{Disposition: RuntimeProviderDocumentInvalid, ExpectedDocumentHash: "expected-1", ResponseHash: "response-3", ResponseBytes: 8, Code: "multiple_json_values"},
		{Disposition: RuntimeProviderDocumentEmpty, ExpectedDocumentHash: "expected-1", Code: "final_output_missing"},
	}
	for _, outcome := range cases {
		if err := outcome.Validate(); err != nil {
			t.Fatalf("valid outcome %+v: %v", outcome, err)
		}
	}
	invalid := cases[0]
	invalid.ExpectedDocumentHash = ""
	if err := invalid.Validate(); err == nil {
		t.Fatal("provider outcome without expected-document identity accepted")
	}
}
