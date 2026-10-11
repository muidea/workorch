package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Reference 保留 858f7681 原 accepted compiler、全 manifest map/source 重建及
// focus compiler 的 manifest.Normalized；不能改为调用待验证的 production compiler。
func TestSemanticBindingIndexedCompilerMatchesIndependentReference(t *testing.T) {
	fixtures := map[string]RuntimeSemanticPlanningLedger{
		"multi_batch": semanticBindingIndexedMultiBatchFixture(t, 8),
	}
	fixtures["definition"], _ = assessmentPurposeGenerationForTest(t, "cwi-source", "source-request", false)
	fixtures["repair"], _ = assessmentPurposeGenerationForTest(t, "cwi-repair", "repair-request", true)
	manifest, document, facts, _ := semanticIndependentProofSetFixture(t)
	fixtures["independent_proofs"] = semanticBindingIndexedDocumentFixture(manifest, document, facts)
	manifest, document, proposal := semanticMissingOwnerFactAcquisitionFixture(t)
	facts = []RuntimeSemanticProofFeasibility{semanticFeasibilityForTest(t, manifest, proposal, RuntimeSemanticProposalTargetRefs(proposal), "", nil)}
	fixtures["acquisition"] = semanticBindingIndexedDocumentFixture(manifest, document, facts)
	for name, ledger := range fixtures {
		t.Run(name, func(t *testing.T) {
			before, _ := json.Marshal(ledger)
			for index, batch := range ledger.Batches {
				document := ledger.BatchDocuments[index]
				facts := ledger.Feasibilities
				if len(ledger.BatchFeasibilityResults) != 0 {
					facts = ledger.BatchFeasibilityResults[index].Feasibilities
				}
				semanticBindingIndexedCompareReference(t, *ledger.Manifest, batch, document, facts)
				validated, err := validateRuntimeSemanticPlanManifest(*ledger.Manifest)
				if err != nil {
					t.Fatal(err)
				}
				validatedBefore, _ := json.Marshal(validated.manifest)
				got, contract, err := compileRuntimeSemanticBindingsWithValidatedManifest(validated, batch.RequestLogicalID, document, facts)
				if err != nil {
					t.Fatal(err)
				}
				want, wantContract, err := semanticBindingIndexedReferenceCompiler(validated, batch.RequestLogicalID, document, facts)
				if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(contract, wantContract) {
					t.Fatalf("私有 compiler 产物不等价: %v", err)
				}
				// 改写 returned focus/binding 后，同一 validated maps 仍应完全不变。
				got[0].CoverageWorkItemIDs[0] = "changed"
				if len(got[0].CoverageEvidenceBindings[0].TargetRefs) != 0 {
					got[0].CoverageEvidenceBindings[0].TargetRefs[0] = "changed"
				}
				if len(got[0].CoverageEvidenceBindings[0].ProspectiveTargetRefs) != 0 {
					got[0].CoverageEvidenceBindings[0].ProspectiveTargetRefs[0] = "changed"
				}
				got[0].CoverageEvidenceBindings[0].Proofs[0].AllowedOperationKinds[0] = "changed"
				got[0].RequiredEvidenceKinds[0] = "changed"
				validatedAfter, _ := json.Marshal(validated.manifest)
				if string(validatedBefore) != string(validatedAfter) {
					t.Fatal("返回值共享 validated manifest 来源")
				}
				again, againContract, err := compileRuntimeSemanticBindingsWithValidatedManifest(validated, batch.RequestLogicalID, document, facts)
				if err != nil || !reflect.DeepEqual(again, want) || !reflect.DeepEqual(againContract, wantContract) {
					t.Fatalf("returned focus/binding 污染 validated maps 或后续调用: %v", err)
				}
			}
			after, _ := json.Marshal(ledger)
			if string(before) != string(after) {
				t.Fatal("编译或输出写入原始 ledger JSON")
			}
			var reloaded RuntimeSemanticPlanningLedger
			if err := json.Unmarshal(before, &reloaded); err != nil {
				t.Fatal(err)
			}
			facts := reloaded.Feasibilities
			if len(reloaded.BatchFeasibilityResults) != 0 {
				facts = reloaded.BatchFeasibilityResults[0].Feasibilities
			}
			semanticBindingIndexedCompareReference(t, *reloaded.Manifest, reloaded.Batches[0], reloaded.BatchDocuments[0], facts)
		})
	}
}

