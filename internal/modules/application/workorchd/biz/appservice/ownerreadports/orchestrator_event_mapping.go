package ownerreadports

import (
	"slices"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func toolCallModelsFromOrchestrator(items []*orchestratorevents.ToolCall) []*runmodel.ToolCall {
	if items == nil {
		return nil
	}
	ret := make([]*runmodel.ToolCall, 0, len(items))
	for _, item := range items {
		if item == nil {
			ret = append(ret, nil)
			continue
		}
		ret = append(ret, &runmodel.ToolCall{
			ID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, StepID: item.StepID,
			ToolName: item.ToolName, Arguments: item.Arguments.Clone(), State: item.State,
			StartedAt: item.StartedAt, EndedAt: item.EndedAt, ExitSummary: item.ExitSummary,
			Output: item.Output.Clone(), Error: item.Error,
		})
	}
	return ret
}

func patchModelsFromOrchestrator(items []*orchestratorevents.Patch) []*runmodel.Patch {
	if items == nil {
		return nil
	}
	ret := make([]*runmodel.Patch, 0, len(items))
	for _, item := range items {
		if item == nil {
			ret = append(ret, nil)
			continue
		}
		var coverage []runmodel.PatchCoverageTarget
		if item.CoverageTargets != nil {
			coverage = make([]runmodel.PatchCoverageTarget, len(item.CoverageTargets))
			for idx, target := range item.CoverageTargets {
				coverage[idx] = runmodel.PatchCoverageTarget{GoalRequirementID: target.GoalRequirementID, TargetText: target.TargetText}
			}
		}
		ret = append(ret, &runmodel.Patch{
			ID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, StepID: item.StepID,
			ToolCallID: item.ToolCallID, ApprovalID: item.ApprovalID, PatchType: item.PatchType,
			TargetRef: item.TargetRef, Content: item.Content, Diff: item.Diff, Summary: item.Summary,
			CoverageTargets: coverage, State: item.State, BeforeHash: item.BeforeHash, AfterHash: item.AfterHash,
			AffectedFiles: slices.Clone(item.AffectedFiles), AddedLines: item.AddedLines, DeletedLines: item.DeletedLines,
			RiskLevel: item.RiskLevel, RiskReasons: slices.Clone(item.RiskReasons),
			Diagnostics: cloneOrchestratorPayloads(item.Diagnostics), OriginalContent: item.OriginalContent,
			OriginalExists: item.OriginalExists, ReverseDiff: item.ReverseDiff, RevertApprovalID: item.RevertApprovalID,
			RevertedAt: item.RevertedAt, CreatedAt: item.CreatedAt, AppliedAt: item.AppliedAt,
		})
	}
	return ret
}

func validationResultModelsFromOrchestrator(items []*orchestratorevents.ValidationResult) []*runmodel.ValidationResult {
	if items == nil {
		return nil
	}
	ret := make([]*runmodel.ValidationResult, 0, len(items))
	for _, item := range items {
		if item == nil {
			ret = append(ret, nil)
			continue
		}
		ret = append(ret, &runmodel.ValidationResult{
			ID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, StepID: item.StepID,
			ValidationKind: item.ValidationKind, TargetRef: item.TargetRef, State: item.State,
			Summary: item.Summary, EvidenceRef: item.EvidenceRef, EvidenceQuality: item.EvidenceQuality,
			EvidenceReason: item.EvidenceReason, ZeroWork: item.ZeroWork,
			EvidenceTags: slices.Clone(item.EvidenceTags), StartedAt: item.StartedAt, EndedAt: item.EndedAt,
		})
	}
	return ret
}

func cloneOrchestratorPayloads(items []workorcheventpayload.Payload) []workorcheventpayload.Payload {
	if items == nil {
		return nil
	}
	ret := make([]workorcheventpayload.Payload, len(items))
	for idx := range items {
		ret[idx] = items[idx].Clone()
	}
	return ret
}
