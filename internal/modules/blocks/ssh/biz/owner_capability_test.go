package ssh

import (
	"bytes"
	"context"
	frameworkdef "github.com/muidea/magicCommon/def"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type sshLifecycleHub struct {
	magicevent.Hub
	subscriptions   atomic.Int32
	unsubscriptions atomic.Int32
}

func (h *sshLifecycleHub) Subscribe(id string, observer magicevent.Observer) *frameworkdef.Error {
	if id == sshevents.TopicAcquireRuntimeCapabilities {
		h.subscriptions.Add(1)
	}
	return h.Hub.Subscribe(id, observer)
}

func (h *sshLifecycleHub) Unsubscribe(id string, observer magicevent.Observer) *frameworkdef.Error {
	if id == sshevents.TopicAcquireRuntimeCapabilities {
		h.unsubscriptions.Add(1)
	}
	return h.Hub.Unsubscribe(id, observer)
}

type sshLifecycleEvent struct {
	magicevent.Event
	dataReads atomic.Int32
}

func (e *sshLifecycleEvent) Data() any {
	e.dataReads.Add(1)
	return e.Event.Data()
}

func TestSSHOwnerAcquiresCapabilitiesThroughTypedEventHubContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	request := runtimecontract.RuntimeCapabilityAcquisitionRequest{
		WorkspaceID: "ws-ssh-capability", RunID: "run-ssh-capability", AuthoritySnapshotID: "authority-ssh-capability",
		PrerequisiteID: "prerequisite-ssh-capability", OwnerModuleID: ModuleID,
		CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredKinds: []string{StepKindCommand},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		sshevents.TopicAcquireRuntimeCapabilities, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, sshevents.AcquireRuntimeCapabilitiesCommand(request),
	))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(sshevents.AcquireRuntimeCapabilitiesResult)
	if !ok {
		t.Fatalf("capability response type = %T", data)
	}
	if err := response.Acquisition.ValidateForRequest(request); err != nil {
		t.Fatalf("validate SSH capability acquisition: %v", err)
	}
}

func TestSSHOwnerLifecycleFencesSubscriptionsAndCapabilityDelivery(t *testing.T) {
	ctx := context.Background()
	hub := &sshLifecycleHub{Hub: magicevent.NewHub(8)}
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	if !owner.started {
		t.Fatal("SSH owner did not enter started lifecycle state")
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	if got := hub.subscriptions.Load(); got != 1 {
		t.Fatalf("capability subscriptions after duplicate Start = %d, want 1", got)
	}
	if strings.Contains(logs.String(), "duplicate eventID") {
		t.Fatalf("duplicate Start reached SimpleObserver registration: %s", logs.String())
	}

	request := runtimecontract.RuntimeCapabilityAcquisitionRequest{
		WorkspaceID: "ws-ssh-lifecycle", RunID: "run-ssh-lifecycle", AuthoritySnapshotID: "authority-ssh-lifecycle",
		PrerequisiteID: "prerequisite-ssh-lifecycle", OwnerModuleID: ModuleID,
		CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredKinds: []string{StepKindCommand},
	}
	newEvent := func() *sshLifecycleEvent {
		return &sshLifecycleEvent{Event: magicevent.NewEventWithContext(
			sshevents.TopicAcquireRuntimeCapabilities, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
			magicevent.NewHeader(), ctx, sshevents.AcquireRuntimeCapabilitiesCommand(request),
		)}
	}
	delivered := newEvent()
	result := hub.Send(delivered)
	if result == nil || result.Error() != nil || delivered.dataReads.Load() != 1 {
		t.Fatalf("started delivery result=%v reads=%d, want one successful delivery", result, delivered.dataReads.Load())
	}

	owner.Teardown(ctx)
	if owner.started {
		t.Fatal("SSH owner remained started after Teardown")
	}
	stopped := newEvent()
	hub.Send(stopped)
	if got := stopped.dataReads.Load(); got != 0 {
		t.Fatalf("post-Stop delivery reads = %d, want 0", got)
	}
	owner.Teardown(ctx)
	if got := hub.unsubscriptions.Load(); got != 1 {
		t.Fatalf("capability unsubscriptions after duplicate Teardown = %d, want 1", got)
	}
	if strings.Contains(logs.String(), "not exist eventID") {
		t.Fatalf("duplicate Teardown reached SimpleObserver unregistration: %s", logs.String())
	}

	restarted := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := restarted.Start(); err != nil {
		t.Fatal(err)
	}
	restartedEvent := newEvent()
	result = hub.Send(restartedEvent)
	if result == nil || result.Error() != nil || restartedEvent.dataReads.Load() != 1 {
		t.Fatalf("restart delivery result=%v reads=%d, want one successful delivery", result, restartedEvent.dataReads.Load())
	}
	if got := hub.subscriptions.Load(); got != 2 {
		t.Fatalf("capability subscriptions after fresh-owner restart = %d, want 2", got)
	}
	restarted.Teardown(ctx)
}
