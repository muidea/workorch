package runtime

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type prospectiveSemanticTransitionFixture struct {
	controller       *RuntimeController
	repository       *controllerRepository
	aggregate        RuntimeAggregate
	manifest         runtimecontract.RuntimeSemanticPlanManifest
	proposal         runtimecontract.RuntimeSemanticProofProposal
	fileCatalog      runtimecontract.RuntimeOperationAdmissibility
	shellCatalog     runtimecontract.RuntimeOperationAdmissibility
	fileObservation  runtimecontract.RuntimePrerequisiteObservation
	shellObservation runtimecontract.RuntimePrerequisiteObservation
	recipe           runtimecontract.RuntimeValidationRecipe
	delta            runtimecontract.RuntimeProspectiveWorkspaceManifestDelta
}

func TestRuntimeProspectiveCompositePersistsOneBatchAndReplaysIdempotently(t *testing.T) {
	fixture := newProspectiveSemanticTransitionFixture(t)
	composite := fixture.compositeObservation(t, false)
	batchID := fixture.aggregate.SemanticPlanning.FeasibilityBatchID
	beforeSaves := fixture.repository.saves

	fixture.apply(t, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticFeasibilityObserved,
		Authority: &RuntimeInternalAuthorityPayload{
			SemanticBatchID: batchID, SemanticFeasibility: &composite,
		},
	})
	if fixture.repository.saves != beforeSaves+1 || len(fixture.aggregate.SemanticPlanning.Feasibilities) != 1 {
		t.Fatalf("prospective composite was not persisted by one CAS: saves=%d ledger=%+v", fixture.repository.saves, fixture.aggregate.SemanticPlanning)
	}
	persisted := fixture.aggregate.SemanticPlanning.Feasibilities[0]
	if persisted.ProspectiveWorkspaceManifestDelta == nil || persisted.OwnerFactProspectiveSatisfactionObservation == nil ||
		persisted.OwnerFactProspectiveSatisfactionObservation.Satisfaction == nil ||
		persisted.OwnerFactProspectiveSatisfactionObservation.FeasibilityBatchID != batchID ||
		persisted.OwnerFactProspectiveSatisfactionObservation.Satisfaction.Outcome != runtimecontract.RuntimeOwnerFactProspectiveWouldSatisfy {
		t.Fatalf("File delta and Shell satisfaction did not share one persisted feasibility batch: %+v", persisted)
	}
	revision := fixture.aggregate.StateRevision
	saves := fixture.repository.saves

	// A fresh controller models restart: the persisted aggregate, not an
	// in-process deduplication map, must fence the owner response replay.
	restarted, err := NewRuntimeController(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := restarted.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind:  RuntimeInternalEventSemanticFeasibilityObserved,
		Scope: RuntimeInternalEventScope{WorkspaceID: fixture.aggregate.Facts.WorkspaceID, RunID: fixture.aggregate.Facts.RunID},
		Authority: &RuntimeInternalAuthorityPayload{
			SemanticBatchID: batchID, SemanticFeasibility: &composite,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Aggregate.StateRevision != revision || fixture.repository.saves != saves || len(result.Aggregate.SemanticPlanning.Feasibilities) != 1 {
		t.Fatalf("prospective composite replay changed durable state: revision=%d saves=%d ledger=%+v", result.Aggregate.StateRevision, fixture.repository.saves, result.Aggregate.SemanticPlanning)
	}
}

func TestRuntimeProspectiveProposalPreProbeRejectsTargetOutsideSealedConstraint(t *testing.T) {
	fixture := newProspectiveSemanticTransitionFixture(t)
	ledger := fixture.aggregate.SemanticPlanning.Clone()
	proposal := fixture.proposal.Clone()
	proposal.Target.ProspectiveTargetRefs = []string{"Makefile"}
	document, found := runtimeSemanticBatchDocument(ledger.BatchDocuments, ledger.ActiveBatchID)
	if !found || len(document.Proposals) != 1 {
		t.Fatalf("prospective pre-probe fixture has no active proposal: %+v", ledger)
	}
	document.Proposals[0] = proposal.Clone()
	ledger.BatchDocuments[0] = document
	acquisition := fixture.manifest.ProofObligations[0].OwnerFactAcquisition
	if acquisition == nil || acquisition.ProspectiveTargetSatisfactionConstraint == nil ||
		runtimecontract.RuntimeProspectiveTargetsSatisfyConstraint(proposal.Target.ProspectiveTargetRefs, *acquisition.ProspectiveTargetSatisfactionConstraint) {
		t.Fatalf("fixture did not seal a rejecting all-of target constraint: %+v", acquisition)
	}
	if err := ValidateRuntimeSemanticProposalBeforeOwnerProbe(ledger, proposal); err == nil {
		t.Fatal("pre-probe guard admitted a target outside the Shell-authored all-of constraint")
	}
}

func TestRuntimeProspectiveNestedStalePersistsBeforeBoundedRefreshAndRestartReplayDoesNotChargeTwice(t *testing.T) {
	fixture := newProspectiveSemanticTransitionFixture(t)
	staleComposite := fixture.compositeObservation(t, true)
	batchID := fixture.aggregate.SemanticPlanning.FeasibilityBatchID

	fixture.apply(t, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticFeasibilityObserved,
		Authority: &RuntimeInternalAuthorityPayload{
			SemanticBatchID: batchID, SemanticFeasibility: &staleComposite,
		},
	})
	if len(fixture.aggregate.SemanticPlanning.Feasibilities) != 1 ||
		fixture.aggregate.SemanticPlanning.Feasibilities[0].OwnerFactProspectiveSatisfactionObservation == nil ||
		fixture.aggregate.SemanticPlanning.Feasibilities[0].OwnerFactProspectiveSatisfactionObservation.Stale == nil ||
		fixture.aggregate.Facts.Recovery.PrerequisiteRefreshes != 0 {
		t.Fatalf("nested typed stale was not persisted before refresh selection: %+v", fixture.aggregate)
	}
	observationRevision := fixture.aggregate.StateRevision
	fixture.apply(t, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticContractRequired,
		Authority: &RuntimeInternalAuthorityPayload{
			SemanticBatchID: batchID, SemanticRefreshGeneration: 0,
		},
	})
	if fixture.aggregate.StateRevision != observationRevision+1 ||
		fixture.aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired ||
		fixture.aggregate.SemanticPlanning.Attempt != 0 || fixture.aggregate.SemanticPlanning.CorrectionDirective != nil ||
		len(fixture.aggregate.SemanticPlanning.Rejections) != 0 ||
		fixture.aggregate.SemanticPlanning.CatalogRefreshAttempts != 1 ||
		len(fixture.aggregate.SemanticPlanning.FeasibilityGaps) != 1 ||
		fixture.aggregate.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("owner-private bounded refresh did not atomically retire the stale generation: %+v", fixture.aggregate)
	}
	if _, found := runtimecontract.RuntimePrerequisiteObservationForID(fixture.aggregate.Facts.Resources.PrerequisiteObservations, fixture.shellCatalog.PrerequisiteID); found {
		t.Fatalf("typed Shell stale left its dependency observation current: %+v", fixture.aggregate.Facts.Resources.PrerequisiteObservations)
	}
	if _, found := runtimecontract.RuntimePrerequisiteObservationForID(fixture.aggregate.Facts.Resources.PrerequisiteObservations, fixture.fileCatalog.PrerequisiteID); !found {
		t.Fatalf("typed Shell stale retired the independent File observation: %+v", fixture.aggregate.Facts.Resources.PrerequisiteObservations)
	}

	saves := fixture.repository.saves
	revision := fixture.aggregate.StateRevision
	restarted, err := NewRuntimeController(fixture.repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := restarted.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind:  RuntimeInternalEventSemanticContractRequired,
		Scope: RuntimeInternalEventScope{WorkspaceID: fixture.aggregate.Facts.WorkspaceID, RunID: fixture.aggregate.Facts.RunID},
		Authority: &RuntimeInternalAuthorityPayload{
			SemanticBatchID: batchID, SemanticRefreshGeneration: 0,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Aggregate.StateRevision != revision || fixture.repository.saves != saves || result.Aggregate.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("restart replay charged prerequisite refresh twice: revision=%d saves=%d recovery=%+v", result.Aggregate.StateRevision, fixture.repository.saves, result.Aggregate.Facts.Recovery)
	}
	if _, err := restarted.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind:  RuntimeInternalEventSemanticFeasibilityObserved,
		Scope: RuntimeInternalEventScope{WorkspaceID: fixture.aggregate.Facts.WorkspaceID, RunID: fixture.aggregate.Facts.RunID},
		Authority: &RuntimeInternalAuthorityPayload{
			SemanticBatchID: batchID, SemanticFeasibility: &staleComposite,
		},
	}); err == nil {
		t.Fatal("delayed stale feasibility event unexpectedly entered the fresh semantic generation")
	}
	if fixture.repository.saves != saves || fixture.repository.aggregate.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("delayed stale event changed refresh budget: saves=%d recovery=%+v", fixture.repository.saves, fixture.repository.aggregate.Facts.Recovery)
	}
}

