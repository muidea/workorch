package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	payload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func preparationMCPSelectionFixture(t *testing.T) (PreparationMCPSelectionDefinition, WorkspacePreparationMCPRegistryEvidence, WorkspacePreparationSkillRegistryEvidence) {
	t.Helper()
	d, skills, souls, effects := preparationSelectionFixture(t)
	skill := skills.Facts.Catalog.Skills[0]
	skill.RequiredMCPServerRefs, skill.RequiredCapabilityRefs = []string{"server"}, []string{"capability"}
	skill.Provenance.ContentHash = workorchskillcontract.StableContentHash(skill.Content())
	skills.Facts.Catalog.CatalogHash, _ = skillmodel.RegistryCatalogHash(skills.Facts.Catalog.Skills)
	skills.Facts.ReobservedCatalog = skills.Facts.Catalog.Clone()
	selection := CompilePreparationSkillSelection(d, skills, &souls, &effects)
	if selection.FailureCode != "" {
		t.Fatal(selection.FailureCode)
	}
	skills.Selection, skills.ContentHash = &selection, ""
	skills.ContentHash = skills.Hash()
	server := &mcpevents.MCPServer{ID: "server", Name: "original", Transport: "stdio", Command: "never-start", Args: []string{"original\r\n"}, Enabled: true, AllowedTools: []string{"tool"}}
	capability := &mcpmodel.Capability{ID: "capability", ServerID: server.ID, Kind: "tool", Name: "tool", DisplayName: "original", Description: "原文\r\n", Enabled: true, Revision: 1, Source: "user", SourceRef: "user:capability"}
	r := mcpevents.RegistrySnapshotResult{Servers: []*mcpevents.MCPServer{server}, Capabilities: []*mcpmodel.Capability{capability}}
	sealPreparationMCPTestSnapshot(t, &r)
	now := time.Now().UTC()
	f := PreparationMCPObservationFacts{Version: PreparationMCPRegistryEvidenceVersion, WorkspaceID: d.WorkspaceID, WorkspaceRevision: skills.Facts.WorkspaceRevision, SnapshotHash: d.SnapshotHash, ReobservedSnapshotHash: d.SnapshotHash, Before: r, After: r.Clone(), StartedAt: now, CompletedAt: now}
	e := WorkspacePreparationMCPRegistryEvidence{Version: PreparationMCPRegistryEvidenceVersion, Ref: PreparationMCPRegistryRefPrefix + strings.Repeat("c", 32), RequestID: skills.RequestID, PayloadHash: skills.PayloadHash, Binding: effects.Binding, Facts: f}
	e.ContentHash = e.Hash()
	return PreparationMCPSelectionDefinition{ConfigurationHash: effects.Binding.AuthoringConfigurationHash}, e, skills
}

func sealPreparationMCPTestSnapshot(t *testing.T, r *mcpevents.RegistrySnapshotResult) {
	t.Helper()
	s := &mcpmodel.RegistrySnapshot{Capabilities: r.Capabilities}
	for _, server := range r.Servers {
		v, err := server.Model()
		if err != nil {
			t.Fatal(err)
		}
		s.Servers = append(s.Servers, v)
	}
	raw, _ := json.Marshal(s)
	sum := sha256.Sum256(raw)
	r.RegistryRevision = "mcp-registry-" + hex.EncodeToString(sum[:])[:24]
	if r.ValidatePreparation() != nil {
		t.Fatal("invalid MCP test registry")
	}
}

func TestPreparationMCPSelectionCompilesFullSourcesAndClones(t *testing.T) {
	d, registry, skills := preparationMCPSelectionFixture(t)
	value := CompilePreparationMCPSelection(d, registry, &skills)
	if value.FailureCode != "" || !reflect.DeepEqual(value.SelectedServerIDs, []string{"server"}) || !reflect.DeepEqual(value.SelectedCapabilityIDs, []string{"capability"}) || value.SkillRegistryRef != skills.Ref || value.SkillRegistryContentHash != skills.ContentHash || value.Selected.RegistryRevision != registry.Facts.Before.RegistryRevision || value.Selected.Capabilities[0].Description != "原文\r\n" {
		t.Fatalf("incomplete selection: %+v", value)
	}
	clone := value.Clone()
	clone.RequiredServerIDs[0] = "caller"
	clone.Selected.Servers[0].Args[0] = "caller"
	clone.Selected.Capabilities[0].Description = "caller"
	if !reflect.DeepEqual(value, CompilePreparationMCPSelection(d, registry, &skills)) {
		t.Fatal("clone aliased original")
	}
	d.DefaultServerIDs, d.DefaultCapabilityIDs = []string{"server"}, []string{"capability"}
	if v := CompilePreparationMCPSelection(d, registry, &skills); v.FailureCode != "" || len(v.Selected.Servers) != 1 {
		t.Fatal("default and Skill refs duplicated", v)
	}
	// Explicitly complete empty Skill selection is not a missing Skill source.
	skills.Selection.Selected = nil
	skills.ContentHash = skills.Hash()
	d.DefaultServerIDs, d.DefaultCapabilityIDs = nil, nil
	v := CompilePreparationMCPSelection(d, registry, &skills)
	if v.FailureCode != "" || len(v.Selected.Servers) != 0 || len(v.Selected.Capabilities) != 0 {
		t.Fatal("explicit empty selection changed")
	}
	if CompilePreparationMCPSelection(d, registry, nil).FailureCode != "skill_source_incomplete" {
		t.Fatal("missing source became empty success")
	}
}

