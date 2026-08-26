package result

import (
	"context"
	"errors"
	"testing"
	"time"

	resultbiz "github.com/muidea/workorch/internal/modules/blocks/result/biz"

	magicevent "github.com/muidea/magicCommon/event"
	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestResultWorkspaceDeletionFenceHidesRetainedDataAndPurgeIsStable(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repositories := memory.NewResultOwnerRepositorySet()
	module := New()
	if err := module.Configure(resultbiz.Dependencies{
		OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	retained := validModuleTestResult()
	retained.WorkspaceID, retained.ID = "ws-result-retain", "result-retain"
	if err := repositories.Results.Create(ctx, retained); err != nil {
		t.Fatal(err)
	}
	retainScope := deletioncontract.Scope{WorkspaceID: retained.WorkspaceID, DeletionID: "delete-retain", DeletionGeneration: 1, Mode: deletioncontract.ModeRetain, QuiescenceID: "quiescence-retain"}
	retainScope.RequestHash = deletioncontract.StableRequestHash(retainScope)
	sealResultWorkspaceDeletion(t, ctx, hub, resultevents.SealWorkspaceResultDeletionCommand{
		WorkspaceID: retainScope.WorkspaceID, DeletionID: retainScope.DeletionID, DeletionGeneration: 1,
		DataDispositionIntent: retainScope.Mode, QuiescenceID: retainScope.QuiescenceID, RequestHash: retainScope.RequestHash,
	})
	query := hub.Send(magicevent.NewEventWithContext(resultevents.TopicResultCommandGet, "test", resultevents.CommandDestination, magicevent.NewHeader(), ctx, resultevents.GetResultCommand{WorkspaceID: retained.WorkspaceID, ResultID: retained.ID}))
	if query == nil || !errors.Is(query.Error(), common.ErrNotFound) {
		t.Fatalf("retained public read error = %v, want not found", query)
	}
	if _, err := repositories.Results.Get(common.ContextWithRuntimeWorkspaceID(ctx, retained.WorkspaceID), retained.ID); err != nil {
		t.Fatalf("retain physically removed result: %v", err)
	}
	late := validModuleTestResult()
	late.WorkspaceID, late.ID = retained.WorkspaceID, "result-late"
	if err := repositories.Results.Create(ctx, late); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("late retained write error = %v, want not found", err)
	}

	purged := validModuleTestResult()
	purged.WorkspaceID, purged.ID = "ws-result-purge", "result-purge"
	artifact := validModuleTestArtifact(purged.ID)
	artifact.WorkspaceID = purged.WorkspaceID
	if err := repositories.Results.Create(ctx, purged); err != nil {
		t.Fatal(err)
	}
	if err := repositories.Artifacts.Create(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	purgeScope := deletioncontract.Scope{WorkspaceID: purged.WorkspaceID, DeletionID: "delete-purge", DeletionGeneration: 1, Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-purge"}
	purgeScope.RequestHash = deletioncontract.StableRequestHash(purgeScope)
	sealCommand := resultevents.SealWorkspaceResultDeletionCommand{WorkspaceID: purgeScope.WorkspaceID, DeletionID: purgeScope.DeletionID, DeletionGeneration: 1, DataDispositionIntent: purgeScope.Mode, QuiescenceID: purgeScope.QuiescenceID, RequestHash: purgeScope.RequestHash}
	sealResultWorkspaceDeletion(t, ctx, hub, sealCommand)
	purgeCommand := resultevents.PurgeWorkspaceResultsCommand{WorkspaceID: sealCommand.WorkspaceID, DeletionID: sealCommand.DeletionID, DeletionGeneration: sealCommand.DeletionGeneration, QuiescenceID: sealCommand.QuiescenceID, RequestHash: sealCommand.RequestHash}
	response := hub.Send(magicevent.NewEventWithContext(resultevents.TopicResultCommandPurgeWorkspace, "blocks/workspace", resultevents.CommandDestination, magicevent.NewHeader(), ctx, purgeCommand))
	if response == nil || response.Error() != nil {
		t.Fatalf("purge response = %v", response)
	}
	data, err := response.Get()
	if err != nil {
		t.Fatal(err)
	}
	result, ok := data.(resultevents.PurgeWorkspaceResultsResult)
	if !ok || result.ValidateFor(purgeCommand) != nil {
		t.Fatalf("purge result = %#v", data)
	}
	replay := hub.Send(magicevent.NewEventWithContext(resultevents.TopicResultCommandPurgeWorkspace, "blocks/workspace", resultevents.CommandDestination, magicevent.NewHeader(), ctx, purgeCommand))
	if replay == nil || replay.Error() != nil {
		t.Fatalf("purge replay = %v", replay)
	}
	replayData, err := replay.Get()
	if err != nil || replayData.(resultevents.PurgeWorkspaceResultsResult) != result {
		t.Fatalf("purge replay drift: first=%#v replay=%#v err=%v", result, replayData, err)
	}
	results, _ := repositories.Results.ListByWorkspace(ctx, purged.WorkspaceID)
	artifacts, _ := repositories.Artifacts.ListByWorkspace(ctx, purged.WorkspaceID)
	if len(results) != 0 || len(artifacts) != 0 {
		t.Fatalf("purge left data: results=%d artifacts=%d", len(results), len(artifacts))
	}
	if tombstone, err := repositories.WorkspaceDeletions.Get(ctx, purged.WorkspaceID); err != nil || tombstone.Phase != resultmodel.WorkspaceDeletionPhasePurged {
		t.Fatalf("purge tombstone = %#v err=%v", tombstone, err)
	}
}

func sealResultWorkspaceDeletion(t *testing.T, ctx context.Context, hub magicevent.Hub, command resultevents.SealWorkspaceResultDeletionCommand) {
	t.Helper()
	var first resultevents.SealWorkspaceResultDeletionResult
	for attempt := 0; attempt < 2; attempt++ {
		response := hub.Send(magicevent.NewEventWithContext(resultevents.TopicResultCommandSealWorkspaceDeletion, "blocks/workspace", resultevents.CommandDestination, magicevent.NewHeader(), ctx, command))
		if response == nil || response.Error() != nil {
			t.Fatalf("seal response = %v", response)
		}
		data, err := response.Get()
		if err != nil {
			t.Fatal(err)
		}
		result, ok := data.(resultevents.SealWorkspaceResultDeletionResult)
		if !ok || result.ValidateFor(command) != nil {
			t.Fatalf("seal result = %#v", data)
		}
		if attempt == 0 {
			first = result
		} else if result != first {
			t.Fatalf("seal replay drift: first=%#v replay=%#v", first, result)
		}
	}
}

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
	if err := New().Configure(resultbiz.Dependencies{
		OwnerRepositories: memory.NewResultOwnerRepositorySet().OwnerRepositories(),
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
	artifact.State = common.ArtifactStateAvailable
	artifact.ArtifactType = resultcommon.ArtifactTypeText
	artifact.URI = "workorch://workspaces/ws-result-test/artifacts/run-result-test/artifact.txt"
	if err := resultRepo.Create(ctx, result); err != nil {
		t.Fatal(err)
	}
	if err := artifactRepo.Create(ctx, artifact); err != nil {
		t.Fatal(err)
	}
	module := New()
	if err := module.Configure(resultbiz.Dependencies{
		OwnerRepositories: resultTestOwnerRepositories(resultRepo, artifactRepo, memory.NewResultWorkspaceDeletionRepository()), EventHub: hub,
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
	content := sendResultCommand[resultevents.ReadArtifactContentResult](t, ctx, hub, resultevents.TopicArtifactCommandReadContent, resultevents.ReadArtifactContentCommand{
		WorkspaceID: artifact.WorkspaceID, RunID: artifact.RunID, URI: artifact.URI,
	})
	if err := content.Validate(); err != nil || string(content.Content) != "artifact\n" {
		t.Fatalf("artifact content = %+v/%q err=%v", content, content.Content, err)
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
		OwnerRepositories: memory.NewResultOwnerRepositorySet().OwnerRepositories(), EventHub: magicevent.NewHub(16),
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
		OwnerRepositories: memory.NewResultOwnerRepositorySet().OwnerRepositories(), EventHub: magicevent.NewHub(16),
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
		OwnerRepositories: resultTestOwnerRepositories(resultRepo, memory.NewArtifactRepository(), memory.NewResultWorkspaceDeletionRepository()), EventHub: hub,
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
		OwnerRepositories: resultTestOwnerRepositories(resultRepo, memory.NewArtifactRepository(), memory.NewResultWorkspaceDeletionRepository()), EventHub: hub,
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
	if err := module.Configure(resultbiz.Dependencies{OwnerRepositories: resultTestOwnerRepositories(results, memory.NewArtifactRepository(), memory.NewResultWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
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
		OwnerRepositories: memory.NewResultOwnerRepositorySet().OwnerRepositories(), EventHub: hub,
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
		OwnerRepositories: memory.NewResultOwnerRepositorySet().OwnerRepositories(), EventHub: magicevent.NewHub(16),
	}
}

type resultTestBoundResults struct {
	repository.ResultRepository
	binding repository.OwnerRepositoryBinding
}

func (r resultTestBoundResults) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type resultTestBoundArtifacts struct {
	repository.ArtifactRepository
	binding repository.OwnerRepositoryBinding
}

func (r resultTestBoundArtifacts) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type resultTestBoundDeletions struct {
	repository.ResultWorkspaceDeletionRepository
	binding repository.OwnerRepositoryBinding
}

func (r resultTestBoundDeletions) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type resultTestBackend struct {
	binding   repository.OwnerRepositoryBinding
	results   repository.ResultRepository
	artifacts repository.ArtifactRepository
	deletions repository.ResultWorkspaceDeletionRepository
}

func (b resultTestBackend) ResultOwnerBinding() repository.OwnerRepositoryBinding { return b.binding }
func (b resultTestBackend) ResultOwnerResults() repository.ResultRepository       { return b.results }
func (b resultTestBackend) ResultOwnerArtifacts() repository.ArtifactRepository   { return b.artifacts }
func (b resultTestBackend) ResultOwnerWorkspaceDeletions() repository.ResultWorkspaceDeletionRepository {
	return b.deletions
}

func resultTestOwnerRepositories(results repository.ResultRepository, artifacts repository.ArtifactRepository, deletions repository.ResultWorkspaceDeletionRepository) repository.ResultOwnerRepositories {
	binding := repository.NewResultOwnerRepositoryBinding()
	owner, err := repository.NewResultOwnerRepositories(resultTestBackend{
		binding:   binding,
		results:   resultTestBoundResults{ResultRepository: results, binding: binding},
		artifacts: resultTestBoundArtifacts{ArtifactRepository: artifacts, binding: binding},
		deletions: resultTestBoundDeletions{ResultWorkspaceDeletionRepository: deletions, binding: binding},
	})
	if err != nil {
		panic(err)
	}
	return owner
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