func TestRuntimeProspectiveCompositeRetiresGenerationWhenDependencyCatalogDriftsBeforeObservationCAS(t *testing.T) {
	fixture := newProspectiveSemanticTransitionFixture(t)
	composite := fixture.compositeObservation(t, false)
	batchID := fixture.aggregate.SemanticPlanning.FeasibilityBatchID
	oldManifestHash := fixture.manifest.Hash

	advancedCatalog := fixture.shellCatalog.Normalized()
	advancedCatalog.ObservedRevision = "shell-policy-v2"
	for idx := range advancedCatalog.ValidationSurfaces {
		surface := &advancedCatalog.ValidationSurfaces[idx]
		constraint := surface.ProspectiveTargetSatisfactionConstraint
		if constraint == nil {
			// Per-CWI preparation constraints still carry the catalog revision
			// even when no recipe-wide prospective constraint is exposed.
		} else {
			value := constraint.Clone()
			value.CatalogObservedRevision = advancedCatalog.ObservedRevision
			surface.ProspectiveTargetSatisfactionConstraint = &value
		}
		for preparationIndex := range surface.ProofRequirementPreparations {
			value := surface.ProofRequirementPreparations[preparationIndex].ProspectiveTargetSatisfactionConstraint.Clone()
			value.CatalogObservedRevision = advancedCatalog.ObservedRevision
			surface.ProofRequirementPreparations[preparationIndex].ProspectiveTargetSatisfactionConstraint = value
		}
	}
	advancedCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(advancedCatalog)
	advancedObservation := prospectiveSemanticCatalogObservation(advancedCatalog, fixture.delta.BaseWorkspaceManifestRevision)
	for idx := range fixture.aggregate.Facts.Resources.OperationAdmissibilities {
		if fixture.aggregate.Facts.Resources.OperationAdmissibilities[idx].PrerequisiteID == advancedCatalog.PrerequisiteID {
			fixture.aggregate.Facts.Resources.OperationAdmissibilities[idx] = advancedCatalog
		}
	}
	for idx := range fixture.aggregate.Facts.Resources.PrerequisiteObservations {
		if fixture.aggregate.Facts.Resources.PrerequisiteObservations[idx].PrerequisiteID == advancedCatalog.PrerequisiteID {
			fixture.aggregate.Facts.Resources.PrerequisiteObservations[idx] = advancedObservation
		}
	}
	fixture.repository.aggregate = &fixture.aggregate

	fixture.apply(t, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticFeasibilityObserved,
		Authority: &RuntimeInternalAuthorityPayload{
			SemanticBatchID: batchID, SemanticFeasibility: &composite,
		},
	})
	if fixture.aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired ||
		len(fixture.aggregate.SemanticPlanning.Feasibilities) != 0 || len(fixture.aggregate.SemanticPlanning.History) != 1 ||
		fixture.aggregate.SemanticPlanning.History[0].Snapshot.Manifest == nil ||
		fixture.aggregate.SemanticPlanning.History[0].Snapshot.Manifest.Hash != oldManifestHash ||
		fixture.aggregate.Facts.Recovery.PrerequisiteRefreshes != 0 {
		t.Fatalf("pre-CAS dependency drift did not retire the exact semantic generation without consuming stale budget: %+v", fixture.aggregate)
	}
}

