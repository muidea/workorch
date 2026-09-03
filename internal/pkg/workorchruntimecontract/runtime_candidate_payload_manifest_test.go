package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDecodeRuntimeCandidatePayloadWithManifestReportsTypedDynamicDiagnostics(t *testing.T) {
	patchManifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go", "support.go"},
		},
	})
	shellManifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedValidation,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{RuntimeBlockKindManagedValidationSession},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectNone}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		},
	})
	gitManifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
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
	cases := []struct {
		name     string
		manifest RuntimeCandidateContractManifest
		payload  string
		path     string
		actual   string
		allowed  string
		required string
	}{
		{
			name: "missing operations", manifest: patchManifest,
			payload: `{}`,
			path:    "$.operations", actual: "missing_operations", allowed: "operations", required: "operations",
		},
		{
			name: "top level dependency", manifest: patchManifest,
			payload: `{"operations":[{"kind":"patch/apply","targetRefs":["main.go"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}],"dependsOn":["prepare"]}`,
			path:    "$.dependsOn", actual: "dependsOn", allowed: "operations",
		},
		{
			name: "operations must be array", manifest: patchManifest,
			payload: `{"operations":{}}`,
			path:    "$.operations", actual: "object", allowed: "array",
		},
		{
			name: "target refs must be array", manifest: patchManifest,
			payload: `{"operations":[{"kind":"patch/apply","targetRefs":"main.go","parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]}`,
			path:    "$.operations[0].targetRefs", actual: "string", allowed: "array",
		},
		{
			name: "managed validation summary parameter", manifest: shellManifest,
			payload: `{"operations":[{"kind":"validation/managed_session","parameters":[{"name":"recipeId","string":"recipe/candidate-test"},{"name":"summary","string":"done"}]}]}`,
			path:    "$.operations[0].parameters[1].name", actual: "summary", allowed: "recipeId",
		},
		{
			name: "parameter alias", manifest: patchManifest,
			payload: `{"operations":[{"kind":"patch/apply","targetRefs":["main.go"],"parameters":[{"name":"filePath","string":"main.go"},{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]}`,
			path:    "$.operations[0].parameters[0].name", actual: "filePath", allowed: "patchType",
		},
		{
			name: "missing target refs", manifest: patchManifest,
			payload: `{"operations":[{"kind":"patch/apply","parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]}`,
			path:    "$.operations[0].targetRefs", actual: "missing_targetRefs", allowed: "targetRefs", required: "targetRefs",
		},
		{
			name: "missing operation parameters field", manifest: patchManifest,
			payload: `{"operations":[{"kind":"patch/apply","targetRefs":["main.go"]}]}`,
			path:    "$.operations[0].parameters", actual: "missing_parameters", allowed: "parameters", required: "parameters",
		},
		{
			name: "missing required operation parameter", manifest: patchManifest,
			payload: `{"operations":[{"kind":"patch/apply","targetRefs":["main.go"],"parameters":[{"name":"patchType","string":"file_write"}]}]}`,
			path:    "$.operations[0].parameters[name=content]", actual: "missing_content", allowed: "file_write", required: "content",
		},
		{
			name: "invalid required operation reference", manifest: gitManifest,
			payload: `{"operations":[{"requiredOperationRef":"unknown","parameters":[{"name":"message","string":"commit"}]}]}`,
			path:    "$.operations[0].requiredOperationRef", actual: "unknown", allowed: "commit",
		},
		{
			name: "fixed variant discriminator echo", manifest: gitManifest,
			payload: `{"operations":[{"requiredOperationRef":"stage","parameters":[{"name":"operation","string":"status"}]}]}`,
			path:    "$.operations[0].parameters[0].name", actual: "locally_derived_parameter_operation", allowed: "pathspecs",
		},
		{
			name: "trailing json", manifest: patchManifest,
			payload: `{"operations":[{"kind":"patch/apply","targetRefs":["main.go"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]} {}`,
			path:    "$", actual: "candidate payload contains multiple JSON values",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := DecodeRuntimeCandidatePayloadWithManifest(testCase.manifest, []byte(testCase.payload))
			var diagnostic *RuntimeCandidatePayloadDiagnostic
			if !errors.As(err, &diagnostic) {
				t.Fatalf("error = %v, want RuntimeCandidatePayloadDiagnostic", err)
			}
			if diagnostic.Path != testCase.path || !strings.Contains(diagnostic.Actual, testCase.actual) ||
				(testCase.allowed != "" && !containsManifestString(diagnostic.Allowed, testCase.allowed)) ||
				(testCase.required != "" && !containsManifestString(diagnostic.Required, testCase.required)) {
				t.Fatalf("diagnostic = %+v, want path=%q actual~%q allowed=%q required=%q", diagnostic, testCase.path, testCase.actual, testCase.allowed, testCase.required)
			}
			_, repeatedErr := DecodeRuntimeCandidatePayloadWithManifest(testCase.manifest, []byte(testCase.payload))
			var repeated *RuntimeCandidatePayloadDiagnostic
			if !errors.As(repeatedErr, &repeated) || repeated.Path != diagnostic.Path || repeated.Actual != diagnostic.Actual ||
				!strings.EqualFold(strings.Join(repeated.Allowed, ","), strings.Join(diagnostic.Allowed, ",")) {
				t.Fatalf("diagnostic is not stable: first=%+v repeated=%+v", diagnostic, repeated)
			}
		})
	}
}

