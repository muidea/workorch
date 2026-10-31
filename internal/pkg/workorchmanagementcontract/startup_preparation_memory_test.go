package workorchmanagementcontract

import (
	"strings"
	"testing"
	"time"
)

func TestPreparationMemoryJudgmentRequiresOriginalReferenceAndVersion(t *testing.T) {
	fixture := func(reason, state string) StartupPreparationView {
		v := preparationViewFixture()
		v.Record.Version, v.OperationState = "workorch-preparation-record", "blocked"
		a := &v.Record.Attempts[0]
		a.State, a.CreationEpoch = "blocked", strings.Repeat("a", 32)
		a.EvaluationVersion = "workorch-preparation-check-evaluation/v1"
		a.MemoryInventoryEvidenceRef = "memory-inventory-observation:" + strings.Repeat("b", 32)
		at := a.StartedAt.Add(time.Second)
		a.CompletedAt = &at
		a.Checks = []PreparationCheckView{{ID: "memory_selection", Owner: "memory", Required: true, State: state, Reason: reason}}
		return v
	}
	for _, pair := range [][2]string{{"memory_list_selection_observed_remaining_checks", "passed"}, {"memory_empty_selection_observed_remaining_checks", "passed"}, {"memory_semantic_evidence_required", "blocked"}, {"memory_selection_candidate_limit", "blocked"}} {
		if fixture(pair[0], pair[1]).ValidateFor("ws") != nil {
			t.Fatal("original Memory result rejected", pair)
		}
		v := fixture(pair[0], pair[1])
		v.Record.Attempts[0].MemoryInventoryEvidenceRef = ""
		if v.ValidateFor("ws") == nil {
			t.Fatal("missing Memory reference accepted", pair)
		}
	}
	for _, mutate := range []func(*PreparationAttemptView){
		func(a *PreparationAttemptView) { a.EvaluationVersion = "" },
		func(a *PreparationAttemptView) { a.EvaluationVersion = "future" },
		func(a *PreparationAttemptView) { a.MemoryInventoryEvidenceRef = "" },
		func(a *PreparationAttemptView) { a.Checks[0].Owner = "file" },
		func(a *PreparationAttemptView) { a.Checks[0].Required = false },
		func(a *PreparationAttemptView) { a.Checks[0].Reason = "memory_semantic_evidence_required" },
		func(a *PreparationAttemptView) { a.Checks[0].Reason = "memory_selection_candidate_limit" },
		func(a *PreparationAttemptView) { a.Checks[0].Reason = "binding_changed" },
		func(a *PreparationAttemptView) { a.Checks[0].Reason = "original_check_interrupted" },
		func(a *PreparationAttemptView) { a.Checks[0].State = "failed" },
		func(a *PreparationAttemptView) { a.State = "ready" },
	} {
		v := fixture("memory_list_selection_observed_remaining_checks", "passed")
		mutate(&v.Record.Attempts[0])
		if v.ValidateFor("ws") == nil {
			t.Fatal("unbound/invented Memory success accepted")
		}
	}
}

func TestPreparationMemoryInventoryReferenceNeverGrantsReadiness(t *testing.T) {
	fixture := func() StartupPreparationView {
		v := preparationViewFixture()
		v.Record.Version, v.OperationState, v.CurrentValidity = "workorch-preparation-record", "blocked", "not_evaluated"
		a := &v.Record.Attempts[0]
		a.State, a.CreationEpoch, a.MemoryInventoryEvidenceRef = "blocked", strings.Repeat("a", 32), "memory-inventory-observation:"+strings.Repeat("b", 32)
		at := a.StartedAt.Add(time.Second)
		a.CompletedAt = &at
		a.Checks = []PreparationCheckView{{ID: "memory_selection", Owner: "memory", Required: true, State: "blocked", Reason: "memory_inventory_observed_selection_pending"}}
		return v
	}
	for _, reason := range []string{"memory_inventory_observed_selection_pending", "binding_changed"} {
		v := fixture()
		v.Record.Attempts[0].Checks[0].Reason = reason
		if v.ValidateFor("ws") != nil {
			t.Fatal("valid historical cache reference rejected", reason)
		}
	}
	for _, change := range []func(*StartupPreparationView){
		func(v *StartupPreparationView) { v.Record.Version = "unsupported-record-contract" },
		func(v *StartupPreparationView) {
			v.Record.Attempts[0].MemoryInventoryEvidenceRef = "memory-inventory-observation:ready"
		},
		func(v *StartupPreparationView) { v.Record.Attempts[0].State = "accepted" },
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks = nil },
		func(v *StartupPreparationView) {
			v.Record.Attempts[0].Checks[0].Reason = "memory_inventory_unavailable_not_saved"
		},
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks[0].Owner = "caller" },
	} {
		v := fixture()
		change(&v)
		if v.ValidateFor("ws") == nil {
			t.Fatal("unbound cache reference accepted")
		}
	}
}
