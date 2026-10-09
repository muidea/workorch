package runtime

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeLineageOwnerReceiptAssessmentANDAndArchive(t *testing.T) {
	for _, changed := range []bool{true, false} {
		t.Run(map[bool]string{true: "ordinary_proof_and_guard", false: "ordinary_proof_cannot_override_guard"}[changed], func(t *testing.T) {
			ctx := context.Background()
			var controller *RuntimeController
			var repository *controllerRepository
			var sealed RuntimeAggregate
			var binding runtimecontract.CoverageEvidenceBinding
			if changed {
				controller, repository, sealed, binding, _ = lineageReceiptSealedAggregate(t)
			} else {
				// 普通 File proof 真实形成；未有 working round，附加 guard 仍不得授 credit。
				rule := runtimecontract.RuntimeLineageAssertionRule{Ref: "required-work-round", Kind: runtimecontract.RuntimeLineageCompletedWorkRounds, ConsumptionPoint: runtimecontract.RuntimeProofPhaseExecution, MinCompletedRounds: 1}
				controller, repository, sealed, binding, _ = lineageReceiptSealedAggregateWithRule(t, rule)
			}
			claimed, receipt := lineageReceiptClaim(t, controller, sealed)
			observed := lineageReceiptObserve(t, controller, claimed, receipt)
			evidence := lineageReceiptEvidence(t, observed, receipt)
			committed, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventOperationEvidenceObserved, Scope: lineageReceiptScope(observed), Transaction: &RuntimeInternalTransactionPayload{ExecutionEvidence: &evidence}})
			if err != nil {
				t.Fatal(err)
			}
			if committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
				t.Fatal("owner receipt 绕过独立 assessment")
			}
			if !canonicalAssessmentProofCompleteWithoutOwnerFactsForPurpose(committed.Aggregate, runtimecontract.RuntimeAssessmentPurposeDefinitionProof) && changed {
				t.Fatal("合法普通 proof 与 guard 未合取满足")
			}
			if _, err := controller.BeginCandidateAttemptForTest(ctx, sealed.Facts.WorkspaceID, sealed.Facts.RunID, 0, "receipt-assessment-prefix"); err != nil {
				t.Fatal(err)
			}
			assessment := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{FocusID: committed.Aggregate.Facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{binding.CoverageWorkItemID}, Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "producer 声称满足，仍必须由 owner facts 与 guard 合取裁决", EvidenceRefs: []string{string(committed.Aggregate.Facts.Evidence.Accepted[0].ID)}}}
			admitted, err := controller.AcceptCandidateAttempt(ctx, sealed.Facts.WorkspaceID, sealed.Facts.RunID, 0, assessment)
			if err != nil {
				t.Fatal(err)
			}
			originalDispatch := *admitted.ActiveDispatch
			settled, err := controller.commitAssessment(ctx, sealed.Facts.WorkspaceID, sealed.Facts.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if err := settled.Validate(); err != nil {
				t.Fatal(err)
			}
			want := runtimecontract.AssessmentDispositionRepairableGap
			if changed {
				want = runtimecontract.AssessmentDispositionSatisfied
			}
			if len(settled.Facts.Assessments) != 1 || settled.Facts.Assessments[0].Disposition != want || settled.Facts.Assessments[0].ClaimedDisposition != runtimecontract.AssessmentDispositionSatisfied {
				t.Fatalf("guard 与 ordinary proof 未在原 assessment CAS 合取: want=%s", want)
			}
			var archived *SealedDispatch
			for index := range settled.CompletedDispatches {
				if settled.CompletedDispatches[index].ID == originalDispatch.ID {
					archived = &settled.CompletedDispatches[index]
				}
			}
			if archived == nil || archived.AssessmentAdjudication == nil || archived.Transaction.EvidenceRevision != originalDispatch.Transaction.EvidenceRevision {
				t.Fatal("归档缺失 exact assessment cutoff 或 adjudication")
			}
			before := settled.CloneRaw()
			encoded, err := json.Marshal(settled)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded RuntimeAggregate
			if err := json.Unmarshal(encoded, &reloaded); err != nil {
				t.Fatal(err)
			}
			if err := reloaded.Validate(); err != nil {
				t.Fatalf("archive fresh-load: %v", err)
			}
			if !reflect.DeepEqual(settled, before) || repository.aggregate.StateRevision != settled.StateRevision {
				t.Fatal("归档校验改写 aggregate 或追加写入")
			}
		})
	}
}
