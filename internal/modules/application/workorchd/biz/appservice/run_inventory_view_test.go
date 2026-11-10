package appservice

import (
	"context"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type inventoryViewOnlyReader struct {
	readmodelcore.RunReader // Full-list and aggregate reads deliberately unavailable.
	inventory               []runmodel.RunInventoryItem
	scope                   common.WorkspaceID
	calls                   int
}

func (r *inventoryViewOnlyReader) ListCompact(context.Context) ([]runmodel.RunInventoryItem, error) {
	r.calls++
	return runmodel.CloneRunInventory(r.inventory), nil
}
func (r *inventoryViewOnlyReader) ListCompactByWorkspaceID(_ context.Context, id common.WorkspaceID) ([]runmodel.RunInventoryItem, error) {
	r.calls++
	r.scope = id
	return runmodel.CloneRunInventory(r.inventory), nil
}

func TestRunListConsumesInventoryWithoutLosingSealedSelection(t *testing.T) {
	selection := rc.ProviderInferenceSelection{Target: rc.ProviderModelTarget{ProviderID: "provider", ModelID: "model"}, ReasoningEffort: "high"}
	original := &runmodel.Run{ID: "terminal", WorkspaceID: "ws", GoalID: "goal", DoDID: "dod", State: common.RunStateFailed, TerminalResultID: "result", WorkspaceRunGeneration: 21, RuntimeBootstrap: &rc.RuntimeBootstrap{Activation: rc.RuntimeActivationContext{ProviderSelection: selection}}}
	// Compare the entire existing list view contract, not just the new field.
	expected := runtimecore.MapRun(original)
	for _, scope := range []common.WorkspaceID{"", "ws"} {
		reader := &inventoryViewOnlyReader{inventory: []runmodel.RunInventoryItem{runmodel.InventoryFromRun(original)}}
		got, err := listRunViews(context.Background(), readModelDependencies{runs: reader}, scope)
		if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], expected) || reader.calls != 1 || reader.scope != scope {
			t.Fatalf("narrow list lost fields or scope: %+v %v", got, err)
		}
		got[0].SealedLLMSelection.Target.ModelID = "consumer edit"
		if original.RuntimeBootstrap.Activation.ProviderSelection != selection {
			t.Fatal("list aliased owner selection")
		}
	}
}
