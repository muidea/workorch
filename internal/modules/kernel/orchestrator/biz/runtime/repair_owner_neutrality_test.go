package runtime

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeRepairSurfaceOwnerRequiresConcreteOwnerConfirmationProtocol(t *testing.T) {
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("a", 64))
	owner, protocol, err := currentRuntimeRepairSurfaceConfirmationContract(aggregate, observation)
	if err != nil {
		t.Fatalf("resolve sealed File repair confirmation contract: %v", err)
	}
	if owner != runtimecontract.RuntimeModuleFile || protocol != runtimecontract.RuntimeRepairSurfaceConfirmationProtocolFileWorkspaceRegularTargets {
		t.Fatalf("repair confirmation contract = owner %q protocol %q", owner, protocol)
	}

	convergence := runtimeValidationFailureRepairConvergenceForTest(1)
	for index := range convergence.Routes {
		convergence.Routes[index].RepairOwnerModuleID = runtimecontract.RuntimeModuleGit
		convergence.Routes[index].RepairOperationKinds = []string{runtimecontract.StepKindGitWrite}
	}
	if err := convergence.Validate(); err == nil || !strings.Contains(err.Error(), "confirmation protocol") {
		t.Fatalf("repair route without a Git-owned typed confirmation path was admitted: %v", err)
	}
}

func TestConfirmedRepairDispatchUsesConfirmationOwnerDescriptor(t *testing.T) {
	operation := runtimecontract.RuntimeOperation{
		ID: "git-repair", Kind: runtimecontract.StepKindGitWrite,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-repair"}, TargetRefs: []string{"repository://current"},
	}.Normalized()
	dispatch := SealedDispatch{
		Candidate: runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{
			Operations: []runtimecontract.StateChangingOperation{{Operation: operation}},
		}},
		OperationTargetAuthorizations: []runtimecontract.RuntimeDispatchOperationTargetAuthorization{{
			OperationID: operation.ID, OperationHash: runtimecontract.StableRuntimeOperationHash(operation), RequireTargets: true,
			Target: runtimecontract.RuntimeOperationTargetAuthorization{
				Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: append([]string(nil), operation.TargetRefs...),
			},
		}},
	}
	confirmation := runtimecontract.RuntimeRepairSurfaceConfirmation{
		OwnerModuleID:       runtimecontract.RuntimeModuleGit,
		Disposition:         runtimecontract.RuntimeRepairSurfaceConfirmed,
		ConfirmedTargetRefs: append([]string(nil), operation.TargetRefs...),
	}
	if err := validateConfirmedRepairDispatchOperations(dispatch, confirmation); err != nil {
		t.Fatalf("synthetic foreign-owner repair dispatch rejected: %v", err)
	}
	confirmation.OwnerModuleID = runtimecontract.RuntimeModuleFile
	if err := validateConfirmedRepairDispatchOperations(dispatch, confirmation); err == nil {
		t.Fatal("repair dispatch descriptor owner mismatch was accepted")
	}
}