func semanticBindingIndexedDocumentFixture(manifest RuntimeSemanticPlanManifest, document RuntimeSemanticPlanDocument, facts []RuntimeSemanticProofFeasibility) RuntimeSemanticPlanningLedger {
	batch := RuntimeSemanticPlanBatch{BatchID: document.BatchID, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageOrderHash: "reference-order", Sequence: document.BatchSequence, CoverageWorkItemIDs: document.BatchCoverageWorkItemIDs, RequestLogicalID: "reference-request", DocumentHash: StableRuntimeSemanticPlanDocumentHash(document), State: RuntimeSemanticPlanBatchAccepted}
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
	return RuntimeSemanticPlanningLedger{Manifest: &manifest, Batches: []RuntimeSemanticPlanBatch{batch}, BatchDocuments: []RuntimeSemanticPlanDocument{document}, Feasibilities: facts}
}

func semanticBindingIndexedCompareReference(t *testing.T, manifest RuntimeSemanticPlanManifest, batch RuntimeSemanticPlanBatch, document RuntimeSemanticPlanDocument, facts []RuntimeSemanticProofFeasibility) {
	t.Helper()
	want, wantContract, wantErr := semanticBindingIndexedReferenceAcceptedBatch(manifest, batch, document, facts)
	got, gotContract, gotErr := CompileRuntimeSemanticAcceptedBatchBindings(manifest, batch, document, facts)
	if (gotErr == nil) != (wantErr == nil) || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotContract, wantContract) {
		t.Fatalf("indexed compiler 与独立858 reference 接受/产物不等价: got=%v want=%v", gotErr, wantErr)
	}
	if gotErr != nil && gotErr.Error() != wantErr.Error() {
		t.Fatalf("indexed compiler 拒绝原因漂移: got=%v want=%v", gotErr, wantErr)
	}
}

func TestSemanticBindingIndexedCompilerRetainsRawBatchDocumentAndOwnerGates(t *testing.T) {
	base := semanticBindingIndexedMultiBatchFixture(t, 4)
	for _, scenario := range []string{"raw_catalog_duplicate", "raw_manifest_coverage_duplicate", "raw_batch_duplicate", "batch_hash", "batch_request", "batch_pending", "document_manifest", "document_coverage_duplicate", "document_proposal_duplicate", "document_sequence", "document_declaration", "owner", "owner_snapshot", "owner_target", "missing_owner", "duplicate_owner", "infeasible_owner"} {
		t.Run(scenario, func(t *testing.T) {
			encoded, _ := json.Marshal(base)
			var ledger RuntimeSemanticPlanningLedger
			if err := json.Unmarshal(encoded, &ledger); err != nil {
				t.Fatal(err)
			}
			manifest, batch, document := ledger.Manifest, &ledger.Batches[0], &ledger.BatchDocuments[0]
			facts := ledger.BatchFeasibilityResults[0].Feasibilities
			switch scenario {
			case "raw_catalog_duplicate":
				manifest.OwnerCatalogs = append(manifest.OwnerCatalogs, manifest.OwnerCatalogs[0])
			case "raw_manifest_coverage_duplicate":
				manifest.CoverageItems = append(manifest.CoverageItems, manifest.CoverageItems[0])
			case "raw_batch_duplicate":
				batch.CoverageWorkItemIDs = append(batch.CoverageWorkItemIDs, batch.CoverageWorkItemIDs[0])
			case "batch_hash":
				batch.Hash = "drift"
			case "batch_request":
				batch.RequestLogicalID = ""
			case "batch_pending":
				batch.PendingCoverageWorkItemIDs = append(batch.PendingCoverageWorkItemIDs, batch.CoverageWorkItemIDs[0])
			case "document_manifest":
				document.ManifestHash = "drift"
			case "document_coverage_duplicate":
				document.BatchCoverageWorkItemIDs = append(document.BatchCoverageWorkItemIDs, document.BatchCoverageWorkItemIDs[0])
			case "document_proposal_duplicate":
				document.Proposals = append(document.Proposals, document.Proposals[0])
			case "document_sequence":
				document.BatchSequence++
			case "document_declaration":
				document.Proposals[0].DeclarationRef = "drift"
			case "owner":
				facts[0].OwnerModuleID = RuntimeModuleShell
			case "owner_snapshot":
				facts[0].CatalogSnapshotID = "drift"
			case "owner_target":
				facts[0].TargetRefs = []string{"drift"}
			case "missing_owner":
				facts = nil
			case "duplicate_owner":
				facts = append(facts, facts[0])
			case "infeasible_owner":
				facts[0].Status = RuntimeSemanticFeasibilityInfeasible
			}
			if scenario != "batch_hash" {
				batch.Hash = StableRuntimeSemanticPlanBatchHash(*batch)
			}
			semanticBindingIndexedCompareReference(t, *manifest, *batch, *document, facts)
			if _, _, err := CompileRuntimeSemanticAcceptedBatchBindings(*manifest, *batch, *document, facts); err == nil {
				t.Fatal("协调篡改获得接受")
			}
		})
	}
}

