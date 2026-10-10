package shell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func conjunctiveSourceRequirement(t *testing.T) (runtimecontract.RuntimeValidationRecipe, runtimecontract.RuntimeValidationProofRequirement, []runtimecontract.RuntimeValidationCheckAttestationBinding) {
	t.Helper()
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "source-package", Command: "go", Args: []string{"test", "-v", "./..."}, AdapterRef: modulecommon.ValidationRecipeAdapterConjunctiveEntrypoint, InputReviewProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocolV2}
	requirement := shellValidationProofRequirementForTest(t, recipe.ID, "shared-source")
	for _, id := range []string{"A", "B"} {
		check := runtimecontract.RuntimeSourceObligationCheck{InventoryHash: "inventory", ObligationID: id, RequirementID: "requirement", VerificationID: "verify-" + id, Criterion: "独立判据 " + id, Declaration: "验证 " + id}
		check.CheckRef = runtimecontract.StableRuntimeSourceObligationCheckRef(requirement.DefinitionRevision, requirement.CoverageWorkItemID, check)
		requirement.SourceObligationChecks = append(requirement.SourceObligationChecks, check)
		requirement.RequiredCheckRefs = append(requirement.RequiredCheckRefs, check.CheckRef)
	}
	requirement = requirement.Normalized()
	requirement.Hash = runtimecontract.StableRuntimeValidationProofRequirementHash(requirement)
	if err := requirement.Validate(); err != nil {
		t.Fatal(err)
	}
	checks, supported := shellValidationCheckAttestations(recipe, requirement)
	if !supported || len(checks) != 3 {
		t.Fatal("complete source package mapping unavailable")
	}
	return recipe, requirement, checks
}

func conjunctiveSourceInput(checks []runtimecontract.RuntimeValidationCheckAttestationBinding, bRef, bBody string) string {
	var source strings.Builder
	source.WriteString("package checks\nimport (\"testing\"; delivery \"fixture/delivery\")\n")
	for _, check := range checks {
		body := `if delivery.Compute(0)!=7 { t.Fatal("CWI/A") }`
		if check.RequiredCheckRef == bRef {
			body = bBody
		}
		fmt.Fprintf(&source, "func %s(t *testing.T){ %s }\n", check.NativeCheckRef, body)
	}
	return source.String()
}

