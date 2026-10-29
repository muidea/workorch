package workorchmanagementcontract

import (
	"bytes"
	"encoding/json"
	"os"
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

func TestPreparationProjectIssueIsFiniteAndBoundToUnresolvedOperation(t *testing.T) {
	v := preparationViewFixture()
	v.Record.Version, v.OperationState = "workorch-preparation-record/v2", "blocked"
	a := &v.Record.Attempts[0]
	a.State, a.CreationEpoch = "blocked", strings.Repeat("a", 32)
	at := a.StartedAt.Add(time.Second)
	a.CompletedAt = &at
	a.SkillHubEffectRef, a.SkillHubEffectState = "skillhub-status:"+strings.Repeat("b", 32), "unresolved"
	a.Checks = []PreparationCheckView{{ID: "exact_seal", Owner: "workspace", Required: true, State: "blocked", Reason: "check_pipeline_incomplete"}}
	v.SkillHubProjectIssue = "project_registration_required"
	if v.ValidateFor("ws") != nil {
		t.Fatal("exact negative diagnostic rejected")
	}
	for _, mutate := range []func(*StartupPreparationView){
		func(v *StartupPreparationView) { v.SkillHubProjectIssue = "ready" },
		func(v *StartupPreparationView) { v.Record.Attempts[0].SkillHubEffectState = "recorded" },
		func(v *StartupPreparationView) { v.Record = nil },
	} {
		bad := v
		if v.Record != nil {
			r := *v.Record
			r.Attempts = append([]PreparationAttemptView(nil), v.Record.Attempts...)
			bad.Record = &r
		}
		mutate(&bad)
		if bad.ValidateFor("ws") == nil {
			t.Fatal("unbound diagnostic accepted")
		}
	}
}

func TestPreparationBootstrapCompilationSummaryIsNotReadiness(t *testing.T) {
	a := preparationViewFixture().Record.Attempts[0]
	a.ExecutionInputVersion = "workorch-preparation-execution-inputs/v1"
	a.DefinitionCompilation = &PreparationDefinitionCompilationView{}
	v := PreparationBootstrapCompilationView{Version: "workorch-original-prepared-bootstrap/v1", SnapshotHash: a.Binding.SnapshotHash, ManifestHash: a.Binding.ManifestHash, BootstrapHash: "prepared-bootstrap-sha256:" + strings.Repeat("a", 64)}
	if v.ValidateFor(a) != nil {
		t.Fatal("exact structural summary rejected")
	}
	for _, change := range []func(*PreparationBootstrapCompilationView){
		func(v *PreparationBootstrapCompilationView) { v.Version = "future" },
		func(v *PreparationBootstrapCompilationView) { v.SnapshotHash = "other" },
		func(v *PreparationBootstrapCompilationView) { v.ManifestHash = "other" },
		func(v *PreparationBootstrapCompilationView) {
			v.BootstrapHash = "prepared-bootstrap-sha256:" + strings.Repeat("z", 64)
		},
		func(v *PreparationBootstrapCompilationView) { v.FailureCode = "ready" },
		func(v *PreparationBootstrapCompilationView) { v.FailureCode = "memory_semantic_evidence_required" },
	} {
		bad := v
		change(&bad)
		if bad.ValidateFor(a) == nil {
			t.Fatal("substituted or contradictory summary accepted")
		}
	}
	v.BootstrapHash, v.FailureCode = "", "memory_semantic_evidence_required"
	if v.ValidateFor(a) != nil {
		t.Fatal("exact finite compilation failure rejected")
	}
}

func TestPreparationNoRunFinalViewRequiresExactVersionAndRemainsHistorical(t *testing.T) {
	fixture := func() StartupPreparationView {
		v := preparationViewFixture()
		v.WorkspaceRevision, v.OperationState, v.Record.Version = 3, "blocked", "workorch-preparation-record/v2"
		a := &v.Record.Attempts[0]
		a.State, a.CommittedWorkspaceRevision, a.CreationEpoch = "blocked", 3, strings.Repeat("a", 32)
		at := a.StartedAt.Add(time.Second)
		a.CompletedAt = &at
		a.EvaluationVersion, a.NoRunEvaluationVersion = "workorch-preparation-check-evaluation/v1", "workorch-preparation-no-run-final-fence/v1"
		a.RunHistoryEvidenceRef, a.SealEvidenceRef = "run-history-observation:"+strings.Repeat("b", 32), "seal-observation:"+strings.Repeat("c", 32)
		a.Checks = []PreparationCheckView{{ID: "exact_seal", Owner: "workspace", Required: true, State: "passed", Reason: "seal_observed_remaining_checks"}, {ID: "first_activation_no_run", Owner: "run", Required: true, State: "passed", Reason: "run_history_empty_final_fence_observed"}, {ID: "persistence_integrity", Owner: "workspace", Required: true, State: "passed", Reason: "persistence_binding_observed_remaining_checks"}}
		return v
	}
	for _, validity := range []string{"stale", "not_evaluated"} {
		v := fixture()
		v.CurrentValidity, v.ActiveRunID = validity, "later-run"
		if v.ValidateFor("ws") != nil {
			t.Fatal("historical proof was rewritten as a current no-Run claim")
		}
	}
	for _, scenario := range []string{"version", "future", "history", "seal", "evaluation", "epoch", "owner", "optional", "reason", "blocked", "seal_blocked", "v1", "ready", "revision", "orphan"} {
		t.Run(scenario, func(t *testing.T) {
			v := fixture()
			a := &v.Record.Attempts[0]
			switch scenario {
			case "version":
				a.NoRunEvaluationVersion = ""
			case "future":
				a.NoRunEvaluationVersion = "future"
			case "history":
				a.RunHistoryEvidenceRef = ""
			case "seal":
				a.SealEvidenceRef = ""
			case "evaluation":
				a.EvaluationVersion = ""
			case "epoch":
				a.PreparationEpoch = 1
			case "owner":
				a.Checks[1].Owner = "workspace"
			case "optional":
				a.Checks[1].Required = false
			case "reason":
				a.Checks[1].Reason = "run_history_empty_fence_not_final"
			case "blocked":
				a.Checks[1].State = "blocked"
			case "seal_blocked":
				a.Checks[0].State, a.Checks[0].Reason = "blocked", "binding_changed"
			case "v1":
				v.Record.Version = "workorch-preparation-record/v1"
			case "ready":
				a.State, v.OperationState = "ready", "ready"
			case "revision":
				a.CommittedWorkspaceRevision++
				v.WorkspaceRevision++
			case "orphan":
				a.Checks = nil
			}
			if v.ValidateFor("ws") == nil {
				t.Fatal("malformed final fence view accepted")
			}
		})
	}
}

func TestPreparationProviderCacheReferenceNeverGrantsReadiness(t *testing.T) {
	fixture := func() StartupPreparationView {
		v := preparationViewFixture()
		v.Record.Version, v.OperationState, v.CurrentValidity = "workorch-preparation-record/v2", "blocked", "not_evaluated"
		a := &v.Record.Attempts[0]
		a.State, a.CreationEpoch, a.ProviderCacheEvidenceRef = "blocked", strings.Repeat("a", 32), "provider-cache-observation:"+strings.Repeat("b", 32)
		at := a.StartedAt.Add(time.Second)
		a.CompletedAt = &at
		a.Checks = []PreparationCheckView{{ID: "provider_cache", Owner: "llm", Required: true, State: "blocked", Reason: "provider_declaration_only"}}
		return v
	}
	for _, reason := range []string{"provider_cache_miss", "provider_cache_expired", "provider_declaration_only", "binding_changed"} {
		v := fixture()
		v.Record.Attempts[0].Checks[0].Reason = reason
		if v.ValidateFor("ws") != nil {
			t.Fatal("valid historical cache reference rejected", reason)
		}
	}
	for _, change := range []func(*StartupPreparationView){
		func(v *StartupPreparationView) { v.Record.Version = "workorch-preparation-record/v1" },
		func(v *StartupPreparationView) {
			v.Record.Attempts[0].ProviderCacheEvidenceRef = "provider-cache-observation:ready"
		},
		func(v *StartupPreparationView) { v.Record.Attempts[0].State = "accepted" },
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks = nil },
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks[0].Reason = "provider_cache_unavailable" },
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks[0].Owner = "caller" },
	} {
		v := fixture()
		change(&v)
		if v.ValidateFor("ws") == nil {
			t.Fatal("unbound cache reference accepted")
		}
	}
}

