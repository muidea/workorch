package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func TestRuntimeCandidateLineagePrivateValidationReuseIsDetachedAndEquivalent(t *testing.T) {
	manifest := runtimeSessionValidationManifestForTest(t)
	validated, err := validateRuntimeCandidateManifestRawCanonical(manifest)
	if err != nil {
		t.Fatal(err)
	}
	before := manifest.Clone()
	candidate := RuntimeCandidate{}
	publicErr := ValidateRuntimeCandidateAgainstManifest(manifest, candidate)
	privateErr := validateRuntimeCandidateAgainstValidatedManifest(validated, candidate)
	if publicErr == nil || privateErr == nil || publicErr.Error() != privateErr.Error() {
		t.Fatalf("candidate rejection changed: public=%v private=%v", publicErr, privateErr)
	}
	content := "测试内容"
	validCandidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{ID: "write", Kind: StepKindFileWrite, BindingRefs: []string{manifest.Bindings[0].ID}, TargetRefs: []string{"target.txt"}, Parameters: []OperationParameter{{Name: "content", String: &content}}}}}}}
	validCandidate, err = DeriveRuntimeCandidateOperationAuthority(manifest, validCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeCandidateAgainstManifest(manifest, validCandidate); err != nil {
		t.Fatal(err)
	}
	if err := validateRuntimeCandidateAgainstValidatedManifest(validated, validCandidate); err != nil {
		t.Fatal(err)
	}
	publicTargets, err := RuntimeCandidateAdmissionTargetAuthorizationsFromManifest(manifest, validCandidate)
	if err != nil {
		t.Fatal(err)
	}
	privateTargets, err := runtimeOperationTargetAuthorizationsFromNormalizedManifest(validated.manifest, validCandidate)
	if err != nil || !reflect.DeepEqual(publicTargets, privateTargets) {
		t.Fatalf("target projection differs: %v", err)
	}
	publicRecipes, err := RuntimeCandidateManagedValidationRecipeSnapshot(manifest, validCandidate)
	if err != nil {
		t.Fatal(err)
	}
	privateRecipes, err := runtimeCandidateManagedValidationRecipeSnapshotFromNormalizedManifest(validated.manifest, validCandidate)
	if err != nil || !reflect.DeepEqual(publicRecipes, privateRecipes) {
		t.Fatalf("recipe projection differs: %v", err)
	}
	if !reflect.DeepEqual(manifest, before) {
		t.Fatal("validation mutated source")
	}
	// 私有路径仅复用本次完整校验的正文；public oracle 仍逐次规范化并重算。
	wantBindingHash := StableRuntimeCoverageEvidenceBindingSnapshotHash(manifest.Bindings)
	if got := stableRuntimeCoverageEvidenceBindingSnapshotHashNormalized(validated.manifest.Bindings); got != wantBindingHash {
		t.Fatalf("binding snapshot identity changed: got=%s want=%s", got, wantBindingHash)
	}
	for _, changed := range []func([]CoverageEvidenceBinding){
		func(values []CoverageEvidenceBinding) { values[0].DeclarationRef = "changed" },
		func(values []CoverageEvidenceBinding) { values[0].TargetRefs = []string{"other.txt"} },
		func(values []CoverageEvidenceBinding) { values[0].Proofs[0].ArtifactRefs = []string{"other-artifact"} },
	} {
		projection := CloneCoverageEvidenceBindings(manifest.Bindings)
		changed(projection)
		projection = NormalizeCoverageEvidenceBindings(projection)
		if reflect.DeepEqual(validated.manifest.Bindings, projection) ||
			stableRuntimeCoverageEvidenceBindingSnapshotHashNormalized(projection) == wantBindingHash ||
			StableRuntimeCoverageEvidenceBindingSnapshotHash(projection) != stableRuntimeCoverageEvidenceBindingSnapshotHashNormalized(projection) {
			t.Fatal("binding body drift escaped full projection/hash comparison")
		}
	}
	manifest.Operations[0].OwnerModuleID = "tampered"
	if !reflect.DeepEqual(validated.manifest, before) {
		t.Fatal("validated manifest aliases caller")
	}
	if _, err := validateRuntimeCandidateManifestRawCanonical(manifest); err == nil {
		t.Fatal("new invocation reused prior successful validation")
	}
	if ValidateRuntimeCandidateAgainstManifest(manifest, candidate) == nil {
		t.Fatal("public candidate gate accepted changed manifest")
	}
}
