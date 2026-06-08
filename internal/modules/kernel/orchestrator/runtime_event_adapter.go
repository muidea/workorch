package orchestrator

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/modules/blocks/pkg/runtimeevent"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type eventRuntimeAdapter struct {
	hub events.Hub
}

func NewEventRuntimeAdapter(hub events.Hub) orchestratorbiz.RuntimeAdapter {
	return eventRuntimeAdapter{hub: hub}
}

func (a eventRuntimeAdapter) ExecuteRuntimeStep(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	if a.hub == nil || request.Kind == "" {
		return runtimemodel.StepExecutionResult{}, common.ErrInvalidArgument
	}
	err := a.hub.Publish(ctx, events.Envelope{
		Topic:        runtimeevent.RuntimeBlockExecuteTopic(request.Kind),
		SourceModule: ModuleID,
		SubjectType:  "runtime_step",
		SubjectID:    string(request.StepID),
		WorkspaceID:  string(request.WorkspaceID),
		RunID:        string(request.RunID),
		StepID:       string(request.StepID),
		Payload: map[string]any{
			events.PayloadKeyRuntimeRequest: request,
		},
	})
	if response, ok := events.ResponseEnvelope(err); ok {
		result, ok := response.Payload[events.PayloadKeyRuntimeResult].(runtimemodel.StepExecutionResult)
		if !ok {
			return runtimemodel.StepExecutionResult{}, common.ErrInvalidArgument
		}
		if message, _ := response.Payload[events.PayloadKeyError].(string); message != "" {
			return result, fmt.Errorf("%s", message)
		}
		return result, nil
	}
	if err != nil {
		return runtimemodel.StepExecutionResult{}, err
	}
	return runtimemodel.StepExecutionResult{}, fmt.Errorf("runtime block command %s returned no response", request.Kind)
}
