package contextregistryports

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	mcpcontracts "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/contracts"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorycontracts "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/contracts"
	skillcontracts "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/contracts"
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soulcontracts "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/contracts"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestUpdateByIDRejectsMissingPathID(t *testing.T) {
	ctx := context.Background()

	if _, err := UpdateSoulByID(ctx, nil, " ", contextregistrycore.Soul{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("UpdateSoulByID err = %v, want ErrInvalidArgument", err)
	}
	if _, err := UpdateMCPServerByID(ctx, nil, " ", contextregistrycore.MCPServer{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("UpdateMCPServerByID err = %v, want ErrInvalidArgument", err)
	}
	if _, err := UpdateMCPCapabilityByID(ctx, nil, " ", contextregistrycore.MCPCapability{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("UpdateMCPCapabilityByID err = %v, want ErrInvalidArgument", err)
	}
}

func TestUpdateByIDRejectsPayloadIDMismatch(t *testing.T) {
	ctx := context.Background()

	if _, err := UpdateSoulByID(ctx, nil, "soul-001", contextregistrycore.Soul{ID: "soul-other"}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("UpdateSoulByID err = %v, want ErrInvalidArgument", err)
	}
	if _, err := UpdateMCPServerByID(ctx, nil, "mcp-001", contextregistrycore.MCPServer{ID: "mcp-other"}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("UpdateMCPServerByID err = %v, want ErrInvalidArgument", err)
	}
	if _, err := UpdateMCPCapabilityByID(ctx, nil, "cap-001", contextregistrycore.MCPCapability{ID: "cap-other"}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("UpdateMCPCapabilityByID err = %v, want ErrInvalidArgument", err)
	}
}

func TestDeleteSoulRejectsMissingIDBeforePublishing(t *testing.T) {
	hub := events.NewMemoryHub()

	if err := DeleteSoul(context.Background(), hub, " "); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("DeleteSoul err = %v, want ErrInvalidArgument", err)
	}
	if published := hub.Published(); len(published) != 0 {
		t.Fatalf("published envelopes = %d, want 0", len(published))
	}
}

func TestDeleteSoulPublishesCommandPayload(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(events.TopicSoulCommandDelete, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(envelope)
	})

	if err := DeleteSoul(ctx, hub, " soul-delete-command "); err != nil {
		t.Fatalf("DeleteSoul: %v", err)
	}

	published := hub.Published()
	if len(published) != 1 {
		t.Fatalf("published len = %d, want 1", len(published))
	}
	cmd, ok := published[0].Payload[events.PayloadKeyCommand].(soulcontracts.DeleteSoulCommand)
	if !ok {
		t.Fatalf("payload command = %T, want DeleteSoulCommand", published[0].Payload[events.PayloadKeyCommand])
	}
	if cmd.SoulID != "soul-delete-command" {
		t.Fatalf("command soulID = %s, want soul-delete-command", cmd.SoulID)
	}
	if _, ok := published[0].Payload[events.PayloadKeySoulID]; ok {
		t.Fatalf("payload contains legacy soulId field")
	}
}

func TestDeleteMemoryItemPublishesCommandPayload(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(events.TopicMemoryCommandDelete, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(envelope)
	})

	if err := DeleteMemoryItem(ctx, hub, "mem-context-delete"); err != nil {
		t.Fatalf("DeleteMemoryItem: %v", err)
	}

	published := hub.Published()
	if len(published) != 1 {
		t.Fatalf("published len = %d, want 1", len(published))
	}
	cmd, ok := published[0].Payload[events.PayloadKeyCommand].(memorycontracts.DeleteMemoryItemCommand)
	if !ok {
		t.Fatalf("payload command = %T, want DeleteMemoryItemCommand", published[0].Payload[events.PayloadKeyCommand])
	}
	if cmd.MemoryID != "mem-context-delete" {
		t.Fatalf("command memoryID = %s, want mem-context-delete", cmd.MemoryID)
	}
	if _, ok := published[0].Payload[events.PayloadKeyMemoryID]; ok {
		t.Fatalf("payload contains legacy memoryId field")
	}
}

