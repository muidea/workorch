package readmodelprojection

import (
	"strconv"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ContextDoDSummary(dod map[string]any) string {
	if len(dod) == 0 {
		return ""
	}
	switch items := dod["items"].(type) {
	case []string:
		return strconv.Itoa(len(items)) + " DoD items"
	case []any:
		return strconv.Itoa(len(items)) + " DoD items"
	default:
		return ""
	}
}

func FormalOutputSummary(items []projectioncontract.Result) string {
	if result := PrimaryTaskProgressResult(items); result != nil {
		return result.Summary
	}
	return ""
}
