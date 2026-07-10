package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorcontracts "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/contracts"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type runtimeDispatchRequestError struct {
	Stage string
	Err   error
}

func (e runtimeDispatchRequestError) Error() string {
	if e.Err == nil {
		return "runtime dispatch request failed"
	}
	if e.Stage == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("runtime dispatch request failed at %s: %v", e.Stage, e.Err)
}

func (e runtimeDispatchRequestError) Unwrap() error {
	return e.Err
}

func wrapRuntimeDispatchRequestError(stage string, err error) error {
	if err == nil {
		return nil
	}
	return runtimeDispatchRequestError{Stage: stage, Err: err}
}

func runtimeDispatchRequestErrorStage(err error) string {
	var dispatchErr runtimeDispatchRequestError
	if !errors.As(err, &dispatchErr) {
		return ""
	}
	return dispatchErr.Stage
}

func (o *RuntimeOrchestrator) EnableEventDrivenDispatch() {
	if o == nil {
		return
	}
	o.eventDrivenDispatch = true
}

func (o *RuntimeOrchestrator) requestDispatchStep(ctx context.Context, runID common.RunID, workspaceID common.WorkspaceID, spec stepDispatchSpec) error {
	if o == nil || runID == "" || spec.ID == "" || spec.Kind == "" {
		return wrapRuntimeDispatchRequestError("validate_request", common.ErrInvalidArgument)
	}
	run, err := o.runtimeEventRun(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), runID)
	if err != nil {
		return wrapRuntimeDispatchRequestError("load_run", err)
	}
	if workspaceID == "" {
		workspaceID = run.WorkspaceID
	}
	prepared, err := o.prepareGapBaselineDispatchSpecs(ctx, run, []stepDispatchSpec{spec})
	if err != nil {
		return wrapRuntimeDispatchRequestError("prepare_authority_snapshot", err)
	}
	if len(prepared) != 1 {
		return wrapRuntimeDispatchRequestError("prepare_dispatch_spec", common.ErrConflict)
	}
	spec = prepared[0]
	if !o.eventDrivenDispatch {
		return wrapRuntimeDispatchRequestError("direct_dispatch", o.dispatchStepWithWorkspace(ctx, run, spec.ID, spec.Kind, spec.Input, nil))
	}
	idempotencyKey := dispatchStepIdempotencyKey(workspaceID, runID, spec.ID, spec.Kind, spec.Input)
	cmd := orchestratorcontracts.DispatchStepCommand{
		WorkspaceID:    workspaceID,
		RunID:          runID,
		StepID:         spec.ID,
		Kind:           spec.Kind,
		IdempotencyKey: idempotencyKey,
		Input:          cloneStepPayload(spec.Input),
	}
	if err := sendDispatchStepRequestedCommand(ctx, o.eventHub, cmd); err != nil {
		return wrapRuntimeDispatchRequestError("event_publish", err)
	}
	return wrapRuntimeDispatchRequestError("event_publish_readback", o.recordDispatchPublishReadback(ctx, run, cmd))
}

type rawMagicHubProvider interface {
	RawHub() magicevent.Hub
}

func sendDispatchStepRequestedCommand(ctx context.Context, hub events.Hub, cmd orchestratorevents.DispatchStepCommand) error {
	if hub == nil || cmd.RunID == "" || cmd.StepID == "" || cmd.Kind == "" {
		return common.ErrInvalidArgument
	}
	provider, ok := hub.(rawMagicHubProvider)
	if !ok || provider.RawHub() == nil {
		return common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorDispatchStepRequested, "kernel/orchestrator", orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, cmd)
	ev.BindLaneKey(events.EventHubLaneKey(events.Envelope{
		Topic:       orchestratorevents.TopicOrchestratorDispatchStepRequested,
		SubjectType: "step",
		SubjectID:   string(cmd.StepID),
		WorkspaceID: string(cmd.WorkspaceID),
		RunID:       string(cmd.RunID),
		StepID:      string(cmd.StepID),
	}))
	result := provider.RawHub().Send(ev)
	if result == nil {
		return common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return err
	}
	if _, err := result.Get(); err != nil {
		return err
	}
	return nil
}

