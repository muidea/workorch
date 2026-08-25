package workorchruntimecontract

import (
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

const runtimeSemanticBindingTestRequestID = "semantic-binding-test-request"

func TestCompileRuntimeSemanticBindingsCombinesIndependentProofsForOneCWI(t *testing.T) {
	manifest, document, facts, _ := semanticIndependentProofSetFixture(t)
	focuses, contract, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 1 || len(focuses[0].CoverageEvidenceBindings) != 1 || len(contract.Bindings) != 1 {
		t.Fatalf("independent proofs did not compile to one canonical CWI binding: focuses=%+v contract=%+v", focuses, contract)
	}
	binding := focuses[0].CoverageEvidenceBindings[0]
	if len(binding.Proofs) != 2 || !sameStrings(binding.EvidenceKinds(), []string{EvidenceKindStateChange, EvidenceKindValidation}) {
		t.Fatalf("binding did not retain both independent proofs: %+v", binding)
	}
	validation, found := CoverageEvidenceProofForKind(binding, EvidenceKindValidation)
	if !found || validation.ValidationRecipeID != "recipe/validate" || !sameStrings(validation.RequiredCheckRefs, []string{"check/validate"}) {
		t.Fatalf("binding lost validation proof detail: %+v", binding)
	}
}

func TestCompileRuntimeSemanticBindingsDoesNotPromoteTargetToArtifact(t *testing.T) {
	manifest, document, facts, _ := semanticIndependentProofSetFixture(t)
	focuses, _, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 1 || len(focuses[0].CoverageEvidenceBindings) != 1 {
		t.Fatalf("compiled focuses = %+v", focuses)
	}
	binding := focuses[0].CoverageEvidenceBindings[0]
	if !sameStrings(binding.TargetRefs, []string{"main.go"}) {
		t.Fatalf("binding targets = %v", binding.TargetRefs)
	}
	if len(focuses[0].ArtifactRefs) != 0 {
		t.Fatalf("focus target was promoted to artifact: %v", focuses[0].ArtifactRefs)
	}
	for _, proof := range binding.Proofs {
		if len(proof.ArtifactRefs) != 0 {
			t.Fatalf("proof target was promoted to artifact: %+v", proof)
		}
	}
}

func TestCompileRuntimeSemanticBindingsSealsSourceProofRequirementsOnUniqueValidationProof(t *testing.T) {
	manifest, document, facts, source := semanticIndependentProofSetFixture(t)
	source.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{{
		OwnerModuleID: RuntimeModuleShell, FactKind: RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: RuntimeBlockKindManagedValidationSession,
	}}
	source.OwnerFactConvergences = []RuntimeOwnerFactConvergence{{Requirement: source.RequiredOwnerFacts[0], RequirePreexisting: true}}
	source.MetricConstraints = []string{"metric/latency"}
	source.ArtifactRefs = []string{"workorch://artifact/validation-report"}
	manifest = sealSemanticTestManifestWithSource(t, manifest, source)
	document.ManifestHash = manifest.Hash
	for index := range facts {
		facts[index] = semanticFeasibilityForTest(t, manifest, document.Proposals[index], facts[index].TargetRefs, facts[index].ValidationRecipeID, facts[index].RequiredCheckRefs)
	}
	focuses, _, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, facts)
	if err != nil {
		t.Fatal(err)
	}
	binding := focuses[0].CoverageEvidenceBindings[0]
	stateChange, found := CoverageEvidenceProofForKind(binding, EvidenceKindStateChange)
	if !found || len(stateChange.MetricConstraints) != 0 || len(stateChange.ArtifactRefs) != 0 {
		t.Fatalf("source proof requirements leaked to state-change owner: %+v", stateChange)
	}
	validation, found := CoverageEvidenceProofForKind(binding, EvidenceKindValidation)
	if !found || !sameStrings(validation.MetricConstraints, source.MetricConstraints) || !sameStrings(validation.ArtifactRefs, source.ArtifactRefs) {
		t.Fatalf("validation proof lost source requirements: %+v", validation)
	}
	if !sameStrings(focuses[0].MetricConstraints, source.MetricConstraints) || !sameStrings(focuses[0].ArtifactRefs, source.ArtifactRefs) {
		t.Fatalf("compiled focus lost source requirements: %+v", focuses[0])
	}
	if !sameRuntimeOwnerFactRequirements(focuses[0].RequiredOwnerFacts, source.RequiredOwnerFacts) {
		t.Fatalf("compiled focus lost source owner-fact postconditions: %+v", focuses[0])
	}
	if !SameRuntimeOwnerFactConvergences(focuses[0].OwnerFactConvergences, source.OwnerFactConvergences) {
		t.Fatalf("compiled focus lost source owner-fact convergence: %+v", focuses[0])
	}
}

