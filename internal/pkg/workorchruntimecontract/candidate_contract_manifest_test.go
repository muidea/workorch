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
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:         []string{StepKindShellCommand, RuntimeBlockKindPatchApply},
			AcceptedEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEvidenceKinds: []string{EvidenceKindStateChange},
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
			TargetRefs:            []string{"new-file.txt"},
		},
	}
	selection := &RuntimeTargetSelection{OwnerModuleID: RuntimeModuleFile, Disposition: RuntimeTargetSelectionCreate}
	manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{TargetSelection: selection})
	if err != nil {
		t.Fatalf("BuildRuntimeCandidateContractManifest: %v", err)
	}
	if manifest.Revision != RuntimeCandidateContractManifestRevision || manifest.Hash == "" ||
		manifest.Hash != StableRuntimeCandidateContractManifestHash(manifest) || !manifest.RequireCoverageWorkItemIDs ||
		manifest.DependenciesAllowed || !reflect.DeepEqual(manifest.RequiredTopLevelFields, []string{"operations"}) ||
		len(manifest.Operations) != 1 || manifest.Operations[0].Kind != RuntimeBlockKindPatchApply {
		t.Fatalf("creation-scope manifest = %+v", manifest)
	}
	if manifest.EffectPolicy.State != ConvergenceStateNeedStateChange ||
		!manifest.EffectPolicy.AllowsOperationEffect(RuntimeEffectWorkspaceDurable) ||
		manifest.EffectPolicy.AllowsOperationEffect(RuntimeEffectNone) {
		t.Fatalf("manifest effect policy = %+v", manifest.EffectPolicy)
	}
	operation := manifest.Operations[0]
	if operation.OwnerModuleID != RuntimeModuleFile ||
		operation.OperationContractRevision == "" ||
		operation.OperationContractRevision != manifest.OperationContractRevision ||
		!operation.RequireTargetRefs || containsManifestString(operation.AllowedOperationFields, "dependsOn") ||
		!containsManifestString(operation.AllowedOperationFields, "targetRefs") ||
		!containsManifestString(operation.ForbiddenParameters, "targetRefs") {
		t.Fatalf("patch operation manifest did not preserve target/dependency contract: %+v", operation)
	}
	if operation.Target.Policy != RuntimeOperationTargetPolicyProspectiveOwnerTarget ||
		operation.Target.ProspectiveTargetRef != "new-file.txt" ||
		len(operation.Target.ExactTargetRefs) != 0 {
		t.Fatalf("create selection must seal prospective_owner_target: %+v", operation.Target)
	}
}

func TestBuildRuntimeCandidateContractManifestScopeAllowsOnlyDescriptorDeclaredGroundedKind(t *testing.T) {
	decision := RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:         []string{StepKindGitWrite, StepKindShellCommand, RuntimeBlockKindPatchApply},
			AcceptedEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes:  []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEvidenceKinds: []string{EvidenceKindStateChange},
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
		},
	}
	selection := &RuntimeTargetSelection{
		OwnerModuleID: RuntimeModuleFile, Disposition: RuntimeTargetSelectionScope,
		CandidateTargetRefs: []string{"go.sum", "go.mod"},
	}
	manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{TargetSelection: selection})
	if err != nil {
		t.Fatalf("BuildRuntimeCandidateContractManifest: %v", err)
	}
	kinds := make([]string, 0, len(manifest.Operations))
	for _, operation := range manifest.Operations {
		kinds = append(kinds, operation.Kind)
	}
	if want := []string{RuntimeBlockKindPatchApply, StepKindShellCommand}; !reflect.DeepEqual(kinds, want) {
		t.Fatalf("scope manifest kinds = %v, want %v", kinds, want)
	}
	for _, operation := range manifest.Operations {
		if operation.Target.Policy != RuntimeOperationTargetPolicyExistingOwnerScope ||
			!reflect.DeepEqual(operation.Target.OwnerScopeTargetRefs, []string{"go.mod", "go.sum"}) {
			t.Fatalf("scope selection must seal existing_owner_scope for %s: %+v", operation.Kind, operation.Target)
		}
	}
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(`{"operations":[{"id":"tidy","kind":"shell/command","coverageWorkItemIds":["cwi-1"],"targetRefs":["go.sum"],"parameters":[{"name":"command","string":"go"},{"name":"args","stringList":["mod","tidy"]},{"name":"stateChanging","boolean":true}]}]}`))
	if err != nil {
		t.Fatalf("decode File-grounded shell candidate: %v", err)
	}
	if err := ValidateRuntimeCandidateAgainstManifest(manifest, candidate); err != nil {
		t.Fatalf("validate File-grounded shell candidate: %v", err)
	}
}

