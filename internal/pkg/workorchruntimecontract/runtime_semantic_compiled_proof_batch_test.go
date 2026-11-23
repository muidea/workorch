package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func compiledProofBatchForManifest(m RuntimeSemanticPlanManifest, coverageID common.CoverageWorkItemID) RuntimeSemanticPlanBatch {
	return RuntimeSemanticPlanBatch{
		BatchID:                    "batch-singleton-1",
		ManifestHash:               m.Hash,
		DefinitionSeedHash:         m.DefinitionSeedHash,
		CoverageOrderHash:          "runtime-semantic-plan-order-test",
		Sequence:                   0,
		CoverageWorkItemIDs:        []common.CoverageWorkItemID{coverageID},
		PendingCoverageWorkItemIDs: nil,
		RequestLogicalID:           "req-singleton-1",
		State:                      RuntimeSemanticPlanBatchRequesting,
	}.Normalized()
}

func TestCompileRuntimeSingletonProofBatchExactOwnerFactAcquisitionPositive(t *testing.T) {
	baseManifest, _, _ := semanticMissingOwnerFactAcquisitionFixture(t)

	for _, withSingleCandidate := range []bool{false, true} {
		name := "selector_only"
		if withSingleCandidate {
			name = "single_candidate_target"
		}
		t.Run(name, func(t *testing.T) {
			manifest := baseManifest.Clone()
			for idx := range manifest.OwnerCatalogs {
				catalog := &manifest.OwnerCatalogs[idx]
				if catalog.OwnerModuleID != RuntimeModuleFile {
					continue
				}
				catalog.TargetRefs = []string{"validation-readiness.sh"}
				for vIdx := range catalog.OperationVariants {
					if catalog.OperationVariants[vIdx].Kind == StepKindFileWrite {
						catalog.OperationVariants[vIdx].AllowedTargetPolicies = []RuntimeOperationTargetPolicy{
							RuntimeOperationTargetPolicyExactTarget,
							RuntimeOperationTargetPolicyExistingOwnerScope,
						}
						catalog.OperationVariants[vIdx].ConstraintRefs = nil
					}
				}
				catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
			}
			obligation := &manifest.ProofObligations[0]
			obligation.OwnerFactAcquisition.AcquisitionTargetPolicy = RuntimeOperationTargetPolicyExistingOwnerScope
			obligation.OwnerFactAcquisition.ProspectiveSatisfactionContractRef = ""
			obligation.OwnerFactAcquisition.ProspectiveTargetSatisfactionConstraint = nil
			obligation.OwnerFactAcquisition.AcquisitionConstraintRefs = nil
			obligation.OwnerFactAcquisition.OwnerCatalogSnapshotID = manifest.OwnerCatalogs[0].SnapshotID
			obligation.OwnerFactAcquisition.OwnerCatalogObservedRevision = manifest.OwnerCatalogs[0].ObservedRevision
			if !withSingleCandidate {
				obligation.DeclarationTargetSelectorCandidateRefs = nil
			} else {
				obligation.DeclarationTargetSelectorCandidateRefs = []string{"validation-readiness.sh"}
			}
			manifest.AllowedTargetPolicies = []RuntimeOperationTargetPolicy{
				RuntimeOperationTargetPolicyExactTarget,
				RuntimeOperationTargetPolicyExistingOwnerScope,
			}
			manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)

			batch := compiledProofBatchForManifest(manifest, obligation.CoverageWorkItemID)
			doc, rule, ok := CompileRuntimeSingletonProofBatch(manifest, batch)
			if !ok {
				t.Fatalf("expected deterministic existing-scope acquisition compilation, manifest=%+v", manifest.ProofObligations[0].OwnerFactAcquisition)
			}
			if rule != RuntimeSemanticCompiledProofBatchRuleSingletonExactOwnerFactAcquisition {
				t.Fatalf("rule = %q, want %q", rule, RuntimeSemanticCompiledProofBatchRuleSingletonExactOwnerFactAcquisition)
			}
			if err := doc.ValidateAgainstManifest(manifest); err != nil {
				t.Fatalf("compiled doc failed manifest validation: %v", err)
			}
			compiled := NewRuntimeSemanticCompiledProofBatch(manifest, batch, rule, doc)
			if err := compiled.Validate(&manifest, &batch, &doc); err != nil {
				t.Fatalf("compiled provenance validate: %v", err)
			}
			if withSingleCandidate {
				if len(doc.Proposals[0].Target.OwnerScopeTargetRefs) != 1 || doc.Proposals[0].Target.OwnerScopeTargetRefs[0] != "validation-readiness.sh" {
					t.Fatalf("expected OwnerScopeTargetRefs with candidate ref, got %+v", doc.Proposals[0].Target)
				}
			} else if len(doc.Proposals[0].Target.OwnerScopeTargetRefs) != 0 {
				t.Fatalf("expected nil OwnerScopeTargetRefs without candidate refs, got %+v", doc.Proposals[0].Target)
			}
		})
	}
}

