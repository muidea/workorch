package orchestrator

import (
	"reflect"
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Real owner receipts/material/ACK; scripted reviewer/private startup. This
// is not a final Git snapshot producer or public implementation-work admission.
func TestDriveRuntimeTerminalMaterialProofOwnerMainlineAndRestart(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectConjunction, ValidationCount: 1, ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol, SourceObligations: true, TerminalMaterialGuards: true}
	baseline := runValidationInputReviewMainline(t, "consistent", "", options)
	for _, cut := range []string{"after_review_document", "after_review_binding", "after_project_receipt", "before_material_seal", "after_material_seal", "after_result_persistence", "after_workspace_persistence", "after_run_persistence"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "consistent", cut, options); calls != baseline {
				t.Fatalf("terminal consumer restart repeated provider budget: %d != %d", calls, baseline)
			}
		})
	}
	for _, mode := range []string{"conflict", "omission", "clarify"} {
		t.Run(mode, func(t *testing.T) { runValidationInputReviewMainline(t, mode, "", options) })
	}
}

func assertTerminalMaterialConsumerTamper(t *testing.T, source runtimecontract.RuntimeStateRecord, binding runtimecontract.RuntimeDeliveryLineageBinding) {
	t.Helper()
	material := source.TerminalMaterialSeal.Material.CloneRaw()
	before := source.Clone()
	if err := runtimecontract.ValidateRuntimeTerminalProofMaterial(source, material); err != nil {
		t.Fatalf("original terminal material proof: %v", err)
	}
	for name, mutate := range map[string]func(*runtimecontract.RuntimeStateRecord, *runtimecontract.RuntimeTerminalResultMaterial){
		"missing top index": func(s *runtimecontract.RuntimeStateRecord, m *runtimecontract.RuntimeTerminalResultMaterial) {
			m.EvidenceRefs = nil
		},
		"missing DoD refs": func(s *runtimecontract.RuntimeStateRecord, m *runtimecontract.RuntimeTerminalResultMaterial) {
			m.DoDAssessment.Items[0].EvidenceRefs = nil
		},
		"source criterion drift": func(s *runtimecontract.RuntimeStateRecord, m *runtimecontract.RuntimeTerminalResultMaterial) {
			m.DoDAssessment.Items[0].Item += "changed"
		},
		"summary omission": func(s *runtimecontract.RuntimeStateRecord, m *runtimecontract.RuntimeTerminalResultMaterial) {
			m.SummaryDetail.DoDItems = nil
		},
		"missing requirement": func(s *runtimecontract.RuntimeStateRecord, m *runtimecontract.RuntimeTerminalResultMaterial) {
			m.RuntimeCloseout.RequirementOutcomes = nil
		},
		"missing owner receipt": func(s *runtimecontract.RuntimeStateRecord, m *runtimecontract.RuntimeTerminalResultMaterial) {
			s.OperationReceipts = nil
		},
		"missing processed fence": func(s *runtimecontract.RuntimeStateRecord, m *runtimecontract.RuntimeTerminalResultMaterial) {
			s.ProcessedEventIDs = nil
		},
		"wrong evidence sequence": func(s *runtimecontract.RuntimeStateRecord, m *runtimecontract.RuntimeTerminalResultMaterial) {
			s.EvidenceSequence++
		},
		"stale binding": func(s *runtimecontract.RuntimeStateRecord, m *runtimecontract.RuntimeTerminalResultMaterial) {
			s.Facts.Coverage[1].CurrentBindingRef.BindingHash = "stale"
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed, body := source.Clone(), material.CloneRaw()
			mutate(&changed, &body)
			// Even a rehashed substituted material cannot replace original facts.
			closeout := body.RuntimeCloseout.WithCloseoutHash()
			body.RuntimeCloseout = &closeout
			if err := runtimecontract.ValidateRuntimeTerminalProofMaterial(changed, body); err == nil {
				t.Fatal("tampered terminal material accepted")
			}
		})
	}
	for _, ownerCount := range []int{0, 1, 2} {
		changed := source.Clone()
		changed.DeliveryAcknowledgements = changed.DeliveryAcknowledgements[:ownerCount]
		state, err := runtimecontract.EvaluateRuntimeDeliveryLineageAssertionFromState(changed, binding)
		if err != nil || state != runtimecontract.RuntimeDeliveryLineagePending {
			t.Fatalf("future ACK inferred at %d owners: %s %v", ownerCount, state, err)
		}
	}
	if !reflect.DeepEqual(before, source) {
		t.Fatal("terminal consumer mutated immutable material, authority or budgets")
	}
}
