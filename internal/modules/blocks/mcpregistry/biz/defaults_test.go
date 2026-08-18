package mcp

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
)

func TestEnsureBuiltinMCPDefaultsSeedsMissingAndPreservesExisting(t *testing.T) {
	testCases := []struct {
		name string
		new  func(*testing.T) (repository.MCPServerRepository, repository.MCPCapabilityRepository)
	}{
		{name: "memory", new: func(*testing.T) (repository.MCPServerRepository, repository.MCPCapabilityRepository) {
			return memoryrepo.NewMCPServerRepository(), memoryrepo.NewMCPCapabilityRepository()
		}},
		{name: "file", new: func(t *testing.T) (repository.MCPServerRepository, repository.MCPCapabilityRepository) {
			dir := t.TempDir()
			return filerepo.NewMCPServerRepository(filepath.Join(dir, "mcp_servers.json")), filerepo.NewMCPCapabilityRepository(filepath.Join(dir, "mcp_capabilities.json"))
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			servers, capabilities := testCase.new(t)
			serverDefaults, err := workorchdefaults.BuiltinMCPServerDefaults()
			if err != nil || len(serverDefaults) == 0 {
				t.Fatalf("load MCP server defaults len=%d err=%v", len(serverDefaults), err)
			}
			capabilityDefaults, err := workorchdefaults.BuiltinMCPCapabilityDefaults()
			if err != nil || len(capabilityDefaults) == 0 {
				t.Fatalf("load MCP capability defaults len=%d err=%v", len(capabilityDefaults), err)
			}

			existingServer := &mcpmodel.Server{
				ID:            common.MCPServerID(serverDefaults[0].ID),
				Name:          "user-owned MCP server",
				Transport:     "stdio",
				Command:       "custom-command",
				Args:          []string{"--custom"},
				Env:           map[string]string{"CUSTOM": "true"},
				Enabled:       false,
				AllowedTools:  []string{"custom.tool"},
				TimeoutMillis: 1234,
			}
			existingCapability := &mcpmodel.Capability{
				ID:          common.MCPCapabilityID(capabilityDefaults[0].ID),
				ServerID:    common.MCPServerID(capabilityDefaults[0].ServerID),
				Kind:        mcpmodel.CapabilityKindTool,
				Name:        "custom.tool",
				DisplayName: "user-owned capability",
				Description: "preserve this customized built-in ID",
				Tags:        []string{"custom"},
				Enabled:     false,
				Status:      "custom-status",
			}
			if err := servers.Create(ctx, existingServer); err != nil {
				t.Fatalf("create customized MCP server: %v", err)
			}
			if err := capabilities.Create(ctx, existingCapability); err != nil {
				t.Fatalf("create customized MCP capability: %v", err)
			}
			existingServer, err = servers.Get(ctx, existingServer.ID)
			if err != nil {
				t.Fatalf("read persisted customized MCP server: %v", err)
			}
			existingCapability, err = capabilities.Get(ctx, existingCapability.ID)
			if err != nil {
				t.Fatalf("read persisted customized MCP capability: %v", err)
			}

			for attempt := 1; attempt <= 2; attempt++ {
				if err := ensureBuiltinMCPDefaults(ctx, servers, capabilities); err != nil {
					t.Fatalf("ensure attempt %d: %v", attempt, err)
				}
				gotServer, err := servers.Get(ctx, existingServer.ID)
				if err != nil {
					t.Fatalf("get customized MCP server after attempt %d: %v", attempt, err)
				}
				if !reflect.DeepEqual(gotServer, existingServer) {
					t.Fatalf("customized MCP server after attempt %d = %+v, want %+v", attempt, gotServer, existingServer)
				}
				gotCapability, err := capabilities.Get(ctx, existingCapability.ID)
				if err != nil {
					t.Fatalf("get customized MCP capability after attempt %d: %v", attempt, err)
				}
				if !reflect.DeepEqual(gotCapability, existingCapability) {
					t.Fatalf("customized MCP capability after attempt %d = %+v, want %+v", attempt, gotCapability, existingCapability)
				}
			}
			gotServers, serverErr := servers.List(ctx)
			gotCapabilities, capabilityErr := capabilities.List(ctx)
			if serverErr != nil || len(gotServers) != len(serverDefaults) {
				t.Fatalf("seeded MCP servers len=%d want=%d err=%v", len(gotServers), len(serverDefaults), serverErr)
			}
			if capabilityErr != nil || len(gotCapabilities) != len(capabilityDefaults) {
				t.Fatalf("seeded MCP capabilities len=%d want=%d err=%v", len(gotCapabilities), len(capabilityDefaults), capabilityErr)
			}
		})
	}
}
