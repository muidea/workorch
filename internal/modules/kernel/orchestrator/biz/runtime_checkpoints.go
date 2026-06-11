package orchestrator

import (
	"context"
	"fmt"
	"time"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type checkpointSubsystem struct {
	evidence RuntimeEvidenceRepositories
	steps    repository.StepRepository
}

func newCheckpointSubsystem(evidence RuntimeEvidenceRepositories, steps repository.StepRepository) *checkpointSubsystem {
	return &checkpointSubsystem{
		evidence: evidence,
		steps:    steps,
	}
}

func (c *checkpointSubsystem) recordRunCheckpoint(ctx context.Context, run *runModel.Run) error {
	return c.recordRunCheckpointAtStep(ctx, run, "")
}

func (c *checkpointSubsystem) recordRunCheckpointAtStep(ctx context.Context, run *runModel.Run, currentStepID common.StepID) error {
	if c == nil || run == nil || c.evidence.RuntimeCheckpoints == nil {
		return nil
	}
	if currentStepID == "" {
		currentStepID = c.latestStepID(ctx, run.ID)
	}
	checkpointType := common.CheckpointTypeRunState
	if run.BlockingTicketID != "" || run.BlockingApprovalID != "" {
		checkpointType = common.CheckpointTypeBlocked
	}
	return c.createRuntimeCheckpoint(ctx, run, currentStepID, checkpointType, map[string]any{
		"event": "run_state_changed",
		"state": string(run.State),
	})
}

func (c *checkpointSubsystem) recordStepCheckpoint(ctx context.Context, run *runModel.Run, step *runModel.Step, event string) error {
	if c == nil || run == nil || step == nil || c.evidence.RuntimeCheckpoints == nil {
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
	return c.createRuntimeCheckpoint(ctx, run, step.ID, common.CheckpointTypeRunState, data)
}

func (c *checkpointSubsystem) recordStepProgressCheckpoint(ctx context.Context, run *runModel.Run, step *runModel.Step, event string, data map[string]any) error {
	if c == nil || run == nil || step == nil || c.evidence.RuntimeCheckpoints == nil {
		return nil
	}
	payload := map[string]any{
		"event":     event,
		"stepState": string(step.State),
		"stepKind":  step.Kind,
	}
	for key, value := range data {
		if key == "" || value == nil {
			continue
		}
		payload[key] = value
	}
	return c.createRuntimeCheckpoint(ctx, run, step.ID, common.CheckpointTypeRunState, payload)
}

func (c *checkpointSubsystem) createRuntimeCheckpoint(
	ctx context.Context,
	run *runModel.Run,
	currentStepID common.StepID,
	checkpointType common.CheckpointType,
	data map[string]any,
) error {
	if c == nil || run == nil || c.evidence.RuntimeCheckpoints == nil {
		return nil
	}
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
	if err := c.evidence.RuntimeCheckpoints.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return err
		}
		return c.evidence.RuntimeCheckpoints.Update(ctx, item)
	}
	return nil
}

func (c *checkpointSubsystem) latestStepID(ctx context.Context, runID common.RunID) common.StepID {
	if c == nil || c.steps == nil || runID == "" {
		return ""
	}
	steps, err := c.steps.ListByRun(ctx, runID)
	if err != nil || len(steps) == 0 {
		return ""
	}
	step := terminalExecutionStep(steps)
	if step == nil {
		return ""
	}
	return step.ID
}
