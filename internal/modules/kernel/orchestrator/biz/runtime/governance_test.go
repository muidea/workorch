package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type governanceRetryProbeExecutor struct {
	owner    *RuntimeController
	requests []RuntimeOperationExecutionRequest
}

type governanceCASFailRepository struct {
	aggregate RuntimeAggregate
}

func (r *governanceCASFailRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return RuntimeAggregate{}, common.ErrNotFound
	}
	data, err := json.Marshal(r.aggregate)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	var clone RuntimeAggregate
	if err := json.Unmarshal(data, &clone); err != nil {
		return RuntimeAggregate{}, err
	}
	return clone, nil
}

func (*governanceCASFailRepository) Save(context.Context, RuntimeAggregate, uint64) error {
	return common.ErrConflict
}

func (e *governanceRetryProbeExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	e.requests = append(e.requests, request)
	if len(e.requests) == 1 {
		return RuntimeOperationExecutionResult{}, errors.New("governed owner response lost before receipt persistence")
	}
	delegate := &recordingOperationExecutor{owner: e.owner}
	return delegate.ExecuteRuntimeOperation(ctx, request)
}

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
	facts := multiOperationFixtureFacts(t)
	sequence := runtimecontract.RuntimeCapabilityStateSnapshot{
		RequirementID: "governance-file-sequence", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		CapabilityStateKind: "test_operation_sequence", SubjectRef: "workspace_deliverable",
		ObservedRevision: "governance-file-sequence-v1", ContentHash: "governance-file-sequence-content-v1",
		AcquisitionKey: "governance-file-sequence-acquisition-v1", QueryReceiptRef: "governance-file-sequence-receipt-v1",
		Disposition: runtimecontract.RuntimeCapabilityStateSequenceRequired,
		RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{
			{RequirementID: "predecessor", Kind: runtimecontract.StepKindFileWrite, TargetRef: "before.txt"},
			{RequirementID: "governed", Kind: runtimecontract.StepKindFileWrite, TargetRef: "a.txt", DependsOnRequirementIDs: []string{"predecessor"}},
			{RequirementID: "sibling", Kind: runtimecontract.StepKindFileWrite, TargetRef: "b.txt", DependsOnRequirementIDs: []string{"governed"}},
		},
	}
	sequence.SnapshotID = runtimecontract.StableRuntimeCapabilityStateSnapshotID(sequence)
	facts.Resources.CapabilityStates = append(facts.Resources.CapabilityStates, sequence)
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindApprovalRequest}
	if len(facts.Resources.OperationAdmissibilities) != 1 {
		t.Fatalf("expected one File owner catalog: %+v", facts.Resources.OperationAdmissibilities)
	}
	ownerCatalog := facts.Resources.OperationAdmissibilities[0].Normalized()
	ownerCatalog.TargetRefs = append(ownerCatalog.TargetRefs, "before.txt")
	ownerCatalog.OperationPolicies = []runtimecontract.RuntimeOperationPolicyFact{{
		ID: "governance-file-write", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite,
		EffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}}
	ownerCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(ownerCatalog)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{ownerCatalog}
	ownerCatalogObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: ownerCatalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: ownerCatalog.OwnerModuleID, ContentHash: ownerCatalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: ownerCatalog.ObservedRevision,
		}},
	}
	ownerCatalogObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(ownerCatalogObservation)
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{ownerCatalogObservation}
	facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, runtimecontract.RuntimePrerequisite{
		ID: ownerCatalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: ownerCatalog.OwnerModuleID, RequiredCapabilities: append([]string(nil), ownerCatalog.OperationKinds...),
	})
	if _, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	// Seed an active sealed dispatch with operation execution ledger.
	if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{
		{Operation: runtimecontract.RuntimeOperation{
			Kind: "file/write", RequiredOperationRef: "predecessor", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"before.txt"}, Parameters: runtimeFileWriteParameters("completed-predecessor"),
		}},
		{Operation: runtimecontract.RuntimeOperation{
			Kind: "file/write", RequiredOperationRef: "governed", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"a.txt"}, Parameters: runtimeFileWriteParameters("governed"),
		}},
		{Operation: runtimecontract.RuntimeOperation{
			Kind: "file/write", RequiredOperationRef: "sibling", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"b.txt"}, Parameters: runtimeFileWriteParameters("governed-sibling"),
		}},
	}}}
	if err := RecordFixtureCandidateFeasibilities(context.Background(), controller, "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	sealed, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.ActiveDispatch == nil {
		t.Fatal("expected active dispatch")
	}
	predecessorOperationID := runtimeTestActiveOperationID(t, sealed, 0)
	governedOperationID := runtimeTestActiveOperationID(t, sealed, 1)
	siblingOperationID := runtimeTestActiveOperationID(t, sealed, 2)
	sealedCatalogSnapshotID := ownerCatalog.SnapshotID
	sealedAcquisitionBinding := ownerCatalog.AcquisitionBinding
	sealedReplayProof := runtimecontract.RuntimeOperationReplayProof{
		OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationID: governedOperationID, TargetRefs: []string{"a.txt"},
		BeforeHash: "before-governance", AfterHash: "after-governance",
		AdmissibilitySnapshotID: sealedCatalogSnapshotID, AcquisitionBinding: sealedAcquisitionBinding,
	}
	completedReplayProof := runtimecontract.RuntimeOperationReplayProof{
		OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationID: predecessorOperationID, TargetRefs: []string{"before.txt"},
		BeforeHash: "before-completed", AfterHash: "after-completed",
		AdmissibilitySnapshotID: sealedCatalogSnapshotID, AcquisitionBinding: sealedAcquisitionBinding,
	}
	siblingReplayProof := runtimecontract.RuntimeOperationReplayProof{
		OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationID: siblingOperationID, TargetRefs: []string{"b.txt"},
		BeforeHash: "before-governance-sibling", AfterHash: "after-governance-sibling",
		AdmissibilitySnapshotID: sealedCatalogSnapshotID, AcquisitionBinding: sealedAcquisitionBinding,
	}
	for idx := range repository.aggregate.ActiveDispatch.OperationExecutions {
		execution := &repository.aggregate.ActiveDispatch.OperationExecutions[idx]
		execution.AdmissibilitySnapshotID = sealedCatalogSnapshotID
		execution.AcquisitionBinding = sealedAcquisitionBinding
		switch execution.OperationID {
		case predecessorOperationID:
			execution.ReplayProof = &completedReplayProof
		case governedOperationID:
			execution.ReplayProof = &sealedReplayProof
		case siblingOperationID:
			execution.ReplayProof = &siblingReplayProof
		}
		execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(*execution)
	}
	repository.aggregate.ActiveDispatch.OperationAuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthoritiesHash(
		repository.aggregate.ActiveDispatch.OperationExecutions,
	)
	repository.aggregate.ActiveDispatch.DispatchAuthorityHash = runtimecontract.StableRuntimeDispatchAuthorityHash(
		sealedDispatchToRuntimeRecord(*repository.aggregate.ActiveDispatch),
	)
	for idx := range repository.aggregate.CandidateAttempts {
		attempt := &repository.aggregate.CandidateAttempts[idx]
		if attempt.State == runtimecontract.RuntimeCandidateAttemptStateAccepted && attempt.DispatchID == repository.aggregate.ActiveDispatch.ID {
			attempt.DispatchAuthorityHash = repository.aggregate.ActiveDispatch.DispatchAuthorityHash
		}
	}
	claimedPredecessor, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", predecessorOperationID)
	if err != nil {
		t.Fatal(err)
	}
	predecessorExecution, ok := operationExecutionRecord(claimedPredecessor.ActiveDispatch.OperationExecutions, predecessorOperationID)
	if !ok {
		t.Fatal("expected claimed predecessor operation execution")
	}
	predecessorReceipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: predecessorExecution.ExecutionKey, OperationID: predecessorExecution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		Changed: true, TargetRefs: []string{"before.txt"},
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "completed predecessor", ContentHash: "completed-predecessor-hash"},
	}
	predecessorReceiptAggregate, _, err := controller.commitOperationReceipt(
		context.Background(), "ws-v2", "run-v2", claimedPredecessor.ActiveDispatch.ID, predecessorExecution.Fence, predecessorReceipt,
	)
	if err != nil {
		t.Fatal(err)
	}
	// Isolate the governance-resume completed-sibling branch without letting the
	// predecessor's proof settle the only CWI. The owner receipt and execution
	// budget were committed through production transitions above; this fixture
	// advances only the already-judged evidence marker and completed-operation
	// index before the governed operation is claimed.
	for idx := range predecessorReceiptAggregate.ActiveDispatch.OperationExecutions {
		if predecessorReceiptAggregate.ActiveDispatch.OperationExecutions[idx].OperationID == predecessorOperationID {
			predecessorReceiptAggregate.ActiveDispatch.OperationExecutions[idx].State = runtimecontract.OperationExecutionEvidenceCommitted
		}
	}
	predecessorReceiptAggregate.ActiveDispatch.CompletedOperationIDs = appendNormalizedString(
		predecessorReceiptAggregate.ActiveDispatch.CompletedOperationIDs,
		predecessorOperationID,
	)
	repository.aggregate = &predecessorReceiptAggregate
	if predecessorReceiptAggregate.ActiveDispatch == nil ||
		!containsString(predecessorReceiptAggregate.ActiveDispatch.CompletedOperationIDs, predecessorOperationID) {
		t.Fatalf("predecessor did not commit before governance: %+v", predecessorReceiptAggregate.ActiveDispatch)
	}
	sourceCompleted, ok := operationExecutionRecord(predecessorReceiptAggregate.ActiveDispatch.OperationExecutions, predecessorOperationID)
	if !ok || sourceCompleted.State != runtimecontract.OperationExecutionEvidenceCommitted {
		t.Fatalf("completed predecessor execution = %+v", sourceCompleted)
	}
	sourceSibling, ok := operationExecutionRecord(repository.aggregate.ActiveDispatch.OperationExecutions, siblingOperationID)
	if !ok {
		t.Fatal("expected sealed sibling operation execution")
	}
	claimed, err := controller.claimOperationExecution(context.Background(), "ws-v2", "run-v2", governedOperationID)
	if err != nil {
		t.Fatal(err)
	}
	execution, ok := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, governedOperationID)
	if !ok {
		t.Fatal("expected claimed operation execution")
	}
	requirement := runtimecontract.RuntimeGovernanceRequirement{
		Kind: runtimecontract.GovernanceApproval, SubjectRef: "a.txt",
		Reason: "patch apply requires approval before side effects", RequestedAction: "patch/apply", RiskLevel: "high",
		AuthorizationContext: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "a.txt")),
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
	next, err := controller.candidateDispatch.recordOperationGovernanceRequired(context.Background(), "ws-v2", "run-v2", governedOperationID, RuntimeOperationExecutionResult{
		EventID: "gov-event-1", OperationID: governedOperationID, Receipt: committed, Governance: &requirement,
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
	closedExecution, closedExecutionFound := operationExecutionRecord(next.CompletedDispatches[0].OperationExecutions, governedOperationID)
	if len(next.CompletedDispatches) != 1 || len(next.CompletedDispatches[0].OperationExecutions) != 3 ||
		!containsString(next.CompletedDispatches[0].CompletedOperationIDs, predecessorOperationID) || !closedExecutionFound ||
		closedExecution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(committed) {
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
	resumeExecution, ok := operationExecutionRecord(continued.Aggregate.ActiveDispatch.OperationExecutions, governedOperationID)
	if !ok || resumeExecution.ExecutionKey != execution.ExecutionKey || resumeExecution.Fence != execution.Fence+1 || resumeExecution.State != runtimecontract.OperationExecutionClaimed {
		t.Fatalf("resume execution = %+v", resumeExecution)
	}
	if resumeExecution.AdmissibilitySnapshotID != sealedCatalogSnapshotID || resumeExecution.AcquisitionBinding != sealedAcquisitionBinding ||
		resumeExecution.ReplayProof == nil || resumeExecution.ReplayProof.BeforeHash != sealedReplayProof.BeforeHash ||
		resumeExecution.ReplayProof.AfterHash != sealedReplayProof.AfterHash {
		t.Fatalf("governance resume lost sealed execution authority: %+v", resumeExecution)
	}
	resumeCompleted, ok := operationExecutionRecord(continued.Aggregate.ActiveDispatch.OperationExecutions, predecessorOperationID)
	if !ok || resumeCompleted.State != runtimecontract.OperationExecutionEvidenceCommitted ||
		resumeCompleted.ExecutionKey != sourceCompleted.ExecutionKey || resumeCompleted.Fence != sourceCompleted.Fence ||
		resumeCompleted.OwnerReceiptRef != sourceCompleted.OwnerReceiptRef || resumeCompleted.OwnerReceiptHash != sourceCompleted.OwnerReceiptHash ||
		resumeCompleted.AuthorityHash != sourceCompleted.AuthorityHash || resumeCompleted.IdempotencyClass != sourceCompleted.IdempotencyClass ||
		!containsString(continued.Aggregate.ActiveDispatch.CompletedOperationIDs, predecessorOperationID) {
		t.Fatalf("governance resume lost completed sibling authority: source=%+v resume=%+v", sourceCompleted, resumeCompleted)
	}
	resumeSibling, ok := operationExecutionRecord(continued.Aggregate.ActiveDispatch.OperationExecutions, siblingOperationID)
	if !ok || resumeSibling.State != runtimecontract.OperationExecutionSealed || resumeSibling.Fence != 0 ||
		resumeSibling.ExecutionKey == sourceSibling.ExecutionKey || resumeSibling.AdmissibilitySnapshotID != sourceSibling.AdmissibilitySnapshotID ||
		resumeSibling.AcquisitionBinding != sourceSibling.AcquisitionBinding || resumeSibling.ReplayProof == nil ||
		resumeSibling.ReplayProof.AfterHash != siblingReplayProof.AfterHash {
		t.Fatalf("governance resume lost pending sibling authority: source=%+v resume=%+v", sourceSibling, resumeSibling)
	}
	driftedResume := continued.Aggregate
	driftedActive := *continued.Aggregate.ActiveDispatch
	driftedActive.OperationExecutions = append([]runtimecontract.RuntimeOperationExecutionRecord(nil), continued.Aggregate.ActiveDispatch.OperationExecutions...)
	for idx := range driftedActive.OperationExecutions {
		if driftedActive.OperationExecutions[idx].OperationID == binding.OperationID {
			driftedActive.OperationExecutions[idx].ExpectedEffectScope = runtimecontract.RuntimeEffectRemoteDurable
		}
	}
	driftedResume.ActiveDispatch = &driftedActive
	if err := driftedResume.Validate(); err == nil {
		t.Fatal("active governance resume accepted an effect scope outside its exact grant")
	}
	driftedIdempotencyResume := continued.Aggregate
	driftedIdempotencyActive := *continued.Aggregate.ActiveDispatch
	driftedIdempotencyActive.OperationExecutions = make([]runtimecontract.RuntimeOperationExecutionRecord, len(continued.Aggregate.ActiveDispatch.OperationExecutions))
	for idx := range continued.Aggregate.ActiveDispatch.OperationExecutions {
		driftedIdempotencyActive.OperationExecutions[idx] = continued.Aggregate.ActiveDispatch.OperationExecutions[idx].Clone()
		if driftedIdempotencyActive.OperationExecutions[idx].OperationID == binding.OperationID {
			driftedIdempotencyActive.OperationExecutions[idx].IdempotencyClass = runtimecontract.RuntimeOperationIdempotencySafeToReplay
		}
	}
	driftedIdempotencyResume.ActiveDispatch = &driftedIdempotencyActive
	if err := driftedIdempotencyResume.Validate(); err == nil {
		t.Fatal("active governance resume accepted an idempotency class outside its exact binding")
	}
	wrongFenceResume := continued.Aggregate
	wrongFenceActive := *continued.Aggregate.ActiveDispatch
	wrongFenceActive.OperationExecutions = make([]runtimecontract.RuntimeOperationExecutionRecord, len(continued.Aggregate.ActiveDispatch.OperationExecutions))
	for idx := range continued.Aggregate.ActiveDispatch.OperationExecutions {
		wrongFenceActive.OperationExecutions[idx] = continued.Aggregate.ActiveDispatch.OperationExecutions[idx].Clone()
		if wrongFenceActive.OperationExecutions[idx].OperationID == binding.OperationID {
			wrongFenceActive.OperationExecutions[idx].Fence++
			wrongFenceActive.OperationExecutions[idx].ClaimedAt = runtimecontract.RuntimeOperationClaimMarkerInitial
		}
	}
	wrongFenceResume.ActiveDispatch = &wrongFenceActive
	if err := wrongFenceResume.Validate(); err == nil {
		t.Fatal("active governance resume accepted a wrong fence as an ordinary claim")
	}
	completedTarget := continued.Aggregate
	completedTarget.CompletedDispatches = append([]SealedDispatch(nil), continued.Aggregate.CompletedDispatches...)
	for idx := range completedTarget.CompletedDispatches {
		if completedTarget.CompletedDispatches[idx].ID == binding.OperationDispatchID {
			completedTarget.CompletedDispatches[idx].CompletedOperationIDs = append(
				append([]string(nil), continued.Aggregate.CompletedDispatches[idx].CompletedOperationIDs...),
				binding.OperationID,
			)
		}
	}
	if _, err := buildGovernanceResumeDispatch(completedTarget, continued.Aggregate.OperationGovernanceBindings[0]); err == nil {
		t.Fatal("governance resume accepted a target already listed as completed")
	}
	tampered := continued.Aggregate
	tampered.CompletedDispatches = append([]SealedDispatch(nil), continued.Aggregate.CompletedDispatches...)
	for dispatchIndex := range tampered.CompletedDispatches {
		if tampered.CompletedDispatches[dispatchIndex].ID != binding.OperationDispatchID {
			continue
		}
		tampered.CompletedDispatches[dispatchIndex].OperationExecutions = append(
			[]runtimecontract.RuntimeOperationExecutionRecord(nil),
			continued.Aggregate.CompletedDispatches[dispatchIndex].OperationExecutions...,
		)
		for executionIndex := range tampered.CompletedDispatches[dispatchIndex].OperationExecutions {
			if tampered.CompletedDispatches[dispatchIndex].OperationExecutions[executionIndex].OperationID == binding.OperationID {
				tampered.CompletedDispatches[dispatchIndex].OperationExecutions[executionIndex].AdmissibilitySnapshotID = "substituted-catalog"
			}
		}
	}
	if _, err := buildGovernanceResumeDispatch(tampered, continued.Aggregate.OperationGovernanceBindings[0]); err == nil {
		t.Fatal("governance resume accepted a source execution that drifted from its granted binding")
	}
	tamperedClass := continued.Aggregate
	tamperedClass.CompletedDispatches = append([]SealedDispatch(nil), continued.Aggregate.CompletedDispatches...)
	for dispatchIndex := range tamperedClass.CompletedDispatches {
		if tamperedClass.CompletedDispatches[dispatchIndex].ID != binding.OperationDispatchID {
			continue
		}
		tamperedClass.CompletedDispatches[dispatchIndex].OperationExecutions = make(
			[]runtimecontract.RuntimeOperationExecutionRecord,
			len(continued.Aggregate.CompletedDispatches[dispatchIndex].OperationExecutions),
		)
		for executionIndex := range continued.Aggregate.CompletedDispatches[dispatchIndex].OperationExecutions {
			tamperedClass.CompletedDispatches[dispatchIndex].OperationExecutions[executionIndex] =
				continued.Aggregate.CompletedDispatches[dispatchIndex].OperationExecutions[executionIndex].Clone()
			if tamperedClass.CompletedDispatches[dispatchIndex].OperationExecutions[executionIndex].OperationID == binding.OperationID {
				tamperedClass.CompletedDispatches[dispatchIndex].OperationExecutions[executionIndex].IdempotencyClass = runtimecontract.RuntimeOperationIdempotencySafeToReplay
			}
		}
	}
	if _, err := buildGovernanceResumeDispatch(tamperedClass, continued.Aggregate.OperationGovernanceBindings[0]); err == nil {
		t.Fatal("governance resume accepted a substituted source idempotency class")
	}
	tamperedPending := continued.Aggregate
	tamperedPending.CompletedDispatches = append([]SealedDispatch(nil), continued.Aggregate.CompletedDispatches...)
	for dispatchIndex := range tamperedPending.CompletedDispatches {
		if tamperedPending.CompletedDispatches[dispatchIndex].ID != binding.OperationDispatchID {
			continue
		}
		tamperedPending.CompletedDispatches[dispatchIndex].OperationExecutions = make(
			[]runtimecontract.RuntimeOperationExecutionRecord,
			len(continued.Aggregate.CompletedDispatches[dispatchIndex].OperationExecutions),
		)
		for executionIndex := range continued.Aggregate.CompletedDispatches[dispatchIndex].OperationExecutions {
			tamperedPending.CompletedDispatches[dispatchIndex].OperationExecutions[executionIndex] =
				continued.Aggregate.CompletedDispatches[dispatchIndex].OperationExecutions[executionIndex].Clone()
			pending := &tamperedPending.CompletedDispatches[dispatchIndex].OperationExecutions[executionIndex]
			if pending.OperationID == siblingOperationID {
				pending.IdempotencyClass = runtimecontract.RuntimeOperationIdempotencySafeToReplay
				pending.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(*pending)
			}
		}
	}
	if _, err := buildGovernanceResumeDispatch(tamperedPending, continued.Aggregate.OperationGovernanceBindings[0]); err == nil {
		t.Fatal("governance resume accepted substituted pending sibling replay authority")
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
		governedOperationID, execution.ExecutionKey, execution.Fence+1, grant); err != nil || found {
		t.Fatalf("authorized resume receipt preflight found=%v err=%v", found, err)
	}
	if _, _, err := controller.QueryOperationReceipt(context.Background(), "ws-v2", "run-v2", continued.Aggregate.ActiveDispatch.ID,
		governedOperationID, execution.ExecutionKey, execution.Fence+1, nil); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("omitted persisted governance grant err=%v, want conflict", err)
	}
	driftedGrant := grant.Normalized()
	driftedGrant.SubjectRef = "other.txt"
	if _, _, err := controller.QueryOperationReceipt(context.Background(), "ws-v2", "run-v2", continued.Aggregate.ActiveDispatch.ID,
		governedOperationID, execution.ExecutionKey, execution.Fence+1, &driftedGrant); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("drifted governance grant err=%v, want conflict", err)
	}
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	casAggregate := continued.Aggregate
	casActive := *continued.Aggregate.ActiveDispatch
	casActive.OperationExecutions = make([]runtimecontract.RuntimeOperationExecutionRecord, len(continued.Aggregate.ActiveDispatch.OperationExecutions))
	for idx := range continued.Aggregate.ActiveDispatch.OperationExecutions {
		casActive.OperationExecutions[idx] = continued.Aggregate.ActiveDispatch.OperationExecutions[idx].Clone()
	}
	casAggregate.ActiveDispatch = &casActive
	casRepository := &governanceCASFailRepository{aggregate: casAggregate}
	casController, err := NewRuntimeController(casRepository)
	if err != nil {
		t.Fatal(err)
	}
	casExecutor := &failIfExecutedOperationExecutor{}
	_, casErr := casController.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind:        RuntimeInternalEventDispatchExecutionRequested,
		Scope:       RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"},
		Transaction: &RuntimeInternalTransactionPayload{Executor: casExecutor},
	})
	if !errors.Is(casErr, common.ErrConflict) || casExecutor.called {
		t.Fatalf("failed governance effect-start CAS reached owner: called=%v err=%v", casExecutor.called, casErr)
	}
	casPersistedExecution, ok := operationExecutionRecord(casRepository.aggregate.ActiveDispatch.OperationExecutions, governedOperationID)
	if !ok || casPersistedExecution.ClaimedAt != runtimecontract.RuntimeOperationClaimMarkerGovernanceGranted {
		t.Fatalf("failed governance effect-start CAS leaked an uncommitted marker: %+v", casPersistedExecution)
	}
	executor := &governanceRetryProbeExecutor{owner: restarted}
	_, firstDriveErr := restarted.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
		Kind:        RuntimeInternalEventDispatchExecutionRequested,
		Scope:       RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"},
		Transaction: &RuntimeInternalTransactionPayload{Executor: executor},
	})
	if operationID, required := RuntimeOperationRecoveryBudgetRequired(firstDriveErr); !required || operationID != governedOperationID || len(executor.requests) != 1 {
		t.Fatalf("governance owner error bypassed recovery handoff: operation=%q required=%v requests=%d err=%v", operationID, required, len(executor.requests), firstDriveErr)
	}
	afterFirstDrive, err := restarted.Load(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	startedExecution, ok := operationExecutionRecord(afterFirstDrive.ActiveDispatch.OperationExecutions, governedOperationID)
	if !ok || startedExecution.ClaimedAt != runtimecontract.RuntimeOperationClaimMarkerGovernanceEffectStarted ||
		afterFirstDrive.Facts.Recovery.OperationRecoveryAttempts != 0 {
		t.Fatalf("governance first-effect edge was not persisted before retry: execution=%+v recovery=%+v", startedExecution, afterFirstDrive.Facts.Recovery)
	}
	exhaustedAggregate := afterFirstDrive
	exhaustedActive := *afterFirstDrive.ActiveDispatch
	exhaustedActive.OperationExecutions = make([]runtimecontract.RuntimeOperationExecutionRecord, len(afterFirstDrive.ActiveDispatch.OperationExecutions))
	for idx := range afterFirstDrive.ActiveDispatch.OperationExecutions {
		exhaustedActive.OperationExecutions[idx] = afterFirstDrive.ActiveDispatch.OperationExecutions[idx].Clone()
	}
	exhaustedAggregate.ActiveDispatch = &exhaustedActive
	exhaustedAggregate.ExecutionBudget = afterFirstDrive.ExecutionBudget.Clone()
	exhaustedAggregate.Facts.Recovery.OperationRecoveryAttempts = exhaustedAggregate.Facts.RecoveryPolicy.MaxOperationRecoveryAttempts
	exhaustedRepository := &controllerRepository{aggregate: &exhaustedAggregate}
	exhaustedController, err := NewRuntimeController(exhaustedRepository)
	if err != nil {
		t.Fatal(err)
	}
	exhaustedExecutor := &failIfExecutedOperationExecutor{}
	_, exhaustedErr := exhaustedController.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", exhaustedExecutor)
	var outcomeUnknown *OperationOutcomeUnknownError
	if !errors.As(exhaustedErr, &outcomeUnknown) || exhaustedExecutor.called {
		t.Fatalf("exhausted governance recovery reached owner: called=%v err=%v", exhaustedExecutor.called, exhaustedErr)
	}
	exhaustedState, err := exhaustedController.Load(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	exhaustedExecution, ok := operationExecutionRecord(exhaustedState.ActiveDispatch.OperationExecutions, governedOperationID)
	if !ok || exhaustedExecution.State != runtimecontract.OperationExecutionOutcomeUnknown {
		t.Fatalf("exhausted governance recovery did not fail closed: %+v", exhaustedExecution)
	}
	outcomeCASAggregate := afterFirstDrive
	outcomeCASActive := *afterFirstDrive.ActiveDispatch
	outcomeCASActive.OperationExecutions = make([]runtimecontract.RuntimeOperationExecutionRecord, len(afterFirstDrive.ActiveDispatch.OperationExecutions))
	for idx := range afterFirstDrive.ActiveDispatch.OperationExecutions {
		outcomeCASActive.OperationExecutions[idx] = afterFirstDrive.ActiveDispatch.OperationExecutions[idx].Clone()
	}
	outcomeCASAggregate.ActiveDispatch = &outcomeCASActive
	outcomeCASAggregate.ExecutionBudget = afterFirstDrive.ExecutionBudget.Clone()
	outcomeCASAggregate.Facts.Recovery.OperationRecoveryAttempts = outcomeCASAggregate.Facts.RecoveryPolicy.MaxOperationRecoveryAttempts
	outcomeCASRepository := &governanceCASFailRepository{aggregate: outcomeCASAggregate}
	outcomeCASController, err := NewRuntimeController(outcomeCASRepository)
	if err != nil {
		t.Fatal(err)
	}
	outcomeCASExecutor := &failIfExecutedOperationExecutor{}
	_, outcomeCASErr := outcomeCASController.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", outcomeCASExecutor)
	if !errors.Is(outcomeCASErr, common.ErrConflict) || outcomeCASExecutor.called {
		t.Fatalf("failed outcome_unknown CAS was hidden: called=%v err=%v", outcomeCASExecutor.called, outcomeCASErr)
	}
	outcomeCASExecution, ok := operationExecutionRecord(outcomeCASRepository.aggregate.ActiveDispatch.OperationExecutions, governedOperationID)
	if !ok || outcomeCASExecution.State != runtimecontract.OperationExecutionClaimed ||
		outcomeCASExecution.ClaimedAt != runtimecontract.RuntimeOperationClaimMarkerGovernanceEffectStarted {
		t.Fatalf("failed outcome_unknown CAS leaked an uncommitted state: %+v", outcomeCASExecution)
	}
	duplicateAfterStart, err := restarted.commitGovernanceResolution(context.Background(), operationResolution)
	if err != nil || duplicateAfterStart.Disposition != CommitDispositionDuplicate ||
		duplicateAfterStart.Aggregate.StateRevision != afterFirstDrive.StateRevision {
		t.Fatalf("duplicate resolution changed started governance resume: %+v err=%v", duplicateAfterStart, err)
	}
	duplicateExecution, ok := operationExecutionRecord(duplicateAfterStart.Aggregate.ActiveDispatch.OperationExecutions, governedOperationID)
	if !ok || duplicateExecution.ClaimedAt != runtimecontract.RuntimeOperationClaimMarkerGovernanceEffectStarted {
		t.Fatalf("duplicate resolution reset governance effect-start marker: %+v", duplicateExecution)
	}
	recoveredController, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	executor.owner = recoveredController
	resumed, err := recoveredController.executeActiveDispatch(context.Background(), "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatal(err)
	}
	if len(executor.requests) != 3 || executor.requests[0].GovernanceGrant == nil || executor.requests[1].GovernanceGrant == nil ||
		!runtimecontract.SameRuntimeOperationGovernanceGrant(*executor.requests[0].GovernanceGrant, *grant) ||
		!runtimecontract.SameRuntimeOperationGovernanceGrant(*executor.requests[1].GovernanceGrant, *grant) ||
		executor.requests[2].Operation.ID != siblingOperationID || executor.requests[2].GovernanceGrant != nil {
		t.Fatalf("resume executor requests = %+v", executor.requests)
	}
	if executor.requests[1].Execution.AdmissibilitySnapshotID != sealedCatalogSnapshotID ||
		executor.requests[1].Execution.AcquisitionBinding != sealedAcquisitionBinding || executor.requests[1].Execution.ReplayProof == nil ||
		executor.requests[1].OwnerCatalog == nil || executor.requests[2].Execution.AdmissibilitySnapshotID != sealedCatalogSnapshotID ||
		executor.requests[2].Execution.AcquisitionBinding != sealedAcquisitionBinding || executor.requests[2].Execution.ReplayProof == nil ||
		executor.requests[2].OwnerCatalog == nil {
		t.Fatalf("fresh-process governance resume lost sealed owner authority: target=%+v targetCatalog=%+v sibling=%+v siblingCatalog=%+v observations=%+v",
			executor.requests[1].Execution, executor.requests[1].OwnerCatalog, executor.requests[2].Execution,
			executor.requests[2].OwnerCatalog, afterFirstDrive.Facts.Resources.PrerequisiteObservations)
	}
	if resumed.Facts.Recovery.OperationRecoveryAttempts != 1 {
		t.Fatalf("governance replay recovery budget = %+v, want one consumed attempt", resumed.Facts.Recovery)
	}
	if len(resumed.OperationReceipts) != 4 {
		t.Fatalf("governance/resume receipts = %+v", resumed.OperationReceipts)
	}
	if _, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(resumed.OperationReceipts, execution.ExecutionKey, execution.Fence); !ok {
		t.Fatal("governance receipt was not preserved")
	}
	if receipt, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(resumed.OperationReceipts, sourceCompleted.ExecutionKey, sourceCompleted.Fence); !ok ||
		receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
		t.Fatalf("completed sibling receipt was not preserved: %+v found=%v", receipt, ok)
	}
	if receipt, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(resumed.OperationReceipts, execution.ExecutionKey, execution.Fence+1); !ok || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
		t.Fatalf("resume receipt = %+v found=%v", receipt, ok)
	}
	resumeAllocation, found = runtimeOperationBudgetAllocationForTest(resumed, resumeExecution)
	if !found || resumeAllocation.State != runtimecontract.RuntimeBudgetAllocationConfirmed {
		t.Fatalf("confirmed governance resume budget allocation = %+v found=%v", resumeAllocation, found)
	}
	completedAllocation, found := runtimeOperationBudgetAllocationForTest(resumed, sourceCompleted)
	if !found || completedAllocation.State != runtimecontract.RuntimeBudgetAllocationConfirmed {
		t.Fatalf("completed sibling budget was not preserved exactly once: %+v found=%v", completedAllocation, found)
	}
	completedAllocationCount := 0
	for _, allocation := range resumed.ExecutionBudget.Allocations {
		allocation = allocation.Normalized()
		if allocation.Dimension == runtimecontract.RuntimeBudgetDimensionCapabilityOperation &&
			allocation.LogicalConsumptionID == sourceCompleted.ExecutionKey &&
			allocation.ScopeRef == runtimeOperationBudgetScopeRef(sourceCompleted.ExecutionKey, sourceCompleted.Fence) {
			completedAllocationCount++
		}
	}
	if completedAllocationCount != 1 {
		t.Fatalf("completed sibling budget allocations = %d, want exactly one", completedAllocationCount)
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
