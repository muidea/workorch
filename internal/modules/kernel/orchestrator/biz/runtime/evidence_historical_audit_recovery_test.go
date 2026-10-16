package runtime

import (
	"context"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Typed owner facts exercise the complete private CAS/reload gate. Actual File
// postimage production and readback are covered separately by the shared-file
// mainline; this fixture must not be presented as a real resource query.
func historicalAuditCandidateFixture(t *testing.T, window string) (RuntimeAggregate, runtimecontract.RuntimeEvidenceApplicabilityObservation) {
	t.Helper()
	ctx := context.Background()
	facts := controllerFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Focus.ValidationRecipeID = runtimeManagedValidationEvidencePolicy().ID
	facts.Focus.RequiredCheckRefs = []string{"check/" + facts.Focus.ValidationRecipeID}
	facts.Activation.EvidencePolicies = runtimeManagedValidationEvidencePolicies()
	facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.DefinitionFocuses = nil
	facts = runtimeFactsWithManagedValidationRecipe(facts)
	runtimeTestSealCurrentPolicies(&facts)
	facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
	var err error
	facts, err = runtimeTestBindManagedValidationProofRequirements(facts)
	if err != nil {
		t.Fatal(err)
	}
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	definition := controllerDefinition()
	definition.CapabilityContract = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
	initial, err := controller.initialize(ctx, facts, definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := beginAndAcceptCandidate(t, controller, initial, runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "historical-audit-effect", Kind: runtimecontract.StepKindFileWrite, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileWriteParameters("完成后的文件内容\n"),
	}}}}})
	if err != nil {
		t.Fatal(err)
	}
	claimed, receipt := lineageReceiptClaim(t, controller, sealed)
	reads := (runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: facts.DefinitionRevision, Entries: []runtimecontract.RuntimeOwnerRevisionRef{{OwnerKind: "capability", OwnerInstanceID: "file/ws-v2", ResourceKind: "workspace_target", ResourceID: "target.txt", Revision: receipt.BeforeHash, DependencyHash: "original-read"}}}).WithHash()
	post := *runtimecontract.CloneRuntimeOwnerRevisionSet(&reads)
	post.Entries[0].ResourceKind, post.Entries[0].Revision, post.Entries[0].DependencyHash = "workspace_target_content", receipt.AfterHash, "original-postimage"
	post = post.WithHash()
	receipt.Evidence.OwnerRevisionSet = &reads
	receipt.Evidence.ApplicabilityDependencies = &runtimecontract.RuntimeEvidenceApplicabilityDependencies{ProtocolRef: runtimecontract.RuntimeEvidenceApplicabilityDependenciesV2, OwnerRevisionSet: post,
		PurposeRef: runtimecontract.RuntimeHistoricalEffectProofV1, OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKind: runtimecontract.StepKindFileWrite, OwnerContractRef: runtimecontract.RuntimeFileHistoricalEffectProofV1}
	committed := lineageReceiptObserve(t, controller, claimed, receipt)
	evidence := lineageReceiptEvidence(t, committed, receipt)
	result, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventOperationEvidenceObserved, Scope: lineageReceiptScope(committed), Transaction: &RuntimeInternalTransactionPayload{ExecutionEvidence: &evidence}})
	if err != nil || result.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("historical behavior must still owe ordinary acceptance: %v", err)
	}
	if window == "pending" || window == "proposal" {
		if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "historical-audit-prefix"); err != nil {
			t.Fatal(err)
		}
		if window == "proposal" {
			candidate := runtimeTestBindCandidateToFacts(t, repository.aggregate.Facts, runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeManagedValidationParameters()}}}}})
			if _, err := controller.recordCandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate); err != nil {
				t.Fatal(err)
			}
		}
	} else {
		manifest, err := BuildRuntimeCandidateContractManifestForAggregate(*repository.aggregate)
		if err != nil {
			t.Fatal(err)
		}
		_, session, err := controller.ensureCandidateRequestSession(ctx, "ws-v2", "run-v2", runtimecontract.RuntimeProviderDocumentOperations, "historical-audit-prefix", manifest.Hash, "")
		if err != nil {
			t.Fatal(err)
		}
		if window == "completed_document" {
			physical := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
			if _, err := controller.markCandidateRequestSubmitted(ctx, "ws-v2", "run-v2", session.LogicalRequestID, session.NextSequence, physical); err != nil {
				t.Fatal(err)
			}
			if _, err := controller.completeCandidateRequestDocument(ctx, "ws-v2", "run-v2", session.LogicalRequestID, physical); err != nil {
				t.Fatal(err)
			}
		}
	}
	before := repository.aggregate.CloneRaw()
	request, err := CompileRuntimeEvidenceApplicabilityRequest(before, before.Facts.Evidence.Accepted[0].ID)
	if err != nil || request.PurposeRef != runtimecontract.RuntimeHistoricalEffectProofV1 {
		t.Fatalf("audit request must reverse-prove original purpose: %v", err)
	}
	current := *runtimecontract.CloneRuntimeOwnerRevisionSet(&post)
	current.Entries[0].Revision = "later-owner-bytes"
	current = current.WithHash()
	return before, runtimecontract.RuntimeEvidenceApplicabilityObservation{Request: request, RecordedAtStateRevision: request.IssuedAtStateRevision + 1, CurrentDependencies: &current}
}

