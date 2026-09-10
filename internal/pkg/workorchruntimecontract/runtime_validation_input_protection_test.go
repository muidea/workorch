package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func inputProtectionCatalog(t *testing.T) RuntimeOperationAdmissibility {
	t.Helper()
	surface := RuntimeValidationSurface{
		ID: "owner-check", OwnerModuleID: RuntimeModuleMCP, OperationKind: StepKindMCPToolCall,
		Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: "schema", InputBindingRef: "input",
		ProofApplicability: RuntimeValidationProofAuxiliary, EffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
		InputTargetOwnerModuleID: RuntimeModuleFile, InputTargetRefs: []string{"acceptance.fixture"},
	}
	catalog := RuntimeOperationAdmissibility{PrerequisiteID: "checks", OwnerModuleID: RuntimeModuleMCP, OperationKinds: []string{StepKindMCPToolCall}, ObservedRevision: "rev-1", ValidationSurfaces: []RuntimeValidationSurface{surface}}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestValidationInputProtectionCompilesIntoOrdinaryMutationAndReplay(t *testing.T) {
	decision := runtimeCandidateTestDecisionWithBindings(t, RuntimeDecision{
		State:          ConvergenceStateNeedStateChange,
		RequiredOutput: RequiredOutputContract{AcceptedKinds: []string{StepKindFileWrite}, AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"acceptance.fixture", "implementation", "another-implementation"}},
	})
	catalog := inputProtectionCatalog(t)
	manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: []RuntimeOperationAdmissibility{catalog}})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.MutationInputCatalogs) != 1 || !reflect.DeepEqual(manifest.Operations[0].Target.ExcludedTargetRefs, []string{"acceptance.fixture"}) {
		t.Fatalf("ordinary mutation lost input protection: %+v", manifest)
	}
	encode := func(target string) []byte {
		return []byte(`{"operations":[{"kind":"file/write","targetRefs":["` + target + `"],"parameters":[{"name":"content","string":"implementation"}]}]}`)
	}
	if _, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, encode("acceptance.fixture")); err == nil {
		t.Fatal("ordinary source-change can modify validation input")
	}
	candidate, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, encode("implementation"))
	if err != nil {
		t.Fatalf("unrelated implementation blocked: %v", err)
	}
	projections, err := RuntimeDispatchOperationTargetAuthorizationsFromManifest(manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var restored RuntimeCandidateContractManifest
	if err := json.Unmarshal(body, &restored); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := ValidateRuntimeCandidateContractManifestRawCanonical(restored); err != nil {
			t.Fatal(err)
		}
		if err := ValidateRuntimeCandidateAgainstManifest(restored, candidate); err != nil {
			t.Fatal(err)
		}
		if err := ValidateRuntimeDispatchOperationTargetAuthorizations(candidate, projections); err != nil {
			t.Fatal(err)
		}
	}
	for _, mutate := range []func(*RuntimeCandidateContractManifest){
		func(m *RuntimeCandidateContractManifest) { m.Operations[0].Target.ExcludedTargetRefs = nil },
		func(m *RuntimeCandidateContractManifest) { m.MutationInputCatalogs = nil },
		func(m *RuntimeCandidateContractManifest) {
			m.MutationInputCatalogs[0].ValidationSurfaces[0].InputTargetRefs = []string{"different"}
		},
	} {
		changed := manifest.Clone()
		mutate(&changed)
		changed.Hash = StableRuntimeCandidateContractManifestHash(changed)
		if err := ValidateRuntimeCandidateContractManifestRawCanonical(changed); err == nil {
			t.Fatal("rehashed mutation-protection drift accepted")
		}
	}
	removed := manifest.Clone()
	removed.MutationInputCatalogs, removed.Operations[0].Target.ExcludedTargetRefs = nil, nil
	removed.Hash = StableRuntimeCandidateContractManifestHash(removed)
	if err := ValidateRuntimeCandidateContractManifestForDecision(removed, decision, RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: []RuntimeOperationAdmissibility{catalog}}); err == nil {
		t.Fatal("coordinated removal of protection and preimage bypassed sealed owner facts")
	}
	if !reflect.DeepEqual(manifest, restored) {
		t.Fatal("validation mutated original manifest")
	}
}

