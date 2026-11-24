package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const testProviderTransportObservedAt = "2026-11-23T08:00:00Z"

func runtimeProviderDocumentRecoveryFactForTest(t *testing.T, kind runtimecontract.RuntimeProviderDocumentKind, framing runtimecontract.RuntimeProviderDocumentFraming) RuntimeProviderRequestRecoveryFact {
	t.Helper()
	fact, err := NormalizeRuntimeProviderRequestRecovery(RuntimeProviderRequestRecoveryInput{DocumentKind: kind, DocumentFraming: &framing})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func runtimeProviderTransportRecoveryFactForTest(t *testing.T, kind runtimecontract.RuntimeProviderDocumentKind, outcome runtimecontract.RuntimeCandidateRequestOutcome, message string, diagnostic *runtimecontract.RuntimeOriginalDiagnostic) RuntimeProviderRequestRecoveryFact {
	t.Helper()
	return runtimeProviderTransportRecoveryFactAtTimeForTest(t, kind, outcome, message, diagnostic, testProviderTransportObservedAt)
}

func runtimeProviderTransportRecoveryFactAtTimeForTest(t *testing.T, kind runtimecontract.RuntimeProviderDocumentKind, outcome runtimecontract.RuntimeCandidateRequestOutcome, message string, diagnostic *runtimecontract.RuntimeOriginalDiagnostic, observedAt string) RuntimeProviderRequestRecoveryFact {
	t.Helper()
	fact, err := NormalizeRuntimeProviderRequestRecovery(RuntimeProviderRequestRecoveryInput{
		DocumentKind: kind, TransportOutcome: &outcome, Message: message, OriginalDiagnostic: diagnostic, ObservedAt: observedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func runtimeProviderConfirmedBudgetSettlementForTest(t *testing.T, logicalRequestID, physicalCallID string, usage *llmaudit.ProviderUsage) *RuntimeProviderBudgetSettlement {
	t.Helper()
	ownerHash := stableRuntimeProviderOwnerFactHash("test-confirmed-provider", logicalRequestID, physicalCallID)
	settlement, err := newRuntimeProviderBudgetSettlement(
		logicalRequestID, physicalCallID, RuntimeProviderBudgetSettlementConfirmed, usage,
		"llm/test-provider-audit/"+physicalCallID+"/"+ownerHash, ownerHash,
	)
	if err != nil {
		t.Fatal(err)
	}
	return &settlement
}

func TestNormalizeRuntimeProviderRequestRecoveryRejectsMissingOrMismatchedTransientDiagnostic(t *testing.T) {
	outcome := runtimecontract.RuntimeCandidateRequestOutcome{
		Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient,
		Code:  "provider_body_idle_timeout", Signature: "provider-body-idle-timeout/exact",
	}
	base := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		OriginalClass: string(runtimecontract.RuntimeCandidateFailureClassProviderTransient),
		OriginalCode:  outcome.Code, OriginalMessage: "provider body became idle", Signature: outcome.Signature,
	}
	tests := []struct {
		name       string
		diagnostic *runtimecontract.RuntimeOriginalDiagnostic
		message    string
	}{
		{name: "missing", message: base.OriginalMessage},
		{name: "owner_mismatch", diagnostic: func() *runtimecontract.RuntimeOriginalDiagnostic {
			value := base
			value.OwnerModuleID = runtimecontract.RuntimeModuleOrchestrator
			return &value
		}(), message: base.OriginalMessage},
		{name: "class_mismatch", diagnostic: func() *runtimecontract.RuntimeOriginalDiagnostic {
			value := base
			value.OriginalClass = string(runtimecontract.RuntimeCandidateFailureClassProviderProtocol)
			return &value
		}(), message: base.OriginalMessage},
		{name: "code_mismatch", diagnostic: func() *runtimecontract.RuntimeOriginalDiagnostic {
			value := base
			value.OriginalCode = "provider_transport_timeout"
			return &value
		}(), message: base.OriginalMessage},
		{name: "signature_mismatch", diagnostic: func() *runtimecontract.RuntimeOriginalDiagnostic {
			value := base
			value.Signature = "provider-transport-timeout/exact"
			return &value
		}(), message: base.OriginalMessage},
		{name: "message_mismatch", diagnostic: &base, message: "different transient message"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormalizeRuntimeProviderRequestRecovery(RuntimeProviderRequestRecoveryInput{
				DocumentKind:     runtimecontract.RuntimeProviderDocumentOperations,
				TransportOutcome: &outcome, Message: tc.message, OriginalDiagnostic: tc.diagnostic, ObservedAt: testProviderTransportObservedAt,
			}); err == nil {
				t.Fatal("mismatched transient diagnostic was accepted")
			}
		})
	}
	if _, err := NormalizeRuntimeProviderRequestRecovery(RuntimeProviderRequestRecoveryInput{
		DocumentKind:     runtimecontract.RuntimeProviderDocumentOperations,
		TransportOutcome: &outcome, Message: base.OriginalMessage, OriginalDiagnostic: &base, ObservedAt: "",
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("missing ObservedAt error = %v, want invalid argument", err)
	}
	if _, err := NormalizeRuntimeProviderRequestRecovery(RuntimeProviderRequestRecoveryInput{
		DocumentKind:     runtimecontract.RuntimeProviderDocumentOperations,
		TransportOutcome: &outcome, Message: base.OriginalMessage, OriginalDiagnostic: &base, ObservedAt: "not-rfc3339",
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("invalid ObservedAt error = %v, want invalid argument", err)
	}
	if _, err := NormalizeRuntimeProviderRequestRecovery(RuntimeProviderRequestRecoveryInput{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		DocumentFraming: &runtimecontract.RuntimeProviderDocumentFraming{
			Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: "hash-1",
			ResponseHash: "resp-1", ResponseBytes: 4, Code: "incomplete_json", Signature: "incomplete",
		},
		ObservedAt: testProviderTransportObservedAt,
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("non-empty ObservedAt on document framing error = %v, want invalid argument", err)
	}

	valid := runtimeProviderTransportRecoveryFactForTest(t, runtimecontract.RuntimeProviderDocumentOperations, outcome, base.OriginalMessage, &base)
	requestMismatch := valid
	requestMismatch.Terminal.RequestOutcome.Code = "provider_transport_timeout"
	if err := requestMismatch.Validate(); err == nil {
		t.Fatal("recovery fact with terminal request-outcome mismatch was accepted")
	}
	diagnosticMismatch := valid
	diagnosticMismatch.Terminal.Failure.OriginalDiagnostic = base.CloneOriginal()
	diagnosticMismatch.Terminal.Failure.OriginalDiagnostic.Signature = "provider-transport-timeout/exact"
	if err := diagnosticMismatch.Validate(); err == nil {
		t.Fatal("recovery fact with terminal original-diagnostic mismatch was accepted")
	}
	messageMismatch := valid
	messageMismatch.Terminal.Failure.OriginalDiagnostic = base.CloneOriginal()
	messageMismatch.Terminal.Failure.OriginalDiagnostic.OriginalMessage = "different persisted transient message"
	if err := messageMismatch.Validate(); err == nil {
		t.Fatal("recovery fact with terminal original-message mismatch was accepted")
	}
}

func TestProviderRequestIncompleteRecoveryCoordinatesPendingAndExhaustedTerminalInOneCAS(t *testing.T) {
	ctx := context.Background()
	aggregate, session := preparedOperationsProviderTerminalSession(t)
	repository := &providerTerminalCASRepository{aggregate: aggregate, mode: providerTerminalCASNormal}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}

	firstFraming := providerIncompleteFramingForTest(session, []byte(`{"operations":[`), runtimecontract.RuntimeProviderDocumentCodeIncompleteJSON)
	firstFact := runtimeProviderDocumentRecoveryFactForTest(t, session.DocumentKind, firstFraming)
	firstSettlement := runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, session.LastPhysicalCallID, nil)
	pending, err := controller.coordinateProviderRequestRecovery(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, session.LastPhysicalCallID, firstFact, firstSettlement)
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves != 1 || pending.StateRevision != aggregate.StateRevision+1 || pending.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionRecoveryPending || pending.CandidateRequestSessions[0].DocumentRecoveryOutcomes != 1 || pending.Facts.Failure != nil {
		t.Fatalf("first incomplete recovery was not one pending CAS: saves=%d aggregate=%+v", repository.saves, pending)
	}
	duplicate, err := controller.coordinateProviderRequestRecovery(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, session.LastPhysicalCallID, firstFact, firstSettlement)
	if err != nil || duplicate.StateRevision != pending.StateRevision || repository.saves != 1 {
		t.Fatalf("duplicate incomplete recovery changed state: saves=%d aggregate=%+v err=%v", repository.saves, duplicate, err)
	}

	current := pending.CandidateRequestSessions[0]
	if current.ExpectedDocument == nil || current.DocumentRecoveryOutcomes > current.ExpectedDocument.MaxRecoveryAttempts {
		t.Fatalf("first incomplete unexpectedly exhausted sealed recovery: %+v", current)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(current.LogicalRequestID, current.NextSequence)
	submitted, err := controller.markCandidateRequestSubmitted(ctx, pending.Facts.WorkspaceID, pending.Facts.RunID, current.LogicalRequestID, current.NextSequence, physicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	secondFraming := providerIncompleteFramingForTest(current, []byte(`{"operations":[{"id":`), runtimecontract.RuntimeProviderDocumentCodeOutputLimit)
	secondFact := runtimeProviderDocumentRecoveryFactForTest(t, current.DocumentKind, secondFraming)
	beforeSaves := repository.saves
	secondSettlement := runtimeProviderConfirmedBudgetSettlementForTest(t, current.LogicalRequestID, physicalCallID, nil)
	terminal, err := controller.coordinateProviderRequestRecovery(ctx, submitted.Facts.WorkspaceID, submitted.Facts.RunID, current.LogicalRequestID, physicalCallID, secondFact, secondSettlement)
	if err != nil {
		t.Fatal(err)
	}
	stored := terminal.CandidateRequestSessions[0]
	if repository.saves != beforeSaves+1 || terminal.StateRevision != submitted.StateRevision+1 || stored.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted || stored.DocumentRecoveryOutcomes != 2 || terminal.Facts.Failure == nil || terminal.Facts.Failure.Code != "candidate_document_recovery_exhausted" || len(terminal.CandidateAttempts) != 0 {
		t.Fatalf("exhausted incomplete recovery was not one terminal CAS: saves=%d/%d aggregate=%+v", repository.saves, beforeSaves, terminal)
	}
}

func TestProviderRequestZeroByteOutputLimitUsesOwnerPrivateWholeDocumentRecovery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(*testing.T) (RuntimeAggregate, runtimecontract.RuntimeCandidateRequestSession)
	}{
		{name: "operations", prepare: preparedOperationsProviderTerminalSession},
		{name: "semantic_plan", prepare: preparedSemanticProviderTerminalSession},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			aggregate, session := tt.prepare(t)
			repository := &providerTerminalCASRepository{aggregate: aggregate, mode: providerTerminalCASNormal}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			if session.ExpectedDocument == nil {
				t.Fatal("prepared request session has no expected document")
			}
			expected := session.ExpectedDocument.Clone()
			framing := runtimecontract.RuntimeProviderDocumentFraming{
				Disposition:          runtimecontract.RuntimeProviderDocumentIncomplete,
				ExpectedDocumentHash: expected.Hash,
				Code:                 "max_output_tokens",
				Signature:            "provider-output-limit",
			}
			fact := runtimeProviderDocumentRecoveryFactForTest(t, session.DocumentKind, framing)
			settlement := runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, session.LastPhysicalCallID, nil)
			pending, err := controller.coordinateProviderRequestRecovery(
				ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID,
				session.LogicalRequestID, session.LastPhysicalCallID, fact, settlement,
			)
			if err != nil {
				t.Fatal(err)
			}
			current := pending.CandidateRequestSessions[0]
			if repository.saves != 1 || current.State != runtimecontract.RuntimeCandidateRequestSessionRecoveryPending || current.DocumentRecoveryOutcomes != 1 || current.ProviderSubmitted || current.LogicalRequestID != session.LogicalRequestID {
				t.Fatalf("zero-byte output limit did not enter one owner-private recovery CAS: saves=%d session=%+v", repository.saves, current)
			}
			if current.ExpectedDocument == nil || current.ExpectedDocument.Hash != expected.Hash || runtimecontract.StableRuntimeExpectedDocumentHash(*current.ExpectedDocument) != runtimecontract.StableRuntimeExpectedDocumentHash(expected) {
				t.Fatalf("whole-document recovery changed sealed expected document: before=%+v after=%+v", expected, current.ExpectedDocument)
			}
			if pending.Facts.Failure != nil || len(pending.CandidateAttempts) != len(aggregate.CandidateAttempts) || pending.ActiveDispatch != nil || len(pending.CompletedDispatches) != len(aggregate.CompletedDispatches) {
				t.Fatalf("zero-byte output limit created candidate/dispatch/terminal authority: %+v", pending)
			}

			nextPhysicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(current.LogicalRequestID, current.NextSequence)
			if nextPhysicalCallID == "" || nextPhysicalCallID == session.LastPhysicalCallID {
				t.Fatalf("recovery did not derive a fresh physical identity: old=%q next=%q", session.LastPhysicalCallID, nextPhysicalCallID)
			}
			resubmitted, err := controller.markCandidateRequestSubmitted(
				ctx, pending.Facts.WorkspaceID, pending.Facts.RunID,
				current.LogicalRequestID, current.NextSequence, nextPhysicalCallID,
			)
			if err != nil {
				t.Fatal(err)
			}
			retried := resubmitted.CandidateRequestSessions[0]
			if retried.LastPhysicalCallID != nextPhysicalCallID || !retried.ProviderSubmitted || retried.LogicalRequestID != session.LogicalRequestID || retried.ExpectedDocument == nil || retried.ExpectedDocument.Hash != expected.Hash {
				t.Fatalf("whole-document retry changed logical/expected identity: %+v", retried)
			}
			if len(resubmitted.CandidateAttempts) != len(aggregate.CandidateAttempts) || resubmitted.ActiveDispatch != nil || len(resubmitted.CompletedDispatches) != len(aggregate.CompletedDispatches) {
				t.Fatalf("physical retry authorization created candidate or dispatch: %+v", resubmitted)
			}
		})
	}
}

