package readmodelprojection

import (
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestGovernanceStepIDs(t *testing.T) {
	t.Parallel()

	if got := DecisionStepID(nil); got != "" {
		t.Fatalf("nil decision step ID = %q, want empty", got)
	}
	if got := ApprovalStepID(nil); got != "" {
		t.Fatalf("nil approval step ID = %q, want empty", got)
	}

	if got := DecisionStepID(&decisionmodel.Ticket{StepID: common.StepID("step-decision")}); got != "step-decision" {
		t.Fatalf("decision step ID = %q, want step-decision", got)
	}
	if got := ApprovalStepID(&approvalmodel.Approval{StepID: common.StepID("step-approval")}); got != "step-approval" {
		t.Fatalf("approval step ID = %q, want step-approval", got)
	}
}
