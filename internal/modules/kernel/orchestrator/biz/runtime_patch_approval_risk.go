package orchestrator

import (
	"path/filepath"
	"strings"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func patchRevertApprovalReasons(patch *runmodel.Patch) []string {
	if patch == nil {
		return nil
	}
	reasons := make([]string, 0, 3)
	if isSensitivePatchTarget(patch.TargetRef) {
		reasons = append(reasons, "sensitive_path")
	}
	if !patch.OriginalExists {
		reasons = append(reasons, "delete_created_file")
	}
	added, deleted := patchLineStats(patch)
	if added+deleted > patchApprovalMaxChangedLines {
		reasons = append(reasons, "large_change")
	}
	return reasons
}

func patchApprovalReasons(request runtimePatchRequest) []string {
	reasons := make([]string, 0, 3)
	if isSensitivePatchTarget(request.TargetRef) {
		reasons = append(reasons, "sensitive_path")
	}
	if isDeletePatchRequest(request) {
		reasons = append(reasons, "delete_file")
	}
	added, deleted := runtimePatchRequestLineStats(request)
	if added+deleted > patchApprovalMaxChangedLines {
		reasons = append(reasons, "large_change")
	}
	if isLockfilePatchTarget(request.TargetRef) && added+deleted > 50 {
		reasons = append(reasons, "lockfile_large_change")
	}
	return reasons
}

func runtimePatchRequestLineStats(request runtimePatchRequest) (int, int) {
	if strings.TrimSpace(request.Diff) != "" {
		filePatch, err := parseSingleFileUnifiedDiff(request.Diff)
		if err == nil {
			return unifiedDiffStats(filePatch)
		}
	}
	if strings.TrimSpace(request.Content) != "" {
		return countTextLines(request.Content), 0
	}
	if strings.TrimSpace(request.PatchType) == "text_replace" {
		return countTextLines(request.NewText), countTextLines(request.OldText)
	}
	return 0, 0
}

func isDeletePatchRequest(request runtimePatchRequest) bool {
	if strings.TrimSpace(request.Diff) == "" {
		return false
	}
	filePatch, err := parseSingleFileUnifiedDiff(request.Diff)
	return err == nil && filePatch.NewRef == "/dev/null"
}

func patchApprovalContext(requirement patchApprovalRequirement) map[string]any {
	return map[string]any{
		"kind":           "patch_approval_required",
		"riskReasons":    compactUniqueStrings(requirement.reasons),
		"patchRequests":  cloneMapSlice(requirement.patches),
		"approvalPolicy": "required_before_apply",
	}
}

func patchApprovalContextRequest(request runtimePatchRequest, reasons []string) map[string]any {
	added, deleted := runtimePatchRequestLineStats(request)
	return map[string]any{
		"patchType":    request.PatchType,
		"targetRef":    request.TargetRef,
		"summary":      request.Summary,
		"content":      request.Content,
		"diff":         request.Diff,
		"oldText":      request.OldText,
		"newText":      request.NewText,
		"baseHash":     request.BaseHash,
		"occurrence":   request.Occurrence,
		"addedLines":   added,
		"deletedLines": deleted,
		"riskReasons":  append([]string(nil), reasons...),
	}
}

func isSensitivePatchTarget(targetRef string) bool {
	clean := strings.ToLower(filepath.ToSlash(filepath.Clean(strings.TrimSpace(targetRef))))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "" || clean == "." {
		return false
	}
	base := filepath.Base(clean)
	switch {
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

func isLockfilePatchTarget(targetRef string) bool {
	base := strings.ToLower(filepath.Base(filepath.ToSlash(filepath.Clean(strings.TrimSpace(targetRef)))))
	ext := strings.TrimPrefix(filepath.Ext(base), ".")
	stem := strings.TrimSuffix(base, "."+ext)
	if base == "lock" || base == "lockfile" || ext == "lock" {
		return true
	}
	return strings.Contains(base, "lockfile") ||
		strings.Contains(base, ".lock.") ||
		strings.HasSuffix(stem, "-lock") ||
		strings.HasSuffix(stem, "_lock")
}
