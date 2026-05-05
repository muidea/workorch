package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	defaultMaxReadBytes = 64 * 1024
	defaultMaxEntries   = 500
)

type RuntimeAdapter struct{}

func NewRuntimeAdapter() *RuntimeAdapter {
	return &RuntimeAdapter{}
}

func (a *RuntimeAdapter) ExecuteRuntimeStep(_ context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	switch strings.TrimSpace(request.Kind) {
	case "file/list":
		return a.list(request)
	case "file/read":
		return a.read(request)
	case "file/edit":
		return a.edit(request)
	default:
		return runtimemodel.StepExecutionResult{}, common.ErrInvalidArgument
	}
}

func (a *RuntimeAdapter) list(request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	relPath := stringInput(request.Input, "path")
	if relPath == "" {
		relPath = "."
	}
	root, target, rel, err := guardedWorkspacePath(request.WorkspaceWorkingDirectory, relPath, false)
	if err != nil {
		return fileErrorResult("file_list_invalid_path", err), err
	}
	info, err := os.Stat(target)
	if err != nil {
		return fileErrorResult("file_list_stat_failed", err), err
	}
	if !info.IsDir() {
		return fileErrorResult("file_list_not_directory", common.ErrInvalidArgument), common.ErrInvalidArgument
	}
	recursive := boolInput(request.Input, "recursive")
	maxEntries := intInput(request.Input, "maxEntries", defaultMaxEntries)
	if maxEntries <= 0 || maxEntries > defaultMaxEntries {
		maxEntries = defaultMaxEntries
	}
	entries := make([]map[string]any, 0)
	addEntry := func(path string, d fs.DirEntry) error {
		if len(entries) >= maxEntries {
			return filepath.SkipAll
		}
		if shouldSkipWorkspaceListPath(path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relInput, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		resolved, _, relEntry, err := guardedWorkspacePath(root, relInput, true)
		if err != nil {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		entry := map[string]any{
			"path":        filepath.ToSlash(relEntry),
			"kind":        fileKind(info),
			"size":        info.Size(),
			"mtime":       info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
			"contentHash": "",
			"isBinary":    false,
			"isIgnored":   false,
		}
		if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			hash, binary := fileHashAndBinary(resolved, 8192)
			entry["contentHash"] = hash
			entry["isBinary"] = binary
		}
		entries = append(entries, entry)
		return nil
	}
	if recursive {
		err = filepath.WalkDir(target, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || path == target {
				return nil
			}
			return addEntry(path, d)
		})
	} else {
		dirEntries, readErr := os.ReadDir(target)
		if readErr != nil {
			err = readErr
		} else {
			for _, entry := range dirEntries {
				if addErr := addEntry(filepath.Join(target, entry.Name()), entry); addErr != nil {
					if addErr == filepath.SkipAll {
						break
					}
					err = addErr
					break
				}
			}
		}
	}
	if err != nil && err != filepath.SkipAll {
		return fileErrorResult("file_list_failed", err), err
	}
	sort.Slice(entries, func(i, j int) bool {
		return fmt.Sprint(entries[i]["path"]) < fmt.Sprint(entries[j]["path"])
	})
	return runtimemodel.StepExecutionResult{Output: map[string]any{
		"status":     "completed",
		"stage":      "file_list",
		"path":       filepath.ToSlash(rel),
		"entries":    entries,
		"truncated":  len(entries) >= maxEntries,
		"maxEntries": maxEntries,
	}}, nil
}

