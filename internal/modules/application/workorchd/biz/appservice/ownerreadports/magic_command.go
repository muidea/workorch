package ownerreadports

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type rawMagicHubProvider interface {
	RawHub() magicevent.Hub
}

func sendMagicCommand[Command any, Result any](ctx context.Context, hub events.Hub, topic string, destination string, subjectType string, subjectID string, command Command) (Result, error) {
	var zero Result
	rawHub, err := rawMagicHub(hub)
	if err != nil {
		return zero, err
	}
	ev := magicevent.NewEventWithContext(topic, sourceModule, destination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(events.EventHubLaneKey(events.Envelope{Topic: topic, SubjectType: subjectType, SubjectID: subjectID}))
	response := rawHub.Send(ev)
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

func rawMagicHub(hub events.Hub) (magicevent.Hub, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	provider, ok := hub.(rawMagicHubProvider)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	raw := provider.RawHub()
	if raw == nil {
		return nil, common.ErrInvalidArgument
	}
	return raw, nil
}
