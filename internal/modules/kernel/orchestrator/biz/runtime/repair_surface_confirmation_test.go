package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRepairSurfaceConfirmationCreatesFreshStateChangeAuthority(t *testing.T) {
	aggregate := validationAggregate(t)
	aggregate.Facts.Focus.TargetRole = "implementation"
	aggregate.Facts.Focus.RequireTargetGrounding = true
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	sealed.ActiveDispatch.State = DispatchStateCommitted
	sealed.CompletedDispatches = append(sealed.CompletedDispatches, *sealed.ActiveDispatch)
	sealed.ActiveDispatch = nil
	repository.aggregate = sealed
	aggregate = sealed
	oldAuthority := aggregate.Decision.AuthoritySnapshot.ID
	observation := runtimecontract.RuntimeValidationFailureObservation{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, AuthoritySnapshotID: oldAuthority,
		DispatchID: aggregate.CompletedDispatches[0].ID, OperationID: "validate-1", FailureSignature: "build-failure", TargetRole: "implementation",
		DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed, CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: []string{"main.go"}, DiagnosticHash: strings.Repeat("a", 64),
	}
	aggregate.Facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassBuildFailure, Code: "build_failed", Signature: observation.FailureSignature, Message: "implementation does not compile", ValidationFailureObservation: &observation}
	repository.aggregate = aggregate
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.recordValidationFailureObservation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, observation); err != nil {
		t.Fatalf("validation fact identity: %v", err)
	}
	confirmation := runtimecontract.RuntimeRepairSurfaceConfirmation{Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Disposition: runtimecontract.RuntimeRepairSurfaceConfirmed, ConfirmedTargetRefs: []string{"main.go"}, ObservedRevision: "tree-v1"}
	confirmed, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, confirmation)
	if err != nil {
		t.Fatalf("confirm repair surface: %v", err)
	}
	if confirmed.Facts.Resources.FailureTarget == nil || confirmed.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("confirmation did not establish File repair authority: %+v", confirmed)
	}
	var snapshotID string
	for _, prerequisite := range confirmed.Facts.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && prerequisite.SubjectRef == "main.go" {
			snapshotID = prerequisite.ID
		}
	}
	if snapshotID == "" {
		t.Fatalf("confirmed repair target did not require fresh File snapshot: %+v", confirmed.Facts.Prerequisites.Pending)
	}
	resumed, err := controller.commitPrerequisite(context.Background(), confirmed.Facts.WorkspaceID, confirmed.Facts.RunID, PrerequisiteResult{PrerequisiteID: snapshotID, TargetSnapshot: &runtimecontract.TargetSnapshot{TargetRef: "main.go", Content: "package main", ContentHash: "content-v1", ContentState: runtimecontract.SnapshotContentStateComplete}})
	if err != nil {
		t.Fatalf("commit fresh target snapshot: %v", err)
	}
	if resumed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || resumed.Decision.AuthoritySnapshot.ID == oldAuthority || resumed.Decision.AuthoritySnapshot.Attempt != 0 {
		t.Fatalf("repair did not create a new attempt-0 state-change authority: %+v", resumed.Decision)
	}
}

func TestRejectedRepairSurfaceFailsBeforeAnotherValidationCandidate(t *testing.T) {
	aggregate := validationAggregate(t)
	aggregate.Facts.Focus.TargetRole = "implementation"
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	sealed.ActiveDispatch.State = DispatchStateCommitted
	sealed.CompletedDispatches = append(sealed.CompletedDispatches, *sealed.ActiveDispatch)
	sealed.ActiveDispatch = nil
	repository.aggregate = sealed
	aggregate = sealed
	observation := runtimecontract.RuntimeValidationFailureObservation{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, DispatchID: aggregate.CompletedDispatches[0].ID, OperationID: "validate-1", FailureSignature: "build-failure", TargetRole: "implementation", DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed, CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: []string{"main.go"}, DiagnosticHash: strings.Repeat("b", 64)}
	aggregate.Facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassBuildFailure, Code: "build_failed", Signature: observation.FailureSignature, Message: "implementation does not compile", ValidationFailureObservation: &observation}
	repository.aggregate = aggregate
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeRepairSurfaceConfirmation{Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Disposition: runtimecontract.RuntimeRepairSurfaceRejected, ObservedRevision: "tree-v1", Reason: "not a regular file"})
	if err != nil {
		t.Fatal(err)
	}
	if failed.Decision.State != runtimecontract.ConvergenceStateFailed || failed.Decision.Terminal == nil || failed.Decision.Terminal.Code != "repair_surface_unavailable" {
		t.Fatalf("unconfirmed repair surface entered a retry loop: %+v", failed.Decision)
	}
	if failed.Facts.Failure == nil || failed.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible || failed.Facts.Failure.Class == runtimecontract.FailureClassNoProgressExhausted || failed.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("rejected repair surface consumed or misclassified repair budget: failure=%+v repair=%+v", failed.Facts.Failure, failed.Facts.Repair)
	}
	beforeRevision := failed.StateRevision
	replayed, err := controller.recordRepairSurfaceConfirmation(context.Background(), failed.Facts.WorkspaceID, failed.Facts.RunID, runtimecontract.RuntimeRepairSurfaceConfirmation{Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Disposition: runtimecontract.RuntimeRepairSurfaceRejected, ObservedRevision: "tree-v1", Reason: "not a regular file"})
	if err != nil || replayed.StateRevision != beforeRevision || len(replayed.RepairSurfaceConfirmations) != 1 {
		t.Fatalf("exact rejected confirmation replay changed state: err=%v revision=%d want=%d records=%+v", err, replayed.StateRevision, beforeRevision, replayed.RepairSurfaceConfirmations)
	}
	_, err = controller.recordRepairSurfaceConfirmation(context.Background(), failed.Facts.WorkspaceID, failed.Facts.RunID, runtimecontract.RuntimeRepairSurfaceConfirmation{Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Disposition: runtimecontract.RuntimeRepairSurfaceConfirmed, ConfirmedTargetRefs: []string{"main.go"}, ObservedRevision: "tree-v1", Reason: "not a regular file"})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("conflicting rejected confirmation was accepted: %v", err)
	}
}

