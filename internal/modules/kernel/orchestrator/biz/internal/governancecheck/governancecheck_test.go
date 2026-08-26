package governancecheck

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestGovernanceIdentityAndOutcomePredicates(t *testing.T) {
	facts := DispatchFacts{
		DecisionState: runtimecontract.ConvergenceStateNeedGovernance, DispatchID: "dispatch-1",
		AuthoritySnapshotID: "authority-1", CandidateGovernance: true,
		DependencyKind: "approval", SubjectRef: "target-1",
	}
	request := runtimecontract.RuntimeGovernanceRequestRecord{DispatchID: "dispatch-1", AuthoritySnapshotID: "authority-1", DependencyKind: "approval", SubjectRef: "target-1"}
	if !RequestMatches(request, facts) {
		t.Fatal("matching governance request was rejected")
	}
	request.SubjectRef = "other"
	if RequestMatches(request, facts) {
		t.Fatal("mismatched governance subject was accepted")
	}
	if !BindingStateAllowsOutcome(runtimecontract.RuntimeOperationGovernanceBindingTicketBound, runtimecontract.RuntimeGovernanceResolutionOutcomeContinue) || BindingStateAllowsOutcome(runtimecontract.RuntimeOperationGovernanceBindingSuspended, runtimecontract.RuntimeGovernanceResolutionOutcomeContinue) {
		t.Fatal("governance binding outcome predicate mismatch")
	}
}

func TestGovernanceRequestResolutionAndBindingFactsRemainExact(t *testing.T) {
	request := runtimecontract.RuntimeGovernanceRequestRecord{
		TicketID: "ticket-1", AuthoritySnapshotID: " authority-1 ", DispatchID: "dispatch-1",
		DependencyKind: "approval", SubjectRef: "target-1",
	}
	if !SameRequestRecord(request, runtimecontract.RuntimeGovernanceRequestRecord{
		TicketID: "ticket-1", AuthoritySnapshotID: "authority-1", DispatchID: " dispatch-1 ",
		DependencyKind: " approval ", SubjectRef: " target-1 ",
	}) {
		t.Fatal("normalized exact request replay was not recognized")
	}
	if !RequestRecordConflicts(request, runtimecontract.RuntimeGovernanceRequestRecord{
		TicketID: "ticket-other", AuthoritySnapshotID: "authority-1", DispatchID: "dispatch-1",
		DependencyKind: "approval", SubjectRef: "target-other",
	}) {
		t.Fatal("same authority/dispatch with another ticket was not a conflict")
	}
	resolution := runtimecontract.RuntimeGovernanceResolution{
		TicketID: "ticket-1", AuthoritySnapshotID: "authority-1", DispatchID: "dispatch-1",
		DependencyKind: "approval", SubjectRef: "target-1", Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue,
	}
	if !RequestBindsResolution(request, resolution) {
		t.Fatal("persisted request did not bind exact resolution")
	}
	if !RequestsBindResolution([]runtimecontract.RuntimeGovernanceRequestRecord{request}, resolution) {
		t.Fatal("persisted request set did not bind exact resolution")
	}
	resolution.AuthoritySnapshotID = "authority-stale"
	if RequestBindsResolution(request, resolution) || RequestsBindResolution([]runtimecontract.RuntimeGovernanceRequestRecord{request}, resolution) {
		t.Fatal("stale authority resolution matched persisted request")
	}
	resolution.AuthoritySnapshotID = "authority-1"
	binding := runtimecontract.RuntimeOperationGovernanceBinding{
		TicketID: "ticket-1", GovernanceDispatchID: "dispatch-1", GovernanceSnapshotID: "authority-1",
		DependencyKind: "approval", SubjectRef: "target-1", State: runtimecontract.RuntimeOperationGovernanceBindingTicketBound,
	}
	if !BindingMatchesResolution(binding, resolution) || !BindingStateAllowsOutcome(binding.State, resolution.Outcome) {
		t.Fatal("ticket-bound binding did not match supported resolution")
	}
	binding.State = runtimecontract.RuntimeOperationGovernanceBindingSuspended
	binding.TicketID, binding.GovernanceDispatchID, binding.GovernanceSnapshotID = "", "", ""
	if !SuspendedBindingMatchesRequest(binding, request) {
		t.Fatal("suspended binding did not match request dependency")
	}
	request.SubjectRef = "other-target"
	if SuspendedBindingMatchesRequest(binding, request) {
		t.Fatal("mismatched dependency promoted a suspended binding")
	}
}

