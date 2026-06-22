package workorchruntimecontract

import "github.com/muidea/workorch/internal/pkg/common"

const CoverageWorkItemSchema = "workorch.coverage_work_item.v1"

type CoverageWorkItem struct {
	ID                    common.CoverageWorkItemID `json:"id"`
	WorkspaceID           common.WorkspaceID        `json:"workspaceId"`
	RunID                 common.RunID              `json:"runId"`
	DefinitionRevision    string                    `json:"definitionRevision"`
	NormalizedSourceKey   string                    `json:"normalizedSourceKey"`
	SourceScope           string                    `json:"sourceScope,omitempty"`
	SourceRequirementID   string                    `json:"sourceRequirementId,omitempty"`
	SourceGoalID          common.GoalID             `json:"sourceGoalId,omitempty"`
	SourceDoDID           common.DoDID              `json:"sourceDoDId,omitempty"`
	SourcePath            string                    `json:"sourcePath,omitempty"`
	TargetText            string                    `json:"targetText,omitempty"`
	RequiredEvidenceKinds []string                  `json:"requiredEvidenceKinds,omitempty"`
	Status                string                    `json:"status"`
	AcceptedEvidenceRefs  []string                  `json:"acceptedEvidenceRefs,omitempty"`
	RejectedEvidenceRefs  []string                  `json:"rejectedEvidenceRefs,omitempty"`
	CurrentGap            string                    `json:"currentGap,omitempty"`
	UpdatedByStepID       common.StepID             `json:"updatedByStepId,omitempty"`
	CreatedAt             string                    `json:"createdAt,omitempty"`
	UpdatedAt             string                    `json:"updatedAt,omitempty"`
	Metadata              map[string]any            `json:"metadata,omitempty"`
}

func (item *CoverageWorkItem) IsActive() bool {
	if item == nil {
		return false
	}
	return CoverageWorkItemStatusActive(item.Status)
}

func CoverageWorkItemStatusActive(status string) bool {
	switch status {
	case "satisfied", "blocked", "skipped", "superseded":
		return false
	default:
		return true
	}
}
