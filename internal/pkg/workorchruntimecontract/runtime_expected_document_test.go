package workorchruntimecontract

import (
	"encoding/json"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeExpectedDocumentContractValidatesOperationsAndSemanticBatch(t *testing.T) {
	operations := RuntimeExpectedDocumentContract{OperationsContextEncoding: RuntimeOperationsContextEncoding,
		SchemaVersion: RuntimeExpectedDocumentSchemaCurrent, DocumentKind: RuntimeProviderDocumentOperations,
		ManifestHash: "manifest-1", WireContractHash: "wire-1", LogicalRequestID: "request-1", StablePrefixHash: "prefix-1",
		MaxOutputTokens: 8192, MaxRecoveryAttempts: 3,
	}
	operations.Hash = StableRuntimeExpectedDocumentHash(operations)
	if err := operations.Validate(); err != nil {
		t.Fatalf("operations contract: %v", err)
	}
	wireDrift := operations
	wireDrift.WireContractHash = "wire-2"
	if StableRuntimeExpectedDocumentHash(wireDrift) == operations.Hash {
		t.Fatal("operations expected-document hash lost wire-contract identity")
	}
	wireDrift.Hash = StableRuntimeExpectedDocumentHash(wireDrift)
	if err := wireDrift.Validate(); err != nil {
		t.Fatalf("operations contract with a separately sealed wire identity: %v", err)
	}
	missingWire := operations
	missingWire.WireContractHash = ""
	missingWire.Hash = StableRuntimeExpectedDocumentHash(missingWire)
	if err := missingWire.Validate(); err == nil {
		t.Fatal("operations contract accepted a missing wire-contract identity")
	}
	batch := RuntimeExpectedDocumentContract{SemanticContextEncoding: RuntimeSemanticContextEncoding,
		SchemaVersion: RuntimeExpectedDocumentSchemaCurrent, DocumentKind: RuntimeProviderDocumentSemanticPlan, PlanStep: RuntimeSemanticPlanStepProofBatch,
		ManifestHash: "manifest-2", DefinitionSeedHash: "seed-1", LogicalRequestID: "request-2", StablePrefixHash: "prefix-1",
		BatchID: "batch-2", BatchSequence: 2, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2", "cwi-3"}, CoverageOrderHash: "order-1",
		CorrectionAttempt: 1, MaxOutputTokens: 4096, MaxRecoveryAttempts: 3,
	}
	batch.Hash = StableRuntimeExpectedDocumentHash(batch)
	if err := batch.Validate(); err != nil {
		t.Fatalf("semantic batch contract: %v", err)
	}
	semanticWithWire := batch
	semanticWithWire.WireContractHash = "wire-1"
	semanticWithWire.Hash = StableRuntimeExpectedDocumentHash(semanticWithWire)
	if err := semanticWithWire.Validate(); err == nil {
		t.Fatal("semantic-plan contract accepted operations wire identity")
	}
	firstBatch := batch
	firstBatch.BatchSequence = 0
	firstBatch.Hash = StableRuntimeExpectedDocumentHash(firstBatch)
	if err := firstBatch.Validate(); err != nil {
		t.Fatalf("first semantic batch contract: %v", err)
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
	contract := RuntimeExpectedDocumentContract{SemanticContextEncoding: RuntimeSemanticContextEncoding,
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

func TestRuntimeProviderDocumentFramingIsOwnerNeutral(t *testing.T) {
	cases := []RuntimeProviderDocumentFraming{
		{Disposition: RuntimeProviderDocumentComplete, ExpectedDocumentHash: "expected-1", ResponseHash: "response-1", ResponseBytes: 32},
		{Disposition: RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: "expected-1", ResponseHash: "response-2", ResponseBytes: 16, Code: RuntimeProviderDocumentCodeOutputLimit, Signature: RuntimeProviderDocumentSignatureOutputLimit},
		{Disposition: RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: "expected-1", Code: RuntimeProviderDocumentCodeOutputLimit, Signature: RuntimeProviderDocumentSignatureOutputLimit},
		{Disposition: RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: "expected-1", ResponseHash: "response-3", ResponseBytes: 8, Code: RuntimeProviderDocumentCodeIncompleteJSON, Signature: RuntimeProviderDocumentSignatureIncompleteJSON},
		{Disposition: RuntimeProviderDocumentInvalid, ExpectedDocumentHash: "expected-1", ResponseHash: "response-4", ResponseBytes: 8, Code: RuntimeProviderDocumentCodeFinalOutputInvalid, Signature: RuntimeProviderDocumentSignatureOutputInvalid},
		{Disposition: RuntimeProviderDocumentEmpty, ExpectedDocumentHash: "expected-1", Code: RuntimeProviderDocumentCodeFinalOutputMissing, Signature: RuntimeProviderDocumentSignatureOutputMissing},
	}
	for _, outcome := range cases {
		if err := outcome.Validate(); err != nil {
			t.Fatalf("valid framing %+v: %v", outcome, err)
		}
	}
	invalid := cases[0]
	invalid.ExpectedDocumentHash = ""
	if err := invalid.Validate(); err == nil {
		t.Fatal("provider framing without expected-document identity accepted")
	}

	invalidCases := []RuntimeProviderDocumentFraming{
		{Disposition: RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: "expected-1", Code: "incomplete_json_document", Signature: "provider-document-incomplete"},
		{Disposition: RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: "expected-1", Code: "max_output_tokens"},
		{Disposition: RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: "expected-1", Code: "max_output_tokens", Signature: "wrong-signature"},
		{Disposition: RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: "expected-1", ResponseHash: "response-without-bytes", Code: "max_output_tokens", Signature: "provider-output-limit"},
		{Disposition: RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: "expected-1", ResponseBytes: 1, Code: "max_output_tokens", Signature: "provider-output-limit"},
		{Disposition: RuntimeProviderDocumentInvalid, ExpectedDocumentHash: "expected-1", ResponseHash: "response-3", ResponseBytes: 8, Code: "multiple_json_values", Signature: "multiple-json-values"},
		{Disposition: RuntimeProviderDocumentInvalid, ExpectedDocumentHash: "expected-1", ResponseHash: "response-3", ResponseBytes: 8, Code: RuntimeProviderDocumentCodeFinalOutputInvalid, Signature: "wrong-signature"},
		{Disposition: RuntimeProviderDocumentEmpty, ExpectedDocumentHash: "expected-1", ResponseHash: "unexpected", Code: RuntimeProviderDocumentCodeFinalOutputMissing, Signature: RuntimeProviderDocumentSignatureOutputMissing},
		{Disposition: RuntimeProviderDocumentEmpty, ExpectedDocumentHash: "expected-1", Code: "final_output_missing", Signature: "final-output-missing"},
	}
	for _, outcome := range invalidCases {
		if err := outcome.Validate(); err == nil {
			t.Fatalf("invalid framing was accepted: %+v", outcome)
		}
	}
}

func TestExpectedOperationsEncodingIsSealedAndSurvivesReload(t *testing.T) {
	current := RuntimeExpectedDocumentContract{OperationsContextEncoding: RuntimeOperationsContextEncoding, SchemaVersion: RuntimeExpectedDocumentSchemaCurrent, DocumentKind: RuntimeProviderDocumentOperations, ManifestHash: "manifest", WireContractHash: "wire", LogicalRequestID: "logical", StablePrefixHash: "prefix", MaxOutputTokens: 4096, MaxRecoveryAttempts: 2}
	current.Hash = StableRuntimeExpectedDocumentHash(current)
	if err := current.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(current)
	var restored RuntimeExpectedDocumentContract
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Validate() != nil || restored.Hash != current.Hash || restored.OperationsContextEncoding != RuntimeOperationsContextEncoding {
		t.Fatal("reload lost exact current encoding")
	}
	for _, encoding := range []string{"", "operations-render/v1", "unknown"} {
		bad := current.Clone()
		bad.OperationsContextEncoding = encoding
		bad.Hash = StableRuntimeExpectedDocumentHash(bad)
		if bad.Validate() == nil {
			t.Fatal("unsupported encoding accepted", encoding)
		}
	}
	semantic := current.Clone()
	semantic.DocumentKind = RuntimeProviderDocumentSemanticPlan
	semantic.Hash = StableRuntimeExpectedDocumentHash(semantic)
	if semantic.Validate() == nil {
		t.Fatal("operations encoding accepted on semantic request")
	}
}
