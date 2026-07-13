package runtimev2

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeControllerRequiresGovernanceTicketBinding(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	sealed := sealGovernanceDispatch(t, controller)
	repository.aggregate.GovernanceRequests = nil
	_, err := controller.CommitGovernanceResolution(context.Background(), governanceResolution(sealed, runtimecontract.RuntimeGovernanceResolutionOutcomeContinue))
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unbound governance resolution err = %v, want conflict", err)
	}
}

func TestRuntimeControllerGovernanceRequestBindingIsIdempotent(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	sealed := sealGovernanceDispatch(t, controller)
	request := sealed.GovernanceRequests[0]
	saves := repository.saves
	replayed, err := controller.BindGovernanceRequest(context.Background(), sealed.Facts.WorkspaceID, sealed.Facts.RunID, request)
	if err != nil || replayed.StateRevision != sealed.StateRevision || repository.saves != saves {
		t.Fatalf("binding replay = %+v saves=%d err=%v", replayed, repository.saves, err)
	}
	request.TicketID = "other-ticket"
	if _, err := controller.BindGovernanceRequest(context.Background(), sealed.Facts.WorkspaceID, sealed.Facts.RunID, request); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("conflicting binding err = %v, want conflict", err)
	}
}

func TestRuntimeControllerCommitsGovernanceContinueAndResumesReducer(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	sealed := sealGovernanceDispatch(t, controller)
	resolution := governanceResolution(sealed, runtimecontract.RuntimeGovernanceResolutionOutcomeContinue)
	result, err := controller.CommitGovernanceResolution(context.Background(), resolution)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != CommitDispositionCommitted || !result.DispatchCompleted || result.Aggregate.ActiveDispatch != nil {
		t.Fatalf("governance commit = %+v", result)
	}
	if result.Aggregate.Decision.Action != runtimecontract.RuntimeActionProduceStateChange || result.Aggregate.Facts.Failure != nil {
		t.Fatalf("governance continuation decision = %+v facts=%+v", result.Aggregate.Decision, result.Aggregate.Facts)
	}
	if len(result.Aggregate.Facts.Evidence.Accepted) != 1 || result.Aggregate.Facts.Evidence.Accepted[0].Kind != runtimecontract.EvidenceKindGovernance || len(result.Aggregate.GovernanceResolutions) != 1 {
		t.Fatalf("governance evidence = %+v records=%+v", result.Aggregate.Facts.Evidence, result.Aggregate.GovernanceResolutions)
	}

	restarted, _ := NewRuntimeController(repository)
	duplicate, err := restarted.CommitGovernanceResolution(context.Background(), resolution)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != result.Aggregate.StateRevision {
		t.Fatalf("duplicate governance resolution = %+v err=%v", duplicate, err)
	}
}

func TestRuntimeControllerCommitsGovernanceFailureAsTerminal(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	sealed := sealGovernanceDispatch(t, controller)
	result, err := controller.CommitGovernanceResolution(context.Background(), governanceResolution(sealed, runtimecontract.RuntimeGovernanceResolutionOutcomeFail))
	if err != nil {
		t.Fatal(err)
	}
	if result.Aggregate.Decision.Action != runtimecontract.RuntimeActionFail || result.Aggregate.Decision.Terminal == nil || result.Aggregate.Decision.Terminal.Code != "governance_rejected" {
		t.Fatalf("governance rejection decision = %+v", result.Aggregate.Decision)
	}
	if len(result.Aggregate.Facts.Evidence.Rejected) != 1 || result.Aggregate.Facts.Evidence.Rejected[0].FailureClass != runtimecontract.FailureClassGovernanceRejected {
		t.Fatalf("governance rejection evidence = %+v", result.Aggregate.Facts.Evidence.Rejected)
	}
}

func TestRuntimeControllerKeepsStaleGovernanceResolutionDiagnosticOnly(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	sealed := sealGovernanceDispatch(t, controller)
	resolution := governanceResolution(sealed, runtimecontract.RuntimeGovernanceResolutionOutcomeContinue)
	resolution.AuthoritySnapshotID = "stale-snapshot"
	result, err := controller.CommitGovernanceResolution(context.Background(), resolution)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != CommitDispositionStale || len(result.Aggregate.Diagnostics) != 1 || result.Aggregate.ActiveDispatch == nil || len(result.Aggregate.GovernanceResolutions) != 0 {
		t.Fatalf("stale governance resolution changed authority: %+v", result.Aggregate)
	}
}

func sealGovernanceDispatch(t *testing.T, controller *RuntimeController) RuntimeAggregate {
	t.Helper()
	facts := controllerFacts()
	facts.Capabilities.GovernanceKinds = []string{"decision/resolution"}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceDependency, Signature: "external-approval", Message: "需要外部裁决",
	}
	definition := controllerDefinition()
	definition.CapabilityContract = append(definition.CapabilityContract, "decision/resolution")
	if _, err := controller.Initialize(context.Background(), facts, definition, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttempt(context.Background(), facts.WorkspaceID, facts.RunID, 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AcceptCandidateAttempt(context.Background(), facts.WorkspaceID, facts.RunID, 0, runtimecontract.RuntimeCandidate{
		Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
			Kind: "external-approval", SubjectRef: "subject-1", Reason: "需要外部裁决",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := controller.BindGovernanceRequest(context.Background(), facts.WorkspaceID, facts.RunID, runtimecontract.RuntimeGovernanceRequestRecord{
		TicketID: "ticket-1", AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID: sealed.ActiveDispatch.ID, DependencyKind: "external-approval", SubjectRef: "subject-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return bound
}

func governanceResolution(aggregate RuntimeAggregate, outcome runtimecontract.RuntimeGovernanceResolutionOutcome) runtimecontract.RuntimeGovernanceResolution {
	return runtimecontract.RuntimeGovernanceResolution{
		ResolutionID: "resolution-1", TicketID: "ticket-1", WorkspaceID: common.WorkspaceID("ws-v2"), RunID: common.RunID("run-v2"),
		AuthoritySnapshotID: aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID, DispatchID: aggregate.ActiveDispatch.ID,
		DependencyKind: "external-approval", SubjectRef: "subject-1", Outcome: outcome,
		Summary: "治理已完成", EvidenceRefs: []string{"decision://resolution-1"},
	}
}
