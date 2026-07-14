package file

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type FileListRequest struct {
	WorkspaceDirectory string
	Path               string
	Recursive          bool
	MaxEntries         int64
}

type FileListEntry struct {
	Path        string
	Kind        string
	Size        int64
	ModifiedAt  string
	ContentHash string
	Binary      bool
	Ignored     bool
}

type FileListResult struct {
	Status         string
	Stage          string
	Path           string
	Entries        []FileListEntry
	Truncated      bool
	MaxEntries     int64
	FailureCode    string
	FailureMessage string
	Evidence       runtimecontract.RuntimeOperationEvidence
}

type FileReadRequest struct {
	WorkspaceDirectory string
	Path               string
	Optional           bool
	MaxBytes           int64
	StartLine          int64
	EndLine            int64
}

type FileReadResult struct {
	Status           string
	Stage            string
	Path             string
	TargetRef        string
	Exists           bool
	Content          string
	ContentHash      string
	Size             int64
	ModifiedAt       string
	LineCount        int64
	Truncated        bool
	Binary           bool
	Redacted         bool
	ContentComplete  bool
	SafeForPatch     bool
	SafeForFileWrite bool
	FailureCode      string
	FailureMessage   string
	Evidence         runtimecontract.RuntimeOperationEvidence
}

type PatchPreviewResult struct {
	Status           string
	Stage            string
	Operation        string
	PatchType        string
	TargetRef        string
	AffectedFiles    []string
	BeforeHash       string
	AfterHash        string
	ContentHash      string
	DiffHash         string
	AddedLines       int64
	DeletedLines     int64
	ApprovalRequired bool
	RiskLevel        string
	RiskReasons      []string
	WillWrite        bool
	MatchCount       int64
	Summary          string
	FailureCode      string
	FailureMessage   string
	Evidence         runtimecontract.RuntimeOperationEvidence
}

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
	Stage          string
	Summary        string
	ContentHash    string
	AfterHash      string
	DiffHash       string
	AddedLines     int64
	DeletedLines   int64
	EvidenceRefs   []string
	TargetRefs     []string
	FailureCode    string
	FailureMessage string
}

