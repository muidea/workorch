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
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("a", 64))
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	oldAuthority := observation.AuthoritySnapshotID
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
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, false, strings.Repeat("b", 64))
	beforeRepairAttempts := aggregate.Facts.Repair.RepairAttempts
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
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
	if failed.Facts.Failure == nil || failed.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible || failed.Facts.Failure.Class == runtimecontract.FailureClassNoProgressExhausted || failed.Facts.Repair.RepairAttempts != beforeRepairAttempts {
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
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("c", 64))
	beforeRepairAttempts := aggregate.Facts.Repair.RepairAttempts
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
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
	if stale.Facts.Failure == nil || stale.Facts.Failure.Class == runtimecontract.FailureClassNoProgressExhausted || stale.Facts.Repair.RepairAttempts != beforeRepairAttempts {
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

func TestRepairSurfaceConfirmationRequiresSealedOwnerAtCommitAndFreshLoad(t *testing.T) {
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("d", 64))
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	invalid := runtimecontract.RuntimeRepairSurfaceConfirmation{
		Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		SubjectRef: "foreign-repair-surface", Disposition: runtimecontract.RuntimeRepairSurfaceConfirmed,
		ConfirmedTargetRefs: append([]string(nil), observation.CandidateTargetRefs...), ObservedRevision: "mcp-v1",
	}.Normalized()
	beforeRevision := repository.aggregate.StateRevision
	if _, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, invalid); !errors.Is(err, common.ErrConflict) ||
		!strings.Contains(err.Error(), "owner does not match sealed repair provenance") {
		t.Fatalf("wrong-owner confirmation commit error = %v", err)
	}
	if repository.aggregate.StateRevision != beforeRevision || len(repository.aggregate.RepairSurfaceConfirmations) != 0 || repository.aggregate.Facts.Resources.FailureTarget != nil {
		t.Fatalf("non-owner confirmation changed Runtime authority: %+v", repository.aggregate)
	}

	tampered := aggregate
	tampered.RepairSurfaceConfirmations = append(tampered.RepairSurfaceConfirmations, runtimecontract.RuntimeRepairSurfaceConfirmationRecord{
		Confirmation:      invalid,
		Signature:         runtimecontract.StableRuntimeRepairSurfaceConfirmationHash(invalid),
		IdentitySignature: runtimecontract.StableRuntimeRepairSurfaceConfirmationIdentityHash(invalid),
	})
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "owner does not match sealed repair provenance") {
		t.Fatalf("fresh-load wrong-owner confirmation error = %v", err)
	}
}

func TestRepairSurfaceConfirmationTerminalResponseFencesLaterRevisions(t *testing.T) {
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("7", 64))
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	confirmed := runtimecontract.RuntimeRepairSurfaceConfirmation{
		Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
		Disposition: runtimecontract.RuntimeRepairSurfaceConfirmed, ConfirmedTargetRefs: append([]string(nil), observation.CandidateTargetRefs...), ObservedRevision: "tree-v1",
	}
	resolved, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, confirmed)
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision := resolved.StateRevision
	lateRejected := confirmed
	lateRejected.Disposition = runtimecontract.RuntimeRepairSurfaceRejected
	lateRejected.ConfirmedTargetRefs = nil
	lateRejected.ObservedRevision = "tree-v2"
	lateRejected.Reason = "late owner response"
	if _, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, lateRejected); !errors.Is(err, common.ErrConflict) ||
		!strings.Contains(err.Error(), "already has terminal target authority") {
		t.Fatalf("late post-confirmation response error = %v", err)
	}
	if repository.aggregate.StateRevision != beforeRevision || len(repository.aggregate.RepairSurfaceConfirmations) != 1 || repository.aggregate.Facts.Resources.FailureTarget == nil {
		t.Fatalf("late response changed confirmed authority: %+v", repository.aggregate)
	}

	tampered := resolved
	lateRecord, err := runtimecontract.NewRuntimeRepairSurfaceConfirmationRecord(lateRejected)
	if err != nil {
		t.Fatal(err)
	}
	tampered.RepairSurfaceConfirmations = append(tampered.RepairSurfaceConfirmations, lateRecord)
	if err := tampered.Validate(); err == nil || (!strings.Contains(err.Error(), "follows terminal response") &&
		!strings.Contains(err.Error(), "exact confirmed owner repair surface") &&
		!strings.Contains(err.Error(), "no exact owner-confirmed failure authority")) {
		t.Fatalf("fresh-load post-terminal confirmation error = %v", err)
	}
}

