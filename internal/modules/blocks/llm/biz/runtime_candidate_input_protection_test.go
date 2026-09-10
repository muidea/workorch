package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func inputProtectionProviderCatalog() runtimecontract.RuntimeOperationAdmissibility {
	catalog := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "owner-inputs", OwnerModuleID: runtimecontract.RuntimeModuleMCP, OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, ObservedRevision: "inputs-1", ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
		ID: "check", OwnerModuleID: runtimecontract.RuntimeModuleMCP, OperationKind: runtimecontract.StepKindMCPToolCall,
		Readiness: runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "schema", InputBindingRef: "input",
		ProofApplicability: runtimecontract.RuntimeValidationProofAuxiliary, EffectScope: runtimecontract.RuntimeEffectNone, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
		InputTargetOwnerModuleID: runtimecontract.RuntimeModuleFile, InputTargetRefs: []string{"input.fixture"},
	}}}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	return catalog
}

func TestRuntimeCandidateInputExclusionsAgreeAcrossProjectionSchemaAndDecoder(t *testing.T) {
	decision := runtimeCandidateDecisionForSchemaTest(t, runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange, RequiredOutput: runtimecontract.RequiredOutputContract{
		AcceptedKinds: []string{runtimecontract.StepKindFileWrite}, AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"input.fixture", "implementation", "support"},
	}})
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{inputProtectionProviderCatalog()}})
	if err != nil {
		t.Fatal(err)
	}
	output := runtimeCandidateOutputContractForTest(t, manifest)
	if len(output.OperationContracts) != 1 || !reflect.DeepEqual(output.OperationContracts[0].Target.ExcludedTargetRefs, []string{"input.fixture"}) || !reflect.DeepEqual(output.OperationContracts[0].Target.AllowedTargetRefs, []string{"implementation", "support"}) {
		t.Fatalf("provider received different target authority: %+v", output.OperationContracts)
	}
	if !strings.Contains(output.StructuredOutputInstruction, "Excluded targets cannot be modified by this operation: input.fixture") {
		t.Fatal("provider instruction lost sealed exclusion")
	}
	schema := string(output.Schema)
	if !strings.Contains(schema, `"not":{"enum":["input.fixture"]}`) || !strings.Contains(schema, `"enum":["implementation","support"]`) {
		t.Fatalf("provider schema lost sealed exclusion: %s", schema)
	}
	for _, target := range []string{"input.fixture", "implementation"} {
		body, _ := json.Marshal(map[string]any{"operations": []any{map[string]any{"kind": "file/write", "targetRefs": []string{target}, "parameters": []any{map[string]string{"name": "content", "string": "new implementation"}}}}})
		_, err := runtimecontract.DecodeRuntimeCandidatePayloadWithManifest(manifest, body)
		if (err != nil) != (target == "input.fixture") {
			t.Fatalf("schema-independent decoding disagrees for %s: %v", target, err)
		}
	}
	output.OperationContracts[0].Target.ExcludedTargetRefs[0] = "different"
	if manifest.Operations[0].Target.ExcludedTargetRefs[0] != "input.fixture" {
		t.Fatal("provider projection aliases sealed input exclusions")
	}
}
