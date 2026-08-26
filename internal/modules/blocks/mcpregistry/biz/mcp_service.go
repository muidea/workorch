package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/common"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const maxRegistryCASAttempts = 8

type service struct {
	registry repository.MCPRegistryRepository
}

func newService(registry repository.MCPRegistryRepository) *service {
	return &service{registry: registry}
}

func (s *service) createServer(ctx context.Context, cmd mcpevents.CreateServerCommand) (*mcpmodel.Server, string, error) {
	server, err := serverFromCommand(cmd)
	if err != nil {
		return nil, "", err
	}
	snapshot, err := s.mutate(ctx, func(next *mcpmodel.RegistrySnapshot) error {
		if findServer(next.Servers, server.ID) != nil {
			return common.ErrConflict
		}
		next.Servers = append(next.Servers, mcpmodel.CloneServer(server))
		return reconcileServerCapabilities(next, server)
	})
	if err != nil {
		return nil, "", err
	}
	return mcpmodel.CloneServer(findServer(snapshot.Servers, server.ID)), registryContentRevision(snapshot), nil
}

func (s *service) updateServer(ctx context.Context, cmd mcpevents.UpdateServerCommand) (*mcpmodel.Server, string, error) {
	server, err := serverFromCommand(mcpevents.CreateServerCommand(cmd))
	if err != nil {
		return nil, "", err
	}
	snapshot, err := s.mutate(ctx, func(next *mcpmodel.RegistrySnapshot) error {
		idx := serverIndex(next.Servers, server.ID)
		if idx < 0 {
			return common.ErrNotFound
		}
		next.Servers[idx] = mcpmodel.CloneServer(server)
		return reconcileServerCapabilities(next, server)
	})
	if err != nil {
		return nil, "", err
	}
	return mcpmodel.CloneServer(findServer(snapshot.Servers, server.ID)), registryContentRevision(snapshot), nil
}

func (s *service) getServer(ctx context.Context, id common.MCPServerID) (*mcpmodel.Server, string, error) {
	id = common.MCPServerID(strings.TrimSpace(string(id)))
	if id == "" {
		return nil, "", common.ErrInvalidArgument
	}
	snapshot, err := s.load(ctx)
	if err != nil {
		return nil, "", err
	}
	item := findServer(snapshot.Servers, id)
	if item == nil {
		return nil, "", common.ErrNotFound
	}
	return mcpmodel.CloneServer(item), registryContentRevision(snapshot), nil
}

func (s *service) listServers(ctx context.Context, cursor common.MCPServerID, limit int) ([]*mcpmodel.Server, common.MCPServerID, string, error) {
	cursorValue, limit, err := normalizeListRequest(string(cursor), limit)
	if err != nil {
		return nil, "", "", err
	}
	snapshot, err := s.load(ctx)
	if err != nil {
		return nil, "", "", err
	}
	start := sort.Search(len(snapshot.Servers), func(idx int) bool { return string(snapshot.Servers[idx].ID) > cursorValue })
	end := start + limit
	if end > len(snapshot.Servers) {
		end = len(snapshot.Servers)
	}
	items := mcpmodel.CloneServers(snapshot.Servers[start:end])
	next := common.MCPServerID("")
	if end < len(snapshot.Servers) && len(items) > 0 {
		next = items[len(items)-1].ID
	}
	return items, next, registryContentRevision(snapshot), nil
}

