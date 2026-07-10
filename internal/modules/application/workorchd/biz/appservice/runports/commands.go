package runports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CreateRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" {
		return common.ErrInvalidArgument
	}
	kind := item.InitialStepKind
	if kind == "" {
		kind = llmcommon.StepKindWorkspaceExecution
	}
	_, err := sendRunMagicCommand[runevents.CreateRunCommand, runevents.RunCommandResult](ctx, hub, runevents.TopicRunCommandCreate, runevents.CommandDestination, "run", string(item.ID), item.WorkspaceID, item.ID, runevents.CreateRunCommand{
		RunID:              item.ID,
		WorkspaceID:        item.WorkspaceID,
		GoalID:             item.GoalID,
		DoDID:              item.DoDID,
		DefinitionRevision: item.DefinitionRevision,
		InitialStepKind:    kind,
		ExecutionContext:   item.ExecutionContext,
		Priority:           item.Priority,
	})
	return err
}

func StartRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	_, err := sendRunMagicCommand[orchestratorevents.StartRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandStartRun, orchestratorevents.CommandDestination, "run", string(item.ID), item.WorkspaceID, item.ID, orchestratorevents.StartRunCommand{
		RunID: item.ID,
	})
	return err
}

func ResumeRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run, executionContextPatch map[string]any) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	_, err := sendRunMagicCommand[orchestratorevents.ResumeRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandResumeRun, orchestratorevents.CommandDestination, "run", string(item.ID), item.WorkspaceID, item.ID, orchestratorevents.ResumeRunCommand{
		WorkspaceID:           item.WorkspaceID,
		RunID:                 item.ID,
		ExecutionContextPatch: cloneMap(executionContextPatch),
	})
	return err
}

func RecoverRuntimeGap(ctx context.Context, hub magicevent.Hub, item *runmodel.Run, recovery RuntimeGapRecovery, executionContextPatch map[string]any) error {
	if item == nil || item.ID == "" || recovery.StepID == "" || recovery.Gap == "" {
		return common.ErrInvalidArgument
	}
	workspaceID := item.WorkspaceID
	if recovery.WorkspaceID != "" {
		workspaceID = recovery.WorkspaceID
	}
	if workspaceID == "" {
		return common.ErrInvalidArgument
	}
	idempotencyKey := recovery.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = runtimeGapRecoveryIdempotencyKey(workspaceID, item.ID, recovery.StepID, string(recovery.Gap), recovery.RequestID, recovery.QueueKey)
	}
	_, err := sendRunMagicCommand[orchestratorevents.RecoverRuntimeGapCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandRecoverRuntimeGap, orchestratorevents.CommandDestination, "run", string(item.ID), workspaceID, item.ID, orchestratorevents.RecoverRuntimeGapCommand{
		WorkspaceID:           workspaceID,
		RunID:                 item.ID,
		StepID:                recovery.StepID,
		StepKind:              recovery.StepKind,
		Gap:                   string(recovery.Gap),
		IdempotencyKey:        idempotencyKey,
		RequestID:             recovery.RequestID,
		QueueKey:              recovery.QueueKey,
		LatestStepTime:        recovery.LatestStepTime,
		RecoveryPolicy:        recovery.RecoveryPolicy,
		RecoverySafety:        recovery.RecoverySafety,
		Reason:                recovery.Reason,
		ExecutionContextPatch: cloneMap(executionContextPatch),
	})
	return err
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
