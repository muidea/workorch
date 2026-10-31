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
	outcome := runtimecontract.RuntimeProviderDocumentFraming{
		Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: session.ExpectedDocument.Hash,
		ResponseHash: runtimecontract.StableRuntimeProviderResponseHash(partial), ResponseBytes: int64(len(partial)),
		Code: runtimecontract.RuntimeProviderDocumentCodeIncompleteJSON, Signature: runtimecontract.RuntimeProviderDocumentSignatureIncompleteJSON,
	}
	recovery := runtimeProviderDocumentRecoveryFactForTest(t, runtimecontract.RuntimeProviderDocumentOperations, outcome)
	settlement := runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, "physical-1", nil)
	continued, err := controller.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-1", recovery, settlement)
	if err != nil {
		t.Fatal(err)
	}
	if len(continued.CandidateAttempts) != 0 || continued.Facts.Repair.RepairAttempts != 0 || continued.ActiveDispatch != nil {
		t.Fatalf("incomplete outcome consumed candidate/repair/dispatch state: %+v", continued)
	}
	stored := continued.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionRecoveryPending || stored.NextSequence != 2 || stored.DocumentRecoveryOutcomes != 1 || stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecovery || stored.LastDocumentFraming == nil {
		t.Fatalf("recovery session = %+v", stored)
	}
	duplicateOutcome, err := controller.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-1", recovery, settlement)
	if err != nil || duplicateOutcome.StateRevision != continued.StateRevision {
		t.Fatalf("duplicate incomplete outcome = %+v err=%v", duplicateOutcome, err)
	}
	conflict := outcome
	conflict.ResponseHash = runtimecontract.StableRuntimeProviderResponseHash([]byte("different"))
	conflictingRecovery := runtimeProviderDocumentRecoveryFactForTest(t, runtimecontract.RuntimeProviderDocumentOperations, conflict)
	_, err = controller.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-1", conflictingRecovery, settlement)
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
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete || stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete || stored.LastDocumentFraming != nil || len(completed.CandidateAttempts) != 0 {
		t.Fatalf("complete document session = %+v aggregate=%+v", stored, completed)
	}
	if _, err := controller.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-2", recovery, runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, "physical-2", nil)); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("incomplete outcome after complete err = %v, want conflict", err)
	}
}

func TestRuntimeCandidateRequestSessionRestartClearsIncompleteFramingAcrossTransportAndComplete(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{aggregate: ptrRuntimeAggregate(stateChangeAggregate(t))}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := controller.ensureCandidateRequestSession(ctx, "ws-1", "run-1", runtimecontract.RuntimeProviderDocumentOperations, "framing-prefix", "manifest-v1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.markCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 1, "physical-incomplete"); err != nil {
		t.Fatal(err)
	}
	partial := []byte(`{"operations":[`)
	framing := runtimecontract.RuntimeProviderDocumentFraming{
		Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: session.ExpectedDocument.Hash,
		ResponseHash: runtimecontract.StableRuntimeProviderResponseHash(partial), ResponseBytes: int64(len(partial)),
		Code: runtimecontract.RuntimeProviderDocumentCodeIncompleteJSON, Signature: runtimecontract.RuntimeProviderDocumentSignatureIncompleteJSON,
	}
	recovery := runtimeProviderDocumentRecoveryFactForTest(t, runtimecontract.RuntimeProviderDocumentOperations, framing)
	if _, err := controller.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-incomplete", recovery, runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, "physical-incomplete", nil)); err != nil {
		t.Fatal(err)
	}

	// Reloading the owner between durable transitions must neither retain a
	// framing fact in an incompatible state nor resend the earlier call.
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.markCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 2, "physical-transient"); err != nil {
		t.Fatal(err)
	}
	transient := runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient, Code: "provider_timeout", Signature: "provider-timeout"}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		OriginalClass: string(runtimecontract.RuntimeCandidateFailureClassProviderTransient), OriginalCode: transient.Code,
		OriginalMessage: "provider transport timed out", Signature: transient.Signature,
	}
	transport := runtimeProviderTransportRecoveryFactForTest(t, runtimecontract.RuntimeProviderDocumentOperations, transient, diagnostic.OriginalMessage, &diagnostic)
	transportPending, err := restarted.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-transient", transport, runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, "physical-transient", nil))
	if err != nil {
		t.Fatal(err)
	}
	stored := transportPending.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionTransportPending || stored.NextSequence != 3 || stored.LastDocumentFraming != nil {
		t.Fatalf("transport successor retained stale incomplete framing: %+v", stored)
	}

	restartedAgain, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restartedAgain.markCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 3, "physical-complete"); err != nil {
		t.Fatal(err)
	}
	completed, err := restartedAgain.completeCandidateRequestDocument(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-complete")
	if err != nil {
		t.Fatal(err)
	}
	stored = completed.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionDocumentComplete || stored.LastDocumentFraming != nil || stored.NextSequence != 3 {
		t.Fatalf("complete successor retained stale framing after restart: %+v", stored)
	}
}

