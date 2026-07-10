package runports

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func sendRunMagicCommand[Command any, Result any](ctx context.Context, hub magicevent.Hub, topic string, destination string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, command Command) (Result, error) {
	var zero Result
	if hub == nil || topic == "" {
		return zero, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(topic, "application/workorchd/appservice", destination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{
		SubjectType: subjectType,
		SubjectID:   subjectID,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))
	response := hub.Send(ev)
	if response == nil {
		return zero, common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return zero, err
	}
	data, err := response.Get()
	if err != nil {
		return zero, err
	}
	typed, ok := data.(Result)
	if !ok {
		return zero, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return typed, nil
}