func TestPreparationMCPSelectionRejectsMissingDisabledDriftAndLimits(t *testing.T) {
	want := map[string]string{"missing_server": "server_missing", "missing_capability": "capability_missing", "disabled_server": "server_disabled", "disabled_capability": "capability_disabled", "unselected_server": "capability_server_not_selected", "allowlist": "capability_not_allowed", "skill_missing": "skill_source_incomplete", "skill_failed": "skill_source_incomplete", "skill_binding": "source_binding_conflict", "skill_snapshot": "source_binding_conflict", "skill_version": "source_binding_conflict", "skill_scope": "source_binding_conflict", "config_binding": "source_binding_conflict", "declaration": "configuration_declaration_invalid", "limit": "selection_limit", "invalid_requirement": "skill_requirement_invalid", "source_subset": "source_binding_conflict"}
	for scenario, code := range want {
		t.Run(scenario, func(t *testing.T) {
			d, r, s := preparationMCPSelectionFixture(t)
			switch scenario {
			case "missing_server":
				r.Facts.Before.Servers = nil
				r.Facts.Before.Capabilities = nil
			case "missing_capability":
				r.Facts.Before.Capabilities = nil
			case "disabled_server":
				r.Facts.Before.Servers[0].Enabled = false
			case "disabled_capability":
				r.Facts.Before.Capabilities[0].Enabled = false
			case "unselected_server":
				s.Selection.Selected[0].Content.RequiredMCPServerRefs = nil
				s.Selection.Selected[1].Content.RequiredMCPServerRefs = nil
				s.Selection.Selected[2].Content.RequiredMCPServerRefs = nil
			case "allowlist":
				r.Facts.Before.Servers[0].AllowedTools = nil
			case "skill_missing":
				s.Selection = nil
			case "skill_failed":
				s.Selection.FailureCode = "sources_incomplete"
			case "skill_binding":
				s.Selection.Definition.StartupInputHash = "other"
			case "skill_snapshot":
				s.Facts.SnapshotHash = "other"
			case "skill_version":
				s.Selection.Version = "unknown"
			case "skill_scope":
				s.Selection.Definition.WorkspaceID = "other"
			case "config_binding":
				d.ConfigurationHash = "other"
			case "declaration":
				d.FailureCode = "configuration_declaration_invalid"
			case "limit":
				for i := 0; i < 65; i++ {
					d.DefaultServerIDs = append(d.DefaultServerIDs, fmt.Sprintf("s-%03d", i))
				}
			case "invalid_requirement":
				s.Selection.Selected[0].Content.RequiredMCPServerRefs = []string{" padded "}
			case "source_subset":
				r.Facts.Before.Capabilities = nil
			}
			if scenario != "source_subset" {
				sealPreparationMCPTestSnapshot(t, &r.Facts.Before)
			}
			r.Facts.After = r.Facts.Before.Clone()
			s.ContentHash = s.Hash()
			if v := CompilePreparationMCPSelection(d, r, &s); v.FailureCode != code {
				t.Fatal("invalid source misclassified", scenario, v.FailureCode, code)
			}
		})
	}
}

func TestPreparationMCPDefinitionUsesExactTypedWorkspacePrecedence(t *testing.T) {
	template := payload.New(payload.ObjectField("shell", payload.New(payload.StringArrayField("defaultMCPServerIds", []string{"z", "a"}), payload.StringArrayField("defaultCapabilityIds", []string{"cap"}))))
	w := &Workspace{ID: common.WorkspaceID("ws"), DefinitionAuthoring: &DefinitionAuthoringRecord{TemplateSnapshot: WorkspaceTemplate{ContextHints: template}}, ContextHints: payload.New(payload.ObjectField("shell", payload.New(payload.StringArrayField("defaultMCPServerIds", []string{"override"}))), payload.ObjectField("other", payload.New(payload.StringArrayField("defaultMCPServerIds", []string{"a", "a"}))))}
	d := PreparationMCPDefinition(w)
	if d.FailureCode != "" || d.ConfigurationHash != WorkspaceConfigurationHash(w) || !reflect.DeepEqual(d.DefaultServerIDs, []string{"a", "override"}) || !reflect.DeepEqual(d.DefaultCapabilityIDs, []string{"cap"}) {
		t.Fatalf("precedence changed: %+v", d)
	}
	w.ContextHints = payload.New(payload.ObjectField("shell", payload.New(payload.StringField("defaultMCPServerIds", "not-an-array"))))
	if PreparationMCPDefinition(w).FailureCode != "configuration_declaration_invalid" {
		t.Fatal("malformed declaration became empty")
	}
	w.ContextHints = payload.New(payload.ObjectField("shell", payload.New(payload.StringArrayField("defaultMCPServerIds", []string{" padded "}))))
	if PreparationMCPDefinition(w).FailureCode != "configuration_declaration_invalid" {
		t.Fatal("padded ID silently repaired")
	}
	for _, ambiguous := range []payload.Payload{
		payload.New(payload.ObjectField("shell", payload.New()), payload.ObjectField("shell", payload.New(payload.StringArrayField("defaultMCPServerIds", []string{"server"})))),
		payload.New(payload.ObjectField("shell", payload.New(payload.StringArrayField("defaultMCPServerIds", nil), payload.StringArrayField("defaultMCPServerIds", []string{"server"})))),
	} {
		w.DefinitionAuthoring.TemplateSnapshot.ContextHints = template
		w.ContextHints = ambiguous
		if PreparationMCPDefinition(w).FailureCode != "configuration_declaration_invalid" {
			t.Fatal("ambiguous typed declaration silently chosen")
		}
		w.ContextHints = payload.New()
		w.DefinitionAuthoring.TemplateSnapshot.ContextHints = ambiguous
		if PreparationMCPDefinition(w).FailureCode != "configuration_declaration_invalid" {
			t.Fatal("ambiguous template silently overridden")
		}
	}
}