func TestSemanticBindingMissingOwnerFactRequiresExactSealedAcquisition(t *testing.T) {
	manifest, document, proposal := semanticMissingOwnerFactAcquisitionFixture(t)
	obligation := manifest.ProofObligations[0]
	evidenceObligation, err := ResolveRuntimeSemanticEvidenceObligation(obligation)
	if err != nil {
		t.Fatal(err)
	}
	if !sameStrings(evidenceObligation.TerminalRequiredEvidenceKinds, []string{EvidenceKindValidation}) ||
		!sameStrings(evidenceObligation.CurrentRequiredEvidenceKinds, []string{EvidenceKindStateChange}) ||
		evidenceObligation.Purpose != RuntimeSemanticEvidenceObligationOwnerFactAcquisition {
		t.Fatalf("owner-fact evidence obligation = %+v", evidenceObligation)
	}
	fact := semanticFeasibilityForTest(t, manifest, proposal, []string{"validation-readiness.sh"}, "", nil)
	focuses, _, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, []RuntimeSemanticProofFeasibility{fact})
	if err != nil {
		t.Fatalf("exact sealed owner-fact acquisition was rejected: %v", err)
	}
	if len(focuses) != 1 || !sameStrings(focuses[0].RequiredEvidenceKinds, []string{EvidenceKindStateChange}) ||
		!sameStrings(obligation.RequiredEvidenceKinds, []string{EvidenceKindValidation}) {
		t.Fatalf("compiled acquisition weakened terminal evidence: focus=%+v obligation=%+v", focuses, obligation)
	}

	terminalInsteadOfCurrent := document.Clone()
	terminalInsteadOfCurrent.Proposals[0].RequiredEvidenceKinds = []string{EvidenceKindValidation}
	if err := terminalInsteadOfCurrent.ValidateAgainstManifest(manifest); err == nil {
		t.Fatal("terminal validation proof was accepted in place of the current owner-fact acquisition")
	}

	wrongTarget := document.Clone()
	wrongTarget.Proposals[0].Target.ProspectiveTargetRef = "provider-selected-target.sh"
	var targetViolation *RuntimeSemanticProposalViolationError
	if err := wrongTarget.ValidateAgainstManifest(manifest); !errors.As(err, &targetViolation) || targetViolation.Violation().Path != "$.proposals[0].target.prospectiveTargetRef" {
		t.Fatalf("provider-selected target was not rejected as an exact typed violation: violation=%+v err=%v", targetViolation, err)
	}

	changedTargetAuthority := manifest.Clone()
	changedTargetAuthority.ProofObligations[0].DeclarationProspectiveTargetRef = "other-activation-target.sh"
	changedTargetAuthority.ProofObligations[0].OwnerFactAcquisition.ProspectiveTargetRef = "other-activation-target.sh"
	changedTargetAuthority.Hash = StableRuntimeSemanticPlanManifestHash(changedTargetAuthority)
	if changedTargetAuthority.Hash == manifest.Hash {
		t.Fatal("semantic manifest hash ignored the exact prospective target authority")
	}
	if err := changedTargetAuthority.Validate(); err != nil {
		t.Fatalf("self-consistent changed prospective target authority was rejected: %v", err)
	}

	parallel := document.Clone()
	second := proposal.Clone()
	second.Target.ProspectiveTargetRef = "other.go"
	parallel.Proposals = append(parallel.Proposals, second)
	if err := parallel.ValidateAgainstManifest(manifest); err == nil {
		t.Fatalf("parallel/unsealed owner-fact acquisition was accepted: %v", err)
	}
}

