package clicore

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
)

type CommandHandler func(context.Context, Command) (map[string]any, []map[string]any, error)

type Dispatcher struct {
	eventHub magicevent.Hub
	handler  CommandHandler
	observer magicevent.SimpleObserver
}

func NewDispatcher(eventHub magicevent.Hub, handler CommandHandler) *Dispatcher {
	return &Dispatcher{eventHub: eventHub, handler: handler}
}

func (d *Dispatcher) Run() error {
	if d == nil || d.eventHub == nil || d.handler == nil {
		return common.ErrInvalidArgument
	}
	if d.observer != nil {
		return nil
	}
	d.observer = magicevent.NewSimpleObserver(workorchdevents.CommandDestination, d.eventHub)
	d.observer.Subscribe(workorchdevents.TopicCLICommand, d.handleCommand)
	return nil
}

func (d *Dispatcher) Close() {
	if d == nil {
		return
	}
	if d.observer != nil {
		d.observer.Unsubscribe(workorchdevents.TopicCLICommand)
		d.observer = nil
	}
}

func (d *Dispatcher) handleCommand(event magicevent.Event, result magicevent.Result) {
	cmd, ok := event.Data().(workorchdevents.CLICommand)
	if !ok {
		result.Set(nil, cd.NewError(cd.IllegalParam, common.ErrInvalidArgument.Error()))
		return
	}
	result.Set(d.dispatchCommand(event.Context(), legacyCommandPayload(cmd)), nil)
}
