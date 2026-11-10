package run

import (
	"context"
	"fmt"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type narrowInventoryOnlyRepository struct {
	repository.RunRepository
	items []runmodel.RunInventoryItem
	reads int
}

func (r *narrowInventoryOnlyRepository) ListInventory(context.Context, common.WorkspaceID) ([]runmodel.RunInventoryItem, error) {
	r.reads++
	return append([]runmodel.RunInventoryItem(nil), r.items...), nil
}

// All other methods deliberately use a nil embedded repository: an accidental
// full read fails, even with a terminal-only historical inventory.
func TestLifecycleQueryConsumesNarrowInventoryAndFindsLaterCurrentFact(t *testing.T) {
	ctx := context.Background()
	repo := &narrowInventoryOnlyRepository{}
	for i := range 21 {
		repo.items = append(repo.items, runmodel.RunInventoryItem{ID: common.RunID(fmt.Sprintf("old-%02d", i)), WorkspaceID: "ws", Revision: 5, WorkspaceRunGeneration: uint64(i + 1), State: common.RunStateFailed, HasRuntimeBootstrap: true, HasRuntimeState: true, RuntimeStateRevision: 4})
	}
	owner := &Biz{runs: repo}
	query := func() runevents.QueryRunLifecycleFactInventoryResult {
		ev := magicevent.NewEventWithContext(runevents.TopicRunCommandLifecycleFactInventory, "test", runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.QueryRunLifecycleFactInventoryCommand{Limit: 1})
		value, err := owner.executeRunQueryCommand(ctx, ev)
		if err != nil {
			t.Fatal(err)
		}
		return value.(runevents.QueryRunLifecycleFactInventoryResult)
	}
	for range 3 {
		if got := query(); len(got.Items) != 0 || got.NextCursor != "" {
			t.Fatal("terminal history entered current inventory", got)
		}
	}
	repo.items = append(repo.items, runmodel.RunInventoryItem{ID: "new", WorkspaceID: "ws", Revision: 2, WorkspaceRunGeneration: 22, State: common.RunStateActive, HasRuntimeBootstrap: true, HasRuntimeState: true, RuntimeStateRevision: 1})
	got := query()
	if len(got.Items) != 1 || got.Items[0].RunID != "new" || got.Items[0].RunRevision != 2 || got.Items[0].RuntimeStateRevision != 1 || repo.reads != 4 {
		t.Fatalf("missed new owner fact: %+v reads=%d", got, repo.reads)
	}
	repo.items[len(repo.items)-1].RuntimeStateRevision = 3
	got = query()
	if got.Items[0].RuntimeStateRevision != 3 {
		t.Fatal("reconciliation reused stale process inventory")
	}
}
