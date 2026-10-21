package workorchmanagementcontract

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func preparationViewFixture() StartupPreparationView {
	return StartupPreparationView{WorkspaceID: "ws", WorkspaceRevision: 2, OperationState: "unknown", CurrentValidity: "stale", Record: &PreparationRecordView{Version: "workorch-preparation-record/v1", Attempts: []PreparationAttemptView{{
		RequestID: "original-request", PayloadHash: "payload", ExpectedWorkspaceRevision: 1, CommittedWorkspaceRevision: 2, StartedAt: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC), State: "unknown",
		Binding: PreparationBindingView{InputHash: "input", DerivationPermitID: "permit", ProposalHash: "proposal", SpecificationHash: "spec", CandidateID: "candidate", CandidateRevision: 1, CandidateHash: "candidate-hash", DefinitionRevision: "definition", StartupInputHash: "startup", AuthoringConfigurationHash: "config", SnapshotHash: "snapshot", ManifestHash: "manifest", CheckPolicyVersion: "policy", CompilerVersion: "compiler"},
	}}}}
}

func TestStartupPreparationViewKeepsOperationAndValiditySeparate(t *testing.T) {
	view := preparationViewFixture()
	if view.ValidateFor("ws") != nil {
		t.Fatal("stale unknown rejected")
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var restored StartupPreparationView
	if err := json.Unmarshal(encoded, &restored); err != nil || restored.ValidateFor("ws") != nil {
		t.Fatalf("roundtrip: %v", err)
	}
	if restored.OperationState != "unknown" || restored.CurrentValidity != "stale" || restored.Record.Attempts[0].RequestID != "original-request" {
		t.Fatal("unknown operation settled")
	}
	for _, field := range []string{`"inputHash"`, `"derivationPermitId"`, `"proposalHash"`, `"specificationHash"`, `"startupInputHash"`, `"manifestHash"`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("missing identity %s", field)
		}
	}
	for name, mutate := range map[string]func(*StartupPreparationView){
		"scope":           func(v *StartupPreparationView) { v.WorkspaceID = "other" },
		"record_missing":  func(v *StartupPreparationView) { v.Record = nil },
		"bad_version":     func(v *StartupPreparationView) { v.Record.Version += "-other" },
		"state_disagrees": func(v *StartupPreparationView) { v.OperationState = "blocked" },
		"false_current":   func(v *StartupPreparationView) { v.CurrentValidity = "valid" },
		"false_ready":     func(v *StartupPreparationView) { v.Record.Attempts[0].State = "ready"; v.OperationState = "ready" },
		"false_m3": func(v *StartupPreparationView) {
			v.Record.Attempts[0].State = "completed"
			v.OperationState = "completed"
		},
		"future_receipt":    func(v *StartupPreparationView) { v.Record.Attempts[0].CommittedWorkspaceRevision++ },
		"unbounded":         func(v *StartupPreparationView) { v.Record.Attempts[0].Binding.ManifestHash = strings.Repeat("x", 513) },
		"missing_binding":   func(v *StartupPreparationView) { v.Record.Attempts[0].Binding.InputHash = "" },
		"duplicate":         func(v *StartupPreparationView) { v.Record.Attempts = append(v.Record.Attempts, v.Record.Attempts[0]) },
		"unknown_completed": func(v *StartupPreparationView) { now := time.Now(); v.Record.Attempts[0].CompletedAt = &now },
	} {
		t.Run(name, func(t *testing.T) {
			changed := preparationViewFixture()
			mutate(&changed)
			if changed.ValidateFor("ws") == nil {
				t.Fatal("malformed projection accepted")
			}
		})
	}
}
