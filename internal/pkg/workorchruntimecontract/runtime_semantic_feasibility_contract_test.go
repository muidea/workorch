package workorchruntimecontract_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeSemanticExistingTargetCorrectionSelectsOwnerReplacementKind(t *testing.T) {
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "semantic-file-catalog", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{
			{
				Kind: runtimecontract.StepKindFileWrite,
				AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
					runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
				},
				SelectorRefs: []string{"workspace-relative-new-file-target"}, ConstraintRefs: []string{"workspace-relative-path"},
			},
			{
				Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "text_replace",
				AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
					runtimecontract.RuntimeOperationTargetPolicyExactTarget,
					runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
				},
			},
		},
		TargetRefs: []string{"existing.go"}, SelectorRefs: []string{"workspace-relative-new-file-target"},
		ConstraintRefs: []string{"workspace-relative-path"}, ObservedRevision: "tree-existing",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   "ws-existing", RunID: "run-existing", DefinitionRevision: "definition-existing", DefinitionSeedHash: "seed-existing",
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		CoverageItems: []runtimecontract.RuntimeCoverageItem{{
			ID: "cwi-existing", SliceID: "slice-existing", SourceScope: "dodItem", SourceID: "dod-existing", TargetText: "revise existing deliverable", Order: 1,
		}},
		AllowedOwnerModuleIDs: []string{runtimecontract.RuntimeModuleFile},
		AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
			runtimecontract.RuntimeOperationTargetPolicyExactTarget,
			runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		},
		ProofObligations: []runtimecontract.RuntimeSemanticProofObligation{{
			CoverageWorkItemID: "cwi-existing", DeclarationRef: "semantic/cwi-existing",
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		}},
		OwnerCatalogs: []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-existing", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKind: runtimecontract.StepKindFileWrite,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"existing.go"},
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef:        "semantic/cwi-existing",
	}
	request := runtimecontract.RuntimeSemanticProofFeasibilityRequest{
		WorkspaceID: "ws-existing", RunID: "run-existing", FeasibilityBatchID: "batch-existing",
		WorkspaceDeliverableDirectory: t.TempDir(), Manifest: manifest, Proposal: proposal, OwnerCatalog: catalog,
	}
	var constraint *runtimecontract.RuntimeSemanticProposalConstraintError
	err := runtimecontract.ValidateRuntimeSemanticProofFeasibilityRequest(request, runtimecontract.RuntimeModuleFile)
	if !errors.As(err, &constraint) || constraint.Field != "operationKind" ||
		len(constraint.Allowed) != 1 || constraint.Allowed[0] != runtimecontract.RuntimeBlockKindPatchApply {
		t.Fatalf("create-only existing-target correction = %+v err=%v", constraint, err)
	}

	request.Proposal.OperationKind = runtimecontract.RuntimeBlockKindPatchApply
	request.Proposal.OperationVariantID = "text_replace"
	if err := runtimecontract.ValidateRuntimeSemanticProofFeasibilityRequest(request, runtimecontract.RuntimeModuleFile); err != nil {
		t.Fatalf("revision-bound replacement proposal rejected: %v", err)
	}
}

func TestRuntimeSemanticProofFeasibilityCommandRequiresExactOwnerAndCatalog(t *testing.T) {
	command := semanticFeasibilityCommandFixture(t)
	if err := runtimecontract.ValidateRuntimeSemanticProofFeasibilityRequest(command, runtimecontract.RuntimeModuleFile); err != nil {
		t.Fatal(err)
	}
	if err := runtimecontract.ValidateRuntimeSemanticProofFeasibilityRequest(command, runtimecontract.RuntimeModuleShell); err == nil || !strings.Contains(err.Error(), "does not match receiver") {
		t.Fatalf("owner mismatch error = %v", err)
	}
	outside := command
	outside.Proposal.Target.ProspectiveConstraintRefs = []string{"outside-owner-contract"}
	if err := runtimecontract.ValidateRuntimeSemanticProofFeasibilityRequest(outside, runtimecontract.RuntimeModuleFile); err == nil || !strings.Contains(err.Error(), "prospectiveConstraintRefs") {
		t.Fatalf("catalog escape error = %v", err)
	}
}

