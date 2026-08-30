package mcp

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type mcpLifecycleHub struct {
	magicevent.Hub
	subscriptions   atomic.Int32
	unsubscriptions atomic.Int32
}

func (h *mcpLifecycleHub) Subscribe(id string, observer magicevent.Observer) {
	if id == mcpevents.TopicAcquireRuntimeCapabilities {
		h.subscriptions.Add(1)
	}
	h.Hub.Subscribe(id, observer)
}

func (h *mcpLifecycleHub) Unsubscribe(id string, observer magicevent.Observer) {
	if id == mcpevents.TopicAcquireRuntimeCapabilities {
		h.unsubscriptions.Add(1)
	}
	h.Hub.Unsubscribe(id, observer)
}

type mcpLifecycleEvent struct {
	magicevent.Event
	dataReads atomic.Int32
}

func (e *mcpLifecycleEvent) Data() any {
	e.dataReads.Add(1)
	return e.Event.Data()
}

func TestMCPOwnerAcquiresCapabilitiesThroughTypedEventHubContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	request := runtimecontract.RuntimeCapabilityAcquisitionRequest{
		WorkspaceID: "ws-mcp-capability", RunID: "run-mcp-capability", AuthoritySnapshotID: "authority-mcp-capability",
		PrerequisiteID: "prerequisite-mcp-capability", OwnerModuleID: ModuleID,
		CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredKinds: []string{modulecommon.StepKindResourceRead},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicAcquireRuntimeCapabilities, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, mcpevents.AcquireRuntimeCapabilitiesCommand(request),
	))
	data, getErr := result.Get()
	if getErr != nil {
		t.Fatal(getErr)
	}
	response, ok := data.(mcpevents.AcquireRuntimeCapabilitiesResult)
	if !ok {
		t.Fatalf("capability response type = %T", data)
	}
	if err := response.Acquisition.ValidateForRequest(request); err != nil {
		t.Fatalf("validate MCP capability acquisition: %v", err)
	}
}

func TestMCPOwnerLifecycleFencesSubscriptionsAndCapabilityDelivery(t *testing.T) {
	ctx := context.Background()
	hub := &mcpLifecycleHub{Hub: magicevent.NewHub(8)}
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	if !owner.started {
		t.Fatal("MCP owner did not enter started lifecycle state")
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
		WorkspaceID: "ws-mcp-lifecycle", RunID: "run-mcp-lifecycle", AuthoritySnapshotID: "authority-mcp-lifecycle",
		PrerequisiteID: "prerequisite-mcp-lifecycle", OwnerModuleID: ModuleID,
		CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredKinds: []string{modulecommon.StepKindResourceRead},
	}
	newEvent := func() *mcpLifecycleEvent {
		return &mcpLifecycleEvent{Event: magicevent.NewEventWithContext(
			mcpevents.TopicAcquireRuntimeCapabilities, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
			magicevent.NewHeader(), ctx, mcpevents.AcquireRuntimeCapabilitiesCommand(request),
		)}
	}
	delivered := newEvent()
	result := hub.Send(delivered)
	if result == nil || result.Error() != nil || delivered.dataReads.Load() != 1 {
		t.Fatalf("started delivery result=%v reads=%d, want one successful delivery", result, delivered.dataReads.Load())
	}

	owner.Teardown(ctx)
	if owner.started {
		t.Fatal("MCP owner remained started after Teardown")
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

	restarted, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
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
