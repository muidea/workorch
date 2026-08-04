package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func newResultQualityTrace(item projectioncontract.Result) readmodelcore.ResultQualityTrace {
	return readmodelcore.ResultQualityTrace{
		ResultID:     item.ID,
		DoDID:        item.DoDID,
		State:        "passed",
		Summary:      strings.TrimSpace(item.Summary),
		EvidenceRefs: compactUniqueStrings(cloneReadModelStringSlice(item.EvidenceRefs)),
	}
}