func TestContextRegistryWritesPublishContractCommands(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()

	hub.Subscribe(events.TopicSoulCommandCreate, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(soulcontracts.CreateSoulCommand)
		if !ok {
			t.Fatalf("soul command payload type = %T", envelope.Payload[events.PayloadKeyCommand])
		}
		if cmd.SoulID != "soul-contract" || cmd.Name != "Soul Contract" {
			t.Fatalf("soul command = %+v", cmd)
		}
		if _, ok := envelope.Payload[events.PayloadKeySoul]; ok {
			t.Fatalf("soul command still carries owner model payload")
		}
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{
			events.PayloadKeySoul: &soulmodel.Soul{ID: cmd.SoulID, Name: cmd.Name, SoulMD: cmd.SoulMD},
		}})
	})
	hub.Subscribe(events.TopicSkillCommandCreate, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(skillcontracts.CreateSkillCommand)
		if !ok {
			t.Fatalf("skill command payload type = %T", envelope.Payload[events.PayloadKeyCommand])
		}
		if cmd.SkillID != "skill-contract" || cmd.Name != "Skill Contract" {
			t.Fatalf("skill command = %+v", cmd)
		}
		if _, ok := envelope.Payload[events.PayloadKeySkill]; ok {
			t.Fatalf("skill command still carries owner model payload")
		}
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{
			events.PayloadKeySkill: &skillmodel.Skill{ID: cmd.SkillID, Name: cmd.Name, Instruction: cmd.Instruction},
		}})
	})
	hub.Subscribe(events.TopicMCPCommandCreateServer, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(mcpcontracts.CreateServerCommand)
		if !ok {
			t.Fatalf("server command payload type = %T", envelope.Payload[events.PayloadKeyCommand])
		}
		if cmd.ServerID != "mcp-contract" || cmd.Command != "python3" || cmd.Env["TOKEN"] != "secret" {
			t.Fatalf("server command = %+v", cmd)
		}
		if _, ok := envelope.Payload[events.PayloadKeyServer]; ok {
			t.Fatalf("server command still carries owner model payload")
		}
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{
			events.PayloadKeyServer: &mcpmodel.Server{ID: cmd.ServerID, Name: cmd.Name, Command: cmd.Command, Env: cmd.Env},
		}})
	})
	hub.Subscribe(events.TopicMCPCommandCreateCapability, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(mcpcontracts.CreateCapabilityCommand)
		if !ok {
			t.Fatalf("capability command payload type = %T", envelope.Payload[events.PayloadKeyCommand])
		}
		if cmd.ID != "cap-contract" || cmd.ServerID != "mcp-contract" || cmd.Schema["type"] != "object" {
			t.Fatalf("capability command = %+v", cmd)
		}
		if _, ok := envelope.Payload[events.PayloadKeyCapability]; ok {
			t.Fatalf("capability command still carries owner model payload")
		}
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{
			events.PayloadKeyCapability: &mcpmodel.Capability{ID: cmd.ID, ServerID: cmd.ServerID, Name: cmd.Name, Schema: cmd.Schema},
		}})
	})
	hub.Subscribe(events.TopicMCPCommandListCapabilities, func(_ context.Context, envelope events.Envelope) error {
		query, ok := envelope.Payload[events.PayloadKeyCapabilityQuery].(mcpcontracts.CapabilityQuery)
		if !ok {
			t.Fatalf("capability query payload type = %T", envelope.Payload[events.PayloadKeyCapabilityQuery])
		}
		if query.ServerID != "mcp-contract" || query.Kind != "tool" {
			t.Fatalf("capability query = %+v", query)
		}
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{
			events.PayloadKeyCapabilities: []*mcpmodel.Capability{{ID: "cap-contract", ServerID: query.ServerID, Kind: query.Kind}},
		}})
	})
	hub.Subscribe(events.TopicSoulCommandValidateCapability, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(soulcontracts.ValidateCapabilitySelectionCommand)
		if !ok {
			t.Fatalf("validate command payload type = %T", envelope.Payload[events.PayloadKeyCommand])
		}
		if cmd.SoulID != "soul-contract" || len(cmd.CapabilityRefs) != 1 || cmd.CapabilityRefs[0] != "cap-contract" {
			t.Fatalf("validate command = %+v", cmd)
		}
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{}})
	})

	if _, err := CreateSoul(ctx, hub, contextregistrycore.Soul{ID: "soul-contract", Name: "Soul Contract", SoulMD: "# soul"}); err != nil {
		t.Fatalf("CreateSoul: %v", err)
	}
	if _, err := CreateSkill(ctx, hub, contextregistrycore.Skill{ID: "skill-contract", Name: "Skill Contract", Instruction: "Do it"}); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}
	if _, err := CreateMCPServer(ctx, hub, contextregistrycore.MCPServer{ID: "mcp-contract", Name: "MCP Contract", Command: "python3", Env: map[string]string{"TOKEN": "secret"}}); err != nil {
		t.Fatalf("CreateMCPServer: %v", err)
	}
	if _, err := CreateMCPCapability(ctx, hub, contextregistrycore.MCPCapability{ID: "cap-contract", ServerID: "mcp-contract", Kind: "tool", Name: "repo.summary", Schema: map[string]any{"type": "object"}}); err != nil {
		t.Fatalf("CreateMCPCapability: %v", err)
	}
	if _, err := ListMCPCapabilities(ctx, hub, contextregistrycore.MCPCapabilityQuery{ServerID: "mcp-contract", Kind: "tool"}); err != nil {
		t.Fatalf("ListMCPCapabilities: %v", err)
	}
	if err := ValidateSoulCapabilitySelection(ctx, hub, "soul-contract", nil, nil, []string{"cap-contract"}, nil, nil); err != nil {
		t.Fatalf("ValidateSoulCapabilitySelection: %v", err)
	}
}

