package orchestrator

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeProspectiveDriveMissingDependencyObservationDoesNotCallShell(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })

	aggregate := prospectiveCompositeDriveAggregate(t)
	manifest := aggregate.SemanticPlanning.Manifest.Clone()
	activeDocument, found := runtimeSemanticActiveFeasibilityDocument(aggregate.SemanticPlanning)
	if !found || len(activeDocument.Proposals) != 1 {
		t.Fatalf("prospective drive fixture has no unique proof proposal: %+v", aggregate.SemanticPlanning)
	}
	proposal := activeDocument.Proposals[0].Clone()
	fileCatalog, found := runtimeSemanticManifestOwnerCatalog(manifest, runtimecontract.RuntimeModuleFile)
	if !found {
		t.Fatalf("prospective drive fixture has no File owner catalog: %+v", manifest)
	}
	fileObservation := prospectiveDriveCatalogObservation(fileCatalog, fileCatalog.ObservedRevision)

	// The manifest keeps the immutable Shell dependency catalog while the
	// corresponding current observation is deliberately absent. This is the
	// exact drive condition that must retire the semantic generation before the
	// Shell EventHub adapter can be invoked.
	resources := aggregate.Facts.Resources.Normalized()
	resources.OperationAdmissibilities = prospectiveDriveReplaceCatalog(resources.OperationAdmissibilities, fileCatalog)
	currentCatalogs := resources.OperationAdmissibilities[:0]
	for _, catalog := range resources.OperationAdmissibilities {
		if catalog.OwnerModuleID != runtimecontract.RuntimeModuleShell {
			currentCatalogs = append(currentCatalogs, catalog)
		}
	}
	resources.OperationAdmissibilities = append([]runtimecontract.RuntimeOperationAdmissibility(nil), currentCatalogs...)
	resources.PrerequisiteObservations = prospectiveDriveReplaceObservation(resources.PrerequisiteObservations, fileObservation)
	filtered := resources.PrerequisiteObservations[:0]
	for _, observation := range resources.PrerequisiteObservations {
		if observation.OwnerModuleID != runtimecontract.RuntimeModuleShell {
			filtered = append(filtered, observation)
		}
	}
	resources.PrerequisiteObservations = append([]runtimecontract.RuntimePrerequisiteObservation(nil), filtered...)
	aggregate.Facts.Resources = resources
	currentSatisfied := aggregate.Facts.Prerequisites.Satisfied[:0]
	for _, prerequisite := range aggregate.Facts.Prerequisites.Satisfied {
		if prerequisite.OwnerModuleID == runtimecontract.RuntimeModuleShell && prerequisite.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility {
			aggregate.Facts.Prerequisites.Pending = append(aggregate.Facts.Prerequisites.Pending, prerequisite)
			continue
		}
		currentSatisfied = append(currentSatisfied, prerequisite)
	}
	aggregate.Facts.Prerequisites.Satisfied = append([]runtimecontract.RuntimePrerequisite(nil), currentSatisfied...)

	repository := &runtimeDriveRepository{aggregate: aggregate}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(workspaceevents.GetWorkspaceCommand)
		if !ok || command.WorkspaceID != aggregate.Facts.WorkspaceID {
			t.Errorf("workspace read command = %#v", event.Data())
			return
		}
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: aggregate.Facts.WorkspaceID, DeliverableDirectory: "/tmp/prospective-drive-workspace",
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	fileCalls := 0
	fileObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	fileObserver.Subscribe(fileevents.TopicProbeRuntimeSemanticProofFeasibility, func(event magicevent.Event, result magicevent.Result) {
		fileCalls++
		command, ok := event.Data().(fileevents.ProbeRuntimeSemanticProofFeasibilityCommand)
		if !ok {
			t.Errorf("File prospective command = %#v", event.Data())
			return
		}
		request := runtimecontract.RuntimeSemanticProofFeasibilityRequest(command)
		acquisition := prospectiveDriveAcquisition(t, request.Manifest, request.Proposal.CoverageWorkItemID)
		delta, err := runtimecontract.NewRuntimeProspectiveWorkspaceManifestDelta(
			request.Manifest, request.Proposal, acquisition.ProspectiveSatisfactionContractRef, request.OwnerCatalog,
			runtimecontract.WorkspaceManifest{Revision: request.OwnerCatalog.ObservedRevision, TargetRefs: append([]string(nil), request.OwnerCatalog.TargetRefs...)},
		)
		if err != nil {
			t.Errorf("File prospective delta: %v", err)
			return
		}
		ownerResult, err := fileevents.NewRuntimeSemanticProofFeasibilityOwnerResult(
			command, runtimecontract.RuntimeSemanticFeasibilityFeasible, request.OwnerCatalog.ObservedRevision,
			[]string{request.Proposal.Target.ProspectiveTargetRef}, &delta, nil,
		)
		if err != nil {
			t.Errorf("File prospective result: %v", err)
			return
		}
		result.Set(ownerResult, nil)
	})
	t.Cleanup(func() { fileObserver.Unsubscribe(fileevents.TopicProbeRuntimeSemanticProofFeasibility) })

	shellCalls := 0
	shellObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleShell, hub)
	shellObserver.Subscribe(shellevents.TopicProbeRuntimeOwnerFactProspectiveSatisfaction, func(_ magicevent.Event, _ magicevent.Result) {
		shellCalls++
	})
	t.Cleanup(func() { shellObserver.Unsubscribe(shellevents.TopicProbeRuntimeOwnerFactProspectiveSatisfaction) })

	next, err := module.probeRuntimeSemanticProposal(ctx, aggregate, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if fileCalls != 1 || shellCalls != 0 {
		t.Fatalf("missing dependency observation owner calls: File=%d Shell=%d", fileCalls, shellCalls)
	}
	if next.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired ||
		len(next.SemanticPlanning.History) != 1 || len(next.SemanticPlanning.Feasibilities) != 0 ||
		next.Facts.Recovery.PrerequisiteRefreshes != 0 || next.Decision.State == runtimecontract.ConvergenceStateFailed {
		t.Fatalf("missing dependency observation did not privately retire the semantic generation: %+v", next)
	}
}

