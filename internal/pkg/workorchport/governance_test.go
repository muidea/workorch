package workorchport

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestNormalizeDecisionResolutionInfersReplan(t *testing.T) {
	got := NormalizeDecisionResolution("auto", workorcheventpayload.New(
		workorcheventpayload.StringField("answer", "请重新规划执行路径"),
		workorcheventpayload.StringField("operator", "tui"),
	))
	if got.ResolutionType != DecisionResolutionTypeReplan ||
		got.Intent != GovernanceIntentRequestReplan ||
		got.Source != "tui" ||
		!got.Inferred {
		t.Fatalf("normalization = %+v, want inferred replan from tui", got)
	}
}

func TestNormalizeDecisionResolutionKeepsExplicitSupportedType(t *testing.T) {
	got := NormalizeDecisionResolution(DecisionResolutionTypeRejectCurrentPath, workorcheventpayload.New(workorcheventpayload.StringField("answer", "继续执行")))
	if got.ResolutionType != DecisionResolutionTypeRejectCurrentPath || got.Inferred {
		t.Fatalf("normalization = %+v, want explicit reject_current_path", got)
	}
	if got.Intent != GovernanceIntentRejectExecution {
		t.Fatalf("intent = %q, want reject execution for explicit reject_current_path", got.Intent)
	}
}

func TestNormalizeApprovalActionInfersApproveAndReject(t *testing.T) {
	approve := NormalizeApprovalAction("", "同意继续执行", "请确认是否执行")
	if approve.Action != "approve" || approve.Intent != GovernanceIntentApproveExecution || approve.Confidence <= 0 {
		t.Fatalf("approve normalization = %+v, want inferred approve", approve)
	}

	reject := NormalizeApprovalAction("", "拒绝执行", "请确认是否执行")
	if reject.Action != "reject" || reject.Intent != GovernanceIntentRejectExecution || reject.Confidence <= 0 {
		t.Fatalf("reject normalization = %+v, want inferred reject", reject)
	}
}
