package run

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeStepQueryCommand(ctx context.Context, ev magicevent.Event) (runevents.RunCommandResult, error) {
	switch ev.ID() {
	case runevents.TopicRunCommandGetStep:
		cmd, ok := ev.Data().(runevents.GetStepCommand)
		if !ok || cmd.StepID == "" || m.steps == nil {
			return runevents.RunCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.steps.Get(ctx, cmd.StepID)
		if err != nil {
			return runevents.RunCommandResult{}, err
		}
		if cmd.RunID != "" && cmd.RunID != item.RunID {
			return runevents.RunCommandResult{}, common.ErrConflict
		}
		if err := m.validateRunReadScope(ctx, item.RunID, cmd.WorkspaceID); err != nil {
			return runevents.RunCommandResult{}, err
		}
		return runevents.RunCommandResult{Step: item}, nil
	case runevents.TopicRunCommandListStepsByRun, runevents.TopicRunCommandListStepSummariesByRun:
		return m.listStepsByRun(ctx, ev)
	default:
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
}

func (m *Biz) listStepsByRun(ctx context.Context, ev magicevent.Event) (runevents.RunCommandResult, error) {
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
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
	if err := m.validateRunReadScope(ctx, cmd.RunID, cmd.WorkspaceID); err != nil {
		return runevents.RunCommandResult{}, err
	}
	var items []*runmodel.Step
	var err error
	if ev.ID() == runevents.TopicRunCommandListStepSummariesByRun {
		items, err = m.steps.ListByRunSummary(ctx, cmd.RunID)
	} else {
		items, err = m.steps.ListByRun(ctx, cmd.RunID)
	}
	if err != nil {
		return runevents.RunCommandResult{}, err
	}
	return runevents.RunCommandResult{Steps: items}, nil
}
