package orchestrator

import (
	"context"
	"fmt"
	"strings"

	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/runtimeevent"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Module) finalizeRuntimeResult(ctx context.Context, topic string, data any) error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	workspaceID, err := runtimeResultWorkspaceID(topic, data)
	if err != nil {
		return err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	runID, stepID, result, execErr, err := runtimeResult(topic, data)
	if err != nil {
		return err
	}
	if err := m.bizPtr.RecordRuntimeStepTerminalResult(ctx, runID, stepID, runtimeTerminalResultCheckpoint(topic, data, result, execErr)); err != nil {
		return err
	}
	return m.bizPtr.FinalizeRuntimeStepResult(ctx, runID, stepID, result, execErr)
}

func runtimeResultWorkspaceID(topic string, data any) (common.WorkspaceID, error) {
	switch topic {
	case runtimeevent.TopicRuntimeBlockExecuteCompleted:
		completed, ok := data.(runtimeevent.CompletedData)
		if !ok || completed.Completed.WorkspaceID == "" {
			return "", common.ErrInvalidArgument
		}
		return completed.Completed.WorkspaceID, nil
	case runtimeevent.TopicRuntimeBlockExecuteFailed:
		failed, ok := data.(runtimeevent.FailedData)
		if !ok || failed.Failed.WorkspaceID == "" {
			return "", common.ErrInvalidArgument
		}
		return failed.Failed.WorkspaceID, nil
	default:
		return "", common.ErrInvalidArgument
	}
}

func (m *Module) recordRuntimeProgress(ctx context.Context, data runtimeevent.ProgressData) error {
	if m == nil || m.bizPtr == nil {
		return common.ErrInvalidArgument
	}
	progress := data.Progress
	if progress.WorkspaceID == "" || progress.RunID == "" || progress.StepID == "" {
		return common.ErrInvalidArgument
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, progress.WorkspaceID)
	return m.bizPtr.RecordRuntimeStepProgress(ctx, progress)
}

func runtimeResult(topic string, data any) (common.RunID, common.StepID, runtimemodel.StepExecutionResult, error, error) {
	switch topic {
	case runtimeevent.TopicRuntimeBlockExecuteCompleted:
		completed, ok := data.(runtimeevent.CompletedData)
		if !ok {
			return "", "", runtimemodel.StepExecutionResult{}, nil, common.ErrInvalidArgument
		}
		result := completed.Result
		if result.Output == nil {
			result.Output = completed.Completed.Output
		}
		return completed.Completed.RunID, completed.Completed.StepID, result, nil, validateRuntimeResultScope(completed.Completed.RunID, completed.Completed.StepID)
	case runtimeevent.TopicRuntimeBlockExecuteFailed:
		failed, ok := data.(runtimeevent.FailedData)
		if !ok {
			return "", "", runtimemodel.StepExecutionResult{}, nil, common.ErrInvalidArgument
		}
		result := failed.Result
		if result.Output == nil {
			result.Output = map[string]any{}
		}
		message := strings.TrimSpace(failed.Error)
		if message == "" {
			message = strings.TrimSpace(failed.Failed.Error)
		}
		if message == "" {
			message = "runtime block execution failed"
		}
		if failed.Failed.FailureClass != "" {
			result.Output["failureClass"] = failed.Failed.FailureClass
		}
		if failed.Failed.FailureSignature != "" {
			result.Output["failureSignature"] = failed.Failed.FailureSignature
		}
		return failed.Failed.RunID, failed.Failed.StepID, result, fmt.Errorf("%s", message), validateRuntimeResultScope(failed.Failed.RunID, failed.Failed.StepID)
	default:
		return "", "", runtimemodel.StepExecutionResult{}, nil, common.ErrInvalidArgument
	}
}

func runtimeTerminalResultCheckpoint(topic string, data any, result runtimemodel.StepExecutionResult, execErr error) orchestratorbiz.RuntimeStepTerminalResultCheckpoint {
	checkpoint := orchestratorbiz.RuntimeStepTerminalResultCheckpoint{
		TerminalTopic: topic,
		Output:        cloneCommandPatch(result.Output),
	}
	switch topic {
	case runtimeevent.TopicRuntimeBlockExecuteCompleted:
		completed, _ := data.(runtimeevent.CompletedData)
		checkpoint.RequestID = completed.Completed.RequestID
		checkpoint.StepKind = completed.Completed.StepKind
		checkpoint.RecoveryPolicy = completed.Completed.RecoveryPolicy
		checkpoint.CompletedAt = completed.Completed.CompletedAt
		checkpoint.EvidenceRefs = append([]string(nil), completed.Completed.EvidenceRefs...)
		checkpoint.RawResultRef = completed.Completed.RawResultRef
		if checkpoint.Output == nil {
			checkpoint.Output = cloneCommandPatch(completed.Completed.Output)
		}
	case runtimeevent.TopicRuntimeBlockExecuteFailed:
		failed, _ := data.(runtimeevent.FailedData)
		checkpoint.RequestID = failed.Failed.RequestID
		checkpoint.StepKind = failed.Failed.StepKind
		checkpoint.RecoveryPolicy = failed.Failed.RecoveryPolicy
		checkpoint.CompletedAt = failed.Failed.CompletedAt
		checkpoint.FailureClass = failed.Failed.FailureClass
		checkpoint.FailureSignature = failed.Failed.FailureSignature
		checkpoint.EvidenceRefs = append([]string(nil), failed.Failed.EvidenceRefs...)
		checkpoint.RawResultRef = failed.Failed.RawResultRef
		checkpoint.Error = strings.TrimSpace(failed.Failed.Error)
		if checkpoint.Error == "" {
			checkpoint.Error = strings.TrimSpace(failed.Error)
		}
	}
	if checkpoint.Error == "" && execErr != nil {
		checkpoint.Error = execErr.Error()
	}
	return checkpoint
}

func runtimeEventBackgroundContext(ctx context.Context, data any) context.Context {
	bgCtx := context.Background()
	workspaceID, _ := runtimeEventWorkspaceID(data)
	if workspaceID != "" {
		bgCtx = common.ContextWithRuntimeWorkspaceID(bgCtx, workspaceID)
	} else if workspaceID := common.RuntimeWorkspaceIDFromContext(ctx); workspaceID != "" {
		bgCtx = common.ContextWithRuntimeWorkspaceID(bgCtx, workspaceID)
	}
	if providerID := common.RuntimeLLMProviderIDFromContext(ctx); providerID != "" {
		bgCtx = common.ContextWithRuntimeLLMProviderID(bgCtx, providerID)
	}
	return bgCtx
}

func runtimeEventWorkspaceID(data any) (common.WorkspaceID, bool) {
	switch value := data.(type) {
	case runtimeevent.ProgressData:
		return value.Progress.WorkspaceID, true
	case runtimeevent.CompletedData:
		return value.Completed.WorkspaceID, true
	case runtimeevent.FailedData:
		return value.Failed.WorkspaceID, true
	default:
		return "", false
	}
}

func validateRuntimeResultScope(runID common.RunID, stepID common.StepID) error {
	if runID == "" || stepID == "" {
		return common.ErrInvalidArgument
	}
	return nil
}