func validationSingletonTestManifest(t *testing.T, targetRefs, selectorRefs []string, extraSurface bool, inputReview *RuntimeValidationInputReviewSubject) RuntimeSemanticPlanManifest {
	t.Helper()
	base, _, _, _ := semanticIndependentProofSetFixture(t)
	manifest := base.Clone()

	// Keep only the single Shell catalog and validation obligation.
	var shellCatalog RuntimeOperationAdmissibility
	for _, cat := range manifest.OwnerCatalogs {
		if cat.OwnerModuleID == RuntimeModuleShell {
			shellCatalog = cat.Clone()
			break
		}
	}
	shellCatalog.TargetRefs = append([]string(nil), targetRefs...)
	shellCatalog.SelectorRefs = append([]string(nil), selectorRefs...)
	requirement := runtimeValidationProofRequirementForTest(
		t, manifest.DefinitionRevision, manifest.CoverageItems[0],
		semanticTestDeclarationRef("cwi-001"), "recipe/validate", []string{"check/validate"},
	)
	surface := bindRuntimeValidationSurfaceForTest(RuntimeValidationSurface{
		ID: "recipe/validate", OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
		Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: "semantic-binding-input-schema", InputBindingRef: "semantic-binding-input",
		EffectScope: RuntimeEffectEphemeralSession, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}, requirement)
	if inputReview != nil {
		cloned := inputReview.Clone()
		surface.ProofRequirementBindings[0].InputReview = &cloned
	}
	shellCatalog.ValidationSurfaces = []RuntimeValidationSurface{surface}
	shellCatalog.ValidationRecipeRefs = []string{"recipe/validate"}
	if extraSurface {
		otherReq := runtimeValidationProofRequirementForTest(
			t, manifest.DefinitionRevision, manifest.CoverageItems[0],
			semanticTestDeclarationRef("cwi-001"), "recipe/validate-alt", []string{"check/validate"},
		)
		other := bindRuntimeValidationSurfaceForTest(RuntimeValidationSurface{
			ID: "recipe/validate-alt", OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
			Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: "semantic-binding-input-schema", InputBindingRef: "semantic-binding-input-alt",
			EffectScope: RuntimeEffectEphemeralSession, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
		}, otherReq)
		shellCatalog.ValidationSurfaces = append(shellCatalog.ValidationSurfaces, other)
		shellCatalog.ValidationRecipeRefs = append(shellCatalog.ValidationRecipeRefs, "recipe/validate-alt")
		requirement.ValidationRecipeID = ""
	}
	shellCatalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(shellCatalog)

	manifest.AllowedOwnerModuleIDs = []string{RuntimeModuleShell}
	manifest.AllowedOperationKinds = []string{RuntimeBlockKindManagedValidationSession}
	manifest.AllowedTargetPolicies = []RuntimeOperationTargetPolicy{
		RuntimeOperationTargetPolicyExactTarget,
		RuntimeOperationTargetPolicyExistingOwnerScope,
	}
	if len(targetRefs) == 0 {
		manifest.AllowedTargetPolicies = []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyExistingOwnerScope}
	}
	manifest.OwnerCatalogs = []RuntimeOperationAdmissibility{shellCatalog}
	obligation := manifest.ProofObligations[0].Clone()
	obligation.RequiredEvidenceKinds = []string{EvidenceKindValidation}
	obligation.DeclarationAcceptedStepKinds = []string{RuntimeBlockKindManagedValidationSession}
	obligation.ValidationProofRequirement = &requirement
	manifest.ProofObligations = []RuntimeSemanticProofObligation{obligation}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	return manifest
}

