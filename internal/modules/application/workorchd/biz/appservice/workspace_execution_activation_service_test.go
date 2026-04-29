package appservice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	memorycommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestActivateWorkspaceExecutionWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := ActivateWorkspaceExecutionWithSession(context.Background(), nil, "ws-activation", ActivateWorkspaceExecutionCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestActivateWorkspaceExecutionWithBackendCreatesAndStartsRun(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-start", "activation start workspace", "", "workspaces/activation-start")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "activation goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"activation dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if !strings.HasPrefix(item.Run.ID, "run-") ||
		item.Run.WorkspaceID != workspace.ID ||
		item.Run.GoalID != goal.ID ||
		item.Run.DoDID != dod.ID ||
		item.Run.State != "failed" {
		t.Fatalf("activation run = %+v, want generated failed llm run with goal/dod", item.Run)
	}
	if item.Snapshot.Workspace.ID != workspace.ID || len(item.Snapshot.Runs) != 1 {
		t.Fatalf("activation snapshot = %+v, want workspace with one run", item.Snapshot)
	}
	compressed := runCompressedContext(t, item.Run)
	if compressed["version"] != "workorch.compressedContext.v1" || compressed["mode"] != "deterministic_recovery_summary" {
		t.Fatalf("compressedContext = %+v, want deterministic recovery summary", compressed)
	}
	if compressed["runId"] != item.Run.ID || compressed["workspaceId"] != workspace.ID {
		t.Fatalf("compressedContext identity = %+v, want activation run/workspace", compressed)
	}
	runSnapshot, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, item.Run.ID)
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	if len(runSnapshot.Steps) != 1 || runSnapshot.Steps[0].Kind != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("steps = %+v, want first llm workspace execution step", runSnapshot.Steps)
	}
	stepContext, _ := runSnapshot.Steps[0].Input["executionContext"].(map[string]any)
	if _, ok := stepContext["compressedContext"].(map[string]any); !ok {
		t.Fatalf("first step input = %+v, want compressed execution context propagated", runSnapshot.Steps[0].Input)
	}
	if stage, _ := runSnapshot.Steps[0].Output["stage"].(string); stage != "llm_provider_unavailable" {
		t.Fatalf("step output = %+v, want llm provider unavailable stage", runSnapshot.Steps[0].Output)
	}
	if len(runSnapshot.ToolCalls) != 1 || runSnapshot.ToolCalls[0].State != "failed" {
		t.Fatalf("toolCalls = %+v, want failed runtime evidence", runSnapshot.ToolCalls)
	}
	if len(runSnapshot.ValidationResults) != 1 || runSnapshot.ValidationResults[0].State != "failed" {
		t.Fatalf("validationResults = %+v, want failed runtime evidence", runSnapshot.ValidationResults)
	}
	if len(runSnapshot.RuntimeCheckpoints) == 0 {
		t.Fatalf("runtimeCheckpoints empty, want runtime checkpoints")
	}
	if len(runSnapshot.Patches) != 0 {
		t.Fatalf("patches = %+v, want no patch evidence until a real patch is applied", runSnapshot.Patches)
	}
}

func TestActivateWorkspaceExecutionWithBackendRequiresGoalAndDoD(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-missing", "activation missing workspace", "", "workspaces/activation-missing")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	if _, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument when goal and DoD are missing", err)
	}
}

func TestActivateWorkspaceExecutionWithBackendRequiresExplicitDefinitionWhenAmbiguous(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-ambiguous", "activation ambiguous workspace", "", "workspaces/activation-ambiguous")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	firstGoal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "first activation goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend first: %v", err)
	}
	if _, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "second activation goal"); err != nil {
		t.Fatalf("DefineGoalWithBackend second: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"activation dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}

	if _, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err = %v, want conflict when goal selection is ambiguous", err)
	}
	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: firstGoal.ID,
		DoDID:  dod.ID,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend explicit: %v", err)
	}
	if item.Run.GoalID != firstGoal.ID || item.Run.DoDID != dod.ID || item.Run.State != "failed" {
		t.Fatalf("activation run = %+v, want explicit goal/dod failed through llm provider validation", item.Run)
	}
}