func semanticMissingOwnerFactAcquisitionFixture(t *testing.T) (RuntimeSemanticPlanManifest, RuntimeSemanticPlanDocument, RuntimeSemanticProofProposal) {
	t.Helper()
	manifest, document, _, source := semanticIndependentProofSetFixture(t)
	requirement := RuntimeOwnerFactRequirement{
		OwnerModuleID: RuntimeModuleShell, FactKind: RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: RuntimeBlockKindManagedValidationSession,
	}
	const declarationSelector = "file/workspace-deliverable"
	const operationSelector = "file/workspace-relative-create"
	const prospectiveTarget = "validation-readiness.sh"
	convergence := RuntimeOwnerFactConvergence{
		Requirement: requirement, MissingTrigger: RuntimeOwnerFactMissingTrigger,
		ProspectiveSatisfactionContractRef: "shell/managed-validation/workspace-manifest-v1",
		AcquisitionOwnerModuleID:           RuntimeModuleFile, AcquisitionOperationKind: StepKindFileWrite,
		AcquisitionTargetPolicy: RuntimeOperationTargetPolicyProspectiveOwnerTarget, AcquisitionTargetSelectorRef: operationSelector,
		TargetSelectorRef: declarationSelector, TargetSelectorOwnerRef: "profile/test", TargetSelectorSubjectRef: "validation_preparation",
		ReobserveOwnerModuleID: RuntimeModuleShell, ReobserveFactKind: RuntimeOwnerFactSelectableValidationSurface,
		MaxAcquisitionAttempts: 1,
	}
	// The Definition's terminal proof remains validation. The sealed acquisition
	// is a distinct, current state-change obligation and must not overwrite it.
	source.RequiredEvidenceKinds = []string{EvidenceKindValidation}
	source.AcceptedStepKinds = []string{StepKindFileWrite, RuntimeBlockKindManagedValidationSession}
	source.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{requirement}
	source.OwnerFactConvergences = []RuntimeOwnerFactConvergence{convergence}
	source.TargetSelectorRef = declarationSelector
	source.TargetSelectorOwnerRef = convergence.TargetSelectorOwnerRef
	source.TargetSelectorSubjectRef = convergence.TargetSelectorSubjectRef
	source.TargetRole = "validation-input"
	source.RequireTargetGrounding = true
	source.ProspectiveTargetRef = prospectiveTarget

	for index := range manifest.OwnerCatalogs {
		if manifest.OwnerCatalogs[index].OwnerModuleID != RuntimeModuleShell {
			continue
		}
		manifest.OwnerCatalogs[index].ValidationRecipeRefs = nil
		for surfaceIndex := range manifest.OwnerCatalogs[index].ValidationSurfaces {
			surface := &manifest.OwnerCatalogs[index].ValidationSurfaces[surfaceIndex]
			surface.Readiness = RuntimeValidationSurfacePrerequisiteRequired
			surface.ReadinessReason = "validation input is not present"
			surface.InputBindingRef = ""
			surface.PrerequisiteRef = "workspace/validation-input"
			surface.ProspectiveSatisfactionContractRef = convergence.ProspectiveSatisfactionContractRef
		}
		manifest.OwnerCatalogs[index].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(manifest.OwnerCatalogs[index])
	}
	manifest = sealSemanticTestManifestWithSource(t, manifest, source)
	proposal := document.Proposals[0].Clone()
	proposal.Target = RuntimeOperationTargetAuthorization{
		Policy: RuntimeOperationTargetPolicyProspectiveOwnerTarget, OwnerSelectorRefs: []string{operationSelector},
		ProspectiveConstraintRefs: []string{"workspace-relative-path"}, ProspectiveTargetRef: prospectiveTarget,
	}
	document.ManifestHash = manifest.Hash
	document.Proposals = []RuntimeSemanticProofProposal{proposal}
	return manifest, document, proposal
}

func TestCompileRuntimeSemanticBindingsRejectsAmbiguousSourceProofRequirementOwner(t *testing.T) {
	proofSet := RuntimeSemanticCWIProofSet{CoverageWorkItemID: "cwi-001", Proposals: []RuntimeSemanticProofProposal{
		{RequiredEvidenceKinds: []string{EvidenceKindValidation}},
		{RequiredEvidenceKinds: []string{EvidenceKindValidation}},
	}}
	source := CoverageFocus{ArtifactRefs: []string{"workorch://artifact/validation-report"}}
	_, err := runtimeSemanticSourceProofRequirementIndex(proofSet, &source)
	if err == nil || !strings.Contains(err.Error(), "cannot uniquely assign source metric or artifact requirements") {
		t.Fatalf("ambiguous source proof requirement owner was accepted: %v", err)
	}
}

