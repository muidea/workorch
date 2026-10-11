package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

// 原完整 manifest 规范化路径独立保留；不调用新的 normalized 或 validated helper。
func referenceSemanticProofObligationValidation(document RuntimeSemanticPlanDocument, manifest RuntimeSemanticPlanManifest) error {
	document = document.Normalized()
	manifest = manifest.Normalized()
	obligations := make(map[common.CoverageWorkItemID]RuntimeSemanticProofObligation, len(manifest.ProofObligations))
	for _, obligation := range manifest.ProofObligations {
		obligations[obligation.CoverageWorkItemID] = obligation
	}
	proofSets, err := OrderedRuntimeSemanticCWIProofSets(document)
	if err != nil {
		return err
	}
	for _, proofSet := range proofSets {
		obligation, found := obligations[proofSet.CoverageWorkItemID]
		if !found {
			return fmt.Errorf("runtime semantic proof for coverage %q has no sealed obligation", proofSet.CoverageWorkItemID)
		}
		evidenceObligation := resolveValidatedRuntimeSemanticEvidenceObligation(obligation)
		sourceValue := runtimeSemanticProofObligationSourceFocus(obligation)
		source := &sourceValue
		if obligation.ImplementationWork != nil {
			if err := validateRuntimeSemanticImplementationWorkProofSet(proofSet, obligation.ImplementationWork); err != nil {
				return err
			}
		} else if err := validateRuntimeSemanticProofSetSourceWithEvidenceKinds(proofSet, source, evidenceObligation.CurrentRequiredEvidenceKinds); err != nil {
			return err
		}
		currentDeclarationRef := RuntimeSemanticCurrentDeclarationRef(obligation)
		for proposalIndex, rawProposal := range proofSet.Proposals {
			proposal := rawProposal.Normalized()
			if proposal.DeclarationRef != currentDeclarationRef {
				return runtimeSemanticProposalViolation(proposal, proposalIndex, "declarationRef", RuntimeSemanticConstraintAllowedValue,
					proposal.DeclarationRef, nil, []string{currentDeclarationRef}, fmt.Errorf("runtime semantic proof declarationRef is not the sealed CWI declaration"))
			}
			if len(proposal.RequiredEvidenceKinds) != 1 || proposal.RequiredEvidenceKinds[0] != EvidenceKindValidation {
				continue
			}
			if proposal.OperationKind == RuntimeBlockKindManagedValidationSession {
				if obligation.ValidationProofRequirement == nil {
					return runtimeSemanticProposalViolation(proposal, proposalIndex, "validationRecipeId", RuntimeSemanticConstraintRelational,
						proposal.ValidationRecipeID, []string{"exact CWI validation proof requirement"}, nil, fmt.Errorf("managed validation cannot prove a CWI without an exact requirement"))
				}
				requirement := obligation.ValidationProofRequirement.Normalized()
				if requirement.ValidationRecipeID != "" && proposal.ValidationRecipeID != requirement.ValidationRecipeID {
					return runtimeSemanticProposalViolation(proposal, proposalIndex, "validationRecipeId", RuntimeSemanticConstraintAllowedValue,
						proposal.ValidationRecipeID, nil, []string{requirement.ValidationRecipeID}, fmt.Errorf("managed validation recipe does not match the sealed CWI requirement"))
				}
				catalog, found := runtimeSemanticManifestCatalogForOwner(manifest, proposal.OwnerModuleID)
				if !found {
					return runtimeSemanticProposalViolation(proposal, proposalIndex, "ownerModuleId", RuntimeSemanticConstraintAllowedValue,
						proposal.OwnerModuleID, nil, manifest.AllowedOwnerModuleIDs, fmt.Errorf("managed validation has no sealed owner catalog"))
				}
				surface, found := RuntimeValidationSurfaceForID(catalog.ValidationSurfaces, proposal.ValidationRecipeID)
				if !found {
					return runtimeSemanticProposalViolation(proposal, proposalIndex, "validationRecipeId", RuntimeSemanticConstraintAllowedValue,
						proposal.ValidationRecipeID, nil, RuntimeSelectableValidationRecipeRefsForProofRequirement(catalog.ValidationSurfaces, requirement), fmt.Errorf("managed validation recipe has no owner surface"))
				}
				if _, _, exact := RuntimeValidationSurfaceForProofRequirement([]RuntimeValidationSurface{surface}, requirement); !exact {
					return runtimeSemanticProposalViolation(proposal, proposalIndex, "validationRecipeId", RuntimeSemanticConstraintAllowedValue,
						proposal.ValidationRecipeID, nil, RuntimeSelectableValidationRecipeRefsForProofRequirement(catalog.ValidationSurfaces, requirement), fmt.Errorf("managed validation recipe is not bound to the exact CWI requirement"))
				}
			}
		}
		proposalIndex := -1
		for index, proposal := range document.Proposals {
			if proposal.CoverageWorkItemID == proofSet.CoverageWorkItemID {
				proposalIndex = index
				break
			}
		}
		if err := validateRuntimeSemanticOwnerFactAcquisition(proofSet, obligation.OwnerFactAcquisition, proposalIndex); err != nil {
			return err
		}
		if err := validateRuntimeSemanticValidationFailureRepair(proofSet, obligation.ValidationFailureRepair, proposalIndex); err != nil {
			return err
		}
	}
	return nil
}