func TestRuntimeProspectiveCompositeRefusesSealWhenDependencyObservationDriftsAfterPersistence(t *testing.T) {
	fixture := newProspectiveSemanticTransitionFixture(t)
	composite := fixture.compositeObservation(t, false)
	batchID := fixture.aggregate.SemanticPlanning.FeasibilityBatchID
	fixture.apply(t, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticFeasibilityObserved,
		Authority: &RuntimeInternalAuthorityPayload{
			SemanticBatchID: batchID, SemanticFeasibility: &composite,
		},
	})
	oldManifestHash := fixture.manifest.Hash

	// Re-observation may preserve identical catalog bytes while changing the
	// exact dependency lineage. Snapshot-only freshness would incorrectly seal
	// this response; the persisted composite observation ID must fence it.
	reobserved := fixture.shellObservation.Normalized()
	reobserved.Dependencies = append(reobserved.Dependencies, runtimecontract.RuntimeFreshnessDependency{
		Domain: runtimecontract.RuntimeResourceRemoteResource, OwnerModuleID: runtimecontract.RuntimeModuleShell,
		SubjectRef: "prospective-fixture-reobservation", Revision: "remote-v2",
	})
	reobserved.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(reobserved)
	if reobserved.ObservationID == fixture.shellObservation.ObservationID {
		t.Fatal("test re-observation did not change exact identity")
	}
	for idx := range fixture.aggregate.Facts.Resources.PrerequisiteObservations {
		if fixture.aggregate.Facts.Resources.PrerequisiteObservations[idx].PrerequisiteID == reobserved.PrerequisiteID {
			fixture.aggregate.Facts.Resources.PrerequisiteObservations[idx] = reobserved
		}
	}
	fixture.repository.aggregate = &fixture.aggregate

	fixture.apply(t, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Authority: &RuntimeInternalAuthorityPayload{}})
	if fixture.aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired ||
		len(fixture.aggregate.SemanticPlanning.History) != 1 ||
		fixture.aggregate.SemanticPlanning.History[0].Snapshot.Manifest == nil ||
		fixture.aggregate.SemanticPlanning.History[0].Snapshot.Manifest.Hash != oldManifestHash ||
		fixture.aggregate.Facts.ReconciliationContract.BindingSetHash != "" ||
		fixture.aggregate.Facts.Recovery.PrerequisiteRefreshes != 0 {
		t.Fatalf("post-persist dependency re-observation crossed the seal freshness fence: %+v", fixture.aggregate)
	}
}

