package governance

import (
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func bindOwnerLane(event magicevent.Event, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID) {
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

func decisionResponse(response magicevent.Result, topic string) (decisionevents.DecisionCommandResult, error) {
	if response == nil {
		return decisionevents.DecisionCommandResult{}, common.ErrInvalidArgument
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
		return decisionevents.DecisionCommandResult{}, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}

func approvalResponse(response magicevent.Result, topic string) (approvalevents.ApprovalCommandResult, error) {
	if response == nil {
		return approvalevents.ApprovalCommandResult{}, common.ErrInvalidArgument
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
		return approvalevents.ApprovalCommandResult{}, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}

func runResponse(response magicevent.Result, topic string) (runevents.RunCommandResult, error) {
	if response == nil {
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
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
		return runevents.RunCommandResult{}, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
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
	data, err := response.Get()
	if err != nil {
		return workspaceevents.WorkspaceCommandResult{}, err
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
	data, err := response.Get()
	if err != nil {
		return memoryevents.MemoryCommandResult{}, err
	}
	result, ok := data.(memoryevents.MemoryCommandResult)
	if !ok {
		return memoryevents.MemoryCommandResult{}, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}

func orchestratorResponse(response magicevent.Result, topic string) (orchestratorevents.OrchestratorCommandResult, error) {
	if response == nil {
		return orchestratorevents.OrchestratorCommandResult{}, common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return orchestratorevents.OrchestratorCommandResult{}, err
	}
	data, err := response.Get()
	if err != nil {
		return orchestratorevents.OrchestratorCommandResult{}, err
	}
	result, ok := data.(orchestratorevents.OrchestratorCommandResult)
	if !ok {
		return orchestratorevents.OrchestratorCommandResult{}, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}