func TestCompileRuntimeSemanticBindingsCombinesProspectiveCreationAndExactValidation(t *testing.T) {
	manifest, document, facts, source := semanticIndependentProofSetFixture(t)
	document.Proposals[0].Target = RuntimeOperationTargetAuthorization{
		Policy: RuntimeOperationTargetPolicyProspectiveOwnerTarget, OwnerSelectorRefs: []string{"file/workspace-relative-create"},
		ProspectiveConstraintRefs: []string{"new-target-requires-owner-selector"}, ProspectiveTargetRef: "new-file.go",
	}
	document.Proposals[1].Target = RuntimeOperationTargetAuthorization{
		Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"new-file.go"},
	}
	fileCatalog := &manifest.OwnerCatalogs[0]
	fileCatalog.SelectorRefs = append(fileCatalog.SelectorRefs, "file/workspace-relative-create")
	fileCatalog.ConstraintRefs = append(fileCatalog.ConstraintRefs, "new-target-requires-owner-selector")
	fileCatalog.OperationVariants = []RuntimeOperationAdmissibilityVariant{{
		Kind: document.Proposals[0].OperationKind, VariantID: document.Proposals[0].OperationVariantID,
		AllowedTargetPolicies: []RuntimeOperationTargetPolicy{RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		SelectorRefs:          []string{"file/workspace-relative-create"}, ConstraintRefs: []string{"new-target-requires-owner-selector"},
	}}
	manifest.OwnerCatalogs[1].TargetRefs = []string{"new-file.go"}
	manifest = sealSemanticTestManifest(t, manifest)
	document.ManifestHash = manifest.Hash
	facts[0] = semanticFeasibilityForTest(t, manifest, document.Proposals[0], []string{"new-file.go"}, "", nil)
	facts[1] = semanticFeasibilityForTest(t, manifest, document.Proposals[1], []string{"new-file.go"}, "recipe/validate", []string{"check/validate"})

	manifest = sealSemanticTestManifestWithSource(t, manifest, source)
	document.ManifestHash = manifest.Hash
	facts[0] = semanticFeasibilityForTest(t, manifest, document.Proposals[0], []string{"new-file.go"}, "", nil)
	facts[1] = semanticFeasibilityForTest(t, manifest, document.Proposals[1], []string{"new-file.go"}, "recipe/validate", []string{"check/validate"})
	focuses, _, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 1 || focuses[0].ProspectiveTargetRef != "new-file.go" || focuses[0].TargetSelectorRef != "file/workspace-relative-create" || len(focuses[0].TargetRefs) != 0 {
		t.Fatalf("prospective creation and exact validation target surface = %+v", focuses)
	}

	mismatched := document.Clone()
	mismatched.Proposals[1].Target = RuntimeOperationTargetAuthorization{
		Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"other.go"},
	}
	mismatchedFact := semanticFeasibilityForTest(t, manifest, mismatched.Proposals[1], []string{"other.go"}, "recipe/validate", []string{"check/validate"})
	if _, _, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, mismatched, []RuntimeSemanticProofFeasibility{facts[0], mismatchedFact}); err == nil {
		t.Fatalf("mismatched creation and validation targets were accepted: %v", err)
	}
}

func TestCompileRuntimeSemanticBindingsUsesDeclaredSelectorOwnerAcrossProofOwners(t *testing.T) {
	manifest, document, _, source := semanticIndependentProofSetFixture(t)
	selector := "file/unique_workspace_entry"
	for index := range document.Proposals {
		document.Proposals[index].Target = RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerSelectorRefs: []string{selector}}
	}
	for index := range manifest.OwnerCatalogs {
		manifest.OwnerCatalogs[index].SelectorRefs = append(manifest.OwnerCatalogs[index].SelectorRefs, selector)
		for variantIndex := range manifest.OwnerCatalogs[index].OperationVariants {
			manifest.OwnerCatalogs[index].OperationVariants[variantIndex].SelectorRefs = append(manifest.OwnerCatalogs[index].OperationVariants[variantIndex].SelectorRefs, selector)
		}
	}
	manifest = sealSemanticTestManifest(t, manifest)
	document.ManifestHash = manifest.Hash
	facts := make([]RuntimeSemanticProofFeasibility, 0, len(document.Proposals))
	for _, proposal := range document.Proposals {
		checks := proposal.RequiredCheckRefs
		if proposal.ValidationRecipeID != "" && len(checks) == 0 {
			checks = []string{"check/validate"}
		}
		facts = append(facts, semanticFeasibilityForTest(t, manifest, proposal, nil, proposal.ValidationRecipeID, checks))
	}
	source.TargetSelectorRef = selector
	source.TargetSelectorOwnerRef = RuntimeModuleFile
	source.TargetSelectorSubjectRef = "deliverable"
	manifest = sealSemanticTestManifestWithSource(t, manifest, source)
	document.ManifestHash = manifest.Hash
	for index := range facts {
		checks := document.Proposals[index].RequiredCheckRefs
		if document.Proposals[index].ValidationRecipeID != "" && len(checks) == 0 {
			checks = []string{"check/validate"}
		}
		facts[index] = semanticFeasibilityForTest(t, manifest, document.Proposals[index], nil, document.Proposals[index].ValidationRecipeID, checks)
	}
	bindings, _, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, facts)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].TargetSelectorRef != selector || bindings[0].TargetSelectorOwnerRef != RuntimeModuleFile {
		t.Fatalf("declared selector owner was not preserved: %+v", bindings)
	}
}

func TestCompileRuntimeSemanticBindingsRejectsMissingSourceRequiredValidationProof(t *testing.T) {
	manifest, document, facts, source := semanticIndependentProofSetFixture(t)
	document.Proposals = document.Proposals[:1]
	facts = facts[:1]
	manifest = sealSemanticTestManifestWithSource(t, manifest, source)
	document.ManifestHash = manifest.Hash
	if _, _, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, facts); err == nil ||
		!strings.Contains(err.Error(), "does not cover source required evidence kinds") {
		t.Fatalf("semantic plan without source-required validation proof was accepted: %v", err)
	}
}

