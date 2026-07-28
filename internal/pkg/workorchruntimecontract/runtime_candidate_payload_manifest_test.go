package workorchruntimecontract

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDecodeRuntimeCandidatePayloadWithManifestReportsTypedDynamicDiagnostics(t *testing.T) {
	patchManifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		Action: RuntimeActionProduceStateChange,
		RequiredOutput: RequiredOutputContract{
			Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		},
	})
	shellManifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		Action: RuntimeActionProduceValidation,
		RequiredOutput: RequiredOutputContract{
			Kind: RequiredOutputKindValidation, AcceptedKinds: []string{StepKindShellCommand},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectNone}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		},
	})
	gitManifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		Action: RuntimeActionProduceStateChange,
		RequiredOutput: RequiredOutputContract{
			Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{StepKindGitWrite},
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
			path:    "$", actual: "missing_operations", allowed: "operations", required: "operations",
		},
		{
			name: "top level dependency", manifest: patchManifest,
			payload: `{"operations":[{"id":"write","kind":"patch/apply","coverageWorkItemIds":["cwi-1"],"targetRefs":["main.go"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}],"dependsOn":["prepare"]}`,
			path:    "$.dependsOn", actual: "dependsOn", allowed: "operations",
		},
		{
			name: "operations must be array", manifest: patchManifest,
			payload: `{"operations":{}}`,
			path:    "$.operations", actual: "object", allowed: "array",
		},
		{
			name: "target refs must be array", manifest: patchManifest,
			payload: `{"operations":[{"id":"write","kind":"patch/apply","coverageWorkItemIds":["cwi-1"],"targetRefs":"main.go","parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]}`,
			path:    "$.operations[0].targetRefs", actual: "string", allowed: "array",
		},
		{
			name: "shell summary parameter", manifest: shellManifest,
			payload: `{"operations":[{"id":"check","kind":"shell/command","coverageWorkItemIds":["cwi-1"],"parameters":[{"name":"command","string":"true"},{"name":"summary","string":"done"}]}]}`,
			path:    "$.operations[0].parameters[1].name", actual: "summary", allowed: "command",
		},
		{
			name: "parameter alias", manifest: patchManifest,
			payload: `{"operations":[{"id":"write","kind":"patch/apply","coverageWorkItemIds":["cwi-1"],"targetRefs":["main.go"],"parameters":[{"name":"filePath","string":"main.go"},{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]}`,
			path:    "$.operations[0].parameters[0].name", actual: "filePath", allowed: "patchType",
		},
		{
			name: "missing target refs", manifest: patchManifest,
			payload: `{"operations":[{"id":"write","kind":"patch/apply","coverageWorkItemIds":["cwi-1"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]}`,
			path:    "$.operations[0].targetRefs", actual: "missing_targetRefs", allowed: "targetRefs", required: "targetRefs",
		},
		{
			name: "invalid required operation reference", manifest: gitManifest,
			payload: `{"operations":[{"id":"commit","kind":"git/write","requiredOperationRef":"unknown","dependsOn":["stage"],"coverageWorkItemIds":["cwi-1"],"parameters":[{"name":"operation","string":"commit"},{"name":"message","string":"commit"}]}]}`,
			path:    "$.operations[0].requiredOperationRef", actual: "unknown", allowed: "commit",
		},
		{
			name: "scalar list mismatch", manifest: shellManifest,
			payload: `{"operations":[{"id":"check","kind":"shell/command","coverageWorkItemIds":["cwi-1"],"parameters":[{"name":"command","string":"true"},{"name":"args","string":"-v"}]}]}`,
			path:    "$.operations[0].parameters[1]", actual: "string", allowed: "string_list",
		},
		{
			name: "sealed variant mismatch", manifest: gitManifest,
			payload: `{"operations":[{"id":"status","kind":"git/write","requiredOperationRef":"stage","coverageWorkItemIds":["cwi-1"],"parameters":[{"name":"operation","string":"status"}]}]}`,
			path:    "$.operations[0].parameters", actual: "unmatched_variant", allowed: "add",
		},
		{
			name: "trailing json", manifest: patchManifest,
			payload: `{"operations":[{"id":"write","kind":"patch/apply","coverageWorkItemIds":["cwi-1"],"targetRefs":["main.go"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]} {}`,
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

func FuzzDecodeRuntimeCandidatePayloadWithManifestRejectsUnknownFieldsWithoutPanicking(f *testing.F) {
	manifest := fuzzRuntimeCandidatePayloadManifest(f)
	for _, payload := range []string{
		`{"operations":[{"id":"write","kind":"patch/apply","coverageWorkItemIds":["cwi-1"],"targetRefs":["main.go"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]}`,
		`{"operations":{}}`,
		`{"operations":[{"id":"write","kind":"patch/apply","unknown":true}]}`,
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
		Action: RuntimeActionProduceStateChange,
		RequiredOutput: RequiredOutputContract{
			Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		},
	}
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

func TestDecodeRuntimeCandidatePayloadWithManifestAcceptsOperationLocalDependency(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		Action: RuntimeActionProduceStateChange,
		RequiredOutput: RequiredOutputContract{
			Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{StepKindGitWrite},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredOperationSequence: []RuntimeRequiredOperationVariant{
				{RequirementID: "stage", Kind: StepKindGitWrite, VariantID: "add"},
				{RequirementID: "commit", Kind: StepKindGitWrite, VariantID: "commit", DependsOnRequirementIDs: []string{"stage"}},
			},
		},
	})
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(`{"operations":[
		{"id":"stage","kind":"git/write","requiredOperationRef":"stage","coverageWorkItemIds":["cwi-1"],"parameters":[{"name":"operation","string":"add"},{"name":"pathspecs","stringList":["."]}]},
		{"id":"commit","kind":"git/write","requiredOperationRef":"commit","dependsOn":["stage"],"coverageWorkItemIds":["cwi-1"],"parameters":[{"name":"operation","string":"commit"},{"name":"message","string":"commit"}]}
	]}`))
	if err != nil || candidate.StateChange == nil || len(candidate.StateChange.Operations) != 2 {
		t.Fatalf("candidate=%+v err=%v", candidate, err)
	}
}

