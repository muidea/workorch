package readmodelprojection

import (
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func newResultQualityTrace(item projectioncontract.Result) projectioncontract.ResultQualityTrace {
	return projectioncontract.ResultQualityTrace{
		ResultID:     item.ID,
		DoDID:        item.DoDID,
		State:        "passed",
		Summary:      strings.TrimSpace(item.Summary),
		EvidenceRefs: compactUniqueStrings(cloneReadModelStringSlice(item.EvidenceRefs)),
	}
}
