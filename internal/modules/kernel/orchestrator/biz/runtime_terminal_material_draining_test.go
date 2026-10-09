package orchestrator

import (
	"errors"
	"reflect"
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceDrainingTerminalMaterialSealIsScopedIdempotentAndBudgetNeutral(t *testing.T) {
	fixture := newDrainingAdministrativeTerminationFixture(t)
	before, err := fixture.runtimeOwner.runtimeController.Load(fixture.ctx, fixture.item.WorkspaceID, fixture.item.ID)
	if err != nil { t.Fatal(err) }
	event := runtime.RuntimeInternalEvent{Kind: runtime.RuntimeInternalEventTerminalMaterialSealRequired,
		Scope: runtime.RuntimeInternalEventScope{WorkspaceID: fixture.item.WorkspaceID, RunID: fixture.item.ID}, Delivery: &runtime.RuntimeInternalDeliveryPayload{}}
	sealed, err := fixture.module.submitRuntimeInternalEvent(fixture.ctx, event)
	if err != nil || sealed.Aggregate.TerminalMaterialSeal == nil || sealed.Aggregate.StateRevision != before.StateRevision+1 || sealed.Aggregate.Facts.Recovery != before.Facts.Recovery {
		t.Fatalf("draining 终态 seal 未完成零预算独立 CAS: %v", err)
	}
	original := sealed.Aggregate.TerminalMaterialSeal.CloneRaw()
	replayed, err := fixture.module.submitRuntimeInternalEvent(fixture.ctx, event)
	if err != nil || replayed.Aggregate.StateRevision != sealed.Aggregate.StateRevision || !reflect.DeepEqual(original, *replayed.Aggregate.TerminalMaterialSeal) {
		t.Fatalf("draining seal replay 修改 generation 或材料: %v", err)
	}
	event.Delivery = &runtime.RuntimeInternalDeliveryPayload{Reason: "caller material"}
	if _, err := fixture.module.submitRuntimeInternalEvent(fixture.ctx, event); !errors.Is(err, common.ErrConflict) { t.Fatalf("draining 允许 caller 材料: %v", err) }
	event.Delivery = &runtime.RuntimeInternalDeliveryPayload{}
	event.Scope.RunID = "wrong-run"
	if _, err := fixture.module.submitRuntimeInternalEvent(fixture.ctx, event); err == nil { t.Fatal("draining 允许跨 scope seal") }
	current, err := fixture.runtimeOwner.runtimeController.Load(fixture.ctx, fixture.item.WorkspaceID, fixture.item.ID)
	if err != nil || current.StateRevision != sealed.Aggregate.StateRevision || !reflect.DeepEqual(original, *current.TerminalMaterialSeal) { t.Fatalf("拒绝路径修改原 seal: %v", err) }
}