func TestRuntimeSemanticProofFeasibilityResultPreservesOriginalDiagnostic(t *testing.T) {
	command := semanticFeasibilityCommandFixture(t)
	diagnostic := runtimecontract.RuntimeSemanticFeasibilityDiagnostic(runtimecontract.RuntimeModuleFile, "target_exists", "the exact owner message", "workspace-1", "workspace-2", []string{"new-file.go"})
	diagnostic.OriginalMessage = "  the exact owner message  "
	response, err := runtimecontract.NewRuntimeSemanticProofFeasibilityObservation(command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, "workspace-2", []string{"new-file.go"}, diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if response.FeasibilityBatchID != command.FeasibilityBatchID || response.Observation.OriginalDiagnostic == diagnostic ||
		response.Observation.OriginalDiagnostic.OriginalMessage != "  the exact owner message  " || response.Observation.OriginalDiagnostic.ExpectedRevision != "workspace-1" {
		t.Fatalf("semantic feasibility response = %+v", response)
	}
	if _, err := runtimecontract.NewRuntimeSemanticProofFeasibilityObservation(command, runtimecontract.RuntimeSemanticFeasibilityFeasible, "workspace-2", nil, diagnostic); err == nil {
		t.Fatal("feasible result accepted failure diagnostic")
	}
}

func TestRuntimeSemanticProofFeasibilityRejectsForgedSealedOwnerFacts(t *testing.T) {
	command := semanticFeasibilityCommandFixture(t)
	valid, err := runtimecontract.NewRuntimeSemanticProofFeasibilityObservation(
		command, runtimecontract.RuntimeSemanticFeasibilityFeasible, "workspace-2", []string{"new-file.go"}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	wrongCatalog := valid.Observation.Clone()
	wrongCatalog.CatalogSnapshotID = "other-catalog"
	if err := wrongCatalog.ValidateForProposal(command.Manifest, command.Proposal); err == nil || !strings.Contains(err.Error(), "catalog identity") {
		t.Fatalf("catalog substitution error = %v", err)
	}
	forgedTarget := valid.Observation.Clone()
	forgedTarget.TargetRefs = []string{"outside.go"}
	if err := forgedTarget.ValidateForProposal(command.Manifest, command.Proposal); err == nil || !strings.Contains(err.Error(), "target does not match") {
		t.Fatalf("prospective target substitution error = %v", err)
	}

	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "shell-catalog", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		TargetRefs:     []string{"main.go"}, SelectorRefs: []string{"workspace-validation-target"},
		ValidationRecipeRefs: []string{"recipe/test"},
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
			ID: "recipe/test", OwnerModuleID: runtimecontract.RuntimeModuleShell, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			Readiness: runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "recipe-test-schema", InputBindingRef: "recipe-test-input",
			CheckRefs: []string{"check/test"}, ProofApplicability: runtimecontract.RuntimeValidationProofDirect,
			EffectScope: runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		}},
		OperationPolicies: []runtimecontract.RuntimeOperationPolicyFact{{
			ID: "recipe-test-policy", OwnerModuleID: runtimecontract.RuntimeModuleShell,
			OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			EffectScope:   runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		}},
		ObservedRevision: "shell-revision",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", DefinitionSeedHash: "seed-1",
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		CoverageItems:             []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "works", Order: 1}},
		AllowedOwnerModuleIDs:     []string{runtimecontract.RuntimeModuleShell},
		AllowedOperationKinds:     []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		AllowedTargetPolicies:     []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope},
		OwnerCatalogs:             []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}
	focus := runtimecontract.RuntimeDefinitionFocus{
		ID: "validation-focus", AssessmentScope: "dodItem", AssessmentID: "dod-1", AssessmentText: "works",
		CoverageWorkItemIDs:    []common.CoverageWorkItemID{"cwi-1"},
		RequiredEvidenceKinds:  []string{runtimecontract.EvidenceKindValidation},
		AcceptedStepKinds:      []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		EvidenceDeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef("dodItem", "dod-1"), ValidationRecipeID: "recipe/test", RequiredCheckRefs: []string{"check/test"},
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(manifest.DefinitionRevision, manifest.CoverageItems, []runtimecontract.RuntimeDefinitionFocus{focus})
	if err != nil || len(requirements) != 1 {
		t.Fatalf("compile exact validation requirement: requirements=%+v err=%v", requirements, err)
	}
	requirement := requirements[0]
	attestations := make([]runtimecontract.RuntimeValidationCheckAttestationBinding, 0, len(requirement.RequiredCheckRefs))
	for index, checkRef := range requirement.RequiredCheckRefs {
		attestations = append(attestations, runtimecontract.RuntimeValidationCheckAttestationBinding{
			RequiredCheckRef: checkRef, ObservationProtocolRef: "test/owner-native-validation/v1",
			NativeCheckRef: fmt.Sprintf("native-check-%d", index+1),
		})
	}
	binding := runtimecontract.RuntimeValidationProofRequirementBinding{
		RequirementHash: requirement.Hash, DefinitionRevision: requirement.DefinitionRevision,
		CoverageWorkItemID: requirement.CoverageWorkItemID, DeclarationRef: requirement.DeclarationRef,
		ValidationRecipeID: requirement.ValidationRecipeID, RequiredCheckRefs: requirement.RequiredCheckRefs,
		CheckAttestations: attestations,
	}
	manifest.OwnerCatalogs[0].ValidationSurfaces[0].CheckRefs = append([]string(nil), requirement.RequiredCheckRefs...)
	manifest.OwnerCatalogs[0].ValidationSurfaces[0].ProofRequirementBindings = []runtimecontract.RuntimeValidationProofRequirementBinding{binding}
	manifest.OwnerCatalogs[0].SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(manifest.OwnerCatalogs[0])
	catalog = manifest.OwnerCatalogs[0]
	manifest.ProofObligations = []runtimecontract.RuntimeSemanticProofObligation{{
		CoverageWorkItemID: "cwi-1", DeclarationRef: requirement.DeclarationRef,
		ValidationProofRequirement: &requirement, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
	}}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-1", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{"workspace-validation-target"},
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession},
		ValidationRecipeID:    "recipe/test", DeclarationRef: requirement.DeclarationRef,
	}
	base := runtimecontract.RuntimeSemanticProofFeasibility{
		SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal),
		OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: catalog.SnapshotID,
		Status: runtimecontract.RuntimeSemanticFeasibilityFeasible, ObservedRevision: catalog.ObservedRevision,
		ValidationRecipeID: proposal.ValidationRecipeID, RequiredCheckRefs: append([]string(nil), requirement.RequiredCheckRefs...),
	}
	if err := base.ValidateForProposal(manifest, proposal); err != nil {
		t.Fatal(err)
	}
	selectorEscape := base.Clone()
	selectorEscape.TargetRefs = []string{"outside.go"}
	if err := selectorEscape.ValidateForProposal(manifest, proposal); err == nil || !strings.Contains(err.Error(), "target is outside") {
		t.Fatalf("selector-only target escape error = %v", err)
	}
	forgedCheck := base.Clone()
	forgedCheck.RequiredCheckRefs = []string{"unsealed-check"}
	if err := forgedCheck.ValidateForProposal(manifest, proposal); err == nil || !strings.Contains(err.Error(), "check is outside") {
		t.Fatalf("check substitution error = %v", err)
	}
}