func TestProviderRequestIncompleteRecoveryCASCutPointsNeverReplayProvider(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []providerTerminalCASMode{providerTerminalCASFailBeforeWrite, providerTerminalCASLoseResponse} {
		t.Run(string(mode), func(t *testing.T) {
			aggregate, session := preparedOperationsProviderTerminalSession(t)
			repository := &providerTerminalCASRepository{aggregate: aggregate, mode: providerTerminalCASNormal}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			firstFraming := providerIncompleteFramingForTest(session, []byte(`{"operations":[`), runtimecontract.RuntimeProviderDocumentCodeIncompleteJSON)
			firstFact := runtimeProviderDocumentRecoveryFactForTest(t, session.DocumentKind, firstFraming)
			firstSettlement := runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, session.LastPhysicalCallID, nil)
			pending, err := controller.coordinateProviderRequestRecovery(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, session.LastPhysicalCallID, firstFact, firstSettlement)
			if err != nil {
				t.Fatal(err)
			}
			current := pending.CandidateRequestSessions[0]
			physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(current.LogicalRequestID, current.NextSequence)
			submitted, err := controller.markCandidateRequestSubmitted(ctx, pending.Facts.WorkspaceID, pending.Facts.RunID, current.LogicalRequestID, current.NextSequence, physicalCallID)
			if err != nil {
				t.Fatal(err)
			}
			framing := providerIncompleteFramingForTest(current, []byte(`{"operations":[{"id":`), runtimecontract.RuntimeProviderDocumentCodeOutputLimit)
			fact := runtimeProviderDocumentRecoveryFactForTest(t, current.DocumentKind, framing)
			settlement := runtimeProviderConfirmedBudgetSettlementForTest(t, current.LogicalRequestID, physicalCallID, nil)
			repository.mode = mode
			repository.fired = false
			beforeSaves := repository.saves
			_, transitionErr := controller.coordinateProviderRequestRecovery(ctx, submitted.Facts.WorkspaceID, submitted.Facts.RunID, current.LogicalRequestID, physicalCallID, fact, settlement)
			if transitionErr == nil {
				t.Fatalf("%s did not surface the injected CAS cut", mode)
			}

			durable := repository.aggregate
			fresh, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case providerTerminalCASFailBeforeWrite:
				if repository.saves != beforeSaves || durable.StateRevision != submitted.StateRevision || !durable.CandidateRequestSessions[0].ProviderSubmitted || durable.Facts.Failure != nil {
					t.Fatalf("CAS-before leaked recovery decision: saves=%d/%d aggregate=%+v", repository.saves, beforeSaves, durable)
				}
				unknown, err := NormalizeRuntimeProviderRequestTerminal(RuntimeProviderRequestTerminalInput{
					DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
					OutcomeClass: runtimecontract.RuntimeCandidateRequestOutcomeProviderSubmissionUnknown,
					TerminalCode: "provider_submission_unknown", TerminalSignature: "provider-submission-unknown",
					Message: "provider request was submitted without a durable outcome",
				})
				if err != nil {
					t.Fatal(err)
				}
				recovered, err := fresh.planning.coordinateProviderRequestTerminal(ctx, durable.Facts.WorkspaceID, durable.Facts.RunID, current.LogicalRequestID, unknown)
				if err != nil || recovered.Facts.Failure == nil || recovered.Facts.Failure.Code != "provider_submission_unknown" {
					t.Fatalf("fresh CAS-before reload did not resolve submitted marker without provider replay: aggregate=%+v err=%v", recovered, err)
				}
			case providerTerminalCASLoseResponse:
				if repository.saves != beforeSaves+1 || durable.CandidateRequestSessions[0].State != runtimecontract.RuntimeCandidateRequestSessionTerminal || durable.Facts.Failure == nil || durable.Facts.Failure.Code != "candidate_document_recovery_exhausted" {
					t.Fatalf("CAS-after lost atomic terminal: saves=%d/%d aggregate=%+v", repository.saves, beforeSaves, durable)
				}
				replayed, err := fresh.coordinateProviderRequestRecovery(ctx, durable.Facts.WorkspaceID, durable.Facts.RunID, current.LogicalRequestID, physicalCallID, fact, settlement)
				if err != nil || replayed.StateRevision != durable.StateRevision || repository.saves != beforeSaves+1 {
					t.Fatalf("fresh CAS-after reload rewrote terminal or required provider replay: saves=%d aggregate=%+v err=%v", repository.saves, replayed, err)
				}
			}
		})
	}
}

