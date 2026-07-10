package orchestrator

import (
	"sync"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/events"
)

type dispatchCommandRecorder struct {
	mu       sync.Mutex
	commands []orchestratorevents.DispatchStepCommand
}

type recordingDispatchMagicHub struct {
	*events.MagicHub

	mu                 sync.Mutex
	dispatchAuditItems []events.Envelope
}

func newRecordingDispatchMagicHub(t *testing.T) (*recordingDispatchMagicHub, *dispatchCommandRecorder) {
	t.Helper()
	rawHub := magicevent.NewHub(32)
	hub := &recordingDispatchMagicHub{
		MagicHub: events.NewMagicHubWithHubOwner(rawHub, "kernel/orchestrator/test"),
	}
	recorder := &dispatchCommandRecorder{}
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, rawHub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorDispatchStepRequested, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(orchestratorevents.DispatchStepCommand)
		if !ok {
			t.Fatalf("dispatch command type = %T, want DispatchStepCommand", ev.Data())
		}
		recorder.mu.Lock()
		recorder.commands = append(recorder.commands, cmd)
		recorder.mu.Unlock()

		hub.recordDispatchAudit(events.Envelope{
			Topic:        orchestratorevents.TopicOrchestratorDispatchStepRequested,
			SourceModule: "kernel/orchestrator/test",
			SubjectType:  "step",
			SubjectID:    string(cmd.StepID),
			WorkspaceID:  string(cmd.WorkspaceID),
			RunID:        string(cmd.RunID),
			StepID:       string(cmd.StepID),
			Payload: map[string]any{
				events.PayloadKeyCommand:        cmd,
				events.PayloadKeyIdempotencyKey: cmd.IdempotencyKey,
			},
		})
		result.Set(orchestratorevents.OrchestratorCommandResult{}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorDispatchStepRequested)
	})
	return hub, recorder
}

func (h *recordingDispatchMagicHub) recordDispatchAudit(envelope events.Envelope) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dispatchAuditItems = append(h.dispatchAuditItems, envelope)
}

func (h *recordingDispatchMagicHub) Published() []events.Envelope {
	if h == nil {
		return nil
	}
	ret := h.MagicHub.Published()
	h.mu.Lock()
	defer h.mu.Unlock()
	ret = append(ret, h.dispatchAuditItems...)
	return ret
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
