package workspaceports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func UpdateWorkspaceSoulBindings(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, defaultSoulID common.SoulID, bindings map[string]common.SoulID) (*workspacemodel.Workspace, error) {
	item, err := ReadWorkspaceRecord(ctx, hub, workspaceID)
	if err != nil {
		return nil, err
	}
	if err := validateSoulRefs(ctx, hub, defaultSoulID, bindings); err != nil {
		return nil, err
	}
	item.DefaultSoulID = defaultSoulID
	item.SoulBindings = bindings
	if err := UpdateWorkspaceRecord(ctx, hub, item); err != nil {
		return nil, err
	}
	return item, nil
}

func ReadWorkspaceSoulBindings(ctx context.Context, hub magicevent.Hub, workspaceID string) (appmodelcore.WorkspaceSoulBindings, error) {
	item, err := ReadWorkspaceRecord(ctx, hub, common.WorkspaceID(strings.TrimSpace(workspaceID)))
	if err != nil {
		return appmodelcore.WorkspaceSoulBindings{}, err
	}
	return appmodelcore.WorkspaceSoulBindingsFromModel(item.ID, item.DefaultSoulID, item.SoulBindings), nil
}

func UpdateWorkspaceSoulBindingsView(ctx context.Context, hub magicevent.Hub, workspaceID string, cmd appmodelcore.WorkspaceSoulBindings) (appmodelcore.WorkspaceSoulBindings, error) {
	defaultSoulID := common.SoulID(strings.TrimSpace(cmd.DefaultSoulID))
	bindings, err := appmodelcore.NormalizeSoulBindings(cmd.Bindings)
	if err != nil {
		return appmodelcore.WorkspaceSoulBindings{}, err
	}
	item, err := UpdateWorkspaceSoulBindings(ctx, hub, common.WorkspaceID(strings.TrimSpace(workspaceID)), defaultSoulID, bindings)
	if err != nil {
		return appmodelcore.WorkspaceSoulBindings{}, err
	}
	return appmodelcore.WorkspaceSoulBindingsFromModel(item.ID, item.DefaultSoulID, item.SoulBindings), nil
}

func validateSoulRefs(ctx context.Context, hub magicevent.Hub, defaultSoulID common.SoulID, bindings map[string]common.SoulID) error {
	if defaultSoulID != "" {
		if _, err := ownerreadports.ReadSoulByOwnerCommand(ctx, hub, defaultSoulID); err != nil {
			return err
		}
	}
	for _, soulID := range bindings {
		if soulID == "" {
			continue
		}
		if _, err := ownerreadports.ReadSoulByOwnerCommand(ctx, hub, soulID); err != nil {
			return err
		}
	}
	return nil
}
