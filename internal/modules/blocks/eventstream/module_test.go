package eventstream

import (
	"context"
	"errors"
	"testing"

	eventstreambiz "github.com/muidea/workorch/internal/modules/blocks/eventstream/biz"

	magicevent "github.com/muidea/magicCommon/event"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func sendEventStreamTestCommand[Command any](ctx context.Context, rawHub magicevent.Hub, topic string, command Command) (eventstreamevents.EventStreamCommandResult, error) {
	ev := magicevent.NewEventWithContext(topic, "eventstream/module_test", eventstreamevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := rawHub.Send(ev)
	if result == nil {
		return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return eventstreamevents.EventStreamCommandResult{}, err.Cause
		}
		return eventstreamevents.EventStreamCommandResult{}, err
	}
	data, err := result.Get()
	if err != nil {
		return eventstreamevents.EventStreamCommandResult{}, err
	}
	response, ok := data.(eventstreamevents.EventStreamCommandResult)
	if !ok {
		return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
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
		{name: "missing event records", deps: func() eventstreambiz.Dependencies { d := base; d.EventRecords = nil; return d }()},
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
	if err := module.Configure(eventstreambiz.Dependencies{
		EventRecords: memory.NewEventRecordRepository(),
		EventHub:     magicevent.NewHub(16),
	}); err != nil {
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
	return eventstreambiz.Dependencies{
		EventRecords: memory.NewEventRecordRepository(),
		EventHub:     magicevent.NewHub(16),
	}
}

func TestPurgeWorkspaceCommandAcceptsContractPayload(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	repo := memory.NewEventRecordRepository()
	workspaceID := common.WorkspaceID("ws-eventstream-contract")
	if err := repo.Append(ctx, &eventstreammodel.EventRecord{ID: "event-contract", WorkspaceID: workspaceID, Topic: "/test"}); err != nil {
		t.Fatalf("seed event record: %v", err)
	}
	module := New()
	if err := module.Configure(eventstreambiz.Dependencies{EventRecords: repo, EventHub: rawHub}); err != nil {
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

	response, err := sendEventStreamTestCommand(ctx, rawHub, eventstreamevents.TopicEventStreamCommandPurgeWorkspace, eventstreamevents.PurgeWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil {
		t.Fatalf("purge err = %v", err)
	}
	if response.DeletedCount != 1 {
		t.Fatalf("deleted count = %d, want 1", response.DeletedCount)
	}
	items, err := repo.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID})
	if err != nil {
		t.Fatalf("list event records: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("event records after purge = %d, want 0", len(items))
	}
}

func TestHighWatermarksCommandReadsPersistedWorkspaceRevisions(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	repo := memory.NewEventRecordRepository()
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
	if err := module.Configure(eventstreambiz.Dependencies{EventRecords: repo, EventHub: rawHub}); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })
	response, err := sendEventStreamTestCommand(ctx, rawHub, eventstreamevents.TopicEventStreamCommandHighWatermarks, eventstreamevents.HighWatermarksCommand{
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
