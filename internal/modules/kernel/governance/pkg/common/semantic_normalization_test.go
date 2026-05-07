package common

import (
	"testing"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
)

func TestNormalizeDecisionResolutionInfersFromText(t *testing.T) {
	item := NormalizeDecisionResolution("auto", map[string]any{
		"answer":   "请重新规划执行路径",
		"operator": "tui",
	})
	if item.ResolutionType != decisioncommon.ResolutionTypeReplan {
		t.Fatalf("resolutionType = %q, want replan", item.ResolutionType)
	}
	if item.Intent != GovernanceIntentRequestReplan || item.Source != "tui" || item.Confidence < 0.9 {
		t.Fatalf("normalization = %+v, want replan intent with source and confidence", item)
	}
}

func TestNormalizeDecisionResolutionKeepsExplicitSupportedType(t *testing.T) {
	item := NormalizeDecisionResolution(decisioncommon.ResolutionTypeRejectCurrentPath, map[string]any{
		"answer": "继续执行",
		"source": "console",
	})
	if item.ResolutionType != decisioncommon.ResolutionTypeRejectCurrentPath || item.Inferred {
		t.Fatalf("normalization = %+v, want explicit resolution type without inference", item)
	}
	if item.Intent != GovernanceIntentRejectExecution {
		t.Fatalf("intent = %q, want reject execution for explicit reject_current_path", item.Intent)
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
