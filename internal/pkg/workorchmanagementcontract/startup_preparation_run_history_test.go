package workorchmanagementcontract

import (
	"strings"
	"testing"
	"time"
)

func TestPreparationRunHistoryReferenceCannotConferNoRunReadiness(t *testing.T) {
	fixture := func() StartupPreparationView {
		v := preparationViewFixture()
		v.Record.Version, v.OperationState = "workorch-preparation-record/v2", "blocked"
		a := &v.Record.Attempts[0]
		a.State, a.CreationEpoch, a.RunHistoryEvidenceRef = "blocked", strings.Repeat("a", 32), "run-history-observation:"+strings.Repeat("b", 32)
		end := a.StartedAt.Add(time.Second)
		a.CompletedAt = &end
		a.Checks = []PreparationCheckView{{ID: "first_activation_no_run", Owner: "run", Required: true, State: "blocked", Reason: "run_history_empty_fence_not_final"}}
		return v
	}
	for _, reason := range []string{"run_history_present", "run_history_retired", "run_history_empty_fence_not_final", "binding_changed"} {
		v := fixture()
		v.Record.Attempts[0].Checks[0].Reason = reason
		if v.ValidateFor("ws") != nil {
			t.Fatal("original blocked history rejected", reason)
		}
	}
	for _, change := range []func(*StartupPreparationView){
		func(v *StartupPreparationView) { v.Record.Version = "workorch-preparation-record/v1" },
		func(v *StartupPreparationView) {
			v.Record.Attempts[0].RunHistoryEvidenceRef = "run-history-observation:ready"
		},
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks = nil },
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks[0].Owner = "caller" },
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks[0].Required = false },
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks[0].State = "passed" },
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks[0].Reason = "run_history_unavailable" },
		func(v *StartupPreparationView) { v.Record.Attempts[0].State = "unknown" },
	} {
		v := fixture()
		change(&v)
		if v.ValidateFor("ws") == nil {
			t.Fatal("unbound history reference or false credit accepted")
		}
	}
}
