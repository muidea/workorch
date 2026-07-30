package ownerevent

import (
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func BindLane(event magicevent.Event, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID) {
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

func Workspace(response magicevent.Result, topic string) (workspaceevents.WorkspaceCommandResult, error) {
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

// WorkspaceLifecycle decodes a Workspace-owned lifecycle/projection receipt.
// Application may coordinate typed facts, but it never interprets the owner
// transition or writes the Workspace record directly.
func WorkspaceLifecycle(response magicevent.Result, topic string) (workspaceevents.WorkspaceLifecycleCommandResult, error) {
	if response == nil {
		return workspaceevents.WorkspaceLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return workspaceevents.WorkspaceLifecycleCommandResult{}, err
	}
	data, err := response.Get()
	if err != nil {
		return workspaceevents.WorkspaceLifecycleCommandResult{}, err
	}
	result, ok := data.(workspaceevents.WorkspaceLifecycleCommandResult)
	if !ok {
		return workspaceevents.WorkspaceLifecycleCommandResult{}, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}

func Run(response magicevent.Result, topic string) (runevents.RunCommandResult, error) {
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

// RunLifecycle decodes the Run owner's RUNTIME-SMO lifecycle receipt. The
// application only submits an intent and never infers or assigns the resulting
// lifecycle state.
func RunLifecycle(response magicevent.Result, topic string) (runevents.RunLifecycleCommandResult, error) {
	if response == nil {
		return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return runevents.RunLifecycleCommandResult{}, err
	}
	data, err := response.Get()
	if err != nil {
		return runevents.RunLifecycleCommandResult{}, err
	}
	result, ok := data.(runevents.RunLifecycleCommandResult)
	if !ok {
		return runevents.RunLifecycleCommandResult{}, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}

func Orchestrator(response magicevent.Result, topic string) (orchestratorevents.OrchestratorCommandResult, error) {
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

func Decision(response magicevent.Result, topic string) (decisionevents.DecisionCommandResult, error) {
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

func Approval(response magicevent.Result, topic string) (approvalevents.ApprovalCommandResult, error) {
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

func Memory(response magicevent.Result, topic string) (memoryevents.MemoryCommandResult, error) {
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

func Result(response magicevent.Result, topic string) (resultevents.ResultCommandResult, error) {
	if response == nil {
		return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
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
		return resultevents.ResultCommandResult{}, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}

func Soul(response magicevent.Result, topic string) (soulevents.SoulCommandResult, error) {
	if response == nil {
		return soulevents.SoulCommandResult{}, common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return soulevents.SoulCommandResult{}, err
	}
	data, err := response.Get()
	if err != nil {
		return soulevents.SoulCommandResult{}, err
	}
	result, ok := data.(soulevents.SoulCommandResult)
	if !ok {
		return soulevents.SoulCommandResult{}, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}

func Skill(response magicevent.Result, topic string) (skillevents.SkillCommandResult, error) {
	if response == nil {
		return skillevents.SkillCommandResult{}, common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return skillevents.SkillCommandResult{}, err
	}
	data, err := response.Get()
	if err != nil {
		return skillevents.SkillCommandResult{}, err
	}
	result, ok := data.(skillevents.SkillCommandResult)
	if !ok {
		return skillevents.SkillCommandResult{}, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}

func MCP(response magicevent.Result, topic string) (mcpevents.MCPCommandResult, error) {
	if response == nil {
		return mcpevents.MCPCommandResult{}, common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return mcpevents.MCPCommandResult{}, err
	}
	data, err := response.Get()
	if err != nil {
		return mcpevents.MCPCommandResult{}, err
	}
	result, ok := data.(mcpevents.MCPCommandResult)
	if !ok {
		return mcpevents.MCPCommandResult{}, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}

func EventStream(response magicevent.Result, topic string) (eventstreamevents.EventStreamCommandResult, error) {
	if response == nil {
		return eventstreamevents.EventStreamCommandResult{}, common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return eventstreamevents.EventStreamCommandResult{}, err
	}
	data, err := response.Get()
	if err != nil {
		return eventstreamevents.EventStreamCommandResult{}, err
	}
	result, ok := data.(eventstreamevents.EventStreamCommandResult)
	if !ok {
		return eventstreamevents.EventStreamCommandResult{}, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}

func Governance(response magicevent.Result, topic string) (governanceevents.GovernanceCommandResult, error) {
	if response == nil {
		return governanceevents.GovernanceCommandResult{}, common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return governanceevents.GovernanceCommandResult{}, err
	}
	data, err := response.Get()
	if err != nil {
		return governanceevents.GovernanceCommandResult{}, err
	}
	result, ok := data.(governanceevents.GovernanceCommandResult)
	if !ok {
		return governanceevents.GovernanceCommandResult{}, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}
