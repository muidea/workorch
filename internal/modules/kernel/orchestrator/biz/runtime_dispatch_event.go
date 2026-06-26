package orchestrator

import (
	"context"

	orchestratorcontracts "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/contracts"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (o *RuntimeOrchestrator) EnableEventDrivenDispatch() {
	if o == nil {
		return
	}
	o.eventDrivenDispatch = true
}

func (o *RuntimeOrchestrator) requestDispatchStep(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID, spec stepDispatchSpec) error {
	if o == nil || runID == "" || spec.ID == "" || spec.Kind == "" {
		return common.ErrInvalidArgument
	}
	run, err := o.runtimeEventRun(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), runID)
	if err != nil {
		return err
	}
	if workspaceID == "" {
		workspaceID = run.WorkspaceID
	}
	prepared, err := o.prepareGapBaselineDispatchSpecs(ctx, run, []stepDispatchSpec{spec})
	if err != nil {
		return err
	}
	if len(prepared) != 1 {
		return common.ErrConflict
	}
	spec = prepared[0]
	if !o.eventDrivenDispatch {
		return o.dispatchStepWithWorkspace(ctx, run, spec.ID, spec.Kind, spec.Input, nil)
	}
	idempotencyKey := dispatchStepIdempotencyKey(workspaceID, runID, spec.ID, spec.Kind)
	cmd := orchestratorcontracts.DispatchStepCommand{
		WorkspaceID:    workspaceID,
		RunID:          runID,
		StepID:         spec.ID,
		Kind:           spec.Kind,
		IdempotencyKey: idempotencyKey,
		Input:          cloneStepPayload(spec.Input),
	}
	return o.eventHub.Publish(ctx, events.Envelope{
		Topic:        orchestratorevents.TopicOrchestratorDispatchStepRequested,
		SourceModule: "kernel/orchestrator",
		SubjectType:  "step",
		SubjectID:    string(spec.ID),
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		StepID:       string(spec.ID),
		Payload: map[string]any{
			events.PayloadKeyCommand:        cmd,
			events.PayloadKeyWorkspaceID:    string(workspaceID),
			events.PayloadKeyRunID:          string(runID),
			events.PayloadKeyStepID:         string(spec.ID),
			events.PayloadKeyKind:           spec.Kind,
			events.PayloadKeyIdempotencyKey: idempotencyKey,
			events.PayloadKeyInput:          cloneStepPayload(spec.Input),
		},
	})
}

func workspaceIDForDispatch(run *runModel.Run, workspace *workspaceModel.Workspace) common.WorkspaceID {
	if workspace != nil && workspace.ID != "" {
		return workspace.ID
	}
	if run != nil {
		return run.WorkspaceID
	}
	return ""
}

func dispatchStepIdempotencyKey(workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, kind string) string {
	return string(workspaceID) + "|" + string(runID) + "|" + string(stepID) + "|" + kind
}

func (o *RuntimeOrchestrator) RequestUniqueDispatchStep(ctx context.Context, run *runModel.Run, stepID common.StepID, kind string, input map[string]any, workspaceID common.WorkspaceID) error {
	if o == nil || run == nil || run.ID == "" || stepID == "" {
		return common.ErrInvalidArgument
	}
	uniqueStepID, err := uniqueDispatchStepID(ctx, o.steps, run.ID, stepID)
	if err != nil {
		return err
	}
	return o.requestDispatchStep(ctx, run.ID, workspaceID, stepDispatchSpec{
		ID:    uniqueStepID,
		Kind:  kind,
		Input: cloneStepPayload(input),
	})
}

func (o *RuntimeOrchestrator) DispatchRequestedStep(ctx context.Context, cmd orchestratorcontracts.DispatchStepCommand) error {
	if o == nil || cmd.RunID == "" || cmd.StepID == "" || cmd.Kind == "" {
		return common.ErrInvalidArgument
	}
	run, err := o.runtimeEventRun(common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID), cmd.RunID)
	if err != nil {
		return err
	}
	var workspace *workspaceModel.Workspace
	if cmd.WorkspaceID != "" && cmd.WorkspaceID != run.WorkspaceID {
		return common.ErrConflict
	}
	if cmd.WorkspaceID != "" {
		workspace, err = o.workspaces.Get(ctx, cmd.WorkspaceID)
	} else {
		workspace, err = o.workspaces.Get(ctx, run.WorkspaceID)
	}
	if err != nil {
		return err
	}
	if err := o.ensureGapBaselineBeforeDispatch(ctx, run); err != nil {
		return err
	}
	if gated, err := gapBaselineDispatchGate(run); gated || err != nil {
		return err
	}
	if context := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]); len(context) > 0 {
		if cmd.Input == nil {
			cmd.Input = map[string]any{}
		}
		focus := mapFromValue(run.ExecutionContext[coverageFocusKey])
		if existing := mapFromValue(cmd.Input[coverageWorkItemContextKey]); len(existing) > 0 {
			cmd.Input[coverageWorkItemContextKey] = coverageWorkItemContextAlignedToFocus(existing, focus)
		} else {
			cmd.Input[coverageWorkItemContextKey] = coverageWorkItemContextAlignedToFocus(context, focus)
		}
	}
	baseline := gapBaselineFromRun(run)
	if len(baseline) == 0 {
		baseline = mapFromValue(run.ExecutionContext[gapBaselineKey])
	}
	if len(baseline) > 0 {
		if cmd.Input == nil {
			cmd.Input = map[string]any{}
		}
		cmd.Input[gapBaselineKey] = cloneStepPayload(baseline)
	} else if cmd.Input != nil {
		delete(cmd.Input, gapBaselineKey)
	}
	authority := map[string]any{}
	if len(mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])) > 0 {
		authority = convergenceAuthoritySnapshotForRun(run).ToMap()
	}
	if dispatchAuthoritySnapshotMeaningful(authority) {
		if cmd.Input == nil {
			cmd.Input = map[string]any{}
		}
		cmd.Input[convergenceAuthorityKey] = cloneStepPayload(authority)
	} else if cmd.Input != nil {
		delete(cmd.Input, convergenceAuthorityKey)
	}
	if canonicalLLMWorkspaceStepKind(cmd.Kind) == llmWorkspaceExecutionStepKind {
		var rebuildErr error
		cmd.Input, rebuildErr = rebuildDispatchInputFromConvergenceAuthority(run, cmd.StepID, cloneStepPayload(cmd.Input))
		if rebuildErr != nil {
			return rebuildErr
		}
	}
	return o.dispatchStepWithWorkspace(ctx, run, cmd.StepID, cmd.Kind, cloneStepPayload(cmd.Input), workspace)
}

func dispatchAuthoritySnapshotMeaningful(authority map[string]any) bool {
	return len(mapFromValue(authority["nextAction"])) > 0 || len(mapFromValue(authority["currentFocus"])) > 0
}
