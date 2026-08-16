package eventmapping

import (
	"fmt"
	"sort"
	"strings"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func MCPServerMutationDTO(item workorchmanagementcontract.MCPServerMutation) workorchdevents.CLIMCPServerMutation {
	return workorchdevents.CLIMCPServerMutation{
		ID: item.ID, Name: item.Name, Transport: item.Transport, Endpoint: item.Endpoint,
		Command: item.Command, Args: append([]string(nil), item.Args...), Env: stringEntries(item.Env), Enabled: item.Enabled,
		AllowedTools: append([]string(nil), item.AllowedTools...), AllowedResources: append([]string(nil), item.AllowedResources...),
		AllowedPrompts: append([]string(nil), item.AllowedPrompts...), DefaultApprovalPolicy: item.DefaultApprovalPolicy,
		SessionPolicy: item.SessionPolicy, TimeoutMillis: item.TimeoutMillis,
	}
}

func MCPServerMutation(item workorchdevents.CLIMCPServerMutation) (workorchmanagementcontract.MCPServerMutation, error) {
	env, err := stringMap(item.Env)
	if err != nil {
		return workorchmanagementcontract.MCPServerMutation{}, err
	}
	return workorchmanagementcontract.MCPServerMutation{
		ID: item.ID, Name: item.Name, Transport: item.Transport, Endpoint: item.Endpoint,
		Command: item.Command, Args: append([]string(nil), item.Args...), Env: env, Enabled: item.Enabled,
		AllowedTools: append([]string(nil), item.AllowedTools...), AllowedResources: append([]string(nil), item.AllowedResources...),
		AllowedPrompts: append([]string(nil), item.AllowedPrompts...), DefaultApprovalPolicy: item.DefaultApprovalPolicy,
		SessionPolicy: item.SessionPolicy, TimeoutMillis: item.TimeoutMillis,
	}, nil
}

func MCPServerDTO(item workorchmanagementcontract.MCPServer) workorchdevents.CLIMCPServer {
	return workorchdevents.CLIMCPServer(MCPServerMutationDTO(workorchmanagementcontract.MCPServerMutation(item)))
}

func MCPServerDTOs(items []workorchmanagementcontract.MCPServer) []workorchdevents.CLIMCPServer {
	if items == nil {
		return nil
	}
	ret := make([]workorchdevents.CLIMCPServer, 0, len(items))
	for _, item := range items {
		ret = append(ret, MCPServerDTO(item))
	}
	return ret
}

func MCPServerManagement(item workorchdevents.CLIMCPServer) (workorchmanagementcontract.MCPServer, error) {
	mutation, err := MCPServerMutation(workorchdevents.CLIMCPServerMutation(item))
	if err != nil {
		return workorchmanagementcontract.MCPServer{}, err
	}
	return workorchmanagementcontract.MCPServer(mutation), nil
}

func MCPServerManagements(items []workorchdevents.CLIMCPServer) ([]workorchmanagementcontract.MCPServer, error) {
	if items == nil {
		return nil, nil
	}
	ret := make([]workorchmanagementcontract.MCPServer, 0, len(items))
	for _, item := range items {
		server, err := MCPServerManagement(item)
		if err != nil {
			return nil, err
		}
		ret = append(ret, server)
	}
	return ret, nil
}

func MCPCapabilityDTO(item workorchmanagementcontract.MCPCapability) (workorchdevents.CLIMCPCapability, error) {
	schema, err := workorcheventpayload.FromMap(item.Schema)
	if err != nil {
		return workorchdevents.CLIMCPCapability{}, err
	}
	return workorchdevents.CLIMCPCapability{
		ID: item.ID, ServerID: item.ServerID, Kind: item.Kind, Name: item.Name, DisplayName: item.DisplayName,
		Description: item.Description, Category: item.Category, Tags: append([]string(nil), item.Tags...), UsageHint: item.UsageHint,
		InputSummary: item.InputSummary, OutputSummary: item.OutputSummary, StageHints: append([]string(nil), item.StageHints...),
		RiskLevel: item.RiskLevel, EffectScope: item.EffectScope, IdempotencyClass: item.IdempotencyClass, ValidationProofApplicability: item.ValidationProofApplicability,
		Schema: schema, URI: item.URI, Enabled: item.Enabled, ApprovalPolicy: item.ApprovalPolicy, Status: item.Status,
	}, nil
}

func MCPCapabilityDTOs(items []workorchmanagementcontract.MCPCapability) ([]workorchdevents.CLIMCPCapability, error) {
	if items == nil {
		return nil, nil
	}
	ret := make([]workorchdevents.CLIMCPCapability, 0, len(items))
	for _, item := range items {
		dto, err := MCPCapabilityDTO(item)
		if err != nil {
			return nil, err
		}
		ret = append(ret, dto)
	}
	return ret, nil
}

func MCPCapabilityManagement(item workorchdevents.CLIMCPCapability) workorchmanagementcontract.MCPCapability {
	return workorchmanagementcontract.MCPCapability{
		ID: item.ID, ServerID: item.ServerID, Kind: item.Kind, Name: item.Name, DisplayName: item.DisplayName,
		Description: item.Description, Category: item.Category, Tags: append([]string(nil), item.Tags...), UsageHint: item.UsageHint,
		InputSummary: item.InputSummary, OutputSummary: item.OutputSummary, StageHints: append([]string(nil), item.StageHints...),
		RiskLevel: item.RiskLevel, EffectScope: item.EffectScope, IdempotencyClass: item.IdempotencyClass, ValidationProofApplicability: item.ValidationProofApplicability,
		Schema: item.Schema.Map(), URI: item.URI, Enabled: item.Enabled, ApprovalPolicy: item.ApprovalPolicy, Status: item.Status,
	}
}

func MCPCapabilityManagements(items []workorchdevents.CLIMCPCapability) []workorchmanagementcontract.MCPCapability {
	if items == nil {
		return nil
	}
	ret := make([]workorchmanagementcontract.MCPCapability, 0, len(items))
	for _, item := range items {
		ret = append(ret, MCPCapabilityManagement(item))
	}
	return ret
}

func stringEntries(input map[string]string) []workorchdevents.CLIStringEntry {
	if input == nil {
		return nil
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]workorchdevents.CLIStringEntry, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, workorchdevents.CLIStringEntry{Key: key, Value: input[key]})
	}
	return ret
}

func stringMap(input []workorchdevents.CLIStringEntry) (map[string]string, error) {
	if input == nil {
		return nil, nil
	}
	ret := make(map[string]string, len(input))
	for _, entry := range input {
		key := strings.TrimSpace(entry.Key)
		if key == "" {
			return nil, fmt.Errorf("%w: MCP environment key is required", common.ErrInvalidArgument)
		}
		if _, exists := ret[key]; exists {
			return nil, fmt.Errorf("%w: duplicate MCP environment key %q", common.ErrInvalidArgument, key)
		}
		ret[key] = entry.Value
	}
	return ret, nil
}
