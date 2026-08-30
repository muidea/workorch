package web

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestWebOwnerAcquiresCapabilitiesThroughTypedEventHubContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter("", nil)}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	request := runtimecontract.RuntimeCapabilityAcquisitionRequest{
		WorkspaceID: "ws-web-capability", RunID: "run-web-capability", AuthoritySnapshotID: "authority-web-capability",
		PrerequisiteID: "prerequisite-web-capability", OwnerModuleID: ModuleID,
		CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredKinds: []string{modulecommon.StepKindSearch},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		webevents.TopicAcquireRuntimeCapabilities, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, webevents.AcquireRuntimeCapabilitiesCommand(request),
	))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(webevents.AcquireRuntimeCapabilitiesResult)
	if !ok {
		t.Fatalf("capability response type = %T", data)
	}
	if err := response.Acquisition.ValidateForRequest(request); err != nil {
		t.Fatalf("validate Web capability acquisition: %v", err)
	}
}
