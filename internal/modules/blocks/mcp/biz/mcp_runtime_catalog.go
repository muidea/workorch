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

type runtimeMCPAdmissibilitySnapshot struct {
	ServerIDs          []string
	ValidationRecipes  []string
	ValidationSurfaces []runtimecontract.RuntimeValidationSurface
	Capabilities       []*mcpmodel.Capability
	Revision           string
}

// OperationAdmissibility returns enabled MCP server IDs, owner-attested
// validation recipe identities, and a stable hash of their effective registry
// state. It performs only the typed registry query; no MCP transport is opened
// and no capability operation is invoked.
func (a *RuntimeAdapter) OperationAdmissibility(ctx context.Context) ([]string, []string, string, error) {
	snapshot, err := a.runtimeOperationAdmissibilitySnapshot(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	return append([]string(nil), snapshot.ServerIDs...), append([]string(nil), snapshot.ValidationRecipes...), snapshot.Revision, nil
}

// runtimeOperationAdmissibilitySnapshot acquires one internally consistent
// registry snapshot. Callable capabilities and currently ready validation
// surfaces deliberately remain separate: a tool can stay executable without
// being advertised as an input-complete semantic validation recipe.
func (a *RuntimeAdapter) runtimeOperationAdmissibilitySnapshot(ctx context.Context) (runtimeMCPAdmissibilitySnapshot, error) {
	if a == nil || a.service == nil {
		return runtimeMCPAdmissibilitySnapshot{}, common.ErrInvalidArgument
	}
	lister, ok := a.service.(serverLister)
	if !ok {
		return runtimeMCPAdmissibilitySnapshot{}, fmt.Errorf("%w: mcp server catalog is unavailable", common.ErrInvalidArgument)
	}
	servers, err := lister.ListServers(ctx)
	if err != nil {
		return runtimeMCPAdmissibilitySnapshot{}, err
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
		return runtimeMCPAdmissibilitySnapshot{}, fmt.Errorf("%w: no enabled mcp server is available", common.ErrNotFound)
	}
	var persistedCapabilities []*mcpmodel.Capability
	if lister, ok := a.service.(capabilityLister); ok {
		allCapabilities, listErr := lister.ListCapabilities(ctx, registryevents.CapabilityQuery{Enabled: boolPtr(true), Kind: mcpmodel.CapabilityKindTool})
		if listErr != nil {
			return runtimeMCPAdmissibilitySnapshot{}, listErr
		}
		for _, capability := range allCapabilities {
			if capability == nil || !capability.Enabled || capability.Kind != mcpmodel.CapabilityKindTool ||
				strings.TrimSpace(string(capability.ID)) == "" || strings.TrimSpace(string(capability.ServerID)) == "" ||
				strings.TrimSpace(capability.Name) == "" || strings.TrimSpace(capability.Name) != capability.Name ||
				strings.TrimSpace(capability.EffectScope) == string(runtimecontract.RuntimeEffectUnknown) ||
				strings.TrimSpace(capability.IdempotencyClass) == string(runtimecontract.RuntimeOperationIdempotencyUnknown) {
				continue
			}
			if _, enabled := serverAllowedTools[string(capability.ServerID)][capability.Name]; !enabled {
				continue
			}
			persistedCapabilities = append(persistedCapabilities, capability)
		}
	} else {
		// Server.AllowedTools is an execution policy, not a capability catalog.
		// Without the registry owner's typed capability query there is no
		// owner-attested recipe/effect/idempotency fact to seal into Runtime.
		// Fail closed instead of manufacturing recipe refs from the server-only
		// whitelist (RSO-19).
		return runtimeMCPAdmissibilitySnapshot{}, &runtimecontract.RuntimeCapabilityCatalogUnavailableError{
			OwnerModuleID: ModuleID,
			SubjectRef:    "mcp_capability_catalog",
			Reason:        "owner_unavailable",
		}
	}
	sort.Slice(persistedCapabilities, func(i, j int) bool {
		left, right := persistedCapabilities[i], persistedCapabilities[j]
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		if left.ServerID != right.ServerID {
			return left.ServerID < right.ServerID
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.ID < right.ID
	})
	raw, err := json.Marshal(struct {
		Servers      []mcpmodel.Server      `json:"servers"`
		Capabilities []*mcpmodel.Capability `json:"capabilities,omitempty"`
	}{Servers: persisted, Capabilities: persistedCapabilities})
	if err != nil {
		return runtimeMCPAdmissibilitySnapshot{}, err
	}
	sum := sha256.Sum256(raw)
	surfaces := make([]runtimecontract.RuntimeValidationSurface, 0, len(persistedCapabilities))
	for _, capability := range persistedCapabilities {
		surface := runtimeMCPValidationSurface(capability)
		surfaces = append(surfaces, surface)
	}
	surfaces = runtimecontract.NormalizeRuntimeValidationSurfaces(surfaces)
	recipeRefs := runtimecontract.RuntimeSelectableValidationRecipeRefs(surfaces)
	return runtimeMCPAdmissibilitySnapshot{
		ServerIDs: append([]string(nil), ids...), ValidationRecipes: recipeRefs,
		ValidationSurfaces: surfaces,
		Capabilities:       mcpmodel.CloneCapabilities(persistedCapabilities),
		Revision:           "mcp-admissibility-" + hex.EncodeToString(sum[:])[:20],
	}, nil
}

func runtimeMCPValidationSurface(capability *mcpmodel.Capability) runtimecontract.RuntimeValidationSurface {
	recipeID := ""
	if capability != nil {
		recipeID = "mcp/tool_call/" + string(capability.ServerID) + "/" + strings.TrimSpace(capability.Name)
	}
	schemaCanonical := ""
	if capability != nil {
		schemaCanonical = capability.Schema.CanonicalString()
	}
	schemaSum := sha256.Sum256([]byte(schemaCanonical))
	schemaHash := "mcp-input-schema-" + hex.EncodeToString(schemaSum[:])[:20]
	effect := runtimecontract.RuntimeEffectUnknown
	idempotency := runtimecontract.RuntimeOperationIdempotencyUnknown
	if capability != nil {
		effect = runtimecontract.NormalizeRuntimeEffectScope(runtimecontract.RuntimeEffectScope(capability.EffectScope))
		idempotency = runtimecontract.NormalizeRuntimeOperationIdempotencyClass(runtimecontract.RuntimeOperationIdempotencyClass(capability.IdempotencyClass))
	}
	surface := runtimecontract.RuntimeValidationSurface{
		ID: recipeID, OwnerModuleID: ModuleID, OperationKind: runtimecontract.StepKindMCPToolCall,
		InputSchemaHash: schemaHash, CheckRefs: []string{recipeID}, ProofApplicability: runtimecontract.RuntimeValidationProofNotDeclared,
		EffectScope: effect, IdempotencyClass: idempotency,
	}
	if capability != nil {
		surface.ProofApplicability = runtimecontract.RuntimeValidationProofApplicability(capability.ValidationProofApplicability)
	}
	switch {
	case !runtimecontract.RuntimeEffectIsNonDurable(effect):
		surface.Readiness = runtimecontract.RuntimeValidationSurfaceUnavailable
		surface.ReadinessReason = "unsafe_effect"
	case idempotency != runtimecontract.RuntimeOperationIdempotencySafeToReplay:
		surface.Readiness = runtimecontract.RuntimeValidationSurfaceUnavailable
		surface.ReadinessReason = "unsafe_replay"
	default:
		requiresInput, err := inspectMCPBehaviorProbeSchema(capability.Schema)
		if err != nil {
			surface.Readiness = runtimecontract.RuntimeValidationSurfaceUnavailable
			surface.ReadinessReason = "invalid_input_schema"
		} else if requiresInput {
			surface.Readiness = runtimecontract.RuntimeValidationSurfacePrerequisiteRequired
			surface.ReadinessReason = "input_binding_required"
			surface.PrerequisiteRef = "mcp-validation-input/" + string(capability.ID) + "/" + schemaHash
		} else {
			surface.Readiness = runtimecontract.RuntimeValidationSurfaceReady
			surface.InputBindingRef = "mcp-validation-input/empty/" + string(capability.ID) + "/" + schemaHash
		}
	}
	return surface.Normalized()
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
