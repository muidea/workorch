package workspaceviewcore

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCloneWorkspaceClonesDefaultLLMSelection(t *testing.T) {
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-a"}
	cloned := cloneWorkspace(appmodelcore.Workspace{ID: "ws-model", DefaultLLMSelection: &runtimecontract.ProviderInferenceSelection{Target: target}})
	target.ModelID = "mutated"
	if cloned.DefaultLLMSelection == nil || cloned.DefaultLLMSelection.Target.ProviderID != "ai-proxy" || cloned.DefaultLLMSelection.Target.ModelID != "model-a" {
		t.Fatalf("cloned target = %+v", cloned.DefaultLLMSelection)
	}
}
