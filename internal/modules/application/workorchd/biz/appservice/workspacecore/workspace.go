package workspacecore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func MapWorkspace(item *workspacemodel.Workspace) appmodelcore.Workspace {
	if item == nil {
		return appmodelcore.Workspace{}
	}
	return appmodelcore.Workspace{
		ID:                   string(item.ID),
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		InternalDirectory:    item.InternalDirectory,
		State:                string(common.CanonicalWorkspaceOwnerState(item.State)),
		ActiveRunID:          string(item.ActiveRunID),
		TemplateID:           item.TemplateID,
		ProfileID:            item.ProfileID,
		DefaultSoulID:        string(item.DefaultSoulID),
		SoulBindings:         appmodelcore.MapSoulBindingIDs(item.SoulBindings),
		ContextHints:         cloneContextMap(item.ContextHints),
		EvidencePolicies:     cloneContextMapSlice(item.EvidencePolicies),
	}
}

func WorkspaceModelFromView(item appmodelcore.Workspace) *workspacemodel.Workspace {
	if item.ID == "" {
		return nil
	}
	return &workspacemodel.Workspace{
		ID:                   common.WorkspaceID(item.ID),
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		InternalDirectory:    item.InternalDirectory,
		State:                common.WorkspaceState(item.State),
		ActiveRunID:          common.RunID(item.ActiveRunID),
		TemplateID:           item.TemplateID,
		ProfileID:            item.ProfileID,
		DefaultSoulID:        common.SoulID(item.DefaultSoulID),
		SoulBindings:         SoulBindingsFromView(item.SoulBindings),
		ContextHints:         cloneContextMap(item.ContextHints),
		EvidencePolicies:     cloneContextMapSlice(item.EvidencePolicies),
	}
}
