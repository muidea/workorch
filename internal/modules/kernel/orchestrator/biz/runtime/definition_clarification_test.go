package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeDefinitionGapObservationForAggregate(t *testing.T, repository *controllerRepository, aggregate RuntimeAggregate) runtimecontract.RuntimeDefinitionGapObservation {
	t.Helper()
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion: runtimecontract.RuntimeExpectedDocumentSchemaCurrent,
		DocumentKind:  runtimecontract.RuntimeProviderDocumentSemanticPlan,
		PlanStep:      runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		ManifestHash:  "semantic-manifest-gap", DefinitionSeedHash: aggregate.Facts.DefinitionSeed.Hash,
		LogicalRequestID: "semantic-request-gap", StablePrefixHash: "semantic-prefix-gap",
		MaxOutputTokens: 4096, MaxRecoveryAttempts: 1,
	}
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	repository.aggregate.CandidateRequestSessions = append(repository.aggregate.CandidateRequestSessions, runtimecontract.RuntimeCandidateRequestSession{
		LogicalRequestID: expected.LogicalRequestID, DocumentKind: expected.DocumentKind, ExpectedDocument: &expected,
		Transaction: aggregate.Decision.AuthorityTransaction(), StablePrefixHash: expected.StablePrefixHash, ManifestHash: expected.ManifestHash,
		State: runtimecontract.RuntimeCandidateRequestSessionDocumentComplete, NextSequence: 1,
		LastPhysicalCallID: "semantic-call-gap", ProviderSubmitted: true,
		LastOutcome: runtimecontract.RuntimeCandidateRequestOutcome{Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete},
	})
	diagnostic := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM, OriginalClass: runtimecontract.RuntimeDefinitionGapResultKind,
		OriginalCode: string(runtimecontract.RuntimeDefinitionGapImplicitOneOf), OriginalMessage: "sealed requirement implies alternatives",
		Signature: "runtime-definition-gap-document-fixture", TargetRefs: []string{"cwi-v2"},
	}
	observation := runtimecontract.RuntimeDefinitionGapObservation{
		SchemaVersion: runtimecontract.RuntimeDefinitionClarificationSchemaCurrent,
		WorkspaceID:   aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		DefinitionRevision: aggregate.Definition.Revision, StartupInputHash: aggregate.Definition.StartupInputHash,
		AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, FocusRef: aggregate.Facts.Focus.ID,
		LogicalRequestID: expected.LogicalRequestID, SemanticDocumentHash: "semantic-document-gap",
		GapKind: runtimecontract.RuntimeDefinitionGapImplicitOneOf, OffendingItemRefs: []string{"cwi-v2"}, OriginalDiagnostic: diagnostic,
	}
	observation.ObservationID = runtimecontract.StableRuntimeDefinitionGapObservationID(observation)
	if err := observation.Validate(); err != nil {
		t.Fatalf("Definition gap fixture: %v", err)
	}
	return observation
}

func TestRuntimeDefinitionClarificationUsesDedicatedSM2SM3Transaction(t *testing.T) {
	ctx := context.Background()
	controller, repository, aggregate := newSemanticPlanningController(t)
	observation := runtimeDefinitionGapObservationForAggregate(t, repository, aggregate)

	observed, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind:      RuntimeInternalEventDefinitionGapObserved,
		Scope:     RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Authority: &RuntimeInternalAuthorityPayload{DefinitionGap: &observation},
	})
	if err != nil {
		t.Fatal(err)
	}
	if observed.Aggregate.DefinitionClarification == nil || observed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedGovernance ||
		observed.Aggregate.ActiveDispatch != nil || len(observed.Aggregate.GovernanceRequests) != 0 || len(observed.Aggregate.CandidateAttempts) != 0 {
		t.Fatalf("gap observation escaped dedicated authority path: %+v", observed.Aggregate)
	}
	afterObservationRevision := observed.Aggregate.StateRevision
	duplicate, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind:      RuntimeInternalEventDefinitionGapObserved,
		Scope:     RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Authority: &RuntimeInternalAuthorityPayload{DefinitionGap: &observation},
	})
	if err != nil || duplicate.Aggregate.StateRevision != afterObservationRevision {
		t.Fatalf("duplicate gap observation = revision %d err=%v", duplicate.Aggregate.StateRevision, err)
	}

	sealed, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind:        RuntimeInternalEventDefinitionClarificationDispatchSealed,
		Scope:       RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Transaction: &RuntimeInternalTransactionPayload{},
	})
	if err != nil || sealed.Aggregate.DefinitionClarification.Dispatch == nil {
		t.Fatalf("seal clarification dispatch: %+v err=%v", sealed.Aggregate.DefinitionClarification, err)
	}
	dispatch := sealed.Aggregate.DefinitionClarification.Dispatch.Clone()
	request := runtimecontract.RuntimeDefinitionClarificationRequestRecord{
		DispatchID: dispatch.DispatchID, TicketID: "ticket-gap", RequestBindingHash: "request-binding-gap",
	}
	bound, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind:        RuntimeInternalEventDefinitionClarificationRequestObserved,
		Scope:       RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Transaction: &RuntimeInternalTransactionPayload{DefinitionClarificationRequest: &request},
	})
	if err != nil || bound.Aggregate.DefinitionClarification.Request == nil || len(bound.Aggregate.GovernanceRequests) != 0 || bound.Aggregate.ActiveDispatch != nil {
		t.Fatalf("bind clarification request: %+v err=%v", bound.Aggregate, err)
	}
	boundRevision := bound.Aggregate.StateRevision
	duplicateBound, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind:        RuntimeInternalEventDefinitionClarificationRequestObserved,
		Scope:       RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Transaction: &RuntimeInternalTransactionPayload{DefinitionClarificationRequest: &request},
	})
	if err != nil || duplicateBound.Aggregate.StateRevision != boundRevision {
		t.Fatalf("duplicate clarification result = revision %d err=%v", duplicateBound.Aggregate.StateRevision, err)
	}

	conflict := observation.Clone()
	conflict.SemanticDocumentHash = "semantic-document-other"
	conflict.ObservationID = runtimecontract.StableRuntimeDefinitionGapObservationID(conflict)
	_, err = controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind:      RuntimeInternalEventDefinitionGapObserved,
		Scope:     RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Authority: &RuntimeInternalAuthorityPayload{DefinitionGap: &conflict},
	})
	if !errors.Is(err, common.ErrConflict) || repository.aggregate.StateRevision != boundRevision {
		t.Fatalf("conflicting gap mutated state: revision=%d err=%v", repository.aggregate.StateRevision, err)
	}
}