func TestProviderRequestBodyIdleTransportBudgetAndExhaustedTerminalAreAtomic(t *testing.T) {
	ctx := context.Background()
	aggregate, session := preparedOperationsProviderTerminalSession(t)
	aggregate.Facts.RecoveryPolicy.MaxProviderTransportAttempts = 1
	if err := aggregate.Validate(); err != nil {
		t.Fatal(err)
	}
	repository := &providerTerminalCASRepository{aggregate: aggregate, mode: providerTerminalCASNormal}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		OriginalClass: string(runtimecontract.RuntimeCandidateFailureClassProviderTransient),
		OriginalCode:  "provider_body_idle_timeout", OriginalMessage: "provider body became idle",
		Signature: "provider-body-idle-timeout/exact",
	}
	outcome := runtimecontract.RuntimeCandidateRequestOutcome{
		Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient,
		Code:  diagnostic.OriginalCode, Signature: diagnostic.Signature,
	}
	fact := runtimeProviderTransportRecoveryFactForTest(t, session.DocumentKind, outcome, diagnostic.OriginalMessage, &diagnostic)
	firstSettlement := runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, session.LastPhysicalCallID, nil)
	pending, err := controller.coordinateProviderRequestRecovery(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, session.LastPhysicalCallID, fact, firstSettlement)
	if err != nil {
		t.Fatal(err)
	}
	stored := pending.CandidateRequestSessions[0]
	wantDueAt := time.Date(2026, 11, 23, 8, 0, 2, 0, time.UTC).Format(time.RFC3339Nano)
	if repository.saves != 1 || stored.State != runtimecontract.RuntimeCandidateRequestSessionTransportPending || stored.TransportRetryDueAt != wantDueAt || stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient || stored.LastOutcome.Code != diagnostic.OriginalCode || stored.LastOutcome.Signature != diagnostic.Signature || pending.Facts.Recovery.ProviderTransportAttempts != 1 || pending.Facts.Failure != nil {
		t.Fatalf("body-idle did not atomically consume one transport allowance: saves=%d aggregate=%+v", repository.saves, pending)
	}
	laterFact := runtimeProviderTransportRecoveryFactAtTimeForTest(t, session.DocumentKind, outcome, diagnostic.OriginalMessage, &diagnostic, "2026-11-23T08:00:05Z")
	duplicate, err := controller.coordinateProviderRequestRecovery(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, session.LastPhysicalCallID, laterFact, firstSettlement)
	if err != nil || repository.saves != 1 || duplicate.StateRevision != pending.StateRevision || duplicate.CandidateRequestSessions[0].TransportRetryDueAt != wantDueAt {
		t.Fatalf("duplicate transport recovery rewrote dueAt or revision: saves=%d aggregate=%+v err=%v", repository.saves, duplicate, err)
	}

	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(stored.LogicalRequestID, stored.NextSequence)
	if _, err := controller.markCandidateRequestSubmitted(ctx, pending.Facts.WorkspaceID, pending.Facts.RunID, stored.LogicalRequestID, stored.NextSequence, physicalCallID, "2026-11-23T08:00:01.999999999Z"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("early transport resubmission err=%v, want conflict", err)
	}
	submitted, err := controller.markCandidateRequestSubmitted(ctx, pending.Facts.WorkspaceID, pending.Facts.RunID, stored.LogicalRequestID, stored.NextSequence, physicalCallID, stored.TransportRetryDueAt)
	if err != nil {
		t.Fatal(err)
	}
	repository.mode = providerTerminalCASLoseResponse
	repository.fired = false
	beforeSaves := repository.saves
	secondSettlement := runtimeProviderConfirmedBudgetSettlementForTest(t, stored.LogicalRequestID, physicalCallID, nil)
	if _, err := controller.coordinateProviderRequestRecovery(ctx, submitted.Facts.WorkspaceID, submitted.Facts.RunID, stored.LogicalRequestID, physicalCallID, fact, secondSettlement); err == nil {
		t.Fatal("transport exhausted CAS response loss was not surfaced")
	}
	durable := repository.aggregate
	terminalSession := durable.CandidateRequestSessions[0]
	if repository.saves != beforeSaves+1 || terminalSession.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || terminalSession.TransportRetryDueAt != "" || terminalSession.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient || terminalSession.LastOutcome.Code != diagnostic.OriginalCode || terminalSession.LastOutcome.Signature != diagnostic.Signature || durable.Facts.Failure == nil || durable.Facts.Failure.Class != runtimecontract.FailureClassProviderTransient || durable.Facts.Failure.Code != "provider_transport_exhausted" || durable.Facts.Failure.OriginalDiagnostic == nil || durable.Facts.Failure.OriginalDiagnostic.OriginalCode != diagnostic.OriginalCode || durable.Facts.Failure.OriginalDiagnostic.Signature != diagnostic.Signature || durable.Facts.Recovery.ProviderTransportAttempts != 1 {
		t.Fatalf("transport exhaustion lost exact transient fact or atomic budget: saves=%d/%d aggregate=%+v", repository.saves, beforeSaves, durable)
	}
	fresh, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := fresh.coordinateProviderRequestRecovery(ctx, durable.Facts.WorkspaceID, durable.Facts.RunID, stored.LogicalRequestID, physicalCallID, fact, secondSettlement)
	if err != nil || replayed.StateRevision != durable.StateRevision || repository.saves != beforeSaves+1 {
		t.Fatalf("fresh transport-terminal reload changed state: saves=%d aggregate=%+v err=%v", repository.saves, replayed, err)
	}
}

