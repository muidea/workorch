package run

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestPreparationHistoryRunOwnerUsesCompleteReadAndExactSourceWithoutWrites(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	repos := memory.NewRunOwnerRepositorySet()
	owner := New()
	if err := owner.Configure(Dependencies{OwnerRepositories: repos, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Stop(ctx)
	cmd := runevents.ObserveWorkspacePreparationHistoryCommand{WorkspaceID: "ws", RequestID: "observation"}
	send := func(source string, data any, requestCtx context.Context) (any, error) {
		ev := magicevent.NewEventWithContext(runevents.TopicObserveWorkspacePreparationHistory, source, runevents.CommandDestination, nil, requestCtx, data)
		return owner.executeCommand(ev)
	}
	for _, source := range []string{"", "application/workorchd ", "application/workorchd/appservice", "kernel/orchestrator", "blocks/file"} {
		if _, err := send(source, cmd, ctx); err == nil {
			t.Fatal("forged query source accepted")
		}
	}
	if _, err := send("blocks/workspace", runevents.ListRunsCommand{}, ctx); err == nil {
		t.Fatal("wrong DTO accepted")
	}
	wrong := common.ContextWithRuntimeWorkspaceID(ctx, "other")
	if _, err := send("blocks/workspace", cmd, wrong); !errors.Is(err, common.ErrConflict) {
		t.Fatal("cross-scope query accepted")
	}
	before, _ := repos.Runs().List(ctx)
	for _, source := range []string{"blocks/workspace", "application/workorchd"} {
		data, err := send(source, cmd, ctx)
		value, ok := data.(runevents.ObserveWorkspacePreparationHistoryResult)
		if err != nil || !ok || value.ValidateFor(cmd) != nil || value.History.GenerationHead != 0 {
			t.Fatalf("query: %+v %v", data, err)
		}
	}
	after, _ := repos.Runs().List(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("read created Run or changed state")
	}
	// Also exercise the registered concrete EventHub receiver rather than only
	// the owner-local router used by the negative matrix above.
	response := hub.Send(magicevent.NewEventWithContext(runevents.TopicObserveWorkspacePreparationHistory, "application/workorchd", runevents.CommandDestination, nil, ctx, cmd))
	if response == nil {
		t.Fatal("registered history receiver returned no result")
	}
	if err := response.Error(); err != nil {
		t.Fatalf("registered history receiver: %v", err)
	}
	data, resultErr := response.Get()
	value, ok := data.(runevents.ObserveWorkspacePreparationHistoryResult)
	if resultErr != nil || !ok || value.ValidateFor(cmd) != nil {
		t.Fatal("registered receiver returned wrong contract")
	}
	// Missing optional atomic reader is unknown, never an ordinary-list fallback.
	original := owner.runs
	owner.runs = &fixedRunInventoryRepository{RunRepository: original}
	if _, err := send("blocks/workspace", cmd, ctx); !errors.Is(err, common.ErrConflict) {
		t.Fatal("unsupported reader treated as empty inventory")
	}
	owner.runs = original
	owner.BeginShutdown()
	if _, err := send("blocks/workspace", cmd, ctx); err != nil {
		t.Fatalf("read-only drain: %v", err)
	}
}

func TestPreparationHistoryObservationSharesFreshIntentOrderingAndDoesNotDrive(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	repos := memory.NewRunOwnerRepositorySet()
	runs := repos.Runs()
	owner := New()
	if err := owner.Configure(Dependencies{OwnerRepositories: repos, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	defer owner.Stop(ctx)
	owner.workspaceExecutionMu.Lock()
	cmd := runevents.ObserveWorkspacePreparationHistoryCommand{WorkspaceID: "ws", RequestID: "ordered"}
	done := make(chan error, 1)
	observed := make(chan runevents.ObserveWorkspacePreparationHistoryResult, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		data, err := owner.executeCommand(magicevent.NewEventWithContext(runevents.TopicObserveWorkspacePreparationHistory, "blocks/workspace", runevents.CommandDestination, nil, ctx, cmd))
		if err == nil {
			value, ok := data.(runevents.ObserveWorkspacePreparationHistoryResult)
			if !ok || value.ValidateFor(cmd) != nil {
				err = common.ErrConflict
			} else {
				observed <- value
			}
		}
		done <- err
	}()
	<-started
	select {
	case err := <-done:
		owner.workspaceExecutionMu.Unlock()
		t.Fatalf("query bypassed fresh intent ordering: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	item := &runmodel.Run{WorkspaceID: "ws", State: common.RunStateCompleted}
	if err := runs.CreateNextWorkspaceRun(ctx, item); err != nil {
		owner.workspaceExecutionMu.Unlock()
		t.Fatal(err)
	}
	owner.workspaceExecutionMu.Unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	value := <-observed
	if value.History.RunCount != 1 || value.History.GenerationHead != 1 {
		t.Fatalf("ordered result omitted earlier accepted Run: %+v", value)
	}
	h, err := runs.(repository.RunPreparationHistoryReader).ObserveWorkspacePreparationHistory(ctx, "ws")
	if err != nil || h.RunCount != 1 || h.GenerationHead != 1 {
		t.Fatalf("complete history: %+v %v", h, err)
	}
}

func TestPreparationHistoryRetiredOwnerFactIsNotVisibilityFiltered(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	defer hub.Terminate(ctx)
	repos := memory.NewRunOwnerRepositorySet()
	owner := New()
	if err := owner.Configure(Dependencies{OwnerRepositories: repos, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Stop(ctx)
	ctx, _, _, _, fence := prepareRetiredReadFixture(t, repos, "retired-history", false)
	retireReadFixture(t, repos.Runs(), ctx, *fence)
	visible, err := owner.executeCommand(magicevent.NewEventWithContext(runevents.TopicRunCommandListByWorkspace, "blocks/workspace", runevents.CommandDestination, nil, ctx, runevents.ListRunsByWorkspaceCommand{WorkspaceID: "retired-history"}))
	if err != nil || len(visible.(runevents.ListRunsByWorkspaceResult).Runs) != 0 {
		t.Fatal("ordinary retired visibility changed")
	}
	c := runevents.ObserveWorkspacePreparationHistoryCommand{WorkspaceID: "retired-history", RequestID: "observe-retired"}
	r := hub.Send(magicevent.NewEventWithContext(runevents.TopicObserveWorkspacePreparationHistory, "application/workorchd", runevents.CommandDestination, nil, ctx, c))
	if r == nil || r.Error() != nil {
		t.Fatal("missing retired history result")
	}
	data, resultErr := r.Get()
	value, ok := data.(runevents.ObserveWorkspacePreparationHistoryResult)
	if resultErr != nil || !ok || value.ValidateFor(c) != nil || value.History.GenerationHead != 1 || value.History.RunCount != 1 || value.History.Retirement == nil || !value.History.Retirement.IsRetired() {
		t.Fatalf("retired record hidden from history: %+v %v", data, resultErr)
	}
	value.History.Retirement.DeletionID = "caller-mutated"
	fresh, err := repos.Runs().GetWorkspaceRetirement(ctx, c.WorkspaceID)
	if err != nil || fresh.DeletionID != fence.DeletionID {
		t.Fatal("query aliases retired owner state")
	}
}

var _ repository.RunPreparationHistoryReader = (*memory.RunRepository)(nil)
