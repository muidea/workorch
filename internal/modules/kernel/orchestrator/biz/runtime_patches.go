package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type runtimePatchRequest struct {
	TargetRef       string
	Content         string
	Diff            string
	OldText         string
	NewText         string
	Summary         string
	PatchType       string
	BaseHash        string
	Occurrence      int
	ApprovalID      common.ApprovalID
	CoverageTargets any
	Extra           map[string]any
}

type runtimePatchRevertRequest struct {
	PatchID    common.PatchID
	ApprovalID common.ApprovalID
	Strict     bool
}

type workspacePatchBeforeState struct {
	Exists  bool
	Content string
	Hash    string
}

type runtimeFailureDiagnosticError interface {
	RuntimeFailureDiagnostics() map[string]any
}

type patchApplyDiagnosticError struct {
	targetRef   string
	patchType   string
	stage       string
	reason      string
	inner       error
	guidance    string
	errorDetail string
}

func (e *patchApplyDiagnosticError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.reason) != "" {
		return e.reason
	}
	if e.inner != nil {
		return e.inner.Error()
	}
	return common.ErrInvalidArgument.Error()
}

func (e *patchApplyDiagnosticError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.inner
}

func (e *patchApplyDiagnosticError) RuntimeFailureDiagnostics() map[string]any {
	if e == nil {
		return nil
	}
	ret := map[string]any{
		"kind":      failureClassPatchApply,
		"targetRef": strings.TrimSpace(e.targetRef),
		"patchType": strings.TrimSpace(e.patchType),
		"stage":     firstNonEmptyString(strings.TrimSpace(e.stage), "patch_apply"),
	}
	if detail := strings.TrimSpace(e.errorDetail); detail != "" {
		ret["errorDetail"] = detail
	}
	if guidance := strings.TrimSpace(e.guidance); guidance != "" {
		ret["guidance"] = guidance
	}
	return ret
}

func newPatchApplyDiagnosticError(targetRef string, patchType string, stage string, inner error, reason string, guidance string, detail string) error {
	return &patchApplyDiagnosticError{
		targetRef:   strings.TrimSpace(targetRef),
		patchType:   strings.TrimSpace(patchType),
		stage:       strings.TrimSpace(stage),
		reason:      strings.TrimSpace(reason),
		inner:       inner,
		guidance:    strings.TrimSpace(guidance),
		errorDetail: strings.TrimSpace(detail),
	}
}

type workspacePathConflictError struct {
	Operation    string
	TargetRef    string
	ConflictRef  string
	ConflictKind string
}

func (e *workspacePathConflictError) Error() string {
	if e == nil {
		return ""
	}
	operation := strings.TrimSpace(e.Operation)
	if operation == "" {
		operation = "workspace patch"
	}
	conflictKind := strings.TrimSpace(e.ConflictKind)
	if conflictKind == "" {
		conflictKind = "file"
	}
	return fmt.Sprintf("%s path conflict: target %q requires %q as a directory, but it is an existing %s", operation, e.TargetRef, e.ConflictRef, conflictKind)
}

func (e *workspacePathConflictError) RuntimeFailureDiagnostics() map[string]any {
	if e == nil {
		return nil
	}
	return map[string]any{
		"kind":         "workspace_path_conflict",
		"operation":    e.Operation,
		"targetRef":    e.TargetRef,
		"conflictRef":  e.ConflictRef,
		"conflictKind": e.ConflictKind,
		"guidance":     "Choose a targetRef whose parent path is a directory, or write the resource at the existing workspace root. Do not treat an existing file as a directory.",
	}
}

func (o *RuntimeOrchestrator) applyRuntimePatches(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	output map[string]any,
	deliverableDirectory string,
) error {
	requests, err := runtimePatchRequests(output)
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		return nil
	}
	if o.evidence.Patches == nil {
		return common.ErrInvalidArgument
	}
	for idx, request := range requests {
		patchID := common.PatchID(fmt.Sprintf("%s/patch/%d", step.ID, idx+1))
		patch := &runModel.Patch{
			ID:              patchID,
			WorkspaceID:     run.WorkspaceID,
			RunID:           run.ID,
			StepID:          step.ID,
			ToolCallID:      toolCall.ID,
			ApprovalID:      request.ApprovalID,
			PatchType:       request.PatchType,
			TargetRef:       request.TargetRef,
			Content:         request.Content,
			Diff:            request.Diff,
			Summary:         request.Summary,
			CoverageTargets: request.CoverageTargets,
			State:           common.PatchStateCreated,
			CreatedAt:       runtimeEvidenceTimestamp(),
		}
		if err := o.evidence.Patches.Create(ctx, patch); err != nil {
			if err != common.ErrConflict {
				return err
			}
			if err := o.evidence.Patches.Update(ctx, patch); err != nil {
				return err
			}
		}
		beforeState, err := workspacePatchBeforeStateForRequest(deliverableDirectory, request)
		if err != nil {
			patch.State = common.PatchStateFailed
			_ = o.evidence.Patches.Update(ctx, patch)
			if publishErr := o.publishPatchChanged(ctx, run, step, toolCall, patch, events.TopicPatchFailed, err); publishErr != nil {
				return errors.Join(err, publishErr)
			}
			return err
		}
		patch.BeforeHash = beforeState.Hash
		patch.OriginalContent = beforeState.Content
		patch.OriginalExists = beforeState.Exists
		if err := applyWorkspacePatch(deliverableDirectory, request); err != nil {
			patch.State = common.PatchStateFailed
			_ = o.evidence.Patches.Update(ctx, patch)
			if publishErr := o.publishPatchChanged(ctx, run, step, toolCall, patch, events.TopicPatchFailed, err); publishErr != nil {
				return errors.Join(err, publishErr)
			}
			return err
		}
		afterHash, err := workspacePatchAfterHash(deliverableDirectory, request.TargetRef)
		if err != nil {
			patch.State = common.PatchStateFailed
			_ = o.evidence.Patches.Update(ctx, patch)
			if publishErr := o.publishPatchChanged(ctx, run, step, toolCall, patch, events.TopicPatchFailed, err); publishErr != nil {
				return errors.Join(err, publishErr)
			}
			return err
		}
		patch.State = common.PatchStateApplied
		patch.AfterHash = afterHash
		patch.ReverseDiff = reverseDiffForPatch(request.TargetRef, beforeState.Content, beforeState.Exists)
		patch.AppliedAt = runtimeEvidenceTimestamp()
		if err := o.evidence.Patches.Update(ctx, patch); err != nil {
			return err
		}
		if err := o.publishPatchChanged(ctx, run, step, toolCall, patch, events.TopicPatchApplied, nil); err != nil {
			return err
		}
	}
	return nil
}

