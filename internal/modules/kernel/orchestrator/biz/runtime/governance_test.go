package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeControllerRequiresGovernanceTicketBinding(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	sealed := sealGovernanceDispatch(t, controller)
	repository.aggregate.GovernanceRequests = nil
	_, err := controller.commitGovernanceResolution(context.Background(), governanceResolution(sealed, runtimecontract.RuntimeGovernanceResolutionOutcomeContinue))
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
	replayed, err := controller.bindGovernanceRequest(context.Background(), sealed.Facts.WorkspaceID, sealed.Facts.RunID, request)
	if err != nil || replayed.StateRevision != sealed.StateRevision || repository.saves != saves {
		t.Fatalf("binding replay = %+v saves=%d err=%v", replayed, repository.saves, err)
	}
	request.TicketID = "other-ticket"
	if _, err := controller.bindGovernanceRequest(context.Background(), sealed.Facts.WorkspaceID, sealed.Facts.RunID, request); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("conflicting binding err = %v, want conflict", err)
	}
}

func TestRuntimeControllerRejectsGovernanceSealWithoutSealedDependencySource(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	governed := sealGovernanceDispatch(t, controller)
	// Simulate a persisted-but-invalid authority that would previously have
	// received a generic decision ticket. The controller is the last write-side
	// fence and must reject it even when a caller provides a syntactically valid
	// governance candidate.
	*repository.aggregate = governed
	repository.aggregate.ActiveDispatch = nil
	repository.aggregate.GovernanceRequests = nil
	repository.aggregate.Facts.Failure.GovernanceDependency = nil
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(repository.aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources((repository.aggregate.Facts).Resources))
	if err != nil {
		t.Fatal(err)
	}
	_, err = controller.sealGovernanceDispatch(context.Background(), repository.aggregate.Facts.WorkspaceID, repository.aggregate.Facts.RunID, manifest, runtimecontract.RuntimeCandidate{
		Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
			Kind: "external-approval", SubjectRef: "subject-1", Reason: "需要外部裁决",
		}},
	})
	if !errors.Is(err, common.ErrConflict) || repository.aggregate.ActiveDispatch != nil || len(repository.aggregate.GovernanceRequests) != 0 {
		t.Fatalf("unsealed governance source err=%v aggregate=%+v", err, repository.aggregate)
	}
}

func TestRuntimeControllerCommitsGovernanceContinueAndResumesReducer(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	sealed := sealGovernanceDispatch(t, controller)
	resolution := governanceResolution(sealed, runtimecontract.RuntimeGovernanceResolutionOutcomeContinue)
	result, err := controller.commitGovernanceResolution(context.Background(), resolution)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != CommitDispositionCommitted || !result.DispatchCompleted || result.Aggregate.ActiveDispatch != nil {
		t.Fatalf("governance commit = %+v", result)
	}
	if result.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || result.Aggregate.Facts.Failure != nil {
		t.Fatalf("governance continuation decision = %+v facts=%+v", result.Aggregate.Decision, result.Aggregate.Facts)
	}
	if len(result.Aggregate.Facts.Evidence.Accepted) != 1 || result.Aggregate.Facts.Evidence.Accepted[0].Kind != runtimecontract.EvidenceKindGovernance || len(result.Aggregate.GovernanceResolutions) != 1 {
		t.Fatalf("governance evidence = %+v records=%+v", result.Aggregate.Facts.Evidence, result.Aggregate.GovernanceResolutions)
	}
	if result.Aggregate.DriveLease == nil || result.Aggregate.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		result.Aggregate.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry ||
		result.Aggregate.DriveLease.AggregateRevision != result.Aggregate.StateRevision || result.Aggregate.DriveLease.RecoveryGeneration != 1 {
		t.Fatalf("governance continuation did not atomically seal successor lease: %+v", result.Aggregate.DriveLease)
	}
	wantSignature, err := RuntimeGovernanceContinuationDriveLeaseSignature(resolution)
	if err != nil || result.Aggregate.DriveLease.FailureSignature != wantSignature {
		t.Fatalf("governance continuation successor signature=%q want=%q err=%v", result.Aggregate.DriveLease.FailureSignature, wantSignature, err)
	}
	committedLease := result.Aggregate.DriveLease.Clone()

	restarted, _ := NewRuntimeController(repository)
	duplicate, err := restarted.commitGovernanceResolution(context.Background(), resolution)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != result.Aggregate.StateRevision ||
		duplicate.Aggregate.DriveLease == nil || !runtimecontract.SameRuntimeDriveLeaseIdentity(*duplicate.Aggregate.DriveLease, committedLease) {
		t.Fatalf("duplicate governance resolution = %+v err=%v", duplicate, err)
	}
}

