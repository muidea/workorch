package orchestrator

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mustRuntimeGovernancePayload(t *testing.T, input map[string]any) []decisionevents.DecisionEvidenceField {
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
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-v2", WorkspaceID: "ws-v2", State: common.RunStateActive}); err != nil {
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

	biz := orchestratorbiz.NewRuntimeOrchestrator(nil, nil, nil, nil, nil)
	if err := biz.ConfigureRuntimeV2Repository(runtimeV2StateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	facts := runtimeV2Aggregate(t, 1).Facts
	facts.Capabilities.GovernanceKinds = []string{"decision/resolution"}
	facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassGovernanceDependency, Signature: "external-approval", Message: "需要外部裁决"}
	definition := runtimeV2Definition()
	definition.CapabilityContract = append(definition.CapabilityContract, "decision/resolution")
	initialized, err := biz.InitializeRuntimeV2(ctx, facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Candidate: runtimecontract.RuntimeCandidate{Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
				Kind: "external-approval", SubjectRef: "subject-1", Reason: "需要外部裁决",
			}}},
		}, nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate) })
	background := &recordingBackgroundRoutine{}
	module := &Module{bizPtr: biz, rawEventHub: hub, evidenceRecords: memory.NewEvidenceRecordRepository(), backgroundRoutine: background}
	sealed, err := module.requestAndSealRuntimeV2CandidateWithRetry(ctx, initialized, "stable-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Decision.Action != runtimecontract.RuntimeActionRequestGovernance || sealed.ActiveDispatch == nil || len(sealed.GovernanceRequests) != 1 {
		t.Fatalf("governed aggregate = %+v", sealed)
	}
	governanceRequest := sealed.GovernanceRequests[0]
	ticket, err := decisionTickets.Get(ctx, governanceRequest.TicketID)
	if err != nil || ticket.WorkspaceID != facts.WorkspaceID || ticket.RunID != facts.RunID || ticket.State != common.DecisionTicketStateOpen {
		t.Fatalf("governance ticket = %+v err=%v", ticket, err)
	}
	replayed, err := module.requestAndSealRuntimeV2CandidateWithRetry(ctx, sealed, "stable-prefix")
	if err != nil || replayed.StateRevision != sealed.StateRevision || len(replayed.GovernanceRequests) != 1 {
		t.Fatalf("governance request replay = %+v err=%v", replayed, err)
	}

	observer := magicevent.NewSimpleObserver(ModuleID, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution, module.handleRuntimeV2GovernanceResolution)
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket, module.handleRuntimeV2GovernanceTicketResolution)
	observer.Subscribe(decisionevents.TopicDecisionCommandRecordEvidence, module.handleDecisionEvidenceCommand)
	t.Cleanup(func() {
		observer.Unsubscribe(decisionevents.TopicDecisionCommandRecordEvidence)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceResolution)
	})

	resolutionResult := hub.Send(magicevent.NewEventWithContext(
		decisionevents.TopicDecisionCommandEnsureResolved,
		"runtime-v2-governance-test",
		decisionevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		decisionevents.EnsureResolvedCommand{
			ResolutionID: "resolution-1", TicketID: governanceRequest.TicketID,
			WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
			StepID:  common.StepID("runtime-v2/" + sealed.ActiveDispatch.ID),
			Type:    decisioncommon.ResolutionTypeProvideMissingInfo,
			Payload: mustRuntimeGovernancePayload(t, map[string]any{"summary": "治理已完成"}),
		},
	))
	if resolutionResult == nil || resolutionResult.Error() != nil {
		var resultErr error
		if resolutionResult != nil {
			resultErr = resolutionResult.Error()
		}
		t.Fatalf("decision resolution result error = %v", resultErr)
	}
	loaded, err := biz.LoadRuntimeV2(ctx, facts.WorkspaceID, facts.RunID)
	if err != nil || loaded.ActiveDispatch != nil || len(loaded.GovernanceResolutions) != 1 || loaded.Decision.Action != runtimecontract.RuntimeActionProduceStateChange {
		t.Fatalf("persisted governance aggregate = %+v err=%v", loaded, err)
	}
	if background.asyncFunctionCalls != 1 || background.lastAsyncFunction == nil {
		t.Fatalf("Runtime V2 governance continuation calls=%d fn=%v", background.asyncFunctionCalls, background.lastAsyncFunction != nil)
	}
}
