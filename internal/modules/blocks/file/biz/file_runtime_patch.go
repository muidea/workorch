package file

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	patchTypeFileWrite   = "file_write"
	patchTypeTextReplace = "text_replace"
	patchTypeUnifiedDiff = "unified_diff"
)

var patchUnifiedDiffHunkHeaderPattern = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

type patchExecutionRequest struct {
	Operation       string
	PatchType       string
	TargetRef       string
	Summary         string
	Content         string
	Diff            string
	OldText         string
	NewText         string
	BaseHash        string
	Occurrence      int
	CoverageTargets any
}

type patchUnifiedDiffFilePatch struct {
	OldRef string
	NewRef string
	Hunks  []patchUnifiedDiffHunk
}

type patchUnifiedDiffHunk struct {
	OldStart    int
	HasOldStart bool
	Lines       []string
}

type patchParentConflictError struct {
	Operation    string
	TargetRef    string
	ConflictRef  string
	ConflictKind string
}

func (e *patchParentConflictError) Error() string {
	if e == nil {
		return ""
	}
	kind := strings.TrimSpace(e.ConflictKind)
	if kind == "" {
		kind = "file"
	}
	return fmt.Sprintf("%s parent path %s is %s", e.Operation, e.ConflictRef, kind)
}

func (e *patchParentConflictError) Unwrap() error {
	return common.ErrConflict
}

func (a *RuntimeAdapter) patchPreview(request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	return a.executePatch(request, "preview", false)
}

func (a *RuntimeAdapter) patchApply(request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	return a.executePatch(request, "apply", true)
}

func (a *RuntimeAdapter) executePatch(request runtimemodel.StepExecutionRequest, operation string, apply bool) (runtimemodel.StepExecutionResult, error) {
	patchReq, err := patchRequestFromInput(request.Input, operation)
	if err != nil {
		return patchErrorResult(patchStage(operation), operation, "", "", err, "patch_invalid_input"), err
	}
	root, target, rel, err := guardedWorkspacePath(request.WorkspaceDeliverableDirectory, patchReq.TargetRef, false)
	if err != nil {
		return patchErrorResult(patchStage(operation), operation, patchReq.PatchType, patchReq.TargetRef, err, "patch_invalid_path"), err
	}
	patchReq.TargetRef = filepath.ToSlash(rel)

	switch patchReq.PatchType {
	case patchTypeFileWrite:
		return executeFileWritePatch(root, target, patchReq, apply)
	case patchTypeTextReplace:
		return executeTextReplacePatch(root, target, patchReq, apply)
	case patchTypeUnifiedDiff:
		return executeUnifiedDiffPatch(root, target, patchReq, apply)
	default:
		err := fmt.Errorf("%w: unsupported patchType %q", common.ErrInvalidArgument, patchReq.PatchType)
		return patchErrorResult(patchStage(operation), patchReq.Operation, patchReq.PatchType, patchReq.TargetRef, err, "patch_type_unsupported"), err
	}
}

func patchRequestFromInput(input map[string]any, operation string) (patchExecutionRequest, error) {
	if strings.TrimSpace(operation) == "" {
		return patchExecutionRequest{}, common.ErrInvalidArgument
	}
	req := patchExecutionRequest{
		Operation:  operation,
		PatchType:  stringInput(input, "patchType"),
		TargetRef:  stringInput(input, "targetRef"),
		Summary:    stringInput(input, "summary"),
		Content:    rawStringInput(input, "content"),
		Diff:       rawStringInput(input, "diff"),
		OldText:    rawStringInput(input, "oldText"),
		NewText:    rawStringInput(input, "newText"),
		BaseHash:   stringInput(input, "baseHash"),
		Occurrence: intInput(input, "occurrence", 0),
	}
	if input != nil {
		req.CoverageTargets = input["coverageTargets"]
	}
	if req.TargetRef == "" {
		req.TargetRef = stringInput(input, "path")
	}
	if req.PatchType == "" {
		switch {
		case req.Diff != "":
			req.PatchType = patchTypeUnifiedDiff
		case req.OldText != "":
			req.PatchType = patchTypeTextReplace
		case input != nil && input["content"] != nil:
			req.PatchType = patchTypeFileWrite
		}
	}
	if req.TargetRef == "" || req.PatchType == "" {
		return patchExecutionRequest{}, common.ErrInvalidArgument
	}
	return req, nil
}

