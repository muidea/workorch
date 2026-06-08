package activationports

import (
	"context"
	"errors"
	"fmt"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const activationRunCommandSource = "application/workorchd/workspace-activation"

func PublishRunCommand(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, payload map[string]any) error {
	if hub == nil || topic == "" {
		return common.ErrInvalidArgument
	}
	if _, err := ownercommandports.PublishWithResponseFrom(ctx, activationRunCommandSource, hub, topic, subjectType, subjectID, workspaceID, runID, payload); err != nil {
		if errors.Is(err, common.ErrInvalidArgument) {
			return fmt.Errorf("activation run command %s returned no response", topic)
		}
		return err
	}
	return nil
}

func SupersedeRun(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, runID common.RunID) error {
	return PublishRunCommand(ctx, hub, orchestratorevents.TopicOrchestratorCommandSupersedeRun, "run", string(runID), workspaceID, runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
}

func CreateWorkspaceExecutionRun(ctx context.Context, hub events.Hub, runID common.RunID, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID, executionContext map[string]any, priority int) error {
	return PublishRunCommand(ctx, hub, runevents.TopicRunCommandCreate, "run", string(runID), workspaceID, runID, map[string]any{
		events.PayloadKeyCommand: runcontracts.CreateRunCommand{
			RunID:              runID,
			WorkspaceID:        workspaceID,
			GoalID:             goalID,
			DoDID:              dodID,
			DefinitionRevision: activationcore.DefinitionRevisionIDFromExecutionContext(executionContext),
			InitialStepKind:    llmcommon.StepKindWorkspaceExecution,
			ExecutionContext:   executionContext,
			Priority:           priority,
		},
	})
}

func StartRun(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, runID common.RunID) error {
	return PublishRunCommand(ctx, hub, orchestratorevents.TopicOrchestratorCommandStartRun, "run", string(runID), workspaceID, runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
}

func FailRun(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, runID common.RunID, failureReason string) error {
	return PublishRunCommand(ctx, hub, orchestratorevents.TopicOrchestratorCommandFailRun, "run", string(runID), workspaceID, runID, map[string]any{
		events.PayloadKeyRunID:         string(runID),
		events.PayloadKeyFailureReason: failureReason,
	})
}
