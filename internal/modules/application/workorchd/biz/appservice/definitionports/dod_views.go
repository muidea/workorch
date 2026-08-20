package definitionports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func DefineOrConsolidateWorkspaceDoDView(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, items []string) (workorchmanagementcontract.DoD, error) {
	item, err := DefineOrConsolidateWorkspaceDoD(ctx, hub, workspaceID, items)
	if err != nil {
		return workorchmanagementcontract.DoD{}, err
	}
	return definitioncore.MapDoD(item), nil
}

func UpdateDoDView(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, dodID common.DoDID, items []string) (workorchmanagementcontract.DoD, error) {
	items = definitioncore.NormalizeDoDItems(items)
	if workspaceID == "" || dodID == "" || len(items) == 0 {
		return workorchmanagementcontract.DoD{}, common.ErrInvalidArgument
	}
	item, err := UpdateDoD(ctx, hub, workspaceID, dodID, items)
	if err != nil {
		return workorchmanagementcontract.DoD{}, err
	}
	return definitioncore.MapDoD(item), nil
}

func ReadDoDView(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, dodID common.DoDID) (workorchmanagementcontract.DoD, error) {
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return workorchmanagementcontract.DoD{}, err
	}
	item, err := ReadDoD(ctx, hub, dodID)
	if err != nil {
		return workorchmanagementcontract.DoD{}, err
	}
	return definitioncore.MapDoD(item), nil
}

func ListConsolidatedWorkspaceDoDViews(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]workorchmanagementcontract.DoD, error) {
	items, err := ListConsolidatedWorkspaceDoDs(ctx, hub, workspaceID)
	if err != nil {
		return nil, err
	}
	return definitioncore.MapDoDs(items), nil
}