func TestActivateWorkspaceExecutionWithBackendBindsSelectedContext(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-activation-context", "activation context workspace", "", "workspaces/activation-context")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "activation context goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"activation context dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateMCPServerWithBackend(ctx, "file", dataRoot, CreateMCPServerCommand{
		ID:           "mcp-activation-context",
		Name:         "Activation Context MCP",
		Transport:    "stdio",
		Enabled:      true,
		AllowedTools: []string{"repo.summary"},
	}); err != nil {
		t.Fatalf("CreateMCPServerWithBackend: %v", err)
	}
	if _, err := CreateSkillWithBackend(ctx, "file", dataRoot, CreateSkillCommand{
		ID:                    "skill-activation-context",
		Name:                  "Activation Context Skill",
		Version:               "1.0.0",
		Instruction:           "Use local repository evidence.",
		RequiredToolKinds:     []string{"shell/command"},
		RequiredMCPServerRefs: []string{"mcp-activation-context"},
	}); err != nil {
		t.Fatalf("CreateSkillWithBackend: %v", err)
	}
	if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
		ID:                   "soul-activation-context",
		Name:                 "Activation Context Soul",
		SystemInstruction:    "Execute workspace goals with evidence.",
		AllowedSkillRefs:     []string{"skill-activation-context"},
		AllowedMCPServerRefs: []string{"mcp-activation-context"},
		AllowedToolKinds:     []string{"shell/command"},
	}); err != nil {
		t.Fatalf("CreateSoulWithBackend: %v", err)
	}
	memory, err := CreateMemoryItemWithBackend(ctx, "file", dataRoot, workspace.ID, CreateMemoryItemCommand{
		MemoryID:   "memory-activation-context",
		SourceType: memorycommon.MemorySourceUser,
		MemoryType: memorycommon.MemoryTypeProjectContext,
		Summary:    "workspace should use LLM activation context",
		Content:    "Use selected soul, skill, MCP server, and memory as planning context.",
	})
	if err != nil {
		t.Fatalf("CreateMemoryItemWithBackend: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID:        goal.ID,
		DoDID:         dod.ID,
		LLMProviderID: "reasoning",
		SoulID:        "soul-activation-context",
		SkillIDs:      []string{"skill-activation-context"},
		MCPServerIDs:  []string{"mcp-activation-context"},
		MemoryIDs:     []string{memory.ID},
		Priority:      7,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.Priority != 7 {
		t.Fatalf("run priority = %d, want 7", item.Run.Priority)
	}
	executionContext := item.Run.ExecutionContext
	if executionContext["soulId"] != "soul-activation-context" {
		t.Fatalf("executionContext = %+v, want selected soul", executionContext)
	}
	if executionContext[llmcommon.ExecutionContextLLMProviderID] != "reasoning" {
		t.Fatalf("executionContext = %+v, want selected llm provider", executionContext)
	}
	goalContext, _ := executionContext["goal"].(map[string]any)
	if goalContext["text"] != "activation context goal" {
		t.Fatalf("goal context = %+v, want goal text", executionContext["goal"])
	}
	dodContext, _ := executionContext["dod"].(map[string]any)
	if !contextStringSliceContains(dodContext["items"], "activation context dod") {
		t.Fatalf("dod context = %+v, want DoD items", executionContext["dod"])
	}
	if !contextStringSliceContains(executionContext["skillIds"], "skill-activation-context") {
		t.Fatalf("skillIds = %+v, want selected skill", executionContext["skillIds"])
	}
	if !contextStringSliceContains(executionContext["mcpServerIds"], "mcp-activation-context") {
		t.Fatalf("mcpServerIds = %+v, want selected MCP server", executionContext["mcpServerIds"])
	}
	if !contextStringSliceContains(executionContext["memoryIds"], memory.ID) {
		t.Fatalf("memoryIds = %+v, want selected memory", executionContext["memoryIds"])
	}

	runSnapshot, err := ReadRunSnapshotWithBackend(ctx, "file", dataRoot, item.Run.ID)
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	stepContext, _ := runSnapshot.Steps[0].Input["executionContext"].(map[string]any)
	if stepContext["soulId"] != "soul-activation-context" {
		t.Fatalf("first step input = %+v, want execution context propagated", runSnapshot.Steps[0].Input)
	}
	if stepContext[llmcommon.ExecutionContextLLMProviderID] != "reasoning" {
		t.Fatalf("first step input = %+v, want llm provider propagated", runSnapshot.Steps[0].Input)
	}
}

func TestWorkspaceSoulBindingsWithBackendRoutesExecutionSoulByStage(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-soul-routing", "soul routing workspace", "", "workspaces/soul-routing")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "route soul by workspace stage")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"soul routing is exposed"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	for _, id := range []string{"soul-default", "soul-execution", "soul-decision", "soul-validation"} {
		if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
			ID:                id,
			Name:              id,
			SystemInstruction: "Use " + id,
		}); err != nil {
			t.Fatalf("CreateSoulWithBackend %s: %v", id, err)
		}
	}

	bindings, err := UpdateWorkspaceSoulBindingsWithBackend(ctx, "file", dataRoot, workspace.ID, WorkspaceSoulBindings{
		DefaultSoulID: "soul-default",
		Bindings: map[string]string{
			SoulBindingStageExecution: "soul-execution",
			SoulBindingStageDecision:  "soul-decision",
		},
	})
	if err != nil {
		t.Fatalf("UpdateWorkspaceSoulBindingsWithBackend: %v", err)
	}
	if bindings.DefaultSoulID != "soul-default" ||
		bindings.Bindings[SoulBindingStageExecution] != "soul-execution" ||
		bindings.Bindings[SoulBindingStageDecision] != "soul-decision" {
		t.Fatalf("bindings = %+v, want default/execution/decision souls", bindings)
	}
	readBack, err := ReadWorkspaceSoulBindingsWithBackend(ctx, "file", dataRoot, workspace.ID)
	if err != nil {
		t.Fatalf("ReadWorkspaceSoulBindingsWithBackend: %v", err)
	}
	if readBack.DefaultSoulID != bindings.DefaultSoulID || readBack.Bindings[SoulBindingStageExecution] != "soul-execution" {
		t.Fatalf("readBack = %+v, want persisted bindings", readBack)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: goal.ID,
		DoDID:  dod.ID,
		SoulBindings: map[string]string{
			SoulBindingStageValidation: "soul-validation",
		},
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	routing := contextMapFromValue(t, item.Run.ExecutionContext["soulRouting"])
	if routing["status"] != "resolved" || routing["effectiveDefaultSoulId"] != "soul-default" {
		t.Fatalf("soulRouting = %+v, want resolved workspace default", routing)
	}
	routingBindings := contextMapFromValue(t, routing["bindings"])
	assertSoulRoutingBinding(t, routingBindings, SoulBindingStagePlanning, "soul-default", "workspace_default")
	assertSoulRoutingBinding(t, routingBindings, SoulBindingStageExecution, "soul-execution", "workspace_stage_binding")
	assertSoulRoutingBinding(t, routingBindings, SoulBindingStageDecision, "soul-decision", "workspace_stage_binding")
	assertSoulRoutingBinding(t, routingBindings, SoulBindingStageValidation, "soul-validation", "run_stage_override")
}