func TestRuntimeProspectiveDriveTreatsConstraintValidOwnerRejectionAsContractViolation(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })

	// An open declaration selector is safe only because the Shell-authored
	// all-of target constraint is already sealed in the semantic manifest. A
	// matching proposal must never trigger a blind replacement-target retry.
	aggregate := prospectiveCompositeDriveAggregateWithCandidates(t, nil)
	activeDocument, found := runtimeSemanticActiveFeasibilityDocument(aggregate.SemanticPlanning)
	if !found || len(activeDocument.Proposals) != 1 {
		t.Fatalf("prospective would-not-satisfy fixture has no unique proposal: %+v", aggregate.SemanticPlanning)
	}
	repository := &runtimeDriveRepository{aggregate: aggregate}
	owner := newRuntimeTestOwner()
	if err := owner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(owner, hub, nil)

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: aggregate.Facts.WorkspaceID, DeliverableDirectory: "/tmp/prospective-drive-workspace",
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	fileCalls := 0
	fileObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	fileObserver.Subscribe(fileevents.TopicProbeRuntimeSemanticProofFeasibility, func(event magicevent.Event, result magicevent.Result) {
		fileCalls++
		command, ok := event.Data().(fileevents.ProbeRuntimeSemanticProofFeasibilityCommand)
		if !ok {
			t.Errorf("File prospective command = %#v", event.Data())
			return
		}
		request := runtimecontract.RuntimeSemanticProofFeasibilityRequest(command)
		acquisition := prospectiveDriveAcquisition(t, request.Manifest, request.Proposal.CoverageWorkItemID)
		delta, err := runtimecontract.NewRuntimeProspectiveWorkspaceManifestDelta(
			request.Manifest, request.Proposal, acquisition.ProspectiveSatisfactionContractRef, request.OwnerCatalog,
			runtimecontract.WorkspaceManifest{Revision: request.OwnerCatalog.ObservedRevision, TargetRefs: append([]string(nil), request.OwnerCatalog.TargetRefs...)},
		)
		if err != nil {
			t.Errorf("File prospective delta: %v", err)
			return
		}
		ownerResult, err := fileevents.NewRuntimeSemanticProofFeasibilityOwnerResult(
			command, runtimecontract.RuntimeSemanticFeasibilityFeasible, request.OwnerCatalog.ObservedRevision,
			[]string{request.Proposal.Target.ProspectiveTargetRef}, &delta, nil,
		)
		if err != nil {
			t.Errorf("File prospective result: %v", err)
			return
		}
		result.Set(ownerResult, nil)
	})
	t.Cleanup(func() { fileObserver.Unsubscribe(fileevents.TopicProbeRuntimeSemanticProofFeasibility) })

	const rejectionReason = "the prospective workspace manifest does not make the required validation surface selectable"
	shellCalls := 0
	shellObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleShell, hub)
	shellObserver.Subscribe(shellevents.TopicProbeRuntimeOwnerFactProspectiveSatisfaction, func(event magicevent.Event, result magicevent.Result) {
		shellCalls++
		command, ok := event.Data().(shellevents.ProbeRuntimeOwnerFactProspectiveSatisfactionCommand)
		if !ok {
			t.Errorf("Shell prospective command = %#v", event.Data())
			return
		}
		ownerResult, err := shellevents.NewRuntimeOwnerFactProspectiveSatisfactionResult(
			command, runtimecontract.RuntimeOwnerFactProspectiveWouldNotSatisfy, nil, rejectionReason,
		)
		if err != nil {
			t.Errorf("Shell prospective result: %v", err)
			return
		}
		result.Set(ownerResult, nil)
	})
	t.Cleanup(func() { shellObserver.Unsubscribe(shellevents.TopicProbeRuntimeOwnerFactProspectiveSatisfaction) })

	observed, err := module.driveRuntimeSemanticPlanning(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if fileCalls != 1 || shellCalls != 1 || observed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFeasibilityRequired ||
		len(observed.SemanticPlanning.Feasibilities) != 1 {
		t.Fatalf("would-not-satisfy persistence: File=%d Shell=%d phase=%q feasibility-count=%d", fileCalls, shellCalls,
			observed.SemanticPlanning.Phase, len(observed.SemanticPlanning.Feasibilities))
	}
	persisted := observed.SemanticPlanning.Feasibilities[0]
	if persisted.ProspectiveWorkspaceManifestDelta == nil || persisted.OwnerFactProspectiveSatisfactionObservation == nil ||
		persisted.OwnerFactProspectiveSatisfactionObservation.Satisfaction == nil ||
		persisted.OwnerFactProspectiveSatisfactionObservation.Satisfaction.Outcome != runtimecontract.RuntimeOwnerFactProspectiveWouldNotSatisfy ||
		persisted.OwnerFactProspectiveSatisfactionObservation.Satisfaction.Reason != rejectionReason {
		t.Fatalf("would-not-satisfy composite lost owner facts: %+v", persisted)
	}
	diagnostic, err := runtime.RuntimeProspectiveSatisfactionContractViolationDiagnostic(persisted)
	if err != nil || diagnostic.OriginalCode != "prospective_satisfaction_contract_violated" ||
		diagnostic.OriginalMessage != rejectionReason || len(diagnostic.SemanticViolations) != 0 {
		t.Fatalf("prospective owner contradiction diagnostic = %+v, err=%v", diagnostic, err)
	}

	failed, err := module.driveRuntimeSemanticPlanning(ctx, observed)
	if err != nil {
		t.Fatal(err)
	}
	if fileCalls != 1 || shellCalls != 1 || failed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed ||
		failed.SemanticPlanning.Failure == nil || failed.SemanticPlanning.Failure.OriginalCode != "prospective_satisfaction_contract_violated" ||
		failed.SemanticPlanning.Failure.OriginalMessage != rejectionReason || failed.SemanticPlanning.Failure.OwnerModuleID != runtimecontract.RuntimeModuleOrchestrator ||
		failed.Facts.Failure == nil || failed.Facts.Failure.Class != runtimecontract.FailureClassRuntimeContractMisalignment ||
		failed.Facts.Failure.OriginalDiagnostic == nil || failed.Facts.Failure.OriginalDiagnostic.OriginalCode != "prospective_satisfaction_contract_violated" ||
		failed.Facts.Failure.OriginalDiagnostic.OwnerModuleID != runtimecontract.RuntimeModuleOrchestrator ||
		failed.Decision.Terminal == nil || failed.Decision.Terminal.Code != "semantic_contract_failed" ||
		failed.SemanticPlanning.Attempt != 0 || failed.SemanticPlanning.CorrectionDirective != nil || len(failed.SemanticPlanning.Rejections) != 0 ||
		len(failed.CandidateAttempts) != 0 || failed.ActiveDispatch != nil {
		failureCode, failureMessage, failureOwner := "", "", ""
		if failed.SemanticPlanning.Failure != nil {
			failureCode = failed.SemanticPlanning.Failure.OriginalCode
			failureMessage = failed.SemanticPlanning.Failure.OriginalMessage
			failureOwner = failed.SemanticPlanning.Failure.OwnerModuleID
		}
		terminalCode := ""
		if failed.Decision.Terminal != nil {
			terminalCode = failed.Decision.Terminal.Code
		}
		factCode, factClass := "", runtimecontract.FailureClass("")
		if failed.Facts.Failure != nil {
			factClass = failed.Facts.Failure.Class
			if failed.Facts.Failure.OriginalDiagnostic != nil {
				factCode = failed.Facts.Failure.OriginalDiagnostic.OriginalCode
			}
		}
		t.Fatalf("persisted would-not-satisfy terminalization: File=%d Shell=%d phase=%q failure=(%q,%q,%q) fact=(%q,%q) terminal=%q attempts=%d dispatch=%t",
			fileCalls, shellCalls, failed.SemanticPlanning.Phase, failureCode, failureMessage, failureOwner,
			factCode, factClass, terminalCode, len(failed.CandidateAttempts), failed.ActiveDispatch != nil)
	}
	duplicate, err := module.driveRuntimeSemanticPlanning(ctx, failed)
	if err != nil {
		t.Fatal(err)
	}
	if fileCalls != 1 || shellCalls != 1 || duplicate.StateRevision != failed.StateRevision ||
		duplicate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed {
		t.Fatalf("duplicate failed drive re-probed prospective owners: File=%d Shell=%d revision=%d want=%d phase=%q",
			fileCalls, shellCalls, duplicate.StateRevision, failed.StateRevision, duplicate.SemanticPlanning.Phase)
	}

	restartedOwner := newRuntimeTestOwner()
	if err := restartedOwner.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	restarted := newTestBiz(restartedOwner, hub, nil)
	reloaded := repository.aggregate
	replayed, err := restarted.driveRuntimeSemanticPlanning(ctx, reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if fileCalls != 1 || shellCalls != 1 || replayed.StateRevision != failed.StateRevision ||
		replayed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed {
		t.Fatalf("restart replay re-probed prospective owners: File=%d Shell=%d revision=%d want=%d phase=%q",
			fileCalls, shellCalls, replayed.StateRevision, failed.StateRevision, replayed.SemanticPlanning.Phase)
	}
}

func prospectiveCompositeDriveAggregate(t *testing.T) runtime.RuntimeAggregate {
	return prospectiveCompositeDriveAggregateWithCandidates(t, []string{"prospective-validation-input.txt"})
}

func prospectiveCompositeDriveAggregateWithCandidates(t *testing.T, candidates []string) runtime.RuntimeAggregate {
	return prospectiveCompositeDriveAggregateWithCandidatesAndExisting(t, candidates, []string{"existing.txt"})
}

func prospectiveCompositeDriveAggregateWithCandidatesAndExisting(t *testing.T, candidates, existingTargets []string) runtime.RuntimeAggregate {
	t.Helper()
	bootstrap := runtimeBootstrap()
	facts, err := bootstrap.InitialFacts("ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	convergence := runtimecontract.RuntimeOwnerFactConvergence{
		Requirement: runtimecontract.RuntimeOwnerFactRequirement{
			OwnerModuleID: runtimecontract.RuntimeModuleShell, FactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
			OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		MissingTrigger:           runtimecontract.RuntimeOwnerFactMissingTrigger,
		AcquisitionOwnerModuleID: runtimecontract.RuntimeModuleFile, AcquisitionOperationKind: runtimecontract.StepKindFileWrite,
		AcquisitionTargetPolicy:            runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		AcquisitionTargetSelectorRef:       "workspace-relative-new-file-target",
		ProspectiveSatisfactionContractRef: "shell/selectable-validation-surface/prospective-workspace-manifest/v1",
		TargetSelectorRef:                  "file/workspace_deliverable", TargetSelectorOwnerRef: "test-profile", TargetSelectorSubjectRef: "validation-preparation",
		ReobserveOwnerModuleID: runtimecontract.RuntimeModuleShell, ReobserveFactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		MaxAcquisitionAttempts: 1,
	}
	prospectiveTarget := "prospective-validation-input.txt"
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Focus.RequiredOwnerFacts = []runtimecontract.RuntimeOwnerFactRequirement{convergence.Requirement}
	facts.Focus.OwnerFactConvergences = []runtimecontract.RuntimeOwnerFactConvergence{convergence}
	facts.Focus.TargetRefs = nil
	facts.Focus.TargetRole = "validation-input"
	facts.Focus.TargetSelectorRef = convergence.TargetSelectorRef
	facts.Focus.TargetSelectorOwnerRef = convergence.TargetSelectorOwnerRef
	facts.Focus.TargetSelectorSubjectRef = convergence.TargetSelectorSubjectRef
	facts.Focus.TargetSelectorCandidateRefs = append([]string(nil), candidates...)
	facts.Focus.RequireTargetGrounding = true
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileWrite}
	facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.DefinitionFocuses = runtimecontract.CompileRuntimeDefinitionFocuses([]runtimecontract.CoverageFocus{facts.Focus})

	recipePolicy := runtimecontract.RuntimeEvidencePolicy{
		ID: "test-prospective-validation-recipe", AppliesTo: runtimecontract.StepKindShellCommand,
		CommandName: "true", RecipeDeclared: true, RecipeArgs: []string{"--prospective"}, RecipeTimeoutMillis: 5_000,
	}
	facts.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{recipePolicy}
	fileCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "prospective-file-admissibility", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: append([]string(nil), existingTargets...),
		SelectorRefs: []string{convergence.AcquisitionTargetSelectorRef}, ConstraintRefs: []string{"workspace-relative-path"},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind:                  runtimecontract.StepKindFileWrite,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
			SelectorRefs:          []string{convergence.AcquisitionTargetSelectorRef}, ConstraintRefs: []string{"workspace-relative-path"},
		}},
		ObservedRevision: "workspace-tree-v1",
	}
	fileCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileCatalog)
	shellSurface := runtimecontract.RuntimeValidationSurface{
		ID: "test-prospective-validation-recipe", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		Readiness:     runtimecontract.RuntimeValidationSurfacePrerequisiteRequired, ReadinessReason: "prospective workspace input required",
		InputSchemaHash: "prospective-validation-schema-v1", PrerequisiteRef: "workspace/prospective-validation-input",
		ProspectiveSatisfactionContractRef: convergence.ProspectiveSatisfactionContractRef,
		ProspectiveTargetSatisfactionConstraint: &runtimecontract.RuntimeProspectiveTargetSatisfactionConstraint{
			SchemaVersion: runtimecontract.RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent,
			OwnerModuleID: runtimecontract.RuntimeModuleShell, ProspectiveSatisfactionContractRef: convergence.ProspectiveSatisfactionContractRef,
			ValidationSurfaceID: "test-prospective-validation-recipe", ValidationRecipeID: "test-prospective-validation-recipe",
			InputSchemaHash: "prospective-validation-schema-v1", CatalogObservedRevision: "shell-policy-v1",
			AllOf: []runtimecontract.RuntimeProspectiveTargetPredicate{{Kind: runtimecontract.RuntimeProspectiveTargetPredicatePathSuffix, Value: ".txt"}},
		},
		CheckRefs: []string{"check/prospective-validation"}, ProofApplicability: runtimecontract.RuntimeValidationProofDirect,
		EffectScope: runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}
	shellCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "prospective-shell-admissibility", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds:     []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{shellSurface}, ObservedRevision: "shell-policy-v1",
	}
	shellCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(shellCatalog)
	fileObservation := prospectiveDriveCatalogObservation(fileCatalog, fileCatalog.ObservedRevision)
	shellObservation := prospectiveDriveCatalogObservation(shellCatalog, fileCatalog.ObservedRevision)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{fileCatalog, shellCatalog}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{fileObservation, shellObservation}
	facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Selection:           runtimecontract.ProviderInferenceSelection{Target: facts.Activation.ProviderSelection.Target},
		ContextWindowTokens: 8192, ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048,
		MaxInputTokens: 4096, MaxBytes: 65536, CapabilityRevision: "prospective-provider-cap-v1", ConfigurationRevision: "prospective-provider-config-v1",
	}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{
		{ID: fileCatalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileWrite}},
		{ID: shellCatalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}},
	}

	repository := &runtimeDriveRepository{}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	definition := bootstrap.Definition
	definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	aggregate, err := runtimeTestControllerInitialize(controller, context.Background(), facts, definition, nil)
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
	if len(manifest.ProofObligations) != 1 || manifest.ProofObligations[0].OwnerFactAcquisition == nil || len(manifest.OwnerFactDependencyCatalogs) != 1 {
		t.Fatalf("prospective drive manifest fixture is incomplete: %+v", manifest)
	}
	acquisition := manifest.ProofObligations[0].OwnerFactAcquisition.Clone()
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: manifest.CoverageItems[0].ID, OwnerModuleID: acquisition.AcquisitionOwnerModuleID,
		OperationKind: acquisition.AcquisitionOperationKind, OperationVariantID: acquisition.AcquisitionOperationVariantID,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: acquisition.AcquisitionTargetPolicy, OwnerSelectorRefs: []string{acquisition.AcquisitionTargetSelectorRef},
			ProspectiveConstraintRefs: append([]string(nil), acquisition.AcquisitionConstraintRefs...),
			ProspectiveTargetRef:      prospectiveTarget,
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef:        "semantic/prospective-drive-regression",
	}
	order := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep:      runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder: []common.CoverageWorkItemID{proposal.CoverageWorkItemID},
	}
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	apply := func(event runtime.RuntimeInternalEvent) {
		t.Helper()
		event.Scope = runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}
		result, applyErr := controller.ApplyInternalEvent(context.Background(), event)
		if applyErr != nil {
			t.Fatalf("apply %s: %v", event.Kind, applyErr)
		}
		aggregate = result.Aggregate
	}
	apply(runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticPlanRequestStarted,
		Transaction: &runtime.RuntimeInternalTransactionPayload{
			SemanticManifest: &manifest, StablePrefixHash: "prospective-drive-prefix", LogicalRequestID: "prospective-drive-order",
		},
	})
	apply(runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventSemanticPlanEvaluated,
		Transaction: &runtime.RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: "prospective-drive-order"},
	})
	batch, err := semanticplan.CompileFocusBatch(
		manifest, order, 0, "prospective-drive-proof", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	apply(runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventSemanticPlanBatchCompiled,
		Transaction: &runtime.RuntimeInternalTransactionPayload{SemanticPlanBatch: &batch},
	})
	proof := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchID:  batch.BatchID, BatchSequence: batch.Sequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...),
		Proposals:                []runtimecontract.RuntimeSemanticProofProposal{proposal},
	}
	if err := proof.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	apply(runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventSemanticPlanEvaluated,
		Transaction: &runtime.RuntimeInternalTransactionPayload{SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID},
	})
	apply(runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventSemanticFeasibilityRequired, Authority: &runtime.RuntimeInternalAuthorityPayload{}})
	return aggregate
}

