package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type runtimePatchRequest struct {
	TargetRef string
	Content   string
	Diff      string
	Summary   string
	PatchType string
}

type runtimeFailureDiagnosticError interface {
	RuntimeFailureDiagnostics() map[string]any
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
		"guidance":     "Choose a targetRef whose parent path is a directory, or write the file at the existing project root. Do not treat an existing file as a directory.",
	}
}

func (o *RuntimeOrchestrator) applyRuntimePatches(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	output map[string]any,
	workingDirectory string,
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
			ID:          patchID,
			WorkspaceID: run.WorkspaceID,
			RunID:       run.ID,
			StepID:      step.ID,
			ToolCallID:  toolCall.ID,
			PatchType:   request.PatchType,
			TargetRef:   request.TargetRef,
			Content:     request.Content,
			Diff:        request.Diff,
			Summary:     request.Summary,
			State:       common.PatchStateCreated,
			CreatedAt:   runtimeEvidenceTimestamp(),
		}
		if err := o.evidence.Patches.Create(ctx, patch); err != nil {
			if err != common.ErrConflict {
				return err
			}
			if err := o.evidence.Patches.Update(ctx, patch); err != nil {
				return err
			}
		}
		if err := applyWorkspacePatch(workingDirectory, request); err != nil {
			patch.State = common.PatchStateFailed
			_ = o.evidence.Patches.Update(ctx, patch)
			return err
		}
		patch.State = common.PatchStateApplied
		patch.AppliedAt = runtimeEvidenceTimestamp()
		if err := o.evidence.Patches.Update(ctx, patch); err != nil {
			return err
		}
	}
	return nil
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
	summary, _ := item["summary"].(string)
	patchType, _ := item["patchType"].(string)
	operation, _ := item["operation"].(string)
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" {
		return runtimePatchRequest{}, common.ErrInvalidArgument
	}
	if isReservedWorkspacePatchTarget(targetRef) {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch targetRef %q is reserved for WorkOrch-managed artifacts; return final query/report content in dodAssessment instead of writing report-only deliverables into the workspace", common.ErrInvalidArgument, targetRef)
	}
	if patchType == "" {
		patchType = strings.TrimSpace(operation)
	}
	if patchType == "" {
		patchType = "file_write"
	}
	switch patchType {
	case "create", "write", "replace", "file":
		patchType = "file_write"
	}
	if patchType != "file_write" && patchType != "unified_diff" {
		return runtimePatchRequest{}, common.ErrInvalidArgument
	}
	if patchType == "unified_diff" && strings.TrimSpace(diff) == "" {
		return runtimePatchRequest{}, common.ErrInvalidArgument
	}
	if patchType == "file_write" && containsUnresolvedRuntimeTemplate(content) {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch content contains unresolved runtime template placeholders", common.ErrInvalidArgument)
	}
	if patchType == "unified_diff" && containsUnresolvedRuntimeTemplate(diff) {
		return runtimePatchRequest{}, fmt.Errorf("%w: patch diff contains unresolved runtime template placeholders", common.ErrInvalidArgument)
	}
	if summary == "" {
		summary = "workspace patch apply"
	}
	return runtimePatchRequest{
		TargetRef: targetRef,
		Content:   content,
		Diff:      diff,
		Summary:   summary,
		PatchType: patchType,
	}, nil
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
	return strings.Contains(value, "{{") && strings.Contains(value, "}}")
}

func applyWorkspacePatch(workingDirectory string, request runtimePatchRequest) error {
	switch request.PatchType {
	case "file_write":
		return applyWorkspaceFileWrite(workingDirectory, request.TargetRef, request.Content)
	case "unified_diff":
		return applyWorkspaceUnifiedDiff(workingDirectory, request.TargetRef, request.Diff)
	default:
		return common.ErrInvalidArgument
	}
}

func applyWorkspaceFileWrite(workingDirectory string, targetRef string, content string) error {
	target, err := workspaceTargetPath(workingDirectory, targetRef)
	if err != nil {
		return err
	}
	if err := ensureWorkspacePatchParentDirectory(workingDirectory, targetRef, target, "file_write"); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(content), 0o644)
}

func ensureWorkspacePatchParentDirectory(workingDirectory string, targetRef string, target string, operation string) error {
	base, err := filepath.Abs(workingDirectory)
	if err != nil {
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

func workspaceTargetPath(workingDirectory string, targetRef string) (string, error) {
	if strings.TrimSpace(workingDirectory) == "" || strings.TrimSpace(targetRef) == "" {
		return "", common.ErrInvalidArgument
	}
	if filepath.IsAbs(targetRef) {
		return "", common.ErrInvalidArgument
	}
	cleanTarget := filepath.Clean(targetRef)
	if cleanTarget == "." || cleanTarget == ".." || strings.HasPrefix(cleanTarget, ".."+string(filepath.Separator)) {
		return "", common.ErrInvalidArgument
	}
	base, err := filepath.Abs(workingDirectory)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(base, cleanTarget))
	if err != nil {
		return "", err
	}
	if target != base && !strings.HasPrefix(target, base+string(filepath.Separator)) {
		return "", common.ErrInvalidArgument
	}
	return target, nil
}

var unifiedDiffHunkHeaderPattern = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

type unifiedDiffFilePatch struct {
	OldRef string
	NewRef string
	Hunks  []unifiedDiffHunk
}

type unifiedDiffHunk struct {
	OldStart int
	Lines    []string
}

func applyWorkspaceUnifiedDiff(workingDirectory string, targetRef string, diff string) error {
	target, err := workspaceTargetPath(workingDirectory, targetRef)
	if err != nil {
		return err
	}
	filePatch, err := parseSingleFileUnifiedDiff(diff)
	if err != nil {
		return err
	}
	if !diffTargetMatches(targetRef, filePatch) {
		return common.ErrInvalidArgument
	}

	currentContent, err := os.ReadFile(target)
	if err != nil {
		if !os.IsNotExist(err) || filePatch.OldRef != "/dev/null" {
			return err
		}
		currentContent = nil
	}
	nextContent, err := applyUnifiedDiffFilePatch(string(currentContent), filePatch)
	if err != nil {
		return err
	}
	if err := ensureWorkspacePatchParentDirectory(workingDirectory, targetRef, target, "unified_diff"); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(nextContent), 0o644)
}

