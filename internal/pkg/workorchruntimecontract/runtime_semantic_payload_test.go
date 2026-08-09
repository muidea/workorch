package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeSemanticPlanRequestSchemaNarrowsOrderAndProofBatch(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.CoverageItems = append(manifest.CoverageItems, RuntimeCoverageItem{ID: "cwi-002", SliceID: "slice-2", SourceScope: "dodItem", SourceID: "dod-002", TargetText: "second", Order: 2})
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	orderSchema, err := RuntimeSemanticPlanJSONSchemaForRequest(manifest, RuntimeSemanticPlanStepCoverageOrder, "", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(orderSchema, `"operationKind"`) || !strings.Contains(orderSchema, `"planStep":{"const":"coverage_order"}`) || !strings.Contains(orderSchema, `"maxItems":0`) {
		t.Fatalf("coverage-order schema was not narrowed: %s", orderSchema)
	}
	proofSchema, err := RuntimeSemanticPlanJSONSchemaForRequest(manifest, RuntimeSemanticPlanStepProofBatch, "batch-1", 1, []common.CoverageWorkItemID{"cwi-002"})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(proofSchema), &decoded); err != nil {
		t.Fatal(err)
	}
	properties := decoded["properties"].(map[string]any)
	if _, found := properties["coverageOrder"]; found {
		t.Fatalf("proof-batch schema repeated SM-3 coverage order: %s", proofSchema)
	}
	proposals := properties["proposals"].(map[string]any)
	proposalProperties := proposals["items"].(map[string]any)["properties"].(map[string]any)
	coverageEnum := proposalProperties["coverageWorkItemId"].(map[string]any)["enum"].([]any)
	if len(coverageEnum) != 1 || coverageEnum[0] != "cwi-002" || proposals["minItems"] != float64(1) || properties["batchId"].(map[string]any)["const"] != "batch-1" {
		t.Fatalf("proof-batch schema did not bind exact batch: %s", proofSchema)
	}
	for _, field := range []string{"requiredEvidenceKinds"} {
		constraint := proposalProperties[field].(map[string]any)
		if constraint["minItems"] != float64(1) || constraint["uniqueItems"] != true {
			t.Fatalf("%s schema permits an empty or duplicate required set: %v", field, constraint)
		}
	}
	if proposalProperties["declarationRef"].(map[string]any)["minLength"] != float64(1) {
		t.Fatalf("declarationRef schema permits an empty value: %v", proposalProperties["declarationRef"])
	}
	target := proposalProperties["target"].(map[string]any)
	if alternatives, ok := target["oneOf"].([]any); !ok || len(alternatives) != 3 {
		t.Fatalf("target schema does not expose the three canonical policies: %v", target)
	}
	prospectiveTarget := target["properties"].(map[string]any)["prospectiveTargetRef"].(map[string]any)
	if prospectiveTarget["minLength"] != float64(1) {
		t.Fatalf("prospective target schema must require a non-empty ref: %v", prospectiveTarget)
	}
	if _, enumBound := prospectiveTarget["enum"]; enumBound {
		t.Fatalf("prospective target schema must not require an existing catalog ref: %v", prospectiveTarget)
	}
	if strings.Contains(proofSchema, `"enum":null`) || strings.Contains(proofSchema, `requiredEffectScopes`) {
		t.Fatalf("semantic schema emitted an invalid null enum: %s", proofSchema)
	}
}

func TestRuntimeSemanticPlanRejectsDynamicOperationVariant(t *testing.T) {
	proposal := RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-001", OwnerModuleID: RuntimeModuleMCP, OperationKind: StepKindMCPToolCall,
		OperationVariantID: "mcp/tool_call/server/tool", Target: RuntimeOperationTargetAuthorization{
			Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{"server"},
		}, RequiredEvidenceKinds: []string{EvidenceKindValidation}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectEphemeralSession},
		ValidationRecipeID: "mcp/tool_call/server/tool", RequiredCheckRefs: []string{"configured-mcp-server"}, DeclarationRef: "dod/cwi-001",
	}
	var constraint *RuntimeSemanticProposalConstraintError
	if err := proposal.Validate(); !errors.As(err, &constraint) || constraint.Field != "operationVariantId" {
		t.Fatalf("dynamic MCP variant error = %v, constraint = %+v", err, constraint)
	}
}