func TestRepairSurfaceConfirmationStaleRevisionMayRefreshBeforeTerminalResponse(t *testing.T) {
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("6", 64))
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimeRepairSurfaceConfirmation{
		Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
		Disposition: runtimecontract.RuntimeRepairSurfaceStale, ObservedRevision: "tree-v1", Reason: "workspace changed during observation",
	}
	if _, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, stale); err != nil {
		t.Fatal(err)
	}
	confirmed := stale
	confirmed.Disposition = runtimecontract.RuntimeRepairSurfaceConfirmed
	confirmed.ConfirmedTargetRefs = append([]string(nil), observation.CandidateTargetRefs...)
	confirmed.ObservedRevision = "tree-v2"
	confirmed.Reason = ""
	refreshed, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, confirmed)
	if err != nil {
		t.Fatal(err)
	}
	if len(refreshed.RepairSurfaceConfirmations) != 2 || refreshed.Facts.Resources.FailureTarget == nil ||
		!sameRuntimeTargetRefs(refreshed.Facts.Resources.FailureTarget.TargetRefs, observation.CandidateTargetRefs) {
		t.Fatalf("stale-to-confirmed sequence did not establish current authority: %+v", refreshed)
	}
}

func TestRuntimeAggregateRejectsValidationFailureObservationWithoutExactShellReceipt(t *testing.T) {
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("e", 64))
	forged := observation.Clone()
	forged.DiagnosticHash = strings.Repeat("f", 64)
	aggregate.Facts.Failure.ValidationFailureObservation = &forged
	if aggregate.Facts.Failure.OriginalDiagnostic == nil {
		t.Fatal("production-shaped failure lost original diagnostic")
	}
	aggregate.Facts.Failure.OriginalDiagnostic.ValidationFailureObservation = &forged
	if err := aggregate.Validate(); err == nil || !strings.Contains(err.Error(), "has no exact persisted validation-owner receipt") {
		t.Fatalf("fresh-load receipt provenance error = %v", err)
	}
}

func TestRuntimeAggregateFreshLoadRejectsCoordinatedValidationRouteDiagnosticTamper(t *testing.T) {
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("6", 64))
	tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, aggregate)
	var changedObservation runtimecontract.RuntimeValidationFailureObservation
	found := false
	for index := range tampered.OperationReceipts {
		receipt := &tampered.OperationReceipts[index]
		if receipt.Feedback == nil || receipt.Feedback.OriginalDiagnostic == nil ||
			receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic == nil ||
			receipt.Feedback.OriginalDiagnostic.ValidationFailureObservation == nil ||
			!runtimecontract.SameRuntimeValidationFailureObservation(*receipt.Feedback.OriginalDiagnostic.ValidationFailureObservation, observation) {
			continue
		}
		receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic.OwnerCode = "tampered_owner_code"
		changedObservation = receipt.Feedback.OriginalDiagnostic.ValidationFailureObservation.Clone()
		changedObservation.DiagnosticHash = runtimecontract.StableRuntimeValidationFailureObservationDiagnosticHash(
			*receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic,
			changedObservation.ValidationScopeTargetRefs,
		)
		receipt.Feedback.OriginalDiagnostic.ValidationFailureObservation = &changedObservation
		found = true
		break
	}
	if !found || tampered.Facts.Failure == nil || tampered.Facts.Failure.ValidationDiagnostic == nil || tampered.Facts.Failure.OriginalDiagnostic == nil || tampered.Facts.Failure.OriginalDiagnostic.ValidationDiagnostic == nil {
		t.Fatal("validation repair fixture omitted exact diagnostic receipt lineage")
	}
	// Coordinate every semantic copy so only the persisted receipt hash/fence
	// can expose the mutation after a JSON-shaped fresh load.
	tampered.Facts.Failure.ValidationDiagnostic.OwnerCode = "tampered_owner_code"
	tampered.Facts.Failure.OriginalDiagnostic.ValidationDiagnostic.OwnerCode = "tampered_owner_code"
	tampered.Facts.Failure.ValidationFailureObservation = &changedObservation
	tampered.Facts.Failure.OriginalDiagnostic.ValidationFailureObservation = &changedObservation
	fresh := cloneRuntimeAggregateForMCPProvenanceTest(t, tampered)
	if err := fresh.Validate(); err == nil || !strings.Contains(err.Error(), "has no exact committed execution projection") {
		t.Fatalf("fresh load accepted coordinated repair-route diagnostic tamper: %v", err)
	}
}