func TestRuntimeControllerRejectsProcessedGovernanceResolutionPayloadDrift(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*runtimecontract.RuntimeGovernanceResolution)
	}{
		{name: "summary", mutate: func(value *runtimecontract.RuntimeGovernanceResolution) { value.Summary = "different summary" }},
		{name: "evidence refs", mutate: func(value *runtimecontract.RuntimeGovernanceResolution) {
			value.EvidenceRefs = []string{"decision://different"}
		}},
		{name: "outcome", mutate: func(value *runtimecontract.RuntimeGovernanceResolution) {
			value.Outcome = runtimecontract.RuntimeGovernanceResolutionOutcomeFail
		}},
		{name: "ticket", mutate: func(value *runtimecontract.RuntimeGovernanceResolution) { value.TicketID = "ticket-different" }},
		{name: "dispatch", mutate: func(value *runtimecontract.RuntimeGovernanceResolution) { value.DispatchID = "dispatch-different" }},
		{name: "dependency", mutate: func(value *runtimecontract.RuntimeGovernanceResolution) {
			value.DependencyKind = "dependency-different"
		}},
		{name: "subject", mutate: func(value *runtimecontract.RuntimeGovernanceResolution) { value.SubjectRef = "subject-different" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repository := &controllerRepository{}
			controller, _ := NewRuntimeController(repository)
			sealed := sealGovernanceDispatch(t, controller)
			resolution := governanceResolution(sealed, runtimecontract.RuntimeGovernanceResolutionOutcomeContinue)
			committed, err := controller.commitGovernanceResolution(context.Background(), resolution)
			if err != nil || committed.Disposition != CommitDispositionCommitted {
				t.Fatalf("commit exact governance resolution=%+v err=%v", committed, err)
			}
			saves := repository.saves
			revision := repository.aggregate.StateRevision
			drifted := resolution
			tc.mutate(&drifted)
			result, err := controller.commitGovernanceResolution(context.Background(), drifted)
			if !errors.Is(err, common.ErrConflict) || repository.saves != saves || repository.aggregate.StateRevision != revision ||
				result.Disposition != "" || len(repository.aggregate.GovernanceResolutions) != 1 {
				t.Fatalf("processed payload drift result=%+v saves=%d/%d revision=%d/%d err=%v", result, saves, repository.saves, revision, repository.aggregate.StateRevision, err)
			}
		})
	}
}

