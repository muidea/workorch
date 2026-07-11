package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
)

func mapDoDCoverage(item *resultmodel.DoDCoverage) *readmodelcore.DoDCoverage {
	if item == nil {
		return nil
	}
	ret := &readmodelcore.DoDCoverage{
		Satisfied: item.Satisfied,
		Summary:   item.Summary,
	}
	if len(item.Items) > 0 {
		ret.Items = make([]readmodelcore.DoDCoverageItem, 0, len(item.Items))
		for _, coverageItem := range item.Items {
			ret.Items = append(ret.Items, mapDoDCoverageItem(coverageItem))
		}
	}
	return ret
}

func mapDoDCoverageItem(item resultmodel.DoDCoverageItem) readmodelcore.DoDCoverageItem {
	return readmodelcore.DoDCoverageItem{
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
