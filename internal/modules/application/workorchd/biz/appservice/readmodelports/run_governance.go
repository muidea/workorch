package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type RunDecisions struct {
	Items              []readmodelcore.Decision
	SelectedTicket     *decisionmodel.Ticket
	SelectedResolution *decisionmodel.Resolution
}

func DecisionsForRun(ctx context.Context, tickets readmodelcore.DecisionTicketReader, resolutions readmodelcore.DecisionResolutionReader, traces readmodelcore.AutoGovernanceTraceReader, run *runmodel.Run) (RunDecisions, error) {
	if run == nil || tickets == nil {
		return RunDecisions{}, nil
	}
	items, err := tickets.ListByRunID(ctx, run.ID)
	if err != nil {
		return RunDecisions{}, err
	}
	resolved, err := ResolutionsForTickets(ctx, resolutions, items)
	if err != nil {
		return RunDecisions{}, err
	}

	ret := RunDecisions{Items: make([]readmodelcore.Decision, 0, len(items))}
	for _, item := range items {
		if item == nil || item.RunID != run.ID {
			continue
		}
		resolution := resolved[decisionResolutionMapKey(item)]
		summary, err := governancecore.LatestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
		if err != nil {
			return RunDecisions{}, err
		}
		ret.Items = append(ret.Items, readmodelprojection.MapDecision(item, resolution, summary))
		if run.BlockingTicketID != "" && item.ID == run.BlockingTicketID {
			ret.SelectedTicket = item
			ret.SelectedResolution = resolution
			continue
		}
		if ret.SelectedTicket == nil || run.BlockingTicketID == "" {
			ret.SelectedTicket = item
			ret.SelectedResolution = resolution
		}
	}
	return ret, nil
}

func DecisionsForWorkspaceItems(ctx context.Context, resolutions readmodelcore.DecisionResolutionReader, traces readmodelcore.AutoGovernanceTraceReader, workspaceID common.WorkspaceID, items []*decisionmodel.Ticket) ([]readmodelcore.Decision, error) {
	resolved, err := ResolutionsForTickets(ctx, resolutions, items)
	if err != nil {
		return nil, err
	}
	ret := make([]readmodelcore.Decision, 0, len(items))
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		summary, err := governancecore.LatestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
		if err != nil {
			return nil, err
		}
		ret = append(ret, readmodelprojection.MapDecision(item, resolved[decisionResolutionMapKey(item)], summary))
	}
	return ret, nil
}