func TestDecodeRuntimeCandidatePayloadWithManifestPreservesExactMultiOperationWireRule(t *testing.T) {
	t.Parallel()
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

	for _, testCase := range []struct {
		name    string
		payload string
		path    string
		actual  string
	}{
		{
			name: "wrong parameters container",
			payload: `{"operations":[
				{"requiredOperationRef":"stage","parameters":[{"name":"pathspecs","stringList":["."]}]},
				{"requiredOperationRef":"commit","parameters":{"name":"message","string":"commit"}}
			]}`,
			path: "$.operations[1].parameters", actual: "object",
		},
		{
			name: "wrong parameter scalar type after wire compilation",
			payload: `{"operations":[
				{"requiredOperationRef":"stage","parameters":[{"name":"pathspecs","stringList":["."]}]},
				{"requiredOperationRef":"commit","parameters":[{"name":"message","integer":1}]}
			]}`,
			path: "$.operations[1].parameters[0]", actual: "integer",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, decodeErr := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(testCase.payload))
			var diagnostic *RuntimeCandidatePayloadDiagnostic
			if !errors.As(decodeErr, &diagnostic) || diagnostic.Path != testCase.path || diagnostic.Actual != testCase.actual || diagnostic.WireRule == nil {
				t.Fatalf("multi-operation diagnostic=%+v err=%v", diagnostic, decodeErr)
			}
			identity := diagnostic.WireRule.Normalized()
			if identity.WireContractHash != wire.Hash || identity.OperationIndex != 1 || identity.RequiredOperationRef != "commit" ||
				identity.OperationKind != StepKindGitWrite || identity.VariantID != "commit" || identity.ValidateAgainstManifest(manifest) != nil {
				t.Fatalf("selected wire rule identity=%+v", identity)
			}
			projected := RuntimeCandidateFailureDiagnosticFromPayloadDiagnostic(diagnostic)
			projected.ManifestHash = manifest.Hash
			if projected.WireRule == nil || projected.WireRule == diagnostic.WireRule ||
				ValidateRuntimeCandidateFailureDiagnosticWireRule(manifest, projected) != nil {
				t.Fatalf("provider-safe wire rule projection=%+v", projected)
			}
		})
	}
}