func (s *service) createCapability(ctx context.Context, cmd mcpevents.CreateCapabilityCommand) (*mcpmodel.Capability, string, error) {
	var id common.MCPCapabilityID
	snapshot, err := s.mutate(ctx, func(next *mcpmodel.RegistrySnapshot) error {
		server := findServer(next.Servers, common.MCPServerID(strings.TrimSpace(string(cmd.ServerID))))
		if server == nil {
			return common.ErrNotFound
		}
		capability, err := capabilityFromCommand(cmd, server, mcpmodel.CapabilitySourceUser, "")
		if err != nil {
			return err
		}
		id = capability.ID
		existingBinding := findCapabilityByBinding(next.Capabilities, capability.ServerID, capability.Kind, capability.Name)
		if existingBinding != nil {
			if existingBinding.Source != mcpmodel.CapabilitySourceServerAllowlist {
				return common.ErrConflict
			}
			if byID := findCapability(next.Capabilities, id); byID != nil && byID != existingBinding {
				return common.ErrConflict
			}
			next.Capabilities[capabilityIndex(next.Capabilities, existingBinding.ID)] = capability
			return nil
		}
		if findCapability(next.Capabilities, id) != nil {
			return common.ErrConflict
		}
		next.Capabilities = append(next.Capabilities, capability)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return mcpmodel.CloneCapability(findCapability(snapshot.Capabilities, id)), registryContentRevision(snapshot), nil
}

func (s *service) updateCapability(ctx context.Context, cmd mcpevents.UpdateCapabilityCommand) (*mcpmodel.Capability, string, error) {
	mutation := mcpevents.CreateCapabilityCommand(cmd)
	id := common.MCPCapabilityID(strings.TrimSpace(string(mutation.CapabilityID)))
	if id == "" {
		return nil, "", common.ErrInvalidArgument
	}
	snapshot, err := s.mutate(ctx, func(next *mcpmodel.RegistrySnapshot) error {
		idx := capabilityIndex(next.Capabilities, id)
		if idx < 0 {
			return common.ErrNotFound
		}
		existing := next.Capabilities[idx]
		if common.MCPServerID(strings.TrimSpace(string(mutation.ServerID))) != existing.ServerID ||
			strings.TrimSpace(mutation.Kind) != existing.Kind || strings.TrimSpace(mutation.Name) != existing.Name {
			return common.ErrConflict
		}
		server := findServer(next.Servers, existing.ServerID)
		if server == nil {
			return common.ErrConflict
		}
		updated, err := capabilityFromCommand(mutation, server, existing.Source, existing.SourceRef)
		if err != nil {
			return err
		}
		updated.Revision = existing.Revision
		if reflect.DeepEqual(updated, existing) {
			return nil
		}
		updated.Revision++
		next.Capabilities[idx] = updated
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return mcpmodel.CloneCapability(findCapability(snapshot.Capabilities, id)), registryContentRevision(snapshot), nil
}

func (s *service) getCapability(ctx context.Context, id common.MCPCapabilityID) (*mcpmodel.Capability, string, error) {
	id = common.MCPCapabilityID(strings.TrimSpace(string(id)))
	if id == "" {
		return nil, "", common.ErrInvalidArgument
	}
	snapshot, err := s.load(ctx)
	if err != nil {
		return nil, "", err
	}
	item := findCapability(snapshot.Capabilities, id)
	if item == nil {
		return nil, "", common.ErrNotFound
	}
	return mcpmodel.CloneCapability(item), registryContentRevision(snapshot), nil
}

func (s *service) listCapabilities(ctx context.Context, query mcpevents.CapabilityQuery, cursor common.MCPCapabilityID, limit int) ([]*mcpmodel.Capability, common.MCPCapabilityID, string, error) {
	cursorValue, limit, err := normalizeListRequest(string(cursor), limit)
	if err != nil {
		return nil, "", "", err
	}
	query, err = normalizeCapabilityQuery(query)
	if err != nil {
		return nil, "", "", err
	}
	snapshot, err := s.load(ctx)
	if err != nil {
		return nil, "", "", err
	}
	items := make([]*mcpmodel.Capability, 0, limit+1)
	for _, item := range snapshot.Capabilities {
		if string(item.ID) <= cursorValue || !matchesCapabilityQuery(item, query) {
			continue
		}
		items = append(items, mcpmodel.CloneCapability(item))
		if len(items) > limit {
			break
		}
	}
	next := common.MCPCapabilityID("")
	if len(items) > limit {
		items = items[:limit]
		next = items[len(items)-1].ID
	}
	return items, next, registryContentRevision(snapshot), nil
}

func (s *service) registrySnapshot(ctx context.Context) (*mcpmodel.RegistrySnapshot, string, error) {
	snapshot, err := s.load(ctx)
	if err != nil {
		return nil, "", err
	}
	return mcpmodel.CloneRegistrySnapshot(snapshot), registryContentRevision(snapshot), nil
}

func (s *service) selectionSnapshot(ctx context.Context, serverIDs []common.MCPServerID, capabilityIDs []common.MCPCapabilityID) (*mcpmodel.RegistrySnapshot, string, error) {
	if len(serverIDs) > modulecommon.MaxSelectionServers || len(capabilityIDs) > modulecommon.MaxSelectionCapabilities {
		return nil, "", common.ErrInvalidArgument
	}
	snapshot, err := s.load(ctx)
	if err != nil {
		return nil, "", err
	}
	selected := &mcpmodel.RegistrySnapshot{Revision: snapshot.Revision}
	selectedServerIDs := make(map[common.MCPServerID]struct{}, len(serverIDs))
	for _, rawID := range serverIDs {
		id := common.MCPServerID(strings.TrimSpace(string(rawID)))
		if !validBoundedString(string(id), modulecommon.MaxIDBytes, true) {
			return nil, "", common.ErrInvalidArgument
		}
		if _, duplicate := selectedServerIDs[id]; duplicate {
			continue
		}
		server := findServer(snapshot.Servers, id)
		if server == nil {
			return nil, "", common.ErrNotFound
		}
		if !server.Enabled {
			return nil, "", common.ErrConflict
		}
		selectedServerIDs[id] = struct{}{}
		selected.Servers = append(selected.Servers, mcpmodel.CloneServer(server))
	}
	seenCapabilities := make(map[common.MCPCapabilityID]struct{}, len(capabilityIDs))
	for _, rawID := range capabilityIDs {
		id := common.MCPCapabilityID(strings.TrimSpace(string(rawID)))
		if !validBoundedString(string(id), modulecommon.MaxIDBytes, true) {
			return nil, "", common.ErrInvalidArgument
		}
		if _, duplicate := seenCapabilities[id]; duplicate {
			continue
		}
		capability := findCapability(snapshot.Capabilities, id)
		if capability == nil {
			return nil, "", common.ErrNotFound
		}
		if !capability.Enabled {
			return nil, "", common.ErrConflict
		}
		if _, selectedServer := selectedServerIDs[capability.ServerID]; !selectedServer {
			return nil, "", common.ErrConflict
		}
		server := findServer(snapshot.Servers, capability.ServerID)
		if server == nil || !serverAllowsCapability(server, capability.Kind, capability.Name) {
			return nil, "", common.ErrConflict
		}
		seenCapabilities[id] = struct{}{}
		selected.Capabilities = append(selected.Capabilities, mcpmodel.CloneCapability(capability))
	}
	return selected, registryContentRevision(snapshot), nil
}

func (s *service) load(ctx context.Context) (*mcpmodel.RegistrySnapshot, error) {
	if s == nil || s.registry == nil {
		return nil, common.ErrInvalidArgument
	}
	snapshot, err := s.registry.Load(ctx)
	if err != nil {
		return nil, err
	}
	return canonicalRegistrySnapshot(snapshot)
}

func (s *service) mutate(ctx context.Context, apply func(*mcpmodel.RegistrySnapshot) error) (*mcpmodel.RegistrySnapshot, error) {
	if s == nil || s.registry == nil || apply == nil {
		return nil, common.ErrInvalidArgument
	}
	for attempt := 0; attempt < maxRegistryCASAttempts; attempt++ {
		current, err := s.load(ctx)
		if err != nil {
			return nil, err
		}
		next := mcpmodel.CloneRegistrySnapshot(current)
		if err := apply(next); err != nil {
			return nil, err
		}
		next, err = canonicalRegistrySnapshot(next)
		if err != nil {
			return nil, err
		}
		if registryContentRevision(current) == registryContentRevision(next) {
			return current, nil
		}
		next.Revision = current.Revision + 1
		if err := s.registry.Commit(ctx, current.Revision, next); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return nil, err
		}
		return next, nil
	}
	return nil, common.ErrConflict
}

func canonicalRegistrySnapshot(snapshot *mcpmodel.RegistrySnapshot) (*mcpmodel.RegistrySnapshot, error) {
	value := mcpmodel.CloneRegistrySnapshot(snapshot)
	if len(value.Servers) > modulecommon.MaxRegistryServers || len(value.Capabilities) > modulecommon.MaxRegistryCapabilities {
		return nil, common.ErrInvalidArgument
	}
	servers := make([]*mcpmodel.Server, 0, len(value.Servers))
	serverByID := make(map[common.MCPServerID]*mcpmodel.Server, len(value.Servers))
	for _, raw := range value.Servers {
		server, err := normalizeServer(raw)
		if err != nil {
			return nil, err
		}
		if _, duplicate := serverByID[server.ID]; duplicate {
			return nil, common.ErrConflict
		}
		serverByID[server.ID] = server
		servers = append(servers, server)
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].ID < servers[j].ID })
	capabilities := make([]*mcpmodel.Capability, 0, len(value.Capabilities))
	capabilityIDs := make(map[common.MCPCapabilityID]struct{}, len(value.Capabilities))
	capabilityBindings := make(map[string]struct{}, len(value.Capabilities))
	for _, raw := range value.Capabilities {
		if raw == nil {
			return nil, common.ErrInvalidArgument
		}
		server := serverByID[common.MCPServerID(strings.TrimSpace(string(raw.ServerID)))]
		if server == nil {
			return nil, common.ErrConflict
		}
		capability, err := normalizeCapability(raw, server)
		if err != nil {
			return nil, err
		}
		if _, duplicate := capabilityIDs[capability.ID]; duplicate {
			return nil, common.ErrConflict
		}
		binding := capabilityBindingKey(capability.ServerID, capability.Kind, capability.Name)
		if _, duplicate := capabilityBindings[binding]; duplicate {
			return nil, common.ErrConflict
		}
		capabilityIDs[capability.ID] = struct{}{}
		capabilityBindings[binding] = struct{}{}
		capabilities = append(capabilities, capability)
	}
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i].ID < capabilities[j].ID })
	value.Servers = servers
	value.Capabilities = capabilities
	return value, nil
}

