package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func CoverageSummaryFromRunSnapshot(snapshot readmodelcore.RunSnapshot) map[string]any {
	ctx := snapshot.CoverageWorkItemContext
	if len(ctx) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{"summary", "currentFocus", "omittedCount", "fullLedgerRef"} {
		if value, ok := ctx[key]; ok && value != nil {
			ret[key] = value
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