func TestDecodeRuntimeCandidatePayloadWithManifestClassifiesMisnestedDeclaredParameterEntries(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		},
	})
	malformed := []byte(`{"operations":[{"kind":"patch/apply","parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"secret complete file content","baseHash":"secret-owner-hash","coverageTargets":["cwi-1"],"summary":"secret change summary"}]}]}`)
	_, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, malformed)
	var diagnostic *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("malformed parameter entry error=%v, want typed diagnostic", err)
	}
	if diagnostic.ViolationKind != RuntimeCandidatePayloadViolationParameterEntryEncoding || len(diagnostic.ParameterEntryEncodings) != 1 || len(diagnostic.Forbidden) != 0 {
		t.Fatalf("parameter-entry diagnostic=%+v", diagnostic)
	}
	encoding := diagnostic.ParameterEntryEncodings[0]
	wantMisplaced := []RuntimeCandidateMisplacedParameter{
		{Name: "baseHash", Type: RuntimeOperationParameterTypeString},
		{Name: "coverageTargets", Type: RuntimeOperationParameterTypeStringList},
		{Name: "summary", Type: RuntimeOperationParameterTypeString},
	}
	if encoding.OperationIndex != 0 || encoding.ParameterIndex != 1 || encoding.OperationKind != RuntimeBlockKindPatchApply ||
		encoding.ContainerParameterName != "content" || !reflect.DeepEqual(encoding.MisplacedParameters, wantMisplaced) {
		t.Fatalf("parameter-entry encoding=%+v", encoding)
	}
	projected := RuntimeCandidateFailureDiagnosticFromPayloadDiagnostic(diagnostic)
	if projected == nil || projected.ViolationKind != diagnostic.ViolationKind ||
		!sameRuntimeCandidateParameterEntryEncodings(projected.ParameterEntryEncodings, diagnostic.ParameterEntryEncodings) {
		t.Fatalf("provider-safe projection=%+v", projected)
	}
	encodedProjection, marshalErr := json.Marshal(projected)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	for _, secret := range []string{"secret complete file content", "secret-owner-hash", "secret change summary"} {
		if strings.Contains(string(encodedProjection), secret) {
			t.Fatalf("provider-safe projection leaked raw value %q: %s", secret, encodedProjection)
		}
	}
	projected.ParameterEntryEncodings[0].MisplacedParameters[0].Name = "mutated"
	if diagnostic.ParameterEntryEncodings[0].MisplacedParameters[0].Name != "baseHash" {
		t.Fatal("diagnostic projection aliases nested encoding facts")
	}

	for name, payload := range map[string]string{
		"undeclared sibling":    `{"operations":[{"kind":"patch/apply","parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"content","baseHash":"hash","invented":"value"}]}]}`,
		"wrong sibling type":    `{"operations":[{"kind":"patch/apply","parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"content","baseHash":42}]}]}`,
		"already named sibling": `{"operations":[{"kind":"patch/apply","parameters":[{"name":"patchType","string":"file_write"},{"name":"baseHash","string":"hash"},{"name":"content","string":"content","baseHash":"other"}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(payload))
			var rejected *RuntimeCandidatePayloadDiagnostic
			if !errors.As(err, &rejected) || rejected.ViolationKind != "" || len(rejected.ParameterEntryEncodings) != 0 {
				t.Fatalf("ambiguous payload gained parameter-entry correction authority: diagnostic=%+v err=%v", rejected, err)
			}
		})
	}

	corrected := []byte(`{"operations":[{"kind":"patch/apply","parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"complete file content"},{"name":"baseHash","string":"owner-hash"},{"name":"coverageTargets","stringList":["cwi-1"]},{"name":"summary","string":"change summary"}]}]}`)
	if _, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, corrected); err != nil {
		t.Fatalf("independent named typed entries rejected: %v", err)
	}
}

func TestDecodeRuntimeCandidatePayloadWithManifestReturnsRejectedBindingFrontier(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"}, TargetRefs: []string{"main.go"},
		},
	})
	payload := []byte(`{"operations":[{"kind":"patch/apply","bindingRefs":["binding/wrong"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]}`)
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, payload)
	var diagnostic *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Path != "$.operations[0].bindingRefs" || diagnostic.Actual != "binding/wrong" || len(diagnostic.Allowed) != 2 {
		t.Fatalf("binding diagnostic=%+v err=%v", diagnostic, err)
	}
	operations, operationErr := runtimeCandidatePayloadOperations(candidate)
	if operationErr != nil || len(operations) != 1 || !reflect.DeepEqual(operations[0].BindingRefs, []string{"binding/wrong"}) {
		t.Fatalf("strictly decoded rejection frontier=%+v err=%v", candidate, operationErr)
	}
	projected := RuntimeCandidateFailureDiagnosticFromPayloadDiagnostic(diagnostic)
	if projected == nil || projected.Path != diagnostic.Path || !reflect.DeepEqual(projected.Allowed, diagnostic.Allowed) {
		t.Fatalf("provider-safe binding diagnostic projection=%+v", projected)
	}
}

