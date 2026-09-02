package workorchruntimecontract

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeCandidateWireContractCompilesRequiredVariantAuthorityLocally(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{StepKindGitWrite},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredOperationSequence: []RuntimeRequiredOperationVariant{
				{RequirementID: "stage", Kind: StepKindGitWrite, VariantID: "add"},
				{RequirementID: "commit", Kind: StepKindGitWrite, VariantID: "commit", DependsOnRequirementIDs: []string{"stage"}},
			},
		},
	})
	wire, err := BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := wire.Validate(manifest); err != nil {
		t.Fatalf("wire contract validation: %v", err)
	}
	stage, found := RuntimeCandidateWireRuleForRequiredOperation(wire, "stage")
	if !found {
		t.Fatal("stage wire rule is missing")
	}
	operation := "add"
	if stage.Kind != StepKindGitWrite || stage.VariantID != "add" ||
		!reflect.DeepEqual(stage.ProviderFields, []string{"requiredOperationRef", "parameters"}) ||
		!reflect.DeepEqual(stage.RequiredProviderFields, []string{"requiredOperationRef", "parameters"}) ||
		!reflect.DeepEqual(stage.RequiredProviderParameterNames, []string{"pathspecs"}) ||
		!reflect.DeepEqual(stage.RequiredNonEmptyParameterNames, []string{"pathspecs"}) ||
		containsManifestString(stage.ProviderParameterNames, "operation") ||
		containsManifestString(stage.ProviderParameterNames, "message") ||
		!reflect.DeepEqual(stage.DerivedBindingRefs, []string{"binding/test/cwi-1"}) ||
		!reflect.DeepEqual(stage.DerivedParameters, []OperationParameter{{Name: "operation", String: &operation}}) {
		t.Fatalf("stage wire rule = %+v", stage)
	}
	identity, err := RuntimeCandidateWireRuleIdentityForOperation(wire, stage, 0)
	if err != nil {
		t.Fatal(err)
	}
	outOfRange := *identity
	outOfRange.OperationIndex = wire.MaxOperationCount
	if err := outOfRange.ValidateAgainstContract(wire); err == nil {
		t.Fatal("wire rule identity accepted an operation index outside contract cardinality")
	}

	drifted := wire.Clone()
	drifted.OperationRules[0].ProviderFields = append(drifted.OperationRules[0].ProviderFields, "targetRefs")
	if drifted.Hash == StableRuntimeCandidateWireContractHash(drifted) {
		t.Fatal("wire field drift did not change the stable hash")
	}
	if err := drifted.Validate(manifest); err == nil {
		t.Fatal("wire field drift passed exact manifest validation")
	}
}

func TestRuntimeCandidateWireContractSupportsRequiredOperationWithoutVariant(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{StepKindFileWrite},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-file"}, TargetRefs: []string{"go.mod"},
			RequiredOperationSequence: []RuntimeRequiredOperationVariant{{RequirementID: "create-module", Kind: StepKindFileWrite, TargetRef: "go.mod"}},
		},
	})
	wire, err := BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	rule, found := RuntimeCandidateWireRuleForRequiredOperation(wire, "create-module")
	if !found || rule.VariantID != "" || rule.Kind != StepKindFileWrite ||
		!reflect.DeepEqual(rule.ProviderFields, []string{"requiredOperationRef", "parameters"}) ||
		!reflect.DeepEqual(rule.DerivedTargetRefs, []string{"go.mod"}) ||
		!reflect.DeepEqual(rule.DerivedBindingRefs, []string{"binding/test/cwi-file"}) ||
		!reflect.DeepEqual(rule.RequiredProviderParameterNames, []string{"content"}) {
		t.Fatalf("variant-free required wire rule = %+v found=%t", rule, found)
	}
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(`{"operations":[{"requiredOperationRef":"create-module","parameters":[{"name":"content","string":"module example"}]}]}`))
	if err != nil || candidate.StateChange == nil || len(candidate.StateChange.Operations) != 1 {
		t.Fatalf("variant-free provider wire candidate = %+v err=%v", candidate, err)
	}
	operation := candidate.StateChange.Operations[0].Operation
	if operation.Kind != StepKindFileWrite || !reflect.DeepEqual(operation.TargetRefs, []string{"go.mod"}) ||
		!reflect.DeepEqual(operation.BindingRefs, []string{"binding/test/cwi-file"}) {
		t.Fatalf("variant-free compiled operation = %+v", operation)
	}
	_, err = DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(`{"operations":[{"requiredOperationRef":"create-module","targetRefs":["go.mod"],"parameters":[{"name":"content","string":"module example"}]}]}`))
	var targetDiagnostic *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &targetDiagnostic) || targetDiagnostic.Path != "$.operations[0].targetRefs" || targetDiagnostic.Actual != "locally_derived_targetRefs" {
		t.Fatalf("fixed target echo diagnostic = %+v err=%v", targetDiagnostic, err)
	}
}