func (o *RuntimeOrchestrator) recordDispatchPublishReadback(ctx context.Context, run *runModel.Run, cmd orchestratorcontracts.DispatchStepCommand) error {
	if o == nil || run == nil || run.ID == "" || cmd.StepID == "" {
		return common.ErrInvalidArgument
	}
	if o.steps == nil || o.runs == nil {
		return common.ErrConflict
	}
	step, err := o.steps.Get(ctx, cmd.StepID)
	if err == nil && step != nil && step.RunID == run.ID {
		if len(mapFromValue(run.ExecutionContext["dispatchReadback"])) > 0 {
			if run.ExecutionContext == nil {
				run.ExecutionContext = map[string]any{}
			}
			delete(run.ExecutionContext, "dispatchReadback")
			return o.runs.Update(ctx, run)
		}
		return nil
	}
	if err != nil && !errors.Is(err, common.ErrNotFound) {
		return err
	}
	if run.ExecutionContext == nil {
		run.ExecutionContext = map[string]any{}
	}
	run.ExecutionContext["dispatchReadback"] = map[string]any{
		"schemaVersion":          "workorch.dispatch_readback.v1",
		"state":                  "pending_owner_consumption",
		"workspaceId":            string(cmd.WorkspaceID),
		"runId":                  string(cmd.RunID),
		"stepId":                 string(cmd.StepID),
		"kind":                   cmd.Kind,
		"idempotencyKey":         cmd.IdempotencyKey,
		"queuedCommandPublished": true,
		"stepExists":             false,
		"recoverability":         "wait_for_eventhub_owner_consumption",
	}
	return o.runs.Update(ctx, run)
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

func dispatchStepIdempotencyKey(workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, kind string, input map[string]any) string {
	parts := []string{string(workspaceID), string(runID), string(stepID), strings.TrimSpace(kind)}
	if scope := dispatchStepActionIdempotencyScope(input); scope != "" {
		parts = append(parts, "action:"+scope)
	}
	return strings.Join(parts, "|")
}

func dispatchStepActionIdempotencyScope(input map[string]any) string {
	for _, action := range []map[string]any{
		mapFromValue(mapFromValue(input[currentActionContractKey])["nextAction"]),
		mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["nextAction"]),
		currentActionNextActionFromInput(input),
		mapFromValue(mapFromValue(input[convergenceAuthorityKey])["nextAction"]),
		mapFromValue(mapFromValue(input[gapBaselineKey])["nextAction"]),
	} {
		if len(action) == 0 {
			continue
		}
		if key := strings.TrimSpace(stringFromMap(action, "idempotencyKey")); key != "" {
			return key
		}
		if id := strings.TrimSpace(stringFromMap(action, "id")); id != "" {
			return id
		}
		if kind := strings.TrimSpace(firstNonEmptyString(stringFromMap(action, "type"), stringFromMap(action, "kind"))); kind != "" {
			hash := sha256HexBytes([]byte(stableCoverageJSON(action)))
			if len(hash) > 16 {
				hash = hash[:16]
			}
			return kind + ":" + hash
		}
	}
	return ""
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
	recoveryScoped := dispatchInputHasRecoveryAuthority(cmd.Input)
	if context := mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]); len(context) > 0 && !recoveryScoped {
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
	if len(baseline) > 0 && !recoveryScoped {
		if cmd.Input == nil {
			cmd.Input = map[string]any{}
		}
		cmd.Input[gapBaselineKey] = cloneStepPayload(baseline)
	} else if cmd.Input != nil && !recoveryScoped {
		delete(cmd.Input, gapBaselineKey)
	}
	authority := map[string]any{}
	if len(mapFromValue(run.ExecutionContext[coverageWorkItemContextKey])) > 0 {
		authority = convergenceAuthoritySnapshotForRun(run).ToMap()
	}
	if dispatchAuthoritySnapshotMeaningful(authority) && !recoveryScoped {
		if cmd.Input == nil {
			cmd.Input = map[string]any{}
		}
		cmd.Input[convergenceAuthorityKey] = cloneStepPayload(authority)
	} else if cmd.Input != nil && !recoveryScoped {
		delete(cmd.Input, convergenceAuthorityKey)
	}
	if canonicalLLMWorkspaceStepKind(cmd.Kind) == llmWorkspaceExecutionStepKind {
		var rebuildErr error
		cmd.Input, rebuildErr = rebuildDispatchInputFromConvergenceAuthority(run, cmd.StepID, cloneStepPayload(cmd.Input))
		if rebuildErr != nil {
			return rebuildErr
		}
	} else if dispatchInputRequiresConvergenceDispatchValidation(cmd.Input) {
		var finalizeErr error
		cmd.Input, finalizeErr = finalizeCurrentAuthorityDispatchInputForRun(run, cmd.StepID, cloneStepPayload(cmd.Input))
		if finalizeErr != nil {
			return finalizeErr
		}
	}
	return o.dispatchStepWithWorkspace(ctx, run, cmd.StepID, cmd.Kind, cloneStepPayload(cmd.Input), workspace)
}

func dispatchAuthoritySnapshotMeaningful(authority map[string]any) bool {
	return len(mapFromValue(authority["nextAction"])) > 0 || len(mapFromValue(authority["currentFocus"])) > 0
}