func TestCompileRuntimeSingletonProofBatchUniqueDirectValidationSurfacePositive(t *testing.T) {
	t.Run("existing_owner_scope_single_selector", func(t *testing.T) {
		manifest := validationSingletonTestManifest(t, nil, []string{"workspace-validation-target"}, false, nil)
		batch := compiledProofBatchForManifest(manifest, manifest.CoverageItems[0].ID)
		doc, rule, ok := CompileRuntimeSingletonProofBatch(manifest, batch)
		if !ok {
			t.Fatal("expected deterministic unique validation surface compilation with single selector")
		}
		if rule != RuntimeSemanticCompiledProofBatchRuleSingletonUniqueDirectValidationSurface {
			t.Fatalf("rule = %q", rule)
		}
		if doc.Proposals[0].Target.Policy != RuntimeOperationTargetPolicyExistingOwnerScope ||
			!sameStrings(doc.Proposals[0].Target.OwnerSelectorRefs, []string{"workspace-validation-target"}) {
			t.Fatalf("unexpected target = %+v", doc.Proposals[0].Target)
		}
		if err := doc.ValidateAgainstManifest(manifest); err != nil {
			t.Fatalf("compiled doc failed manifest validation: %v", err)
		}
		compiled := NewRuntimeSemanticCompiledProofBatch(manifest, batch, rule, doc)
		if err := compiled.Validate(&manifest, &batch, &doc); err != nil {
			t.Fatalf("compiled provenance validate: %v", err)
		}
	})

	t.Run("exact_target_single_target", func(t *testing.T) {
		manifest := validationSingletonTestManifest(t, []string{"main.go"}, nil, false, nil)
		batch := compiledProofBatchForManifest(manifest, manifest.CoverageItems[0].ID)
		doc, rule, ok := CompileRuntimeSingletonProofBatch(manifest, batch)
		if !ok {
			t.Fatal("expected deterministic unique validation surface compilation with single exact target")
		}
		if rule != RuntimeSemanticCompiledProofBatchRuleSingletonUniqueDirectValidationSurface {
			t.Fatalf("rule = %q", rule)
		}
		if doc.Proposals[0].Target.Policy != RuntimeOperationTargetPolicyExactTarget ||
			!sameStrings(doc.Proposals[0].Target.ExactTargetRefs, []string{"main.go"}) {
			t.Fatalf("unexpected target = %+v", doc.Proposals[0].Target)
		}
		if err := doc.ValidateAgainstManifest(manifest); err != nil {
			t.Fatalf("compiled doc failed manifest validation: %v", err)
		}
		compiled := NewRuntimeSemanticCompiledProofBatch(manifest, batch, rule, doc)
		if err := compiled.Validate(&manifest, &batch, &doc); err != nil {
			t.Fatalf("compiled provenance validate: %v", err)
		}
	})
}

