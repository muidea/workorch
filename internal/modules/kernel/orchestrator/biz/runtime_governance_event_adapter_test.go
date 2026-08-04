package orchestrator

// Owner Biz governance adapter tests.

import (
	"context"
	"path/filepath"
	"testing"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	magicevent "github.com/muidea/magicCommon/event"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mustRuntimeGovernancePayload(t *testing.T, input map[string]any) []workorcheventpayload.Field {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatalf("DecisionFieldsFromMap: %v", err)
	}
	return payload.Fields
}

func TestRuntimeGovernanceResolutionCommitsThroughTypedEventHubCommand(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	definition := runtimeDefinition()
	definition.CapabilityContract = append(definition.CapabilityContract, "decision/resolution")
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, definition)); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	decisionTickets := memory.NewDecisionTicketRepository()
	decisionOwner := decisionmodule.New()
	if err := decisionOwner.Configure(decisionmodule.Dependencies{
		Tickets: decisionTickets, Resolutions: memory.NewDecisionResolutionRepository(), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := decisionOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = decisionOwner.Stop(ctx) })

	biz := newRuntimeTestOwner()
	if err := biz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	facts := runtimeAggregate(t, 1).Facts
	facts.Capabilities.GovernanceKinds = []string{"decision/resolution"}
	facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassGovernanceDependency, Signature: "external-approval", Message: "需要外部裁决", GovernanceDependency: &runtimecontract.ExternalDependency{Kind: "external-approval", SubjectRef: "subject-1", Reason: "需要外部裁决"}}
	initialized, err := biz.InitializeRuntime(ctx, facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeCandidateResultForTest(t, command, runtimecontract.RuntimeCandidate{Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
			Kind: "external-approval", SubjectRef: "subject-1", Reason: "需要外部裁决",
		}}}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	background := &recordingBackgroundRoutine{}
	module := newTestBiz(biz, hub, background)
	module.evidenceRecords = memory.NewEvidenceRecordRepository()
	sealed, err := module.requestAndSealRuntimeCandidateWithRetry(ctx, initialized, "stable-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Decision.State != runtimecontract.ConvergenceStateNeedGovernance || sealed.ActiveDispatch == nil || len(sealed.GovernanceRequests) != 1 {
		t.Fatalf("governed aggregate = %+v", sealed)
	}
	governanceRequest := sealed.GovernanceRequests[0]
	ticket, err := decisionTickets.Get(ctx, governanceRequest.TicketID)
	if err != nil || ticket.WorkspaceID != facts.WorkspaceID || ticket.RunID != facts.RunID || ticket.State != common.DecisionTicketStateOpen {
		t.Fatalf("governance ticket = %+v err=%v", ticket, err)
	}
	replayed, err := module.requestAndSealRuntimeCandidateWithRetry(ctx, sealed, "stable-prefix")
	if err != nil || replayed.StateRevision != sealed.StateRevision || len(replayed.GovernanceRequests) != 1 {
		t.Fatalf("governance request replay = %+v err=%v", replayed, err)
	}

	observer := magicevent.NewSimpleObserver(ModuleID, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution, module.handleRuntimeGovernanceResolution)
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket, module.handleRuntimeGovernanceTicketResolution)
	observer.Subscribe(decisionevents.TopicDecisionCommandRecordEvidence, module.handleDecisionEvidenceCommand)
	t.Cleanup(func() {
		observer.Unsubscribe(decisionevents.TopicDecisionCommandRecordEvidence)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution)
	})

	resolutionResult := hub.Send(magicevent.NewEventWithContext(
		decisionevents.TopicDecisionCommandEnsureResolved,
		"runtime-governance-test",
		decisionevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		decisionevents.EnsureResolvedCommand{
			ResolutionID: "resolution-1", TicketID: governanceRequest.TicketID,
			WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
			StepID:       common.StepID("runtime/" + sealed.ActiveDispatch.ID),
			DecisionKind: "runtime_governance",
			Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
			Payload:      mustRuntimeGovernancePayload(t, map[string]any{"summary": "治理已完成"}),
		},
	))
	if resolutionResult == nil || resolutionResult.Error() != nil {
		var resultErr error
		if resolutionResult != nil {
			resultErr = resolutionResult.Error()
		}
		t.Fatalf("decision resolution result error = %v", resultErr)
	}
	loaded, err := biz.loadRuntime(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || loaded.ActiveDispatch != nil || len(loaded.GovernanceResolutions) != 1 || loaded.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("persisted governance aggregate = %+v err=%v", loaded, err)
	}
	duplicateResult := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket,
		"runtime-governance-test",
		orchestratorevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		orchestratorevents.CommitRuntimeGovernanceTicketResolutionCommand{
			ResolutionID: "resolution-1", TicketID: governanceRequest.TicketID,
			WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
			Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "治理已完成",
		},
	))
	if duplicateResult == nil || duplicateResult.Error() != nil {
		t.Fatalf("duplicate governance continuation result = %+v", duplicateResult)
	}
	duplicateData, duplicateErr := duplicateResult.Get()
	duplicateResponse, duplicateOK := duplicateData.(orchestratorevents.CommitRuntimeGovernanceResolutionResult)
	if duplicateErr != nil || !duplicateOK || duplicateResponse.Disposition != orchestratorevents.RuntimeGovernanceResolutionDispositionDuplicate {
		t.Fatalf("duplicate governance continuation = %+v err=%v", duplicateData, duplicateErr)
	}
	if background.asyncFunctionCalls != 0 {
		t.Fatalf("governance continuation must wait for RunStateChanged(active), calls=%d", background.asyncFunctionCalls)
	}
}