func assertSemanticProofObligationValidationEquivalent(t testing.TB, document RuntimeSemanticPlanDocument, validated runtimeValidatedSemanticManifest) {
	t.Helper()
	before, err := json.Marshal(validated.manifest)
	if err != nil {
		t.Fatal(err)
	}
	documentBefore, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	want := cwiFocusBindingsError(referenceSemanticProofObligationValidation(document, validated.manifest))
	for range 2 {
		got := cwiFocusBindingsError(validateRuntimeSemanticPlanProofObligationsWithValidatedManifest(document, validated))
		if got != want || cwiFocusBindingsError(validateRuntimeSemanticPlanProofObligations(document, validated.manifest)) != want {
			t.Fatalf("proof obligation validation differs: %q != %q", got, want)
		}
	}
	after, _ := json.Marshal(validated.manifest)
	documentAfter, _ := json.Marshal(document)
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(documentBefore, documentAfter) {
		t.Fatal("read-only obligation validation changed input")
	}
}

func TestSemanticProofObligationValidatedManifestPreservesOriginalGates(t *testing.T) {
	fixtures := map[string]RuntimeSemanticPlanningLedger{"multi_batch": semanticBindingIndexedMultiBatchFixture(t, 4)}
	fixtures["definition"], _ = assessmentPurposeGenerationForTest(t, "cwi-source", "source-request", false)
	fixtures["repair"], _ = assessmentPurposeGenerationForTest(t, "cwi-repair", "repair-request", true)
	manifest, document, facts, _ := semanticIndependentProofSetFixture(t)
	fixtures["independent"] = semanticBindingIndexedDocumentFixture(manifest, document, facts)
	manifest, document, proposal := semanticMissingOwnerFactAcquisitionFixture(t)
	fixtures["acquisition"] = semanticBindingIndexedDocumentFixture(manifest, document, []RuntimeSemanticProofFeasibility{semanticFeasibilityForTest(t, manifest, proposal, RuntimeSemanticProposalTargetRefs(proposal), "", nil)})
	for name, ledger := range fixtures {
		t.Run(name, func(t *testing.T) {
			validated, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			for _, original := range ledger.BatchDocuments {
				if err := validateRuntimeSemanticPlanProofObligationsWithValidatedManifest(original, validated); err != nil {
					t.Fatal(err)
				}
				for _, scenario := range []string{"valid", "declaration", "coverage", "required_kind", "owner", "target", "duplicate", "raw_trim", "recipe"} {
					t.Run(scenario, func(t *testing.T) {
						changed := original.Clone()
						switch scenario {
						case "declaration":
							changed.Proposals[0].DeclarationRef = "wrong"
						case "coverage":
							changed.Proposals[0].CoverageWorkItemID = "wrong"
						case "required_kind":
							changed.Proposals[0].RequiredEvidenceKinds = []string{EvidenceKindArtifact}
						case "owner":
							changed.Proposals[0].OwnerModuleID = "other"
						case "target":
							changed.Proposals[0].Target.ExactTargetRefs = []string{"other"}
						case "duplicate":
							changed.Proposals = append(changed.Proposals, changed.Proposals[0])
						case "raw_trim":
							changed.Proposals[0].DeclarationRef = " " + changed.Proposals[0].DeclarationRef + " "
						case "recipe":
							changed.Proposals[0].ValidationRecipeID = "other"
						}
						assertSemanticProofObligationValidationEquivalent(t, changed, validated)
						body, _ := json.Marshal(changed)
						var reloaded RuntimeSemanticPlanDocument
						if err := json.Unmarshal(body, &reloaded); err != nil {
							t.Fatal(err)
						}
						assertSemanticProofObligationValidationEquivalent(t, reloaded, validated)
					})
				}
				changed := original.Clone()
				changed.Proposals[0].DeclarationRef = "later-invalid"
				if validateRuntimeSemanticPlanProofObligationsWithValidatedManifest(changed, validated) == nil {
					t.Fatal("later call reused document success")
				}
			}
			changed := ledger.Manifest.Clone()
			changed.Hash = "later-invalid"
			if _, err := validateRuntimeSemanticPlanManifest(changed); err == nil {
				t.Fatal("later manifest call reused success")
			}
		})
	}
}

