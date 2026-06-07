package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func newResultQualityTrace(item readmodelcore.Result) readmodelcore.ResultQualityTrace {
	return readmodelcore.ResultQualityTrace{
		ResultID:     item.ID,
		DoDID:        item.DoDID,
		State:        "passed",
		Summary:      strings.TrimSpace(item.Summary),
		EvidenceRefs: compactUniqueStrings(cloneReadModelStringSlice(item.EvidenceRefs)),
	}
}
