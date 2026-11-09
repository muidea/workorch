package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeFailureReviewReportForTest(m rc.RuntimeSemanticPlanManifest, b rc.RuntimeValidationProofRequirementBinding, conclusion string) *rc.RuntimeValidationInputReview {
	r := &rc.RuntimeValidationInputReview{SubjectHash: b.InputReview.Hash}
	finding := rc.RuntimeValidationInputCovered
	if conclusion == rc.RuntimeValidationInputConflict {
		finding = rc.RuntimeValidationInputConflicting
	}
	if conclusion == rc.RuntimeValidationInputInconclusive {
		finding = rc.RuntimeValidationInputUnknown
	}
	refs := []string{b.InputReview.Files[0].TargetRef}
	for _, check := range b.RequiredCheckRefs {
		c := rc.RuntimeValidationInputCheckReview{RequiredCheckRef: check, Conclusion: conclusion, Reason: "exact input versus original criterion", InputTargetRefs: refs}
		for _, item := range m.CoverageItems {
			if item.ID == b.CoverageWorkItemID {
				c.Obligations = []rc.RuntimeValidationInputObligationReview{{CriterionText: item.TargetText, Finding: finding, Reason: "original criterion retained", InputTargetRefs: refs}}
			}
		}
		if conclusion == rc.RuntimeValidationInputInconclusive && b.InputReview.EvaluationScope != "" {
			c.UncertaintyKind = rc.RuntimeValidationSemanticInformationMissing
		}
		r.Checks = append(r.Checks, c)
	}
	return r
}

