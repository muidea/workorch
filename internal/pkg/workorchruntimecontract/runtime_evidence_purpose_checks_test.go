package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestEvidencePurposeAcceptanceReviewUsesEachOriginalMappedCriterion(t *testing.T) {
	manifest, document := semanticSourceObligationReviewWithChecks(t, func(revision string, item RuntimeCoverageItem) []RuntimeSourceObligationCheck {
		i, _, _ := evidencePurposeMappingFixture()
		i.DefinitionRevision = revision
		for n := range i.Sources {
			if i.Sources[n].CandidateItemRef == "acceptance" {
				i.Sources[n].SourceScope, i.Sources[n].SourceID, i.Sources[n].Text = item.SourceScope, item.SourceID, item.TargetText
			} else if i.Sources[n].CandidateItemRef == "dod" {
				i.Sources[n].SourceID = "dod-historical"
			}
		}
		i = i.WithHash()
		checks, err := CompileRuntimeSourceObligationChecks(i, item)
		if err != nil {
			t.Fatal(err)
		}
		return checks
	})
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	payload, err := EncodeRuntimeSemanticPlanPayloadWithManifest(manifest, document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, payload)
	if err != nil || !reflect.DeepEqual(decoded.Normalized(), document.Normalized()) {
		t.Fatalf("mapped source review strict-wire roundtrip failed: %v", err)
	}
	for _, source := range manifest.CoverageItems[0].SourceObligationChecks {
		bad := cloneRuntimeSemanticPlanDocumentRaw(document)
		for n := range bad.Proposals[1].ValidationInputReview.Checks {
			check := &bad.Proposals[1].ValidationInputReview.Checks[n]
			if check.RequiredCheckRef == source.CheckRef {
				check.Obligations[0].CriterionText = manifest.CoverageItems[0].TargetText
			}
		}
		if strings.TrimSpace(source.Criterion) != manifest.CoverageItems[0].TargetText && bad.ValidateAgainstManifest(manifest) == nil {
			t.Fatal("consumer prose substituted original historical criterion in input review")
		}
	}
}

