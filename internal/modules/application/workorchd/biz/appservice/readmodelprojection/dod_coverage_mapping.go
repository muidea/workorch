package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorchresultcontract"

func mapDoDCoverage(item *workorchresultcontract.DoDCoverage) *workorchresultcontract.DoDCoverage {
	if item == nil {
		return nil
	}
	ret := &workorchresultcontract.DoDCoverage{
		Satisfied: item.Satisfied,
		Summary:   item.Summary,
	}
	if len(item.Items) > 0 {
		ret.Items = make([]workorchresultcontract.DoDCoverageItem, 0, len(item.Items))
		for _, coverageItem := range item.Items {
			ret.Items = append(ret.Items, mapDoDCoverageItem(coverageItem))
		}
	}
	return ret
}

func mapDoDCoverageItem(item workorchresultcontract.DoDCoverageItem) workorchresultcontract.DoDCoverageItem {
	return workorchresultcontract.DoDCoverageItem{
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
