package readmodelprojection

import (
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mapDoDCoverage(item *resultmodel.DoDCoverage) *projectioncontract.DoDCoverage {
	if item == nil {
		return nil
	}
	ret := &projectioncontract.DoDCoverage{
		Satisfied: item.Satisfied,
		Summary:   item.Summary,
	}
	if len(item.Items) > 0 {
		ret.Items = make([]projectioncontract.DoDCoverageItem, 0, len(item.Items))
		for _, coverageItem := range item.Items {
			ret.Items = append(ret.Items, mapDoDCoverageItem(coverageItem))
		}
	}
	return ret
}

func mapDoDCoverageItem(item resultmodel.DoDCoverageItem) projectioncontract.DoDCoverageItem {
	return projectioncontract.DoDCoverageItem{
		ID:           item.ID,
		Deliverable:  item.Deliverable,
		Name:         item.Name,
		Path:         item.Path,
		Goal:         item.Goal,
		GoalItem:     item.GoalItem,
		Item:         item.Item,
		Requirement:  item.Requirement,
		State:        item.State,
		EvidenceRefs: cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs: dodCoverageArtifactRefs(item),
	}
}