func (o *RuntimeOrchestrator) applyRuntimePatchRevert(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	output map[string]any,
	deliverableDirectory string,
) error {
	request, ok, err := runtimePatchRevertRequestFromOutput(output)
	if err != nil || !ok {
		return err
	}
	if o.evidence.Patches == nil {
		return common.ErrInvalidArgument
	}
	patch, err := o.evidence.Patches.Get(ctx, request.PatchID)
	if err != nil {
		return err
	}
	if patch.WorkspaceID != run.WorkspaceID || patch.RunID != run.ID {
		return common.ErrConflict
	}
	if patch.State != common.PatchStateApplied {
		return fmt.Errorf("%w: patch %s is not applied", common.ErrInvalidArgument, patch.ID)
	}
	if patch.AfterHash == "" {
		return fmt.Errorf("%w: patch %s has no afterHash and cannot be reverted", common.ErrInvalidArgument, patch.ID)
	}
	currentHash, err := workspacePatchAfterHash(deliverableDirectory, patch.TargetRef)
	if err != nil {
		return err
	}
	if !strings.EqualFold(currentHash, patch.AfterHash) {
		return fmt.Errorf("%w: patch revert hash mismatch", common.ErrConflict)
	}
	if patch.OriginalExists {
		target, err := workspaceTargetPath(deliverableDirectory, patch.TargetRef)
		if err != nil {
			return err
		}
		if err := ensureWorkspacePatchParentDirectory(deliverableDirectory, patch.TargetRef, target, "patch_revert"); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(patch.OriginalContent), 0o644); err != nil {
			return err
		}
	} else {
		target, err := workspaceTargetPath(deliverableDirectory, patch.TargetRef)
		if err != nil {
			return err
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	patch.State = common.PatchStateReverted
	patch.RevertApprovalID = request.ApprovalID
	patch.RevertedAt = runtimeEvidenceTimestamp()
	if err := o.evidence.Patches.Update(ctx, patch); err != nil {
		return err
	}
	output["patchRevert"] = map[string]any{
		"patchId":    string(patch.ID),
		"state":      string(patch.State),
		"targetRef":  patch.TargetRef,
		"beforeHash": patch.AfterHash,
		"afterHash":  patch.BeforeHash,
		"approvalId": string(request.ApprovalID),
	}
	return o.publishPatchChanged(ctx, run, step, toolCall, patch, events.TopicPatchReverted, nil)
}

func runtimePatchRevertRequestFromOutput(output map[string]any) (runtimePatchRevertRequest, bool, error) {
	if output == nil || output["patchRevert"] == nil {
		return runtimePatchRevertRequest{}, false, nil
	}
	value := mapFromValue(output["patchRevert"])
	if len(value) == 0 {
		return runtimePatchRevertRequest{}, false, common.ErrInvalidArgument
	}
	patchID := common.PatchID(strings.TrimSpace(stringFromAny(value["patchId"])))
	if patchID == "" {
		return runtimePatchRevertRequest{}, false, common.ErrInvalidArgument
	}
	return runtimePatchRevertRequest{
		PatchID:    patchID,
		ApprovalID: common.ApprovalID(strings.TrimSpace(stringFromAny(value["approvalId"]))),
		Strict:     boolFromAnyDefault(value["strict"], true),
	}, true, nil
}

func (o *RuntimeOrchestrator) publishPatchPreviewFromOutput(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, output map[string]any) error {
	if output == nil {
		return nil
	}
	preview := mapFromValue(output["patchPreview"])
	if len(preview) == 0 {
		return nil
	}
	return o.publishPatchPreview(ctx, run, step, toolCall, preview)
}

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
	if containsNonSubstantivePatchIntent(summary) || containsNonSubstantivePatchIntent(content) || containsNonSubstantivePatchIntent(diff) || containsNonSubstantivePatchIntent(newText) {
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

func containsNonSubstantivePatchIntent(value string) bool {
	return nonSubstantivePatchIntentPattern.MatchString(value)
}

var unresolvedRuntimeSymbolPattern = regexp.MustCompile(`__[A-Z][A-Z0-9_]{8,}__`)
var unresolvedDollarPlaceholderPattern = regexp.MustCompile(`\$\{\s*[A-Za-z0-9_.-]+\s*\}`)
var unresolvedUnifiedDiffHunkPattern = regexp.MustCompile(`(?m)^@@\s*\.\.\.\s*@@`)
var unresolvedAnglePlaceholderPattern = regexp.MustCompile(`<<\s*[A-Za-z0-9_-]*(FROM|PATCH|DIFF|CONTENT|IMPLEMENTATION|PLACEHOLDER|TODO)[A-Za-z0-9_\s.-]*\s*>>`)
var unresolvedPseudoImplementationPattern = regexp.MustCompile(`(?i)(TODO:\s*(implement|fill|replace)|\.\.\.\s*(省略|omitted|placeholder)|省略(实现|代码|内容)|伪代码|占位(实现|代码|内容))`)
var runtimeRedactionMarkerPattern = regexp.MustCompile(`(?i)\[(?:REDACTED|SECRET|MASKED)\]`)
var nonSubstantivePatchIntentPattern = regexp.MustCompile(`(?i)(ensure|satisfy|force|only|just).{0,48}(patch|state[-_ ]?change|contract|node|anchor)|(?:patch|state[-_ ]?change|contract|node|anchor).{0,48}(ensure|satisfy|force|only|just)|确保.{0,24}补丁.{0,24}(节点|存在)|为了.{0,24}(补丁|状态变更|patch)|(?:满足|确保).{0,24}(状态变更|契约|contract)|runtime[-_ ]?repair.{0,48}(contract|convergence|marker)|仅.{0,12}(添加|增加).{0,12}(注释|comment).{0,24}(满足|确保|避免)|占位.{0,12}(补丁|patch|变更)|严格收敛.{0,32}(锚点|注释|marker|占位|最小状态变更|有界验证)|验证锚点|收敛修复.{0,32}(锚点|注释|marker|占位|最小状态变更)|仅为.{0,24}验证|state[-_ ]?change.{0,24}anchor|verification.{0,24}anchor`)

func applyWorkspacePatch(deliverableDirectory string, request runtimePatchRequest) error {
	switch request.PatchType {
	case "file_write":
		return applyWorkspaceFileWrite(deliverableDirectory, request.TargetRef, request.Content, request.BaseHash)
	case "unified_diff":
		return applyWorkspaceUnifiedDiff(deliverableDirectory, request.TargetRef, request.Diff, request.BaseHash)
	case "text_replace":
		return applyWorkspaceTextReplace(deliverableDirectory, request)
	default:
		return common.ErrInvalidArgument
	}
}

func workspacePatchBeforeStateForRequest(deliverableDirectory string, request runtimePatchRequest) (workspacePatchBeforeState, error) {
	target, err := workspaceTargetPath(deliverableDirectory, request.TargetRef)
	if err != nil {
		return workspacePatchBeforeState{}, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
			return workspacePatchBeforeState{Exists: false}, nil
		}
		return workspacePatchBeforeState{}, err
	}
	return workspacePatchBeforeState{
		Exists:  true,
		Content: string(data),
		Hash:    sha256HexBytes(data),
	}, nil
}

func workspacePatchAfterHash(deliverableDirectory string, targetRef string) (string, error) {
	target, err := workspaceTargetPath(deliverableDirectory, targetRef)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", err
	}
	return sha256HexBytes(data), nil
}

