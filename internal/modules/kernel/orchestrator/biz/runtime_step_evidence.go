package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) recordToolCallStarted(ctx context.Context, run *runModel.Run, step *runModel.Step) (*runModel.ToolCall, error) {
	if run == nil || step == nil || o.evidence.ToolCalls == nil {
		return &runModel.ToolCall{}, nil
	}
	now := runtimeEvidenceTimestamp()
	toolCall := &runModel.ToolCall{
		ID:          common.ToolCallID(fmt.Sprintf("%s/tool-call/runtime-execute", step.ID)),
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		StepID:      step.ID,
		ToolName:    step.Kind,
		Arguments:   runtimeToolCallArguments(o.llmContextManager(), run, step),
		State:       common.ToolCallStateRunning,
		StartedAt:   now,
	}
	if toolCall.ToolName == "" {
		toolCall.ToolName = "workorch/runtime-step"
	}
	if err := o.evidence.ToolCalls.Create(ctx, toolCall); err != nil {
		if err != common.ErrConflict {
			return nil, err
		}
		existing, getErr := o.evidence.ToolCalls.Get(ctx, toolCall.ID)
		if getErr != nil {
			return nil, getErr
		}
		existing.WorkspaceID = toolCall.WorkspaceID
		existing.RunID = toolCall.RunID
		existing.StepID = toolCall.StepID
		existing.ToolName = toolCall.ToolName
		existing.Arguments = toolCall.Arguments
		existing.State = common.ToolCallStateRunning
		existing.StartedAt = now
		existing.EndedAt = ""
		existing.ExitSummary = ""
		existing.Output = nil
		existing.Error = ""
		if err := o.evidence.ToolCalls.Update(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}
	return toolCall, nil
}

func (o *RuntimeOrchestrator) recordStepDispatchEvidence(ctx context.Context, run *runModel.Run, step *runModel.Step) (*runModel.ToolCall, error) {
	var toolCall *runModel.ToolCall
	var toolCallMu sync.Mutex
	err := runParallel(
		func() error {
			item, err := o.recordToolCallStarted(ctx, run, step)
			toolCallMu.Lock()
			toolCall = item
			toolCallMu.Unlock()
			return err
		},
		func() error {
			return o.recordStepCheckpoint(ctx, run, step, "step_dispatched")
		},
	)
	if err != nil {
		return nil, err
	}
	if toolCall == nil {
		return &runModel.ToolCall{}, nil
	}
	return toolCall, nil
}

func (o *RuntimeOrchestrator) recordToolCallCompleted(ctx context.Context, toolCall *runModel.ToolCall, output map[string]any, dialogLanguage string) error {
	if toolCall == nil || toolCall.ID == "" || o.evidence.ToolCalls == nil {
		return nil
	}
	toolCall.State = common.ToolCallStateCompleted
	toolCall.EndedAt = runtimeEvidenceTimestamp()
	toolCall.ExitSummary = localizedStepCompletionSummary(dialogLanguage)
	toolCall.Output = runtimeToolCallOutput(o.llmContextManager(), toolCall, output)
	toolCall.Error = ""
	if err := o.evidence.ToolCalls.Update(ctx, toolCall); err != nil {
		if err == common.ErrNotFound {
			return o.evidence.ToolCalls.Create(ctx, toolCall)
		}
		return err
	}
	return nil
}

func (o *RuntimeOrchestrator) recordCompletedRuntimeStepEvidence(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, output map[string]any) error {
	evidenceRef := ""
	if toolCall != nil {
		evidenceRef = string(toolCall.ID)
	}
	language := runDialogLanguage(run)
	return runParallel(
		func() error {
			return o.recordToolCallCompleted(ctx, toolCall, output, language)
		},
		func() error {
			return o.recordValidationResult(ctx, run, step, common.ValidationResultStatePassed, localizedStepCompletionSummary(language), evidenceRef, "")
		},
	)
}

func (o *RuntimeOrchestrator) persistCompletedStepState(ctx context.Context, run *runModel.Run, step *runModel.Step) error {
	return runParallel(
		func() error {
			return o.steps.Update(ctx, step)
		},
		func() error {
			return o.recordStepCheckpoint(ctx, run, step, "step_completed")
		},
	)
}

func (o *RuntimeOrchestrator) recordToolCallFailed(ctx context.Context, toolCall *runModel.ToolCall, execErr error, output map[string]any) error {
	if toolCall == nil || toolCall.ID == "" || o.evidence.ToolCalls == nil {
		return nil
	}
	toolCall.State = common.ToolCallStateFailed
	toolCall.EndedAt = runtimeEvidenceTimestamp()
	toolCall.ExitSummary = "runtime step failed"
	toolCall.Output = runtimeToolCallOutput(o.llmContextManager(), toolCall, output)
	if execErr != nil {
		toolCall.Error = execErr.Error()
	}
	if err := o.evidence.ToolCalls.Update(ctx, toolCall); err != nil {
		if err == common.ErrNotFound {
			return o.evidence.ToolCalls.Create(ctx, toolCall)
		}
		return err
	}
	return nil
}

func runtimeToolCallArguments(llmContext *llmContextSubsystem, run *runModel.Run, step *runModel.Step) map[string]any {
	if run == nil || step == nil {
		return nil
	}
	if llmContext == nil {
		llmContext = newLLMContextSubsystem()
	}
	ret := map[string]any{
		"workspaceId": string(run.WorkspaceID),
		"runId":       string(run.ID),
		"stepId":      string(step.ID),
		"stepKind":    step.Kind,
		"inputRef":    runtimeRawPayloadRef(run.WorkspaceID, run.ID, "steps", string(step.ID), "input.json"),
	}
	if summary := llmContext.compactStepInput(step.Input); len(summary) > 0 {
		ret["inputSummary"] = summary
	}
	return ret
}

func runtimeToolCallOutput(llmContext *llmContextSubsystem, toolCall *runModel.ToolCall, output map[string]any) map[string]any {
	if llmContext == nil {
		llmContext = newLLMContextSubsystem()
	}
	if toolCall == nil {
		return llmContext.compactStepOutput(output)
	}
	ret := map[string]any{
		"workspaceId": string(toolCall.WorkspaceID),
		"runId":       string(toolCall.RunID),
		"stepId":      string(toolCall.StepID),
		"toolCallId":  string(toolCall.ID),
		"outputRef":   runtimeRawPayloadRef(toolCall.WorkspaceID, toolCall.RunID, "steps", string(toolCall.StepID), "output.json"),
	}
	if summary := llmContext.compactStepOutput(output); len(summary) > 0 {
		for key, value := range summary {
			ret[key] = value
		}
	}
	return ret
}

func runtimeRawPayloadRef(workspaceID common.WorkspaceID, runID common.RunID, collection string, itemPath string, fileName string) string {
	return fmt.Sprintf(
		"workorch://workspaces/%s/runs/%s/%s/%s/%s",
		workspaceID,
		runID,
		collection,
		strings.Trim(strings.TrimSpace(itemPath), "/"),
		strings.TrimSpace(fileName),
	)
}

func (o *RuntimeOrchestrator) recordValidationResult(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	state common.ValidationResultState,
	summary string,
	evidenceRef string,
	errText string,
) error {
	if run == nil || step == nil || o.evidence.ValidationResults == nil {
		return nil
	}
	now := runtimeEvidenceTimestamp()
	quality := runtimeStepEvidenceQuality(run, step, state, step.Output)
	item := &runModel.ValidationResult{
		ID:              common.ValidationResultID(fmt.Sprintf("%s/validation/runtime-step", step.ID)),
		WorkspaceID:     run.WorkspaceID,
		RunID:           run.ID,
		StepID:          step.ID,
		ValidationKind:  "runtime_step",
		TargetRef:       string(step.ID),
		State:           state,
		Summary:         summary,
		EvidenceRef:     evidenceRef,
		EvidenceQuality: quality.Quality,
		EvidenceReason:  quality.Reason,
		ZeroWork:        quality.ZeroWork,
		EvidenceTags:    quality.Tags,
		StartedAt:       now,
		EndedAt:         now,
	}
	if errText != "" {
		item.Summary = fmt.Sprintf("%s: %s", summary, errText)
	} else if quality.Quality != "" && quality.Quality != common.EvidenceQualityValid && quality.Reason != "" {
		item.Summary = fmt.Sprintf("%s; evidence %s: %s", summary, quality.Quality, quality.Reason)
	}
	if err := o.evidence.ValidationResults.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return err
		}
		return o.evidence.ValidationResults.Update(ctx, item)
	}
	return nil
}