func TestDecodeRuntimeCandidatePayloadWithManifestAllowsPresentEmptyStringWhenContractAllows(t *testing.T) {
	t.Parallel()
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		},
	})
	payload := []byte(`{"operations":[{"kind":"patch/apply","parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":""}]}]}`)
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, payload)
	if err != nil {
		t.Fatal(err)
	}
	operations, err := runtimeCandidatePayloadOperations(candidate)
	if err != nil || len(operations) != 1 || operations[0].ID == "" {
		t.Fatalf("empty-but-present content changed candidate authority: candidate=%+v err=%v", candidate, err)
	}
}

func FuzzDecodeRuntimeCandidatePayloadWithManifestRejectsUnknownFieldsWithoutPanicking(f *testing.F) {
	manifest := fuzzRuntimeCandidatePayloadManifest(f)
	for _, payload := range []string{
		`{"operations":[{"kind":"patch/apply","parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]}`,
		`{"operations":{}}`,
		`{"operations":[{"kind":"patch/apply","id":"write"}]}`,
		`{"operations":[`,
	} {
		f.Add(payload)
	}
	f.Fuzz(func(t *testing.T, payload string) {
		candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(payload))
		if err == nil {
			if candidate.StateChange == nil || candidate.Validation != nil || candidate.Assessment != nil || candidate.Governance != nil {
				t.Fatalf("accepted payload produced wrong candidate shape: %+v", candidate)
			}
			if runtimeCandidatePayloadTestHasUnknownField(manifest, []byte(payload)) {
				t.Fatalf("accepted payload with unknown field: %s", payload)
			}
			return
		}
		var diagnostic *RuntimeCandidatePayloadDiagnostic
		if !errors.As(err, &diagnostic) || diagnostic == nil {
			t.Fatalf("payload error is not typed diagnostic: %T %v", err, err)
		}
	})
}

func fuzzRuntimeCandidatePayloadManifest(f *testing.F) RuntimeCandidateContractManifest {
	f.Helper()
	decision := RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		},
	}
	decision = runtimeCandidateTestDecisionWithBindings(f, decision)
	manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		f.Fatal(err)
	}
	return manifest
}

func runtimeCandidatePayloadTestHasUnknownField(manifest RuntimeCandidateContractManifest, payload []byte) bool {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(payload, &envelope) != nil {
		return false
	}
	for field := range envelope {
		if !runtimeCandidatePayloadContains(manifest.AllowedTopLevelFields, field) {
			return true
		}
	}
	var operations []map[string]json.RawMessage
	if json.Unmarshal(envelope["operations"], &operations) != nil {
		return false
	}
	for _, operation := range operations {
		kind := runtimeCandidatePayloadRawString(operation["kind"])
		operationManifest, found := runtimeCandidatePayloadOperationManifest(manifest, kind)
		if !found {
			continue
		}
		for field := range operation {
			if !runtimeCandidatePayloadContains(operationManifest.AllowedOperationFields, field) {
				return true
			}
		}
		var parameters []map[string]json.RawMessage
		if json.Unmarshal(operation["parameters"], &parameters) != nil {
			continue
		}
		for _, parameter := range parameters {
			for field := range parameter {
				if field != "name" && field != "string" && field != "integer" && field != "boolean" && field != "stringList" {
					return true
				}
			}
		}
	}
	return false
}

