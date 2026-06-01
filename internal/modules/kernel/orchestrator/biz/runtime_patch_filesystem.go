package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/muidea/workorch/internal/pkg/common"
)

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
