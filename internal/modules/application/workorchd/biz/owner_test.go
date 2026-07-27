package biz

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestDispatchRuntimeRunPublishesConcreteCommandForEachAction(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	biz := &Biz{Base: basebiz.New(ModuleID, rawHub, nil)}

	observer := magicevent.NewSimpleObserver(runevents.CommandDestination, rawHub)
	t.Cleanup(func() {
		observer.Unsubscribe(runevents.TopicRunCommandActivate)
		observer.Unsubscribe(runevents.TopicRunCommandContinue)
	})

	observer.Subscribe(runevents.TopicRunCommandActivate, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(runevents.ActivateRunCommand)
		if !ok || command.WorkspaceID != "ws-runtime" || command.RunID != "run-start" || common.RuntimeWorkspaceIDFromContext(event.Context()) != "ws-runtime" {
			t.Fatalf("start command = %#v, workspace = %q", event.Data(), common.RuntimeWorkspaceIDFromContext(event.Context()))
		}
		result.Set(runevents.RunLifecycleCommandResult{}, nil)
	})
	observer.Subscribe(runevents.TopicRunCommandContinue, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(runevents.ContinueRunCommand)
		if !ok || command.WorkspaceID != "ws-runtime" || command.RunID != "run-resume" || common.RuntimeWorkspaceIDFromContext(event.Context()) != "ws-runtime" {
			t.Fatalf("resume command = %#v, workspace = %q", event.Data(), common.RuntimeWorkspaceIDFromContext(event.Context()))
		}
		result.Set(runevents.RunLifecycleCommandResult{}, nil)
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
