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
	evidenceRecordSourceTypeRuntimeStep       = "runtime_step"
	evidenceRecordSourceTypeTargetSnapshot    = "target_snapshot"
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
	return o.recordValidationEvidenceRecordForStep(ctx, run, nil, validation)
}

func (o *RuntimeOrchestrator) recordValidationEvidenceRecordForStep(ctx context.Context, run *runModel.Run, step *runModel.Step, validation *runModel.ValidationResult) error {
	if o == nil || o.evidence.EvidenceRecords == nil || run == nil || validation == nil {
		return nil
	}
	record := evidenceRecordForValidationResult(run, validation)
	if record == nil {
		return common.ErrInvalidArgument
	}
	enrichEvidenceRecordWithRuntimeStepCoverageScope(record, step)
	enrichEvidenceRecordWithValidationFailureDiagnostic(record, step)
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

func enrichEvidenceRecordWithRuntimeStepCoverageScope(record *orchestratormodel.EvidenceRecord, step *runModel.Step) {
	if record == nil || step == nil {
		return
	}
	scope := runtimeStepCoverageScopeMetadata(step)
	if len(scope) == 0 {
		return
	}
	if record.Metadata == nil {
		record.Metadata = map[string]any{}
	}
	for key, value := range scope {
		record.Metadata[key] = value
	}
	if refs := stringSliceFromValue(scope["coverageWorkItemIds"]); len(refs) > 0 {
		record.CoverageItemRefs = compactUniqueStrings(append(record.CoverageItemRefs, refs...))
	}
}

func enrichEvidenceRecordWithValidationFailureDiagnostic(record *orchestratormodel.EvidenceRecord, step *runModel.Step) {
	if record == nil ||
		step == nil ||
		record.Kind != orchestratormodel.EvidenceKindValidation ||
		record.Status != orchestratormodel.EvidenceStatusRejected {
		return
	}
	diagnostic := validationFailureDiagnosticFromStep(step, nil)
	if len(diagnostic) == 0 {
		return
	}
	if record.Metadata == nil {
		record.Metadata = map[string]any{}
	}
	record.Metadata["validationFailureDiagnostic"] = diagnostic
	if signature := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeySignature)); signature != "" {
		record.Metadata["failureSignature"] = signature
	}
	if class := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass)); class != "" {
		record.Metadata["failureClass"] = class
	}
}

func evidenceRecordForValidationResult(run *runModel.Run, validation *runModel.ValidationResult) *orchestratormodel.EvidenceRecord {
	if run == nil || validation == nil || validation.ID == "" {
		return nil
	}
	status := evidenceStatusForValidationResult(validation)
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

func (o *RuntimeOrchestrator) recordAcceptedStateChangeEvidenceForRuntimeStep(ctx context.Context, run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall) error {
	if o == nil || o.evidence.EvidenceRecords == nil || run == nil || step == nil {
		return nil
	}
	record := acceptedStateChangeEvidenceRecordForRuntimeStep(run, step, toolCall)
	if record == nil {
		return nil
	}
	return repository.CreateOrUpdateEvidenceRecord(ctx, o.evidence.EvidenceRecords, record)
}

func acceptedStateChangeEvidenceRecordForRuntimeStep(run *runModel.Run, step *runModel.Step, toolCall *runModel.ToolCall) *orchestratormodel.EvidenceRecord {
	if run == nil || step == nil || step.ID == "" || step.State != common.StepStateCompleted {
		return nil
	}
	if !runtimeStepDeclaresStateChangingEvidence(step) || runtimeStepStateChangeEvidenceRecordedByDedicatedApply(step) {
		return nil
	}
	sourceRef := runtimeRawPayloadRef(run.WorkspaceID, run.ID, "steps", string(step.ID), "output.json")
	timestamp := runtimeEvidenceTimestamp()
	scope := runtimeStepCoverageScopeMetadata(step)
	metadata := map[string]any{
		"sourceStepId":    strings.TrimSpace(string(step.ID)),
		"stepKind":        strings.TrimSpace(step.Kind),
		"stateChanging":   true,
		"sourceOutputRef": sourceRef,
	}
	if toolCall != nil && toolCall.ID != "" {
		metadata["toolCallId"] = string(toolCall.ID)
	}
	for key, value := range scope {
		metadata[key] = value
	}
	if graphRootID := graphRootStepID(step); graphRootID != "" {
		metadata["graphRootStepId"] = string(graphRootID)
	}
	if nodeID := strings.TrimSpace(graphNodeID(step)); nodeID != "" {
		metadata["graphNodeId"] = nodeID
	}
	subjectID := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(scope, "targetRef"),
		firstStringFromSlice(stringSliceFromValue(scope["targetRefs"])),
		firstStringFromSlice(stringSliceFromValue(scope["coverageTargets"])),
		string(step.ID),
	))
	return &orchestratormodel.EvidenceRecord{
		ID:                 orchestratormodel.StableEvidenceRecordID(sourceRef, "runtime-step", "state-change"),
		WorkspaceID:        run.WorkspaceID,
		RunID:              firstNonEmptyRunID(step.RunID, run.ID),
		StepID:             step.ID,
		DefinitionRevision: strings.TrimSpace(run.DefinitionRevision),
		Kind:               orchestratormodel.EvidenceKindStateChange,
		Status:             orchestratormodel.EvidenceStatusAccepted,
		SourceType:         evidenceRecordSourceTypeRuntimeStep,
		SourceRef:          sourceRef,
		SubjectType:        evidenceRecordSubjectRuntimeStep,
		SubjectID:          subjectID,
		CoverageItemRefs:   stringSliceFromValue(scope["coverageWorkItemIds"]),
		CreatedAt:          timestamp,
		UpdatedAt:          timestamp,
		Metadata:           metadata,
	}
}