func TestCompileRuntimeSingletonProofBatchNegativeGuards(t *testing.T) {
	t.Run("multi_cwi_batch", func(t *testing.T) {
		manifest := validationSingletonTestManifest(t, []string{"main.go"}, nil, false, nil)
		secondItem := manifest.CoverageItems[0]
		secondItem.ID = "cwi-002"
		manifest.CoverageItems = append(manifest.CoverageItems, secondItem)
		secondObligation := manifest.ProofObligations[0].Clone()
		secondObligation.CoverageWorkItemID = "cwi-002"
		manifest.ProofObligations = append(manifest.ProofObligations, secondObligation)
		manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)

		batch := compiledProofBatchForManifest(manifest, manifest.CoverageItems[0].ID)
		batch.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-001", "cwi-002"}
		if _, _, ok := CompileRuntimeSingletonProofBatch(manifest, batch); ok {
			t.Fatal("multi-CWI batch must not deterministically compile")
		}
	})

	t.Run("prospective_owner_fact_acquisition", func(t *testing.T) {
		manifest, _, _ := semanticMissingOwnerFactAcquisitionFixture(t)
		batch := compiledProofBatchForManifest(manifest, manifest.CoverageItems[0].ID)
		if _, _, ok := CompileRuntimeSingletonProofBatch(manifest, batch); ok {
			t.Fatal("prospective owner-fact acquisition must require LLM target selection")
		}
	})

	t.Run("ambiguous_multiple_surfaces", func(t *testing.T) {
		manifest := validationSingletonTestManifest(t, []string{"main.go"}, nil, true, nil)
		batch := compiledProofBatchForManifest(manifest, manifest.CoverageItems[0].ID)
		if _, _, ok := CompileRuntimeSingletonProofBatch(manifest, batch); ok {
			t.Fatal("ambiguous multiple direct surfaces must not compile deterministically")
		}
	})

	t.Run("validation_input_review_required", func(t *testing.T) {
		manifest := validationSingletonTestManifest(t, []string{"main.go"}, nil, false, &RuntimeValidationInputReviewSubject{
			ProtocolRef: "shell/validation-input-review/v1",
			Hash:        "sha256-subject",
		})
		batch := compiledProofBatchForManifest(manifest, manifest.CoverageItems[0].ID)
		if _, _, ok := CompileRuntimeSingletonProofBatch(manifest, batch); ok {
			t.Fatal("surface requiring validation input review must use LLM")
		}
	})

	t.Run("failure_input_review_active", func(t *testing.T) {
		manifest := validationSingletonTestManifest(t, []string{"main.go"}, nil, false, nil)
		manifest.FailureInputReview = &RuntimeValidationFailureInputReview{
			SubjectHash: "sha256-failure-subject",
			Feedback: RuntimeSemanticExecutionFeedback{
				CoverageWorkItemID: "cwi-001",
			},
		}
		manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
		batch := compiledProofBatchForManifest(manifest, manifest.CoverageItems[0].ID)
		if _, _, ok := CompileRuntimeSingletonProofBatch(manifest, batch); ok {
			t.Fatal("active failure input review must not deterministically compile")
		}
	})

	t.Run("implementation_work_obligation", func(t *testing.T) {
		manifest := validationSingletonTestManifest(t, []string{"main.go"}, nil, false, nil)
		manifest.ProofObligations[0].ImplementationWork = &RuntimeSemanticImplementationWork{
			PurposeChoiceOnly: true,
		}
		manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
		batch := compiledProofBatchForManifest(manifest, manifest.CoverageItems[0].ID)
		if _, _, ok := CompileRuntimeSingletonProofBatch(manifest, batch); ok {
			t.Fatal("implementation-work obligation must not deterministically compile")
		}
	})

	t.Run("validation_failure_repair_obligation", func(t *testing.T) {
		manifest := validationSingletonTestManifest(t, []string{"main.go"}, nil, false, nil)
		manifest.ProofObligations[0].ValidationFailureRepair = &RuntimeSemanticValidationFailureRepair{
			ConvergenceID: "repair-1",
			RouteID:       "route-1",
		}
		manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
		batch := compiledProofBatchForManifest(manifest, manifest.CoverageItems[0].ID)
		if _, _, ok := CompileRuntimeSingletonProofBatch(manifest, batch); ok {
			t.Fatal("validation-failure-repair obligation must not deterministically compile")
		}
	})

	t.Run("historical_acceptance_obligation", func(t *testing.T) {
		manifest := validationSingletonTestManifest(t, []string{"main.go"}, nil, false, nil)
		manifest.ProofObligations[0].HistoricalAcceptance = &RuntimeHistoricalAcceptanceSource{
			HistoricalCoverageWorkItemID: manifest.CoverageItems[0].ID,
			AcceptanceCoverageWorkItemID: manifest.CoverageItems[0].ID,
		}
		manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
		batch := compiledProofBatchForManifest(manifest, manifest.CoverageItems[0].ID)
		if _, _, ok := CompileRuntimeSingletonProofBatch(manifest, batch); ok {
			t.Fatal("historical-acceptance obligation must not deterministically compile")
		}
	})

	t.Run("ambiguous_targets_and_selectors", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			targets   []string
			selectors []string
		}{
			{name: "two_targets", targets: []string{"a.go", "b.go"}},
			{name: "two_selectors", selectors: []string{"sel-1", "sel-2"}},
			{name: "both_target_and_selector", targets: []string{"a.go"}, selectors: []string{"sel-1"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				manifest := validationSingletonTestManifest(t, tc.targets, tc.selectors, false, nil)
				batch := compiledProofBatchForManifest(manifest, manifest.CoverageItems[0].ID)
				if _, _, ok := CompileRuntimeSingletonProofBatch(manifest, batch); ok {
					t.Fatalf("ambiguous catalog (%v / %v) must not compile", tc.targets, tc.selectors)
				}
			})
		}
	})
}

