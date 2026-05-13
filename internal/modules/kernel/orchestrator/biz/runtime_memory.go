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

type runtimeMemoryWriteRequest struct {
	MemoryType string
	Summary    string
	Content    string
	Tags       []string
}

func (o *RuntimeOrchestrator) recordMemoryWrites(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	output map[string]any,
) ([]string, error) {
	explicitWrites := output != nil && output[stepOutputKeyMemoryWrites] != nil
	requests, err := runtimeMemoryWriteRequests(output)
	if err != nil {
		return nil, err
	}
	requests = appendNecessaryOutputMemoryWriteRequests(requests, run, step, output)
	requests = append(requests, acceptedInputMemoryWriteRequests(run, step)...)
	if len(requests) == 0 {
		return nil, nil
	}
	if o.evidence.MemoryItems == nil {
		if !explicitWrites {
			return nil, nil
		}
		return nil, common.ErrInvalidArgument
	}
	refs := make([]string, len(requests))
	tasks := make([]func() error, 0, len(requests))
	for idx, request := range requests {
		memoryID := common.MemoryID(fmt.Sprintf("%s/memory/%d", step.ID, idx+1))
		item := &memorymodel.MemoryItem{
			ID:          memoryID,
			WorkspaceID: run.WorkspaceID,
			RunID:       run.ID,
			SourceType:  memorycommon.MemorySourceRun,
			SourceRef:   string(run.ID),
			MemoryType:  request.MemoryType,
			Content:     request.Content,
			Summary:     request.Summary,
			Tags:        append([]string(nil), request.Tags...),
			Importance:  50,
			Confidence:  50,
			Visibility:  "workspace",
		}
		refs[idx] = string(memoryID)
		tasks = append(tasks, func() error {
			topic := events.TopicMemoryCreated
			if err := o.evidence.MemoryItems.Create(ctx, item); err != nil {
				if err != common.ErrConflict {
					return err
				}
				if err := o.evidence.MemoryItems.Update(ctx, item); err != nil {
					return err
				}
				topic = events.TopicMemoryUpdated
			}
			return o.publishMemoryChanged(ctx, topic, item)
		})
	}
	if err := runParallel(tasks...); err != nil {
		return nil, err
	}
	return refs, nil
}

func runtimeMemoryWriteRequests(output map[string]any) ([]runtimeMemoryWriteRequest, error) {
	if output == nil || output[stepOutputKeyMemoryWrites] == nil {
		return runtimeRunSummaryMemoryWrite(output), nil
	}
	ret := []runtimeMemoryWriteRequest{}
	switch items := output[stepOutputKeyMemoryWrites].(type) {
	case []map[string]any:
		for _, item := range items {
			request, err := runtimeMemoryWriteRequestFromMap(item)
			if err != nil {
				return nil, err
			}
			if shouldPersistRuntimeMemoryWrite(request) {
				ret = append(ret, request)
			}
		}
	case []any:
		for _, item := range items {
			mapped, ok := item.(map[string]any)
			if !ok {
				return nil, common.ErrInvalidArgument
			}
			request, err := runtimeMemoryWriteRequestFromMap(mapped)
			if err != nil {
				return nil, err
			}
			if shouldPersistRuntimeMemoryWrite(request) {
				ret = append(ret, request)
			}
		}
	default:
		return nil, common.ErrInvalidArgument
	}
	return appendRunSummaryMemoryWrite(ret, output), nil
}

func runtimeMemoryWriteRequestFromMap(item map[string]any) (runtimeMemoryWriteRequest, error) {
	memoryType, _ := item["memoryType"].(string)
	summary, _ := item["summary"].(string)
	content, _ := item["content"].(string)
	memoryType = strings.TrimSpace(memoryType)
	summary = strings.TrimSpace(summary)
	if memoryType == "" || summary == "" || !memorycommon.IsSupportedMemoryType(memoryType) {
		return runtimeMemoryWriteRequest{}, common.ErrInvalidArgument
	}
	return runtimeMemoryWriteRequest{
		MemoryType: memoryType,
		Summary:    summary,
		Content:    strings.TrimSpace(content),
		Tags:       stringSliceFromValue(item["tags"]),
	}, nil
}