func TestHistoricalAuditPreservesOrdinaryCandidateAcrossCASAndRestart(t *testing.T) {
	for _, window := range []string{"prepared", "completed_document", "pending", "proposal"} {
		for _, fault := range []string{"none", "before_cas", "after_cas"} {
			t.Run(window+"/"+fault, func(t *testing.T) {
				before, observation := historicalAuditCandidateFixture(t, window)
				repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &before}, failBefore: fault == "before_cas", failAfter: fault == "after_cas"}
				controller, err := NewRuntimeController(repository)
				if err != nil {
					t.Fatal(err)
				}
				result, err := controller.ApplyInternalEvent(context.Background(), applicabilityInternalEvent(observation))
				if fault != "none" {
					if err == nil {
						t.Fatal("CAS fault was lost")
					}
					if fault == "before_cas" && !reflect.DeepEqual(*repository.aggregate, before) {
						t.Fatal("failed audit CAS leaked state")
					}
					reloaded := runtimeTestEvidenceAggregateRoundTrip(t, *repository.aggregate)
					repository.aggregate = &reloaded
					controller, err = NewRuntimeController(repository)
					if err != nil {
						t.Fatal(err)
					}
					result, err = controller.ApplyInternalEvent(context.Background(), applicabilityInternalEvent(observation))
				}
				if err != nil {
					t.Fatal(err)
				}
				after := result.Aggregate
				if repository.saves != 1 || after.StateRevision != before.StateRevision+1 || len(after.Facts.EvidenceApplicabilityObservations) != 1 ||
					!reflect.DeepEqual(after.Decision, before.Decision) || !reflect.DeepEqual(after.Facts.Coverage, before.Facts.Coverage) ||
					!reflect.DeepEqual(after.CandidateAttempts, before.CandidateAttempts) || !reflect.DeepEqual(after.CandidateRequestSessions, before.CandidateRequestSessions) ||
					!reflect.DeepEqual(after.OperationReceipts, before.OperationReceipts) || !reflect.DeepEqual(after.Facts.Evidence, before.Facts.Evidence) ||
					!reflect.DeepEqual(after.ExecutionBudget, before.ExecutionBudget) || !reflect.DeepEqual(after.SemanticPlanning, before.SemanticPlanning) || !reflect.DeepEqual(after.Reconciliation, before.Reconciliation) {
					t.Fatalf("historical audit changed current authority/candidate/effect/budget: saves=%d revisions=%d/%d observations=%d decision=%v coverage=%v attempts=%v sessions=%v receipts=%v evidence=%v budget=%v semantic=%v reconciliation=%v", repository.saves, after.StateRevision, before.StateRevision, len(after.Facts.EvidenceApplicabilityObservations), reflect.DeepEqual(after.Decision, before.Decision), reflect.DeepEqual(after.Facts.Coverage, before.Facts.Coverage), reflect.DeepEqual(after.CandidateAttempts, before.CandidateAttempts), reflect.DeepEqual(after.CandidateRequestSessions, before.CandidateRequestSessions), reflect.DeepEqual(after.OperationReceipts, before.OperationReceipts), reflect.DeepEqual(after.Facts.Evidence, before.Facts.Evidence), reflect.DeepEqual(after.ExecutionBudget, before.ExecutionBudget), reflect.DeepEqual(after.SemanticPlanning, before.SemanticPlanning), reflect.DeepEqual(after.Reconciliation, before.Reconciliation))
				}
				fact := after.Facts.Evidence.Accepted[0]
				if !runtimecontract.RuntimeEvidenceHasStaleApplicability(fact, after.Facts.EvidenceApplicabilityObservations, after.Facts.EvidenceRevision) ||
					runtimecontract.RuntimeEvidenceHasFreshApplicability(fact, after.Facts.EvidenceApplicabilityObservations, after.Facts.EvidenceRevision, after.Reconciliation.RequestHash) {
					t.Fatal("audit promoted historical bytes to current")
				}
				reloaded := runtimeTestEvidenceAggregateRoundTrip(t, after)
				repository.aggregate = &reloaded
				controller, err = NewRuntimeController(repository)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := controller.ApplyInternalEvent(context.Background(), applicabilityInternalEvent(observation)); err != nil || repository.saves != 1 {
					t.Fatalf("audit duplicate changed original observation: %v", err)
				}
				forged := reloaded.CloneRaw()
				forged.Facts.EvidenceApplicabilityObservations[0].Request.PurposeRef = ""
				forged.Facts.EvidenceApplicabilityObservations[0].Request = forged.Facts.EvidenceApplicabilityObservations[0].Request.WithID()
				if forged.Validate() == nil {
					t.Fatal("coordinated request rehash erased immutable receipt purpose")
				}
				for name, mutate := range map[string]func(*RuntimeAggregate){
					"action attempt": func(value *RuntimeAggregate) { value.Decision.AuthoritySnapshot.Attempt++ },
					"credit": func(value *RuntimeAggregate) {
						value.Facts.Coverage[0].State = runtimecontract.RuntimeCoverageItemStateSatisfied
					},
					"receipt":  func(value *RuntimeAggregate) { value.OperationReceipts[0].AfterHash = "rewritten" },
					"provider": func(value *RuntimeAggregate) { value.CandidateRequestSessions[0].NextSequence++ },
				} {
					changed := after.CloneRaw()
					mutate(&changed)
					if validateEvidenceApplicabilityObservationTransition(before, changed) == nil {
						t.Fatalf("audit CAS allowed an unrelated %s mutation", name)
					}
				}
				// Resume the original ordinary transaction, not merely its labels.
				// Prepared/completed sessions use the explicit test provider fact
				// helper; pending/proposal windows must reuse their exact attempt.
				if window == "prepared" || window == "completed_document" {
					if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "historical-audit-prefix"); err != nil {
						t.Fatalf("audit stranded original ordinary request: %v", err)
					}
				}
				candidate := runtimeTestBindCandidateToFacts(t, repository.aggregate.Facts, runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeManagedValidationParameters()}}}}})
				if err := RecordFixtureCandidateFeasibilities(context.Background(), controller, "ws-v2", "run-v2", 0, candidate); err != nil {
					t.Fatalf("audit stranded original ordinary candidate: %v", err)
				}
				sealed, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate)
				if err != nil || sealed.ActiveDispatch == nil || sealed.ActiveDispatch.Transaction != before.Decision.AuthorityTransaction() {
					t.Fatalf("ordinary candidate could not seal its original transaction after audit: %v", err)
				}
				if !reflect.DeepEqual(sealed.OperationReceipts, before.OperationReceipts) || len(sealed.Facts.Evidence.Accepted) != len(before.Facts.Evidence.Accepted) {
					t.Fatal("resuming ordinary validation manufactured an effect or acceptance fact")
				}
			})
		}
	}
}

