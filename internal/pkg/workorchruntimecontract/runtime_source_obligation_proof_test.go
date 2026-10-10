package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeSourceObligationRequirementRejectsOnlyAEvidenceForSharedCWI(t *testing.T) {
	inventory := sourceObligationInventoryFixture()
	item := RuntimeCoverageItem{ID: "cwi-shared", SliceID: "slice-shared", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "共同判据"}
	checks, err := CompileRuntimeSourceObligationChecks(inventory, item)
	if err != nil {
		t.Fatal(err)
	}
	item.SourceObligationChecks = checks
	requirement := runtimeValidationProofRequirementForTest(t, inventory.DefinitionRevision, item, "", "shared-recipe", nil)
	if len(requirement.RequiredCheckRefs) != 3 || len(requirement.SourceObligationChecks) != 2 {
		t.Fatalf("missing per-obligation proof: %+v", requirement)
	}
	attestations := runtimeValidationCheckAttestationsForTest(requirement.RequiredCheckRefs)
	proof := CoverageEvidenceProof{
		EvidenceKind: EvidenceKindValidation, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectEphemeralSession},
		AllowedOperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "shared-recipe",
		ValidationProofRequirementHash: requirement.Hash, RequiredCheckRefs: requirement.RequiredCheckRefs, ValidationCheckAttestations: attestations,
	}
	if err := proof.Validate(); err != nil {
		t.Fatal(err)
	}
	observation := RuntimeValidationObservation{RecipeID: "shared-recipe", ObservationProtocolRef: attestations[0].ObservationProtocolRef}
	var bNative string
	for _, attestation := range attestations {
		if attestation.RequiredCheckRef == checks[1].CheckRef {
			bNative = attestation.NativeCheckRef
			continue
		}
		observation.PassedChecks = append(observation.PassedChecks, attestation.NativeCheckRef)
	}
	if bNative == "" {
		t.Fatal("fixture lacks B attestation")
	}
	if RuntimeValidationObservationSatisfiesCoverageProof(proof, observation) {
		t.Fatal("A and CWI marker closed unproven B")
	}
	observation.PassedChecks = append(observation.PassedChecks, bNative)
	if !RuntimeValidationObservationSatisfiesCoverageProof(proof, observation) {
		t.Fatal("complete independent all_of attestations rejected")
	}
	forged := requirement.Clone()
	forged.RequiredCheckRefs = nil
	for _, ref := range requirement.RequiredCheckRefs {
		if ref != checks[1].CheckRef {
			forged.RequiredCheckRefs = append(forged.RequiredCheckRefs, ref)
		}
	}
	forged.Hash = StableRuntimeValidationProofRequirementHash(forged)
	if forged.Validate() == nil {
		t.Fatal("self-consistent requirement omitting B check accepted")
	}
}

func TestRuntimeSourceObligationSeedRejectsSelfConsistentOmittedChecks(t *testing.T) {
	inventory := sourceObligationInventoryFixture()
	item := RuntimeCoverageItem{ID: "cwi-shared", SliceID: "slice-shared", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "共同判据"}
	checks, err := CompileRuntimeSourceObligationChecks(inventory, item)
	if err != nil {
		t.Fatal(err)
	}
	item.SourceObligationChecks = checks
	seed := RuntimeDefinitionSeed{SchemaVersion: RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: inventory.DefinitionRevision, PolicyHash: "policy",
		CoverageItems: []RuntimeCoverageItem{item}, Slices: []RuntimeAcceptanceSlice{{ID: item.SliceID, CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}}}, SourceObligations: &inventory}
	seed.Hash = StableRuntimeDefinitionSeedHash(seed)
	if err := seed.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*RuntimeDefinitionSeed)
	}{
		{"missing B", func(s *RuntimeDefinitionSeed) {
			s.CoverageItems[0].SourceObligationChecks = s.CoverageItems[0].SourceObligationChecks[:1]
		}},
		{"missing inventory", func(s *RuntimeDefinitionSeed) { s.SourceObligations = nil }},
		{"changed source criterion", func(s *RuntimeDefinitionSeed) { s.CoverageItems[0].SourceObligationChecks[0].Criterion = "changed" }},
		{"different definition", func(s *RuntimeDefinitionSeed) {
			s.SourceObligations.DefinitionRevision = "wrong"
			value := s.SourceObligations.WithHash()
			s.SourceObligations = &value
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := seed.Clone()
			tc.mutate(&changed)
			changed.Hash = StableRuntimeDefinitionSeedHash(changed)
			if changed.Validate() == nil {
				t.Fatal("self-consistent seed lost exact source mapping")
			}
			if err := seed.Validate(); err != nil {
				t.Fatalf("clone mutation changed original: %v", err)
			}
		})
	}
}
