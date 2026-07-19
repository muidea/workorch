package activationports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestResolveActivationProviderTargetPrecedence(t *testing.T) {
	explicit := runtimecontract.ProviderModelTarget{ProviderID: "explicit-provider", ModelID: "explicit-model"}
	workspaceDefault := runtimecontract.ProviderModelTarget{ProviderID: "workspace-provider", ModelID: "workspace-model"}

	got, err := resolveActivationProviderTarget(context.Background(), nil, &explicit, &workspaceDefault)
	if err != nil {
		t.Fatalf("resolve explicit target: %v", err)
	}
	if !got.Same(explicit) {
		t.Fatalf("explicit target = %+v, want %+v", got, explicit)
	}

	got, err = resolveActivationProviderTarget(context.Background(), nil, nil, &workspaceDefault)
	if err != nil {
		t.Fatalf("resolve workspace target: %v", err)
	}
	if !got.Same(workspaceDefault) {
		t.Fatalf("workspace target = %+v, want %+v", got, workspaceDefault)
	}
}

func TestResolveActivationProviderTargetUsesLatestInjectedSystemDefault(t *testing.T) {
	ctx := configcore.ContextWithLoader(context.Background(), func() (workorchconfig.Config, error) {
		return workorchconfig.Config{LLM: workorchconfig.LLMConfig{DefaultChat: workorchconfig.ProviderModelTargetConfig{ProviderID: "latest-provider", ModelID: "latest-model"}}}, nil
	})
	got, err := resolveActivationProviderTarget(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("resolve system default: %v", err)
	}
	want := runtimecontract.ProviderModelTarget{ProviderID: "latest-provider", ModelID: "latest-model"}
	if !got.Same(want) {
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

func TestSealedRunProviderTargetDoesNotDriftAfterWorkspacePreferenceChanges(t *testing.T) {
	ctx := context.Background()
	runs := memory.NewRunRepository()
	workspaces := memory.NewWorkspaceRepository()
	sealed := runtimecontract.ProviderModelTarget{ProviderID: "provider-a", ModelID: "model-a"}
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-sealed", State: common.WorkspaceStateActive, DefaultLLMTarget: &sealed}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	bootstrap := runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{ProviderTarget: sealed}}
	if err := runs.Create(ctx, &runmodel.Run{ID: "run-sealed", WorkspaceID: "ws-sealed", State: common.RunStateActive, RuntimeBootstrap: &bootstrap}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	workspacePreference := runtimecontract.ProviderModelTarget{ProviderID: "provider-b", ModelID: "model-b"}
	workspace, err := workspaces.Get(ctx, "ws-sealed")
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspace.DefaultLLMTarget = &workspacePreference
	if err := workspaces.Update(ctx, workspace); err != nil {
		t.Fatalf("update workspace preference: %v", err)
	}
	stored, err := runs.GetByWorkspaceID(ctx, "ws-sealed", "run-sealed")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if stored.RuntimeBootstrap == nil || !stored.RuntimeBootstrap.Activation.ProviderTarget.Same(sealed) {
		t.Fatalf("sealed run target = %+v, want %+v", stored.RuntimeBootstrap, sealed)
	}
}

func TestActiveRunProviderTargetStaleUsesDesiredActivationTarget(t *testing.T) {
	sealed := runtimecontract.ProviderModelTarget{ProviderID: "provider-a", ModelID: "model-a"}
	run := &runmodel.Run{
		ID: "run-provider-target-stale",
		RuntimeBootstrap: &runtimecontract.RuntimeBootstrap{
			Activation: runtimecontract.RuntimeActivationContext{ProviderTarget: sealed},
		},
	}

	stale, err := ActiveRunProviderTargetStale(context.Background(), run, nil, &sealed)
	if err != nil {
		t.Fatal(err)
	}
	if stale {
		t.Fatal("unchanged workspace provider target marked stale")
	}

	changed := runtimecontract.ProviderModelTarget{ProviderID: "provider-b", ModelID: "model-b"}
	stale, err = ActiveRunProviderTargetStale(context.Background(), run, nil, &changed)
	if err != nil {
		t.Fatal(err)
	}
	if !stale {
		t.Fatal("changed workspace provider target did not require a new activation")
	}

	explicit := runtimecontract.ProviderModelTarget{ProviderID: "provider-c", ModelID: "model-c"}
	stale, err = ActiveRunProviderTargetStale(context.Background(), run, &explicit, &sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !stale {
		t.Fatal("explicit provider target did not take precedence for stale detection")
	}
}
