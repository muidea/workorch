package workorchmanagementcontract

import (
	"strings"
	"testing"
	"time"
)

func TestPreparationPerCheckJudgmentShapeNeverGrantsReadiness(t *testing.T) {
	fixture := func() StartupPreparationView {
		v := preparationViewFixture()
		v.Record.Version, v.OperationState = "workorch-preparation-record/v2", "blocked"
		a := &v.Record.Attempts[0]
		a.State, a.CreationEpoch = "blocked", strings.Repeat("a", 32)
		a.EvaluationVersion = "workorch-preparation-check-evaluation/v1"
		a.DirectoryEvidenceHash = strings.Repeat("b", 64)
		at := a.StartedAt.Add(time.Second)
		a.CompletedAt = &at
		a.Checks = []PreparationCheckView{{ID: "directory_boundary", Owner: "file", Required: true, State: "passed", Reason: "directory_observed_remaining_checks"}, {ID: "exact_seal", Owner: "workspace", Required: true, State: "blocked", Reason: "check_pipeline_incomplete"}}
		return v
	}
	if fixture().ValidateFor("ws") != nil {
		t.Fatal("mixed original judgments rejected")
	}
	for _, mutate := range []func(*PreparationAttemptView){
		func(a *PreparationAttemptView) { a.EvaluationVersion = "" },
		func(a *PreparationAttemptView) { a.EvaluationVersion = "future" },
		func(a *PreparationAttemptView) { a.DirectoryEvidenceHash = "" },
		func(a *PreparationAttemptView) { a.Checks[0].Owner = "workspace" },
		func(a *PreparationAttemptView) { a.Checks[0].Required = false },
		func(a *PreparationAttemptView) { a.Checks[0].Reason = "binding_changed" },
		func(a *PreparationAttemptView) { a.Checks[1].State = "passed" },
		func(a *PreparationAttemptView) { a.State = "ready" },
	} {
		v := fixture()
		mutate(&v.Record.Attempts[0])
		if v.ValidateFor("ws") == nil {
			t.Fatal("unbound/invented success accepted")
		}
	}
}

func TestPreparationSoulJudgmentRequiresVersionAndOriginalReference(t *testing.T) {
	fixture := func() StartupPreparationView {
		v := preparationViewFixture()
		v.Record.Version, v.OperationState = "workorch-preparation-record/v2", "blocked"
		a := &v.Record.Attempts[0]
		a.State, a.CreationEpoch = "blocked", strings.Repeat("a", 32)
		a.EvaluationVersion = "workorch-preparation-check-evaluation/v1"
		a.SoulEvidenceRef = "soul-observation:" + strings.Repeat("c", 32)
		at := a.StartedAt.Add(time.Second)
		a.CompletedAt = &at
		a.Checks = []PreparationCheckView{{ID: "soul_selection", Owner: "soul", Required: true, State: "passed", Reason: "souls_observed_remaining_checks"}}
		return v
	}
	if fixture().ValidateFor("ws") != nil {
		t.Fatal("exact original Soul judgment rejected")
	}
	for _, mutate := range []func(*PreparationAttemptView){
		func(a *PreparationAttemptView) { a.EvaluationVersion = "" },
		func(a *PreparationAttemptView) { a.SoulEvidenceRef = "" },
		func(a *PreparationAttemptView) { a.Checks[0].Owner = "file" },
		func(a *PreparationAttemptView) { a.Checks[0].Required = false },
		func(a *PreparationAttemptView) { a.Checks[0].Reason = "binding_changed" },
		func(a *PreparationAttemptView) { a.Checks[0].Reason = "original_check_interrupted" },
		func(a *PreparationAttemptView) { a.Checks[0].State = "failed" },
		func(a *PreparationAttemptView) { a.State = "ready" },
	} {
		v := fixture()
		mutate(&v.Record.Attempts[0])
		if v.ValidateFor("ws") == nil {
			t.Fatal("unbound/stale Soul judgment accepted")
		}
	}
}
