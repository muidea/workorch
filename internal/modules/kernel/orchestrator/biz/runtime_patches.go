package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type runtimePatchRequest struct {
	TargetRef       string
	Content         string
	Diff            string
	OldText         string
	NewText         string
	Summary         string
	PatchType       string
	BaseHash        string
	Occurrence      int
	ApprovalID      common.ApprovalID
	CoverageTargets any
	Extra           map[string]any
}

type runtimePatchRevertRequest struct {
	PatchID    common.PatchID
	ApprovalID common.ApprovalID
	Strict     bool
}

type workspacePatchBeforeState struct {
	Exists  bool
	Content string
	Hash    string
}

type runtimeFailureDiagnosticError interface {
	RuntimeFailureDiagnostics() map[string]any
}

type patchEventPublisher interface {
	publishPatchChanged(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, patch *runModel.Patch, topic string, failure error) error
	publishPatchPreview(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, preview map[string]any) error
}

type patchSubsystem struct {
	evidence  RuntimeEvidenceRepositories
	approvals repository.ApprovalRepository
	events    patchEventPublisher
}

func newPatchSubsystem(evidence RuntimeEvidenceRepositories, approvals repository.ApprovalRepository, events patchEventPublisher) *patchSubsystem {
	return &patchSubsystem{
		evidence:  evidence,
		approvals: approvals,
		events:    events,
	}
}

type patchApplyDiagnosticError struct {
	targetRef   string
	patchType   string
	stage       string
	reason      string
	inner       error
	guidance    string
	errorDetail string
}

func (e *patchApplyDiagnosticError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.reason) != "" {
		return e.reason
	}
	if e.inner != nil {
		return e.inner.Error()
	}
	return common.ErrInvalidArgument.Error()
}

func (e *patchApplyDiagnosticError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.inner
}

func (e *patchApplyDiagnosticError) RuntimeFailureDiagnostics() map[string]any {
	if e == nil {
		return nil
	}
	ret := map[string]any{
		"kind":      failureClassPatchApply,
		"targetRef": strings.TrimSpace(e.targetRef),
		"patchType": strings.TrimSpace(e.patchType),
		"stage":     firstNonEmptyString(strings.TrimSpace(e.stage), "patch_apply"),
	}
	if detail := strings.TrimSpace(e.errorDetail); detail != "" {
		ret["errorDetail"] = detail
	}
	if guidance := strings.TrimSpace(e.guidance); guidance != "" {
		ret["guidance"] = guidance
	}
	return ret
}

func newPatchApplyDiagnosticError(targetRef string, patchType string, stage string, inner error, reason string, guidance string, detail string) error {
	return &patchApplyDiagnosticError{
		targetRef:   strings.TrimSpace(targetRef),
		patchType:   strings.TrimSpace(patchType),
		stage:       strings.TrimSpace(stage),
		reason:      strings.TrimSpace(reason),
		inner:       inner,
		guidance:    strings.TrimSpace(guidance),
		errorDetail: strings.TrimSpace(detail),
	}
}

type workspacePathConflictError struct {
	Operation    string
	TargetRef    string
	ConflictRef  string
	ConflictKind string
}

func (e *workspacePathConflictError) Error() string {
	if e == nil {
		return ""
	}
	operation := strings.TrimSpace(e.Operation)
	if operation == "" {
		operation = "workspace patch"
	}
	conflictKind := strings.TrimSpace(e.ConflictKind)
	if conflictKind == "" {
		conflictKind = "file"
	}
	return fmt.Sprintf("%s path conflict: target %q requires %q as a directory, but it is an existing %s", operation, e.TargetRef, e.ConflictRef, conflictKind)
}

func (e *workspacePathConflictError) RuntimeFailureDiagnostics() map[string]any {
	if e == nil {
		return nil
	}
	return map[string]any{
		"kind":         "workspace_path_conflict",
		"operation":    e.Operation,
		"targetRef":    e.TargetRef,
		"conflictRef":  e.ConflictRef,
		"conflictKind": e.ConflictKind,
		"guidance":     "Choose a targetRef whose parent path is a directory, or write the resource at the existing workspace root. Do not treat an existing file as a directory.",
	}
}

