package orchestrator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

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