func TestProviderRequestSemanticRecoveryCoordinatesTransportAndIncompleteTerminals(t *testing.T) {
	ctx := context.Background()
	for _, recoveryKind := range []string{"transport", "incomplete"} {
		t.Run(recoveryKind, func(t *testing.T) {
			aggregate, session := preparedSemanticProviderTerminalSession(t)
			repository := &providerTerminalCASRepository{aggregate: aggregate, mode: providerTerminalCASNormal}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			var fact RuntimeProviderRequestRecoveryFact
			physicalCallID := session.LastPhysicalCallID
			if recoveryKind == "transport" {
				repository.aggregate.Facts.Recovery.ProviderTransportAttempts = repository.aggregate.Facts.RecoveryPolicy.Normalized().MaxProviderTransportAttempts
				diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
					SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
					OwnerModuleID: runtimecontract.RuntimeModuleLLM,
					OriginalClass: string(runtimecontract.RuntimeCandidateFailureClassProviderTransient),
					OriginalCode:  "provider_body_idle_timeout", OriginalMessage: "semantic provider body became idle",
					Signature: "semantic-provider-body-idle/exact",
				}
				outcome := runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient, Code: diagnostic.OriginalCode, Signature: diagnostic.Signature}
				fact = runtimeProviderTransportRecoveryFactForTest(t, session.DocumentKind, outcome, diagnostic.OriginalMessage, &diagnostic)
			} else {
				firstFraming := providerIncompleteFramingForTest(session, []byte(`{"coverageOrder":[`), runtimecontract.RuntimeProviderDocumentCodeIncompleteJSON)
				firstFact := runtimeProviderDocumentRecoveryFactForTest(t, session.DocumentKind, firstFraming)
				pending, err := controller.coordinateProviderRequestRecovery(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, physicalCallID, firstFact, runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, physicalCallID, nil))
				if err != nil {
					t.Fatal(err)
				}
				current := pending.CandidateRequestSessions[0]
				physicalCallID = runtimecontract.StableRuntimeCandidatePhysicalCallID(current.LogicalRequestID, current.NextSequence)
				if _, err := controller.markCandidateRequestSubmitted(ctx, pending.Facts.WorkspaceID, pending.Facts.RunID, current.LogicalRequestID, current.NextSequence, physicalCallID); err != nil {
					t.Fatal(err)
				}
				framing := providerIncompleteFramingForTest(current, []byte(`{"coverageOrder":["cwi`), runtimecontract.RuntimeProviderDocumentCodeOutputLimit)
				fact = runtimeProviderDocumentRecoveryFactForTest(t, current.DocumentKind, framing)
			}
			before := repository.aggregate.StateRevision
			beforeSaves := repository.saves
			terminal, err := controller.coordinateProviderRequestRecovery(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, physicalCallID, fact, runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, physicalCallID, nil))
			if err != nil {
				t.Fatal(err)
			}
			stored := terminal.CandidateRequestSessions[0]
			if terminal.StateRevision != before+1 || repository.saves != beforeSaves+1 || stored.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || terminal.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed || terminal.Facts.Failure == nil || terminal.Facts.Failure.Class == "" || terminal.Decision.State != runtimecontract.ConvergenceStateFailed || len(terminal.CandidateAttempts) != 0 {
				t.Fatalf("semantic %s recovery was not one terminal CAS: saves=%d/%d aggregate=%+v", recoveryKind, repository.saves, beforeSaves, terminal)
			}
			if recoveryKind == "transport" && (stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient || stored.LastOutcome.Code != "provider_body_idle_timeout" || stored.LastOutcome.Signature != "semantic-provider-body-idle/exact" || terminal.Facts.Failure.OriginalDiagnostic == nil || terminal.Facts.Failure.OriginalDiagnostic.OriginalCode != stored.LastOutcome.Code || terminal.Facts.Failure.OriginalDiagnostic.Signature != stored.LastOutcome.Signature) {
				t.Fatalf("semantic transport terminal lost exact transient identity: session=%+v failure=%+v", stored, terminal.Facts.Failure)
			}
		})
	}
}