func TestCompileRuntimeSemanticBindingsRejectsProposalVariantOutsideSourceDeclaration(t *testing.T) {
	source := CoverageFocus{ID: "focus-source", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, AcceptedStepKinds: []string{StepKindGitWrite}, AcceptedOperationVariants: []RuntimeOperationVariantRef{{Kind: StepKindGitWrite, VariantID: "commit"}}}
	proofSet := RuntimeSemanticCWIProofSet{CoverageWorkItemID: "cwi-1", Proposals: []RuntimeSemanticProofProposal{{
		CoverageWorkItemID: "cwi-1", OwnerModuleID: RuntimeModuleGit, OperationKind: StepKindGitWrite,
		OperationVariantID: "add", RequiredEvidenceKinds: []string{EvidenceKindStateChange},
	}}}
	if err := validateRuntimeSemanticProofSetSource(proofSet, &source); err == nil || !strings.Contains(err.Error(), "operation variant \"add\" is outside source declaration") {
		t.Fatalf("source variant mismatch error = %v", err)
	}
}

func TestValidateRuntimeSemanticPlanSourceDeclarationsDoesNotReusePriorGenerationOperationKind(t *testing.T) {
	proofSet := RuntimeSemanticCWIProofSet{
		CoverageWorkItemID: "cwi-1",
		Proposals: []RuntimeSemanticProofProposal{{
			CoverageWorkItemID: "cwi-1", OperationKind: RuntimeBlockKindPatchApply,
			RequiredEvidenceKinds: []string{EvidenceKindStateChange},
		}},
	}
	priorGeneration := CoverageFocus{
		ID: "semantic-focus-generation-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange}, AcceptedStepKinds: []string{StepKindFileWrite},
	}
	if err := validateRuntimeSemanticProofSetSource(proofSet, &priorGeneration); err == nil || !strings.Contains(err.Error(), "outside source declaration") {
		t.Fatalf("prior generation must demonstrate the old false declaration gate, got %v", err)
	}
	definitionDeclaration := CoverageFocus{
		ID: "definition-focus-cwi-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange},
	}
	if err := validateRuntimeSemanticProofSetSource(proofSet, &definitionDeclaration); err != nil {
		t.Fatalf("immutable declaration must allow an owner-feasible next-generation operation kind: %v", err)
	}
}

func TestRuntimeSemanticFeasibilitySetUsesProposalIdentityWithinOneCWI(t *testing.T) {
	manifest, document, facts, _ := semanticIndependentProofSetFixture(t)
	if err := ValidateRuntimeSemanticFeasibilitySet(manifest, document, facts, true); err != nil {
		t.Fatalf("independent proof feasibility facts for one CWI must be accepted: %v", err)
	}
	duplicate := append(append([]RuntimeSemanticProofFeasibility(nil), facts...), facts[0])
	if err := ValidateRuntimeSemanticFeasibilitySet(manifest, document, duplicate, true); err == nil ||
		!strings.Contains(err.Error(), "duplicates proof") {
		t.Fatalf("duplicate proposal feasibility was accepted: %v", err)
	}
}

func TestCompileRuntimeSemanticBindingsSealsOneBindingPerCWI(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	proposal := semanticContractTestProposal("cwi-001")
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch,
		BatchID: "batch-1", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		Proposals: []RuntimeSemanticProofProposal{proposal},
	}
	fact := RuntimeSemanticProofFeasibility{
		SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID,
		CatalogSnapshotID: manifest.OwnerCatalogs[0].SnapshotID, Status: RuntimeSemanticFeasibilityFeasible, ObservedRevision: "workspace-1", TargetRefs: []string{proposal.Target.ProspectiveTargetRef},
	}
	focuses, contract, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, []RuntimeSemanticProofFeasibility{fact})
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 1 || len(focuses[0].CoverageWorkItemIDs) != 1 || len(focuses[0].CoverageEvidenceBindings) != 1 ||
		focuses[0].ProspectiveTargetRef != "new-file.go" || focuses[0].TargetSelectorRef != "file/workspace-relative-create" {
		t.Fatalf("compiled focuses = %+v", focuses)
	}
	if len(contract.Bindings) != 1 || contract.BindingSetHash == "" || contract.Bindings[0].BindingHash != focuses[0].CoverageEvidenceBindings[0].Hash {
		t.Fatalf("compiled reconciliation = %+v", contract)
	}
}

