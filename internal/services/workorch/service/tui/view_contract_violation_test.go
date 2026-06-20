package tui

import (
	"strings"
	"testing"

	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeExecutionStatusTextShowsContractViolationProjection(t *testing.T) {
	item := &backend.RuntimeExecutionInfo{
		Pending: true,
		ContractViolation: &runtimemodel.ContractViolation{
			ViolatedRule:   "state_changing_required_missing",
			Recoverability: "retry_same_contract",
			Attempt:        1,
			MaxAttempts:    2,
		},
	}

	got := runtimeExecutionStatusText(item)
	for _, want := range []string{
		"contract violation",
		"state_changing_required_missing",
		"retry_same_contract",
		"attempt 1/2",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtimeExecutionStatusText = %q, want %q", got, want)
		}
	}
}