func (a *RuntimeAdapter) ListFiles(request FileListRequest) (FileListResult, error) {
	if a == nil || strings.TrimSpace(request.WorkspaceDirectory) == "" {
		return FileListResult{FailureCode: "file_list_invalid_input", FailureMessage: common.ErrInvalidArgument.Error()}, common.ErrInvalidArgument
	}
	relPath := strings.TrimSpace(request.Path)
	if relPath == "" {
		relPath = "."
	}
	root, target, rel, err := guardedWorkspacePath(request.WorkspaceDirectory, relPath, false)
	if err != nil {
		return fileListFailure("file_list_invalid_path", err), err
	}
	info, err := os.Stat(target)
	if err != nil {
		return fileListFailure("file_list_stat_failed", err), err
	}
	if !info.IsDir() {
		return fileListFailure("file_list_not_directory", common.ErrInvalidArgument), common.ErrInvalidArgument
	}
	maxEntries := int(request.MaxEntries)
	if maxEntries <= 0 || maxEntries > defaultMaxEntries {
		maxEntries = defaultMaxEntries
	}
	entries := make([]FileListEntry, 0)
	addEntry := func(path string, entry fs.DirEntry) error {
		if len(entries) >= maxEntries {
			return filepath.SkipAll
		}
		if shouldSkipWorkspaceListPath(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relInput, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		_, resolved, relEntry, pathErr := guardedWorkspacePath(root, relInput, true)
		if pathErr != nil {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		entryInfo, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}
		item := FileListEntry{
			Path: filepath.ToSlash(relEntry), Kind: fileKind(entryInfo), Size: entryInfo.Size(),
			ModifiedAt: entryInfo.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if !entryInfo.IsDir() && entryInfo.Mode()&os.ModeSymlink == 0 {
			item.ContentHash, item.Binary = fileHashAndBinary(resolved, 8192)
		}
		entries = append(entries, item)
		return nil
	}
	if request.Recursive {
		err = filepath.WalkDir(target, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || path == target {
				return nil
			}
			return addEntry(path, entry)
		})
	} else {
		var dirEntries []os.DirEntry
		dirEntries, err = os.ReadDir(target)
		if err == nil {
			for _, entry := range dirEntries {
				addErr := addEntry(filepath.Join(target, entry.Name()), entry)
				if addErr == filepath.SkipAll {
					break
				}
				if addErr == filepath.SkipDir {
					continue
				}
				if addErr != nil {
					err = addErr
					break
				}
			}
		}
	}
	if err != nil && err != filepath.SkipAll {
		return fileListFailure("file_list_failed", err), err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	ret := FileListResult{
		Status: "completed", Stage: "file_list", Path: filepath.ToSlash(rel), Entries: entries,
		Truncated: len(entries) >= maxEntries, MaxEntries: int64(maxEntries),
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "completed file/list"},
	}
	for _, entry := range entries {
		if entry.Path != "" {
			ret.Evidence.EvidenceRefs = append(ret.Evidence.EvidenceRefs, "workorch://workspace/"+entry.Path)
		}
	}
	return ret, nil
}

func (a *RuntimeAdapter) ReadFile(ctx context.Context, request FileReadRequest) (FileReadResult, error) {
	_ = ctx
	if a == nil || strings.TrimSpace(request.WorkspaceDirectory) == "" || strings.TrimSpace(request.Path) == "" {
		return FileReadResult{FailureCode: "file_read_invalid_input", FailureMessage: common.ErrInvalidArgument.Error()}, common.ErrInvalidArgument
	}
	_, target, rel, err := guardedWorkspacePath(request.WorkspaceDirectory, request.Path, !request.Optional)
	if err != nil {
		return fileReadFailure("file_read_invalid_path", err), err
	}
	info, err := os.Stat(target)
	if err != nil {
		if request.Optional && os.IsNotExist(err) {
			return FileReadResult{Status: "completed", Stage: "file_read", Path: filepath.ToSlash(rel)}, nil
		}
		return fileReadFailure("file_read_stat_failed", err), err
	}
	if info.IsDir() {
		return fileReadFailure("file_read_is_directory", common.ErrInvalidArgument), common.ErrInvalidArgument
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return fileReadFailure("file_read_failed", err), err
	}
	isBinary := looksBinary(data)
	lineCount := strings.Count(string(data), "\n")
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		lineCount++
	}
	maxBytes := int(request.MaxBytes)
	if maxBytes <= 0 || maxBytes > defaultMaxReadBytes {
		maxBytes = defaultMaxReadBytes
	}
	content := ""
	truncated := false
	ranged := request.StartLine > 0 || request.EndLine > 0
	if !isBinary {
		content = string(data)
		if ranged {
			content = lineRange(content, int(request.StartLine), int(request.EndLine))
		}
		if len(content) > maxBytes {
			content = content[:maxBytes]
			truncated = true
			for !utf8.ValidString(content) && len(content) > 0 {
				content = content[:len(content)-1]
			}
		}
	} else {
		truncated = len(data) > 0
	}
	redacted := containsRuntimeRedactionMarker(content)
	contentComplete := !isBinary && !truncated && !redacted && !ranged
	targetRef := filepath.ToSlash(rel)
	ret := FileReadResult{
		Status: "completed", Stage: "file_read", Path: targetRef, TargetRef: targetRef, Exists: true,
		Content: content, ContentHash: sha256Hex(data), Size: info.Size(),
		ModifiedAt: info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"), LineCount: int64(lineCount),
		Truncated: truncated, Binary: isBinary, Redacted: redacted, ContentComplete: contentComplete,
		SafeForPatch: contentComplete && len(data) > 0, SafeForFileWrite: contentComplete && len(data) > 0,
	}
	ret.Evidence = runtimecontract.RuntimeOperationEvidence{
		Summary: "completed file/read " + targetRef, ContentHash: ret.ContentHash,
		EvidenceRefs: []string{"workorch://workspace/" + targetRef},
	}
	return ret, nil
}

func fileListFailure(stage string, err error) FileListResult {
	return FileListResult{Status: "failed", Stage: stage, FailureCode: stage, FailureMessage: err.Error()}
}

func fileReadFailure(stage string, err error) FileReadResult {
	return FileReadResult{Status: "failed", Stage: stage, FailureCode: stage, FailureMessage: err.Error()}
}

