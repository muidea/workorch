package runtimecore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func CompressedContextMap(input projectioncontract.CompressedContext) map[string]any {
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

func progressMilestoneMaps(items []projectioncontract.ProgressMilestone) []any {
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

func progressBlockerMaps(items []projectioncontract.ProgressBlocker) []any {
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

func recoveryPlanMap(item projectioncontract.RecoveryPlan) map[string]any {
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

func contextEvidenceMaps(items []projectioncontract.ContextEvidence) []any {
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
	if item == nil || item.RuntimeBootstrap == nil {
		return fallback
	}
	if normalized := localizationcore.LanguageNameFromLocale(item.RuntimeBootstrap.Activation.DialogLanguage); normalized != "" {
		return normalized
	}
	return fallback
}