func TestRuntimeSemanticCompiledProofBatchRejectsForgedProvenance(t *testing.T) {
	manifest := validationSingletonTestManifest(t, []string{"main.go"}, nil, false, nil)
	batch := compiledProofBatchForManifest(manifest, manifest.CoverageItems[0].ID)
	doc, rule, ok := CompileRuntimeSingletonProofBatch(manifest, batch)
	if !ok {
		t.Fatal("expected valid compiled doc")
	}
	compiled := NewRuntimeSemanticCompiledProofBatch(manifest, batch, rule, doc)

	wrongRule := *compiled
	wrongRule.Rule = RuntimeSemanticCompiledProofBatchRuleSingletonExactOwnerFactAcquisition
	if wrongRule.Validate(&manifest, &batch, &doc) == nil {
		t.Fatal("accepted mismatched compiled rule")
	}

	wrongManifest := *compiled
	wrongManifest.ManifestHash = "foreign-manifest"
	if wrongManifest.Validate(&manifest, &batch, &doc) == nil {
		t.Fatal("accepted foreign manifest hash")
	}

	wrongBatch := *compiled
	wrongBatch.BatchID = "foreign-batch"
	if wrongBatch.Validate(&manifest, &batch, &doc) == nil {
		t.Fatal("accepted foreign batch identity")
	}

	wrongDocHash := *compiled
	wrongDocHash.DocumentHash = "foreign-doc-hash"
	if wrongDocHash.Validate(&manifest, &batch, &doc) == nil {
		t.Fatal("accepted foreign document hash")
	}

	alteredDoc := doc.Clone()
	alteredDoc.Proposals[0].Target.ExactTargetRefs = []string{"other.go"}
	if compiled.Validate(&manifest, &batch, &alteredDoc) == nil {
		t.Fatal("accepted altered document")
	}

	orderDoc := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: []common.CoverageWorkItemID{manifest.CoverageItems[0].ID},
	}
	orderDoc.SchemaVersion = RuntimeSemanticPlanDocumentSchemaCurrent
	batch.CoverageOrderHash = StableRuntimeSemanticPlanCoverageOrderHash(orderDoc)
	doc, rule, ok = CompileRuntimeSingletonProofBatch(manifest, batch)
	if !ok {
		t.Fatal("expected compiled doc for coverage order test")
	}
	compiled = NewRuntimeSemanticCompiledProofBatch(manifest, batch, rule, doc)
	batch.DocumentHash = StableRuntimeSemanticPlanDocumentHash(doc)
	batch.State = RuntimeSemanticPlanBatchDocumentComplete
	batch.Hash = StableRuntimeSemanticPlanBatchHash(batch)

	requiredWithCompiled := RuntimeSemanticPlanningLedger{
		SchemaVersion:        RuntimeSemanticPlanningLedgerSchemaCurrent,
		DefinitionSeedHash:   manifest.DefinitionSeedHash,
		Phase:                RuntimeSemanticPlanningRequired,
		CompiledProofBatches: []RuntimeSemanticCompiledProofBatch{*compiled},
	}
	if requiredWithCompiled.Validate() == nil {
		t.Fatal("accepted non-empty CompiledProofBatches in Required phase")
	}

	dupCompiledLedger := RuntimeSemanticPlanningLedger{
		SchemaVersion:         RuntimeSemanticPlanningLedgerSchemaCurrent,
		DefinitionSeedHash:    manifest.DefinitionSeedHash,
		Manifest:              &manifest,
		StablePrefixHash:      "prefix-1",
		LogicalRequestID:      "req-singleton-1",
		CoverageOrderDocument: &orderDoc,
		Document:              &orderDoc,
		DocumentHash:          StableRuntimeSemanticPlanDocumentHash(orderDoc),
		Batches:               []RuntimeSemanticPlanBatch{batch},
		BatchDocuments:        []RuntimeSemanticPlanDocument{doc},
		ActiveBatchID:         batch.BatchID,
		NextBatchSequence:     1,
		CompiledProofBatches:  []RuntimeSemanticCompiledProofBatch{*compiled, *compiled},
		Phase:                 RuntimeSemanticPlanningProofBatchDocumentAccepted,
	}
	if dupCompiledLedger.Validate() == nil {
		t.Fatal("accepted duplicate BatchID in CompiledProofBatches")
	}
}

func reusedReviewSingletonManifestFixture(t *testing.T, inputBody string) (RuntimeSemanticPlanManifest, RuntimeSemanticPlanDocument, RuntimeValidationProofRequirementBinding, RuntimeValidationInputReview) {
	t.Helper()
	manifest := validationSingletonTestManifest(t, []string{"main.go"}, nil, false, nil)
	catalog := &manifest.OwnerCatalogs[0]
	surface := &catalog.ValidationSurfaces[0]
	binding := surface.ProofRequirementBindings[0].Clone()
	subject := RuntimeValidationInputReviewSubject{
		ProtocolRef: RuntimeValidationInputReviewProtocol,
		Files: []RuntimeValidationInputReviewFile{{
			TargetRef:   "checks/input_test.go",
			Content:     inputBody,
			ContentHash: RuntimeValidationInputContentHash(inputBody),
		}},
	}
	subject.Hash = StableRuntimeValidationInputReviewSubjectHash(subject, binding)
	binding.InputReview = &subject
	surface.ProofRequirementBindings[0] = binding.Clone()
	catalog.ValidationRecipes = []RuntimeValidationRecipe{{
		ID: binding.ValidationRecipeID, AdapterRef: "owner/review", Command: "check",
		InputReviewProtocolRef: RuntimeValidationInputReviewProtocol,
	}}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		t.Fatalf("manifest validate: %v", err)
	}
	orderDoc := RuntimeSemanticPlanDocument{
		SchemaVersion:      RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:       manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep:           RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder:      []common.CoverageWorkItemID{manifest.CoverageItems[0].ID},
	}
	review := RuntimeValidationInputReview{SubjectHash: subject.Hash}
	for _, check := range binding.RequiredCheckRefs {
		review.Checks = append(review.Checks, RuntimeValidationInputCheckReview{
			RequiredCheckRef: check,
			Conclusion:       RuntimeValidationInputConsistent,
			Reason:           "Preserves the original requirement without skipping or waiving checks.",
			InputTargetRefs:  []string{"checks/input_test.go"},
			Obligations: []RuntimeValidationInputObligationReview{{
				CriterionText:   manifest.CoverageItems[0].TargetText,
				Finding:         RuntimeValidationInputCovered,
				Reason:          "Original criterion verified",
				InputTargetRefs: []string{"checks/input_test.go"},
			}},
		})
	}
	return manifest, orderDoc, binding, review
}