func newProspectiveSemanticTransitionFixture(t *testing.T) *prospectiveSemanticTransitionFixture {
	t.Helper()
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	convergence := runtimeTestSelectableValidationOwnerFactConvergence()
	prospectiveTarget := "prospective-validation-input.txt"
	recipePolicy := runtimecontract.RuntimeEvidencePolicy{
		ID: "prospective-validation-recipe", AppliesTo: runtimecontract.StepKindShellCommand,
		CommandName: "true", RecipeDeclared: true, RecipeArgs: []string{"--prospective"}, RecipeTimeoutMillis: 5_000,
	}
	recipe := runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies([]runtimecontract.RuntimeEvidencePolicy{recipePolicy})[0]
	facts.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{recipePolicy}
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Focus.RequiredOwnerFacts = []runtimecontract.RuntimeOwnerFactRequirement{runtimeTestSelectableValidationOwnerFact()}
	facts.Focus.OwnerFactConvergences = []runtimecontract.RuntimeOwnerFactConvergence{convergence}
	facts.Focus.TargetRefs = nil
	facts.Focus.TargetRole = "validation-input"
	facts.Focus.TargetSelectorRef = convergence.TargetSelectorRef
	facts.Focus.TargetSelectorOwnerRef = convergence.TargetSelectorOwnerRef
	facts.Focus.TargetSelectorSubjectRef = convergence.TargetSelectorSubjectRef
	facts.Focus.TargetSelectorCandidateRefs = []string{prospectiveTarget, "prospective-validation-input-alt.txt"}
	facts.Focus.RequireTargetGrounding = true
	facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}

	fileCatalog := facts.Resources.OperationAdmissibilities[0].Normalized()
	fileCatalog.TargetRefs = []string{"existing.txt"}
	fileCatalog.SelectorRefs = []string{convergence.AcquisitionTargetSelectorRef}
	fileCatalog.ConstraintRefs = []string{"workspace-relative-path"}
	fileCatalog.OperationVariants = []runtimecontract.RuntimeOperationAdmissibilityVariant{{
		Kind: runtimecontract.StepKindFileWrite,
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
			runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		},
		SelectorRefs: []string{convergence.AcquisitionTargetSelectorRef}, ConstraintRefs: []string{"workspace-relative-path"},
	}}
	fileCatalog.ObservedRevision = "workspace-tree-v1"
	fileCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileCatalog)
	surface := runtimecontract.RuntimeValidationSurface{
		ID: recipe.ID, OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		Readiness:     runtimecontract.RuntimeValidationSurfacePrerequisiteRequired, ReadinessReason: "prospective workspace input required",
		InputSchemaHash: "prospective-validation-schema-v1", PrerequisiteRef: "workspace/prospective-validation-input",
		ProspectiveSatisfactionContractRef: convergence.ProspectiveSatisfactionContractRef,
		ProspectiveTargetSatisfactionConstraint: &runtimecontract.RuntimeProspectiveTargetSatisfactionConstraint{
			SchemaVersion: runtimecontract.RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent,
			OwnerModuleID: runtimecontract.RuntimeModuleShell, ProspectiveSatisfactionContractRef: convergence.ProspectiveSatisfactionContractRef,
			ValidationSurfaceID: recipe.ID, ValidationRecipeID: recipe.ID,
			InputSchemaHash: "prospective-validation-schema-v1", CatalogObservedRevision: "shell-policy-v1",
			AllOf: []runtimecontract.RuntimeProspectiveTargetClause{{
				ID: "text-input", AllOf: []runtimecontract.RuntimeProspectiveTargetPredicate{{Kind: runtimecontract.RuntimeProspectiveTargetPredicatePathSuffix, Value: ".txt"}},
			}},
		},
		CheckRefs: []string{"check/prospective-validation"}, ProofApplicability: runtimecontract.RuntimeValidationProofDirect,
		EffectScope: runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}
	shellCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "prospective-shell-admissibility", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds:     []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{surface}, ObservedRevision: "shell-policy-v1",
	}
	shellCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(shellCatalog)
	if err := shellCatalog.Validate(); err != nil {
		t.Fatalf("Shell dependency catalog: %v", err)
	}
	fileObservation := prospectiveSemanticCatalogObservation(fileCatalog, fileCatalog.ObservedRevision)
	shellObservation := prospectiveSemanticCatalogObservation(shellCatalog, fileCatalog.ObservedRevision)
	filePrerequisite := runtimecontract.RuntimePrerequisite{
		ID: fileCatalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: fileCatalog.OwnerModuleID, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite},
	}
	shellPrerequisite := runtimecontract.RuntimePrerequisite{
		ID: shellCatalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: shellCatalog.OwnerModuleID, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{fileCatalog, shellCatalog}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{fileObservation, shellObservation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{filePrerequisite, shellPrerequisite}
	facts.DefinitionFocuses = nil
	facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
	facts, err = runtimeTestBindManagedValidationProofRequirements(facts)
	if err != nil {
		t.Fatalf("bind prospective validation proof preparation: %v", err)
	}
	for _, catalog := range facts.Resources.OperationAdmissibilities {
		if catalog.OwnerModuleID == runtimecontract.RuntimeModuleShell {
			shellCatalog = catalog
			break
		}
	}
	shellObservation = prospectiveSemanticCatalogObservation(shellCatalog, fileCatalog.ObservedRevision)
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{fileObservation, shellObservation}
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	aggregate, err := controller.initialize(ctx, facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.CompileRuntimeSemanticPlanManifestForCoverage(
		aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Definition,
		aggregate.Facts.DefinitionSeed, aggregate.Facts.DefinitionFocuses,
		aggregate.Facts.Activation.ExecutionContract, aggregate.Facts.Resources.OperationAdmissibilities,
		runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.ProofObligations) != 1 || manifest.ProofObligations[0].OwnerFactAcquisition == nil ||
		len(manifest.OwnerFactDependencyCatalogs) != 1 {
		t.Fatalf("prospective semantic manifest fixture is incomplete: %+v", manifest)
	}
	acquisition := manifest.ProofObligations[0].OwnerFactAcquisition.Clone()
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: manifest.CoverageItems[0].ID, OwnerModuleID: acquisition.AcquisitionOwnerModuleID,
		OperationKind: acquisition.AcquisitionOperationKind, OperationVariantID: acquisition.AcquisitionOperationVariantID,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: acquisition.AcquisitionTargetPolicy, OwnerSelectorRefs: []string{acquisition.AcquisitionTargetSelectorRef},
			ProspectiveConstraintRefs: append([]string(nil), acquisition.AcquisitionConstraintRefs...),
			ProspectiveTargetRefs:     []string{prospectiveTarget},
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef:        runtimeTestSemanticDeclarationRef(manifest, manifest.CoverageItems[0].ID),
	}
	proof := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchID:  "prospective-fixture", BatchSequence: 0,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{proposal.CoverageWorkItemID},
		Proposals:                []runtimecontract.RuntimeSemanticProofProposal{proposal},
	}
	order := proof.Clone()
	order.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	order.BatchID = ""
	order.BatchSequence = 0
	order.BatchCoverageWorkItemIDs = nil
	order.CoverageOrder = []common.CoverageWorkItemID{proposal.CoverageWorkItemID}
	order.Proposals = nil
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	fixture := &prospectiveSemanticTransitionFixture{
		controller: controller, repository: repository, aggregate: aggregate,
		manifest: manifest, proposal: proposal, fileCatalog: fileCatalog, shellCatalog: shellCatalog,
		fileObservation: fileObservation, shellObservation: shellObservation, recipe: recipe,
	}
	fixture.apply(t, RuntimeInternalEvent{
		Kind: RuntimeInternalEventSemanticPlanRequestStarted,
		Transaction: &RuntimeInternalTransactionPayload{
			SemanticManifest: &manifest, StablePrefixHash: "prospective-prefix", LogicalRequestID: "prospective-order-request",
		},
	})
	fixture.apply(t, RuntimeInternalEvent{
		Kind:        RuntimeInternalEventSemanticPlanEvaluated,
		Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: "prospective-order-request"},
	})
	batch, err := semanticplan.CompileFocusBatch(
		manifest, order, 0, "prospective-proof-request", fixture.aggregate.Facts.Focus.CoverageWorkItemIDs,
		fixture.aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.aggregate, err = controller.compileSemanticPlanBatch(ctx, fixture.aggregate.Facts.WorkspaceID, fixture.aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	proof.BatchID = batch.BatchID
	proof.BatchSequence = batch.Sequence
	proof.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...)
	if err := proof.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	fixture.apply(t, RuntimeInternalEvent{
		Kind:        RuntimeInternalEventSemanticPlanEvaluated,
		Transaction: &RuntimeInternalTransactionPayload{SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID},
	})
	fixture.apply(t, RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
	delta, err := runtimecontract.NewRuntimeProspectiveWorkspaceManifestDelta(
		manifest, proposal, acquisition.ProspectiveSatisfactionContractRef, fileCatalog,
		runtimecontract.WorkspaceManifest{Revision: fileCatalog.ObservedRevision, TargetRefs: []string{"existing.txt"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.delta = delta
	return fixture
}

func (f *prospectiveSemanticTransitionFixture) compositeObservation(t *testing.T, stale bool) runtimecontract.RuntimeSemanticProofFeasibility {
	return f.compositeObservationWithOutcome(t, stale, runtimecontract.RuntimeOwnerFactProspectiveWouldSatisfy, "")
}

func (f *prospectiveSemanticTransitionFixture) wouldNotSatisfyCompositeObservation(t *testing.T, reason string) runtimecontract.RuntimeSemanticProofFeasibility {
	return f.compositeObservationWithOutcome(t, false, runtimecontract.RuntimeOwnerFactProspectiveWouldNotSatisfy, reason)
}

func (f *prospectiveSemanticTransitionFixture) compositeObservationWithOutcome(t *testing.T, stale bool, outcome runtimecontract.RuntimeOwnerFactProspectiveSatisfactionOutcome, reason string) runtimecontract.RuntimeSemanticProofFeasibility {
	t.Helper()
	batchID := f.aggregate.SemanticPlanning.FeasibilityBatchID
	outer, err := runtimecontract.NewRuntimeSemanticProofFeasibilityOwnerObservation(runtimecontract.RuntimeSemanticProofFeasibilityRequest{
		WorkspaceID: f.aggregate.Facts.WorkspaceID, RunID: f.aggregate.Facts.RunID,
		FeasibilityBatchID: batchID, WorkspaceDeliverableDirectory: "/tmp/prospective-semantic-fixture",
		Manifest: f.manifest, Proposal: f.proposal, OwnerCatalog: f.fileCatalog, CatalogObservation: &f.fileObservation,
	}, runtimecontract.RuntimeSemanticFeasibilityFeasible, f.fileCatalog.ObservedRevision,
		append([]string(nil), f.proposal.Target.ProspectiveTargetRefs...), &f.delta, nil)
	if err != nil {
		t.Fatal(err)
	}
	acquisition := f.manifest.ProofObligations[0].OwnerFactAcquisition.Clone()
	request := runtimecontract.RuntimeOwnerFactProspectiveSatisfactionRequest{
		WorkspaceID: f.aggregate.Facts.WorkspaceID, RunID: f.aggregate.Facts.RunID, FeasibilityBatchID: batchID,
		Manifest: f.manifest, Proposal: f.proposal, Requirement: acquisition.Requirement,
		ProspectiveSatisfactionContractRef: acquisition.ProspectiveSatisfactionContractRef,
		OwnerCatalog:                       f.shellCatalog, CatalogObservation: &f.shellObservation,
		ValidationRecipes:                 []runtimecontract.RuntimeValidationRecipe{f.recipe},
		ProspectiveWorkspaceManifestDelta: f.delta,
	}
	var nested runtimecontract.RuntimeOwnerFactProspectiveSatisfactionObservation
	if stale {
		nested, err = runtimecontract.NewRuntimeOwnerFactProspectiveSatisfactionStaleObservation(request, "shell-policy-v2", "Shell policy advanced while evaluating the File delta")
	} else {
		selectedRecipes := []string{f.recipe.ID}
		if outcome == runtimecontract.RuntimeOwnerFactProspectiveWouldNotSatisfy {
			selectedRecipes = nil
		}
		nested, err = runtimecontract.NewRuntimeOwnerFactProspectiveSatisfactionObservation(request, outcome, selectedRecipes, reason)
	}
	if err != nil {
		t.Fatal(err)
	}
	composite := outer.Observation.Clone()
	composite.OwnerFactProspectiveSatisfactionObservation = &nested
	if err := composite.ValidateCompositeForProposal(f.manifest, f.proposal); err != nil {
		t.Fatal(err)
	}
	return composite
}

func (f *prospectiveSemanticTransitionFixture) apply(t *testing.T, event RuntimeInternalEvent) {
	t.Helper()
	event.Scope = RuntimeInternalEventScope{WorkspaceID: f.aggregate.Facts.WorkspaceID, RunID: f.aggregate.Facts.RunID}
	result, err := f.controller.ApplyInternalEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("apply %s: %v", event.Kind, err)
	}
	f.aggregate = result.Aggregate
}

func prospectiveSemanticCatalogObservation(catalog runtimecontract.RuntimeOperationAdmissibility, workspaceRevision string) runtimecontract.RuntimePrerequisiteObservation {
	catalog = catalog.Normalized()
	dependencies := []runtimecontract.RuntimeFreshnessDependency{{
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", Revision: workspaceRevision,
	}}
	if catalog.OwnerModuleID != runtimecontract.RuntimeModuleFile {
		dependencies = append(dependencies, runtimecontract.RuntimeFreshnessDependency{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: catalog.OwnerModuleID,
			SubjectRef: "prospective-validation-policy", Revision: catalog.ObservedRevision,
		})
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: catalog.OwnerModuleID, ContentHash: catalog.SnapshotID, Dependencies: dependencies,
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}
