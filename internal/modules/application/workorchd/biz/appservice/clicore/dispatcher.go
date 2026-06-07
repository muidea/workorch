package clicore

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type CommandHandler func(context.Context, events.CLICommandPayload) (map[string]any, []map[string]any, error)

type Dispatcher struct {
	eventHub events.Hub
	handler  CommandHandler
	unsub    func()
}

func NewDispatcher(eventHub events.Hub, handler CommandHandler) *Dispatcher {
	return &Dispatcher{eventHub: eventHub, handler: handler}
}

func (d *Dispatcher) Run() error {
	if d == nil || d.eventHub == nil || d.handler == nil {
		return common.ErrInvalidArgument
	}
	if d.unsub != nil {
		return nil
	}
	d.unsub = d.eventHub.Subscribe(events.TopicCLICommand, d.handleEnvelope)
	return nil
}

func (d *Dispatcher) Close() {
	if d == nil {
		return
	}
	if d.unsub != nil {
		d.unsub()
		d.unsub = nil
	}
}

func (d *Dispatcher) handleEnvelope(ctx context.Context, envelope events.Envelope) error {
	cmd, ok := events.DecodeCLICommandPayload(envelope.Payload)
	if !ok {
		return common.ErrInvalidArgument
	}
	result := d.dispatchCommand(ctx, cmd)
	resultEnvelope := events.Envelope{
		Topic:        events.TopicCLIResult,
		SourceModule: "workorchd-cli-dispatcher",
		SubjectType:  cmd.SubjectType,
		SubjectID:    cmd.SubjectID,
		WorkspaceID:  string(cmd.WorkspaceID),
		RunID:        string(cmd.RunID),
		Payload:      events.CLIResultPayloadMap(result),
	}
	if asyncHub, ok := d.eventHub.(events.AsyncHub); ok {
		_ = asyncHub.PublishAsync(context.Background(), resultEnvelope)
	} else {
		_ = d.eventHub.Publish(context.Background(), resultEnvelope)
	}
	return events.Response(resultEnvelope)
}
