package ssh

import (
	"context"
	"errors"
	"strings"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type sshPanicDataEvent struct{ magicevent.Event }

func (sshPanicDataEvent) Data() any {
	panic("SSH Runtime ingress inspected payload before its call/source guard")
}

func TestSSHRuntimeIngressTopicMatrixEnforcesStrongOrchestratorCalls(t *testing.T) {
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
		{"capabilities", sshevents.TopicAcquireRuntimeCapabilities, owner.handleRuntimeCapabilities},
		{"admissibility", sshevents.TopicAcquireRuntimeOperationAdmissibility, owner.handleRuntimeOperationAdmissibility},
		{"feasibility", sshevents.TopicProbeRuntimeOperationFeasibility, owner.handleRuntimeOperationFeasibility},
		{"semantic-proof", sshevents.TopicProbeRuntimeSemanticProofFeasibility, owner.handleRuntimeSemanticProofFeasibility},
		{"execute", sshevents.TopicExecuteOperation, owner.handleSSHOperation},
	}
	for _, item := range topics {
		t.Run(item.name, func(t *testing.T) {
			wrongSource := sshPanicDataEvent{Event: magicevent.NewEventWithContext(item.topic, "blocks/file", ModuleID, magicevent.NewHeader(), ctx, nil)}
			assertSSHRuntimeIngressInvalid(t, hub.Send(wrongSource), "source")

			hub.Post(sshPanicDataEvent{Event: magicevent.NewEventWithContext(item.topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, nil)})
			barrier := hub.Send(magicevent.NewEventWithContext(item.topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, struct{}{}))
			assertSSHRuntimeIngressInvalid(t, barrier, "payload")

			item.handler(sshPanicDataEvent{Event: magicevent.NewEventWithContext(item.topic, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, nil)}, nil)
		})
	}
}

func assertSSHRuntimeIngressInvalid(t *testing.T, result magicevent.Result, fragment string) {
	t.Helper()
	if result == nil {
		t.Fatal("Runtime ingress returned nil Result")
	}
	err := result.Error()
	if err == nil || err.Code != cd.IllegalParam || !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Message, fragment) {
		t.Fatalf("Runtime ingress error = %#v, want typed invalid %q error", err, fragment)
	}
}