func TestExternalDependencyComparisonIncludesReason(t *testing.T) {
	left := runtimecontract.ExternalDependency{Kind: "approval", SubjectRef: "target-1", Reason: " owner reason "}
	right := runtimecontract.ExternalDependency{Kind: " approval ", SubjectRef: " target-1 ", Reason: "owner reason"}
	if !ExternalDependencyMatches(left, right) {
		t.Fatal("normalized sealed dependencies did not match")
	}
	right.Reason = "different reason"
	if ExternalDependencyMatches(left, right) {
		t.Fatal("different dependency reason was ignored")
	}
}

func TestGovernanceRequestLookupsUseExactNormalizedIdentity(t *testing.T) {
	requests := []runtimecontract.RuntimeGovernanceRequestRecord{{
		TicketID: "ticket-1", AuthoritySnapshotID: " authority-1 ", DispatchID: " dispatch-1 ",
		DependencyKind: " approval ", SubjectRef: " target-1 ",
	}}
	byTicket, found := RequestForTicket(requests, common.TicketID("ticket-1"))
	if !found || byTicket.DispatchID != "dispatch-1" || byTicket.AuthoritySnapshotID != "authority-1" || byTicket.DependencyKind != "approval" {
		t.Fatalf("ticket lookup = %+v found=%v", byTicket, found)
	}
	byDispatch, found := RequestForDispatch(requests, "dispatch-1", "authority-1")
	if !found || byDispatch.TicketID != "ticket-1" || byDispatch.SubjectRef != "target-1" {
		t.Fatalf("dispatch lookup = %+v found=%v", byDispatch, found)
	}
	if _, found := RequestForDispatch(requests, "dispatch-1", "authority-stale"); found {
		t.Fatal("stale authority matched persisted governance request")
	}
	if !RequestDispatchMatches(byDispatch, DispatchFacts{DispatchID: "dispatch-1", AuthoritySnapshotID: "authority-1", CandidateGovernance: true}) ||
		RequestDispatchMatches(byDispatch, DispatchFacts{DispatchID: "dispatch-1", AuthoritySnapshotID: "authority-1"}) {
		t.Fatal("request/governed-dispatch identity predicate mismatch")
	}
}

