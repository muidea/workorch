package governancesemantics

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestNormalizeDecisionResolutionInfersFromText(t *testing.T) {
	item := NormalizeDecisionResolution("auto", workorcheventpayload.New(
		workorcheventpayload.StringField("answer", "请重新规划执行路径"),
		workorcheventpayload.StringField("operator", "tui"),
	))
	if item.ResolutionType != DecisionResolutionTypeReplan || item.Intent != IntentRequestReplan || item.Source != "tui" || item.Confidence < 0.9 || !item.Inferred {
		t.Fatalf("normalization = %+v, want inferred replan", item)
	}
}

func TestNormalizeDecisionResolutionKeepsExplicitSupportedType(t *testing.T) {
	item := NormalizeDecisionResolution(DecisionResolutionTypeRejectCurrentPath, workorcheventpayload.New(
		workorcheventpayload.StringField("answer", "继续执行"),
	))
	if item.ResolutionType != DecisionResolutionTypeRejectCurrentPath || item.Inferred || item.Intent != IntentRejectExecution {
		t.Fatalf("normalization = %+v, want explicit rejection", item)
	}
}

func TestNormalizeApprovalActionInfersApproveAndReject(t *testing.T) {
	approve := NormalizeApprovalAction("", "同意继续执行", "请确认是否执行")
	if approve.Action != "approve" || approve.Intent != IntentApproveExecution || approve.Confidence <= 0 {
		t.Fatalf("approve normalization = %+v", approve)
	}
	reject := NormalizeApprovalAction("", "拒绝执行", "请确认是否执行")
	if reject.Action != "reject" || reject.Intent != IntentRejectExecution || reject.Confidence <= 0 {
		t.Fatalf("reject normalization = %+v", reject)
	}
}