func prospectiveDriveAcquisition(t *testing.T, manifest runtimecontract.RuntimeSemanticPlanManifest, coverageID common.CoverageWorkItemID) runtimecontract.RuntimeSemanticOwnerFactAcquisition {
	t.Helper()
	for _, obligation := range manifest.ProofObligations {
		if obligation.CoverageWorkItemID == coverageID && obligation.OwnerFactAcquisition != nil {
			return obligation.OwnerFactAcquisition.Clone()
		}
	}
	t.Fatalf("prospective drive fixture has no acquisition for %q", coverageID)
	return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}
}

func prospectiveDriveCatalogObservation(catalog runtimecontract.RuntimeOperationAdmissibility, workspaceRevision string) runtimecontract.RuntimePrerequisiteObservation {
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

func prospectiveDriveReplaceCatalog(values []runtimecontract.RuntimeOperationAdmissibility, replacement runtimecontract.RuntimeOperationAdmissibility) []runtimecontract.RuntimeOperationAdmissibility {
	ret := make([]runtimecontract.RuntimeOperationAdmissibility, 0, len(values)+1)
	replaced := false
	for _, value := range values {
		if value.OwnerModuleID == replacement.OwnerModuleID {
			if !replaced {
				ret = append(ret, replacement)
				replaced = true
			}
			continue
		}
		ret = append(ret, value)
	}
	if !replaced {
		ret = append(ret, replacement)
	}
	return ret
}

func prospectiveDriveReplaceObservation(values []runtimecontract.RuntimePrerequisiteObservation, replacement runtimecontract.RuntimePrerequisiteObservation) []runtimecontract.RuntimePrerequisiteObservation {
	ret := make([]runtimecontract.RuntimePrerequisiteObservation, 0, len(values)+1)
	replaced := false
	for _, value := range values {
		if value.OwnerModuleID == replacement.OwnerModuleID {
			if !replaced {
				ret = append(ret, replacement)
				replaced = true
			}
			continue
		}
		ret = append(ret, value)
	}
	if !replaced {
		ret = append(ret, replacement)
	}
	return ret
}
