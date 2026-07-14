package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

type FileEditRequest struct {
	WorkspaceDirectory string
	Path               string
	OldText            string
	NewText            string
	BaseHash           string
	Occurrence         int64
}

type PatchApplyRequest struct {
	WorkspaceDirectory string
	TargetRef          string
	PatchType          string
	Content            string
	ContentPresent     bool
	Diff               string
	OldText            string
	NewText            string
	NewTextPresent     bool
	BaseHash           string
	Occurrence         int64
	CoverageTargets    []string
	Summary            string
}

type RuntimeFileOperationResult struct {
	Summary        string
	ContentHash    string
	EvidenceRefs   []string
	TargetRefs     []string
	FailureCode    string
	FailureMessage string
}

func (a *RuntimeAdapter) ApplyFileEdit(request FileEditRequest) (RuntimeFileOperationResult, error) {
	if a == nil || strings.TrimSpace(request.WorkspaceDirectory) == "" || strings.TrimSpace(request.Path) == "" || request.OldText == "" || request.OldText == request.NewText || request.Occurrence < 0 {
		return runtimeFileFailure("file_edit_invalid_input", common.ErrInvalidArgument, request.Path), common.ErrInvalidArgument
	}
	_, target, rel, err := guardedWorkspacePath(request.WorkspaceDirectory, request.Path, true)
	if err != nil {
		return runtimeFileFailure("file_edit_invalid_path", err, request.Path), err
	}
	info, err := os.Stat(target)
	if err != nil {
		return runtimeFileFailure("file_edit_stat_failed", err, rel), err
	}
	if info.IsDir() {
		return runtimeFileFailure("file_edit_is_directory", common.ErrInvalidArgument, rel), common.ErrInvalidArgument
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return runtimeFileFailure("file_edit_read_failed", err, rel), err
	}
	if looksBinary(data) {
		return runtimeFileFailure("file_edit_binary_file", common.ErrInvalidArgument, rel), common.ErrInvalidArgument
	}
	beforeHash := sha256Hex(data)
	if strings.TrimSpace(request.BaseHash) != "" && !strings.EqualFold(beforeHash, strings.TrimSpace(request.BaseHash)) {
		err = fmt.Errorf("%w: file/edit baseHash mismatch", common.ErrConflict)
		return runtimeFileFailure("file_edit_conflict", err, rel), err
	}
	content := string(data)
	matchCount := strings.Count(content, request.OldText)
	if matchCount == 0 {
		err = fmt.Errorf("%w: file/edit oldText not found", common.ErrConflict)
		return runtimeFileFailure("file_edit_conflict", err, rel), err
	}
	occurrence := int(request.Occurrence)
	if occurrence <= 0 && matchCount != 1 {
		err = fmt.Errorf("%w: file/edit oldText matched %d occurrences", common.ErrConflict, matchCount)
		return runtimeFileFailure("file_edit_conflict", err, rel), err
	}
	if occurrence > matchCount {
		err = fmt.Errorf("%w: file/edit occurrence %d exceeds %d matches", common.ErrConflict, occurrence, matchCount)
		return runtimeFileFailure("file_edit_conflict", err, rel), err
	}
	if occurrence <= 0 {
		occurrence = 1
	}
	nextContent := replaceOccurrence(content, request.OldText, request.NewText, occurrence)
	if err := os.WriteFile(target, []byte(nextContent), info.Mode().Perm()); err != nil {
		return runtimeFileFailure("file_edit_write_failed", err, rel), err
	}
	ref := filepath.ToSlash(rel)
	return RuntimeFileOperationResult{
		Summary: "file/edit applied", ContentHash: sha256Hex([]byte(nextContent)), TargetRefs: []string{ref},
		EvidenceRefs: []string{"workorch://workspace/" + ref},
	}, nil
}

func (a *RuntimeAdapter) ApplyPatch(request PatchApplyRequest) (RuntimeFileOperationResult, error) {
	if a == nil || strings.TrimSpace(request.WorkspaceDirectory) == "" || strings.TrimSpace(request.TargetRef) == "" || strings.TrimSpace(request.PatchType) == "" || request.Occurrence < 0 {
		return runtimeFileFailure("patch_invalid_input", common.ErrInvalidArgument, request.TargetRef), common.ErrInvalidArgument
	}
	patchRequest := patchExecutionRequest{
		Operation: "apply", PatchType: request.PatchType, TargetRef: request.TargetRef, Summary: request.Summary,
		Content: request.Content, Diff: request.Diff, OldText: request.OldText, NewText: request.NewText,
		BaseHash: request.BaseHash, Occurrence: int(request.Occurrence), CoverageTargets: append([]string(nil), request.CoverageTargets...),
	}
	if request.PatchType == patchTypeFileWrite && !request.ContentPresent {
		return runtimeFileFailure("patch_invalid_input", common.ErrInvalidArgument, request.TargetRef), common.ErrInvalidArgument
	}
	if request.PatchType == patchTypeTextReplace && (!request.NewTextPresent || request.OldText == "" || request.OldText == request.NewText) {
		return runtimeFileFailure("patch_invalid_input", common.ErrInvalidArgument, request.TargetRef), common.ErrInvalidArgument
	}
	stepResult, err := a.executePatchRequest(request.WorkspaceDirectory, patchRequest, true)
	return runtimeFileOperationResultFromOutput(stepResult.Output, request.TargetRef, err), err
}

func runtimeFileOperationResultFromOutput(output map[string]any, fallbackTarget string, executeErr error) RuntimeFileOperationResult {
	ret := RuntimeFileOperationResult{TargetRefs: []string{strings.TrimSpace(fallbackTarget)}}
	if value, ok := output["summary"].(string); ok {
		ret.Summary = strings.TrimSpace(value)
	}
	if ret.Summary == "" {
		if value, ok := output["stage"].(string); ok {
			ret.Summary = strings.TrimSpace(value)
		}
	}
	if value, ok := output["contentHash"].(string); ok {
		ret.ContentHash = strings.TrimSpace(value)
	}
	if value, ok := output["targetRef"].(string); ok && strings.TrimSpace(value) != "" {
		ret.TargetRefs = []string{strings.TrimSpace(value)}
	}
	if executeErr != nil {
		ret.FailureMessage = executeErr.Error()
		if diagnostics, ok := output["diagnostics"].([]map[string]any); ok && len(diagnostics) > 0 {
			if value, ok := diagnostics[0]["code"].(string); ok {
				ret.FailureCode = strings.TrimSpace(value)
			}
			if value, ok := diagnostics[0]["message"].(string); ok && strings.TrimSpace(value) != "" {
				ret.FailureMessage = strings.TrimSpace(value)
			}
		}
		if ret.FailureCode == "" {
			ret.FailureCode = "patch_apply_failed"
		}
		return ret
	}
	for _, target := range ret.TargetRefs {
		if target != "" {
			ret.EvidenceRefs = append(ret.EvidenceRefs, "workorch://workspace/"+target)
		}
	}
	return ret
}

func runtimeFileFailure(code string, err error, targetRef string) RuntimeFileOperationResult {
	ret := RuntimeFileOperationResult{FailureCode: code, TargetRefs: []string{strings.TrimSpace(targetRef)}}
	if err != nil {
		ret.FailureMessage = err.Error()
	}
	return ret
}
