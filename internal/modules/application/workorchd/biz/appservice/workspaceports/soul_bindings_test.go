package workspaceports

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceSoulBindingsViewUsesOwnerPorts(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	workspaceID := common.WorkspaceID("ws-workspaceports-soul-bindings")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "workspaceports soul bindings",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	for _, id := range []common.SoulID{"soul-default-port", "soul-execution-port"} {
		if err := exports.Repositories.Souls.Create(ctx, &soulmodel.Soul{ID: id, Name: string(id), SoulMD: "# " + string(id)}); err != nil {
			t.Fatalf("seed soul %s: %v", id, err)
		}
	}

	updated, err := UpdateWorkspaceSoulBindingsView(ctx, exports.Infrastructure.EventHub, string(workspaceID), appmodelcore.WorkspaceSoulBindings{
		DefaultSoulID: " soul-default-port ",
		Bindings: map[string]string{
			appmodelcore.SoulBindingStageExecution: " soul-execution-port ",
		},
	})
	if err != nil {
		t.Fatalf("UpdateWorkspaceSoulBindingsView: %v", err)
	}
	if updated.DefaultSoulID != "soul-default-port" || updated.Bindings[appmodelcore.SoulBindingStageExecution] != "soul-execution-port" {
		t.Fatalf("updated = %+v, want normalized default/execution bindings", updated)
	}
	readBack, err := ReadWorkspaceSoulBindings(ctx, exports.Infrastructure.EventHub, string(workspaceID))
	if err != nil {
		t.Fatalf("ReadWorkspaceSoulBindings: %v", err)
	}
	if readBack.DefaultSoulID != updated.DefaultSoulID || readBack.Bindings[appmodelcore.SoulBindingStageExecution] != "soul-execution-port" {
		t.Fatalf("readBack = %+v, want persisted bindings", readBack)
	}
}

func TestUpdateWorkspaceSoulBindingsViewRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := runtime.Exports()
	workspaceID := common.WorkspaceID("ws-workspaceports-soul-bindings-invalid")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "workspaceports soul bindings invalid",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	if _, err := UpdateWorkspaceSoulBindingsView(ctx, exports.Infrastructure.EventHub, string(workspaceID), appmodelcore.WorkspaceSoulBindings{
		Bindings: map[string]string{"not-a-stage": "soul-missing"},
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("invalid stage err = %v, want ErrInvalidArgument", err)
	}
	if _, err := UpdateWorkspaceSoulBindingsView(ctx, exports.Infrastructure.EventHub, string(workspaceID), appmodelcore.WorkspaceSoulBindings{
		DefaultSoulID: "soul-missing",
	}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing soul err = %v, want ErrNotFound", err)
	}
}