func (a *RuntimeAdapter) read(request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	relPath := stringInput(request.Input, "path")
	_, target, rel, err := guardedWorkspacePath(request.WorkspaceWorkingDirectory, relPath, true)
	if err != nil {
		return fileErrorResult("file_read_invalid_path", err), err
	}
	info, err := os.Stat(target)
	if err != nil {
		return fileErrorResult("file_read_stat_failed", err), err
	}
	if info.IsDir() {
		return fileErrorResult("file_read_is_directory", common.ErrInvalidArgument), common.ErrInvalidArgument
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return fileErrorResult("file_read_failed", err), err
	}
	hashBytes := sha256.Sum256(data)
	isBinary := looksBinary(data)
	lineCount := strings.Count(string(data), "\n")
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		lineCount++
	}
	maxBytes := intInput(request.Input, "maxBytes", defaultMaxReadBytes)
	if maxBytes <= 0 || maxBytes > defaultMaxReadBytes {
		maxBytes = defaultMaxReadBytes
	}
	startLine := intInput(request.Input, "startLine", 0)
	endLine := intInput(request.Input, "endLine", 0)
	content := ""
	truncated := false
	if !isBinary {
		content = string(data)
		if startLine > 0 || endLine > 0 {
			content = lineRange(content, startLine, endLine)
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
	return runtimemodel.StepExecutionResult{Output: map[string]any{
		"status":      "completed",
		"stage":       "file_read",
		"path":        filepath.ToSlash(rel),
		"content":     content,
		"contentHash": hex.EncodeToString(hashBytes[:]),
		"size":        info.Size(),
		"mtime":       info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
		"lineCount":   lineCount,
		"truncated":   truncated,
		"isBinary":    isBinary,
	}}, nil
}

func (a *RuntimeAdapter) edit(request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	relPath := stringInput(request.Input, "path")
	oldText := rawStringInput(request.Input, "oldText")
	newText := rawStringInput(request.Input, "newText")
	baseHash := stringInput(request.Input, "baseHash")
	occurrence := intInput(request.Input, "occurrence", 0)
	if strings.TrimSpace(relPath) == "" || oldText == "" {
		return fileErrorResult("file_edit_invalid_input", common.ErrInvalidArgument), common.ErrInvalidArgument
	}
	_, target, rel, err := guardedWorkspacePath(request.WorkspaceWorkingDirectory, relPath, true)
	if err != nil {
		return fileErrorResult("file_edit_invalid_path", err), err
	}
	info, err := os.Stat(target)
	if err != nil {
		return fileErrorResult("file_edit_stat_failed", err), err
	}
	if info.IsDir() {
		return fileErrorResult("file_edit_is_directory", common.ErrInvalidArgument), common.ErrInvalidArgument
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return fileErrorResult("file_edit_read_failed", err), err
	}
	if looksBinary(data) {
		return fileErrorResult("file_edit_binary_file", common.ErrInvalidArgument), common.ErrInvalidArgument
	}
	beforeHash := sha256Hex(data)
	if baseHash != "" && !strings.EqualFold(beforeHash, baseHash) {
		err := fmt.Errorf("%w: file/edit baseHash mismatch", common.ErrConflict)
		return fileErrorResult("file_edit_conflict", err), err
	}
	content := string(data)
	matchCount := strings.Count(content, oldText)
	if matchCount == 0 {
		err := fmt.Errorf("%w: file/edit oldText not found", common.ErrConflict)
		return fileErrorResult("file_edit_conflict", err), err
	}
	if occurrence <= 0 && matchCount != 1 {
		err := fmt.Errorf("%w: file/edit oldText matched %d occurrences", common.ErrConflict, matchCount)
		return fileErrorResult("file_edit_conflict", err), err
	}
	if occurrence > matchCount {
		err := fmt.Errorf("%w: file/edit occurrence %d exceeds %d matches", common.ErrConflict, occurrence, matchCount)
		return fileErrorResult("file_edit_conflict", err), err
	}
	selectedOccurrence := occurrence
	if selectedOccurrence <= 0 {
		selectedOccurrence = 1
	}
	nextContent := replaceOccurrence(content, oldText, newText, selectedOccurrence)
	afterHash := sha256Hex([]byte(nextContent))
	diff := unifiedDiffForEdit(filepath.ToSlash(rel), content, nextContent)
	added, deleted := diffLineStats(diff)
	diffHash := sha256Hex([]byte(diff))
	preview := map[string]any{
		"patchType":         "unified_diff",
		"targetRef":         filepath.ToSlash(rel),
		"diff":              diff,
		"summary":           "file/edit preview",
		"baseHash":          beforeHash,
		"contentHashBefore": beforeHash,
		"contentHashAfter":  afterHash,
		"addedLines":        added,
		"deletedLines":      deleted,
		"diffHash":          diffHash,
		"approvalRequired":  editApprovalRequired(filepath.ToSlash(rel), added+deleted),
	}
	return runtimemodel.StepExecutionResult{Output: map[string]any{
		"status":             "completed",
		"stage":              "file_edit_preview",
		"path":               filepath.ToSlash(rel),
		"baseHash":           beforeHash,
		"contentHashBefore":  beforeHash,
		"contentHashAfter":   afterHash,
		"matchCount":         matchCount,
		"selectedOccurrence": selectedOccurrence,
		"patchPreview":       preview,
	}}, nil
}

func guardedWorkspacePath(workingDirectory string, inputPath string, requireExisting bool) (string, string, string, error) {
	if strings.TrimSpace(workingDirectory) == "" || strings.TrimSpace(inputPath) == "" {
		return "", "", "", common.ErrInvalidArgument
	}
	if filepath.IsAbs(inputPath) {
		return "", "", "", common.ErrInvalidArgument
	}
	root, err := filepath.Abs(workingDirectory)
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

func stringInput(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func boolInput(input map[string]any, key string) bool {
	if input == nil {
		return false
	}
	value, _ := input[key].(bool)
	return value
}

func intInput(input map[string]any, key string, fallback int) int {
	if input == nil || input[key] == nil {
		return fallback
	}
	switch value := input[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		if value == float64(int(value)) {
			return int(value)
		}
	}
	return fallback
}

func rawStringInput(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, _ := input[key].(string)
	return value
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

func fileErrorResult(stage string, err error) runtimemodel.StepExecutionResult {
	return runtimemodel.StepExecutionResult{Output: map[string]any{
		"status": "failed",
		"stage":  stage,
		"error":  err.Error(),
	}}
}

var _ interface {
	ExecuteRuntimeStep(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error)
} = (*RuntimeAdapter)(nil)