func TestRuntimeAggregateRequiresCanonicalValidationFailureReceiptCopies(t *testing.T) {
	aggregate, _ := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("8", 64))
	tests := []struct {
		name      string
		mutate    func(*runtimecontract.FailureFact)
		wantError string
	}{
		{
			name: "missing reducer observation",
			mutate: func(failure *runtimecontract.FailureFact) {
				failure.ValidationFailureObservation = nil
			},
			wantError: "validation failure observation copies must both be present",
		},
		{
			name: "missing owner observation",
			mutate: func(failure *runtimecontract.FailureFact) {
				failure.OriginalDiagnostic.ValidationFailureObservation = nil
			},
			wantError: "validation failure observation copies must both be present",
		},
		{
			name: "missing reducer diagnostic",
			mutate: func(failure *runtimecontract.FailureFact) {
				failure.ValidationDiagnostic = nil
			},
			wantError: "validation diagnostic copies must both be present",
		},
		{
			name: "drifted reducer diagnostic",
			mutate: func(failure *runtimecontract.FailureFact) {
				failure.ValidationDiagnostic.Class = runtimecontract.ValidationDiagnosticTimeout
			},
			wantError: "validation diagnostic disagrees with original diagnostic",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tampered := aggregate
			failure := aggregate.Facts.Failure.Clone()
			test.mutate(&failure)
			tampered.Facts.Failure = &failure
			if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("fresh-load canonical-copy error = %v", err)
			}
		})
	}

	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision := repository.aggregate.StateRevision
	missingOwnerCopy := aggregate.Facts.Failure.Clone()
	missingOwnerCopy.OriginalDiagnostic.ValidationFailureObservation = nil
	if _, err := controller.recordFailureFact(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, missingOwnerCopy); !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "generic failure ingress cannot carry capability-owner authority") {
		t.Fatalf("generic-ingress canonical-copy error = %v", err)
	}
	if repository.aggregate.StateRevision != beforeRevision {
		t.Fatalf("invalid canonical-copy commit changed revision: got=%d want=%d", repository.aggregate.StateRevision, beforeRevision)
	}
}

func TestValidationFailureObservationRequiresEvidenceCommittedReceiptTransaction(t *testing.T) {
	aggregate := validationRepairSurfaceAggregateForTest(t, true)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	operationID := runtimeTestCandidateOperationID(t, sealed.ActiveDispatch.Candidate, 0)
	observation := runtimecontract.RuntimeValidationFailureObservation{
		WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID:          sealed.ActiveDispatch.ID, OperationID: operationID,
		FailureSignature: "owner-committed-only", TargetRole: "implementation",
		DiagnosticClass:     runtimecontract.ValidationDiagnosticBuildFailed,
		CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: []string{"main.go"}, DiagnosticHash: strings.Repeat("9", 64),
	}
	evidence := runtimeRejectedValidationEvidenceForTest(t, sealed, "event-validation-owner-committed-only", observation.DiagnosticClass)
	evidence.Failure.Signature = observation.FailureSignature
	evidence.Failure.ValidationDiagnostic.TargetRefs = append([]string(nil), observation.CandidateTargetRefs...)
	evidence.Failure.ValidationFailureObservation = &observation
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	if runtimeValidationFailureObservationHasPersistedReceipt(repository.aggregate, observation) {
		t.Fatal("owner-committed receipt was treated as an evidence-committed validation fact")
	}

	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision := repository.aggregate.StateRevision
	beforeCompletedCount := len(repository.aggregate.CompletedDispatches)
	ownerFailure := failureFactFromOperationReceiptFeedback(*evidence.Receipt)
	if ownerFailure == nil {
		t.Fatal("owner receipt did not reconstruct its typed failure")
	}
	if _, err := controller.recordDriveFailureFact(context.Background(), sealed.Facts.WorkspaceID, sealed.Facts.RunID, *ownerFailure); err == nil ||
		!strings.Contains(err.Error(), "generic failure ingress cannot carry capability-owner authority") {
		t.Fatalf("generic drive failure owner-authority error = %v", err)
	}
	if repository.aggregate.StateRevision != beforeRevision || repository.aggregate.ActiveDispatch == nil ||
		len(repository.aggregate.CompletedDispatches) != beforeCompletedCount {
		t.Fatalf("generic failure ingress invalidated owner-committed dispatch: %+v", repository.aggregate)
	}
}

