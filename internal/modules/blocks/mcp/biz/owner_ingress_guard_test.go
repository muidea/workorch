package mcp

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type mcpPanicDataEvent struct{ magicevent.Event }

func (mcpPanicDataEvent) Data() any {
	panic("MCP Runtime ingress inspected payload before its call/source guard")
}

type mcpCountingDataEvent struct {
	magicevent.Event
	reads atomic.Int32
}

func (e *mcpCountingDataEvent) Data() any {
	e.reads.Add(1)
	return struct{}{}
}

func TestMCPRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	topics := []struct {
		name    string
		topic   string
		handler func(magicevent.Event, magicevent.Result)
	}{
		{"capabilities", mcpevents.TopicAcquireRuntimeCapabilities, owner.handleRuntimeCapabilities},
		{"admissibility", mcpevents.TopicAcquireRuntimeOperationAdmissibility, owner.handleRuntimeOperationAdmissibility},
		{"feasibility", mcpevents.TopicProbeRuntimeOperationFeasibility, owner.handleRuntimeOperationFeasibility},
		{"semantic-proof", mcpevents.TopicProbeRuntimeSemanticProofFeasibility, owner.handleRuntimeSemanticProofFeasibility},
		{"cwi-reconciliation", mcpevents.TopicQueryRuntimeCWIReconciliation, owner.handleRuntimeCWIReconciliationQuery},
		{"execute", mcpevents.TopicExecuteOperation, owner.handleMCPOperation},
	}
	for _, item := range topics {
		t.Run(item.name, func(t *testing.T) {
			wrongSource := mcpPanicDataEvent{Event: magicevent.NewEventWithContext(item.topic, "blocks/file", ModuleID, magicevent.NewHeader(), ctx, nil)}
			assertMCPRuntimeIngressInvalid(t, hub.Send(wrongSource), "source")

			hub.Post(mcpPanicDataEvent{Event: magicevent.NewEventWithContext(item.topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, nil)})
			barrier := hub.Send(magicevent.NewEventWithContext(item.topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, struct{}{}))
			assertMCPRuntimeIngressInvalid(t, barrier, "payload")

			item.handler(mcpPanicDataEvent{Event: magicevent.NewEventWithContext(item.topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, nil)}, nil)
		})
	}
}

func TestMCPOwnerLifecycleCoversEveryRuntimeTopic(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	topics := []string{
		mcpevents.TopicAcquireRuntimeCapabilities,
		mcpevents.TopicAcquireRuntimeOperationAdmissibility,
		mcpevents.TopicProbeRuntimeOperationFeasibility,
		mcpevents.TopicProbeRuntimeSemanticProofFeasibility,
		mcpevents.TopicQueryRuntimeCWIReconciliation,
		mcpevents.TopicExecuteOperation,
	}
	for _, topic := range topics {
		event := &mcpCountingDataEvent{Event: magicevent.NewEventWithContext(topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, nil)}
		assertMCPRuntimeIngressInvalid(t, hub.Send(event), "payload")
		if got := event.reads.Load(); got != 1 {
			t.Fatalf("started topic %s payload reads = %d, want exactly 1 after duplicate Start", topic, got)
		}
	}
	owner.Teardown(ctx)
	for _, topic := range topics {
		event := &mcpCountingDataEvent{Event: magicevent.NewEventWithContext(topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, nil)}
		hub.Send(event)
		if got := event.reads.Load(); got != 0 {
			t.Fatalf("post-Stop topic %s payload reads = %d, want 0", topic, got)
		}
	}
}

func assertMCPRuntimeIngressInvalid(t *testing.T, result magicevent.Result, fragment string) {
	t.Helper()
	if result == nil {
		t.Fatal("Runtime ingress returned nil Result")
	}
	err := result.Error()
	if err == nil || err.Code != cd.IllegalParam || !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Message, fragment) {
		t.Fatalf("Runtime ingress error = %#v, want typed invalid %q error", err, fragment)
	}
}