// TestRuntimeGovernanceResolutionFileBackedRestartReplaysExactlyOnce covers
// the durable Decision -> Orchestrator recovery boundary.  The initial
// resolution is fully committed, then every EventHub owner and every file
// repository instance is rebuilt.  A retry with the same resolution identity
// must join the persisted Orchestrator continuation rather than being rejected
// by Decision or creating another candidate/evidence/budget charge.
func TestRuntimeGovernanceResolutionFileBackedRestartReplaysExactlyOnce(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runsPath := filepath.Join(root, "runs.json")
	workspacesPath := filepath.Join(root, "workspaces.json")
	ticketsPath := filepath.Join(root, "decision-tickets.json")
	resolutionsPath := filepath.Join(root, "decision-resolutions.json")

	runs := filerepository.NewRunRepository(runsPath)
	workspaces := filerepository.NewWorkspaceRepository(workspacesPath)
	definition := runtimeDefinition()
	definition.CapabilityContract = append(definition.CapabilityContract, "decision/resolution")
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, definition)); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}

	hub := magicevent.NewHub(16)
	background := &recordingBackgroundRoutine{}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	decisionOwner := decisionmodule.New()
	if err := decisionOwner.Configure(decisionmodule.Dependencies{
		Tickets: filerepository.NewDecisionTicketRepository(ticketsPath), Resolutions: filerepository.NewDecisionResolutionRepository(resolutionsPath), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := decisionOwner.Start(); err != nil {
		t.Fatal(err)
	}
	biz := newRuntimeTestOwner()
	if err := biz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(biz, hub, background)
	module.evidenceRecords = memory.NewEvidenceRecordRepository()
	facts := runtimeAggregate(t, 1).Facts
	facts.Capabilities.GovernanceKinds = []string{"decision/resolution"}
	facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassGovernanceDependency, Signature: "external-approval", Message: "需要外部裁决", GovernanceDependency: &runtimecontract.ExternalDependency{Kind: "external-approval", SubjectRef: "subject-1", Reason: "需要外部裁决"}}
	initialized, err := biz.InitializeRuntime(ctx, facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	candidateCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeCandidateResultForTest(t, command, runtimecontract.RuntimeCandidate{Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
			Kind: "external-approval", SubjectRef: "subject-1", Reason: "需要外部裁决",
		}}}), nil)
	})
	observer := magicevent.NewSimpleObserver(ModuleID, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution, module.handleRuntimeGovernanceResolution)
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket, module.handleRuntimeGovernanceTicketResolution)
	observer.Subscribe(decisionevents.TopicDecisionCommandRecordEvidence, module.handleDecisionEvidenceCommand)

	sealed, err := module.requestAndSealRuntimeCandidateWithRetry(ctx, initialized, "stable-prefix")
	if err != nil || sealed.ActiveDispatch == nil || len(sealed.GovernanceRequests) != 1 {
		t.Fatalf("initial governed aggregate=%+v err=%v", sealed, err)
	}
	request := sealed.GovernanceRequests[0]
	resolve := decisionevents.EnsureResolvedCommand{
		ResolutionID: "resolution-file-restart", TicketID: request.TicketID,
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
		StepID: common.StepID("runtime/" + sealed.ActiveDispatch.ID), DecisionKind: "runtime_governance",
		Type:    decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload: mustRuntimeGovernancePayload(t, map[string]any{"summary": "治理已完成"}),
	}
	initialResult := hub.Send(magicevent.NewEventWithContext(
		decisionevents.TopicDecisionCommandEnsureResolved, "runtime-file-restart", decisionevents.CommandDestination,
		magicevent.NewHeader(), ctx, resolve,
	))
	if initialResult == nil || initialResult.Error() != nil {
		t.Fatalf("initial decision resolution=%+v", initialResult)
	}
	committed, err := biz.loadRuntime(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || len(committed.GovernanceResolutions) != 1 || len(committed.Facts.Evidence.Accepted) != 1 || len(committed.CandidateAttempts) != 0 || committed.ActiveDispatch != nil || candidateCalls != 0 {
		t.Fatalf("initial persisted governance aggregate=%+v candidateCalls=%d err=%v", committed, candidateCalls, err)
	}
	if background.asyncFunctionCalls != 0 {
		t.Fatalf("initial continuation must wait for RunStateChanged(active), calls=%d", background.asyncFunctionCalls)
	}
	if err := decisionOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate)
	observer.Unsubscribe(decisionevents.TopicDecisionCommandRecordEvidence)
	observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket)
	observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution)
	hub.Terminate(ctx)

	// Reconstruct process-local infrastructure and repository instances from the
	// same durable files; no old EventHub observer or controller is reused.
	restartedHub := magicevent.NewHub(16)
	restartedRuns := filerepository.NewRunRepository(runsPath)
	restartedRunOwner := runmodule.New()
	if err := restartedRunOwner.Configure(runmodule.Dependencies{Runs: restartedRuns, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	restartedDecisionOwner := decisionmodule.New()
	if err := restartedDecisionOwner.Configure(decisionmodule.Dependencies{
		Tickets: filerepository.NewDecisionTicketRepository(ticketsPath), Resolutions: filerepository.NewDecisionResolutionRepository(resolutionsPath), EventHub: restartedHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := restartedDecisionOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedDecisionOwner.Stop(ctx) })
	restartedBiz := newRuntimeTestOwner()
	if err := restartedBiz.ConfigureRuntimeRepository(runtimeStateRepository{hub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	restartedBackground := &recordingBackgroundRoutine{}
	restartedModule := newTestBiz(restartedBiz, restartedHub, restartedBackground)
	restartedModule.evidenceRecords = memory.NewEvidenceRecordRepository()
	restartedObserver := magicevent.NewSimpleObserver(ModuleID, restartedHub)
	restartedObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution, restartedModule.handleRuntimeGovernanceResolution)
	restartedObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket, restartedModule.handleRuntimeGovernanceTicketResolution)
	restartedObserver.Subscribe(decisionevents.TopicDecisionCommandRecordEvidence, restartedModule.handleDecisionEvidenceCommand)
	t.Cleanup(func() {
		restartedObserver.Unsubscribe(decisionevents.TopicDecisionCommandRecordEvidence)
		restartedObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket)
		restartedObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution)
		restartedHub.Terminate(ctx)
	})

	for replay := 0; replay < 2; replay++ {
		result := restartedHub.Send(magicevent.NewEventWithContext(
			decisionevents.TopicDecisionCommandEnsureResolved, "runtime-file-restart", decisionevents.CommandDestination,
			magicevent.NewHeader(), ctx, resolve,
		))
		if result == nil || result.Error() != nil {
			t.Fatalf("replay %d resolution=%+v", replay, result)
		}
		data, getErr := result.Get()
		response, ok := data.(decisionevents.DecisionCommandResult)
		if getErr != nil || !ok || response.Ticket == nil || response.Resolution == nil || response.Resolution.ID != resolve.ResolutionID {
			t.Fatalf("replay %d result=%+v getErr=%v", replay, data, getErr)
		}
	}
	reloaded, err := restartedBiz.loadRuntime(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.StateRevision != committed.StateRevision || len(reloaded.GovernanceResolutions) != 1 || len(reloaded.Facts.Evidence.Accepted) != 1 ||
		len(reloaded.CandidateAttempts) != len(committed.CandidateAttempts) || len(reloaded.OperationReceipts) != 0 {
		t.Fatalf("duplicate replay changed persisted authority: before=%+v after=%+v", committed, reloaded)
	}
	if candidateCalls != 0 || restartedBackground.asyncFunctionCalls != 0 {
		t.Fatalf("restart replay candidateCalls=%d continuationCalls=%d", candidateCalls, restartedBackground.asyncFunctionCalls)
	}
	if reloaded.Facts.Recovery.ProviderTransportAttempts != committed.Facts.Recovery.ProviderTransportAttempts ||
		reloaded.Facts.Recovery.OperationRecoveryAttempts != committed.Facts.Recovery.OperationRecoveryAttempts ||
		reloaded.Facts.Recovery.CASReloadAttempts != committed.Facts.Recovery.CASReloadAttempts ||
		reloaded.Facts.Recovery.TerminalDeliveryAttempts != committed.Facts.Recovery.TerminalDeliveryAttempts ||
		reloaded.Facts.Recovery.PrerequisiteRefreshes != committed.Facts.Recovery.PrerequisiteRefreshes {
		t.Fatalf("duplicate replay charged recovery budget: before=%+v after=%+v", committed.Facts.Recovery, reloaded.Facts.Recovery)
	}
}