func providerIncompleteFramingForTest(session runtimecontract.RuntimeCandidateRequestSession, content []byte, code string) runtimecontract.RuntimeProviderDocumentFraming {
	signature := runtimecontract.RuntimeProviderDocumentSignatureIncompleteJSON
	if code == runtimecontract.RuntimeProviderDocumentCodeOutputLimit {
		signature = runtimecontract.RuntimeProviderDocumentSignatureOutputLimit
	}
	return runtimecontract.RuntimeProviderDocumentFraming{
		Disposition:          runtimecontract.RuntimeProviderDocumentIncomplete,
		ExpectedDocumentHash: session.ExpectedDocument.Hash,
		ResponseHash:         runtimecontract.StableRuntimeProviderResponseHash(content),
		ResponseBytes:        int64(len(content)), Code: code, Signature: signature,
	}
}

func TestProviderRequestRecoveryRejectsStaleAndConflictingFactsWithoutCAS(t *testing.T) {
	ctx := context.Background()
	aggregate, session := preparedOperationsProviderTerminalSession(t)
	repository := &providerTerminalCASRepository{aggregate: aggregate, mode: providerTerminalCASNormal}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	framing := providerIncompleteFramingForTest(session, []byte(`{"operations":[`), runtimecontract.RuntimeProviderDocumentCodeIncompleteJSON)
	fact := runtimeProviderDocumentRecoveryFactForTest(t, session.DocumentKind, framing)
	settlement := runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, session.LastPhysicalCallID, nil)
	if _, err := controller.coordinateProviderRequestRecovery(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "stale-logical-request", session.LastPhysicalCallID, fact, settlement); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("stale recovery err=%v, want not found", err)
	}
	if _, err := controller.coordinateProviderRequestRecovery(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, "stale-physical-call", fact, settlement); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale physical recovery err=%v, want conflict", err)
	}
	semanticFact := runtimeProviderDocumentRecoveryFactForTest(t, runtimecontract.RuntimeProviderDocumentSemanticPlan, framing)
	if _, err := controller.coordinateProviderRequestRecovery(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, session.LastPhysicalCallID, semanticFact, settlement); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("conflicting document-kind recovery err=%v, want conflict", err)
	}
	if repository.saves != 0 {
		t.Fatalf("stale/conflicting recovery changed state: saves=%d", repository.saves)
	}
}

