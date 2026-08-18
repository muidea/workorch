package mcp

import (
	"context"
	"errors"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
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
		id := common.MCPServerID(item.ID)
		if _, err := servers.Get(ctx, id); err == nil {
			continue
		} else if !errors.Is(err, common.ErrNotFound) {
			return err
		}
		value := &mcpmodel.Server{ID: common.MCPServerID(item.ID), Name: item.Name, Transport: item.Transport, Endpoint: item.Endpoint, Command: item.Command, Args: append([]string(nil), item.Args...), Env: cloneStringMap(item.Env), Enabled: item.Enabled, AllowedTools: append([]string(nil), item.AllowedTools...), AllowedResources: append([]string(nil), item.AllowedResources...), AllowedPrompts: append([]string(nil), item.AllowedPrompts...), DefaultApprovalPolicy: item.DefaultApprovalPolicy, SessionPolicy: item.SessionPolicy, TimeoutMillis: item.TimeoutMillis}
		if err := servers.Create(ctx, value); err != nil {
			if !errors.Is(err, common.ErrConflict) {
				return err
			}
			if _, getErr := servers.Get(ctx, id); getErr != nil {
				return getErr
			}
		}
	}
	capabilityDefaults, err := workorchdefaults.BuiltinMCPCapabilityDefaults()
	if err != nil {
		return err
	}
	for _, item := range capabilityDefaults {
		id := common.MCPCapabilityID(item.ID)
		if _, err := capabilities.Get(ctx, id); err == nil {
			continue
		} else if !errors.Is(err, common.ErrNotFound) {
			return err
		}
		schema, err := workorcheventpayload.FromMap(item.Schema)
		if err != nil {
			return err
		}
		value := &mcpmodel.Capability{ID: common.MCPCapabilityID(item.ID), ServerID: common.MCPServerID(item.ServerID), Kind: item.Kind, Name: item.Name, DisplayName: item.DisplayName, Description: item.Description, Category: item.Category, Tags: append([]string(nil), item.Tags...), UsageHint: item.UsageHint, InputSummary: item.InputSummary, OutputSummary: item.OutputSummary, StageHints: append([]string(nil), item.StageHints...), RiskLevel: item.RiskLevel, EffectScope: item.EffectScope, IdempotencyClass: item.IdempotencyClass, ValidationProofApplicability: item.ValidationProofApplicability, Schema: schema, URI: item.URI, Enabled: item.Enabled, ApprovalPolicy: item.ApprovalPolicy, Status: item.Status}
		if err := capabilities.Create(ctx, value); err != nil {
			if !errors.Is(err, common.ErrConflict) {
				return err
			}
			if _, getErr := capabilities.Get(ctx, id); getErr != nil {
				return getErr
			}
		}
	}
	return nil
}
