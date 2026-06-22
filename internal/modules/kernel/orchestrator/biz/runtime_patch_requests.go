package orchestrator

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func runtimePatchRequests(output map[string]any) ([]runtimePatchRequest, error) {
	if output == nil || output[stepOutputKeyPatches] == nil {
		return nil, nil
	}
	switch items := output[stepOutputKeyPatches].(type) {
	case []map[string]any:
		ret := make([]runtimePatchRequest, 0, len(items))
		for _, item := range items {
			request, err := runtimePatchRequestFromMap(item)
			if err != nil {
				return nil, err
			}
			ret = append(ret, request)
		}
		return ret, nil
	case []any:
		ret := make([]runtimePatchRequest, 0, len(items))
		for _, item := range items {
			mapped, ok := item.(map[string]any)
			if !ok {
				return nil, common.ErrInvalidArgument
			}
			request, err := runtimePatchRequestFromMap(mapped)
			if err != nil {
				return nil, err
			}
			ret = append(ret, request)
		}
		return ret, nil
	default:
		return nil, common.ErrInvalidArgument
	}
}

func runtimePatchRequestFromMap(item map[string]any) (runtimePatchRequest, error) {
	targetRef, _ := item["targetRef"].(string)
	content, _ := item["content"].(string)
	diff, _ := item["diff"].(string)
	oldText, _ := item["oldText"].(string)
	newText, _ := item["newText"].(string)
	summary, _ := item["summary"].(string)
	patchType, _ := item["patchType"].(string)
	operation, _ := item["operation"].(string)
	baseHash, _ := item["baseHash"].(string)
	approvalID, _ := item["approvalId"].(string)
	occurrence := intFromAny(item["occurrence"])
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch targetRef is required", common.ErrInvalidArgument)
	}
	if isReservedWorkspacePatchTarget(targetRef) {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch targetRef %q is reserved for WorkOrch-managed artifacts; return final query/report content in dodAssessment instead of writing report-only deliverables into the workspace", common.ErrInvalidArgument, targetRef)
	}
	if patchType == "" {
		patchType = strings.TrimSpace(operation)
	}
	if patchType == "" && oldText != "" {
		patchType = "text_replace"
	}
	if patchType == "" {
		patchType = "file_write"
	}
	switch patchType {
	case "create", "write", "replace", "file":
		patchType = "file_write"
	case "edit", "file_edit", "file/edit", "old_text_replace", "exact_replace":
		patchType = "text_replace"
	}
	if patchType != "file_write" && patchType != "unified_diff" && patchType != "text_replace" {
		return runtimePatchRequest{}, fmt.Errorf("%w: patchType %q is not supported", common.ErrInvalidArgument, patchType)
	}
	if patchType == "file_write" && strings.TrimSpace(content) == "" {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch content is required for file_write", common.ErrInvalidArgument)
	}
	if patchType == "unified_diff" && strings.TrimSpace(diff) == "" {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch diff is required for unified_diff", common.ErrInvalidArgument)
	}
	if patchType == "text_replace" && oldText == "" {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch oldText is required for text_replace", common.ErrInvalidArgument)
	}
	if patchType == "text_replace" && oldText == newText {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch text_replace oldText/newText must describe a substantive change", common.ErrInvalidArgument)
	}
	if containsUnresolvedPatchBaseHash(baseHash) {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch baseHash contains unresolved runtime template placeholders", common.ErrInvalidArgument)
	}
	if patchType == "file_write" && containsUnresolvedRuntimeTemplate(content) {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch content contains unresolved runtime template placeholders", common.ErrInvalidArgument)
	}
	if patchType == "unified_diff" && containsUnresolvedRuntimeTemplate(diff) {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch diff contains unresolved runtime template placeholders", common.ErrInvalidArgument)
	}
	if patchType == "text_replace" && (containsUnresolvedRuntimeTemplate(oldText) || containsUnresolvedRuntimeTemplate(newText)) {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch text_replace contains unresolved runtime template placeholders", common.ErrInvalidArgument)
	}
	if patchType == "file_write" && containsRuntimeRedactionMarker(content) {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch content contains redacted runtime markers", common.ErrInvalidArgument)
	}
	if patchType == "unified_diff" && containsRuntimeRedactionMarker(diff) {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch diff contains redacted runtime markers", common.ErrInvalidArgument)
	}
	if patchType == "text_replace" && (containsRuntimeRedactionMarker(oldText) || containsRuntimeRedactionMarker(newText)) {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch text_replace contains redacted runtime markers", common.ErrInvalidArgument)
	}
	if summary == "" {
		summary = "workspace patch apply"
	}
	if patchDeclaresNonSubstantiveChange(item) {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch describes a non-substantive state-change placeholder", common.ErrInvalidArgument)
	}
	return runtimePatchRequest{
		TargetRef:       targetRef,
		Content:         content,
		Diff:            diff,
		OldText:         oldText,
		NewText:         newText,
		Summary:         summary,
		PatchType:       patchType,
		BaseHash:        strings.TrimSpace(baseHash),
		Occurrence:      occurrence,
		ApprovalID:      common.ApprovalID(strings.TrimSpace(approvalID)),
		CoverageTargets: patchCoverageTargetsValue(item["coverageTargets"]),
		Extra:           cloneStepPayload(item),
	}, nil
}

