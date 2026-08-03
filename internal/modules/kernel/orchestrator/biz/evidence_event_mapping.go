package orchestrator

import (
	"slices"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func toolCallEventDTOs(items []*runmodel.ToolCall) []*orchestratorevents.ToolCall {
	if items == nil {
		return nil
	}
	ret := make([]*orchestratorevents.ToolCall, 0, len(items))
	for _, item := range items {
		if item == nil {
			ret = append(ret, nil)
			continue
		}
		ret = append(ret, &orchestratorevents.ToolCall{
			ID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, StepID: item.StepID,
			ToolName: item.ToolName, Arguments: item.Arguments.Clone(), State: item.State,
			StartedAt: item.StartedAt, EndedAt: item.EndedAt, ExitSummary: item.ExitSummary,
			Output: item.Output.Clone(), Error: item.Error,
		})
	}
	return ret
}

func patchEventDTOs(items []*runmodel.Patch) []*orchestratorevents.Patch {
	if items == nil {
		return nil
	}
	ret := make([]*orchestratorevents.Patch, 0, len(items))
	for _, item := range items {
		if item == nil {
			ret = append(ret, nil)
			continue
		}
		var coverage []orchestratorevents.PatchCoverageTarget
		if item.CoverageTargets != nil {
			coverage = make([]orchestratorevents.PatchCoverageTarget, len(item.CoverageTargets))
			for idx, target := range item.CoverageTargets {
				coverage[idx] = orchestratorevents.PatchCoverageTarget{GoalRequirementID: target.GoalRequirementID, TargetText: target.TargetText}
			}
		}
		ret = append(ret, &orchestratorevents.Patch{
			ID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, StepID: item.StepID,
			ToolCallID: item.ToolCallID, ApprovalID: item.ApprovalID, PatchType: item.PatchType,
			TargetRef: item.TargetRef, Content: item.Content, Diff: item.Diff, Summary: item.Summary,
			CoverageTargets: coverage, State: item.State, BeforeHash: item.BeforeHash, AfterHash: item.AfterHash,
			AffectedFiles: slices.Clone(item.AffectedFiles), AddedLines: item.AddedLines, DeletedLines: item.DeletedLines,
			RiskLevel: item.RiskLevel, RiskReasons: slices.Clone(item.RiskReasons),
			Diagnostics: cloneEvidencePayloads(item.Diagnostics), OriginalContent: item.OriginalContent,
			OriginalExists: item.OriginalExists, ReverseDiff: item.ReverseDiff, RevertApprovalID: item.RevertApprovalID,
			RevertedAt: item.RevertedAt, CreatedAt: item.CreatedAt, AppliedAt: item.AppliedAt,
		})
	}
	return ret
}

func validationResultEventDTOs(items []*runmodel.ValidationResult) []*orchestratorevents.ValidationResult {
	if items == nil {
		return nil
	}
	ret := make([]*orchestratorevents.ValidationResult, 0, len(items))
	for _, item := range items {
		if item == nil {
			ret = append(ret, nil)
			continue
		}
		ret = append(ret, &orchestratorevents.ValidationResult{
			ID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, StepID: item.StepID,
			ValidationKind: item.ValidationKind, TargetRef: item.TargetRef, State: item.State,
			Summary: item.Summary, EvidenceRef: item.EvidenceRef, EvidenceQuality: item.EvidenceQuality,
			EvidenceReason: item.EvidenceReason, ZeroWork: item.ZeroWork,
			EvidenceTags: slices.Clone(item.EvidenceTags), StartedAt: item.StartedAt, EndedAt: item.EndedAt,
		})
	}
	return ret
}

func cloneEvidencePayloads(items []workorcheventpayload.Payload) []workorcheventpayload.Payload {
	if items == nil {
		return nil
	}
	ret := make([]workorcheventpayload.Payload, len(items))
	for idx := range items {
		ret[idx] = items[idx].Clone()
	}
	return ret
}
