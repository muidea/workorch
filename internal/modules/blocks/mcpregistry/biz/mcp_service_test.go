package mcp

import (
	"context"
	"errors"
	"testing"

	mcpservice "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestServiceCreateAndListServer(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMCPServerRepository(), backend.NewMCPCapabilityRepository())

	cmd := mcpservice.CreateServerCommand{
		ServerID:              common.MCPServerID("mcp-local-context"),
		Name:                  "local context",
		Transport:             "stdio",
		Endpoint:              "repo-context",
		Enabled:               true,
		AllowedTools:          []string{"repo.summary"},
		AllowedResources:      []string{"repo://summary"},
		AllowedPrompts:        []string{"delivery.plan"},
		DefaultApprovalPolicy: "manual_for_write",
		SessionPolicy:         "long_lived",
		TimeoutMillis:         30000,
	}
	if err := service.CreateServer(ctx, cmd); err != nil {
		t.Fatalf("create mcp server: %v", err)
	}
	got, err := service.GetServer(ctx, cmd.ServerID)
	if err != nil {
		t.Fatalf("get mcp server: %v", err)
	}
	if got.Name != cmd.Name ||
		got.AllowedTools[0] != "repo.summary" ||
		got.AllowedPrompts[0] != "delivery.plan" ||
		got.SessionPolicy != "long_lived" {
		t.Fatalf("mcp server = %+v, want command values", got)
	}
	list, err := service.ListServers(ctx)
	if err != nil {
		t.Fatalf("list mcp servers: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("mcp servers len = %d, want 1", len(list))
	}
	capabilities, err := service.ListCapabilities(ctx, mcpservice.CapabilityQuery{ServerID: cmd.ServerID})
	if err != nil {
		t.Fatalf("list mcp capabilities: %v", err)
	}
	if len(capabilities) != 3 {
		t.Fatalf("mcp capabilities len = %d, want 3", len(capabilities))
	}
	if capabilities[0].DisplayName == "" || !capabilities[0].Enabled {
		t.Fatalf("default capability = %+v, want display name and enabled", capabilities[0])
	}
	var toolCapabilityDisplayName string
	for _, capability := range capabilities {
		if capability.Kind == mcpmodel.CapabilityKindTool && capability.Name == "repo.summary" {
			toolCapabilityDisplayName = capability.DisplayName
			break
		}
	}
	if toolCapabilityDisplayName != "工具 repo.summary" {
		t.Fatalf("tool capability display name = %q, want generic display name", toolCapabilityDisplayName)
	}
}

func TestServiceKeepsGeneratedCapabilityMetadataDomainNeutral(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMCPServerRepository(), backend.NewMCPCapabilityRepository())

	serverID := common.MCPServerID("mcp-acceptance")
	if err := service.CreateServer(ctx, mcpservice.CreateServerCommand{
		ServerID:         serverID,
		Name:             "verification server",
		Transport:        "stdio",
		Enabled:          true,
		AllowedResources: []string{"workspace://verification-context"},
		AllowedPrompts:   []string{"workorch.verification-context"},
	}); err != nil {
		t.Fatalf("create server: %v", err)
	}

	resources, err := service.ListCapabilities(ctx, mcpservice.CapabilityQuery{ServerID: serverID, Kind: mcpmodel.CapabilityKindResource})
	if err != nil {
		t.Fatalf("list resource capabilities: %v", err)
	}
	if len(resources) != 1 || resources[0].DisplayName != "资源 workspace://verification-context" || resources[0].URI != "workspace://verification-context" || resources[0].Category != "" || len(resources[0].Tags) != 0 {
		t.Fatalf("resource capabilities = %+v, want generic owner metadata", resources)
	}

	prompts, err := service.ListCapabilities(ctx, mcpservice.CapabilityQuery{ServerID: serverID, Kind: mcpmodel.CapabilityKindPrompt})
	if err != nil {
		t.Fatalf("list prompt capabilities: %v", err)
	}
	if len(prompts) != 1 || prompts[0].DisplayName != "提示词 workorch.verification-context" || prompts[0].Category != "" || len(prompts[0].Tags) != 0 {
		t.Fatalf("prompt capabilities = %+v, want generic owner metadata", prompts)
	}
}

