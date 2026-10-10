package orchestrator

// Owner Biz state adapter tests.

import (
	"context"
	"errors"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRunSnapshotReaderPropagatesExplicitWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	var captured runevents.GetRunCommand
	observer := magicevent.NewSimpleObserver(runevents.CommandDestination, rawHub)
	observer.Subscribe(runevents.TopicRunCommandGet, func(ev magicevent.Event, result magicevent.Result) {
		captured, _ = ev.Data().(runevents.GetRunCommand)
		result.Set(runevents.GetRunResult{
			Run: &runmodel.Run{
				ID:          "run-001",
				WorkspaceID: "ws-run-event-scope",
			},
		}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunCommandGet) })

	item, err := (runSnapshotReader{hub: rawHub}).Read(ctx, "ws-run-event-scope", "run-001")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if item.WorkspaceID != "ws-run-event-scope" {
		t.Fatalf("run workspace = %q", item.WorkspaceID)
	}
	if captured.WorkspaceID != "ws-run-event-scope" || captured.RunID != "run-001" {
		t.Fatalf("captured scope = workspace %q run %q", captured.WorkspaceID, captured.RunID)
	}
}

func TestWorkspaceSnapshotReaderRejectsMismatchedOwnerIdentity(t *testing.T) {
	rawHub := magicevent.NewHub(8)
	observer := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, rawHub)
	observer.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(ev magicevent.Event, result magicevent.Result) {
		command, ok := ev.Data().(workspaceevents.GetWorkspaceCommand)
		if !ok {
			t.Fatalf("command type = %T", ev.Data())
		}
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: command.WorkspaceID + "-foreign"}}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	_, err := (workspaceSnapshotReader{hub: rawHub}).Read(context.Background(), "ws-owner-result-contract")
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Read error = %v, want owner identity conflict", err)
	}
}

func TestRunAndWorkspaceSnapshotReadersExposeOnlyRead(t *testing.T) {
	for _, value := range []any{runSnapshotReader{}, workspaceSnapshotReader{}} {
		typeOf := reflect.TypeOf(value)
		if typeOf.NumMethod() != 1 || typeOf.Method(0).Name != "Read" {
			t.Fatalf("%s methods = %v, want only Read", typeOf, exportedMethodNames(typeOf))
		}
	}
}

func exportedMethodNames(typeOf reflect.Type) []string {
	ret := make([]string, 0, typeOf.NumMethod())
	for index := 0; index < typeOf.NumMethod(); index++ {
		ret = append(ret, typeOf.Method(index).Name)
	}
	return ret
}