func TestBuildRuntimeCandidateContractManifestProjectsDescriptorParametersAndVariants(t *testing.T) {
	decision := RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
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
	manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		t.Fatalf("BuildRuntimeCandidateContractManifest: %v", err)
	}
	if len(manifest.Operations) != 1 || !manifest.DependenciesAllowed {
		t.Fatalf("git manifest = %+v", manifest)
	}
	operation := manifest.Operations[0]
	if operation.OwnerModuleID != RuntimeModuleGit ||
		operation.Target.Policy != RuntimeOperationTargetPolicyExactTarget ||
		!containsManifestString(operation.AllowedOperationFields, "dependsOn") ||
		!containsManifestString(operation.AllowedOperationFields, "requiredOperationRef") ||
		!reflect.DeepEqual(operation.AllowedVariantIDs, []string{"add", "commit"}) ||
		!reflect.DeepEqual(operation.RequiredOperationReferences, []string{"commit", "stage"}) ||
		!reflect.DeepEqual(operation.AcceptedEffectScopes, []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}) ||
		!reflect.DeepEqual(operation.RequiredEffectScopes, []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}) {
		t.Fatalf("git operation manifest = %+v", operation)
	}
	if !containsManifestParameter(operation.Parameters, "operation", RuntimeOperationParameterTypeString) ||
		!containsManifestString(operation.ForbiddenParameters, "payload") {
		t.Fatalf("git descriptor parameters were not projected: %+v", operation)
	}
	cloned := manifest.Clone()
	if cloned.Hash != manifest.Hash || StableRuntimeCandidateContractManifestHash(cloned) != manifest.Hash {
		t.Fatalf("clone/hash drifted: original=%+v clone=%+v", manifest, cloned)
	}

	permuted := decision
	permuted.RequiredOutput.AcceptedOperationVariants = []RuntimeOperationVariantRef{{Kind: StepKindGitWrite, VariantID: "add"}, {Kind: StepKindGitWrite, VariantID: "commit"}}
	permuted.RequiredOutput.RequiredOperationSequence = []RuntimeRequiredOperationVariant{
		{RequirementID: "commit", Kind: StepKindGitWrite, VariantID: "commit", DependsOnRequirementIDs: []string{"stage"}},
		{RequirementID: "stage", Kind: StepKindGitWrite, VariantID: "add"},
	}
	again, err := BuildRuntimeCandidateContractManifest(permuted, RuntimeCandidateManifestOwnerFacts{})
	if err != nil || again.Hash != manifest.Hash {
		t.Fatalf("manifest canonicalization drifted: first=%+v again=%+v err=%v", manifest, again, err)
	}
}

