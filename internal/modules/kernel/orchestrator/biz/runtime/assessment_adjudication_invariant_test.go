package runtime

import (
	"context"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAssessmentAdjudicationRejectsCoordinatedDispositionAndProofRewrite(t *testing.T) {
	controller, _, _ := prepareOwnerFactAssessment(t, true)
	completed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if err := completed.Validate(); err != nil {
		t.Fatalf("baseline assessment aggregate: %v", err)
	}
	dispatchIndex, factIndex := runtimeAssessmentAdjudicationIndexes(t, completed)
	if completed.CompletedDispatches[dispatchIndex].AssessmentAdjudication == nil || len(completed.CompletedDispatches[dispatchIndex].AssessmentAdjudication.Proofs) == 0 {
		t.Fatal("baseline assessment has no immutable adjudication proof")
	}
	sealedAuthorityHash := completed.CompletedDispatches[dispatchIndex].DispatchAuthorityHash
	postSealMutation := sealedDispatchToRuntimeRecord(completed.CompletedDispatches[dispatchIndex])
	postSealMutation.AssessmentAdjudication.Disposition = runtimecontract.AssessmentDispositionRepairableGap
	if runtimecontract.StableRuntimeDispatchAuthorityHash(postSealMutation) != sealedAuthorityHash {
		t.Fatal("post-seal assessment adjudication entered immutable dispatch authority hash")
	}
	repairableController, _, _ := prepareOwnerFactAssessment(t, false)
	repairable, err := repairableController.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	repairableDispatchIndex, repairableFactIndex := runtimeAssessmentAdjudicationIndexes(t, repairable)
	if repairable.CompletedDispatches[repairableDispatchIndex].AssessmentAdjudication == nil ||
		repairable.CompletedDispatches[repairableDispatchIndex].AssessmentAdjudication.Disposition != runtimecontract.AssessmentDispositionRepairableGap ||
		repairable.Facts.Assessments[repairableFactIndex].Disposition != runtimecontract.AssessmentDispositionRepairableGap {
		t.Fatalf("baseline assessment is not canonically repairable: adjudication=%+v fact=%+v decision=%+v", repairable.CompletedDispatches[repairableDispatchIndex].AssessmentAdjudication, repairable.Facts.Assessments[repairableFactIndex], repairable.Decision)
	}

	t.Run("satisfied to repairable coordinated flip", func(t *testing.T) {
		drifted := cloneRuntimeAggregateForMCPProvenanceTest(t, completed)
		dispatch := &drifted.CompletedDispatches[dispatchIndex]
		dispatch.AssessmentAdjudication.Disposition = runtimecontract.AssessmentDispositionRepairableGap
		dispatch.AssessmentAdjudicationHash = runtimecontract.StableRuntimeAssessmentAdjudicationHash(*dispatch.AssessmentAdjudication)
		drifted.Facts.Assessments[factIndex].Disposition = runtimecontract.AssessmentDispositionRepairableGap
		drifted.Facts.Assessments[factIndex].AdjudicationHash = dispatch.AssessmentAdjudicationHash
		if err := drifted.Validate(); err == nil {
			t.Fatal("coordinated satisfied-to-repairable disposition flip was accepted")
		}
	})

	t.Run("repairable to satisfied coordinated flip", func(t *testing.T) {
		drifted := cloneRuntimeAggregateForMCPProvenanceTest(t, repairable)
		dispatch := &drifted.CompletedDispatches[repairableDispatchIndex]
		dispatch.AssessmentAdjudication.Disposition = runtimecontract.AssessmentDispositionSatisfied
		dispatch.AssessmentAdjudicationHash = runtimecontract.StableRuntimeAssessmentAdjudicationHash(*dispatch.AssessmentAdjudication)
		drifted.Facts.Assessments[repairableFactIndex].Disposition = runtimecontract.AssessmentDispositionSatisfied
		drifted.Facts.Assessments[repairableFactIndex].AdjudicationHash = dispatch.AssessmentAdjudicationHash
		if err := drifted.Validate(); err == nil {
			t.Fatal("coordinated repairable-to-satisfied disposition flip was accepted")
		}
	})

	t.Run("proof and hash coordinated rewrite", func(t *testing.T) {
		drifted := cloneRuntimeAggregateForMCPProvenanceTest(t, completed)
		dispatch := &drifted.CompletedDispatches[dispatchIndex]
		dispatch.AssessmentAdjudication.Proofs[0].EvidenceFactHash = "coordinated-fact-hash"
		dispatch.AssessmentAdjudicationHash = runtimecontract.StableRuntimeAssessmentAdjudicationHash(*dispatch.AssessmentAdjudication)
		drifted.Facts.Assessments[factIndex].AdjudicationHash = dispatch.AssessmentAdjudicationHash
		if err := drifted.Validate(); err == nil {
			t.Fatal("coordinated assessment proof/hash rewrite was accepted")
		}
	})

	t.Run("raw normalization laundering", func(t *testing.T) {
		drifted := cloneRuntimeAggregateForMCPProvenanceTest(t, completed)
		dispatch := &drifted.CompletedDispatches[dispatchIndex]
		dispatch.AssessmentAdjudication.DispatchID += " "
		// The stable hash normalizes this drift; only the raw-canonical fence can
		// reject it before normalization launders the persisted record.
		dispatch.AssessmentAdjudicationHash = runtimecontract.StableRuntimeAssessmentAdjudicationHash(*dispatch.AssessmentAdjudication)
		drifted.Facts.Assessments[factIndex].AdjudicationHash = dispatch.AssessmentAdjudicationHash
		if err := drifted.Validate(); err == nil {
			t.Fatal("non-canonical raw adjudication was accepted")
		}
	})
}

func TestRuntimeAssessmentAdjudicationSurvivesRestartAndRawCloneDetaches(t *testing.T) {
	controller, repository, bootstrap := prepareOwnerFactAssessment(t, true)
	completed, err := controller.commitAssessment(context.Background(), "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	dispatchIndex, _ := runtimeAssessmentAdjudicationIndexes(t, completed)
	record := runtimecontract.RuntimeStateRecord{CompletedDispatches: []runtimecontract.RuntimeDispatchRecord{sealedDispatchToRuntimeRecord(completed.CompletedDispatches[dispatchIndex])}}
	clone := record.Clone()
	clone.CompletedDispatches[0].AssessmentAdjudication.Proofs[0].EvidenceFactHash = "mutated-clone"
	if completed.CompletedDispatches[dispatchIndex].AssessmentAdjudication.Proofs[0].EvidenceFactHash == "mutated-clone" {
		t.Fatal("runtime aggregate clone shares assessment adjudication proof backing storage")
	}
	// Restart must replay the exact Definition that was sealed with the
	// capability-aware bootstrap, not reconstruct an older generic fixture.
	restarted, err := controller.initialize(context.Background(), bootstrap, completed.Definition, nil)
	if err != nil {
		t.Fatal(err)
	}
	if repository.saves == 0 || restarted.StateRevision != completed.StateRevision {
		t.Fatalf("restart changed committed assessment revision: restarted=%d completed=%d", restarted.StateRevision, completed.StateRevision)
	}
	if err := restarted.Validate(); err != nil {
		t.Fatalf("restarted assessment aggregate: %v", err)
	}
}

func runtimeAssessmentAdjudicationIndexes(t *testing.T, aggregate RuntimeAggregate) (int, int) {
	t.Helper()
	dispatchIndex := -1
	for idx := range aggregate.CompletedDispatches {
		if aggregate.CompletedDispatches[idx].Candidate.Assessment != nil {
			dispatchIndex = idx
			break
		}
	}
	if dispatchIndex < 0 {
		t.Fatal("completed aggregate has no assessment dispatch")
	}
	factIndex := -1
	for idx := range aggregate.Facts.Assessments {
		if aggregate.Facts.Assessments[idx].ID == aggregate.CompletedDispatches[dispatchIndex].ID {
			factIndex = idx
			break
		}
	}
	if factIndex < 0 {
		t.Fatal("completed assessment dispatch has no fact")
	}
	return dispatchIndex, factIndex
}