func TestCompileRuntimeSingletonProofBatchReusesCommittedConsistentInputReview(t *testing.T) {
	manifest1, orderDoc1, _, consistentReview := reusedReviewSingletonManifestFixture(t, "package checks\n// stable input\n")
	cwi := manifest1.CoverageItems[0].ID
	orderHash1 := StableRuntimeSemanticPlanCoverageOrderHash(orderDoc1)

	batch1 := compiledProofBatchForManifest(manifest1, cwi)
	batch1.CoverageOrderHash = orderHash1
	batch1.State = RuntimeSemanticPlanBatchRequesting
	batch1.Hash = StableRuntimeSemanticPlanBatchHash(batch1)
	variant1, ok := compileRuntimeSingletonVariantID(manifest1.OwnerCatalogs[0], RuntimeBlockKindManagedValidationSession)
	if !ok {
		t.Fatal("missing singleton variant")
	}
	doc1 := RuntimeSemanticPlanDocument{
		SchemaVersion:            RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:             manifest1.Hash,
		DefinitionSeedHash:       manifest1.DefinitionSeedHash,
		PlanStep:                 RuntimeSemanticPlanStepProofBatch,
		BatchID:                  batch1.BatchID,
		BatchSequence:            0,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{cwi},
		Proposals: []RuntimeSemanticProofProposal{{
			CoverageWorkItemID:    cwi,
			OwnerModuleID:         RuntimeModuleShell,
			OperationKind:         RuntimeBlockKindManagedValidationSession,
			OperationVariantID:    variant1,
			Target:                RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"main.go"}},
			RequiredEvidenceKinds: []string{EvidenceKindValidation},
			RequiredEffectScopes:  []RuntimeEffectScope{RuntimeEffectEphemeralSession},
			ValidationRecipeID:    manifest1.OwnerCatalogs[0].ValidationSurfaces[0].ID,
			DeclarationRef:        RuntimeSemanticCurrentDeclarationRef(manifest1.ProofObligations[0]),
			ValidationInputReview: &consistentReview,
		}},
	}.Normalized()
	if err := doc1.ValidateAgainstManifest(manifest1); err != nil {
		t.Fatalf("source doc1 invalid: %v", err)
	}
	batch1.DocumentHash = StableRuntimeSemanticPlanDocumentHash(doc1)
	batch1.State = RuntimeSemanticPlanBatchFeasibilityPending
	batch1.Hash = StableRuntimeSemanticPlanBatchHash(batch1)

	gen1 := RuntimeSemanticPlanningGeneration{
		RetiredAtStateRevision: 10,
		Reason:                 RuntimeSemanticPlanningRetirementCWIReopened,
		ReopenedCoverageIDs:    []common.CoverageWorkItemID{cwi},
		ReopenTriggerRefs:      []string{"receipt/source-edited"},
		Snapshot: RuntimeSemanticPlanningLedger{
			SchemaVersion:         RuntimeSemanticPlanningLedgerSchemaCurrent,
			DefinitionSeedHash:    manifest1.DefinitionSeedHash,
			Manifest:              &manifest1,
			StablePrefixHash:      "prefix-1",
			LogicalRequestID:      "req-gen-1",
			CoverageOrderDocument: &orderDoc1,
			Document:              &orderDoc1,
			DocumentHash:          StableRuntimeSemanticPlanDocumentHash(orderDoc1),
			Batches:               []RuntimeSemanticPlanBatch{batch1},
			BatchDocuments:        []RuntimeSemanticPlanDocument{doc1},
			ActiveBatchID:         batch1.BatchID,
			NextBatchSequence:     1,
			FeasibilityBatchID:    StableRuntimeSemanticFeasibilityBatchID(manifest1, doc1),
			Phase:                 RuntimeSemanticPlanningFeasibilityRequired,
		},
	}

	// Same input body in a later generation compiles deterministically without calling LLM.
	manifest2, orderDoc2, _, _ := reusedReviewSingletonManifestFixture(t, "package checks\n// stable input\n")
	// Perturb non-review catalog revision so manifest2 has a distinct ManifestHash.
	manifest2.OwnerCatalogs[0].ObservedRevision = "shell-policy-v2"
	manifest2.OwnerCatalogs[0].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(manifest2.OwnerCatalogs[0])
	manifest2.Hash = StableRuntimeSemanticPlanManifestHash(manifest2)
	orderDoc2.ManifestHash = manifest2.Hash
	batch2 := compiledProofBatchForManifest(manifest2, cwi)
	batch2.CoverageOrderHash = StableRuntimeSemanticPlanCoverageOrderHash(orderDoc2)
	batch2.RequestLogicalID = "req-gen-2"

	doc2, compiled2, ok := CompileRuntimeSingletonProofBatchWithHistory(manifest2, batch2, []RuntimeSemanticPlanningGeneration{gen1})
	if !ok || compiled2 == nil {
		t.Fatal("expected deterministic reuse of committed consistent ValidationInputReview")
	}
	if compiled2.Rule != RuntimeSemanticCompiledProofBatchRuleSingletonUniqueDirectValidationSurfaceReusedReview ||
		compiled2.ReusedInputReviewManifestHash != manifest1.Hash ||
		compiled2.ReusedInputReviewDocumentHash != StableRuntimeSemanticPlanDocumentHash(doc1) ||
		compiled2.ReusedInputReviewSubjectHash != consistentReview.SubjectHash {
		t.Fatalf("unexpected compiled provenance: %+v", compiled2)
	}
	if !reflect.DeepEqual(doc2.Proposals[0].ValidationInputReview, &consistentReview) {
		t.Fatalf("reused review body mismatch: %+v", doc2.Proposals[0].ValidationInputReview)
	}
	if err := compiled2.ValidateWithHistory(&manifest2, &batch2, &doc2, []RuntimeSemanticPlanningGeneration{gen1}); err != nil {
		t.Fatalf("ValidateWithHistory failed: %v", err)
	}

	batch2.DocumentHash = StableRuntimeSemanticPlanDocumentHash(doc2)
	batch2.State = RuntimeSemanticPlanBatchFeasibilityPending
	batch2.Hash = StableRuntimeSemanticPlanBatchHash(batch2)
	ledger2 := RuntimeSemanticPlanningLedger{
		SchemaVersion:         RuntimeSemanticPlanningLedgerSchemaCurrent,
		DefinitionSeedHash:    manifest2.DefinitionSeedHash,
		History:               []RuntimeSemanticPlanningGeneration{gen1},
		Manifest:              &manifest2,
		StablePrefixHash:      "prefix-2",
		LogicalRequestID:      "req-gen-2",
		CoverageOrderDocument: &orderDoc2,
		Document:              &orderDoc2,
		DocumentHash:          StableRuntimeSemanticPlanDocumentHash(orderDoc2),
		Batches:               []RuntimeSemanticPlanBatch{batch2},
		BatchDocuments:        []RuntimeSemanticPlanDocument{doc2},
		CompiledProofBatches:  []RuntimeSemanticCompiledProofBatch{*compiled2},
		ActiveBatchID:         batch2.BatchID,
		NextBatchSequence:     1,
		FeasibilityBatchID:    StableRuntimeSemanticFeasibilityBatchID(manifest2, doc2),
		Phase:                 RuntimeSemanticPlanningFeasibilityRequired,
	}
	if err := ledger2.Validate(); err != nil {
		t.Fatalf("active ledger with reused-review compiled batch failed Validate: %v", err)
	}

	// Retiring gen2 into History and compiling gen3 must preserve cold JSON round-trip validation
	// and keep gen3 pointing 1-hop to gen1 (anti-chaining).
	gen2 := RuntimeSemanticPlanningGeneration{
		RetiredAtStateRevision: 20,
		Reason:                 RuntimeSemanticPlanningRetirementCWIReopened,
		ReopenedCoverageIDs:    []common.CoverageWorkItemID{cwi},
		ReopenTriggerRefs:      []string{"receipt/source-edited-2"},
		Snapshot:               ledger2.WithoutHistory().CloneRaw(),
	}
	ledger3 := ledger2.CloneRaw()
	ledger3.History = []RuntimeSemanticPlanningGeneration{gen1, gen2}
	rawJSON, err := json.Marshal(ledger3)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeSemanticPlanningLedger
	if err := json.Unmarshal(rawJSON, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Validate(); err != nil {
		t.Fatalf("fresh cold load of ledger with historical reused-review compiled batch failed: %v", err)
	}

	// Anti-chaining: only passing gen2 (compiled) as preceding history must fail both compile & validate.
	if _, _, chained := CompileRuntimeSingletonProofBatchWithHistory(manifest2, batch2, []RuntimeSemanticPlanningGeneration{gen2}); chained {
		t.Fatal("compiled proof batch chained onto another compiled batch instead of original provider review")
	}
	if compiled2.ValidateWithHistory(&manifest2, &batch2, &doc2, []RuntimeSemanticPlanningGeneration{gen2}) == nil {
		t.Fatal("ValidateWithHistory allowed chaining onto a compiled generation")
	}

	// Negative: 1-byte SubjectHash drift, changed RequirementHash, non-consistent conclusion,
	// FailureInputReview != nil, or forged provenance are all rejected.
	driftedManifest, driftedOrder, _, _ := reusedReviewSingletonManifestFixture(t, "package checks\n// 1-byte edited input\n")
	driftedBatch := compiledProofBatchForManifest(driftedManifest, cwi)
	driftedBatch.CoverageOrderHash = StableRuntimeSemanticPlanCoverageOrderHash(driftedOrder)
	if _, _, reused := CompileRuntimeSingletonProofBatchWithHistory(driftedManifest, driftedBatch, []RuntimeSemanticPlanningGeneration{gen1}); reused {
		t.Fatal("reused committed review after input file content changed")
	}

	reqChanged := manifest2.Clone()
	reqChanged.ProofObligations[0].ValidationProofRequirement.RequiredCheckRefs = []string{"check/validate", "check/extra"}
	reqChanged.ProofObligations[0].ValidationProofRequirement.Hash = StableRuntimeValidationProofRequirementHash(*reqChanged.ProofObligations[0].ValidationProofRequirement)
	reqChanged.Hash = StableRuntimeSemanticPlanManifestHash(reqChanged)
	if _, _, reused := CompileRuntimeSingletonProofBatchWithHistory(reqChanged, compiledProofBatchForManifest(reqChanged, cwi), []RuntimeSemanticPlanningGeneration{gen1}); reused {
		t.Fatal("reused committed review across changed RequirementHash")
	}

	failureReviewManifest := manifest2.Clone()
	failureReviewManifest.FailureInputReview = &RuntimeValidationFailureInputReview{SubjectHash: "review-failure"}
	failureReviewManifest.Hash = StableRuntimeSemanticPlanManifestHash(failureReviewManifest)
	if _, _, reused := CompileRuntimeSingletonProofBatchWithHistory(failureReviewManifest, compiledProofBatchForManifest(failureReviewManifest, cwi), []RuntimeSemanticPlanningGeneration{gen1}); reused {
		t.Fatal("compiled singleton proof batch during FailureInputReview")
	}

	nonConsistentGen := gen1.Clone()
	badReview := consistentReview.Clone()
	badReview.Checks[0].Conclusion = RuntimeValidationInputConflict
	badReview.Checks[0].Obligations[0].Finding = RuntimeValidationInputConflicting
	nonConsistentDoc := nonConsistentGen.Snapshot.BatchDocuments[0].Clone()
	nonConsistentDoc.Proposals[0].ValidationInputReview = &badReview
	nonConsistentGen.Snapshot.BatchDocuments[0] = nonConsistentDoc
	nonConsistentGen.Snapshot.Batches[0].DocumentHash = StableRuntimeSemanticPlanDocumentHash(nonConsistentDoc)
	nonConsistentGen.Snapshot.Batches[0].Hash = StableRuntimeSemanticPlanBatchHash(nonConsistentGen.Snapshot.Batches[0])
	if _, _, reused := CompileRuntimeSingletonProofBatchWithHistory(manifest2, batch2, []RuntimeSemanticPlanningGeneration{nonConsistentGen}); reused {
		t.Fatal("reused non-consistent review")
	}

	forged := *compiled2
	forged.ReusedInputReviewDocumentHash = "forged-doc-hash"
	if forged.ValidateWithHistory(&manifest2, &batch2, &doc2, []RuntimeSemanticPlanningGeneration{gen1}) == nil {
		t.Fatal("accepted forged ReusedInputReviewDocumentHash")
	}
}