func executeFileWritePatch(root string, target string, req patchExecutionRequest, apply bool) (runtimemodel.StepExecutionResult, error) {
	if err := ensurePatchParentDirectory(root, target, req.TargetRef, req.PatchType, apply); err != nil {
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_parent_conflict"), err
	}
	before, exists, err := readPatchTarget(target)
	if err != nil {
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_read_failed"), err
	}
	if exists && looksBinary(before) {
		err := fmt.Errorf("%w: patch target is binary", common.ErrInvalidArgument)
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_binary_target"), err
	}
	if err := verifyPatchBaseHash(target, before, exists, req.BaseHash); err != nil {
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_basehash_mismatch"), err
	}
	after := []byte(req.Content)
	if apply {
		if err := os.WriteFile(target, after, 0o644); err != nil {
			return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_write_failed"), err
		}
	}
	addedLines, deletedLines := patchChangedLineStats(string(before), string(after), exists)
	return patchSuccessResult(req, before, after, exists, sha256Hex(after), "", addedLines, deletedLines), nil
}

func executeTextReplacePatch(root string, target string, req patchExecutionRequest, apply bool) (runtimemodel.StepExecutionResult, error) {
	_ = root
	before, exists, err := readPatchTarget(target)
	if err != nil || !exists {
		if err == nil {
			err = fmt.Errorf("%w: patch text_replace target does not exist", common.ErrNotFound)
		}
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_read_failed"), err
	}
	if looksBinary(before) {
		err := fmt.Errorf("%w: patch text_replace target is binary", common.ErrInvalidArgument)
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_binary_target"), err
	}
	if err := verifyPatchBaseHash(target, before, exists, req.BaseHash); err != nil {
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_basehash_mismatch"), err
	}
	next, matchCount, err := applyPatchTextReplacement(string(before), req.OldText, req.NewText, req.Occurrence)
	if err != nil {
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_text_replace_conflict"), err
	}
	after := []byte(next)
	if apply {
		if err := os.WriteFile(target, after, 0o644); err != nil {
			return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_write_failed"), err
		}
	}
	addedLines, deletedLines := patchChangedLineStats(req.OldText, req.NewText, true)
	result := patchSuccessResult(req, before, after, exists, sha256Hex(after), "", addedLines, deletedLines)
	result.Output["matchCount"] = matchCount
	return result, nil
}

func executeUnifiedDiffPatch(root string, target string, req patchExecutionRequest, apply bool) (runtimemodel.StepExecutionResult, error) {
	filePatch, err := parsePatchUnifiedDiff(req.Diff)
	if err != nil {
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "unified_diff_invalid"), err
	}
	if !patchDiffTargetMatches(req.TargetRef, filePatch) {
		err := fmt.Errorf("%w: unified diff target does not match targetRef", common.ErrInvalidArgument)
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "unified_diff_target_mismatch"), err
	}
	if err := ensurePatchParentDirectory(root, target, req.TargetRef, req.PatchType, apply); err != nil {
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_parent_conflict"), err
	}
	before, exists, err := readPatchTarget(target)
	if err != nil {
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_read_failed"), err
	}
	if !exists && filePatch.OldRef != "/dev/null" {
		err := fmt.Errorf("%w: unified diff target does not exist", common.ErrNotFound)
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_target_missing"), err
	}
	if exists && filePatch.OldRef == "/dev/null" {
		err := fmt.Errorf("%w: unified diff add-file target already exists", common.ErrConflict)
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_target_exists"), err
	}
	if exists && looksBinary(before) {
		err := fmt.Errorf("%w: patch target is binary", common.ErrInvalidArgument)
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_binary_target"), err
	}
	if err := verifyPatchBaseHash(target, before, exists, req.BaseHash); err != nil {
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_basehash_mismatch"), err
	}
	next, err := applyPatchUnifiedDiff(string(before), filePatch)
	if err != nil {
		return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "unified_diff_apply_conflict"), err
	}
	after := []byte(next)
	if apply {
		if err := os.WriteFile(target, after, 0o644); err != nil {
			return patchErrorResult(patchStage(req.Operation), req.Operation, req.PatchType, req.TargetRef, err, "patch_write_failed"), err
		}
	}
	added, deleted := patchUnifiedDiffStats(filePatch)
	return patchSuccessResult(req, before, after, exists, "", sha256Hex([]byte(req.Diff)), added, deleted), nil
}

