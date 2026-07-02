package system

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestSystemModuleConfigureRequiresCoreRepositories(t *testing.T) {
	base := systemModuleContractDependencies()
	tests := []struct {
		name string
		deps Dependencies
	}{
		{name: "missing souls", deps: func() Dependencies { d := base; d.Souls = nil; return d }()},
		{name: "missing templates", deps: func() Dependencies { d := base; d.Templates = nil; return d }()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}

func TestSystemModuleIdentityStartStopContract(t *testing.T) {
	ctx := context.Background()
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Start(ctx); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(systemModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.deps.Souls == nil || module.deps.Templates == nil {
		t.Fatalf("module not fully configured: %+v", module.deps)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.deps != (Dependencies{}) {
		t.Fatalf("deps after Stop = %+v, want cleared", module.deps)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestSystemModuleStartEnsuresBuiltinDefaults(t *testing.T) {
	ctx := context.Background()
	souls := memoryrepo.NewSoulRepository()
	templates := memoryrepo.NewWorkspaceTemplateRepository()
	mcpServers := memoryrepo.NewMCPServerRepository()
	mcpCapabilities := memoryrepo.NewMCPCapabilityRepository()

	module := New()
	if err := module.Configure(Dependencies{
		Souls:           souls,
		Templates:       templates,
		MCPServers:      mcpServers,
		MCPCapabilities: mcpCapabilities,
	}); err != nil {
		t.Fatalf("Configure error=%v", err)
	}
	if err := module.Start(ctx); err != nil {
		t.Fatalf("Start error=%v", err)
	}
	if got, err := souls.List(ctx); err != nil || len(got) == 0 {
		t.Fatalf("souls after Start len=%d err=%v", len(got), err)
	}
	if got, err := templates.List(ctx); err != nil || len(got) == 0 {
		t.Fatalf("templates after Start len=%d err=%v", len(got), err)
	}
}

func systemModuleContractDependencies() Dependencies {
	return Dependencies{
		Souls:           memoryrepo.NewSoulRepository(),
		Templates:       memoryrepo.NewWorkspaceTemplateRepository(),
		MCPServers:      memoryrepo.NewMCPServerRepository(),
		MCPCapabilities: memoryrepo.NewMCPCapabilityRepository(),
	}
}
