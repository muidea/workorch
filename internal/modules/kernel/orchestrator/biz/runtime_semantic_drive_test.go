package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
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

func TestSemanticOwnerFeasibilityTerminalCodePreservesMCPInputBindingPrerequisite(t *testing.T) {
	fact := runtimecontract.RuntimeSemanticProofFeasibility{OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{
		OriginalCode: "mcp_validation_input_binding_required",
	}}
	if got := semanticOwnerFeasibilityTerminalCode(fact); got != "mcp_validation_input_binding_required" {
		t.Fatalf("semantic prerequisite terminal code = %q", got)
	}
}

func TestRuntimeSemanticPrerequisitePathPublishesFileCatalogBeforeShellValidationInventory(t *testing.T) {
	pending := []runtimecontract.RuntimePrerequisite{
		{ID: "shell-admissibility", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleShell},
		{ID: "file-admissibility", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile},
		{ID: "file-manifest", Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile},
	}
	ordered := runtimeOrderedPrerequisites(pending)
	if len(ordered) != 3 || ordered[0].Kind != runtimecontract.PrerequisiteKindManifest || ordered[1].OwnerModuleID != runtimecontract.RuntimeModuleFile || ordered[2].OwnerModuleID != runtimecontract.RuntimeModuleShell {
		t.Fatalf("semantic prerequisite order = %+v, want File manifest/catalog before Shell validation inventory", ordered)
	}
}

func TestRuntimeSemanticActiveFeasibilityDocumentUsesBoundedBatch(t *testing.T) {
	order := runtimecontract.RuntimeSemanticPlanDocument{
		PlanStep:      runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder: []common.CoverageWorkItemID{"cwi-001"},
	}
	proof := runtimecontract.RuntimeSemanticPlanDocument{
		PlanStep:                 runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchID:                  "batch-001",
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
		CoverageOrder:            []common.CoverageWorkItemID{"cwi-001"},
		Proposals: []runtimecontract.RuntimeSemanticProofProposal{{
			CoverageWorkItemID: "cwi-001",
		}},
	}
	document, found := runtimeSemanticActiveFeasibilityDocument(runtimecontract.RuntimeSemanticPlanningLedger{
		Document:       &order,
		ActiveBatchID:  proof.BatchID,
		BatchDocuments: []runtimecontract.RuntimeSemanticPlanDocument{proof},
	})
	if !found || document.PlanStep != runtimecontract.RuntimeSemanticPlanStepProofBatch || len(document.Proposals) != 1 || document.Proposals[0].CoverageWorkItemID != "cwi-001" {
		t.Fatalf("active feasibility document = %+v found=%t, want bounded proof batch", document, found)
	}
}

func TestRuntimeSemanticCorrectionExpectedDocumentKeepsPlanStepAndBatch(t *testing.T) {
	aggregate := runtimeAggregate(t, 1)
	manifest := runtimecontract.RuntimeSemanticPlanManifest{Hash: "manifest-1"}
	aggregate.SemanticPlanning.Manifest = &manifest
	aggregate.SemanticPlanning.Attempt = 1

	coverageExpected, err := runtimeSemanticExpectedDocumentForLedger(aggregate, aggregate.SemanticPlanning)
	if err != nil {
		t.Fatal(err)
	}
	if coverageExpected.PlanStep != runtimecontract.RuntimeSemanticPlanStepCoverageOrder || coverageExpected.CorrectionAttempt != 1 || coverageExpected.BatchID != "" {
		t.Fatalf("coverage-order correction expected document = %+v", coverageExpected)
	}

	batch := runtimecontract.RuntimeSemanticPlanBatch{
		BatchID: "batch-0", Sequence: 0, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, CoverageOrderHash: "order-1",
	}
	aggregate.SemanticPlanning.ActiveBatchID = batch.BatchID
	aggregate.SemanticPlanning.Batches = []runtimecontract.RuntimeSemanticPlanBatch{batch}
	batchExpected, err := runtimeSemanticExpectedDocumentForLedger(aggregate, aggregate.SemanticPlanning)
	if err != nil {
		t.Fatal(err)
	}
	if batchExpected.PlanStep != runtimecontract.RuntimeSemanticPlanStepProofBatch || batchExpected.CorrectionAttempt != 1 || batchExpected.BatchID != batch.BatchID || batchExpected.BatchSequence != 0 || len(batchExpected.BatchCoverageWorkItemIDs) != 1 || batchExpected.BatchCoverageWorkItemIDs[0] != "cwi-v2" {
		t.Fatalf("proof-batch correction expected document = %+v", batchExpected)
	}
}

