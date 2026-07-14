package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	defaultMaxReadBytes = 64 * 1024
	defaultMaxEntries   = 500
)

type RuntimeAdapter struct{}

func NewRuntimeAdapter() *RuntimeAdapter {
	return &RuntimeAdapter{}
}

func (a *RuntimeAdapter) AcquireTargetSnapshot(ctx context.Context, workspaceDirectory, targetRef string, maxBytes int) (runtimemodel.TargetSnapshot, error) {
	result, err := a.ReadFile(ctx, FileReadRequest{
		WorkspaceDirectory: workspaceDirectory,
		Path:               targetRef,
		MaxBytes:           int64(maxBytes),
	})
	if err != nil {
		return runtimemodel.TargetSnapshot{}, err
	}
	snapshot := runtimemodel.TargetSnapshot{
		TargetRef:   result.TargetRef,
		Content:     result.Content,
		ContentHash: result.ContentHash,
		Size:        result.Size,
		LineCount:   int(result.LineCount),
	}
	switch {
	case !result.Exists:
		snapshot.ContentState = runtimemodel.SnapshotContentStateMissing
	case result.Binary:
		snapshot.ContentState = runtimemodel.SnapshotContentStateBinary
	case result.Redacted:
		snapshot.ContentState = runtimemodel.SnapshotContentStateRedacted
	case result.Truncated:
		snapshot.ContentState = runtimemodel.SnapshotContentStateTruncated
	case result.ContentComplete:
		snapshot.ContentState = runtimemodel.SnapshotContentStateComplete
	}
	if !snapshot.Complete() {
		return snapshot, fmt.Errorf("%w: target snapshot %q is not complete (%s)", common.ErrInvalidArgument, snapshot.TargetRef, snapshot.ContentState)
	}
	return snapshot, nil
}

func (a *RuntimeAdapter) AcquireWorkspaceManifest(workspaceDirectory string) (runtimemodel.WorkspaceManifest, error) {
	result, err := a.ListFiles(FileListRequest{
		WorkspaceDirectory: workspaceDirectory,
		Path:               ".",
		Recursive:          true,
	})
	if err != nil {
		return runtimemodel.WorkspaceManifest{}, err
	}
	if result.Truncated {
		return runtimemodel.WorkspaceManifest{}, fmt.Errorf("%w: workspace manifest exceeds bounded file listing", common.ErrInvalidArgument)
	}
	targetRefs := make([]string, 0, len(result.Entries))
	h := sha256.New()
	for _, entry := range result.Entries {
		if entry.Kind != "file" {
			continue
		}
		ref := strings.TrimSpace(entry.Path)
		if ref == "" {
			continue
		}
		targetRefs = append(targetRefs, ref)
		_, _ = fmt.Fprintf(h, "%d:%s|%s|%d|%s;", len(ref), ref, entry.ContentHash, entry.Size, entry.ModifiedAt)
	}
	return runtimemodel.WorkspaceManifest{
		Revision:   hex.EncodeToString(h.Sum(nil)),
		TargetRefs: targetRefs,
	}, nil
}

func guardedWorkspacePath(deliverableDirectory string, inputPath string, requireExisting bool) (string, string, string, error) {
	if strings.TrimSpace(deliverableDirectory) == "" || strings.TrimSpace(inputPath) == "" {
		return "", "", "", common.ErrInvalidArgument
	}
	if filepath.IsAbs(inputPath) {
		return "", "", "", common.ErrInvalidArgument
	}
	root, err := filepath.Abs(deliverableDirectory)
	if err != nil {
		return "", "", "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", "", err
	}
	clean := filepath.Clean(inputPath)
	if clean == "." {
		// ok
	} else if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", "", "", common.ErrInvalidArgument
	}
	target := filepath.Join(root, clean)
	if requireExisting {
		target, err = filepath.EvalSymlinks(target)
		if err != nil {
			return "", "", "", err
		}
	} else if existing := nearestExistingPath(target); existing != "" {
		resolved, evalErr := filepath.EvalSymlinks(existing)
		if evalErr != nil {
			return "", "", "", evalErr
		}
		if !pathInside(root, resolved) {
			return "", "", "", common.ErrInvalidArgument
		}
	}
	if !pathInside(root, target) {
		return "", "", "", common.ErrInvalidArgument
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", "", "", err
	}
	return root, target, rel, nil
}

func nearestExistingPath(path string) string {
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

func pathInside(root string, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	return target == root || strings.HasPrefix(target, root+string(filepath.Separator))
}

func shouldSkipWorkspaceListPath(path string) bool {
	base := filepath.Base(path)
	switch base {
	case ".git", "node_modules", "vendor", "dist", "build", ".workorch-data":
		return true
	default:
		return false
	}
}

func fileKind(info os.FileInfo) string {
	if info.Mode()&os.ModeSymlink != 0 {
		return "symlink"
	}
	if info.IsDir() {
		return "directory"
	}
	return "file"
}

func fileHashAndBinary(path string, sampleLimit int) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	hash := sha256.Sum256(data)
	if len(data) > sampleLimit {
		data = data[:sampleLimit]
	}
	return hex.EncodeToString(hash[:]), looksBinary(data)
}

func looksBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return !utf8.Valid(data)
}

func lineRange(content string, startLine int, endLine int) string {
	lines := strings.SplitAfter(content, "\n")
	if startLine <= 0 {
		startLine = 1
	}
	if endLine <= 0 || endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine > endLine || startLine > len(lines) {
		return ""
	}
	return strings.Join(lines[startLine-1:endLine], "")
}

func replaceOccurrence(content string, oldText string, newText string, occurrence int) string {
	if occurrence <= 0 {
		occurrence = 1
	}
	start := 0
	for idx := 1; idx <= occurrence; idx++ {
		pos := strings.Index(content[start:], oldText)
		if pos < 0 {
			return content
		}
		absolute := start + pos
		if idx == occurrence {
			return content[:absolute] + newText + content[absolute+len(oldText):]
		}
		start = absolute + len(oldText)
	}
	return content
}

func unifiedDiffForEdit(path string, before string, after string) string {
	beforeLines, _ := splitEditTextLines(before)
	afterLines, _ := splitEditTextLines(after)
	prefix := 0
	for prefix < len(beforeLines) && prefix < len(afterLines) && beforeLines[prefix] == afterLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(beforeLines)-prefix &&
		suffix < len(afterLines)-prefix &&
		beforeLines[len(beforeLines)-1-suffix] == afterLines[len(afterLines)-1-suffix] {
		suffix++
	}
	const contextLines = 3
	contextStart := prefix - contextLines
	if contextStart < 0 {
		contextStart = 0
	}
	beforeChangeEnd := len(beforeLines) - suffix
	afterChangeEnd := len(afterLines) - suffix
	beforeEnd := beforeChangeEnd + contextLines
	if beforeEnd > len(beforeLines) {
		beforeEnd = len(beforeLines)
	}
	afterEnd := afterChangeEnd + contextLines
	if afterEnd > len(afterLines) {
		afterEnd = len(afterLines)
	}
	oldCount := beforeEnd - contextStart
	newCount := afterEnd - contextStart
	oldStart := contextStart + 1
	newStart := contextStart + 1
	var b strings.Builder
	b.WriteString("--- a/" + path + "\n")
	b.WriteString("+++ b/" + path + "\n")
	b.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount))
	for i := contextStart; i < prefix; i++ {
		b.WriteString(" " + beforeLines[i] + "\n")
	}
	for i := prefix; i < beforeChangeEnd; i++ {
		b.WriteString("-" + beforeLines[i] + "\n")
	}
	for i := prefix; i < afterChangeEnd; i++ {
		b.WriteString("+" + afterLines[i] + "\n")
	}
	for i := beforeChangeEnd; i < beforeEnd; i++ {
		b.WriteString(" " + beforeLines[i] + "\n")
	}
	return b.String()
}

func splitEditTextLines(content string) ([]string, bool) {
	if content == "" {
		return nil, false
	}
	finalNewline := strings.HasSuffix(content, "\n")
	content = strings.TrimSuffix(content, "\n")
	if content == "" {
		return []string{""}, finalNewline
	}
	return strings.Split(content, "\n"), finalNewline
}

func diffLineStats(diff string) (int, int) {
	added := 0
	deleted := 0
	for _, line := range strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			deleted++
		}
	}
	return added, deleted
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func containsRuntimeRedactionMarker(value string) bool {
	value = strings.ToUpper(value)
	return strings.Contains(value, "[REDACTED]") ||
		strings.Contains(value, "[SECRET]") ||
		strings.Contains(value, "[MASKED]")
}

func editApprovalRequired(path string, changedLines int) bool {
	clean := strings.ToLower(filepath.ToSlash(filepath.Clean(strings.TrimSpace(path))))
	base := filepath.Base(clean)
	switch {
	case changedLines > 500:
		return true
	case clean == ".git" || strings.HasPrefix(clean, ".git/"):
		return true
	case clean == ".ssh" || strings.HasPrefix(clean, ".ssh/"):
		return true
	case clean == ".github/workflows" || strings.HasPrefix(clean, ".github/workflows/"):
		return true
	case base == ".env" || strings.HasPrefix(base, ".env."):
		return true
	case strings.Contains(base, "id_rsa") || strings.Contains(base, "id_ed25519"):
		return true
	case strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") || strings.HasSuffix(base, ".crt"):
		return true
	default:
		return false
	}
}
