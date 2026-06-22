package governancecore

import governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"

func WithApprovalSemanticPayload(payload map[string]any, semantic governancecommon.ApprovalSemanticNormalization, source string) map[string]any {
	ret := CloneApprovalMap(payload)
	if ret == nil {
		ret = map[string]any{}
	}
	ret["governanceSemantic"] = map[string]any{
		"action":     semantic.Action,
		"intent":     semantic.Intent,
		"confidence": semantic.Confidence,
		"rawInput":   semantic.RawInput,
		"source":     source,
		"inferred":   semantic.Inferred,
		"version":    "v1",
	}
	return ret
}
