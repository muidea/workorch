package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type providerTerminalCASMode string

const (
	providerTerminalCASNormal          providerTerminalCASMode = "normal"
	providerTerminalCASFailBeforeWrite providerTerminalCASMode = "fail_before_write"
	providerTerminalCASLoseResponse    providerTerminalCASMode = "lose_response_after_write"
)

// providerTerminalCASRepository models the two persistence boundaries around
// the coordinated SM-3/SM-2 CAS. Load and Save clone every field mutated by
// the transition so a failed Save cannot leak through a test slice alias.
type providerTerminalCASRepository struct {
	aggregate RuntimeAggregate
	mode      providerTerminalCASMode
	fired     bool
	saves     int
}

func (r *providerTerminalCASRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if r == nil || r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return RuntimeAggregate{}, common.ErrNotFound
	}
	return cloneProviderTerminalAggregate(r.aggregate), nil
}

func (r *providerTerminalCASRepository) Save(_ context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	if r == nil || r.aggregate.StateRevision != expectedRevision {
		return common.ErrConflict
	}
	if !r.fired && r.mode == providerTerminalCASFailBeforeWrite {
		r.fired = true
		return common.ErrConflict
	}
	r.aggregate = cloneProviderTerminalAggregate(aggregate)
	r.saves++
	if !r.fired && r.mode == providerTerminalCASLoseResponse {
		r.fired = true
		return errors.New("provider terminal CAS response lost after durable write")
	}
	return nil
}

func cloneProviderTerminalAggregate(value RuntimeAggregate) RuntimeAggregate {
	cloned := value
	cloned.Facts = value.Facts.Normalized()
	cloned.SemanticPlanning = value.SemanticPlanning.Clone()
	cloned.ExecutionBudget = value.ExecutionBudget.Clone()
	cloned.CandidateRequestSessions = make([]runtimecontract.RuntimeCandidateRequestSession, len(value.CandidateRequestSessions))
	for idx := range value.CandidateRequestSessions {
		cloned.CandidateRequestSessions[idx] = value.CandidateRequestSessions[idx].Clone()
	}
	return cloned
}

func TestNormalizeRuntimeProviderRequestTerminalMatrixPreservesProducerDiagnostic(t *testing.T) {
	classes := []struct {
		name         string
		class        runtimecontract.RuntimeCandidateRequestOutcomeClass
		terminalCode string
		failureClass runtimecontract.FailureClass
	}{
		{name: "authentication", class: runtimecontract.RuntimeCandidateRequestOutcomeProviderAuthentication, terminalCode: "provider_authentication", failureClass: runtimecontract.FailureClassProviderAuthentication},
		{name: "configuration", class: runtimecontract.RuntimeCandidateRequestOutcomeProviderConfiguration, terminalCode: "provider_configuration", failureClass: runtimecontract.FailureClassProviderConfiguration},
		{name: "transient", class: runtimecontract.RuntimeCandidateRequestOutcomeProviderTransient, terminalCode: "provider_transport_exhausted", failureClass: runtimecontract.FailureClassProviderTransient},
		{name: "protocol", class: runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol, terminalCode: "provider_protocol_failure", failureClass: runtimecontract.FailureClassProviderProtocol},
		{name: "submission_unknown", class: runtimecontract.RuntimeCandidateRequestOutcomeProviderSubmissionUnknown, terminalCode: "provider_submission_unknown", failureClass: runtimecontract.FailureClassProviderSubmissionUnknown},
		{name: "document_recovery_exhausted", class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentRecoveryExhausted, terminalCode: "candidate_document_recovery_exhausted", failureClass: runtimecontract.FailureClassCandidateDocumentRecoveryExhausted},
	}
	for _, kind := range []runtimecontract.RuntimeProviderDocumentKind{
		runtimecontract.RuntimeProviderDocumentOperations,
		runtimecontract.RuntimeProviderDocumentSemanticPlan,
	} {
		for _, tc := range classes {
			t.Run(string(kind)+"/"+tc.name, func(t *testing.T) {
				diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
					SchemaVersion:   runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
					OwnerModuleID:   runtimecontract.RuntimeModuleLLM,
					OriginalClass:   providerRequestTerminalOriginalClass(tc.class),
					OriginalCode:    "producer_" + tc.name,
					OriginalMessage: "producer exact " + tc.name,
					Signature:       "producer-exact-" + tc.name,
				}
				fact, err := NormalizeRuntimeProviderRequestTerminal(RuntimeProviderRequestTerminalInput{
					DocumentKind: kind, OutcomeClass: tc.class,
					TerminalCode: tc.terminalCode, TerminalSignature: "terminal-" + tc.name,
					Message: "stable " + tc.name, OriginalDiagnostic: &diagnostic,
				})
				if err != nil {
					t.Fatal(err)
				}
				if fact.RequestOutcome.Class != tc.class || fact.RequestOutcome.Code != diagnostic.OriginalCode || fact.RequestOutcome.Signature != diagnostic.Signature {
					t.Fatalf("request outcome rewrote producer identity: %+v", fact.RequestOutcome)
				}
				if fact.Failure.OriginalDiagnostic == nil || !reflect.DeepEqual(fact.Failure.OriginalDiagnostic.Normalized(), diagnostic.Normalized()) {
					t.Fatalf("failure rewrote producer diagnostic: %+v", fact.Failure)
				}
				if kind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
					if fact.Failure.Code != "semantic_contract_failed" {
						t.Fatalf("semantic failure = %+v", fact.Failure)
					}
				} else if fact.Failure.Class != tc.failureClass || fact.Failure.Code != tc.terminalCode {
					t.Fatalf("operations failure = %+v", fact.Failure)
				}
			})
		}
	}
}

