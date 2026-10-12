package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func candidateManifestHashReuseFixture(t testing.TB) RuntimeCandidateContractManifest {
	t.Helper()
	decision := runtimeCandidateTestDecisionWithBindings(t, RuntimeDecision{
		State: ConvergenceStateNeedValidation,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds:        []string{RuntimeBlockKindManagedValidationSession},
			AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectNone, RuntimeEffectEphemeralSession}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectNone},
			RequiredEvidenceKinds: []string{EvidenceKindValidation}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		},
	})
	manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{})
	if err != nil {
		t.Fatal(err)
	}
	manifest.Operations[0].ValidationRecipes = []RuntimeValidationRecipe{{ID: "recipe/hash-reuse", Command: "go", Args: []string{"test", "./..."}, TimeoutMillis: 60_000}}
	manifest = manifest.Normalized()
	manifest.Hash = StableRuntimeCandidateContractManifestHash(manifest)
	if err := ValidateRuntimeCandidateContractManifestRawCanonical(manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

// 原门禁在完整 Normalized 后仍调用公开 hash，从而再规范化一次；此对照只比较这一步的原输入/输出。
func referenceCandidateManifestHashAfterNormalization(manifest RuntimeCandidateContractManifest) string {
	canonical := manifest.Normalized()
	return StableRuntimeCandidateContractManifestHash(canonical)
}

func assertCandidateManifestHashReuseEquivalent(t testing.TB, manifest RuntimeCandidateContractManifest) {
	t.Helper()
	before, _ := json.Marshal(manifest)
	canonical := manifest.Normalized()
	want := referenceCandidateManifestHashAfterNormalization(manifest)
	for range 2 {
		got := stableRuntimeCandidateContractManifestHashNormalized(canonical)
		if got != want {
			t.Fatalf("原完整 hash 规范化路径漂移: got=%s want=%s", got, want)
		}
	}
	after, _ := json.Marshal(manifest)
	if string(before) != string(after) {
		t.Fatal("hash 产物复用改写原 manifest bytes")
	}
}

func TestCandidateManifestRawGateReusesNormalizedHashInput(t *testing.T) {
	base := candidateManifestHashReuseFixture(t)
	for _, scenario := range []string{"valid", "whitespace", "duplicate_binding", "duplicate_operation", "duplicate_recipe", "nil_empty", "hash", "recipe_args", "variant"} {
		t.Run(scenario, func(t *testing.T) {
			manifest := cloneRuntimeCandidateContractManifestRaw(base)
			switch scenario {
			case "whitespace":
				manifest.Revision = " " + manifest.Revision + "\r\n"
			case "duplicate_binding":
				manifest.Bindings = append(manifest.Bindings, manifest.Bindings[0])
			case "duplicate_operation":
				manifest.Operations = append(manifest.Operations, manifest.Operations[0])
			case "duplicate_recipe":
				manifest.Operations[0].ValidationRecipes = append(manifest.Operations[0].ValidationRecipes, manifest.Operations[0].ValidationRecipes[0])
			case "nil_empty":
				manifest.TargetRefs = []string{}
			case "hash":
				manifest.Hash = "forged"
			case "recipe_args":
				manifest.Operations[0].ValidationRecipes[0].Args = []string{"test", " 原文\r\n "}
			case "variant":
				manifest.Operations[0].Variants = append(manifest.Operations[0].Variants, RuntimeOperationVariantContract{ID: " hash-variant ", RequiredParameters: []string{"z", "a"}})
			}
			assertCandidateManifestHashReuseEquivalent(t, manifest)
			if scenario != "hash" {
				manifest.Hash = StableRuntimeCandidateContractManifestHash(manifest)
			}
			before, _ := json.Marshal(manifest)
			validated, err := validateRuntimeCandidateManifestRawCanonical(manifest)
			canonical := manifest.Normalized()
			if !reflect.DeepEqual(manifest, canonical) || scenario == "hash" || scenario == "duplicate_binding" {
				if err == nil {
					t.Fatal("raw 非法正文被 hash 复用洗成合法")
				}
			} else if err != nil {
				t.Fatalf("合法 canonical 正文拒绝: %v", err)
			}
			if err == nil {
				validated.manifest.Operations[0].ValidationRecipes[0].Args[0] = "caller-change"
			}
			after, _ := json.Marshal(manifest)
			if string(before) != string(after) {
				t.Fatal("完整 gate 返回产物共享原 manifest")
			}
			var reload RuntimeCandidateContractManifest
			if err := json.Unmarshal(before, &reload); err != nil {
				t.Fatal(err)
			}
			assertCandidateManifestHashReuseEquivalent(t, reload)
		})
	}
	current := testing.AllocsPerRun(20, func() { _ = stableRuntimeCandidateContractManifestHashNormalized(base.Normalized()) })
	reference := testing.AllocsPerRun(20, func() { _ = referenceCandidateManifestHashAfterNormalization(base) })
	if current >= reference {
		t.Fatalf("完整规范化加 hash 未减少重复复制: current=%v reference=%v", current, reference)
	}
}

func TestCandidateManifestHashBufferMatchesOriginalWriter(t *testing.T) {
	manifest := candidateManifestHashReuseFixture(t)
	for _, value := range []string{"", "短字段", string(make([]byte, 120)), string(make([]byte, 121)), string(make([]byte, 4096))} {
		changed := cloneRuntimeCandidateContractManifestRaw(manifest)
		changed.Operations[0].ValidationRecipes[0].Args = append(changed.Operations[0].ValidationRecipes[0].Args, value)
		canonical := changed.Normalized()
		if got, want := stableRuntimeCandidateContractManifestHashNormalized(canonical), encodeRuntimeCandidateContractManifestHashNormalized(canonical, sha256.New()); got != want {
			t.Fatalf("原完整 manifest 编码 bytes 漂移: got=%s want=%s", got, want)
		}
	}
	current := testing.AllocsPerRun(20, func() { _ = stableRuntimeCandidateContractManifestHashNormalized(manifest) })
	reference := testing.AllocsPerRun(20, func() { _ = encodeRuntimeCandidateContractManifestHashNormalized(manifest, sha256.New()) })
	if current >= reference {
		t.Fatalf("完整 hash 短字段编码未减少分配: current=%v reference=%v", current, reference)
	}
}

func FuzzCandidateManifestNormalizedHashMatchesOriginalPath(f *testing.F) {
	base := candidateManifestHashReuseFixture(f)
	for selector := range uint8(5) {
		f.Add(selector, " 原文\r\n ")
	}
	f.Fuzz(func(t *testing.T, selector uint8, value string) {
		manifest := cloneRuntimeCandidateContractManifestRaw(base)
		switch selector % 5 {
		case 0:
			manifest.Revision = value
		case 1:
			manifest.Operations[0].ValidationRecipes[0].Args = append(manifest.Operations[0].ValidationRecipes[0].Args, value)
		case 2:
			manifest.Operations[0].Target.ExactTargetRefs = append(manifest.Operations[0].Target.ExactTargetRefs, value)
		case 3:
			manifest.Operations[0].RequiredOperationFields = append(manifest.Operations[0].RequiredOperationFields, value)
		case 4:
			manifest.Bindings[0].DeclarationRef = value
		}
		assertCandidateManifestHashReuseEquivalent(t, manifest)
		canonical := manifest.Normalized()
		if stableRuntimeCandidateContractManifestHashNormalized(canonical) != encodeRuntimeCandidateContractManifestHashNormalized(canonical, sha256.New()) {
			t.Fatal("完整 manifest buffered hash 与原 writer 漂移")
		}
	})
}
