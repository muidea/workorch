package events

import (
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestApprovalCommandResultCloneDetachesOwnerState(t *testing.T) {
	item := &approvalmodel.Approval{Context: workorcheventpayload.New(workorcheventpayload.StringField("key", "owner"))}
	cloned := (ApprovalCommandResult{Approval: item}).Clone()
	item.Context = workorcheventpayload.New(workorcheventpayload.StringField("key", "mutated"))
	value, _ := cloned.Approval.Context.String("key")
	if value != "owner" {
		t.Fatalf("cloned approval context = %q", value)
	}
}