func shouldPersistRuntimeMemoryWrite(request runtimeMemoryWriteRequest) bool {
	request.MemoryType = strings.TrimSpace(request.MemoryType)
	request.Summary = strings.TrimSpace(request.Summary)
	request.Content = strings.TrimSpace(request.Content)
	if request.MemoryType == "" || request.Summary == "" {
		return false
	}
	if !memorycommon.IsWritableMemoryTypeForSource(memorycommon.MemorySourceRun, request.MemoryType) {
		return false
	}
	joinedTags := strings.ToLower(strings.Join(request.Tags, " "))
	if memorycommon.ContainsSensitiveMemoryText(request.Summary, request.Content, joinedTags) {
		return false
	}
	switch request.MemoryType {
	case memorycommon.MemoryTypeProjectContext:
		return true
	case memorycommon.MemoryTypeUserPreference:
		return true
	case memorycommon.MemoryTypeDoDFeedback,
		memorycommon.MemoryTypeDecision,
		memorycommon.MemoryTypeExecutionLesson,
		memorycommon.MemoryTypeSkillFeedback,
		memorycommon.MemoryTypeToolFeedback:
		return true
	default:
		return false
	}
}

func appendNecessaryOutputMemoryWriteRequests(requests []runtimeMemoryWriteRequest, run *runModel.Run, step *runModel.Step, output map[string]any) []runtimeMemoryWriteRequest {
	requests = appendRepairLessonMemoryWrite(requests, run, step, output)
	requests = appendProviderRetryToolMemoryWrite(requests, run, step, output)
	return requests
}

func appendRepairLessonMemoryWrite(requests []runtimeMemoryWriteRequest, _ *runModel.Run, step *runModel.Step, output map[string]any) []runtimeMemoryWriteRequest {
	if step == nil || step.Input == nil || !boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) || hasMemoryWriteType(requests, memorycommon.MemoryTypeExecutionLesson) {
		return requests
	}
	attempt := int(int64FromValue(step.Input[stepInputKeyRuntimeRepairAttempt]))
	lines := []string{
		fmt.Sprintf("Repair attempt: %d", attempt),
	}
	for _, key := range []string{
		"failedStepId",
		"failedStepKind",
		"failureReason",
		failureOutputKeyClass,
		failureOutputKeySignature,
		stepInputKeyRepairOfRootStepID,
	} {
		if value := strings.TrimSpace(memoryRecallTraceStringValue(step.Input[key])); value != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", key, value))
		}
	}
	for _, key := range []string{"stage", "status", "summary", "nextStep", "nextKind"} {
		if value := strings.TrimSpace(stringFromMap(output, key)); value != "" {
			lines = append(lines, fmt.Sprintf("output.%s: %s", key, value))
		}
	}
	if missing := mapSliceFromValue(step.Input["missingDoDItems"]); len(missing) > 0 {
		lines = append(lines, "missingDoDItems:")
		for _, item := range limitedMapSlice(missing, 6) {
			if name := strings.TrimSpace(stringFromMap(item, "item")); name != "" {
				lines = append(lines, "- "+name)
			}
		}
	}
	if graph := mapFromValue(output[stepOutputKeyExecutionGraph]); len(graph) > 0 {
		lines = append(lines, "output.executionGraph: present")
	}
	summary := repairLessonSummary(step, output)
	if lesson := repairLessonContentHint(step, output); lesson != "" {
		lines = append(lines, "lesson: "+lesson)
	}
	return append(requests, runtimeMemoryWriteRequest{
		MemoryType: memorycommon.MemoryTypeExecutionLesson,
		Summary:    summary,
		Content:    strings.Join(lines, "\n"),
		Tags:       []string{"repair_loop", "necessary_output"},
	})
}

func repairLessonSummary(step *runModel.Step, output map[string]any) string {
	if summary := repairLessonContentHint(step, output); summary != "" {
		return summary
	}
	if summary := strings.TrimSpace(stringFromMap(output, "summary")); summary != "" {
		return summary
	}
	return "Runtime repair lesson captured"
}

