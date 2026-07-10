package definitionports

import (
	"context"
	"errors"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ConsolidateDoD(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, canonicalID common.DoDID, items []string, obsoleteIDs []common.DoDID) (*workspacemodel.DoD, error) {
	updated, err := UpdateDoD(ctx, hub, workspaceID, canonicalID, items)
	if err != nil {
		return nil, err
	}
	for _, dodID := range obsoleteIDs {
		if dodID == "" || dodID == canonicalID {
			continue
		}
		if err := DeleteDoD(ctx, hub, workspaceID, dodID); err != nil && !errors.Is(err, common.ErrNotFound) {
			return nil, err
		}
	}
	return updated, nil
}

func ConsolidateWorkspaceDoD(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, existing []*workspacemodel.DoD, appended []string) (*workspacemodel.DoD, error) {
	plan, err := definitioncore.WorkspaceDoDConsolidationPlan(existing, workspaceID, appended)
	if err != nil {
		return nil, err
	}
	return ConsolidateDoD(ctx, hub, workspaceID, plan.CanonicalID, plan.Items, plan.ObsoleteIDs)
}

func DefineOrConsolidateWorkspaceDoD(ctx context.Context, hub magicevent.Hub, eventRecords activationcore.EventRecordLister, workspaceID common.WorkspaceID, items []string) (*workspacemodel.DoD, error) {
	items = definitioncore.NormalizeDoDItems(items)
	if len(items) == 0 {
		return nil, common.ErrInvalidArgument
	}
	existing, err := ListDoDsByWorkspace(ctx, hub, workspaceID)
	if err != nil {
		return nil, err
	}
	existing = activationcore.OrderedWorkspaceDoDs(ctx, eventRecords, workspaceID, existing)
	if len(existing) > 0 {
		return ConsolidateWorkspaceDoD(ctx, hub, workspaceID, existing, items)
	}
	for attempt := 0; attempt < 8; attempt++ {
		id := common.DoDID(definitioncore.GenerateBusinessID("dod"))
		item, err := DefineDoD(ctx, hub, workspaceID, id, items)
		if err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return nil, err
		}
		return item, nil
	}
	return nil, common.ErrConflict
}

func ListConsolidatedWorkspaceDoDs(ctx context.Context, hub magicevent.Hub, eventRecords activationcore.EventRecordLister, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
	items, err := ListDoDsByWorkspace(ctx, hub, workspaceID)
	if err != nil {
		return nil, err
	}
	items = activationcore.OrderedWorkspaceDoDs(ctx, eventRecords, workspaceID, items)
	if len(items) <= 1 {
		return items, nil
	}
	item, err := ConsolidateWorkspaceDoD(ctx, hub, workspaceID, items, nil)
	if err != nil {
		return nil, err
	}
	return []*workspacemodel.DoD{item}, nil
}