func TestRuntimeCandidateWireContractCompilesAuxiliarySingletonAuthorityLocally(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:             []string{RuntimeBlockKindPatchApply},
			AcceptedOperationVariants: []RuntimeOperationVariantRef{{Kind: RuntimeBlockKindPatchApply, VariantID: "text_replace"}},
			AcceptedEffectScopes:      []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes:      []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs:       []common.CoverageWorkItemID{"cwi-aux"},
			TargetRefs:                []string{"internal/lsp/server.go"},
		},
	})
	wire, err := BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	rule, found := RuntimeCandidateWireRuleForAuxiliaryKind(wire, RuntimeBlockKindPatchApply)
	patchType := "text_replace"
	if !found || rule.VariantID != patchType ||
		!reflect.DeepEqual(rule.ProviderFields, []string{"kind", "parameters"}) ||
		!reflect.DeepEqual(rule.RequiredProviderFields, []string{"kind", "parameters"}) ||
		!reflect.DeepEqual(rule.RequiredProviderParameterNames, []string{"newText", "oldText"}) ||
		!reflect.DeepEqual(rule.RequiredNonEmptyParameterNames, []string{"oldText"}) ||
		containsManifestString(rule.ProviderParameterNames, "patchType") ||
		containsManifestString(rule.ProviderParameterNames, "content") ||
		containsManifestString(rule.ProviderParameterNames, "diff") ||
		!reflect.DeepEqual(rule.DerivedBindingRefs, []string{"binding/test/cwi-aux"}) ||
		!reflect.DeepEqual(rule.DerivedTargetRefs, []string{"internal/lsp/server.go"}) ||
		!reflect.DeepEqual(rule.DerivedParameters, []OperationParameter{{Name: "patchType", String: &patchType}}) {
		t.Fatalf("auxiliary singleton wire rule = %+v found=%t", rule, found)
	}

	payload := []byte(`{"operations":[{"kind":"patch/apply","parameters":[{"name":"oldText","string":"before"},{"name":"newText","string":"after"}]}]}`)
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, payload)
	if err != nil || candidate.StateChange == nil || len(candidate.StateChange.Operations) != 1 {
		t.Fatalf("compiled auxiliary candidate = %+v err=%v", candidate, err)
	}
	operation := candidate.StateChange.Operations[0].Operation
	if !reflect.DeepEqual(operation.BindingRefs, rule.DerivedBindingRefs) ||
		!reflect.DeepEqual(operation.TargetRefs, rule.DerivedTargetRefs) ||
		!runtimeCandidateTestHasStringParameter(operation.Parameters, "patchType", patchType) {
		t.Fatalf("compiled auxiliary operation = %+v", operation)
	}
	for _, testCase := range []struct {
		name, payload, path, actual string
	}{
		{name: "binding echo", payload: `{"operations":[{"kind":"patch/apply","bindingRefs":["binding/test/cwi-aux"],"parameters":[{"name":"oldText","string":"before"},{"name":"newText","string":"after"}]}]}`, path: "$.operations[0].bindingRefs", actual: "locally_derived_bindingRefs"},
		{name: "target echo", payload: `{"operations":[{"kind":"patch/apply","targetRefs":["internal/lsp/server.go"],"parameters":[{"name":"oldText","string":"before"},{"name":"newText","string":"after"}]}]}`, path: "$.operations[0].targetRefs", actual: "locally_derived_targetRefs"},
		{name: "variant echo", payload: `{"operations":[{"kind":"patch/apply","parameters":[{"name":"patchType","string":"text_replace"},{"name":"oldText","string":"before"},{"name":"newText","string":"after"}]}]}`, path: "$.operations[0].parameters[0].name", actual: "locally_derived_parameter_patchType"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(testCase.payload))
			var diagnostic *RuntimeCandidatePayloadDiagnostic
			if !errors.As(err, &diagnostic) || diagnostic.Path != testCase.path || diagnostic.Actual != testCase.actual {
				t.Fatalf("fixed auxiliary echo diagnostic = %+v err=%v", diagnostic, err)
			}
		})
	}
}

