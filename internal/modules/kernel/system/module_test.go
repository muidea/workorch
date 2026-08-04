package system

import (
	"context"
	"errors"
	"testing"

	mcpregistrybiz "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/biz"
	soulbiz "github.com/muidea/workorch/internal/modules/blocks/soul/biz"
	workspacebiz "github.com/muidea/workorch/internal/modules/blocks/workspace/biz"
	systembiz "github.com/muidea/workorch/internal/modules/kernel/system/biz"

	magicevent "github.com/muidea/magicCommon/event"
	mcpmodule "github.com/muidea/workorch/internal/modules/blocks/mcpregistry"
	soulmodule "github.com/muidea/workorch/internal/modules/blocks/soul"
	workspacemodule "github.com/muidea/workorch/internal/modules/blocks/workspace"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestSystemModuleConfigureRequiresEventHub(t *testing.T) {
	if err := New().Configure(systembiz.Dependencies{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Configure err = %v, want invalid argument", err)
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
	deps := systemModuleContractDependencies()
	if err := module.Configure(deps); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil || module.bizPtr.EventHub() == nil {
		t.Fatal("system biz is not configured after Configure")
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatal("system biz was not cleared after Stop")
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	deps.EventHub.Terminate(ctx)
}

func TestSystemModuleStartEnsuresBuiltinDefaultsThroughEventHubConsumers(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(64)
	defer hub.Terminate(ctx)
	souls := memoryrepo.NewSoulRepository()
	templates := memoryrepo.NewWorkspaceTemplateRepository()
	mcpServers := memoryrepo.NewMCPServerRepository()
	mcpCapabilities := memoryrepo.NewMCPCapabilityRepository()

	soulOwner := soulmodule.New()
	if err := soulOwner.Configure(soulbiz.Dependencies{Souls: souls, EventHub: hub}); err != nil {
		t.Fatalf("configure soul owner: %v", err)
	}
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspacebiz.Dependencies{
		Workspaces: memoryrepo.NewWorkspaceRepository(),
		Goals:      memoryrepo.NewGoalRepository(),
		DoDs:       memoryrepo.NewDoDRepository(),
		Templates:  templates,
		EventHub:   hub,
	}); err != nil {
		t.Fatalf("configure workspace owner: %v", err)
	}
	mcpOwner := mcpmodule.New()
	if err := mcpOwner.Configure(mcpregistrybiz.Dependencies{Servers: mcpServers, Capabilities: mcpCapabilities, EventHub: hub}); err != nil {
		t.Fatalf("configure MCP owner: %v", err)
	}
	for name, start := range map[string]func() error{
		"soul":      soulOwner.Start,
		"workspace": workspaceOwner.Start,
		"mcp":       mcpOwner.Start,
	} {
		if err := start(); err != nil {
			t.Fatalf("start %s owner: %v", name, err)
		}
	}
	t.Cleanup(func() {
		_ = mcpOwner.Stop(ctx)
		_ = workspaceOwner.Stop(ctx)
		_ = soulOwner.Stop(ctx)
	})

	module := New()
	if err := module.Configure(systembiz.Dependencies{EventHub: hub}); err != nil {
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
	if got, err := mcpServers.List(ctx); err != nil || len(got) == 0 {
		t.Fatalf("MCP servers after Start len=%d err=%v", len(got), err)
	}
	if got, err := mcpCapabilities.List(ctx); err != nil || len(got) == 0 {
		t.Fatalf("MCP capabilities after Start len=%d err=%v", len(got), err)
	}
}

func systemModuleContractDependencies() systembiz.Dependencies {
	return systembiz.Dependencies{EventHub: magicevent.NewHub(16)}
}