func TestBuildRuntimeCandidateContractManifestSealsThreeTargetPolicies(t *testing.T) {
	t.Parallel()
	base := RequiredOutputContract{
		AcceptedKinds:        []string{RuntimeBlockKindPatchApply},
		AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
		RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
	}
	t.Run("exact_target from required output", func(t *testing.T) {
		decision := RuntimeDecision{State: ConvergenceStateNeedStateChange, RequiredOutput: base}
		decision.RequiredOutput.TargetRefs = []string{"main.go"}
		manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{})
		if err != nil {
			t.Fatalf("BuildRuntimeCandidateContractManifest: %v", err)
		}
		operation := manifest.Operations[0]
		if operation.Target.Policy != RuntimeOperationTargetPolicyExactTarget ||
			!reflect.DeepEqual(operation.Target.ExactTargetRefs, []string{"main.go"}) ||
			!operation.RequireTargetRefs ||
			!reflect.DeepEqual(operation.Target.AllowedTargetRefs(), []string{"main.go"}) {
			t.Fatalf("exact target authorization = %+v", operation)
		}
	})
	t.Run("existing_owner_scope from complete selection", func(t *testing.T) {
		decision := RuntimeDecision{State: ConvergenceStateNeedStateChange, RequiredOutput: base}
		selection := &RuntimeTargetSelection{
			PrerequisiteID: "selection-1", OwnerModuleID: RuntimeModuleFile, TargetRole: "deliverable",
			SelectorRef: "file/workspace_deliverable", Disposition: RuntimeTargetSelectionScope,
			TargetRefs: []string{"a.go", "b.go"}, CandidateTargetRefs: []string{"a.go", "b.go"},
			CandidateCount: 2, ObservedRevision: "rev-1",
		}
		selection.SnapshotID = StableRuntimeTargetSelectionSnapshotID(*selection)
		manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{TargetSelection: selection})
		if err != nil {
			t.Fatalf("BuildRuntimeCandidateContractManifest: %v", err)
		}
		operation := manifest.Operations[0]
		if operation.Target.Policy != RuntimeOperationTargetPolicyExistingOwnerScope ||
			!reflect.DeepEqual(operation.Target.OwnerScopeTargetRefs, []string{"a.go", "b.go"}) ||
			!reflect.DeepEqual(operation.Target.OwnerSelectorRefs, []string{"file/workspace_deliverable"}) ||
			!reflect.DeepEqual(operation.PrerequisiteSnapshotRefs, []string{selection.SnapshotID}) ||
			!operation.RequireTargetRefs {
			t.Fatalf("existing owner scope authorization = %+v", operation)
		}
	})
	t.Run("prospective_owner_target from create selection", func(t *testing.T) {
		decision := RuntimeDecision{State: ConvergenceStateNeedStateChange, RequiredOutput: base}
		selection := &RuntimeTargetSelection{
			PrerequisiteID: "selection-create", OwnerModuleID: RuntimeModuleFile, TargetRole: "deliverable",
			SelectorRef: "file/workspace_deliverable", Disposition: RuntimeTargetSelectionCreate,
			ProspectiveTargetRef: "created.go", ObservedRevision: "empty-v1",
		}
		selection.SnapshotID = StableRuntimeTargetSelectionSnapshotID(*selection)
		manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{TargetSelection: selection})
		if err != nil {
			t.Fatalf("BuildRuntimeCandidateContractManifest: %v", err)
		}
		operation := manifest.Operations[0]
		if operation.Target.Policy != RuntimeOperationTargetPolicyProspectiveOwnerTarget ||
			operation.Target.ProspectiveTargetRef != "created.go" ||
			!reflect.DeepEqual(operation.Target.OwnerSelectorRefs, []string{"file/workspace_deliverable"}) ||
			!reflect.DeepEqual(operation.PrerequisiteSnapshotRefs, []string{selection.SnapshotID}) ||
			!operation.RequireTargetRefs ||
			!reflect.DeepEqual(operation.Target.AllowedTargetRefs(), []string{"created.go"}) {
			t.Fatalf("prospective owner target authorization = %+v", operation)
		}
	})
	t.Run("one authorization per kind and hash covers target policy", func(t *testing.T) {
		decision := RuntimeDecision{
			State: ConvergenceStateNeedStateChange,
			RequiredOutput: RequiredOutputContract{
				AcceptedKinds:        []string{RuntimeBlockKindPatchApply, StepKindFileEdit},
				AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
				RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
				CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
			},
		}
		manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{})
		if err != nil {
			t.Fatalf("BuildRuntimeCandidateContractManifest: %v", err)
		}
		if len(manifest.Operations) != 2 {
			t.Fatalf("expected one authorization per kind, got %d", len(manifest.Operations))
		}
		seen := map[string]struct{}{}
		for _, operation := range manifest.Operations {
			if _, duplicate := seen[operation.Kind]; duplicate {
				t.Fatalf("duplicate authorization for kind %q", operation.Kind)
			}
			seen[operation.Kind] = struct{}{}
			if operation.OwnerModuleID == "" || operation.Target.Policy == "" || operation.OperationContractRevision == "" {
				t.Fatalf("authorization incomplete: %+v", operation)
			}
		}
		mutated := manifest.Clone()
		mutated.Operations[0].Target.ExactTargetRefs = []string{"other.go"}
		if StableRuntimeCandidateContractManifestHash(mutated) == manifest.Hash {
			t.Fatal("target policy fields must participate in manifest hash")
		}
		mutated = manifest.Clone()
		mutated.Operations[0].OwnerModuleID = "blocks/other"
		if StableRuntimeCandidateContractManifestHash(mutated) == manifest.Hash {
			t.Fatal("owner module must participate in manifest hash")
		}
	})
	t.Run("incomplete scope is typed pre-provider failure", func(t *testing.T) {
		decision := RuntimeDecision{State: ConvergenceStateNeedStateChange, RequiredOutput: base}
		selection := &RuntimeTargetSelection{
			OwnerModuleID: RuntimeModuleFile, Disposition: RuntimeTargetSelectionScope,
			TargetRefs: []string{"a.go"}, CandidateCount: 2,
		}
		if _, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{TargetSelection: selection}); err == nil {
			t.Fatal("incomplete owner scope must not seal as authorization")
		}
	})
}