func TestRuntimeControllerReplaysExactStaleGovernanceResolutionAsDiagnosticOnly(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	sealed := sealGovernanceDispatch(t, controller)
	stale := governanceResolution(sealed, runtimecontract.RuntimeGovernanceResolutionOutcomeContinue)
	stale.AuthoritySnapshotID = "authority-stale-replay"
	first, err := controller.commitGovernanceResolution(context.Background(), stale)
	if err != nil || first.Disposition != CommitDispositionStale || len(first.Aggregate.Diagnostics) != 1 ||
		len(first.Aggregate.GovernanceResolutions) != 0 || first.Aggregate.ActiveDispatch == nil || first.Aggregate.DriveLease != nil {
		t.Fatalf("first stale governance diagnostic=%+v err=%v", first, err)
	}
	saves := repository.saves
	revision := first.Aggregate.StateRevision
	replayed, err := controller.commitGovernanceResolution(context.Background(), stale)
	if err != nil || replayed.Disposition != CommitDispositionStale || replayed.Aggregate.StateRevision != revision ||
		repository.saves != saves || len(replayed.Aggregate.Diagnostics) != 1 || len(replayed.Aggregate.GovernanceResolutions) != 0 ||
		replayed.Aggregate.ActiveDispatch == nil || replayed.Aggregate.DriveLease != nil {
		t.Fatalf("exact stale replay=%+v saves=%d/%d err=%v", replayed, saves, repository.saves, err)
	}
	drifted := stale
	drifted.Summary = "drifted stale summary"
	driftResult, err := controller.commitGovernanceResolution(context.Background(), drifted)
	if !errors.Is(err, common.ErrConflict) || driftResult.Disposition != "" || repository.saves != saves ||
		repository.aggregate.StateRevision != revision || len(repository.aggregate.Diagnostics) != 1 {
		t.Fatalf("stale payload drift result=%+v saves=%d/%d revision=%d/%d err=%v", driftResult, saves, repository.saves, revision, repository.aggregate.StateRevision, err)
	}
}

