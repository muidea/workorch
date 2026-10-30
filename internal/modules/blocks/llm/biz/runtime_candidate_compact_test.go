package llm

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCompactOperationsKeepsRulesAndRejectsInvalidPayloads(t *testing.T) {
	for _, kind := range []string{rc.StepKindFileWrite, rc.StepKindFileEdit, rc.RuntimeBlockKindPatchApply} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			decision := runtimeCandidateDecisionForSchemaTest(t, rc.RuntimeDecision{State: rc.ConvergenceStateNeedStateChange, RequiredOutput: rc.RequiredOutputContract{AcceptedKinds: []string{kind}, TargetRefs: []string{"target.txt"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}}})
			manifest := runtimeCandidateManifestForSchema(t, decision, nil)
			wire := runtimeCandidateWireForTest(t, manifest)
			legacy := runtimeCandidateOutputContractForManifest(manifest, wire)
			compact := runtimeCandidateCompactOutputContract(manifest, wire)
			before, _ := json.Marshal(legacy)
			after, _ := json.Marshal(compact)
			if len(after) >= len(before) {
				t.Fatalf("dynamic bytes did not decrease: %d -> %d", len(before), len(after))
			}
			t.Logf("%s outputContract bytes: %d -> %d", kind, len(before), len(after))
			compact.StructuredOutputInstruction = legacy.StructuredOutputInstruction
			compact.RequiredOperationSequence = nil
			if !reflect.DeepEqual(compact, legacy) {
				t.Fatal("compact rendering removed a rule, schema, target, value constraint or example")
			}
			// Both modes retain the exact original local decoder. These are true
			// missing/type/duplicate/extra/target/variant failures, not prose checks.
			for _, body := range []string{`{}`, `{"operations":{}}`, `{"operations":[],"unknown":1}`, `{"operations":[],"operations":[]}`, `{"operations":[{"kind":"file/write","targetRefs":["unauthorized"],"parameters":[{"name":"content","string":"x"}]}]}`, `{"operations":[{"kind":"file/write","targetRefs":["target.txt"],"parameters":[{"name":"content","integer":1}]}]}`} {
				if _, err := rc.DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(body)); err == nil {
					t.Fatalf("invalid candidate accepted: %s", body)
				}
			}
		})
	}
}

func TestCompactContractKeepsExactRequiredSlotTargetCorrespondence(t *testing.T) {
	// The relation used to exist only in the long instruction. Neither the
	// union of target lists nor a wire hash can substitute for this exact map.
	manifest := runtimeCandidateManifestForSchema(t, rc.RuntimeDecision{State: rc.ConvergenceStateNeedStateChange, RequiredOutput: rc.RequiredOutputContract{
		AcceptedKinds: []string{rc.StepKindFileWrite}, TargetRefs: []string{"a", "b"}, RequiredOperationSequence: []rc.RuntimeRequiredOperationVariant{
			{RequirementID: "first", Kind: rc.StepKindFileWrite, TargetRef: "a"},
			{RequirementID: "second", Kind: rc.StepKindFileWrite, TargetRef: "b", DependsOnRequirementIDs: []string{"first"}},
		},
	}}, nil)
	output := runtimeCandidateCompactOutputContract(manifest, runtimeCandidateWireForTest(t, manifest))
	body := []byte(`{"operations":[{"requiredOperationRef":"first","parameters":[{"name":"content","string":"one"}]},{"requiredOperationRef":"second","parameters":[{"name":"content","string":"two"}]}]}`)
	if _, err := rc.DecodeRuntimeCandidatePayloadWithManifest(manifest, body); err != nil {
		t.Fatalf("valid minimal required-slot document rejected: %v", err)
	}
	if len(output.RequiredOperationSequence) != 2 || output.RequiredOperationSequence[0].TargetRef != "a" || output.RequiredOperationSequence[1].TargetRef != "b" || output.RequiredOperationSequence[1].DependsOnRequirementIDs[0] != "first" {
		t.Fatal("required slot-to-target/dependency mapping lost")
	}
	output.RequiredOperationSequence[1].DependsOnRequirementIDs[0] = "modified"
	if manifest.RequiredOperationSequence[1].DependsOnRequirementIDs[0] != "first" {
		t.Fatal("renderer mutated sealed manifest")
	}
}
