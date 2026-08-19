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

func TestEnsureBuiltinMCPDefaultsIsAtomicAndIdempotent(t *testing.T) {
	testCases := []struct {
		name string
		new  func(*testing.T) repository.MCPRegistryRepository
	}{
		{name: "memory", new: func(*testing.T) repository.MCPRegistryRepository { return memoryrepo.NewMCPRegistryRepository() }},
		{name: "file", new: func(t *testing.T) repository.MCPRegistryRepository {
			return filerepo.NewMCPRegistryRepository(filepath.Join(t.TempDir(), "mcp_registry.json"))
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			registry := testCase.new(t)
			service := NewService(registry)
			firstRevision, err := service.EnsureBuiltinDefaults(ctx)
			if err != nil {
				t.Fatalf("EnsureBuiltinDefaults: %v", err)
			}
			first, err := registry.Load(ctx)
			if err != nil || len(first.Servers) == 0 || len(first.Capabilities) == 0 {
				t.Fatalf("first defaults = %+v, err=%v", first, err)
			}
			secondRevision, err := service.EnsureBuiltinDefaults(ctx)
			if err != nil {
				t.Fatalf("second EnsureBuiltinDefaults: %v", err)
			}
			second, _ := registry.Load(ctx)
			if firstRevision != secondRevision || !reflect.DeepEqual(first, second) {
				t.Fatalf("defaults not idempotent: first=%q/%+v second=%q/%+v", firstRevision, first, secondRevision, second)
			}
			for _, capability := range second.Capabilities {
				if capability.Source == "" || capability.SourceRef == "" || capability.Revision == 0 {
					t.Fatalf("default capability missing provenance: %+v", capability)
				}
			}
		})
	}
}

func TestEnsureBuiltinMCPDefaultsPreservesCustomizedBuiltinID(t *testing.T) {
	ctx := context.Background()
	serverDefaults, err := workorchdefaults.BuiltinMCPServerDefaults()
	if err != nil || len(serverDefaults) == 0 {
		t.Fatalf("BuiltinMCPServerDefaults = %d, err=%v", len(serverDefaults), err)
	}
	capabilityDefaults, err := workorchdefaults.BuiltinMCPCapabilityDefaults()
	if err != nil || len(capabilityDefaults) == 0 {
		t.Fatalf("BuiltinMCPCapabilityDefaults = %d, err=%v", len(capabilityDefaults), err)
	}
	server := &mcpmodel.Server{
		ID: common.MCPServerID(serverDefaults[0].ID), Name: "user-owned MCP server", Transport: "stdio", Command: "custom-command",
		Args: []string{"--custom"}, Env: map[string]string{"CUSTOM": "env://CUSTOM"}, Enabled: false,
		AllowedTools: []string{"custom.tool"}, TimeoutMillis: 1234,
	}
	capability := &mcpmodel.Capability{
		ID: common.MCPCapabilityID(capabilityDefaults[0].ID), ServerID: server.ID, Kind: mcpmodel.CapabilityKindTool,
		Name: "custom.tool", DisplayName: "user-owned capability", Description: "preserve customized built-in ID",
		Tags: []string{"custom"}, Enabled: false, EffectScope: "unknown", IdempotencyClass: "unknown",
		ValidationProofApplicability: mcpmodel.ValidationProofApplicabilityNotDeclared,
		Source:                       mcpmodel.CapabilitySourceUser, SourceRef: "user/custom", Revision: 7,
	}
	registry := memoryrepo.NewMCPRegistryRepository()
	seed := &mcpmodel.RegistrySnapshot{Revision: 1, Servers: []*mcpmodel.Server{server}, Capabilities: []*mcpmodel.Capability{capability}}
	if err := registry.Commit(ctx, 0, seed); err != nil {
		t.Fatalf("seed registry: %v", err)
	}
	service := NewService(registry)
	if _, err := service.EnsureBuiltinDefaults(ctx); err != nil {
		t.Fatalf("EnsureBuiltinDefaults: %v", err)
	}
	after, err := registry.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(after.Servers[0], server) || !reflect.DeepEqual(after.Capabilities[0], capability) {
		t.Fatalf("custom defaults overwritten: %+v", after)
	}
}