func TestRuntimeCandidateRequestSessionNonDocumentTerminalClearsIncompleteFraming(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{aggregate: ptrRuntimeAggregate(stateChangeAggregate(t))}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := controller.ensureCandidateRequestSession(ctx, "ws-1", "run-1", runtimecontract.RuntimeProviderDocumentOperations, "terminal-framing-prefix", "manifest-v1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.markCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 1, "physical-incomplete"); err != nil {
		t.Fatal(err)
	}
	partial := []byte(`{"operations":[`)
	framing := runtimecontract.RuntimeProviderDocumentFraming{
		Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: session.ExpectedDocument.Hash,
		ResponseHash: runtimecontract.StableRuntimeProviderResponseHash(partial), ResponseBytes: int64(len(partial)),
		Code: runtimecontract.RuntimeProviderDocumentCodeIncompleteJSON, Signature: runtimecontract.RuntimeProviderDocumentSignatureIncompleteJSON,
	}
	recovery := runtimeProviderDocumentRecoveryFactForTest(t, runtimecontract.RuntimeProviderDocumentOperations, framing)
	if _, err := controller.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-incomplete", recovery, runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, "physical-incomplete", nil)); err != nil {
		t.Fatal(err)
	}
	fact, err := NormalizeRuntimeProviderRequestTerminal(RuntimeProviderRequestTerminalInput{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		OutcomeClass: runtimecontract.RuntimeCandidateRequestOutcomeProviderConfiguration,
		TerminalCode: "provider_configuration", TerminalSignature: "provider-configuration",
		Message: "provider configuration is unavailable",
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := controller.planning.coordinateProviderRequestTerminal(ctx, "ws-1", "run-1", session.LogicalRequestID, fact)
	if err != nil {
		t.Fatal(err)
	}
	stored := terminal.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderConfiguration || stored.LastDocumentFraming != nil {
		t.Fatalf("non-document terminal retained stale incomplete framing: %+v", stored)
	}
}

func TestRuntimeCandidateNoDocumentRestartKeepsRecoveryOutsideCandidateAttempts(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{aggregate: ptrRuntimeAggregate(stateChangeAggregate(t))}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := controller.EnsureSemanticContractSealedForOperationTest(ctx, "ws-1", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	const stablePrefix = "no-document-recovery-prefix"
	_, session, err := controller.ensureCandidateRequestSession(ctx, "ws-1", "run-1", runtimecontract.RuntimeProviderDocumentOperations, stablePrefix, manifest.Hash, "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1)
	if _, err := controller.markCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 1, physicalCallID); err != nil {
		t.Fatal(err)
	}
	framing := runtimecontract.RuntimeProviderDocumentFraming{
		Disposition: runtimecontract.RuntimeProviderDocumentEmpty, ExpectedDocumentHash: session.ExpectedDocument.Hash,
		Code: runtimecontract.RuntimeProviderDocumentCodeFinalOutputMissing, Signature: runtimecontract.RuntimeProviderDocumentSignatureOutputMissing,
	}
	settlement := runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, physicalCallID, nil)
	fact := runtimeProviderDocumentRecoveryFactForTest(t, runtimecontract.RuntimeProviderDocumentOperations, framing)
	recovered, err := controller.planning.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", session.LogicalRequestID, physicalCallID, fact, settlement)
	if err != nil {
		t.Fatal(err)
	}
	stored := recovered.CandidateRequestSessions[len(recovered.CandidateRequestSessions)-1]
	if len(recovered.CandidateAttempts) != 0 || stored.State != runtimecontract.RuntimeCandidateRequestSessionRecoveryPending ||
		stored.DocumentRecoveryOutcomes != 1 || stored.NextSequence != 2 || stored.ProviderSubmitted {
		t.Fatalf("no-document recovery crossed the candidate-attempt boundary: %+v", recovered)
	}
	saves := repository.saves
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := restarted.planning.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", session.LogicalRequestID, physicalCallID, fact, settlement)
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != saves || duplicate.StateRevision != recovered.StateRevision || len(duplicate.CandidateAttempts) != 0 {
		t.Fatalf("duplicate no-document result rewrote aggregate: saves=%d want=%d", repository.saves, saves)
	}
	invalidBody := []byte("provider prose")
	conflicting := runtimecontract.RuntimeProviderDocumentFraming{
		Disposition: runtimecontract.RuntimeProviderDocumentInvalid, ExpectedDocumentHash: session.ExpectedDocument.Hash,
		ResponseHash: runtimecontract.StableRuntimeProviderResponseHash(invalidBody), ResponseBytes: int64(len(invalidBody)),
		Code: runtimecontract.RuntimeProviderDocumentCodeFinalOutputInvalid, Signature: runtimecontract.RuntimeProviderDocumentSignatureOutputInvalid,
	}
	conflictingFact := runtimeProviderDocumentRecoveryFactForTest(t, runtimecontract.RuntimeProviderDocumentOperations, conflicting)
	if _, err := restarted.planning.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", session.LogicalRequestID, physicalCallID, conflictingFact, settlement); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("conflicting duplicate document framing err=%v, want conflict", err)
	}
}

