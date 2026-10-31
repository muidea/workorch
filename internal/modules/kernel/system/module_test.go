package system

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	mcpregistrybiz "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/biz"
	soulbiz "github.com/muidea/workorch/internal/modules/blocks/soul/biz"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	systembiz "github.com/muidea/workorch/internal/modules/kernel/system/biz"
	workspacebiz "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"

	magicevent "github.com/muidea/magicCommon/event"
	mcpmodule "github.com/muidea/workorch/internal/modules/blocks/mcpregistry"
	soulmodule "github.com/muidea/workorch/internal/modules/blocks/soul"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
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
	t.Cleanup(func() { hub.Terminate(ctx) })
	souls := memoryrepo.NewSoulRepository()
	workspaceRepositories := memoryrepo.NewWorkspaceOwnerRepositorySet()
	templates := workspaceRepositories.OwnerRepositories().Templates()
	mcpRegistry := memoryrepo.NewMCPRegistryRepository()

	soulOwner := soulmodule.New()
	if err := soulOwner.Configure(soulbiz.Dependencies{Souls: souls, EventHub: hub}); err != nil {
		t.Fatalf("configure soul owner: %v", err)
	}
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspacebiz.Dependencies{
		OwnerRepositories: workspaceRepositories.OwnerRepositories(),
		EventHub:          hub,
	}); err != nil {
		t.Fatalf("configure workspace owner: %v", err)
	}
	mcpOwner := mcpmodule.New()
	if err := mcpOwner.Configure(mcpregistrybiz.Dependencies{Registry: mcpRegistry, EventHub: hub}); err != nil {
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
	if got, err := templates.List(ctx); err != nil || len(got) != 0 {
		t.Fatalf("templates after Start len=%d err=%v", len(got), err)
	}
	_ = readSystemTestTemplates(t, hub)
	if got, err := mcpRegistry.Load(ctx); err != nil || len(got.Servers) == 0 || len(got.Capabilities) == 0 {
		t.Fatalf("MCP registry after Start = %+v err=%v", got, err)
	}
}

