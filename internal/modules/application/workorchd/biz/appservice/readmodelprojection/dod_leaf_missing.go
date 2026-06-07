package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func missingRequiredDoDLeafItems(required []string, assessed []readmodelcore.DoDAssessmentItem) []string {
	if len(required) == 0 {
		return nil
	}
	covered := coveredDoDLeafItems(assessed)
	missing := make([]string, 0)
	for _, raw := range required {
		requiredNormalized := normalizeReadModelDoDText(raw)
		if requiredNormalized == "" {
			continue
		}
		if !isDoDLeafCovered(requiredNormalized, covered) {
			missing = append(missing, raw)
		}
	}
	return compactUniqueStrings(missing)
}

func coveredDoDLeafItems(assessed []readmodelcore.DoDAssessmentItem) []string {
	covered := make([]string, 0, len(assessed))
	for _, item := range assessed {
		if strings.TrimSpace(item.State) != "satisfied" {
			continue
		}
		normalized := normalizeReadModelDoDText(item.Item)
		if normalized != "" {
			covered = append(covered, normalized)
		}
	}
	return covered
}