func TestPreparationSkillHubEffectViewIsNotReadiness(t *testing.T) {
	fixture := func(state, summary string) StartupPreparationView {
		v := preparationViewFixture()
		v.Record.Version = "workorch-preparation-record/v2"
		v.OperationState = state
		a := &v.Record.Attempts[0]
		a.State = state
		a.CreationEpoch = strings.Repeat("a", 32)
		a.SkillHubEffectRef = "skillhub-status:" + strings.Repeat("b", 32)
		a.SkillHubEffectState = summary
		a.Checks = []PreparationCheckView{{ID: "exact_seal", Owner: "workspace", Required: true, State: "pending", Reason: "not_observed"}}
		if state == "blocked" {
			at := a.StartedAt.Add(time.Second)
			a.CompletedAt = &at
			a.Checks[0].State = "blocked"
			a.Checks[0].Reason = "check_pipeline_incomplete"
		}
		return v
	}
	for _, pair := range [][2]string{{"accepted", "reserved"}, {"unknown", "reserved"}, {"blocked", "recorded"}, {"blocked", "unresolved"}} {
		if fixture(pair[0], pair[1]).ValidateFor("ws") != nil {
			t.Fatal("historical state rejected", pair)
		}
	}
	for _, pair := range [][2]string{{"accepted", "recorded"}, {"blocked", "reserved"}, {"blocked", "ready"}, {"unknown", "unresolved"}} {
		if fixture(pair[0], pair[1]).ValidateFor("ws") == nil {
			t.Fatal("invented state accepted", pair)
		}
	}
	selectionView := fixture("blocked", "recorded")
	selected := &selectionView.Record.Attempts[0]
	selected.SkillRegistryEvidenceRef = "skill-registry-observation:" + strings.Repeat("c", 32)
	selected.SoulEvidenceRef = "soul-observation:" + strings.Repeat("d", 32)
	selected.SkillHubInstalledState = "observed_selection_pending"
	selected.SkillSelectionState = "compiled_checks_pending"
	selected.Checks = []PreparationCheckView{{ID: "skill_selection", Owner: "skill", Required: true, State: "blocked", Reason: "skill_selection_compiled_checks_pending"}}
	if selectionView.ValidateFor("ws") != nil {
		t.Fatal("valid selection history rejected")
	}
	for _, state := range []string{"compiled_checks_pending", "bounded_checks_pending", "rejected"} {
		selected.SkillContextState = state
		if selectionView.ValidateFor("ws") != nil {
			t.Fatal("valid pure context state rejected", state)
		}
	}
	for _, change := range []string{"ready", "missing_selection", "missing_registry", "v1", "accepted"} {
		v := selectionView
		record := *v.Record
		record.Attempts = append([]PreparationAttemptView(nil), record.Attempts...)
		v.Record = &record
		a := &v.Record.Attempts[0]
		switch change {
		case "ready":
			a.SkillContextState = "ready"
		case "missing_selection":
			a.SkillSelectionState = ""
		case "missing_registry":
			a.SkillRegistryEvidenceRef = ""
		case "v1":
			v.Record.Version = "workorch-preparation-record/v1"
		case "accepted":
			a.State = "accepted"
		}
		if v.ValidateFor("ws") == nil {
			t.Fatal("invented context success or unbound context accepted", change)
		}
	}
	selected.SkillContextState = ""
	for _, state := range []string{"observed_checks_pending", "sources_incomplete", "source_unresolved", "unavailable"} {
		selected.SkillReferenceState = state
		if selectionView.ValidateFor("ws") != nil {
			t.Fatal("valid reference observation rejected", state)
		}
	}
	for _, change := range []string{"ready", "missing_selection", "accepted", "v1"} {
		v := selectionView
		record := *v.Record
		record.Attempts = append([]PreparationAttemptView(nil), record.Attempts...)
		v.Record = &record
		a := &v.Record.Attempts[0]
		switch change {
		case "ready":
			a.SkillReferenceState = "ready"
		case "missing_selection":
			a.SkillSelectionState = ""
		case "accepted":
			a.State = "accepted"
		case "v1":
			v.Record.Version = "workorch-preparation-record/v1"
		}
		if v.ValidateFor("ws") == nil {
			t.Fatal("unbound reference observation accepted", change)
		}
	}
	selected.SkillReferenceState = ""
	for _, state := range []string{"observed_freshness_pending", "status_blocked", "version_conflict", "sources_incomplete"} {
		selected.SkillStatusState = state
		if selectionView.ValidateFor("ws") != nil {
			t.Fatal("valid original status diagnosis rejected", state)
		}
	}
	for _, change := range []string{"ready", "missing_selection", "accepted", "v1"} {
		v := selectionView
		record := *v.Record
		record.Attempts = append([]PreparationAttemptView(nil), record.Attempts...)
		v.Record = &record
		a := &v.Record.Attempts[0]
		switch change {
		case "ready":
			a.SkillStatusState = "ready"
		case "missing_selection":
			a.SkillSelectionState = ""
		case "accepted":
			a.State = "accepted"
		case "v1":
			v.Record.Version = "workorch-preparation-record/v1"
		}
		if v.ValidateFor("ws") == nil {
			t.Fatal("unbound status diagnosis accepted", change)
		}
	}
	selected.SkillStatusState = ""
	for _, change := range []string{"ready", "missing_registry", "missing_soul", "missing_installed", "mismatched_reason", "accepted"} {
		v := selectionView
		record := *selectionView.Record
		record.Attempts = append([]PreparationAttemptView(nil), record.Attempts...)
		record.Attempts[0].Checks = append([]PreparationCheckView(nil), selected.Checks...)
		v.Record = &record
		a := &v.Record.Attempts[0]
		switch change {
		case "ready":
			a.SkillSelectionState = "ready"
		case "missing_registry":
			a.SkillRegistryEvidenceRef = ""
		case "missing_soul":
			a.SoulEvidenceRef = ""
		case "missing_installed":
			a.SkillHubInstalledState = ""
		case "mismatched_reason":
			a.Checks[0].Reason = "skill_selection_conflict"
		case "accepted":
			a.State = "accepted"
		}
		if v.ValidateFor("ws") == nil {
			t.Fatal("malformed selection accepted", change)
		}
	}
	for _, state := range []string{"incomplete", "observed_selection_pending"} {
		v := fixture("blocked", "recorded")
		v.Record.Attempts[0].SkillHubInstalledState = state
		if v.ValidateFor("ws") != nil {
			t.Fatal("valid installed history rejected")
		}
		for _, mutate := range []func(*StartupPreparationView){
			func(v *StartupPreparationView) { v.Record.Attempts[0].SkillHubInstalledState = "ready" },
			func(v *StartupPreparationView) { v.Record.Attempts[0].SkillHubEffectRef = "" },
			func(v *StartupPreparationView) { v.Record.Attempts[0].State = "accepted" },
			func(v *StartupPreparationView) { v.Record.Version = "workorch-preparation-record/v1" },
		} {
			bad := fixture("blocked", "recorded")
			bad.Record.Attempts[0].SkillHubInstalledState = state
			mutate(&bad)
			if bad.ValidateFor("ws") == nil {
				t.Fatal("invalid installed state accepted")
			}
		}
	}
	for _, mutate := range []func(*StartupPreparationView){
		func(v *StartupPreparationView) { v.Record.Attempts[0].SkillHubEffectRef = "" },
		func(v *StartupPreparationView) { v.Record.Attempts[0].SkillHubEffectRef = "skillhub-status:bad" },
		func(v *StartupPreparationView) { v.Record.Version = "workorch-preparation-record/v1" },
	} {
		v := fixture("blocked", "unresolved")
		mutate(&v)
		if v.ValidateFor("ws") == nil {
			t.Fatal("invalid reference accepted")
		}
	}
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

func TestPreparationManagedConfigReferenceIsOpaqueAndBlockedOnly(t *testing.T) {
	fixture := func() StartupPreparationView {
		v := preparationViewFixture()
		v.Record.Version = "workorch-preparation-record/v2"
		v.OperationState = "blocked"
		a := &v.Record.Attempts[0]
		a.State = "blocked"
		a.CreationEpoch = strings.Repeat("a", 32)
		at := a.StartedAt.Add(time.Second)
		a.CompletedAt = &at
		a.ManagedConfigEvidenceRef = "managed-config-observation:" + strings.Repeat("b", 32)
		a.Checks = []PreparationCheckView{{ID: "managed_configuration", Owner: "file", Required: true, State: "blocked", Reason: "managed_configs_observed_remaining_checks"}}
		return v
	}
	if fixture().ValidateFor("ws") != nil {
		t.Fatal("valid opaque historical reference rejected")
	}
	for _, reason := range []string{"managed_config_observation_unavailable", "managed_config_observation_incomplete", "managed_config_declaration_invalid", "managed_config_content_invalid", "managed_config_content_conflict"} {
		v := fixture()
		v.Record.Attempts[0].Checks[0].Reason = reason
		if v.ValidateFor("ws") != nil {
			t.Fatal("negative code rejected", reason)
		}
	}
	for _, mutate := range []func(*StartupPreparationView){
		func(v *StartupPreparationView) {
			v.Record.Attempts[0].ManagedConfigEvidenceRef = strings.Repeat("b", 64)
		},
		func(v *StartupPreparationView) {
			v.Record.Attempts[0].ManagedConfigEvidenceRef = "managed-config-observation:wrong"
		},
		func(v *StartupPreparationView) { v.Record.Version = "workorch-preparation-record/v1" },
		func(v *StartupPreparationView) { v.Record.Attempts[0].State = "unknown"; v.OperationState = "unknown" },
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks[0].Reason = "ready" },
		func(v *StartupPreparationView) {
			v.Record.Attempts[0].Checks[0].Reason = "resource_observed_remaining_checks"
		},
	} {
		v := fixture()
		mutate(&v)
		if v.ValidateFor("ws") == nil {
			t.Fatal("malformed/cross-purpose reference accepted")
		}
	}
}

