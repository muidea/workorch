package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	registryevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// OperationAdmissibility returns enabled MCP server IDs, owner-attested
// validation recipe identities, and a stable hash of their effective registry
// state. It performs only the typed registry query; no MCP transport is opened
// and no capability operation is invoked.
func (a *RuntimeAdapter) OperationAdmissibility(ctx context.Context) ([]string, []string, string, error) {
	if a == nil || a.service == nil {
		return nil, nil, "", common.ErrInvalidArgument
	}
	lister, ok := a.service.(serverLister)
	if !ok {
		return nil, nil, "", fmt.Errorf("%w: mcp server catalog is unavailable", common.ErrInvalidArgument)
	}
	servers, err := lister.ListServers(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	orderedServers := append([]*mcpmodel.Server(nil), servers...)
	sort.Slice(orderedServers, func(i, j int) bool {
		if orderedServers[i] == nil {
			return false
		}
		if orderedServers[j] == nil {
			return true
		}
		return string(orderedServers[i].ID) < string(orderedServers[j].ID)
	})
	ids := make([]string, 0, len(orderedServers))
	recipeRefs := make([]string, 0)
	persisted := make([]mcpmodel.Server, 0, len(orderedServers))
	serverAllowedTools := make(map[string]map[string]struct{}, len(orderedServers))
	for _, server := range orderedServers {
		if server == nil || !server.Enabled || strings.TrimSpace(string(server.ID)) == "" {
			continue
		}
		ids = append(ids, string(server.ID))
		allowed := make(map[string]struct{}, len(server.AllowedTools))
		for _, tool := range server.AllowedTools {
			if tool = strings.TrimSpace(tool); tool != "" {
				allowed[tool] = struct{}{}
			}
		}
		serverAllowedTools[string(server.ID)] = allowed
		persisted = append(persisted, *server)
	}
	if len(ids) == 0 {
		return nil, nil, "", fmt.Errorf("%w: no enabled mcp server is available", common.ErrNotFound)
	}
	var persistedCapabilities []*mcpmodel.Capability
	if lister, ok := a.service.(capabilityLister); ok {
		allCapabilities, listErr := lister.ListCapabilities(ctx, registryevents.CapabilityQuery{Enabled: boolPtr(true), Kind: mcpmodel.CapabilityKindTool})
		if listErr != nil {
			return nil, nil, "", listErr
		}
		for _, capability := range allCapabilities {
			if capability == nil || !capability.Enabled || strings.TrimSpace(capability.Name) == "" ||
				strings.TrimSpace(capability.EffectScope) == string(runtimecontract.RuntimeEffectUnknown) ||
				strings.TrimSpace(capability.IdempotencyClass) == string(runtimecontract.RuntimeOperationIdempotencyUnknown) {
				continue
			}
			if _, enabled := serverAllowedTools[string(capability.ServerID)][capability.Name]; !enabled {
				continue
			}
			persistedCapabilities = append(persistedCapabilities, capability)
		}
		for _, capability := range persistedCapabilities {
			recipeRefs = append(recipeRefs, "mcp/tool_call/"+string(capability.ServerID)+"/"+capability.Name)
		}
	} else {
		// Server.AllowedTools is an execution policy, not a capability catalog.
		// Without the registry owner's typed capability query there is no
		// owner-attested recipe/effect/idempotency fact to seal into Runtime.
		// Fail closed instead of manufacturing recipe refs from the server-only
		// whitelist (RSO-19).
		return nil, nil, "", &runtimecontract.RuntimeCapabilityCatalogUnavailableError{
			OwnerModuleID: ModuleID,
			SubjectRef:    "mcp_capability_catalog",
			Reason:        "owner_unavailable",
		}
	}
	raw, err := json.Marshal(struct {
		Servers      []mcpmodel.Server      `json:"servers"`
		Capabilities []*mcpmodel.Capability `json:"capabilities,omitempty"`
	}{Servers: persisted, Capabilities: persistedCapabilities})
	if err != nil {
		return nil, nil, "", err
	}
	sum := sha256.Sum256(raw)
	sort.Strings(recipeRefs)
	return ids, recipeRefs, "mcp-admissibility-" + hex.EncodeToString(sum[:])[:20], nil
}

// EffectiveCapabilities returns the registry owner's detached capability facts
// for the enabled MCP surface. Runtime never derives effect metadata from a
// server risk label or tool name; an absent capability catalog is therefore an
// explicit prerequisite failure for semantic proof feasibility.
func (a *RuntimeAdapter) EffectiveCapabilities(ctx context.Context) ([]*mcpmodel.Capability, error) {
	if a == nil || a.service == nil {
		return nil, common.ErrInvalidArgument
	}
	lister, ok := a.service.(capabilityLister)
	if !ok {
		return nil, fmt.Errorf("%w: mcp capability catalog is unavailable", common.ErrInvalidArgument)
	}
	items, err := lister.ListCapabilities(ctx, registryevents.CapabilityQuery{Enabled: boolPtr(true)})
	if err != nil {
		return nil, err
	}
	return mcpmodel.CloneCapabilities(items), nil
}

func boolPtr(value bool) *bool { return &value }