func TestHistoricalAuditMissingResourcePreservesOrdinaryCandidate(t *testing.T) {
	before, observation := historicalAuditCandidateFixture(t, "prepared")
	observation.Missing, observation.CurrentDependencies = true, nil
	repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &before}}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	result, err := controller.ApplyInternalEvent(context.Background(), applicabilityInternalEvent(observation))
	if err != nil {
		t.Fatal(err)
	}
	after := runtimeTestEvidenceAggregateRoundTrip(t, result.Aggregate)
	if !after.Facts.EvidenceApplicabilityObservations[0].Missing ||
		!reflect.DeepEqual(before.Decision, after.Decision) || !reflect.DeepEqual(before.CandidateRequestSessions, after.CandidateRequestSessions) ||
		!reflect.DeepEqual(before.Facts.Coverage, after.Facts.Coverage) || !reflect.DeepEqual(before.OperationReceipts, after.OperationReceipts) ||
		!reflect.DeepEqual(before.ExecutionBudget, after.ExecutionBudget) || repository.saves != 1 {
		t.Fatal("historical missing-resource audit mutated ordinary authority or original facts")
	}
	fact := after.Facts.Evidence.Accepted[0]
	if !runtimecontract.RuntimeEvidenceHasStaleApplicability(fact, after.Facts.EvidenceApplicabilityObservations, after.Facts.EvidenceRevision) ||
		runtimecontract.RuntimeEvidenceHasFreshApplicability(fact, after.Facts.EvidenceApplicabilityObservations, after.Facts.EvidenceRevision, after.Reconciliation.RequestHash) {
		t.Fatal("missing historical resource became current")
	}
}
