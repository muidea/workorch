package workorchruntimecontract

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestBuildRuntimeCandidateContractManifestNarrowsCreationScopeWithoutSecondFieldRules(t *testing.T) {
	decision := RuntimeDecision{
		Action: RuntimeActionProduceStateChange,
		RequiredOutput: RequiredOutputContract{
			Kind:                  RequiredOutputKindStateChange,
			AcceptedKinds:         []string{StepKindShellCommand, RuntimeBlockKindPatchApply},
			AcceptedEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEvidenceKinds: []string{EvidenceKindStateChange},
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
			TargetRefs:            []string{"new-file.txt"},
		},
	}
	selection := &RuntimeTargetSelection{OwnerModuleID: RuntimeModuleFile, Disposition: RuntimeTargetSelectionCreate}
	manifest, err := BuildRuntimeCandidateContractManifest(decision, selection)
	if err != nil {
		t.Fatalf("BuildRuntimeCandidateContractManifest: %v", err)
	}
	if manifest.Revision != RuntimeCandidateContractManifestRevision || manifest.Hash == "" ||
		manifest.Hash != StableRuntimeCandidateContractManifestHash(manifest) || !manifest.RequireCoverageWorkItemIDs ||
		manifest.DependenciesAllowed || !reflect.DeepEqual(manifest.RequiredTopLevelFields, []string{"operations"}) ||
		len(manifest.Operations) != 1 || manifest.Operations[0].Kind != RuntimeBlockKindPatchApply {
		t.Fatalf("creation-scope manifest = %+v", manifest)
	}
	operation := manifest.Operations[0]
	if !operation.RequireTargetRefs || containsManifestString(operation.AllowedOperationFields, "dependsOn") ||
		!containsManifestString(operation.AllowedOperationFields, "targetRefs") ||
		!containsManifestString(operation.ForbiddenParameters, "targetRefs") {
		t.Fatalf("patch operation manifest did not preserve target/dependency contract: %+v", operation)
	}
}

func TestBuildRuntimeCandidateContractManifestProjectsDescriptorParametersAndVariants(t *testing.T) {
	decision := RuntimeDecision{
		Action: RuntimeActionProduceStateChange,
		RequiredOutput: RequiredOutputContract{
			Kind:                      RequiredOutputKindStateChange,
			AcceptedKinds:             []string{StepKindGitWrite},
			AcceptedOperationVariants: []RuntimeOperationVariantRef{{Kind: StepKindGitWrite, VariantID: "commit"}, {Kind: StepKindGitWrite, VariantID: "add"}},
			AcceptedEffectScopes:      []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes:      []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEvidenceKinds:     []string{EvidenceKindStateChange},
			CoverageWorkItemIDs:       []common.CoverageWorkItemID{"cwi-1"},
			RequiredOperationSequence: []RuntimeRequiredOperationVariant{
				{RequirementID: "stage", Kind: StepKindGitWrite, VariantID: "add"},
				{RequirementID: "commit", Kind: StepKindGitWrite, VariantID: "commit", DependsOnRequirementIDs: []string{"stage"}},
			},
		},
	}
	manifest, err := BuildRuntimeCandidateContractManifest(decision, nil)
	if err != nil {
		t.Fatalf("BuildRuntimeCandidateContractManifest: %v", err)
	}
	if len(manifest.Operations) != 1 || !manifest.DependenciesAllowed {
		t.Fatalf("git manifest = %+v", manifest)
	}
	operation := manifest.Operations[0]
	if !containsManifestString(operation.AllowedOperationFields, "dependsOn") ||
		!containsManifestString(operation.AllowedOperationFields, "requiredOperationRef") ||
		!reflect.DeepEqual(operation.AllowedVariantIDs, []string{"add", "commit"}) ||
		!reflect.DeepEqual(operation.RequiredOperationReferences, []string{"commit", "stage"}) {
		t.Fatalf("git operation manifest = %+v", operation)
	}
	if !containsManifestParameter(operation.Parameters, "operation", RuntimeOperationParameterTypeString) ||
		!containsManifestString(operation.ForbiddenParameters, "payload") {
		t.Fatalf("git descriptor parameters were not projected: %+v", operation)
	}

	permuted := decision
	permuted.RequiredOutput.AcceptedOperationVariants = []RuntimeOperationVariantRef{{Kind: StepKindGitWrite, VariantID: "add"}, {Kind: StepKindGitWrite, VariantID: "commit"}}
	permuted.RequiredOutput.RequiredOperationSequence = []RuntimeRequiredOperationVariant{
		{RequirementID: "commit", Kind: StepKindGitWrite, VariantID: "commit", DependsOnRequirementIDs: []string{"stage"}},
		{RequirementID: "stage", Kind: StepKindGitWrite, VariantID: "add"},
	}
	again, err := BuildRuntimeCandidateContractManifest(permuted, nil)
	if err != nil || again.Hash != manifest.Hash {
		t.Fatalf("manifest canonicalization drifted: first=%+v again=%+v err=%v", manifest, again, err)
	}
}

