package activationcore

import (
	"strings"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
)

func RejectedContinuationDoDItems(items []resultmodel.DoDAssessmentItem, satisfied bool) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		state := strings.TrimSpace(item.State)
		isSatisfied := strings.EqualFold(state, "satisfied")
		if satisfied != isSatisfied {
			continue
		}
		mapped := map[string]any{
			"item":         CompactContextValue(item.Item, 1),
			"evidenceRefs": CompactStringRefs(item.EvidenceRefs, 10),
		}
		if !satisfied {
			mapped["state"] = state
			mapped["reason"] = CompactContextValue(item.FailureReason, 1)
		}
		ret = append(ret, mapped)
		if len(ret) >= 20 {
			break
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
