package contextregistryports

import (
	"context"
	"errors"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func TestUpdateByIDRejectsMissingPathID(t *testing.T) {
	ctx := context.Background()

	if _, err := UpdateSoulByID(ctx, nil, " ", workorchmanagementcontract.SoulMutation{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("UpdateSoulByID err = %v, want ErrInvalidArgument", err)
	}
	if _, err := UpdateMCPServerByID(ctx, nil, " ", workorchmanagementcontract.MCPServerMutation{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("UpdateMCPServerByID err = %v, want ErrInvalidArgument", err)
	}
	if _, err := UpdateMCPCapabilityByID(ctx, nil, " ", workorchmanagementcontract.MCPCapabilityMutation{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("UpdateMCPCapabilityByID err = %v, want ErrInvalidArgument", err)
	}
}

func TestUpdateByIDRejectsPayloadIDMismatch(t *testing.T) {
	ctx := context.Background()

	if _, err := UpdateSoulByID(ctx, nil, "soul-001", workorchmanagementcontract.SoulMutation{ID: "soul-other"}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("UpdateSoulByID err = %v, want ErrInvalidArgument", err)
	}
	if _, err := UpdateMCPServerByID(ctx, nil, "mcp-001", workorchmanagementcontract.MCPServerMutation{ID: "mcp-other"}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("UpdateMCPServerByID err = %v, want ErrInvalidArgument", err)
	}
	if _, err := UpdateMCPCapabilityByID(ctx, nil, "cap-001", workorchmanagementcontract.MCPCapabilityMutation{ID: "cap-other"}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("UpdateMCPCapabilityByID err = %v, want ErrInvalidArgument", err)
	}
}

func TestDeleteSoulRejectsMissingIDBeforePublishing(t *testing.T) {
	hub := magicevent.NewHub(8)

	if err := DeleteSoul(context.Background(), hub, " "); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("DeleteSoul err = %v, want ErrInvalidArgument", err)
	}
}

func TestDeleteSoulPublishesTypedCommand(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	var captured soulevents.DeleteSoulCommand
	subscribeMagicCommand(t, rawHub, soulevents.TopicSoulCommandDelete, soulevents.CommandDestination, func(cmd soulevents.DeleteSoulCommand, result magicevent.Result) {
		captured = cmd
		result.Set(soulevents.SoulCommandResult{}, nil)
	})

	if err := DeleteSoul(ctx, rawHub, " soul-delete-command "); err != nil {
		t.Fatalf("DeleteSoul: %v", err)
	}

	if captured.SoulID != "soul-delete-command" {
		t.Fatalf("command soulID = %s, want soul-delete-command", captured.SoulID)
	}
}

func TestDeleteMemoryItemPublishesCommandPayload(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-memory-delete")
	rawHub := magicevent.NewHub(16)
	var captured memoryevents.DeleteMemoryCommand
	subscribeMagicCommand(t, rawHub, memoryevents.TopicMemoryCommandDelete, memoryevents.CommandDestination, func(cmd memoryevents.DeleteMemoryCommand, result magicevent.Result) {
		captured = cmd
		result.Set(memoryevents.DeleteMemoryResult{WorkspaceID: cmd.WorkspaceID, MemoryID: cmd.MemoryID}, nil)
	})

	if err := DeleteMemoryItem(ctx, rawHub, "mem-context-delete"); err != nil {
		t.Fatalf("DeleteMemoryItem: %v", err)
	}
	if captured.MemoryID != "mem-context-delete" {
		t.Fatalf("command memoryID = %s, want mem-context-delete", captured.MemoryID)
	}
}

func TestPublishMemoryItemPublishesScopedCommandEnvelope(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	subscribeMagicCommand(t, rawHub, memoryevents.TopicMemoryCommandCreate, memoryevents.CommandDestination, func(cmd memoryevents.CreateMemoryCommand, result magicevent.Result) {
		if cmd.MemoryID != "mem-write-scope" || cmd.WorkspaceID != "ws-memory-write" || cmd.RunID != "run-memory-write" {
			t.Fatalf("command = %+v, want scoped memory command", cmd)
		}
		result.Set(memoryevents.CreateMemoryResult{WorkspaceID: cmd.WorkspaceID, MemoryID: cmd.MemoryID}, nil)
	})

	if err := PublishMemoryItem(ctx, rawHub, memoryevents.TopicMemoryCommandCreate, &memorymodel.MemoryItem{
		ID:          "mem-write-scope",
		WorkspaceID: "ws-memory-write",
		RunID:       "run-memory-write",
		Summary:     "scoped write",
	}); err != nil {
		t.Fatalf("PublishMemoryItem: %v", err)
	}
}

func TestMemoryQueriesPublishScopedOwnerReadCommands(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-memory-query-scope")
	rawHub := magicevent.NewHub(16)
	subscribeMagicCommand(t, rawHub, memoryevents.TopicMemoryCommandGet, memoryevents.CommandDestination, func(cmd memoryevents.GetMemoryCommand, result magicevent.Result) {
		if cmd.WorkspaceID != "ws-memory-query-scope" || cmd.MemoryID != "mem-query-scope" {
			t.Fatalf("get command = %+v, want scoped get", cmd)
		}
		result.Set(memoryevents.GetMemoryResult{Item: &memorymodel.MemoryItem{ID: "mem-query-scope", WorkspaceID: "ws-memory-query-scope"}}, nil)
	})
	subscribeMagicCommand(t, rawHub, memoryevents.TopicMemoryCommandListByWorkspace, memoryevents.CommandDestination, func(cmd memoryevents.ListMemoryByWorkspaceCommand, result magicevent.Result) {
		if cmd.WorkspaceID != "ws-memory-query-scope" {
			t.Fatalf("list command = %+v, want scoped list", cmd)
		}
		result.Set(memoryevents.ListMemoryByWorkspaceResult{Items: []*memorymodel.MemoryItem{}}, nil)
	})
	subscribeMagicCommand(t, rawHub, memoryevents.TopicMemoryCommandSearch, memoryevents.CommandDestination, func(cmd memoryevents.SearchMemoryCommand, result magicevent.Result) {
		if cmd.Query.WorkspaceID != "ws-memory-query-scope" {
			t.Fatalf("search command = %+v, want scoped search", cmd)
		}
		result.Set(memoryevents.SearchMemoryResult{Items: []*memorymodel.MemoryItem{}}, nil)
	})

	if _, err := ReadMemoryItem(ctx, rawHub, "mem-query-scope"); err != nil {
		t.Fatalf("ReadMemoryItem: %v", err)
	}
	if _, err := ListMemoryByWorkspace(context.Background(), rawHub, "ws-memory-query-scope"); err != nil {
		t.Fatalf("ListMemoryByWorkspace: %v", err)
	}
	if _, err := SearchMemory(context.Background(), rawHub, workorchquerycontract.MemorySearchQuery{WorkspaceID: "ws-memory-query-scope", Query: "scope"}); err != nil {
		t.Fatalf("SearchMemory: %v", err)
	}
}

func TestEnsureWorkspaceExistsPublishesScopedOwnerReadCommand(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(8)
	defer rawHub.Terminate(ctx)
	observer := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, rawHub)
	observer.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(event magicevent.Event, result magicevent.Result) {
		cmd, ok := event.Data().(workspaceevents.GetWorkspaceCommand)
		if !ok || cmd.WorkspaceID != "ws-memory-workspace-scope" || common.RuntimeWorkspaceIDFromContext(event.Context()) != "ws-memory-workspace-scope" {
			t.Fatalf("workspace get command = %+v context workspace=%q", cmd, common.RuntimeWorkspaceIDFromContext(event.Context()))
		}
		result.Set(workspaceevents.WorkspaceCommandResult{Workspace: &workspaceevents.Workspace{ID: cmd.WorkspaceID}}, nil)
	})
	defer observer.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet)

	if err := EnsureWorkspaceExists(ctx, rawHub, "ws-memory-workspace-scope"); err != nil {
		t.Fatalf("EnsureWorkspaceExists err = %v", err)
	}
}

