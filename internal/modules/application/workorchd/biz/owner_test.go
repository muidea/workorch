package biz

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDispatchRuntimeRunPublishesConcreteCommandForEachAction(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	biz := &Biz{Base: basebiz.New(ModuleID, rawHub, nil)}

	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, rawHub)
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommandStartRun)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommandResumeRun)
	})

	observer.Subscribe(orchestratorevents.TopicOrchestratorCommandStartRun, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(orchestratorevents.StartRunCommand)
		if !ok || command.RunID != "run-start" || common.RuntimeWorkspaceIDFromContext(event.Context()) != "ws-runtime" {
			t.Fatalf("start command = %#v, workspace = %q", event.Data(), common.RuntimeWorkspaceIDFromContext(event.Context()))
		}
		result.Set(orchestratorevents.OrchestratorCommandResult{}, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommandResumeRun, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(orchestratorevents.ResumeRunCommand)
		if !ok || command.WorkspaceID != "ws-runtime" || command.RunID != "run-resume" || common.RuntimeWorkspaceIDFromContext(event.Context()) != "ws-runtime" {
			t.Fatalf("resume command = %#v, workspace = %q", event.Data(), common.RuntimeWorkspaceIDFromContext(event.Context()))
		}
		result.Set(orchestratorevents.OrchestratorCommandResult{}, nil)
	})

	if err := biz.DispatchRuntimeRun(context.Background(), "ws-runtime", "run-start", "start"); err != nil {
		t.Fatalf("DispatchRuntimeRun start: %v", err)
	}
	if err := biz.DispatchRuntimeRun(context.Background(), "ws-runtime", "run-resume", "resume"); err != nil {
		t.Fatalf("DispatchRuntimeRun resume: %v", err)
	}
	if err := biz.DispatchRuntimeRun(context.Background(), "ws-runtime", "run-invalid", "invalid"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("DispatchRuntimeRun invalid action err = %v, want invalid argument", err)
	}
}