func TestRuntimeCandidateWireContractRetainsNonSingletonAuxiliaryChoices(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-a", "cwi-b"},
			TargetRefs:           []string{"main.go", "support.go"},
		},
	})
	wire, err := BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		t.Fatal(err)
	}
	rule, found := RuntimeCandidateWireRuleForAuxiliaryKind(wire, RuntimeBlockKindPatchApply)
	if !found || rule.VariantID != "" || len(rule.DerivedBindingRefs) != 0 || len(rule.DerivedTargetRefs) != 0 || len(rule.DerivedParameters) != 0 ||
		!reflect.DeepEqual(rule.AllowedBindingRefs, []string{"binding/test/cwi-a", "binding/test/cwi-b"}) ||
		!reflect.DeepEqual(rule.ProviderFields, []string{"kind", "bindingRefs", "targetRefs", "parameters"}) ||
		!reflect.DeepEqual(rule.RequiredProviderFields, []string{"kind", "bindingRefs", "targetRefs", "parameters"}) ||
		!containsManifestString(rule.ProviderParameterNames, "patchType") {
		t.Fatalf("non-singleton auxiliary wire rule = %+v found=%t", rule, found)
	}
}

func runtimeCandidateTestHasStringParameter(values []OperationParameter, name, wanted string) bool {
	for _, value := range values {
		if value.Name == name && value.String != nil && *value.String == wanted {
			return true
		}
	}
	return false
}

func TestRuntimeCandidateProviderWireRejectsNullContainersAndFixedFieldEcho(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{StepKindGitWrite},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs:       []common.CoverageWorkItemID{"cwi-1"},
			RequiredOperationSequence: []RuntimeRequiredOperationVariant{{RequirementID: "init", Kind: StepKindGitWrite, VariantID: "init"}},
		},
	})
	tests := []struct {
		name     string
		payload  string
		path     string
		actual   string
		required string
		allowed  string
	}{
		{name: "null operations", payload: `{"operations":null}`, path: "$.operations", actual: "null", required: "operations", allowed: "array"},
		{name: "object operations", payload: `{"operations":{}}`, path: "$.operations", actual: "object", required: "operations", allowed: "array"},
		{name: "null parameters", payload: `{"operations":[{"requiredOperationRef":"init","parameters":null}]}`, path: "$.operations[0].parameters", actual: "null", required: "parameters", allowed: "array"},
		{name: "object parameters", payload: `{"operations":[{"requiredOperationRef":"init","parameters":{}}]}`, path: "$.operations[0].parameters", actual: "object", required: "parameters", allowed: "array"},
		{name: "fixed kind echo", payload: `{"operations":[{"requiredOperationRef":"init","kind":"git/write"}]}`, path: "$.operations[0].kind", actual: "locally_derived_kind"},
		{name: "fixed binding echo", payload: `{"operations":[{"requiredOperationRef":"init","bindingRefs":["binding/test/cwi-1"]}]}`, path: "$.operations[0].bindingRefs", actual: "locally_derived_bindingRefs"},
		{name: "fixed discriminator echo", payload: `{"operations":[{"requiredOperationRef":"init","parameters":[{"name":"operation","string":"init"}]}]}`, path: "$.operations[0].parameters[0].name", actual: "locally_derived_parameter_operation"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(test.payload))
			var diagnostic *RuntimeCandidatePayloadDiagnostic
			if !errors.As(err, &diagnostic) || diagnostic.Path != test.path || !strings.Contains(diagnostic.Actual, test.actual) ||
				(test.required != "" && !slices.Contains(diagnostic.Required, test.required)) ||
				(test.allowed != "" && !slices.Contains(diagnostic.Allowed, test.allowed)) {
				t.Fatalf("diagnostic = %+v err=%v, want path=%q actual~%q", diagnostic, err, test.path, test.actual)
			}
		})
	}
}