func TestContextRegistryWritesPublishContractCommands(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)

	subscribeMagicCommand(t, rawHub, soulevents.TopicSoulCommandCreate, soulevents.CommandDestination, func(cmd soulevents.CreateSoulCommand, result magicevent.Result) {
		if cmd.SoulID != "soul-contract" || cmd.Name != "Soul Contract" {
			t.Fatalf("soul command = %+v", cmd)
		}
		result.Set(soulevents.SoulCommandResult{Soul: &soulmodel.Soul{ID: cmd.SoulID, Name: cmd.Name, SoulMD: cmd.SoulMD}}, nil)
	})
	subscribeMagicCommand(t, rawHub, skillevents.TopicSkillCommandCreate, skillevents.CommandDestination, func(cmd skillevents.CreateSkillCommand, result magicevent.Result) {
		if cmd.SkillID != "skill-contract" || cmd.Name != "Skill Contract" {
			t.Fatalf("skill command = %+v", cmd)
		}
		result.Set(skillevents.CreateSkillResult{Skill: registrySkillFixture(cmd.SkillID, cmd.Name, "v1", cmd.Instruction)}, nil)
	})
	subscribeMagicCommand(t, rawHub, mcpevents.TopicMCPCommandCreateServer, mcpevents.CommandDestination, func(cmd mcpevents.CreateServerCommand, result magicevent.Result) {
		env, err := mcpevents.EnvironmentVariableMap(cmd.Env)
		if err != nil || cmd.ServerID != "mcp-contract" || cmd.Command != "python3" || env["TOKEN"] != "secret" {
			t.Fatalf("server command = %+v", cmd)
		}
		result.Set(mcpevents.MCPCommandResult{Server: &mcpevents.MCPServer{ID: cmd.ServerID, Name: cmd.Name, Command: cmd.Command, Env: append([]mcpevents.EnvironmentVariable(nil), cmd.Env...)}}, nil)
	})
	subscribeMagicCommand(t, rawHub, mcpevents.TopicMCPCommandCreateCapability, mcpevents.CommandDestination, func(cmd mcpevents.CreateCapabilityCommand, result magicevent.Result) {
		schema := cmd.Schema.Map()
		if cmd.CapabilityID != "cap-contract" || cmd.ServerID != "mcp-contract" || schema["type"] != "object" {
			t.Fatalf("capability command = %+v", cmd)
		}
		result.Set(mcpevents.MCPCommandResult{Capability: &mcpmodel.Capability{ID: cmd.CapabilityID, ServerID: cmd.ServerID, Name: cmd.Name, Schema: cmd.Schema.Clone()}}, nil)
	})
	subscribeMagicCommand(t, rawHub, mcpevents.TopicMCPCommandListCapabilities, mcpevents.CommandDestination, func(cmd mcpevents.ListCapabilitiesCommand, result magicevent.Result) {
		query := cmd.Query
		if query.ServerID != "mcp-contract" || query.Kind != "tool" {
			t.Fatalf("capability query = %+v", query)
		}
		result.Set(mcpevents.MCPCommandResult{Capabilities: []*mcpmodel.Capability{{ID: "cap-contract", ServerID: query.ServerID, Kind: query.Kind}}}, nil)
	})
	subscribeMagicCommand(t, rawHub, soulevents.TopicSoulCommandValidateCapability, soulevents.CommandDestination, func(cmd soulevents.ValidateCapabilitySelectionCommand, result magicevent.Result) {
		if cmd.SoulID != "soul-contract" || len(cmd.CapabilityRefs) != 1 || cmd.CapabilityRefs[0] != "cap-contract" {
			t.Fatalf("validate command = %+v", cmd)
		}
		result.Set(soulevents.SoulCommandResult{}, nil)
	})

	if _, err := CreateSoul(ctx, rawHub, workorchmanagementcontract.SoulMutation{ID: "soul-contract", Name: "Soul Contract", SoulMD: "# soul"}); err != nil {
		t.Fatalf("CreateSoul: %v", err)
	}
	if _, err := CreateSkill(ctx, rawHub, workorchmanagementcontract.SkillMutation{ID: "skill-contract", Name: "Skill Contract", Instruction: "Do it"}); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}
	if _, err := CreateMCPServer(ctx, rawHub, workorchmanagementcontract.MCPServerMutation{ID: "mcp-contract", Name: "MCP Contract", Command: "python3", Env: map[string]string{"TOKEN": "secret"}}); err != nil {
		t.Fatalf("CreateMCPServer: %v", err)
	}
	if _, err := CreateMCPCapability(ctx, rawHub, workorchmanagementcontract.MCPCapabilityMutation{ID: "cap-contract", ServerID: "mcp-contract", Kind: "tool", Name: "repo.summary", Schema: map[string]any{"type": "object"}}); err != nil {
		t.Fatalf("CreateMCPCapability: %v", err)
	}
	if _, err := ListMCPCapabilities(ctx, rawHub, contextregistrycore.MCPCapabilityQuery{ServerID: "mcp-contract", Kind: "tool"}); err != nil {
		t.Fatalf("ListMCPCapabilities: %v", err)
	}
	if err := ValidateSoulCapabilitySelection(ctx, rawHub, "soul-contract", nil, nil, []string{"cap-contract"}, nil, nil); err != nil {
		t.Fatalf("ValidateSoulCapabilitySelection: %v", err)
	}
}

