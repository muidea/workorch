package run

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/run/pkg/common"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runModel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runStateNotifier interface {
	PublishRunState(context.Context, string, *runModel.Run) error
}

type eventRunStateNotifier struct {
	post func(magicevent.Event)
}

func cloneRuntimeBootstrap(value *runtimemodel.RuntimeBootstrap) *runtimemodel.RuntimeBootstrap {
	if value == nil {
		return nil
	}
	cloned := value.Clone()
	return &cloned
}

func (m *Biz) publishRunEvent(ctx context.Context, topic string, run *runModel.Run) error {
	if m == nil || m.notifier == nil || run == nil {
		return nil
	}
	return m.notifier.PublishRunState(ctx, topic, run)
}

func (n eventRunStateNotifier) PublishRunState(ctx context.Context, topic string, run *runModel.Run) error {
	if n.post == nil || run == nil {
		return nil
	}
	event := magicevent.NewEventWithContext(topic, modulecommon.ModuleID, runevents.NotificationDestination, magicevent.NewHeader(), ctx, runevents.RunStateChangedDataFromModel(run))
	event.BindLaneKey(events.EventHubLaneKeyFor(runevents.Contracts(), topic, eventcontract.LaneScope{
		SubjectType: "run",
		SubjectID:   string(run.ID),
		WorkspaceID: string(run.WorkspaceID),
		RunID:       string(run.ID),
	}))
	n.post(event)
	return nil
}