// P3-B3 governance boundary: a persisted governance dispatch and its bound
// ticket are already authoritative. Rebuilding all file-backed owners while
// it waits must preserve that binding and make zero LLM candidate requests.
func TestRuntimeFileBackedBoundGovernanceRestartPreservesBindingWithoutCandidateCall(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runsPath := filepath.Join(root, "runs.json")
	workspacesPath := filepath.Join(root, "workspaces.json")
	ticketsPath := filepath.Join(root, "decision-tickets.json")
	resolutionsPath := filepath.Join(root, "decision-resolutions.json")
	definition := runtimeDefinition()
	definition.CapabilityContract = append(definition.CapabilityContract, "decision/resolution")

	runs := filerepository.NewRunRepository(runsPath)
	workspaces := filerepository.NewWorkspaceRepository(workspacesPath)
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, definition)); err != nil {
		t.Fatal(err)
	}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-v2", State: common.WorkspaceStateActive, ActiveRunID: "run-v2"}); err != nil {
		t.Fatal(err)
	}

	firstHub := magicevent.NewHub(16)
	firstRunOwner := runmodule.New()
	if err := firstRunOwner.Configure(runmodule.Dependencies{Runs: runs, EventHub: firstHub}); err != nil {
		t.Fatal(err)
	}
	if err := firstRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	firstDecisionOwner := decisionmodule.New()
	if err := firstDecisionOwner.Configure(decisionmodule.Dependencies{
		Tickets: filerepository.NewDecisionTicketRepository(ticketsPath), Resolutions: filerepository.NewDecisionResolutionRepository(resolutionsPath), EventHub: firstHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := firstDecisionOwner.Start(); err != nil {
		t.Fatal(err)
	}

	firstBiz := newRuntimeTestOwner()
	if err := firstBiz.ConfigureRuntimeRepository(runtimeStateRepository{hub: firstHub}); err != nil {
		t.Fatal(err)
	}
	facts := runtimeAggregate(t, 1).Facts
	facts.Capabilities.GovernanceKinds = []string{"decision/resolution"}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceDependency, Signature: "external-approval", Message: "requires external approval",
		GovernanceDependency: &runtimecontract.ExternalDependency{Kind: "external-approval", SubjectRef: "subject-1", Reason: "requires external approval"},
	}
	initialized, err := firstBiz.InitializeRuntime(ctx, facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	firstCandidateCalls := 0
	firstLLMObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, firstHub)
	firstLLMObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { firstCandidateCalls++ })
	sealed, err := newTestBiz(firstBiz, firstHub, nil).requestAndSealRuntimeCandidateWithRetry(ctx, initialized, "governance-restart-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if firstCandidateCalls != 0 || sealed.ActiveDispatch == nil || sealed.ActiveDispatch.Candidate.Governance == nil || len(sealed.GovernanceRequests) != 1 || len(sealed.CandidateAttempts) != 0 {
		t.Fatalf("initial governance dispatch was not candidate-free: calls=%d aggregate=%+v", firstCandidateCalls, sealed)
	}
	request := sealed.GovernanceRequests[0]
	dispatchID := sealed.ActiveDispatch.ID
	if _, err := filerepository.NewDecisionTicketRepository(ticketsPath).Get(ctx, request.TicketID); err != nil {
		t.Fatalf("bound governance ticket was not durable: %v", err)
	}
	firstLLMObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate)
	if err := firstDecisionOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := firstRunOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	firstHub.Terminate(ctx)

	// Recreate all process-local owners from their file roots. No initial Hub,
	// controller, provider response, or ticket repository instance is reused.
	restartedHub := magicevent.NewHub(16)
	restartedRuns := filerepository.NewRunRepository(runsPath)
	restartedRunOwner := runmodule.New()
	if err := restartedRunOwner.Configure(runmodule.Dependencies{Runs: restartedRuns, EventHub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	if err := restartedRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedRunOwner.Stop(ctx) })
	restartedDecisionOwner := decisionmodule.New()
	if err := restartedDecisionOwner.Configure(decisionmodule.Dependencies{
		Tickets: filerepository.NewDecisionTicketRepository(ticketsPath), Resolutions: filerepository.NewDecisionResolutionRepository(resolutionsPath), EventHub: restartedHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := restartedDecisionOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restartedDecisionOwner.Stop(ctx) })
	restartedBiz := newRuntimeTestOwner()
	if err := restartedBiz.ConfigureRuntimeRepository(runtimeStateRepository{hub: restartedHub}); err != nil {
		t.Fatal(err)
	}
	restartedCandidateCalls := 0
	restartedLLMObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, restartedHub)
	restartedLLMObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { restartedCandidateCalls++ })
	t.Cleanup(func() {
		restartedLLMObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate)
		restartedHub.Terminate(ctx)
	})
	reloaded, err := restartedBiz.loadRuntime(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := newTestBiz(restartedBiz, restartedHub, nil).requestAndSealRuntimeCandidateWithRetry(ctx, reloaded, "governance-restart-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if restartedCandidateCalls != 0 || waiting.StateRevision != sealed.StateRevision || waiting.ActiveDispatch == nil ||
		waiting.ActiveDispatch.ID != dispatchID || len(waiting.GovernanceRequests) != 1 ||
		waiting.GovernanceRequests[0].TicketID != request.TicketID || len(waiting.CandidateAttempts) != 0 {
		t.Fatalf("governance restart rebuilt candidate authority instead of waiting: calls=%d aggregate=%+v", restartedCandidateCalls, waiting)
	}
}
