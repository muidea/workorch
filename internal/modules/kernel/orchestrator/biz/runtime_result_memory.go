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

const runSummaryDoDGateVersionTag = "dod_gate_version:v2"

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
		lines := []string{"Evidence executions:"}
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
		lines := []string{"State-change targets:"}
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
		return o.eventPublisher().publishMemoryChanged(ctx, events.TopicMemoryUpdated, existing)
	}
	return o.eventPublisher().publishMemoryChanged(ctx, events.TopicMemoryCreated, item)
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
		Content:     completionRunSummaryMemoryContent(run, summary, assessment, evidenceRefs, o.resultManager().resultPatchDetails(ctx, run.ID), o.resultManager().resultEvidenceDetails(ctx, evidenceRefs), o.resultManager().resultWebSourceDetails(ctx, run.ID)),
		Summary:     summary,
		Tags:        []string{"run_summary", "dod_assessment", "runtime_result", runSummaryDoDGateVersionTag},
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
		return o.eventPublisher().publishMemoryChanged(ctx, events.TopicMemoryUpdated, existing)
	}
	return o.eventPublisher().publishMemoryChanged(ctx, events.TopicMemoryCreated, item)
}

func (o *RuntimeOrchestrator) recordValidationCommandMemoriesFromValidatedDoDAssessment(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	assessment map[string]any,
) error {
	if run == nil || step == nil || o.evidence.MemoryItems == nil || !requiresDoDAssessmentCompletionGate(run) || assessment == nil {
		return nil
	}
	evidenceRefs := doDAssessmentEvidenceRefs(step.ID, assessment)
	evidenceDetails := o.resultManager().resultEvidenceDetails(ctx, evidenceRefs)
	commands := validationCommandMemoryEntries(evidenceDetails)
	if len(commands) == 0 {
		return nil
	}
	if err := o.upsertRunMemoryItem(ctx, &memorymodel.MemoryItem{
		ID:          common.MemoryID(fmt.Sprintf("%s/memory/validation-commands", run.ID)),
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		SourceType:  memorycommon.MemorySourceRun,
		SourceRef:   string(step.ID),
		MemoryType:  memorycommon.MemoryTypeProjectContext,
		Summary:     "Validation command baseline captured",
		Content:     validationCommandMemoryContent(run, commands),
		Tags:        []string{"validation_command", "verification", "provenance"},
		Importance:  65,
		Confidence:  70,
		Visibility:  "workspace",
	}); err != nil {
		return err
	}
	lessons := validationFailureMemoryEntries(commands)
	if len(lessons) == 0 {
		return nil
	}
	return o.upsertRunMemoryItem(ctx, &memorymodel.MemoryItem{
		ID:          common.MemoryID(fmt.Sprintf("%s/memory/validation-failure-lessons", run.ID)),
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		SourceType:  memorycommon.MemorySourceRun,
		SourceRef:   string(step.ID),
		MemoryType:  memorycommon.MemoryTypeExecutionLesson,
		Summary:     "Validation failure signals captured for repair",
		Content:     validationFailureLessonContent(run, lessons),
		Tags:        []string{"validation_failure", "repair_loop", "provenance"},
		Importance:  70,
		Confidence:  70,
		Visibility:  "workspace",
	})
}

type validationCommandEntry struct {
	Command  string
	ExitCode string
	Ref      string
	Stdout   string
	Stderr   string
	Summary  string
}

func validationCommandMemoryEntries(details []resultEvidenceDetail) []validationCommandEntry {
	if len(details) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	ret := make([]validationCommandEntry, 0, len(details))
	for _, detail := range details {
		command := strings.TrimSpace(detail.Command)
		if command == "" {
			continue
		}
		ref := strings.TrimSpace(detail.Ref)
		exitCode := strings.TrimSpace(detail.ExitCode)
		key := command + "|" + exitCode + "|" + ref
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ret = append(ret, validationCommandEntry{
			Command:  command,
			ExitCode: firstNonEmptyOrchestratorString(exitCode, "unknown"),
			Ref:      ref,
			Stdout:   strings.TrimSpace(detail.Stdout),
			Stderr:   strings.TrimSpace(detail.Stderr),
			Summary:  strings.TrimSpace(detail.Summary),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func validationCommandMemoryContent(run *runModel.Run, items []validationCommandEntry) string {
	var builder strings.Builder
	builder.WriteString("Validation commands with provenance evidence:\n")
	for _, item := range items {
		builder.WriteString(fmt.Sprintf("- %s | exitCode=%s | evidence=%s", item.Command, item.ExitCode, firstNonEmptyOrchestratorString(item.Ref, "n/a")))
		if signal := validationCommandEvidenceSignal(item); signal != "" {
			builder.WriteString(fmt.Sprintf(" | signal=%s", signal))
		}
		builder.WriteString("\n")
	}
	if run != nil {
		builder.WriteString(fmt.Sprintf("\nRun: %s\n", run.ID))
		if run.GoalID != "" {
			builder.WriteString(fmt.Sprintf("Goal: %s\n", run.GoalID))
		}
		if run.DoDID != "" {
			builder.WriteString(fmt.Sprintf("DoD: %s\n", run.DoDID))
		}
	}
	return strings.TrimSpace(builder.String())
}

func validationCommandEvidenceSignal(item validationCommandEntry) string {
	return compactValidationEvidenceSignal(firstNonEmptyOrchestratorString(item.Summary, firstUsefulEvidenceLine(item.Stdout), firstUsefulEvidenceLine(item.Stderr)))
}

func firstUsefulEvidenceLine(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return ""
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func compactValidationEvidenceSignal(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	const maxLen = 180
	if len(text) <= maxLen {
		return text
	}
	return strings.TrimSpace(text[:maxLen]) + "..."
}

func validationFailureMemoryEntries(items []validationCommandEntry) []validationCommandEntry {
	ret := make([]validationCommandEntry, 0)
	for _, item := range items {
		if strings.TrimSpace(item.ExitCode) != "" && strings.TrimSpace(item.ExitCode) != "0" {
			ret = append(ret, item)
			continue
		}
		if strings.TrimSpace(item.Stderr) != "" {
			ret = append(ret, item)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func validationFailureLessonContent(run *runModel.Run, items []validationCommandEntry) string {
	var builder strings.Builder
	builder.WriteString("Validation failures observed during execution and used for repair:\n")
	for _, item := range items {
		builder.WriteString(fmt.Sprintf("- %s | exitCode=%s | evidence=%s", item.Command, item.ExitCode, firstNonEmptyOrchestratorString(item.Ref, "n/a")))
		if item.Stderr != "" {
			builder.WriteString(fmt.Sprintf(" | stderr=%s", item.Stderr))
		}
		builder.WriteString("\n")
	}
	if run != nil {
		builder.WriteString(fmt.Sprintf("\nRun: %s", run.ID))
	}
	return strings.TrimSpace(builder.String())
}

func (o *RuntimeOrchestrator) upsertRunMemoryItem(ctx context.Context, item *memorymodel.MemoryItem) error {
	if o == nil || o.evidence.MemoryItems == nil || item == nil {
		return nil
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
		return o.eventPublisher().publishMemoryChanged(ctx, events.TopicMemoryUpdated, existing)
	}
	return o.eventPublisher().publishMemoryChanged(ctx, events.TopicMemoryCreated, item)
}