func TestProviderRequestTerminalCoordinatesOperationsSM3AndSM2InOneCAS(t *testing.T) {
	ctx := context.Background()
	aggregate, session := preparedOperationsProviderTerminalSession(t)
	repository := &providerTerminalCASRepository{aggregate: aggregate, mode: providerTerminalCASNormal}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		OriginalClass: string(runtimecontract.RuntimeCandidateFailureClassProviderProtocol),
		OriginalCode:  "response_byte_limit_exceeded", OriginalMessage: "provider response exceeded its byte contract",
		Signature: "response-byte-limit-exceeded",
	}
	fact, err := NormalizeRuntimeProviderRequestTerminal(RuntimeProviderRequestTerminalInput{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		OutcomeClass: runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol,
		TerminalCode: "provider_protocol_failure", TerminalSignature: "provider-protocol-failure",
		Message: "provider response violated the protocol contract", OriginalDiagnostic: &diagnostic,
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision := aggregate.StateRevision
	terminal, err := controller.planning.coordinateProviderRequestTerminal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, fact)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.StateRevision != beforeRevision+1 || repository.saves != 1 || !ProviderRequestTerminalSessionApplied(terminal, terminal.CandidateRequestSessions[0]) {
		t.Fatalf("coordinated terminal did not commit one complete revision: saves=%d aggregate=%+v", repository.saves, terminal)
	}
	if terminal.CandidateRequestSessions[0].ProviderSubmitted || !sameRuntimeFailureFactValue(terminal.Facts.Failure, fact.Failure) || len(terminal.CandidateAttempts) != 0 || terminal.ActiveDispatch != nil {
		t.Fatalf("coordinated terminal leaked or lost authority: %+v", terminal)
	}

	duplicate, err := controller.planning.coordinateProviderRequestTerminal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, fact)
	if err != nil || duplicate.StateRevision != terminal.StateRevision || repository.saves != 1 {
		t.Fatalf("duplicate terminal changed state: saves=%d aggregate=%+v err=%v", repository.saves, duplicate, err)
	}
	if _, err := controller.planning.coordinateProviderRequestTerminal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, "stale-logical-request", fact); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("stale terminal request err = %v, want not found", err)
	}
	staleKind, err := NormalizeRuntimeProviderRequestTerminal(RuntimeProviderRequestTerminalInput{
		DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan,
		OutcomeClass: runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol,
		TerminalCode: "semantic_provider_contract_error", TerminalSignature: "semantic-provider-contract-error",
		Message: "semantic provider terminal belongs to another request kind", OriginalDiagnostic: &diagnostic,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.planning.coordinateProviderRequestTerminal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, staleKind); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("stale document-kind terminal err = %v, want conflict", err)
	}
	if repository.saves != 1 {
		t.Fatalf("stale facts changed state: saves=%d", repository.saves)
	}
}