func TestRuntimeSemanticPlanKeepsOwnerCatalogFactsScoped(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	mcpCatalog := semanticTestOwnerCatalog("semantic-mcp-catalog", RuntimeModuleMCP, []string{StepKindMCPToolCall})
	mcpCatalog.SelectorRefs = []string{"mcp-lsp-bridge"}
	mcpCatalog.ConstraintRefs = []string{"configured-mcp-server"}
	mcpCatalog.ValidationRecipeRefs = []string{"mcp/tool_call/mcp-lsp-bridge/lsp_status"}
	mcpCatalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(mcpCatalog)
	shellCatalog := semanticTestOwnerCatalog("semantic-shell-catalog", RuntimeModuleShell, []string{RuntimeBlockKindManagedValidationSession})
	shellCatalog.ValidationRecipeRefs = []string{"software-dev-validation-evidence"}
	shellCatalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(shellCatalog)
	manifest.AllowedOwnerModuleIDs = append(manifest.AllowedOwnerModuleIDs, RuntimeModuleMCP, RuntimeModuleShell)
	manifest.AllowedOperationKinds = append(manifest.AllowedOperationKinds, StepKindMCPToolCall, RuntimeBlockKindManagedValidationSession)
	manifest.AllowedTargetPolicies = append(manifest.AllowedTargetPolicies, RuntimeOperationTargetPolicyExistingOwnerScope)
	manifest.OwnerCatalogs = append(manifest.OwnerCatalogs, mcpCatalog, shellCatalog)
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	payload := `{"documentKind":"semantic_plan","schemaVersion":"workorch.runtime_semantic_plan_document","manifestHash":"` + manifest.Hash + `","definitionSeedHash":"` + manifest.DefinitionSeedHash + `","planStep":"proof_batch","batchId":"batch-1","batchSequence":0,"batchCoverageWorkItemIds":["cwi-001"],"proposals":[{"coverageWorkItemId":"cwi-001","ownerModuleId":"blocks/shell","operationKind":"validation/managed_session","target":{"policy":"existing_owner_scope","ownerSelectorRefs":["mcp-lsp-bridge"]},"requiredEvidenceKinds":["validation"],"validationRecipeId":"software-dev-validation-evidence","declarationRef":"dod/cwi-001"}]}`
	_, err := DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, []byte(payload))
	var diagnostic *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Path != "$.proposals[0].target.ownerSelectorRefs" || len(diagnostic.SemanticViolations) != 1 {
		t.Fatalf("owner-scoped selector diagnostic = %+v err=%v", diagnostic, err)
	}
	violation := diagnostic.SemanticViolations[0]
	if len(violation.Allowed) != 0 || violation.Rule != RuntimeSemanticConstraintAllowedValue {
		t.Fatalf("owner-scoped selector violation = %+v", violation)
	}
}

func TestRuntimeSemanticPlanDoesNotExposeMCPEffectToProvider(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	mcpCatalog := semanticTestOwnerCatalog("semantic-mcp-catalog", RuntimeModuleMCP, []string{StepKindMCPToolCall})
	mcpCatalog.SelectorRefs = []string{"mcp-lsp-bridge"}
	mcpCatalog.ValidationRecipeRefs = []string{RuntimeMCPToolValidationRecipeID("mcp-lsp-bridge", "lsp_status")}
	mcpCatalog.MCPCapabilities = []RuntimeMCPCapabilityContext{{
		ID: "mcp-lsp-bridge:tool:lsp_status", ServerID: "mcp-lsp-bridge", Kind: "tool", OperationKind: StepKindMCPToolCall,
		Name: "lsp_status", EffectScope: string(RuntimeEffectNone), IdempotencyClass: string(RuntimeOperationIdempotencySafeToReplay), Enabled: true,
	}}
	mcpCatalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(mcpCatalog)
	manifest.AllowedOwnerModuleIDs = append(manifest.AllowedOwnerModuleIDs, RuntimeModuleMCP)
	manifest.AllowedOperationKinds = append(manifest.AllowedOperationKinds, StepKindMCPToolCall)
	manifest.AllowedTargetPolicies = append(manifest.AllowedTargetPolicies, RuntimeOperationTargetPolicyExistingOwnerScope)
	manifest.OwnerCatalogs = append(manifest.OwnerCatalogs, mcpCatalog)
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	schema, err := RuntimeSemanticPlanJSONSchema(manifest)
	if err != nil || !strings.Contains(schema, `"mcp/tool_call/mcp-lsp-bridge/lsp_status"`) || strings.Contains(schema, `requiredEffectScopes`) {
		t.Fatalf("MCP effect leaked into provider semantic schema: err=%v schema=%s", err, schema)
	}
	oldPayload := `{"documentKind":"semantic_plan","schemaVersion":"workorch.runtime_semantic_plan_document","manifestHash":"` + manifest.Hash + `","definitionSeedHash":"` + manifest.DefinitionSeedHash + `","planStep":"proof_batch","batchId":"batch-1","batchSequence":0,"batchCoverageWorkItemIds":["cwi-001"],"proposals":[{"coverageWorkItemId":"cwi-001","ownerModuleId":"blocks/mcp","operationKind":"mcp/tool_call","target":{"policy":"existing_owner_scope","ownerSelectorRefs":["mcp-lsp-bridge"]},"requiredEvidenceKinds":["validation"],"requiredEffectScopes":["none"],"validationRecipeId":"mcp/tool_call/mcp-lsp-bridge/lsp_status","declarationRef":"dod/cwi-001"}]}`
	_, err = DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, []byte(oldPayload))
	var diagnostic *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Path != "$.proposals[0].requiredEffectScopes" || diagnostic.Actual != "unknown_field" {
		t.Fatalf("old provider effect field was not rejected: %+v err=%v", diagnostic, err)
	}
}

