package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type catalogOnlyShellGovernanceFixture struct {
	controller        *RuntimeController
	repository        *controllerRepository
	aggregate         RuntimeAggregate
	firstBinding      runtimecontract.RuntimeOperationGovernanceBinding
	firstResolution   runtimecontract.RuntimeGovernanceResolution
	policy            runtimecontract.RuntimeOperationPolicyFact
	firstOperationID  string
	secondOperationID string
}

func newCatalogOnlyShellGovernanceFixture(t *testing.T) catalogOnlyShellGovernanceFixture {
	t.Helper()
	ctx := context.Background()
	stateChanging := true
	policy := runtimecontract.RuntimeOperationPolicyFact{
		ID: "shell/catalog-only-durable", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKind:    runtimecontract.StepKindShellCommand,
		ParameterMatches: []runtimecontract.RuntimeOperationParameterMatch{{Name: "stateChanging", Boolean: &stateChanging}},
		EffectScope:      runtimecontract.RuntimeEffectWorkspaceDurable,
		IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:shell-governance", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.StepKindShellCommand}, TargetRefs: []string{"a.txt", "b.txt"},
		OperationPolicies: []runtimecontract.RuntimeOperationPolicyFact{policy}, ObservedRevision: "shell-governance-v1",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: catalog.OwnerModuleID, ContentHash: catalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleShell,
			SubjectRef: "shell_runtime_policy", Revision: catalog.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.StepKindShellCommand},
	}

	facts := controllerFacts()
	facts.Focus.TargetRefs = []string{"a.txt", "b.txt"}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindShellCommand}
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindShellCommand}
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindApprovalRequest}
	facts.Activation.OperationPolicies = nil
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}

	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindShellCommand}
	initialized := runtimeTestInitializeAndSealSemanticContract(t, ctx, controller, facts, definition, nil)
	facts = initialized.Facts
	command := "sh"
	argsA := []string{"-c", "printf a > a.txt"}
	argsB := []string{"-c", "printf b > b.txt"}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{
		{Operation: runtimecontract.RuntimeOperation{
			Kind:                runtimecontract.StepKindShellCommand,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"a.txt"},
			Parameters: []runtimecontract.OperationParameter{
				{Name: "command", String: &command}, {Name: "args", StringList: argsA}, {Name: "stateChanging", Boolean: &stateChanging},
			},
		}},
		{Operation: runtimecontract.RuntimeOperation{
			Kind:                runtimecontract.StepKindShellCommand,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"b.txt"},
			Parameters: []runtimecontract.OperationParameter{
				{Name: "command", String: &command}, {Name: "args", StringList: argsB}, {Name: "stateChanging", Boolean: &stateChanging},
			},
		}},
	}}}
	sealed, err := beginAndAcceptCandidate(t, controller, initialized, candidate)
	if err != nil {
		t.Fatal(err)
	}
	firstOperationID := runtimeTestActiveOperationID(t, sealed, 0)
	secondOperationID := runtimeTestActiveOperationID(t, sealed, 1)
	claimed, err := controller.claimOperationExecution(ctx, facts.WorkspaceID, facts.RunID, firstOperationID)
	if err != nil {
		t.Fatal(err)
	}
	execution, found := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, firstOperationID)
	if !found {
		t.Fatal("first Shell execution is missing")
	}
	requirement := runtimecontract.RuntimeGovernanceRequirement{
		Kind: runtimecontract.GovernanceApproval, SubjectRef: "a.txt",
		Reason: "first shell mutation requires approval", RequestedAction: "shell/command", RiskLevel: "high",
		AuthorizationContext: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "a.txt")),
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeGovernanceRequired, ObservedEffectScope: runtimecontract.RuntimeEffectGovernanceOnly,
		Governance: &requirement,
	}
	_, committed, err := controller.commitOperationReceipt(ctx, facts.WorkspaceID, facts.RunID, sealed.ActiveDispatch.ID, execution.Fence, receipt)
	if err != nil {
		t.Fatal(err)
	}
	governanceAggregate, err := controller.candidateDispatch.recordOperationGovernanceRequired(ctx, facts.WorkspaceID, facts.RunID, firstOperationID, RuntimeOperationExecutionResult{
		EventID: "governance-shell-1", OperationID: firstOperationID, Receipt: committed, Governance: &requirement,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		governanceAggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(governanceAggregate.Facts.Resources),
	)
	if err != nil {
		t.Fatalf("build first governance manifest: %v decision=%+v failure=%+v", err, governanceAggregate.Decision, governanceAggregate.Facts.Failure)
	}
	governed, err := controller.sealGovernanceDispatch(ctx, facts.WorkspaceID, facts.RunID, manifest, runtimecontract.RuntimeCandidate{
		Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: runtimecontract.ExternalDependency{
			Kind: string(requirement.Kind), SubjectRef: requirement.SubjectRef, Reason: requirement.Reason,
			RequestedAction: requirement.RequestedAction, RiskLevel: requirement.RiskLevel,
			AuthorizationContext: requirement.AuthorizationContext.Clone(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := controller.bindGovernanceRequest(ctx, facts.WorkspaceID, facts.RunID, runtimecontract.RuntimeGovernanceRequestRecord{
		ApprovalID: "approval-shell-1", AuthoritySnapshotID: governed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID: governed.ActiveDispatch.ID, DependencyKind: string(requirement.Kind), SubjectRef: requirement.SubjectRef,
		RequestedAction: requirement.RequestedAction, RiskLevel: requirement.RiskLevel,
		AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(requirement.AuthorizationContext),
	})
	if err != nil {
		t.Fatal(err)
	}
	resolution := runtimecontract.RuntimeGovernanceResolution{
		ResolutionID: "resolution-shell-1", ApprovalID: "approval-shell-1", WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
		AuthoritySnapshotID: governed.ActiveDispatch.Transaction.AuthoritySnapshotID, DispatchID: governed.ActiveDispatch.ID,
		DependencyKind: string(requirement.Kind), SubjectRef: requirement.SubjectRef,
		Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "first shell approval granted",
	}
	continued, err := controller.commitGovernanceResolution(ctx, resolution)
	if err != nil {
		t.Fatal(err)
	}
	if len(bound.OperationGovernanceBindings) != 1 || len(continued.Aggregate.OperationGovernanceBindings) != 1 {
		t.Fatalf("first Shell governance binding was not preserved: bound=%+v continued=%+v", bound.OperationGovernanceBindings, continued.Aggregate.OperationGovernanceBindings)
	}
	return catalogOnlyShellGovernanceFixture{
		controller: controller, repository: repository, aggregate: continued.Aggregate,
		firstBinding: continued.Aggregate.OperationGovernanceBindings[0], firstResolution: resolution, policy: policy,
		firstOperationID: firstOperationID, secondOperationID: secondOperationID,
	}
}

func TestRejectedGovernanceResumeReceiptBindsResumeDispatchAcrossFreshLoad(t *testing.T) {
	ctx := context.Background()
	fixture := newCatalogOnlyShellGovernanceFixture(t)
	resume, execution, receipt := rejectedGovernanceResumeReceiptForTest(t, &fixture)

	originDispatchID, err := runtimecontract.RuntimeOperationExecutionKeyOrigin(
		fixture.aggregate.Facts.WorkspaceID,
		fixture.aggregate.Facts.RunID,
		execution.ExecutionKey,
		execution.OperationID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if originDispatchID == resume.ID || receipt.Feedback == nil || receipt.Feedback.DispatchID != resume.ID {
		t.Fatalf("governance-resume regression fixture lost origin/current split: origin=%q resume=%q feedback=%+v", originDispatchID, resume.ID, receipt.Feedback)
	}

	committed, persisted, err := fixture.controller.commitOperationReceipt(
		ctx,
		fixture.aggregate.Facts.WorkspaceID,
		fixture.aggregate.Facts.RunID,
		resume.ID,
		execution.Fence,
		receipt,
	)
	if err != nil {
		t.Fatalf("commit rejected governance-resume receipt: %v", err)
	}
	committedExecution, found := operationExecutionRecord(committed.ActiveDispatch.OperationExecutions, execution.OperationID)
	if !found || committedExecution.State != runtimecontract.OperationExecutionOwnerCommitted ||
		committedExecution.ExecutionKey != execution.ExecutionKey || committedExecution.Fence != execution.Fence ||
		committedExecution.OwnerReceiptRef != "runtime-receipt/"+execution.ExecutionKey ||
		committedExecution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(persisted) {
		t.Fatalf("rejected governance-resume receipt projection = %+v", committedExecution)
	}

	reloaded := governanceResumeAggregateJSONRoundTrip(t, committed)
	if err := reloaded.Validate(); err != nil {
		t.Fatalf("fresh-load rejected governance-resume receipt: %v", err)
	}
}

func TestRejectedGovernanceResumeReceiptRejectsOriginOrUnknownFeedbackDispatch(t *testing.T) {
	for _, test := range []struct {
		name          string
		feedbackScope func(originDispatchID string) string
	}{
		{name: "execution-key origin", feedbackScope: func(originDispatchID string) string { return originDispatchID }},
		{name: "unknown dispatch", feedbackScope: func(string) string { return "dispatch-other" }},
	} {
		t.Run(test.name+" at commit", func(t *testing.T) {
			fixture := newCatalogOnlyShellGovernanceFixture(t)
			resume, execution, receipt := rejectedGovernanceResumeReceiptForTest(t, &fixture)
			originDispatchID, err := runtimecontract.RuntimeOperationExecutionKeyOrigin(
				fixture.aggregate.Facts.WorkspaceID,
				fixture.aggregate.Facts.RunID,
				execution.ExecutionKey,
				execution.OperationID,
			)
			if err != nil {
				t.Fatal(err)
			}
			receipt.Feedback.DispatchID = test.feedbackScope(originDispatchID)
			revision, saves, receiptCount := fixture.aggregate.StateRevision, fixture.repository.saves, len(fixture.aggregate.OperationReceipts)
			if _, _, err := fixture.controller.commitOperationReceipt(
				context.Background(), fixture.aggregate.Facts.WorkspaceID, fixture.aggregate.Facts.RunID,
				resume.ID, execution.Fence, receipt,
			); err == nil || !strings.Contains(err.Error(), "operation receipt feedback does not match receipt identity") {
				t.Fatalf("feedback dispatch %q commit error = %v", receipt.Feedback.DispatchID, err)
			}
			if fixture.repository.aggregate.StateRevision != revision || fixture.repository.saves != saves || len(fixture.repository.aggregate.OperationReceipts) != receiptCount {
				t.Fatalf("drifted governance-resume receipt changed aggregate: revision=%d saves=%d receipts=%d", fixture.repository.aggregate.StateRevision, fixture.repository.saves, len(fixture.repository.aggregate.OperationReceipts))
			}
		})

		t.Run(test.name+" at fresh load", func(t *testing.T) {
			fixture := newCatalogOnlyShellGovernanceFixture(t)
			resume, execution, receipt := rejectedGovernanceResumeReceiptForTest(t, &fixture)
			originDispatchID, err := runtimecontract.RuntimeOperationExecutionKeyOrigin(
				fixture.aggregate.Facts.WorkspaceID,
				fixture.aggregate.Facts.RunID,
				execution.ExecutionKey,
				execution.OperationID,
			)
			if err != nil {
				t.Fatal(err)
			}
			committed, _, err := fixture.controller.commitOperationReceipt(
				context.Background(), fixture.aggregate.Facts.WorkspaceID, fixture.aggregate.Facts.RunID,
				resume.ID, execution.Fence, receipt,
			)
			if err != nil {
				t.Fatal(err)
			}

			badReceipt := receipt.Clone()
			badReceipt.Fence = execution.Fence
			badReceipt.Feedback.DispatchID = test.feedbackScope(originDispatchID)
			badReceipt = badReceipt.Normalized()
			for index := range committed.OperationReceipts {
				if committed.OperationReceipts[index].ExecutionKey == badReceipt.ExecutionKey && committed.OperationReceipts[index].Fence == badReceipt.Fence {
					committed.OperationReceipts[index] = badReceipt
				}
			}
			for index := range committed.ActiveDispatch.OperationExecutions {
				projection := &committed.ActiveDispatch.OperationExecutions[index]
				if projection.ExecutionKey == badReceipt.ExecutionKey && projection.Fence == badReceipt.Fence && projection.OperationID == badReceipt.OperationID {
					projection.OwnerReceiptHash = runtimecontract.StableRuntimeOperationReceiptHash(badReceipt)
				}
			}
			reloaded := governanceResumeAggregateJSONRoundTrip(t, committed)
			if err := reloaded.Validate(); err == nil || !strings.Contains(err.Error(), "does not match an exact committed execution projection") {
				t.Fatalf("feedback dispatch %q fresh-load error = %v", badReceipt.Feedback.DispatchID, err)
			}
		})
	}
}

func rejectedGovernanceResumeReceiptForTest(t *testing.T, fixture *catalogOnlyShellGovernanceFixture) (SealedDispatch, runtimecontract.RuntimeOperationExecutionRecord, runtimecontract.RuntimeOperationReceipt) {
	t.Helper()
	if fixture.aggregate.ActiveDispatch == nil {
		t.Fatal("governance continuation has no resume dispatch")
	}
	started, grant, err := fixture.controller.candidateDispatch.markGovernanceResumeExecutionStarted(
		context.Background(), fixture.aggregate.Facts.WorkspaceID, fixture.aggregate.Facts.RunID, fixture.firstOperationID,
	)
	if err != nil || grant == nil {
		t.Fatalf("mark rejected governance-resume execution started: grant=%+v err=%v", grant, err)
	}
	fixture.aggregate = started
	resume := *started.ActiveDispatch
	execution, found := operationExecutionRecord(resume.OperationExecutions, fixture.firstOperationID)
	if !found || execution.State != runtimecontract.OperationExecutionClaimed || execution.Fence == 0 ||
		execution.ClaimedAt != runtimecontract.RuntimeOperationClaimMarkerGovernanceEffectStarted {
		t.Fatalf("governance resume execution = %+v", execution)
	}
	failure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionRejected, Code: "governed_owner_rejected",
		Signature: "governed-owner-rejected", Message: "governed owner rejected the resumed operation",
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: execution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeRejected, ObservedEffectScope: execution.ExpectedEffectScope,
		Feedback: runtimeOperationFeedbackForTest(t, fixture.aggregate, execution.OperationID, failure),
	}
	return resume, execution, receipt
}

func governanceResumeAggregateJSONRoundTrip(t *testing.T, aggregate RuntimeAggregate) RuntimeAggregate {
	t.Helper()
	payload, err := json.Marshal(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeAggregate
	if err := json.Unmarshal(payload, &reloaded); err != nil {
		t.Fatal(err)
	}
	return reloaded
}

func TestGovernanceResumeUsesCatalogOnlyDynamicOwnerPolicyForPendingSibling(t *testing.T) {
	fixture := newCatalogOnlyShellGovernanceFixture(t)
	if _, found := runtimecontract.RuntimeOperationPolicyForOperation(
		runtimecontract.RuntimeOperation{Kind: runtimecontract.StepKindShellCommand},
		fixture.aggregate.Facts.Activation.OperationPolicies,
	); found {
		t.Fatal("Shell policy unexpectedly existed in the activation seed")
	}
	effective := runtimecontract.RuntimeActivationForFacts(fixture.aggregate.Facts)
	if len(effective.OperationPolicies) != 1 || effective.OperationPolicies[0].ID != fixture.policy.ID {
		t.Fatalf("catalog-only Shell policy was not projected into effective activation: %+v", effective.OperationPolicies)
	}
	resume, err := buildGovernanceResumeDispatch(fixture.aggregate, fixture.firstBinding)
	if err != nil {
		t.Fatalf("catalog-only Shell policy was misclassified as pending sibling drift: %v", err)
	}
	pending, found := operationExecutionRecord(resume.OperationExecutions, fixture.secondOperationID)
	if !found || pending.ExpectedEffectScope != fixture.policy.EffectScope || pending.IdempotencyClass != fixture.policy.IdempotencyClass {
		t.Fatalf("pending sibling lost catalog-only policy authority: %+v", pending)
	}
}

func TestSequentialGovernanceResumePreservesCompletedGovernedExecution(t *testing.T) {
	ctx := context.Background()
	fixture := newCatalogOnlyShellGovernanceFixture(t)
	firstResumeID := fixture.aggregate.ActiveDispatch.ID
	firstExecution, found := operationExecutionRecord(fixture.aggregate.ActiveDispatch.OperationExecutions, fixture.firstOperationID)
	if !found {
		t.Fatal("first governed execution is missing from its resume dispatch")
	}
	started, grant, err := fixture.controller.candidateDispatch.markGovernanceResumeExecutionStarted(
		ctx, fixture.aggregate.Facts.WorkspaceID, fixture.aggregate.Facts.RunID, fixture.firstOperationID,
	)
	if err != nil || grant == nil {
		t.Fatalf("mark first governed execution started: grant=%+v err=%v", grant, err)
	}
	firstExecution, found = operationExecutionRecord(started.ActiveDispatch.OperationExecutions, fixture.firstOperationID)
	if !found {
		t.Fatal("started first governed execution is missing")
	}
	firstReceipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: firstExecution.ExecutionKey, Fence: firstExecution.Fence, OperationID: firstExecution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: firstExecution.ExpectedEffectScope,
		Changed: false, TargetRefs: []string{"a.txt"},
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "first governed shell operation completed without CWI credit"},
	}
	ownerCommitted, _, err := fixture.controller.commitOperationReceipt(
		ctx, started.Facts.WorkspaceID, started.Facts.RunID, firstResumeID, firstExecution.Fence, firstReceipt,
	)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the CWI open so this fixture can exercise a second governance edge.
	// The receipt and execution budget use production transitions; only the
	// already-judged evidence marker advances without crediting the CWI.
	runtimeTestCommitAcceptedReceiptEvidence(t, &ownerCommitted, fixture.firstOperationID)
	fixture.repository.aggregate = &ownerCommitted
	claimed, err := fixture.controller.claimOperationExecution(ctx, ownerCommitted.Facts.WorkspaceID, ownerCommitted.Facts.RunID, fixture.secondOperationID)
	if err != nil {
		t.Fatal(err)
	}
	secondExecution, found := operationExecutionRecord(claimed.ActiveDispatch.OperationExecutions, fixture.secondOperationID)
	if !found {
		t.Fatal("second Shell execution is missing")
	}
	secondRequirement := runtimecontract.RuntimeGovernanceRequirement{
		Kind: runtimecontract.GovernanceApproval, SubjectRef: "b.txt",
		Reason: "second shell mutation requires approval", RequestedAction: "shell/command", RiskLevel: "high",
		AuthorizationContext: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "b.txt")),
	}
	secondReceipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: secondExecution.ExecutionKey, Fence: secondExecution.Fence, OperationID: secondExecution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeGovernanceRequired, ObservedEffectScope: runtimecontract.RuntimeEffectGovernanceOnly,
		Governance: &secondRequirement,
	}
	_, committed, err := fixture.controller.commitOperationReceipt(
		ctx, claimed.Facts.WorkspaceID, claimed.Facts.RunID, firstResumeID, secondExecution.Fence, secondReceipt,
	)
	if err != nil {
		t.Fatal(err)
	}
	progressed, err := fixture.controller.candidateDispatch.recordOperationGovernanceRequired(ctx, claimed.Facts.WorkspaceID, claimed.Facts.RunID, fixture.secondOperationID, RuntimeOperationExecutionResult{
		EventID: "governance-shell-2", OperationID: fixture.secondOperationID, Receipt: committed, Governance: &secondRequirement,
	})
	if err != nil {
		t.Fatal(err)
	}
	if progressed.Decision.State != runtimecontract.ConvergenceStateNeedGovernance || progressed.ActiveDispatch != nil {
		t.Fatalf("second operation did not enter governance: %+v", progressed)
	}
	var source *SealedDispatch
	for idx := range progressed.CompletedDispatches {
		if progressed.CompletedDispatches[idx].ID == firstResumeID {
			value := progressed.CompletedDispatches[idx]
			source = &value
			break
		}
	}
	if source == nil {
		t.Fatalf("first resume dispatch %q was not completed", firstResumeID)
	}
	firstCompleted, found := operationExecutionRecord(source.OperationExecutions, fixture.firstOperationID)
	if !found || firstCompleted.State != runtimecontract.OperationExecutionEvidenceCommitted {
		t.Fatalf("first governed execution did not commit evidence before second governance: %+v", firstCompleted)
	}

	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		progressed.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(progressed.Facts.Resources),
	)
	if err != nil {
		t.Fatal(err)
	}
	requirement := progressed.Facts.Failure.GovernanceDependency
	governed, err := fixture.controller.sealGovernanceDispatch(ctx, progressed.Facts.WorkspaceID, progressed.Facts.RunID, manifest, runtimecontract.RuntimeCandidate{
		Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: requirement.Clone()},
	})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := fixture.controller.bindGovernanceRequest(ctx, progressed.Facts.WorkspaceID, progressed.Facts.RunID, runtimecontract.RuntimeGovernanceRequestRecord{
		ApprovalID: "approval-shell-2", AuthoritySnapshotID: governed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID: governed.ActiveDispatch.ID, DependencyKind: requirement.Kind, SubjectRef: requirement.SubjectRef,
		RequestedAction: requirement.RequestedAction, RiskLevel: requirement.RiskLevel,
		AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(requirement.AuthorizationContext),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bound.OperationGovernanceBindings) != 2 {
		t.Fatalf("second governance request lost first binding: %+v", bound.OperationGovernanceBindings)
	}
	resolution := runtimecontract.RuntimeGovernanceResolution{
		ResolutionID: "resolution-shell-2", ApprovalID: "approval-shell-2",
		WorkspaceID: progressed.Facts.WorkspaceID, RunID: progressed.Facts.RunID,
		AuthoritySnapshotID: governed.ActiveDispatch.Transaction.AuthoritySnapshotID, DispatchID: governed.ActiveDispatch.ID,
		DependencyKind: requirement.Kind, SubjectRef: requirement.SubjectRef,
		Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "second shell approval granted",
	}
	continued, err := fixture.controller.commitGovernanceResolution(ctx, resolution)
	if err != nil {
		t.Fatal(err)
	}
	if continued.Aggregate.ActiveDispatch == nil {
		t.Fatal("second governance continuation did not create an active resume dispatch")
	}
	cloned, found := operationExecutionRecord(continued.Aggregate.ActiveDispatch.OperationExecutions, fixture.firstOperationID)
	if !found || cloned.State != runtimecontract.OperationExecutionEvidenceCommitted ||
		cloned.ExecutionKey != firstCompleted.ExecutionKey || cloned.Fence != firstCompleted.Fence ||
		cloned.OwnerReceiptRef != firstCompleted.OwnerReceiptRef || cloned.OwnerReceiptHash != firstCompleted.OwnerReceiptHash ||
		cloned.AuthorityHash != firstCompleted.AuthorityHash || cloned.IdempotencyClass != firstCompleted.IdempotencyClass {
		t.Fatalf("second governance resume did not clone the completed governed execution: source=%+v clone=%+v", firstCompleted, cloned)
	}
	if err := continued.Aggregate.Validate(); err != nil {
		t.Fatalf("aggregate with sequential governance resumes is invalid: %v", err)
	}
	activeID := continued.Aggregate.ActiveDispatch.ID
	duplicate, err := fixture.controller.commitGovernanceResolution(ctx, fixture.firstResolution)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.ActiveDispatch == nil || duplicate.Aggregate.ActiveDispatch.ID != activeID {
		t.Fatalf("old governance resolution reopened or replaced the second resume: duplicate=%+v err=%v", duplicate, err)
	}
}