func TestCompileRuntimeSemanticBindingsRetainsSourceSelectorCandidates(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	proposal := semanticContractTestProposal("cwi-001")
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch,
		BatchID: "batch-selector", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		Proposals: []RuntimeSemanticProofProposal{proposal},
	}
	fact := RuntimeSemanticProofFeasibility{
		SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID,
		CatalogSnapshotID: manifest.OwnerCatalogs[0].SnapshotID, Status: RuntimeSemanticFeasibilityFeasible,
		ObservedRevision: manifest.OwnerCatalogs[0].ObservedRevision, TargetRefs: []string{proposal.Target.ProspectiveTargetRef},
	}
	source := CoverageFocus{
		ID: "source-selector", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange},
		TargetRole:            "deliverable", TargetDiscoverySources: []string{RuntimeValidationDiscoverySourceWorkspaceManifest},
		TargetSelectorRef: "file/workspace-relative-create", TargetSelectorOwnerRef: "profile-test", TargetSelectorSubjectRef: "deliverable",
		TargetSelectorCandidateRefs: []string{"main.go"}, RequireTargetGrounding: true,
	}
	manifest = sealSemanticTestManifestWithSource(t, manifest, source)
	document.ManifestHash = manifest.Hash
	fact = semanticFeasibilityForTest(t, manifest, proposal, []string{proposal.Target.ProspectiveTargetRef}, "", nil)
	focuses, contract, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, []RuntimeSemanticProofFeasibility{fact})
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 1 || len(focuses[0].CoverageEvidenceBindings) != 1 {
		t.Fatalf("compiled focuses = %+v", focuses)
	}
	binding := focuses[0].CoverageEvidenceBindings[0]
	if !SameRuntimeTargetSelectorCandidateRefs(binding.TargetSelectorCandidateRefs, []string{"main.go"}) ||
		!SameRuntimeTargetSelectorCandidateRefs(focuses[0].TargetSelectorCandidateRefs, []string{"main.go"}) {
		t.Fatalf("compiled selector declaration = binding:%+v focus:%+v", binding, focuses[0])
	}
	if len(contract.Bindings) != 1 || !SameRuntimeTargetSelectorCandidateRefs(contract.Bindings[0].TargetSelectorCandidateRefs, []string{"main.go"}) {
		t.Fatalf("reconciliation contract lost selector candidates: %+v", contract)
	}
}

func TestCompileRuntimeSemanticBindingsRejectsPartialOrNonFeasibleBatch(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	proposal := semanticContractTestProposal("cwi-001")
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch,
		BatchID: "batch-1", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		Proposals: []RuntimeSemanticProofProposal{proposal},
	}
	if _, _, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, nil); err == nil {
		t.Fatal("partial feasibility batch was sealed")
	}
	fact := RuntimeSemanticProofFeasibility{
		SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID,
		CatalogSnapshotID: manifest.OwnerCatalogs[0].SnapshotID, Status: RuntimeSemanticFeasibilityInfeasible, ObservedRevision: "workspace-1",
		OriginalDiagnostic: semanticContractTestDiagnostic(),
	}
	if _, _, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, []RuntimeSemanticProofFeasibility{fact}); err == nil {
		t.Fatal("non-feasible owner batch was sealed")
	}
}

func TestCompileRuntimeSemanticBindingsRequiresConcreteProspectiveOwnerFact(t *testing.T) {
	manifest := semanticContractTestManifest(t)
	proposal := semanticContractTestProposal("cwi-001")
	document := RuntimeSemanticPlanDocument{SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: "batch-1", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, Proposals: []RuntimeSemanticProofProposal{proposal}}
	fact := RuntimeSemanticProofFeasibility{SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: manifest.OwnerCatalogs[0].SnapshotID, Status: RuntimeSemanticFeasibilityFeasible, ObservedRevision: "workspace-1", TargetRefs: []string{proposal.Target.ProspectiveTargetRef}}
	focuses, _, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, []RuntimeSemanticProofFeasibility{fact})
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 1 || focuses[0].ProspectiveTargetRef != "new-file.go" {
		t.Fatalf("compiled focus = %+v", focuses)
	}
	fact.TargetRefs = nil
	if _, _, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, []RuntimeSemanticProofFeasibility{fact}); err == nil {
		t.Fatal("feasible prospective owner fact without concrete target was accepted")
	}
}