func reverseDiffForPatch(targetRef string, originalContent string, originalExists bool) string {
	if !originalExists {
		return fmt.Sprintf("--- a/%s\n+++ /dev/null\n@@ -1,1 +0,0 @@\n-\n", filepath.ToSlash(filepath.Clean(targetRef)))
	}
	return fmt.Sprintf("--- a/%s\n+++ b/%s\n@@ -1,0 +1,%d @@\n%s",
		filepath.ToSlash(filepath.Clean(targetRef)),
		filepath.ToSlash(filepath.Clean(targetRef)),
		countTextLines(originalContent),
		prefixDiffLines(originalContent, "+"),
	)
}

func prefixDiffLines(content string, prefix string) string {
	if content == "" {
		return ""
	}
	lines, _ := splitTextLines(content)
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(prefix)
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func sha256HexBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func applyWorkspaceFileWrite(deliverableDirectory string, targetRef string, content string, baseHash string) error {
	target, err := workspaceTargetPath(deliverableDirectory, targetRef)
	if err != nil {
		return err
	}
	if err := verifyWorkspacePatchBaseHash(target, baseHash); err != nil {
		return err
	}
	if err := ensureWorkspacePatchParentDirectory(deliverableDirectory, targetRef, target, "file_write"); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(content), 0o644)
}

func applyWorkspaceTextReplace(deliverableDirectory string, request runtimePatchRequest) error {
	target, err := workspaceTargetPath(deliverableDirectory, request.TargetRef)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	if patchRepairLooksBinary(data) {
		return fmt.Errorf("%w: patch text_replace target is binary", common.ErrInvalidArgument)
	}
	if err := verifyWorkspacePatchBaseHashBytes(target, data, request.BaseHash); err != nil {
		return err
	}
	nextContent, _, err := applyExactTextReplacement(string(data), request.OldText, request.NewText, request.Occurrence)
	if err != nil {
		return err
	}
	return os.WriteFile(target, []byte(nextContent), 0o644)
}

