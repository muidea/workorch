package governancecore

import (
	"github.com/muidea/workorch/internal/pkg/governancesemantics"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func WithApprovalSemanticPayload(payload workorcheventpayload.Payload, semantic governancesemantics.ApprovalNormalization, source string) workorcheventpayload.Payload {
	return CloneApprovalPayload(payload).Merge(workorcheventpayload.New(workorcheventpayload.ObjectField("governanceSemantic", workorcheventpayload.New(
		workorcheventpayload.StringField("action", semantic.Action),
		workorcheventpayload.StringField("intent", semantic.Intent),
		workorcheventpayload.FloatField("confidence", semantic.Confidence),
		workorcheventpayload.StringField("rawInput", semantic.RawInput),
		workorcheventpayload.StringField("source", source),
		workorcheventpayload.BoolField("inferred", semantic.Inferred),
		workorcheventpayload.StringField("version", "v1"),
	))))
}
