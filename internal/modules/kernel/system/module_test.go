package system

import (
	"context"
	"testing"

	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestSystemModuleConfigureRequiresCoreRepositories(t *testing.T) {
	module := New()
	if err := module.Configure(Dependencies{}); err == nil {
		t.Fatalf("expected error for missing repositories")
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
