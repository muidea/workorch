package governance

import (
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
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

func workspaceResponse[Response any](response magicevent.Result, topic string) (Response, error) {
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

func memoryResponse[Response any](response magicevent.Result, topic string) (Response, error) {
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
