package orchestrator

import (
	"context"
	"testing"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	magicevent "github.com/muidea/magicCommon/event"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeSemanticPrerequisitePathUsesCapabilityStateBeforeAdmissibility(t *testing.T) {
	pending := []runtimecontract.RuntimePrerequisite{
		{ID: "git-admissibility", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleGit},
		{ID: "git-state", Kind: runtimecontract.PrerequisiteKindCapabilityState, OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state"},
	}
	if !runtimeSemanticPrerequisitesPending(pending) {
		t.Fatal("semantic prerequisite path did not recognize owner prerequisites")
	}
	ordered := runtimeOrderedPrerequisites(pending)
	if len(ordered) != 2 || ordered[0].Kind != runtimecontract.PrerequisiteKindCapabilityState || ordered[1].Kind != runtimecontract.PrerequisiteKindOperationAdmissibility {
		t.Fatalf("semantic prerequisite order = %+v, want capability state before admissibility", ordered)
	}
}

func TestRuntimeSemanticDriveUsesPersistedRequestSessionBeforeEvaluatingDocument(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	bootstrap := runtimeBootstrap()
	facts, err := bootstrap.InitialFacts("ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	repository := &runtimeDriveRepository{}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := runtimeTestControllerInitialize(controller, ctx, facts, bootstrap.Definition, bootstrap.Requirements)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Target:              aggregate.Facts.Activation.ProviderTarget,
		ContextWindowTokens: 8192, ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048,
		MaxInputTokens: 4096, MaxBytes: 65536, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1",
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt"}, ObservedRevision: "file-revision-1"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	repository.aggregate = aggregate

	module := newRuntimeGroundedTargetSequenceModule(t, hub, repository)
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	requestCount := 0
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan || command.CandidateManifest.Hash != "" || command.CandidateContext.WorkspaceID != "" {
			t.Errorf("semantic request leaked operations authority: %+v", command)
			return
		}
		requestCount++
		item := command.SemanticManifest.CoverageItems[0]
		proposal := runtimecontract.RuntimeSemanticProofProposal{
			CoverageWorkItemID: item.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileEdit,
			Target:                runtimecontract.RuntimeOperationTargetAuthorization{Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"target.txt"}},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			DeclarationRef: "semantic/test-cwi",
		}
		document := runtimecontract.RuntimeSemanticPlanDocument{
			SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
			ManifestHash:  command.SemanticManifest.Hash, DefinitionSeedHash: command.SemanticManifest.DefinitionSeedHash,
			CoverageOrder: []common.CoverageWorkItemID{item.ID},
			Proposals:     []runtimecontract.RuntimeSemanticProofProposal{proposal},
		}
		payload, encodeErr := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(command.SemanticManifest, document)
		if encodeErr != nil {
			t.Errorf("encode semantic document: %v", encodeErr)
			return
		}
		result.Set(llmevents.RequestRuntimeCandidateResult{
			DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, DocumentContent: payload,
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	started, err := module.driveRuntimeSemanticPlanning(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if started.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequestOpen || started.SemanticPlanning.LogicalRequestID == "" || len(started.CandidateRequestSessions) != 1 || requestCount != 0 {
		t.Fatalf("semantic request was not persisted before provider I/O: %+v", started.SemanticPlanning)
	}
	evaluated, err := module.driveRuntimeSemanticPlanning(ctx, started)
	if err != nil {
		t.Fatal(err)
	}
	if evaluated.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningProposalAccepted || evaluated.SemanticPlanning.Document == nil || requestCount != 1 || len(evaluated.CandidateAttempts) != 0 || evaluated.ActiveDispatch != nil {
		t.Fatalf("semantic document crossed into operation authority: ledger=%+v requests=%d", evaluated.SemanticPlanning, requestCount)
	}
}

func TestRuntimeSemanticDriveClosesProviderSessionBeforeSemanticTerminal(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	bootstrap := runtimeBootstrap()
	facts, err := bootstrap.InitialFacts("ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	repository := &runtimeDriveRepository{}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := runtimeTestControllerInitialize(controller, ctx, facts, bootstrap.Definition, bootstrap.Requirements)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Target:              aggregate.Facts.Activation.ProviderTarget,
		ContextWindowTokens: 8192, ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048,
		MaxInputTokens: 4096, MaxBytes: 65536, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1",
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt"}, ObservedRevision: "file-revision-1"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	repository.aggregate = aggregate
	module := newRuntimeGroundedTargetSequenceModule(t, hub, repository)
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		result.Set(llmevents.RequestRuntimeCandidateResult{
			DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "semantic_provider_protocol_invalid",
				Signature: "semantic-provider-protocol-invalid", Reason: "provider returned an invalid semantic envelope",
			}),
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	started, err := module.driveRuntimeSemanticPlanning(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := module.driveRuntimeSemanticPlanning(ctx, started)
	if err != nil {
		t.Fatal(err)
	}
	if failed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed || failed.Decision.Terminal == nil || len(failed.CandidateRequestSessions) != 1 {
		t.Fatalf("semantic provider failure did not seal one terminal lineage: %+v", failed)
	}
	session := failed.CandidateRequestSessions[0]
	if session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || session.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol || len(failed.CandidateAttempts) != 0 || failed.ActiveDispatch != nil {
		t.Fatalf("semantic terminal retained open provider or operation work: session=%+v aggregate=%+v", session, failed)
	}
}

func TestRuntimeSemanticDriveFailsOnlyAfterPersistedCatalogRefreshBudgetIsExhausted(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	bootstrap := runtimeBootstrap()
	facts, err := bootstrap.InitialFacts("ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	facts.RecoveryPolicy.MaxPrerequisiteRefreshes = 1
	facts.Recovery.PrerequisiteRefreshes = 1
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt"}, ObservedRevision: "file-revision-1",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit},
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: catalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: catalog.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, prerequisite)
	repository := &runtimeDriveRepository{}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := runtimeTestControllerInitialize(controller, ctx, facts, bootstrap.Definition, bootstrap.Requirements)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.CompileRuntimeSemanticPlanManifest(
		aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.Definition, aggregate.Facts.DefinitionSeed,
		aggregate.Facts.Activation.ExecutionContract, aggregate.Facts.Resources.OperationAdmissibilities,
	)
	if err != nil {
		t.Fatal(err)
	}
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: manifest.CoverageItems[0].ID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKind: runtimecontract.StepKindFileEdit,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"target.txt"},
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef:        "semantic/budget-test",
	}
	document := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, Proposals: []runtimecontract.RuntimeSemanticProofProposal{proposal},
	}
	stablePrefixHash := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	aggregate, session, err := runtimeTestControllerEnsureCandidateRequest(controller, ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentSemanticPlan, stablePrefixHash, manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticPlanRequestStarted, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		SemanticManifest: &manifest, StablePrefixHash: stablePrefixHash, LogicalRequestID: session.LogicalRequestID,
	})
	if err != nil {
		t.Fatal(err)
	}
	aggregate = result.Aggregate
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1)
	aggregate, err = runtimeTestControllerMarkCandidateRequestSubmitted(controller, ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, 1, physicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = runtimeTestControllerCompleteCandidateDocument(controller, ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, physicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	result, err = controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticPlanEvaluated, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		SemanticDocument: &document, LogicalRequestID: session.LogicalRequestID,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err = controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticFeasibilityRequired, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
	})
	if err != nil {
		t.Fatal(err)
	}
	aggregate = result.Aggregate
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OriginalClass: "semantic_feasibility", OriginalCode: "semantic_catalog_stale",
		OriginalMessage: "owner catalog remained stale after bounded refresh", Signature: "semantic-feasibility/file/budget-stale",
		ExpectedRevision: "file-revision-1", ObservedRevision: "file-revision-2",
	}
	feasibility := runtimecontract.RuntimeSemanticProofFeasibility{
		SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: proposal.CoverageWorkItemID,
		ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: proposal.OwnerModuleID,
		CatalogSnapshotID: catalog.SnapshotID, CatalogObservationID: observation.ObservationID,
		Status: runtimecontract.RuntimeSemanticFeasibilityStale, ObservedRevision: "file-revision-2", OriginalDiagnostic: &diagnostic,
	}
	result, err = controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticFeasibilityObserved, WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility,
	})
	if err != nil {
		t.Fatal(err)
	}
	aggregate = result.Aggregate
	module := newRuntimeGroundedTargetSequenceModule(t, hub, repository)
	failed, err := module.driveRuntimeSemanticPlanning(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if failed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed || failed.SemanticPlanning.Failure == nil ||
		failed.SemanticPlanning.Failure.OriginalMessage != diagnostic.OriginalMessage || failed.Decision.Terminal == nil ||
		failed.Facts.Recovery.PrerequisiteRefreshes != 1 || len(failed.CandidateAttempts) != 0 || failed.ActiveDispatch != nil ||
		len(failed.PrerequisiteInvalidations) != 0 {
		t.Fatalf("semantic catalog refresh exhaustion lost diagnostic or created side effects: %+v", failed)
	}
}
