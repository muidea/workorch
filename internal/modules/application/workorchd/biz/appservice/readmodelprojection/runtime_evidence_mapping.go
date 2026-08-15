package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: patch_mapping.go

func mapPatches(items []*runtimeevidencemodel.Patch) []projectioncontract.Patch {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.Patch, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, projectPatch(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: tool_call_base_projection.go

func projectToolCallBase(item *runtimeevidencemodel.ToolCall) projectioncontract.ToolCall {
	return projectioncontract.ToolCall{
		ID:          string(item.ID),
		WorkspaceID: string(item.WorkspaceID),
		RunID:       string(item.RunID),
		StepID:      string(item.StepID),
		ToolName:    item.ToolName,
		State:       string(item.State),
		StartedAt:   item.StartedAt,
		EndedAt:     item.EndedAt,
		ExitSummary: item.ExitSummary,
		Error:       item.Error,
	}
}

// source: tool_call_mapping.go

func mapToolCalls(items []*runtimeevidencemodel.ToolCall) []projectioncontract.ToolCall {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.ToolCall, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		projected := projectToolCallBase(item)
		projected.Arguments = cloneReadModelMap(item.Arguments.Map())
		projected.Output = cloneReadModelMap(item.Output.Map())
		ret = append(ret, projected)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: tool_call_projection.go

func projectToolCall(item *runtimeevidencemodel.ToolCall) projectioncontract.ToolCall {
	if item == nil {
		return projectioncontract.ToolCall{}
	}
	return projectToolCallBase(item)
}

func projectToolCallView(item projectioncontract.ToolCall) projectioncontract.ToolCall {
	return projectioncontract.ToolCall{
		ID:          item.ID,
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		StepID:      item.StepID,
		ToolName:    item.ToolName,
		State:       item.State,
		StartedAt:   item.StartedAt,
		EndedAt:     item.EndedAt,
		ExitSummary: item.ExitSummary,
		Error:       item.Error,
	}
}

// source: validation_activity.go

func mapValidationResults(items []*runtimeevidencemodel.ValidationResult) []projectioncontract.ValidationResult {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.ValidationResult, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, projectValidationResult(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: validation_projection.go

func projectValidationResult(item *runtimeevidencemodel.ValidationResult) projectioncontract.ValidationResult {
	if item == nil {
		return projectioncontract.ValidationResult{}
	}
	return projectioncontract.ValidationResult{
		ID:              string(item.ID),
		WorkspaceID:     string(item.WorkspaceID),
		RunID:           string(item.RunID),
		StepID:          string(item.StepID),
		ValidationKind:  item.ValidationKind,
		TargetRef:       item.TargetRef,
		State:           string(item.State),
		Summary:         item.Summary,
		EvidenceRef:     item.EvidenceRef,
		EvidenceQuality: string(item.EvidenceQuality),
		EvidenceReason:  item.EvidenceReason,
		ZeroWork:        item.ZeroWork,
		EvidenceTags:    cloneReadModelStringSlice(item.EvidenceTags),
		StartedAt:       item.StartedAt,
		EndedAt:         item.EndedAt,
	}
}
