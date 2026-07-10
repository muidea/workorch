package orchestrator

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRunEventRepositoryGetPropagatesRuntimeWorkspaceScope(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-run-event-scope")
	rawHub := magicevent.NewHub(16)
	var captured runevents.GetRunCommand
	observer := magicevent.NewSimpleObserver(runevents.CommandDestination, rawHub)
	observer.Subscribe(runevents.TopicRunCommandGet, func(ev magicevent.Event, result magicevent.Result) {
		captured, _ = ev.Data().(runevents.GetRunCommand)
		result.Set(runevents.RunCommandResult{
			Run: &runmodel.Run{
				ID:          "run-001",
				WorkspaceID: "ws-run-event-scope",
			},
		}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunCommandGet) })

	item, err := (runEventRepository{hub: rawHub}).Get(ctx, "run-001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if item.WorkspaceID != "ws-run-event-scope" {
		t.Fatalf("run workspace = %q", item.WorkspaceID)
	}
	if captured.WorkspaceID != "ws-run-event-scope" || captured.RunID != "run-001" {
		t.Fatalf("captured scope = workspace %q run %q", captured.WorkspaceID, captured.RunID)
	}
}
