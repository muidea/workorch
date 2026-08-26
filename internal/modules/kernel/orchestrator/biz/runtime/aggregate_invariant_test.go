package runtime

import (
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAggregateRejectsOwnerFactAcquisitionTerminalCredit(t *testing.T) {
	for _, retireManifest := range []bool{false, true} {
		name := "active_manifest"
		if retireManifest {
			name = "retired_manifest"
		}
		t.Run(name, func(t *testing.T) {
			_, repository, _, _ := prepareTerminalValidationOwnerFactAcquisitionAssessment(t, retireManifest)
			base := *repository.aggregate
			if err := base.Validate(); err != nil {
				t.Fatalf("valid acquisition aggregate rejected: %v", err)
			}

			corrupt := base
			corrupt.Facts = base.Facts.Normalized()
			corrupt.Facts.Coverage[0].State = runtimecontract.RuntimeCoverageItemStateSatisfied
			corrupt.Facts.Coverage[0].CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateSettled
			if err := corrupt.Validate(); err == nil || !strings.Contains(err.Error(), "acquisition CWI") {
				t.Fatalf("acquisition terminal credit validation err=%v", err)
			}
		})
	}
}

func TestRuntimeAggregateSemanticPurposeInvariantFailsClosedWithoutExactLineage(t *testing.T) {
	_, repository, _, _ := prepareTerminalValidationOwnerFactAcquisitionAssessment(t)
	base := *repository.aggregate

	withoutAssessment := base
	withoutAssessment.Facts = base.Facts.Normalized()
	withoutAssessment.Facts.Focus.RequireAssessment = false
	if err := validateRuntimeSemanticBindingPurposeInvariant(withoutAssessment); err == nil || !strings.Contains(err.Error(), "requires assessment") {
		t.Fatalf("acquisition without assessment validation err=%v", err)
	}

	missing := base
	missing.Facts = base.Facts.Normalized()
	ref := missing.Facts.Coverage[0].CurrentBindingRef.Clone()
	ref.SemanticManifestHash = "missing-semantic-manifest"
	missing.Facts.Coverage[0].CurrentBindingRef = &ref
	missing.Facts.Coverage[0].BindingLineage[len(missing.Facts.Coverage[0].BindingLineage)-1] = ref
	if err := validateRuntimeSemanticBindingPurposeInvariant(missing); err == nil || !strings.Contains(err.Error(), "no exact semantic purpose lineage") {
		t.Fatalf("missing semantic purpose validation err=%v", err)
	}
}

func TestRuntimeAggregateSemanticPurposeInvariantAllowsDefinitionTerminalCredit(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts = aggregate.Facts.Normalized()
	aggregate.Facts.Coverage[0].State = runtimecontract.RuntimeCoverageItemStateSatisfied
	aggregate.Facts.Coverage[0].CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateSettled
	if err := validateRuntimeSemanticBindingPurposeInvariant(aggregate); err != nil {
		t.Fatalf("Definition terminal proof was rejected: %v", err)
	}
}

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
			name: "sm4 non-terminal delivery budget",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Recovery.TerminalDeliveryAttempts = 1
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