func (p *patchSubsystem) apply(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	output map[string]any,
	deliverableDirectory string,
) error {
	blockRequest, blockApplied, err := runtimePatchRequestFromAppliedBlockResult(step, output)
	if err != nil {
		return err
	}
	requests := []runtimePatchRequest(nil)
	if blockApplied {
		requests = []runtimePatchRequest{blockRequest}
	} else {
		requests, err = runtimePatchRequests(output)
		if err != nil {
			return err
		}
		if len(requests) > 0 {
			request := requests[0]
			return newPatchApplyDiagnosticError(
				request.TargetRef,
				request.PatchType,
				"patch_apply_requires_runtime_block",
				common.ErrInvalidArgument,
				fmt.Sprintf("%s: patch/apply filesystem side effects must be executed by blocks/file runtime block before orchestrator admission", common.ErrInvalidArgument),
				"Dispatch a patch/apply runtime block and return its completed block result; orchestrator only admits formal Patch evidence after block execution.",
				"direct_orchestrator_patch_apply_disabled",
			)
		}
	}
	if len(requests) == 0 {
		return nil
	}
	if p == nil || p.evidence.Patches == nil {
		return common.ErrInvalidArgument
	}
	for idx, request := range requests {
		patchID := common.PatchID(fmt.Sprintf("%s/patch/%d", step.ID, idx+1))
		patch := &runModel.Patch{
			ID:              patchID,
			WorkspaceID:     run.WorkspaceID,
			RunID:           run.ID,
			StepID:          step.ID,
			ToolCallID:      toolCall.ID,
			ApprovalID:      request.ApprovalID,
			PatchType:       request.PatchType,
			TargetRef:       request.TargetRef,
			Content:         request.Content,
			Diff:            request.Diff,
			Summary:         request.Summary,
			CoverageTargets: request.CoverageTargets,
			AffectedFiles:   []string{request.TargetRef},
			State:           common.PatchStateCreated,
			CreatedAt:       runtimeEvidenceTimestamp(),
		}
		if err := p.evidence.Patches.Create(ctx, patch); err != nil {
			if err != common.ErrConflict {
				return err
			}
			if err := p.evidence.Patches.Update(ctx, patch); err != nil {
				return err
			}
		}
		if blockApplied {
			if err := p.admitAppliedBlockPatchResult(ctx, run, step, toolCall, output, idx, patch); err != nil {
				return err
			}
		}
	}
	return nil
}

func runtimePatchRequestFromAppliedBlockResult(step *runModel.Step, output map[string]any) (runtimePatchRequest, bool, error) {
	if step == nil || strings.TrimSpace(step.Kind) != "patch/apply" || output == nil {
		return runtimePatchRequest{}, false, nil
	}
	if strings.TrimSpace(stringFromMap(output, "status")) != "completed" ||
		strings.TrimSpace(stringFromMap(output, "stage")) != "patch_apply" {
		return runtimePatchRequest{}, false, nil
	}
	request, err := runtimePatchRequestFromMap(step.Input)
	if err != nil {
		return runtimePatchRequest{}, true, err
	}
	if targetRef := strings.TrimSpace(stringFromMap(output, "targetRef")); targetRef != "" && filepath.Clean(targetRef) != filepath.Clean(request.TargetRef) {
		return runtimePatchRequest{}, true, fmt.Errorf("%w: patch block result targetRef %q does not match step input targetRef %q", common.ErrConflict, targetRef, request.TargetRef)
	}
	if patchType := strings.TrimSpace(stringFromMap(output, "patchType")); patchType != "" && patchType != request.PatchType {
		return runtimePatchRequest{}, true, fmt.Errorf("%w: patch block result patchType %q does not match step input patchType %q", common.ErrConflict, patchType, request.PatchType)
	}
	return request, true, nil
}