func TestBuildRuntimeCandidateContractManifestRejectsUnknownSealedVariant(t *testing.T) {
	decision := RuntimeDecision{
		Action: RuntimeActionProduceValidation,
		RequiredOutput: RequiredOutputContract{
			Kind:                      RequiredOutputKindValidation,
			AcceptedKinds:             []string{StepKindGitRead},
			AcceptedOperationVariants: []RuntimeOperationVariantRef{{Kind: StepKindGitRead, VariantID: "not-a-real-variant"}},
			AcceptedEffectScopes:      []RuntimeEffectScope{RuntimeEffectNone},
			RequiredEffectScopes:      []RuntimeEffectScope{RuntimeEffectNone},
			RequiredEvidenceKinds:     []string{EvidenceKindValidation},
			CoverageWorkItemIDs:       []common.CoverageWorkItemID{"cwi-1"},
		},
	}
	if _, err := BuildRuntimeCandidateContractManifest(decision, nil); err == nil {
		t.Fatal("unknown sealed operation variant was accepted")
	}
}

func TestValidateRuntimeCandidateAgainstManifestReportsTypedSemanticDiagnostics(t *testing.T) {
	decision := RuntimeDecision{
		Action: RuntimeActionProduceStateChange,
		RequiredOutput: RequiredOutputContract{
			Kind: RequiredOutputKindStateChange, AcceptedKinds: []string{RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		},
	}
	manifest, err := BuildRuntimeCandidateContractManifest(decision, nil)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(`{"operations":[{"id":"write","kind":"patch/apply","coverageWorkItemIds":["cwi-1"],"targetRefs":["main.go"],"parameters":[{"name":"targetRef","string":"main.go"},{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeCandidateAgainstManifest(manifest, candidate); err != nil {
		t.Fatalf("valid candidate = %v", err)
	}

	for _, testCase := range []struct {
		name     string
		mutate   func(*RuntimeOperation)
		path     string
		actual   string
		allowed  string
		required string
	}{
		{
			name: "kind", mutate: func(operation *RuntimeOperation) { operation.Kind = StepKindShellCommand },
			path: "$.operations[0].kind", actual: StepKindShellCommand, allowed: RuntimeBlockKindPatchApply,
		},
		{
			name: "coverage", mutate: func(operation *RuntimeOperation) {
				operation.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-other"}
			},
			path: "$.operations[0].coverageWorkItemIds", actual: "cwi-other", allowed: "cwi-1", required: "cwi-1",
		},
		{
			name: "target", mutate: func(operation *RuntimeOperation) { operation.TargetRefs = nil },
			path: "$.operations[0].targetRefs", actual: "missing_targetRefs", required: "targetRefs",
		},
		{
			name: "target parameter mismatch", mutate: func(operation *RuntimeOperation) { operation.TargetRefs = []string{"other.go"} },
			path: "$.operations[0].targetRefs", actual: "main.go", allowed: "other.go", required: "targetRef",
		},
		{
			name: "parameter", mutate: func(operation *RuntimeOperation) {
				value := "invalid"
				operation.Parameters = append(operation.Parameters, OperationParameter{Name: "notAllowed", String: &value})
			},
			path: "$.operations[0].parameters[name=notAllowed].name", actual: "notAllowed", allowed: "content",
		},
		{
			name: "dependency", mutate: func(operation *RuntimeOperation) { operation.DependsOn = []string{"missing-operation"} },
			path: "$.operations[0].dependsOn", actual: "missing-operation", allowed: "write",
		},
		{
			name: "variant", mutate: func(operation *RuntimeOperation) {
				for index := range operation.Parameters {
					if operation.Parameters[index].Name == "patchType" {
						value := "not-a-variant"
						operation.Parameters[index].String = &value
					}
				}
			},
			path: "$.operations[0].parameters", actual: "unmatched_variant", allowed: "file_write",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			mutated := candidate
			mutated.StateChange = &StateChangeCandidate{Operations: append([]StateChangingOperation(nil), candidate.StateChange.Operations...)}
			operation := mutated.StateChange.Operations[0].Operation
			testCase.mutate(&operation)
			mutated.StateChange.Operations[0].Operation = operation
			err := ValidateRuntimeCandidateAgainstManifest(manifest, mutated)
			var diagnostic *RuntimeCandidateContractDiagnostic
			if !errors.As(err, &diagnostic) || diagnostic.OperationID != "write" || diagnostic.Path != testCase.path ||
				(testCase.allowed != "" && !containsManifestString(diagnostic.Allowed, testCase.allowed)) || !strings.Contains(diagnostic.Actual, testCase.actual) ||
				(testCase.required != "" && !containsManifestString(diagnostic.Required, testCase.required)) {
				t.Fatalf("semantic diagnostic = %+v err=%v", diagnostic, err)
			}
		})
	}
}

func containsManifestString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsManifestParameter(values []RuntimeOperationParameterContract, want string, parameterType RuntimeOperationParameterType) bool {
	for _, value := range values {
		if value.Name == want && value.Type == parameterType {
			return true
		}
	}
	return false
}