func readPatchTarget(target string) ([]byte, bool, error) {
	data, err := os.ReadFile(target)
	if err == nil {
		return data, true, nil
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return nil, false, err
}

func verifyPatchBaseHash(target string, content []byte, exists bool, baseHash string) error {
	_ = target
	baseHash = strings.TrimSpace(baseHash)
	if baseHash == "" {
		return nil
	}
	if !exists {
		return fmt.Errorf("%w: patch baseHash mismatch: target does not exist", common.ErrConflict)
	}
	if !strings.EqualFold(sha256Hex(content), baseHash) {
		return fmt.Errorf("%w: patch baseHash mismatch", common.ErrConflict)
	}
	return nil
}

func ensurePatchParentDirectory(root string, target string, targetRef string, operation string, create bool) error {
	parent := filepath.Dir(target)
	if parent == root {
		return nil
	}
	relParent, err := filepath.Rel(root, parent)
	if err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(relParent, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Stat(current)
		if statErr == nil {
			if !info.IsDir() {
				conflictRef, relErr := filepath.Rel(root, current)
				if relErr != nil {
					conflictRef = targetRef
				}
				return &patchParentConflictError{
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
		if !create {
			return nil
		}
		break
	}
	if !create {
		return nil
	}
	return os.MkdirAll(parent, 0o755)
}

func patchSuccessResult(req patchExecutionRequest, before []byte, after []byte, beforeExists bool, contentHash string, diffHash string, addedLines int, deletedLines int) runtimemodel.StepExecutionResult {
	beforeHash := ""
	if beforeExists {
		beforeHash = sha256Hex(before)
	}
	afterHash := sha256Hex(after)
	changedLines := addedLines + deletedLines
	approvalRequired := editApprovalRequired(req.TargetRef, changedLines)
	riskLevel := "low"
	var riskReasons []string
	if approvalRequired {
		riskLevel = "high"
		riskReasons = append(riskReasons, "approval_required")
	}
	output := map[string]any{
		"status":           "completed",
		"stage":            patchStage(req.Operation),
		"operation":        req.Operation,
		"patchType":        req.PatchType,
		"targetRef":        req.TargetRef,
		"affectedFiles":    []string{req.TargetRef},
		"beforeHash":       beforeHash,
		"afterHash":        afterHash,
		"addedLines":       addedLines,
		"deletedLines":     deletedLines,
		"approvalRequired": approvalRequired,
		"riskLevel":        riskLevel,
		"riskReasons":      riskReasons,
		"willWrite":        req.Operation == "apply",
	}
	if req.Summary != "" {
		output["summary"] = req.Summary
	}
	if contentHash != "" {
		output["contentHash"] = contentHash
	}
	if diffHash != "" {
		output["diffHash"] = diffHash
	}
	if req.BaseHash != "" {
		output["baseHash"] = req.BaseHash
	}
	if req.CoverageTargets != nil {
		output["coverageTargets"] = req.CoverageTargets
	}
	return runtimemodel.StepExecutionResult{Output: output}
}

func patchErrorResult(stage string, operation string, patchType string, targetRef string, err error, code string) runtimemodel.StepExecutionResult {
	diagnostic := map[string]any{
		"code":    code,
		"message": err.Error(),
	}
	output := map[string]any{
		"status":      "failed",
		"stage":       stage,
		"operation":   operation,
		"patchType":   patchType,
		"targetRef":   targetRef,
		"error":       err.Error(),
		"diagnostics": []map[string]any{diagnostic},
	}
	if errors.Is(err, common.ErrConflict) {
		output["conflict"] = map[string]any{
			"code":    code,
			"message": err.Error(),
		}
	}
	var parentConflict *patchParentConflictError
	if errors.As(err, &parentConflict) {
		output["runtimeDiagnostics"] = map[string]any{
			"kind":         "workspace_path_conflict",
			"operation":    parentConflict.Operation,
			"targetRef":    parentConflict.TargetRef,
			"conflictRef":  parentConflict.ConflictRef,
			"conflictKind": parentConflict.ConflictKind,
			"guidance":     "Choose a targetRef whose parent path is a directory, or write the resource at the existing workspace root. Do not treat an existing file as a directory.",
		}
	}
	return runtimemodel.StepExecutionResult{Output: output}
}

func patchStage(operation string) string {
	switch operation {
	case "apply":
		return "patch_apply"
	case "revert":
		return "patch_revert"
	default:
		return "patch_preview"
	}
}

func patchChangedLineStats(before string, after string, beforeExists bool) (int, int) {
	if !beforeExists {
		return patchLineCount(after), 0
	}
	return patchLineCount(after), patchLineCount(before)
}

func patchLineCount(content string) int {
	if content == "" {
		return 0
	}
	count := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		count++
	}
	return count
}

func applyPatchTextReplacement(content string, oldText string, newText string, occurrence int) (string, int, error) {
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
	return replaceOccurrence(content, oldText, newText, selected), matchCount, nil
}

func parsePatchUnifiedDiff(diff string) (patchUnifiedDiffFilePatch, error) {
	lines := splitPatchDiffLines(diff)
	if len(lines) < 3 {
		return patchUnifiedDiffFilePatch{}, fmt.Errorf("%w: unified diff must include file header and at least one hunk", common.ErrInvalidArgument)
	}
	if patch, ok, err := parsePatchApplyPatchDiff(lines); ok || err != nil {
		return patch, err
	}
	idx := 0
	for idx < len(lines) && !strings.HasPrefix(lines[idx], "--- ") {
		idx++
	}
	if idx+2 > len(lines) || !strings.HasPrefix(lines[idx], "--- ") || !strings.HasPrefix(lines[idx+1], "+++ ") {
		return patchUnifiedDiffFilePatch{}, fmt.Errorf("%w: unified diff must begin with ---/+++ file headers", common.ErrInvalidArgument)
	}
	filePatch := patchUnifiedDiffFilePatch{
		OldRef: normalizePatchUnifiedDiffPath(strings.TrimSpace(strings.TrimPrefix(lines[idx], "--- "))),
		NewRef: normalizePatchUnifiedDiffPath(strings.TrimSpace(strings.TrimPrefix(lines[idx+1], "+++ "))),
	}
	idx += 2
	for idx < len(lines) {
		if strings.HasPrefix(lines[idx], "--- ") {
			return patchUnifiedDiffFilePatch{}, fmt.Errorf("%w: unified diff must contain exactly one file", common.ErrInvalidArgument)
		}
		if strings.TrimSpace(lines[idx]) == "" {
			idx++
			continue
		}
		if !isPatchUnifiedDiffHunkHeader(lines[idx]) {
			return patchUnifiedDiffFilePatch{}, fmt.Errorf("%w: unified diff hunk header is required before changed lines", common.ErrInvalidArgument)
		}
		hunk, nextIdx, err := parsePatchUnifiedDiffHunk(lines, idx, false)
		if err != nil {
			return patchUnifiedDiffFilePatch{}, err
		}
		filePatch.Hunks = append(filePatch.Hunks, hunk)
		idx = nextIdx
	}
	if filePatch.NewRef == "" || len(filePatch.Hunks) == 0 {
		return patchUnifiedDiffFilePatch{}, fmt.Errorf("%w: unified diff must include at least one non-empty hunk", common.ErrInvalidArgument)
	}
	return filePatch, nil
}

func parsePatchApplyPatchDiff(lines []string) (patchUnifiedDiffFilePatch, bool, error) {
	idx := 0
	for idx < len(lines) && strings.TrimSpace(lines[idx]) == "" {
		idx++
	}
	if idx >= len(lines) || strings.TrimSpace(lines[idx]) != "*** Begin Patch" {
		return patchUnifiedDiffFilePatch{}, false, nil
	}
	idx++
	filePatch := patchUnifiedDiffFilePatch{}
	seenFile := false
	for idx < len(lines) {
		line := strings.TrimSpace(lines[idx])
		if line == "" {
			idx++
			continue
		}
		if line == "*** End Patch" {
			if !seenFile || filePatch.NewRef == "" || len(filePatch.Hunks) == 0 {
				return patchUnifiedDiffFilePatch{}, true, common.ErrInvalidArgument
			}
			return filePatch, true, nil
		}
		switch {
		case strings.HasPrefix(line, "*** Update File: "):
			if seenFile {
				return patchUnifiedDiffFilePatch{}, true, common.ErrInvalidArgument
			}
			ref := normalizePatchUnifiedDiffPath(strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: ")))
			if ref == "" || ref == "/dev/null" {
				return patchUnifiedDiffFilePatch{}, true, common.ErrInvalidArgument
			}
			filePatch.OldRef = ref
			filePatch.NewRef = ref
			seenFile = true
			idx++
		case strings.HasPrefix(line, "*** Add File: "):
			if seenFile {
				return patchUnifiedDiffFilePatch{}, true, common.ErrInvalidArgument
			}
			ref := normalizePatchUnifiedDiffPath(strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: ")))
			if ref == "" || ref == "/dev/null" {
				return patchUnifiedDiffFilePatch{}, true, common.ErrInvalidArgument
			}
			filePatch.OldRef = "/dev/null"
			filePatch.NewRef = ref
			seenFile = true
			idx++
		case strings.HasPrefix(line, "*** Delete File: "):
			return patchUnifiedDiffFilePatch{}, true, common.ErrInvalidArgument
		default:
			if !seenFile || !isPatchUnifiedDiffHunkHeader(lines[idx]) {
				return patchUnifiedDiffFilePatch{}, true, common.ErrInvalidArgument
			}
			hunk, nextIdx, err := parsePatchUnifiedDiffHunk(lines, idx, true)
			if err != nil {
				return patchUnifiedDiffFilePatch{}, true, err
			}
			filePatch.Hunks = append(filePatch.Hunks, hunk)
			idx = nextIdx
		}
	}
	return patchUnifiedDiffFilePatch{}, true, common.ErrInvalidArgument
}

func isPatchUnifiedDiffHunkHeader(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "@@")
}

func parsePatchUnifiedDiffHunk(lines []string, idx int, allowBareHeader bool) (patchUnifiedDiffHunk, int, error) {
	matches := patchUnifiedDiffHunkHeaderPattern.FindStringSubmatch(lines[idx])
	hunk := patchUnifiedDiffHunk{}
	if matches == nil {
		if !isPatchUnifiedDiffHunkHeader(lines[idx]) {
			return patchUnifiedDiffHunk{}, idx, fmt.Errorf("%w: unified diff hunk header is required", common.ErrInvalidArgument)
		}
		if !allowBareHeader {
			return patchUnifiedDiffHunk{}, idx, fmt.Errorf("%w: unified diff hunk header must include explicit line ranges like @@ -12,3 +12,4 @@", common.ErrInvalidArgument)
		}
	} else {
		oldStart, err := strconv.Atoi(matches[1])
		if err != nil {
			return patchUnifiedDiffHunk{}, idx, fmt.Errorf("%w: unified diff hunk header line range is invalid", common.ErrInvalidArgument)
		}
		hunk.OldStart = oldStart
		hunk.HasOldStart = true
	}
	idx++
	for idx < len(lines) {
		line := lines[idx]
		if isPatchUnifiedDiffHunkHeader(line) || strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "*** End Patch") {
			break
		}
		if line == `\ No newline at end of file` {
			idx++
			continue
		}
		if line == "" {
			return patchUnifiedDiffHunk{}, idx, fmt.Errorf("%w: unified diff hunk contains an empty raw line; every line must start with space, +, or -", common.ErrInvalidArgument)
		}
		switch line[0] {
		case ' ', '-', '+':
			hunk.Lines = append(hunk.Lines, line)
		default:
			return patchUnifiedDiffHunk{}, idx, fmt.Errorf("%w: unified diff hunk line must start with space, +, or -", common.ErrInvalidArgument)
		}
		idx++
	}
	if len(hunk.Lines) == 0 {
		return patchUnifiedDiffHunk{}, idx, fmt.Errorf("%w: unified diff hunk cannot be empty", common.ErrInvalidArgument)
	}
	return hunk, idx, nil
}

func applyPatchUnifiedDiff(content string, filePatch patchUnifiedDiffFilePatch) (string, error) {
	sourceLines, sourceHadFinalNewline := splitPatchTextLines(content)
	output := make([]string, 0, len(sourceLines))
	cursor := 0
	for _, hunk := range filePatch.Hunks {
		start, err := patchUnifiedDiffHunkStart(sourceLines, cursor, hunk)
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
	return joinPatchTextLines(output, sourceHadFinalNewline || filePatch.OldRef == "/dev/null"), nil
}

func patchUnifiedDiffHunkStart(sourceLines []string, cursor int, hunk patchUnifiedDiffHunk) (int, error) {
	if hunk.HasOldStart {
		start := hunk.OldStart - 1
		if start < 0 {
			start = 0
		}
		oldLines := patchUnifiedDiffHunkOldLines(hunk)
		if len(oldLines) == 0 || (start >= cursor && start+len(oldLines) <= len(sourceLines) && patchUnifiedDiffOldLinesMatch(sourceLines[start:start+len(oldLines)], oldLines)) {
			return start, nil
		}
		if matchedStart, err := patchUnifiedDiffHunkSearchStart(sourceLines, cursor, oldLines); err == nil {
			return matchedStart, nil
		}
		return start, nil
	}
	return patchUnifiedDiffHunkSearchStart(sourceLines, cursor, patchUnifiedDiffHunkOldLines(hunk))
}

func patchUnifiedDiffHunkOldLines(hunk patchUnifiedDiffHunk) []string {
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

func patchUnifiedDiffHunkSearchStart(sourceLines []string, cursor int, oldLines []string) (int, error) {
	if len(oldLines) == 0 {
		if len(sourceLines) == 0 {
			return 0, nil
		}
		return 0, common.ErrInvalidArgument
	}
	matches := 0
	matchedStart := -1
	for start := cursor; start+len(oldLines) <= len(sourceLines); start++ {
		if patchUnifiedDiffOldLinesMatch(sourceLines[start:start+len(oldLines)], oldLines) {
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

func patchUnifiedDiffOldLinesMatch(sourceLines []string, oldLines []string) bool {
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

func splitPatchDiffLines(diff string) []string {
	normalized := strings.ReplaceAll(diff, "\r\n", "\n")
	normalized = strings.TrimSuffix(normalized, "\n")
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, "\n")
}

func splitPatchTextLines(content string) ([]string, bool) {
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

func joinPatchTextLines(lines []string, finalNewline bool) string {
	if len(lines) == 0 {
		return ""
	}
	ret := strings.Join(lines, "\n")
	if finalNewline {
		ret += "\n"
	}
	return ret
}

func patchUnifiedDiffStats(filePatch patchUnifiedDiffFilePatch) (int, int) {
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

func normalizePatchUnifiedDiffPath(input string) string {
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

func patchDiffTargetMatches(targetRef string, filePatch patchUnifiedDiffFilePatch) bool {
	cleanTarget := filepath.Clean(targetRef)
	if filePatch.NewRef != "/dev/null" {
		return cleanTarget == filePatch.NewRef
	}
	return cleanTarget == filePatch.OldRef
}