func TestCompileRuntimeSemanticBindingsScopesBindingIdentityToExactSemanticGeneration(t *testing.T) {
	proposal := semanticContractTestProposal("cwi-001")
	compile := func(manifest RuntimeSemanticPlanManifest, logicalRequestID string) CoverageEvidenceBinding {
		t.Helper()
		document := RuntimeSemanticPlanDocument{
			SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
			DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: RuntimeSemanticPlanStepProofBatch,
			BatchID: "batch-1", BatchSequence: 0, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			Proposals: []RuntimeSemanticProofProposal{proposal},
		}
		fact := RuntimeSemanticProofFeasibility{
			SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
			CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID,
			CatalogSnapshotID: manifest.OwnerCatalogs[0].SnapshotID, Status: RuntimeSemanticFeasibilityFeasible,
			ObservedRevision: manifest.OwnerCatalogs[0].ObservedRevision, TargetRefs: []string{proposal.Target.ProspectiveTargetRef},
		}
		focuses, _, err := CompileRuntimeSemanticBindings(manifest, logicalRequestID, document, []RuntimeSemanticProofFeasibility{fact})
		if err != nil {
			t.Fatal(err)
		}
		return focuses[0].CoverageEvidenceBindings[0]
	}

	firstManifest := semanticContractTestManifest(t)
	first := compile(firstManifest, "semantic-generation-request-1")
	replayed := compile(firstManifest, "semantic-generation-request-1")
	if first.ID != replayed.ID || first.FocusID != replayed.FocusID || first.Hash != replayed.Hash {
		t.Fatalf("same semantic generation was not deterministic: first=%+v replayed=%+v", first, replayed)
	}
	second := compile(firstManifest, "semantic-generation-request-2")
	if first.ID == second.ID || first.FocusID == second.FocusID || first.Hash == second.Hash {
		t.Fatalf("binding authority was reused across logical semantic requests: first=%+v second=%+v", first, second)
	}

	secondManifest := firstManifest.Clone()
	secondManifest.OwnerCatalogs[0].ObservedRevision = "workspace-2"
	secondManifest.OwnerCatalogs[0].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(secondManifest.OwnerCatalogs[0])
	secondManifest.Hash = StableRuntimeSemanticPlanManifestHash(secondManifest)
	third := compile(secondManifest, "semantic-generation-request-3")

	if second.ID == third.ID {
		t.Fatalf("binding ID %q was reused across semantic generations", first.ID)
	}
	if second.FocusID == third.FocusID {
		t.Fatalf("focus ID %q was reused across semantic generations", first.FocusID)
	}
	if second.Hash == third.Hash {
		t.Fatalf("binding hash %q did not retain changed catalog lineage", first.Hash)
	}
}

func TestCompileRuntimeSemanticBindingsRequiresLogicalRequestIdentity(t *testing.T) {
	manifest, document, facts, _ := semanticIndependentProofSetFixture(t)
	if _, _, err := CompileRuntimeSemanticBindings(manifest, "", document, facts); err == nil ||
		!strings.Contains(err.Error(), "logical-request generation identity") {
		t.Fatalf("semantic binding without logical request identity was accepted: %v", err)
	}
}

func semanticIndependentProofSetFixture(t *testing.T) (RuntimeSemanticPlanManifest, RuntimeSemanticPlanDocument, []RuntimeSemanticProofFeasibility, CoverageFocus) {
	t.Helper()
	manifest := semanticContractTestManifest(t)
	shellCatalog := semanticTestOwnerCatalog("semantic-shell-catalog", RuntimeModuleShell, []string{RuntimeBlockKindManagedValidationSession})
	shellCatalog.TargetRefs = []string{"main.go"}
	shellCatalog.ValidationRecipeRefs = []string{"recipe/validate"}
	shellCatalog.ValidationSurfaces = []RuntimeValidationSurface{{
		ID: "recipe/validate", OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
		Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: "semantic-binding-input-schema", InputBindingRef: "semantic-binding-input",
		CheckRefs: []string{"check/validate"}, ProofApplicability: RuntimeValidationProofDirect,
		EffectScope: RuntimeEffectEphemeralSession, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}}
	shellCatalog.OperationPolicies = []RuntimeOperationPolicyFact{{
		ID: "semantic-binding-managed-validation", OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
		EffectScope: RuntimeEffectEphemeralSession, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}}
	manifest.AllowedOwnerModuleIDs = append(manifest.AllowedOwnerModuleIDs, RuntimeModuleShell)
	manifest.AllowedOperationKinds = append(manifest.AllowedOperationKinds, RuntimeBlockKindManagedValidationSession)
	manifest.AllowedTargetPolicies = append(manifest.AllowedTargetPolicies,
		RuntimeOperationTargetPolicyExactTarget,
		RuntimeOperationTargetPolicyExistingOwnerScope,
	)
	manifest.OwnerCatalogs = append(manifest.OwnerCatalogs, shellCatalog)
	source := CoverageFocus{
		ID: "source-independent-proofs", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange, EvidenceKindValidation}, ValidationRecipeID: "recipe/validate",
	}
	manifest = sealSemanticTestManifestWithSource(t, manifest, source)

	stateChange := semanticContractTestProposal("cwi-001")
	stateChange.Target = RuntimeOperationTargetAuthorization{
		Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"main.go"},
	}
	validation := RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-001", OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
		Target: RuntimeOperationTargetAuthorization{
			Policy: RuntimeOperationTargetPolicyExistingOwnerScope, OwnerScopeTargetRefs: []string{"main.go"},
		},
		RequiredEvidenceKinds: []string{EvidenceKindValidation}, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectEphemeralSession},
		ValidationRecipeID: "recipe/validate", DeclarationRef: stateChange.DeclarationRef,
	}
	document := RuntimeSemanticPlanDocument{
		SchemaVersion: RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: RuntimeSemanticPlanStepProofBatch, BatchID: "batch-independent-proofs", BatchSequence: 0,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}, Proposals: []RuntimeSemanticProofProposal{stateChange, validation},
	}
	facts := []RuntimeSemanticProofFeasibility{
		semanticFeasibilityForTest(t, manifest, stateChange, []string{"main.go"}, "", nil),
		semanticFeasibilityForTest(t, manifest, validation, []string{"main.go"}, "recipe/validate", []string{"check/validate"}),
	}
	return manifest, document, facts, source
}