func applyExactTextReplacement(content string, oldText string, newText string, occurrence int) (string, int, error) {
	if oldText == "" {
		return "", 0, fmt.Errorf("%w: patch oldText is required for text_replace", common.ErrInvalidArgument)
	}
	matchCount := strings.Count(content, oldText)
	if matchCount == 0 {
		return "", 0, fmt.Errorf("%w: patch text_replace oldText not found", common.ErrConflict)
	}
	if occurrence <= 0 && matchCount != 1 {
		return "", 0, fmt.Errorf("%w: patch text_replace oldText matched %d occurrences", common.ErrConflict, matchCount)
	}
	if occurrence > matchCount {
		return "", 0, fmt.Errorf("%w: patch text_replace occurrence %d exceeds %d matches", common.ErrConflict, occurrence, matchCount)
	}
	selected := occurrence
	if selected <= 0 {
		selected = 1
	}
	start := 0
	for current := 1; ; current++ {
		pos := strings.Index(content[start:], oldText)
		if pos < 0 {
			return "", 0, fmt.Errorf("%w: patch text_replace oldText not found", common.ErrConflict)
		}
		absolute := start + pos
		if current == selected {
			return content[:absolute] + newText + content[absolute+len(oldText):], matchCount, nil
		}
		start = absolute + len(oldText)
	}
}

func ensureWorkspacePatchParentDirectory(deliverableDirectory string, targetRef string, target string, operation string) error {
	base, err := filepath.Abs(deliverableDirectory)
	if err != nil {
		return err
	}
	if base, err = filepath.EvalSymlinks(base); err != nil {
		return err
	}
	parent := filepath.Dir(target)
	if parent == base {
		return nil
	}
	relParent, err := filepath.Rel(base, parent)
	if err != nil {
		return err
	}
	current := base
	for _, part := range strings.Split(relParent, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Stat(current)
		if statErr == nil {
			if !info.IsDir() {
				conflictRef, relErr := filepath.Rel(base, current)
				if relErr != nil {
					conflictRef = filepath.Clean(targetRef)
				}
				return &workspacePathConflictError{
					Operation:    operation,
					TargetRef:    filepath.ToSlash(filepath.Clean(targetRef)),
					ConflictRef:  filepath.ToSlash(conflictRef),
					ConflictKind: "file",
				}
			}
			continue
		}
		if !os.IsNotExist(statErr) {
			return statErr
		}
		break
	}
	return os.MkdirAll(parent, 0o755)
}

func workspaceTargetPath(deliverableDirectory string, targetRef string) (string, error) {
	if strings.TrimSpace(deliverableDirectory) == "" || strings.TrimSpace(targetRef) == "" {
		return "", common.ErrInvalidArgument
	}
	if filepath.IsAbs(targetRef) {
		return "", common.ErrInvalidArgument
	}
	cleanTarget := filepath.Clean(targetRef)
	if cleanTarget == "." || cleanTarget == ".." || strings.HasPrefix(cleanTarget, ".."+string(filepath.Separator)) {
		return "", common.ErrInvalidArgument
	}
	base, err := filepath.Abs(deliverableDirectory)
	if err != nil {
		return "", err
	}
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(base, cleanTarget))
	if err != nil {
		return "", err
	}
	if existing := nearestExistingWorkspacePatchPath(target); existing != "" {
		resolvedExisting, evalErr := filepath.EvalSymlinks(existing)
		if evalErr != nil {
			return "", evalErr
		}
		if !workspacePatchPathInside(base, resolvedExisting) {
			return "", common.ErrInvalidArgument
		}
		if existing == target {
			target = resolvedExisting
		}
	}
	if !workspacePatchPathInside(base, target) {
		return "", common.ErrInvalidArgument
	}
	return target, nil
}

func nearestExistingWorkspacePatchPath(path string) string {
	current := path
	for {
		if _, err := os.Lstat(current); err == nil {
			return current
		}
		next := filepath.Dir(current)
		if next == current {
			return ""
		}
		current = next
	}
}

func workspacePatchPathInside(base string, target string) bool {
	base = filepath.Clean(base)
	target = filepath.Clean(target)
	return target == base || strings.HasPrefix(target, base+string(filepath.Separator))
}

func verifyWorkspacePatchBaseHash(target string, baseHash string) error {
	baseHash = strings.TrimSpace(baseHash)
	if baseHash == "" {
		return nil
	}
	currentHash, err := fileSHA256(target)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: patch baseHash mismatch: target does not exist", common.ErrConflict)
		}
		return err
	}
	if !strings.EqualFold(currentHash, baseHash) {
		return fmt.Errorf("%w: patch baseHash mismatch", common.ErrConflict)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

var unifiedDiffHunkHeaderPattern = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

type unifiedDiffFilePatch struct {
	OldRef string
	NewRef string
	Hunks  []unifiedDiffHunk
}

type unifiedDiffHunk struct {
	OldStart    int
	HasOldStart bool
	Lines       []string
}

func applyWorkspaceUnifiedDiff(deliverableDirectory string, targetRef string, diff string, baseHash string) error {
	target, err := workspaceTargetPath(deliverableDirectory, targetRef)
	if err != nil {
		return err
	}
	filePatch, err := parseSingleFileUnifiedDiff(diff)
	if err != nil {
		return wrapPatchApplyParseError(targetRef, "unified_diff", err)
	}
	if !diffTargetMatches(targetRef, filePatch) {
		return newPatchApplyDiagnosticError(
			targetRef,
			"unified_diff",
			"patch_apply",
			common.ErrInvalidArgument,
			fmt.Sprintf("%s: unified diff target does not match targetRef", common.ErrInvalidArgument),
			"Regenerate the patch for the current targetRef and keep the diff scoped to exactly one file.",
			"unified_diff_target_mismatch",
		)
	}

	currentContent, err := os.ReadFile(target)
	if err != nil {
		if !os.IsNotExist(err) || filePatch.OldRef != "/dev/null" {
			return err
		}
		currentContent = nil
	}
	if err := verifyWorkspacePatchBaseHashBytes(target, currentContent, baseHash); err != nil {
		return err
	}
	nextContent, err := applyUnifiedDiffFilePatch(string(currentContent), filePatch)
	if err != nil {
		return wrapPatchApplyExecutionError(targetRef, "unified_diff", err)
	}
	if err := ensureWorkspacePatchParentDirectory(deliverableDirectory, targetRef, target, "unified_diff"); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(nextContent), 0o644)
}