func TestRuntimeSemanticPlanRequestSchemaRejectsUnscopedProofBatch(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	if _, err := RuntimeSemanticPlanJSONSchemaForRequest(manifest, RuntimeSemanticPlanStepProofBatch, "", 0, nil); err == nil {
		t.Fatal("proof-batch schema accepted an unscoped request")
	}
}

func TestRuntimeSemanticPlanPayloadStrictRoundTrip(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	proposal := semanticContractTestProposal("cwi-001")
	proposal.Target.ProspectiveTargetRef = "new-file.go"
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch,
		BatchID: "batch-1", BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, Proposals: []RuntimeSemanticProofProposal{proposal},
	}
	payload, err := EncodeRuntimeSemanticPlanPayloadWithManifest(manifest, document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, payload)
	if err != nil {
		t.Fatal(err)
	}
	if StableRuntimeSemanticPlanDocumentHash(decoded) != StableRuntimeSemanticPlanDocumentHash(document) || !strings.Contains(string(payload), `"documentKind":"semantic_plan"`) {
		t.Fatalf("semantic round trip changed document: %s", payload)
	}
	if !strings.Contains(string(payload), `"batchSequence":0`) || strings.Contains(string(payload), `"coverageOrder"`) {
		t.Fatalf("proof-batch wire identity was not minimal and complete: %s", payload)
	}
}

func TestRuntimeSemanticPlanPayloadRejectsUnknownMissingAndWrongKind(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	base := `{"documentKind":"semantic_plan","schemaVersion":"workorch.runtime_semantic_plan_document","manifestHash":"` + manifest.Hash + `","definitionSeedHash":"` + manifest.DefinitionSeedHash + `","planStep":"proof_batch","batchId":"batch-1","batchSequence":0,"batchCoverageWorkItemIds":["cwi-001"],"proposals":[{"coverageWorkItemId":"cwi-001","ownerModuleId":"blocks/file","operationKind":"file/write","target":{"policy":"exact_target","exactTargetRefs":["main.go"]},"requiredEvidenceKinds":["state_change"],"declarationRef":"dod/cwi-001"}]}`
	tests := []struct {
		name    string
		payload string
		path    string
	}{
		{name: "unknown proposal", payload: strings.Replace(base, `"declarationRef":"dod/cwi-001"`, `"declarationRef":"dod/cwi-001","dependsOn":[]`, 1), path: "$.proposals[0].dependsOn"},
		{name: "repeated global order", payload: strings.Replace(base, `"proposals":`, `"coverageOrder":["cwi-001"],"proposals":`, 1), path: "$.coverageOrder"},
		{name: "missing target", payload: strings.Replace(base, `"target":{"policy":"exact_target","exactTargetRefs":["main.go"]},`, "", 1), path: "$.proposals[0]"},
		{name: "wrong kind", payload: strings.Replace(base, `"documentKind":"semantic_plan"`, `"documentKind":"operations"`, 1), path: "$.documentKind"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, []byte(test.payload))
			var diagnostic *RuntimeCandidatePayloadDiagnostic
			if !errors.As(err, &diagnostic) || diagnostic.Path != test.path {
				t.Fatalf("diagnostic = %+v err=%v", diagnostic, err)
			}
		})
	}
}

