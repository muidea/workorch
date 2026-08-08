package appservice

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func UpdateWorkspaceLLMSelectionWithHub(ctx context.Context, hub magicevent.Hub, workspaceID string, selection *runtimecontract.ProviderInferenceSelection, expectedRevision uint64) (appmodelcore.Workspace, error) {
	id := common.WorkspaceID(strings.TrimSpace(workspaceID))
	if hub == nil || id == "" {
		return appmodelcore.Workspace{}, common.ErrInvalidArgument
	}
	var normalized *runtimecontract.ProviderInferenceSelection
	if selection != nil {
		value := selection.Normalized()
		if err := value.Validate(); err != nil {
			return appmodelcore.Workspace{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
		}
		validated, err := activationports.EnsureProviderInferenceSelection(ctx, hub, value)
		if err != nil {
			return appmodelcore.Workspace{}, err
		}
		normalized = &validated
	}
	item, err := workspaceports.UpdateWorkspaceLLMSelection(ctx, hub, id, normalized, expectedRevision)
	if err != nil {
		return appmodelcore.Workspace{}, err
	}
	return workspacecore.MapWorkspace(item), nil
}

func UpdateWorkspaceLLMSelectionWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string, selection *runtimecontract.ProviderInferenceSelection, expectedRevision uint64) (appmodelcore.Workspace, error) {
	if session == nil {
		return appmodelcore.Workspace{}, common.ErrInvalidArgument
	}
	hub, err := session.RequiredCommandHub()
	if err != nil {
		return appmodelcore.Workspace{}, err
	}
	return UpdateWorkspaceLLMSelectionWithHub(ctx, hub, workspaceID, selection, expectedRevision)
}