func TestMCPServerViewAndDerivedEntriesUseOwnerEventHubCommand(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	callCount := 0
	subscribeMagicCommand(t, rawHub, mcpevents.TopicMCPCommandGetServer, mcpevents.CommandDestination, func(cmd mcpevents.GetServerCommand, result magicevent.Result) {
		callCount++
		if got := string(cmd.ServerID); got != "mcp-stdio" {
			t.Fatalf("server id = %v, want mcp-stdio", got)
		}
		result.Set(mcpevents.MCPCommandResult{
			Server: &mcpevents.MCPServer{
				ID:                    "mcp-stdio",
				Name:                  "stdio verification",
				Transport:             "stdio",
				Endpoint:              "/tmp/mcp_stdio_helper.py",
				Command:               "python3",
				Args:                  []string{"helper.py"},
				Env:                   []mcpevents.EnvironmentVariable{{Name: "TOKEN", Value: "secret"}},
				Enabled:               true,
				AllowedTools:          []string{"repo.summary"},
				AllowedResources:      []string{"workspace://acceptance"},
				AllowedPrompts:        []string{"workorch.acceptance"},
				DefaultApprovalPolicy: "operator",
				SessionPolicy:         "per_run",
				TimeoutMillis:         5000,
			},
		}, nil)
	})

	server, err := ReadMCPServerView(ctx, rawHub, "mcp-stdio")
	if err != nil {
		t.Fatalf("ReadMCPServerView err = %v", err)
	}
	if server.ID != "mcp-stdio" || server.Name != "stdio verification" || server.Transport != "stdio" {
		t.Fatalf("server view = %#v", server)
	}
	if server.Env["TOKEN"] == "secret" {
		t.Fatalf("server env was not redacted: %#v", server.Env)
	}

	tools, err := ListMCPServerTools(ctx, rawHub, "mcp-stdio")
	if err != nil {
		t.Fatalf("ListMCPServerTools err = %v", err)
	}
	if len(tools) != 1 || tools[0].ServerID != "mcp-stdio" || tools[0].Name != "repo.summary" {
		t.Fatalf("tools = %#v", tools)
	}
	resources, err := ListMCPServerResources(ctx, rawHub, "mcp-stdio")
	if err != nil {
		t.Fatalf("ListMCPServerResources err = %v", err)
	}
	if len(resources) != 1 || resources[0].ServerID != "mcp-stdio" || resources[0].URI != "workspace://acceptance" {
		t.Fatalf("resources = %#v", resources)
	}
	prompts, err := ListMCPServerPrompts(ctx, rawHub, "mcp-stdio")
	if err != nil {
		t.Fatalf("ListMCPServerPrompts err = %v", err)
	}
	if len(prompts) != 1 || prompts[0].ServerID != "mcp-stdio" || prompts[0].Name != "workorch.acceptance" {
		t.Fatalf("prompts = %#v", prompts)
	}
	if callCount != 4 {
		t.Fatalf("owner get command calls = %d, want 4", callCount)
	}
}

