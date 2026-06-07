package activationcore

import (
	"strings"

	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
)

func RejectedContinuationDiagnostics(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{"gateFailureReason", "rejectionReason", "sourceSummary"} {
		if value, ok := input[key].(string); ok && strings.TrimSpace(value) != "" {
			ret[key] = CompactContextValue(value, 1)
		}
	}
	if diagnostics := mapValue(input["runtimeDiagnostics"]); len(diagnostics) > 0 {
		ret["runtimeDiagnostics"] = CompactContextValue(diagnostics, 1)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func RejectedContinuationRequiredEvidence(summary resultmodel.ResultSummary) []any {
	values := make([]string, 0, len(summary.NextActions)+len(summary.ResidualRisks)+len(summary.KnownLimitations))
	values = append(values, summary.NextActions...)
	values = append(values, summary.ResidualRisks...)
	values = append(values, summary.KnownLimitations...)
	return CompactStringRefs(values, 20)
}
