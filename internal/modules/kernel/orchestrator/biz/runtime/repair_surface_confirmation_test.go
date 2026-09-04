package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func bindRuntimeRepairSurfaceConfirmationRequestForTest(t testing.TB, aggregate RuntimeAggregate, confirmation runtimecontract.RuntimeRepairSurfaceConfirmation) runtimecontract.RuntimeRepairSurfaceConfirmation {
	t.Helper()
	plan, err := CurrentRuntimeRepairSurfaceConfirmationPlan(aggregate)
	if err != nil || !plan.Required() {
		t.Fatalf("repair-surface confirmation test plan = %+v err=%v", plan, err)
	}
	confirmation.RequestID = plan.RequestID
	confirmation.IssuedAtStateRevision = plan.IssuedAtStateRevision
	confirmation.ExpectedSubjectRevision = plan.ExpectedSubjectRevision
	switch confirmation.Disposition {
	case runtimecontract.RuntimeRepairSurfaceConfirmed, runtimecontract.RuntimeRepairSurfaceRejected:
		confirmation.ObservedRevision = plan.ExpectedSubjectRevision
	case runtimecontract.RuntimeRepairSurfaceStale:
		if strings.TrimSpace(confirmation.ObservedRevision) == "" || confirmation.ObservedRevision == plan.ExpectedSubjectRevision {
			confirmation.ObservedRevision = plan.ExpectedSubjectRevision + "-stale"
		}
	}
	return confirmation
}

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
	confirmation := bindRuntimeRepairSurfaceConfirmationRequestForTest(t, aggregate, runtimecontract.RuntimeRepairSurfaceConfirmation{Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Disposition: runtimecontract.RuntimeRepairSurfaceConfirmed, ConfirmedTargetRefs: []string{"main.go"}, ObservedRevision: "tree-v1"})
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

func TestRuntimeAggregateRejectsCoordinatedRepairSurfaceRequestFenceTamper(t *testing.T) {
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("5", 64))
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	confirmation := bindRuntimeRepairSurfaceConfirmationRequestForTest(t, aggregate, runtimecontract.RuntimeRepairSurfaceConfirmation{
		Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef:  runtimecontract.RuntimeRepairSurfaceFileWorkspaceSubjectRef,
		Disposition: runtimecontract.RuntimeRepairSurfaceConfirmed, ConfirmedTargetRefs: append([]string(nil), observation.CandidateTargetRefs...),
		ObservedRevision: "tree-fence-v1",
	})
	confirmed, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, confirmation)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("self-consistent request hash cannot change the issued lineage", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, confirmed)
		record := &tampered.RepairSurfaceConfirmations[0]
		record.Confirmation.RequestID = stableRuntimeRepairSurfaceConfirmationRequestID(
			record.Confirmation.Observation,
			record.Confirmation.OwnerModuleID,
			runtimecontract.RuntimeRepairSurfaceConfirmationProtocolFileWorkspaceRegularTargets,
			record.Confirmation.SubjectRef,
			record.Confirmation.ExpectedSubjectRevision,
			record.Confirmation.IssuedAtStateRevision,
			"forged-refresh-lineage",
		)
		record.Signature = runtimecontract.StableRuntimeRepairSurfaceConfirmationHash(record.Confirmation)
		record.IdentitySignature = runtimecontract.StableRuntimeRepairSurfaceConfirmationIdentityHash(record.Confirmation)
		if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "request fence does not match its persisted window") {
			t.Fatalf("fresh load accepted coordinated request-id/hash tamper: %v", err)
		}
	})

	t.Run("issued and recorded revisions cannot move beyond the owner CAS", func(t *testing.T) {
		tampered := cloneRuntimeAggregateForMCPProvenanceTest(t, confirmed)
		record := &tampered.RepairSurfaceConfirmations[0]
		record.Confirmation.IssuedAtStateRevision++
		record.RecordedAtStateRevision = record.Confirmation.IssuedAtStateRevision + 1
		record.Confirmation.RequestID = stableRuntimeRepairSurfaceConfirmationRequestID(
			record.Confirmation.Observation,
			record.Confirmation.OwnerModuleID,
			runtimecontract.RuntimeRepairSurfaceConfirmationProtocolFileWorkspaceRegularTargets,
			record.Confirmation.SubjectRef,
			record.Confirmation.ExpectedSubjectRevision,
			record.Confirmation.IssuedAtStateRevision,
			"initial",
		)
		record.Signature = runtimecontract.StableRuntimeRepairSurfaceConfirmationHash(record.Confirmation)
		record.IdentitySignature = runtimecontract.StableRuntimeRepairSurfaceConfirmationIdentityHash(record.Confirmation)
		if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "recorded after the current state revision") {
			t.Fatalf("fresh load accepted coordinated request revision/hash tamper: %v", err)
		}
	})
}