func serverFromCommand(cmd mcpevents.CreateServerCommand) (*mcpmodel.Server, error) {
	env, err := mcpevents.EnvironmentReferenceMap(cmd.EnvironmentRefs)
	if err != nil {
		return nil, err
	}
	return normalizeServer(&mcpmodel.Server{
		ID: cmd.ServerID, Name: cmd.Name, Transport: cmd.Transport, Endpoint: cmd.Endpoint, Command: cmd.Command,
		Args: append([]string(nil), cmd.Args...), Env: env, Enabled: cmd.Enabled,
		AllowedTools: append([]string(nil), cmd.AllowedTools...), AllowedResources: append([]string(nil), cmd.AllowedResources...),
		AllowedPrompts: append([]string(nil), cmd.AllowedPrompts...), DefaultApprovalPolicy: cmd.DefaultApprovalPolicy,
		SessionPolicy: cmd.SessionPolicy, TimeoutMillis: cmd.TimeoutMillis,
	})
}

func normalizeServer(raw *mcpmodel.Server) (*mcpmodel.Server, error) {
	if raw == nil {
		return nil, common.ErrInvalidArgument
	}
	server := mcpmodel.CloneServer(raw)
	server.ID = common.MCPServerID(strings.TrimSpace(string(server.ID)))
	server.Name = strings.TrimSpace(server.Name)
	server.Transport = strings.ToLower(strings.TrimSpace(server.Transport))
	server.Endpoint = strings.TrimSpace(server.Endpoint)
	server.Command = strings.TrimSpace(server.Command)
	server.DefaultApprovalPolicy = strings.TrimSpace(server.DefaultApprovalPolicy)
	server.SessionPolicy = strings.TrimSpace(server.SessionPolicy)
	if !validBoundedString(string(server.ID), modulecommon.MaxIDBytes, true) || !validBoundedString(server.Name, modulecommon.MaxNameBytes, true) || server.TimeoutMillis < 0 {
		return nil, common.ErrInvalidArgument
	}
	switch server.Transport {
	case "stdio":
		if server.Command == "" && server.Endpoint == "" {
			return nil, common.ErrInvalidArgument
		}
	case "http", "https":
		parsed, err := url.ParseRequestURI(server.Endpoint)
		if err != nil || parsed.Scheme != server.Transport || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, common.ErrInvalidArgument
		}
	default:
		return nil, common.ErrInvalidArgument
	}
	switch server.SessionPolicy {
	case "", "transient", "long_lived":
	default:
		return nil, common.ErrInvalidArgument
	}
	var err error
	server.Args, err = normalizeOrderedStrings(server.Args, modulecommon.MaxStringListItems)
	if err != nil {
		return nil, err
	}
	server.AllowedTools, err = normalizeStringSet(server.AllowedTools, modulecommon.MaxStringListItems)
	if err != nil {
		return nil, err
	}
	server.AllowedResources, err = normalizeStringSet(server.AllowedResources, modulecommon.MaxStringListItems)
	if err != nil {
		return nil, err
	}
	server.AllowedPrompts, err = normalizeStringSet(server.AllowedPrompts, modulecommon.MaxStringListItems)
	if err != nil {
		return nil, err
	}
	refs := mcpevents.EnvironmentReferencesFromMap(server.Env)
	server.Env, err = mcpevents.EnvironmentReferenceMap(refs)
	if err != nil {
		return nil, err
	}
	for _, value := range []string{server.Endpoint, server.Command, server.DefaultApprovalPolicy, server.SessionPolicy} {
		if !validBoundedString(value, modulecommon.MaxTextBytes, false) {
			return nil, common.ErrInvalidArgument
		}
	}
	return server, nil
}

