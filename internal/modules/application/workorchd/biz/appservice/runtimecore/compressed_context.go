package runtimecore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CompressedContextMap(input readmodelcore.CompressedContext) map[string]any {
	return map[string]any{
		"version":            input.Version,
		"mode":               input.Mode,
		"workspaceId":        input.WorkspaceID,
		"runId":              input.RunID,
		"summary":            input.Summary,
		"currentMilestone":   input.CurrentMilestone,
		"milestones":         progressMilestoneMaps(input.Milestones),
		"blockers":           progressBlockerMaps(input.Blockers),
		"recovery":           recoveryPlanMap(input.Recovery),
		"recentEvidence":     contextEvidenceMaps(input.RecentEvidence),
		"resultRefs":         stringAnySlice(input.ResultRefs),
		"sourceSectionRefs":  stringAnySlice(input.SourceSectionRefs),
		"sourceEvidenceRefs": stringAnySlice(input.SourceEvidenceRefs),
		"retentionPolicy":    input.RetentionPolicy,
	}
}

func progressMilestoneMaps(items []readmodelcore.ProgressMilestone) []any {
	ret := make([]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, map[string]any{
			"id":           item.ID,
			"title":        item.Title,
			"state":        item.State,
			"evidenceRefs": stringAnySlice(item.EvidenceRefs),
		})
	}
	return ret
}

func progressBlockerMaps(items []readmodelcore.ProgressBlocker) []any {
	ret := make([]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, map[string]any{
			"id":          item.ID,
			"kind":        item.Kind,
			"state":       item.State,
			"reason":      item.Reason,
			"evidenceRef": item.EvidenceRef,
		})
	}
	return ret
}

func recoveryPlanMap(item readmodelcore.RecoveryPlan) map[string]any {
	return map[string]any{
		"mode":               item.Mode,
		"nextAction":         item.NextAction,
		"runId":              item.RunID,
		"stepId":             item.StepID,
		"blockingTicketId":   item.BlockingTicketID,
		"blockingApprovalId": item.BlockingApprovalID,
		"checkpointId":       item.CheckpointID,
		"failureClass":       item.FailureClass,
		"failureSignature":   item.FailureSignature,
		"reason":             item.Reason,
	}
}

func contextEvidenceMaps(items []readmodelcore.ContextEvidence) []any {
	ret := make([]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, map[string]any{
			"id":           item.ID,
			"kind":         item.Kind,
			"state":        item.State,
			"summary":      item.Summary,
			"occurredAt":   item.OccurredAt,
			"evidenceRefs": stringAnySlice(item.EvidenceRefs),
			"data":         cloneRuntimeContextMap(item.Data),
		})
	}
	return ret
}

func stringAnySlice(values []string) []any {
	ret := make([]any, 0, len(values))
	for _, value := range values {
		ret = append(ret, value)
	}
	return ret
}

func cloneRuntimeContextMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func RunDialogLanguage(item *runmodel.Run, fallback string) string {
	if item == nil || item.ExecutionContext == nil {
		return fallback
	}
	if lang, ok := item.ExecutionContext["dialogLanguage"].(string); ok {
		if normalized := localizationcore.LanguageNameFromLocale(lang); normalized != "" {
			return normalized
		}
	}
	return fallback
}

func CreatedRunCompressedContext(item *runmodel.Run, language string) map[string]any {
	if item == nil || item.State != common.RunStatePending || item.ID == "" || item.WorkspaceID == "" {
		return nil
	}
	return CompressedContextMap(readmodelcore.CompressedContext{
		Version:          "workorch.compressedContext.v1",
		Mode:             "deterministic_recovery_summary",
		WorkspaceID:      string(item.WorkspaceID),
		RunID:            string(item.ID),
		Summary:          localizationcore.CreatedRunSummary(language),
		CurrentMilestone: "execution_steps",
		Milestones: []readmodelcore.ProgressMilestone{
			{ID: "run_created", Title: localizationcore.MilestoneTitle(language, "run_created"), State: "completed", EvidenceRefs: []string{string(item.ID)}},
			{ID: "execution_steps", Title: localizationcore.MilestoneTitle(language, "execution_steps"), State: "pending"},
			{ID: "governance", Title: localizationcore.MilestoneTitle(language, "governance"), State: "pending"},
			{ID: "validation", Title: localizationcore.MilestoneTitle(language, "validation"), State: "pending"},
			{ID: "formal_result", Title: localizationcore.MilestoneTitle(language, "formal_result"), State: "pending"},
		},
		Recovery: readmodelcore.RecoveryPlan{
			Mode:       "resume",
			NextAction: localizationcore.RecoveryNextAction(language),
			RunID:      string(item.ID),
		},
		SourceSectionRefs:  []string{"workspace", "run", "task_progress", "recovery"},
		SourceEvidenceRefs: []string{string(item.ID)},
		RetentionPolicy:    localizationcore.RetentionPolicy(language),
	})
}