func parseSingleFileUnifiedDiff(diff string) (unifiedDiffFilePatch, error) {
	lines := splitDiffLines(diff)
	if len(lines) < 3 {
		return unifiedDiffFilePatch{}, common.ErrInvalidArgument
	}
	idx := 0
	for idx < len(lines) && !strings.HasPrefix(lines[idx], "--- ") {
		idx++
	}
	if idx+2 > len(lines) || !strings.HasPrefix(lines[idx], "--- ") || !strings.HasPrefix(lines[idx+1], "+++ ") {
		return unifiedDiffFilePatch{}, common.ErrInvalidArgument
	}
	filePatch := unifiedDiffFilePatch{
		OldRef: normalizeUnifiedDiffPath(strings.TrimSpace(strings.TrimPrefix(lines[idx], "--- "))),
		NewRef: normalizeUnifiedDiffPath(strings.TrimSpace(strings.TrimPrefix(lines[idx+1], "+++ "))),
	}
	idx += 2
	for idx < len(lines) {
		if strings.HasPrefix(lines[idx], "--- ") {
			return unifiedDiffFilePatch{}, common.ErrInvalidArgument
		}
		if strings.TrimSpace(lines[idx]) == "" {
			idx++
			continue
		}
		if !strings.HasPrefix(lines[idx], "@@ ") {
			return unifiedDiffFilePatch{}, common.ErrInvalidArgument
		}
		hunk, nextIdx, err := parseUnifiedDiffHunk(lines, idx)
		if err != nil {
			return unifiedDiffFilePatch{}, err
		}
		filePatch.Hunks = append(filePatch.Hunks, hunk)
		idx = nextIdx
	}
	if filePatch.NewRef == "" || len(filePatch.Hunks) == 0 {
		return unifiedDiffFilePatch{}, common.ErrInvalidArgument
	}
	return filePatch, nil
}

func parseUnifiedDiffHunk(lines []string, idx int) (unifiedDiffHunk, int, error) {
	matches := unifiedDiffHunkHeaderPattern.FindStringSubmatch(lines[idx])
	if matches == nil {
		return unifiedDiffHunk{}, idx, common.ErrInvalidArgument
	}
	oldStart, err := strconv.Atoi(matches[1])
	if err != nil {
		return unifiedDiffHunk{}, idx, common.ErrInvalidArgument
	}
	hunk := unifiedDiffHunk{OldStart: oldStart}
	idx++
	for idx < len(lines) {
		line := lines[idx]
		if strings.HasPrefix(line, "@@ ") || strings.HasPrefix(line, "--- ") {
			break
		}
		if line == `\ No newline at end of file` {
			idx++
			continue
		}
		if line == "" {
			return unifiedDiffHunk{}, idx, common.ErrInvalidArgument
		}
		switch line[0] {
		case ' ', '-', '+':
			hunk.Lines = append(hunk.Lines, line)
		default:
			return unifiedDiffHunk{}, idx, common.ErrInvalidArgument
		}
		idx++
	}
	if len(hunk.Lines) == 0 {
		return unifiedDiffHunk{}, idx, common.ErrInvalidArgument
	}
	return hunk, idx, nil
}

func applyUnifiedDiffFilePatch(content string, filePatch unifiedDiffFilePatch) (string, error) {
	sourceLines, sourceHadFinalNewline := splitTextLines(content)
	output := make([]string, 0, len(sourceLines))
	cursor := 0
	for _, hunk := range filePatch.Hunks {
		start := hunk.OldStart - 1
		if start < 0 {
			start = 0
		}
		if start < cursor || start > len(sourceLines) {
			return "", common.ErrInvalidArgument
		}
		output = append(output, sourceLines[cursor:start]...)
		cursor = start
		for _, diffLine := range hunk.Lines {
			switch diffLine[0] {
			case ' ':
				if cursor >= len(sourceLines) || sourceLines[cursor] != diffLine[1:] {
					return "", common.ErrInvalidArgument
				}
				output = append(output, sourceLines[cursor])
				cursor++
			case '-':
				if cursor >= len(sourceLines) || sourceLines[cursor] != diffLine[1:] {
					return "", common.ErrInvalidArgument
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