func capabilityFromCommand(cmd mcpevents.CreateCapabilityCommand, server *mcpmodel.Server, source string, sourceRef string) (*mcpmodel.Capability, error) {
	id := common.MCPCapabilityID(strings.TrimSpace(string(cmd.CapabilityID)))
	kind := strings.TrimSpace(cmd.Kind)
	name := strings.TrimSpace(cmd.Name)
	if id == "" {
		id = DefaultCapabilityID(server.ID, kind, name)
	}
	if sourceRef == "" {
		sourceRef = string(id)
	}
	if server == nil || !serverAllowsCapability(server, kind, name) || (cmd.Enabled && !server.Enabled) {
		return nil, common.ErrConflict
	}
	return normalizeCapability(&mcpmodel.Capability{
		ID: id, ServerID: cmd.ServerID, Kind: kind, Name: name, DisplayName: cmd.DisplayName,
		Description: cmd.Description, Category: cmd.Category, Tags: append([]string(nil), cmd.Tags...), UsageHint: cmd.UsageHint,
		InputSummary: cmd.InputSummary, OutputSummary: cmd.OutputSummary, StageHints: append([]string(nil), cmd.StageHints...),
		RiskLevel: cmd.RiskLevel, EffectScope: cmd.EffectScope, IdempotencyClass: cmd.IdempotencyClass,
		ValidationProofApplicability: cmd.ValidationProofApplicability, Schema: cmd.Schema.Clone(), URI: cmd.URI,
		Enabled: cmd.Enabled, ApprovalPolicy: cmd.ApprovalPolicy, Source: source, SourceRef: sourceRef, Revision: 1,
	}, server)
}