func TestSystemModuleStartConvergesAfterPartialFailureWithoutOverwritingUserChanges(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(64)
	t.Cleanup(func() { hub.Terminate(ctx) })
	souls := memoryrepo.NewSoulRepository()
	workspaceRepositories := memoryrepo.NewWorkspaceOwnerRepositorySet()
	templates := workspaceRepositories.OwnerRepositories().Templates()
	mcpRegistry := memoryrepo.NewMCPRegistryRepository()

	soulOwner := soulmodule.New()
	if err := soulOwner.Configure(soulbiz.Dependencies{Souls: souls, EventHub: hub}); err != nil {
		t.Fatalf("configure soul owner: %v", err)
	}
	workspaceOwner := workspacemodule.New()
	if err := workspaceOwner.Configure(workspacebiz.Dependencies{
		OwnerRepositories: workspaceRepositories.OwnerRepositories(),
		EventHub:          hub,
	}); err != nil {
		t.Fatalf("configure workspace owner: %v", err)
	}
	mcpOwner := mcpmodule.New()
	if err := mcpOwner.Configure(mcpregistrybiz.Dependencies{Registry: mcpRegistry, EventHub: hub}); err != nil {
		t.Fatalf("configure MCP owner: %v", err)
	}
	if err := soulOwner.Start(); err != nil {
		t.Fatalf("start soul owner: %v", err)
	}
	if err := workspaceOwner.Start(); err != nil {
		t.Fatalf("start workspace owner: %v", err)
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
	t.Cleanup(func() { _ = module.Stop(ctx) })
	if err := module.Start(ctx); err == nil {
		t.Fatal("Start error=nil, want missing MCP subscriber to fail startup")
	}

	seededSouls, err := souls.List(ctx)
	if err != nil || len(seededSouls) == 0 {
		t.Fatalf("souls after partial Start len=%d err=%v", len(seededSouls), err)
	}
	seededTemplates := readSystemTestTemplates(t, hub)
	_, err = templates.List(ctx)
	if err != nil || len(seededTemplates) == 0 {
		t.Fatalf("templates after partial Start len=%d err=%v", len(seededTemplates), err)
	}
	if got, err := mcpRegistry.Load(ctx); err != nil || len(got.Servers) != 0 || len(got.Capabilities) != 0 {
		t.Fatalf("MCP registry after partial Start = %+v err=%v, want empty", got, err)
	}

	customSoul := seededSouls[0]
	customSoul.Name = "user-customized soul"
	customSoul.AllowedSkillRefs = []string{"user-customized-skill"}
	customSoul.SourceProvenance = soulmodel.SoulSourceUserManaged
	customSoul.ContentHash = soulmodel.StableSoulContentHash(customSoul)
	if err := souls.Update(ctx, customSoul); err != nil {
		t.Fatalf("customize seeded soul: %v", err)
	}
	customTemplate := &workspacemodel.WorkspaceTemplate{ID: "user-template", Name: "user-customized template"}
	customTemplate.DoDItems = []string{"user-customized acceptance"}
	if err := templates.Create(ctx, customTemplate); err != nil {
		t.Fatalf("customize seeded template: %v", err)
	}

	if err := mcpOwner.Start(); err != nil {
		t.Fatalf("start MCP owner before retry: %v", err)
	}
	if err := module.Start(ctx); err != nil {
		t.Fatalf("retry Start after MCP owner becomes available: %v", err)
	}
	gotSoul, err := souls.Get(ctx, customSoul.ID)
	if err != nil || gotSoul.Name != customSoul.Name || len(gotSoul.AllowedSkillRefs) != 1 || gotSoul.AllowedSkillRefs[0] != customSoul.AllowedSkillRefs[0] {
		t.Fatalf("soul after retry = %+v err=%v, want user customization preserved", gotSoul, err)
	}
	gotTemplate, err := templates.Get(ctx, customTemplate.ID)
	if err != nil || gotTemplate.Name != customTemplate.Name || len(gotTemplate.DoDItems) != 1 || gotTemplate.DoDItems[0] != customTemplate.DoDItems[0] {
		t.Fatalf("template after retry = %+v err=%v, want user customization preserved", gotTemplate, err)
	}

	registrySnapshot, err := mcpRegistry.Load(ctx)
	if err != nil || len(registrySnapshot.Servers) == 0 || len(registrySnapshot.Capabilities) == 0 {
		t.Fatalf("MCP registry after retry = %+v err=%v", registrySnapshot, err)
	}
	customServer := registrySnapshot.Servers[0]
	customServer.Name = "user-customized MCP server"
	customServer.Enabled = false
	customCapability := registrySnapshot.Capabilities[0]
	customCapability.DisplayName = "user-customized MCP capability"
	for _, capability := range registrySnapshot.Capabilities {
		if capability.ServerID == customServer.ID {
			capability.Enabled = false
		}
	}
	registrySnapshot.Revision++
	if err := mcpRegistry.Commit(ctx, registrySnapshot.Revision-1, registrySnapshot); err != nil {
		t.Fatalf("customize seeded MCP aggregate: %v", err)
	}

	if err := module.Start(ctx); err != nil {
		t.Fatalf("repeat Start: %v", err)
	}
	gotRegistry, err := mcpRegistry.Load(ctx)
	if err != nil || gotRegistry.Servers[0].Name != customServer.Name || gotRegistry.Servers[0].Enabled != customServer.Enabled ||
		gotRegistry.Capabilities[0].DisplayName != customCapability.DisplayName || gotRegistry.Capabilities[0].Enabled != customCapability.Enabled {
		t.Fatalf("MCP aggregate after repeat Start = %+v err=%v, want user customization preserved", gotRegistry, err)
	}
}

func TestSystemFreshProcessConvergesFileBackedPartialStartupWithoutOverwritingUserChanges(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	soulPath := filepath.Join(root, "souls")
	templatePath := filepath.Join(root, "system", "workspace_templates.json")
	mcpPath := filepath.Join(root, "mcp_registry.json")

	firstHub := magicevent.NewHub(64)
	firstSouls := filerepo.NewSoulRepository(soulPath)
	firstWorkspaceRepositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	firstSoulOwner := soulmodule.New()
	if err := firstSoulOwner.Configure(soulbiz.Dependencies{Souls: firstSouls, EventHub: firstHub}); err != nil {
		t.Fatal(err)
	}
	firstWorkspaceOwner := workspacemodule.New()
	if err := firstWorkspaceOwner.Configure(workspacebiz.Dependencies{
		OwnerRepositories: firstWorkspaceRepositories.WorkspaceOwner,
		EventHub:          firstHub,
	}); err != nil {
		t.Fatal(err)
	}
	if err := firstSoulOwner.Start(); err != nil {
		t.Fatal(err)
	}
	if err := firstWorkspaceOwner.Start(); err != nil {
		t.Fatal(err)
	}
	firstSystem := New()
	if err := firstSystem.Configure(systembiz.Dependencies{EventHub: firstHub}); err != nil {
		t.Fatal(err)
	}
	if err := firstSystem.Start(ctx); err == nil {
		t.Fatal("partial file-backed Start unexpectedly succeeded without MCP Registry owner")
	}
	if err := firstSystem.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := firstWorkspaceOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := firstSoulOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	firstHub.Terminate(ctx)

	freshSouls := filerepo.NewSoulRepository(soulPath)
	seededSouls, err := freshSouls.List(ctx)
	if err != nil || len(seededSouls) == 0 {
		t.Fatalf("fresh Soul repository after partial Start len=%d err=%v", len(seededSouls), err)
	}
	customSoul := seededSouls[0]
	customSoul.Name = "fresh-process customized soul"
	customSoul.SourceProvenance = soulmodel.SoulSourceUserManaged
	customSoul.ContentHash = soulmodel.StableSoulContentHash(customSoul)
	if err := freshSouls.Update(ctx, customSoul); err != nil {
		t.Fatal(err)
	}
	freshTemplates := filerepo.NewWorkspaceTemplateRepository(templatePath)
	seededTemplates, err := freshTemplates.List(ctx)
	if err != nil || len(seededTemplates) != 0 {
		t.Fatalf("fresh Template repository after partial Start len=%d err=%v", len(seededTemplates), err)
	}
	customTemplate := &workspacemodel.WorkspaceTemplate{ID: "user-template", Name: "fresh-process customized template"}
	if err := freshTemplates.Create(ctx, customTemplate); err != nil {
		t.Fatal(err)
	}

	secondHub := magicevent.NewHub(64)
	secondSoulOwner := soulmodule.New()
	if err := secondSoulOwner.Configure(soulbiz.Dependencies{Souls: filerepo.NewSoulRepository(soulPath), EventHub: secondHub}); err != nil {
		t.Fatal(err)
	}
	secondWorkspaceOwner := workspacemodule.New()
	secondWorkspaceRepositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
	if err := secondWorkspaceOwner.Configure(workspacebiz.Dependencies{
		OwnerRepositories: secondWorkspaceRepositories.WorkspaceOwner,
		EventHub:          secondHub,
	}); err != nil {
		t.Fatal(err)
	}
	secondMCPOwner := mcpmodule.New()
	if err := secondMCPOwner.Configure(mcpregistrybiz.Dependencies{Registry: filerepo.NewMCPRegistryRepository(mcpPath), EventHub: secondHub}); err != nil {
		t.Fatal(err)
	}
	for name, start := range map[string]func() error{
		"soul": secondSoulOwner.Start, "workspace": secondWorkspaceOwner.Start, "mcp": secondMCPOwner.Start,
	} {
		if err := start(); err != nil {
			t.Fatalf("start fresh %s owner: %v", name, err)
		}
	}
	t.Cleanup(func() {
		_ = secondMCPOwner.Stop(ctx)
		_ = secondWorkspaceOwner.Stop(ctx)
		_ = secondSoulOwner.Stop(ctx)
		secondHub.Terminate(ctx)
	})
	secondSystem := New()
	if err := secondSystem.Configure(systembiz.Dependencies{EventHub: secondHub}); err != nil {
		t.Fatal(err)
	}
	if err := secondSystem.Start(ctx); err != nil {
		t.Fatalf("fresh file-backed Start: %v", err)
	}
	t.Cleanup(func() { _ = secondSystem.Stop(ctx) })

	gotSoul, err := filerepo.NewSoulRepository(soulPath).Get(ctx, customSoul.ID)
	if err != nil || gotSoul.Name != customSoul.Name {
		t.Fatalf("custom Soul after fresh retry = %+v err=%v", gotSoul, err)
	}
	gotTemplate, err := filerepo.NewWorkspaceTemplateRepository(templatePath).Get(ctx, customTemplate.ID)
	if err != nil || gotTemplate.Name != customTemplate.Name {
		t.Fatalf("custom Template after fresh retry = %+v err=%v", gotTemplate, err)
	}
	registry, err := filerepo.NewMCPRegistryRepository(mcpPath).Load(ctx)
	if err != nil || len(registry.Servers) == 0 || len(registry.Capabilities) == 0 {
		t.Fatalf("MCP defaults after fresh retry = %+v err=%v", registry, err)
	}
}

func systemModuleContractDependencies() systembiz.Dependencies {
	return systembiz.Dependencies{EventHub: magicevent.NewHub(16)}
}

func readSystemTestTemplates(t *testing.T, hub magicevent.Hub) []*workspaceevents.WorkspaceTemplate {
	t.Helper()
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceTemplateCommandList, "application/workorchd/appservice", workspaceevents.CommandDestination, magicevent.NewHeader(), context.Background(), workspaceevents.ListWorkspaceTemplatesCommand{})
	result := hub.Send(ev)
	if result == nil {
		t.Fatal("missing template result")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	typed, ok := data.(workspaceevents.WorkspaceTemplateListResult)
	if !ok || len(typed.Templates) == 0 {
		t.Fatal("missing current builtins")
	}
	return typed.Templates
}