func TestValidationFailureRepairAuthorityRequiresExactOwnerConfirmationAtCommitAndFreshLoad(t *testing.T) {
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("1", 64))
	forgedFailure := aggregate.Facts.Failure.Clone()
	forgedFailure.TargetRefs = append([]string(nil), observation.CandidateTargetRefs...)
	forgedFailure.TargetObservation = &runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
		TargetRole: observation.TargetRole, ObservedRevision: "tree-v1", TargetRefs: append([]string(nil), observation.CandidateTargetRefs...),
	}
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision := repository.aggregate.StateRevision
	if _, err := controller.recordFailureFact(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, forgedFailure); !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "generic failure ingress cannot carry capability-owner authority") {
		t.Fatalf("unconfirmed repair authority commit error = %v", err)
	}
	if repository.aggregate.StateRevision != beforeRevision || repository.aggregate.Facts.Resources.FailureTarget != nil {
		t.Fatalf("unconfirmed repair authority changed persisted state: %+v", repository.aggregate)
	}

	tampered := aggregate
	tampered.Facts.Failure = &forgedFailure
	tampered.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{
		Signature: observation.FailureSignature, TargetRefs: append([]string(nil), observation.CandidateTargetRefs...),
		TargetObservation: forgedFailure.TargetObservation,
	}
	if err := tampered.Validate(); err == nil || (!strings.Contains(err.Error(), "requires an exact confirmed owner repair surface") &&
		!strings.Contains(err.Error(), "no exact owner-confirmed failure authority") &&
		!strings.Contains(err.Error(), "must come from its exact owner confirmation")) {
		t.Fatalf("fresh-load unconfirmed repair authority error = %v", err)
	}
}

func validationRepairSurfaceAggregateForTest(t *testing.T, requireTargetGrounding bool) RuntimeAggregate {
	t.Helper()
	return validationAggregateWithPreparation(t, func(aggregate *RuntimeAggregate) {
		aggregate.Facts.Focus.TargetRole = "implementation"
		aggregate.Facts.Focus.RequireTargetGrounding = requireTargetGrounding
	})
}

func validationRepairSurfaceObservedAggregateForTest(t *testing.T, requireTargetGrounding bool, diagnosticHash string) (RuntimeAggregate, runtimecontract.RuntimeValidationFailureObservation) {
	t.Helper()
	repository := &durableRuntimeStateRepository{aggregate: validationRepairSurfaceAggregateForTest(t, requireTargetGrounding)}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	operationID := runtimeTestCandidateOperationID(t, sealed.ActiveDispatch.Candidate, 0)
	observation := runtimecontract.RuntimeValidationFailureObservation{
		WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID:          sealed.ActiveDispatch.ID, OperationID: operationID,
		FailureSignature: "build-failure", TargetRole: sealed.ActiveDispatch.FocusTargetRole,
		DiagnosticClass:     runtimecontract.ValidationDiagnosticBuildFailed,
		CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: []string{"main.go"}, DiagnosticHash: diagnosticHash,
	}
	evidence := runtimeRejectedValidationEvidenceForTest(t, sealed, "event-validation-repair-surface/"+diagnosticHash[:8], observation.DiagnosticClass)
	evidence.Failure.Signature = observation.FailureSignature
	evidence.Failure.ValidationDiagnostic.TargetRefs = append([]string(nil), observation.CandidateTargetRefs...)
	observation.DiagnosticHash = runtimecontract.StableRuntimeValidationFailureObservationDiagnosticHash(*evidence.Failure.ValidationDiagnostic, observation.ValidationScopeTargetRefs)
	evidence.Failure.ValidationFailureObservation = &observation
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatalf("commit validation failure observation: %v", err)
	}
	// The owner confirmation is intentionally a later owner observation. Model
	// the workspace revision edge by retiring the pre-validation target snapshot
	// so a confirmed/stale response must reacquire current File authority instead
	// of reusing the semantic-seal fixture snapshot.
	committed.Aggregate.Facts.Resources.TargetSnapshots = nil
	return committed.Aggregate, observation
}