func TestDecodeRuntimeCandidatePayloadWithManifestDerivesOperationAuthority(t *testing.T) {
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
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(`{"operations":[
		{"requiredOperationRef":"stage","parameters":[{"name":"pathspecs","stringList":["."]}]},
		{"requiredOperationRef":"commit","parameters":[{"name":"message","string":"commit"}]}
	]}`))
	if err != nil || candidate.StateChange == nil || len(candidate.StateChange.Operations) != 2 {
		t.Fatalf("candidate=%+v err=%v", candidate, err)
	}
	stage := candidate.StateChange.Operations[0].Operation
	commit := candidate.StateChange.Operations[1].Operation
	if stage.ID == "" || commit.ID == "" || stage.ID == commit.ID || !reflect.DeepEqual(commit.DependsOn, []string{stage.ID}) {
		t.Fatalf("derived operation authority = stage:%+v commit:%+v", stage, commit)
	}
}

func TestDecodeRuntimeCandidatePayloadWithManifestRejectsProviderOperationAuthority(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		},
	})
	for _, testCase := range []struct {
		name    string
		payload string
		path    string
	}{
		{name: "id", payload: `{"operations":[{"id":"provider-id","kind":"patch/apply","parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]}`, path: "$.operations[0].id"},
		{name: "dependsOn", payload: `{"operations":[{"kind":"patch/apply","dependsOn":["provider-id"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]}`, path: "$.operations[0].dependsOn"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(testCase.payload))
			var diagnostic *RuntimeCandidatePayloadDiagnostic
			if !errors.As(err, &diagnostic) || diagnostic.Path != testCase.path {
				t.Fatalf("provider authority diagnostic = %+v err=%v", diagnostic, err)
			}
		})
	}
}

func TestDecodeRuntimeCandidatePayloadWithManifestRejectsManifestExposingOwnerCoverageAuthority(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		},
	})
	manifest.Operations[0].AllowedOperationFields = append(manifest.Operations[0].AllowedOperationFields, "coverageWorkItemIds")
	manifest = manifest.Normalized()
	manifest.Hash = StableRuntimeCandidateContractManifestHash(manifest)

	_, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(`{"operations":[{"kind":"patch/apply","parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]}`))
	var diagnostic *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Path != "$.operations" || diagnostic.Actual != "invalid_operation_fields" || !strings.Contains(err.Error(), "coverageWorkItemIds") {
		t.Fatalf("owner coverage manifest field diagnostic = %+v err=%v", diagnostic, err)
	}
}

