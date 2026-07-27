package orchestrator

// EventHub response helpers belong to the Runtime V2 owner Biz.

import (
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func bindOwnerCommandLane(event magicevent.Event, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID) {
	if event == nil {
		return
	}
	event.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{
		SubjectType: subjectType,
		SubjectID:   subjectID,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))
}

func bindGovernanceCommandLane(event magicevent.Event, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID) {
	if event == nil {
		return
	}
	event.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{
		SubjectType: subjectType,
		SubjectID:   subjectID,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
		StepID:      string(stepID),
	}))
}

func runCommandResponse(response magicevent.Result, topic string) (runevents.RunCommandResult, error) {
	if response == nil {
		return runevents.RunCommandResult{}, fmt.Errorf("run command %s returned no response", topic)
	}
	if err := response.Error(); err != nil {
		return runevents.RunCommandResult{}, err
	}
	data, err := response.Get()
	if err != nil {
		return runevents.RunCommandResult{}, err
	}
	result, ok := data.(runevents.RunCommandResult)
	if !ok {
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
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
	data, err := response.Get()
	if err != nil {
		return workspaceevents.WorkspaceCommandResult{}, err
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
	data, err := response.Get()
	if err != nil {
		return memoryevents.MemoryCommandResult{}, err
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
	data, err := response.Get()
	if err != nil {
		return decisionevents.DecisionCommandResult{}, err
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
	data, err := response.Get()
	if err != nil {
		return approvalevents.ApprovalCommandResult{}, err
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
	data, err := response.Get()
	if err != nil {
		return resultevents.ResultCommandResult{}, err
	}
	result, ok := data.(resultevents.ResultCommandResult)
	if !ok {
		return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
	}
	return result, nil
}
