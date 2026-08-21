package run

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeRunQueryCommand(ctx context.Context, ev magicevent.Event) (any, error) {
	switch ev.ID() {
	case runevents.TopicRunCommandGet:
		cmd, ok := ev.Data().(runevents.GetRunCommand)
		if !ok || cmd.RunID == "" {
			return nil, common.ErrInvalidArgument
		}
		var item *runmodel.Run
		var err error
		if cmd.WorkspaceID != "" {
			item, err = m.runs.GetByWorkspaceID(ctx, cmd.WorkspaceID, cmd.RunID)
		} else {
			item, err = m.runs.Get(ctx, cmd.RunID)
		}
		if err != nil {
			return nil, err
		}
		return runevents.GetRunResult{Run: item}, nil
	case runevents.TopicRunCommandList:
		cmd, ok := ev.Data().(runevents.ListRunsCommand)
		if !ok {
			return nil, common.ErrInvalidArgument
		}
		items, err := m.runs.List(ctx)
		if err != nil {
			return nil, err
		}
		return runevents.ListRunsResult{Runs: projectRunCommandListItems(items, runStateFilter(cmd.States), cmd.Compact)}, nil
	case runevents.TopicRunCommandListByWorkspace:
		cmd, ok := ev.Data().(runevents.ListRunsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" {
			return nil, common.ErrInvalidArgument
		}
		items, err := m.runs.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		if err != nil {
			return nil, err
		}
		return runevents.ListRunsByWorkspaceResult{Runs: projectRunCommandListItems(items, runStateFilter(cmd.States), cmd.Compact)}, nil
	default:
		return nil, common.ErrInvalidArgument
	}
}

func runStateFilter(states []common.RunState) map[common.RunState]struct{} {
	if len(states) == 0 {
		return nil
	}
	ret := map[common.RunState]struct{}{}
	for _, state := range states {
		if state != "" {
			ret[state] = struct{}{}
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func projectRunCommandListItems(items []*runmodel.Run, states map[common.RunState]struct{}, compact bool) []*runmodel.Run {
	if len(items) == 0 {
		return items
	}
	ret := make([]*runmodel.Run, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if len(states) > 0 {
			if _, ok := states[item.State]; !ok {
				continue
			}
		}
		if compact {
			ret = append(ret, compactRunListItem(item))
			continue
		}
		ret = append(ret, item)
	}
	return ret
}

func compactRunListItem(item *runmodel.Run) *runmodel.Run {
	if item == nil {
		return nil
	}
	return &runmodel.Run{
		ID:                      item.ID,
		WorkspaceID:             item.WorkspaceID,
		GoalID:                  item.GoalID,
		DoDID:                   item.DoDID,
		Lineage:                 item.Lineage,
		DefinitionRevision:      item.DefinitionRevision,
		StartupInputHash:        item.StartupInputHash,
		InitialStepKind:         item.InitialStepKind,
		RuntimeActivationStatus: item.RuntimeActivationStatus.Clone(),
		State:                   item.State,
		Priority:                item.Priority,
		BlockingTicketID:        item.BlockingTicketID,
		BlockingApprovalID:      item.BlockingApprovalID,
		FailureReason:           item.FailureReason,
		CreatedAt:               item.CreatedAt,
		StartedAt:               item.StartedAt,
		EndedAt:                 item.EndedAt,
	}
}
