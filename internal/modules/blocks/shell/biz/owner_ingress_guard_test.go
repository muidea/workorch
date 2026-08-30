package shell

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type shellPanicDataEvent struct{ magicevent.Event }

func (shellPanicDataEvent) Data() any {
	panic("Shell Runtime ingress inspected payload before its call/source guard")
}

type shellCountingDataEvent struct {
	magicevent.Event
	reads atomic.Int32
}

func (e *shellCountingDataEvent) Data() any {
	e.reads.Add(1)
	return struct{}{}
}

func TestShellRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	topics := []struct {
		name    string
		topic   string
		handler func(magicevent.Event, magicevent.Result)
	}{
		{"capabilities", shellevents.TopicAcquireRuntimeCapabilities, owner.handleRuntimeCapabilities},
		{"admissibility", shellevents.TopicAcquireRuntimeOperationAdmissibility, owner.handleRuntimeOperationAdmissibility},
		{"feasibility", shellevents.TopicProbeRuntimeOperationFeasibility, owner.handleRuntimeOperationFeasibility},
		{"semantic-proof", shellevents.TopicProbeRuntimeSemanticProofFeasibility, owner.handleRuntimeSemanticProofFeasibility},
		{"prospective-satisfaction", shellevents.TopicProbeRuntimeOwnerFactProspectiveSatisfaction, owner.handleRuntimeOwnerFactProspectiveSatisfaction},
		{"cwi-reconciliation", shellevents.TopicQueryRuntimeCWIReconciliation, owner.handleRuntimeCWIReconciliationQuery},
		{"execute", shellevents.TopicExecuteOperation, owner.handleShellOperation},
	}
	for _, item := range topics {
		t.Run(item.name, func(t *testing.T) {
			wrongSource := shellPanicDataEvent{Event: magicevent.NewEventWithContext(item.topic, "blocks/file", ModuleID, magicevent.NewHeader(), ctx, nil)}
			assertShellRuntimeIngressInvalid(t, hub.Send(wrongSource), "source")

			hub.Post(shellPanicDataEvent{Event: magicevent.NewEventWithContext(item.topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, nil)})
			barrier := hub.Send(magicevent.NewEventWithContext(item.topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, struct{}{}))
			assertShellRuntimeIngressInvalid(t, barrier, "payload")

			item.handler(shellPanicDataEvent{Event: magicevent.NewEventWithContext(item.topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, nil)}, nil)
		})
	}
}

func TestShellOwnerLifecycleCoversEveryRuntimeTopic(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	topics := []string{
		shellevents.TopicAcquireRuntimeCapabilities,
		shellevents.TopicAcquireRuntimeOperationAdmissibility,
		shellevents.TopicProbeRuntimeOperationFeasibility,
		shellevents.TopicProbeRuntimeSemanticProofFeasibility,
		shellevents.TopicProbeRuntimeOwnerFactProspectiveSatisfaction,
		shellevents.TopicQueryRuntimeCWIReconciliation,
		shellevents.TopicExecuteOperation,
	}
	for _, topic := range topics {
		event := &shellCountingDataEvent{Event: magicevent.NewEventWithContext(topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, nil)}
		assertShellRuntimeIngressInvalid(t, hub.Send(event), "payload")
		if got := event.reads.Load(); got != 1 {
			t.Fatalf("started topic %s payload reads = %d, want exactly 1 after duplicate Start", topic, got)
		}
	}
	owner.Teardown(ctx)
	for _, topic := range topics {
		event := &shellCountingDataEvent{Event: magicevent.NewEventWithContext(topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, nil)}
		hub.Send(event)
		if got := event.reads.Load(); got != 0 {
			t.Fatalf("post-Stop topic %s payload reads = %d, want 0", topic, got)
		}
	}
}

func assertShellRuntimeIngressInvalid(t *testing.T, result magicevent.Result, fragment string) {
	t.Helper()
	if result == nil {
		t.Fatal("Runtime ingress returned nil Result")
	}
	err := result.Error()
	if err == nil || err.Code != cd.IllegalParam || !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Message, fragment) {
		t.Fatalf("Runtime ingress error = %#v, want typed invalid %q error", err, fragment)
	}
}
