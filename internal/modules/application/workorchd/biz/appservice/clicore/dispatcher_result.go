package clicore

import (
	"context"
	"time"

	"github.com/muidea/workorch/internal/pkg/events"
)

func (d *Dispatcher) dispatchCommand(ctx context.Context, cmd events.CLICommandPayload) events.CLIResultPayload {
	result := events.CLIResultPayload{
		CorrelationID: cmd.CorrelationID,
		CommandID:     cmd.CommandID,
		OccurredAt:    time.Now().UTC(),
		WorkspaceID:   cmd.WorkspaceID,
		RunID:         cmd.RunID,
	}
	resource, resources, err := d.handler(ctx, cmd)
	if err != nil {
		result.OK = false
		result.Error = err.Error()
		return result
	}
	result.OK = true
	result.Resource = resource
	result.Resources = resources
	return result
}