func TestRuntimeRepairSurfaceConfirmationChronologyIsGlobalAcrossObservations(t *testing.T) {
	first := runtimeRepairSurfaceConfirmationChronologyRecordForTest(t, "first", 10)
	second := runtimeRepairSurfaceConfirmationChronologyRecordForTest(t, "second", 12)
	if err := validateRuntimeRepairSurfaceConfirmationChronology(13, []runtimecontract.RuntimeRepairSurfaceConfirmationRecord{first, second}); err != nil {
		t.Fatalf("valid global confirmation chronology: %v", err)
	}

	for name, records := range map[string][]runtimecontract.RuntimeRepairSurfaceConfirmationRecord{
		"duplicate recorded revision": {
			first,
			runtimeRepairSurfaceConfirmationChronologyRecordForTest(t, "second-same-revision", 10),
		},
		"reverse append order": {
			runtimeRepairSurfaceConfirmationChronologyRecordForTest(t, "first-later", 20),
			runtimeRepairSurfaceConfirmationChronologyRecordForTest(t, "second-earlier", 10),
		},
		"overlapping request window": {
			first,
			runtimeRepairSurfaceConfirmationChronologyRecordForTest(t, "second-overlap", first.RecordedAtStateRevision),
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateRuntimeRepairSurfaceConfirmationChronology(64, records); err == nil ||
				!strings.Contains(err.Error(), "does not follow the global confirmation history") {
				t.Fatalf("invalid cross-observation chronology was accepted: %v", err)
			}
		})
	}

	if err := validateRuntimeRepairSurfaceConfirmationChronology(second.RecordedAtStateRevision-1, []runtimecontract.RuntimeRepairSurfaceConfirmationRecord{first, second}); err == nil ||
		!strings.Contains(err.Error(), "recorded after the current state revision") {
		t.Fatalf("future confirmation record was accepted: %v", err)
	}
}

func runtimeRepairSurfaceConfirmationChronologyRecordForTest(t testing.TB, sequence string, issuedAt uint64) runtimecontract.RuntimeRepairSurfaceConfirmationRecord {
	t.Helper()
	observation := runtimecontract.RuntimeValidationFailureObservation{
		WorkspaceID: "ws-confirmation-chronology", RunID: "run-confirmation-chronology",
		AuthoritySnapshotID: "authority-" + sequence, DispatchID: "dispatch-" + sequence,
		OperationID: "operation-" + sequence, FailureSignature: "failure-" + sequence,
		TargetRole: "implementation", DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed,
		CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: []string{"main.go"},
		DiagnosticHash: strings.Repeat("a", 64),
	}
	record, err := runtimecontract.NewRuntimeRepairSurfaceConfirmationRecord(runtimecontract.RuntimeRepairSurfaceConfirmation{
		RequestID: "request-" + sequence, IssuedAtStateRevision: issuedAt, ExpectedSubjectRevision: "tree-expected-" + sequence,
		Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef:       runtimecontract.RuntimeRepairSurfaceFileWorkspaceSubjectRef,
		Disposition:      runtimecontract.RuntimeRepairSurfaceStale,
		ObservedRevision: "tree-observed-" + sequence,
	})
	if err != nil {
		t.Fatalf("build confirmation chronology record: %v", err)
	}
	return record
}