func (p *patchSubsystem) admitAppliedBlockPatchResult(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	output map[string]any,
	idx int,
	patch *runModel.Patch,
) error {
	beforeHash := strings.TrimSpace(stringFromMap(output, "beforeHash"))
	afterHash := strings.TrimSpace(stringFromMap(output, "afterHash"))
	patch.AffectedFiles = patchResultAffectedFiles(output, patch.TargetRef)
	patch.AddedLines = intFromAny(output["addedLines"])
	patch.DeletedLines = intFromAny(output["deletedLines"])
	patch.RiskLevel = strings.TrimSpace(stringFromMap(output, "riskLevel"))
	patch.RiskReasons = stringSliceFromValue(output["riskReasons"])
	patch.Diagnostics = mapSliceFromValue(output["diagnostics"])
	if afterHash == "" {
		err := fmt.Errorf("%w: patch block result afterHash is required", common.ErrInvalidArgument)
		patch.Diagnostics = appendPatchDiagnostic(patch.Diagnostics, "patch_after_hash_required", err.Error())
		patch.State = common.PatchStateFailed
		_ = p.evidence.Patches.Update(ctx, patch)
		if publishErr := p.publishChanged(ctx, run, step, toolCall, patch, orchestratorevents.TopicPatchFailed, err); publishErr != nil {
			return errors.Join(err, publishErr)
		}
		return err
	}
	patch.BeforeHash = beforeHash
	patch.AfterHash = afterHash
	patch.OriginalExists = beforeHash != ""
	patch.OriginalContent = strings.TrimSpace(stringFromMap(output, "originalContent"))
	patch.ReverseDiff = strings.TrimSpace(stringFromMap(output, "reverseDiff"))
	if beforeHash != "" && strings.EqualFold(beforeHash, afterHash) {
		err := newPatchApplyDiagnosticError(
			patch.TargetRef,
			patch.PatchType,
			"patch_apply_noop",
			common.ErrInvalidArgument,
			fmt.Sprintf("invalid argument: runtime repair patch/apply %s for target %q is non-substantive", patch.PatchType, patch.TargetRef),
			"Return a substantive state-changing artifact, a bounded validation graph when the current snapshot already satisfies the intended change, or a real governance blocker.",
			"before/after content hashes are identical",
		)
		patch.Diagnostics = appendPatchDiagnostic(patch.Diagnostics, "patch_apply_noop", err.Error())
		patch.State = common.PatchStateFailed
		_ = p.evidence.Patches.Update(ctx, patch)
		if publishErr := p.publishChanged(ctx, run, step, toolCall, patch, orchestratorevents.TopicPatchFailed, err); publishErr != nil {
			return errors.Join(err, publishErr)
		}
		return err
	}
	patch.State = common.PatchStateApplied
	patch.AppliedAt = runtimeEvidenceTimestamp()
	if err := p.evidence.Patches.Update(ctx, patch); err != nil {
		return err
	}
	if err := p.recordAcceptedStateChangeEvidenceForPatch(ctx, run, step, toolCall, patch); err != nil {
		return err
	}
	annotateAppliedPatchOutput(output, idx, patch)
	annotateAppliedBlockPatchOutput(output, patch)
	return p.publishChanged(ctx, run, step, toolCall, patch, orchestratorevents.TopicPatchApplied, nil)
}

func annotateAppliedPatchOutput(output map[string]any, idx int, patch *runModel.Patch) {
	if output == nil || idx < 0 || patch == nil {
		return
	}
	annotations := map[string]any{
		"patchId":     string(patch.ID),
		"evidenceRef": string(patch.ID),
		"state":       string(patch.State),
		"toolCallId":  string(patch.ToolCallID),
		"beforeHash":  patch.BeforeHash,
		"afterHash":   patch.AfterHash,
	}
	switch items := output[stepOutputKeyPatches].(type) {
	case []map[string]any:
		if idx >= len(items) || items[idx] == nil {
			return
		}
		for key, value := range annotations {
			if strings.TrimSpace(fmt.Sprint(value)) != "" {
				items[idx][key] = value
			}
		}
	case []any:
		if idx >= len(items) {
			return
		}
		item, ok := items[idx].(map[string]any)
		if !ok || item == nil {
			return
		}
		for key, value := range annotations {
			if strings.TrimSpace(fmt.Sprint(value)) != "" {
				item[key] = value
			}
		}
	}
}