func TestRuntimeCandidateOperationAuthorityIsManifestDerivedAndRestartStable(t *testing.T) {
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
	stage := `{"requiredOperationRef":"stage","parameters":[{"name":"pathspecs","stringList":["."]}]}`
	commit := `{"requiredOperationRef":"commit","parameters":[{"name":"message","string":"commit"}]}`
	decode := func(document string) RuntimeCandidate {
		t.Helper()
		candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(document))
		if err != nil {
			t.Fatal(err)
		}
		return candidate
	}
	first := decode(`{"operations":[` + stage + `,` + commit + `]}`)
	restarted := decode(`{"operations":[` + stage + `,` + commit + `]}`)
	reordered := decode(`{"operations":[` + commit + `,` + stage + `]}`)
	byRef := func(candidate RuntimeCandidate) map[string]RuntimeOperation {
		ret := make(map[string]RuntimeOperation)
		for _, operation := range candidate.StateChange.Operations {
			ret[operation.Operation.RequiredOperationRef] = operation.Operation.Normalized()
		}
		return ret
	}
	firstByRef, restartedByRef, reorderedByRef := byRef(first), byRef(restarted), byRef(reordered)
	if firstByRef["stage"].ID == "" || firstByRef["commit"].ID == "" || firstByRef["stage"].ID == firstByRef["commit"].ID {
		t.Fatalf("required IDs were not derived uniquely: %+v", firstByRef)
	}
	if firstByRef["stage"].ID != restartedByRef["stage"].ID || firstByRef["commit"].ID != restartedByRef["commit"].ID ||
		firstByRef["stage"].ID != reorderedByRef["stage"].ID || firstByRef["commit"].ID != reorderedByRef["commit"].ID {
		t.Fatalf("required IDs drifted across restart/order: first=%+v restarted=%+v reordered=%+v", firstByRef, restartedByRef, reorderedByRef)
	}
	if len(firstByRef["stage"].DependsOn) != 0 || !reflect.DeepEqual(firstByRef["commit"].DependsOn, []string{firstByRef["stage"].ID}) ||
		!reflect.DeepEqual(reorderedByRef["commit"].DependsOn, []string{reorderedByRef["stage"].ID}) {
		t.Fatalf("required dependency graph was not derived exactly: first=%+v reordered=%+v", firstByRef, reorderedByRef)
	}

	tampered := cloneRuntimeCandidate(first)
	tampered.StateChange.Operations[0].Operation.ID = "attempt-local-id"
	tampered.StateChange.Operations[1].Operation.DependsOn = nil
	derived, err := DeriveRuntimeCandidateOperationAuthority(manifest, tampered)
	if err != nil || StableRuntimeCandidateHash(derived) != StableRuntimeCandidateHash(first) {
		t.Fatalf("attempt-local authority affected derivation: derived=%+v err=%v", derived, err)
	}
	if err := ValidateRuntimeCandidateAgainstManifest(manifest, tampered); err == nil {
		t.Fatal("semantic admission accepted tampered operation authority")
	}

	document, err := EncodeRuntimeCandidatePayloadWithManifest(manifest, first)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(document), `"id"`) || strings.Contains(string(document), `"dependsOn"`) {
		t.Fatalf("encoder leaked owner-derived authority: %s", document)
	}
	roundTrip := decode(string(document))
	if StableRuntimeCandidateHash(roundTrip) != StableRuntimeCandidateHash(first) {
		t.Fatalf("encoder roundtrip authority drifted: first=%+v roundTrip=%+v", first, roundTrip)
	}
}

func TestRuntimeCandidateAuxiliaryOperationIDsUseCanonicalSemanticKey(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{StepKindFileWrite, RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go", "support.go"},
		},
	})
	fileOperation := func(content string) string {
		return `{"kind":"file/write","targetRefs":["main.go"],"parameters":[{"name":"content","string":"` + content + `"}]}`
	}
	patchOperation := func(content string) string {
		return `{"kind":"patch/apply","targetRefs":["support.go"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"` + content + `"}]}`
	}
	decodeOperations := func(left, right string) map[string]RuntimeOperation {
		t.Helper()
		candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(`{"operations":[`+left+`,`+right+`]}`))
		if err != nil {
			t.Fatal(err)
		}
		ret := make(map[string]RuntimeOperation, 2)
		for _, value := range candidate.StateChange.Operations {
			operation := value.Operation.Normalized()
			ret[StableRuntimeOperationShape(operation)] = operation
		}
		return ret
	}
	first := decodeOperations(fileOperation("first"), patchOperation("second"))
	restarted := decodeOperations(fileOperation("first"), patchOperation("second"))
	reordered := decodeOperations(patchOperation("second"), fileOperation("first"))
	if len(first) != 2 || len(restarted) != 2 || len(reordered) != 2 {
		t.Fatalf("auxiliary semantic slots = first:%+v restarted:%+v reordered:%+v", first, restarted, reordered)
	}
	for semanticKey, operation := range first {
		if operation.ID == "" || restarted[semanticKey].ID != operation.ID || reordered[semanticKey].ID != operation.ID {
			t.Fatalf("auxiliary semantic ID drifted for %s: first=%+v restarted=%+v reordered=%+v", semanticKey, operation, restarted[semanticKey], reordered[semanticKey])
		}
	}
	changed := decodeOperations(fileOperation("changed"), patchOperation("second"))
	fileKey := StableRuntimeOperationShape(firstOperationForKind(first, StepKindFileWrite))
	changedFile := firstOperationForKind(changed, StepKindFileWrite)
	if changedFile.ID == "" || changedFile.ID == first[fileKey].ID {
		t.Fatalf("execution-semantic content change retained auxiliary ID: first=%+v changed=%+v", first[fileKey], changedFile)
	}
}

