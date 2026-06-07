package governancecore

import governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"

func WithDecisionSemanticPayload(payload map[string]any, semantic governancecommon.DecisionSemanticNormalization) map[string]any {
	ret := CloneDecisionPayload(payload)
	if ret == nil {
		ret = map[string]any{}
	}
	ret["governanceSemantic"] = map[string]any{
		"resolutionType": semantic.ResolutionType,
		"intent":         semantic.Intent,
		"confidence":     semantic.Confidence,
		"rawInput":       semantic.RawInput,
		"source":         semantic.Source,
		"inferred":       semantic.Inferred,
		"version":        "v1",
	}
	return ret
}
