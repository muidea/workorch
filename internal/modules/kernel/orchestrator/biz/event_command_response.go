package orchestrator

// EventHub response helpers belong to the Runtime owner Biz.

import (
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
)

func bindOwnerCommandLane(event magicevent.Event, ownerContracts []eventcontract.Contract, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID) {
	if event == nil {
		return
	}
	event.BindLaneKey(events.EventHubLaneKeyFor(ownerContracts, topic, eventcontract.LaneScope{
		SubjectType: subjectType,
		SubjectID:   subjectID,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))
}

func bindGovernanceCommandLane(event magicevent.Event, ownerContracts []eventcontract.Contract, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID) {
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

func runCommandResponse[Result any](response magicevent.Result, topic string) (Result, error) {
	var zero Result
	if response == nil {
		return zero, fmt.Errorf("run command %s returned no response", topic)
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
		return zero, common.ErrInvalidArgument
	}
	return result, nil
}

func workspaceCommandResponse(response magicevent.Result, topic string) (workspaceevents.WorkspaceCommandResult, error) {
	if response == nil {
		return workspaceevents.WorkspaceCommandResult{}, fmt.Errorf("workspace command %s returned no response", topic)
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
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
	return result, nil
}

func memoryCommandResponse(response magicevent.Result, topic string) (memoryevents.MemoryCommandResult, error) {
	if response == nil {
		return memoryevents.MemoryCommandResult{}, fmt.Errorf("memory command %s returned no response", topic)
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
		return memoryevents.MemoryCommandResult{}, common.ErrInvalidArgument
	}
	return result, nil
}

func decisionCommandResponse(response magicevent.Result, topic string) (decisionevents.DecisionCommandResult, error) {
	if response == nil {
		return decisionevents.DecisionCommandResult{}, fmt.Errorf("decision command %s returned no response", topic)
	}
	if err := response.Error(); err != nil {
		return decisionevents.DecisionCommandResult{}, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return decisionevents.DecisionCommandResult{}, resultErr
	}
	result, ok := data.(decisionevents.DecisionCommandResult)
	if !ok {
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
	}
	return result, nil
}

func approvalCommandResponse(response magicevent.Result, topic string) (approvalevents.ApprovalCommandResult, error) {
	if response == nil {
		return approvalevents.ApprovalCommandResult{}, fmt.Errorf("approval command %s returned no response", topic)
	}
	if err := response.Error(); err != nil {
		return approvalevents.ApprovalCommandResult{}, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return approvalevents.ApprovalCommandResult{}, resultErr
	}
	result, ok := data.(approvalevents.ApprovalCommandResult)
	if !ok {
		return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
	}
	return result, nil
}

func resultCommandResponse(response magicevent.Result, topic string) (resultevents.ResultCommandResult, error) {
	if response == nil {
		return resultevents.ResultCommandResult{}, fmt.Errorf("result command %s returned no response", topic)
	}
	if err := response.Error(); err != nil {
		return resultevents.ResultCommandResult{}, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return resultevents.ResultCommandResult{}, resultErr
	}
	result, ok := data.(resultevents.ResultCommandResult)
	if !ok {
		return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
	}
	return result, nil
}