func verifyWorkspacePatchBaseHashBytes(target string, content []byte, baseHash string) error {
	baseHash = strings.TrimSpace(baseHash)
	if baseHash == "" {
		return nil
	}
	if content == nil {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			return fmt.Errorf("%w: patch baseHash mismatch: target does not exist", common.ErrConflict)
		}
	}
	sum := sha256.Sum256(content)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), baseHash) {
		return fmt.Errorf("%w: patch baseHash mismatch", common.ErrConflict)
	}
	return nil
}

func previewWorkspacePatch(deliverableDirectory string, request runtimePatchRequest) (map[string]any, error) {
	target, err := workspaceTargetPath(deliverableDirectory, request.TargetRef)
	if err != nil {
		return nil, err
	}
	if err := verifyWorkspacePatchBaseHash(target, request.BaseHash); err != nil {
		if request.PatchType != "unified_diff" {
			return nil, err
		}
	}
	preview := map[string]any{
		"targetRef": request.TargetRef,
		"patchType": request.PatchType,
		"summary":   request.Summary,
		"baseHash":  request.BaseHash,
		"willWrite": false,
	}
	switch request.PatchType {
	case "file_write":
		if err := ensureWorkspacePatchParentDirectoryPreview(deliverableDirectory, request.TargetRef, target, "file_write"); err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(request.Content))
		preview["contentHash"] = hex.EncodeToString(sum[:])
		preview["addedLines"] = countTextLines(request.Content)
		preview["deletedLines"] = 0
		preview["bytes"] = len([]byte(request.Content))
	case "unified_diff":
		filePatch, err := parseSingleFileUnifiedDiff(request.Diff)
		if err != nil {
			return nil, err
		}
		if !diffTargetMatches(request.TargetRef, filePatch) {
			return nil, common.ErrInvalidArgument
		}
		currentContent, err := os.ReadFile(target)
		if err != nil {
			if !os.IsNotExist(err) || filePatch.OldRef != "/dev/null" {
				return nil, err
			}
			currentContent = nil
		}
		if err := verifyWorkspacePatchBaseHashBytes(target, currentContent, request.BaseHash); err != nil {
			return nil, err
		}
		if _, err := applyUnifiedDiffFilePatch(string(currentContent), filePatch); err != nil {
			return nil, err
		}
		added, deleted := unifiedDiffStats(filePatch)
		sum := sha256.Sum256([]byte(request.Diff))
		preview["diffHash"] = hex.EncodeToString(sum[:])
		preview["addedLines"] = added
		preview["deletedLines"] = deleted
	case "text_replace":
		currentContent, err := os.ReadFile(target)
		if err != nil {
			return nil, err
		}
		if patchRepairLooksBinary(currentContent) {
			return nil, fmt.Errorf("%w: patch text_replace target is binary", common.ErrInvalidArgument)
		}
		if err := verifyWorkspacePatchBaseHashBytes(target, currentContent, request.BaseHash); err != nil {
			return nil, err
		}
		nextContent, matchCount, err := applyExactTextReplacement(string(currentContent), request.OldText, request.NewText, request.Occurrence)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(nextContent))
		preview["contentHash"] = hex.EncodeToString(sum[:])
		preview["oldTextHash"] = sha256HexBytes([]byte(request.OldText))
		preview["newTextHash"] = sha256HexBytes([]byte(request.NewText))
		preview["matchCount"] = matchCount
		preview["addedLines"] = countTextLines(request.NewText)
		preview["deletedLines"] = countTextLines(request.OldText)
		preview["bytes"] = len([]byte(request.NewText))
	default:
		return nil, common.ErrInvalidArgument
	}
	return preview, nil
}

func ensureWorkspacePatchParentDirectoryPreview(deliverableDirectory string, targetRef string, target string, operation string) error {
	base, err := filepath.Abs(deliverableDirectory)
	if err != nil {
		return err
	}
	if base, err = filepath.EvalSymlinks(base); err != nil {
		return err
	}
	parent := filepath.Dir(target)
	if parent == base {
		return nil
	}
	relParent, err := filepath.Rel(base, parent)
	if err != nil {
		return err
	}
	current := base
	for _, part := range strings.Split(relParent, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Stat(current)
		if statErr == nil {
			if !info.IsDir() {
				conflictRef, relErr := filepath.Rel(base, current)
				if relErr != nil {
					conflictRef = filepath.Clean(targetRef)
				}
				return &workspacePathConflictError{
					Operation:    operation,
					TargetRef:    filepath.ToSlash(filepath.Clean(targetRef)),
					ConflictRef:  filepath.ToSlash(conflictRef),
					ConflictKind: "file",
				}
			}
			continue
		}
		if !os.IsNotExist(statErr) {
			return statErr
		}
		return nil
	}
	return nil
}

