package workspace

import (
	"strings"
	"testing"
)

func TestPreparationMCPSelectionConsumerNeverGrantsReadiness(t *testing.T) {
	d, registry, skills := preparationMCPSelectionFixture(t)
	c := CompilePreparationMCPSelection(d, registry, &skills)
	if c.BlockedReason() != "mcp_selection_compiled_connection_pending" {
		t.Fatal("complete nonempty selection credited or misclassified")
	}
	c.SelectedServerIDs, c.SelectedCapabilityIDs = nil, nil
	if c.BlockedReason() != "mcp_selection_empty_policy_pending" {
		t.Fatal("empty selection became an exemption")
	}
	for _, code := range []string{"source_binding_conflict", "configuration_source_unavailable", "configuration_declaration_invalid", "selection_limit", "skill_source_incomplete", "skill_requirement_invalid", "server_disabled", "server_missing", "capability_disabled", "capability_server_not_selected", "capability_not_allowed", "capability_missing"} {
		c.FailureCode = code
		if c.BlockedReason() != "mcp_selection_"+code {
			t.Fatal("known failure lost exact classification", code)
		}
	}
	c.FailureCode = "unknown"
	if c.BlockedReason() != "" || IsPreparationMCPSelectionReason("mcp_selection_unknown") || IsPreparationMCPSelectionReason("mcp_selection_ready") {
		t.Fatal("unknown prefix or success trusted")
	}
	for _, invented := range []string{"empty_policy_pending", "compiled_connection_pending", "server_missing:raw-config"} {
		c.FailureCode = invented
		if c.BlockedReason() != "" {
			t.Fatal("invented failure code trusted", invented)
		}
	}
	c.FailureCode, c.Version = "", "unknown"
	if c.BlockedReason() != "" {
		t.Fatal("unknown compilation version consumed")
	}
}

func TestPreparationMCPSelectionReasonRequiresOriginalVersionedEvidence(t *testing.T) {
	for _, reason := range []string{"mcp_selection_compiled_connection_pending", "mcp_selection_empty_policy_pending", "mcp_selection_server_missing", "mcp_selection_skill_source_incomplete"} {
		fixture := func() WorkspacePreparationRecord {
			w := evaluationWorkspaceFixture()
			a := &w.StartupPreparation.Attempts[0]
			a.MCPRegistryEvidenceRef = PreparationMCPRegistryRefPrefix + strings.Repeat("b", 32)
			a.Checks = []WorkspacePreparationCheckResult{{ID: "mcp_selection", Owner: "mcpregistry", Required: true, State: "blocked", Reason: reason}}
			return w.StartupPreparation.Clone()
		}
		r := fixture()
		if r.Validate("ws") != nil {
			t.Fatal("valid historical classification rejected", reason)
		}
		for _, change := range []string{"ref", "version", "owner", "required", "passed", "failed", "unknown", "interrupted"} {
			r := fixture()
			a := &r.Attempts[0]
			switch change {
			case "ref":
				a.MCPRegistryEvidenceRef = ""
			case "version":
				a.EvaluationVersion, a.ObservationSnapshotHash = "", ""
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
			case "interrupted":
				a.State = WorkspacePreparationUnknown
			}
			if r.Validate("ws") == nil {
				t.Fatal("unbound classification accepted", reason, change)
			}
		}
	}
}