func repairLessonContentHint(step *runModel.Step, output map[string]any) string {
	if step == nil {
		return ""
	}
	failedKind := strings.TrimSpace(memoryRecallTraceStringValue(step.Input["failedStepKind"]))
	stage := strings.TrimSpace(stringFromMap(output, "stage"))
	failureText := strings.ToLower(strings.Join([]string{
		memoryRecallTraceStringValue(step.Input["failureReason"]),
		memoryRecallTraceStringValue(step.Input[failureOutputKeyClass]),
		memoryRecallTraceStringValue(step.Input[failureOutputKeySignature]),
		stringFromMap(output, "summary"),
		stringFromMap(output, "error"),
	}, "\n"))
	if failedKind == "file/read" || strings.Contains(stage, "file_read") || strings.Contains(failureText, "no such file") || strings.Contains(failureText, "not found") {
		return "Missing file reads require optional probes or listing before repair"
	}
	return ""
}

func appendProviderRetryToolMemoryWrite(requests []runtimeMemoryWriteRequest, _ *runModel.Run, step *runModel.Step, output map[string]any) []runtimeMemoryWriteRequest {
	if step == nil || step.Input == nil || !boolValue(step.Input[stepInputKeyProviderRetryFollowUp]) || hasMemoryWriteType(requests, memorycommon.MemoryTypeToolFeedback) {
		return requests
	}
	attempt := int(int64FromValue(step.Input[stepInputKeyProviderRetryAttempt]))
	lines := []string{
		fmt.Sprintf("Provider retry attempt: %d", attempt),
	}
	for _, key := range []string{"failedStepId", "failureReason", failureOutputKeyClass, failureOutputKeySignature} {
		if value := strings.TrimSpace(memoryRecallTraceStringValue(step.Input[key])); value != "" {
			lines = append(lines, fmt.Sprintf("%s: %s", key, value))
		}
	}
	for _, key := range []string{"stage", "status", "summary"} {
		if value := strings.TrimSpace(stringFromMap(output, key)); value != "" {
			lines = append(lines, fmt.Sprintf("output.%s: %s", key, value))
		}
	}
	return append(requests, runtimeMemoryWriteRequest{
		MemoryType: memorycommon.MemoryTypeToolFeedback,
		Summary:    "Provider retry outcome captured",
		Content:    strings.Join(lines, "\n"),
		Tags:       []string{"provider_retry", "tool_context", "necessary_output"},
	})
}

func acceptedInputMemoryWriteRequests(run *runModel.Run, step *runModel.Step) []runtimeMemoryWriteRequest {
	if run == nil || step == nil || step.Kind != llmWorkspaceExecutionStepKind {
		return nil
	}
	pkg := mapFromValue(step.Input[stepInputKeyExecutionContextPackage])
	selected := mapFromValue(pkg["selectedContext"])
	ret := make([]runtimeMemoryWriteRequest, 0, 3)
	if request, ok := acceptedSkillInputMemoryWriteRequest(run, step, selected); ok {
		ret = append(ret, request)
	}
	if request, ok := acceptedToolInputMemoryWriteRequest(run, step, selected); ok {
		ret = append(ret, request)
	}
	if request, ok := acceptedSoulPromptInputMemoryWriteRequest(run, step); ok {
		ret = append(ret, request)
	}
	return ret
}

func acceptedSkillInputMemoryWriteRequest(run *runModel.Run, step *runModel.Step, selected map[string]any) (runtimeMemoryWriteRequest, bool) {
	skills := compactSkillContexts(mapSliceFromValue(selected["skills"]))
	if len(skills) == 0 {
		skills = compactSkillContexts(mapSliceFromValue(run.ExecutionContext["skills"]))
	}
	if len(skills) == 0 {
		return runtimeMemoryWriteRequest{}, false
	}
	ids := contextIDs(skills)
	summary := "Accepted skill context"
	if len(ids) > 0 {
		summary = "Accepted skill context: " + strings.Join(ids, ", ")
	}
	lines := []string{
		fmt.Sprintf("Stage: %s", firstNonEmptyString(stringFromMap(step.Input, stepInputKeyRuntimeStage), stringFromMap(step.Input, stepInputKeySoulStage), stringFromMap(step.Output, "stage"), "unknown")),
	}
	for _, skill := range skills {
		lines = append(lines, fmt.Sprintf(
			"- %s: %s",
			firstNonEmptyString(stringFromMap(skill, "id"), stringFromMap(skill, "name"), "unknown"),
			firstNonEmptyString(stringFromMap(skill, "description"), stringFromMap(skill, "instruction"), "selected skill"),
		))
		if tools := stringSliceFromValue(skill["requiredToolKinds"]); len(tools) > 0 {
			lines = append(lines, "  requiredToolKinds: "+strings.Join(tools, ", "))
		}
		if constraints := stringSliceFromValue(skill["constraints"]); len(constraints) > 0 {
			lines = append(lines, "  constraints: "+strings.Join(constraints, "; "))
		}
	}
	if selection := mapFromValue(run.ExecutionContext["skillSelection"]); len(selection) > 0 {
		if reason := strings.TrimSpace(stringFromMap(selection, "reason")); reason != "" {
			lines = append(lines, "Selection reason: "+reason)
		}
	}
	return runtimeMemoryWriteRequest{
		MemoryType: memorycommon.MemoryTypeSkillFeedback,
		Summary:    summary,
		Content:    strings.Join(lines, "\n"),
		Tags:       []string{"accepted_input", "skill_context"},
	}, true
}