func TestSameOperationCanRequireGovernanceAgainAtNextFence(t *testing.T) {
	ctx := context.Background()
	fixture := newCatalogOnlyShellGovernanceFixture(t)
	firstGrant := fixture.firstBinding.Grant
	if firstGrant == nil || fixture.aggregate.ActiveDispatch == nil {
		t.Fatal("first governance continuation is missing")
	}
	firstResumeID := fixture.aggregate.ActiveDispatch.ID
	started, startedGrant, err := fixture.controller.candidateDispatch.markGovernanceResumeExecutionStarted(
		ctx, fixture.aggregate.Facts.WorkspaceID, fixture.aggregate.Facts.RunID, fixture.firstOperationID,
	)
	if err != nil || startedGrant == nil || !runtimecontract.SameRuntimeOperationGovernanceGrant(*startedGrant, *firstGrant) {
		t.Fatalf("mark first same-operation governance resume started: grant=%+v err=%v", startedGrant, err)
	}
	firstResumeExecution, found := operationExecutionRecord(started.ActiveDispatch.OperationExecutions, fixture.firstOperationID)
	if !found || firstResumeExecution.Fence != firstGrant.ResumeFence ||
		firstResumeExecution.ClaimedAt != runtimecontract.RuntimeOperationClaimMarkerGovernanceEffectStarted {
		t.Fatalf("first same-operation governance execution was not started exactly: %+v", firstResumeExecution)
	}

	secondRequirement := runtimecontract.RuntimeGovernanceRequirement{
		Kind: runtimecontract.GovernanceApproval, SubjectRef: "a.txt",
		Reason: "owner requires a fresh approval at the next execution fence", RequestedAction: "shell/command", RiskLevel: "high",
		AuthorizationContext: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "a.txt")),
	}
	secondReceipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: firstResumeExecution.ExecutionKey, Fence: firstResumeExecution.Fence, OperationID: firstResumeExecution.OperationID,
		Outcome: runtimecontract.RuntimeOperationOutcomeGovernanceRequired, ObservedEffectScope: runtimecontract.RuntimeEffectGovernanceOnly,
		Governance: &secondRequirement,
	}
	_, committedReceipt, err := fixture.controller.commitOperationReceipt(
		ctx, started.Facts.WorkspaceID, started.Facts.RunID, firstResumeID, firstResumeExecution.Fence, secondReceipt,
	)
	if err != nil {
		t.Fatal(err)
	}
	progressed, err := fixture.controller.candidateDispatch.recordOperationGovernanceRequired(
		ctx, started.Facts.WorkspaceID, started.Facts.RunID, fixture.firstOperationID, RuntimeOperationExecutionResult{
			EventID: "governance-shell-1-again", OperationID: fixture.firstOperationID, Receipt: committedReceipt, Governance: &secondRequirement,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if progressed.Decision.State != runtimecontract.ConvergenceStateNeedGovernance || progressed.ActiveDispatch != nil || len(progressed.OperationGovernanceBindings) != 2 {
		t.Fatalf("same operation did not enter its second governance edge: decision=%+v bindings=%+v", progressed.Decision, progressed.OperationGovernanceBindings)
	}
	secondBinding := progressed.OperationGovernanceBindings[1].Normalized()
	if secondBinding.OperationID != fixture.firstBinding.OperationID || secondBinding.ExecutionKey != fixture.firstBinding.ExecutionKey ||
		secondBinding.Fence != firstGrant.ResumeFence || secondBinding.State != runtimecontract.RuntimeOperationGovernanceBindingSuspended {
		t.Fatalf("second same-operation governance binding lost the next execution fence: first=%+v second=%+v", fixture.firstBinding, secondBinding)
	}

	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		progressed.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(progressed.Facts.Resources),
	)
	if err != nil {
		t.Fatal(err)
	}
	dependency := progressed.Facts.Failure.GovernanceDependency
	if dependency == nil {
		t.Fatal("second same-operation governance dependency is missing")
	}
	governed, err := fixture.controller.sealGovernanceDispatch(ctx, progressed.Facts.WorkspaceID, progressed.Facts.RunID, manifest, runtimecontract.RuntimeCandidate{
		Governance: &runtimecontract.GovernanceCandidate{ExternalDependency: dependency.Clone()},
	})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := fixture.controller.bindGovernanceRequest(ctx, progressed.Facts.WorkspaceID, progressed.Facts.RunID, runtimecontract.RuntimeGovernanceRequestRecord{
		ApprovalID: "approval-shell-1-again", AuthoritySnapshotID: governed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID: governed.ActiveDispatch.ID, DependencyKind: dependency.Kind, SubjectRef: dependency.SubjectRef,
		RequestedAction: dependency.RequestedAction, RiskLevel: dependency.RiskLevel,
		AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(dependency.AuthorizationContext),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bound.OperationGovernanceBindings) != 2 || bound.OperationGovernanceBindings[1].State != runtimecontract.RuntimeOperationGovernanceBindingTicketBound {
		t.Fatalf("second same-operation governance request did not bind only the new fence: %+v", bound.OperationGovernanceBindings)
	}
	secondResolution := runtimecontract.RuntimeGovernanceResolution{
		ResolutionID: "resolution-shell-1-again", ApprovalID: "approval-shell-1-again",
		WorkspaceID: progressed.Facts.WorkspaceID, RunID: progressed.Facts.RunID,
		AuthoritySnapshotID: governed.ActiveDispatch.Transaction.AuthoritySnapshotID, DispatchID: governed.ActiveDispatch.ID,
		DependencyKind: dependency.Kind, SubjectRef: dependency.SubjectRef,
		Outcome: runtimecontract.RuntimeGovernanceResolutionOutcomeContinue, Summary: "fresh approval granted for the next fence",
		EvidenceRefs: []string{"approval://shell-1-again"},
	}
	continued, err := fixture.controller.commitGovernanceResolution(ctx, secondResolution)
	if err != nil {
		t.Fatal(err)
	}
	if continued.Aggregate.ActiveDispatch == nil || len(continued.Aggregate.OperationGovernanceBindings) != 2 ||
		len(continued.Aggregate.GovernanceResolutions) != 2 {
		t.Fatalf("second same-operation governance continuation is incomplete: %+v", continued.Aggregate)
	}
	secondBinding = continued.Aggregate.OperationGovernanceBindings[1].Normalized()
	if secondBinding.State != runtimecontract.RuntimeOperationGovernanceBindingGranted || secondBinding.Grant == nil ||
		secondBinding.Grant.GovernanceFence != firstGrant.ResumeFence || secondBinding.Grant.ResumeFence != firstGrant.ResumeFence+1 {
		t.Fatalf("second same-operation grant did not advance exactly one fence: %+v", secondBinding)
	}
	secondResumeExecution, found := operationExecutionRecord(continued.Aggregate.ActiveDispatch.OperationExecutions, fixture.firstOperationID)
	if !found || secondResumeExecution.ExecutionKey != firstResumeExecution.ExecutionKey ||
		secondResumeExecution.Fence != secondBinding.Grant.ResumeFence ||
		secondResumeExecution.ClaimedAt != runtimecontract.RuntimeOperationClaimMarkerGovernanceGranted {
		t.Fatalf("second same-operation resume execution is not exact: %+v", secondResumeExecution)
	}
	resolutionRecord := continued.Aggregate.GovernanceResolutions[1].Normalized()
	wantResolutionHash := runtimecontract.StableRuntimeGovernanceResolutionHash(secondResolution)
	if resolutionRecord.ResolutionHash != wantResolutionHash || secondBinding.Grant.ResolutionHash != wantResolutionHash {
		t.Fatalf("second same-operation resolution/grant hash lineage drifted: record=%+v grant=%+v", resolutionRecord, secondBinding.Grant)
	}
	if err := continued.Aggregate.Validate(); err != nil {
		t.Fatalf("aggregate with repeated same-operation governance is invalid: %v", err)
	}

	activeID := continued.Aggregate.ActiveDispatch.ID
	replayed, err := fixture.controller.commitGovernanceResolution(ctx, secondResolution)
	if err != nil || replayed.Disposition != CommitDispositionDuplicate || replayed.Aggregate.ActiveDispatch == nil || replayed.Aggregate.ActiveDispatch.ID != activeID {
		t.Fatalf("same-operation governance replay was not idempotent: result=%+v err=%v", replayed, err)
	}
}