func normalizeCapability(raw *mcpmodel.Capability, server *mcpmodel.Server) (*mcpmodel.Capability, error) {
	if raw == nil || server == nil {
		return nil, common.ErrInvalidArgument
	}
	capability := mcpmodel.CloneCapability(raw)
	capability.ID = common.MCPCapabilityID(strings.TrimSpace(string(capability.ID)))
	capability.ServerID = common.MCPServerID(strings.TrimSpace(string(capability.ServerID)))
	capability.Kind = strings.TrimSpace(capability.Kind)
	capability.Name = strings.TrimSpace(capability.Name)
	capability.DisplayName = strings.TrimSpace(capability.DisplayName)
	if capability.DisplayName == "" {
		capability.DisplayName = capability.Name
	}
	capability.Description = strings.TrimSpace(capability.Description)
	capability.Category = strings.TrimSpace(capability.Category)
	capability.UsageHint = strings.TrimSpace(capability.UsageHint)
	capability.InputSummary = strings.TrimSpace(capability.InputSummary)
	capability.OutputSummary = strings.TrimSpace(capability.OutputSummary)
	capability.RiskLevel = strings.TrimSpace(capability.RiskLevel)
	capability.EffectScope = strings.TrimSpace(capability.EffectScope)
	if capability.EffectScope == "" {
		capability.EffectScope = string(runtimecontract.RuntimeEffectUnknown)
	}
	capability.IdempotencyClass = strings.TrimSpace(capability.IdempotencyClass)
	if capability.IdempotencyClass == "" {
		capability.IdempotencyClass = string(runtimecontract.RuntimeOperationIdempotencyUnknown)
	}
	capability.ValidationProofApplicability = strings.TrimSpace(capability.ValidationProofApplicability)
	if capability.ValidationProofApplicability == "" {
		capability.ValidationProofApplicability = mcpmodel.ValidationProofApplicabilityNotDeclared
	}
	capability.URI = strings.TrimSpace(capability.URI)
	capability.ApprovalPolicy = strings.TrimSpace(capability.ApprovalPolicy)
	capability.Source = strings.TrimSpace(capability.Source)
	capability.SourceRef = strings.TrimSpace(capability.SourceRef)
	if capability.ServerID != server.ID || !isCapabilityKind(capability.Kind) {
		return nil, common.ErrConflict
	}
	if capability.Enabled && (!server.Enabled || !serverAllowsCapability(server, capability.Kind, capability.Name)) {
		return nil, common.ErrConflict
	}
	if !validBoundedString(string(capability.ID), modulecommon.MaxIDBytes, true) || !validBoundedString(capability.Name, modulecommon.MaxNameBytes, true) ||
		!validBoundedString(capability.DisplayName, modulecommon.MaxNameBytes, true) || capability.Revision == 0 || capability.SourceRef == "" {
		return nil, common.ErrInvalidArgument
	}
	switch capability.Source {
	case mcpmodel.CapabilitySourceUser, mcpmodel.CapabilitySourceBuiltin, mcpmodel.CapabilitySourceServerAllowlist:
	default:
		return nil, common.ErrInvalidArgument
	}
	if !runtimecontract.ValidRuntimeEffectScope(runtimecontract.RuntimeEffectScope(capability.EffectScope)) ||
		runtimecontract.NormalizeRuntimeOperationIdempotencyClass(runtimecontract.RuntimeOperationIdempotencyClass(capability.IdempotencyClass)) != runtimecontract.RuntimeOperationIdempotencyClass(capability.IdempotencyClass) ||
		!mcpmodel.ValidValidationProofApplicability(capability.ValidationProofApplicability) {
		return nil, common.ErrInvalidArgument
	}
	var err error
	capability.Tags, err = normalizeStringSet(capability.Tags, modulecommon.MaxStringListItems)
	if err != nil {
		return nil, err
	}
	capability.StageHints, err = normalizeStringSet(capability.StageHints, modulecommon.MaxStringListItems)
	if err != nil {
		return nil, err
	}
	for _, value := range []string{capability.Description, capability.Category, capability.UsageHint, capability.InputSummary, capability.OutputSummary, capability.RiskLevel, capability.URI, capability.ApprovalPolicy, capability.SourceRef} {
		if !validBoundedString(value, modulecommon.MaxTextBytes, false) {
			return nil, common.ErrInvalidArgument
		}
	}
	if len(capability.Schema.CanonicalString()) > modulecommon.MaxSchemaBytes {
		return nil, common.ErrInvalidArgument
	}
	return capability, nil
}

