package workspace

import (
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	mcpcommon "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/common"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	payload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const PreparationMCPSelectionVersion = "workorch-preparation-mcp-selection/v1"

// Owner-derived declarations only, never a caller selection or readiness flag.
// Full configuration may contain unrelated sensitive values and is not copied.
type PreparationMCPSelectionDefinition struct {
	ConfigurationHash                      string
	DefaultServerIDs, DefaultCapabilityIDs []string
	FailureCode                            string
}

func (d PreparationMCPSelectionDefinition) Clone() PreparationMCPSelectionDefinition {
	d.DefaultServerIDs, d.DefaultCapabilityIDs = slices.Clone(d.DefaultServerIDs), slices.Clone(d.DefaultCapabilityIDs)
	return d
}

// Mirror the existing recursive Workspace-over-Template tool-hint precedence,
// but consume typed payloads and reject malformed declarations instead of
// silently dropping them. Set order is canonical; this preview is not a Run.
func PreparationMCPDefinition(w *Workspace) PreparationMCPSelectionDefinition {
	d := PreparationMCPSelectionDefinition{}
	if w == nil || w.DefinitionAuthoring == nil {
		d.FailureCode = "configuration_source_unavailable"
		return d
	}
	d.ConfigurationHash = WorkspaceConfigurationHash(w)
	// Typed payloads can be constructed without the map/JSON decoder. Reject
	// duplicate keys before merging: first-key lookup and last-key encoding
	// must never compile different declarations from the same source.
	if !preparationMCPUnambiguousHints(w.DefinitionAuthoring.TemplateSnapshot.ContextHints) || !preparationMCPUnambiguousHints(w.ContextHints) {
		d.FailureCode = "configuration_declaration_invalid"
		return d
	}
	var merge func(payload.Payload, payload.Payload) payload.Payload
	merge = func(base, overlay payload.Payload) payload.Payload {
		out := base.Clone()
		for _, field := range overlay.Clone().Fields {
			found := false
			for i := range out.Fields {
				if out.Fields[i].Name != field.Name {
					continue
				}
				if out.Fields[i].Value.Kind == payload.ValueObject && field.Value.Kind == payload.ValueObject {
					field.Value.Object = merge(payload.New(out.Fields[i].Value.Object...), payload.New(field.Value.Object...)).Fields
				}
				out.Fields[i], found = field, true
				break
			}
			if !found {
				out.Fields = append(out.Fields, field)
			}
		}
		return out
	}
	effective := merge(w.DefinitionAuthoring.TemplateSnapshot.ContextHints, w.ContextHints)
	for _, field := range effective.Fields {
		if field.Value.Kind != payload.ValueObject {
			continue
		}
		for _, key := range []string{"defaultMCPServerIds", "defaultCapabilityIds"} {
			object := payload.New(field.Value.Object...)
			if _, exists := object.Field(key); !exists {
				continue
			}
			ids, ok := object.StringSlice(key)
			if !ok || len(ids) > mcpcommon.MaxSelectionCapabilities {
				d.FailureCode = "configuration_declaration_invalid"
				return d
			}
			for _, id := range ids {
				if !preparationMCPID(id) {
					d.FailureCode = "configuration_declaration_invalid"
					return d
				}
			}
			if key == "defaultMCPServerIds" {
				d.DefaultServerIDs = append(d.DefaultServerIDs, ids...)
			} else {
				d.DefaultCapabilityIDs = append(d.DefaultCapabilityIDs, ids...)
			}
		}
	}
	d.DefaultServerIDs, d.DefaultCapabilityIDs = preparationMCPSet(d.DefaultServerIDs), preparationMCPSet(d.DefaultCapabilityIDs)
	if len(d.DefaultServerIDs) > mcpcommon.MaxSelectionServers || len(d.DefaultCapabilityIDs) > mcpcommon.MaxSelectionCapabilities {
		d.FailureCode = "selection_limit"
	}
	return d
}

func preparationMCPUnambiguousHints(hints payload.Payload) bool {
	seen := make(map[string]bool, len(hints.Fields))
	for _, field := range hints.Fields {
		if seen[field.Name] {
			return false
		}
		seen[field.Name] = true
		if field.Value.Kind != payload.ValueObject {
			continue
		}
		keys := make(map[string]bool, len(field.Value.Object))
		for _, nested := range field.Value.Object {
			if keys[nested.Name] {
				return false
			}
			keys[nested.Name] = true
		}
	}
	return true
}

// Full selected owner values stay in the private archive. This compilation
// does not resolve environment references, connect, authorize, or grant PASS.
type PreparationMCPSelectionCompilation struct {
	Version                                    string
	Definition                                 PreparationMCPSelectionDefinition
	SkillRegistryRef, SkillRegistryContentHash string
	RequiredServerIDs, RequiredCapabilityIDs   []string
	SelectedServerIDs, SelectedCapabilityIDs   []string
	Selected                                   mcpevents.SelectionSnapshotResult
	FailureCode                                string
}

func (c PreparationMCPSelectionCompilation) Clone() PreparationMCPSelectionCompilation {
	c.Definition = c.Definition.Clone()
	c.RequiredServerIDs, c.RequiredCapabilityIDs = slices.Clone(c.RequiredServerIDs), slices.Clone(c.RequiredCapabilityIDs)
	c.SelectedServerIDs, c.SelectedCapabilityIDs = slices.Clone(c.SelectedServerIDs), slices.Clone(c.SelectedCapabilityIDs)
	r := (mcpevents.RegistrySnapshotResult{Servers: c.Selected.Servers, Capabilities: c.Selected.Capabilities, RegistryRevision: c.Selected.RegistryRevision}).Clone()
	c.Selected = mcpevents.SelectionSnapshotResult{Servers: r.Servers, Capabilities: r.Capabilities, RegistryRevision: r.RegistryRevision}
	return c
}

func CompilePreparationMCPSelection(d PreparationMCPSelectionDefinition, registry WorkspacePreparationMCPRegistryEvidence, skills *WorkspacePreparationSkillRegistryEvidence) PreparationMCPSelectionCompilation {
	c := PreparationMCPSelectionCompilation{Version: PreparationMCPSelectionVersion, Definition: d.Clone()}
	fail := func(code string) PreparationMCPSelectionCompilation { c.FailureCode = code; return c.Clone() }
	if d.ConfigurationHash != registry.Binding.AuthoringConfigurationHash || registry.Facts.Validate() != nil {
		return fail("source_binding_conflict")
	}
	if d.FailureCode != "" {
		return fail(d.FailureCode)
	}
	if skills == nil {
		return fail("skill_source_incomplete")
	}
	c.SkillRegistryRef, c.SkillRegistryContentHash = skills.Ref, skills.ContentHash
	if skills.Validate() != nil || skills.RequestID != registry.RequestID || skills.PayloadHash != registry.PayloadHash || skills.Facts.WorkspaceID != registry.Facts.WorkspaceID || skills.Facts.WorkspaceRevision != registry.Facts.WorkspaceRevision || skills.Facts.SnapshotHash != registry.Facts.SnapshotHash {
		return fail("source_binding_conflict")
	}
	s := skills.Selection
	if s == nil || s.FailureCode != "" {
		return fail("skill_source_incomplete")
	}
	if s.Version != PreparationSkillSelectionVersion || s.Definition.WorkspaceID != registry.Facts.WorkspaceID || s.Definition.StartupInputHash != registry.Binding.StartupInputHash || s.Definition.DefinitionRevision != registry.Binding.DefinitionRevision || s.Definition.SnapshotHash != registry.Facts.SnapshotHash {
		return fail("source_binding_conflict")
	}
	for _, skill := range s.Selected {
		for _, id := range skill.Content.RequiredMCPServerRefs {
			if !preparationMCPID(id) {
				return fail("skill_requirement_invalid")
			}
			c.RequiredServerIDs = append(c.RequiredServerIDs, id)
		}
		for _, id := range skill.Content.RequiredCapabilityRefs {
			if !preparationMCPID(id) {
				return fail("skill_requirement_invalid")
			}
			c.RequiredCapabilityIDs = append(c.RequiredCapabilityIDs, id)
		}
	}
	c.RequiredServerIDs, c.RequiredCapabilityIDs = preparationMCPSet(c.RequiredServerIDs), preparationMCPSet(c.RequiredCapabilityIDs)
	c.SelectedServerIDs = preparationMCPSet(append(slices.Clone(d.DefaultServerIDs), c.RequiredServerIDs...))
	c.SelectedCapabilityIDs = preparationMCPSet(append(slices.Clone(d.DefaultCapabilityIDs), c.RequiredCapabilityIDs...))
	if len(c.SelectedServerIDs) > mcpcommon.MaxSelectionServers || len(c.SelectedCapabilityIDs) > mcpcommon.MaxSelectionCapabilities {
		return fail("selection_limit")
	}
	c.Selected.RegistryRevision = registry.Facts.Before.RegistryRevision
	for _, id := range c.SelectedServerIDs {
		found := false
		for _, server := range registry.Facts.Before.Servers {
			if string(server.ID) != id {
				continue
			}
			if !server.Enabled {
				return fail("server_disabled")
			}
			c.Selected.Servers, found = append(c.Selected.Servers, server), true
		}
		if !found {
			return fail("server_missing")
		}
	}
	for _, id := range c.SelectedCapabilityIDs {
		found := false
		for _, capability := range registry.Facts.Before.Capabilities {
			if string(capability.ID) != id {
				continue
			}
			if !capability.Enabled {
				return fail("capability_disabled")
			}
			if !slices.Contains(c.SelectedServerIDs, string(capability.ServerID)) {
				return fail("capability_server_not_selected")
			}
			allowed := false
			for _, server := range c.Selected.Servers {
				if server.ID != capability.ServerID {
					continue
				}
				var names []string
				switch capability.Kind {
				case "tool":
					names = server.AllowedTools
				case "resource":
					names = server.AllowedResources
				case "prompt":
					names = server.AllowedPrompts
				}
				allowed = slices.Contains(names, capability.Name)
			}
			if !allowed {
				return fail("capability_not_allowed")
			}
			c.Selected.Capabilities, found = append(c.Selected.Capabilities, capability), true
		}
		if !found {
			return fail("capability_missing")
		}
	}
	return c.Clone()
}

func preparationMCPID(id string) bool {
	return id != "" && utf8.ValidString(id) && len(id) <= mcpcommon.MaxIDBytes && strings.TrimSpace(id) == id && !strings.ContainsAny(id, "\x00\r\n")
}
func preparationMCPSet(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

func validatePreparationMCPSelectionPersistence(w *Workspace) error {
	for _, registry := range w.PreparationMCPRegistryEvidence {
		if registry.Selection == nil {
			continue
		} // A stale settlement can retain observations without a compilation.
		var skills *WorkspacePreparationSkillRegistryEvidence
		for i := range w.PreparationSkillRegistryEvidence {
			e := &w.PreparationSkillRegistryEvidence[i]
			if e.RequestID == registry.RequestID && e.PayloadHash == registry.PayloadHash {
				skills = e
			}
		}
		expected := CompilePreparationMCPSelection(registry.Selection.Definition, registry, skills)
		if !reflect.DeepEqual(*registry.Selection, expected) {
			return common.ErrConflict
		}
	}
	return nil
}