func semanticFeasibilityForTest(t *testing.T, manifest RuntimeSemanticPlanManifest, proposal RuntimeSemanticProofProposal, targetRefs []string, recipeID string, checkRefs []string) RuntimeSemanticProofFeasibility {
	t.Helper()
	for _, catalog := range manifest.OwnerCatalogs {
		if catalog.OwnerModuleID != proposal.OwnerModuleID {
			continue
		}
		fact := RuntimeSemanticProofFeasibility{
			SchemaVersion: RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
			CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal),
			OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: catalog.SnapshotID, Status: RuntimeSemanticFeasibilityFeasible,
			ObservedRevision: catalog.ObservedRevision, TargetRefs: targetRefs, ValidationRecipeID: recipeID, RequiredCheckRefs: checkRefs,
		}
		if acquisition, found := runtimeSemanticOwnerFactAcquisitionForCoverage(manifest, proposal.CoverageWorkItemID); found &&
			acquisition.AcquisitionTargetPolicy == RuntimeOperationTargetPolicyProspectiveOwnerTarget {
			fact.CatalogObservationID = "file-owner-observation-test"
			delta := RuntimeProspectiveWorkspaceManifestDelta{
				SchemaVersion: RuntimeProspectiveWorkspaceManifestDeltaSchemaCurrent,
				WorkspaceID:   manifest.WorkspaceID, RunID: manifest.RunID, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
				CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal),
				OwnerModuleID: proposal.OwnerModuleID, CatalogSnapshotID: catalog.SnapshotID,
				ProspectiveSatisfactionContractRef: acquisition.ProspectiveSatisfactionContractRef,
				BaseWorkspaceManifestRevision:      catalog.ObservedRevision, ProspectiveTargetRef: proposal.Target.ProspectiveTargetRef,
				ResultingTargetRefs: append(append([]string(nil), catalog.TargetRefs...), proposal.Target.ProspectiveTargetRef),
			}.Normalized()
			fact.ProspectiveWorkspaceManifestDelta = &delta
			dependencyCatalog, found := runtimeSemanticOwnerFactDependencyCatalogFor(manifest, acquisition.Requirement.OwnerModuleID)
			if !found {
				t.Fatalf("missing prospective dependency catalog %q", acquisition.Requirement.OwnerModuleID)
			}
			satisfaction := RuntimeOwnerFactProspectiveSatisfaction{
				SchemaVersion: RuntimeOwnerFactProspectiveSatisfactionSchemaCurrent,
				WorkspaceID:   manifest.WorkspaceID, RunID: manifest.RunID, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
				CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: StableRuntimeSemanticProofProposalHash(proposal),
				Requirement: acquisition.Requirement, ProspectiveSatisfactionContractRef: acquisition.ProspectiveSatisfactionContractRef,
				OwnerModuleID: acquisition.Requirement.OwnerModuleID, CatalogSnapshotID: dependencyCatalog.SnapshotID,
				CatalogObservationID: "required-owner-observation-test", ObservedRevision: dependencyCatalog.ObservedRevision,
				ProspectiveWorkspaceManifestDeltaHash: StableRuntimeProspectiveWorkspaceManifestDeltaHash(delta),
				Outcome:                               RuntimeOwnerFactProspectiveWouldSatisfy, ProspectiveValidationRecipeRefs: []string{"recipe/prospective-validation"},
			}.Normalized()
			observation := RuntimeOwnerFactProspectiveSatisfactionObservation{
				FeasibilityBatchID: "test-feasibility-batch", RequestHash: "test-prospective-request", Satisfaction: &satisfaction,
			}
			fact.OwnerFactProspectiveSatisfactionObservation = &observation
		}
		return fact
	}
	t.Fatalf("missing owner catalog %q", proposal.OwnerModuleID)
	return RuntimeSemanticProofFeasibility{}
}