func BenchmarkSemanticBindingIndexedCompilerAgainstReference(b *testing.B) {
	for _, count := range []int{1, 4, 8} {
		b.Run(fmt.Sprintf("batches_%d", count), func(b *testing.B) {
			ledger := semanticBindingIndexedMultiBatchFixture(b, count)
			for _, mode := range []string{"reference_858", "indexed"} {
				b.Run(mode, func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						for index, batch := range ledger.Batches {
							var err error
							if mode == "reference_858" {
								_, _, err = semanticBindingIndexedReferenceAcceptedBatch(*ledger.Manifest, batch, ledger.BatchDocuments[index], ledger.BatchFeasibilityResults[index].Feasibilities)
							} else {
								_, _, err = CompileRuntimeSemanticAcceptedBatchBindings(*ledger.Manifest, batch, ledger.BatchDocuments[index], ledger.BatchFeasibilityResults[index].Feasibilities)
							}
							if err != nil {
								b.Fatal(err)
							}
						}
					}
				})
			}
		})
	}
}

func semanticBindingIndexedReferenceAcceptedBatch(manifest RuntimeSemanticPlanManifest, batch RuntimeSemanticPlanBatch, document RuntimeSemanticPlanDocument, feasibilities []RuntimeSemanticProofFeasibility) ([]CoverageFocus, RuntimeCWIReconciliationContract, error) {
	validated, err := validateRuntimeSemanticPlanManifest(manifest)
	if err != nil {
		return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding batch: %w", err)
	}
	if err := validateRuntimeSemanticPlanBatchAgainstValidatedManifest(batch, validated); err != nil {
		return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding batch: %w", err)
	}
	batch = batch.Normalized()
	documentHash := StableRuntimeSemanticPlanDocumentHash(document)
	if batch.State != RuntimeSemanticPlanBatchAccepted || batch.DocumentHash != documentHash ||
		strings.TrimSpace(document.BatchID) != batch.BatchID || document.BatchSequence != batch.Sequence {
		return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding requires the exact accepted batch document")
	}
	return semanticBindingIndexedReferenceCompiler(validated, batch.RequestLogicalID, document, feasibilities)
}

func semanticBindingIndexedReferenceCompiler(validated runtimeValidatedSemanticManifest, logicalRequestID string, document RuntimeSemanticPlanDocument, feasibilities []RuntimeSemanticProofFeasibility) ([]CoverageFocus, RuntimeCWIReconciliationContract, error) {
	manifest := validated.manifest
	generationID := StableRuntimeSemanticGenerationID(manifest.Hash, logicalRequestID)
	if generationID == "" {
		return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding requires manifest and logical-request generation identity")
	}
	if err := validateRuntimeSemanticPlanDocumentAgainstValidatedManifest(document, validated); err != nil {
		return nil, RuntimeCWIReconciliationContract{}, err
	}
	document = document.Normalized()
	if err := ValidateRuntimeSemanticInputReviewConsistency(document); err != nil {
		return nil, RuntimeCWIReconciliationContract{}, err
	}
	if err := validateRuntimeSemanticFeasibilitySetWithValidatedManifest(validated, document, feasibilities, true); err != nil {
		return nil, RuntimeCWIReconciliationContract{}, err
	}
	sourceByCoverage := make(map[common.CoverageWorkItemID]CoverageFocus, len(manifest.ProofObligations))
	obligationByCoverage := make(map[common.CoverageWorkItemID]RuntimeSemanticProofObligation, len(manifest.ProofObligations))
	for _, obligation := range manifest.ProofObligations {
		sourceByCoverage[obligation.CoverageWorkItemID] = runtimeSemanticProofObligationSourceFocus(obligation)
		obligationByCoverage[obligation.CoverageWorkItemID] = obligation.Clone()
	}
	items := make(map[string]RuntimeCoverageItem, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		items[string(item.ID)] = item
	}
	facts := make(map[string]RuntimeSemanticProofFeasibility, len(feasibilities))
	for _, fact := range feasibilities {
		fact = fact.Clone()
		if fact.Status != RuntimeSemanticFeasibilityFeasible {
			return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding requires a complete feasible owner batch")
		}
		facts[fact.ProposalHash] = fact
	}
	proofSets, err := OrderedRuntimeSemanticCWIProofSets(document)
	if err != nil {
		return nil, RuntimeCWIReconciliationContract{}, err
	}
	if err := validateRuntimeSemanticPlanProofObligations(document, manifest); err != nil {
		return nil, RuntimeCWIReconciliationContract{}, err
	}
	focuses := make([]CoverageFocus, 0, len(proofSets))
	for _, proofSet := range proofSets {
		item, found := items[string(proofSet.CoverageWorkItemID)]
		if !found {
			return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding references unknown coverage %q", proofSet.CoverageWorkItemID)
		}
		proofFacts := make(map[string]RuntimeSemanticProofFeasibility, len(proofSet.Proposals))
		for _, proposal := range proofSet.Proposals {
			proposalHash := StableRuntimeSemanticProofProposalHash(proposal)
			fact, found := facts[proposalHash]
			if !found {
				return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding lacks owner fact for proof %q", proposalHash)
			}
			proofFacts[proposalHash] = fact
		}
		var source *CoverageFocus
		if value, found := sourceByCoverage[proofSet.CoverageWorkItemID]; found {
			value := value
			source = &value
		}
		obligation, found := obligationByCoverage[proofSet.CoverageWorkItemID]
		if !found {
			return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("runtime semantic binding has no proof obligation for coverage %q", proofSet.CoverageWorkItemID)
		}
		evidenceObligation := resolveValidatedRuntimeSemanticEvidenceObligation(obligation)
		if obligation.ImplementationWork != nil {
			return nil, RuntimeCWIReconciliationContract{}, fmt.Errorf("implementation work cannot compile terminal coverage bindings")
		}
		if err := validateRuntimeSemanticProofSetSourceWithEvidenceKinds(proofSet, source, evidenceObligation.CurrentRequiredEvidenceKinds); err != nil {
			return nil, RuntimeCWIReconciliationContract{}, err
		}
		focus, err := semanticBindingIndexedReferenceFocus(manifest, generationID, item, proofSet, proofFacts, source, evidenceObligation)
		if err != nil {
			return nil, RuntimeCWIReconciliationContract{}, err
		}
		focuses = append(focuses, focus)
	}
	// Do not normalize/sort this queue: batch membership is the persisted
	// provider order constrained by the active focus dependency closure.
	// Sorting by CWI ID here would silently replace that semantic order.
	contract, err := CompileRuntimeCWIReconciliationContract(manifest.DefinitionRevision, focuses)
	if err != nil {
		return nil, RuntimeCWIReconciliationContract{}, err
	}
	return focuses, contract, nil
}

