package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeCandidateContractManifestRawCanonicalRejectsNormalizationLaundering(t *testing.T) {
	decision := RuntimeDecision{
		State: ConvergenceStateNeedValidation,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:         []string{RuntimeBlockKindManagedValidationSession},
			AcceptedEffectScopes:  []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession},
			RequiredEffectScopes:  []RuntimeEffectScope{RuntimeEffectNone},
			RequiredEvidenceKinds: []string{EvidenceKindValidation},
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
		},
	}
	decision = runtimeCandidateTestDecisionWithBindings(t, decision)
	recipe := RuntimeValidationRecipe{ID: "recipe/raw-canonical", Command: "go", Args: []string{"test", "./..."}, TimeoutMillis: 60_000}
	manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{
		ValidationRecipes: []RuntimeValidationRecipe{recipe},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Operations[0].ValidationRecipes) == 0 {
		// The raw-shape fence is independent from owner recipe admission. Keep a
		// canonical nested recipe in this structural fixture even when the current
		// manifest compiler filters a recipe that lacks an owner catalog preimage.
		manifest.Operations[0].ValidationRecipes = []RuntimeValidationRecipe{recipe}
		manifest = manifest.Normalized()
		manifest.Hash = StableRuntimeCandidateContractManifestHash(manifest)
	}
	if err := ValidateRuntimeCandidateContractManifestRawCanonical(manifest); err != nil {
		t.Fatalf("built manifest is not raw-canonical: %v", err)
	}
	validSession := runtimeManifestBoundSessionForTest(t, manifest)

	tests := []struct {
		name   string
		mutate func(*RuntimeCandidateContractManifest)
	}{
		{"duplicate operation kind", func(value *RuntimeCandidateContractManifest) {
			value.Operations = append(value.Operations, value.Operations[0])
		}},
		{"duplicate target", func(value *RuntimeCandidateContractManifest) {
			value.Operations[0].Target.ExactTargetRefs = []string{"target-1", "target-1"}
		}},
		{"duplicate binding", func(value *RuntimeCandidateContractManifest) {
			value.Bindings = append(value.Bindings, value.Bindings[0])
		}},
		{"duplicate recipe", func(value *RuntimeCandidateContractManifest) {
			value.Operations[0].ValidationRecipes = append(value.Operations[0].ValidationRecipes, value.Operations[0].ValidationRecipes[0])
		}},
		{"noncanonical order", func(value *RuntimeCandidateContractManifest) {
			values := value.Operations[0].AllowedOperationFields
			for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
				values[left], values[right] = values[right], values[left]
			}
			value.Operations[0].AllowedOperationFields = values
		}},
		{"whitespace", func(value *RuntimeCandidateContractManifest) {
			value.Revision = " " + value.Revision + " "
		}},
		{"nil empty drift", func(value *RuntimeCandidateContractManifest) {
			value.TargetRefs = []string{}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := manifest
			// Detach every nested collection before introducing a raw-only drift.
			mutated = cloneRuntimeCandidateContractManifestRaw(mutated)
			test.mutate(&mutated)
			mutated.Hash = StableRuntimeCandidateContractManifestHash(mutated)
			if err := ValidateRuntimeCandidateContractManifestRawCanonical(mutated); err == nil {
				t.Fatal("raw-canonical validator accepted normalization laundering")
			}
			if _, err := BuildRuntimeCandidateWireContract(mutated); err == nil {
				t.Fatal("公开 wire compiler 接受非 canonical manifest")
			}
			session := validSession.Clone()
			session.CandidateManifest = &mutated
			session.ManifestHash = mutated.Hash
			session.ExpectedDocument.ManifestHash = mutated.Hash
			session.ExpectedDocument.Hash = StableRuntimeExpectedDocumentHash(*session.ExpectedDocument)
			if err := session.Validate(); err == nil || !strings.Contains(err.Error(), "persisted operations manifest") {
				t.Fatalf("完整合法 request session 未在原始 manifest gate 拒绝: %v", err)
			}
		})
	}
}
