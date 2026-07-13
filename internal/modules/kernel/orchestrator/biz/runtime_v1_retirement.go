package orchestrator

import (
	"context"
	"strings"
	"time"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeV1ResourceReleaseResult struct {
	ReleasedValidationResources int
	ClosedSteps                 int
	ClosedToolCalls             int
	ClosedArtifactContinuations int
}

func (r RuntimeV1ResourceReleaseResult) Total() int {
	return r.ReleasedValidationResources + r.ClosedSteps + r.ClosedToolCalls + r.ClosedArtifactContinuations
}

func (o *RuntimeOrchestrator) ReleaseRuntimeV1Resources(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeV1ResourceReleaseResult, error) {
	if o == nil || workspaceID == "" || runID == "" || o.steps == nil {
		return RuntimeV1ResourceReleaseResult{}, common.ErrInvalidArgument
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	run, err := o.runtimeEventRun(ctx, runID)
	if err != nil {
		return RuntimeV1ResourceReleaseResult{}, err
	}
	if run.WorkspaceID != workspaceID {
		return RuntimeV1ResourceReleaseResult{}, common.ErrNotFound
	}
	if run.RuntimeState != nil || run.RuntimeBootstrap != nil || strings.TrimSpace(run.RuntimeContractVersion) == runtimecontract.RuntimeContractVersionV2 {
		return RuntimeV1ResourceReleaseResult{}, common.ErrConflict
	}
	result := RuntimeV1ResourceReleaseResult{}
	steps, err := o.steps.ListByRun(ctx, runID)
	if err != nil {
		return result, err
	}
	for _, step := range steps {
		if step == nil {
			continue
		}
		result.ReleasedValidationResources += countOpenManagedValidationResources(step)
		changed := finalizeManagedValidationResourcesForStep(step, "released", "runtime_v1_retirement")
		if step.State == common.StepStatePending || step.State == common.StepStateExecuting {
			step.State = common.StepStateSkipped
			if step.Output == nil {
				step.Output = map[string]any{}
			}
			step.Output["status"] = "skipped"
			step.Output["reason"] = "runtime_contract_v1_retired"
			result.ClosedSteps++
			changed = true
		}
		if changed {
			if err := o.steps.Update(ctx, step); err != nil {
				return result, err
			}
		}
	}
	if o.evidence.ToolCalls != nil {
		items, err := o.evidence.ToolCalls.ListByRun(ctx, runID)
		if err != nil {
			return result, err
		}
		for _, item := range items {
			if item == nil || (item.State != common.ToolCallStatePending && item.State != common.ToolCallStateRunning) {
				continue
			}
			item.State = common.ToolCallStateFailed
			item.Error = "runtime_contract_v1_retired"
			item.ExitSummary = "Runtime V1 retirement closed pending tool call"
			if item.EndedAt == "" {
				item.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
			}
			if err := o.evidence.ToolCalls.Update(ctx, item); err != nil {
				return result, err
			}
			result.ClosedToolCalls++
		}
	}
	if o.evidence.ArtifactContinuations != nil {
		items, err := o.evidence.ArtifactContinuations.ListOpenByRun(ctx, runID)
		if err != nil {
			return result, err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			item.State = common.ArtifactContinuationStateFailed
			item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if item.Metadata == nil {
				item.Metadata = map[string]any{}
			}
			item.Metadata["failureReason"] = "runtime_contract_v1_retired"
			if err := o.evidence.ArtifactContinuations.Update(ctx, item); err != nil {
				return result, err
			}
			result.ClosedArtifactContinuations++
		}
	}
	return result, nil
}

func countOpenManagedValidationResources(step *runmodel.Step) int {
	if step == nil {
		return 0
	}
	count := 0
	for _, resource := range mapSliceFromValue(step.Input["validationResources"]) {
		if !managedValidationResourceStatusTerminal(stringFromMap(resource, "status")) {
			count++
		}
	}
	return count
}