func TestActivateWorkspaceExecutionWithBackendUsesSystemDefaultSoul(t *testing.T) {
	useUnavailableLLMProvider(t)
	useSystemDefaultSoulConfig(t, "soul-system-default")
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-system-soul-routing", "system soul routing workspace", "", "workspaces/system-soul-routing")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "use system default soul")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"system default soul is used"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
		ID:                "soul-system-default",
		Name:              "System Default Soul",
		SystemInstruction: "Use system default soul.",
	}); err != nil {
		t.Fatalf("CreateSoulWithBackend: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: goal.ID,
		DoDID:  dod.ID,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ExecutionContext["soulId"] != "soul-system-default" {
		t.Fatalf("executionContext = %+v, want system default soul id", item.Run.ExecutionContext)
	}
	routing := contextMapFromValue(t, item.Run.ExecutionContext["soulRouting"])
	if routing["effectiveDefaultSoulId"] != "soul-system-default" {
		t.Fatalf("soulRouting = %+v, want system default soul", routing)
	}
	routingBindings := contextMapFromValue(t, routing["bindings"])
	assertSoulRoutingBinding(t, routingBindings, SoulBindingStagePlanning, "soul-system-default", "system_default")
	assertSoulRoutingBinding(t, routingBindings, SoulBindingStageValidation, "soul-system-default", "system_default")
}

func TestWorkspaceSoulBindingsWithBackendRejectsInvalidBinding(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-soul-routing-invalid", "soul routing invalid workspace", "", "workspaces/soul-routing-invalid")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	if _, err := UpdateWorkspaceSoulBindingsWithBackend(ctx, "file", dataRoot, workspace.ID, WorkspaceSoulBindings{
		Bindings: map[string]string{"not-a-stage": "soul-missing"},
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument for unsupported stage", err)
	}
	if _, err := UpdateWorkspaceSoulBindingsWithBackend(ctx, "file", dataRoot, workspace.ID, WorkspaceSoulBindings{
		DefaultSoulID: "soul-missing",
	}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("err = %v, want not found for missing soul", err)
	}
}

