package workspaceports

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	soulevents "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/events"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceSoulBindingsViewUsesOwnerPorts(t *testing.T) {
	ctx := context.Background()
	testHub := newWorkspaceCommandTestHub(t)
	workspaceID := common.WorkspaceID("ws-workspaceports-soul-bindings")
	workspace := &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "workspaceports soul bindings",
		State: common.WorkspaceStateActive,
	}
	souls := map[common.SoulID]*soulmodel.Soul{}
	for _, id := range []common.SoulID{"soul-default-port", "soul-execution-port"} {
		souls[id] = &soulmodel.Soul{ID: id, Name: string(id), SoulMD: "# " + string(id)}
	}
	testHub.subscribe(workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandGet, func(any) (any, error) {
		return workspaceevents.WorkspaceCommandResult{Workspace: workspace}, nil
	})
	testHub.subscribe(workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandUpdate, func(data any) (any, error) {
		cmd := data.(workspaceevents.UpdateWorkspaceCommand)
		workspace.DefaultSoulID = cmd.DefaultSoulID
		workspace.SoulBindings = cmd.SoulBindings
		return workspaceevents.WorkspaceCommandResult{Workspace: workspace}, nil
	})
	testHub.subscribe(soulevents.CommandDestination, soulevents.TopicSoulCommandGet, func(data any) (any, error) {
		cmd := data.(soulevents.GetSoulCommand)
		item := souls[cmd.SoulID]
		if item == nil {
			return nil, common.ErrNotFound
		}
		return soulevents.SoulCommandResult{Soul: item}, nil
	})

	updated, err := UpdateWorkspaceSoulBindingsView(ctx, testHub.raw, string(workspaceID), appmodelcore.WorkspaceSoulBindings{
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
	readBack, err := ReadWorkspaceSoulBindings(ctx, testHub.raw, string(workspaceID))
	if err != nil {
		t.Fatalf("ReadWorkspaceSoulBindings: %v", err)
	}
	if readBack.DefaultSoulID != updated.DefaultSoulID || readBack.Bindings[appmodelcore.SoulBindingStageExecution] != "soul-execution-port" {
		t.Fatalf("readBack = %+v, want persisted bindings", readBack)
	}
}

func TestUpdateWorkspaceSoulBindingsViewRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	testHub := newWorkspaceCommandTestHub(t)
	workspaceID := common.WorkspaceID("ws-workspaceports-soul-bindings-invalid")
	workspace := &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "workspaceports soul bindings invalid",
		State: common.WorkspaceStateActive,
	}
	testHub.subscribe(workspaceevents.CommandDestination, workspaceevents.TopicWorkspaceCommandGet, func(any) (any, error) {
		return workspaceevents.WorkspaceCommandResult{Workspace: workspace}, nil
	})
	testHub.subscribe(soulevents.CommandDestination, soulevents.TopicSoulCommandGet, func(any) (any, error) {
		return nil, common.ErrNotFound
	})

	if _, err := UpdateWorkspaceSoulBindingsView(ctx, testHub.raw, string(workspaceID), appmodelcore.WorkspaceSoulBindings{
		Bindings: map[string]string{"not-a-stage": "soul-missing"},
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("invalid stage err = %v, want ErrInvalidArgument", err)
	}
	if _, err := UpdateWorkspaceSoulBindingsView(ctx, testHub.raw, string(workspaceID), appmodelcore.WorkspaceSoulBindings{
		DefaultSoulID: "soul-missing",
	}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("missing soul err = %v, want ErrNotFound", err)
	}
}
