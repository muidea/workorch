package workspaceviewcore

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCloneWorkspaceClonesDefaultLLMTarget(t *testing.T) {
	target := runtimecontract.ProviderModelTarget{ProviderID: "custom", ModelID: "model-a"}
	cloned := cloneWorkspace(appmodelcore.Workspace{ID: "ws-model", DefaultLLMTarget: &target})
	target.ModelID = "mutated"
	if cloned.DefaultLLMTarget == nil || cloned.DefaultLLMTarget.ProviderID != "custom" || cloned.DefaultLLMTarget.ModelID != "model-a" {
		t.Fatalf("cloned target = %+v", cloned.DefaultLLMTarget)
	}
}