func TestConjunctiveInputPreparationReviewAndIdentity(t *testing.T) {
	recipe, requirement, checks := conjunctiveSourceRequirement(t)
	root := t.TempDir()
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	manifest := shellTestWorkspaceManifestBinding("empty-input", []string{"go.mod"})
	surfaces := shellValidationSurfaces([]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, nil, []runtimecontract.RuntimeValidationProofRequirement{requirement}, manifest, "policy", root)
	if len(surfaces) != 1 || surfaces[0].Validate() != nil || len(surfaces[0].ProofRequirementPreparations) != 1 || len(surfaces[0].ProofRequirementBindings) != 0 {
		t.Fatalf("incomplete source input published proof: %+v", surfaces)
	}
	preparation := surfaces[0].ProofRequirementPreparations[0]
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	if preparation.TargetRef != target || !reflect.DeepEqual(preparation.CheckAttestations, checks) || preparation.ContentContractRef != shellConjunctiveInputContentContractRef {
		t.Fatal("preparation lost the complete conjunctive identity")
	}
	for _, check := range checks {
		if shellVersionedNativeCheckTarget(check.NativeCheckRef) != target {
			t.Fatal("member escaped its requirement input package")
		}
	}
	source := conjunctiveSourceInput(checks, requirement.SourceObligationChecks[1].CheckRef, `if delivery.Compute(4)!=11 {t.Fatal("B")}`)
	writeNativeStructureSource(t, root, target, source)
	writeNativeStructureSource(t, root, "delivery/value.go", "package delivery\nfunc Compute(v int) int{return v+7}\n")
	refs := []string{"go.mod", target, "delivery/value.go"}
	manifest = shellTestWorkspaceManifestBinding("complete-input", refs)
	surfaces = shellValidationSurfaces([]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationProofRequirement{requirement}, manifest, "policy", root)
	if len(surfaces) != 1 || surfaces[0].Validate() != nil || len(surfaces[0].ProofRequirementBindings) != 1 {
		t.Fatalf("complete source input unavailable: %+v", surfaces)
	}
	binding := surfaces[0].ProofRequirementBindings[0]
	if binding.InputReview == nil || len(binding.InputReview.Files) != 2 || binding.InputReview.ProtocolRef != runtimecontract.RuntimeValidationInputReviewProtocolV2 || binding.InputReview.ScopeContractRef != shellConjunctiveReviewScopeContractRef || binding.InputReview.ValidateAgainstBinding(binding) != nil {
		t.Fatal("complete source input review lost member/scope identity")
	}
	found := false
	for _, file := range binding.InputReview.Files {
		if file.TargetRef == target {
			found = file.Content == source
		}
	}
	if !found {
		t.Fatal("review replaced or truncated source bytes")
	}
	changed := requirement.Clone()
	changed.SourceObligationChecks[1].Criterion += " 增补"
	changed.SourceObligationChecks[1].CheckRef = runtimecontract.StableRuntimeSourceObligationCheckRef(changed.DefinitionRevision, changed.CoverageWorkItemID, changed.SourceObligationChecks[1])
	for i, ref := range changed.RequiredCheckRefs {
		if ref == requirement.SourceObligationChecks[1].CheckRef {
			changed.RequiredCheckRefs[i] = changed.SourceObligationChecks[1].CheckRef
		}
	}
	changed = changed.Normalized()
	changed.Hash = runtimecontract.StableRuntimeValidationProofRequirementHash(changed)
	changedChecks, supported := shellValidationCheckAttestations(recipe, changed)
	if !supported || shellValidationNativeCheckTargetRef(changed, changedChecks...) == target {
		t.Fatal("changed B reused old package identity")
	}
	mixed := runtimecontract.CloneRuntimeValidationCheckAttestationBindings(checks)
	mixed[1] = changedChecks[1]
	if _, err := shellIsolatedValidationPackage(mixed); err == nil {
		t.Fatal("mixed requirement packages admitted")
	}
}

func TestConjunctiveInputRejectsReviewDowngradeAndCapacityOverflow(t *testing.T) {
	recipe, requirement, checks := conjunctiveSourceRequirement(t)
	for _, protocol := range []string{"", runtimecontract.RuntimeValidationInputReviewProtocol} {
		downgraded := recipe
		downgraded.InputReviewProtocolRef = protocol
		if _, supported := shellValidationCheckAttestations(downgraded, requirement); supported {
			t.Fatal("conjunctive source input bypassed exact per-criterion review")
		}
		if resolved := resolveRuntimeValidationRecipeDeclarations([]runtimecontract.RuntimeValidationRecipe{downgraded}, runtimecontract.WorkspaceManifest{}); len(resolved) != 0 {
			t.Fatal("downgraded recipe remained selectable")
		}
	}
	for _, scenario := range []string{"files", "bytes"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			target := shellValidationNativeCheckTargetRef(requirement, checks...)
			writeNativeStructureSource(t, root, target, conjunctiveSourceInput(checks, requirement.SourceObligationChecks[1].CheckRef, `if delivery.Compute(4)!=11 {t.Fatal("B")}`))
			refs := []string{target}
			if scenario == "bytes" {
				ref := filepath.ToSlash(filepath.Join(filepath.Dir(target), "fixture.txt"))
				writeNativeStructureSource(t, root, ref, strings.Repeat("x", runtimecontract.RuntimeValidationInputReviewMaxBytes))
				refs = append(refs, ref)
			} else {
				for index := range runtimecontract.RuntimeValidationInputReviewMaxFiles {
					ref := fmt.Sprintf("%s/fixture-%d.txt", filepath.ToSlash(filepath.Dir(target)), index)
					writeNativeStructureSource(t, root, ref, "data")
					refs = append(refs, ref)
				}
			}
			binding, err := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, recipe.ID, checks...)
			if err != nil {
				t.Fatal(err)
			}
			if subject, code := shellVersionedInputReview(root, refs, binding); subject != nil || code != "validation_input_review_capacity_exceeded" {
				t.Fatalf("oversized subject was truncated or admitted: %+v %s", subject, code)
			}
		})
	}
}

