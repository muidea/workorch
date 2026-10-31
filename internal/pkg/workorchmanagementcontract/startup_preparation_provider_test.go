package workorchmanagementcontract

import (
	"strings"
	"testing"
	"time"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestPreparationProviderSelectionWireRequiresOriginalBoundBlockedEvidence(t *testing.T) {
	for _, reason := range []string{"provider_selection_budget_observed_authorization_pending", "provider_selection_budget_unavailable", "provider_selection_budget_source_conflict", "provider_selection_authorization_pending", "provider_selection_source_binding_conflict", "provider_selection_configuration_source_unavailable", "provider_selection_invalid", "provider_selection_model_missing", "provider_selection_capability_incomplete", "provider_selection_operation_unsupported", "provider_selection_reasoning_unsupported"} {
		fixture := func() StartupPreparationView {
			v := preparationViewFixture()
			v.Record.Version, v.OperationState = "workorch-preparation-record", "blocked"
			a := &v.Record.Attempts[0]
			a.State, a.CreationEpoch, a.EvaluationVersion = "blocked", strings.Repeat("a", 32), "workorch-preparation-check-evaluation/v1"
			a.ProviderCacheEvidenceRef = "provider-cache-observation:" + strings.Repeat("b", 32)
			at := a.StartedAt.Add(time.Second)
			a.CompletedAt = &at
			a.Checks = []PreparationCheckView{{ID: "provider_cache", Owner: "llm", Required: true, State: "blocked", Reason: reason}}
			return v
		}
		if !workspace.IsPreparationProviderSelectionReason(reason) || !isPreparationProviderSelectionReason(reason) || fixture().ValidateFor("ws") != nil {
			t.Fatal("finite owner/wire classification differs", reason)
		}
		for _, change := range []string{"ref", "version", "owner", "required", "passed", "failed", "unknown", "success", "raw", "scope", "legacy"} {
			v := fixture()
			a := &v.Record.Attempts[0]
			switch change {
			case "ref":
				a.ProviderCacheEvidenceRef = ""
			case "version":
				a.EvaluationVersion = ""
			case "owner":
				a.Checks[0].Owner = "caller"
			case "required":
				a.Checks[0].Required = false
			case "passed":
				a.Checks[0].State = "passed"
			case "failed":
				a.Checks[0].State = "failed"
			case "unknown":
				a.Checks[0].Reason = "provider_selection_unknown"
			case "success":
				a.Checks[0].Reason = "provider_selection_ready"
			case "raw":
				a.Checks[0].Reason = "provider_selection_model_missing:raw-config"
			case "scope":
				a.Checks[0].ID = "exact_seal"
			case "legacy":
				v.Record.Version = "unsupported-record-contract"
			}
			if v.ValidateFor("ws") == nil {
				t.Fatal("unbound classification accepted", reason, change)
			}
		}
	}
}

func TestPreparationProviderConfigurationWireRequiresOriginalEvidence(t *testing.T) {
	fixture := func() StartupPreparationView {
		v := preparationViewFixture()
		v.Record.Version, v.OperationState = "workorch-preparation-record", "blocked"
		a := &v.Record.Attempts[0]
		a.State, a.CreationEpoch, a.EvaluationVersion = "blocked", strings.Repeat("a", 32), "workorch-preparation-check-evaluation/v1"
		a.ProviderCacheEvidenceRef = "provider-cache-observation:" + strings.Repeat("b", 32)
		at := a.StartedAt.Add(time.Second)
		a.CompletedAt = &at
		a.Checks = []PreparationCheckView{{ID: "provider_cache", Owner: "llm", Required: true, State: "passed", Reason: workspace.PreparationProviderConfigurationReason}}
		return v
	}
	if fixture().ValidateFor("ws") != nil {
		t.Fatal("bound configuration observation rejected")
	}
	for _, change := range []string{"ref", "version", "owner", "required", "blocked", "legacy"} {
		v := fixture()
		a := &v.Record.Attempts[0]
		switch change {
		case "ref":
			a.ProviderCacheEvidenceRef = ""
		case "version":
			a.EvaluationVersion = ""
		case "owner":
			a.Checks[0].Owner = "caller"
		case "required":
			a.Checks[0].Required = false
		case "blocked":
			a.Checks[0].State = "blocked"
		case "legacy":
			v.Record.Version = "unsupported-record-contract"
		}
		if v.ValidateFor("ws") == nil {
			t.Fatal("unbound configuration observation accepted", change)
		}
	}
}