func annotateAppliedBlockPatchOutput(output map[string]any, patch *runModel.Patch) {
	if output == nil || patch == nil {
		return
	}
	patchOutput := map[string]any{
		"patchId":         string(patch.ID),
		"evidenceRef":     string(patch.ID),
		"state":           string(patch.State),
		"toolCallId":      string(patch.ToolCallID),
		"patchType":       patch.PatchType,
		"targetRef":       patch.TargetRef,
		"summary":         patch.Summary,
		"beforeHash":      patch.BeforeHash,
		"afterHash":       patch.AfterHash,
		"affectedFiles":   append([]string(nil), patch.AffectedFiles...),
		"addedLines":      patch.AddedLines,
		"deletedLines":    patch.DeletedLines,
		"riskLevel":       patch.RiskLevel,
		"riskReasons":     append([]string(nil), patch.RiskReasons...),
		"coverageTargets": patch.CoverageTargets,
	}
	if len(patch.Diagnostics) > 0 {
		patchOutput["diagnostics"] = cloneMapSlice(patch.Diagnostics)
	}
	output["patchId"] = string(patch.ID)
	output["evidenceRef"] = string(patch.ID)
	output["state"] = string(patch.State)
	output["toolCallId"] = string(patch.ToolCallID)
	output[stepOutputKeyPatches] = []map[string]any{patchOutput}
}

func patchResultAffectedFiles(output map[string]any, fallback string) []string {
	affected := compactUniqueStrings(stringSliceFromValue(output["affectedFiles"]))
	if len(affected) > 0 {
		return affected
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return nil
	}
	return []string{fallback}
}

func appendPatchDiagnostic(items []map[string]any, code string, message string) []map[string]any {
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if code == "" && message == "" {
		return cloneMapSlice(items)
	}
	return append(cloneMapSlice(items), map[string]any{
		"code":    code,
		"message": message,
	})
}

func (p *patchSubsystem) preflightApprovalCandidates(output map[string]any, deliverableDirectory string) error {
	requests, err := runtimePatchRequests(output)
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		return nil
	}
	for _, request := range requests {
		if request.ApprovalID != "" {
			continue
		}
		reasons := patchApprovalReasons(request)
		if len(requests) > patchApprovalMaxFiles {
			reasons = append(reasons, "many_files")
		}
		if len(reasons) == 0 {
			continue
		}
		if err := preflightWorkspacePatch(deliverableDirectory, request); err != nil {
			return err
		}
	}
	return nil
}

func (p *patchSubsystem) revert(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	output map[string]any,
	deliverableDirectory string,
) error {
	request, ok, err := runtimePatchRevertRequestFromOutput(output)
	if err != nil || !ok {
		return err
	}
	return fmt.Errorf("%w: patch/revert is phase-gated until blocks/file receives formal reverse material for patch %s", common.ErrInvalidArgument, request.PatchID)
}

func runtimePatchRevertRequestFromOutput(output map[string]any) (runtimePatchRevertRequest, bool, error) {
	if output == nil || output["patchRevert"] == nil {
		return runtimePatchRevertRequest{}, false, nil
	}
	value := mapFromValue(output["patchRevert"])
	if len(value) == 0 {
		return runtimePatchRevertRequest{}, false, common.ErrInvalidArgument
	}
	patchID := common.PatchID(strings.TrimSpace(stringFromAny(value["patchId"])))
	if patchID == "" {
		return runtimePatchRevertRequest{}, false, common.ErrInvalidArgument
	}
	return runtimePatchRevertRequest{
		PatchID:    patchID,
		ApprovalID: common.ApprovalID(strings.TrimSpace(stringFromAny(value["approvalId"]))),
		Strict:     boolFromAnyDefault(value["strict"], true),
	}, true, nil
}

func boolFromAnyDefault(value any, fallback bool) bool {
	if value == nil {
		return fallback
	}
	typed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return typed
}

func (p *patchSubsystem) publishPreviewFromOutput(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, output map[string]any) error {
	if output == nil {
		return nil
	}
	preview := mapFromValue(output["patchPreview"])
	if len(preview) == 0 &&
		strings.TrimSpace(stringFromMap(output, "status")) == "completed" &&
		strings.TrimSpace(stringFromMap(output, "stage")) == "patch_preview" &&
		strings.TrimSpace(stringFromMap(output, "targetRef")) != "" {
		preview = cloneStepPayload(output)
	}
	if len(preview) == 0 {
		return nil
	}
	return p.publishPreview(ctx, run, step, toolCall, preview)
}

func (p *patchSubsystem) publishChanged(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, patch *runModel.Patch, topic string, failure error) error {
	if p == nil || p.events == nil {
		return nil
	}
	return p.events.publishPatchChanged(ctx, run, step, toolCall, patch, topic, failure)
}

func (p *patchSubsystem) publishPreview(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, preview map[string]any) error {
	if p == nil || p.events == nil {
		return nil
	}
	return p.events.publishPatchPreview(ctx, run, step, toolCall, preview)
}