func TestRuntimeSemanticCorrectionDiagnosticBindsProofBatchWithoutReclassifyingOrder(t *testing.T) {
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		FieldViolations: []runtimecontract.RuntimeFieldViolation{{Path: "$.proposals", Actual: "missing_required"}},
	}
	manifest := runtimecontract.RuntimeSemanticPlanManifest{CoverageItems: []runtimecontract.RuntimeCoverageItem{{ID: "cwi-002"}, {ID: "cwi-001"}}}
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		PlanStep:                 runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-002", "cwi-001"},
	}
	got := runtimeSemanticCorrectionDiagnostic(diagnostic, manifest, expected)
	if len(got.SemanticViolations) != 1 || got.SemanticViolations[0].Kind != runtimecontract.RuntimeSemanticContractViolationProofBatchDocument ||
		got.SemanticViolations[0].Path != "$.proposals" || len(got.SemanticViolations[0].RequiredCoverageWorkItemIDs) != 2 ||
		got.SemanticViolations[0].RequiredCoverageWorkItemIDs[0] != "cwi-002" || got.SemanticViolations[0].RequiredCoverageWorkItemIDs[1] != "cwi-001" {
		t.Fatalf("proof-batch correction diagnostic = %+v", got)
	}
	orderExpected := expected
	orderExpected.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	order := runtimeSemanticCorrectionDiagnostic(diagnostic, manifest, orderExpected)
	if len(order.SemanticViolations) != 1 || order.SemanticViolations[0].Kind != runtimecontract.RuntimeSemanticContractViolationCoverageOrderDocument ||
		order.SemanticViolations[0].Path != "$.proposals" || order.SemanticViolations[0].Actual != "missing_required" ||
		len(order.SemanticViolations[0].RequiredCoverageWorkItemIDs) != 2 {
		t.Fatalf("coverage-order structural diagnostic = %+v", order)
	}
}