func TestConjunctiveInputExecutesEverySourceAndRejectsUnreviewedMembers(t *testing.T) {
	for _, scenario := range []string{"all pass", "B fails", "B skipped", "B missing", "extra test", "unlisted helper", "supplement"} {
		t.Run(scenario, func(t *testing.T) {
			recipe, requirement, checks := conjunctiveSourceRequirement(t)
			root := t.TempDir()
			target := shellValidationNativeCheckTargetRef(requirement, checks...)
			bBody := `if delivery.Compute(4)!=11 {t.Fatal("B")}`
			if scenario == "B fails" {
				bBody = `if delivery.Compute(4)!=12 {t.Fatal("B")}`
			}
			if scenario == "B skipped" {
				bBody = `t.Run("required",func(t *testing.T){t.Skip("B unavailable"); if delivery.Compute(4)!=11 {t.Fatal("B")}})`
			}
			inputChecks := checks
			if scenario == "B missing" {
				inputChecks = nil
				for _, check := range checks {
					if check.RequiredCheckRef != requirement.SourceObligationChecks[1].CheckRef {
						inputChecks = append(inputChecks, check)
					}
				}
			}
			input := conjunctiveSourceInput(inputChecks, requirement.SourceObligationChecks[1].CheckRef, bBody)
			if scenario == "extra test" {
				input += "func TestUnlisted(t *testing.T){t.Fatal(\"unlisted\")}\n"
			}
			files := map[string]string{
				"go.mod": "module fixture\ngo 1.26\n", target: input,
				"delivery/value.go":                             "package delivery\nfunc Compute(v int) int{return v+7}\n",
				"workorch_validation/v4/neighbor/input_test.go": "package old\nfunc broken(\n",
			}
			if scenario == "supplement" {
				files[shellSupplementTargetRef(checks[0].NativeCheckRef)] = fmt.Sprintf("package checks\nimport \"testing\"\nfunc %s(t *testing.T){if 2+2!=4 {t.Fatal(\"supplement\")}}\n", shellSupplementCheckRef(checks[0].NativeCheckRef))
			}
			var refs []string
			for ref, content := range files {
				writeNativeStructureSource(t, root, ref, content)
				refs = append(refs, ref)
			}
			if scenario == "unlisted helper" {
				writeNativeStructureSource(t, root, filepath.ToSlash(filepath.Join(filepath.Dir(target), "unlisted.go")), "package checks\nfunc init(){panic(\"unreviewed\")}\n")
			}
			checks = shellValidationSupplementAttestations(checks, refs)
			request := RuntimeCommandRequest{WorkspaceDeliverableDirectory: root, ValidationRecipeID: recipe.ID, Command: recipe.Command, Args: recipe.Args, TimeoutMillis: 60000, TargetRefs: []string{target}, ValidationScopeTargetRefs: refs, ValidationCheckAttestations: checks, ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}}
			if err := instrumentManagedValidationNativeChecks(&request); err != nil {
				t.Fatal(err)
			}
			proof := runtimecontract.CoverageEvidenceProof{EvidenceKind: runtimecontract.EvidenceKindValidation, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession}, AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: recipe.ID, ValidationProofRequirementHash: requirement.Hash, RequiredCheckRefs: requirement.RequiredCheckRefs, ValidationCheckAttestations: checks}
			result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), request)
			observation := result.ValidationObservation
			passed := err == nil && result.Evidence.Quality == common.EvidenceQualityValid && observation != nil && runtimecontract.RuntimeValidationObservationSatisfiesCoverageProof(proof, *observation)
			if passed != (scenario == "all pass" || scenario == "supplement") {
				t.Fatalf("source all_of=%t err=%v code=%s observation=%+v", passed, err, result.FailureCode, observation)
			}
			if scenario == "B missing" || scenario == "extra test" || scenario == "unlisted helper" {
				if err == nil || observation != nil || result.FailureCode != "validation_behavior_contract_rejected" {
					t.Fatalf("unreviewable input reached execution: %+v %v", result, err)
				}
			} else if observation == nil || len(observation.ExecutedChecks) < 3 {
				t.Fatal("actual owner omitted independent source observation")
			}
			for ref, want := range files {
				got, err := os.ReadFile(filepath.Join(root, ref))
				if err != nil || string(got) != want {
					t.Fatalf("execution rewrote input %s", ref)
				}
			}
		})
	}
}