func TestRuntimeSemanticPlanPayloadPreservesTypedDependencyOrderViolation(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.CoverageItems[0].ReopenDependencies = []RuntimeCoverageReopenDependency{{CoverageWorkItemID: "cwi-000", TriggerEvidenceKinds: []string{EvidenceKindStateChange}}}
	manifest.CoverageItems = append(manifest.CoverageItems, RuntimeCoverageItem{ID: "cwi-000", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-000", TargetText: "prerequisite"})
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	payload := `{"documentKind":"semantic_plan","schemaVersion":"workorch.runtime_semantic_plan_document","manifestHash":"` + manifest.Hash + `","definitionSeedHash":"` + manifest.DefinitionSeedHash + `","planStep":"coverage_order","coverageOrder":["cwi-001","cwi-000"],"proposals":[]}`
	_, err := DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, []byte(payload))
	var diagnostic *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Path != "$.coverageOrder" || len(diagnostic.SemanticViolations) != 1 {
		t.Fatalf("diagnostic = %+v err=%v", diagnostic, err)
	}
	violation := diagnostic.SemanticViolations[0]
	if violation.RequiredBeforeID != "cwi-000" || violation.RequiredAfterID != "cwi-001" || violation.ActualBeforePosition != 1 || violation.ActualAfterPosition != 0 {
		t.Fatalf("typed dependency violation = %+v", violation)
	}
}

func TestRuntimeSemanticPlanPayloadRejectsDeletedEffectField(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	payload := `{"documentKind":"semantic_plan","schemaVersion":"workorch.runtime_semantic_plan_document","manifestHash":"` + manifest.Hash + `","definitionSeedHash":"` + manifest.DefinitionSeedHash + `","planStep":"proof_batch","batchId":"batch-1","batchSequence":0,"batchCoverageWorkItemIds":["cwi-001"],"proposals":[{"coverageWorkItemId":"cwi-001","ownerModuleId":"blocks/file","operationKind":"file/write","target":{"policy":"prospective_owner_target","ownerSelectorRefs":["file/workspace-relative-create"],"prospectiveConstraintRefs":["workspace-relative-path"],"prospectiveTargetRef":"main.go"},"requiredEvidenceKinds":["state_change"],"requiredEffectScopes":[],"declarationRef":"dod/cwi-001"}]}`
	_, err := DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, []byte(payload))
	var diagnostic *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Path != "$.proposals[0].requiredEffectScopes" || diagnostic.Actual != "unknown_field" {
		t.Fatalf("diagnostic = %+v err=%v", diagnostic, err)
	}
}

func TestRuntimeSemanticPlanPayloadPreservesTypedCatalogRecipeViolation(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.OwnerCatalogs[0].OwnerModuleID = RuntimeModuleShell
	manifest.OwnerCatalogs[0].OperationKinds = []string{RuntimeBlockKindManagedValidationSession}
	manifest.OwnerCatalogs[0].SelectorRefs = []string{"workspace-validation-target"}
	manifest.OwnerCatalogs[0].ConstraintRefs = nil
	manifest.OwnerCatalogs[0].ValidationRecipeRefs = []string{"recipe/allowed"}
	manifest.OwnerCatalogs[0].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(manifest.OwnerCatalogs[0])
	manifest.AllowedOwnerModuleIDs = []string{RuntimeModuleShell}
	manifest.AllowedOperationKinds = []string{RuntimeBlockKindManagedValidationSession}
	manifest.AllowedTargetPolicies = []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExistingOwnerScope}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	payload := `{"documentKind":"semantic_plan","schemaVersion":"workorch.runtime_semantic_plan_document","manifestHash":"` + manifest.Hash + `","definitionSeedHash":"` + manifest.DefinitionSeedHash + `","planStep":"proof_batch","batchId":"batch-1","batchSequence":0,"batchCoverageWorkItemIds":["cwi-001"],"proposals":[{"coverageWorkItemId":"cwi-001","ownerModuleId":"blocks/shell","operationKind":"validation/managed_session","target":{"policy":"existing_owner_scope","ownerSelectorRefs":["workspace-validation-target"]},"requiredEvidenceKinds":["validation"],"validationRecipeId":"recipe/not-allowed","declarationRef":"dod/cwi-001"}]}`
	_, err := DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, []byte(payload))
	var diagnostic *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Path != "$.proposals[0].validationRecipeId" || diagnostic.Actual != "recipe/not-allowed" || len(diagnostic.SemanticViolations) != 1 {
		t.Fatalf("diagnostic = %+v err=%v", diagnostic, err)
	}
	violation := diagnostic.SemanticViolations[0]
	if violation.Rule != RuntimeSemanticConstraintAllowedValue || violation.CoverageWorkItemID != "cwi-001" || len(violation.Allowed) != 1 || violation.Allowed[0] != "recipe/allowed" {
		t.Fatalf("typed catalog violation = %+v", violation)
	}
}