func TestSemanticRequestMayReplaceOnlyBindingFreeProvisionalTerminal(t *testing.T) {
	aggregate := RuntimeAggregate{
		ResultFeedbackVersion: runtimecontract.RuntimeResultFeedbackVersionCurrent,
		Facts: runtimecontract.RuntimeFacts{
			Focus: runtimecontract.CoverageFocus{ID: "semantic-pending/cwi-2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}},
			Coverage: []runtimecontract.RuntimeCoverageItemFact{{
				ID: "cwi-2", State: runtimecontract.RuntimeCoverageItemStateOpen,
				CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStatePlanning,
			}},
		},
		Decision: runtimecontract.RuntimeDecision{
			State: runtimecontract.ConvergenceStateFailed,
			Terminal: &runtimecontract.RuntimeTerminalDecision{
				Code: "target_grounding_unresolved", Summary: "binding-free focus has no target yet",
			},
		},
		SemanticPlanning: runtimecontract.RuntimeSemanticPlanningLedger{Phase: runtimecontract.RuntimeSemanticPlanningRequestOpen},
	}
	if SemanticOperationAuthorityReady(aggregate) {
		t.Fatal("binding-free provisional terminal unexpectedly has semantic operation authority")
	}
	if err := candidateRequestSessionActionAllowed(aggregate, runtimecontract.RuntimeProviderDocumentSemanticPlan); err != nil {
		t.Fatalf("fresh semantic generation was blocked by provisional reducer terminal: %v", err)
	}
	if err := candidateRequestSessionActionAllowed(aggregate, runtimecontract.RuntimeProviderDocumentOperations); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("operations request crossed binding-free terminal window: %v", err)
	}
	aggregate.Decision.Terminal.Code = "runtime_contract_defect"
	if err := candidateRequestSessionActionAllowed(aggregate, runtimecontract.RuntimeProviderDocumentSemanticPlan); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("semantic request replaced an authoritative non-grounding terminal: %v", err)
	}
	aggregate.Decision.Terminal.Code = "target_grounding_unresolved"

	aggregate.SemanticPlanning.Phase = runtimecontract.RuntimeSemanticPlanningSealed
	if err := candidateRequestSessionActionAllowed(aggregate, runtimecontract.RuntimeProviderDocumentSemanticPlan); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("semantic request replaced an authoritative sealed terminal: %v", err)
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
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		OriginalClass: string(runtimecontract.RuntimeCandidateFailureClassProviderTransient),
		OriginalCode:  transient.Code, OriginalMessage: "provider transport timed out", Signature: transient.Signature,
	}
	transport := runtimeProviderTransportRecoveryFactForTest(t, runtimecontract.RuntimeProviderDocumentOperations, transient, diagnostic.OriginalMessage, &diagnostic)
	pending, err := controller.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-timeout", transport, runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, "physical-timeout", nil))
	if err != nil {
		t.Fatal(err)
	}
	if pending.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTransportPending || pending.CandidateRequestSessions[0].NextSequence != 2 || pending.CandidateRequestSessions[0].TransportOutcomes != 1 || len(pending.CandidateAttempts) != 0 || pending.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("transport pending aggregate = %+v", pending)
	}
	duplicate, err := controller.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-timeout", transport, runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, "physical-timeout", nil))
	if err != nil || duplicate.StateRevision != pending.StateRevision {
		t.Fatalf("duplicate transport = %+v err=%v", duplicate, err)
	}
	if _, err := controller.markCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 2, "physical-auth"); err != nil {
		t.Fatal(err)
	}
	fact, err := NormalizeRuntimeProviderRequestTerminal(RuntimeProviderRequestTerminalInput{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		OutcomeClass: runtimecontract.RuntimeCandidateRequestOutcomeProviderAuthentication,
		TerminalCode: "authentication_failed", TerminalSignature: "auth", Message: "provider authentication failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := controller.planning.coordinateProviderRequestTerminal(ctx, "ws-1", "run-1", session.LogicalRequestID, fact)
	if err != nil {
		t.Fatal(err)
	}
	stored := terminal.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderAuthentication || len(terminal.CandidateAttempts) != 0 || terminal.ActiveDispatch != nil {
		t.Fatalf("terminal session aggregate = %+v", terminal)
	}
	duplicateTerminal, err := controller.planning.coordinateProviderRequestTerminal(ctx, "ws-1", "run-1", session.LogicalRequestID, fact)
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
	documentOutcome := runtimecontract.RuntimeProviderDocumentFraming{
		Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: session.ExpectedDocument.Hash,
		ResponseHash: runtimecontract.StableRuntimeProviderResponseHash(partial), ResponseBytes: int64(len(partial)),
		Code: runtimecontract.RuntimeProviderDocumentCodeIncompleteJSON, Signature: runtimecontract.RuntimeProviderDocumentSignatureIncompleteJSON,
	}
	documentRecovery := runtimeProviderDocumentRecoveryFactForTest(t, runtimecontract.RuntimeProviderDocumentOperations, documentOutcome)
	pending, err := controller.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-1", documentRecovery, runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, "physical-1", nil))
	if err != nil {
		t.Fatal(err)
	}
	current := pending.CandidateRequestSessions[0]
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(current.LogicalRequestID, current.NextSequence)
	if _, err := controller.markCandidateRequestSubmitted(ctx, "ws-1", "run-1", current.LogicalRequestID, current.NextSequence, physicalCallID); err != nil {
		t.Fatal(err)
	}
	terminal, err := controller.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", current.LogicalRequestID, physicalCallID, documentRecovery, runtimeProviderConfirmedBudgetSettlementForTest(t, current.LogicalRequestID, physicalCallID, nil))
	if err != nil {
		t.Fatal(err)
	}
	stored := terminal.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted ||
		stored.DocumentRecoveryOutcomes != 2 || stored.LastDocumentFraming == nil || len(terminal.CandidateAttempts) != 0 || terminal.ActiveDispatch != nil ||
		terminal.Facts.Repair.RepairAttempts != 0 {
		t.Fatalf("document recovery exhaustion terminal aggregate = %+v", terminal)
	}
	duplicate, err := controller.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", current.LogicalRequestID, physicalCallID, documentRecovery, runtimeProviderConfirmedBudgetSettlementForTest(t, current.LogicalRequestID, physicalCallID, nil))
	if err != nil || duplicate.StateRevision != terminal.StateRevision {
		t.Fatalf("duplicate document recovery exhaustion terminal = %+v err=%v", duplicate, err)
	}
}

func ptrRuntimeAggregate(value RuntimeAggregate) *RuntimeAggregate { return &value }

func TestOperationsRequestWriterRejectsRendererDriftBeforeCAS(t *testing.T) {
	for _, invalid := range []string{"", "unknown-encoding"} {
		ctx := context.Background()
		repo := &controllerRepository{aggregate: ptrRuntimeAggregate(stateChangeAggregate(t))}
		controller, err := NewRuntimeController(repo)
		if err != nil {
			t.Fatal(err)
		}
		expected := runtimeTestOperationsExpectedDocument(*repo.aggregate, "manifest", 0)
		expected.OperationsContextEncoding = runtimecontract.RuntimeOperationsContextEncoding
		_, session, err := controller.ensureCandidateRequestSessionWithExpected(ctx, "ws-1", "run-1", runtimecontract.RuntimeProviderDocumentOperations, "prefix", "manifest", "", &expected)
		if err != nil {
			t.Fatal(err)
		}
		before := repo.saves
		restarted, err := NewRuntimeController(repo)
		if err != nil {
			t.Fatal(err)
		}
		expected = *session.ExpectedDocument
		expected.OperationsContextEncoding = invalid
		if _, _, err := restarted.ensureCandidateRequestSessionWithExpected(ctx, "ws-1", "run-1", runtimecontract.RuntimeProviderDocumentOperations, "prefix", "manifest", "correction", &expected); err == nil {
			t.Fatal("correction upgraded/downgraded sealed renderer")
		}
		if repo.saves != before {
			t.Fatal("renderer rejection wrote owner state")
		}
	}
}