// Start from a real sealed Definition proof before any operation; test-only
// assembly adds the owner's declared input protocol at that admission boundary.
func runtimeFailureInputReviewFixture(t *testing.T, missing bool, maintenance ...bool) (*durableRuntimeStateRepository, *DispatchCoordinator, ExecutionEvidence, RuntimeAggregate) {
	t.Helper()
	convergence := runtimeValidationFailureRepairConvergenceForTest(2)
	if len(maintenance) > 1 && maintenance[1] {
		for i := range convergence.Routes {
			convergence.Routes[i].RepairOperationKinds = []string{rc.StepKindFileEdit}
			convergence.Routes[i].RepairTargetPolicy = rc.RuntimeOperationTargetPolicyExistingOwnerScope
			convergence.Routes[i].RepairOperationTargetSelectorRef = "file/workspace-relative-existing-target"
		}
	}
	a := runtimeValidationFailureRepairAggregateForTest(t, convergence, func(a *RuntimeAggregate) {
		a.Facts.Resources.ProviderCeiling = semanticRequestTestCeiling(a.Facts)
		a.Facts.Capabilities.GovernanceKinds = []string{rc.RuntimeBlockKindDecisionRequest}
		if len(maintenance) > 0 && maintenance[0] {
			policy := rc.RuntimeImplementationWorkPolicy{ProtocolRef: rc.RuntimeImplementationWorkProtocolV1, OwnerModuleID: rc.RuntimeModuleFile,
				AllowedOperations: []rc.RuntimeOperationVariantRef{{Kind: rc.StepKindFileEdit}}, AllowedTargetPolicies: []rc.RuntimeOperationTargetPolicy{rc.RuntimeOperationTargetPolicyExistingOwnerScope},
				TargetSelectorRef: "file/workspace-relative-existing-target", TargetSelectorOwnerRef: "profile/work-test", TargetSelectorSubjectRef: "implementation", TargetSelectorCandidateRefs: []string{"input.go", "main.go"},
				MaxRepeatedWorkStates: 2, MaxTargetsPerRound: 2, RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true, InputRevisionProtocolRef: rc.RuntimeValidationInputRevisionProtocolV1}
			a.Facts.DefinitionFocuses[0].ImplementationWorkPolicy = &policy
			a.Facts.Activation.ExecutionContract.ProfileContracts = []rc.RuntimeProfileContract{{ID: "work-test", EvidenceRequirements: []rc.RuntimeProfileEvidenceRequirement{{ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{rc.EvidenceKindValidation}, AcceptedStepKinds: []string{rc.RuntimeBlockKindManagedValidationSession}, ImplementationWorkPolicy: &policy}}}}
		}
	})
	a.SemanticPlanning = rc.RuntimeSemanticPlanningLedger{}
	runtimeTestEnsureSemanticPlanningLedger(t, &a, func(a *RuntimeAggregate) {
		for i := range a.Facts.Resources.OperationAdmissibilities {
			c := &a.Facts.Resources.OperationAdmissibilities[i]
			if c.OwnerModuleID != rc.RuntimeModuleShell {
				continue
			}
			for j := range c.ValidationSurfaces {
				c.ValidationSurfaces[j].InputTargetRefs = []string{"input.go"}
				c.ValidationSurfaces[j].InputTargetOwnerModuleID = rc.RuntimeModuleFile
				for k := range c.ValidationSurfaces[j].ProofRequirementBindings {
					b := &c.ValidationSurfaces[j].ProofRequirementBindings[k]
					content := "original exact independent input\r\n"
					b.InputReview = &rc.RuntimeValidationInputReviewSubject{ProtocolRef: rc.RuntimeValidationInputReviewProtocol, EvaluationScope: rc.RuntimeValidationInputSemantics, Files: []rc.RuntimeValidationInputReviewFile{{TargetRef: "input.go", Content: content, ContentHash: rc.RuntimeValidationInputContentHash(content)}}}
					b.InputReview.Hash = rc.StableRuntimeValidationInputReviewSubjectHash(*b.InputReview, *b)
				}
			}
			c.ValidationRecipes[0].InputReviewProtocolRef = rc.RuntimeValidationInputReviewProtocol
			c.ValidationRecipes[0].AdapterRef = "owner/review"
			c.SnapshotID = rc.StableRuntimeOperationAdmissibilitySnapshotID(*c)
		}
	})
	runtimeTestProjectSemanticBindingPreimages(t, &a)
	runtimeTestSealCanonicalCWILedger(t, &a)
	a.Decision = mustReduceRuntimeState(t, a.Facts)
	if err := a.Validate(); err != nil {
		t.Fatal("review baseline", err)
	}
	repository := &durableRuntimeStateRepository{aggregate: a}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	evidence := runtimeRejectedValidationEvidenceForTest(t, sealed, "input-review-rejection", rc.ValidationDiagnosticTestFailed)
	if len(maintenance) > 1 && maintenance[1] {
		sealedScope, err := runtimeValidationScopeTargetRefs(*sealed.ActiveDispatch, sealed.ActiveDispatch.Candidate.Validation.Operations[0].Operation)
		if err != nil {
			t.Fatal(err)
		}
		observation := rc.RuntimeValidationFailureObservation{WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID, DispatchID: sealed.ActiveDispatch.ID, OperationID: evidence.OperationID, FailureSignature: evidence.Failure.Signature, TargetRole: sealed.ActiveDispatch.FocusTargetRole, DiagnosticClass: rc.ValidationDiagnosticTestFailed, CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: sealedScope}
		evidence.Failure.ValidationDiagnostic.TargetRefs = observation.CandidateTargetRefs
		observation.DiagnosticHash = rc.StableRuntimeValidationFailureObservationDiagnosticHash(*evidence.Failure.ValidationDiagnostic, observation.ValidationScopeTargetRefs)
		evidence.Failure.ValidationFailureObservation = &observation
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	// Native failure is factual input, not a custom WORKORCH shape requirement.
	o := evidence.Receipt.Evidence.Validation
	check := o.ExecutedChecks[0]
	o.PassedChecks = nil
	o.FailedChecks = []string{check}
	o.FailureDetailProtocolRef = rc.RuntimeValidationFailureDetailProtocol
	body := "expected 15, got 13"
	sum := sha256.Sum256([]byte(body))
	o.CheckFailures = []rc.RuntimeValidationCheckFailure{{CheckRef: check, CaseRef: check, TargetRef: "input.go", Line: 9, DiagnosticHash: hex.EncodeToString(sum[:]), ComparisonHash: hex.EncodeToString(sum[:]), AssertionText: body}}
	if missing {
		o.CheckFailures[0].AssertionText = ""
	}
	deps := rc.CloneRuntimeOwnerRevisionSet(evidence.Receipt.Evidence.OwnerRevisionSet)
	if deps == nil {
		deps = &rc.RuntimeOwnerRevisionSet{DefinitionRevision: a.Definition.Revision}
	}
	deps.Entries = append(deps.Entries, rc.RuntimeOwnerRevisionRef{OwnerKind: "capability", OwnerInstanceID: rc.RuntimeModuleShell + "/" + string(a.Facts.WorkspaceID), ResourceKind: string(rc.RuntimeResourceWorkspaceTarget), ResourceID: "input.go", Revision: rc.RuntimeValidationInputContentHash("original exact independent input\r\n"), DependencyHash: strings.Repeat("a", 64)})
	*deps = deps.WithHash()
	evidence.Receipt.Evidence.OwnerRevisionSet = deps
	// This fixture's receipt is not committed until the evidence event below.
	for i := range repository.aggregate.OperationReceipts {
		if repository.aggregate.OperationReceipts[i].ExecutionKey == evidence.Receipt.ExecutionKey {
			repository.aggregate.OperationReceipts[i] = evidence.Receipt.Clone()
		}
	}
	for i := range repository.aggregate.ActiveDispatch.OperationExecutions {
		repository.aggregate.ActiveDispatch.OperationExecutions[i].OwnerReceiptHash = rc.StableRuntimeOperationReceiptHash(*evidence.Receipt)
	}
	return repository, coordinator, evidence, a
}

func mustReduceRuntimeState(t *testing.T, f rc.RuntimeFacts) rc.RuntimeDecision {
	t.Helper()
	d, err := ReduceRuntimeState(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestFailureInputReviewOpensOnceFromOriginalReceiptAndRecovers(t *testing.T) {
	repository, coordinator, e, before := runtimeFailureInputReviewFixture(t, false)
	got, err := coordinator.commitEvidence(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	a := got.Aggregate
	if a.SemanticPlanning.Phase != rc.RuntimeSemanticPlanningRequired || a.SemanticPlanning.FailureInputReview == nil || SemanticOperationAuthorityReady(a) {
		t.Fatal("rejected validation did not fence repair behind review")
	}
	if !reflect.DeepEqual(before.Facts.Coverage[0].CurrentBindingRef, a.Facts.Coverage[0].CurrentBindingRef) || !reflect.DeepEqual(before.ExecutionBudget.Policy, a.ExecutionBudget.Policy) {
		t.Fatal("review changed original binding or budget")
	}
	if a.Facts.Repair.RepairAttempts != before.Facts.Repair.RepairAttempts+1 {
		t.Fatal("review charged implementation repair")
	}
	snapshot, _ := json.Marshal(a)
	var fresh RuntimeAggregate
	if err := json.Unmarshal(snapshot, &fresh); err != nil {
		t.Fatal(err)
	}
	if err := fresh.Validate(); err != nil {
		t.Fatal("fresh review", err)
	}
	duplicate, err := coordinator.commitEvidence(context.Background(), e)
	if err != nil || duplicate.Aggregate.StateRevision != a.StateRevision {
		t.Fatal("duplicate reopened review", err)
	}
	m, err := CompileRuntimeSemanticManifest(fresh)
	if err != nil {
		t.Fatal("manifest", err)
	}
	if m.FailureInputReview == nil || len(m.CoverageItems) != 1 {
		t.Fatal("review manifest scope")
	}
	c := rc.RuntimeSemanticPlanRequestContext{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, Definition: a.Definition, PlanStep: rc.RuntimeSemanticPlanStepProofBatch, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{m.CoverageItems[0].ID}}
	feedback, err := SemanticExecutionFeedback(fresh, c, m)
	if err != nil || len(feedback) != 1 || feedback[0].Validation.CheckFailures[0].AssertionText != "expected 15, got 13" {
		t.Fatal("exact failure context lost", err)
	}
	for name, mutate := range map[string]func(*RuntimeAggregate){
		"receipt":   func(a *RuntimeAggregate) { a.OperationReceipts[0].AfterHash = "tampered" },
		"evidence":  func(a *RuntimeAggregate) { a.Facts.Evidence.Rejected[0].Summary = "tampered" },
		"workspace": func(a *RuntimeAggregate) { a.Facts.WorkspaceID = "other" },
		"subject":   func(a *RuntimeAggregate) { a.SemanticPlanning.FailureInputReview.SubjectHash = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			var v RuntimeAggregate
			json.Unmarshal(snapshot, &v)
			mutate(&v)
			if err := v.Validate(); err == nil {
				t.Fatal("bad provenance accepted")
			}
		})
	}
	if repository.aggregate.StateRevision != a.StateRevision {
		t.Fatal("read projection mutated owner")
	}
}

func TestFailureInputReviewMissingAssertionsStopsBlindRepair(t *testing.T) {
	_, coordinator, e, _ := runtimeFailureInputReviewFixture(t, true)
	got, err := coordinator.commitEvidence(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	a := got.Aggregate
	if a.Facts.Failure == nil || a.Facts.Failure.Code != "validation_failure_assertions_unavailable" || a.SemanticPlanning.FailureInputReview != nil || a.ActiveDispatch != nil {
		t.Fatalf("missing assertion allowed blind repair: %+v", a.Facts.Failure)
	}
}

func TestFailureInputReviewDoesNotUseUnrelatedHistoricalText(t *testing.T) {
	// A hash is not actionable body; do not fill text from a prior failure.
	f := rc.RuntimeValidationObservation{FailedChecks: []string{"check"}, FailureDetailProtocolRef: rc.RuntimeValidationFailureDetailProtocol,
		CheckFailures: []rc.RuntimeValidationCheckFailure{{CheckRef: "check", DiagnosticHash: strings.Repeat("a", 64)}}}
	if !rc.ProjectRuntimeValidationExecutionFeedback(&f).FailureDetailsIncomplete {
		t.Fatal("location/hash credited as complete assertion")
	}
}

func TestFailureInputReviewPriorReportUsesExactAcceptedBinding(t *testing.T) {
	_, coordinator, evidence, _ := runtimeFailureInputReviewFixture(t, false)
	got, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	a := got.Aggregate
	source := a.SemanticPlanning.FailureInputReview
	want := source.Feedback.PreviousInputReview
	if want == nil || want.Report.SubjectHash != source.SubjectHash {
		t.Fatal("original formal review missing")
	}
	// A later request can see the same manifest and subject, but its batch
	// identity and report cannot replace the original binding's formal source.
	var unrelated rc.RuntimeSemanticPlanningLedger
	for _, g := range a.SemanticPlanning.AllHistory() {
		if g.Snapshot.Manifest != nil && g.Snapshot.Manifest.Hash == source.BindingRef.SemanticManifestHash {
			unrelated = g.Snapshot.CloneRaw()
			break
		}
	}
	if unrelated.Manifest == nil {
		t.Fatal("missing original accepted source")
	}
	for i := range unrelated.BatchDocuments {
		d := &unrelated.BatchDocuments[i]
		for j := range d.Proposals {
			if review := d.Proposals[j].ValidationInputReview; review != nil {
				review.Checks[0].Reason = "another request's independent report"
			}
		}
		for j := range unrelated.Batches {
			b := &unrelated.Batches[j]
			if b.BatchID == d.BatchID {
				b.RequestLogicalID = "unrelated-generation-request"
				b.DocumentHash = rc.StableRuntimeSemanticPlanDocumentHash(*d)
				b.Hash = rc.StableRuntimeSemanticPlanBatchHash(*b)
			}
		}
	}
	a.SemanticPlanning.History = append(a.SemanticPlanning.History, rc.RuntimeSemanticPlanningGeneration{Snapshot: unrelated})
	actual, err := runtimeFailurePreviousInputReview(a, source.BindingRef, source.SubjectHash, source.Feedback.CoverageWorkItemID)
	if err != nil || !reflect.DeepEqual(want, actual) {
		t.Fatal("unrelated accepted report displaced original source", err)
	}
	changed := source.Clone()
	changed.Feedback.PreviousInputReview.Report.Checks[0].Reason = "replacement report"
	changed.Feedback.PreviousInputReview.ReportHash = rc.StableRuntimeValidationInputReviewHash(changed.Feedback.PreviousInputReview.Report)
	changed.Feedback.Hash = rc.StableRuntimeSemanticExecutionFeedbackHash(changed.Feedback)
	if _, _, _, err := runtimeFailureInputReviewSource(a, *changed); err == nil {
		t.Fatal("self-consistent changed report accepted as original source")
	}
	a.SemanticPlanning.History = nil
	a.SemanticPlanning.HistoryArchive = nil
	if _, err := runtimeFailurePreviousInputReview(a, source.BindingRef, source.SubjectHash, source.Feedback.CoverageWorkItemID); err == nil {
		t.Fatal("missing original report filled from feedback copy")
	}
}

func runtimeSubmitFailureInputReportForTest(t *testing.T, repository *durableRuntimeStateRepository, conclusion string) (*RuntimeController, RuntimeAggregate) {
	t.Helper()
	ctx := context.Background()
	a := repository.aggregate
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	m, err := CompileRuntimeSemanticManifest(a)
	if err != nil {
		t.Fatal(err)
	}
	a, _ = startSemanticRequestForTest(t, controller, a, m, "failure-review-prefix")
	a, err = controller.planning.compileSingletonSemanticOrder(ctx, a.Facts.WorkspaceID, a.Facts.RunID, a.SemanticPlanning.LogicalRequestID)
	if err != nil {
		t.Fatal("singleton", err)
	}
	a, err = controller.authority.projectEligibleRuntimeFocus(ctx, a.Facts.WorkspaceID, a.Facts.RunID, m.Hash, a.SemanticPlanning.CoverageOrderDocument.CoverageOrder)
	if err != nil {
		t.Fatal("focus", err)
	}
	identityHash := SemanticRequestIdentityHashForKey(a.SemanticPlanning, "semantic-proof-batch-0")
	logical := rc.StableRuntimeCandidateRequestSessionID(rc.RuntimeProviderDocumentSemanticPlan, a.Facts.WorkspaceID, a.Facts.RunID, a.Decision.AuthorityTransaction(), a.SemanticPlanning.StablePrefixHash, m.Hash, identityHash)
	batch, err := semanticplan.CompileFocusBatch(m, *a.SemanticPlanning.CoverageOrderDocument, 0, logical, a.Facts.Focus.CoverageWorkItemIDs, a.SemanticPlanning.PendingCoverageIDs, 1)
	if err != nil {
		t.Fatal(err)
	}
	a, err = controller.compileSemanticPlanBatch(ctx, a.Facts.WorkspaceID, a.Facts.RunID, batch)
	if err != nil {
		t.Fatal("batch", err)
	}
	expected, err := SemanticExpectedDocument(a)
	if err != nil {
		t.Fatal("expected", err)
	}
	a, session, err := controller.ensureCandidateRequestSessionWithExpected(ctx, a.Facts.WorkspaceID, a.Facts.RunID, rc.RuntimeProviderDocumentSemanticPlan, a.SemanticPlanning.StablePrefixHash, m.Hash, identityHash, &expected)
	if err != nil {
		t.Fatal("session", err)
	}
	physical := rc.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, 1)
	a, err = controller.markCandidateRequestSubmitted(ctx, a.Facts.WorkspaceID, a.Facts.RunID, session.LogicalRequestID, 1, physical)
	if err != nil {
		t.Fatal("submit", err)
	}
	a, err = controller.completeCandidateRequestDocument(ctx, a.Facts.WorkspaceID, a.Facts.RunID, session.LogicalRequestID, physical)
	if err != nil {
		t.Fatal("complete", err)
	}
	original := a.SemanticPlanning.History[0].Snapshot.Document.Clone()
	original.ManifestHash = m.Hash
	original.BatchID = batch.BatchID
	original.BatchSequence = batch.Sequence
	original.BatchCoverageWorkItemIDs = batch.CoverageWorkItemIDs
	for i := range original.Proposals {
		p := &original.Proposals[i]
		if b, ok := rc.RuntimeSemanticValidationInputReviewBinding(*p, m.OwnerCatalogs); ok && b.InputReview != nil {
			p.ValidationInputReview = runtimeFailureReviewReportForTest(m, b, conclusion)
		}
	}
	a, err = controller.evaluateSemanticPlan(ctx, a.Facts.WorkspaceID, a.Facts.RunID, session.LogicalRequestID, original)
	if err != nil {
		t.Fatal("report", err)
	}
	return controller, a
}

func TestFailureInputReviewConsistentContinuesOriginalRepair(t *testing.T) {
	repository, coordinator, e, _ := runtimeFailureInputReviewFixture(t, false)
	if _, err := coordinator.commitEvidence(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	controller, a := runtimeSubmitFailureInputReportForTest(t, repository, rc.RuntimeValidationInputConsistent)
	attempts := a.Facts.Repair.RepairAttempts
	updated, err := controller.requireSemanticFeasibility(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID)
	if err != nil {
		t.Fatal("classify", err)
	}
	if updated.SemanticPlanning.FailureInputReview != nil || updated.Facts.Coverage[0].CurrentBindingRef != nil || updated.Facts.Repair.RepairAttempts != attempts {
		t.Fatal("consistent report did not preserve original bounded repair")
	}
	if _, required, err := CurrentRuntimeValidationFailureRepairGenerationInput(updated); err != nil || !required {
		t.Fatal("original repair did not resume", err)
	}
	count := 0
	for _, g := range updated.SemanticPlanning.AllHistory() {
		if g.Reason == rc.RuntimeSemanticPlanningRetirementFailureInputReviewed {
			count++
		}
	}
	if count != 1 {
		t.Fatal("review not committed exactly once")
	}
	if err := updated.Validate(); err != nil {
		t.Fatal("consistent reload", err)
	}
}

func TestFailureInputReviewInconclusiveUsesExistingFactualDecision(t *testing.T) {
	repository, coordinator, e, _ := runtimeFailureInputReviewFixture(t, false)
	if _, err := coordinator.commitEvidence(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	controller, a := runtimeSubmitFailureInputReportForTest(t, repository, rc.RuntimeValidationInputInconclusive)
	updated, err := controller.requireSemanticFeasibility(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID)
	if err != nil {
		t.Fatal("classify", err)
	}
	if updated.Facts.Failure == nil || updated.Facts.Failure.Code != "validation_input_review_inconclusive" || updated.Decision.State != rc.ConvergenceStateNeedGovernance || updated.SemanticPlanning.FailureInputReview == nil {
		t.Fatalf("inconclusive bypassed factual decision: %+v decision=%+v", updated.Facts.Failure, updated.Decision)
	}
}

func TestFailureInputReviewConflictOpensOnlyProtectedInputMaintenance(t *testing.T) {
	repository, coordinator, e, _ := runtimeFailureInputReviewFixture(t, false, true)
	if _, err := coordinator.commitEvidence(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	controller, a := runtimeSubmitFailureInputReportForTest(t, repository, rc.RuntimeValidationInputConflict)
	attempts := a.Facts.Repair.RepairAttempts
	updated, err := controller.requireSemanticFeasibility(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID)
	if err != nil {
		t.Fatal("classify", err)
	}
	if updated.SemanticPlanning.FailureInputReview != nil || updated.Facts.Failure != nil || updated.Facts.Coverage[0].CurrentBindingRef != nil || updated.Facts.Repair.RepairAttempts != attempts {
		t.Fatalf("input conflict did not enter maintenance: failure=%+v phase=%s source=%t binding=%+v attempts=%d/%d", updated.Facts.Failure, updated.SemanticPlanning.Phase, updated.SemanticPlanning.FailureInputReview != nil, updated.Facts.Coverage[0].CurrentBindingRef, updated.Facts.Repair.RepairAttempts, attempts)
	}
	v, err := rc.PendingRuntimeValidationInputRevision(updated.SemanticPlanning, updated.ImplementationWork, updated.Facts.Focus.CoverageWorkItemIDs[0])
	if err != nil || v == nil || !reflect.DeepEqual(v.InputTargetRefs, []string{"input.go"}) {
		t.Fatal("exact protected maintenance source lost", err)
	}
	if updated.Facts.RepairDirective != nil || SemanticOperationAuthorityReady(updated) {
		t.Fatal("retained repair counters preempted the pending input-maintenance transaction")
	}
	reduced, decision, err := reduceRuntimeAggregateWithDirective(updated, updated.Facts, &updated.Decision.AuthoritySnapshot)
	if err != nil || reduced.RepairDirective != nil || reduced.Repair.RepairAttempts != attempts || decision.Terminal != nil {
		t.Fatal("reduction reconstructed generic repair authority during maintenance", err)
	}
	if v.Binding.InputReview.Files[0].Content != "original exact independent input\r\n" {
		t.Fatal("input body changed")
	}
	if err := updated.Validate(); err != nil {
		t.Fatal("maintenance reload", err)
	}
}

func TestFailureInputReviewReceiptCASRecovery(t *testing.T) {
	for _, mode := range []failureAuthorityCASMode{failureAuthorityCASFailBeforeWrite, failureAuthorityCASLoseResponse} {
		t.Run(map[failureAuthorityCASMode]string{failureAuthorityCASFailBeforeWrite: "before-write", failureAuthorityCASLoseResponse: "response-loss"}[mode], func(t *testing.T) {
			original, _, e, _ := runtimeFailureInputReviewFixture(t, false)
			repo := &failureAuthorityCASRepository{aggregate: original.aggregate.CloneRaw(), mode: mode}
			coordinator, err := NewDispatchCoordinator(repo)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = coordinator.commitEvidence(context.Background(), e); err == nil {
				t.Fatal("missing injected CAS failure")
			}
			if mode == failureAuthorityCASFailBeforeWrite && repo.aggregate.SemanticPlanning.FailureInputReview != nil {
				t.Fatal("failed CAS leaked review")
			}
			repo.mode = failureAuthorityCASNormal
			restarted, _ := NewDispatchCoordinator(repo)
			got, err := restarted.commitEvidence(context.Background(), e)
			if err != nil {
				t.Fatal(err)
			}
			if got.Aggregate.SemanticPlanning.FailureInputReview == nil || got.Aggregate.Facts.Repair.RepairAttempts != 1 || len(got.Aggregate.Facts.Evidence.Rejected) != 1 {
				t.Fatal("recovery duplicated/lost original receipt review")
			}
			rev := got.Aggregate.StateRevision
			duplicate, err := restarted.commitEvidence(context.Background(), e)
			if err != nil || duplicate.Aggregate.StateRevision != rev {
				t.Fatal("duplicate changed review", err)
			}
		})
	}
}

func TestFailureInputReviewRequiresActualOriginalInputReads(t *testing.T) {
	repo, c, e, _ := runtimeFailureInputReviewFixture(t, false)
	e.Receipt.Evidence.OwnerRevisionSet.Entries[0].Revision = "different-original-input"
	*e.Receipt.Evidence.OwnerRevisionSet = e.Receipt.Evidence.OwnerRevisionSet.WithHash()
	for i := range repo.aggregate.OperationReceipts {
		if repo.aggregate.OperationReceipts[i].ExecutionKey == e.Receipt.ExecutionKey {
			repo.aggregate.OperationReceipts[i] = e.Receipt.Clone()
		}
	}
	for i := range repo.aggregate.ActiveDispatch.OperationExecutions {
		repo.aggregate.ActiveDispatch.OperationExecutions[i].OwnerReceiptHash = rc.StableRuntimeOperationReceiptHash(*e.Receipt)
	}
	got, err := c.commitEvidence(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	if got.Aggregate.Facts.Failure.Code != "validation_failure_input_snapshot_unavailable" || got.Aggregate.SemanticPlanning.FailureInputReview != nil || len(got.Aggregate.Facts.Evidence.Rejected) != 1 {
		t.Fatal("wrong original input granted review or lost failure")
	}
}

func TestFailureInputReviewConflictWithoutPermissionKeepsOriginalBinding(t *testing.T) {
	repo, c, e, before := runtimeFailureInputReviewFixture(t, false)
	if _, err := c.commitEvidence(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	controller, a := runtimeSubmitFailureInputReportForTest(t, repo, rc.RuntimeValidationInputConflict)
	updated, err := controller.requireSemanticFeasibility(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Facts.Failure == nil || updated.Facts.Failure.OriginalDiagnostic == nil || updated.Facts.Failure.OriginalDiagnostic.OriginalCode != "validation_input_review_conflict" || !reflect.DeepEqual(before.Facts.Coverage[0].CurrentBindingRef, updated.Facts.Coverage[0].CurrentBindingRef) {
		t.Fatalf("unpermitted conflict changed failure/binding: failure=%+v binding=%+v want=%+v", updated.Facts.Failure, updated.Facts.Coverage[0].CurrentBindingRef, before.Facts.Coverage[0].CurrentBindingRef)
	}
}

func TestFailureInputReviewConsistentStructuredRepairRetainsCurrentAuthority(t *testing.T) {
	repo, c, e, _ := runtimeFailureInputReviewFixture(t, false, false, true)
	if _, err := c.commitEvidence(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	controller, a := runtimeSubmitFailureInputReportForTest(t, repo, rc.RuntimeValidationInputConsistent)
	updated, err := controller.requireSemanticFeasibility(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Facts.Coverage[0].CurrentBindingRef == nil || updated.SemanticPlanning.FailureInputReview != nil || !SemanticOperationAuthorityReady(updated) || updated.Facts.Failure == nil || updated.Facts.Failure.ValidationFailureObservation == nil {
		t.Fatal("consistent review bypassed/lost structured owner confirmation")
	}
	if err := updated.Validate(); err != nil {
		t.Fatal("reload", err)
	}
	// The same already-reviewed failure consumes the formal report. A genuinely
	// changed native failure on this same binding opens a new bounded review.
	fact := updated.Facts.Evidence.Rejected[0]
	dispatch, _, _ := operationReceiptDispatch(updated, fact.DispatchID)
	op, _ := candidateOperationByID(dispatch.Candidate, fact.OperationID)
	copy := updated.CloneRaw()
	pending, err := openRuntimeFailureInputReview(&copy, fact, op)
	if err != nil || pending || !reflect.DeepEqual(copy, updated) {
		t.Fatal("same subject/failure was reviewed twice", err)
	}
	copy = updated.CloneRaw()
	copy.Facts.Repair.Exhausted = true
	pending, err = openRuntimeFailureInputReview(&copy, fact, op)
	if err != nil || !pending || copy.Facts.Failure.Code != "validation_failure_repair_exhausted" || copy.SemanticPlanning.FailureInputReview != nil {
		t.Fatal("review bypassed repair exhaustion", err)
	}
}

func TestFailureInputReviewInformationRetainsExactSourceAndExcludesWork(t *testing.T) {
	repo, c, e, _ := runtimeFailureInputReviewFixture(t, false, true)
	if _, err := c.commitEvidence(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	controller, a := runtimeSubmitFailureInputReportForTest(t, repo, rc.RuntimeValidationInputInconclusive)
	a, err := controller.requireSemanticFeasibility(context.Background(), a.Facts.WorkspaceID, a.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	source := a.SemanticPlanning.FailureInputReview.Clone()
	document, _ := runtimeInputReviewDocument(a.SemanticPlanning)
	resolution := rc.RuntimeGovernanceResolution{ResolutionID: "facts-resolution", TicketID: "facts-ticket", DependencyKind: string(rc.GovernanceDecision), SubjectRef: runtimeInputReviewRef(a, a.SemanticPlanning, document), Outcome: rc.RuntimeGovernanceResolutionOutcomeContinue, Summary: "exact supplied facts; original criteria remain"}
	stale := a.CloneRaw()
	wrong := resolution
	wrong.SubjectRef = "other-review"
	if retireRuntimeInputReviewForInformation(&stale, wrong) == nil || !reflect.DeepEqual(stale, a) {
		t.Fatal("stale facts retired review")
	}
	if err := retireRuntimeInputReviewForInformation(&a, resolution); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(source, a.SemanticPlanning.FailureInputReview) || a.SemanticPlanning.Phase != rc.RuntimeSemanticPlanningRequired {
		t.Fatal("facts response discarded original failure source")
	}
	// Governance continuation clears its mutable failure; the persisted review
	// still excludes unrelated purpose/scope and all operation authority.
	a.Facts.Failure = nil
	a.Facts.RepairDirective = nil
	if RuntimeImplementationWorkPurposePending(a) || SemanticOperationAuthorityReady(a) {
		t.Fatal("facts response authorized ordinary work")
	}
	if req, err := NextRuntimeImplementationWorkScopeRequest(a); err != nil || req != nil {
		t.Fatal("facts response opened unrelated work scope", err)
	}
	if _, err := RuntimeImplementationWorkScopeForCurrentSources(a, source.Feedback.CoverageWorkItemID); err == nil {
		t.Fatal("direct scope admission bypassed failure review")
	}
}

func TestFailureInputReviewCannotCompileExecutionCredit(t *testing.T) {
	repo, c, e, _ := runtimeFailureInputReviewFixture(t, false)
	if _, err := c.commitEvidence(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	_, a := runtimeSubmitFailureInputReportForTest(t, repo, rc.RuntimeValidationInputConsistent)
	d, _ := runtimeInputReviewDocument(a.SemanticPlanning)
	if _, _, err := rc.CompileRuntimeSemanticBindings(*a.SemanticPlanning.Manifest, a.SemanticPlanning.LogicalRequestID, d, nil); err == nil {
		t.Fatal("failure review compiled execution binding")
	}
	m := a.SemanticPlanning.Manifest.Clone()
	m.WorkspaceID = "other-workspace"
	m.Hash = rc.StableRuntimeSemanticPlanManifestHash(m)
	if m.Validate() == nil {
		t.Fatal("failure review crossed workspace boundary")
	}
}

// Producer detail bounds and the provider envelope are distinct contracts.
// The SM-2 receipt admission must keep the known failure/receipt and reject an
// over-capacity review before another semantic request or blind repair.
func TestFailureInputReviewWholeEnvelopeCapacityFromCommittedReceipt(t *testing.T) {
	for _, count := range []int{39, 6} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			repository, coordinator, evidence, before := runtimeFailureInputReviewFixture(t, false)
			observation := evidence.Receipt.Evidence.Validation
			original := observation.CheckFailures[0]
			observation.CheckFailures = nil
			for n := 0; n < count; n++ {
				detail := original
				detail.Line = n + 1
				detail.CaseRef = detail.CheckRef + "/" + strings.Repeat("native_case_", 8) + fmt.Sprint(n)
				detail.AssertionText = fmt.Sprintf("failure=%d ", n) + strings.Repeat("x", 300)
				if count == 6 {
					detail.AssertionText += strings.Repeat("y", 12000)
				}
				sum := sha256.Sum256([]byte(detail.AssertionText))
				detail.DiagnosticHash = hex.EncodeToString(sum[:])
				detail.ComparisonHash = detail.DiagnosticHash
				observation.CheckFailures = append(observation.CheckFailures, detail)
			}
			if err := observation.ValidateFailureDetails(); err != nil {
				t.Fatal("valid native details rejected before envelope", err)
			}
			receipt := evidence.Receipt.Clone()
			for i := range repository.aggregate.OperationReceipts {
				if repository.aggregate.OperationReceipts[i].ExecutionKey == receipt.ExecutionKey {
					repository.aggregate.OperationReceipts[i] = receipt.Clone()
				}
			}
			for i := range repository.aggregate.ActiveDispatch.OperationExecutions {
				repository.aggregate.ActiveDispatch.OperationExecutions[i].OwnerReceiptHash = rc.StableRuntimeOperationReceiptHash(receipt)
			}
			budget := repository.aggregate.ExecutionBudget.Clone()
			got, err := coordinator.commitEvidence(context.Background(), evidence)
			if err != nil {
				t.Fatal(err)
			}
			a := got.Aggregate
			stored, found := rc.RuntimeOperationReceiptForExecutionKeyFence(a.OperationReceipts, receipt.ExecutionKey, receipt.Fence)
			if !found || !reflect.DeepEqual(stored, receipt) || !reflect.DeepEqual(a.ExecutionBudget, budget) || !reflect.DeepEqual(a.ExecutionBudget.Policy, before.ExecutionBudget.Policy) {
				t.Fatal("envelope admission changed original receipt or execution budget")
			}
			if count == 39 {
				if a.SemanticPlanning.FailureInputReview == nil || len(a.SemanticPlanning.FailureInputReview.Feedback.Validation.CheckFailures) != 39 || a.SemanticPlanning.FailureInputReview.Feedback.PreviousInputReview == nil {
					t.Fatal("39 complete native details did not reach review with exact prior report")
				}
			} else if a.Facts.Failure == nil || a.Facts.Failure.Code != "validation_failure_feedback_capacity_exceeded" || a.SemanticPlanning.FailureInputReview != nil || a.ActiveDispatch != nil {
				t.Fatalf("overflow lost precise bounded result: %+v", a.Facts.Failure)
			}
			snapshot, _ := json.Marshal(a)
			var reloaded RuntimeAggregate
			if err := json.Unmarshal(snapshot, &reloaded); err != nil {
				t.Fatal(err)
			}
			if err := reloaded.Validate(); err != nil {
				t.Fatal("receipt reload", err)
			}
			duplicate, err := coordinator.commitEvidence(context.Background(), evidence)
			if err != nil || duplicate.Aggregate.StateRevision != a.StateRevision || !reflect.DeepEqual(duplicate.Aggregate.ExecutionBudget, a.ExecutionBudget) {
				t.Fatal("duplicate capacity result retried or charged budget", err)
			}
		})
	}
}