func acceptedToolInputMemoryWriteRequest(run *runModel.Run, step *runModel.Step, selected map[string]any) (runtimeMemoryWriteRequest, bool) {
	servers := compactMCPServerContexts(mapSliceFromValue(selected["mcpServers"]))
	if len(servers) == 0 {
		servers = compactMCPServerContexts(mapSliceFromValue(run.ExecutionContext["mcpServers"]))
	}
	capabilities := compactMCPCapabilityContexts(mapSliceFromValue(selected["mcpCapabilities"]))
	if len(capabilities) == 0 {
		capabilities = compactMCPCapabilityContexts(mapSliceFromValue(run.ExecutionContext["mcpCapabilities"]))
	}
	soul := mapFromValue(run.ExecutionContext["soul"])
	toolKinds := stringSliceFromValue(soul["allowedToolKinds"])
	if len(servers) == 0 && len(capabilities) == 0 && len(toolKinds) == 0 {
		return runtimeMemoryWriteRequest{}, false
	}
	lines := []string{
		fmt.Sprintf("Stage: %s", firstNonEmptyString(stringFromMap(step.Input, stepInputKeyRuntimeStage), stringFromMap(step.Input, stepInputKeySoulStage), stringFromMap(step.Output, "stage"), "unknown")),
	}
	if len(toolKinds) > 0 {
		lines = append(lines, "allowedToolKinds: "+strings.Join(toolKinds, ", "))
	}
	if ids := contextIDs(servers); len(ids) > 0 {
		lines = append(lines, "mcpServers: "+strings.Join(ids, ", "))
	}
	if ids := contextIDs(capabilities); len(ids) > 0 {
		lines = append(lines, "mcpCapabilities: "+strings.Join(ids, ", "))
	}
	if selection := mapFromValue(run.ExecutionContext["mcpSelection"]); len(selection) > 0 {
		if auto := stringSliceFromValue(selection["autoBoundMCPServerIds"]); len(auto) > 0 {
			lines = append(lines, "autoBoundMCPServerIds: "+strings.Join(auto, ", "))
		}
		if auto := stringSliceFromValue(selection["autoBoundCapabilityIds"]); len(auto) > 0 {
			lines = append(lines, "autoBoundCapabilityIds: "+strings.Join(auto, ", "))
		}
	}
	return runtimeMemoryWriteRequest{
		MemoryType: memorycommon.MemoryTypeToolFeedback,
		Summary:    "Accepted tool and MCP context",
		Content:    strings.Join(lines, "\n"),
		Tags:       []string{"accepted_input", "tool_context", "mcp_context"},
	}, true
}