func TestMCPServerViewAndDerivedEntriesUseOwnerEventHubCommand(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	callCount := 0
	hub.Subscribe(events.TopicMCPCommandGetServer, func(_ context.Context, envelope events.Envelope) error {
		callCount++
		if envelope.SourceModule != sourceModule {
			t.Fatalf("source module = %q, want %q", envelope.SourceModule, sourceModule)
		}
		if got := envelope.Payload[events.PayloadKeyServerID]; got != "mcp-stdio" {
			t.Fatalf("server id payload = %v, want mcp-stdio", got)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyServer: &mcpmodel.Server{
					ID:                    "mcp-stdio",
					Name:                  "stdio verification",
					Transport:             "stdio",
					Endpoint:              "/tmp/mcp_stdio_helper.py",
					Command:               "python3",
					Args:                  []string{"helper.py"},
					Env:                   map[string]string{"TOKEN": "secret"},
					Enabled:               true,
					AllowedTools:          []string{"repo.summary"},
					AllowedResources:      []string{"workspace://acceptance"},
					AllowedPrompts:        []string{"workorch.acceptance"},
					DefaultApprovalPolicy: "operator",
					SessionPolicy:         "per_run",
					TimeoutMillis:         5000,
				},
			},
		})
	})

	server, err := ReadMCPServerView(ctx, hub, "mcp-stdio")
	if err != nil {
		t.Fatalf("ReadMCPServerView err = %v", err)
	}
	if server.ID != "mcp-stdio" || server.Name != "stdio verification" || server.Transport != "stdio" {
		t.Fatalf("server view = %#v", server)
	}
	if server.Env["TOKEN"] == "secret" {
		t.Fatalf("server env was not redacted: %#v", server.Env)
	}

	tools, err := ListMCPServerTools(ctx, hub, "mcp-stdio")
	if err != nil {
		t.Fatalf("ListMCPServerTools err = %v", err)
	}
	if len(tools) != 1 || tools[0].ServerID != "mcp-stdio" || tools[0].Name != "repo.summary" {
		t.Fatalf("tools = %#v", tools)
	}
	resources, err := ListMCPServerResources(ctx, hub, "mcp-stdio")
	if err != nil {
		t.Fatalf("ListMCPServerResources err = %v", err)
	}
	if len(resources) != 1 || resources[0].ServerID != "mcp-stdio" || resources[0].URI != "workspace://acceptance" {
		t.Fatalf("resources = %#v", resources)
	}
	prompts, err := ListMCPServerPrompts(ctx, hub, "mcp-stdio")
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
	global     []contextregistrycore.Skill
	workspaces map[string][]contextregistrycore.Skill
}

func (f fakeSkillHubCatalog) ListGlobalSkills(context.Context) ([]contextregistrycore.Skill, error) {
	return append([]contextregistrycore.Skill(nil), f.global...), nil
}

func (f fakeSkillHubCatalog) ListWorkspaceSkills(_ context.Context, workspaceID string) ([]contextregistrycore.Skill, error) {
	return append([]contextregistrycore.Skill(nil), f.workspaces[workspaceID]...), nil
}

func TestReadSkillViewFallsBackToSkillHubWhenRepositorySkillMissing(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(events.TopicSkillCommandGet, func(_ context.Context, envelope events.Envelope) error {
		if got := envelope.Payload[events.PayloadKeySkillID]; got != "skillhub:workspace:ws-001:workspace-skill" {
			t.Fatalf("skill id payload = %v", got)
		}
		return common.ErrNotFound
	})
	ref := "skillhub:workspace:ws-001:workspace-skill"
	catalog := fakeSkillHubCatalog{
		workspaces: map[string][]contextregistrycore.Skill{
			"ws-001": {{ID: ref, SourceSkillID: "workspace-skill", Source: contextregistrycore.SkillSourceSkillHub}},
		},
	}

	item, err := ReadSkillView(ctx, hub, ref, contextregistrycore.SkillQuery{WorkspaceID: "ws-001"}, catalog)
	if err != nil {
		t.Fatalf("ReadSkillView err = %v", err)
	}
	if item.ID != ref || item.Source != contextregistrycore.SkillSourceSkillHub {
		t.Fatalf("skill = %#v", item)
	}
}

func TestListSkillsViewMergesRepositoryAndSkillHubSkills(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(events.TopicSkillCommandList, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeySkills: []*skillmodel.Skill{
					{ID: "repo-skill", Name: "Repository Skill"},
				},
			},
		})
	})
	catalog := fakeSkillHubCatalog{
		global: []contextregistrycore.Skill{{ID: "skillhub:global:global-skill", Source: contextregistrycore.SkillSourceSkillHub}},
		workspaces: map[string][]contextregistrycore.Skill{
			"ws-001": {{ID: "skillhub:workspace:ws-001:workspace-skill", Source: contextregistrycore.SkillSourceSkillHub}},
		},
	}

	items, err := ListSkillsView(ctx, hub, contextregistrycore.SkillQuery{WorkspaceID: "ws-001"}, catalog)
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
