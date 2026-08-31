package runtime

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeRepairSurfaceOwnerComesFromSealedProfile(t *testing.T) {
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("a", 64))
	convergence := runtimeValidationFailureRepairConvergenceForTest(1)
	convergence.RepairOwnerModuleID = runtimecontract.RuntimeModuleGit
	convergence.RepairOperationKinds = []string{runtimecontract.StepKindGitWrite}
	if err := convergence.Validate(); err != nil {
		t.Fatalf("synthetic foreign-owner repair convergence: %v", err)
	}
	value := convergence.Clone()
	aggregate.Facts.DefinitionFocuses = []runtimecontract.RuntimeDefinitionFocus{{
		ID: "definition-git-repair", CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), aggregate.Facts.Focus.CoverageWorkItemIDs...),
		ValidationFailureRepairConvergence: &value,
	}}

	owner, err := runtimeExpectedRepairSurfaceOwner(aggregate, observation)
	if err != nil {
		t.Fatalf("derive synthetic foreign-owner repair owner: %v", err)
	}
	if owner != runtimecontract.RuntimeModuleGit {
		var sourceOperation runtimecontract.RuntimeOperation
		for _, dispatch := range aggregate.CompletedDispatches {
			if dispatch.ID == observation.DispatchID {
				sourceOperation, _ = candidateOperationByID(dispatch.Candidate, observation.OperationID)
			}
		}
		t.Fatalf("repair owner = %q, want %q; operation=%+v declarations=%+v", owner, runtimecontract.RuntimeModuleGit, sourceOperation, aggregate.Facts.DefinitionFocuses)
	}

	confirmation := runtimecontract.RuntimeRepairSurfaceConfirmation{
		Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleGit,
		SubjectRef: "repository", Disposition: runtimecontract.RuntimeRepairSurfaceConfirmed,
		ConfirmedTargetRefs: append([]string(nil), observation.CandidateTargetRefs...), ObservedRevision: "repository-r1",
	}
	record, err := runtimecontract.NewRuntimeRepairSurfaceConfirmationRecord(confirmation)
	if err != nil {
		t.Fatalf("synthetic non-owner confirmation: %v", err)
	}
	aggregate.RepairSurfaceConfirmations = append(aggregate.RepairSurfaceConfirmations, record)
	resolved, err := runtimeConfirmedRepairSurfaceForTargets(aggregate, observation, observation.CandidateTargetRefs)
	if err != nil || resolved.OwnerModuleID != runtimecontract.RuntimeModuleGit {
		t.Fatalf("resolve synthetic non-owner confirmation: confirmation=%+v err=%v", resolved, err)
	}

	wrongOwner := confirmation
	wrongOwner.OwnerModuleID = runtimecontract.RuntimeModuleFile
	wrongRecord, err := runtimecontract.NewRuntimeRepairSurfaceConfirmationRecord(wrongOwner)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.RepairSurfaceConfirmations = []runtimecontract.RuntimeRepairSurfaceConfirmationRecord{wrongRecord}
	if _, err := runtimeConfirmedRepairSurfaceForTargets(aggregate, observation, observation.CandidateTargetRefs); err == nil {
		t.Fatal("confirmation owner outside sealed Profile was accepted")
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