func TestEvaluateOperationSuspensionRequiresExactDurableReceipt(t *testing.T) {
	requirement := runtimecontract.RuntimeGovernanceRequirement{
		Kind: runtimecontract.GovernanceApproval, SubjectRef: " target.txt ", Reason: " owner approval required ",
		RequestedAction:      "file/write",
		AuthorizationContext: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "target.txt")),
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "execution-1", Fence: 3, OperationID: "write-1",
		Outcome:             runtimecontract.RuntimeOperationOutcomeGovernanceRequired,
		ObservedEffectScope: runtimecontract.RuntimeEffectGovernanceOnly,
		Governance:          &requirement,
	}
	receipt = receipt.Normalized()
	execution := runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: "write-1", OperationHash: runtimecontract.StableRuntimeOperationHash(runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite}), ExecutionKey: "execution-1", Fence: 3,
		ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		IdempotencyClass:    runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		State:               runtimecontract.OperationExecutionOwnerCommitted,
		ClaimedAt:           runtimecontract.RuntimeOperationClaimMarkerInitial,
		OwnerReceiptRef:     "runtime-receipt/execution-1",
		OwnerReceiptHash:    runtimecontract.StableRuntimeOperationReceiptHash(receipt),
	}
	execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(execution)
	finding, err := EvaluateOperationSuspension(" dispatch-1 ", execution, receipt, receipt, requirement)
	if err != nil {
		t.Fatal(err)
	}
	if finding.Binding.OperationDispatchID != "dispatch-1" || finding.Binding.ExecutionKey != execution.ExecutionKey ||
		finding.Binding.Fence != execution.Fence || finding.Binding.State != runtimecontract.RuntimeOperationGovernanceBindingSuspended ||
		finding.Dependency.Kind != "approval" || finding.Dependency.SubjectRef != "target.txt" || finding.Dependency.Reason != "owner approval required" {
		t.Fatalf("operation suspension finding = %+v", finding)
	}
	if !BindingMatchesExecution(finding.Binding, execution) || !BindingMatchesReceipt(finding.Binding, receipt) {
		t.Fatal("suspension binding did not match its exact execution and receipt")
	}
	if finding.Binding.IdempotencyClass != runtimecontract.RuntimeOperationIdempotencyAtMostOnce {
		t.Fatalf("suspension binding lost sealed idempotency: %+v", finding.Binding)
	}
	driftedScope := finding.Binding
	driftedScope.ExpectedEffectScope = runtimecontract.RuntimeEffectRemoteDurable
	if BindingMatchesExecution(driftedScope, execution) {
		t.Fatal("suspension binding matched a different sealed effect scope")
	}
	driftedIdempotency := finding.Binding
	driftedIdempotency.IdempotencyClass = runtimecontract.RuntimeOperationIdempotencySafeToReplay
	if BindingMatchesExecution(driftedIdempotency, execution) {
		t.Fatal("suspension binding matched a different sealed idempotency class")
	}
	bound := finding.Binding
	bound.ApprovalID = "approval-1"
	bound.GovernanceDispatchID = "governance-dispatch-1"
	bound.GovernanceSnapshotID = "authority-1"
	request := runtimecontract.RuntimeGovernanceRequestRecord{
		ApprovalID: "approval-1", DispatchID: "governance-dispatch-1", AuthoritySnapshotID: "authority-1",
		DependencyKind: "approval", SubjectRef: "target.txt", RequestedAction: "file/write",
		AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(requirement.AuthorizationContext),
	}
	if !BindingMatchesRequest(bound, request) {
		t.Fatal("approval-bound suspension did not match exact request")
	}
	resolution := runtimecontract.RuntimeGovernanceResolutionRecord{
		ResolutionID: "resolution-1", ApprovalID: "approval-1", DispatchID: "governance-dispatch-1", AuthoritySnapshotID: "authority-1",
		DependencyKind: "approval", SubjectRef: "target.txt",
	}
	if !ResolutionRecordMatchesRequest(resolution, request) || !ResolutionRecordMatchesDispatch(resolution, DispatchFacts{
		DispatchID: "governance-dispatch-1", AuthoritySnapshotID: "authority-1", CandidateGovernance: true,
	}) {
		t.Fatal("governance resolution record identity did not match request/dispatch")
	}
	drifted := receipt.Clone()
	drifted.Governance.Reason = "different reason"
	if _, err := EvaluateOperationSuspension("dispatch-1", execution, receipt, drifted, requirement); err == nil {
		t.Fatal("conflicting owner response matched durable governance receipt")
	}
	stale := execution
	stale.Fence++
	if _, err := EvaluateOperationSuspension("dispatch-1", stale, receipt, receipt, requirement); err == nil {
		t.Fatal("stale execution fence matched governance receipt")
	}
}

