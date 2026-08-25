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
	// CreateOnly is selected only by the File owner's file/write command
	// adapter. It makes file/write a creation operation and prevents it from
	// replacing an owner-observed resource without a revision-bound patch.
	CreateOnly bool
	// ApprovalGranted is set only after the File owner has validated the persisted
	// Runtime governance grant for this exact ExecutionKey + resume Fence.
	ApprovalGranted bool
	// ReplayProof is an optional sealed File-owner proof supplied only through
	// the execution envelope. It permits postcondition confirmation after a
	// lost receipt CAS; it never changes normal patch semantics.
	ReplayProof *runtimecontract.RuntimeOperationReplayProof
}

type RuntimeFileOperationResult struct {
	Stage          string
	Summary        string
	BeforeHash     string
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
	truncated := false
	streamHashAndType := a.streamHashAndType
	if streamHashAndType == nil {
		streamHashAndType = streamFileHashAndBinary
	}
	addEntry := func(path string, entry fs.DirEntry) error {
		if shouldSkipWorkspaceListPath(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relInput, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		_, resolved, relEntry, pathErr := guardedWorkspacePath(root, relInput, true)
		if pathErr != nil {
			// WalkDir never follows symlinks. An unsafe or broken symlink is
			// therefore an explicit non-deliverable entry and may be skipped;
			// failures for ordinary files/directories make the inventory
			// incomplete and must fail closed.
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			return pathErr
		}
		entryInfo, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		// Reaching the configured bound is not itself truncation. Only an
		// additional entry that would otherwise be returned proves overflow.
		// Skipped or uninspectable paths therefore do not consume the bound.
		if len(entries) >= maxEntries {
			truncated = true
			return filepath.SkipAll
		}
		entryRef := filepath.ToSlash(relEntry)
		if entry.Type()&os.ModeSymlink != 0 {
			// guardedWorkspacePath has already proved the alias resolves inside
			// the workspace. Preserve the alias's own canonical tree identity in
			// the bounded inventory so rename/replace topology cannot retain the
			// previous workspace-manifest revision.
			entryRef = filepath.ToSlash(filepath.Clean(relInput))
		}
		item := FileListEntry{
			Path: entryRef, Kind: fileKind(entryInfo), Size: entryInfo.Size(),
			ModifiedAt: entryInfo.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if !entryInfo.IsDir() && entryInfo.Mode()&os.ModeSymlink == 0 {
			item.ContentHash, item.Binary, infoErr = streamHashAndType(resolved)
			if infoErr != nil {
				return infoErr
			}
		}
		entries = append(entries, item)
		return nil
	}
	if request.Recursive {
		walkDir := a.walkDir
		if walkDir == nil {
			walkDir = filepath.WalkDir
		}
		err = walkDir(target, func(path string, entry fs.DirEntry, walkErr error) error {
			if path == target {
				return walkErr
			}
			if walkErr != nil {
				if entry != nil && shouldSkipWorkspaceListPath(path) {
					if entry.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				return walkErr
			}
			if entry == nil {
				return fmt.Errorf("file/list walker returned a nil entry without an error for %q", path)
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
		Truncated: truncated, MaxEntries: int64(maxEntries),
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
	maxBytes := int(request.MaxBytes)
	if maxBytes <= 0 || maxBytes > defaultMaxReadBytes {
		maxBytes = defaultMaxReadBytes
	}
	scan, err := scanBoundedFile(ctx, target, request.StartLine, request.EndLine, maxBytes)
	if err != nil {
		return fileReadFailure("file_read_failed", err), err
	}
	isBinary := scan.Binary
	content := ""
	truncated := scan.Truncated
	ranged := request.StartLine > 0 || request.EndLine > 0
	if !isBinary {
		content = string(scan.Content)
		for !utf8.ValidString(content) && len(content) > 0 {
			content = content[:len(content)-1]
		}
	} else {
		truncated = scan.Size > 0
	}
	redacted := containsRuntimeRedactionMarker(content)
	contentComplete := !isBinary && !truncated && !redacted && !ranged
	targetRef := filepath.ToSlash(rel)
	ret := FileReadResult{
		Status: "completed", Stage: "file_read", Path: targetRef, TargetRef: targetRef, Exists: true,
		Content: content, ContentHash: scan.ContentHash, Size: info.Size(),
		ModifiedAt: info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"), LineCount: scan.LineCount,
		Truncated: truncated, Binary: isBinary, Redacted: redacted, ContentComplete: contentComplete,
		SafeForPatch: contentComplete && scan.Size > 0, SafeForFileWrite: contentComplete && scan.Size > 0,
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
		CreateOnly: request.CreateOnly,
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

func (a *RuntimeAdapter) ApplyPatch(request PatchApplyRequest) (RuntimeFileOperationResult, error) {
	if a == nil || strings.TrimSpace(request.WorkspaceDirectory) == "" || strings.TrimSpace(request.TargetRef) == "" || strings.TrimSpace(request.PatchType) == "" || request.Occurrence < 0 {
		return runtimeFileFailure("patch_invalid_input", common.ErrInvalidArgument, request.TargetRef), common.ErrInvalidArgument
	}
	unlock := a.lockWorkspaceEffect(request.WorkspaceDirectory)
	defer unlock()
	patchRequest := patchExecutionRequest{
		Operation: "apply", PatchType: request.PatchType, TargetRef: request.TargetRef, Summary: request.Summary,
		Content: request.Content, Diff: request.Diff, OldText: request.OldText, NewText: request.NewText,
		BaseHash: request.BaseHash, Occurrence: int(request.Occurrence), CoverageTargets: append([]string(nil), request.CoverageTargets...),
		CreateOnly: request.CreateOnly,
	}
	if request.PatchType == patchTypeFileWrite && !request.ContentPresent {
		return runtimeFileFailure("patch_invalid_input", common.ErrInvalidArgument, request.TargetRef), common.ErrInvalidArgument
	}
	if request.PatchType == patchTypeTextReplace && (!request.NewTextPresent || request.OldText == "" || request.OldText == request.NewText) {
		return runtimeFileFailure("patch_invalid_input", common.ErrInvalidArgument, request.TargetRef), common.ErrInvalidArgument
	}
	if request.ReplayProof != nil {
		proof := request.ReplayProof.Normalized()
		if err := proof.Validate(); err != nil {
			return runtimeFileFailure("patch_replay_proof_invalid", err, request.TargetRef), err
		}
		if replay, confirmed, err := confirmPatchReplayProof(request, proof); confirmed || err != nil {
			return replay, err
		}
		preview, previewErr := a.PreviewPatch(request)
		if previewErr != nil {
			// This is a recovery-time postcondition check, not a fresh candidate
			// validation failure. A changed target can make a previously valid
			// patch syntactically inapplicable (for example, a unified-diff hunk
			// no longer matches). Preserve the replay boundary for every variant:
			// the owner must refuse to write and report a conflict rather than
			// leaking a variant-specific invalid-input classification.
			err := fmt.Errorf("%w: patch replay proof cannot confirm sealed postcondition: %v", common.ErrConflict, previewErr)
			return runtimeFileFailure("patch_replay_proof_mismatch", err, request.TargetRef), err
		}
		if preview.TargetRef != request.TargetRef || preview.BeforeHash != proof.BeforeHash || preview.AfterHash != proof.AfterHash {
			err := fmt.Errorf("%w: patch replay proof does not match current sealed operation (target %q/%q, before %q/%q, after %q/%q)", common.ErrConflict, preview.TargetRef, request.TargetRef, preview.BeforeHash, proof.BeforeHash, preview.AfterHash, proof.AfterHash)
			return runtimeFileFailure("patch_replay_proof_mismatch", err, request.TargetRef), err
		}
	}
	// A sealed file_write carrying the owner-verified input revision can be
	// replayed after its durable write succeeded but the Orchestrator receipt
	// CAS did not. The persisted execution key/fence still makes the receipt
	// unique; this check merely proves that the sealed postcondition already
	// holds and therefore must not write the workspace a second time.
	if replay, confirmed, err := confirmFileWritePatchReplay(request); confirmed || err != nil {
		return replay, err
	}
	// RVC-R11 INV-06: preview first; if approval is required, do not apply side effects.
	previewRequest := patchRequest
	previewRequest.Operation = "preview"
	preview, previewErr := a.executePatchRequest(request.WorkspaceDirectory, previewRequest, false)
	if previewErr == nil && preview.ApprovalRequired && !request.ApprovalGranted {
		err := fmt.Errorf("%w: patch apply requires approval before side effects", common.ErrInvalidArgument)
		ret := runtimeFileOperationResultFromExecution(preview, request.TargetRef, err)
		ret.FailureCode = "approval_required"
		ret.FailureMessage = err.Error()
		return ret, err
	}
	if previewErr == nil {
		if capacityErr := a.validateProspectivePatchManifestCapacity(request); capacityErr != nil {
			return runtimeFileFailure("workspace_manifest_capacity_exhausted", capacityErr, request.TargetRef), capacityErr
		}
	}
	result, err := a.executePatchRequest(request.WorkspaceDirectory, patchRequest, true)
	return runtimeFileOperationResultFromExecution(result, request.TargetRef, err), err
}

func (a *RuntimeAdapter) validateProspectivePatchManifestCapacity(request PatchApplyRequest) error {
	_, target, _, err := guardedWorkspacePath(request.WorkspaceDirectory, request.TargetRef, false)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	observation, err := a.acquireWorkspaceManifestObservation(request.WorkspaceDirectory)
	if err != nil {
		return err
	}
	additional, err := runtimeFileProspectiveTargetAdditionalEntries(request.WorkspaceDirectory, request.TargetRef)
	if err != nil {
		return err
	}
	if additional < 1 || observation.entryCount+additional > defaultMaxEntries {
		return fmt.Errorf("%w: File manifest capacity %d is exhausted before creating %q", common.ErrConflict, defaultMaxEntries, request.TargetRef)
	}
	return nil
}

// confirmPatchReplayProof verifies only the owner-observed postcondition that
// was sealed before the original claim. It works for all patch/apply variants:
// no operation text is reinterpreted and no write is attempted when the target
// already has the exact expected output revision.
func confirmPatchReplayProof(request PatchApplyRequest, proof runtimecontract.RuntimeOperationReplayProof) (RuntimeFileOperationResult, bool, error) {
	proof = proof.Normalized()
	_, target, rel, err := guardedWorkspacePath(request.WorkspaceDirectory, request.TargetRef, false)
	if err != nil {
		return RuntimeFileOperationResult{}, false, err
	}
	ref := filepath.ToSlash(rel)
	found := false
	for _, targetRef := range proof.TargetRefs {
		if targetRef == ref {
			found = true
			break
		}
	}
	if !found {
		return RuntimeFileOperationResult{}, false, fmt.Errorf("%w: patch replay proof target mismatch", common.ErrConflict)
	}
	content, exists, readErr := readPatchTarget(target)
	if readErr != nil || !exists || looksBinary(content) {
		return RuntimeFileOperationResult{}, false, readErr
	}
	actualHash := sha256Hex(content)
	if actualHash != proof.AfterHash {
		return RuntimeFileOperationResult{}, false, nil
	}
	return RuntimeFileOperationResult{
		Stage: "patch_apply", Summary: "patch replay proof confirmed",
		BeforeHash: proof.BeforeHash, AfterHash: proof.AfterHash, ContentHash: proof.AfterHash,
		TargetRefs: []string{ref}, EvidenceRefs: []string{"workorch://workspace/" + ref},
	}, true, nil
}

// confirmFileWritePatchReplay recognizes only a fully sealed file_write
// postcondition. It intentionally does not infer replay success for text
// replacement or unified diff: their postconditions require their own
// owner-verified effect proof. A caller supplied baseHash is accepted here
// only after feasibility preflight has checked it against the owner snapshot.
func confirmFileWritePatchReplay(request PatchApplyRequest) (RuntimeFileOperationResult, bool, error) {
	if request.PatchType != patchTypeFileWrite || !request.ContentPresent || strings.TrimSpace(request.BaseHash) == "" {
		return RuntimeFileOperationResult{}, false, nil
	}
	_, target, rel, err := guardedWorkspacePath(request.WorkspaceDirectory, request.TargetRef, false)
	if err != nil {
		return RuntimeFileOperationResult{}, false, err
	}
	content, exists, err := readPatchTarget(target)
	if err != nil || !exists || looksBinary(content) {
		return RuntimeFileOperationResult{}, false, err
	}
	actualHash := sha256Hex(content)
	baseHash := strings.TrimSpace(request.BaseHash)
	if strings.EqualFold(actualHash, baseHash) || string(content) != request.Content {
		return RuntimeFileOperationResult{}, false, nil
	}
	ref := filepath.ToSlash(rel)
	return RuntimeFileOperationResult{
		Stage: "patch_apply", Summary: "patch file_write replay confirmed",
		// BeforeHash is the owner-verified sealed input revision, not a fresh
		// observation of the already-written target. It lets the receipt retain
		// the original durable resource delta after a lost CAS response.
		BeforeHash: baseHash, AfterHash: actualHash, ContentHash: actualHash,
		TargetRefs: []string{ref}, EvidenceRefs: []string{"workorch://workspace/" + ref},
	}, true, nil
}

func runtimeFileOperationResultFromExecution(result patchExecutionResult, fallbackTarget string, executeErr error) RuntimeFileOperationResult {
	ret := RuntimeFileOperationResult{
		Stage: result.Stage, BeforeHash: result.BeforeHash, ContentHash: result.ContentHash, AfterHash: result.AfterHash, DiffHash: result.DiffHash,
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