func TestBuildRuntimeCandidateContractManifestRejectsUnknownSealedVariant(t *testing.T) {
	decision := RuntimeDecision{
		State: ConvergenceStateNeedValidation,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:             []string{StepKindGitRead},
			AcceptedOperationVariants: []RuntimeOperationVariantRef{{Kind: StepKindGitRead, VariantID: "not-a-real-variant"}},
			AcceptedEffectScopes:      []RuntimeEffectScope{RuntimeEffectNone},
			RequiredEffectScopes:      []RuntimeEffectScope{RuntimeEffectNone},
			RequiredEvidenceKinds:     []string{EvidenceKindValidation},
			CoverageWorkItemIDs:       []common.CoverageWorkItemID{"cwi-1"},
		},
	}
	if _, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{}); err == nil {
		t.Fatal("unknown sealed operation variant was accepted")
	}
}

func TestValidateRuntimeCandidateAgainstManifestReportsTypedSemanticDiagnostics(t *testing.T) {
	decision := RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		},
	}
	manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(`{"operations":[{"id":"write","kind":"patch/apply","coverageWorkItemIds":["cwi-1"],"targetRefs":["main.go"],"parameters":[{"name":"patchType","string":"file_write"},{"name":"content","string":"package main"}]}]}`))
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
			path: "$.operations[0].targetRefs", actual: "missing_targetRefs", required: "one",
		},
		{
			// Sealed exact_target membership is the first-failure for an outside
			// TargetRefs set. Parameter/targetRefs consistency is only checked
			// after membership succeeds.
			name: "target outside sealed policy", mutate: func(operation *RuntimeOperation) { operation.TargetRefs = []string{"other.go"} },
			path: "$.operations[0].targetRefs", actual: "other.go", allowed: "main.go",
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

func TestBuildRuntimeCandidateContractManifestCompilesOperationAdmissibility(t *testing.T) {
	t.Parallel()
	base := RequiredOutputContract{
		AcceptedKinds:        []string{StepKindFileEdit},
		AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
		RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
	}
	t.Run("admissibility inventory seals existing_owner_scope", func(t *testing.T) {
		decision := RuntimeDecision{State: ConvergenceStateNeedStateChange, RequiredOutput: base}
		inventory := RuntimeOperationAdmissibility{
			PrerequisiteID: "operation_admissibility:file", OwnerModuleID: RuntimeModuleFile,
			OperationKinds: []string{StepKindFileEdit}, TargetRefs: []string{"a.go", "b.go"},
			SelectorRefs: []string{"file/workspace_deliverable"}, ObservedRevision: "tree-v1",
		}
		inventory.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(inventory)
		manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{
			OperationAdmissibilities: []RuntimeOperationAdmissibility{inventory},
		})
		if err != nil {
			t.Fatalf("BuildRuntimeCandidateContractManifest: %v", err)
		}
		operation := manifest.Operations[0]
		if operation.Target.Policy != RuntimeOperationTargetPolicyExistingOwnerScope ||
			!reflect.DeepEqual(operation.Target.OwnerScopeTargetRefs, []string{"a.go", "b.go"}) ||
			!reflect.DeepEqual(operation.Target.OwnerSelectorRefs, []string{"file/workspace_deliverable"}) ||
			!reflect.DeepEqual(operation.AdmissibilitySnapshotRefs, []string{inventory.SnapshotID}) ||
			!operation.RequireTargetRefs {
			t.Fatalf("admissibility-compiled authorization = %+v", operation)
		}
		if err := ValidateRuntimeCandidateContractManifestForDecision(manifest, decision, RuntimeCandidateManifestOwnerFacts{
			OperationAdmissibilities: []RuntimeOperationAdmissibility{inventory},
		}); err != nil {
			t.Fatalf("validate with same owner facts: %v", err)
		}
		if err := ValidateRuntimeCandidateContractManifestForDecision(manifest, decision, RuntimeCandidateManifestOwnerFacts{}); err == nil {
			t.Fatal("missing admissibility must not match sealed authorization")
		}
	})
	t.Run("selector-only admissibility seals prospective create path", func(t *testing.T) {
		decision := RuntimeDecision{
			State: ConvergenceStateNeedStateChange,
			RequiredOutput: RequiredOutputContract{
				AcceptedKinds:        []string{StepKindFileWrite},
				AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
				RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
				CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
			},
		}
		inventory := RuntimeOperationAdmissibility{
			PrerequisiteID: "operation_admissibility:file-create", OwnerModuleID: RuntimeModuleFile,
			OperationKinds: []string{StepKindFileWrite}, SelectorRefs: []string{"file/workspace_deliverable"},
			ConstraintRefs: []string{"create:empty_workspace"}, ObservedRevision: "empty-v1",
		}
		inventory.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(inventory)
		manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{
			OperationAdmissibilities: []RuntimeOperationAdmissibility{inventory},
		})
		if err != nil {
			t.Fatalf("BuildRuntimeCandidateContractManifest: %v", err)
		}
		operation := manifest.Operations[0]
		if operation.Target.Policy != RuntimeOperationTargetPolicyProspectiveOwnerTarget ||
			!reflect.DeepEqual(operation.Target.OwnerSelectorRefs, []string{"file/workspace_deliverable"}) ||
			!reflect.DeepEqual(operation.Target.ProspectiveConstraintRefs, []string{"create:empty_workspace"}) ||
			!reflect.DeepEqual(operation.AdmissibilitySnapshotRefs, []string{inventory.SnapshotID}) {
			t.Fatalf("selector-only prospective authorization = %+v", operation)
		}
	})
	t.Run("mcp selector-only inventory does not force targetRefs", func(t *testing.T) {
		// MCP catalogs bind server selectors, not workspace membership targets.
		// Selector-only inventory must remain existing_owner_scope without a
		// wire-level targetRefs obligation so tool_call candidates stay legal.
		decision := RuntimeDecision{
			State: ConvergenceStateNeedValidation,
			RequiredOutput: RequiredOutputContract{
				AcceptedKinds:        []string{StepKindMCPToolCall},
				AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession},
				RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone},
				CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
			},
		}
		inventory := RuntimeOperationAdmissibility{
			PrerequisiteID: "operation_admissibility:mcp", OwnerModuleID: RuntimeModuleMCP,
			OperationKinds: []string{StepKindMCPToolCall}, SelectorRefs: []string{"server-a"},
			ConstraintRefs: []string{"configured-mcp-server"}, ObservedRevision: "catalog-v1",
		}
		inventory.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(inventory)
		manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{
			OperationAdmissibilities: []RuntimeOperationAdmissibility{inventory},
		})
		if err != nil {
			t.Fatalf("BuildRuntimeCandidateContractManifest: %v", err)
		}
		operation := manifest.Operations[0]
		if operation.Target.Policy != RuntimeOperationTargetPolicyExistingOwnerScope ||
			len(operation.Target.OwnerScopeTargetRefs) != 0 ||
			!reflect.DeepEqual(operation.Target.OwnerSelectorRefs, []string{"server-a"}) ||
			operation.RequireTargetRefs {
			t.Fatalf("mcp selector-only authorization = %+v", operation)
		}
		if _, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(`{"operations":[{"id":"mcp-stdio-call","kind":"mcp/tool_call","coverageWorkItemIds":["cwi-1"],"parameters":[{"name":"serverId","string":"server-a"},{"name":"tool","string":"repo.summary"}]}]}`)); err != nil {
			t.Fatalf("mcp tool_call without targetRefs must decode under selector-only inventory: %v", err)
		}
	})
	t.Run("oversized admissibility is typed pre-provider failure", func(t *testing.T) {
		decision := RuntimeDecision{State: ConvergenceStateNeedStateChange, RequiredOutput: base}
		refs := make([]string, 0, RuntimeOwnerInventoryItemsMaxCount+1)
		for i := 0; i < RuntimeOwnerInventoryItemsMaxCount+1; i++ {
			refs = append(refs, "file-"+string(rune('a'+i%26))+string(rune('0'+i/26)))
		}
		inventory := RuntimeOperationAdmissibility{
			PrerequisiteID: "operation_admissibility:file-oversize", OwnerModuleID: RuntimeModuleFile,
			OperationKinds: []string{StepKindFileEdit}, TargetRefs: refs, ObservedRevision: "tree-oversize",
		}
		inventory.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(inventory)
		if _, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{
			OperationAdmissibilities: []RuntimeOperationAdmissibility{inventory},
		}); err == nil {
			t.Fatal("oversized owner inventory must not seal as authorization")
		}
	})
	t.Run("owner facts from resources keep build/validate hash stable", func(t *testing.T) {
		decision := RuntimeDecision{State: ConvergenceStateNeedStateChange, RequiredOutput: base}
		inventory := RuntimeOperationAdmissibility{
			PrerequisiteID: "operation_admissibility:file", OwnerModuleID: RuntimeModuleFile,
			OperationKinds: []string{StepKindFileEdit}, TargetRefs: []string{"main.go"}, ObservedRevision: "tree-v2",
		}
		inventory.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(inventory)
		resources := RuntimeResourceFacts{OperationAdmissibilities: []RuntimeOperationAdmissibility{inventory}}
		ownerFacts := RuntimeCandidateManifestOwnerFactsFromResources(resources)
		manifest, err := BuildRuntimeCandidateContractManifest(decision, ownerFacts)
		if err != nil {
			t.Fatalf("BuildRuntimeCandidateContractManifest: %v", err)
		}
		if err := ValidateRuntimeCandidateContractManifestForDecision(manifest, decision, ownerFacts); err != nil {
			t.Fatalf("validate from resources: %v", err)
		}
		mutated := ownerFacts
		mutated.OperationAdmissibilities[0].TargetRefs = []string{"other.go"}
		mutated.OperationAdmissibilities[0].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(mutated.OperationAdmissibilities[0])
		if err := ValidateRuntimeCandidateContractManifestForDecision(manifest, decision, mutated); err == nil {
			t.Fatal("changed admissibility inventory must invalidate sealed manifest")
		}
	})
}