func (a *RuntimeAdapter) PreviewPatch(request PatchApplyRequest) (PatchPreviewResult, error) {
	if a == nil || strings.TrimSpace(request.WorkspaceDirectory) == "" || strings.TrimSpace(request.TargetRef) == "" || strings.TrimSpace(request.PatchType) == "" || request.Occurrence < 0 {
		return PatchPreviewResult{FailureCode: "patch_invalid_input", FailureMessage: common.ErrInvalidArgument.Error()}, common.ErrInvalidArgument
	}
	patchRequest := patchExecutionRequest{
		Operation: "preview", PatchType: request.PatchType, TargetRef: request.TargetRef, Summary: request.Summary,
		Content: request.Content, Diff: request.Diff, OldText: request.OldText, NewText: request.NewText,
		BaseHash: request.BaseHash, Occurrence: int(request.Occurrence), CoverageTargets: append([]string(nil), request.CoverageTargets...),
	}
	if request.PatchType == patchTypeFileWrite && !request.ContentPresent {
		return PatchPreviewResult{FailureCode: "patch_invalid_input", FailureMessage: common.ErrInvalidArgument.Error(), TargetRef: request.TargetRef}, common.ErrInvalidArgument
	}
	if request.PatchType == patchTypeTextReplace && (!request.NewTextPresent || request.OldText == "" || request.OldText == request.NewText) {
		return PatchPreviewResult{FailureCode: "patch_invalid_input", FailureMessage: common.ErrInvalidArgument.Error(), TargetRef: request.TargetRef}, common.ErrInvalidArgument
	}
	result, err := a.executePatchRequest(request.WorkspaceDirectory, patchRequest, false)
	return patchPreviewResultFromExecution(result, request.TargetRef, err), err
}

func patchPreviewResultFromExecution(result patchExecutionResult, fallbackTarget string, executeErr error) PatchPreviewResult {
	ret := PatchPreviewResult{
		Status: result.Status, Stage: result.Stage, Operation: result.Operation, PatchType: result.PatchType,
		TargetRef: result.TargetRef, AffectedFiles: append([]string(nil), result.AffectedFiles...),
		BeforeHash: result.BeforeHash, AfterHash: result.AfterHash, ContentHash: result.ContentHash, DiffHash: result.DiffHash,
		AddedLines: result.AddedLines, DeletedLines: result.DeletedLines, ApprovalRequired: result.ApprovalRequired,
		RiskLevel: result.RiskLevel, RiskReasons: append([]string(nil), result.RiskReasons...), WillWrite: result.WillWrite,
		MatchCount: result.MatchCount, Summary: result.Summary, FailureCode: result.FailureCode, FailureMessage: result.FailureMessage,
	}
	if ret.TargetRef == "" {
		ret.TargetRef = strings.TrimSpace(fallbackTarget)
	}
	if executeErr != nil && ret.FailureCode == "" {
		ret.FailureCode = "patch_preview_failed"
		ret.FailureMessage = executeErr.Error()
	}
	if ret.Summary == "" {
		ret.Summary = ret.Stage
	}
	ret.Evidence = runtimecontract.RuntimeOperationEvidence{Summary: ret.Summary, ContentHash: firstNonEmpty(ret.ContentHash, ret.DiffHash)}
	if ret.TargetRef != "" {
		ret.Evidence.EvidenceRefs = []string{"workorch://workspace/" + ret.TargetRef}
	}
	return ret
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
	result, err := a.executePatchRequest(request.WorkspaceDirectory, patchRequest, true)
	return runtimeFileOperationResultFromExecution(result, request.TargetRef, err), err
}

func runtimeFileOperationResultFromExecution(result patchExecutionResult, fallbackTarget string, executeErr error) RuntimeFileOperationResult {
	ret := RuntimeFileOperationResult{
		Stage: result.Stage, ContentHash: result.ContentHash, AfterHash: result.AfterHash, DiffHash: result.DiffHash,
		AddedLines: result.AddedLines, DeletedLines: result.DeletedLines,
		TargetRefs: []string{strings.TrimSpace(fallbackTarget)},
	}
	ret.Summary = strings.TrimSpace(result.Summary)
	if ret.Summary == "" {
		ret.Summary = strings.TrimSpace(result.Stage)
	}
	ret.ContentHash = strings.TrimSpace(result.ContentHash)
	if strings.TrimSpace(result.TargetRef) != "" {
		ret.TargetRefs = []string{strings.TrimSpace(result.TargetRef)}
	}
	if executeErr != nil {
		ret.FailureCode = strings.TrimSpace(result.FailureCode)
		ret.FailureMessage = firstNonEmpty(result.FailureMessage, executeErr.Error())
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