func acceptedSoulPromptInputMemoryWriteRequest(run *runModel.Run, step *runModel.Step) (runtimeMemoryWriteRequest, bool) {
	soul := mapFromValue(run.ExecutionContext["soul"])
	soulRouting := mapFromValue(run.ExecutionContext["soulRouting"])
	stage := firstNonEmptyString(stringFromMap(step.Input, stepInputKeyRuntimeStage), stringFromMap(step.Input, stepInputKeySoulStage), stringFromMap(step.Output, "stage"))
	if len(soul) == 0 && len(soulRouting) == 0 {
		return runtimeMemoryWriteRequest{}, false
	}
	lines := []string{}
	if stage != "" {
		lines = append(lines, "Stage: "+stage)
	}
	if id := strings.TrimSpace(stringFromMap(soul, "id")); id != "" {
		lines = append(lines, "soulId: "+id)
	}
	if name := strings.TrimSpace(stringFromMap(soul, "name")); name != "" {
		lines = append(lines, "soulName: "+name)
	}
	if description := strings.TrimSpace(stringFromMap(soul, "description")); description != "" {
		lines = append(lines, "soulDescription: "+description)
	}
	if tools := stringSliceFromValue(soul["allowedToolKinds"]); len(tools) > 0 {
		lines = append(lines, "allowedToolKinds: "+strings.Join(tools, ", "))
	}
	if len(soulRouting) > 0 {
		lines = append(lines, "soulRoutingPolicy: accepted runtime stage routing; follow the current stage soul constraints.")
	}
	lines = append(lines, "promptPolicy: record only effective constraints and provenance, not the full prompt text.")
	return runtimeMemoryWriteRequest{
		MemoryType: memorycommon.MemoryTypeProjectContext,
		Summary:    "Accepted Soul and prompt constraints",
		Content:    strings.Join(lines, "\n"),
		Tags:       []string{"accepted_input", "soul_context", "prompt_policy"},
	}, true
}

func contextIDs(items []map[string]any) []string {
	ret := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		id := strings.TrimSpace(stringFromMap(item, "id"))
		if id == "" {
			id = strings.TrimSpace(stringFromMap(item, "name"))
		}
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ret = append(ret, id)
	}
	return ret
}

func runtimeRunSummaryMemoryWrite(output map[string]any) []runtimeMemoryWriteRequest {
	return appendRunSummaryMemoryWrite(nil, output)
}

func appendRunSummaryMemoryWrite(requests []runtimeMemoryWriteRequest, output map[string]any) []runtimeMemoryWriteRequest {
	if output == nil {
		return requests
	}
	stage, _ := output["stage"].(string)
	if stage != "llm_dod_assessment" {
		return requests
	}
	return appendDoDFeedbackMemoryWrite(requests, output)
}

func appendDoDFeedbackMemoryWrite(requests []runtimeMemoryWriteRequest, output map[string]any) []runtimeMemoryWriteRequest {
	if hasDoDFeedbackMemoryWrite(requests) {
		return requests
	}
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return requests
	}
	items, ok := doDAssessmentItemMaps(assessment["items"])
	if !ok || len(items) == 0 {
		return requests
	}
	lines := make([]string, 0, len(items)+2)
	for _, item := range items {
		state := strings.ToLower(strings.TrimSpace(stringFromMap(item, "state")))
		if state == "" || state == "satisfied" {
			continue
		}
		name := strings.TrimSpace(stringFromMap(item, "item"))
		if name == "" {
			continue
		}
		reason := strings.TrimSpace(firstNonEmptyString(stringFromMap(item, "reason"), stringFromMap(item, "summary")))
		if reason != "" {
			lines = append(lines, fmt.Sprintf("- [%s] %s: %s", state, name, reason))
			continue
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s", state, name))
	}
	if len(lines) == 0 {
		return requests
	}
	summary := strings.TrimSpace(stringFromMap(assessment, "summary"))
	if summary == "" {
		summary = "DoD assessment feedback captured"
	}
	content := strings.Join(append([]string{summary, "DoD gaps:"}, lines...), "\n")
	return append(requests, runtimeMemoryWriteRequest{
		MemoryType: memorycommon.MemoryTypeDoDFeedback,
		Summary:    summary,
		Content:    content,
		Tags:       []string{"dod_feedback", "repair_loop"},
	})
}

func hasDoDFeedbackMemoryWrite(requests []runtimeMemoryWriteRequest) bool {
	return hasMemoryWriteType(requests, memorycommon.MemoryTypeDoDFeedback)
}

func hasMemoryWriteType(requests []runtimeMemoryWriteRequest, memoryType string) bool {
	for _, request := range requests {
		if request.MemoryType == memoryType {
			return true
		}
	}
	return false
}
