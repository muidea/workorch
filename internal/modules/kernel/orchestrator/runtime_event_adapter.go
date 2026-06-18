package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/muidea/workorch/internal/modules/blocks/pkg/runtimeevent"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type eventRuntimeAdapter struct {
	hub events.Hub
}

type runtimeEventResult struct {
	result runtimemodel.StepExecutionResult
	err    error
}

func NewEventRuntimeAdapter(hub events.Hub) orchestratorbiz.RuntimeAdapter {
	return eventRuntimeAdapter{hub: hub}
}

func (a eventRuntimeAdapter) RequestRuntimeStepExecution(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.RuntimeStepExecutionAccepted, error) {
	if a.hub == nil || request.Kind == "" {
		return runtimemodel.RuntimeStepExecutionAccepted{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return a.publishRuntimeStepExecutionRequest(ctx, request, runtimeEventRequestID(request))
}

func (a eventRuntimeAdapter) publishRuntimeStepExecutionRequest(ctx context.Context, request runtimemodel.StepExecutionRequest, requestID string) (runtimemodel.RuntimeStepExecutionAccepted, error) {
	err := a.hub.Publish(ctx, events.Envelope{
		Topic:        runtimeevent.RuntimeBlockExecuteTopic(request.Kind),
		SourceModule: ModuleID,
		SubjectType:  "runtime_step",
		SubjectID:    string(request.StepID),
		WorkspaceID:  string(request.WorkspaceID),
		RunID:        string(request.RunID),
		StepID:       string(request.StepID),
		Payload: map[string]any{
			events.PayloadKeyRuntimeRequest:   request,
			events.PayloadKeyRuntimeRequestID: requestID,
		},
	})
	if response, ok := events.ResponseEnvelope(err); ok {
		accepted, ok := response.Payload[events.PayloadKeyRuntimeAccepted].(runtimemodel.RuntimeStepExecutionAccepted)
		if !ok {
			return runtimemodel.RuntimeStepExecutionAccepted{}, common.ErrInvalidArgument
		}
		return accepted, nil
	}
	if err != nil {
		return runtimemodel.RuntimeStepExecutionAccepted{}, err
	}
	return runtimemodel.RuntimeStepExecutionAccepted{}, fmt.Errorf("runtime block command %s returned no response", request.Kind)
}

func (a eventRuntimeAdapter) ExecuteRuntimeStep(ctx context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	if a.hub == nil || request.Kind == "" {
		return runtimemodel.StepExecutionResult{}, common.ErrInvalidArgument
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestID := runtimeEventRequestID(request)
	resultCh := make(chan runtimeEventResult, 1)
	var once sync.Once
	complete := func(value runtimeEventResult) {
		once.Do(func() {
			resultCh <- value
		})
	}
	unsubscribeCompleted := a.hub.Subscribe(runtimeevent.TopicRuntimeBlockExecuteCompleted, func(_ context.Context, envelope events.Envelope) error {
		if !runtimeResultMatches(envelope, request, requestID) {
			return nil
		}
		result, ok := envelope.Payload[events.PayloadKeyRuntimeResult].(runtimemodel.StepExecutionResult)
		if !ok {
			if completed, ok := envelope.Payload[events.PayloadKeyRuntimeCompleted].(runtimemodel.RuntimeStepExecutionCompleted); ok {
				result = runtimemodel.StepExecutionResult{Output: completed.Output}
			} else {
				complete(runtimeEventResult{err: common.ErrInvalidArgument})
				return nil
			}
		}
		complete(runtimeEventResult{result: result})
		return nil
	})
	defer unsubscribeCompleted()
	unsubscribeFailed := a.hub.Subscribe(runtimeevent.TopicRuntimeBlockExecuteFailed, func(_ context.Context, envelope events.Envelope) error {
		if !runtimeResultMatches(envelope, request, requestID) {
			return nil
		}
		result, _ := envelope.Payload[events.PayloadKeyRuntimeResult].(runtimemodel.StepExecutionResult)
		message, _ := envelope.Payload[events.PayloadKeyError].(string)
		if message == "" {
			if failed, ok := envelope.Payload[events.PayloadKeyRuntimeFailed].(runtimemodel.RuntimeStepExecutionFailed); ok {
				message = failed.Error
			}
		}
		if message == "" {
			message = "runtime block execution failed"
		}
		complete(runtimeEventResult{result: result, err: fmt.Errorf("%s", message)})
		return nil
	})
	defer unsubscribeFailed()
	if _, err := a.publishRuntimeStepExecutionRequest(ctx, request, requestID); err != nil {
		return runtimemodel.StepExecutionResult{}, err
	}
	select {
	case value := <-resultCh:
		return value.result, value.err
	case <-ctx.Done():
		return runtimemodel.StepExecutionResult{}, ctx.Err()
	}
}

func runtimeEventRequestID(request runtimemodel.StepExecutionRequest) string {
	parts := []string{string(request.WorkspaceID), string(request.RunID), string(request.StepID), strings.TrimSpace(request.Kind)}
	return strings.Join(parts, "|")
}

func runtimeResultMatches(envelope events.Envelope, request runtimemodel.StepExecutionRequest, requestID string) bool {
	if envelope.WorkspaceID != "" && envelope.WorkspaceID != string(request.WorkspaceID) {
		return false
	}
	if envelope.RunID != "" && envelope.RunID != string(request.RunID) {
		return false
	}
	if envelope.StepID != "" && envelope.StepID != string(request.StepID) {
		return false
	}
	if value, _ := envelope.Payload[events.PayloadKeyRuntimeRequestID].(string); strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value) == requestID
	}
	return true
}