func TestSemanticProofObligationValidatedManifestReducesAllocations(t *testing.T) {
	ledger := semanticBindingIndexedMultiBatchFixture(t, 8)
	validated, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	old := testing.AllocsPerRun(10, func() { _ = referenceSemanticProofObligationValidation(ledger.BatchDocuments[0], validated.manifest) })
	current := testing.AllocsPerRun(10, func() {
		_ = validateRuntimeSemanticPlanProofObligationsWithValidatedManifest(ledger.BatchDocuments[0], validated)
	})
	if current >= old {
		t.Fatalf("same-call manifest reuse did not reduce allocations: %f >= %f", current, old)
	}
}

func BenchmarkSemanticProofObligationValidatedManifest(b *testing.B) {
	ledger := semanticBindingIndexedMultiBatchFixture(b, 8)
	validated, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
	if err != nil {
		b.Fatal(err)
	}
	for _, reference := range []bool{false, true} {
		name := "same_call"
		if reference {
			name = "reference"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var err error
				if reference {
					err = referenceSemanticProofObligationValidation(ledger.BatchDocuments[0], validated.manifest)
				} else {
					err = validateRuntimeSemanticPlanProofObligationsWithValidatedManifest(ledger.BatchDocuments[0], validated)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func FuzzSemanticProofObligationValidatedManifestMatchesOriginal(f *testing.F) {
	ledger := semanticBindingIndexedMultiBatchFixture(f, 2)
	validated, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
	if err != nil {
		f.Fatal(err)
	}
	for _, value := range []string{"", " 原文\r\n", "\x00\xff"} {
		f.Add(value, uint8(0))
	}
	f.Fuzz(func(t *testing.T, value string, mode uint8) {
		document := ledger.BatchDocuments[0].Clone()
		switch mode % 4 {
		case 0:
			document.Proposals[0].DeclarationRef = value
		case 1:
			document.Proposals[0].CoverageWorkItemID = common.CoverageWorkItemID(value)
		case 2:
			document.Proposals[0].RequiredEvidenceKinds = []string{value}
		case 3:
			document.Proposals[0].Target.ExactTargetRefs = []string{value}
		}
		assertSemanticProofObligationValidationEquivalent(t, document, validated)
	})
}