func reconcileServerCapabilities(snapshot *mcpmodel.RegistrySnapshot, server *mcpmodel.Server) error {
	if snapshot == nil || server == nil {
		return common.ErrInvalidArgument
	}
	desired := defaultCapabilities(server)
	desiredByBinding := make(map[string]*mcpmodel.Capability, len(desired))
	for _, capability := range desired {
		desiredByBinding[capabilityBindingKey(capability.ServerID, capability.Kind, capability.Name)] = capability
	}
	for _, capability := range snapshot.Capabilities {
		if capability == nil || capability.ServerID != server.ID {
			continue
		}
		binding := capabilityBindingKey(capability.ServerID, capability.Kind, capability.Name)
		_, allowed := desiredByBinding[binding]
		if (!allowed || !server.Enabled) && capability.Enabled {
			capability.Enabled = false
			capability.Revision++
		}
		if allowed {
			delete(desiredByBinding, binding)
		}
	}
	for _, capability := range desiredByBinding {
		snapshot.Capabilities = append(snapshot.Capabilities, capability)
	}
	return nil
}

func capabilityBindingKey(serverID common.MCPServerID, kind, name string) string {
	return string(serverID) + "\x00" + strings.TrimSpace(kind) + "\x00" + strings.TrimSpace(name)
}