func TestRuntimeContractBudgetsAreIndependentTypes(t *testing.T) {
	t.Parallel()
	owner := DefaultRuntimeOwnerInventoryBudget()
	contextBudget := DefaultRuntimeCandidateContextBudget()
	attempt := DefaultRuntimeCandidateAttemptBudget()
	refresh := DefaultRuntimePrerequisiteRefreshBudget()
	if err := owner.Validate(); err != nil {
		t.Fatalf("owner inventory budget: %v", err)
	}
	if err := contextBudget.Validate(); err != nil {
		t.Fatalf("candidate context budget: %v", err)
	}
	if err := attempt.Validate(); err != nil {
		t.Fatalf("candidate attempt budget: %v", err)
	}
	if err := refresh.Validate(); err != nil {
		t.Fatalf("prerequisite refresh budget: %v", err)
	}
	if !RuntimeOwnerInventoryWithinBudget(owner, RuntimeOwnerInventoryItemsMaxCount) {
		t.Fatal("complete owner inventory at max must fit owner budget")
	}
	if RuntimeOwnerInventoryWithinBudget(owner, RuntimeOwnerInventoryItemsMaxCount+1) {
		t.Fatal("owner inventory over max must not fit owner budget")
	}
	// Even when numeric ceilings coincide, the two budgets remain distinct DTO
	// types and must not be assignable substitutes in API signatures.
	var _ RuntimeOwnerInventoryBudget = owner
	var _ RuntimeCandidateContextBudget = contextBudget
	if owner.MaxItems != RuntimeOwnerInventoryItemsMaxCount || contextBudget.MaxItems != RuntimeCandidateContextItemsMaxCount {
		t.Fatalf("default budgets drifted: owner=%+v context=%+v", owner, contextBudget)
	}
	if attempt.MaxAttempts != 2 {
		t.Fatalf("candidate attempt budget = %d, want 2", attempt.MaxAttempts)
	}
}

