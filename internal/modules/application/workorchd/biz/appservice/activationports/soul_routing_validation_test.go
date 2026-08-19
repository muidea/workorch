package activationports

import (
	"errors"
	"slices"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceExecutionSoulUsesExecutionBindingInsteadOfDefault(t *testing.T) {
	defaultSoul := &soulmodel.Soul{ID: "soul-default", AllowedSkillRefs: []string{"skill-default"}}
	executionSoul := &soulmodel.Soul{ID: "soul-execution", AllowedSkillRefs: []string{"skill-execution"}}
	got := workspaceExecutionSoul(defaultSoul, map[string]*soulmodel.Soul{appmodelcore.SoulBindingStageExecution: executionSoul})
	if got != executionSoul || slices.Contains(got.AllowedSkillRefs, "skill-default") {
		t.Fatalf("execution Soul = %+v, want stage binding", got)
	}
	if got := workspaceExecutionSoul(defaultSoul, nil); got != defaultSoul {
		t.Fatalf("fallback Soul = %+v, want default", got)
	}
}

func TestSelectedMCPCapabilityOwnershipRequiresBoundServerAndCategory(t *testing.T) {
	valid := WorkspaceMCPSelectionResult{
		MCPServerIDs:  []string{"mcp-a"},
		CapabilityIDs: []string{"cap-a"},
		Capabilities:  []*mcpmodel.Capability{{ID: "cap-a", ServerID: "mcp-a", Kind: "tool", Category: "repository"}},
	}
	if err := validateSelectedMCPCapabilityOwnership(valid); err != nil {
		t.Fatalf("valid ownership: %v", err)
	}
	wrongServer := valid
	wrongServer.Capabilities = []*mcpmodel.Capability{{ID: "cap-a", ServerID: "mcp-b", Kind: "tool", Category: "repository"}}
	if err := validateSelectedMCPCapabilityOwnership(wrongServer); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong server err = %v, want conflict", err)
	}
	missingCategory := valid
	missingCategory.Capabilities = []*mcpmodel.Capability{{ID: "cap-a", ServerID: "mcp-a", Kind: "tool"}}
	if err := validateSelectedMCPCapabilityOwnership(missingCategory); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("missing category err = %v, want conflict", err)
	}
	if got := runtimeMCPCapabilityToolKinds(valid.Capabilities); !slices.Equal(got, []string{"mcp/tool_call"}) {
		t.Fatalf("tool kinds = %#v", got)
	}
}