func parseSingleFileUnifiedDiff(diff string) (unifiedDiffFilePatch, error) {
	lines := splitDiffLines(diff)
	if len(lines) < 3 {
		return unifiedDiffFilePatch{}, fmt.Errorf("%w: unified diff must include file header and at least one hunk", common.ErrInvalidArgument)
	}
	if patch, ok, err := parseSingleFileApplyPatchDiff(lines); ok || err != nil {
		return patch, err
	}
	idx := 0
	for idx < len(lines) && !strings.HasPrefix(lines[idx], "--- ") {
		idx++
	}
	if idx+2 > len(lines) || !strings.HasPrefix(lines[idx], "--- ") || !strings.HasPrefix(lines[idx+1], "+++ ") {
		return unifiedDiffFilePatch{}, fmt.Errorf("%w: unified diff must begin with ---/+++ file headers", common.ErrInvalidArgument)
	}
	filePatch := unifiedDiffFilePatch{
		OldRef: normalizeUnifiedDiffPath(strings.TrimSpace(strings.TrimPrefix(lines[idx], "--- "))),
		NewRef: normalizeUnifiedDiffPath(strings.TrimSpace(strings.TrimPrefix(lines[idx+1], "+++ "))),
	}
	idx += 2
	for idx < len(lines) {
		if strings.HasPrefix(lines[idx], "--- ") {
			return unifiedDiffFilePatch{}, fmt.Errorf("%w: unified diff must contain exactly one file", common.ErrInvalidArgument)
		}
		if strings.TrimSpace(lines[idx]) == "" {
			idx++
			continue
		}
		if !isUnifiedDiffHunkHeaderLine(lines[idx]) {
			return unifiedDiffFilePatch{}, fmt.Errorf("%w: unified diff hunk header is required before changed lines", common.ErrInvalidArgument)
		}
		hunk, nextIdx, err := parseUnifiedDiffHunk(lines, idx, false)
		if err != nil {
			return unifiedDiffFilePatch{}, err
		}
		filePatch.Hunks = append(filePatch.Hunks, hunk)
		idx = nextIdx
	}
	if filePatch.NewRef == "" || len(filePatch.Hunks) == 0 {
		return unifiedDiffFilePatch{}, fmt.Errorf("%w: unified diff must include at least one non-empty hunk", common.ErrInvalidArgument)
	}
	return filePatch, nil
}

func parseSingleFileApplyPatchDiff(lines []string) (unifiedDiffFilePatch, bool, error) {
	idx := 0
	for idx < len(lines) && strings.TrimSpace(lines[idx]) == "" {
		idx++
	}
	if idx >= len(lines) || strings.TrimSpace(lines[idx]) != "*** Begin Patch" {
		return unifiedDiffFilePatch{}, false, nil
	}
	idx++
	filePatch := unifiedDiffFilePatch{}
	seenFile := false
	for idx < len(lines) {
		line := strings.TrimSpace(lines[idx])
		if line == "" {
			idx++
			continue
		}
		if line == "*** End Patch" {
			if !seenFile || filePatch.NewRef == "" || len(filePatch.Hunks) == 0 {
				return unifiedDiffFilePatch{}, true, common.ErrInvalidArgument
			}
			return filePatch, true, nil
		}
		switch {
		case strings.HasPrefix(line, "*** Update File: "):
			if seenFile {
				return unifiedDiffFilePatch{}, true, common.ErrInvalidArgument
			}
			ref := normalizeUnifiedDiffPath(strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: ")))
			if ref == "" || ref == "/dev/null" {
				return unifiedDiffFilePatch{}, true, common.ErrInvalidArgument
			}
			filePatch.OldRef = ref
			filePatch.NewRef = ref
			seenFile = true
			idx++
		case strings.HasPrefix(line, "*** Add File: "):
			if seenFile {
				return unifiedDiffFilePatch{}, true, common.ErrInvalidArgument
			}
			ref := normalizeUnifiedDiffPath(strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: ")))
			if ref == "" || ref == "/dev/null" {
				return unifiedDiffFilePatch{}, true, common.ErrInvalidArgument
			}
			filePatch.OldRef = "/dev/null"
			filePatch.NewRef = ref
			seenFile = true
			idx++
		case strings.HasPrefix(line, "*** Delete File: "):
			return unifiedDiffFilePatch{}, true, common.ErrInvalidArgument
		default:
			if !seenFile || !isUnifiedDiffHunkHeaderLine(lines[idx]) {
				return unifiedDiffFilePatch{}, true, common.ErrInvalidArgument
			}
			hunk, nextIdx, err := parseUnifiedDiffHunk(lines, idx, true)
			if err != nil {
				return unifiedDiffFilePatch{}, true, err
			}
			filePatch.Hunks = append(filePatch.Hunks, hunk)
			idx = nextIdx
		}
	}
	return unifiedDiffFilePatch{}, true, common.ErrInvalidArgument
}

func isUnifiedDiffHunkHeaderLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "@@")
}

