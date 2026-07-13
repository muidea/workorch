package web

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWebModuleIdentityConfigureStartStopContract(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	defer rawHub.Terminate(ctx)

	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(Dependencies{EventHub: rawHub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.runtimeAdapter == nil || module.eventHub == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstObserver := module.capabilityObserver
	if firstObserver == nil {
		t.Fatal("capability observer is nil after Start")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if module.capabilityObserver != firstObserver {
		t.Fatal("second Start replaced capability observer")
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.runtimeAdapter != nil || module.eventHub != nil || module.capabilityObserver != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestWebModuleStartRequiresRuntimeDependencies(t *testing.T) {
	module := New()
	if err := module.Configure(Dependencies{}); err != nil {
		t.Fatalf("Configure with empty dependencies: %v", err)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start without EventHub err = %v, want invalid argument", err)
	}
}

func TestWebApprovalGetterPublishesScopedOwnerCommand(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(8)
	approvalID := common.ApprovalID("approval-web-scope")
	runID := common.RunID("run-web-scope")
	var captured approvalevents.GetApprovalCommand
	var source string
	observer := magicevent.NewSimpleObserver(approvalevents.CommandDestination, rawHub)
	observer.Subscribe(approvalevents.TopicApprovalCommandGet, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(approvalevents.GetApprovalCommand)
		if !ok {
			t.Fatalf("command type = %T, want GetApprovalCommand", ev.Data())
		}
		captured = cmd
		source = ev.Source()
		result.Set(approvalevents.ApprovalCommandResult{Approval: &approvalmodel.Approval{
			ID:    approvalID,
			RunID: runID,
			State: common.ApprovalStateApproved,
		}}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(approvalevents.TopicApprovalCommandGet) })

	item, err := (webApprovalGetter{hub: rawHub}).GetForRun(ctx, approvalID, runID)
	if err != nil {
		t.Fatalf("GetForRun() error = %v", err)
	}
	if item == nil || item.ID != approvalID {
		t.Fatalf("approval = %+v, want %s", item, approvalID)
	}
	if source != ModuleID {
		t.Fatalf("source = %q, want %q", source, ModuleID)
	}
	if captured.ApprovalID != approvalID || captured.RunID != runID {
		t.Fatalf("command = %+v, want approval/run scope", captured)
	}
}