func TestPreparationSoulReferenceIsHistoricalAndBlockedOnly(t *testing.T) {
	fixture := func() StartupPreparationView {
		v := preparationViewFixture()
		v.Record.Version = "workorch-preparation-record/v2"
		v.OperationState = "blocked"
		a := &v.Record.Attempts[0]
		a.State = "blocked"
		a.CreationEpoch = strings.Repeat("a", 32)
		at := a.StartedAt.Add(time.Second)
		a.CompletedAt = &at
		a.SoulEvidenceRef = "soul-observation:" + strings.Repeat("b", 32)
		a.Checks = []PreparationCheckView{{ID: "soul_selection", Owner: "soul", Required: true, State: "blocked", Reason: "souls_observed_remaining_checks"}}
		return v
	}
	if fixture().ValidateFor("ws") != nil {
		t.Fatal("valid historical Soul ref rejected")
	}
	v := fixture()
	v.Record.Attempts[0].SoulEvidenceRef = ""
	v.Record.Attempts[0].Checks[0].Reason = "soul_observation_unavailable_not_saved"
	if v.ValidateFor("ws") != nil {
		t.Fatal("explicit not-saved observation rejected")
	}
	for _, mutate := range []func(*StartupPreparationView){
		func(v *StartupPreparationView) { v.Record.Attempts[0].SoulEvidenceRef = strings.Repeat("a", 64) },
		func(v *StartupPreparationView) { v.Record.Attempts[0].SoulEvidenceRef = "soul-observation:invalid" },
		func(v *StartupPreparationView) { v.Record.Attempts[0].SoulEvidenceRef = "" },
		func(v *StartupPreparationView) { v.Record.Version = "workorch-preparation-record/v1" },
		func(v *StartupPreparationView) {
			v.Record.Attempts[0].State = "accepted"
			v.OperationState = "accepted"
		},
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks[0].Reason = "ready" },
		func(v *StartupPreparationView) {
			v.Record.Attempts[0].Checks[0].Reason = "managed_configs_observed_remaining_checks"
		},
	} {
		v := fixture()
		mutate(&v)
		if v.ValidateFor("ws") == nil {
			t.Fatal("malformed Soul history accepted")
		}
	}
}
func TestPreparationSkillRegistryReferenceIsHistoricalAndBlockedOnly(t *testing.T) {
	fixture := func() StartupPreparationView {
		v := preparationViewFixture()
		v.Record.Version = "workorch-preparation-record/v2"
		v.OperationState = "blocked"
		a := &v.Record.Attempts[0]
		a.State = "blocked"
		a.CreationEpoch = strings.Repeat("a", 32)
		at := a.StartedAt.Add(time.Second)
		a.CompletedAt = &at
		a.SkillRegistryEvidenceRef = "skill-registry-observation:" + strings.Repeat("b", 32)
		a.Checks = []PreparationCheckView{{ID: "skill_selection", Owner: "skill", Required: true, State: "blocked", Reason: "skill_registry_observed_selection_pending"}}
		return v
	}
	if fixture().ValidateFor("ws") != nil {
		t.Fatal("valid historical SkillRegistry ref rejected")
	}
	v := fixture()
	v.Record.Attempts[0].SkillRegistryEvidenceRef = ""
	v.Record.Attempts[0].Checks[0].Reason = "skill_registry_observation_unavailable_not_saved"
	if v.ValidateFor("ws") != nil {
		t.Fatal("explicit not-saved observation rejected")
	}
	for _, mutate := range []func(*StartupPreparationView){
		func(v *StartupPreparationView) {
			v.Record.Attempts[0].SkillRegistryEvidenceRef = strings.Repeat("a", 64)
		},
		func(v *StartupPreparationView) {
			v.Record.Attempts[0].SkillRegistryEvidenceRef = "skill-registry-observation:invalid"
		},
		func(v *StartupPreparationView) { v.Record.Attempts[0].SkillRegistryEvidenceRef = "" },
		func(v *StartupPreparationView) { v.Record.Version = "workorch-preparation-record/v1" },
		func(v *StartupPreparationView) {
			v.Record.Attempts[0].State = "accepted"
			v.OperationState = "accepted"
		},
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks[0].Reason = "ready" },
		func(v *StartupPreparationView) {
			v.Record.Attempts[0].Checks[0].Reason = "managed_configs_observed_remaining_checks"
		},
	} {
		v := fixture()
		mutate(&v)
		if v.ValidateFor("ws") == nil {
			t.Fatal("malformed SkillRegistry history accepted")
		}
	}
}

