package orchestrator

import (
	"context"
	"fmt"
	"strings"

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

func (p *patchSubsystem) approvalBlockSpec(ctx context.Context, runID common.RunID, language string, output map[string]any) (runtimeApprovalBlockSpec, error) {
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
			approved, err := p.approved(ctx, runID, request.ApprovalID)
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
		Reason:     localizedPatchApprovalReason(language),
		ApprovalID: "",
		Action:     localizedPatchApprovalAction(language),
		Context:    patchApprovalContext(requirement),
	}, nil
}

func (p *patchSubsystem) approved(ctx context.Context, runID common.RunID, approvalID common.ApprovalID) (bool, error) {
	if p == nil || p.approvals == nil || approvalID == "" {
		return false, nil
	}
	item, err := p.approvals.Get(ctx, approvalID)
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

func (p *patchSubsystem) revertApprovalBlockSpec(ctx context.Context, runID common.RunID, language string, output map[string]any) (runtimeApprovalBlockSpec, error) {
	request, ok, err := runtimePatchRevertRequestFromOutput(output)
	if err != nil || !ok {
		return runtimeApprovalBlockSpec{}, err
	}
	if p == nil || p.evidence.Patches == nil {
		return runtimeApprovalBlockSpec{}, common.ErrInvalidArgument
	}
	patch, err := p.evidence.Patches.Get(ctx, request.PatchID)
	if err != nil {
		return runtimeApprovalBlockSpec{}, err
	}
	reasons := patchRevertApprovalReasons(patch)
	if len(reasons) == 0 {
		return runtimeApprovalBlockSpec{}, nil
	}
	if request.ApprovalID != "" {
		approved, err := p.approved(ctx, runID, request.ApprovalID)
		if err != nil {
			return runtimeApprovalBlockSpec{}, err
		}
		if approved {
			return runtimeApprovalBlockSpec{}, nil
		}
		return runtimeApprovalBlockSpec{}, fmt.Errorf("%w: patch revert approval %s is not approved", common.ErrBlocked, request.ApprovalID)
	}
	return runtimeApprovalBlockSpec{
		Reason: localizedPatchRevertApprovalReason(language),
		Action: localizedPatchRevertApprovalAction(language),
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
