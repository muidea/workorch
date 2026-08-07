package workorchruntimecontract

import (
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeSemanticPlanPayloadStrictRoundTrip(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageOrder: []common.CoverageWorkItemID{"cwi-001"}, Proposals: []RuntimeSemanticProofProposal{semanticContractTestProposal("cwi-001")},
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
}

func TestRuntimeSemanticPlanPayloadRejectsUnknownMissingAndWrongKind(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	base := `{"documentKind":"semantic_plan","schemaVersion":"workorch.runtime_semantic_plan_document","manifestHash":"` + manifest.Hash + `","definitionSeedHash":"` + manifest.DefinitionSeedHash + `","planStep":"proof_batch","coverageOrder":["cwi-001"],"proposals":[{"coverageWorkItemId":"cwi-001","ownerModuleId":"blocks/file","operationKind":"file/write","target":{"policy":"exact_target","exactTargetRefs":["main.go"]},"requiredEvidenceKinds":["state_change"],"requiredEffectScopes":["workspace_durable"],"declarationRef":"dod/cwi-001"}]}`
	tests := []struct {
		name    string
		payload string
		path    string
	}{
		{name: "unknown proposal", payload: strings.Replace(base, `"declarationRef":"dod/cwi-001"`, `"declarationRef":"dod/cwi-001","dependsOn":[]`, 1), path: "$.proposals[0].dependsOn"},
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
		Activation:      RuntimeActivationContext{ProviderTarget: target, ExecutionContract: RuntimeExecutionContract{OperationContractRevision: manifest.OperationContractRevision}},
		ProviderCeiling: &ProviderCeiling{Target: target, ContextWindowTokens: 8192, ProviderMaxOutputTokens: 2048, EffectiveMaxOutputTokens: 1024, MaxInputTokens: 4096, MaxBytes: 65536, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1"},
	}
	if err := context.Validate(manifest); err != nil {
		t.Fatal(err)
	}
	stale := manifest
	stale.DefinitionSeedHash = "stale"
	stale.Hash = StableRuntimeSemanticPlanManifestHash(stale)
	if err := context.Validate(stale); err == nil {
		t.Fatal("semantic request context accepted stale seed")
	}
}
