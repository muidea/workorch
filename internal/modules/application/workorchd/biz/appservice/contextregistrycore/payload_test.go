package contextregistrycore

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestSoulFromPayload(t *testing.T) {
	got, err := SoulFromPayload(map[string]any{
		recordpayload.PayloadKeySoul: &soulmodel.Soul{
			ID:               "soul-001",
			Name:             "execution",
			AllowedToolKinds: []string{"mcp/tool_call"},
		},
	}, recordpayload.PayloadKeySoul)
	if err != nil {
		t.Fatalf("SoulFromPayload err = %v", err)
	}
	if got.ID != "soul-001" || got.Name != "execution" || len(got.AllowedToolKinds) != 1 {
		t.Fatalf("SoulFromPayload = %#v", got)
	}

	if _, err := SoulFromPayload(map[string]any{recordpayload.PayloadKeySoul: "bad"}, recordpayload.PayloadKeySoul); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("SoulFromPayload bad err = %v", err)
	}
}

func TestSkillsFromPayload(t *testing.T) {
	got, err := SkillsFromPayload(map[string]any{
		recordpayload.PayloadKeySkills: []*skillmodel.Skill{
			{ID: "skill-001", Name: "skill", Version: "v1", RequiredToolKinds: []string{"shell/command"}},
		},
	}, recordpayload.PayloadKeySkills)
	if err != nil {
		t.Fatalf("SkillsFromPayload err = %v", err)
	}
	if len(got) != 1 || got[0].ID != "skill-001" || got[0].Source != SkillSourceRepository || got[0].ManagedBy != SkillManagedBy {
		t.Fatalf("SkillsFromPayload = %#v", got)
	}

	if _, err := SkillsFromPayload(map[string]any{recordpayload.PayloadKeySkills: []any{}}, recordpayload.PayloadKeySkills); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("SkillsFromPayload bad err = %v", err)
	}
}

func TestMCPServerAndCapabilityPayloads(t *testing.T) {
	server, err := MCPServerFromPayload(map[string]any{
		recordpayload.PayloadKeyServer: &mcpmodel.Server{
			ID:               "srv-001",
			Name:             "server",
			Transport:        "stdio",
			Env:              map[string]string{"TOKEN": "secret", "EMPTY": ""},
			AllowedTools:     []string{"repo.summary"},
			AllowedResources: []string{"workspace://acceptance"},
			AllowedPrompts:   []string{"workorch.acceptance"},
		},
	}, recordpayload.PayloadKeyServer)
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
		recordpayload.PayloadKeyCapabilities: []*mcpmodel.Capability{
			{ID: "cap-001", ServerID: "srv-001", Kind: "tool", Name: "repo.summary", Enabled: true},
		},
	}, recordpayload.PayloadKeyCapabilities)
	if err != nil {
		t.Fatalf("MCPCapabilitiesFromPayload err = %v", err)
	}
	if len(caps) != 1 || caps[0].ID != "cap-001" || caps[0].ServerID != "srv-001" || !caps[0].Enabled {
		t.Fatalf("MCPCapabilitiesFromPayload = %#v", caps)
	}
}

func TestMemoryItemsFromPayloadAndMapping(t *testing.T) {
	items, err := MemoryItemsFromPayload(map[string]any{
		recordpayload.PayloadKeyMemoryItems: []*memorymodel.MemoryItem{
			{
				ID:          "mem-001",
				WorkspaceID: "ws-001",
				MemoryType:  "fact",
				Summary:     "summary",
				Vector:      []float64{0.1, 0.2},
				VectorScore: 0.75,
			},
		},
	}, recordpayload.PayloadKeyMemoryItems)
	if err != nil {
		t.Fatalf("MemoryItemsFromPayload err = %v", err)
	}

	views := MapMemoryItems(items)
	if len(views) != 1 || views[0].ID != "mem-001" || !views[0].VectorReady || views[0].VectorDimension != 2 || views[0].VectorScore != 0.75 {
		t.Fatalf("MapMemoryItems = %#v", views)
	}

	if _, err := MemoryItemsFromPayload(map[string]any{recordpayload.PayloadKeyMemoryItems: []any{}}, recordpayload.PayloadKeyMemoryItems); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("MemoryItemsFromPayload bad err = %v", err)
	}
}
