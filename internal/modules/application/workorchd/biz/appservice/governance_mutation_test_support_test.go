package appservice

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type testCanonicalGovernanceRun func(context.Context, common.WorkspaceID) (*runmodel.Run, error)

// subscribeTestCanonicalGovernanceOwnerReads supplies only typed Workspace/Run
// owner read contracts to focused application tests. It never mutates either
// owner and deliberately derives the current blocker on every request.
func subscribeTestCanonicalGovernanceOwnerReads(t *testing.T, hub magicevent.Hub, readRun testCanonicalGovernanceRun) {
	t.Helper()
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(workspaceevents.GetWorkspaceCommand)
		if !ok {
			t.Fatalf("workspace admission command type = %T, want GetWorkspaceCommand", ev.Data())
		}
		run, err := readRun(ev.Context(), cmd.WorkspaceID)
		if err != nil {
			t.Fatalf("read canonical governance Run for workspace %s: %v", cmd.WorkspaceID, err)
		}
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: cmd.WorkspaceID, ActiveRunID: run.ID,
		}}, nil)
	})

	runObserver := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
	runObserver.Subscribe(runevents.TopicRunCommandGet, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(runevents.GetRunCommand)
		if !ok {
			t.Fatalf("run admission command type = %T, want GetRunCommand", ev.Data())
		}
		run, err := readRun(ev.Context(), cmd.WorkspaceID)
		if err != nil {
			t.Fatalf("read canonical governance Run %s/%s: %v", cmd.WorkspaceID, cmd.RunID, err)
		}
		if run.ID != cmd.RunID || run.WorkspaceID != cmd.WorkspaceID {
			t.Fatalf("canonical governance Run = %s/%s, want %s/%s", run.WorkspaceID, run.ID, cmd.WorkspaceID, cmd.RunID)
		}
		result.Set(runevents.GetRunResult{Run: runmodel.CloneRun(run)}, nil)
	})
}
