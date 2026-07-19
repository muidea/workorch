package appservice

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func UpdateWorkspaceLLMTargetWithHub(ctx context.Context, hub magicevent.Hub, workspaceID string, target *runtimecontract.ProviderModelTarget) (Workspace, error) {
	id := common.WorkspaceID(strings.TrimSpace(workspaceID))
	if hub == nil || id == "" {
		return Workspace{}, common.ErrInvalidArgument
	}
	var normalized *runtimecontract.ProviderModelTarget
	if target != nil {
		value := target.Normalized()
		if err := value.Validate(); err != nil {
			return Workspace{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
		}
		if err := activationports.EnsureProviderTargetReady(ctx, hub, value); err != nil {
			return Workspace{}, err
		}
		normalized = &value
	}
	item, err := workspaceports.UpdateWorkspaceLLMTarget(ctx, hub, id, normalized)
	if err != nil {
		return Workspace{}, err
	}
	return workspacecore.MapWorkspace(item), nil
}

func UpdateWorkspaceLLMTargetWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, target *runtimecontract.ProviderModelTarget) (Workspace, error) {
	if session == nil {
		return Workspace{}, common.ErrInvalidArgument
	}
	hub, err := session.RequiredCommandHub()
	if err != nil {
		return Workspace{}, err
	}
	return UpdateWorkspaceLLMTargetWithHub(ctx, hub, workspaceID, target)
}
