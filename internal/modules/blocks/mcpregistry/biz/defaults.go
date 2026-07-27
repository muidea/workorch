package mcp

import (
	"context"
	"errors"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
)

func ensureBuiltinMCPDefaults(ctx context.Context, servers repository.MCPServerRepository, capabilities repository.MCPCapabilityRepository) error {
	if servers == nil || capabilities == nil {
		return common.ErrInvalidArgument
	}
	serverDefaults, err := workorchdefaults.BuiltinMCPServerDefaults()
	if err != nil {
		return err
	}
	for _, item := range serverDefaults {
		value := &mcpmodel.Server{ID: item.ID, Name: item.Name, Transport: item.Transport, Endpoint: item.Endpoint, Command: item.Command, Args: append([]string(nil), item.Args...), Env: cloneStringMap(item.Env), Enabled: item.Enabled, AllowedTools: append([]string(nil), item.AllowedTools...), AllowedResources: append([]string(nil), item.AllowedResources...), AllowedPrompts: append([]string(nil), item.AllowedPrompts...), DefaultApprovalPolicy: item.DefaultApprovalPolicy, SessionPolicy: item.SessionPolicy, TimeoutMillis: item.TimeoutMillis}
		if err := servers.Update(ctx, value); err != nil {
			if !errors.Is(err, common.ErrNotFound) {
				return err
			}
			if err := servers.Create(ctx, value); err != nil {
				return err
			}
		}
	}
	capabilityDefaults, err := workorchdefaults.BuiltinMCPCapabilityDefaults()
	if err != nil {
		return err
	}
	for _, item := range capabilityDefaults {
		value := &mcpmodel.Capability{ID: item.ID, ServerID: item.ServerID, Kind: item.Kind, Name: item.Name, DisplayName: item.DisplayName, Description: item.Description, Category: item.Category, Tags: append([]string(nil), item.Tags...), UsageHint: item.UsageHint, InputSummary: item.InputSummary, OutputSummary: item.OutputSummary, StageHints: append([]string(nil), item.StageHints...), RiskLevel: item.RiskLevel, Schema: item.Schema.Clone(), URI: item.URI, Enabled: item.Enabled, ApprovalPolicy: item.ApprovalPolicy, Status: item.Status}
		if err := capabilities.Update(ctx, value); err != nil {
			if !errors.Is(err, common.ErrNotFound) {
				return err
			}
			if err := capabilities.Create(ctx, value); err != nil {
				return err
			}
		}
	}
	return nil
}
