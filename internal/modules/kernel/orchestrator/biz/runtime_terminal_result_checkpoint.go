package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeStepResultAvailableEvent = "runtime_step_result_available"

type RuntimeStepTerminalResultCheckpoint struct {
	RequestID        string
	TerminalTopic    string
	StepKind         string
	RecoveryPolicy   runtimemodel.RuntimeRecoveryPolicy
	Output           map[string]any
	Error            string
	FailureClass     string
	FailureSignature string
	CompletedAt      string
	EvidenceRefs     []string
	RawResultRef     string
}

func (o *RuntimeOrchestrator) RecordRuntimeStepTerminalResult(
	ctx context.Context,
	runID common.RunID,
	stepID common.StepID,
	terminal RuntimeStepTerminalResultCheckpoint,
) error {
	if o == nil || o.runs == nil || o.steps == nil || runID == "" || stepID == "" {
		return common.ErrInvalidArgument
	}
	if o.evidence.RuntimeCheckpoints == nil {
		return nil
	}
	run, err := o.runs.Get(ctx, runID)
	if err != nil {
		return err
	}
	step, err := o.steps.Get(ctx, stepID)
	if err != nil {
		return err
	}
	if step.RunID != run.ID {
		return common.ErrConflict
	}
	data := map[string]any{
		"event":         runtimeStepResultAvailableEvent,
		"terminalTopic": strings.TrimSpace(terminal.TerminalTopic),
		"stepKind":      firstNonEmptyString(strings.TrimSpace(terminal.StepKind), step.Kind),
	}
	if requestID := strings.TrimSpace(terminal.RequestID); requestID != "" {
		data["requestId"] = requestID
	}
	policy := terminal.RecoveryPolicy.Normalized()
	data["runtimeRecoveryPolicy"] = policy.ToMap()
	data["recoverySafety"] = policy.WatchdogRecoverySafety()
	if len(terminal.Output) > 0 {
		data["output"] = cloneStepPayload(terminal.Output)
	}
	if message := strings.TrimSpace(terminal.Error); message != "" {
		data["error"] = message
	}
	if failureClass := strings.TrimSpace(terminal.FailureClass); failureClass != "" {
		data["failureClass"] = failureClass
	}
	if failureSignature := strings.TrimSpace(terminal.FailureSignature); failureSignature != "" {
		data["failureSignature"] = failureSignature
	}
	if completedAt := strings.TrimSpace(terminal.CompletedAt); completedAt != "" {
		data["completedAt"] = completedAt
	}
	if len(terminal.EvidenceRefs) > 0 {
		data["evidenceRefs"] = append([]string(nil), terminal.EvidenceRefs...)
	}
	if rawResultRef := strings.TrimSpace(terminal.RawResultRef); rawResultRef != "" {
		data["rawResultRef"] = rawResultRef
	}
	now := runtimeEvidenceTimestamp()
	item := &runModel.RuntimeCheckpoint{
		ID:                 runtimeTerminalResultCheckpointID(step.ID, terminal.RequestID, terminal.TerminalTopic),
		WorkspaceID:        run.WorkspaceID,
		RunID:              run.ID,
		CheckpointType:     common.CheckpointTypeRecovery,
		CurrentStepID:      step.ID,
		RunStateSnapshot:   run.State,
		BlockingTicketID:   run.BlockingTicketID,
		BlockingApprovalID: run.BlockingApprovalID,
		ThreadRef:          "main",
		OccurredAt:         firstNonEmptyString(strings.TrimSpace(terminal.CompletedAt), now),
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

func (o *RuntimeOrchestrator) recoverRuntimeTerminalResultForRunningRun(ctx context.Context, run *runModel.Run) (bool, error) {
	if o == nil || run == nil || run.State != common.RunStateRunning || o.steps == nil {
		return false, nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return false, err
	}
	step := terminalExecutionStep(steps)
	if step == nil || step.State != common.StepStateDispatched {
		return false, nil
	}
	result, execErr, ok, err := o.runtimeTerminalResultFromCheckpoint(ctx, run, step)
	if err != nil || !ok {
		return false, err
	}
	return true, o.FinalizeRuntimeStepResult(ctx, run.ID, step.ID, result, execErr)
}

func (o *RuntimeOrchestrator) runtimeTerminalResultFromCheckpoint(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
) (runtimemodel.StepExecutionResult, error, bool, error) {
	if o == nil || run == nil || step == nil || o.evidence.RuntimeCheckpoints == nil {
		return runtimemodel.StepExecutionResult{}, nil, false, nil
	}
	checkpoints, err := o.evidence.RuntimeCheckpoints.ListByRun(ctx, run.ID)
	if err != nil {
		return runtimemodel.StepExecutionResult{}, nil, false, err
	}
	var latest *runModel.RuntimeCheckpoint
	for _, checkpoint := range checkpoints {
		if checkpoint == nil || checkpoint.CurrentStepID != step.ID || checkpoint.Data == nil {
			continue
		}
		if strings.TrimSpace(stringFromMap(checkpoint.Data, "event")) != runtimeStepResultAvailableEvent {
			continue
		}
		if latest == nil ||
			checkpoint.OccurredAt > latest.OccurredAt ||
			(checkpoint.OccurredAt == latest.OccurredAt && checkpoint.ID > latest.ID) {
			latest = checkpoint
		}
	}
	if latest == nil {
		return runtimemodel.StepExecutionResult{}, nil, false, nil
	}
	output := cloneStepPayload(mapFromValue(latest.Data["output"]))
	result := runtimemodel.StepExecutionResult{Output: output}
	message := strings.TrimSpace(stringFromMap(latest.Data, "error"))
	if message == "" {
		return result, nil, true, nil
	}
	return result, errors.New(message), true, nil
}

func runtimeTerminalResultCheckpointID(stepID common.StepID, requestID string, terminalTopic string) common.CheckpointID {
	fingerprint := strings.TrimSpace(requestID)
	if fingerprint == "" {
		fingerprint = strings.TrimSpace(terminalTopic)
	}
	if fingerprint == "" {
		fingerprint = string(stepID)
	}
	sum := sha256.Sum256([]byte(fingerprint))
	return common.CheckpointID(fmt.Sprintf("%s/checkpoint/runtime-result/%s", stepID, hex.EncodeToString(sum[:8])))
}
