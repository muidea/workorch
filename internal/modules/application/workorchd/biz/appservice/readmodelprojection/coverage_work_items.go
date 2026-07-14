package readmodelprojection

import (
	"sort"
	"strings"

	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func MapCoverageWorkItems(items []*orchestratormodel.CoverageWorkItem) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	sorted := append([]*orchestratormodel.CoverageWorkItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left := sorted[i]
		right := sorted[j]
		if left == nil || right == nil {
			return right != nil
		}
		for _, compare := range []int{
			strings.Compare(left.DefinitionRevision, right.DefinitionRevision),
			strings.Compare(left.NormalizedSourceKey, right.NormalizedSourceKey),
			strings.Compare(string(left.ID), string(right.ID)),
		} {
			if compare < 0 {
				return true
			}
			if compare > 0 {
				return false
			}
		}
		return false
	})
	ret := make([]map[string]any, 0, len(sorted))
	for _, item := range sorted {
		mapped := mapCoverageWorkItem(item)
		if len(mapped) > 0 {
			ret = append(ret, mapped)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapCoverageWorkItem(item *orchestratormodel.CoverageWorkItem) map[string]any {
	if item == nil || item.ID == "" {
		return nil
	}
	ret := cloneReadModelMap(item.Metadata.Map())
	if ret == nil {
		ret = map[string]any{}
	}
	if strings.TrimSpace(stringValue(ret, "id")) == "" {
		ret["id"] = string(item.ID)
	}
	ret["repositoryId"] = string(item.ID)
	if item.WorkspaceID != "" {
		ret["workspaceId"] = string(item.WorkspaceID)
	}
	if item.RunID != "" {
		ret["runId"] = string(item.RunID)
	}
	if item.DefinitionRevision != "" {
		ret["definitionRevision"] = item.DefinitionRevision
	}
	if item.NormalizedSourceKey != "" {
		ret["normalizedSourceKey"] = item.NormalizedSourceKey
	}
	if item.SourceScope != "" {
		ret["sourceScope"] = item.SourceScope
	}
	if item.SourceRequirementID != "" {
		ret["sourceRequirementId"] = item.SourceRequirementID
	}
	if item.SourceGoalID != "" {
		ret["sourceGoalId"] = string(item.SourceGoalID)
	}
	if item.SourceDoDID != "" {
		ret["sourceDoDId"] = string(item.SourceDoDID)
	}
	if item.SourcePath != "" {
		ret["sourcePath"] = item.SourcePath
	}
	if item.TargetText != "" {
		ret["targetText"] = item.TargetText
	}
	if len(item.RequiredEvidenceKinds) > 0 {
		ret["requiredEvidenceKinds"] = append([]string(nil), item.RequiredEvidenceKinds...)
	}
	if item.Status != "" {
		ret["status"] = item.Status
	}
	if len(item.AcceptedEvidenceRefs) > 0 {
		ret["acceptedEvidenceRefs"] = append([]string(nil), item.AcceptedEvidenceRefs...)
	}
	if len(item.RejectedEvidenceRefs) > 0 {
		ret["rejectedEvidenceRefs"] = append([]string(nil), item.RejectedEvidenceRefs...)
	}
	if item.CurrentGap != "" {
		ret["currentGap"] = item.CurrentGap
	}
	if item.UpdatedByStepID != "" {
		ret["updatedByStepId"] = string(item.UpdatedByStepID)
	}
	return ret
}