func defaultCapabilities(server *mcpmodel.Server) []*mcpmodel.Capability {
	ret := make([]*mcpmodel.Capability, 0, len(server.AllowedTools)+len(server.AllowedResources)+len(server.AllowedPrompts))
	for _, name := range server.AllowedTools {
		ret = append(ret, defaultCapability(server, mcpmodel.CapabilityKindTool, name, ""))
	}
	for _, uri := range server.AllowedResources {
		ret = append(ret, defaultCapability(server, mcpmodel.CapabilityKindResource, uri, uri))
	}
	for _, name := range server.AllowedPrompts {
		ret = append(ret, defaultCapability(server, mcpmodel.CapabilityKindPrompt, name, ""))
	}
	return ret
}

func defaultCapability(server *mcpmodel.Server, kind string, name string, uri string) *mcpmodel.Capability {
	id := DefaultCapabilityID(server.ID, kind, name)
	return &mcpmodel.Capability{
		ID: id, ServerID: server.ID, Kind: kind, Name: name, DisplayName: defaultCapabilityDisplayName(kind, name), URI: uri,
		Enabled: server.Enabled, ApprovalPolicy: server.DefaultApprovalPolicy, RiskLevel: "read_only",
		EffectScope: string(runtimecontract.RuntimeEffectUnknown), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencyUnknown),
		ValidationProofApplicability: mcpmodel.ValidationProofApplicabilityNotDeclared,
		Source:                       mcpmodel.CapabilitySourceServerAllowlist, SourceRef: string(id), Revision: 1,
	}
}

func DefaultCapabilityID(serverID common.MCPServerID, kind string, name string) common.MCPCapabilityID {
	return common.MCPCapabilityID(string(serverID) + ":" + strings.TrimSpace(kind) + ":" + url.PathEscape(strings.TrimSpace(name)))
}

func defaultCapabilityDisplayName(kind string, name string) string {
	switch kind {
	case mcpmodel.CapabilityKindTool:
		return "工具 " + name
	case mcpmodel.CapabilityKindResource:
		return "资源 " + name
	case mcpmodel.CapabilityKindPrompt:
		return "提示词 " + name
	default:
		return name
	}
}

func matchesCapabilityQuery(item *mcpmodel.Capability, query mcpevents.CapabilityQuery) bool {
	if query.ServerID != "" && item.ServerID != query.ServerID {
		return false
	}
	if value := strings.TrimSpace(query.Kind); value != "" && item.Kind != value {
		return false
	}
	if value := strings.TrimSpace(query.Category); value != "" && item.Category != value {
		return false
	}
	if value := strings.TrimSpace(query.RiskLevel); value != "" && item.RiskLevel != value {
		return false
	}
	if query.Enabled != nil && item.Enabled != *query.Enabled {
		return false
	}
	if value := strings.TrimSpace(query.Stage); value != "" && !containsString(item.StageHints, value) {
		return false
	}
	q := strings.ToLower(strings.TrimSpace(query.Query))
	return q == "" || capabilityContains(item, q)
}

func capabilityContains(item *mcpmodel.Capability, query string) bool {
	values := []string{string(item.ID), string(item.ServerID), item.Kind, item.Name, item.DisplayName, item.Description, item.Category, item.UsageHint, item.InputSummary, item.OutputSummary, item.RiskLevel, item.Source, item.SourceRef}
	values = append(values, item.Tags...)
	values = append(values, item.StageHints...)
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func isCapabilityKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case mcpmodel.CapabilityKindTool, mcpmodel.CapabilityKindResource, mcpmodel.CapabilityKindPrompt:
		return true
	default:
		return false
	}
}

