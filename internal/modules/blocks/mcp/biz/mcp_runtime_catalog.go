package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

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
	reader, ok := a.service.(registrySnapshotGetter)
	if !ok {
		return runtimeMCPAdmissibilitySnapshot{}, &runtimecontract.RuntimeCapabilityCatalogUnavailableError{
			OwnerModuleID: ModuleID,
			SubjectRef:    "mcp_capability_catalog",
			Reason:        "owner_unavailable",
		}
	}
	registry, registryRevision, err := reader.RegistrySnapshot(ctx)
	if err != nil {
		return runtimeMCPAdmissibilitySnapshot{}, err
	}
	if registry == nil || strings.TrimSpace(registryRevision) == "" {
		return runtimeMCPAdmissibilitySnapshot{}, common.ErrInvalidArgument
	}
	servers := registry.Servers
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
	type allowedSurfaces struct {
		tools     map[string]struct{}
		resources map[string]struct{}
		prompts   map[string]struct{}
	}
	serverAllowed := make(map[string]allowedSurfaces, len(orderedServers))
	for _, server := range orderedServers {
		if server == nil || !server.Enabled || strings.TrimSpace(string(server.ID)) == "" {
			continue
		}
		ids = append(ids, string(server.ID))
		allowed := allowedSurfaces{
			tools: make(map[string]struct{}, len(server.AllowedTools)), resources: make(map[string]struct{}, len(server.AllowedResources)),
			prompts: make(map[string]struct{}, len(server.AllowedPrompts)),
		}
		for _, tool := range server.AllowedTools {
			if tool = strings.TrimSpace(tool); tool != "" {
				allowed.tools[tool] = struct{}{}
			}
		}
		for _, resource := range server.AllowedResources {
			if resource = strings.TrimSpace(resource); resource != "" {
				allowed.resources[resource] = struct{}{}
			}
		}
		for _, prompt := range server.AllowedPrompts {
			if prompt = strings.TrimSpace(prompt); prompt != "" {
				allowed.prompts[prompt] = struct{}{}
			}
		}
		serverAllowed[string(server.ID)] = allowed
	}
	if len(ids) == 0 {
		return runtimeMCPAdmissibilitySnapshot{}, fmt.Errorf("%w: no enabled mcp server is available", common.ErrNotFound)
	}
	persistedCapabilities := make([]*mcpmodel.Capability, 0, len(registry.Capabilities))
	for _, capability := range registry.Capabilities {
		if capability == nil || !capability.Enabled ||
			strings.TrimSpace(string(capability.ID)) == "" || strings.TrimSpace(string(capability.ServerID)) == "" ||
			strings.TrimSpace(capability.Name) == "" || strings.TrimSpace(capability.Name) != capability.Name ||
			strings.TrimSpace(capability.EffectScope) == string(runtimecontract.RuntimeEffectUnknown) ||
			strings.TrimSpace(capability.IdempotencyClass) == string(runtimecontract.RuntimeOperationIdempotencyUnknown) {
			continue
		}
		allowed, serverEnabled := serverAllowed[string(capability.ServerID)]
		if !serverEnabled || !mcpCapabilityAllowed(capability, allowed.tools, allowed.resources, allowed.prompts) {
			continue
		}
		persistedCapabilities = append(persistedCapabilities, capability)
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
	surfaces := make([]runtimecontract.RuntimeValidationSurface, 0, len(persistedCapabilities))
	for _, capability := range persistedCapabilities {
		if capability.Kind != mcpmodel.CapabilityKindTool {
			continue
		}
		surface := runtimeMCPValidationSurface(capability)
		surfaces = append(surfaces, surface)
	}
	surfaces = runtimecontract.NormalizeRuntimeValidationSurfaces(surfaces)
	recipeRefs := runtimecontract.RuntimeSelectableValidationRecipeRefs(surfaces)
	return runtimeMCPAdmissibilitySnapshot{
		ServerIDs: append([]string(nil), ids...), ValidationRecipes: recipeRefs,
		ValidationSurfaces: surfaces,
		Capabilities:       mcpmodel.CloneCapabilities(persistedCapabilities),
		Revision:           registryRevision,
	}, nil
}

func mcpCapabilityAllowed(capability *mcpmodel.Capability, tools, resources, prompts map[string]struct{}) bool {
	if capability == nil {
		return false
	}
	switch capability.Kind {
	case mcpmodel.CapabilityKindTool:
		_, found := tools[strings.TrimSpace(capability.Name)]
		return found
	case mcpmodel.CapabilityKindResource:
		ref := strings.TrimSpace(capability.URI)
		if ref == "" {
			ref = strings.TrimSpace(capability.Name)
		}
		_, found := resources[ref]
		return found
	case mcpmodel.CapabilityKindPrompt:
		_, found := prompts[strings.TrimSpace(capability.Name)]
		return found
	default:
		return false
	}
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
	reader, ok := a.service.(registrySnapshotGetter)
	if !ok {
		return nil, &runtimecontract.RuntimeCapabilityCatalogUnavailableError{
			OwnerModuleID: ModuleID,
			SubjectRef:    "mcp_capability_catalog",
			Reason:        "owner_unavailable",
		}
	}
	snapshot, _, err := reader.RegistrySnapshot(ctx)
	if err != nil {
		return nil, err
	}
	ret := make([]*mcpmodel.Capability, 0, len(snapshot.Capabilities))
	for _, item := range snapshot.Capabilities {
		if item != nil && item.Enabled {
			ret = append(ret, mcpmodel.CloneCapability(item))
		}
	}
	return ret, nil
}
