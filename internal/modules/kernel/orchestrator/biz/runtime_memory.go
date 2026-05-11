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

var runtimeMemoryLongTermPreferenceKeywords = []string{
	"preference", "prefer", "default", "always", "habit", "convention", "policy",
	"偏好", "默认", "习惯", "优先", "长期",
}

var runtimeMemoryConfirmedConventionKeywords = []string{
	"confirmed", "agreed", "adopted", "convention", "rule", "guideline",
	"已确认", "确认", "约定", "规范", "规则", "统一",
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
	if request.MemoryType != memorycommon.MemoryTypeProjectContext &&
		request.MemoryType != memorycommon.MemoryTypeUserPreference {
		return false
	}
	joinedTags := strings.ToLower(strings.Join(request.Tags, " "))
	if memorycommon.ContainsSensitiveMemoryText(request.Summary, request.Content, joinedTags) {
		return false
	}
	if request.MemoryType == memorycommon.MemoryTypeProjectContext {
		return containsAnyKeyword(request.Summary, request.Content, joinedTags, runtimeMemoryConfirmedConventionKeywords)
	}
	return containsAnyKeyword(request.Summary, request.Content, joinedTags, runtimeMemoryLongTermPreferenceKeywords)
}

func containsAnyKeyword(summary string, content string, tags string, keywords []string) bool {
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{summary, content, tags}, "\n")))
	if text == "" {
		return false
	}
	for _, keyword := range keywords {
		if strings.Contains(text, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func runtimeRunSummaryMemoryWrite(output map[string]any) []runtimeMemoryWriteRequest {
	return appendRunSummaryMemoryWrite(nil, output)
}

func appendRunSummaryMemoryWrite(requests []runtimeMemoryWriteRequest, output map[string]any) []runtimeMemoryWriteRequest {
	if output == nil {
		return requests
	}
	stage, _ := output["stage"].(string)
	if stage != "llm_dod_assessment" || hasRunSummaryMemoryWrite(requests) {
		return requests
	}
	summary := runSummaryMemorySummary(output)
	if summary == "" {
		return requests
	}
	return append(requests, runtimeMemoryWriteRequest{
		MemoryType: memorycommon.MemoryTypeRunSummary,
		Summary:    summary,
		Content:    runSummaryMemoryContent(output, summary),
		Tags:       []string{"run_summary"},
	})
}

func runSummaryMemorySummary(output map[string]any) string {
	if output == nil {
		return ""
	}
	if summary := strings.TrimSpace(stringFromMap(output, "summary")); summary != "" {
		return summary
	}
	if assessment := doDAssessmentFromOutput(output); assessment != nil {
		return strings.TrimSpace(stringFromMap(assessment, "summary"))
	}
	return ""
}

func runSummaryMemoryContent(output map[string]any, summary string) string {
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return summary
	}
	parts := []string{summary}
	for _, key := range []string{"goalOutcome", "implementationSummary", "verificationSummary"} {
		value := strings.TrimSpace(stringFromMap(assessment, key))
		if value != "" && value != summary {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "\n\n")
}

func hasRunSummaryMemoryWrite(requests []runtimeMemoryWriteRequest) bool {
	for _, request := range requests {
		if request.MemoryType == memorycommon.MemoryTypeRunSummary {
			return true
		}
	}
	return false
}