func runtimeStepDeclaresStateChangingEvidence(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	return boolValue(step.Input["stateChanging"]) || boolValue(step.Input["stateChange"])
}

func runtimeStepStateChangeEvidenceRecordedByDedicatedApply(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	switch strings.TrimSpace(step.Kind) {
	case "patch/apply", artifactApplyStepKind:
		return true
	default:
		return false
	}
}

func runtimeStepCoverageScopeMetadata(step *runModel.Step) map[string]any {
	ret := map[string]any{}
	if step == nil {
		return ret
	}
	addString := func(key, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			ret[key] = value
		}
	}
	addStrings := func(key string, values []string) {
		if values = compactUniqueStrings(values); len(values) > 0 {
			ret[key] = values
		}
	}
	input := step.Input
	output := step.Output
	requiredOutputs := []map[string]any{
		mapFromValue(input["requiredOutput"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]),
		mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["requiredOutput"]),
	}
	focusPayloads := []map[string]any{
		mapFromValue(input["coverageFocus"]),
		mapFromValue(mapFromValue(input["coverageWorkItemContext"])["currentFocus"]),
		mapFromValue(mapFromValue(input["acceptance"])["currentFocus"]),
	}
	addString("coverageFocusId", firstNonEmptyString(stringFromMap(input, "coverageFocusId"), stringFromMap(output, "coverageFocusId")))
	coverageWorkItemIDs := append(stringSliceFromValue(input["coverageWorkItemIds"]), stringSliceFromValue(output["coverageWorkItemIds"])...)
	addStrings("coverageTargets", runtimeStepCoverageTargetRefs(step))
	addString("targetRef", firstNonEmptyString(stringFromMap(input, "targetRef"), stringFromMap(output, "targetRef")))
	evidence := coverageValidationEvidenceMetadataFromStep(step)
	coverageWorkItemIDs = append(coverageWorkItemIDs, stringSliceFromValue(evidence["coverageWorkItemIds"])...)
	for _, payload := range requiredOutputs {
		coverageWorkItemIDs = append(coverageWorkItemIDs, stringSliceFromValue(payload["coverageWorkItemIds"])...)
	}
	for _, payload := range focusPayloads {
		coverageWorkItemIDs = append(coverageWorkItemIDs, stringSliceFromValue(payload["coverageWorkItemIds"])...)
		if id := strings.TrimSpace(stringFromMap(payload, "id")); strings.HasPrefix(id, "cwi-") {
			coverageWorkItemIDs = append(coverageWorkItemIDs, id)
		}
	}
	addStrings("coverageWorkItemIds", coverageWorkItemIDs)
	rawTargetRefs := append(stringSliceFromValue(input["targetRefs"]), stringSliceFromValue(output["targetRefs"])...)
	rawTargetRefs = append(rawTargetRefs, coverageValidationTargetRefsFromEvidence(evidence)...)
	for _, payload := range requiredOutputs {
		rawTargetRefs = append(rawTargetRefs, stringSliceFromValue(payload["targetRefs"])...)
		rawTargetRefs = append(rawTargetRefs, stringSliceFromValue(payload["structuredTargetRefs"])...)
	}
	for _, payload := range focusPayloads {
		rawTargetRefs = append(rawTargetRefs, stringSliceFromValue(payload["targetRefs"])...)
		rawTargetRefs = append(rawTargetRefs, stringSliceFromValue(payload["structuredTargetRefs"])...)
	}
	addStrings("targetRefs", coverageStructuredBindingRefsFromValue(rawTargetRefs))
	structuredTargetRefs := append(
		append(coverageStructuredBindingRefsFromValue(input["structuredTargetRefs"]), coverageStructuredBindingRefsFromValue(output["structuredTargetRefs"])...),
		coverageValidationStructuredTargetRefsFromEvidence(evidence)...,
	)
	for _, payload := range requiredOutputs {
		structuredTargetRefs = append(structuredTargetRefs, coverageStructuredBindingRefsFromValue(payload["structuredTargetRefs"])...)
	}
	for _, payload := range focusPayloads {
		structuredTargetRefs = append(structuredTargetRefs, coverageStructuredBindingRefsFromValue(payload["structuredTargetRefs"])...)
	}
	addStrings("structuredTargetRefs", compactUniqueStrings(structuredTargetRefs))
	observedClaims := coverageObservedClaimsFromEvidence(evidence)
	for _, payload := range requiredOutputs {
		observedClaims = append(observedClaims, stringSliceFromValue(payload["observedClaims"])...)
	}
	addStrings("observedClaims", observedClaims)
	addString("mode", stringFromMap(evidence, "mode"))
	addString("observationType", stringFromMap(evidence, "observationType"))
	addString("proofKind", firstNonEmptyString(stringFromMap(evidence, "proofKind"), stringFromMap(evidence, "mode")))
	addString("coverageAlignment", stringFromMap(evidence, "coverageAlignment"))
	if runtimeStepDeclaresStateChangingEvidence(step) {
		ret["stateChanging"] = true
	}
	return ret
}

