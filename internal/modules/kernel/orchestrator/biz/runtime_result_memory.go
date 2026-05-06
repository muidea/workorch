package orchestrator

import (
	"context"
	"fmt"
	"strings"

	memorycommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func completionRunSummaryMemoryContent(
	run *runModel.Run,
	summary string,
	assessment map[string]any,
	evidenceRefs []string,
	patchDetails []resultPatchDetail,
	evidenceDetails []resultEvidenceDetail,
	webSourceDetails []resultWebSourceDetail,
) string {
	parts := []string{summary}
	if run != nil {
		parts = append(parts, fmt.Sprintf("Run: %s", run.ID))
		if run.GoalID != "" {
			parts = append(parts, fmt.Sprintf("Goal: %s", run.GoalID))
		}
		if run.DoDID != "" {
			parts = append(parts, fmt.Sprintf("DoD: %s", run.DoDID))
		}
	}
	for _, key := range []string{"goalOutcome", "implementationSummary", "verificationSummary"} {
		value := strings.TrimSpace(stringFromMap(assessment, key))
		if value != "" && value != summary {
			parts = append(parts, fmt.Sprintf("%s: %s", key, value))
		}
	}
	if items, ok := doDAssessmentItemMaps(assessment["items"]); ok && len(items) > 0 {
		lines := []string{"DoD items:"}
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("- [%s] %s", firstNonEmptyOrchestratorString(stringFromMap(item, "state"), "unknown"), stringFromMap(item, "item")))
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	if len(evidenceRefs) > 0 {
		parts = append(parts, "Evidence refs: "+strings.Join(evidenceRefs, ", "))
	}
	if len(evidenceDetails) > 0 {
		lines := []string{"Verification commands:"}
		for _, detail := range evidenceDetails {
			if detail.Command == "" && detail.ExitCode == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s exitCode=%s", firstNonEmptyOrchestratorString(detail.Command, detail.Kind, detail.Ref), firstNonEmptyOrchestratorString(detail.ExitCode, "unknown")))
		}
		if len(lines) > 1 {
			parts = append(parts, strings.Join(lines, "\n"))
		}
	}
	if len(patchDetails) > 0 {
		lines := []string{"Patch targets:"}
		for _, patch := range patchDetails {
			lines = append(lines, fmt.Sprintf("- %s %s %s", patch.PatchType, patch.TargetRef, patch.State))
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	if len(webSourceDetails) > 0 {
		lines := []string{"Web sources:"}
		for _, source := range webSourceDetails {
			sourceURL := firstNonEmptyOrchestratorString(source.FinalURL, source.URL)
			if sourceURL == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf(
				"- %s evidence=%s hash=%s",
				sourceURL,
				source.StepID,
				firstNonEmptyOrchestratorString(source.ContentHash, "unknown"),
			))
		}
		if len(lines) > 1 {
			parts = append(parts, strings.Join(lines, "\n"))
		}
	}
	return strings.Join(parts, "\n\n")
}

func (o *RuntimeOrchestrator) recordSkillFeedbackMemory(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	output map[string]any,
) error {
	if run == nil || step == nil || !requiresDoDAssessmentCompletionGate(run) {
		return nil
	}
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return nil
	}
	summary, _ := assessment["summary"].(string)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	return o.recordSkillFeedbackMemoryFromValidatedDoDAssessment(ctx, run, step, assessment, summary)
}

func (o *RuntimeOrchestrator) recordSkillFeedbackMemoryFromValidatedDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	assessment map[string]any,
	summary string,
) error {
	if run == nil || step == nil || o.evidence.MemoryItems == nil || !requiresDoDAssessmentCompletionGate(run) || assessment == nil {
		return nil
	}
	skillIDs := stringSliceFromValue(run.ExecutionContext["skillIds"])
	if len(skillIDs) == 0 {
		return nil
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	item := &memorymodel.MemoryItem{
		ID:          common.MemoryID(fmt.Sprintf("%s/memory/skill-feedback", run.ID)),
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		SourceType:  memorycommon.MemorySourceRun,
		SourceRef:   string(step.ID),
		MemoryType:  memorycommon.MemoryTypeSkillFeedback,
		Content: fmt.Sprintf(
			"Run %s completed with selected skills: %s.\nDoD summary: %s",
			run.ID,
			strings.Join(skillIDs, ", "),
			summary,
		),
		Summary:    fmt.Sprintf("Selected skills completed run: %s", strings.Join(skillIDs, ", ")),
		Tags:       []string{"skill_feedback", "run_completed"},
		Importance: 50,
		Confidence: 50,
		Visibility: "workspace",
	}
	if err := o.evidence.MemoryItems.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return err
		}
		existing, getErr := o.evidence.MemoryItems.Get(ctx, item.ID)
		if getErr != nil {
			return getErr
		}
		existing.WorkspaceID = item.WorkspaceID
		existing.RunID = item.RunID
		existing.SourceType = item.SourceType
		existing.SourceRef = item.SourceRef
		existing.MemoryType = item.MemoryType
		existing.Content = item.Content
		existing.Summary = item.Summary
		existing.Tags = item.Tags
		existing.Importance = item.Importance
		existing.Confidence = item.Confidence
		existing.Visibility = item.Visibility
		if err := o.evidence.MemoryItems.Update(ctx, existing); err != nil {
			return err
		}
		return o.publishMemoryChanged(ctx, events.TopicMemoryUpdated, existing)
	}
	return o.publishMemoryChanged(ctx, events.TopicMemoryCreated, item)
}

func (o *RuntimeOrchestrator) recordRunSummaryMemoryFromValidatedDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	assessment map[string]any,
	summary string,
) error {
	if run == nil || step == nil || o.evidence.MemoryItems == nil || !requiresDoDAssessmentCompletionGate(run) || assessment == nil {
		return nil
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	evidenceRefs := doDAssessmentEvidenceRefs(step.ID, assessment)
	item := &memorymodel.MemoryItem{
		ID:          common.MemoryID(fmt.Sprintf("%s/memory/run-summary", run.ID)),
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		SourceType:  memorycommon.MemorySourceRun,
		SourceRef:   string(step.ID),
		MemoryType:  memorycommon.MemoryTypeRunSummary,
		Content:     completionRunSummaryMemoryContent(run, summary, assessment, evidenceRefs, o.resultPatchDetails(ctx, run.ID), o.resultEvidenceDetails(ctx, evidenceRefs), o.resultWebSourceDetails(ctx, run.ID)),
		Summary:     summary,
		Tags:        []string{"run_summary", "dod_assessment", "code_agent"},
		Importance:  70,
		Confidence:  70,
		Visibility:  "workspace",
	}
	if err := o.evidence.MemoryItems.Create(ctx, item); err != nil {
		if err != common.ErrConflict {
			return err
		}
		existing, getErr := o.evidence.MemoryItems.Get(ctx, item.ID)
		if getErr != nil {
			return getErr
		}
		existing.WorkspaceID = item.WorkspaceID
		existing.RunID = item.RunID
		existing.SourceType = item.SourceType
		existing.SourceRef = item.SourceRef
		existing.MemoryType = item.MemoryType
		existing.Content = item.Content
		existing.Summary = item.Summary
		existing.Tags = item.Tags
		existing.Importance = item.Importance
		existing.Confidence = item.Confidence
		existing.Visibility = item.Visibility
		if err := o.evidence.MemoryItems.Update(ctx, existing); err != nil {
			return err
		}
		return o.publishMemoryChanged(ctx, events.TopicMemoryUpdated, existing)
	}
	return o.publishMemoryChanged(ctx, events.TopicMemoryCreated, item)
}