type fakeSkillHubCatalog struct {
	global     []workorchmanagementcontract.Skill
	workspaces map[string][]workorchmanagementcontract.Skill
}

func (f fakeSkillHubCatalog) ListGlobalSkills(context.Context) ([]workorchmanagementcontract.Skill, error) {
	return append([]workorchmanagementcontract.Skill(nil), f.global...), nil
}

func (f fakeSkillHubCatalog) ListWorkspaceSkills(_ context.Context, workspaceID string) ([]workorchmanagementcontract.Skill, error) {
	return append([]workorchmanagementcontract.Skill(nil), f.workspaces[workspaceID]...), nil
}

func TestReadSkillViewFallsBackToSkillHubWhenRepositorySkillMissing(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	subscribeMagicCommand(t, rawHub, skillevents.TopicSkillCommandGet, skillevents.CommandDestination, func(cmd skillevents.GetSkillCommand, result magicevent.Result) {
		if got := string(cmd.SkillID); got != "skillhub:workspace:ws-001:workspace-skill" {
			t.Fatalf("skill id = %v", got)
		}
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: common.ErrNotFound.Error(), Cause: common.ErrNotFound})
	})
	ref := "skillhub:workspace:ws-001:workspace-skill"
	catalog := fakeSkillHubCatalog{
		workspaces: map[string][]workorchmanagementcontract.Skill{
			"ws-001": {{ID: ref, SourceSkillID: "workspace-skill", Source: contextregistrycore.SkillSourceSkillHub}},
		},
	}

	item, err := ReadSkillView(ctx, rawHub, ref, contextregistrycore.SkillQuery{WorkspaceID: "ws-001"}, catalog)
	if err != nil {
		t.Fatalf("ReadSkillView err = %v", err)
	}
	if item.ID != ref || item.Source != contextregistrycore.SkillSourceSkillHub {
		t.Fatalf("skill = %#v", item)
	}
}