func TestStaleRepairSurfaceReacquiresOwnerPrerequisiteWithoutTerminal(t *testing.T) {
	aggregate := validationAggregate(t)
	aggregate.Facts.Focus.TargetRole = "implementation"
	aggregate.Facts.Focus.RequireTargetGrounding = true
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	sealed.ActiveDispatch.State = DispatchStateCommitted
	sealed.CompletedDispatches = append(sealed.CompletedDispatches, *sealed.ActiveDispatch)
	sealed.ActiveDispatch = nil
	aggregate = sealed
	observation := runtimecontract.RuntimeValidationFailureObservation{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, DispatchID: aggregate.CompletedDispatches[0].ID,
		OperationID: "validate-1", FailureSignature: "build-failure", TargetRole: "implementation",
		DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed, CandidateTargetRefs: []string{"main.go"},
		ValidationScopeTargetRefs: []string{"main.go"}, DiagnosticHash: strings.Repeat("c", 64),
	}
	aggregate.Facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassBuildFailure, Code: "build_failed", Signature: observation.FailureSignature,
		Message: "implementation does not compile", TargetRefs: []string{"main.go"}, ValidationFailureObservation: &observation,
	}
	repository.aggregate = aggregate
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID,
		runtimecontract.RuntimeRepairSurfaceConfirmation{
			Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
			Disposition: runtimecontract.RuntimeRepairSurfaceStale, ObservedRevision: "tree-v2", Reason: "workspace revision changed",
		})
	if err != nil {
		t.Fatal(err)
	}
	if stale.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("stale repair surface did not reacquire prerequisite: decision=%+v pending=%+v", stale.Decision, stale.Facts.Prerequisites.Pending)
	}
	if stale.Facts.Resources.FailureTarget != nil || stale.Facts.Resources.TargetSelection != nil || len(stale.Facts.Resources.TargetSnapshots) != 0 {
		t.Fatalf("stale repair surface retained target authority: resources=%+v", stale.Facts.Resources)
	}
	if stale.Facts.Failure == nil || stale.Facts.Failure.Class == runtimecontract.FailureClassNoProgressExhausted || stale.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("stale repair surface changed failure class/budget: failure=%+v repair=%+v", stale.Facts.Failure, stale.Facts.Repair)
	}
	foundSnapshot := false
	for _, prerequisite := range stale.Facts.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && prerequisite.SubjectRef == "main.go" {
			foundSnapshot = true
			break
		}
	}
	if !foundSnapshot {
		t.Fatalf("stale repair surface did not schedule target refresh: %+v", stale.Facts.Prerequisites.Pending)
	}
	beforeRevision := stale.StateRevision
	replayed, err := controller.recordRepairSurfaceConfirmation(context.Background(), stale.Facts.WorkspaceID, stale.Facts.RunID, runtimecontract.RuntimeRepairSurfaceConfirmation{Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Disposition: runtimecontract.RuntimeRepairSurfaceStale, ObservedRevision: "tree-v2", Reason: "workspace revision changed"})
	if err != nil || replayed.StateRevision != beforeRevision || len(replayed.RepairSurfaceConfirmations) != 1 {
		t.Fatalf("exact stale confirmation replay changed state: err=%v revision=%d want=%d records=%+v", err, replayed.StateRevision, beforeRevision, replayed.RepairSurfaceConfirmations)
	}
	_, err = controller.recordRepairSurfaceConfirmation(context.Background(), stale.Facts.WorkspaceID, stale.Facts.RunID, runtimecontract.RuntimeRepairSurfaceConfirmation{Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Disposition: runtimecontract.RuntimeRepairSurfaceRejected, ObservedRevision: "tree-v2", Reason: "workspace revision changed"})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("conflicting stale confirmation was accepted: %v", err)
	}
}
