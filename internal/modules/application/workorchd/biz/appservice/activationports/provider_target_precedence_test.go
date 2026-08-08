package activationports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestConfiguredActivationProviderSelectionUsesWorkspaceOwnerPreference(t *testing.T) {
	workspaceDefault := runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "workspace-provider", ModelID: "workspace-model"}, ReasoningEffort: "high"}
	got, err := configuredActivationProviderSelection(context.Background(), &workspaceDefault)
	if err != nil {
		t.Fatalf("resolve workspace target: %v", err)
	}
	if !got.Same(workspaceDefault) {
		t.Fatalf("workspace target = %+v, want %+v", got, workspaceDefault)
	}
}

func TestConfiguredActivationProviderSelectionUsesLatestInjectedSystemDefault(t *testing.T) {
	ctx := configcore.ContextWithLoader(context.Background(), func() (workorchconfig.Config, error) {
		return workorchconfig.Config{LLM: workorchconfig.LLMConfig{DefaultChat: workorchconfig.ProviderModelTargetConfig{ProviderID: "latest-provider", ModelID: "latest-model"}}}, nil
	})
	got, err := configuredActivationProviderSelection(ctx, nil)
	if err != nil {
		t.Fatalf("resolve system default: %v", err)
	}
	want := runtimecontract.ProviderModelTarget{ProviderID: "latest-provider", ModelID: "latest-model"}
	if !got.Target.Same(want) || got.ReasoningEffort != "" {
		t.Fatalf("system target = %+v, want %+v", got, want)
	}
}

func TestApplyConfiguredRuntimeRepairLimitUsesInjectedLatestValue(t *testing.T) {
	ctx := configcore.ContextWithLoader(context.Background(), func() (workorchconfig.Config, error) {
		return workorchconfig.Config{Runtime: workorchconfig.RuntimeConfig{MaxRuntimeRepairAttempts: 7}}, nil
	})
	contract := map[string]any{"stateChangePolicy": "allowed_but_not_required"}
	if err := applyConfiguredRuntimeRepairLimit(ctx, contract); err != nil {
		t.Fatal(err)
	}
	if contract["maxRepairAttemptsPerFocus"] != 7 {
		t.Fatalf("maxRepairAttemptsPerFocus = %+v, want injected latest value 7", contract["maxRepairAttemptsPerFocus"])
	}

	withoutLoader := map[string]any{"stateChangePolicy": "allowed_but_not_required"}
	if err := applyConfiguredRuntimeRepairLimit(context.Background(), withoutLoader); err != nil {
		t.Fatal(err)
	}
	if _, ok := withoutLoader["maxRepairAttemptsPerFocus"]; ok {
		t.Fatalf("runtime repair limit unexpectedly read a global fallback: %+v", withoutLoader)
	}
}

func TestSealedRunProviderSelectionDoesNotDriftAfterWorkspacePreferenceChanges(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	sealed := runtimecontract.ProviderModelTarget{ProviderID: "provider-a", ModelID: "model-a"}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-sealed", State: common.WorkspaceStateActive, DefaultLLMSelection: &runtimecontract.ProviderInferenceSelection{Target: sealed}}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	bootstrap := runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: sealed}}}
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-sealed", WorkspaceID: "ws-sealed", State: common.RunStateActive, RuntimeBootstrap: &bootstrap}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	workspacePreference := runtimecontract.ProviderModelTarget{ProviderID: "provider-b", ModelID: "model-b"}
	workspace, err := workspaces.Get(ctx, "ws-sealed")
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspace.DefaultLLMSelection = &runtimecontract.ProviderInferenceSelection{Target: workspacePreference, ReasoningEffort: "high"}
	if err := workspaces.Update(ctx, workspace); err != nil {
		t.Fatalf("update workspace preference: %v", err)
	}
	stored, err := runs.GetByWorkspaceID(ctx, "ws-sealed", "run-sealed")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if stored.RuntimeBootstrap == nil || !stored.RuntimeBootstrap.Activation.ProviderSelection.Target.Same(sealed) {
		t.Fatalf("sealed run target = %+v, want %+v", stored.RuntimeBootstrap, sealed)
	}
}
