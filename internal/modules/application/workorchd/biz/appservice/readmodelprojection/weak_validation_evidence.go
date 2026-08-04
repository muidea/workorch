package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func weakValidationEvidenceRefIndex(snapshot readmodelcore.RunSnapshotState) map[string]bool {
	ret := map[string]bool{}
	for _, item := range snapshot.ValidationResults {
		quality := strings.TrimSpace(item.EvidenceQuality)
		if !item.ZeroWork && (quality == "" || quality == string(common.EvidenceQualityValid)) {
			continue
		}
		if ref := strings.TrimSpace(item.ID); ref != "" {
			ret[ref] = true
		}
		if ref := strings.TrimSpace(item.EvidenceRef); ref != "" {
			ret[ref] = true
		}
		if ref := strings.TrimSpace(item.StepID); ref != "" {
			ret[ref] = true
		}
	}
	return ret
}
