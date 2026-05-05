package orchestrator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	patchApprovalMaxFiles        = 10
	patchApprovalMaxChangedLines = 500
)

type patchApprovalRequirement struct {
	required bool
	reasons  []string
	patches  []map[string]any
}

func (o *RuntimeOrchestrator) patchApprovalBlockSpec(ctx context.Context, runID common.RunID, output map[string]any) (runtimeApprovalBlockSpec, error) {
	requests, err := runtimePatchRequests(output)
	if err != nil || len(requests) == 0 {
		return runtimeApprovalBlockSpec{}, err
	}
	requirement := patchApprovalRequirement{}
	for _, request := range requests {
		reasons := patchApprovalReasons(request)
		if len(requests) > patchApprovalMaxFiles {
			reasons = append(reasons, "many_files")
		}
		if len(reasons) == 0 {
			continue
		}
		if request.ApprovalID != "" {
			approved, err := o.patchApprovalApproved(ctx, runID, request.ApprovalID)
			if err != nil {
				return runtimeApprovalBlockSpec{}, err
			}
			if approved {
				continue
			}
			return runtimeApprovalBlockSpec{}, fmt.Errorf("%w: patch approval %s is not approved", common.ErrBlocked, request.ApprovalID)
		}
		requirement.required = true
		requirement.reasons = append(requirement.reasons, reasons...)
		requirement.patches = append(requirement.patches, patchApprovalContextRequest(request, reasons))
	}
	if !requirement.required {
		return runtimeApprovalBlockSpec{}, nil
	}
	return runtimeApprovalBlockSpec{
		Reason:     "Patch requires approval before workspace files can be modified.",
		ApprovalID: "",
		Action:     "Approve applying the pending workspace patch.",
		Context:    patchApprovalContext(requirement),
	}, nil
}

func (o *RuntimeOrchestrator) patchApprovalApproved(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) (bool, error) {
	if o == nil || o.approvals == nil || approvalID == "" {
		return false, nil
	}
	item, err := o.approvals.Get(ctx, approvalID)
	if err != nil {
		return false, err
	}
	return item.RunID == runID && item.State == common.ApprovalStateApproved, nil
}

func applyPatchApprovalBlockOutput(output map[string]any, spec runtimeApprovalBlockSpec) {
	if output == nil || spec.Reason == "" {
		return
	}
	pending := cloneStepPayload(spec.Context)
	delete(output, stepOutputKeyPatches)
	output["pendingPatchApproval"] = pending
	output[stepOutputKeyBlockApprovalReason] = spec.Reason
	output[stepOutputKeyBlockApprovalAction] = spec.Action
	output[stepOutputKeyBlockApprovalContext] = cloneStepPayload(spec.Context)
	output[stepOutputKeyNextStep] = "llm-after-patch-approval"
	output[stepOutputKeyNextKind] = defaultInitialStepKind
	output[stepOutputKeyNextInput] = map[string]any{
		"pendingPatchApproval": pending,
	}
}

func (o *RuntimeOrchestrator) patchRevertApprovalBlockSpec(ctx context.Context, runID common.RunID, output map[string]any) (runtimeApprovalBlockSpec, error) {
	request, ok, err := runtimePatchRevertRequestFromOutput(output)
	if err != nil || !ok {
		return runtimeApprovalBlockSpec{}, err
	}
	if o == nil || o.evidence.Patches == nil {
		return runtimeApprovalBlockSpec{}, common.ErrInvalidArgument
	}
	patch, err := o.evidence.Patches.Get(ctx, request.PatchID)
	if err != nil {
		return runtimeApprovalBlockSpec{}, err
	}
	reasons := patchRevertApprovalReasons(patch)
	if len(reasons) == 0 {
		return runtimeApprovalBlockSpec{}, nil
	}
	if request.ApprovalID != "" {
		approved, err := o.patchApprovalApproved(ctx, runID, request.ApprovalID)
		if err != nil {
			return runtimeApprovalBlockSpec{}, err
		}
		if approved {
			return runtimeApprovalBlockSpec{}, nil
		}
		return runtimeApprovalBlockSpec{}, fmt.Errorf("%w: patch revert approval %s is not approved", common.ErrBlocked, request.ApprovalID)
	}
	return runtimeApprovalBlockSpec{
		Reason: "Patch revert requires approval before workspace files can be modified.",
		Action: "Approve reverting the pending workspace patch.",
		Context: map[string]any{
			"kind":           "patch_revert_approval_required",
			"patchId":        string(request.PatchID),
			"targetRef":      patch.TargetRef,
			"riskReasons":    reasons,
			"approvalPolicy": "required_before_revert",
		},
	}, nil
}

func applyPatchRevertApprovalBlockOutput(output map[string]any, spec runtimeApprovalBlockSpec) {
	if output == nil || spec.Reason == "" {
		return
	}
	pending := cloneStepPayload(spec.Context)
	delete(output, "patchRevert")
	output["pendingPatchRevertApproval"] = pending
	output[stepOutputKeyBlockApprovalReason] = spec.Reason
	output[stepOutputKeyBlockApprovalAction] = spec.Action
	output[stepOutputKeyBlockApprovalContext] = cloneStepPayload(spec.Context)
	output[stepOutputKeyNextStep] = "llm-after-patch-revert-approval"
	output[stepOutputKeyNextKind] = defaultInitialStepKind
	output[stepOutputKeyNextInput] = map[string]any{
		"pendingPatchRevertApproval": pending,
	}
}

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
		"baseHash":     request.BaseHash,
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
	switch base {
	case "go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "composer.lock", "poetry.lock", "cargo.lock", "gemfile.lock":
		return true
	default:
		return false
	}
}

func cloneMapSlice(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, cloneStepPayload(item))
	}
	return ret
}

func compactUniqueStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		ret = append(ret, item)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
