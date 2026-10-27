package events

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/common"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const PreparationSnapshotMaxBytes = 1024 * 1024

func (r RegistrySnapshotResult) Clone() RegistrySnapshotResult {
	r.Servers = slices.Clone(r.Servers)
	for i, server := range r.Servers {
		if server == nil {
			continue
		}
		v := *server
		v.Args = slices.Clone(server.Args)
		v.EnvironmentRefs = slices.Clone(server.EnvironmentRefs)
		v.AllowedTools = slices.Clone(server.AllowedTools)
		v.AllowedResources = slices.Clone(server.AllowedResources)
		v.AllowedPrompts = slices.Clone(server.AllowedPrompts)
		r.Servers[i] = &v
	}
	r.Capabilities = slices.Clone(r.Capabilities)
	for i, c := range r.Capabilities {
		if c == nil {
			continue
		}
		v := *c
		v.Tags, v.StageHints, v.Schema = slices.Clone(c.Tags), slices.Clone(c.StageHints), c.Schema.Clone()
		r.Capabilities[i] = &v
	}
	return r
}

// Complete bounded registry history only. Content revision is checked against
// the complete raw value; it is not a connection epoch or successful probe.
// Never truncate or filter an oversized/invalid result into an empty selection.
func (r RegistrySnapshotResult) ValidatePreparation() error {
	if len(r.Servers) > modulecommon.MaxRegistryServers || len(r.Capabilities) > modulecommon.MaxRegistryCapabilities {
		return common.ErrConflict
	}
	raw, err := json.Marshal(r)
	if err != nil || len(raw) > PreparationSnapshotMaxBytes {
		return common.ErrConflict
	}
	snapshot := &mcpmodel.RegistrySnapshot{}
	servers := make(map[common.MCPServerID]bool, len(r.Servers))
	last := ""
	for _, server := range r.Servers {
		if server == nil || string(server.ID) <= last || !preparationRegistryID(string(server.ID)) || server.Name == "" {
			return common.ErrConflict
		}
		last = string(server.ID)
		if !preparationText(server.Name, server.Transport, server.Endpoint, server.Command, server.DefaultApprovalPolicy, server.SessionPolicy) || !preparationText(server.Args...) || !preparationText(server.AllowedTools...) || !preparationText(server.AllowedResources...) || !preparationText(server.AllowedPrompts...) {
			return common.ErrConflict
		}
		value, err := server.Model()
		if err != nil {
			return common.ErrConflict
		}
		// Model conversion must not trim, reorder or collapse the original
		// environment references used by this complete observation.
		refs := EnvironmentReferencesFromMap(value.Env)
		if !slices.Equal(refs, server.EnvironmentRefs) {
			return common.ErrConflict
		}
		snapshot.Servers = append(snapshot.Servers, value)
		servers[server.ID] = true
	}
	last = ""
	bindings := make(map[[3]string]bool, len(r.Capabilities))
	for _, c := range r.Capabilities {
		if c == nil || string(c.ID) <= last || !preparationRegistryID(string(c.ID)) || !servers[c.ServerID] || c.Name == "" || c.Revision == 0 || c.SourceRef == "" || len(c.Schema.CanonicalString()) > modulecommon.MaxSchemaBytes {
			return common.ErrConflict
		}
		if c.Kind != mcpmodel.CapabilityKindTool && c.Kind != mcpmodel.CapabilityKindResource && c.Kind != mcpmodel.CapabilityKindPrompt {
			return common.ErrConflict
		}
		if !preparationText(string(c.ServerID), c.Kind, c.Name, c.DisplayName, c.Description, c.Category, c.UsageHint, c.InputSummary, c.OutputSummary, c.RiskLevel, c.EffectScope, c.IdempotencyClass, c.ValidationProofApplicability, c.URI, c.ApprovalPolicy, c.Source, c.SourceRef) || !preparationText(c.Tags...) || !preparationText(c.StageHints...) {
			return common.ErrConflict
		}
		key := [3]string{string(c.ServerID), c.Kind, c.Name}
		if bindings[key] {
			return common.ErrConflict
		}
		bindings[key], last = true, string(c.ID)
		snapshot.Capabilities = append(snapshot.Capabilities, mcpmodel.CloneCapability(c))
	}
	// Same owner content revision format, not a new authority or normalization.
	raw, err = json.Marshal(snapshot)
	if err != nil {
		return common.ErrConflict
	}
	sum := sha256.Sum256(raw)
	if r.RegistryRevision != "mcp-registry-"+hex.EncodeToString(sum[:])[:24] {
		return common.ErrConflict
	}
	return nil
}

func preparationRegistryID(id string) bool {
	return id != "" && utf8.ValidString(id) && len(id) <= modulecommon.MaxIDBytes && strings.TrimSpace(id) == id && !strings.ContainsAny(id, "\x00\r\n")
}

func preparationText(values ...string) bool {
	for _, value := range values {
		if !utf8.ValidString(value) {
			return false
		}
	}
	return true
}