func parseUnifiedDiffHunk(lines []string, idx int, allowBareHeader bool) (unifiedDiffHunk, int, error) {
	matches := unifiedDiffHunkHeaderPattern.FindStringSubmatch(lines[idx])
	hunk := unifiedDiffHunk{}
	if matches == nil {
		if !isUnifiedDiffHunkHeaderLine(lines[idx]) {
			return unifiedDiffHunk{}, idx, fmt.Errorf("%w: unified diff hunk header is required", common.ErrInvalidArgument)
		}
		if allowBareHeader {
			idx++
			goto parseLines
		}
		return unifiedDiffHunk{}, idx, fmt.Errorf("%w: unified diff hunk header must include explicit line ranges like @@ -12,3 +12,4 @@", common.ErrInvalidArgument)
	} else {
		oldStart, err := strconv.Atoi(matches[1])
		if err != nil {
			return unifiedDiffHunk{}, idx, fmt.Errorf("%w: unified diff hunk header line range is invalid", common.ErrInvalidArgument)
		}
		hunk.OldStart = oldStart
		hunk.HasOldStart = true
	}
	idx++
parseLines:
	for idx < len(lines) {
		line := lines[idx]
		if isUnifiedDiffHunkHeaderLine(line) || strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "*** End Patch") {
			break
		}
		if line == `\ No newline at end of file` {
			idx++
			continue
		}
		if line == "" {
			return unifiedDiffHunk{}, idx, fmt.Errorf("%w: unified diff hunk contains an empty raw line; every line must start with space, +, or -", common.ErrInvalidArgument)
		}
		switch line[0] {
		case ' ', '-', '+':
			hunk.Lines = append(hunk.Lines, line)
		default:
			return unifiedDiffHunk{}, idx, fmt.Errorf("%w: unified diff hunk line must start with space, +, or -", common.ErrInvalidArgument)
		}
		idx++
	}
	if len(hunk.Lines) == 0 {
		return unifiedDiffHunk{}, idx, fmt.Errorf("%w: unified diff hunk cannot be empty", common.ErrInvalidArgument)
	}
	return hunk, idx, nil
}

func unifiedDiffHasSubstantiveNonBlankChange(diff string) (bool, string) {
	filePatch, err := parseSingleFileUnifiedDiff(diff)
	if err != nil {
		return false, "invalid_unified_diff"
	}
	var removed []string
	var added []string
	hasNonBlankChange := false
	for _, hunk := range filePatch.Hunks {
		for _, line := range hunk.Lines {
			if line == "" {
				continue
			}
			switch line[0] {
			case '-':
				value := line[1:]
				removed = append(removed, value)
				if strings.TrimSpace(value) != "" {
					hasNonBlankChange = true
				}
			case '+':
				value := line[1:]
				added = append(added, value)
				if strings.TrimSpace(value) != "" {
					hasNonBlankChange = true
				}
			}
		}
	}
	if len(removed) == 0 && len(added) == 0 {
		return false, "no_changed_lines"
	}
	if !hasNonBlankChange {
		return false, "blank_line_only_change"
	}
	if len(removed) == len(added) {
		allSame := true
		for idx := range removed {
			if removed[idx] != added[idx] {
				allSame = false
				break
			}
		}
		if allSame {
			return false, "identical_removed_and_added_lines"
		}
	}
	return true, ""
}

func wrapPatchApplyParseError(targetRef string, patchType string, err error) error {
	if err == nil {
		return nil
	}
	errText := strings.ToLower(strings.TrimSpace(err.Error()))
	detail := "patch_parse_invalid"
	guidance := "Return a complete single-file patch with ---/+++ headers and explicit @@ -old,+new @@ hunk ranges, or use file/edit with exact oldText."
	switch {
	case strings.Contains(errText, "explicit line ranges"):
		detail = "unified_diff_missing_line_ranges"
		guidance = "Do not emit bare @@ hunks. Include explicit line ranges like @@ -12,3 +12,4 @@, or switch to file/edit with exact oldText/newText from the current snapshot."
	case strings.Contains(errText, "must begin with ---/+++"):
		detail = "unified_diff_missing_file_headers"
	case strings.Contains(errText, "exactly one file"):
		detail = "unified_diff_multiple_files"
	case strings.Contains(errText, "hunk contains an empty raw line"):
		detail = "unified_diff_invalid_blank_line"
	}
	return newPatchApplyDiagnosticError(
		targetRef,
		patchType,
		"patch_apply",
		err,
		fmt.Sprintf("%s: %v", common.ErrInvalidArgument, err),
		guidance,
		detail,
	)
}

func wrapPatchApplyExecutionError(targetRef string, patchType string, err error) error {
	if err == nil {
		return nil
	}
	errText := strings.ToLower(strings.TrimSpace(err.Error()))
	detail := "patch_apply_invalid"
	guidance := "Read the current target snapshot and regenerate the smallest exact patch; do not retry a stale diff."
	switch {
	case strings.Contains(errText, "hunk context mismatch"):
		detail = "unified_diff_hunk_context_mismatch"
		guidance = "The diff context is stale. Read the current target and rebuild the smallest exact unified_diff or use file/edit with exact oldText."
	case strings.Contains(errText, "hunk removal mismatch"):
		detail = "unified_diff_hunk_removal_mismatch"
		guidance = "The removed lines no longer match the current file. Re-read the target and patch only the exact visible lines."
	case strings.Contains(errText, "basehash mismatch"):
		detail = "patch_basehash_mismatch"
		guidance = "The target changed since the previous snapshot. Use the current contentHash from patchRepairContext as baseHash or omit baseHash."
	case strings.Contains(errText, "start out of range"):
		detail = "unified_diff_hunk_start_out_of_range"
	case errors.Is(err, common.ErrConflict):
		detail = "patch_apply_conflict"
	}
	return newPatchApplyDiagnosticError(
		targetRef,
		patchType,
		"patch_apply",
		err,
		fmt.Sprintf("%s: %v", common.ErrInvalidArgument, err),
		guidance,
		detail,
	)
}