func semanticFeasibilityCommandFixture(t *testing.T) runtimecontract.RuntimeSemanticProofFeasibilityRequest {
	t.Helper()
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", DefinitionSeedHash: "seed-1",
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
		CoverageItems:             []runtimecontract.RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "works", Order: 1}},
		AllowedOwnerModuleIDs:     []string{runtimecontract.RuntimeModuleFile}, AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		ProofObligations: []runtimecontract.RuntimeSemanticProofObligation{{
			CoverageWorkItemID: "cwi-1", DeclarationRef: "semantic/dod-1",
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		}},
	}
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-1", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget, OwnerSelectorRefs: []string{"workspace-relative-new-file-target"},
			ProspectiveConstraintRefs: []string{"workspace-relative-path"}, ProspectiveTargetRefs: []string{"new-file.go"},
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, DeclarationRef: "semantic/dod-1",
	}
	catalog, err := runtimecontract.NewRuntimeOperationAdmissibility("semantic-catalog-file", runtimecontract.RuntimeModuleFile, []string{runtimecontract.StepKindFileWrite}, []string{"main.go"},
		[]string{"workspace-relative-new-file-target"}, []string{"workspace-relative-path"}, "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	catalog.OperationVariants = []runtimecontract.RuntimeOperationAdmissibilityVariant{{
		Kind: runtimecontract.StepKindFileWrite,
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
			runtimecontract.RuntimeOperationTargetPolicyExactTarget,
			runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		},
		SelectorRefs: []string{"workspace-relative-new-file-target"}, ConstraintRefs: []string{"workspace-relative-path"},
	}}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest.OwnerCatalogs = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	return runtimecontract.RuntimeSemanticProofFeasibilityRequest{
		WorkspaceID: "ws-1", RunID: "run-1", FeasibilityBatchID: "batch-1", WorkspaceDeliverableDirectory: t.TempDir(),
		Manifest: manifest, Proposal: proposal, OwnerCatalog: catalog,
	}
}
