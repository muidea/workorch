package activationports

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const activationRunCommandSource = "application/workorchd/workspace-activation"

func sendActivationMagicCommand[Command any, Result any](ctx context.Context, hub magicevent.Hub, topic string, destination string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, command Command) error {
	if hub == nil || topic == "" || destination == "" {
		return common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(topic, activationRunCommandSource, destination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(events.EventHubLaneKeyFor(topic, events.EventHubLaneScope{
		SubjectType: subjectType,
		SubjectID:   subjectID,
		WorkspaceID: string(workspaceID),
		RunID:       string(runID),
	}))
	response := hub.Send(ev)
	if response == nil {
		return fmt.Errorf("activation run command %s returned no response", topic)
	}
	if responseErr := response.Error(); responseErr != nil {
		return responseErr
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return resultErr
	}
	if _, ok := data.(Result); !ok {
		return fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return nil
}

func SupersedeRun(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID) error {
	return sendActivationMagicCommand[orchestratorevents.SupersedeRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandSupersedeRun, orchestratorevents.CommandDestination, "run", string(runID), workspaceID, runID, orchestratorevents.SupersedeRunCommand{
		RunID: runID,
	})
}

func CreateWorkspaceExecutionRun(ctx context.Context, hub magicevent.Hub, runID common.RunID, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID, executionContext map[string]any, priority int) error {
	return sendActivationMagicCommand[runevents.CreateRunCommand, runevents.RunCommandResult](ctx, hub, runevents.TopicRunCommandCreate, runevents.CommandDestination, "run", string(runID), workspaceID, runID, runevents.CreateRunCommand{
		RunID:              runID,
		WorkspaceID:        workspaceID,
		GoalID:             goalID,
		DoDID:              dodID,
		DefinitionRevision: activationcore.DefinitionRevisionIDFromExecutionContext(executionContext),
		InitialStepKind:    llmcommon.StepKindWorkspaceExecution,
		ExecutionContext:   executionContext,
		Priority:           priority,
	})
}

func StartRun(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID) error {
	return sendActivationMagicCommand[orchestratorevents.StartRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandStartRun, orchestratorevents.CommandDestination, "run", string(runID), workspaceID, runID, orchestratorevents.StartRunCommand{
		RunID: runID,
	})
}

func FailRun(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID, failureReason string) error {
	return sendActivationMagicCommand[orchestratorevents.FailRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandFailRun, orchestratorevents.CommandDestination, "run", string(runID), workspaceID, runID, orchestratorevents.FailRunCommand{
		RunID:  runID,
		Reason: failureReason,
	})
}