func semanticBindingIndexedReferenceFocus(manifest RuntimeSemanticPlanManifest, generationID string, item RuntimeCoverageItem, proofSet RuntimeSemanticCWIProofSet, facts map[string]RuntimeSemanticProofFeasibility, source *CoverageFocus, evidenceObligation RuntimeSemanticEvidenceObligation) (CoverageFocus, error) {
	manifest = manifest.Normalized()
	proofSetHash := StableRuntimeSemanticCWIProofSetHash(proofSet.CoverageWorkItemID, proofSet.Proposals)
	idSuffix := strings.TrimPrefix(proofSetHash, "runtime-semantic-cwi-proof-set-")
	generationSuffix := strings.TrimPrefix(strings.TrimSpace(generationID), "runtime-semantic-generation-")
	if generationSuffix == "" {
		return CoverageFocus{}, fmt.Errorf("runtime semantic focus requires an exact semantic generation identity")
	}
	// The proposal set is CWI-scoped. A generation-specific focus/binding ID
	// therefore cannot be reused after catalog re-acquisition, even when every
	// provider proof text happens to be identical.
	focusID := "semantic-focus-" + idSuffix + "-" + generationSuffix
	bindingID := "semantic-binding-" + idSuffix + "-" + generationSuffix

	proofs := make([]CoverageEvidenceProof, 0, len(proofSet.Proposals))
	acceptedKinds := make([]string, 0, len(proofSet.Proposals))
	acceptedVariants := make([]RuntimeOperationVariantRef, 0, len(proofSet.Proposals))
	prerequisiteRefs := make([]string, 0, len(proofSet.Proposals)*2)
	var targetRefs []string
	var prospectiveTargetRefs []string
	var selectorRef, declarationRef, selectorOwner string
	var validationRecipeID string
	validationChecks := make([]string, 0)
	var validationRepair *RuntimeSemanticValidationFailureRepair
	var validationProofRequirement *RuntimeValidationProofRequirement
	for _, obligation := range manifest.ProofObligations {
		if obligation.CoverageWorkItemID == proofSet.CoverageWorkItemID {
			if obligation.ValidationFailureRepair != nil {
				value := obligation.ValidationFailureRepair.Clone()
				validationRepair = &value
			}
			if obligation.ValidationProofRequirement != nil {
				value := obligation.ValidationProofRequirement.Clone()
				validationProofRequirement = &value
			}
			break
		}
	}
	proofRequirementIndex := -1
	if evidenceObligation.Purpose == RuntimeSemanticEvidenceObligationDefinitionProof {
		var err error
		proofRequirementIndex, err = runtimeSemanticSourceProofRequirementIndex(proofSet, source)
		if err != nil {
			return CoverageFocus{}, err
		}
	}
	for proofIndex, rawProposal := range proofSet.Proposals {
		proposal := rawProposal.Normalized()
		proposal.RequiredEffectScopes = deriveRuntimeSemanticProposalEffects(manifest, runtimeSemanticProposalPayload{
			OwnerModuleID: proposal.OwnerModuleID, OperationKind: proposal.OperationKind,
			OperationVariantID: proposal.OperationVariantID, RequiredEvidenceKinds: proposal.RequiredEvidenceKinds,
			ValidationRecipeID: proposal.ValidationRecipeID,
		})
		proposalHash := StableRuntimeSemanticProofProposalHash(rawProposal)
		fact, found := facts[proposalHash]
		if !found {
			return CoverageFocus{}, fmt.Errorf("runtime semantic proof set for coverage %q lacks fact %q", proofSet.CoverageWorkItemID, proposalHash)
		}
		if validationRepair != nil && proposal.Target.Policy == RuntimeOperationTargetPolicyProspectiveOwnerTarget &&
			!sameStrings(fact.TargetRefs, proposal.Target.ProspectiveTargetRefs) {
			return CoverageFocus{}, fmt.Errorf("runtime semantic validation repair prospective target lacks exact capability-owner confirmation")
		}
		evidenceKind := proposal.RequiredEvidenceKinds[0]
		proof := CoverageEvidenceProof{
			EvidenceKind: evidenceKind, RequiredEffectScopes: semanticProofEffectsForEvidenceKind(proposal.RequiredEffectScopes, evidenceKind),
			AllowedOperationKinds: []string{proposal.OperationKind},
		}
		if source != nil && proofIndex == proofRequirementIndex {
			declaration := source.Normalized()
			proof.MetricConstraints = append([]string(nil), declaration.MetricConstraints...)
			proof.ArtifactRefs = append([]string(nil), declaration.ArtifactRefs...)
		}
		if proposal.OperationVariantID != "" {
			proof.AllowedOperationVariants = []RuntimeOperationVariantRef{{Kind: proposal.OperationKind, VariantID: proposal.OperationVariantID}}
			acceptedVariants = append(acceptedVariants, proof.AllowedOperationVariants[0])
		}
		if evidenceKind == EvidenceKindValidation {
			proof.ValidationRecipeID = fact.ValidationRecipeID
			proof.RequiredCheckRefs = append([]string(nil), fact.RequiredCheckRefs...)
			if proposal.OperationKind == RuntimeBlockKindManagedValidationSession {
				if validationProofRequirement == nil {
					return CoverageFocus{}, fmt.Errorf("managed validation proof for coverage %q lacks exact proof requirement", proofSet.CoverageWorkItemID)
				}
				proof.ValidationProofRequirementHash = validationProofRequirement.Hash
				attestations, attestationErr := runtimeSemanticValidationCheckAttestations(manifest, fact, *validationProofRequirement)
				if attestationErr != nil {
					return CoverageFocus{}, fmt.Errorf("managed validation proof for coverage %q: %w", proofSet.CoverageWorkItemID, attestationErr)
				}
				proof.ValidationCheckAttestations = attestations
			}
			validationRecipeID = fact.ValidationRecipeID
			validationChecks = append(validationChecks, fact.RequiredCheckRefs...)
		}
		if err := proof.Validate(); err != nil {
			return CoverageFocus{}, fmt.Errorf("runtime semantic proof %q for coverage %q: %w", proposalHash, proofSet.CoverageWorkItemID, err)
		}
		currentTargets, currentSelector, currentProspective, err := semanticBindingTarget(proposal, fact)
		if err != nil {
			return CoverageFocus{}, fmt.Errorf("runtime semantic proof %q target: %w", proposalHash, err)
		}
		currentSelectorOwner := proposal.OwnerModuleID
		if source != nil && currentSelector != "" {
			declaration := source.Normalized()
			if declaration.TargetSelectorRef == currentSelector && declaration.TargetSelectorOwnerRef != "" {
				currentSelectorOwner = declaration.TargetSelectorOwnerRef
			}
		}
		if proofIndex == 0 {
			targetRefs, selectorRef, prospectiveTargetRefs = currentTargets, currentSelector, currentProspective
			declarationRef, selectorOwner = proposal.DeclarationRef, currentSelectorOwner
		} else {
			if declarationRef != proposal.DeclarationRef {
				return CoverageFocus{}, fmt.Errorf("runtime semantic proofs for coverage %q do not share one target and declaration surface", proofSet.CoverageWorkItemID)
			}
			targetRefs, selectorRef, prospectiveTargetRefs, selectorOwner, err = mergeRuntimeSemanticBindingTarget(
				targetRefs, selectorRef, prospectiveTargetRefs, selectorOwner,
				currentTargets, currentSelector, currentProspective, currentSelectorOwner,
			)
			if err != nil {
				return CoverageFocus{}, fmt.Errorf("runtime semantic proofs for coverage %q do not share one target and declaration surface", proofSet.CoverageWorkItemID)
			}
		}
		acceptedKinds = append(acceptedKinds, proposal.OperationKind)
		prerequisiteRefs = append(prerequisiteRefs, fact.CatalogSnapshotID)
		prerequisiteRefs = append(prerequisiteRefs, "semantic-feasibility/"+strings.TrimPrefix(StableRuntimeSemanticFeasibilitySetHash([]RuntimeSemanticProofFeasibility{fact}), "runtime-semantic-feasibility-set-"))
		proofs = append(proofs, proof)
	}
	binding := CoverageEvidenceBinding{
		ID: bindingID, CoverageWorkItemID: proofSet.CoverageWorkItemID, FocusID: focusID, DefinitionRevision: manifest.DefinitionRevision,
		Proofs: proofs, TargetRefs: targetRefs, TargetSelectorRef: selectorRef, DeclarationRef: declarationRef,
		ProspectiveTargetRefs: prospectiveTargetRefs, PrerequisiteRefs: prerequisiteRefs,
	}
	if evidenceObligation.Purpose == RuntimeSemanticEvidenceObligationDefinitionProof {
		for _, obligation := range manifest.ProofObligations {
			if obligation.CoverageWorkItemID != item.ID {
				continue
			}
			var err error
			binding.LineageAssertions, err = CompileRuntimeLineageAssertions(obligation.LineageAssertions, declarationRef, item)
			if err != nil {
				return CoverageFocus{}, err
			}
		}
	}
	if source != nil {
		binding.VerificationPolicy = cloneRuntimeVerificationPolicy(source.VerificationPolicy)
	}
	if validationRepair != nil {
		repair := validationRepair.Normalized()
		binding.PrerequisiteRefs = append(binding.PrerequisiteRefs,
			"semantic-purpose/"+string(RuntimeSemanticEvidenceObligationValidationFailureRepair),
			"validation-repair-convergence/"+repair.ConvergenceHash,
			"validation-repair-failure/"+repair.FailureSignature,
			"validation-repair-failed-focus/"+repair.FailedFocusID,
			"validation-repair-owner-catalog/"+repair.OwnerCatalogSnapshotID,
		)
	}
	if selectorRef != "" {
		binding.TargetSelectorOwnerRef = selectorOwner
		binding.TargetSelectorSubjectRef = "workspace_deliverable"
		if source != nil {
			declaration := source.Normalized()
			if declaration.TargetSelectorRef == selectorRef {
				binding.TargetSelectorOwnerRef = declaration.TargetSelectorOwnerRef
				binding.TargetSelectorSubjectRef = declaration.TargetSelectorSubjectRef
				binding.TargetSelectorCandidateRefs = append([]string(nil), declaration.TargetSelectorCandidateRefs...)
				binding.ProspectiveTargetAllOfConstraintRefs = append([]string(nil), declaration.ProspectiveTargetAllOfConstraintRefs...)
			}
		}
		if validationRepair != nil {
			repair := validationRepair.Normalized()
			binding.TargetSelectorOwnerRef = repair.RepairDeclarationTargetSelectorOwnerRef
			binding.TargetSelectorSubjectRef = repair.RepairDeclarationTargetSelectorSubjectRef
			binding.TargetSelectorCandidateRefs = append([]string(nil), repair.RepairDeclarationTargetSelectorCandidateRefs...)
		}
	}
	if source != nil && len(prospectiveTargetRefs) != 0 {
		declaration := source.Normalized()
		binding.ProspectiveTargetAllOfConstraintRefs = append([]string(nil), declaration.ProspectiveTargetAllOfConstraintRefs...)
	}
	binding = binding.Normalized()
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	if err := binding.Validate(); err != nil {
		return CoverageFocus{}, err
	}
	focus := CoverageFocus{
		ID: focusID, SliceID: item.SliceID, AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{proofSet.CoverageWorkItemID}, CoverageEvidenceBindings: []CoverageEvidenceBinding{binding},
		EvidenceDeclarationRef: declarationRef, RequiredEvidenceKinds: referenceCoverageBindingEvidenceKinds(binding),
		TargetRefs: targetRefs, TargetRole: "semantic_proof_target", TargetSelectorRef: selectorRef,
		TargetSelectorOwnerRef: binding.TargetSelectorOwnerRef, TargetSelectorSubjectRef: binding.TargetSelectorSubjectRef,
		ProspectiveTargetRefs: prospectiveTargetRefs, RequireTargetGrounding: true,
		AcceptedStepKinds: acceptedKinds, AcceptedOperationVariants: acceptedVariants, RequireDirectEvidence: true, RequireAssessment: true,
		ValidationRecipeID: validationRecipeID, RequiredCheckRefs: validationChecks,
	}
	if source != nil {
		declaration := source.Normalized()
		focus.EvidenceRole = declaration.EvidenceRole
		if declaration.TargetRole != "" {
			focus.TargetRole = declaration.TargetRole
		}
		focus.RequireDirectEvidence = focus.RequireDirectEvidence || declaration.RequireDirectEvidence
		focus.RequireClaimEvidence = declaration.RequireClaimEvidence
		focus.RequireExecutableBehaviorEvidence = declaration.RequireExecutableBehaviorEvidence
		focus.RequireCoverageTargets = declaration.RequireCoverageTargets
		focus.RequireAssessment = focus.RequireAssessment || declaration.RequireAssessment
		focus.RequiredOwnerFacts = append([]RuntimeOwnerFactRequirement(nil), declaration.RequiredOwnerFacts...)
		focus.OwnerFactConvergences = CloneRuntimeOwnerFactConvergences(declaration.OwnerFactConvergences)
		focus.MetricConstraints = append([]string(nil), declaration.MetricConstraints...)
		focus.ArtifactRefs = append([]string(nil), declaration.ArtifactRefs...)
		if len(prospectiveTargetRefs) != 0 {
			focus.ProspectiveTargetAllOfConstraintRefs = append([]string(nil), declaration.ProspectiveTargetAllOfConstraintRefs...)
		}
	}
	if validationRepair != nil {
		repair := validationRepair.Normalized()
		focus.TargetRole = repair.RepairTargetRole
		focus.TargetSelectorRef = repair.RepairOperationTargetSelectorRef
		focus.TargetSelectorOwnerRef = repair.RepairDeclarationTargetSelectorOwnerRef
		focus.TargetSelectorSubjectRef = repair.RepairDeclarationTargetSelectorSubjectRef
		focus.TargetSelectorCandidateRefs = append([]string(nil), repair.RepairDeclarationTargetSelectorCandidateRefs...)
		focus.RequireTargetGrounding = true
	}
	if source != nil {
		declaration := source.Normalized()
		if selectorRef != "" && declaration.TargetSelectorRef == selectorRef {
			focus.TargetRole = declaration.TargetRole
			focus.TargetDiscoverySources = append([]string(nil), declaration.TargetDiscoverySources...)
			focus.TargetSelectorOwnerRef = declaration.TargetSelectorOwnerRef
			focus.TargetSelectorSubjectRef = declaration.TargetSelectorSubjectRef
			focus.TargetSelectorCandidateRefs = append([]string(nil), declaration.TargetSelectorCandidateRefs...)
			focus.ProspectiveTargetAllOfConstraintRefs = append([]string(nil), declaration.ProspectiveTargetAllOfConstraintRefs...)
			focus.RequireTargetGrounding = declaration.RequireTargetGrounding
		}
	}
	focus = focus.Normalized()
	if err := ValidateCoverageEvidenceBindingsForFocus(focus, manifest.DefinitionRevision); err != nil {
		return CoverageFocus{}, err
	}
	return focus, nil
}