func TestRuntimeSemanticDriveUsesPersistedRequestSessionAndExcludesRetiredGenerationFromProviderCommand(t *testing.T) {
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
		Selection:           runtimecontract.ProviderInferenceSelection{Target: aggregate.Facts.Activation.ProviderSelection.Target},
		ContextWindowTokens: 8192, ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048,
		MaxInputTokens: 4096, MaxBytes: 65536, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1",
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt"},
		SelectorRefs: []string{"workspace-relative-new-file-target"}, ConstraintRefs: []string{"workspace-relative-path"},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind: runtimecontract.StepKindFileEdit,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
				runtimecontract.RuntimeOperationTargetPolicyExactTarget,
				runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			},
			SelectorRefs: []string{"workspace-relative-new-file-target"}, ConstraintRefs: []string{"workspace-relative-path"},
		}},
		ObservedRevision: "file-revision-1",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	const retiredHistorySentinel = "retired-semantic-history-must-not-reach-provider"
	// The retired generation is test-only history. Detach its facts before the
	// semantic projection mutates CWI rows so active authority remains intact.
	retiredAggregate := aggregate
	retiredAggregate.Facts = aggregate.Facts.Normalized()
	retired := runtimeTestAggregateWithSemanticPlanning(t, retiredAggregate)
	if retired.SemanticPlanning.Manifest == nil || len(retired.SemanticPlanning.Manifest.OwnerCatalogs) == 0 {
		t.Fatalf("retired semantic fixture has no owner catalog: %+v", retired.SemanticPlanning)
	}
	retiredSnapshot := retired.SemanticPlanning.Clone()
	retiredSnapshot.History = nil
	aggregate.SemanticPlanning.History = []runtimecontract.RuntimeSemanticPlanningGeneration{{
		RetiredAtStateRevision: 1,
		Reason:                 runtimecontract.RuntimeSemanticPlanningRetirementResourceInvalidated,
		InvalidationSignatures: []string{retiredHistorySentinel},
		PrerequisiteIDs:        []string{retiredSnapshot.Manifest.OwnerCatalogs[0].PrerequisiteID},
		Snapshot:               retiredSnapshot,
	}}
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("semantic history fixture is invalid: %v", err)
	}
	repository.aggregate = aggregate

	module := newRuntimeGroundedTargetSequenceModule(t, hub, repository)
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	requestCount := 0
	proofManifestHash := ""
	proofStablePrefixHash := ""
	proofBatchID := ""
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan || command.CandidateManifest.Hash != "" || command.CandidateContext.WorkspaceID != "" {
			t.Errorf("semantic request leaked operations authority: %+v", command)
			return
		}
		encoded, encodeErr := json.Marshal(command)
		if encodeErr != nil {
			t.Errorf("marshal semantic provider command: %v", encodeErr)
			return
		}
		if strings.Contains(string(encoded), retiredHistorySentinel) {
			t.Errorf("retired semantic history leaked into provider command: %s", encoded)
			return
		}
		wantStablePrefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(command.SemanticContext.Definition, command.SemanticContext.Activation.ExecutionContract)
		if command.StablePrefixHash != wantStablePrefix {
			t.Errorf("semantic provider stable prefix = %q, want current definition/execution prefix %q", command.StablePrefixHash, wantStablePrefix)
			return
		}
		requestCount++
		item := command.SemanticManifest.CoverageItems[0]
		document := runtimecontract.RuntimeSemanticPlanDocument{
			SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
			ManifestHash:  command.SemanticManifest.Hash, DefinitionSeedHash: command.SemanticManifest.DefinitionSeedHash,
			PlanStep: command.SemanticContext.PlanStep,
		}
		if command.SemanticContext.PlanStep == runtimecontract.RuntimeSemanticPlanStepCoverageOrder {
			document.CoverageOrder = []common.CoverageWorkItemID{item.ID}
		} else if command.SemanticContext.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch {
			if len(repository.aggregate.CandidateRequestSessions) < 2 {
				t.Errorf("proof-batch provider call preceded persisted expected-document session: %+v", repository.aggregate.CandidateRequestSessions)
				return
			}
			if command.CandidateAttempt == 0 {
				proofManifestHash = command.SemanticManifest.Hash
				proofStablePrefixHash = command.StablePrefixHash
				proofBatchID = command.SemanticContext.BatchID
			} else if command.CandidateAttempt != 1 || command.SemanticManifest.Hash != proofManifestHash || command.StablePrefixHash != proofStablePrefixHash || command.SemanticContext.BatchID != proofBatchID || command.SemanticCorrectionDirective == nil || command.PreviousSemanticRejection == nil {
				t.Errorf("semantic correction changed immutable request identity or lost typed rejection: %+v", command)
				return
			}
			document.BatchID = command.SemanticContext.BatchID
			document.BatchSequence = command.SemanticContext.BatchSequence
			document.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), command.SemanticContext.BatchCoverageWorkItemIDs...)
			proposal := runtimecontract.RuntimeSemanticProofProposal{
				CoverageWorkItemID: item.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				OperationKind: runtimecontract.StepKindFileEdit,
				Target: runtimecontract.RuntimeOperationTargetAuthorization{
					Policy:            runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
					OwnerSelectorRefs: []string{"workspace-relative-new-file-target"}, ProspectiveConstraintRefs: []string{"workspace-relative-path"},
					ProspectiveTargetRefs: []string{"new-target.txt"},
				},
				RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
				RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
				DeclarationRef:        "semantic/session-test",
			}
			if command.CandidateAttempt == 1 {
				proposal.Target = runtimecontract.RuntimeOperationTargetAuthorization{
					Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"target.txt"},
				}
			}
			document.Proposals = []runtimecontract.RuntimeSemanticProofProposal{proposal}
		}
		payload, encodeErr := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(command.SemanticManifest, document)
		if encodeErr != nil {
			t.Errorf("encode semantic document: %v", encodeErr)
			return
		}
		if command.SemanticContext.PlanStep == runtimecontract.RuntimeSemanticPlanStepProofBatch && command.CandidateAttempt == 0 {
			payload = []byte(strings.Replace(string(payload), `"prospectiveTargetRefs":["new-target.txt"]`, `"prospectiveTargetRefs":["target.txt"]`, 1))
		}
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
			DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: command.ExpectedDocument.Hash, DocumentOutcome: runtimeCompleteDocumentOutcome(command.ExpectedDocument, payload),
		}), nil)
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
	if evaluated.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningOrderAccepted || evaluated.SemanticPlanning.Document == nil || requestCount != 1 || len(evaluated.CandidateAttempts) != 0 || evaluated.ActiveDispatch != nil {
		t.Fatalf("semantic document crossed into operation authority: ledger=%+v requests=%d", evaluated.SemanticPlanning, requestCount)
	}
	batched, err := module.driveRuntimeSemanticPlanning(ctx, evaluated)
	if err != nil {
		t.Fatal(err)
	}
	if batched.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningProofBatchRequestOpen || len(batched.CandidateRequestSessions) != 1 || batched.SemanticPlanning.LogicalRequestID == started.SemanticPlanning.LogicalRequestID || requestCount != 1 {
		t.Fatalf("semantic proof batch was not persisted before request-session creation: ledger=%+v sessions=%+v requests=%d", batched.SemanticPlanning, batched.CandidateRequestSessions, requestCount)
	}
	proofObserved, err := module.driveRuntimeSemanticPlanning(ctx, batched)
	if err != nil {
		t.Fatal(err)
	}
	if proofObserved.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningCorrectionRequired || proofObserved.SemanticPlanning.Attempt != 1 || len(proofObserved.SemanticPlanning.Rejections) != 1 || len(proofObserved.CandidateRequestSessions) != 3 || requestCount != 2 {
		t.Fatalf("contradictory proof did not open one bounded correction: ledger=%+v sessions=%+v requests=%d", proofObserved.SemanticPlanning, proofObserved.CandidateRequestSessions, requestCount)
	}
	corrected, err := module.driveRuntimeSemanticPlanning(ctx, proofObserved)
	if err != nil {
		t.Fatal(err)
	}
	if corrected.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningProofBatchDocumentAccepted || corrected.SemanticPlanning.Attempt != 1 || len(corrected.SemanticPlanning.Rejections) != 1 || requestCount != 3 || len(corrected.CandidateAttempts) != 0 || corrected.ActiveDispatch != nil {
		t.Fatalf("corrected proof lost bounded lineage or crossed into operation authority: ledger=%+v requests=%d", corrected.SemanticPlanning, requestCount)
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
		Selection:           runtimecontract.ProviderInferenceSelection{Target: aggregate.Facts.Activation.ProviderSelection.Target},
		ContextWindowTokens: 8192, ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048,
		MaxInputTokens: 4096, MaxBytes: 65536, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1",
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt"}, ObservedRevision: "file-revision-1"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	repository.aggregate = aggregate
	module := newRuntimeGroundedTargetSequenceModule(t, hub, repository)
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
			DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: command.ExpectedDocument.Hash,
			Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "semantic_provider_protocol_invalid",
				Signature: "semantic-provider-protocol-invalid", Reason: "provider returned an invalid semantic envelope",
			}),
		}), nil)
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
	if providerCalls != 1 || failed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed || failed.Decision.Terminal == nil || len(failed.CandidateRequestSessions) != 1 {
		t.Fatalf("semantic provider failure did not seal one terminal lineage: %+v", failed)
	}
	session := failed.CandidateRequestSessions[0]
	if session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || session.ProviderSubmitted || session.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol || session.LastOutcome.Code != "semantic_provider_protocol_invalid" || session.LastOutcome.Signature != "semantic-provider-protocol-invalid" || len(failed.CandidateAttempts) != 0 || failed.ActiveDispatch != nil {
		t.Fatalf("semantic terminal retained open provider or operation work: session=%+v aggregate=%+v", session, failed)
	}
	reloaded, err := repository.Load(ctx, failed.Facts.WorkspaceID, failed.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	restarted := newRuntimeGroundedTargetSequenceModule(t, hub, repository)
	replayed, err := restarted.driveRuntimeSemanticPlanning(ctx, reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 1 || replayed.StateRevision != failed.StateRevision || replayed.Facts.Failure == nil || replayed.Facts.Failure.OriginalDiagnostic == nil || replayed.Facts.Failure.OriginalDiagnostic.OriginalCode != "semantic_provider_protocol_invalid" || replayed.Facts.Failure.OriginalDiagnostic.Signature != "semantic-provider-protocol-invalid" {
		t.Fatalf("semantic fresh reload changed exact failure or replayed provider: providerCalls=%d aggregate=%+v", providerCalls, replayed)
	}
}

func TestRuntimeSemanticDriveEmptyDocumentTerminatesWithoutWholeDocumentRecovery(t *testing.T) {
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
		Selection:           runtimecontract.ProviderInferenceSelection{Target: aggregate.Facts.Activation.ProviderSelection.Target},
		ContextWindowTokens: 8192, ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048,
		MaxInputTokens: 4096, MaxBytes: 65536, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1",
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt"}, ObservedRevision: "file-revision-1"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	repository.aggregate = aggregate
	module := newRuntimeGroundedTargetSequenceModule(t, hub, repository)
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.DocumentKind != runtimecontract.RuntimeProviderDocumentSemanticPlan {
			t.Errorf("semantic empty-document request kind = %q", command.DocumentKind)
		}
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
			DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: command.ExpectedDocument.Hash,
			DocumentOutcome:      runtimeProviderFinalOutputMissingOutcome(command.ExpectedDocument),
		}), nil)
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
	if providerCalls != 1 || failed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed ||
		failed.Decision.State != runtimecontract.ConvergenceStateFailed || failed.Decision.Terminal == nil ||
		failed.Decision.Terminal.Code != "semantic_contract_failed" || failed.Facts.Failure == nil ||
		failed.Facts.Failure.OriginalDiagnostic == nil || failed.Facts.Failure.OriginalDiagnostic.OriginalCode != "provider_final_output_missing" ||
		len(failed.CandidateAttempts) != 0 || failed.ActiveDispatch != nil || failed.Facts.Recovery != aggregate.Facts.Recovery {
		t.Fatalf("semantic empty document did not seal one terminal without recovery: calls=%d aggregate=%+v", providerCalls, failed)
	}
	if len(failed.CandidateRequestSessions) != 1 {
		t.Fatalf("semantic empty-document sessions = %+v", failed.CandidateRequestSessions)
	}
	session := failed.CandidateRequestSessions[0]
	if session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || session.ProviderSubmitted ||
		session.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol ||
		session.LastOutcome.Code != "provider_final_output_missing" || session.IncompleteOutcomes != 0 || session.NextSequence != 1 ||
		session.LastDocumentFraming != nil || session.ExpectedDocument == nil || session.ExpectedDocument.MaxRecoveryAttempts <= 0 {
		t.Fatalf("semantic empty-document terminal session = %+v", session)
	}

	reloaded, err := repository.Load(ctx, failed.Facts.WorkspaceID, failed.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := newRuntimeGroundedTargetSequenceModule(t, hub, repository).driveRuntimeSemanticPlanning(ctx, reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 1 || replayed.StateRevision != failed.StateRevision || replayed.Facts.Recovery != failed.Facts.Recovery {
		t.Fatalf("semantic empty-document terminal replayed provider or entered recovery: calls=%d aggregate=%+v", providerCalls, replayed)
	}
}

func TestRuntimeSemanticDriveBodyIdleExhaustionReloadsWithoutProviderReplay(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	bootstrap := runtimeBootstrap()
	facts, err := bootstrap.InitialFacts("ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	facts.RecoveryPolicy.MaxProviderTransportAttempts = 1
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
		Selection:           runtimecontract.ProviderInferenceSelection{Target: aggregate.Facts.Activation.ProviderSelection.Target},
		ContextWindowTokens: 8192, ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048,
		MaxInputTokens: 4096, MaxBytes: 65536, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1",
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt"}, ObservedRevision: "file-revision-1"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	repository.aggregate = aggregate
	module := newRuntimeGroundedTargetSequenceModule(t, hub, repository)
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
			DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: command.ExpectedDocument.Hash,
			Feedback: runtimeProviderFeedbackForTest(t, command, runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassProviderTransient, Code: "provider_body_idle_timeout",
				Signature: "semantic-provider-body-idle/exact", Reason: "semantic provider body became idle",
			}),
		}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	started, err := module.driveRuntimeSemanticPlanning(ctx, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := module.driveRuntimeSemanticPlanning(ctx, started)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 1 || pending.Facts.Recovery.ProviderTransportAttempts != 1 || len(pending.CandidateRequestSessions) != 1 || pending.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTransportPending || pending.Facts.Failure != nil {
		t.Fatalf("semantic body-idle did not consume one atomic recovery allowance: calls=%d aggregate=%+v", providerCalls, pending)
	}
	failed, err := module.driveRuntimeSemanticPlanning(ctx, pending)
	if err != nil {
		t.Fatal(err)
	}
	session := failed.CandidateRequestSessions[0]
	if providerCalls != 2 || failed.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed || failed.Decision.State != runtimecontract.ConvergenceStateFailed || failed.Facts.Failure == nil || failed.Facts.Failure.OriginalDiagnostic == nil || failed.Facts.Failure.OriginalDiagnostic.OriginalCode != "provider_body_idle_timeout" || failed.Facts.Failure.OriginalDiagnostic.Signature != "semantic-provider-body-idle/exact" || session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || session.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient || session.LastOutcome.Code != "provider_body_idle_timeout" || session.LastOutcome.Signature != "semantic-provider-body-idle/exact" || session.TransportOutcomes != 2 || len(failed.CandidateAttempts) != 0 || failed.ActiveDispatch != nil {
		t.Fatalf("semantic body-idle exhaustion lost exact atomic terminal: calls=%d aggregate=%+v", providerCalls, failed)
	}
	reloaded, err := repository.Load(ctx, failed.Facts.WorkspaceID, failed.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	restarted := newRuntimeGroundedTargetSequenceModule(t, hub, repository)
	replayed, err := restarted.driveRuntimeSemanticPlanning(ctx, reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 2 || replayed.StateRevision != failed.StateRevision || replayed.Facts.Failure == nil || replayed.Facts.Failure.OriginalDiagnostic == nil || replayed.Facts.Failure.OriginalDiagnostic.OriginalCode != "provider_body_idle_timeout" {
		t.Fatalf("semantic body-idle fresh reload replayed provider or changed failure: calls=%d aggregate=%+v", providerCalls, replayed)
	}
}

func TestRuntimeSemanticDriveMissingCatalogObservationReopensGenerationWithoutOwnerProbe(t *testing.T) {
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
	aggregate = runtimeTestAggregateWithSemanticPlanning(t, aggregate)
	if aggregate.SemanticPlanning.Manifest == nil || aggregate.SemanticPlanning.Document == nil || len(aggregate.SemanticPlanning.Document.Proposals) == 0 {
		t.Fatalf("sealed semantic fixture has no proposal: %+v", aggregate.SemanticPlanning)
	}
	catalog := aggregate.SemanticPlanning.Manifest.OwnerCatalogs[0].Normalized()
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	// Deliberately omit the prerequisite observation. The sealed manifest is
	// therefore stale even though the admissibility catalog itself is present.
	aggregate.Facts.Resources.PrerequisiteObservations = nil
	repository.aggregate = aggregate

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir(),
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	ownerProbeCalls := 0
	fileObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	fileObserver.Subscribe(fileevents.TopicProbeRuntimeSemanticProofFeasibility, func(_ magicevent.Event, result magicevent.Result) {
		ownerProbeCalls++
		result.Set(nil, nil)
	})
	t.Cleanup(func() { fileObserver.Unsubscribe(fileevents.TopicProbeRuntimeSemanticProofFeasibility) })

	module := newRuntimeGroundedTargetSequenceModule(t, hub, repository)
	proposal := aggregate.SemanticPlanning.Document.Proposals[0]
	beforeRevision := aggregate.StateRevision
	updated, err := module.probeRuntimeSemanticProposal(ctx, aggregate, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if ownerProbeCalls != 0 {
		t.Fatalf("missing catalog observation called capability owner %d time(s)", ownerProbeCalls)
	}
	if updated.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired {
		t.Fatalf("missing observation did not reopen a fresh semantic generation: %+v", updated.SemanticPlanning)
	}
	if len(updated.SemanticPlanning.History) != 1 || updated.SemanticPlanning.History[0].Snapshot.Manifest == nil ||
		updated.SemanticPlanning.History[0].Snapshot.Manifest.Hash != aggregate.SemanticPlanning.Manifest.Hash {
		t.Fatalf("missing observation did not retain the retired generation: %+v", updated.SemanticPlanning.History)
	}
	if updated.StateRevision != beforeRevision+1 || updated.Facts.Recovery.PrerequisiteRefreshes != aggregate.Facts.Recovery.PrerequisiteRefreshes ||
		len(updated.PrerequisiteInvalidations) != len(aggregate.PrerequisiteInvalidations) {
		t.Fatalf("missing observation changed an unrelated recovery/invalidation ledger: before=%+v after=%+v", aggregate, updated)
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
		aggregate.Facts.DefinitionFocuses,
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
	order := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder: []common.CoverageWorkItemID{manifest.CoverageItems[0].ID},
	}
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	stablePrefixHash := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(aggregate.Definition, aggregate.Facts.Activation.ExecutionContract)
	expected := runtimeSemanticExpectedDocument(aggregate, runtimecontract.RuntimeSemanticPlanStepCoverageOrder, nil, 0)
	expected.MaxOutputTokens = 4096
	expected.MaxRecoveryAttempts = 1
	aggregate, session, err := runtimeTestControllerEnsureCandidateRequestWithExpected(controller, ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, stablePrefixHash, manifest.Hash, "", expected)
	if err != nil {
		t.Fatal(err)
	}
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticPlanRequestStarted, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: stablePrefixHash, LogicalRequestID: session.LogicalRequestID},
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
		Kind: runtime.RuntimeInternalEventSemanticPlanEvaluated, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{SemanticDocument: &order, LogicalRequestID: session.LogicalRequestID},
	})
	if err != nil {
		t.Fatal(err)
	}
	aggregate = result.Aggregate
	batch, err := semanticplan.CompileFocusBatch(
		manifest, order, 0, "semantic-budget-proof-request", aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err = controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticPlanBatchCompiled, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{SemanticPlanBatch: &batch},
	})
	if err != nil {
		t.Fatal(err)
	}
	aggregate = result.Aggregate
	proof := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchID: batch.BatchID, BatchSequence: batch.Sequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...),
		Proposals:                []runtimecontract.RuntimeSemanticProofProposal{proposal},
	}
	if err := proof.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	result, err = controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticPlanEvaluated, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Transaction: &runtime.RuntimeInternalTransactionPayload{SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err = controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticFeasibilityRequired, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{},
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
		Kind: runtime.RuntimeInternalEventSemanticFeasibilityObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility},
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