func TestActivateWorkspaceExecutionWithBackendAutoSelectsSkillAndBindsRequiredMCP(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-auto-skill-context", "auto skill context workspace", "", "workspaces/auto-skill-context")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "complete auto delivery verification")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"auto delivery evidence is produced"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateMCPServerWithBackend(ctx, "file", dataRoot, CreateMCPServerCommand{
		ID:           "mcp-auto-skill",
		Name:         "Auto Skill MCP",
		Transport:    "stdio",
		Enabled:      true,
		AllowedTools: []string{"repo.summary"},
	}); err != nil {
		t.Fatalf("CreateMCPServerWithBackend: %v", err)
	}
	if _, err := CreateSkillWithBackend(ctx, "file", dataRoot, CreateSkillCommand{
		ID:                    "skill-auto-delivery",
		Name:                  "Auto Delivery Skill",
		Version:               "1.0.0",
		Description:           "Skill for auto delivery verification",
		TriggerRules:          []string{"auto delivery"},
		Instruction:           "Use this skill when Goal and DoD request auto delivery.",
		RequiredToolKinds:     []string{"llm/chat"},
		RequiredMCPServerRefs: []string{"mcp-auto-skill"},
	}); err != nil {
		t.Fatalf("CreateSkillWithBackend: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID:        goal.ID,
		DoDID:         dod.ID,
		LLMProviderID: "reasoning",
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	executionContext := item.Run.ExecutionContext
	if !contextStringSliceContains(executionContext["skillIds"], "skill-auto-delivery") {
		t.Fatalf("skillIds = %+v, want auto selected skill", executionContext["skillIds"])
	}
	if !contextStringSliceContains(executionContext["mcpServerIds"], "mcp-auto-skill") {
		t.Fatalf("mcpServerIds = %+v, want required MCP auto bound", executionContext["mcpServerIds"])
	}
	selectionPolicy, selectionStatus := skillSelectionPolicyAndStatus(executionContext["skillSelection"])
	if selectionPolicy != "goal_dod_auto_match" || selectionStatus != "selected" {
		t.Fatalf("skillSelection = %+v, want auto selected evidence", executionContext["skillSelection"])
	}
	mcpSelection, _ := executionContext["mcpSelection"].(map[string]any)
	if !contextStringSliceContains(mcpSelection["autoBoundMCPServerIds"], "mcp-auto-skill") {
		t.Fatalf("mcpSelection = %+v, want auto bound MCP evidence", mcpSelection)
	}
}

func TestActivateWorkspaceExecutionWithBackendUsesSemanticMemoryRecall(t *testing.T) {
	ctx := context.Background()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/embeddings":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"embedding":[1,0,0]}]}`))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"completionState\":\"completed\",\"dodAssessment\":{\"satisfied\":true,\"items\":[{\"item\":\"semantic memory selected\",\"state\":\"satisfied\",\"evidenceRefs\":[\"memory-semantic-match\"]}]}}"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	useActivationLLMConfig(t, provider.URL+"/v1", "local")
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-activation-semantic-memory", "semantic memory workspace", "", "workspaces/activation-semantic-memory")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "use semantic project context")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"semantic memory selected"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateMemoryItemWithBackend(ctx, "file", dataRoot, workspace.ID, CreateMemoryItemCommand{
		MemoryID:            "memory-semantic-match",
		SourceType:          memorycommon.MemorySourceUser,
		MemoryType:          memorycommon.MemoryTypeProjectContext,
		Summary:             "matching semantic project context",
		Content:             "This memory should be retrieved by vector similarity.",
		EmbeddingProviderID: "local",
		EmbeddingModel:      "embedding-model",
		EmbeddingText:       "matching semantic project context",
		Vector:              []float64{1, 0, 0},
	}); err != nil {
		t.Fatalf("CreateMemoryItemWithBackend match: %v", err)
	}
	if _, err := CreateMemoryItemWithBackend(ctx, "file", dataRoot, workspace.ID, CreateMemoryItemCommand{
		MemoryID:            "memory-semantic-other",
		SourceType:          memorycommon.MemorySourceUser,
		MemoryType:          memorycommon.MemoryTypeProjectContext,
		Summary:             "unrelated semantic project context",
		Content:             "This memory should not pass the vector score threshold.",
		EmbeddingProviderID: "local",
		EmbeddingModel:      "embedding-model",
		EmbeddingText:       "unrelated semantic project context",
		Vector:              []float64{0, 1, 0},
	}); err != nil {
		t.Fatalf("CreateMemoryItemWithBackend other: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: goal.ID,
		DoDID:  dod.ID,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if !contextStringSliceContains(item.Run.ExecutionContext["memoryIds"], "memory-semantic-match") ||
		contextStringSliceContains(item.Run.ExecutionContext["memoryIds"], "memory-semantic-other") {
		t.Fatalf("memoryIds = %+v, want only semantic match", item.Run.ExecutionContext["memoryIds"])
	}
	retrieval, _ := item.Run.ExecutionContext["memoryRetrieval"].(map[string]any)
	if retrieval["policy"] != "semantic_vector_search" ||
		retrieval["status"] != "selected" ||
		retrieval["providerId"] != "local" ||
		!contextNumericEquals(retrieval["vectorDimension"], 3) {
		t.Fatalf("memoryRetrieval = %+v, want semantic evidence", retrieval)
	}
}

func TestActivateWorkspaceExecutionWithBackendReportsSemanticMemoryDegradation(t *testing.T) {
	ctx := context.Background()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"completionState\":\"continue\"}"}}]}`))
	}))
	defer provider.Close()
	useActivationLLMConfig(t, provider.URL+"/v1", "missing-embedding")
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-activation-semantic-degraded", "semantic degradation workspace", "", "workspaces/activation-semantic-degraded")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "use semantic memory degradation")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"semantic degradation visible"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateMemoryItemWithBackend(ctx, "file", dataRoot, workspace.ID, CreateMemoryItemCommand{
		MemoryID:   "memory-degraded-fallback-should-not-select",
		SourceType: memorycommon.MemorySourceUser,
		MemoryType: memorycommon.MemoryTypeProjectContext,
		Summary:    "available deterministic fallback",
		Content:    "Configured semantic recall is degraded, so this must not be silently selected.",
	}); err != nil {
		t.Fatalf("CreateMemoryItemWithBackend: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID: goal.ID,
		DoDID:  dod.ID,
	})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ExecutionContext["memoryIds"] != nil {
		t.Fatalf("memoryIds = %+v, want no silent fallback when semantic recall degrades", item.Run.ExecutionContext["memoryIds"])
	}
	retrieval, _ := item.Run.ExecutionContext["memoryRetrieval"].(map[string]any)
	if retrieval["policy"] != "semantic_vector_search" || retrieval["status"] != "degraded" || retrieval["reason"] == "" {
		t.Fatalf("memoryRetrieval = %+v, want explicit degradation evidence", retrieval)
	}
}

func TestActivateWorkspaceExecutionWithBackendRejectsSoulDisallowedSkill(t *testing.T) {
	useUnavailableLLMProvider(t)
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(ctx, "file", dataRoot, "ws-activation-context-reject", "activation context reject workspace", "", "workspaces/activation-context-reject")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	goal, err := DefineGoalWithBackend(ctx, "file", dataRoot, workspace.ID, "activation context reject goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(ctx, "file", dataRoot, workspace.ID, []string{"activation context reject dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	if _, err := CreateSkillWithBackend(ctx, "file", dataRoot, CreateSkillCommand{
		ID:          "skill-disallowed",
		Name:        "Disallowed Skill",
		Version:     "1.0.0",
		Instruction: "Not allowed by selected soul.",
	}); err != nil {
		t.Fatalf("CreateSkillWithBackend: %v", err)
	}
	if _, err := CreateSoulWithBackend(ctx, "file", dataRoot, CreateSoulCommand{
		ID:                "soul-restrictive",
		Name:              "Restrictive Soul",
		SystemInstruction: "Only use explicitly allowed capabilities.",
	}); err != nil {
		t.Fatalf("CreateSoulWithBackend: %v", err)
	}

	_, err = ActivateWorkspaceExecutionWithBackend(ctx, "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{
		GoalID:   goal.ID,
		DoDID:    dod.ID,
		SoulID:   "soul-restrictive",
		SkillIDs: []string{"skill-disallowed"},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument for disallowed skill", err)
	}
}

func TestActivateWorkspaceExecutionWithBackendReturnsActiveRun(t *testing.T) {
	useUnavailableLLMProvider(t)
	dataRoot := t.TempDir()
	workspace, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, "ws-activation-active", "activation active workspace", "", "workspaces/activation-active")
	if err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}
	activeRunID := "run-activation-active-blocked-decision"
	started := prepareDecisionRun(t, dataRoot, workspace.ID, activeRunID)
	if started.State != "blocked_on_decision" {
		t.Fatalf("started run = %+v, want blocked_on_decision", started)
	}
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := session.Bootstrap.Exports()
	workspaceItem, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspace.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspaceItem.ActiveRunID = common.RunID(activeRunID)
	if err := exports.Repositories.Workspaces.Update(ctx, workspaceItem); err != nil {
		t.Fatalf("update workspace active run: %v", err)
	}

	item, err := ActivateWorkspaceExecutionWithBackend(context.Background(), "file", dataRoot, workspace.ID, ActivateWorkspaceExecutionCommand{})
	if err != nil {
		t.Fatalf("ActivateWorkspaceExecutionWithBackend: %v", err)
	}
	if item.Run.ID != activeRunID || item.Run.State != "blocked_on_decision" {
		t.Fatalf("activation run = %+v, want existing active blocked run", item.Run)
	}
	if len(item.Snapshot.Runs) != 1 || item.Snapshot.Workspace.ActiveRunID != activeRunID {
		t.Fatalf("activation snapshot = %+v, want active run projection", item.Snapshot)
	}
}

func contextStringSliceContains(value any, want string) bool {
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if item == want {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if item == want {
				return true
			}
		}
	}
	return false
}

func contextNumericEquals(value any, want float64) bool {
	switch typed := value.(type) {
	case int:
		return float64(typed) == want
	case int64:
		return float64(typed) == want
	case float64:
		return typed == want
	default:
		return false
	}
}

func contextMapFromValue(t *testing.T, value any) map[string]any {
	t.Helper()
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal context value: %v", err)
	}
	var ret map[string]any
	if err := json.Unmarshal(payload, &ret); err != nil {
		t.Fatalf("unmarshal context value: %v", err)
	}
	return ret
}

func assertSoulRoutingBinding(t *testing.T, bindings map[string]any, stage string, soulID string, source string) {
	t.Helper()
	binding := contextMapFromValue(t, bindings[stage])
	if binding["soulId"] != soulID || binding["source"] != source {
		t.Fatalf("binding[%s] = %+v, want %s/%s", stage, binding, soulID, source)
	}
}

func skillSelectionPolicyAndStatus(value any) (string, string) {
	switch typed := value.(type) {
	case skillSelectionEvidence:
		return typed.Policy, typed.Status
	case map[string]any:
		policy, _ := typed["policy"].(string)
		status, _ := typed["status"].(string)
		return policy, status
	default:
		return "", ""
	}
}

func useUnavailableLLMProvider(t *testing.T) {
	t.Helper()
	t.Setenv(llmcommon.EnvProvider, "")
	t.Setenv(llmcommon.EnvBaseURL, "")
	t.Setenv(llmcommon.EnvAPIKey, "")
	t.Setenv(llmcommon.EnvModel, "")
	t.Setenv(llmcommon.EnvDisabled, "")
	t.Setenv(llmcommon.EnvTimeout, "")
}

func useActivationLLMConfig(t *testing.T, baseURL string, embeddingProviderID string) {
	t.Helper()
	configDir := t.TempDir()
	content := strings.ReplaceAll(`
endpointName = "workorchd"

[repository]
backend = "file"

[llm]
defaultProvider = "local"
defaultEmbeddingProvider = "__EMBEDDING_PROVIDER__"

[llm.providers.local]
provider = "compatible"
baseURL = "__BASE_URL__"
apiKey = "secret-value"
model = "test-model"
timeout = "5s"
disabled = false
chatCompletions = true
streaming = false
nonChatOperations = ["embeddings"]
`, "__BASE_URL__", baseURL)
	content = strings.ReplaceAll(content, "__EMBEDDING_PROVIDER__", embeddingProviderID)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write application config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
}

func useSystemDefaultSoulConfig(t *testing.T, defaultSoulID string) {
	t.Helper()
	configDir := t.TempDir()
	content := strings.ReplaceAll(`
endpointName = "workorchd"

[repository]
backend = "file"

[runtime]
defaultSoulId = "__DEFAULT_SOUL_ID__"

[llm]
defaultProvider = "local"

[llm.providers.local]
provider = "compatible"
baseURL = ""
apiKey = ""
model = ""
timeout = "5s"
disabled = false
`, "__DEFAULT_SOUL_ID__", defaultSoulID)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write application config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
}
