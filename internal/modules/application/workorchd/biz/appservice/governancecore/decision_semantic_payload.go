package governancecore

import (
	governancecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func WithDecisionSemanticPayload(payload workorcheventpayload.Payload, semantic governancecommon.DecisionSemanticNormalization) workorcheventpayload.Payload {
	return CloneDecisionPayload(payload).Merge(workorcheventpayload.New(workorcheventpayload.ObjectField("governanceSemantic", workorcheventpayload.New(
		workorcheventpayload.StringField("resolutionType", semantic.ResolutionType),
		workorcheventpayload.StringField("intent", semantic.Intent),
		workorcheventpayload.FloatField("confidence", semantic.Confidence),
		workorcheventpayload.StringField("rawInput", semantic.RawInput),
		workorcheventpayload.StringField("source", semantic.Source),
		workorcheventpayload.BoolField("inferred", semantic.Inferred),
		workorcheventpayload.StringField("version", "v1"),
	))))
}