func semanticBindingIndexedMultiBatchFixture(t testing.TB, count int) RuntimeSemanticPlanningLedger {
	t.Helper()
	definition := RuntimeDefinitionContext{StartupInputHash: "startup-preimage", Revision: "definition-preimage", GoalID: "goal-preimage", GoalDefinition: "deliver", DoDID: "dod-preimage", DoDDefinition: "works", CapabilityContract: []string{StepKindFileWrite}}
	execution := RuntimeExecutionContract{OperationContractRevision: RuntimeOperationContractRevision(), StateChangePolicy: RuntimeStateChangeAllowedButNotRequired}
	items := make([]RuntimeCoverageItem, count)
	slices := make([]RuntimeAcceptanceSlice, count)
	ids := make([]common.CoverageWorkItemID, count)
	targets := make([]string, count)
	for i := range items {
		ids[i] = common.CoverageWorkItemID(fmt.Sprintf("cwi-%02d", i))
		targets[i] = fmt.Sprintf("resource-%02d", i)
		items[i] = RuntimeCoverageItem{ID: ids[i], SliceID: fmt.Sprintf("slice-%02d", i), SourceScope: "dodItem", SourceID: fmt.Sprintf("source-%02d", i), TargetText: "works", Order: i + 1}
		slices[i] = RuntimeAcceptanceSlice{ID: items[i].SliceID, CoverageWorkItemIDs: []common.CoverageWorkItemID{ids[i]}}
	}
	seed, err := CompileRuntimeDefinitionSeed(definition, RuntimeActivationContext{ExecutionContract: execution}, items, slices)
	if err != nil {
		t.Fatal(err)
	}
	catalog := semanticTestOwnerCatalog("preimage-catalog", RuntimeModuleFile, []string{StepKindFileWrite})
	catalog.TargetRefs = targets
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest, err := CompileRuntimeSemanticPlanManifest("ws-preimage", "run-preimage", definition, seed, semanticDefinitionFocuses(items), execution, []RuntimeOperationAdmissibility{catalog})
	if err != nil {
		t.Fatal(err)
	}
	order := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: seed.Hash, PlanStep: RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: ids}
	ledger := RuntimeSemanticPlanningLedger{SchemaVersion: RuntimeSemanticPlanningLedgerSchemaCurrent, Phase: RuntimeSemanticPlanningSealed, DefinitionSeed: seed, Manifest: &manifest, StablePrefixHash: "stable-preimage", CoverageOrderDocument: &order, NextBatchSequence: count, SealedBindingSetHash: "sealed-preimage"}
	for i, id := range ids {
		request := fmt.Sprintf("request-%02d", i)
		batch := semanticContractTestPendingBatch(t, manifest, order, i, request, ids[i:i+1], ids[i:])
		batch.BatchID = fmt.Sprintf("preimage-batch-%02d", i)
		var declaration string
		for _, obligation := range manifest.ProofObligations {
			if obligation.CoverageWorkItemID == id {
				declaration = RuntimeSemanticCurrentDeclarationRef(obligation)
			}
		}
		proposal := RuntimeSemanticProofProposal{CoverageWorkItemID: id, OwnerModuleID: RuntimeModuleFile, OperationKind: StepKindFileWrite, Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: targets[i : i+1]}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, DeclarationRef: declaration}
		document := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: seed.Hash, PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: batch.BatchID, BatchSequence: i, BatchCoverageWorkItemIDs: ids[i : i+1], Proposals: []RuntimeSemanticProofProposal{proposal}}
		batch.State, batch.DocumentHash = RuntimeSemanticPlanBatchAccepted, StableRuntimeSemanticPlanDocumentHash(document)
		batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)
		facts := []RuntimeSemanticProofFeasibility{{SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: seed.Hash, CoverageWorkItemID: id, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: RuntimeModuleFile, CatalogSnapshotID: catalog.SnapshotID, Status: RuntimeSemanticFeasibilityFeasible, ObservedRevision: catalog.ObservedRevision, TargetRefs: targets[i : i+1]}}
		result := RuntimeSemanticPlanBatchFeasibilityResult{BatchID: batch.BatchID, BatchHash: batch.Hash, DocumentHash: batch.DocumentHash, Feasibilities: facts}
		result.Hash = StableRuntimeSemanticPlanBatchFeasibilityResultHash(result)
		ledger.Batches = append(ledger.Batches, batch)
		ledger.BatchDocuments = append(ledger.BatchDocuments, document)
		ledger.BatchFeasibilityResults = append(ledger.BatchFeasibilityResults, result)
		ledger.CompletedBatchIDs = append(ledger.CompletedBatchIDs, batch.BatchID)
		ledger.LogicalRequestID, ledger.Document, ledger.DocumentHash = request, &document, batch.DocumentHash
		ledger.Feasibilities, ledger.FeasibilitySetHash = facts, StableRuntimeSemanticFeasibilitySetHash(facts)
		ledger.MergedDocument, ledger.MergedDocumentHash = &document, batch.DocumentHash
		ledger.FeasibilityBatchID = StableRuntimeSemanticFeasibilityBatchID(manifest, document)
	}
	ledger.SealedLineageRefs = []string{manifest.Hash, StableRuntimeSemanticPlanCoverageOrderHash(order), ledger.DocumentHash, ledger.FeasibilityBatchID, ledger.FeasibilitySetHash, ledger.SealedBindingSetHash}
	if err := ledger.Validate(); err != nil {
		t.Fatalf("同代多批 fixture 非法: %v", err)
	}
	return ledger
}