func runtimeStepCoverageTargetRefs(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	refs := make([]string, 0)
	addFromPayload := func(payload map[string]any) {
		if len(payload) == 0 {
			return
		}
		refs = append(refs, stringSliceFromValue(payload["coverageTargets"])...)
		for _, target := range mapSliceFromValue(payload["coverageTargets"]) {
			refs = append(refs,
				stringFromMap(target, "id"),
				stringFromMap(target, "coverageTargetId"),
				stringFromMap(target, "coverageTargetToken"),
				stringFromMap(target, "sourceRequirementId"),
				stringFromMap(target, "targetRef"),
				stringFromMap(target, "targetText"),
			)
		}
	}
	addFromPayload(step.Input)
	addFromPayload(step.Output)
	return compactUniqueStrings(refs)
}

func firstStringFromSlice(values []string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func evidenceStatusForValidationResult(validation *runModel.ValidationResult) string {
	if validation == nil {
		return ""
	}
	switch validation.State {
	case common.ValidationResultStatePassed:
		if !validationEvidenceCanSatisfyDoD(qualityFromValidationResult(validation)) {
			return orchestratormodel.EvidenceStatusRejected
		}
		return orchestratormodel.EvidenceStatusAccepted
	case common.ValidationResultStateFailed, common.ValidationResultStateWarning:
		return orchestratormodel.EvidenceStatusRejected
	default:
		return ""
	}
}

func (o *RuntimeOrchestrator) recordAcceptedTargetSnapshotEvidenceForStep(ctx context.Context, run *runModel.Run, step *runModel.Step) error {
	if o == nil || o.evidence.EvidenceRecords == nil || run == nil || step == nil {
		return nil
	}
	record := acceptedTargetSnapshotEvidenceRecordForStep(run, step)
	if record == nil {
		return nil
	}
	return repository.CreateOrUpdateEvidenceRecord(ctx, o.evidence.EvidenceRecords, record)
}

func acceptedTargetSnapshotEvidenceRecordForStep(run *runModel.Run, step *runModel.Step) *orchestratormodel.EvidenceRecord {
	if run == nil || step == nil || step.ID == "" {
		return nil
	}
	output := mapFromValue(step.Output)
	if len(output) == 0 || strings.TrimSpace(stringFromMap(output, "stage")) != "file_read" {
		return nil
	}
	if !boolValue(output["exists"]) || boolValue(output["isBinary"]) || boolValue(output["truncated"]) || boolValue(output["redacted"]) {
		return nil
	}
	targetRef := strings.TrimSpace(firstNonEmptyString(stringFromMap(output, "path"), stringFromMap(output, "targetRef")))
	if targetRef == "" {
		return nil
	}
	content := stringFromMap(output, "content")
	if strings.TrimSpace(content) == "" || containsRuntimeRedactionMarker(content) {
		return nil
	}
	contentComplete := true
	if value, ok := output["contentComplete"]; ok {
		contentComplete = boolValue(value)
	}
	safeForFileWrite := true
	if value, ok := output["safeForFileWrite"]; ok {
		safeForFileWrite = boolValue(value)
	}
	if !contentComplete || !safeForFileWrite {
		return nil
	}
	contentHash := strings.TrimSpace(stringFromMap(output, "contentHash"))
	if contentHash == "" {
		contentHash = sha256HexBytes([]byte(content))
	}
	sourceRef := runtimeRawPayloadRef(run.WorkspaceID, run.ID, "steps", string(step.ID), "output.json")
	timestamp := runtimeEvidenceTimestamp()
	graphRootID := graphRootStepID(step)
	metadata := map[string]any{
		"targetRef":         targetRef,
		"contentHash":       contentHash,
		"contentComplete":   true,
		"safeForFileWrite":  true,
		"redacted":          false,
		"truncated":         false,
		"isBinary":          false,
		"snapshotIntegrity": "complete_unredacted_text",
		"snapshotValidity":  "valid_for_recorded_step_output",
		"capturedAt":        timestamp,
		"sourceStepId":      strings.TrimSpace(string(step.ID)),
		"sourceOutputRef":   sourceRef,
		"consumption":       "state_change_target_snapshot",
	}
	if graphRootID != "" {
		metadata["graphRootStepId"] = string(graphRootID)
	}
	if nodeID := strings.TrimSpace(graphNodeID(step)); nodeID != "" {
		metadata["graphNodeId"] = nodeID
	}
	if size, ok := output["size"]; ok {
		metadata["size"] = compactLLMContextValue(size, 1)
	}
	if source := strings.TrimSpace(stringFromMap(output, "source")); source != "" {
		metadata["source"] = source
	}
	return &orchestratormodel.EvidenceRecord{
		ID:                 orchestratormodel.StableEvidenceRecordID(sourceRef, "target-snapshot", targetRef),
		WorkspaceID:        run.WorkspaceID,
		RunID:              firstNonEmptyRunID(step.RunID, run.ID),
		StepID:             step.ID,
		DefinitionRevision: strings.TrimSpace(run.DefinitionRevision),
		Kind:               orchestratormodel.EvidenceKindTargetSnapshot,
		Status:             orchestratormodel.EvidenceStatusAccepted,
		SourceType:         evidenceRecordSourceTypeTargetSnapshot,
		SourceRef:          sourceRef,
		SubjectType:        evidenceRecordSubjectWorkspaceItem,
		SubjectID:          targetRef,
		CreatedAt:          timestamp,
		UpdatedAt:          timestamp,
		Metadata:           metadata,
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