func TestValidateRuntimeCandidateAgainstManifestRejectsTargetOutsideSealedPolicy(t *testing.T) {
	decision := RuntimeDecision{
		State: ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{StepKindFileEdit},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		},
	}
	manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = DecodeRuntimeCandidatePayloadWithManifest(manifest, []byte(`{"operations":[{"id":"edit","kind":"file/edit","coverageWorkItemIds":["cwi-1"],"targetRefs":["other.go"],"parameters":[{"name":"oldText","string":"old"},{"name":"newText","string":"new"}]}]}`))
	if err == nil {
		t.Fatal("decoder must reject target outside sealed AllowedTargetRefs")
	}
	var diagnostic *RuntimeCandidatePayloadDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Path != "$.operations[0].targetRefs" || diagnostic.Actual != "other.go" || !containsManifestString(diagnostic.Allowed, "main.go") {
		t.Fatalf("outside-target diagnostic = %+v err=%v", diagnostic, err)
	}
	// Semantic validator must also reject an already-decoded outside target.
	decoded := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
		ID: "edit", Kind: StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"other.go"},
		Parameters: []OperationParameter{
			{Name: "oldText", String: semanticShapeStringPointer("old")},
			{Name: "newText", String: semanticShapeStringPointer("new")},
		},
	}}}}}
	err = ValidateRuntimeCandidateAgainstManifest(manifest, decoded)
	var semantic *RuntimeCandidateContractDiagnostic
	if !errors.As(err, &semantic) || semantic.Path != "$.operations[0].targetRefs" || semantic.Actual != "other.go" || !containsManifestString(semantic.Allowed, "main.go") {
		t.Fatalf("semantic outside-target diagnostic = %+v err=%v", semantic, err)
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