func TestProviderRequestRecoveryRejectsReleasedOrUnknownPhysicalSubmissionWithoutCAS(t *testing.T) {
	ctx := context.Background()
	for _, status := range []RuntimeProviderBudgetSettlementStatus{RuntimeProviderBudgetSettlementReleased, RuntimeProviderBudgetSettlementUnknown} {
		t.Run(string(status), func(t *testing.T) {
			aggregate, session := preparedOperationsProviderTerminalSession(t)
			repository := &providerTerminalCASRepository{aggregate: aggregate, mode: providerTerminalCASNormal}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			framing := providerIncompleteFramingForTest(session, []byte(`{"operations":[`), runtimecontract.RuntimeProviderDocumentCodeIncompleteJSON)
			fact := runtimeProviderDocumentRecoveryFactForTest(t, session.DocumentKind, framing)
			var settlement RuntimeProviderBudgetSettlement
			if status == RuntimeProviderBudgetSettlementUnknown {
				settlement, err = NewRuntimeProviderBudgetUnknownSettlement(session.LogicalRequestID, session.LastPhysicalCallID)
			} else {
				audit := runtimecontract.ProviderRequestAuditView{
					RequestID: session.LastPhysicalCallID, LogicalRequestID: session.LogicalRequestID, PhysicalCallID: session.LastPhysicalCallID,
					WorkspaceID: string(aggregate.Facts.WorkspaceID), RunID: string(aggregate.Facts.RunID),
					Status: "failed", ProviderSubmitted: false, PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming,
					ContextBudget: llmaudit.ContextBudget{ProviderID: "test-provider", ModelID: "test-model"},
				}
				settlement, err = NewRuntimeProviderBudgetSettlementFromAudit(string(aggregate.Facts.WorkspaceID), string(aggregate.Facts.RunID), session.LogicalRequestID, session.LastPhysicalCallID, &audit, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := controller.coordinateProviderRequestRecovery(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, session.LastPhysicalCallID, fact, &settlement); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("%s recovery err=%v, want conflict", status, err)
			}
			if repository.saves != 0 || repository.aggregate.StateRevision != aggregate.StateRevision || repository.aggregate.Facts.Recovery.ProviderTransportAttempts != aggregate.Facts.Recovery.ProviderTransportAttempts || !repository.aggregate.CandidateRequestSessions[0].ProviderSubmitted {
				t.Fatalf("%s recovery changed state: saves=%d aggregate=%+v", status, repository.saves, repository.aggregate)
			}
		})
	}
}

func TestProviderRequestRecoveryEnforcesPerRequestExhaustionAcrossDocumentKinds(t *testing.T) {
	ctx := context.Background()
	wantDeltas := []time.Duration{
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		32 * time.Second,
		60 * time.Second,
		60 * time.Second,
		60 * time.Second,
	}
	for _, kind := range []runtimecontract.RuntimeProviderDocumentKind{
		runtimecontract.RuntimeProviderDocumentOperations,
		runtimecontract.RuntimeProviderDocumentSemanticPlan,
	} {
		t.Run(string(kind), func(t *testing.T) {
			var aggregate RuntimeAggregate
			var session runtimecontract.RuntimeCandidateRequestSession
			if kind == runtimecontract.RuntimeProviderDocumentOperations {
				aggregate, session = preparedOperationsProviderTerminalSession(t)
			} else {
				aggregate, session = preparedSemanticProviderTerminalSession(t)
			}
			if aggregate.Facts.RecoveryPolicy.MaxProviderTransportAttempts != runtimecontract.DefaultMaxProviderTransportAttempts {
				t.Fatalf("default MaxProviderTransportAttempts = %d, want %d", aggregate.Facts.RecoveryPolicy.MaxProviderTransportAttempts, runtimecontract.DefaultMaxProviderTransportAttempts)
			}
			repository := &providerTerminalCASRepository{aggregate: aggregate, mode: providerTerminalCASNormal}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
				SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
				OwnerModuleID: runtimecontract.RuntimeModuleLLM,
				OriginalClass: string(runtimecontract.RuntimeCandidateFailureClassProviderTransient),
				OriginalCode:  "provider_http_status_503", OriginalMessage: "upstream provider temporarily unavailable (HTTP 503)",
				Signature: "provider-http-status-503/exact",
			}
			outcome := runtimecontract.RuntimeCandidateRequestOutcome{
				Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient,
				Code:  diagnostic.OriginalCode, Signature: diagnostic.Signature,
			}

			tNow := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
			currentCallID := session.LastPhysicalCallID
			var current RuntimeAggregate
			for attempt := 1; attempt <= runtimecontract.DefaultMaxProviderTransportAttemptsPerRequest; attempt++ {
				fact := runtimeProviderTransportRecoveryFactAtTimeForTest(t, kind, outcome, diagnostic.OriginalMessage, &diagnostic, tNow.Format(time.RFC3339Nano))
				settlement := runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, currentCallID, nil)
				pending, recErr := controller.coordinateProviderRequestRecovery(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, currentCallID, fact, settlement)
				if recErr != nil {
					t.Fatalf("attempt %d recovery failed: %v", attempt, recErr)
				}
				stored := pending.CandidateRequestSessions[0]
				if stored.State != runtimecontract.RuntimeCandidateRequestSessionTransportPending ||
					stored.TransportOutcomes != attempt ||
					pending.Facts.Recovery.ProviderTransportAttempts != attempt ||
					pending.Facts.Failure != nil {
					t.Fatalf("attempt %d did not persist pending transport state: session=%+v recovery=%+v", attempt, stored, pending.Facts.Recovery)
				}
				dueAt, parseErr := time.Parse(time.RFC3339Nano, stored.TransportRetryDueAt)
				if parseErr != nil {
					t.Fatalf("attempt %d invalid TransportRetryDueAt %q: %v", attempt, stored.TransportRetryDueAt, parseErr)
				}
				if gotDelta := dueAt.Sub(tNow); gotDelta != wantDeltas[attempt-1] {
					t.Fatalf("attempt %d backoff delta = %v, want %v", attempt, gotDelta, wantDeltas[attempt-1])
				}

				nextCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(stored.LogicalRequestID, stored.NextSequence)
				earlyAt := dueAt.Add(-time.Nanosecond).Format(time.RFC3339Nano)
				beforeSaves := repository.saves
				if _, earlyErr := controller.markCandidateRequestSubmitted(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, stored.LogicalRequestID, stored.NextSequence, nextCallID, earlyAt); !errors.Is(earlyErr, common.ErrConflict) {
					t.Fatalf("attempt %d early submit err = %v, want conflict", attempt, earlyErr)
				}
				if repository.saves != beforeSaves {
					t.Fatalf("attempt %d early submit mutated repository: saves=%d/%d", attempt, repository.saves, beforeSaves)
				}
				tNow = dueAt
				current, err = controller.markCandidateRequestSubmitted(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, stored.LogicalRequestID, stored.NextSequence, nextCallID, tNow.Format(time.RFC3339Nano))
				if err != nil {
					t.Fatalf("attempt %d submit at dueAt failed: %v", attempt, err)
				}
				currentCallID = nextCallID
			}
			_ = current

			ninthFact := runtimeProviderTransportRecoveryFactAtTimeForTest(t, kind, outcome, diagnostic.OriginalMessage, &diagnostic, tNow.Format(time.RFC3339Nano))
			ninthSettlement := runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, currentCallID, nil)
			terminal, err := controller.coordinateProviderRequestRecovery(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, currentCallID, ninthFact, ninthSettlement)
			if err != nil {
				t.Fatalf("9th recovery failed: %v", err)
			}
			if err := terminal.Validate(); err != nil {
				t.Fatalf("per-request terminal aggregate failed Validate: %v", err)
			}
			stored := terminal.CandidateRequestSessions[0]
			if stored.State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
				stored.TransportRetryDueAt != "" ||
				stored.TransportOutcomes != 9 ||
				terminal.Facts.Recovery.ProviderTransportAttempts != runtimecontract.DefaultMaxProviderTransportAttemptsPerRequest ||
				terminal.Decision.State != runtimecontract.ConvergenceStateFailed ||
				terminal.Facts.Failure == nil ||
				terminal.Facts.Failure.Class != runtimecontract.FailureClassProviderTransient ||
				terminal.Facts.Failure.OriginalDiagnostic == nil ||
				terminal.Facts.Failure.OriginalDiagnostic.OriginalCode != diagnostic.OriginalCode ||
				terminal.Facts.Failure.OriginalDiagnostic.Signature != diagnostic.Signature {
				t.Fatalf("per-request exhaustion state mismatch: session=%+v recovery=%+v decision=%+v failure=%+v", stored, terminal.Facts.Recovery, terminal.Decision, terminal.Facts.Failure)
			}
			if kind == runtimecontract.RuntimeProviderDocumentOperations {
				if terminal.Facts.Failure.Code != "provider_transport_exhausted" {
					t.Fatalf("operations exhaustion code = %q, want provider_transport_exhausted", terminal.Facts.Failure.Code)
				}
			} else {
				if terminal.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed ||
					terminal.Facts.Failure.Code != "semantic_contract_failed" ||
					terminal.SemanticPlanning.Failure == nil {
					t.Fatalf("semantic exhaustion mismatch: semantic=%+v failure=%+v", terminal.SemanticPlanning, terminal.Facts.Failure)
				}
			}

			beforeReplayRev := terminal.StateRevision
			beforeReplaySaves := repository.saves
			laterFact := runtimeProviderTransportRecoveryFactAtTimeForTest(t, kind, outcome, diagnostic.OriginalMessage, &diagnostic, tNow.Add(30*time.Second).Format(time.RFC3339Nano))
			replayed, err := controller.coordinateProviderRequestRecovery(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, currentCallID, laterFact, ninthSettlement)
			if err != nil || replayed.StateRevision != beforeReplayRev || repository.saves != beforeReplaySaves {
				t.Fatalf("duplicate 9th recovery mutated state: saves=%d/%d rev=%d/%d err=%v", repository.saves, beforeReplaySaves, replayed.StateRevision, beforeReplayRev, err)
			}
		})
	}
}

