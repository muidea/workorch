package governance

import (
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
)

func bindOwnerLane(event magicevent.Event, ownerContracts []eventcontract.Contract, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID) {
	if event == nil {
		return
	}
	event.BindLaneKey(events.EventHubLaneKeyFor(ownerContracts, topic, eventcontract.LaneScope{
		SubjectType: subjectType,
		SubjectID:   subjectID,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
		StepID:      string(stepID),
	}))
}

func decisionResponse[Result any](response magicevent.Result, topic string) (Result, error) {
	var zero Result
	if response == nil {
		return zero, common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return zero, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return zero, resultErr
	}
	result, ok := data.(Result)
	if !ok {
		return zero, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}

func approvalResponse[Response any](response magicevent.Result, topic string) (Response, error) {
	var zero Response
	if response == nil {
		return zero, common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return zero, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return zero, resultErr
	}
	result, ok := data.(Response)
	if !ok {
		return zero, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}

func runResponse[Result any](response magicevent.Result, topic string) (Result, error) {
	var zero Result
	if response == nil {
		return zero, common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return zero, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return zero, resultErr
	}
	result, ok := data.(Result)
	if !ok {
		return zero, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}

func workspaceResponse(response magicevent.Result, topic string) (workspaceevents.WorkspaceCommandResult, error) {
	if response == nil {
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return workspaceevents.WorkspaceCommandResult{}, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return workspaceevents.WorkspaceCommandResult{}, resultErr
	}
	result, ok := data.(workspaceevents.WorkspaceCommandResult)
	if !ok {
		return workspaceevents.WorkspaceCommandResult{}, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}

func memoryResponse(response magicevent.Result, topic string) (memoryevents.MemoryCommandResult, error) {
	if response == nil {
		return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return memoryevents.MemoryCommandResult{}, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return memoryevents.MemoryCommandResult{}, resultErr
	}
	result, ok := data.(memoryevents.MemoryCommandResult)
	if !ok {
		return memoryevents.MemoryCommandResult{}, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}
