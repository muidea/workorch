package orchestrator

import (
	"context"
	"sync"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
)

type dispatchCommandRecorder struct {
	mu       sync.Mutex
	commands []orchestratorevents.DispatchStepCommand
}

type recordingDispatchMagicHub struct {
	rawHub   magicevent.Hub
	recorder *dispatchCommandRecorder
}

func newRecordingDispatchMagicHub(t *testing.T) (*recordingDispatchMagicHub, *dispatchCommandRecorder) {
	t.Helper()
	rawHub := magicevent.NewHub(32)
	recorder := &dispatchCommandRecorder{}
	hub := &recordingDispatchMagicHub{
		rawHub:   rawHub,
		recorder: recorder,
	}
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, rawHub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorDispatchStepRequested, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(orchestratorevents.DispatchStepCommand)
		if !ok {
			t.Fatalf("dispatch command type = %T, want DispatchStepCommand", ev.Data())
		}
		recorder.mu.Lock()
		recorder.commands = append(recorder.commands, cmd)
		recorder.mu.Unlock()
		result.Set(orchestratorevents.OrchestratorCommandResult{}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorDispatchStepRequested)
		rawHub.Terminate(context.Background())
	})
	return hub, recorder
}

func (h *recordingDispatchMagicHub) DispatchCommands() []orchestratorevents.DispatchStepCommand {
	if h == nil {
		return nil
	}
	return h.recorder.Commands()
}

func (r *dispatchCommandRecorder) Commands() []orchestratorevents.DispatchStepCommand {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]orchestratorevents.DispatchStepCommand, len(r.commands))
	copy(out, r.commands)
	return out
}
