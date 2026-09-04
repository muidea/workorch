package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCompileRuntimeSemanticValidationFailureRepairSealsOneFileProofAndFreshValidationLineage(t *testing.T) {
	definition := RuntimeDefinitionContext{
		StartupInputHash: "startup-repair", Revision: "definition-repair", GoalID: "goal-repair", GoalDefinition: "implement behavior",
		DoDID: "dod-repair", DoDDefinition: "behavior passes validation",
		CapabilityContract: []string{StepKindFileWrite, RuntimeBlockKindPatchApply, RuntimeBlockKindManagedValidationSession},
	}
	execution := RuntimeExecutionContract{OperationContractRevision: RuntimeOperationContractRevision(), StateChangePolicy: RuntimeStateChangeAllowedButNotRequired}
	activation := RuntimeActivationContext{ExecutionContract: execution}
	items := []RuntimeCoverageItem{{ID: "cwi-repair", SliceID: "slice-repair", SourceScope: "dodItem", SourceID: "dod-item-repair", TargetText: "behavior passes validation", Order: 1}}
	slices := []RuntimeAcceptanceSlice{{ID: "slice-repair", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-repair"}}}
	seed, err := CompileRuntimeDefinitionSeed(definition, activation, items, slices)
	if err != nil {
		t.Fatal(err)
	}
	convergence := testValidationFailureRepairConvergence()
	for index := range convergence.Routes {
		convergence.Routes[index].RepairDeclarationTargetSelectorCandidateRefs = []string{"internal/main.go", "main.go"}
	}
	definitionFocuses := CompileRuntimeDefinitionFocuses([]CoverageFocus{{
		ID: "definition-focus-repair", SliceID: "slice-repair", AssessmentScope: "dodItem", AssessmentID: "dod-item-repair", AssessmentText: "behavior passes validation",
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-repair"}, RequiredEvidenceKinds: []string{EvidenceKindValidation},
		TargetRole: "deliverable_implementation", TargetSelectorRef: "file/workspace_deliverable",
		TargetSelectorOwnerRef: "software-dev", TargetSelectorSubjectRef: "dodItem", RequireTargetGrounding: true,
		AcceptedStepKinds: []string{StepKindFileWrite, RuntimeBlockKindPatchApply, RuntimeBlockKindManagedValidationSession},
		RequireAssessment: true,
	}})
	definitionFocuses[0].ValidationFailureRepairConvergence = &convergence
	definitionFocuses = NormalizeRuntimeDefinitionFocuses(definitionFocuses)
	fileCatalog := semanticValidationRepairFileCatalog()
	shellCatalog := semanticTestOwnerCatalog("shell-repair-catalog", RuntimeModuleShell, []string{RuntimeBlockKindManagedValidationSession})
	shellCatalog.ValidationRecipes = []RuntimeValidationRecipe{{ID: "recipe/go-test", Command: "go", Args: []string{"test", "./..."}}}
	shellCatalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(shellCatalog)
	input := RuntimeValidationFailureRepairGenerationInput{
		CoverageWorkItemID: "cwi-repair", FailedFocusID: "definition-focus-repair", FailureSignature: "failure/build/1",
		TriggerOwnerModuleID: RuntimeModuleShell, TriggerOperationKind: RuntimeBlockKindManagedValidationSession,
		TriggerDiagnosticClass: ValidationDiagnosticBuildFailed, TriggerEvidenceTargetType: ValidationEvidenceTargetFailureSurface,
		TriggerOwnerDiagnosticCode: "shell_exit_nonzero",
	}
	manifest, err := CompileRuntimeSemanticPlanManifestForCoverageWithValidationFailureRepair(
		"ws-repair", "run-repair", definition, seed, definitionFocuses, execution,
		[]RuntimeOperationAdmissibility{fileCatalog, shellCatalog}, []common.CoverageWorkItemID{"cwi-repair"}, input,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.ProofObligations) != 1 || len(manifest.OwnerCatalogs) != 1 || manifest.OwnerCatalogs[0].OwnerModuleID != RuntimeModuleFile ||
		len(manifest.AllowedOwnerModuleIDs) != 1 || manifest.AllowedOwnerModuleIDs[0] != RuntimeModuleFile {
		t.Fatalf("repair manifest was not narrowed to one File proof: %+v", manifest)
	}
	batchSequence := 0
	expectedGap := RuntimeExpectedDocumentContract{
		SchemaVersion: RuntimeExpectedDocumentSchemaCurrent, DocumentKind: RuntimeProviderDocumentSemanticPlan,
		PlanStep: RuntimeSemanticPlanStepProofBatch, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		LogicalRequestID: "repair-gap-request", StablePrefixHash: "repair-gap-prefix", BatchID: "repair-batch", BatchSequence: batchSequence,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-repair"}, CoverageOrderHash: "repair-order",
		MaxOutputTokens: 4096, MaxRecoveryAttempts: 1,
	}
	expectedGap.Hash = StableRuntimeExpectedDocumentHash(expectedGap)
	gap := RuntimeDefinitionGapProviderDocument{
		DocumentKind: RuntimeProviderDocumentSemanticPlan, SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, ResultKind: RuntimeDefinitionGapResultKind,
		PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: expectedGap.BatchID, BatchSequence: &batchSequence,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-repair"}, GapKind: RuntimeDefinitionGapSemanticAmbiguity,
		OffendingItemRefs: []string{"cwi-repair"}, DiagnosticMessage: "The repair target is not clear enough to propose a state change.",
	}
	if admission := EvaluateRuntimeDefinitionGapAdmission(manifest, expectedGap, gap); admission.Disposition != RuntimeDefinitionGapAdmissionRejected ||
		admission.Code != RuntimeDefinitionGapAdmissionValidationRepairForbidden {
		t.Fatalf("repair-purpose Definition gap admission = %+v", admission)
	}
	obligation := manifest.ProofObligations[0]
	if len(obligation.RequiredEvidenceKinds) != 1 || obligation.RequiredEvidenceKinds[0] != EvidenceKindValidation || obligation.ValidationFailureRepair == nil {
		t.Fatalf("terminal validation obligation was weakened: %+v", obligation)
	}
	evidence, err := ResolveRuntimeSemanticEvidenceObligation(obligation)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Purpose != RuntimeSemanticEvidenceObligationValidationFailureRepair || len(evidence.CurrentRequiredEvidenceKinds) != 1 || evidence.CurrentRequiredEvidenceKinds[0] != EvidenceKindStateChange {
		t.Fatalf("repair evidence obligation = %+v", evidence)
	}
	if repair := obligation.ValidationFailureRepair.Normalized(); repair.FailedFocusID != input.FailedFocusID || repair.FailureSignature != input.FailureSignature ||
		repair.OwnerCatalogSnapshotID != fileCatalog.SnapshotID || repair.RepairOperationTargetSelectorRef != "workspace-relative-new-file-target" ||
		!repair.RequireFreshValidation || repair.MaxAttempts != 2 {
		t.Fatalf("sealed repair generation = %+v", repair)
	}
	schema, err := RuntimeSemanticPlanJSONSchemaForRequest(
		manifest, RuntimeSemanticPlanStepProofBatch, "repair-batch", 0, []common.CoverageWorkItemID{"cwi-repair"},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"blocks/file"`, `"state_change"`, `"workspace-relative-new-file-target"`,
		`"dod-validation-repair"`, `"internal/main.go"`, `"main.go"`,
	} {
		if !strings.Contains(schema, expected) {
			t.Fatalf("repair schema does not contain %s: %s", expected, schema)
		}
	}

	proposal := RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-repair", OwnerModuleID: RuntimeModuleFile, OperationKind: StepKindFileWrite,
		Target: RuntimeOperationTargetAuthorization{
			Policy:                    RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			OwnerSelectorRefs:         []string{"workspace-relative-new-file-target"},
			ProspectiveConstraintRefs: []string{"new-target-requires-owner-selector", "workspace-relative-path"},
			ProspectiveTargetRefs:     []string{"main.go"},
		},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
		DeclarationRef: convergence.ID,
	}
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: "repair-batch", BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-repair"},
		Proposals: []RuntimeSemanticProofProposal{proposal},
	}
	if err := document.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	wrongDeclaration := document
	wrongProposal := proposal
	wrongProposal.DeclarationRef = "other-convergence"
	wrongDeclaration.Proposals = []RuntimeSemanticProofProposal{wrongProposal}
	if err := wrongDeclaration.ValidateAgainstManifest(manifest); err == nil || !strings.Contains(err.Error(), "declarationRef") {
		t.Fatalf("repair document admitted wrong convergence declaration: %v", err)
	}
	outOfBounds := document
	outOfBoundsProposal := proposal
	outOfBoundsProposal.Target.ProspectiveTargetRefs = []string{"README.md"}
	outOfBounds.Proposals = []RuntimeSemanticProofProposal{outOfBoundsProposal}
	if err := outOfBounds.ValidateAgainstManifest(manifest); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("repair document admitted target outside declaration candidates: %v", err)
	}
	fact := RuntimeSemanticProofFeasibility{
		SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal),
		OwnerModuleID: RuntimeModuleFile, CatalogSnapshotID: fileCatalog.SnapshotID, Status: RuntimeSemanticFeasibilityFeasible,
		ObservedRevision: fileCatalog.ObservedRevision, TargetRefs: []string{"main.go"},
	}
	focuses, _, err := CompileRuntimeSemanticBindings(manifest, "logical-repair-1", document, []RuntimeSemanticProofFeasibility{fact})
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 1 || len(focuses[0].CoverageEvidenceBindings) != 1 {
		t.Fatalf("repair binding focuses = %+v", focuses)
	}
	binding := focuses[0].CoverageEvidenceBindings[0]
	if len(binding.ProspectiveTargetRefs) != 1 || binding.ProspectiveTargetRefs[0] != "main.go" ||
		binding.TargetSelectorOwnerRef != "software-dev" || binding.TargetSelectorSubjectRef != "dodItem_repair" ||
		len(binding.Proofs) != 1 || binding.Proofs[0].EvidenceKind != EvidenceKindStateChange ||
		!containsString(binding.PrerequisiteRefs, "semantic-purpose/validation_failure_repair") {
		t.Fatalf("repair binding = %+v", binding)
	}
	if focuses[0].TargetRole != "deliverable_implementation" || focuses[0].TargetSelectorRef != "workspace-relative-new-file-target" ||
		len(focuses[0].RequiredEvidenceKinds) != 1 || focuses[0].RequiredEvidenceKinds[0] != EvidenceKindStateChange {
		t.Fatalf("repair focus = %+v", focuses[0])
	}
}

func TestCompileRuntimeSemanticValidationFailureRepairRejectsWrongFocusAndCoverageCardinality(t *testing.T) {
	definition := RuntimeDefinitionContext{StartupInputHash: "startup", Revision: "definition", GoalID: "goal", GoalDefinition: "goal", DoDID: "dod", DoDDefinition: "done", CapabilityContract: []string{StepKindFileWrite, RuntimeBlockKindManagedValidationSession}}
	execution := RuntimeExecutionContract{OperationContractRevision: RuntimeOperationContractRevision(), StateChangePolicy: RuntimeStateChangeAllowedButNotRequired}
	seed, err := CompileRuntimeDefinitionSeed(definition, RuntimeActivationContext{ExecutionContract: execution},
		[]RuntimeCoverageItem{{ID: "cwi", SliceID: "slice", SourceScope: "dodItem", SourceID: "item"}},
		[]RuntimeAcceptanceSlice{{ID: "slice", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}}})
	if err != nil {
		t.Fatal(err)
	}
	convergence := testValidationFailureRepairConvergence()
	for index := range convergence.Routes {
		convergence.Routes[index].RepairOperationKinds = []string{StepKindFileWrite}
	}
	focuses := CompileRuntimeDefinitionFocuses([]CoverageFocus{{
		ID: "focus-exact", SliceID: "slice", AssessmentScope: "dodItem", AssessmentID: "item", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"},
		RequiredEvidenceKinds: []string{EvidenceKindValidation}, RequireAssessment: true,
	}})
	focuses[0].ValidationFailureRepairConvergence = &convergence
	focuses = NormalizeRuntimeDefinitionFocuses(focuses)
	fileCatalog := semanticValidationRepairFileCatalog()
	fileCatalog.OperationKinds = []string{StepKindFileWrite}
	fileCatalog.OperationVariants = fileCatalog.OperationVariants[:1]
	fileCatalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(fileCatalog)
	shellCatalog := semanticTestOwnerCatalog("shell", RuntimeModuleShell, []string{RuntimeBlockKindManagedValidationSession})
	shellCatalog.ValidationRecipes = []RuntimeValidationRecipe{{ID: "recipe", Command: "go", Args: []string{"test"}}}
	shellCatalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(shellCatalog)
	input := RuntimeValidationFailureRepairGenerationInput{
		CoverageWorkItemID: "cwi", FailedFocusID: "wrong-focus", FailureSignature: "failure",
		TriggerOwnerModuleID: RuntimeModuleShell, TriggerOperationKind: RuntimeBlockKindManagedValidationSession,
		TriggerDiagnosticClass: ValidationDiagnosticTestFailed, TriggerEvidenceTargetType: ValidationEvidenceTargetBehaviorEvidence,
		TriggerOwnerDiagnosticCode: "shell_exit_nonzero",
	}
	_, err = CompileRuntimeSemanticPlanManifestForCoverageWithValidationFailureRepair("ws", "run", definition, seed, focuses, execution, []RuntimeOperationAdmissibility{fileCatalog, shellCatalog}, []common.CoverageWorkItemID{"cwi"}, input)
	if err == nil || !strings.Contains(err.Error(), "does not match Definition focus") {
		t.Fatalf("wrong focus err=%v", err)
	}
	input.FailedFocusID = "focus-exact"
	_, err = CompileRuntimeSemanticPlanManifestForCoverageWithValidationFailureRepair("ws", "run", definition, seed, focuses, execution, []RuntimeOperationAdmissibility{fileCatalog, shellCatalog}, []common.CoverageWorkItemID{"cwi", "other"}, input)
	if err == nil || !strings.Contains(err.Error(), "exactly its rejected coverage") {
		t.Fatalf("multi-coverage repair err=%v", err)
	}
}

func semanticValidationRepairFileCatalog() RuntimeOperationAdmissibility {
	catalog := RuntimeOperationAdmissibility{
		PrerequisiteID: "file-repair-catalog", OwnerModuleID: RuntimeModuleFile,
		OperationKinds: []string{StepKindFileWrite, RuntimeBlockKindPatchApply},
		SelectorRefs:   []string{"workspace-relative-new-file-target"},
		ConstraintRefs: []string{"workspace-relative-path", "new-target-requires-owner-selector"},
		OperationVariants: []RuntimeOperationAdmissibilityVariant{
			{Kind: StepKindFileWrite, AllowedTargetPolicies: []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyProspectiveOwnerTarget}, SelectorRefs: []string{"workspace-relative-new-file-target"}, ConstraintRefs: []string{"workspace-relative-path", "new-target-requires-owner-selector"}},
			{Kind: RuntimeBlockKindPatchApply, VariantID: "file_write", AllowedTargetPolicies: []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyProspectiveOwnerTarget}, SelectorRefs: []string{"workspace-relative-new-file-target"}, ConstraintRefs: []string{"workspace-relative-path", "new-target-requires-owner-selector"}},
			{Kind: RuntimeBlockKindPatchApply, VariantID: "unified_diff", AllowedTargetPolicies: []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyProspectiveOwnerTarget}, SelectorRefs: []string{"workspace-relative-new-file-target"}, ConstraintRefs: []string{"workspace-relative-path", "new-target-requires-owner-selector"}},
		},
		ObservedRevision: "file-repair-revision",
	}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	return catalog
}