func serverAllowsCapability(server *mcpmodel.Server, kind string, name string) bool {
	if server == nil {
		return false
	}
	var values []string
	switch kind {
	case mcpmodel.CapabilityKindTool:
		values = server.AllowedTools
	case mcpmodel.CapabilityKindResource:
		values = server.AllowedResources
	case mcpmodel.CapabilityKindPrompt:
		values = server.AllowedPrompts
	default:
		return false
	}
	return containsString(values, name)
}

func containsString(values []string, target string) bool {
	idx := sort.SearchStrings(values, target)
	return idx < len(values) && values[idx] == target
}

func findServer(items []*mcpmodel.Server, id common.MCPServerID) *mcpmodel.Server {
	idx := serverIndex(items, id)
	if idx < 0 {
		return nil
	}
	return items[idx]
}

func serverIndex(items []*mcpmodel.Server, id common.MCPServerID) int {
	for idx, item := range items {
		if item != nil && item.ID == id {
			return idx
		}
	}
	return -1
}

func findCapability(items []*mcpmodel.Capability, id common.MCPCapabilityID) *mcpmodel.Capability {
	idx := capabilityIndex(items, id)
	if idx < 0 {
		return nil
	}
	return items[idx]
}

func capabilityIndex(items []*mcpmodel.Capability, id common.MCPCapabilityID) int {
	for idx, item := range items {
		if item != nil && item.ID == id {
			return idx
		}
	}
	return -1
}

func registryContentRevision(snapshot *mcpmodel.RegistrySnapshot) string {
	value := mcpmodel.CloneRegistrySnapshot(snapshot)
	value.Revision = 0
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return "mcp-registry-" + hex.EncodeToString(sum[:])[:24]
}

func normalizeListRequest(cursor string, limit int) (string, int, error) {
	cursor = strings.TrimSpace(cursor)
	if !validBoundedString(cursor, modulecommon.MaxIDBytes, false) || limit < 0 || limit > modulecommon.MaxListLimit {
		return "", 0, common.ErrInvalidArgument
	}
	if limit == 0 {
		limit = modulecommon.DefaultListLimit
	}
	return cursor, limit, nil
}

func normalizeCapabilityQuery(query mcpevents.CapabilityQuery) (mcpevents.CapabilityQuery, error) {
	query.ServerID = common.MCPServerID(strings.TrimSpace(string(query.ServerID)))
	query.Kind = strings.TrimSpace(query.Kind)
	query.Category = strings.TrimSpace(query.Category)
	query.Stage = strings.TrimSpace(query.Stage)
	query.RiskLevel = strings.TrimSpace(query.RiskLevel)
	query.Query = strings.TrimSpace(query.Query)
	if !validBoundedString(string(query.ServerID), modulecommon.MaxIDBytes, false) ||
		(query.Kind != "" && !isCapabilityKind(query.Kind)) ||
		!validBoundedString(query.Category, modulecommon.MaxNameBytes, false) ||
		!validBoundedString(query.Stage, modulecommon.MaxNameBytes, false) ||
		!validBoundedString(query.RiskLevel, modulecommon.MaxNameBytes, false) ||
		!validBoundedString(query.Query, modulecommon.MaxQueryBytes, false) {
		return mcpevents.CapabilityQuery{}, common.ErrInvalidArgument
	}
	return query, nil
}

func normalizeStringSet(values []string, maxItems int) ([]string, error) {
	if len(values) > maxItems {
		return nil, common.ErrInvalidArgument
	}
	seen := make(map[string]struct{}, len(values))
	ret := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if !validBoundedString(value, modulecommon.MaxStringItemBytes, true) {
			return nil, common.ErrInvalidArgument
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Strings(ret)
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func normalizeOrderedStrings(values []string, maxItems int) ([]string, error) {
	if len(values) > maxItems {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if !validBoundedString(value, modulecommon.MaxStringItemBytes, true) {
			return nil, common.ErrInvalidArgument
		}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func validBoundedString(value string, maxBytes int, required bool) bool {
	if !utf8.ValidString(value) || len(value) > maxBytes {
		return false
	}
	return !required || value != ""
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
