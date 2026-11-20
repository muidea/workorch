package workorchruntimecontract

import (
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