func TestServiceRejectsInvalidServer(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMCPServerRepository(), backend.NewMCPCapabilityRepository())

	err := service.CreateServer(ctx, mcpservice.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-invalid"),
		Name:          "invalid",
		Transport:     "stdio",
		AllowedTools:  []string{""},
		TimeoutMillis: -1,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create invalid mcp server err = %v, want invalid argument", err)
	}

	err = service.CreateServer(ctx, mcpservice.CreateServerCommand{
		ServerID:  common.MCPServerID("mcp-blank-name"),
		Name:      "   ",
		Transport: "stdio",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create blank mcp server name err = %v, want invalid argument", err)
	}

	err = service.CreateServer(ctx, mcpservice.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-invalid-session"),
		Name:          "invalid session",
		Transport:     "stdio",
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		SessionPolicy: "sticky",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create invalid mcp server session err = %v, want invalid argument", err)
	}
}

func TestServiceCreateAndFilterCapability(t *testing.T) {
	ctx := context.Background()
	service := NewService(backend.NewMCPServerRepository(), backend.NewMCPCapabilityRepository())
	if err := service.CreateServer(ctx, mcpservice.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-capability"),
		Name:          "capability server",
		Transport:     "stdio",
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		SessionPolicy: "transient",
	}); err != nil {
		t.Fatalf("create server: %v", err)
	}
	if _, err := service.CreateCapability(ctx, mcpservice.CreateCapabilityCommand{
		ServerID:                     common.MCPServerID("mcp-capability"),
		Kind:                         "tool",
		Name:                         "invalid-proof-applicability",
		ValidationProofApplicability: "inferred_from_name",
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create capability with invalid proof applicability err = %v, want invalid argument", err)
	}
	created, err := service.CreateCapability(ctx, mcpservice.CreateCapabilityCommand{
		ServerID:                     common.MCPServerID("mcp-capability"),
		Kind:                         "resource",
		Name:                         "workspace://acceptance",
		DisplayName:                  "工作空间验收资料",
		Category:                     "验收",
		StageHints:                   []string{"validation", "acceptance"},
		RiskLevel:                    "read_only",
		EffectScope:                  "none",
		IdempotencyClass:             "safe_to_replay",
		ValidationProofApplicability: "auxiliary",
		Enabled:                      true,
		OutputSummary:                "返回 DoD、Result、Artifact 和证据引用。",
	})
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}
	if created.ID == "" || created.URI != "" || created.EffectScope != "none" || created.IdempotencyClass != "safe_to_replay" || created.ValidationProofApplicability != "auxiliary" {
		t.Fatalf("created capability = %+v, want generated id, explicit runtime metadata, and uri only when provided", created)
	}
	if created.Status != "draft" {
		t.Fatalf("created status = %q, want owner-derived draft", created.Status)
	}
	update := mcpservice.UpdateCapabilityCommand(mcpservice.CreateCapabilityCommand{
		CapabilityID: created.ID, ServerID: created.ServerID, Kind: created.Kind, Name: created.Name,
		DisplayName: created.DisplayName, Category: created.Category, StageHints: created.StageHints,
		RiskLevel: created.RiskLevel, EffectScope: created.EffectScope, IdempotencyClass: created.IdempotencyClass, ValidationProofApplicability: created.ValidationProofApplicability,
		Enabled: created.Enabled, OutputSummary: created.OutputSummary,
	})
	updated, err := service.UpdateCapability(ctx, update)
	if err != nil {
		t.Fatalf("update capability: %v", err)
	}
	if updated.Status != "draft" {
		t.Fatalf("updated status = %q, want owner state preserved", updated.Status)
	}
	list, err := service.ListCapabilities(ctx, mcpservice.CapabilityQuery{
		ServerID: common.MCPServerID("mcp-capability"),
		Stage:    "acceptance",
		Query:    "验收",
	})
	if err != nil {
		t.Fatalf("list capabilities: %v", err)
	}
	if len(list) != 1 || list[0].DisplayName != "工作空间验收资料" {
		t.Fatalf("filtered capabilities = %+v, want acceptance capability", list)
	}
}
