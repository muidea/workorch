package runtime

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSemanticExecutionFeedbackUsesExactPersistedRepairOrigin(t *testing.T) {
	a := runtimeValidationFailureRepairAggregateForTest(t, runtimeValidationFailureRepairConvergenceForTest(2))
	repository := &durableRuntimeStateRepository{aggregate: a}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	evidence := runtimeRejectedValidationEvidenceForTest(t, sealed, "feedback-rejection", runtimecontract.ValidationDiagnosticBuildFailed)
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	a = committed.Aggregate
	input, required, err := CurrentRuntimeValidationFailureRepairGenerationInput(a)
	if err != nil || !required {
		t.Fatalf("repair input: %+v required=%v err=%v", input, required, err)
	}
	c := runtimecontract.RuntimeSemanticPlanRequestContext{WorkspaceID: a.Facts.WorkspaceID, RunID: a.Facts.RunID, Definition: a.Definition,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{input.CoverageWorkItemID}}
	m := runtimecontract.RuntimeSemanticPlanManifest{Hash: "feedback-manifest", ProofObligations: []runtimecontract.RuntimeSemanticProofObligation{{
		CoverageWorkItemID: input.CoverageWorkItemID, ValidationFailureRepair: &runtimecontract.RuntimeSemanticValidationFailureRepair{
			FailedFocusID: input.FailedFocusID, FailureSignature: input.FailureSignature, TriggerOwnerModuleID: input.TriggerOwnerModuleID, TriggerOperationKind: input.TriggerOperationKind,
		},
	}}}
	baseline, err := SemanticExecutionFeedback(a, c, m)
	if err != nil || len(baseline) != 1 {
		t.Fatalf("receipt projection: %+v %v", baseline, err)
	}
	expectedDiagnostic, err := runtimecontract.ProjectRuntimeSemanticExecutionDiagnostic(a.OperationReceipts[0].Feedback, input.TriggerOwnerModuleID)
	if err != nil || expectedDiagnostic == nil || !reflect.DeepEqual(baseline[0].Diagnostic, expectedDiagnostic) {
		t.Fatalf("semantic repair lost committed owner diagnostic: %+v %v", baseline[0].Diagnostic, err)
	}
	// Serialization is a test-only fresh-load boundary, not an authority bridge.
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	reload := func() RuntimeAggregate {
		var fresh RuntimeAggregate
		if err := json.Unmarshal(data, &fresh); err != nil {
			t.Fatal(err)
		}
		return fresh
	}
	fresh := reload()
	if err := fresh.Validate(); err != nil {
		t.Fatal(err)
	}
	replayed, err := SemanticExecutionFeedback(fresh, c, m)
	if err != nil || !reflect.DeepEqual(baseline, replayed) {
		t.Fatalf("restart changed feedback: %+v %v", replayed, err)
	}
	for name, mutate := range map[string]func(*RuntimeAggregate){
		"missing_receipt":  func(v *RuntimeAggregate) { v.OperationReceipts = nil },
		"missing_evidence": func(v *RuntimeAggregate) { v.Facts.Evidence.Rejected = nil },
		"duplicate_evidence": func(v *RuntimeAggregate) {
			v.Facts.Evidence.Rejected = append(v.Facts.Evidence.Rejected, v.Facts.Evidence.Rejected[0])
		},
		"wrong_signature":     func(v *RuntimeAggregate) { v.Facts.Evidence.Rejected[0].FailureSignature = "unrelated" },
		"wrong_payload":       func(v *RuntimeAggregate) { v.Facts.Evidence.Rejected[0].Summary = "tampered" },
		"receipt_hash":        func(v *RuntimeAggregate) { v.OperationReceipts[0].AfterHash = "tampered" },
		"diagnostic preimage": func(v *RuntimeAggregate) { v.OperationReceipts[0].Feedback.OriginalDiagnostic.OriginalCode = "other" },
		"wrong_trigger":       func(v *RuntimeAggregate) { v.Facts.Coverage[0].ReopenLineage[0].TriggerRef = "unrelated" },
		"wrong_binding": func(v *RuntimeAggregate) {
			v.Facts.Coverage[0].ReopenLineage[0].PreviousBindingRef.BindingHash = "unrelated"
		},
		"wrong_commit": func(v *RuntimeAggregate) {
			v.CompletedDispatches[0].OperationExecutions[0].State = runtimecontract.OperationExecutionOwnerCommitted
		},
		"wrong_workspace": func(v *RuntimeAggregate) { v.Facts.WorkspaceID = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := reload()
			mutate(&changed)
			if _, err := SemanticExecutionFeedback(changed, c, m); err == nil {
				t.Fatal("invalid execution provenance accepted")
			}
		})
	}
	// Unrelated history and mutable current failure are not causal inputs.
	fresh.Facts.Failure = nil
	fresh.Facts.Evidence.Rejected = append(fresh.Facts.Evidence.Rejected, runtimecontract.EvidenceFact{ID: "unrelated", FailureSignature: input.FailureSignature})
	replayed, err = SemanticExecutionFeedback(fresh, c, m)
	if err != nil || !reflect.DeepEqual(baseline, replayed) {
		t.Fatalf("mutable current failure or unrelated history changed feedback: %+v %v", replayed, err)
	}
	if after, _ := json.Marshal(a); string(after) != string(data) {
		t.Fatal("projection mutated aggregate")
	}
}