func TestRuntimeInternalGovernanceContinueRequiresExactAtomicSuccessorLease(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	sealed := sealGovernanceDispatch(t, controller)
	resolution := governanceResolution(sealed, runtimecontract.RuntimeGovernanceResolutionOutcomeContinue)
	saves := repository.saves

	result, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind:        RuntimeInternalEventGovernanceResolutionObserved,
		Scope:       RuntimeInternalEventScope{WorkspaceID: resolution.WorkspaceID, RunID: resolution.RunID},
		Transaction: &RuntimeInternalTransactionPayload{GovernanceResolution: &resolution},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || result.Aggregate.Facts.WorkspaceID != "" || repository.saves != saves {
		t.Fatalf("bare governance continuation event result=%+v saves=%d want=%d err=%v", result, repository.saves, saves, err)
	}
	rejection := governanceResolution(sealed, runtimecontract.RuntimeGovernanceResolutionOutcomeFail)
	result, err = controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind:        RuntimeInternalEventGovernanceResolutionObserved,
		Scope:       RuntimeInternalEventScope{WorkspaceID: rejection.WorkspaceID, RunID: rejection.RunID},
		Transaction: &RuntimeInternalTransactionPayload{GovernanceResolution: &rejection},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || result.Aggregate.Facts.WorkspaceID != "" || repository.saves != saves {
		t.Fatalf("bare governance rejection event result=%+v saves=%d want=%d err=%v", result, repository.saves, saves, err)
	}

	signature, err := RuntimeGovernanceContinuationDriveLeaseSignature(resolution)
	if err != nil {
		t.Fatal(err)
	}
	wrong := runtimecontract.RuntimeDriveLease{
		WorkspaceID: resolution.WorkspaceID, RunID: resolution.RunID,
		Reason: runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry, FailureSignature: signature + "/wrong",
		DueAt: "2026-08-23T00:00:00Z",
	}
	result, err = controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind:  RuntimeInternalEventGovernanceResolutionObserved,
		Scope: RuntimeInternalEventScope{WorkspaceID: resolution.WorkspaceID, RunID: resolution.RunID},
		Transaction: &RuntimeInternalTransactionPayload{
			GovernanceResolution: &resolution, GovernanceContinuationDriveLease: &wrong,
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || result.Aggregate.Facts.WorkspaceID != "" || repository.saves != saves {
		t.Fatalf("wrong governance successor event result=%+v saves=%d want=%d err=%v", result, repository.saves, saves, err)
	}
}

func TestRuntimeControllerCommitsGovernanceFailureAsTerminal(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	sealed := sealGovernanceDispatch(t, controller)
	result, err := controller.commitGovernanceResolution(context.Background(), governanceResolution(sealed, runtimecontract.RuntimeGovernanceResolutionOutcomeFail))
	if err != nil {
		t.Fatal(err)
	}
	if result.Aggregate.Decision.State != runtimecontract.ConvergenceStateFailed || result.Aggregate.Decision.Terminal == nil || result.Aggregate.Decision.Terminal.Code != "governance_rejected" {
		t.Fatalf("governance rejection decision = %+v", result.Aggregate.Decision)
	}
	if len(result.Aggregate.Facts.Evidence.Rejected) != 1 || result.Aggregate.Facts.Evidence.Rejected[0].FailureClass != runtimecontract.FailureClassGovernanceRejected {
		t.Fatalf("governance rejection evidence = %+v", result.Aggregate.Facts.Evidence.Rejected)
	}
	if result.Aggregate.DriveLease == nil || result.Aggregate.DriveLease.State != runtimecontract.RuntimeDriveLeaseScheduled ||
		result.Aggregate.DriveLease.AggregateRevision != result.Aggregate.StateRevision || result.Aggregate.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry {
		t.Fatalf("governance rejection did not atomically seal terminal successor: %+v", result.Aggregate.DriveLease)
	}
	wantSignature, err := RuntimeGovernanceResolutionDriveLeaseSignature(governanceResolution(sealed, runtimecontract.RuntimeGovernanceResolutionOutcomeFail))
	if err != nil || result.Aggregate.DriveLease.FailureSignature != wantSignature {
		t.Fatalf("governance rejection successor signature=%q want=%q err=%v", result.Aggregate.DriveLease.FailureSignature, wantSignature, err)
	}
}

func TestRuntimeControllerKeepsStaleGovernanceResolutionDiagnosticOnly(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	sealed := sealGovernanceDispatch(t, controller)
	resolution := governanceResolution(sealed, runtimecontract.RuntimeGovernanceResolutionOutcomeContinue)
	resolution.AuthoritySnapshotID = "stale-snapshot"
	result, err := controller.commitGovernanceResolution(context.Background(), resolution)
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != CommitDispositionStale || len(result.Aggregate.Diagnostics) != 1 || result.Aggregate.ActiveDispatch == nil || len(result.Aggregate.GovernanceResolutions) != 0 {
		t.Fatalf("stale governance resolution changed authority: %+v", result.Aggregate)
	}
}

func TestRuntimeControllerStaleGovernanceDiagnosticReissuesCurrentContinuationSuccessor(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	sealed := sealGovernanceDispatch(t, controller)
	resolution := governanceResolution(sealed, runtimecontract.RuntimeGovernanceResolutionOutcomeContinue)
	committed, err := controller.commitGovernanceResolution(context.Background(), resolution)
	if err != nil || committed.Aggregate.DriveLease == nil {
		t.Fatalf("commit exact continuation=%+v err=%v", committed, err)
	}
	firstLease := committed.Aggregate.DriveLease.Clone()
	stale := resolution
	stale.ResolutionID = "resolution-stale-after-continuation"
	staleResult, err := controller.commitGovernanceResolution(context.Background(), stale)
	if err != nil || staleResult.Disposition != CommitDispositionStale || staleResult.Aggregate.DriveLease == nil {
		t.Fatalf("stale continuation diagnostic=%+v err=%v", staleResult, err)
	}
	reissued := staleResult.Aggregate.DriveLease
	if reissued.FailureSignature != firstLease.FailureSignature || reissued.RecoveryGeneration != firstLease.RecoveryGeneration+1 ||
		reissued.AggregateRevision != staleResult.Aggregate.StateRevision || reissued.State != runtimecontract.RuntimeDriveLeaseScheduled {
		t.Fatalf("stale diagnostic lost current successor: before=%+v after=%+v", firstLease, reissued)
	}
}

func sealGovernanceDispatch(t *testing.T, controller *RuntimeController) RuntimeAggregate {
	t.Helper()
	facts := controllerFacts()
	facts.Capabilities.GovernanceKinds = []string{"decision/resolution"}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceDependency, Signature: "external-approval", Message: "需要外部裁决",
		GovernanceDependency: &runtimecontract.ExternalDependency{Kind: "external-approval", SubjectRef: "subject-1", Reason: "需要外部裁决"},
	}
	definition := controllerDefinition()
	definition.CapabilityContract = append(definition.CapabilityContract, "decision/resolution")
	if _, err := controller.initialize(context.Background(), facts, definition, nil); err != nil {
		t.Fatal(err)
	}
	aggregate, err := controller.Load(context.Background(), facts.WorkspaceID, facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources((aggregate.Facts).Resources))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.sealGovernanceDispatch(context.Background(), facts.WorkspaceID, facts.RunID, manifest, runtimecontract.RuntimeCandidate{
		Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
			Kind: "external-approval", SubjectRef: "subject-1", Reason: "需要外部裁决",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := controller.bindGovernanceRequest(context.Background(), facts.WorkspaceID, facts.RunID, runtimecontract.RuntimeGovernanceRequestRecord{
		TicketID: "ticket-1", AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID: sealed.ActiveDispatch.ID, DependencyKind: "external-approval", SubjectRef: "subject-1",
		AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(runtimecontract.ExternalDependency{}.AuthorizationContext),
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

func TestRecordOperationGovernanceRequiredDoesNotUseExecutionRepair(t *testing.T) {
	repository := &controllerRepository{}
	controller, _ := NewRuntimeController(repository)
	facts := controllerFacts()
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindApprovalRequest}
	if _, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	// Seed an active sealed dispatch with operation execution ledger.
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{
		Operation: runtimecontract.RuntimeOperation{
			ID: "write-1", Kind: "file/write", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("governed"),
		},
	}}}}
	sealed, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.ActiveDispatch == nil {
		t.Fatal("expected active dispatch")
	}
	claimed, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", "write-1")
	if err != nil {
		t.Fatal(err)
	}
	execution, ok := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, "write-1")
	if !ok {
		t.Fatal("expected claimed operation execution")
	}
	requirement := runtimecontract.RuntimeGovernanceRequirement{
		Kind: runtimecontract.GovernanceApproval, SubjectRef: "target.txt",
		Reason: "patch apply requires approval before side effects", RequestedAction: "patch/apply", RiskLevel: "high",
		AuthorizationContext: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "target.txt")),
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeGovernanceRequired, ObservedEffectScope: runtimecontract.RuntimeEffectGovernanceOnly,
		Governance: &requirement,
	}
	_, committed, err := controller.commitOperationReceipt(context.Background(), "ws-v2", "run-v2", sealed.ActiveDispatch.ID, execution.Fence, receipt)
	if err != nil {
		t.Fatal(err)
	}
	next, err := controller.candidateDispatch.recordOperationGovernanceRequired(context.Background(), "ws-v2", "run-v2", "write-1", RuntimeOperationExecutionResult{
		EventID: "gov-event-1", OperationID: "write-1", Receipt: committed, Governance: &requirement,
	})
	if err != nil {
		t.Fatalf("recordOperationGovernanceRequired: %v", err)
	}
	if next.ActiveDispatch != nil {
		t.Fatal("active dispatch must close when governance is required")
	}
	if next.Facts.Failure == nil || next.Facts.Failure.Class != runtimecontract.FailureClassGovernanceRequired {
		t.Fatalf("failure = %+v, want governance_required", next.Facts.Failure)
	}
	if next.Decision.State != runtimecontract.ConvergenceStateNeedGovernance {
		t.Fatalf("decision action = %s, want request_governance", next.Decision.State)
	}
	if len(next.CompletedDispatches) != 1 || len(next.CompletedDispatches[0].OperationExecutions) != 1 ||
		next.CompletedDispatches[0].OperationExecutions[0].OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(committed) {
		t.Fatalf("governance closeout lost owner receipt authority: %+v", next.CompletedDispatches)
	}
	if len(next.OperationGovernanceBindings) != 1 {
		t.Fatalf("operation governance bindings = %+v", next.OperationGovernanceBindings)
	}
	binding := next.OperationGovernanceBindings[0]
	if binding.State != runtimecontract.RuntimeOperationGovernanceBindingSuspended || binding.OperationDispatchID != sealed.ActiveDispatch.ID ||
		binding.ExecutionKey != execution.ExecutionKey || binding.Fence != execution.Fence || binding.GovernanceReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(committed) {
		t.Fatalf("suspended operation governance binding = %+v", binding)
	}
	governanceAggregate, err := controller.Load(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(governanceAggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources((governanceAggregate.Facts).Resources))
	if err != nil {
		t.Fatal(err)
	}
	governed, err := controller.sealGovernanceDispatch(context.Background(), "ws-v2", "run-v2", manifest, runtimecontract.RuntimeCandidate{
		Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
			Kind: string(requirement.Kind), SubjectRef: requirement.SubjectRef, Reason: requirement.Reason,
			RequestedAction: requirement.RequestedAction, RiskLevel: requirement.RiskLevel, AuthorizationContext: requirement.AuthorizationContext.Clone(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := controller.bindGovernanceRequest(context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeGovernanceRequestRecord{
		ApprovalID: "approval-operation-1", AuthoritySnapshotID: governed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID: governed.ActiveDispatch.ID, DependencyKind: string(requirement.Kind), SubjectRef: requirement.SubjectRef,
		RequestedAction: requirement.RequestedAction, RiskLevel: requirement.RiskLevel,
		AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(requirement.AuthorizationContext),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bound.OperationGovernanceBindings) != 1 || bound.OperationGovernanceBindings[0].State != runtimecontract.RuntimeOperationGovernanceBindingTicketBound ||
		bound.OperationGovernanceBindings[0].ApprovalID != "approval-operation-1" || bound.OperationGovernanceBindings[0].GovernanceDispatchID != governed.ActiveDispatch.ID {
		t.Fatalf("approval-bound operation governance binding = %+v", bound.OperationGovernanceBindings)
	}
	operationResolution := runtimecontract.RuntimeGovernanceResolution{
		ResolutionID: "operation-resolution-1", ApprovalID: "approval-operation-1", WorkspaceID: "ws-v2", RunID: "run-v2",
		AuthoritySnapshotID: governed.ActiveDispatch.Transaction.AuthoritySnapshotID, DispatchID: governed.ActiveDispatch.ID,
		DependencyKind: string(requirement.Kind), SubjectRef: requirement.SubjectRef,
		Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "approval granted",
	}
	continued, err := controller.commitGovernanceResolution(context.Background(), operationResolution)
	if err != nil {
		t.Fatal(err)
	}
	if len(continued.Aggregate.OperationGovernanceBindings) != 1 || continued.Aggregate.OperationGovernanceBindings[0].State != runtimecontract.RuntimeOperationGovernanceBindingGranted {
		t.Fatalf("granted operation governance binding = %+v", continued.Aggregate.OperationGovernanceBindings)
	}
	if continued.Aggregate.ActiveDispatch == nil {
		t.Fatal("governance continue must create resume dispatch")
	}
	duplicate, err := controller.commitGovernanceResolution(context.Background(), operationResolution)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.ActiveDispatch == nil ||
		duplicate.Aggregate.ActiveDispatch.ID != continued.Aggregate.ActiveDispatch.ID {
		t.Fatalf("duplicate operation governance resolution = %+v err=%v", duplicate, err)
	}
	resumeExecution, ok := operationExecutionRecord(continued.Aggregate.ActiveDispatch.OperationExecutions, "write-1")
	if !ok || resumeExecution.ExecutionKey != execution.ExecutionKey || resumeExecution.Fence != execution.Fence+1 || resumeExecution.State != runtimecontract.OperationExecutionClaimed {
		t.Fatalf("resume execution = %+v", resumeExecution)
	}
	resumeAllocation, found := runtimeOperationBudgetAllocationForTest(continued.Aggregate, resumeExecution)
	if !found || resumeAllocation.State != runtimecontract.RuntimeBudgetAllocationReserved {
		t.Fatalf("governance resume budget allocation = %+v found=%v", resumeAllocation, found)
	}
	grant := continued.Aggregate.OperationGovernanceBindings[0].Grant
	if grant == nil || grant.ExecutionKey != execution.ExecutionKey || grant.GovernanceFence != execution.Fence || grant.ResumeFence != execution.Fence+1 {
		t.Fatalf("governance grant = %+v", grant)
	}
	if _, found, err := controller.QueryOperationReceipt(context.Background(), "ws-v2", "run-v2", continued.Aggregate.ActiveDispatch.ID,
		"write-1", execution.ExecutionKey, execution.Fence+1, grant); err != nil || found {
		t.Fatalf("authorized resume receipt preflight found=%v err=%v", found, err)
	}
	if _, _, err := controller.QueryOperationReceipt(context.Background(), "ws-v2", "run-v2", continued.Aggregate.ActiveDispatch.ID,
		"write-1", execution.ExecutionKey, execution.Fence+1, nil); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("omitted persisted governance grant err=%v, want conflict", err)
	}
	driftedGrant := grant.Normalized()
	driftedGrant.SubjectRef = "other.txt"
	if _, _, err := controller.QueryOperationReceipt(context.Background(), "ws-v2", "run-v2", continued.Aggregate.ActiveDispatch.ID,
		"write-1", execution.ExecutionKey, execution.Fence+1, &driftedGrant); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("drifted governance grant err=%v, want conflict", err)
	}
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	executor := &recordingOperationExecutor{owner: restarted}
	resumed, err := restarted.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatal(err)
	}
	if len(executor.requests) != 1 || executor.requests[0].GovernanceGrant == nil ||
		!runtimecontract.SameRuntimeOperationGovernanceGrant(*executor.requests[0].GovernanceGrant, *grant) {
		t.Fatalf("resume executor requests = %+v", executor.requests)
	}
	if len(resumed.OperationReceipts) != 2 {
		t.Fatalf("governance/resume receipts = %+v", resumed.OperationReceipts)
	}
	if _, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(resumed.OperationReceipts, execution.ExecutionKey, execution.Fence); !ok {
		t.Fatal("governance receipt was not preserved")
	}
	if receipt, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(resumed.OperationReceipts, execution.ExecutionKey, execution.Fence+1); !ok || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
		t.Fatalf("resume receipt = %+v found=%v", receipt, ok)
	}
	resumeAllocation, found = runtimeOperationBudgetAllocationForTest(resumed, resumeExecution)
	if !found || resumeAllocation.State != runtimecontract.RuntimeBudgetAllocationConfirmed {
		t.Fatalf("confirmed governance resume budget allocation = %+v found=%v", resumeAllocation, found)
	}
}

func runtimeOperationBudgetAllocationForTest(aggregate RuntimeAggregate, execution runtimecontract.RuntimeOperationExecutionRecord) (runtimecontract.RuntimeBudgetAllocation, bool) {
	scopeRef := runtimeOperationBudgetScopeRef(execution.ExecutionKey, execution.Fence)
	for _, allocation := range aggregate.ExecutionBudget.Allocations {
		allocation = allocation.Normalized()
		if allocation.Dimension == runtimecontract.RuntimeBudgetDimensionCapabilityOperation && allocation.LogicalConsumptionID == execution.ExecutionKey && allocation.ScopeRef == scopeRef {
			return allocation, true
		}
	}
	return runtimecontract.RuntimeBudgetAllocation{}, false
}
