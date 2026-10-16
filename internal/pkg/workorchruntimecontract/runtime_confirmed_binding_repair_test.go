package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestConfirmedBindingRepairRuleOptInPreservesLegacyIdentity(t *testing.T) {
	legacy := RuntimeLineageAssertionRule{Ref: "original-repair", Kind: RuntimeLineageRepairThenFreshValidation, ConsumptionPoint: RuntimeProofPhaseExecution}
	body, err := json.Marshal(legacy)
	if err != nil || bytes.Contains(body, []byte("RepairHistoryContractRef")) {
		t.Fatal("legacy encoding upgraded")
	}
	opted := legacy
	opted.RepairHistoryContractRef = RuntimeConfirmedBindingRepairProofV1
	if opted.Validate() != nil || StableRuntimeLineageAssertionRuleHash(opted) == StableRuntimeLineageAssertionRuleHash(legacy) {
		t.Fatal("new contract not sealed")
	}
	body, _ = json.Marshal(opted)
	var reloaded RuntimeLineageAssertionRule
	if json.Unmarshal(body, &reloaded) != nil || !reflect.DeepEqual(reloaded, opted) || StableRuntimeLineageAssertionRuleHash(reloaded) != StableRuntimeLineageAssertionRuleHash(opted) {
		t.Fatal("reload changed rule identity")
	}
	for _, scenario := range []string{"version", "kind", "phase", "whitespace"} {
		changed := opted
		switch scenario {
		case "version":
			changed.RepairHistoryContractRef = "runtime/confirmed-binding-repair-proof/v0"
		case "kind":
			changed.Kind = RuntimeLineageCompletedWorkRounds
			changed.MinCompletedRounds = 2
		case "phase":
			changed.ConsumptionPoint = RuntimeProofPhasePostTerminalDelivery
		case "whitespace":
			changed.RepairHistoryContractRef = " " + RuntimeConfirmedBindingRepairProofV1
		}
		if changed.Validate() == nil {
			t.Fatal("unsupported repair consumer accepted", scenario)
		}
	}
	if runtimeConfirmedBindingRepairSatisfied(CoverageEvidenceBinding{}, RuntimeLineageAssertionSources{}, "runtime-evidence-00000000000000000001") {
		t.Fatal("missing original history generated proof")
	}
}

func TestConfirmedBindingRepairLaterRejectionDoesNotRequireCredit(t *testing.T) {
	binding := CoverageEvidenceBinding{ID: "ordinary-binding", CoverageWorkItemID: "original-cwi"}
	const after = "runtime-evidence-00000000000000000003"
	const cutoff = "runtime-evidence-00000000000000000005"
	for _, scenario := range []struct {
		name string
		fact EvidenceFact
		want bool
	}{
		{"uncredited exact binding", EvidenceFact{Kind: EvidenceKindValidation, BindingRefs: []string{binding.ID}, Revision: cutoff}, true},
		{"same CWI new binding", EvidenceFact{Kind: EvidenceKindValidation, BindingRefs: []string{"new-binding"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{binding.CoverageWorkItemID}, Revision: cutoff}, true},
		{"unrelated source", EvidenceFact{Kind: EvidenceKindValidation, BindingRefs: []string{"other-binding"}, Revision: cutoff}, false},
		{"original rejected validation", EvidenceFact{Kind: EvidenceKindValidation, BindingRefs: []string{binding.ID}, Revision: after}, false},
		{"future rejected validation", EvidenceFact{Kind: EvidenceKindValidation, BindingRefs: []string{binding.ID}, Revision: "runtime-evidence-00000000000000000006"}, false},
		{"historical effect", EvidenceFact{Kind: EvidenceKindStateChange, BindingRefs: []string{binding.ID}, Revision: cutoff}, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			original := []EvidenceFact{scenario.fact}
			body, err := json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			var reloaded []EvidenceFact
			if err := json.Unmarshal(body, &reloaded); err != nil {
				t.Fatal(err)
			}
			for _, facts := range [][]EvidenceFact{original, reloaded} {
				if got := runtimeConfirmedBindingHasLaterFailure(binding, facts, after, cutoff); got != scenario.want {
					t.Fatalf("later rejection: got %v, want %v", got, scenario.want)
				}
			}
			if !reflect.DeepEqual(original, reloaded) {
				t.Fatal("rejection source changed on reload")
			}
		})
	}
}

func TestConfirmedBindingValidationTargetScopeDoesNotInventAllTargetObligations(t *testing.T) {
	binding := CoverageEvidenceBinding{TargetRefs: []string{"entry", "implementation", "input"}}
	accepted := []EvidenceFact{{TargetRefs: []string{"entry"}}}
	for _, scenario := range []struct {
		name        string
		prospective []string
		facts       []EvidenceFact
		want        bool
	}{
		{"existing finite scope", nil, accepted, true},
		{"no accepted validation", nil, nil, false},
		{"complete prospective singleton", []string{"entry"}, accepted, true},
		{"missing prospective member", []string{"entry", "second"}, accepted, false},
		{"complete prospective set", []string{"entry", "second"}, []EvidenceFact{{TargetRefs: []string{"entry"}}, {TargetRefs: []string{"second"}}}, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			b := binding
			b.ProspectiveTargetRefs = scenario.prospective
			if got := runtimeConfirmedBindingValidationTargetsComplete(b, scenario.facts); got != scenario.want {
				t.Fatalf("target obligation completeness=%t want=%t", got, scenario.want)
			}
			body, err := json.Marshal(b)
			var reload CoverageEvidenceBinding
			if err != nil || json.Unmarshal(body, &reload) != nil || runtimeConfirmedBindingValidationTargetsComplete(reload, scenario.facts) != scenario.want {
				t.Fatal("reload changed explicit target obligations")
			}
		})
	}
}
