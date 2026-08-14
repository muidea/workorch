package workorchruntimecontract

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const CoverageWorkItemSchema = "workorch.coverage_work_item"

type CoverageWorkItem struct {
	ID                    common.CoverageWorkItemID    `json:"id"`
	WorkspaceID           common.WorkspaceID           `json:"workspaceId"`
	RunID                 common.RunID                 `json:"runId"`
	DefinitionRevision    string                       `json:"definitionRevision"`
	NormalizedSourceKey   string                       `json:"normalizedSourceKey"`
	SourceScope           string                       `json:"sourceScope,omitempty"`
	SourceRequirementID   string                       `json:"sourceRequirementId,omitempty"`
	SourceGoalID          common.GoalID                `json:"sourceGoalId,omitempty"`
	SourceDoDID           common.DoDID                 `json:"sourceDoDId,omitempty"`
	SourcePath            string                       `json:"sourcePath,omitempty"`
	TargetText            string                       `json:"targetText,omitempty"`
	RequiredEvidenceKinds []string                     `json:"requiredEvidenceKinds,omitempty"`
	Status                string                       `json:"status"`
	AcceptedEvidenceRefs  []string                     `json:"acceptedEvidenceRefs,omitempty"`
	RejectedEvidenceRefs  []string                     `json:"rejectedEvidenceRefs,omitempty"`
	CurrentGap            string                       `json:"currentGap,omitempty"`
	UpdatedByStepID       common.StepID                `json:"updatedByStepId,omitempty"`
	CreatedAt             string                       `json:"createdAt,omitempty"`
	UpdatedAt             string                       `json:"updatedAt,omitempty"`
	Metadata              workorcheventpayload.Payload `json:"metadata,omitempty"`
}

func CloneCoverageWorkItem(item *CoverageWorkItem) *CoverageWorkItem {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.RequiredEvidenceKinds = append([]string(nil), item.RequiredEvidenceKinds...)
	cloned.AcceptedEvidenceRefs = append([]string(nil), item.AcceptedEvidenceRefs...)
	cloned.RejectedEvidenceRefs = append([]string(nil), item.RejectedEvidenceRefs...)
	cloned.Metadata = item.Metadata.Clone()
	return &cloned
}

func CloneCoverageWorkItems(items []*CoverageWorkItem) []*CoverageWorkItem {
	if items == nil {
		return nil
	}
	ret := make([]*CoverageWorkItem, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneCoverageWorkItem(item))
	}
	return ret
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
