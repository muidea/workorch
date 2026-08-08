package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeCandidateRequestSessionCASLifecycleDoesNotAllocateCandidateAttempt(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{aggregate: ptrRuntimeAggregate(stateChangeAggregate(t))}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}

	prepared, session, err := controller.ensureCandidateRequestSession(ctx, "ws-1", "run-1", runtimecontract.RuntimeProviderDocumentOperations, "stable-prefix", "manifest-v1", "")
	if err != nil {
		t.Fatal(err)
	}
	if session.State != runtimecontract.RuntimeCandidateRequestSessionPrepared || session.NextSequence != 1 || len(prepared.CandidateAttempts) != 0 || prepared.StateRevision != 2 {
		t.Fatalf("prepared request session = %+v aggregate=%+v", session, prepared)
	}
	duplicate, duplicateSession, err := controller.ensureCandidateRequestSession(ctx, "ws-1", "run-1", runtimecontract.RuntimeProviderDocumentOperations, "stable-prefix", "manifest-v1", "")
	if err != nil || duplicate.StateRevision != prepared.StateRevision || duplicateSession.LogicalRequestID != session.LogicalRequestID || repository.saves != 1 {
		t.Fatalf("duplicate ensure = aggregate=%+v session=%+v saves=%d err=%v", duplicate, duplicateSession, repository.saves, err)
	}

	if _, err := controller.markCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 1, "physical-1"); err != nil {
		t.Fatal(err)
	}
	partial := []byte(`{"operations":[`)
	outcome := runtimecontract.RuntimeProviderDocumentOutcome{
		Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: session.ExpectedDocument.Hash,
		ResponseHash: runtimecontract.StableRuntimeProviderResponseHash(partial), ResponseBytes: int64(len(partial)),
		Code: "provider_output_incomplete", Signature: "provider-output-incomplete",
	}
	continued, err := controller.recordCandidateRequestRecoveryRequired(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-1", outcome)
	if err != nil {
		t.Fatal(err)
	}
	if len(continued.CandidateAttempts) != 0 || continued.Facts.Repair.RepairAttempts != 0 || continued.ActiveDispatch != nil {
		t.Fatalf("incomplete outcome consumed candidate/repair/dispatch state: %+v", continued)
	}
	stored := continued.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionRecoveryPending || stored.NextSequence != 2 || stored.IncompleteOutcomes != 1 || stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeDocumentIncomplete || stored.LastDocumentOutcome == nil {
		t.Fatalf("recovery session = %+v", stored)
	}
	duplicateOutcome, err := controller.recordCandidateRequestRecoveryRequired(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-1", outcome)
	if err != nil || duplicateOutcome.StateRevision != continued.StateRevision {
		t.Fatalf("duplicate incomplete outcome = %+v err=%v", duplicateOutcome, err)
	}
	conflict := outcome
	conflict.ResponseHash = runtimecontract.StableRuntimeProviderResponseHash([]byte("different"))
	_, err = controller.recordCandidateRequestRecoveryRequired(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-1", conflict)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("conflicting incomplete outcome err = %v, want conflict", err)
	}

	if _, err := controller.markCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 2, "physical-2"); err != nil {
		t.Fatal(err)
	}
	completed, err := controller.completeCandidateRequestDocument(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-2")
	if err != nil {
		t.Fatal(err)
	}
	stored = completed.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete || stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete || len(completed.CandidateAttempts) != 0 {
		t.Fatalf("complete document session = %+v aggregate=%+v", stored, completed)
	}
	if _, err := controller.recordCandidateRequestRecoveryRequired(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-2", outcome); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("incomplete outcome after complete err = %v, want conflict", err)
	}
}

func TestBeginCandidateAttemptRequiresCompletedSessionAndPersistsManifestBinding(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources((aggregate.Facts).Resources))
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := controller.ensureCandidateRequestSession(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentOperations, "session-prefix", manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.beginCandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "session-prefix", manifest, session.LogicalRequestID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("prepared session admitted an attempt: %v", err)
	}
	if len(repository.aggregate.CandidateAttempts) != 0 {
		t.Fatalf("prepared session allocated an attempt: %+v", repository.aggregate.CandidateAttempts)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	if _, err := controller.markCandidateRequestSubmitted(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.completeCandidateRequestDocument(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, physicalCallID); err != nil {
		t.Fatal(err)
	}
	bound, err := controller.beginCandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "session-prefix", manifest, session.LogicalRequestID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bound.CandidateAttempts) != 1 || bound.CandidateAttempts[0].ManifestHash != manifest.Hash || bound.CandidateAttempts[0].LogicalRequestID != session.LogicalRequestID {
		t.Fatalf("attempt did not retain complete session identity: %+v", bound.CandidateAttempts)
	}
}

func TestCandidateDocumentResponseAfterTerminationCannotCreateRuntimeProgress(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(aggregate.Facts.Resources))
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := controller.ensureCandidateRequestSession(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentOperations, "termination-prefix", manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	if _, err := controller.markCandidateRequestSubmitted(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
		t.Fatal(err)
	}
	terminated, err := controller.requestTermination(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeTerminationSuperseded, "run-owner", "replacement run activated")
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision := terminated.StateRevision
	beforeSaves := repository.saves

	if _, err := controller.completeCandidateRequestDocument(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, physicalCallID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("complete document after termination err = %v, want conflict", err)
	}
	if repository.aggregate.StateRevision != beforeRevision || repository.saves != beforeSaves || len(repository.aggregate.CandidateAttempts) != 0 || repository.aggregate.ActiveDispatch != nil {
		t.Fatalf("late provider response changed terminated runtime: revision=%d saves=%d attempts=%d dispatch=%+v", repository.aggregate.StateRevision, repository.saves, len(repository.aggregate.CandidateAttempts), repository.aggregate.ActiveDispatch)
	}
	stored := repository.aggregate.CandidateRequestSessions[0]
	if stored.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete || !stored.ProviderSubmitted {
		t.Fatalf("late provider response changed request session: %+v", stored)
	}
}

func TestBeginCandidateAttemptRejectsSessionManifestDriftWithoutProposalOrDispatch(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources((aggregate.Facts).Resources))
	if err != nil {
		t.Fatal(err)
	}
	drift := manifest.Clone()
	drift.TargetRefs = []string{"unsealed-target"}
	drift.Hash = runtimecontract.StableRuntimeCandidateContractManifestHash(drift)
	_, session, err := controller.ensureCandidateRequestSession(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentOperations, "drift-prefix", drift.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	if _, err := controller.markCandidateRequestSubmitted(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.completeCandidateRequestDocument(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, physicalCallID); err != nil {
		t.Fatal(err)
	}
	before := repository.aggregate.StateRevision
	if _, err := controller.beginCandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "drift-prefix", drift, session.LogicalRequestID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("manifest drift admitted an attempt: %v", err)
	}
	if repository.aggregate.StateRevision != before || len(repository.aggregate.CandidateAttempts) != 0 || repository.aggregate.ActiveDispatch != nil {
		t.Fatalf("manifest drift changed SM-3 state: %+v", repository.aggregate)
	}
}

func TestRuntimeCandidateRequestSessionTransportAndTerminalOutcomesAreAttemptFree(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{aggregate: ptrRuntimeAggregate(stateChangeAggregate(t))}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := controller.ensureCandidateRequestSession(ctx, "ws-1", "run-1", runtimecontract.RuntimeProviderDocumentOperations, "stable-prefix", "manifest-v1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.markCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 1, "physical-timeout"); err != nil {
		t.Fatal(err)
	}
	transient := runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient, Code: "provider_timeout", Signature: "timeout"}
	pending, err := controller.recordCandidateRequestTransportOutcome(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-timeout", transient)
	if err != nil {
		t.Fatal(err)
	}
	if pending.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTransportPending || len(pending.CandidateAttempts) != 0 || pending.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("transport pending aggregate = %+v", pending)
	}
	duplicate, err := controller.recordCandidateRequestTransportOutcome(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-timeout", transient)
	if err != nil || duplicate.StateRevision != pending.StateRevision {
		t.Fatalf("duplicate transport = %+v err=%v", duplicate, err)
	}
	if _, err := controller.markCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 1, "physical-auth"); err != nil {
		t.Fatal(err)
	}
	terminal, err := controller.terminateCandidateRequestSession(ctx, "ws-1", "run-1", session.LogicalRequestID, runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderAuthentication, Code: "authentication_failed", Signature: "auth"})
	if err != nil {
		t.Fatal(err)
	}
	stored := terminal.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderAuthentication || len(terminal.CandidateAttempts) != 0 || terminal.ActiveDispatch != nil {
		t.Fatalf("terminal session aggregate = %+v", terminal)
	}
	duplicateTerminal, err := controller.terminateCandidateRequestSession(ctx, "ws-1", "run-1", session.LogicalRequestID, stored.LastOutcome)
	if err != nil || duplicateTerminal.StateRevision != terminal.StateRevision {
		t.Fatalf("duplicate terminal = %+v err=%v", duplicateTerminal, err)
	}
}