func TestEncodeRuntimeCandidatePayloadWithManifestRoundTripsAllCandidateShapes(t *testing.T) {
	content := "package main\n"
	command := "go test ./..."
	cases := []struct {
		name      string
		manifest  RuntimeCandidateContractManifest
		candidate RuntimeCandidate
	}{
		{
			name: "durable state change",
			manifest: mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
				Action: RuntimeActionProduceStateChange,
				RequiredOutput: RequiredOutputContract{
					Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{RuntimeBlockKindPatchApply},
					AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
				},
			}),
			candidate: RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
				ID: "write", Kind: RuntimeBlockKindPatchApply, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
				Parameters: []OperationParameter{{Name: "patchType", String: stringPtr("file_write")}, {Name: "content", String: &content}},
			}}}}},
		},
		{
			name: "validation",
			manifest: mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
				Action: RuntimeActionProduceValidation,
				RequiredOutput: RequiredOutputContract{
					Kind: RequiredOutputKindValidation, AcceptedKinds: []string{StepKindShellCommand},
					AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectNone}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone},
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
				},
			}),
			candidate: RuntimeCandidate{Validation: &ValidationCandidate{Operations: []ValidationOperation{{Operation: RuntimeOperation{
				ID: "test", Kind: StepKindShellCommand, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Parameters: []OperationParameter{{Name: "command", String: &command}},
			}}}}},
		},
		{
			name:     "assessment",
			manifest: mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{Action: RuntimeActionProduceAssessment, RequiredOutput: RequiredOutputContract{Kind: RequiredOutputKindAssessment, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}}),
			candidate: RuntimeCandidate{Assessment: &AssessmentCandidate{FocusID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Disposition: AssessmentDispositionRepairableGap,
				Items: []RuntimeAssessmentItemFact{{CoverageWorkItemID: "cwi-1", Satisfied: false, Reason: "missing validation"}}, Summary: "validation is required"}},
		},
		{
			name:      "governance",
			manifest:  mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{Action: RuntimeActionRequestGovernance, RequiredOutput: RequiredOutputContract{Kind: RequiredOutputKindGovernance}}),
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
			if got, want := StableRuntimeCandidateHash(decoded), StableRuntimeCandidateHash(testCase.candidate); got != want {
				t.Fatalf("candidate semantic hash = %s, want %s; payload=%s", got, want, payload)
			}
		})
	}
}

func TestEncodeRuntimeCandidatePayloadWithManifestRejectsCandidateOutsideManifest(t *testing.T) {
	manifest := mustRuntimeCandidatePayloadManifest(t, RuntimeDecision{
		Action:         RuntimeActionProduceStateChange,
		RequiredOutput: RequiredOutputContract{Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{StepKindFileWrite}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
	})
	if _, err := EncodeRuntimeCandidatePayloadWithManifest(manifest, RuntimeCandidate{Validation: &ValidationCandidate{}}); err == nil {
		t.Fatal("encoder accepted a candidate shape outside the sealed manifest")
	}
}

func stringPtr(value string) *string { return &value }

func mustRuntimeCandidatePayloadManifest(t *testing.T, decision RuntimeDecision) RuntimeCandidateContractManifest {
	t.Helper()
	manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}
