package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/blocks/pkg/runtimeevent"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Module) finalizeRuntimeResult(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	workspaceID, err := runtimeResultWorkspaceIDFromEnvelope(envelope)
	if err != nil {
		return err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	runID, stepID, result, execErr, err := runtimeResultFromEnvelope(envelope)
	if err != nil {
		return err
	}
	if err := m.bizPtr.RecordRuntimeStepTerminalResult(ctx, runID, stepID, runtimeTerminalResultCheckpointFromEnvelope(envelope, result, execErr)); err != nil {
		return err
	}
	return m.bizPtr.FinalizeRuntimeStepResult(ctx, runID, stepID, result, execErr)
}

func runtimeResultWorkspaceIDFromEnvelope(envelope events.Envelope) (common.WorkspaceID, error) {
	workspaceID := common.WorkspaceID(envelope.WorkspaceID)
	var payloadWorkspaceID common.WorkspaceID
	switch envelope.Topic {
	case runtimeevent.TopicRuntimeBlockExecuteCompleted:
		completed, _ := envelope.Payload[events.PayloadKeyRuntimeCompleted].(runtimemodel.RuntimeStepExecutionCompleted)
		payloadWorkspaceID = completed.WorkspaceID
	case runtimeevent.TopicRuntimeBlockExecuteFailed:
		failed, _ := envelope.Payload[events.PayloadKeyRuntimeFailed].(runtimemodel.RuntimeStepExecutionFailed)
		payloadWorkspaceID = failed.WorkspaceID
	default:
		return "", common.ErrInvalidArgument
	}
	if workspaceID == "" {
		workspaceID = payloadWorkspaceID
	} else if payloadWorkspaceID != "" && payloadWorkspaceID != workspaceID {
		return "", common.ErrConflict
	}
	if workspaceID == "" {
		return "", common.ErrInvalidArgument
	}
	return workspaceID, nil
}

func (m *Module) recordRuntimeProgress(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	progress, ok := envelope.Payload[events.PayloadKeyRuntimeProgress].(runtimemodel.RuntimeStepExecutionProgressed)
	if !ok {
		return common.ErrInvalidArgument
	}
	if progress.RunID == "" {
		progress.RunID = common.RunID(envelope.RunID)
	}
	if progress.StepID == "" {
		progress.StepID = common.StepID(envelope.StepID)
	}
	if progress.WorkspaceID == "" {
		progress.WorkspaceID = common.WorkspaceID(envelope.WorkspaceID)
	}
	if progress.RunID == "" || progress.StepID == "" {
		return common.ErrInvalidArgument
	}
	return m.bizPtr.RecordRuntimeStepProgress(ctx, progress)
}

func runtimeResultFromEnvelope(envelope events.Envelope) (common.RunID, common.StepID, runtimemodel.StepExecutionResult, error, error) {
	result, _ := envelope.Payload[events.PayloadKeyRuntimeResult].(runtimemodel.StepExecutionResult)
	runID := common.RunID(envelope.RunID)
	stepID := common.StepID(envelope.StepID)
	switch envelope.Topic {
	case runtimeevent.TopicRuntimeBlockExecuteCompleted:
		completed, _ := envelope.Payload[events.PayloadKeyRuntimeCompleted].(runtimemodel.RuntimeStepExecutionCompleted)
		if runID == "" {
			runID = completed.RunID
		}
		if stepID == "" {
			stepID = completed.StepID
		}
		if result.Output == nil {
			result.Output = completed.Output
		}
	case runtimeevent.TopicRuntimeBlockExecuteFailed:
		failed, _ := envelope.Payload[events.PayloadKeyRuntimeFailed].(runtimemodel.RuntimeStepExecutionFailed)
		if runID == "" {
			runID = failed.RunID
		}
		if stepID == "" {
			stepID = failed.StepID
		}
		if result.Output == nil {
			result.Output = map[string]any{}
		}
		message := strings.TrimSpace(failed.Error)
		if message == "" {
			message, _ = envelope.Payload[events.PayloadKeyError].(string)
		}
		if message == "" {
			message = "runtime block execution failed"
		}
		if failed.FailureClass != "" {
			result.Output["failureClass"] = failed.FailureClass
		}
		if failed.FailureSignature != "" {
			result.Output["failureSignature"] = failed.FailureSignature
		}
		return runID, stepID, result, fmt.Errorf("%s", message), validateRuntimeResultScope(runID, stepID)
	default:
		return "", "", runtimemodel.StepExecutionResult{}, nil, common.ErrInvalidArgument
	}
	return runID, stepID, result, nil, validateRuntimeResultScope(runID, stepID)
}

func runtimeTerminalResultCheckpointFromEnvelope(envelope events.Envelope, result runtimemodel.StepExecutionResult, execErr error) orchestratorbiz.RuntimeStepTerminalResultCheckpoint {
	checkpoint := orchestratorbiz.RuntimeStepTerminalResultCheckpoint{
		TerminalTopic: envelope.Topic,
		Output:        cloneCommandPatch(result.Output),
	}
	switch envelope.Topic {
	case runtimeevent.TopicRuntimeBlockExecuteCompleted:
		completed, _ := envelope.Payload[events.PayloadKeyRuntimeCompleted].(runtimemodel.RuntimeStepExecutionCompleted)
		checkpoint.RequestID = completed.RequestID
		checkpoint.StepKind = completed.StepKind
		checkpoint.RecoveryPolicy = completed.RecoveryPolicy
		checkpoint.CompletedAt = completed.CompletedAt
		checkpoint.EvidenceRefs = append([]string(nil), completed.EvidenceRefs...)
		checkpoint.RawResultRef = completed.RawResultRef
		if checkpoint.Output == nil {
			checkpoint.Output = cloneCommandPatch(completed.Output)
		}
	case runtimeevent.TopicRuntimeBlockExecuteFailed:
		failed, _ := envelope.Payload[events.PayloadKeyRuntimeFailed].(runtimemodel.RuntimeStepExecutionFailed)
		checkpoint.RequestID = failed.RequestID
		checkpoint.StepKind = failed.StepKind
		checkpoint.RecoveryPolicy = failed.RecoveryPolicy
		checkpoint.CompletedAt = failed.CompletedAt
		checkpoint.FailureClass = failed.FailureClass
		checkpoint.FailureSignature = failed.FailureSignature
		checkpoint.EvidenceRefs = append([]string(nil), failed.EvidenceRefs...)
		checkpoint.RawResultRef = failed.RawResultRef
		checkpoint.Error = strings.TrimSpace(failed.Error)
	}
	if checkpoint.RequestID == "" {
		checkpoint.RequestID, _ = envelope.Payload[events.PayloadKeyRuntimeRequestID].(string)
	}
	if checkpoint.Error == "" && execErr != nil {
		checkpoint.Error = execErr.Error()
	}
	return checkpoint
}

func validateRuntimeResultScope(runID common.RunID, stepID common.StepID) error {
	if runID == "" || stepID == "" {
		return common.ErrInvalidArgument
	}
	return nil
}
