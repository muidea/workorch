package runtime

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAggregateValidationPhasesRejectOwnedInvalidState(t *testing.T) {
	base := validationAggregate(t)
	if err := base.Validate(); err != nil {
		t.Fatalf("valid aggregate rejected: %v", err)
	}

	tests := []struct {
		name     string
		mutate   func(*RuntimeAggregate)
		validate func(RuntimeAggregate) error
	}{
		{
			name: "sm2 authority",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.WorkspaceID = ""
			},
			validate: validateRuntimeAggregateSM2Authority,
		},
		{
			name: "sm3 active dispatch",
			mutate: func(value *RuntimeAggregate) {
				value.ActiveDispatch = &SealedDispatch{}
			},
			validate: validateRuntimeAggregateSM3ActiveDispatch,
		},
		{
			name: "activation fence",
			mutate: func(value *RuntimeAggregate) {
				value.Termination = &runtimecontract.RuntimeTerminationRecord{}
			},
			validate: validateRuntimeAggregateActivationFence,
		},
		{
			name: "sm3 candidate transaction",
			mutate: func(value *RuntimeAggregate) {
				value.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{}}
			},
			validate: validateRuntimeAggregateSM3CandidateTransaction,
		},
		{
			name: "governance ledger",
			mutate: func(value *RuntimeAggregate) {
				value.GovernanceRequests = []runtimecontract.RuntimeGovernanceRequestRecord{{}}
			},
			validate: validateRuntimeAggregateGovernanceLedgers,
		},
		{
			name: "sm4 delivery acknowledgement",
			mutate: func(value *RuntimeAggregate) {
				value.DeliveryAcknowledgements = []runtimecontract.RuntimeDeliveryAcknowledgement{{}}
			},
			validate: validateRuntimeAggregateSM4DeliveryAcknowledgements,
		},
		{
			name: "sm2 repair ledger",
			mutate: func(value *RuntimeAggregate) {
				value.PrerequisiteInvalidations = []runtimecontract.RuntimePrerequisiteInvalidationRecord{{}}
			},
			validate: validateRuntimeAggregateSM2RepairLedgers,
		},
		{
			name: "sm4 drive lease",
			mutate: func(value *RuntimeAggregate) {
				value.DriveLease = &runtimecontract.RuntimeDriveLease{}
			},
			validate: validateRuntimeAggregateSM4DriveLease,
		},
		{
			name: "cross-machine governance fence",
			mutate: func(value *RuntimeAggregate) {
				value.GovernanceRequests = []runtimecontract.RuntimeGovernanceRequestRecord{{}}
			},
			validate: validateRuntimeAggregateCrossMachineFences,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.mutate(&value)
			if err := test.validate(value); err == nil {
				t.Fatal("invalid state was accepted by its validation phase")
			}
		})
	}
}