func TestRuntimeCandidateAuxiliaryOperationSemanticSlotCollisionFailsClosed(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		},
	})
	base := `{"kind":"patch/apply","parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}`
	summaryOnlyDifference := `{"kind":"patch/apply","parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"},{"name":"summary","string":"display only"}]}`
	for _, testCase := range []struct {
		name   string
		second string
	}{
		{name: "exact duplicate", second: base},
		{name: "canonical semantic collision", second: summaryOnlyDifference},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(`{"operations":[`+base+`,`+testCase.second+`]}`))
			var diagnostic *RuntimeCandidatePayloadDiagnostic
			if !errors.As(err, &diagnostic) || diagnostic.Path != "$.operations[1]" || diagnostic.Actual != "duplicate_auxiliary_semantic_slot" {
				t.Fatalf("semantic collision diagnostic = %+v err=%v", diagnostic, err)
			}
		})
	}
}

func TestRuntimeCandidateOperationCardinalityUsesManifestSourceOfTruth(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go", "support.go"},
		},
	})
	if manifest.MaxOperationCount != RuntimeCandidateOperationMaxCount {
		t.Fatalf("compiled maxOperationCount = %d, want %d", manifest.MaxOperationCount, RuntimeCandidateOperationMaxCount)
	}
	limited := manifest.Clone()
	limited.MaxOperationCount = 1
	limited.Hash = StableRuntimeCandidateContractManifestHash(limited)
	if limited.Hash == manifest.Hash {
		t.Fatal("maxOperationCount did not participate in manifest identity")
	}
	first := `{"kind":"patch/apply","targetRefs":["main.go"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"first"}]}`
	second := `{"kind":"patch/apply","targetRefs":["support.go"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"second"}]}`
	payload := []byte(`{"operations":[` + first + `,` + second + `]}`)
	_, err := DecodeRuntimeCandidatePayloadWithManifest(limited, payload)
	var payloadDiagnostic *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &payloadDiagnostic) || payloadDiagnostic.Path != "$.operations" || payloadDiagnostic.Actual != "operation_count_2" {
		t.Fatalf("strict cardinality diagnostic = %+v err=%v", payloadDiagnostic, err)
	}

	allowed := limited.Clone()
	allowed.MaxOperationCount = 2
	allowed.Hash = StableRuntimeCandidateContractManifestHash(allowed)
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(allowed, payload)
	if err != nil {
		t.Fatalf("same-kind multi-operation candidate rejected below sealed max: %v", err)
	}
	if err := ValidateRuntimeCandidateAgainstManifest(limited, candidate); err == nil || !strings.Contains(err.Error(), "operation_count_2") {
		t.Fatalf("local evaluator did not enforce sealed maxOperationCount: %v", err)
	}
}

func firstOperationForKind(values map[string]RuntimeOperation, kind string) RuntimeOperation {
	for _, operation := range values {
		if operation.Kind == kind {
			return operation
		}
	}
	return RuntimeOperation{}
}

