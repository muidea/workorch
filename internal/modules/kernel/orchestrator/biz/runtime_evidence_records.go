package orchestrator

import (
	"context"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	evidenceRecordSourceTypePatch             = "patch"
	evidenceRecordSourceTypeValidationResult  = "validation_result"
	evidenceRecordSourceTypeContractViolation = "contract_violation"
	evidenceRecordSubjectWorkspaceItem        = "workspace_target"
	evidenceRecordSubjectRuntimeStep          = "runtime_step"
)

func (p *patchSubsystem) recordAcceptedStateChangeEvidenceForPatch(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, patch *runModel.Patch) error {
	if p == nil || p.evidence.EvidenceRecords == nil || run == nil || step == nil || patch == nil {
		return nil
	}
	record := acceptedStateChangeEvidenceRecordForPatch(run, step, toolCall, patch)
	if record == nil {
		return common.ErrInvalidArgument
	}
	if err := p.evidence.EvidenceRecords.Create(ctx, record); err != nil {
		if err != common.ErrConflict {
			return err
		}
		existing, getErr := p.evidence.EvidenceRecords.Get(ctx, record.ID)
		if getErr != nil {
			return getErr
		}
		record.CreatedAt = firstNonEmptyString(existing.CreatedAt, record.CreatedAt)
		if err := p.evidence.EvidenceRecords.Update(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func acceptedStateChangeEvidenceRecordForPatch(run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall, patch *runModel.Patch) *orchestratormodel.EvidenceRecord {
	if run == nil || step == nil || patch == nil || patch.ID == "" {
		return nil
	}
	timestamp := firstNonEmptyString(strings.TrimSpace(patch.AppliedAt), runtimeEvidenceTimestamp())
	metadata := map[string]any{
		"patchId":    string(patch.ID),
		"patchType":  strings.TrimSpace(patch.PatchType),
		"targetRef":  strings.TrimSpace(patch.TargetRef),
		"beforeHash": strings.TrimSpace(patch.BeforeHash),
		"afterHash":  strings.TrimSpace(patch.AfterHash),
	}
	if toolCall != nil && toolCall.ID != "" {
		metadata["toolCallId"] = string(toolCall.ID)
	} else if patch.ToolCallID != "" {
		metadata["toolCallId"] = string(patch.ToolCallID)
	}
	if patch.ApprovalID != "" {
		metadata["approvalId"] = string(patch.ApprovalID)
	}
	if targets := stringSliceFromValue(patch.CoverageTargets); len(targets) > 0 {
		metadata["coverageTargets"] = targets
	}
	return &orchestratormodel.EvidenceRecord{
		ID:                 stateChangeEvidenceRecordIDForPatch(patch.ID),
		WorkspaceID:        firstNonEmptyWorkspaceID(patch.WorkspaceID, run.WorkspaceID),
		RunID:              firstNonEmptyRunID(patch.RunID, run.ID),
		StepID:             firstNonEmptyStepID(patch.StepID, step.ID),
		DefinitionRevision: strings.TrimSpace(run.DefinitionRevision),
		Kind:               orchestratormodel.EvidenceKindStateChange,
		Status:             orchestratormodel.EvidenceStatusAccepted,
		SourceType:         evidenceRecordSourceTypePatch,
		SourceRef:          string(patch.ID),
		SubjectType:        evidenceRecordSubjectWorkspaceItem,
		SubjectID:          strings.TrimSpace(patch.TargetRef),
		CreatedAt:          timestamp,
		UpdatedAt:          timestamp,
		Metadata:           metadata,
	}
}

func (o *RuntimeOrchestrator) recordValidationEvidenceRecord(ctx context.Context, run *runModel.Run, validation *runModel.ValidationResult) error {
	if o == nil || o.evidence.EvidenceRecords == nil || run == nil || validation == nil {
		return nil
	}
	record := evidenceRecordForValidationResult(run, validation)
	if record == nil {
		return common.ErrInvalidArgument
	}
	if err := o.evidence.EvidenceRecords.Create(ctx, record); err != nil {
		if err != common.ErrConflict {
			return err
		}
		existing, getErr := o.evidence.EvidenceRecords.Get(ctx, record.ID)
		if getErr != nil {
			return getErr
		}
		record.CreatedAt = firstNonEmptyString(existing.CreatedAt, record.CreatedAt)
		if err := o.evidence.EvidenceRecords.Update(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func evidenceRecordForValidationResult(run *runModel.Run, validation *runModel.ValidationResult) *orchestratormodel.EvidenceRecord {
	if run == nil || validation == nil || validation.ID == "" {
		return nil
	}
	status := evidenceStatusForValidationResult(validation.State)
	if status == "" {
		return nil
	}
	timestamp := firstNonEmptyString(strings.TrimSpace(validation.EndedAt), strings.TrimSpace(validation.StartedAt), runtimeEvidenceTimestamp())
	metadata := map[string]any{
		"validationResultId": string(validation.ID),
		"validationKind":     strings.TrimSpace(validation.ValidationKind),
		"validationState":    string(validation.State),
		"targetRef":          strings.TrimSpace(validation.TargetRef),
	}
	if validation.EvidenceRef != "" {
		metadata["evidenceRef"] = strings.TrimSpace(validation.EvidenceRef)
	}
	if validation.EvidenceQuality != "" {
		metadata["evidenceQuality"] = string(validation.EvidenceQuality)
	}
	if validation.EvidenceReason != "" {
		metadata["evidenceReason"] = strings.TrimSpace(validation.EvidenceReason)
	}
	if validation.ZeroWork {
		metadata["zeroWork"] = true
	}
	if len(validation.EvidenceTags) > 0 {
		metadata["evidenceTags"] = append([]string(nil), validation.EvidenceTags...)
	}
	if validation.Summary != "" {
		metadata["summary"] = strings.TrimSpace(validation.Summary)
	}
	return &orchestratormodel.EvidenceRecord{
		ID:                 validationEvidenceRecordID(validation.ID),
		WorkspaceID:        firstNonEmptyWorkspaceID(validation.WorkspaceID, run.WorkspaceID),
		RunID:              firstNonEmptyRunID(validation.RunID, run.ID),
		StepID:             validation.StepID,
		DefinitionRevision: strings.TrimSpace(run.DefinitionRevision),
		Kind:               orchestratormodel.EvidenceKindValidation,
		Status:             status,
		SourceType:         evidenceRecordSourceTypeValidationResult,
		SourceRef:          string(validation.ID),
		SubjectType:        evidenceRecordSubjectRuntimeStep,
		SubjectID:          strings.TrimSpace(firstNonEmptyString(validation.TargetRef, string(validation.StepID))),
		CreatedAt:          timestamp,
		UpdatedAt:          timestamp,
		Metadata:           metadata,
	}
}

func evidenceStatusForValidationResult(state common.ValidationResultState) string {
	switch state {
	case common.ValidationResultStatePassed:
		return orchestratormodel.EvidenceStatusAccepted
	case common.ValidationResultStateFailed, common.ValidationResultStateWarning:
		return orchestratormodel.EvidenceStatusRejected
	default:
		return ""
	}
}

func (o *RuntimeOrchestrator) recordRejectedDiagnosticEvidenceForContractViolation(ctx context.Context, run *runModel.Run, step *runModel.Step) error {
	if o == nil || o.evidence.EvidenceRecords == nil || run == nil || step == nil {
		return nil
	}
	record := rejectedDiagnosticEvidenceRecordForContractViolation(run, step)
	if record == nil {
		return nil
	}
	return repository.CreateOrUpdateEvidenceRecord(ctx, o.evidence.EvidenceRecords, record)
}

func rejectedDiagnosticEvidenceRecordForContractViolation(run *runModel.Run, step *runModel.Step) *orchestratormodel.EvidenceRecord {
	if run == nil || step == nil || step.ID == "" {
		return nil
	}
	violation := contractViolationFromFailedOutput(step)
	if len(violation) == 0 {
		return nil
	}
	sourceRef := strings.TrimSpace(stringFromMap(violation, "id"))
	if sourceRef == "" {
		sourceRef = strings.TrimSpace(string(step.ID)) + "/contract-violation"
	}
	timestamp := firstNonEmptyString(strings.TrimSpace(stringFromMap(violation, "createdAt")), runtimeEvidenceTimestamp())
	metadata := map[string]any{
		"contractViolationId": sourceRef,
		"sourceStepId":        strings.TrimSpace(firstNonEmptyString(stringFromMap(violation, "sourceStepId"), string(step.ID))),
		"violatedRule":        strings.TrimSpace(stringFromMap(violation, "violatedRule")),
		"violationType":       strings.TrimSpace(stringFromMap(step.Output, outputKeyContractViolationType)),
		"failureClass":        strings.TrimSpace(firstNonEmptyString(stringFromMap(violation, "failureClass"), stringFromMap(step.Output, failureOutputKeyClass))),
		"failureSignature":    strings.TrimSpace(firstNonEmptyString(stringFromMap(violation, "failureSignature"), stringFromMap(step.Output, failureOutputKeySignature))),
		"recoverability":      strings.TrimSpace(stringFromMap(violation, "recoverability")),
		"retryHint":           strings.TrimSpace(stringFromMap(violation, "retryHint")),
		"rejectedOutputRef":   strings.TrimSpace(stringFromMap(violation, "rejectedOutputRef")),
	}
	if runtimeStage := strings.TrimSpace(firstNonEmptyString(stringFromMap(violation, "runtimeStage"), runtimeStageFromStepInput(step.Input))); runtimeStage != "" {
		metadata["runtimeStage"] = runtimeStage
	}
	if stage := strings.TrimSpace(stringFromMap(violation, "stage")); stage != "" {
		metadata["stage"] = stage
	}
	if correctionMode := strings.TrimSpace(stringFromMap(violation, "correctionMode")); correctionMode != "" {
		metadata["correctionMode"] = correctionMode
	}
	if targetScope := strings.TrimSpace(stringFromMap(violation, "targetScope")); targetScope != "" {
		metadata["targetScope"] = targetScope
	}
	if refs := stringSliceFromValue(violation["sourceRefs"]); len(refs) > 0 {
		metadata["sourceRefs"] = refs
	}
	if expected := mapFromValue(violation["expectedContract"]); len(expected) > 0 {
		metadata["expectedContract"] = expected
	}
	if actual := mapFromValue(violation["actualOutputSummary"]); len(actual) > 0 {
		metadata["actualOutputSummary"] = actual
	}
	metadata["contractViolation"] = cloneStepPayload(violation)
	return &orchestratormodel.EvidenceRecord{
		ID:                 orchestratormodel.StableEvidenceRecordID(sourceRef, "contract-violation", "diagnostic"),
		WorkspaceID:        firstNonEmptyWorkspaceID(common.WorkspaceID(stringFromMap(violation, "workspaceId")), run.WorkspaceID),
		RunID:              firstNonEmptyRunID(common.RunID(stringFromMap(violation, "runId")), run.ID),
		StepID:             firstNonEmptyStepID(common.StepID(stringFromMap(violation, "stepId")), step.ID),
		DefinitionRevision: strings.TrimSpace(run.DefinitionRevision),
		Kind:               orchestratormodel.EvidenceKindDiagnostic,
		Status:             orchestratormodel.EvidenceStatusRejected,
		SourceType:         evidenceRecordSourceTypeContractViolation,
		SourceRef:          sourceRef,
		SubjectType:        evidenceRecordSubjectRuntimeStep,
		SubjectID:          strings.TrimSpace(string(step.ID)),
		CreatedAt:          timestamp,
		UpdatedAt:          timestamp,
		Metadata:           metadata,
	}
}

func stateChangeEvidenceRecordIDForPatch(patchID common.PatchID) common.EvidenceRecordID {
	raw := strings.TrimSpace(string(patchID))
	base := runtimeReadableIDPart(raw)
	if base == "" {
		base = "patch"
	}
	if len(base) > 48 {
		base = strings.Trim(base[:48], "-")
	}
	sum := sha256HexBytes([]byte(raw))
	if len(sum) > 12 {
		sum = sum[:12]
	}
	return common.EvidenceRecordID(base + "-" + sum + "-state-change")
}

func validationEvidenceRecordID(validationID common.ValidationResultID) common.EvidenceRecordID {
	raw := strings.TrimSpace(string(validationID))
	base := runtimeReadableIDPart(raw)
	if base == "" {
		base = "validation"
	}
	if len(base) > 48 {
		base = strings.Trim(base[:48], "-")
	}
	sum := sha256HexBytes([]byte(raw))
	if len(sum) > 12 {
		sum = sum[:12]
	}
	return common.EvidenceRecordID(base + "-" + sum + "-validation")
}

func firstNonEmptyWorkspaceID(values ...common.WorkspaceID) common.WorkspaceID {
	for _, value := range values {
		if strings.TrimSpace(string(value)) != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyRunID(values ...common.RunID) common.RunID {
	for _, value := range values {
		if strings.TrimSpace(string(value)) != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyStepID(values ...common.StepID) common.StepID {
	for _, value := range values {
		if strings.TrimSpace(string(value)) != "" {
			return value
		}
	}
	return ""
}