func TestValidationInputProtectionAppliesToEveryTargetPolicy(t *testing.T) {
	for _, target := range []RuntimeOperationTargetAuthorization{
		{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"protected", "ordinary"}},
		{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: []string{"protected", "ordinary"}},
		{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{"owner-selector"}},
		{Policy: RuntimeOperationTargetPolicyProspectiveOwnerTarget, OwnerSelectorRefs: []string{"owner-selector"}, ProspectiveTargetRefs: []string{"protected", "ordinary"}},
	} {
		target.ExcludedTargetRefs = []string{"protected"}
		for range 2 {
			if ValidateRuntimeOperationTargetsAgainstAuthorization(target, []string{"protected"}, true) == nil {
				t.Fatalf("policy %s bypassed negative membership", target.Policy)
			}
			if err := ValidateRuntimeOperationTargetsAgainstAuthorization(target, []string{"ordinary"}, true); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestValidationInputProtectionOnlyOpensExactCriterionPreservingRepair(t *testing.T) {
	repair := testValidationInputRepair()
	catalog := inputProtectionCatalog(t)
	catalog.ValidationSurfaces[0].InputTargetRefs = []string{repair.TargetRef, "other-input"}
	facts := RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: []RuntimeOperationAdmissibility{catalog}, InputRepair: repair}
	binding := CoverageEvidenceBinding{Proofs: []CoverageEvidenceProof{{ValidationCheckAttestations: []RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: repair.RequiredCheckRef, ObservationProtocolRef: repair.ObservationProtocolRef, NativeCheckRef: repair.OriginalNativeCheckRef, SupplementalNativeCheckRef: repair.SupplementalNativeCheckRef}}}}}
	for _, confirmed := range []bool{false, true} {
		required := RequiredOutputContract{}
		if confirmed {
			required.RepairPurpose, required.RepairClosureID = RuntimeDispatchRepairPurposeConfirmedValidation, "closure"
		}
		got := runtimeValidationInputMutationExclusions(RuntimeModuleFile, facts, required, []CoverageEvidenceBinding{binding})
		if containsString(got, repair.TargetRef) == confirmed || !containsString(got, "other-input") {
			t.Fatalf("repair exemption widened or lost: %v", got)
		}
	}
	if got := runtimeValidationInputMutationExclusions(RuntimeModuleGit, facts, RequiredOutputContract{}, nil); len(got) != 0 {
		t.Fatalf("input protection crossed resource owner: %v", got)
	}
}

func TestValidationInputProtectionEmptyExtensionPreservesLegacyIdentity(t *testing.T) {
	catalog := inputProtectionCatalog(t)
	protectedHash := catalog.SnapshotID
	catalog.ValidationSurfaces[0].InputTargetRefs, catalog.ValidationSurfaces[0].InputTargetOwnerModuleID = nil, ""
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if catalog.SnapshotID == protectedHash {
		t.Fatal("input protection missing from owner identity")
	}
	body, _ := json.Marshal(catalog)
	if strings.Contains(string(body), "InputTarget") {
		t.Fatal("empty protection extension rewrites legacy JSON")
	}
	var restored RuntimeOperationAdmissibility
	if err := json.Unmarshal(body, &restored); err != nil || restored.Validate() != nil || StableRuntimeOperationAdmissibilitySnapshotID(restored) != catalog.SnapshotID {
		t.Fatalf("legacy catalog identity changed: %v", err)
	}
	for _, refs := range [][]string{{"a", "a"}, {""}, {" padded"}} {
		bad := inputProtectionCatalog(t)
		bad.ValidationSurfaces[0].InputTargetRefs = refs
		bad.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(bad)
		if err := bad.Validate(); err == nil {
			t.Fatalf("invalid raw input identities accepted: %v", refs)
		}
	}
}

func TestValidationInputProtectionHasIndependentCompleteObservationBound(t *testing.T) {
	for _, count := range []int{RuntimeOwnerInventoryItemsMaxCount + 1, RuntimeValidationInputTargetsMaxCount, RuntimeValidationInputTargetsMaxCount + 1} {
		catalog := inputProtectionCatalog(t)
		refs := make([]string, count)
		for index := range refs {
			refs[index] = fmt.Sprintf("input-%04d", index)
		}
		catalog.ValidationSurfaces[0].InputTargetRefs = refs
		catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
		if (catalog.Validate() != nil) != (count > RuntimeValidationInputTargetsMaxCount) {
			t.Fatalf("protection observation bound confused with selectable inventory: count=%d", count)
		}
		if count <= RuntimeValidationInputTargetsMaxCount {
			got := runtimeValidationInputMutationExclusions(RuntimeModuleFile, RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: []RuntimeOperationAdmissibility{catalog}}, RequiredOutputContract{}, nil)
			if !reflect.DeepEqual(got, refs) {
				t.Fatal("protection observations were truncated into writable targets")
			}
		}
	}
}

func TestValidationInputExclusionsCannotBeAuthoredBySemanticProvider(t *testing.T) {
	proposal := RuntimeSemanticProofProposal{CoverageWorkItemID: "cwi-1", OwnerModuleID: RuntimeModuleFile, OperationKind: StepKindFileWrite, DeclarationRef: "definition/source", RequiredEvidenceKinds: []string{EvidenceKindStateChange}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"implementation"}}}
	if err := proposal.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, refs := range [][]string{{"input.fixture"}, {""}} {
		changed := proposal.Clone()
		changed.Target.ExcludedTargetRefs = refs
		if err := changed.Validate(); err == nil {
			t.Fatal("semantic provider authored operation-only mutation authority")
		}
		if refs[0] != "" && StableRuntimeSemanticProofProposalHash(proposal) == StableRuntimeSemanticProofProposalHash(changed) {
			t.Fatal("excluded target drift hidden from semantic archive identity")
		}
	}
}
