package web

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type webPanicDataEvent struct{ magicevent.Event }

func (webPanicDataEvent) Data() any {
	panic("Web Runtime ingress inspected payload before its call/source guard")
}

type webCountingDataEvent struct {
	magicevent.Event
	reads atomic.Int32
}

func (e *webCountingDataEvent) Data() any {
	e.reads.Add(1)
	return struct{}{}
}

func TestWebRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter("", nil)}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	topics := []struct {
		name    string
		topic   string
		handler func(magicevent.Event, magicevent.Result)
	}{
		{"capabilities", webevents.TopicAcquireRuntimeCapabilities, owner.handleRuntimeCapabilities},
		{"admissibility", webevents.TopicAcquireRuntimeOperationAdmissibility, owner.handleRuntimeOperationAdmissibility},
		{"feasibility", webevents.TopicProbeRuntimeOperationFeasibility, owner.handleRuntimeOperationFeasibility},
		{"semantic-proof", webevents.TopicProbeRuntimeSemanticProofFeasibility, owner.handleRuntimeSemanticProofFeasibility},
		{"execute", webevents.TopicExecuteOperation, owner.handleWebOperation},
	}
	for _, item := range topics {
		t.Run(item.name, func(t *testing.T) {
			wrongSource := webPanicDataEvent{Event: magicevent.NewEventWithContext(item.topic, "blocks/file", ModuleID, magicevent.NewHeader(), ctx, nil)}
			assertWebRuntimeIngressInvalid(t, hub.Send(wrongSource), "source")

			hub.Post(webPanicDataEvent{Event: magicevent.NewEventWithContext(item.topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, nil)})
			barrier := hub.Send(magicevent.NewEventWithContext(item.topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, struct{}{}))
			assertWebRuntimeIngressInvalid(t, barrier, "payload")

			item.handler(webPanicDataEvent{Event: magicevent.NewEventWithContext(item.topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, nil)}, nil)
		})
	}
}

func TestWebOwnerLifecycleCoversEveryRuntimeTopic(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter("", nil)}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	topics := []string{
		webevents.TopicAcquireRuntimeCapabilities,
		webevents.TopicAcquireRuntimeOperationAdmissibility,
		webevents.TopicProbeRuntimeOperationFeasibility,
		webevents.TopicProbeRuntimeSemanticProofFeasibility,
		webevents.TopicExecuteOperation,
	}
	for _, topic := range topics {
		event := &webCountingDataEvent{Event: magicevent.NewEventWithContext(topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, nil)}
		assertWebRuntimeIngressInvalid(t, hub.Send(event), "payload")
		if got := event.reads.Load(); got != 1 {
			t.Fatalf("started topic %s payload reads = %d, want exactly 1 after duplicate Start", topic, got)
		}
	}
	owner.Teardown(ctx)
	for _, topic := range topics {
		event := &webCountingDataEvent{Event: magicevent.NewEventWithContext(topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, nil)}
		hub.Send(event)
		if got := event.reads.Load(); got != 0 {
			t.Fatalf("post-Stop topic %s payload reads = %d, want 0", topic, got)
		}
	}
}

func assertWebRuntimeIngressInvalid(t *testing.T, result magicevent.Result, fragment string) {
	t.Helper()
	if result == nil {
		t.Fatal("Runtime ingress returned nil Result")
	}
	err := result.Error()
	if err == nil || err.Code != cd.IllegalParam || !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Message, fragment) {
		t.Fatalf("Runtime ingress error = %#v, want typed invalid %q error", err, fragment)
	}
}
