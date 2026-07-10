package governance

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func sendOwnerMagicCommand[Command any, Result any](ctx context.Context, hub magicevent.Hub, topic string, destination string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, command Command) (Result, error) {
	var zero Result
	if hub == nil || topic == "" || destination == "" {
		return zero, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(topic, ModuleID, destination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{
		SubjectType: subjectType,
		SubjectID:   subjectID,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
		StepID:      string(stepID),
	}))
	response := hub.Send(ev)
	if response == nil {
		return zero, common.ErrInvalidArgument
	}
	if responseErr := response.Error(); responseErr != nil {
		return zero, responseErr
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return zero, resultErr
	}
	typed, ok := data.(Result)
	if !ok {
		return zero, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return typed, nil
}