func TestRejectedRepairSurfaceFailsBeforeAnotherValidationCandidate(t *testing.T) {
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, false, strings.Repeat("b", 64))
	beforeRepairAttempts := aggregate.Facts.Repair.RepairAttempts
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	confirmation := bindRuntimeRepairSurfaceConfirmationRequestForTest(t, aggregate, runtimecontract.RuntimeRepairSurfaceConfirmation{Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Disposition: runtimecontract.RuntimeRepairSurfaceRejected, ObservedRevision: "tree-v1", Reason: "not a regular file"})
	failed, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, confirmation)
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
	replayed, err := controller.recordRepairSurfaceConfirmation(context.Background(), failed.Facts.WorkspaceID, failed.Facts.RunID, confirmation)
	if err != nil || replayed.StateRevision != beforeRevision || len(replayed.RepairSurfaceConfirmations) != 1 {
		t.Fatalf("exact rejected confirmation replay changed state: err=%v revision=%d want=%d records=%+v", err, replayed.StateRevision, beforeRevision, replayed.RepairSurfaceConfirmations)
	}
	conflicting := confirmation
	conflicting.Disposition = runtimecontract.RuntimeRepairSurfaceConfirmed
	conflicting.ConfirmedTargetRefs = []string{"main.go"}
	_, err = controller.recordRepairSurfaceConfirmation(context.Background(), failed.Facts.WorkspaceID, failed.Facts.RunID, conflicting)
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
	confirmation := bindRuntimeRepairSurfaceConfirmationRequestForTest(t, aggregate, runtimecontract.RuntimeRepairSurfaceConfirmation{
		Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
		Disposition: runtimecontract.RuntimeRepairSurfaceStale, ObservedRevision: "tree-v2", Reason: "workspace revision changed",
	})
	stale, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, confirmation)
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
	replayed, err := controller.recordRepairSurfaceConfirmation(context.Background(), stale.Facts.WorkspaceID, stale.Facts.RunID, confirmation)
	if err != nil || replayed.StateRevision != beforeRevision || len(replayed.RepairSurfaceConfirmations) != 1 {
		t.Fatalf("exact stale confirmation replay changed state: err=%v revision=%d want=%d records=%+v", err, replayed.StateRevision, beforeRevision, replayed.RepairSurfaceConfirmations)
	}
	conflicting := confirmation
	conflicting.Disposition = runtimecontract.RuntimeRepairSurfaceRejected
	conflicting.ObservedRevision = conflicting.ExpectedSubjectRevision
	_, err = controller.recordRepairSurfaceConfirmation(context.Background(), stale.Facts.WorkspaceID, stale.Facts.RunID, conflicting)
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
	invalid = bindRuntimeRepairSurfaceConfirmationRequestForTest(t, aggregate, invalid)
	beforeRevision := repository.aggregate.StateRevision
	if _, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, invalid); !errors.Is(err, common.ErrConflict) ||
		!strings.Contains(err.Error(), "owner does not match sealed repair provenance") {
		t.Fatalf("wrong-owner confirmation commit error = %v", err)
	}
	if repository.aggregate.StateRevision != beforeRevision || len(repository.aggregate.RepairSurfaceConfirmations) != 0 || repository.aggregate.Facts.Resources.FailureTarget != nil {
		t.Fatalf("non-owner confirmation changed Runtime authority: %+v", repository.aggregate)
	}
	wrongSubject := invalid
	wrongSubject.OwnerModuleID = runtimecontract.RuntimeModuleFile
	wrongSubject.SubjectRef = "workspace_other"
	wrongSubject = bindRuntimeRepairSurfaceConfirmationRequestForTest(t, aggregate, wrongSubject)
	if _, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, wrongSubject); !errors.Is(err, common.ErrConflict) ||
		!strings.Contains(err.Error(), "subject does not match current protocol") {
		t.Fatalf("wrong-subject confirmation commit error = %v", err)
	}
	if repository.aggregate.StateRevision != beforeRevision || len(repository.aggregate.RepairSurfaceConfirmations) != 0 || repository.aggregate.Facts.Resources.FailureTarget != nil {
		t.Fatalf("wrong-subject confirmation changed Runtime authority: %+v", repository.aggregate)
	}

	invalidRecord, err := runtimecontract.NewRuntimeRepairSurfaceConfirmationRecord(invalid)
	if err != nil {
		t.Fatal(err)
	}
	tampered := aggregate
	tampered.RepairSurfaceConfirmations = append(tampered.RepairSurfaceConfirmations, invalidRecord)
	tampered.StateRevision = invalidRecord.RecordedAtStateRevision
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "owner does not match sealed repair provenance") {
		t.Fatalf("fresh-load wrong-owner confirmation error = %v", err)
	}
	wrongSubjectRecord, err := runtimecontract.NewRuntimeRepairSurfaceConfirmationRecord(wrongSubject)
	if err != nil {
		t.Fatal(err)
	}
	tamperedSubject := aggregate
	tamperedSubject.RepairSurfaceConfirmations = append(tamperedSubject.RepairSurfaceConfirmations, wrongSubjectRecord)
	tamperedSubject.StateRevision = wrongSubjectRecord.RecordedAtStateRevision
	if err := tamperedSubject.Validate(); err == nil || !strings.Contains(err.Error(), "subject does not match sealed owner protocol") {
		t.Fatalf("fresh-load wrong-subject confirmation error = %v", err)
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
	confirmed = bindRuntimeRepairSurfaceConfirmationRequestForTest(t, aggregate, confirmed)
	resolved, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, confirmed)
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision := resolved.StateRevision
	lateRejected := confirmed
	lateRejected.Disposition = runtimecontract.RuntimeRepairSurfaceRejected
	lateRejected.ConfirmedTargetRefs = nil
	lateRejected.ObservedRevision = lateRejected.ExpectedSubjectRevision
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
		!strings.Contains(err.Error(), "conflict for issued request") &&
		!strings.Contains(err.Error(), "exact confirmed owner repair surface") &&
		!strings.Contains(err.Error(), "no exact owner-confirmed failure authority")) {
		t.Fatalf("fresh-load post-terminal confirmation error = %v", err)
	}
}

