package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRejectedValidationFeedbackCommitsAtomicallyAndReplaysWithoutDebt(t *testing.T) {
	seed := &durableRuntimeStateRepository{aggregate: validationAggregateWithPreparation(t, func(a *RuntimeAggregate) {
		a.Facts.Focus.RequiredCheckRefs = []string{"workorch-cwi-check-feedback"}
	})}
	sealed := sealCandidateForTest(t, seed, validationCandidate())
	operation := candidateRuntimeOperations(sealed.ActiveDispatch.Candidate)[0]
	binding, found := runtimecontract.CoverageEvidenceBindingForRef(sealed.Facts.Focus.CoverageEvidenceBindings, operation.BindingRefs[0])
	if !found {
		t.Fatal("sealed validation has no proof binding")
	}
	proof, found := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindValidation)
	if !found || len(proof.ValidationCheckAttestations) == 0 {
		t.Fatalf("fixture requires an exact native check: %+v", proof)
	}
	check := proof.ValidationCheckAttestations[0]
	failure := orchestratorevents.RuntimeOperationFailure(sealed.ActiveDispatch.Transaction, operation.Kind, "test_failed", "native check failed", nil)
	evidence := ExecutionEvidence{EventID: "native-check-rejected", WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID,
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: operation.ID, Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusRejected, Failure: failure,
		Evidence: runtimecontract.RuntimeOperationEvidence{Validation: &runtimecontract.RuntimeValidationObservation{
			RecipeID: proof.ValidationRecipeID, ObservationProtocolRef: check.ObservationProtocolRef,
			ExecutedChecks: []string{check.NativeCheckRef}, FailedChecks: []string{check.NativeCheckRef},
			CheckFailures: []runtimecontract.RuntimeValidationCheckFailure{{CheckRef: check.NativeCheckRef, TargetRef: "checks/a.test", Line: 47, DiagnosticHash: strings.Repeat("a", 64)}},
		}},
	}
	attachRuntimeTestOwnerReceipt(t, &seed.aggregate, &evidence)
	for name, mode := range map[string]failureAuthorityCASMode{
		"normal": failureAuthorityCASNormal, "before commit": failureAuthorityCASFailBeforeWrite, "lost response": failureAuthorityCASLoseResponse,
	} {
		t.Run(name, func(t *testing.T) {
			before := cloneRuntimeAggregateForMCPProvenanceTest(t, seed.aggregate)
			repository := &failureAuthorityCASRepository{aggregate: before, mode: mode}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			_, err = controller.commitEvidence(context.Background(), evidence)
			if mode == failureAuthorityCASNormal && err != nil {
				t.Fatal(err)
			}
			if mode != failureAuthorityCASNormal && !errors.Is(err, errFailureAuthorityCASCut) {
				t.Fatalf("CAS cut not observed: %v", err)
			}
			if mode == failureAuthorityCASFailBeforeWrite && !reflect.DeepEqual(repository.aggregate, before) {
				t.Fatal("pre-commit failure partially changed feedback or repair debt")
			}
			repository.mode = failureAuthorityCASNormal
			controller, err = NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			committed, err := controller.commitEvidence(context.Background(), evidence)
			if err != nil {
				t.Fatal(err)
			}
			if len(committed.Aggregate.Facts.Evidence.Rejected) != 1 || committed.Aggregate.StateRevision != before.StateRevision+1 {
				t.Fatal("recovery duplicated or lost rejection")
			}
			fact := committed.Aggregate.Facts.Evidence.Rejected[0]
			if fact.Validation == nil || len(fact.Validation.CheckFailures) != 1 || fact.Validation.CheckFailures[0].Line != 47 ||
				runtimecontract.RuntimeValidationFailureOccurrences(committed.Aggregate.Facts.Evidence, fact, committed.Aggregate.Facts.Focus.CoverageEvidenceBindings...) != 1 || committed.Aggregate.Facts.Repair.NoProgressCount != 0 {
				t.Fatalf("recovery lost native facts or counted a replay as repeated failure: observation=%+v repeats=%d debt=%+v", fact.Validation,
					runtimecontract.RuntimeValidationFailureOccurrences(committed.Aggregate.Facts.Evidence, fact, committed.Aggregate.Facts.Focus.CoverageEvidenceBindings...), committed.Aggregate.Facts.Repair)
			}
			if len(fact.CoverageWorkItemIDs) != 0 {
				t.Fatal("failure comparison manufactured CWI credit")
			}
			writes, debt := repository.saveAttempts, committed.Aggregate.Facts.Repair
			duplicate, err := controller.commitEvidence(context.Background(), evidence)
			if err != nil || duplicate.Disposition != CommitDispositionDuplicate || repository.saveAttempts != writes || !reflect.DeepEqual(duplicate.Aggregate.Facts.Repair, debt) {
				t.Fatal("duplicate failure feedback consumed repair debt")
			}
		})
	}
}

func TestRejectedValidationRepairDebtSurvivesNewTransactionIdentity(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{WorkspaceID: "ws", RunID: "run", Focus: runtimecontract.CoverageFocus{ID: "focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}},
		ConvergencePolicy: runtimecontract.RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 4}}
	facts.Focus.CoverageEvidenceBindings = []runtimecontract.CoverageEvidenceBinding{{ID: "binding-cwi", CoverageWorkItemID: "cwi"}}
	operation := runtimecontract.RuntimeOperation{Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}
	for i, outputHash := range []string{"sealed-output-before-repair", "sealed-output-new-closure", "sealed-output-another-closure"} {
		failure := orchestratorevents.RuntimeOperationFailure(runtimecontract.AuthorityTransaction{RequiredOutputHash: outputHash}, operation.Kind, "test_failed", "same check failed", nil)
		fact := runtimecontract.EvidenceFact{ID: common.EvidenceRecordID(outputHash), Kind: runtimecontract.EvidenceKindValidation,
			FocusID: "focus", BindingRefs: []string{"binding-cwi"}, FailureClass: failure.Class, FailureCode: failure.Code, FailureSignature: failure.Signature,
			Validation: &runtimecontract.RuntimeValidationObservation{RecipeID: "recipe", ObservationProtocolRef: "owner/checks/v1", FailedChecks: []string{"check-A"}}}
		gap := rejectedExecutionAssessmentGap(facts, fact, *failure)
		ledger, err := rejectedExecutionRepairLedger(facts, ExecutionEvidence{EventID: outputHash}, operation, fact, *failure, gap, failure.Signature)
		if err != nil {
			t.Fatal(err)
		}
		if ledger.NoProgressCount != i {
			t.Fatalf("round %d: same check failure erased by new closure: %+v", i, ledger)
		}
		if repairFailureSignatureCount(ledger, failure.Signature) != 1 {
			t.Fatal("transaction signatures were merged instead of compared separately")
		}
		facts.Repair = ledger
		facts.Evidence.Rejected = append(facts.Evidence.Rejected, fact)
	}
	if len(facts.Repair.SignatureCounts) != 3 || facts.Repair.RepairAttempts != 3 {
		t.Fatal("transaction repair accounting changed")
	}
}