func TestEncodeRuntimeCandidatePayloadWithManifestRoundTripsAllCandidateShapes(t *testing.T) {
	content := "package main\n"
	cases := []struct {
		name      string
		manifest  RuntimeCandidateContractManifest
		candidate RuntimeCandidate
	}{
		{
			name: "durable state change",
			manifest: mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
				State: ConvergenceStateNeedStateChange,
				RequiredOutput: RequiredOutputContract{
					AcceptedKinds:        []string{RuntimeBlockKindPatchApply},
					AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
				},
			}),
			candidate: RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
				ID: "write", Kind: RuntimeBlockKindPatchApply, BindingRefs: []string{"binding/test/cwi-1"}, TargetRefs: []string{"main.go"},
				Parameters: []OperationParameter{{Name: "patchType", String: stringPtr("file_write")}, {Name: "content", String: &content}},
			}}}}},
		},
		{
			name: "validation",
			manifest: mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
				State: ConvergenceStateNeedValidation,
				RequiredOutput: RequiredOutputContract{
					AcceptedKinds:        []string{RuntimeBlockKindManagedValidationSession},
					AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectNone}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone},
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
				},
			}),
			candidate: RuntimeCandidate{Validation: &ValidationCandidate{Operations: []ValidationOperation{{Operation: RuntimeOperation{
				ID: "test", Kind: RuntimeBlockKindManagedValidationSession, BindingRefs: []string{"binding/test/cwi-1"}, Parameters: []OperationParameter{{Name: "recipeId", String: stringPtr("recipe/candidate-test")}},
			}}}}},
		},
		{
			name:     "assessment",
			manifest: mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{State: ConvergenceStateNeedAssessment, RequiredOutput: RequiredOutputContract{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}}),
			candidate: RuntimeCandidate{Assessment: &AssessmentCandidate{FocusID: "focus-candidate-test", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Disposition: AssessmentDispositionRepairableGap,
				Items: []RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-1", Satisfied: false, Reason: "missing validation"}}, Summary: "validation is required"}},
		},
		{
			name:      "governance",
			manifest:  mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{State: ConvergenceStateNeedGovernance, RequiredOutput: RequiredOutputContract{}}),
			candidate: RuntimeCandidate{Governance: &GovernanceCandidate{ExternalDependency: ExternalDependency{Kind: "permission", SubjectRef: "remote-system", Reason: "grant is required"}}},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			payload, err := EncodeRuntimeCandidatePayloadWithManifest(testCase.manifest, testCase.candidate)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := DecodeRuntimeCandidatePayloadWithManifest(testCase.manifest, payload)
			if err != nil {
				t.Fatal(err)
			}
			wantCandidate, err := DeriveRuntimeCandidateOperationAuthority(testCase.manifest, testCase.candidate)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := StableRuntimeCandidateHash(decoded), StableRuntimeCandidateHash(wantCandidate); got != want {
				t.Fatalf("candidate semantic hash = %s, want %s; payload=%s", got, want, payload)
			}
		})
	}
}

func TestEncodeRuntimeCandidatePayloadWithManifestRejectsCandidateOutsideManifest(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{StepKindFileWrite},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
		},
	})
	if _, err := EncodeRuntimeCandidatePayloadWithManifest(manifest, RuntimeCandidate{Validation: &ValidationCandidate{}}); err == nil {
		t.Fatal("encoder accepted a candidate shape outside the sealed manifest")
	}
}

func stringPtr(value string) *string { return &value }

func mustRuntimeCandidatePayloadManifest(t *testing.T, decision RuntimeDecision) RuntimeCandidateContractManifest {
	t.Helper()
	decision = runtimeCandidateTestDecisionWithBindings(t, decision)
	ownerFacts := RuntimeCandidateManifestOwnerFacts{ValidationRecipes: []RuntimeValidationRecipe{{
		ID: "recipe/candidate-test", Command: "sh", Args: []string{"-c", "true"},
	}}}
	if decision.State == ConvergenceStateNeedAssessment {
		ownerFacts.AssessmentPurpose = RuntimeAssessmentPurposeDefinitionProof
		ownerFacts.AssessmentPurposeManifestHash = "semantic-manifest/candidate-payload"
	}
	manifest, err := BuildRuntimeCandidateContractManifest(decision, ownerFacts)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}
