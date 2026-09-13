package git

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
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type gitLifecycleHub struct {
	magicevent.Hub
	subscriptions   atomic.Int32
	unsubscriptions atomic.Int32
}

func (h *gitLifecycleHub) Subscribe(id string, observer magicevent.Observer) *frameworkdef.Error {
	if id == gitevents.TopicAcquireRuntimeCapabilities {
		h.subscriptions.Add(1)
	}
	return h.Hub.Subscribe(id, observer)
}

func (h *gitLifecycleHub) Unsubscribe(id string, observer magicevent.Observer) *frameworkdef.Error {
	if id == gitevents.TopicAcquireRuntimeCapabilities {
		h.unsubscriptions.Add(1)
	}
	return h.Hub.Unsubscribe(id, observer)
}

type gitLifecycleEvent struct {
	magicevent.Event
	dataReads atomic.Int32
}

func (e *gitLifecycleEvent) Data() any {
	e.dataReads.Add(1)
	return e.Event.Data()
}

func TestGitOwnerAcquiresCapabilitiesThroughTypedEventHubContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	request := runtimecontract.RuntimeCapabilityAcquisitionRequest{
		WorkspaceID: "ws-git-capability", RunID: "run-git-capability", AuthoritySnapshotID: "authority-git-capability",
		PrerequisiteID: "prerequisite-git-capability", OwnerModuleID: ModuleID,
		CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredKinds: []string{modulecommon.StepKindRead},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		gitevents.TopicAcquireRuntimeCapabilities, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, gitevents.AcquireRuntimeCapabilitiesCommand(request),
	))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(gitevents.AcquireRuntimeCapabilitiesResult)
	if !ok {
		t.Fatalf("capability response type = %T", data)
	}
	if err := response.Acquisition.ValidateForRequest(request); err != nil {
		t.Fatalf("validate Git capability acquisition: %v", err)
	}
}

func TestGitOwnerLifecycleFencesSubscriptionsAndCapabilityDelivery(t *testing.T) {
	ctx := context.Background()
	hub := &gitLifecycleHub{Hub: magicevent.NewHub(8)}
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
		t.Fatal("Git owner did not enter started lifecycle state")
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
		WorkspaceID: "ws-git-lifecycle", RunID: "run-git-lifecycle", AuthoritySnapshotID: "authority-git-lifecycle",
		PrerequisiteID: "prerequisite-git-lifecycle", OwnerModuleID: ModuleID,
		CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredKinds: []string{modulecommon.StepKindRead},
	}
	newEvent := func() *gitLifecycleEvent {
		return &gitLifecycleEvent{Event: magicevent.NewEventWithContext(
			gitevents.TopicAcquireRuntimeCapabilities, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
			magicevent.NewHeader(), ctx, gitevents.AcquireRuntimeCapabilitiesCommand(request),
		)}
	}
	delivered := newEvent()
	result := hub.Send(delivered)
	if result == nil || result.Error() != nil || delivered.dataReads.Load() != 1 {
		t.Fatalf("started delivery result=%v reads=%d, want one successful delivery", result, delivered.dataReads.Load())
	}

	owner.Teardown(ctx)
	if owner.started {
		t.Fatal("Git owner remained started after Teardown")
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