func preparationCompletionViewFixture() StartupPreparationView {
	v := preparationViewFixture()
	v.WorkspaceRevision, v.OperationState, v.CurrentValidity, v.Record.Version = 3, "completed", "observed", "workorch-preparation-record/v2"
	a := &v.Record.Attempts[0]
	a.State, a.CommittedWorkspaceRevision, a.CreationEpoch = "blocked", 3, strings.Repeat("a", 32)
	at := a.StartedAt.Add(time.Second)
	a.CompletedAt = &at
	a.EvaluationVersion, a.NoRunEvaluationVersion = "workorch-preparation-check-evaluation/v1", "workorch-preparation-no-run-final-fence/v1"
	a.RunHistoryEvidenceRef, a.SealEvidenceRef = "run-history-observation:"+strings.Repeat("b", 32), "seal-observation:"+strings.Repeat("c", 32)
	a.Checks = []PreparationCheckView{{ID: "exact_seal", Owner: "workspace", Required: true, State: "passed", Reason: "seal_observed_remaining_checks"}, {ID: "first_activation_no_run", Owner: "run", Required: true, State: "passed", Reason: "run_history_empty_final_fence_observed"}, {ID: "persistence_integrity", Owner: "workspace", Required: true, State: "passed", Reason: "persistence_binding_observed_remaining_checks"}}
	a.Assessment = &PreparationAssessmentView{Version: "workorch-preparation-all-of-assessment/v1", ManifestHash: a.Binding.ManifestHash, Outcome: "checks_passed", RequiredCount: 3, PassedRequiredCount: 3, Outstanding: []PreparationOutstandingCheckView{}}
	a.DefinitionCompilation = &PreparationDefinitionCompilationView{CompilerVersion: "workorch-preparation-definition-compiler/v1", SnapshotHash: a.Binding.SnapshotHash, ManifestHash: a.Binding.ManifestHash, SourceInventoryHash: "inventory", ProfileContractsHash: "profiles", SourceCount: 2, RequirementCount: 2, ObligationCount: 1, VerificationCount: 1}
	v.Record.Completions = []PreparationCompletionView{{Version: "workorch-preparation-completion/v1", CompletionID: "preparation-completion-sha256:" + strings.Repeat("d", 64), RequestID: a.RequestID, BootstrapHash: "prepared-bootstrap-sha256:" + strings.Repeat("e", 64), CreationEpoch: a.CreationEpoch, CompletedAt: at, ExpiresAt: at.Add(30 * time.Second), CommittedWorkspaceRevision: 3}}
	return v
}

