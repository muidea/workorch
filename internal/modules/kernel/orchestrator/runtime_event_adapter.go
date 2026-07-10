package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/runtimeevent"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type eventRuntimeAdapter struct {
	hub magicevent.Hub
}

type runtimeEventResult struct {
	result runtimemodel.StepExecutionResult
	err    error
}

func NewEventRuntimeAdapter(hub events.Hub) orchestratorbiz.RuntimeAdapter {
	provider, _ := hub.(rawMagicHubProvider)
	if provider == nil {
		return eventRuntimeAdapter{}
	}
	return eventRuntimeAdapter{hub: provider.RawHub()}
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
	event := magicevent.NewEventWithContext(
		runtimeevent.RuntimeBlockExecuteTopic(request.Kind),
		ModuleID,
		runtimeevent.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		runtimeevent.ExecuteCommand{Request: request, RequestID: requestID},
	)
	result := a.hub.Send(event)
	if result == nil {
		return runtimemodel.RuntimeStepExecutionAccepted{}, common.ErrInvalidArgument
	}
	if magicErr := result.Error(); magicErr != nil {
		if magicErr.Cause != nil {
			return runtimemodel.RuntimeStepExecutionAccepted{}, magicErr.Cause
		}
		return runtimemodel.RuntimeStepExecutionAccepted{}, magicErr
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		if magicErr.Cause != nil {
			return runtimemodel.RuntimeStepExecutionAccepted{}, magicErr.Cause
		}
		return runtimemodel.RuntimeStepExecutionAccepted{}, magicErr
	}
	response, ok := data.(runtimeevent.ExecuteCommandResult)
	if !ok || response.Accepted.RequestID == "" {
		return runtimemodel.RuntimeStepExecutionAccepted{}, fmt.Errorf("runtime block command %s returned no accepted result", request.Kind)
	}
	return response.Accepted, nil
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
		once.Do(func() { resultCh <- value })
	}
	observer := magicevent.NewSimpleObserverWithMatchID(ModuleID+"/runtime-wait/"+requestID, runtimeevent.NotificationDestination, a.hub)
	observer.Subscribe(runtimeevent.TopicRuntimeBlockExecuteCompleted, func(event magicevent.Event, _ magicevent.Result) {
		data, ok := event.Data().(runtimeevent.CompletedData)
		if !ok || !runtimeCompletedMatches(data, request, requestID) {
			return
		}
		complete(runtimeEventResult{result: data.Result})
	})
	defer observer.Unsubscribe(runtimeevent.TopicRuntimeBlockExecuteCompleted)
	observer.Subscribe(runtimeevent.TopicRuntimeBlockExecuteFailed, func(event magicevent.Event, _ magicevent.Result) {
		data, ok := event.Data().(runtimeevent.FailedData)
		if !ok || !runtimeFailedMatches(data, request, requestID) {
			return
		}
		message := strings.TrimSpace(data.Error)
		if message == "" {
			message = strings.TrimSpace(data.Failed.Error)
		}
		if message == "" {
			message = "runtime block execution failed"
		}
		complete(runtimeEventResult{result: data.Result, err: fmt.Errorf("%s", message)})
	})
	defer observer.Unsubscribe(runtimeevent.TopicRuntimeBlockExecuteFailed)
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

func runtimeCompletedMatches(data runtimeevent.CompletedData, request runtimemodel.StepExecutionRequest, requestID string) bool {
	return runtimeScopeMatches(data.Completed.WorkspaceID, data.Completed.RunID, data.Completed.StepID, data.Completed.RequestID, request, requestID)
}

func runtimeFailedMatches(data runtimeevent.FailedData, request runtimemodel.StepExecutionRequest, requestID string) bool {
	return runtimeScopeMatches(data.Failed.WorkspaceID, data.Failed.RunID, data.Failed.StepID, data.Failed.RequestID, request, requestID)
}

func runtimeScopeMatches(workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, actualRequestID string, request runtimemodel.StepExecutionRequest, requestID string) bool {
	if workspaceID != "" && workspaceID != request.WorkspaceID {
		return false
	}
	if runID != "" && runID != request.RunID {
		return false
	}
	if stepID != "" && stepID != request.StepID {
		return false
	}
	return strings.TrimSpace(actualRequestID) == "" || strings.TrimSpace(actualRequestID) == requestID
}