func TestEvidencePurposeAcceptanceChecksPreserveEveryOriginalObligation(t *testing.T) {
	i, _, items := evidencePurposeMappingFixture()
	before, _ := json.Marshal(i)
	history, err := CompileRuntimeSourceObligationChecks(i, items[0])
	if err != nil {
		t.Fatal(err)
	}
	current, err := CompileRuntimeSourceObligationChecks(i, items[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || len(current) != 3 {
		t.Fatal("ordinary consumer omitted original historical or own obligations")
	}
	for n, original := range history {
		consumer := current[n]
		if original.Criterion != consumer.Criterion || original.Declaration != consumer.Declaration ||
			original.ObligationID != consumer.ObligationID || original.VerificationID != consumer.VerificationID ||
			original.RequirementID != consumer.RequirementID || original.InventoryHash != consumer.InventoryHash ||
			original.CheckRef == consumer.CheckRef ||
			consumer.CheckRef != StableRuntimeSourceObligationCheckRef(i.DefinitionRevision, items[1].ID, consumer) {
			t.Fatal("original criterion/declaration lost, or historical check reused as consumer proof")
		}
	}
	var reload RuntimeSourceObligationInventory
	if err := json.Unmarshal(before, &reload); err != nil {
		t.Fatal(err)
	}
	recompiled, err := CompileRuntimeSourceObligationChecks(reload, items[1])
	if err != nil || !reflect.DeepEqual(current, recompiled) {
		t.Fatal("source checks changed across original inventory reload")
	}
	current[0].Criterion = "changed"
	after, _ := json.Marshal(i)
	if !slices.Equal(before, after) {
		t.Fatal("compiler aliases immutable source bytes")
	}
	legacy := i.Clone()
	legacy.EvidencePurposeMappings = nil
	legacy = legacy.WithHash()
	unchanged, err := CompileRuntimeSourceObligationChecks(legacy, items[1])
	if err != nil || len(unchanged) != 1 || unchanged[0].ObligationID != "C" {
		t.Fatal("unmapped source implicitly consumed predecessor checks")
	}
}

func TestEvidencePurposeAcceptanceCannotPassOnlySuccessorOrHistoricalChecks(t *testing.T) {
	i, _, items := evidencePurposeMappingFixture()
	current := items[1]
	checks, err := CompileRuntimeSourceObligationChecks(i, current)
	if err != nil {
		t.Fatal(err)
	}
	current.SourceObligationChecks = checks
	requirement := runtimeValidationProofRequirementForTest(t, i.DefinitionRevision, current, "", "mapped-recipe", nil)
	if len(requirement.RequiredCheckRefs) != 4 || len(requirement.SourceObligationChecks) != 3 {
		t.Fatal("source conjunction did not reach ordinary validation requirement")
	}
	attestations := runtimeValidationCheckAttestationsForTest(requirement.RequiredCheckRefs)
	proof := CoverageEvidenceProof{
		EvidenceKind: EvidenceKindValidation, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectEphemeralSession},
		AllowedOperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "mapped-recipe",
		ValidationProofRequirementHash: requirement.Hash, RequiredCheckRefs: requirement.RequiredCheckRefs,
		ValidationCheckAttestations: attestations,
	}
	if err := proof.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, omitted := range attestations {
		observation := RuntimeValidationObservation{RecipeID: "mapped-recipe", ObservationProtocolRef: omitted.ObservationProtocolRef}
		for _, attestation := range attestations {
			if attestation.RequiredCheckRef != omitted.RequiredCheckRef {
				observation.PassedChecks = append(observation.PassedChecks, attestation.NativeCheckRef)
			}
		}
		if RuntimeValidationObservationSatisfiesCoverageProof(proof, observation) {
			t.Fatalf("omitted original check %s still granted complete proof", omitted.RequiredCheckRef)
		}
		observation.PassedChecks = append(observation.PassedChecks, omitted.NativeCheckRef)
		if !RuntimeValidationObservationSatisfiesCoverageProof(proof, observation) {
			t.Fatal("complete original all-of proof rejected")
		}
	}
}

func TestEvidencePurposeAcceptanceSeedRejectsCoordinatedCheckSubstitution(t *testing.T) {
	_, _, seed := purposeEndpointFixture(t)
	for name, mutate := range map[string]func(*RuntimeDefinitionSeed){
		"omit history": func(s *RuntimeDefinitionSeed) {
			s.CoverageItems[1].SourceObligationChecks = s.CoverageItems[1].SourceObligationChecks[2:]
		},
		"omit own": func(s *RuntimeDefinitionSeed) {
			s.CoverageItems[1].SourceObligationChecks = s.CoverageItems[1].SourceObligationChecks[:2]
		},
		"substitute original criterion": func(s *RuntimeDefinitionSeed) {
			check := &s.CoverageItems[1].SourceObligationChecks[0]
			check.Criterion = s.CoverageItems[1].TargetText
			check.CheckRef = StableRuntimeSourceObligationCheckRef(s.DefinitionRevision, s.CoverageItems[1].ID, *check)
		},
		"reuse historical check": func(s *RuntimeDefinitionSeed) {
			s.CoverageItems[1].SourceObligationChecks[0] = s.CoverageItems[0].SourceObligationChecks[0]
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := seed.Clone()
			mutate(&changed)
			changed.Hash = StableRuntimeDefinitionSeedHash(changed)
			if ValidateRuntimeSourceObligationSeed(changed) == nil {
				t.Fatal("coordinated seed hash substituted original mapped obligations")
			}
		})
	}
	if err := ValidateRuntimeSourceObligationSeed(seed); err != nil {
		t.Fatal(err)
	}
}

func TestEvidencePurposeSharedObligationIsRequiredOnceAndManySourcesAllOf(t *testing.T) {
	i, _, items := evidencePurposeMappingFixture()
	// A belongs to both endpoints, but remains one original verification.
	i.Obligations[0].DoDItemRefs = append(i.Obligations[0].DoDItemRefs, "acceptance")
	i = i.WithHash()
	checks, err := CompileRuntimeSourceObligationChecks(i, items[1])
	if err != nil || len(checks) != 3 {
		t.Fatalf("shared original obligation duplicated or lost: %v", err)
	}
	// Another historical endpoint has its own immutable obligation and may
	// point at the same ordinary source, without selecting by runtime order.
	i.Sources = append(i.Sources, RuntimeDefinitionSource{CandidateItemRef: "second-history", ItemRef: "second-formal", SourceScope: "dodItem", SourceID: "dod-003", Text: "另一个原历史行为"})
	i.Obligations = append(i.Obligations, RuntimeSourceObligation{ID: "D", RequirementID: "requirement", DoDItemRefs: []string{"second-history"}, Text: " D 原始义务\n"})
	i.Verifications = append(i.Verifications, RuntimeSourceVerification{ID: "verify-D", ObligationID: "D", Declaration: "原始验证 D", ProfileID: "profile", EvidenceRequirementID: "dod-003", Scope: "dodItem"})
	i.EvidencePurposeMappings = append(i.EvidencePurposeMappings, RuntimeEvidencePurposeMapping{ContractRef: RuntimeCrossCWIEvidencePurpose, HistoricalItemRef: "second-history", AcceptanceItemRef: "acceptance"})
	i = i.WithHash()
	checks, err = CompileRuntimeSourceObligationChecks(i, items[1])
	if err != nil || len(checks) != 4 || checks[3].Criterion != " D 原始义务\n" {
		t.Fatalf("confirmed many-source conjunction lost original bytes: %v", err)
	}
}