func TestProviderRequestTransportPendingStalePrerequisiteClearsDueAt(t *testing.T) {
	ctx := context.Background()
	aggregate := stateChangeAggregate(t)
	prerequisite, observation := addFreshFileAdmissibilityObservation(t, &aggregate)
	repository := &controllerRepository{aggregate: &aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, session, err := controller.ensureCandidateRequestSession(ctx, "ws-1", "run-1", runtimecontract.RuntimeProviderDocumentOperations, "stale-transport-prefix", "manifest-v1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.markCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 1, "physical-1"); err != nil {
		t.Fatal(err)
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		OriginalClass: string(runtimecontract.RuntimeCandidateFailureClassProviderTransient),
		OriginalCode:  "provider_timeout", OriginalMessage: "provider timed out",
		Signature: "provider-timeout",
	}
	outcome := runtimecontract.RuntimeCandidateRequestOutcome{
		Class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient,
		Code:  diagnostic.OriginalCode, Signature: diagnostic.Signature,
	}
	recovery := runtimeProviderTransportRecoveryFactForTest(t, runtimecontract.RuntimeProviderDocumentOperations, outcome, diagnostic.OriginalMessage, &diagnostic)
	pending, err := controller.coordinateProviderRequestRecovery(ctx, "ws-1", "run-1", session.LogicalRequestID, "physical-1", recovery, runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, "physical-1", nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.markCandidateRequestSubmitted(ctx, "ws-1", "run-1", session.LogicalRequestID, 2, "physical-2", pending.CandidateRequestSessions[0].TransportRetryDueAt); err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", ExpectedRevision: "tree-v1", ObservedRevision: "tree-v2", Reason: "workspace tree changed",
	}
	applied, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind:  RuntimeInternalEventCandidateRequestStaleObserved,
		Scope: RuntimeInternalEventScope{WorkspaceID: "ws-1", RunID: "run-1"},
		Transaction: &RuntimeInternalTransactionPayload{
			LogicalRequestID:         session.LogicalRequestID,
			Stale:                    &stale,
			ProviderBudgetSettlement: runtimeProviderConfirmedBudgetSettlementForTest(t, session.LogicalRequestID, "physical-2", nil),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := applied.Aggregate.Validate(); err != nil {
		t.Fatalf("stale terminal aggregate failed Validate: %v", err)
	}
	stored := applied.Aggregate.CandidateRequestSessions[0]
	if stored.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || stored.TransportRetryDueAt != "" || stored.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomePrerequisiteStale || stored.ProviderSubmitted {
		t.Fatalf("stale invalidation did not seal terminal or clear TransportRetryDueAt: %+v", stored)
	}
}
