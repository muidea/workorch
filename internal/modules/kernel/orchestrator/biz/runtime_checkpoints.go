package orchestrator

import (
	"context"
	"fmt"
	"time"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) recordRunCheckpoint(ctx context.Context, run *runModel.Run) error {
	return o.recordRunCheckpointAtStep(ctx, run, "")
}

func (o *RuntimeOrchestrator) recordRunCheckpointAtStep(ctx context.Context, run *runModel.Run, currentStepID common.StepID) error {
	if run == nil || o.evidence.RuntimeCheckpoints == nil {
		return nil
	}
	if currentStepID == "" {
		currentStepID = o.latestStepID(ctx, run.ID)
	}
	checkpointType := common.CheckpointTypeRunState
	if run.BlockingTicketID != "" || run.BlockingApprovalID != "" {
		checkpointType = common.CheckpointTypeBlocked
	}
	return o.createRuntimeCheckpoint(ctx, run, currentStepID, checkpointType, map[string]any{
		"event": "run_state_changed",
		"state": string(run.State),
	})
}

func (o *RuntimeOrchestrator) recordStepCheckpoint(ctx context.Context, run *runModel.Run, step *runModel.Step, event string) error {
	if run == nil || step == nil || o.evidence.RuntimeCheckpoints == nil {
		return nil
	}
	data := map[string]any{
		"event":     event,
		"stepState": string(step.State),
		"stepKind":  step.Kind,
	}
	if class := stringFromMap(step.Output, failureOutputKeyClass); class != "" {
		data[failureOutputKeyClass] = class
	}
	if signature := stringFromMap(step.Output, failureOutputKeySignature); signature != "" {
		data[failureOutputKeySignature] = signature
	}
	if nodeID := graphNodeID(step); nodeID != "" {
		data[stepInputKeyGraphNodeID] = nodeID
	}
	if branch := graphBranch(step); branch != "" {
		data[stepOutputKeyGraphBranch] = branch
	}
	if completed := graphCompletedNodesFromStep(step); len(completed) > 0 {
		data[stepInputKeyGraphCompletedNodes] = completed
	}
	return o.createRuntimeCheckpoint(ctx, run, step.ID, common.CheckpointTypeRunState, data)
}

func (o *RuntimeOrchestrator) createRuntimeCheckpoint(
	ctx context.Context,
	run *runModel.Run,
	currentStepID common.StepID,
	checkpointType common.CheckpointType,
	data map[string]any,
) error {
	now := runtimeEvidenceTimestamp()
	item := &runModel.RuntimeCheckpoint{
		ID:                 common.CheckpointID(fmt.Sprintf("%s/checkpoint/%d", run.ID, time.Now().UTC().UnixNano())),
		WorkspaceID:        run.WorkspaceID,
		RunID:              run.ID,
		CheckpointType:     checkpointType,
		CurrentStepID:      currentStepID,
		RunStateSnapshot:   run.State,
		BlockingTicketID:   run.BlockingTicketID,
		BlockingApprovalID: run.BlockingApprovalID,
		ThreadRef:          "main",
		OccurredAt:         now,
		Data:               data,
	}
	if err := o.evidence.RuntimeCheckpoints.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return err
		}
		return o.evidence.RuntimeCheckpoints.Update(ctx, item)
	}
	return nil
}

func (o *RuntimeOrchestrator) latestStepID(ctx context.Context, runID common.RunID) common.StepID {
	if o == nil || o.steps == nil || runID == "" {
		return ""
	}
	steps, err := o.steps.ListByRun(ctx, runID)
	if err != nil || len(steps) == 0 {
		return ""
	}
	step := terminalExecutionStep(steps)
	if step == nil {
		return ""
	}
	return step.ID
}