func TestListSkillsViewMergesRepositoryAndSkillHubSkills(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	subscribeMagicCommand(t, rawHub, skillevents.TopicSkillCommandList, skillevents.CommandDestination, func(_ skillevents.ListSkillsCommand, result magicevent.Result) {
		result.Set(skillevents.ListSkillsResult{
			Skills: []*skillmodel.Skill{registrySkillFixture("repo-skill", "Repository Skill", "v1", "repository instruction")},
		}, nil)
	})
	catalog := fakeSkillHubCatalog{
		global: []workorchmanagementcontract.Skill{{ID: "skillhub:global:global-skill", Source: contextregistrycore.SkillSourceSkillHub}},
		workspaces: map[string][]workorchmanagementcontract.Skill{
			"ws-001": {{ID: "skillhub:workspace:ws-001:workspace-skill", Source: contextregistrycore.SkillSourceSkillHub}},
		},
	}

	items, err := ListSkillsView(ctx, rawHub, contextregistrycore.SkillQuery{WorkspaceID: "ws-001"}, catalog)
	if err != nil {
		t.Fatalf("ListSkillsView err = %v", err)
	}
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.ID)
	}
	want := []string{"repo-skill", "skillhub:global:global-skill", "skillhub:workspace:ws-001:workspace-skill"}
	if len(got) != len(want) {
		t.Fatalf("ids = %#v, want %#v", got, want)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("ids = %#v, want %#v", got, want)
		}
	}
}

func subscribeMagicCommand[Command any](t *testing.T, hub magicevent.Hub, topic string, destination string, handle func(Command, magicevent.Result)) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(destination, hub)
	observer.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(Command)
		if !ok {
			t.Fatalf("event %s command type = %T", topic, ev.Data())
		}
		handle(cmd, result)
	})
}

func registrySkillFixture(id common.SkillID, name, version, instruction string) *skillmodel.Skill {
	item := &skillmodel.Skill{ID: id, Name: name, Version: version, Instruction: instruction}
	item.Provenance = workorchskillcontract.Provenance{
		Source: workorchskillcontract.SourceWorkOrchRegistry, Scope: workorchskillcontract.ScopeGlobal,
		ContentHash: workorchskillcontract.StableContentHash(item.Content()),
	}
	return item
}
