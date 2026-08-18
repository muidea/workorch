package result

import (
	"context"
	"errors"
	"testing"
	"time"

	resultbiz "github.com/muidea/workorch/internal/modules/blocks/result/biz"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestModuleSetupRequiresRepositories(t *testing.T) {
	if ModuleID != "blocks/result" {
		t.Fatalf("ModuleID = %q, want relocated stateful block ID", ModuleID)
	}
	if New().ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", New().ID(), ModuleID)
	}
	if err := New().Configure(resultbiz.Dependencies{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Setup err = %v, want invalid argument", err)
	}
	if err := New().Configure(resultbiz.Dependencies{Results: memory.NewResultRepository()}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Setup with missing artifacts err = %v, want invalid argument", err)
	}
	if err := New().Configure(resultbiz.Dependencies{
		Results:   memory.NewResultRepository(),
		Artifacts: memory.NewArtifactRepository(),
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Setup with missing EventHub err = %v, want invalid argument", err)
	}
}

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}

	if err := module.Configure(resultModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil || module.bizPtr.EventHub() == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleManagesResultsAndArtifacts(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	resultRepo := memory.NewResultRepository()
	artifactRepo := memory.NewArtifactRepository()
	result := validModuleTestResult()
	artifact := validModuleTestArtifact(result.ID)
	if err := resultRepo.Create(ctx, result); err != nil {
		t.Fatal(err)
	}
	if err := artifactRepo.Create(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	module := New()
	if err := module.Configure(resultbiz.Dependencies{
		Results:   resultRepo,
		Artifacts: artifactRepo,
		EventHub:  hub,
	}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	results := sendResultCommand[resultevents.ListResultsByRunResult](t, ctx, hub, resultevents.TopicResultCommandListByRun, resultevents.ListResultsByRunCommand{WorkspaceID: result.WorkspaceID, RunID: result.RunID}).Results
	if len(results) != 1 || results[0].ID != result.ID {
		t.Fatalf("results = %+v, want created result", results)
	}
	artifacts := sendResultCommand[resultevents.ListArtifactsByResultResult](t, ctx, hub, resultevents.TopicArtifactCommandListByResult, resultevents.ListArtifactsByResultCommand{WorkspaceID: result.WorkspaceID, ResultID: result.ID}).Artifacts
	if len(artifacts) != 1 || artifacts[0].ID != artifact.ID {
		t.Fatalf("artifacts = %+v, want created artifact", artifacts)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("biz after teardown = %v, want nil", module.bizPtr)
	}
}

func TestModuleRunIsIdempotent(t *testing.T) {
	ctx := context.Background()
	module := New()
	if err := module.Configure(resultbiz.Dependencies{
		Results:   memory.NewResultRepository(),
		Artifacts: memory.NewArtifactRepository(),
		EventHub:  magicevent.NewHub(16),
	}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if err := module.Configure(resultbiz.Dependencies{
		Results:   memory.NewResultRepository(),
		Artifacts: memory.NewArtifactRepository(),
		EventHub:  magicevent.NewHub(16),
	}); err != nil {
		t.Fatalf("Setup after Teardown: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run after Teardown: %v", err)
	}
}

func TestModuleRejectsInvalidResultCommandWithoutSideEffect(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	resultRepo := memory.NewResultRepository()
	module := New()
	if err := module.Configure(resultbiz.Dependencies{
		Results:   resultRepo,
		Artifacts: memory.NewArtifactRepository(),
		EventHub:  hub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	result := hub.Send(magicevent.NewEventWithContext(
		resultevents.TopicResultCommandGet,
		"test",
		resultevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		resultevents.GetResultCommand{},
	))
	if result == nil || result.Error() == nil {
		t.Fatalf("invalid create result = %v, want an error", result)
	}
	items, err := resultRepo.ListByRun(ctx, "run-result-invalid")
	if err != nil {
		t.Fatalf("ListByRun after invalid command: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %+v, want no side effects from invalid command", items)
	}
}

func TestModuleHandlesResultCommandsOverEventHub(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	resultRepo := memory.NewResultRepository()
	result := validModuleTestResult()
	if err := resultRepo.Create(ctx, result); err != nil {
		t.Fatal(err)
	}
	module := New()
	if err := module.Configure(resultbiz.Dependencies{
		Results:   resultRepo,
		Artifacts: memory.NewArtifactRepository(),
		EventHub:  hub,
	}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	response := sendResultCommand[resultevents.ListResultsByRunResult](t, ctx, hub, resultevents.TopicResultCommandListByRun, resultevents.ListResultsByRunCommand{WorkspaceID: result.WorkspaceID, RunID: result.RunID})
	if len(response.Results) != 1 || response.Results[0].ID != result.ID {
		t.Fatalf("items = %+v, want created result", response.Results)
	}
}

func TestResultOwnerListContractsAreBoundedAndCursorBased(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	results := memory.NewResultRepository()
	for _, id := range []common.ResultID{"result-a", "result-b", "result-c"} {
		item := validModuleTestResult()
		item.ID = id
		if err := results.Create(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	module := New()
	if err := module.Configure(resultbiz.Dependencies{Results: results, Artifacts: memory.NewArtifactRepository(), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	first := sendResultCommand[resultevents.ListResultsByRunResult](t, ctx, hub, resultevents.TopicResultCommandListByRun, resultevents.ListResultsByRunCommand{
		WorkspaceID: "ws-result-test", RunID: "run-result-test", Limit: 2,
	})
	if len(first.Results) != 2 || first.Results[0].ID != "result-a" || first.Results[1].ID != "result-b" || first.NextCursor != "result-b" {
		t.Fatalf("first page = %+v", first)
	}
	second := sendResultCommand[resultevents.ListResultsByRunResult](t, ctx, hub, resultevents.TopicResultCommandListByRun, resultevents.ListResultsByRunCommand{
		WorkspaceID: "ws-result-test", RunID: "run-result-test", Cursor: first.NextCursor, Limit: 2,
	})
	if len(second.Results) != 1 || second.Results[0].ID != "result-c" || second.NextCursor != "" {
		t.Fatalf("second page = %+v", second)
	}

	invalid := hub.Send(magicevent.NewEventWithContext(
		resultevents.TopicResultCommandListByRun, "test", resultevents.CommandDestination, magicevent.NewHeader(), ctx,
		resultevents.ListResultsByRunCommand{WorkspaceID: "ws-result-test", RunID: "run-result-test", Limit: resultevents.MaxListLimit + 1},
	))
	if invalid == nil || invalid.Error() == nil {
		t.Fatalf("oversized list response = %v, want error", invalid)
	}
}

func TestResultOwnerCommandsPublishLifecycleNotifications(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := New()
	if err := module.Configure(resultbiz.Dependencies{
		Results:   memory.NewResultRepository(),
		Artifacts: memory.NewArtifactRepository(),
		EventHub:  hub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	const observerID = "/result/consumers/blocks/result/owner-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	results := make(chan struct {
		topic string
		data  resultevents.ResultChangedData
	}, 2)
	for _, topic := range []string{resultevents.TopicResultAccepted, resultevents.TopicResultRejected} {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			if data, ok := event.Data().(resultevents.ResultChangedData); ok {
				results <- struct {
					topic string
					data  resultevents.ResultChangedData
				}{topic: topic, data: data}
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
	}
	t.Cleanup(func() {
		observer.Unsubscribe(resultevents.TopicResultAccepted)
		observer.Unsubscribe(resultevents.TopicResultRejected)
	})

	item := validModuleTestResult()
	createdResult := sendResultLifecycleCommand(t, ctx, hub, terminalDeliveryCommand(item, runtimecontract.TerminalOutcomeKindCompleted, "delivery-accepted"))
	if createdResult.Result == nil || !createdResult.Transitioned {
		t.Fatalf("terminal result response = %+v", createdResult)
	}
	created := receiveResultNotification(t, results)
	wantAcceptedID := expectedRuntimeResultID(item.WorkspaceID, item.RunID)
	if created.topic != resultevents.TopicResultAccepted || created.data.ResultID != wantAcceptedID || created.data.State != item.State || created.data.NotificationID == "" {
		t.Fatalf("created notification = %+v", created)
	}

	rejectedItem := validModuleTestResult()
	rejectedItem.RunID = "run-result-test-rejected"
	rejectedItem.State = common.ResultStateRejected
	rejectedItem.Summary = "rejected"
	rejectedResult := sendResultLifecycleCommand(t, ctx, hub, terminalDeliveryCommand(rejectedItem, runtimecontract.TerminalOutcomeKindFailed, "delivery-rejected"))
	if rejectedResult.Result == nil || !rejectedResult.Transitioned {
		t.Fatalf("rejected terminal result response = %+v", rejectedResult)
	}
	updated := receiveResultNotification(t, results)
	if updated.topic != resultevents.TopicResultRejected || updated.data.ResultID != expectedRuntimeResultID(rejectedItem.WorkspaceID, rejectedItem.RunID) || updated.data.Summary != "rejected" {
		t.Fatalf("updated notification = %+v", updated)
	}
}

func receiveResultNotification(t *testing.T, received <-chan struct {
	topic string
	data  resultevents.ResultChangedData
}) struct {
	topic string
	data  resultevents.ResultChangedData
} {
	t.Helper()
	select {
	case item := <-received:
		return item
	case <-time.After(time.Second):
		t.Fatal("typed result notification not received")
		return struct {
			topic string
			data  resultevents.ResultChangedData
		}{}
	}
}

func resultModuleContractDependencies() resultbiz.Dependencies {
	return resultbiz.Dependencies{
		Results:   memory.NewResultRepository(),
		Artifacts: memory.NewArtifactRepository(),
		EventHub:  magicevent.NewHub(16),
	}
}

func sendResultCommand[Result any, Command any](t *testing.T, ctx context.Context, hub magicevent.Hub, topic string, command Command) Result {
	t.Helper()
	ev := magicevent.NewEventWithContext(topic, "test", resultevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := hub.Send(ev)
	if result == nil || result.Error() != nil {
		t.Fatalf("send %s error: %v", topic, result)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatalf("result %s: %v", topic, err)
	}
	typed, ok := data.(Result)
	if !ok {
		t.Fatalf("result %s type = %T", topic, data)
	}
	return typed
}

func sendResultLifecycleCommand(t *testing.T, ctx context.Context, hub magicevent.Hub, command resultevents.RuntimeTerminalDeliveryFact) resultevents.ResultLifecycleCommandResult {
	t.Helper()
	ev := magicevent.NewEventWithContext(resultevents.TopicResultFactTerminalDelivery, "kernel/orchestrator", resultevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := hub.Send(ev)
	if result == nil || result.Error() != nil {
		t.Fatalf("send terminal delivery error: %v", result)
	}
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	typed, ok := data.(resultevents.ResultLifecycleCommandResult)
	if !ok {
		t.Fatalf("terminal delivery result type = %T", data)
	}
	return typed
}

func terminalDeliveryCommand(item *resultmodel.Result, kind runtimecontract.TerminalOutcomeKind, deliveryID string) resultevents.RuntimeTerminalDeliveryFact {
	return resultevents.RuntimeTerminalDeliveryFact{
		RuntimeTerminalDeliveryFact: runtimecontract.RuntimeTerminalDeliveryFact{RuntimeTerminalDeliveryIdentity: runtimecontract.RuntimeTerminalDeliveryIdentity{
			WorkspaceID: item.WorkspaceID, RunID: item.RunID, TerminalDeliveryID: deliveryID,
			AuthoritySnapshotID: "authority-" + deliveryID, DefinitionRevision: "definition-" + deliveryID,
			EvidenceRevision: "evidence-" + deliveryID, DispatchID: "dispatch-" + deliveryID,
			Outcome: runtimecontract.TerminalOutcome{Kind: kind, Code: string(kind), Summary: item.Summary},
		}, SourceOwner: "kernel/orchestrator", CausationID: "causation-" + deliveryID},
		Material: resultevents.ResultMaterial{
			WorkspaceID: item.WorkspaceID, RunID: item.RunID, GoalID: item.GoalID, DoDID: item.DoDID,
			Title: item.Title, Summary: item.Summary, SourceType: item.SourceType, SourceRef: item.SourceRef,
		},
	}
}

func validModuleTestResult() *resultmodel.Result {
	return &resultmodel.Result{
		ID:          "result-test",
		WorkspaceID: "ws-result-test",
		RunID:       "run-result-test",
		GoalID:      "goal-result-test",
		DoDID:       "dod-result-test",
		State:       common.ResultStateAccepted,
		Title:       "accepted result",
		Summary:     "accepted",
	}
}

func expectedRuntimeResultID(workspaceID common.WorkspaceID, runID common.RunID) common.ResultID {
	return common.ResultID(string(workspaceID) + "/" + string(runID) + "/result/dod-assessment")
}

func validModuleTestArtifact(resultID common.ResultID) *resultmodel.Artifact {
	return &resultmodel.Artifact{
		ID:          "artifact-test",
		WorkspaceID: "ws-result-test",
		RunID:       "run-result-test",
		ResultID:    resultID,
		State:       common.ArtifactStateCreated,
		Name:        "artifact",
		Summary:     "artifact",
	}
}