func TestGrantForExecutionReturnsDetachedExactResumeGrant(t *testing.T) {
	grant := runtimecontract.RuntimeOperationGovernanceGrant{
		ResolutionID: "resolution-1", ResolutionHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TicketID: "ticket-1", ExecutionKey: "execution-1",
		GovernanceFence: 2, ResumeFence: 3, ResumeDispatchID: "resume-dispatch-1", DependencyKind: "approval", SubjectRef: "target.txt",
		GovernanceReceiptHash: "receipt-hash", EvidenceRefs: []string{"decision://resolution-1"},
	}
	execution := runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: "write-1", OperationHash: runtimecontract.StableRuntimeOperationHash(runtimecontract.RuntimeOperation{ID: "write-1", Kind: runtimecontract.StepKindFileWrite}), ExecutionKey: "execution-1", Fence: 3,
		ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		IdempotencyClass:    runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}
	execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(execution)
	bindings := []runtimecontract.RuntimeOperationGovernanceBinding{{
		OperationID: "write-1", ExecutionKey: "execution-1", ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		IdempotencyClass:       runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		ExecutionAuthorityHash: execution.AuthorityHash,
		State:                  runtimecontract.RuntimeOperationGovernanceBindingGranted, Grant: &grant,
	}}
	found := GrantForExecution(bindings, execution)
	if found == nil || !SameGrant(*found, grant) {
		t.Fatalf("grant = %+v", found)
	}
	found.EvidenceRefs[0] = "mutated"
	if bindings[0].Grant.EvidenceRefs[0] != "decision://resolution-1" {
		t.Fatal("returned grant aliases persisted binding")
	}
	driftedOperation := execution
	driftedOperation.OperationID = "write-2"
	if GrantForExecution(bindings, driftedOperation) != nil {
		t.Fatal("grant matched a different operation")
	}
	driftedScope := execution
	driftedScope.ExpectedEffectScope = runtimecontract.RuntimeEffectRemoteDurable
	if GrantForExecution(bindings, driftedScope) != nil {
		t.Fatal("grant matched a different effect scope")
	}
	driftedIdempotency := execution
	driftedIdempotency.IdempotencyClass = runtimecontract.RuntimeOperationIdempotencySafeToReplay
	if GrantForExecution(bindings, driftedIdempotency) != nil {
		t.Fatal("grant matched a different idempotency class")
	}
	execution.Fence = 4
	if GrantForExecution(bindings, execution) != nil {
		t.Fatal("grant matched a different resume fence")
	}
}

func TestDependencyForFactsPreservesSuspensionAndProviderPolicy(t *testing.T) {
	authorization := workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "target.txt"))
	facts := runtimecontract.RuntimeFacts{Failure: &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceRequired, Message: "approval required",
		GovernanceDependency: &runtimecontract.ExternalDependency{
			Kind: "approval", SubjectRef: "target.txt", Reason: "approval required", RequestedAction: "file/write", AuthorizationContext: authorization,
		},
	}}
	bindings := []runtimecontract.RuntimeOperationGovernanceBinding{{
		DependencyKind: "approval", SubjectRef: "target.txt", State: runtimecontract.RuntimeOperationGovernanceBindingSuspended,
		RequestedAction: "file/write", AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(authorization),
	}}
	dependency, found := DependencyForFacts(facts, bindings)
	if !found || dependency.Kind != "approval" || dependency.SubjectRef != "target.txt" || dependency.Reason != "approval required" {
		t.Fatalf("suspension dependency = %+v found=%v", dependency, found)
	}
	facts.Failure.GovernanceDependency.SubjectRef = "other.txt"
	if _, found := DependencyForFacts(facts, bindings); found {
		t.Fatal("mismatched failure dependency was accepted for suspended operation")
	}
	providerFacts := runtimecontract.RuntimeFacts{
		Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassProviderAuthentication},
		Resources: runtimecontract.RuntimeResourceFacts{ProviderCeiling: &runtimecontract.ProviderCeiling{
			Selection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "provider-1", ModelID: "model-1"}},
		}},
	}
	dependency, found = DependencyForFacts(providerFacts, nil)
	if !found || dependency.Kind != string(runtimecontract.GovernanceConfiguration) || dependency.SubjectRef != "provider-1/model-1" {
		t.Fatalf("provider dependency = %+v found=%v", dependency, found)
	}
}
