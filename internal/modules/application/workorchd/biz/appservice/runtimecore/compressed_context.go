package runtimecore

import (
	"encoding/json"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CompressedContextMap(input readmodelcore.CompressedContext) map[string]any {
	payload, err := json.Marshal(input)
	if err != nil {
		return nil
	}
	var ret map[string]any
	if err := json.Unmarshal(payload, &ret); err != nil {
		return nil
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
	if item == nil || item.State != common.RunStateCreated || item.ID == "" || item.WorkspaceID == "" {
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
			Mode:       "continue",
			NextAction: localizationcore.RecoveryNextAction(language),
			RunID:      string(item.ID),
		},
		SourceSectionRefs:  []string{"workspace", "run", "task_progress", "recovery"},
		SourceEvidenceRefs: []string{string(item.ID)},
		RetentionPolicy:    localizationcore.RetentionPolicy(language),
	})
}
