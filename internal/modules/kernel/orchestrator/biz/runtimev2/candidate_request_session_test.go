package runtimev2

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

	prepared, session, err := controller.EnsureCandidateRequestSession(ctx, "ws-1", "run-1", "stable-prefix", "manifest-v1", "")
	if err != nil {
		t.Fatal(err)
	}
	if session.State != runtimecontract.RuntimeCandidateRequestSessionPrepared || session.NextSequence != 1 || len(prepared.CandidateAttempts) != 0 || prepared.StateRevision != 2 {
		t.Fatalf("prepared request session = %+v aggregate=%+v", session, prepared)
	}
	duplicate, duplicateSession, err := controller.EnsureCandidateRequestSession(ctx, "ws-1", "run-1", "stable-prefix", "manifest-v1", "")
	if err != nil || duplicate.StateRevision != prepared.StateRevision || duplicateSession.LogicalRequestID != session.LogicalRequestID || repository.saves != 1 {
		t.Fatalf("duplicate ensure = aggregate=%+v session=%+v saves=%d err=%v", duplicate, duplicateSession, repository.saves, err)
	}

	if _, err := controller.MarkCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 1, "physical-1"); err != nil {
		t.Fatal(err)
	}
	fragment := runtimecontract.RuntimeCandidateRequestFragment{Sequence: 1, ContentHash: "fragment-1", Bytes: 17, PhysicalCallID: "physical-1"}
	continued, err := controller.RecordCandidateRequestFragment(ctx, "ws-1", "run-1", session.LogicalRequestID, fragment)
	if err != nil {
		t.Fatal(err)
	}
	if len(continued.CandidateAttempts) != 0 || continued.Facts.Repair.RepairAttempts != 0 || continued.ActiveDispatch != nil {
		t.Fatalf("fragment consumed candidate/repair/dispatch state: %+v", continued)
	}
	stored := continued.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionContinuationPending || stored.NextSequence != 2 || len(stored.Fragments) != 1 || stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeDocumentIncomplete {
		t.Fatalf("continued session = %+v", stored)
	}
	duplicateFragment, err := controller.RecordCandidateRequestFragment(ctx, "ws-1", "run-1", session.LogicalRequestID, fragment)
	if err != nil || duplicateFragment.StateRevision != continued.StateRevision {
		t.Fatalf("duplicate fragment = %+v err=%v", duplicateFragment, err)
	}
	_, err = controller.RecordCandidateRequestFragment(ctx, "ws-1", "run-1", session.LogicalRequestID, runtimecontract.RuntimeCandidateRequestFragment{Sequence: 1, ContentHash: "different-fragment", Bytes: 17, PhysicalCallID: "physical-1"})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("conflicting fragment err = %v, want conflict", err)
	}

	if _, err := controller.MarkCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 2, "physical-2"); err != nil {
		t.Fatal(err)
	}
	completed, err := controller.CompleteCandidateRequestDocument(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-2")
	if err != nil {
		t.Fatal(err)
	}
	stored = completed.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete || stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete || len(completed.CandidateAttempts) != 0 {
		t.Fatalf("complete document session = %+v aggregate=%+v", stored, completed)
	}
	if _, err := controller.RecordCandidateRequestFragment(ctx, "ws-1", "run-1", session.LogicalRequestID, runtimecontract.RuntimeCandidateRequestFragment{Sequence: 2, ContentHash: "fragment-2", Bytes: 1, PhysicalCallID: "physical-2"}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("fragment after complete err = %v, want conflict", err)
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
	_, session, err := controller.EnsureCandidateRequestSession(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "session-prefix", manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "session-prefix", manifest, session.LogicalRequestID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("prepared session admitted an attempt: %v", err)
	}
	if len(repository.aggregate.CandidateAttempts) != 0 {
		t.Fatalf("prepared session allocated an attempt: %+v", repository.aggregate.CandidateAttempts)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	if _, err := controller.MarkCandidateRequestSubmitted(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.CompleteCandidateRequestDocument(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, physicalCallID); err != nil {
		t.Fatal(err)
	}
	bound, err := controller.BeginCandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "session-prefix", manifest, session.LogicalRequestID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bound.CandidateAttempts) != 1 || bound.CandidateAttempts[0].ManifestHash != manifest.Hash || bound.CandidateAttempts[0].LogicalRequestID != session.LogicalRequestID {
		t.Fatalf("attempt did not retain complete session identity: %+v", bound.CandidateAttempts)
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
	_, session, err := controller.EnsureCandidateRequestSession(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "drift-prefix", drift.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	if _, err := controller.MarkCandidateRequestSubmitted(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.CompleteCandidateRequestDocument(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, physicalCallID); err != nil {
		t.Fatal(err)
	}
	before := repository.aggregate.StateRevision
	if _, err := controller.BeginCandidateAttempt(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "drift-prefix", drift, session.LogicalRequestID); !errors.Is(err, common.ErrConflict) {
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
	_, session, err := controller.EnsureCandidateRequestSession(ctx, "ws-1", "run-1", "stable-prefix", "manifest-v1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.MarkCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 1, "physical-timeout"); err != nil {
		t.Fatal(err)
	}
	transient := runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient, Code: "provider_timeout", Signature: "timeout"}
	pending, err := controller.RecordCandidateRequestTransportOutcome(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-timeout", transient)
	if err != nil {
		t.Fatal(err)
	}
	if pending.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTransportPending || len(pending.CandidateAttempts) != 0 || pending.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("transport pending aggregate = %+v", pending)
	}
	duplicate, err := controller.RecordCandidateRequestTransportOutcome(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-timeout", transient)
	if err != nil || duplicate.StateRevision != pending.StateRevision {
		t.Fatalf("duplicate transport = %+v err=%v", duplicate, err)
	}
	if _, err := controller.MarkCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 1, "physical-auth"); err != nil {
		t.Fatal(err)
	}
	terminal, err := controller.TerminateCandidateRequestSession(ctx, "ws-1", "run-1", session.LogicalRequestID, runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderAuthentication, Code: "authentication_failed", Signature: "auth"})
	if err != nil {
		t.Fatal(err)
	}
	stored := terminal.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderAuthentication || len(terminal.CandidateAttempts) != 0 || terminal.ActiveDispatch != nil {
		t.Fatalf("terminal session aggregate = %+v", terminal)
	}
	duplicateTerminal, err := controller.TerminateCandidateRequestSession(ctx, "ws-1", "run-1", session.LogicalRequestID, stored.LastOutcome)
	if err != nil || duplicateTerminal.StateRevision != terminal.StateRevision {
		t.Fatalf("duplicate terminal = %+v err=%v", duplicateTerminal, err)
	}
}

func TestRuntimeCandidateRequestSessionContinuationExhaustedTerminalIsAttemptFree(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{aggregate: ptrRuntimeAggregate(stateChangeAggregate(t))}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := controller.EnsureCandidateRequestSession(ctx, "ws-1", "run-1", "stable-prefix", "manifest-v1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.MarkCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 1, "physical-1"); err != nil {
		t.Fatal(err)
	}
	fragment := runtimecontract.RuntimeCandidateRequestFragment{Sequence: 1, ContentHash: "fragment-1", Bytes: 17, PhysicalCallID: "physical-1"}
	if _, err := controller.RecordCandidateRequestFragment(ctx, "ws-1", "run-1", session.LogicalRequestID, fragment); err != nil {
		t.Fatal(err)
	}
	outcome := runtimecontract.RuntimeCandidateRequestOutcome{
		Class: runtimecontract.RuntimeCandidateRequestOutcomeContinuationExhausted,
		Code:  "candidate_output_continuation_exhausted", Signature: "candidate-output-continuation-exhausted",
	}
	terminal, err := controller.TerminateCandidateRequestSession(ctx, "ws-1", "run-1", session.LogicalRequestID, outcome)
	if err != nil {
		t.Fatal(err)
	}
	stored := terminal.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || stored.LastOutcome != outcome ||
		len(stored.Fragments) != 1 || len(terminal.CandidateAttempts) != 0 || terminal.ActiveDispatch != nil ||
		terminal.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("continuation exhaustion terminal aggregate = %+v", terminal)
	}
	duplicate, err := controller.TerminateCandidateRequestSession(ctx, "ws-1", "run-1", session.LogicalRequestID, outcome)
	if err != nil || duplicate.StateRevision != terminal.StateRevision {
		t.Fatalf("duplicate continuation exhaustion terminal = %+v err=%v", duplicate, err)
	}
}

func ptrRuntimeAggregate(value RuntimeAggregate) *RuntimeAggregate { return &value }