func TestRuntimeSemanticPlanPayloadEnrichesEmptyExactTargetWithCatalogRefs(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	manifest.AllowedTargetPolicies = []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExactTarget}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	payload := `{"documentKind":"semantic_plan","schemaVersion":"workorch.runtime_semantic_plan_document","manifestHash":"` + manifest.Hash + `","definitionSeedHash":"` + manifest.DefinitionSeedHash + `","planStep":"proof_batch","batchId":"batch-1","batchSequence":0,"batchCoverageWorkItemIds":["cwi-001"],"proposals":[{"coverageWorkItemId":"cwi-001","ownerModuleId":"blocks/file","operationKind":"file/write","target":{"policy":"exact_target","exactTargetRefs":[]},"requiredEvidenceKinds":["state_change"],"declarationRef":"dod/cwi-001"}]}`
	_, err := DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, []byte(payload))
	var diagnostic *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Path != "$.proposals[0].target.exactTargetRefs" || len(diagnostic.SemanticViolations) != 1 {
		t.Fatalf("diagnostic = %+v err=%v", diagnostic, err)
	}
	violation := diagnostic.SemanticViolations[0]
	if violation.Rule != RuntimeSemanticConstraintNonEmpty || len(violation.Allowed) != 1 || violation.Allowed[0] != "main.go" {
		t.Fatalf("target correction did not include sealed catalog refs: %+v", violation)
	}
}

func TestRuntimeSemanticPlanSchemaAndRequestContextAreManifestBound(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	schema, err := RuntimeSemanticPlanJSONSchema(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"semantic_plan"`, `"cwi-001"`, `"blocks/file"`, `"file/write"`, `"prospective_owner_target"`} {
		if !strings.Contains(schema, expected) {
			t.Fatalf("semantic schema does not contain %s: %s", expected, schema)
		}
	}
	target := ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"}
	seed := RuntimeDefinitionSeed{
		SchemaVersion: RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: manifest.DefinitionRevision, PolicyHash: "policy-1",
		CoverageItems: manifest.CoverageItems, Slices: []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}}},
	}
	seed.Hash = StableRuntimeDefinitionSeedHash(seed)
	manifest.DefinitionSeedHash = seed.Hash
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	context := RuntimeSemanticPlanRequestContext{
		WorkspaceID: manifest.WorkspaceID, RunID: manifest.RunID,
		Definition:      RuntimeDefinitionContext{Revision: manifest.DefinitionRevision, GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"file/write"}},
		DefinitionSeed:  seed,
		Activation:      RuntimeActivationContext{ProviderSelection: ProviderInferenceSelection{Target: target}, ExecutionContract: RuntimeExecutionContract{OperationContractRevision: manifest.OperationContractRevision}},
		ProviderCeiling: &ProviderCeiling{Selection: ProviderInferenceSelection{Target: target}, ContextWindowTokens: 8192, ProviderMaxOutputTokens: 2048, EffectiveMaxOutputTokens: 1024, MaxInputTokens: 4096, MaxBytes: 65536, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1"},
		PlanStep:        RuntimeSemanticPlanStepProofBatch, BatchID: "batch-1", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
	}
	if err := context.Validate(manifest); err != nil {
		t.Fatal(err)
	}
	implicitStep := context
	implicitStep.PlanStep = ""
	if err := implicitStep.Validate(manifest); err == nil || !strings.Contains(err.Error(), "invalid plan step") {
		t.Fatalf("empty semantic plan step error = %v", err)
	}
	stale := manifest
	stale.DefinitionSeedHash = "stale"
	stale.Hash = StableRuntimeSemanticPlanManifestHash(stale)
	if err := context.Validate(stale); err == nil {
		t.Fatal("semantic request context accepted stale seed")
	}
}