func TestProviderRequestTerminalCASBeforeWriteReloadClassifiesUnknownWithoutProviderReplay(t *testing.T) {
	ctx := context.Background()
	aggregate, session := preparedOperationsProviderTerminalSession(t)
	repository := &providerTerminalCASRepository{aggregate: aggregate, mode: providerTerminalCASFailBeforeWrite}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := NormalizeRuntimeProviderRequestTerminal(RuntimeProviderRequestTerminalInput{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		OutcomeClass: runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol,
		TerminalCode: "provider_protocol_failure", TerminalSignature: "provider-protocol-failure",
		Message: "provider returned an invalid terminal envelope",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.planning.coordinateProviderRequestTerminal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, observed); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("CAS-before-write err = %v, want conflict", err)
	}
	interrupted := repository.aggregate
	if interrupted.StateRevision != aggregate.StateRevision || interrupted.Facts.Failure != nil || interrupted.CandidateRequestSessions[0].State == runtimecontract.RuntimeCandidateRequestSessionTerminal || !interrupted.CandidateRequestSessions[0].ProviderSubmitted {
		t.Fatalf("CAS-before-write leaked a partial terminal: %+v", interrupted)
	}

	// A fresh controller sees only the durable submitted marker. It records the
	// deterministic submission-unknown successor and has no provider API to
	// invoke, proving that recovery cannot repeat the physical call.
	fresh, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
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
	recovered, err := fresh.planning.coordinateProviderRequestTerminal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, unknown)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.StateRevision != aggregate.StateRevision+1 || repository.saves != 1 || recovered.Facts.Failure == nil || recovered.Facts.Failure.Code != "provider_submission_unknown" || !ProviderRequestTerminalSessionApplied(recovered, recovered.CandidateRequestSessions[0]) {
		t.Fatalf("fresh reload did not seal deterministic unknown outcome: saves=%d aggregate=%+v", repository.saves, recovered)
	}
}

func TestProviderRequestTerminalCASAfterWriteReloadReplaysExactFailure(t *testing.T) {
	ctx := context.Background()
	aggregate, session := preparedOperationsProviderTerminalSession(t)
	repository := &providerTerminalCASRepository{aggregate: aggregate, mode: providerTerminalCASLoseResponse}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		OriginalClass: string(runtimecontract.RuntimeCandidateFailureClassProviderProtocol),
		OriginalCode:  "provider_frame_invalid", OriginalMessage: "provider frame was invalid",
		Signature: "provider-frame-invalid/exact",
	}
	fact, err := NormalizeRuntimeProviderRequestTerminal(RuntimeProviderRequestTerminalInput{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		OutcomeClass: runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol,
		TerminalCode: "provider_protocol_failure", TerminalSignature: "provider-protocol-failure",
		Message: "provider terminal framing failed", OriginalDiagnostic: &diagnostic,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.planning.coordinateProviderRequestTerminal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, fact); err == nil {
		t.Fatal("CAS-after-write response loss was not observed")
	}
	durable := repository.aggregate
	if durable.StateRevision != aggregate.StateRevision+1 || repository.saves != 1 || !sameRuntimeFailureFactValue(durable.Facts.Failure, fact.Failure) || !ProviderRequestTerminalSessionApplied(durable, durable.CandidateRequestSessions[0]) {
		t.Fatalf("CAS-after-write did not persist the complete fact: saves=%d aggregate=%+v", repository.saves, durable)
	}
	fresh, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := fresh.planning.coordinateProviderRequestTerminal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, fact)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.StateRevision != durable.StateRevision || repository.saves != 1 || replayed.Facts.Failure.OriginalDiagnostic.OriginalCode != diagnostic.OriginalCode || replayed.Facts.Failure.OriginalDiagnostic.Signature != diagnostic.Signature {
		t.Fatalf("fresh replay changed exact terminal failure: saves=%d aggregate=%+v", repository.saves, replayed)
	}
}

