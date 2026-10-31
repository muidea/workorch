package workorchmanagementcontract

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestPreparationDiagnosticDTORejectsContradictionsWithoutGrantingCompletion(t *testing.T) {
	raw, err := os.ReadFile("testdata/startup_preparation_completed.json")
	if err != nil {
		t.Fatal(err)
	}
	var v StartupPreparationView
	if json.Unmarshal(raw, &v) != nil {
		t.Fatal("fixture decode")
	}
	v.Record.Completions = nil
	v.OperationState = "blocked"
	v.CurrentValidity = "stale"
	v.CurrentValidityReason = "owner_observation_stale"
	a := &v.Record.Attempts[0]
	a.Finalization = &PreparationFinalizationView{Code: "owner_observation_incomplete", Owner: "mcpregistry", Stage: "final_owner_revalidation"}
	a.Observations = []PreparationObservationView{{Phase: "final", Owner: "mcpregistry", Stage: "registry_after", Code: "deadline_exceeded", StartedAt: a.StartedAt, CompletedAt: *a.CompletedAt, Deadline: a.CompletedAt, Attempted: true, ErrorByteCount: 30, ErrorSHA256: strings.Repeat("a", 64)}}
	if err := v.ValidateFor(v.WorkspaceID); err != nil {
		t.Fatal("valid final rejection", err)
	}
	encoded, _ := json.Marshal(v)
	if strings.Contains(string(encoded), "ErrorBytes") || strings.Contains(string(encoded), "errorBytes") {
		t.Fatal("raw errors in public list DTO")
	}
	for _, mutate := range []func(*PreparationAttemptView){
		func(a *PreparationAttemptView) { a.Finalization.Code = "ready" },
		func(a *PreparationAttemptView) { a.Observations[0].Code = "ready" },
		func(a *PreparationAttemptView) { a.Observations = append(a.Observations, a.Observations[0]) },
		func(a *PreparationAttemptView) { a.Observations[0].ErrorByteCount = 16385 },
		func(a *PreparationAttemptView) {
			a.Observations[0].Attempted = false
			a.Observations[0].Code = "observed"
		},
	} {
		var copy StartupPreparationView
		json.Unmarshal(encoded, &copy)
		mutate(&copy.Record.Attempts[0])
		if copy.ValidateFor(copy.WorkspaceID) == nil {
			t.Fatal("contradictory diagnostic accepted")
		}
	}
}
