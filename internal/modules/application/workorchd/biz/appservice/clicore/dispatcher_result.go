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
	result.Resource, result.TypedResource = splitTypedResource(resource)
	result.Resources, result.TypedResources = splitTypedResources(resources)
	return result
}

func splitTypedResource(resource map[string]any) (map[string]any, any) {
	if resource == nil {
		return nil, nil
	}
	typed := resource[events.CLIResultTypedResourceKey]
	delete(resource, events.CLIResultTypedResourceKey)
	return resource, typed
}

func splitTypedResources(resources []map[string]any) ([]map[string]any, []any) {
	if len(resources) == 0 {
		return resources, nil
	}
	typed := make([]any, 0, len(resources))
	for _, resource := range resources {
		_, item := splitTypedResource(resource)
		if item != nil {
			typed = append(typed, item)
		}
	}
	if len(typed) == 0 {
		return resources, nil
	}
	return resources, typed
}
