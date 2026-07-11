package readmodelports

import (
	"context"
	"errors"
	"sync"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ResolutionsForTickets(ctx context.Context, reader readmodelcore.DecisionResolutionReader, items []*decisionmodel.Ticket) (map[string]*decisionmodel.Resolution, error) {
	ret := make(map[string]*decisionmodel.Resolution)
	if reader == nil || len(items) == 0 {
		return ret, nil
	}
	var mu sync.Mutex
	err := readmodelcore.RunTasks(resolvedTicketTasks(items, func(item *decisionmodel.Ticket) func() error {
		return func() error {
			resolutionCtx := common.ContextWithRuntimeWorkspaceID(ctx, item.WorkspaceID)
			resolution, err := reader.GetByTicketID(resolutionCtx, item.ID)
			if errors.Is(err, common.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			mu.Lock()
			ret[decisionResolutionMapKey(item)] = resolution
			mu.Unlock()
			return nil
		}
	})...)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func decisionResolutionMapKey(item *decisionmodel.Ticket) string {
	if item == nil {
		return ""
	}
	return string(item.WorkspaceID) + "\x00" + string(item.ID)
}

func resolvedTicketTasks(items []*decisionmodel.Ticket, build func(*decisionmodel.Ticket) func() error) []func() error {
	if len(items) == 0 || build == nil {
		return nil
	}
	ret := make([]func() error, 0, len(items))
	for _, item := range items {
		if item == nil || item.State != common.DecisionTicketStateResolved {
			continue
		}
		ret = append(ret, build(item))
	}
	return ret
}