func TestPreparationCompletionViewKeepsHistoricalSuccessAndRejectsForgedMetadata(t *testing.T) {
	fixture := preparationCompletionViewFixture
	for _, validity := range []string{"observed", "stale"} {
		v := fixture()
		v.CurrentValidity = validity
		if validity == "stale" {
			v.ActiveRunID = "later-run"
			v.WorkspaceRevision++
		}
		if err := v.ValidateFor("ws"); err != nil {
			t.Fatal("historical completion lost", err)
		}
	}
	for _, scenario := range []string{"hash", "request", "expiry", "no_run", "assessment", "duplicate", "missing", "projection"} {
		t.Run(scenario, func(t *testing.T) {
			v := fixture()
			c := &v.Record.Completions[0]
			switch scenario {
			case "hash":
				c.BootstrapHash = "prepared-bootstrap-sha256:" + strings.Repeat("g", 64)
			case "request":
				c.RequestID = "other"
			case "expiry":
				c.ExpiresAt = c.ExpiresAt.Add(time.Second)
			case "no_run":
				v.Record.Attempts[0].NoRunEvaluationVersion = ""
			case "assessment":
				v.Record.Attempts[0].Assessment.Outcome = "blocked"
			case "duplicate":
				v.Record.Completions = append(v.Record.Completions, *c)
			case "missing":
				v.Record.Completions = nil
			case "projection":
				v.OperationState = "blocked"
			}
			if v.ValidateFor("ws") == nil {
				t.Fatal("forged completion accepted")
			}
		})
	}
}

// The same synthetic wire fixture is consumed by the HTTP and Web tests. Keep
// this assertion on marshaled bytes: decoding into uint64 would hide omission.
func TestPreparationCompletionJSONMatchesWebWireFixture(t *testing.T) {
	v := preparationCompletionViewFixture()
	if err := v.ValidateFor("ws"); err != nil {
		t.Fatal(err)
	}
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/startup_preparation_completed.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bytes.TrimSpace(want)) {
		t.Fatalf("completion wire fixture differs from Go JSON: %s", got)
	}
	if !bytes.Contains(got, []byte(`"preparationEpoch": 0`)) {
		t.Fatal("first preparation identity omitted")
	}
}
