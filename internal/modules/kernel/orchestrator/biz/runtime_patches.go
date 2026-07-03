package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	requests, err := runtimePatchRequests(output)
	if err != nil {
		return err
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
		beforeState, err := workspacePatchBeforeStateForRequest(deliverableDirectory, request)
		if err != nil {
			patch.State = common.PatchStateFailed
			_ = p.evidence.Patches.Update(ctx, patch)
			if publishErr := p.publishChanged(ctx, run, step, toolCall, patch, orchestratorevents.TopicPatchFailed, err); publishErr != nil {
				return errors.Join(err, publishErr)
			}
			return err
		}
		patch.BeforeHash = beforeState.Hash
		patch.OriginalContent = beforeState.Content
		patch.OriginalExists = beforeState.Exists
		if err := applyWorkspacePatch(deliverableDirectory, request); err != nil {
			patch.State = common.PatchStateFailed
			_ = p.evidence.Patches.Update(ctx, patch)
			if publishErr := p.publishChanged(ctx, run, step, toolCall, patch, orchestratorevents.TopicPatchFailed, err); publishErr != nil {
				return errors.Join(err, publishErr)
			}
			return err
		}
		afterHash, err := workspacePatchAfterHash(deliverableDirectory, request.TargetRef)
		if err != nil {
			patch.State = common.PatchStateFailed
			_ = p.evidence.Patches.Update(ctx, patch)
			if publishErr := p.publishChanged(ctx, run, step, toolCall, patch, orchestratorevents.TopicPatchFailed, err); publishErr != nil {
				return errors.Join(err, publishErr)
			}
			return err
		}
		if strings.EqualFold(strings.TrimSpace(beforeState.Hash), strings.TrimSpace(afterHash)) {
			err = newPatchApplyDiagnosticError(
				request.TargetRef,
				request.PatchType,
				"patch_apply_noop",
				common.ErrInvalidArgument,
				fmt.Sprintf("invalid argument: runtime repair patch/apply %s for target %q is non-substantive", request.PatchType, request.TargetRef),
				"Return a substantive state-changing artifact, a bounded validation graph when the current snapshot already satisfies the intended change, or a real governance blocker.",
				"before/after content hashes are identical",
			)
			patch.State = common.PatchStateFailed
			patch.AfterHash = afterHash
			_ = p.evidence.Patches.Update(ctx, patch)
			if publishErr := p.publishChanged(ctx, run, step, toolCall, patch, orchestratorevents.TopicPatchFailed, err); publishErr != nil {
				return errors.Join(err, publishErr)
			}
			return err
		}
		patch.State = common.PatchStateApplied
		patch.AfterHash = afterHash
		patch.ReverseDiff = reverseDiffForPatch(request.TargetRef, beforeState.Content, beforeState.Exists)
		patch.AppliedAt = runtimeEvidenceTimestamp()
		if err := p.evidence.Patches.Update(ctx, patch); err != nil {
			return err
		}
		if err := p.recordAcceptedStateChangeEvidenceForPatch(ctx, run, step, toolCall, patch); err != nil {
			return err
		}
		annotateAppliedPatchOutput(output, idx, patch)
		if err := p.publishChanged(ctx, run, step, toolCall, patch, orchestratorevents.TopicPatchApplied, nil); err != nil {
			return err
		}
	}
	return nil
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
	if p == nil || p.evidence.Patches == nil {
		return common.ErrInvalidArgument
	}
	patch, err := p.evidence.Patches.Get(ctx, request.PatchID)
	if err != nil {
		return err
	}
	if patch.WorkspaceID != run.WorkspaceID || patch.RunID != run.ID {
		return common.ErrConflict
	}
	if patch.State != common.PatchStateApplied {
		return fmt.Errorf("%w: patch %s is not applied", common.ErrInvalidArgument, patch.ID)
	}
	if patch.AfterHash == "" {
		return fmt.Errorf("%w: patch %s has no afterHash and cannot be reverted", common.ErrInvalidArgument, patch.ID)
	}
	currentHash, err := workspacePatchAfterHash(deliverableDirectory, patch.TargetRef)
	if err != nil {
		return err
	}
	if !strings.EqualFold(currentHash, patch.AfterHash) {
		return fmt.Errorf("%w: patch revert hash mismatch", common.ErrConflict)
	}
	if patch.OriginalExists {
		target, err := workspaceTargetPath(deliverableDirectory, patch.TargetRef)
		if err != nil {
			return err
		}
		if err := ensureWorkspacePatchParentDirectory(deliverableDirectory, patch.TargetRef, target, "patch_revert"); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(patch.OriginalContent), 0o644); err != nil {
			return err
		}
	} else {
		target, err := workspaceTargetPath(deliverableDirectory, patch.TargetRef)
		if err != nil {
			return err
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	patch.State = common.PatchStateReverted
	patch.RevertApprovalID = request.ApprovalID
	patch.RevertedAt = runtimeEvidenceTimestamp()
	if err := p.evidence.Patches.Update(ctx, patch); err != nil {
		return err
	}
	output["patchRevert"] = map[string]any{
		"patchId":    string(patch.ID),
		"state":      string(patch.State),
		"targetRef":  patch.TargetRef,
		"beforeHash": patch.AfterHash,
		"afterHash":  patch.BeforeHash,
		"approvalId": string(request.ApprovalID),
	}
	return p.publishChanged(ctx, run, step, toolCall, patch, orchestratorevents.TopicPatchReverted, nil)
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

func (p *patchSubsystem) publishPreviewFromOutput(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, output map[string]any) error {
	if output == nil {
		return nil
	}
	preview := mapFromValue(output["patchPreview"])
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
