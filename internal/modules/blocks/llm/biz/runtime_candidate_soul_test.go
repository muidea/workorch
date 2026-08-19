package llm

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func TestRuntimeCandidateUsesStageSoulToFilterSharedActivationContext(t *testing.T) {
	skills, budget := sealedSoulFilterSkills(t,
		runtimecontract.RuntimeSkillContext{ID: "skill-execution", Name: "execution", Instruction: "execute"},
		runtimecontract.RuntimeSkillContext{ID: "skill-validation", Name: "validation", Instruction: "validate"},
	)
	activation := runtimecontract.RuntimeActivationContext{
		DefaultSoul: &runtimecontract.RuntimeSoulContext{ID: "soul-default", AllowedSkillRefs: []string{"*"}},
		StageSoulBindings: []runtimecontract.RuntimeStageSoulBinding{
			{Stage: "execution", Soul: runtimecontract.RuntimeSoulContext{ID: "soul-execution", AllowedSkillRefs: []string{"skill-execution"}}},
			{Stage: "validation", Soul: runtimecontract.RuntimeSoulContext{ID: "soul-validation", AllowedSkillRefs: []string{"skill-validation"}}},
		},
		Skills: skills, SkillContextBudget: budget,
	}.Normalized()

	execution := runtimeCandidateActivationForStage(activation, "execution", runtimecontract.ConvergenceStateNeedStateChange)
	if len(execution.Skills) != 1 || execution.Skills[0].ID != "skill-execution" || execution.SkillContextBudget == nil || execution.SkillContextBudget.OriginalInstructionBytes != len("execute") {
		t.Fatalf("execution projection = %+v budget=%+v", execution.Skills, execution.SkillContextBudget)
	}
	validation := runtimeCandidateActivationForStage(activation, "validation", runtimecontract.ConvergenceStateNeedValidation)
	if len(validation.Skills) != 1 || validation.Skills[0].ID != "skill-validation" || validation.SkillContextBudget == nil || validation.SkillContextBudget.OriginalInstructionBytes != len("validate") {
		t.Fatalf("validation projection = %+v budget=%+v", validation.Skills, validation.SkillContextBudget)
	}
}

func TestRuntimeCandidateSoulFiltersMCPIdentityCategoryAndToolKind(t *testing.T) {
	soul := &runtimecontract.RuntimeSoulContext{
		ID: "soul-execution", AllowedMCPServerRefs: []string{"mcp-a"}, AllowedCapabilityRefs: []string{"cap-a"},
		AllowedCapabilityCategories: []string{"repository"}, AllowedToolKinds: []string{"mcp/tool_call"},
	}
	values := []runtimecontract.RuntimeMCPCapabilityContext{
		{ID: "cap-a", ServerID: "mcp-a", Category: "repository", OperationKind: "mcp/tool_call"},
		{ID: "cap-wrong-server", ServerID: "mcp-b", Category: "repository", OperationKind: "mcp/tool_call"},
		{ID: "cap-wrong-category", ServerID: "mcp-a", Category: "weather", OperationKind: "mcp/tool_call"},
	}
	got := runtimeCandidateMCPCapabilitiesForSoul(values, soul)
	if len(got) != 1 || got[0].ID != "cap-a" {
		t.Fatalf("filtered capabilities = %+v", got)
	}
	servers := runtimeCandidateMCPServerIDsForSoul([]common.MCPServerID{"mcp-a", "mcp-b"}, soul)
	if len(servers) != 1 || servers[0] != "mcp-a" {
		t.Fatalf("filtered servers = %+v", servers)
	}
}

func sealedSoulFilterSkills(t *testing.T, values ...runtimecontract.RuntimeSkillContext) ([]runtimecontract.RuntimeSkillContext, *runtimecontract.RuntimeSkillContextBudget) {
	t.Helper()
	for idx := range values {
		content := workorchskillcontract.Content{ID: values[idx].ID, Name: values[idx].Name, Instruction: values[idx].Instruction}
		values[idx].Provenance = workorchskillcontract.Provenance{
			Source: workorchskillcontract.SourceWorkOrchRegistry, Scope: workorchskillcontract.ScopeGlobal,
			ContentHash: workorchskillcontract.StableContentHash(content),
		}
	}
	sealed, budget, err := runtimecontract.SealRuntimeSkillContexts(values)
	if err != nil {
		t.Fatal(err)
	}
	return sealed, budget
}
