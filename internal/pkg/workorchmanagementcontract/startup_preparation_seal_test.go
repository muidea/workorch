package workorchmanagementcontract

import (
	"strings"
	"testing"
	"time"
)

func TestPreparationSealWireRequiresOriginalBlockedReference(t *testing.T) {
	for _, id := range []string{"exact_seal", "persistence_integrity"} {
		fixture := func() StartupPreparationView {
			v := preparationViewFixture()
			v.Record.Version, v.OperationState = "workorch-preparation-record/v2", "blocked"
			a := &v.Record.Attempts[0]
			a.State, a.CreationEpoch, a.EvaluationVersion = "blocked", strings.Repeat("a", 32), "workorch-preparation-check-evaluation/v1"
			a.SealEvidenceRef = "seal-observation:" + strings.Repeat("b", 32)
			at := a.StartedAt.Add(time.Second)
			a.CompletedAt = &at
			reason := "seal_observed_remaining_checks"
			if id == "persistence_integrity" {
				reason = "persistence_binding_observed_remaining_checks"
			}
			a.Checks = []PreparationCheckView{{ID: id, Owner: "workspace", Required: true, State: "passed", Reason: reason}}
			return v
		}
		if fixture().ValidateFor("ws") != nil {
			t.Fatal("valid original shape rejected", id)
		}
		for _, change := range []string{"ref", "bad_ref", "version", "owner", "required", "ready", "reason", "legacy"} {
			v := fixture()
			a := &v.Record.Attempts[0]
			switch change {
			case "ref":
				a.SealEvidenceRef = ""
			case "bad_ref":
				a.SealEvidenceRef = "seal-observation:raw"
			case "version":
				a.EvaluationVersion = ""
			case "owner":
				a.Checks[0].Owner = "caller"
			case "required":
				a.Checks[0].Required = false
			case "ready":
				a.State, v.OperationState = "ready", "ready"
			case "reason":
				a.Checks[0].Reason = "ready"
			case "legacy":
				v.Record.Version = "workorch-preparation-record/v1"
			}
			if v.ValidateFor("ws") == nil {
				t.Fatal("unbound judgment accepted", id, change)
			}
		}
	}
}