func applyUnifiedDiffFilePatch(content string, filePatch unifiedDiffFilePatch) (string, error) {
	sourceLines, sourceHadFinalNewline := splitTextLines(content)
	output := make([]string, 0, len(sourceLines))
	cursor := 0
	for _, hunk := range filePatch.Hunks {
		start, err := unifiedDiffHunkStart(sourceLines, cursor, hunk)
		if err != nil {
			return "", err
		}
		if start < cursor || start > len(sourceLines) {
			return "", fmt.Errorf("%w: unified diff hunk start out of range", common.ErrInvalidArgument)
		}
		output = append(output, sourceLines[cursor:start]...)
		cursor = start
		for _, diffLine := range hunk.Lines {
			switch diffLine[0] {
			case ' ':
				if cursor >= len(sourceLines) || sourceLines[cursor] != diffLine[1:] {
					return "", fmt.Errorf("%w: unified diff hunk context mismatch", common.ErrInvalidArgument)
				}
				output = append(output, sourceLines[cursor])
				cursor++
			case '-':
				if cursor >= len(sourceLines) || sourceLines[cursor] != diffLine[1:] {
					return "", fmt.Errorf("%w: unified diff hunk removal mismatch", common.ErrInvalidArgument)
				}
				cursor++
			case '+':
				output = append(output, diffLine[1:])
			}
		}
	}
	output = append(output, sourceLines[cursor:]...)
	return joinTextLines(output, sourceHadFinalNewline || filePatch.OldRef == "/dev/null"), nil
}

func unifiedDiffHunkStart(sourceLines []string, cursor int, hunk unifiedDiffHunk) (int, error) {
	if hunk.HasOldStart {
		start := hunk.OldStart - 1
		if start < 0 {
			start = 0
		}
		oldLines := unifiedDiffHunkOldLines(hunk)
		if len(oldLines) == 0 || (start >= cursor && start+len(oldLines) <= len(sourceLines) && unifiedDiffOldLinesMatch(sourceLines[start:start+len(oldLines)], oldLines)) {
			return start, nil
		}
		if matchedStart, err := unifiedDiffHunkSearchStart(sourceLines, cursor, oldLines); err == nil {
			return matchedStart, nil
		}
		return start, nil
	}
	oldLines := unifiedDiffHunkOldLines(hunk)
	return unifiedDiffHunkSearchStart(sourceLines, cursor, oldLines)
}

func unifiedDiffHunkOldLines(hunk unifiedDiffHunk) []string {
	oldLines := make([]string, 0, len(hunk.Lines))
	for _, line := range hunk.Lines {
		if line == "" {
			return nil
		}
		switch line[0] {
		case ' ', '-':
			oldLines = append(oldLines, line[1:])
		}
	}
	return oldLines
}

func unifiedDiffHunkSearchStart(sourceLines []string, cursor int, oldLines []string) (int, error) {
	if len(oldLines) == 0 {
		if len(sourceLines) == 0 {
			return 0, nil
		}
		return 0, common.ErrInvalidArgument
	}
	matches := 0
	matchedStart := -1
	for start := cursor; start+len(oldLines) <= len(sourceLines); start++ {
		if unifiedDiffOldLinesMatch(sourceLines[start:start+len(oldLines)], oldLines) {
			matches++
			matchedStart = start
			if matches > 1 {
				return 0, common.ErrConflict
			}
		}
	}
	if matches == 0 {
		return 0, common.ErrInvalidArgument
	}
	return matchedStart, nil
}

func unifiedDiffOldLinesMatch(sourceLines []string, oldLines []string) bool {
	if len(sourceLines) != len(oldLines) {
		return false
	}
	for idx := range oldLines {
		if sourceLines[idx] != oldLines[idx] {
			return false
		}
	}
	return true
}

func splitDiffLines(diff string) []string {
	normalized := strings.ReplaceAll(diff, "\r\n", "\n")
	normalized = strings.TrimSuffix(normalized, "\n")
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, "\n")
}

func splitTextLines(content string) ([]string, bool) {
	if content == "" {
		return nil, false
	}
	hadFinalNewline := strings.HasSuffix(content, "\n")
	trimmed := strings.TrimSuffix(content, "\n")
	if trimmed == "" {
		return []string{""}, hadFinalNewline
	}
	return strings.Split(trimmed, "\n"), hadFinalNewline
}

func joinTextLines(lines []string, finalNewline bool) string {
	if len(lines) == 0 {
		return ""
	}
	ret := strings.Join(lines, "\n")
	if finalNewline {
		ret += "\n"
	}
	return ret
}

func countTextLines(content string) int {
	if content == "" {
		return 0
	}
	count := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		count++
	}
	return count
}

func unifiedDiffStats(filePatch unifiedDiffFilePatch) (int, int) {
	added := 0
	deleted := 0
	for _, hunk := range filePatch.Hunks {
		for _, line := range hunk.Lines {
			if line == "" {
				continue
			}
			switch line[0] {
			case '+':
				added++
			case '-':
				deleted++
			}
		}
	}
	return added, deleted
}

func normalizeUnifiedDiffPath(input string) string {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return ""
	}
	input = fields[0]
	if input == "/dev/null" {
		return input
	}
	input = strings.TrimPrefix(input, "a/")
	input = strings.TrimPrefix(input, "b/")
	return filepath.Clean(input)
}

func diffTargetMatches(targetRef string, filePatch unifiedDiffFilePatch) bool {
	cleanTarget := filepath.Clean(targetRef)
	if filePatch.NewRef != "/dev/null" {
		return cleanTarget == filePatch.NewRef
	}
	return cleanTarget == filePatch.OldRef
}
