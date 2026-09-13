package shell

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
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type shellLifecycleHub struct {
	magicevent.Hub
	subscriptions   atomic.Int32
	unsubscriptions atomic.Int32
}

func (h *shellLifecycleHub) Subscribe(id string, observer magicevent.Observer) *frameworkdef.Error {
	if id == shellevents.TopicAcquireRuntimeCapabilities {
		h.subscriptions.Add(1)
	}
	return h.Hub.Subscribe(id, observer)
}

func (h *shellLifecycleHub) Unsubscribe(id string, observer magicevent.Observer) *frameworkdef.Error {
	if id == shellevents.TopicAcquireRuntimeCapabilities {
		h.unsubscriptions.Add(1)
	}
	return h.Hub.Unsubscribe(id, observer)
}

type shellLifecycleEvent struct {
	magicevent.Event
	dataReads atomic.Int32
}

func (e *shellLifecycleEvent) Data() any {
	e.dataReads.Add(1)
	return e.Event.Data()
}

func TestShellOwnerAcquiresManagedValidationCapabilityThroughTypedEventHubContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	request := runtimecontract.RuntimeCapabilityAcquisitionRequest{
		WorkspaceID: "ws-shell-capability", RunID: "run-shell-capability", AuthoritySnapshotID: "authority-shell-capability",
		PrerequisiteID: "prerequisite-shell-capability", OwnerModuleID: ModuleID,
		CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicAcquireRuntimeCapabilities, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, shellevents.AcquireRuntimeCapabilitiesCommand(request),
	))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(shellevents.AcquireRuntimeCapabilitiesResult)
	if !ok {
		t.Fatalf("capability response type = %T", data)
	}
	if err := response.Acquisition.ValidateForRequest(request); err != nil {
		t.Fatalf("validate Shell capability acquisition: %v", err)
	}
	if facts := response.Acquisition.Capabilities.PrerequisiteFacts; len(facts) != 1 || facts[0].Requirement.OwnerModuleID != runtimecontract.RuntimeModuleFile {
		t.Fatalf("managed-validation prerequisite facts = %+v, want one File-owner manifest fact", facts)
	}
}

func TestShellOwnerLifecycleFencesSubscriptionsAndCapabilityDelivery(t *testing.T) {
	ctx := context.Background()
	hub := &shellLifecycleHub{Hub: magicevent.NewHub(8)}
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
		t.Fatal("Shell owner did not enter started lifecycle state")
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
		WorkspaceID: "ws-shell-lifecycle", RunID: "run-shell-lifecycle", AuthoritySnapshotID: "authority-shell-lifecycle",
		PrerequisiteID: "prerequisite-shell-lifecycle", OwnerModuleID: ModuleID,
		CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	newEvent := func() *shellLifecycleEvent {
		return &shellLifecycleEvent{Event: magicevent.NewEventWithContext(
			shellevents.TopicAcquireRuntimeCapabilities, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
			magicevent.NewHeader(), ctx, shellevents.AcquireRuntimeCapabilitiesCommand(request),
		)}
	}
	delivered := newEvent()
	result := hub.Send(delivered)
	if result == nil || result.Error() != nil || delivered.dataReads.Load() != 1 {
		t.Fatalf("started delivery result=%v reads=%d, want one successful delivery", result, delivered.dataReads.Load())
	}

	owner.Teardown(ctx)
	if owner.started {
		t.Fatal("Shell owner remained started after Teardown")
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
