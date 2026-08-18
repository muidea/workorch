package run

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeStepQueryCommand(ctx context.Context, ev magicevent.Event) (any, error) {
	switch ev.ID() {
	case runevents.TopicRunCommandGetStep:
		cmd, ok := ev.Data().(runevents.GetStepCommand)
		if !ok || cmd.StepID == "" || m.steps == nil {
			return nil, common.ErrInvalidArgument
		}
		item, err := m.steps.Get(ctx, cmd.StepID)
		if err != nil {
			return nil, err
		}
		if cmd.RunID != "" && cmd.RunID != item.RunID {
			return nil, common.ErrConflict
		}
		if err := m.validateRunReadScope(ctx, item.RunID, cmd.WorkspaceID); err != nil {
			return nil, err
		}
		return runevents.GetStepResult{Step: item}, nil
	case runevents.TopicRunCommandListStepsByRun, runevents.TopicRunCommandListStepSummariesByRun:
		return m.listStepsByRun(ctx, ev)
	default:
		return nil, common.ErrInvalidArgument
	}
}

func (m *Biz) listStepsByRun(ctx context.Context, ev magicevent.Event) (any, error) {
	var cmd runevents.ListStepsByRunCommand
	var ok bool
	if ev.ID() == runevents.TopicRunCommandListStepSummariesByRun {
		var summaryCommand runevents.ListStepSummariesByRunCommand
		summaryCommand, ok = ev.Data().(runevents.ListStepSummariesByRunCommand)
		cmd = runevents.ListStepsByRunCommand(summaryCommand)
	} else {
		cmd, ok = ev.Data().(runevents.ListStepsByRunCommand)
	}
	if !ok || cmd.RunID == "" || m.steps == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := m.validateRunReadScope(ctx, cmd.RunID, cmd.WorkspaceID); err != nil {
		return nil, err
	}
	var items []*runmodel.Step
	var err error
	if ev.ID() == runevents.TopicRunCommandListStepSummariesByRun {
		items, err = m.steps.ListByRunSummary(ctx, cmd.RunID)
	} else {
		items, err = m.steps.ListByRun(ctx, cmd.RunID)
	}
	if err != nil {
		return nil, err
	}
	if ev.ID() == runevents.TopicRunCommandListStepSummariesByRun {
		return runevents.ListStepSummariesByRunResult{Steps: items}, nil
	}
	return runevents.ListStepsByRunResult{Steps: items}, nil
}