func TestRepairSurfaceConfirmationFencesInFlightResponseAfterAdministrativeTermination(t *testing.T) {
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("8", 64))
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	terminated, err := controller.requestTermination(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID,
		runtimecontract.RuntimeTerminationCancelled, "blocks/run", "operator cancelled the run")
	if err != nil {
		t.Fatal(err)
	}
	confirmation := runtimecontract.RuntimeRepairSurfaceConfirmation{
		Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
		Disposition: runtimecontract.RuntimeRepairSurfaceConfirmed, ConfirmedTargetRefs: append([]string(nil), observation.CandidateTargetRefs...),
		ObservedRevision: "tree-after-cancel-v1",
	}
	confirmation = bindRuntimeRepairSurfaceConfirmationRequestForTest(t, aggregate, confirmation)
	if _, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, confirmation); !errors.Is(err, common.ErrConflict) ||
		!strings.Contains(err.Error(), "administrative termination") {
		t.Fatalf("in-flight confirmation after termination error = %v", err)
	}
	if repository.aggregate.StateRevision != terminated.StateRevision || len(repository.aggregate.RepairSurfaceConfirmations) != 0 {
		t.Fatalf("in-flight confirmation changed terminated authority: %+v", repository.aggregate)
	}
}

func TestRepairSurfaceConfirmationExactReplayRemainsNoOpAfterAdministrativeTermination(t *testing.T) {
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("9", 64))
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	confirmation := runtimecontract.RuntimeRepairSurfaceConfirmation{
		Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
		Disposition: runtimecontract.RuntimeRepairSurfaceConfirmed, ConfirmedTargetRefs: append([]string(nil), observation.CandidateTargetRefs...),
		ObservedRevision: "tree-before-cancel-v1",
	}
	confirmation = bindRuntimeRepairSurfaceConfirmationRequestForTest(t, aggregate, confirmation)
	confirmed, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, confirmation)
	if err != nil {
		t.Fatal(err)
	}
	terminated, err := controller.requestTermination(context.Background(), confirmed.Facts.WorkspaceID, confirmed.Facts.RunID,
		runtimecontract.RuntimeTerminationCancelled, "blocks/run", "operator cancelled the run")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, confirmation)
	if err != nil {
		t.Fatalf("exact persisted confirmation replay after termination: %v", err)
	}
	if replayed.StateRevision != terminated.StateRevision || replayed.Termination == nil || len(replayed.RepairSurfaceConfirmations) != 1 {
		t.Fatalf("exact replay changed terminated authority: %+v", replayed)
	}
}

