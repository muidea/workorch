package orchestrator

import (
	"context"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	evidenceRecordSourceTypePatch      = "patch"
	evidenceRecordSubjectWorkspaceItem = "workspace_target"
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
