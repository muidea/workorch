package run

import (
	"context"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeQueryTransfersDetachedRepositoryImage(t *testing.T) {
	for _, storage := range []string{"memory", "file"} {
		t.Run(storage, func(t *testing.T) {
			var repos repository.RunOwnerRepositories = memoryrepo.NewRunOwnerRepositorySet()
			if storage == "file" {
				repos = workorchrepo.NewWorkspaceScopedFileRepositorySet(t.TempDir()).RunOwner
			}
			workspaceID := common.WorkspaceID("query-isolation-" + storage)
			ctx, runID, _, _, _ := prepareRetiredReadFixture(t, repos, workspaceID, storage == "file")
			run, err := repos.Runs().GetByWorkspaceID(ctx, workspaceID, runID)
			if err != nil {
				t.Fatal(err)
			}
			old := run.RuntimeState.StateRevision
			run.RuntimeState.StateRevision++
			run.RuntimeState.ProcessedEventIDs = []string{"original"}
			run.RuntimeLifecycleFence.RuntimeStateRevision = run.RuntimeState.StateRevision
			run.RuntimeLifecycleFence.RuntimeStateRef = runevents.RuntimeStateRef(workspaceID, runID, run.RuntimeState.StateRevision)
			run.RuntimeLifecycleFence.RuntimeStateHash = rc.StableRuntimeStateRecordHash(*run.RuntimeState)
			if err := repos.Runs().CompareAndSwapRuntimeState(ctx, run, run.Revision, old); err != nil {
				t.Fatal(err)
			}
			want := run.RuntimeState.Clone()
			hub := magicevent.NewHub(32)
			t.Cleanup(func() { hub.Terminate(context.Background()) })
			biz := New()
			if err := biz.Configure(Dependencies{OwnerRepositories: repos, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := biz.PrepareRuntimeRecoveryInventory(); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				ev := magicevent.NewEventWithContext(runevents.TopicRunCommandRuntimeStateRecord, workspaceRetirementFactQuerySource, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.QueryRuntimeStateRecordCommand{WorkspaceID: workspaceID, RunID: runID})
				result := hub.Send(ev)
				data, err := result.Get()
				if err != nil {
					t.Fatal(err)
				}
				response := data.(runevents.QueryRuntimeStateRecordResult)
				if !reflect.DeepEqual(want, *response.RuntimeState) {
					t.Fatal("response mutation reached stored image")
				}
				response.RuntimeState.ProcessedEventIDs[0] = "caller mutation"
				response.RuntimeState.StateRevision++
			}
			stored, err := repos.Runs().GetByWorkspaceID(ctx, workspaceID, runID)
			if err != nil || !reflect.DeepEqual(want, *stored.RuntimeState) {
				t.Fatal("response altered durable owner state", err)
			}
		})
	}
}
