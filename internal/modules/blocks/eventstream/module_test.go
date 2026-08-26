package eventstream

import (
	"context"
	"errors"
	"testing"
	"time"

	eventstreambiz "github.com/muidea/workorch/internal/modules/blocks/eventstream/biz"

	magicevent "github.com/muidea/magicCommon/event"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func sendEventStreamTestCommand[Command any, Response any](ctx context.Context, rawHub magicevent.Hub, topic string, command Command) (Response, error) {
	return sendEventStreamTestCommandFrom[Command, Response](ctx, rawHub, topic, "application/workorchd/appservice", command)
}

func sendEventStreamTestCommandFrom[Command any, Response any](ctx context.Context, rawHub magicevent.Hub, topic, source string, command Command) (Response, error) {
	var zero Response
	ev := magicevent.NewEventWithContext(topic, source, eventstreamevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := rawHub.Send(ev)
	if result == nil {
		return zero, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return zero, err.Cause
		}
		return zero, err
	}
	data, err := result.Get()
	if err != nil {
		return zero, err
	}
	response, ok := data.(Response)
	if !ok {
		return zero, common.ErrInvalidArgument
	}
	return response, nil
}

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(eventstreamModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil {
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

func TestModuleConfigureRequiresCoreDependencies(t *testing.T) {
	base := eventstreamModuleContractDependencies()
	tests := []struct {
		name string
		deps eventstreambiz.Dependencies
	}{
		{name: "missing owner bundle", deps: eventstreambiz.Dependencies{EventHub: base.EventHub}},
		{name: "missing event hub", deps: func() eventstreambiz.Dependencies { d := base; d.EventHub = nil; return d }()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}

func TestModuleRunIsIdempotentAndTeardownUnsubscribes(t *testing.T) {
	ctx := context.Background()
	module := New()
	owner := memory.NewEventStreamOwnerRepositorySet()
	if err := module.Configure(eventstreambiz.Dependencies{OwnerRepositories: eventStreamTestOwnerRepositories(
		owner.EventRecords, owner.WorkspaceDeletions, owner.EventRecords), EventHub: magicevent.NewHub(16)}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("module subscriptions were not cleared: %+v", module)
	}
}

func eventstreamModuleContractDependencies() eventstreambiz.Dependencies {
	owner := memory.NewEventStreamOwnerRepositorySet()
	return eventstreambiz.Dependencies{OwnerRepositories: eventStreamTestOwnerRepositories(
		owner.EventRecords, owner.WorkspaceDeletions, owner.EventRecords), EventHub: magicevent.NewHub(16)}

}

func TestPurgeWorkspaceCommandAcceptsContractPayload(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	owner := memory.NewEventStreamOwnerRepositorySet()
	repo := owner.EventRecords
	workspaceID := common.WorkspaceID("ws-eventstream-contract")
	if err := repo.Append(ctx, &eventstreammodel.EventRecord{ID: "event-contract", WorkspaceID: workspaceID, Topic: "/test"}); err != nil {
		t.Fatalf("seed event record: %v", err)
	}
	module := New()
	if err := module.Configure(eventstreambiz.Dependencies{OwnerRepositories: eventStreamTestOwnerRepositories(repo, owner.WorkspaceDeletions, repo), EventHub: rawHub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(context.Background()); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	scope := deletioncontract.Scope{WorkspaceID: workspaceID, DeletionID: "deletion-eventstream-contract", DeletionGeneration: 1, Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-eventstream-contract"}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	sealCommand := eventstreamevents.SealWorkspaceEventStreamDeletionCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
	seal, err := sendEventStreamTestCommandFrom[eventstreamevents.SealWorkspaceEventStreamDeletionCommand, eventstreamevents.SealWorkspaceEventStreamDeletionResult](ctx, rawHub, eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion, "blocks/workspace", sealCommand)
	if err != nil || seal.ValidateFor(sealCommand) != nil {
		t.Fatalf("seal = %+v err=%v", seal, err)
	}
	purgeCommand := eventstreamevents.PurgeWorkspaceCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
	response, err := sendEventStreamTestCommandFrom[eventstreamevents.PurgeWorkspaceCommand, eventstreamevents.PurgeWorkspaceResult](ctx, rawHub, eventstreamevents.TopicEventStreamCommandPurgeWorkspace, "blocks/workspace", purgeCommand)
	if err != nil {
		t.Fatalf("purge err = %v", err)
	}
	if err := response.ValidateFor(purgeCommand); err != nil {
		t.Fatalf("purge response = %+v: %v", response, err)
	}
	items, err := repo.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: workspaceID})
	if err != nil {
		t.Fatalf("list event records: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("event records after purge = %d, want 0", len(items))
	}
}

func TestWorkspaceDeletionSealPostedWithoutResultDoesNotFence(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	owner := memory.NewEventStreamOwnerRepositorySet()
	repo := owner.EventRecords
	workspaceID := common.WorkspaceID("ws-eventstream-post-purge")
	if err := repo.Append(ctx, &eventstreammodel.EventRecord{ID: "event-post-purge", WorkspaceID: workspaceID, Topic: "/test"}); err != nil {
		t.Fatal(err)
	}
	module := New()
	if err := module.Configure(eventstreambiz.Dependencies{OwnerRepositories: eventStreamTestOwnerRepositories(repo, owner.WorkspaceDeletions, repo), EventHub: rawHub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	scope := deletioncontract.Scope{WorkspaceID: workspaceID, DeletionID: "deletion-eventstream-post", DeletionGeneration: 1, Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-eventstream-post"}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	command := eventstreamevents.SealWorkspaceEventStreamDeletionCommand{WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration, DataDispositionIntent: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash}
	forged := rawHub.Send(magicevent.NewEventWithContext(
		eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion, "untrusted/module",
		eventstreamevents.CommandDestination, magicevent.NewHeader(), ctx,
		command,
	))
	if forged == nil || forged.Error() == nil || !errors.Is(forged.Error().Cause, common.ErrInvalidArgument) {
		t.Fatalf("forged deletion seal result = %+v, want invalid argument", forged)
	}

	rawHub.Post(magicevent.NewEventWithContext(
		eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion, "blocks/workspace",
		eventstreamevents.CommandDestination, magicevent.NewHeader(), ctx,
		command,
	))
	time.Sleep(20 * time.Millisecond)
	if _, err := repo.Get(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), "event-post-purge"); err != nil {
		t.Fatalf("posted deletion seal mutated owner state: %v", err)
	}
}

func TestListEventsCommandUsesConcreteBoundedResult(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	owner := memory.NewEventStreamOwnerRepositorySet()
	repo := owner.EventRecords
	workspaceID := common.WorkspaceID("ws-eventstream-list")
	for _, item := range []*eventstreammodel.EventRecord{
		{ID: "event-list-1", WorkspaceID: workspaceID, Topic: "/test", Revision: 1},
		{ID: "event-list-2", WorkspaceID: workspaceID, Topic: "/test", Revision: 2},
	} {
		if err := repo.Append(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	module := New()
	if err := module.Configure(eventstreambiz.Dependencies{OwnerRepositories: eventStreamTestOwnerRepositories(repo, owner.WorkspaceDeletions, repo), EventHub: rawHub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	response, err := sendEventStreamTestCommand[eventstreamevents.ListEventsCommand, eventstreamevents.ListEventsResult](ctx, rawHub, eventstreamevents.TopicEventStreamCommandList, eventstreamevents.ListEventsCommand{Query: eventstreamevents.EventQuery{WorkspaceID: workspaceID, Limit: 1}})
	if err != nil || len(response.Events) != 1 || response.Events[0].ID != "event-list-2" {
		t.Fatalf("list response = %+v err=%v", response, err)
	}
	if _, err := sendEventStreamTestCommand[eventstreamevents.ListEventsCommand, eventstreamevents.ListEventsResult](ctx, rawHub, eventstreamevents.TopicEventStreamCommandList, eventstreamevents.ListEventsCommand{Query: eventstreamevents.EventQuery{WorkspaceID: workspaceID, Limit: eventstreamevents.MaxListLimit + 1}}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("oversized list err = %v, want invalid argument", err)
	}
}

func TestHighWatermarksCommandReadsPersistedWorkspaceRevisions(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	owner := memory.NewEventStreamOwnerRepositorySet()
	repo := owner.EventRecords
	for _, item := range []*eventstreammodel.EventRecord{
		{ID: "event-a-1", WorkspaceID: "ws-a", Topic: "/test", Revision: 1},
		{ID: "event-a-7", WorkspaceID: "ws-a", Topic: "/test", Revision: 7},
		{ID: "event-b-3", WorkspaceID: "ws-b", Topic: "/test", Revision: 3},
	} {
		if err := repo.Append(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	module := New()
	if err := module.Configure(eventstreambiz.Dependencies{OwnerRepositories: eventStreamTestOwnerRepositories(repo, owner.WorkspaceDeletions, repo), EventHub: rawHub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	response, err := sendEventStreamTestCommand[eventstreamevents.HighWatermarksCommand, eventstreamevents.HighWatermarksResult](ctx, rawHub, eventstreamevents.TopicEventStreamCommandHighWatermarks, eventstreamevents.HighWatermarksCommand{
		WorkspaceIDs: []common.WorkspaceID{"ws-b", "ws-a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.HighWatermarks) != 2 || response.HighWatermarks[0].WorkspaceID != "ws-a" || response.HighWatermarks[0].Revision != 7 ||
		response.HighWatermarks[1].WorkspaceID != "ws-b" || response.HighWatermarks[1].Revision != 3 {
		t.Fatalf("high watermarks = %+v", response.HighWatermarks)
	}
}