func TestProviderRequestTerminalCoordinatesSemanticSM3AndSM2InOneCAS(t *testing.T) {
	ctx := context.Background()
	aggregate, session := preparedSemanticProviderTerminalSession(t)
	repository := &providerTerminalCASRepository{aggregate: aggregate, mode: providerTerminalCASNormal}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		OriginalClass: string(runtimecontract.RuntimeCandidateFailureClassProviderProtocol),
		OriginalCode:  "semantic_provider_envelope_invalid", OriginalMessage: "semantic provider envelope invalid",
		Signature: "semantic-provider-envelope-invalid/exact",
	}
	fact, err := NormalizeRuntimeProviderRequestTerminal(RuntimeProviderRequestTerminalInput{
		DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan,
		OutcomeClass: runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol,
		TerminalCode: "semantic_provider_contract_error", TerminalSignature: "semantic-provider-contract-error",
		Message: "semantic provider contract failed", OriginalDiagnostic: &diagnostic,
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision := aggregate.StateRevision
	terminal, err := controller.planning.coordinateProviderRequestTerminal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, session.LogicalRequestID, fact)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.StateRevision != beforeRevision+1 || repository.saves != 1 || terminal.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningFailed || terminal.Decision.State != runtimecontract.ConvergenceStateFailed || !ProviderRequestTerminalSessionApplied(terminal, terminal.CandidateRequestSessions[0]) {
		t.Fatalf("semantic coordinated terminal = %+v saves=%d", terminal, repository.saves)
	}
	if terminal.SemanticPlanning.Failure == nil || terminal.SemanticPlanning.Failure.OriginalCode != diagnostic.OriginalCode || terminal.SemanticPlanning.Failure.Signature != diagnostic.Signature || !sameRuntimeFailureFactValue(terminal.Facts.Failure, fact.Failure) {
		t.Fatalf("semantic exact diagnostic was not replayable: %+v", terminal.SemanticPlanning)
	}
}

func preparedOperationsProviderTerminalSession(t *testing.T) (RuntimeAggregate, runtimecontract.RuntimeCandidateRequestSession) {
	t.Helper()
	ctx := context.Background()
	base := stateChangeAggregate(t)
	repository := &controllerRepository{aggregate: &base}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	prepared, session, err := controller.ensureCandidateRequestSession(ctx, base.Facts.WorkspaceID, base.Facts.RunID, runtimecontract.RuntimeProviderDocumentOperations, "provider-terminal-prefix", "provider-terminal-manifest", "")
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	submitted, err := controller.markCandidateRequestSubmitted(ctx, prepared.Facts.WorkspaceID, prepared.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	return cloneProviderTerminalAggregate(submitted), submitted.CandidateRequestSessions[0].Clone()
}

func preparedSemanticProviderTerminalSession(t *testing.T) (RuntimeAggregate, runtimecontract.RuntimeCandidateRequestSession) {
	t.Helper()
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	catalog, prerequisite, observation := semanticPlanningCatalogFixture()
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	aggregate, err := controller.initialize(ctx, facts, controllerDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest, _, _ := semanticPlanningFixture(t, aggregate)
	expected := semanticCoverageOrderExpectedDocument(aggregate, manifest.Hash, "semantic-provider-terminal-prefix")
	aggregate, session, err := controller.ensureCandidateRequestSessionWithExpected(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimecontract.RuntimeProviderDocumentSemanticPlan, expected.StablePrefixHash, manifest.Hash, "", &expected)
	if err != nil {
		t.Fatal(err)
	}
	started, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind:        RuntimeInternalEventSemanticPlanRequestStarted,
		Scope:       RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Transaction: &RuntimeInternalTransactionPayload{SemanticManifest: &manifest, StablePrefixHash: expected.StablePrefixHash, LogicalRequestID: session.LogicalRequestID},
	})
	if err != nil {
		t.Fatal(err)
	}
	physicalCallID := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
	submitted, err := controller.markCandidateRequestSubmitted(ctx, started.Aggregate.Facts.WorkspaceID, started.Aggregate.Facts.RunID, session.LogicalRequestID, session.NextSequence, physicalCallID)
	if err != nil {
		t.Fatal(err)
	}
	return cloneProviderTerminalAggregate(submitted), submitted.CandidateRequestSessions[0].Clone()
}
