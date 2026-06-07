package definitionports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func DefineOrConsolidateWorkspaceDoDView(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, items []string) (appmodelcore.DoD, error) {
	eventRecords := readmodelports.EventRecordReader{Hub: hub}
	item, err := DefineOrConsolidateWorkspaceDoD(ctx, hub, eventRecords, workspaceID, items)
	if err != nil {
		return appmodelcore.DoD{}, err
	}
	return definitioncore.MapDoD(item), nil
}

func UpdateDoDView(ctx context.Context, hub events.Hub, dodID common.DoDID, items []string) (appmodelcore.DoD, error) {
	items = definitioncore.NormalizeDoDItems(items)
	if dodID == "" || len(items) == 0 {
		return appmodelcore.DoD{}, common.ErrInvalidArgument
	}
	item, err := UpdateDoD(ctx, hub, "", dodID, items)
	if err != nil {
		return appmodelcore.DoD{}, err
	}
	return definitioncore.MapDoD(item), nil
}

func ReadDoDView(ctx context.Context, hub events.Hub, dodID common.DoDID) (appmodelcore.DoD, error) {
	item, err := ReadDoD(ctx, hub, dodID)
	if err != nil {
		return appmodelcore.DoD{}, err
	}
	return definitioncore.MapDoD(item), nil
}

func ListConsolidatedWorkspaceDoDViews(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]appmodelcore.DoD, error) {
	eventRecords := readmodelports.EventRecordReader{Hub: hub}
	items, err := ListConsolidatedWorkspaceDoDs(ctx, hub, eventRecords, workspaceID)
	if err != nil {
		return nil, err
	}
	return definitioncore.MapDoDs(items), nil
}
