package events

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestGetStepResultCloneDetachesOwnerState(t *testing.T) {
	step := &runmodel.Step{Input: workorcheventpayload.New(workorcheventpayload.StringField("key", "owner"))}
	cloned := (GetStepResult{Step: step}).Clone()
	step.Input = workorcheventpayload.New(workorcheventpayload.StringField("key", "mutated"))
	value, _ := cloned.Step.Input.String("key")
	if value != "owner" {
		t.Fatalf("cloned step input = %q", value)
	}
}
