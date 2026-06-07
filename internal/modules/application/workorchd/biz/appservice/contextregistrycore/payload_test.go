package contextregistrycore

import (
	"errors"
	"testing"

	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestSoulFromPayload(t *testing.T) {
	got, err := SoulFromPayload(map[string]any{
		events.PayloadKeySoul: &soulmodel.Soul{
			ID:               "soul-001",
			Name:             "execution",
			AllowedToolKinds: []string{"mcp/tool_call"},
		},
	}, events.PayloadKeySoul)
	if err != nil {
		t.Fatalf("SoulFromPayload err = %v", err)
	}
	if got.ID != "soul-001" || got.Name != "execution" || len(got.AllowedToolKinds) != 1 {
		t.Fatalf("SoulFromPayload = %#v", got)
	}

	if _, err := SoulFromPayload(map[string]any{events.PayloadKeySoul: "bad"}, events.PayloadKeySoul); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("SoulFromPayload bad err = %v", err)
	}
}

func TestSkillsFromPayload(t *testing.T) {
	got, err := SkillsFromPayload(map[string]any{
		events.PayloadKeySkills: []*skillmodel.Skill{
			{ID: "skill-001", Name: "skill", Version: "v1", RequiredToolKinds: []string{"shell/command"}},
		},
	}, events.PayloadKeySkills)
	if err != nil {
		t.Fatalf("SkillsFromPayload err = %v", err)
	}
	if len(got) != 1 || got[0].ID != "skill-001" || got[0].Source != SkillSourceRepository || got[0].ManagedBy != SkillManagedBy {
		t.Fatalf("SkillsFromPayload = %#v", got)
	}

	if _, err := SkillsFromPayload(map[string]any{events.PayloadKeySkills: []any{}}, events.PayloadKeySkills); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("SkillsFromPayload bad err = %v", err)
	}
}

func TestMCPServerAndCapabilityPayloads(t *testing.T) {
	server, err := MCPServerFromPayload(map[string]any{
		events.PayloadKeyServer: &mcpmodel.Server{
			ID:               "srv-001",
			Name:             "server",
			Transport:        "stdio",
			Env:              map[string]string{"TOKEN": "secret", "EMPTY": ""},
			AllowedTools:     []string{"repo.summary"},
			AllowedResources: []string{"workspace://acceptance"},
			AllowedPrompts:   []string{"workorch.acceptance"},
		},
	}, events.PayloadKeyServer)
	if err != nil {
		t.Fatalf("MCPServerFromPayload err = %v", err)
	}
	if server.ID != "srv-001" || server.Env["TOKEN"] != "redacted" || server.Env["EMPTY"] != "" {
		t.Fatalf("MCPServerFromPayload = %#v", server)
	}
	if tools := MCPToolsFromServer(server); len(tools) != 1 || tools[0].ServerID != "srv-001" || tools[0].Name != "repo.summary" {
		t.Fatalf("MCPToolsFromServer = %#v", tools)
	}
	if resources := MCPResourcesFromServer(server); len(resources) != 1 || resources[0].ServerID != "srv-001" || resources[0].URI != "workspace://acceptance" {
		t.Fatalf("MCPResourcesFromServer = %#v", resources)
	}
	if prompts := MCPPromptsFromServer(server); len(prompts) != 1 || prompts[0].ServerID != "srv-001" || prompts[0].Name != "workorch.acceptance" {
		t.Fatalf("MCPPromptsFromServer = %#v", prompts)
	}

	caps, err := MCPCapabilitiesFromPayload(map[string]any{
		events.PayloadKeyCapabilities: []*mcpmodel.Capability{
			{ID: "cap-001", ServerID: "srv-001", Kind: "tool", Name: "repo.summary", Enabled: true},
		},
	}, events.PayloadKeyCapabilities)
	if err != nil {
		t.Fatalf("MCPCapabilitiesFromPayload err = %v", err)
	}
	if len(caps) != 1 || caps[0].ID != "cap-001" || caps[0].ServerID != "srv-001" || !caps[0].Enabled {
		t.Fatalf("MCPCapabilitiesFromPayload = %#v", caps)
	}
}

func TestMemoryItemsFromPayloadAndMapping(t *testing.T) {
	items, err := MemoryItemsFromPayload(map[string]any{
		events.PayloadKeyMemoryItems: []*memorymodel.MemoryItem{
			{
				ID:          "mem-001",
				WorkspaceID: "ws-001",
				MemoryType:  "fact",
				Summary:     "summary",
				Vector:      []float64{0.1, 0.2},
				VectorScore: 0.75,
			},
		},
	}, events.PayloadKeyMemoryItems)
	if err != nil {
		t.Fatalf("MemoryItemsFromPayload err = %v", err)
	}

	views := MapMemoryItems(items)
	if len(views) != 1 || views[0].ID != "mem-001" || !views[0].VectorReady || views[0].VectorDimension != 2 || views[0].VectorScore != 0.75 {
		t.Fatalf("MapMemoryItems = %#v", views)
	}

	if _, err := MemoryItemsFromPayload(map[string]any{events.PayloadKeyMemoryItems: []any{}}, events.PayloadKeyMemoryItems); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("MemoryItemsFromPayload bad err = %v", err)
	}
}

func TestMemorySearchRepositoryQuery(t *testing.T) {
	got, err := MemorySearchRepositoryQuery(MemoryQuery{
		WorkspaceID: " ws-001 ",
		Query:       " context ",
		Tags:        []string{"tag"},
		Types:       []string{"fact"},
		Limit:       3,
	})
	if err != nil {
		t.Fatalf("MemorySearchRepositoryQuery err = %v", err)
	}
	if got.WorkspaceID != common.WorkspaceID("ws-001") || got.Query != "context" || got.Limit != 3 {
		t.Fatalf("MemorySearchRepositoryQuery = %#v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "tag" || len(got.Types) != 1 || got.Types[0] != "fact" {
		t.Fatalf("MemorySearchRepositoryQuery filters = tags:%#v types:%#v", got.Tags, got.Types)
	}

	if _, err := MemorySearchRepositoryQuery(MemoryQuery{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("MemorySearchRepositoryQuery missing workspace err = %v", err)
	}
}