func TestRepairSurfaceConfirmationStaleRevisionRequiresPrerequisiteReplanBeforeFreshResponse(t *testing.T) {
	aggregate, observation := validationRepairSurfaceObservedAggregateForTest(t, true, strings.Repeat("6", 64), func(aggregate *RuntimeAggregate) {
		aggregate.Facts.ConvergencePolicy.MaxValidationOnlyRetriesPerFocus = 2
	})
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimeRepairSurfaceConfirmation{
		Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
		Disposition: runtimecontract.RuntimeRepairSurfaceStale, ObservedRevision: "tree-v1", Reason: "workspace changed during observation",
	}
	stale = bindRuntimeRepairSurfaceConfirmationRequestForTest(t, aggregate, stale)
	staleCommitted, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, stale)
	if err != nil {
		t.Fatal(err)
	}
	if staleCommitted.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(staleCommitted.Facts.Prerequisites.Pending) == 0 {
		t.Fatalf("stale confirmation did not enter prerequisite reacquisition: %+v", staleCommitted)
	}

	// This response belongs to a read that was already in flight when the stale
	// response committed. Its different observed revision used to bypass the
	// response-identity fence and establish repair authority before SM-2 had
	// reacquired its prerequisite.
	lateOldConfirmed := stale
	lateOldConfirmed.Disposition = runtimecontract.RuntimeRepairSurfaceConfirmed
	lateOldConfirmed.ConfirmedTargetRefs = append([]string(nil), observation.CandidateTargetRefs...)
	lateOldConfirmed.ObservedRevision = lateOldConfirmed.ExpectedSubjectRevision
	lateOldConfirmed.Reason = ""
	beforeLateRevision, beforeLateSaves := repository.aggregate.StateRevision, repository.saves
	if _, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, lateOldConfirmed); !errors.Is(err, common.ErrConflict) ||
		!strings.Contains(err.Error(), "does not require repair surface confirmation") {
		t.Fatalf("late pre-replan confirmation error = %v", err)
	}
	if repository.saves != beforeLateSaves || repository.aggregate.StateRevision != beforeLateRevision ||
		len(repository.aggregate.RepairSurfaceConfirmations) != 1 || repository.aggregate.Facts.Resources.FailureTarget != nil {
		t.Fatalf("late pre-replan confirmation changed authority: saves=%d/%d aggregate=%+v", repository.saves, beforeLateSaves, repository.aggregate)
	}

	var snapshotPrerequisiteID string
	for _, prerequisite := range staleCommitted.Facts.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && prerequisite.SubjectRef == "main.go" {
			snapshotPrerequisiteID = prerequisite.ID
			break
		}
	}
	if snapshotPrerequisiteID == "" {
		t.Fatalf("stale confirmation did not request a fresh target snapshot: %+v", staleCommitted.Facts.Prerequisites.Pending)
	}
	refreshed, err := controller.commitPrerequisite(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, PrerequisiteResult{
		PrerequisiteID: snapshotPrerequisiteID,
		TargetSnapshot: &runtimecontract.TargetSnapshot{
			TargetRef: "main.go", Content: "package main", ContentHash: "content-current-v3", ContentState: runtimecontract.SnapshotContentStateComplete,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	refreshed = commitRefreshedFileRepairSurfaceCatalogForTest(t, controller, aggregate, refreshed, "tree-current-v3")
	plan, err := CurrentRuntimeRepairSurfaceConfirmationPlan(refreshed)
	if err != nil || !plan.Required() || plan.Protocol != runtimecontract.RuntimeRepairSurfaceConfirmationProtocolFileWorkspaceRegularTargets ||
		plan.OwnerModuleID != runtimecontract.RuntimeModuleFile || !runtimecontract.SameRuntimeValidationFailureObservation(plan.Observation, observation) {
		t.Fatalf("reacquired prerequisite did not reopen the exact current confirmation plan: plan=%+v err=%v aggregate=%+v", plan, err, refreshed)
	}
	if plan.RequestID == stale.RequestID || plan.IssuedAtStateRevision == stale.IssuedAtStateRevision {
		t.Fatalf("reopened confirmation window reused stale request fence: stale=%+v current=%+v", stale, plan)
	}
	beforeReopenLateRevision, beforeReopenLateSaves := repository.aggregate.StateRevision, repository.saves
	if _, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, lateOldConfirmed); !errors.Is(err, common.ErrConflict) ||
		!strings.Contains(err.Error(), "does not match current confirmation window") {
		t.Fatalf("late response crossed reopened confirmation window: %v", err)
	}
	if repository.saves != beforeReopenLateSaves || repository.aggregate.StateRevision != beforeReopenLateRevision ||
		len(repository.aggregate.RepairSurfaceConfirmations) != 1 || repository.aggregate.Facts.Resources.FailureTarget != nil {
		t.Fatalf("late reopened-window response changed authority: saves=%d/%d aggregate=%+v", repository.saves, beforeReopenLateSaves, repository.aggregate)
	}

	currentConfirmed := lateOldConfirmed
	currentConfirmed.RequestID = plan.RequestID
	currentConfirmed.IssuedAtStateRevision = plan.IssuedAtStateRevision
	currentConfirmed.ExpectedSubjectRevision = plan.ExpectedSubjectRevision
	currentConfirmed.ObservedRevision = plan.ExpectedSubjectRevision
	confirmed, err := controller.recordRepairSurfaceConfirmation(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, currentConfirmed)
	if err != nil {
		t.Fatal(err)
	}
	if len(confirmed.RepairSurfaceConfirmations) != 2 || confirmed.Facts.Resources.FailureTarget == nil ||
		!sameRuntimeTargetRefs(confirmed.Facts.Resources.FailureTarget.TargetRefs, observation.CandidateTargetRefs) {
		t.Fatalf("post-replan current confirmation did not establish authority: %+v", confirmed)
	}
	reopenedTamper := cloneRuntimeAggregateForMCPProvenanceTest(t, confirmed)
	reopenedRecord := &reopenedTamper.RepairSurfaceConfirmations[1]
	reopenedRecord.Confirmation.RequestID = stableRuntimeRepairSurfaceConfirmationRequestID(
		reopenedRecord.Confirmation.Observation,
		reopenedRecord.Confirmation.OwnerModuleID,
		runtimecontract.RuntimeRepairSurfaceConfirmationProtocolFileWorkspaceRegularTargets,
		reopenedRecord.Confirmation.SubjectRef,
		reopenedRecord.Confirmation.ExpectedSubjectRevision,
		reopenedRecord.Confirmation.IssuedAtStateRevision,
		"initial",
	)
	reopenedRecord.Signature = runtimecontract.StableRuntimeRepairSurfaceConfirmationHash(reopenedRecord.Confirmation)
	reopenedRecord.IdentitySignature = runtimecontract.StableRuntimeRepairSurfaceConfirmationIdentityHash(reopenedRecord.Confirmation)
	if err := reopenedTamper.Validate(); err == nil || !strings.Contains(err.Error(), "request fence does not match its persisted window") {
		t.Fatalf("fresh load accepted reopened confirmation with erased stale lineage: %v", err)
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

func validationRepairSurfaceAggregateForTest(t *testing.T, requireTargetGrounding bool, prepare ...func(*RuntimeAggregate)) RuntimeAggregate {
	t.Helper()
	return validationAggregateWithPreparation(t, func(aggregate *RuntimeAggregate) {
		aggregate.Facts.Focus.TargetRole = "implementation"
		aggregate.Facts.Focus.RequireTargetGrounding = requireTargetGrounding
		for _, apply := range prepare {
			if apply != nil {
				apply(aggregate)
			}
		}
	})
}

func validationRepairSurfaceObservedAggregateForTest(t *testing.T, requireTargetGrounding bool, diagnosticHash string, prepare ...func(*RuntimeAggregate)) (RuntimeAggregate, runtimecontract.RuntimeValidationFailureObservation) {
	t.Helper()
	repository := &durableRuntimeStateRepository{aggregate: validationRepairSurfaceAggregateForTest(t, requireTargetGrounding, prepare...)}
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

func commitRefreshedFileRepairSurfaceCatalogForTest(t testing.TB, controller *RuntimeController, source, current RuntimeAggregate, observedRevision string) RuntimeAggregate {
	t.Helper()
	var prerequisite runtimecontract.RuntimePrerequisite
	for _, candidate := range current.Facts.Prerequisites.Pending {
		if candidate.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility && candidate.OwnerModuleID == runtimecontract.RuntimeModuleFile {
			prerequisite = candidate
			break
		}
	}
	if prerequisite.ID == "" {
		t.Fatalf("repair-surface refresh has no pending File catalog: %+v", current.Facts.Prerequisites.Pending)
	}
	var catalog runtimecontract.RuntimeOperationAdmissibility
	for _, candidate := range source.Facts.Resources.OperationAdmissibilities {
		if candidate.PrerequisiteID == prerequisite.ID && candidate.OwnerModuleID == prerequisite.OwnerModuleID {
			catalog = candidate.Normalized()
			break
		}
		if catalog.PrerequisiteID == "" && candidate.OwnerModuleID == prerequisite.OwnerModuleID {
			catalog = candidate.Normalized()
		}
	}
	if catalog.PrerequisiteID == "" {
		t.Fatalf("repair-surface refresh source has no File catalog %q: catalogs=%+v observations=%+v", prerequisite.ID, source.Facts.Resources.OperationAdmissibilities, source.Facts.Resources.PrerequisiteObservations)
	}
	catalog.PrerequisiteID = prerequisite.ID
	catalog.ObservedRevision = strings.TrimSpace(observedRevision)
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: catalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: runtimecontract.RuntimeRepairSurfaceFileWorkspaceSubjectRef, Revision: catalog.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	updated, err := controller.commitPrerequisite(context.Background(), current.Facts.WorkspaceID, current.Facts.RunID, PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, OperationAdmissibility: &catalog, Observation: &observation,
	})
	if err != nil {
		t.Fatalf("commit refreshed File repair-surface catalog: %v", err)
	}
	return updated
}
