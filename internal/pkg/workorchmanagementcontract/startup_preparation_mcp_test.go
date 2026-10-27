package workorchmanagementcontract

import (
	"strings"
	"testing"
	"time"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func TestPreparationMCPRegistryReferenceNeverGrantsConnectionReadiness(t *testing.T) {
	fixture := func() StartupPreparationView {
		v := preparationViewFixture()
		v.Record.Version, v.OperationState = "workorch-preparation-record/v2", "blocked"
		a := &v.Record.Attempts[0]
		a.State, a.CreationEpoch = "blocked", strings.Repeat("a", 32)
		a.MCPRegistryEvidenceRef = "mcp-registry-observation:" + strings.Repeat("b", 32)
		at := a.StartedAt.Add(time.Second)
		a.CompletedAt = &at
		a.Checks = []PreparationCheckView{{ID: "mcp_selection", Owner: "mcpregistry", Required: true, State: "blocked", Reason: "mcp_registry_observed_connection_pending"}}
		return v
	}
	for _, reason := range []string{"mcp_registry_observed_connection_pending", "binding_changed"} {
		v := fixture()
		v.Record.Attempts[0].Checks[0].Reason = reason
		if v.ValidateFor("ws") != nil {
			t.Fatal("historical MCP reference rejected", reason)
		}
	}
	for _, change := range []func(*StartupPreparationView){
		func(v *StartupPreparationView) { v.Record.Version = "workorch-preparation-record/v1" },
		func(v *StartupPreparationView) {
			v.Record.Attempts[0].MCPRegistryEvidenceRef = "mcp-registry-observation:ready"
		},
		func(v *StartupPreparationView) { v.Record.Attempts[0].MCPRegistryEvidenceRef = "" },
		func(v *StartupPreparationView) { v.Record.Attempts[0].State = "accepted" },
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks = nil },
		func(v *StartupPreparationView) {
			v.Record.Attempts[0].Checks[0].Reason = "mcp_registry_observation_unavailable_not_saved"
		},
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks[0].Owner = "caller" },
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks[0].Required = false },
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks[0].State = "passed" },
		func(v *StartupPreparationView) { v.Record.Attempts[0].Checks[0].State = "failed" },
	} {
		v := fixture()
		change(&v)
		if v.ValidateFor("ws") == nil {
			t.Fatal("unbound MCP proof accepted")
		}
	}
}

func TestPreparationMCPSelectionProjectionRequiresVersionAndExactOwnerShape(t *testing.T) {
	reasons := []string{"mcp_selection_empty_policy_pending", "mcp_selection_compiled_connection_pending", "mcp_selection_source_binding_conflict", "mcp_selection_configuration_source_unavailable", "mcp_selection_configuration_declaration_invalid", "mcp_selection_selection_limit", "mcp_selection_skill_source_incomplete", "mcp_selection_skill_requirement_invalid", "mcp_selection_server_disabled", "mcp_selection_server_missing", "mcp_selection_capability_disabled", "mcp_selection_capability_server_not_selected", "mcp_selection_capability_not_allowed", "mcp_selection_capability_missing"}
	for _, reason := range reasons {
		fixture := func() StartupPreparationView {
			v := preparationViewFixture()
			v.Record.Version, v.OperationState = "workorch-preparation-record/v2", "blocked"
			a := &v.Record.Attempts[0]
			a.State, a.CreationEpoch, a.EvaluationVersion = "blocked", strings.Repeat("a", 32), "workorch-preparation-check-evaluation/v1"
			a.MCPRegistryEvidenceRef = "mcp-registry-observation:" + strings.Repeat("b", 32)
			at := a.StartedAt.Add(time.Second)
			a.CompletedAt = &at
			a.Checks = []PreparationCheckView{{ID: "mcp_selection", Owner: "mcpregistry", Required: true, State: "blocked", Reason: reason}}
			return v
		}
		if !workspace.IsPreparationMCPSelectionReason(reason) || !isPreparationMCPSelectionReason(reason) || fixture().ValidateFor("ws") != nil {
			t.Fatal("finite classification not shared by owner/public contract", reason)
		}
		for _, change := range []string{"ref", "version", "owner", "required", "passed", "failed", "unknown", "success", "arbitrary", "scope", "legacy"} {
			v := fixture()
			a := &v.Record.Attempts[0]
			switch change {
			case "ref":
				a.MCPRegistryEvidenceRef = ""
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
				a.Checks[0].Reason = "mcp_selection_unknown"
			case "success":
				a.Checks[0].Reason = "mcp_selection_ready"
			case "arbitrary":
				a.Checks[0].Reason = "mcp_selection_server_missing:raw-config"
			case "scope":
				a.Checks[0].ID = "exact_seal"
			case "legacy":
				v.Record.Version = "workorch-preparation-record/v1"
			}
			if v.ValidateFor("ws") == nil {
				t.Fatal("unbound or unversioned classification accepted", reason, change)
			}
		}
	}
}
