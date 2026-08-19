package mcp

import (
	"context"
	"strings"

	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func (s *Service) EnsureBuiltinDefaults(ctx context.Context) (string, error) {
	serverDefaults, err := workorchdefaults.BuiltinMCPServerDefaults()
	if err != nil {
		return "", err
	}
	capabilityDefaults, err := workorchdefaults.BuiltinMCPCapabilityDefaults()
	if err != nil {
		return "", err
	}
	snapshot, err := s.mutate(ctx, func(next *mcpmodel.RegistrySnapshot) error {
		createdServerIDs := make(map[common.MCPServerID]struct{}, len(serverDefaults))
		for _, item := range serverDefaults {
			id := common.MCPServerID(strings.TrimSpace(item.ID))
			if findServer(next.Servers, id) != nil {
				continue
			}
			server, err := serverFromCommand(mcpevents.CreateServerCommand{
				ServerID: id, Name: item.Name, Transport: item.Transport, Endpoint: item.Endpoint, Command: item.Command,
				Args: append([]string(nil), item.Args...), EnvironmentRefs: mcpevents.EnvironmentReferencesFromMap(item.EnvironmentRefs), Enabled: item.Enabled,
				AllowedTools: append([]string(nil), item.AllowedTools...), AllowedResources: append([]string(nil), item.AllowedResources...),
				AllowedPrompts: append([]string(nil), item.AllowedPrompts...), DefaultApprovalPolicy: item.DefaultApprovalPolicy,
				SessionPolicy: item.SessionPolicy, TimeoutMillis: item.TimeoutMillis,
			})
			if err != nil {
				return err
			}
			next.Servers = append(next.Servers, server)
			createdServerIDs[id] = struct{}{}
		}
		for _, item := range capabilityDefaults {
			id := common.MCPCapabilityID(strings.TrimSpace(item.ID))
			serverID := common.MCPServerID(strings.TrimSpace(item.ServerID))
			if findCapability(next.Capabilities, id) != nil || findCapabilityByBinding(next.Capabilities, serverID, strings.TrimSpace(item.Kind), strings.TrimSpace(item.Name)) != nil {
				continue
			}
			server := findServer(next.Servers, serverID)
			if server == nil || !serverAllowsCapability(server, strings.TrimSpace(item.Kind), strings.TrimSpace(item.Name)) || (item.Enabled && !server.Enabled) {
				if server != nil {
					if _, created := createdServerIDs[serverID]; !created {
						continue
					}
				}
				return common.ErrInvalidArgument
			}
			schema, err := workorcheventpayload.FromMap(item.Schema)
			if err != nil {
				return err
			}
			capability, err := capabilityFromCommand(mcpevents.CreateCapabilityCommand{
				CapabilityID: id, ServerID: server.ID, Kind: item.Kind, Name: item.Name, DisplayName: item.DisplayName,
				Description: item.Description, Category: item.Category, Tags: append([]string(nil), item.Tags...), UsageHint: item.UsageHint,
				InputSummary: item.InputSummary, OutputSummary: item.OutputSummary, StageHints: append([]string(nil), item.StageHints...),
				RiskLevel: item.RiskLevel, EffectScope: item.EffectScope, IdempotencyClass: item.IdempotencyClass,
				ValidationProofApplicability: item.ValidationProofApplicability, Schema: schema, URI: item.URI,
				Enabled: item.Enabled, ApprovalPolicy: item.ApprovalPolicy,
			}, server, mcpmodel.CapabilitySourceBuiltin, string(id))
			if err != nil {
				return err
			}
			next.Capabilities = append(next.Capabilities, capability)
		}
		for _, server := range next.Servers {
			if err := reconcileServerCapabilities(next, server); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return registryContentRevision(snapshot), nil
}

func findCapabilityByBinding(items []*mcpmodel.Capability, serverID common.MCPServerID, kind, name string) *mcpmodel.Capability {
	wanted := capabilityBindingKey(serverID, kind, name)
	for _, item := range items {
		if item != nil && capabilityBindingKey(item.ServerID, item.Kind, item.Name) == wanted {
			return item
		}
	}
	return nil
}
