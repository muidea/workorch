package shell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestShellSourceObligationAttestationsHaveIndependentNativeChecks(t *testing.T) {
	item := runtimecontract.RuntimeCoverageItem{ID: "shared", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "共同判据"}
	for _, id := range []string{"A", "B"} {
		check := runtimecontract.RuntimeSourceObligationCheck{InventoryHash: "inventory", ObligationID: id, RequirementID: "requirement", VerificationID: "verify-" + id, Criterion: "判据 " + id, Declaration: "声明 " + id}
		check.CheckRef = runtimecontract.StableRuntimeSourceObligationCheckRef("definition", item.ID, check)
		item.SourceObligationChecks = append(item.SourceObligationChecks, check)
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements("definition", []runtimecontract.RuntimeCoverageItem{item}, []runtimecontract.RuntimeDefinitionFocus{{
		ID: "focus", AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, EvidenceDeclarationRef: "definition/dodItem/dod-001",
	}})
	if err != nil || len(requirements) != 1 {
		t.Fatalf("requirement: %+v %v", requirements, err)
	}
	for _, adapter := range []string{"", modulecommon.ValidationRecipeAdapterProjectNative, modulecommon.ValidationRecipeAdapterProjectEntrypoint, modulecommon.ValidationRecipeAdapterConjunctiveEntrypoint} {
		t.Run(adapter, func(t *testing.T) {
			recipe := runtimecontract.RuntimeValidationRecipe{Command: "go", Args: []string{"test", "./..."}, AdapterRef: adapter}
			if adapter == modulecommon.ValidationRecipeAdapterConjunctiveEntrypoint {
				recipe.InputReviewProtocolRef = runtimecontract.RuntimeValidationInputReviewProtocolV2
			}
			attestations, supported := shellValidationCheckAttestations(recipe, requirements[0])
			if !supported || len(attestations) != 3 {
				t.Fatalf("owner lost independent all_of checks: %+v %t", attestations, supported)
			}
			if err := runtimecontract.ValidateRuntimeValidationCheckAttestationBindings(attestations, requirements[0].RequiredCheckRefs); err != nil {
				t.Fatal(err)
			}
			seenNative := make(map[string]bool)
			for _, attestation := range attestations {
				if seenNative[attestation.NativeCheckRef] || !strings.HasPrefix(attestation.NativeCheckRef, "TestWorkOrchCWI_") {
					t.Fatal("source native identity aliases another obligation")
				}
				seenNative[attestation.NativeCheckRef] = true
			}
		})
	}
	// 单输入的版本化协议不能借新增来源声明自动变成多输入协议。
	for _, adapter := range []string{modulecommon.ValidationRecipeAdapterVersionedEntrypoint, modulecommon.ValidationRecipeAdapterIsolatedEntrypoint} {
		if _, supported := shellValidationCheckAttestations(runtimecontract.RuntimeValidationRecipe{Command: "go", Args: []string{"test", "./..."}, AdapterRef: adapter}, requirements[0]); supported {
			t.Fatal("single-input protocol silently widened")
		}
	}
}

func TestShellSourceObligationActualOwnerExecutionRequiresBoth(t *testing.T) {
	item := runtimecontract.RuntimeCoverageItem{ID: "shared", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "共同判据"}
	for _, id := range []string{"A", "B"} {
		check := runtimecontract.RuntimeSourceObligationCheck{InventoryHash: "inventory", ObligationID: id, RequirementID: "requirement", VerificationID: "verify-" + id, Criterion: "独立判据 " + id, Declaration: "验证 " + id}
		check.CheckRef = runtimecontract.StableRuntimeSourceObligationCheckRef("definition", item.ID, check)
		item.SourceObligationChecks = append(item.SourceObligationChecks, check)
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements("definition", []runtimecontract.RuntimeCoverageItem{item}, []runtimecontract.RuntimeDefinitionFocus{{
		ID: "focus", AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, EvidenceDeclarationRef: "definition/dodItem/dod-001",
	}})
	if err != nil || len(requirements) != 1 {
		t.Fatalf("compile requirement: %v", err)
	}
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "source-owner-proof", AdapterRef: modulecommon.ValidationRecipeAdapterProjectNative, Command: "go", Args: []string{"test", "./..."}, TimeoutMillis: 60_000}
	attestations, supported := shellValidationCheckAttestations(recipe, requirements[0])
	if !supported || len(attestations) != 3 {
		t.Fatal("owner has no complete source native mapping")
	}
	proof := runtimecontract.CoverageEvidenceProof{EvidenceKind: runtimecontract.EvidenceKindValidation, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession}, AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: recipe.ID,
		ValidationProofRequirementHash: requirements[0].Hash, RequiredCheckRefs: requirements[0].RequiredCheckRefs, ValidationCheckAttestations: attestations}
	for _, test := range []struct {
		name, bBody string
		want        bool
		declared    bool
	}{
		{"A and B", `if got := compute(4); got != 11 { t.Fatalf("B: got %d", got) }`, true, true},
		{"only A", `if got := compute(4); got != 12 { t.Fatalf("B: got %d", got) }`, false, true},
		{"B skipped", `t.Skip("B unavailable")`, false, false},
		{"B required subcheck skipped", `t.Run("required", func(t *testing.T) { t.Skip("B unavailable"); if got := compute(4); got != 11 { t.Fatalf("B: got %d", got) } })`, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			text := "package fixture\nimport \"testing\"\n"
			for _, attestation := range attestations {
				body := `if got := compute(0); got != 7 { t.Fatalf("A/CWI: got %d", got) }`
				if attestation.RequiredCheckRef == item.SourceObligationChecks[1].CheckRef {
					body = test.bBody
				}
				text += fmt.Sprintf("func %s(t *testing.T) { %s }\n", attestation.NativeCheckRef, body)
			}
			workspace := newGoValidationWorkspace(t, text)
			if err := os.WriteFile(filepath.Join(workspace, "fixture.go"), []byte("package fixture\nfunc compute(v int) int { return v+7 }\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			request := RuntimeCommandRequest{WorkspaceDeliverableDirectory: workspace, ValidationRecipeID: recipe.ID, Command: recipe.Command, Args: recipe.Args, TimeoutMillis: recipe.TimeoutMillis,
				ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRefs: []string{"fixture_test.go"}, ValidationScopeTargetRefs: []string{"fixture.go", "fixture_test.go", "go.mod"}, ValidationCheckAttestations: attestations}
			if err := instrumentManagedValidationNativeChecks(&request); err != nil {
				t.Fatal(err)
			}
			declared := shellNativeValidationChecksDeclared(workspace, request.ValidationScopeTargetRefs, attestations)
			if declared != test.declared {
				t.Fatalf("independent input declaration=%t want=%t", declared, test.declared)
			}
			// 直接执行同时验证观察层；结构检查拒绝的输入仍不能获得 owner proof。
			result, executeErr := NewRuntimeAdapter().ExecuteOperation(context.Background(), request)
			observation := result.Evidence.Validation
			passed := declared && executeErr == nil && observation != nil && runtimecontract.RuntimeValidationObservationSatisfiesCoverageProof(proof, *observation)
			if passed != test.want {
				t.Fatalf("actual owner all_of=%t want=%t error=%v observation=%+v", passed, test.want, executeErr, observation)
			}
			if observation == nil || len(observation.ExecutedChecks) != 3 {
				t.Fatal("actual owner omitted a source check observation")
			}
			if test.name == "B required subcheck skipped" && len(observation.IncompleteChecks) == 0 {
				t.Fatal("skipped B subcheck lost its incomplete observation")
			}
		})
	}
}