func TestRuntimeCandidateRequestSessionDocumentRecoveryExhaustedTerminalIsAttemptFree(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{aggregate: ptrRuntimeAggregate(stateChangeAggregate(t))}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := controller.ensureCandidateRequestSession(ctx, "ws-1", "run-1", runtimecontract.RuntimeProviderDocumentOperations, "stable-prefix", "manifest-v1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.markCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 1, "physical-1"); err != nil {
		t.Fatal(err)
	}
	partial := []byte(`{"operations":[`)
	documentOutcome := runtimecontract.RuntimeProviderDocumentOutcome{
		Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: session.ExpectedDocument.Hash,
		ResponseHash: runtimecontract.StableRuntimeProviderResponseHash(partial), ResponseBytes: int64(len(partial)),
		Code: "provider_output_incomplete", Signature: "provider-output-incomplete",
	}
	if _, err := controller.recordCandidateRequestRecoveryRequired(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-1", documentOutcome); err != nil {
		t.Fatal(err)
	}
	outcome := runtimecontract.RuntimeCandidateRequestOutcome{
		Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted,
		Code:  "candidate_document_recovery_exhausted", Signature: "candidate-document-recovery-exhausted",
	}
	terminal, err := controller.terminateCandidateRequestSession(ctx, "ws-1", "run-1", session.LogicalRequestID, outcome)
	if err != nil {
		t.Fatal(err)
	}
	stored := terminal.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || stored.LastOutcome != outcome ||
		stored.IncompleteOutcomes != 1 || len(terminal.CandidateAttempts) != 0 || terminal.ActiveDispatch != nil ||
		terminal.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("document recovery exhaustion terminal aggregate = %+v", terminal)
	}
	duplicate, err := controller.terminateCandidateRequestSession(ctx, "ws-1", "run-1", session.LogicalRequestID, outcome)
	if err != nil || duplicate.StateRevision != terminal.StateRevision {
		t.Fatalf("duplicate document recovery exhaustion terminal = %+v err=%v", duplicate, err)
	}
}

func ptrRuntimeAggregate(value RuntimeAggregate) *RuntimeAggregate { return &value }