func patchCoverageTargetsValue(value any) any {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				ret = append(ret, text)
			}
		}
		if len(ret) == 0 {
			return nil
		}
		return ret
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			if item != nil {
				ret = append(ret, item)
			}
		}
		if len(ret) == 0 {
			return nil
		}
		return ret
	case map[string]any:
		if len(typed) == 0 {
			return nil
		}
		return cloneStepPayload(typed)
	default:
		text := strings.TrimSpace(fmt.Sprint(typed))
		if text == "" || text == "<nil>" {
			return nil
		}
		return text
	}
}

func isReservedWorkspacePatchTarget(targetRef string) bool {
	cleanTarget := filepath.ToSlash(filepath.Clean(strings.TrimSpace(targetRef)))
	cleanTarget = strings.TrimPrefix(cleanTarget, "./")
	if cleanTarget == "." || cleanTarget == "" {
		return false
	}
	parts := strings.Split(strings.ToLower(cleanTarget), "/")
	if len(parts) == 0 {
		return false
	}
	switch parts[0] {
	case ".workorch-data", "delivery":
		return true
	default:
		return isReservedWorkspaceReportPatchTarget(cleanTarget)
	}
}

func isReservedWorkspaceReportPatchTarget(cleanTarget string) bool {
	base := strings.ToLower(pathBaseLower(cleanTarget))
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".md", ".txt", ".adoc", ".rst", ".json":
	default:
		return false
	}
	stem := strings.TrimSuffix(base, ext)
	stem = strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(stem)
	reportTerms := strings.FieldsFunc(stem, func(r rune) bool { return r == '_' })
	hasTerm := func(term string) bool {
		for _, item := range reportTerms {
			if item == term {
				return true
			}
		}
		return false
	}
	switch {
	case hasTerm("decision") && hasTerm("record"):
		return true
	case hasTerm("approval") && hasTerm("record"):
		return true
	case hasTerm("delivery") && hasTerm("summary"):
		return true
	case hasTerm("deliverable") && hasTerm("manifest"):
		return true
	case hasTerm("workspace") && hasTerm("deliverable") && hasTerm("manifest"):
		return true
	case hasTerm("result") && hasTerm("summary"):
		return true
	case hasTerm("run") && hasTerm("summary"):
		return true
	case hasTerm("acceptance") && hasTerm("evidence"):
		return true
	case hasTerm("evidence") && hasTerm("chain"):
		return true
	case hasTerm("dod") && hasTerm("assessment"):
		return true
	case hasTerm("final") && (hasTerm("summary") || hasTerm("result") || hasTerm("report")):
		return true
	default:
		return false
	}
}

func containsUnresolvedRuntimeTemplate(value string) bool {
	return runtimeTemplatePattern.MatchString(value) ||
		unresolvedUnifiedDiffHunkPattern.MatchString(value) ||
		unresolvedRuntimeSymbolPattern.MatchString(value) ||
		unresolvedAnglePlaceholderPattern.MatchString(value) ||
		unresolvedPseudoImplementationPattern.MatchString(value)
}

func containsUnresolvedPatchBaseHash(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	return runtimeTemplatePattern.MatchString(value) ||
		unresolvedDollarPlaceholderPattern.MatchString(value) ||
		unresolvedRuntimeSymbolPattern.MatchString(value) ||
		unresolvedAnglePlaceholderPattern.MatchString(value)
}

func containsRuntimeRedactionMarker(value string) bool {
	return runtimeRedactionMarkerPattern.MatchString(value)
}

func patchDeclaresNonSubstantiveChange(item map[string]any) bool {
	if item == nil {
		return false
	}
	for _, key := range []string{"nonSubstantive", "zeroWork"} {
		if boolFromValue(item[key]) {
			return true
		}
	}
	for _, key := range []string{"substantive", "meaningfulChange"} {
		value, ok := item[key].(bool)
		if ok && !value {
			return true
		}
	}
	return false
}

var unresolvedRuntimeSymbolPattern = regexp.MustCompile(`__[A-Z][A-Z0-9_]{8,}__`)
var unresolvedDollarPlaceholderPattern = regexp.MustCompile(`\$\{\s*[A-Za-z0-9_.-]+\s*\}`)
var unresolvedUnifiedDiffHunkPattern = regexp.MustCompile(`(?m)^@@\s*\.\.\.\s*@@`)
var unresolvedAnglePlaceholderPattern = regexp.MustCompile(`<<\s*[A-Za-z0-9_-]*(FROM|PATCH|DIFF|CONTENT|IMPLEMENTATION|PLACEHOLDER|TODO)[A-Za-z0-9_\s.-]*\s*>>`)
var unresolvedPseudoImplementationPattern = regexp.MustCompile(`(?i)(TODO:\s*(implement|fill|replace)|\.\.\.\s*(省略|omitted|placeholder)|省略(实现|代码|内容)|伪代码|占位(实现|代码|内容))`)
var runtimeRedactionMarkerPattern = regexp.MustCompile(`(?i)\[(?:REDACTED|SECRET|MASKED)\]`)
