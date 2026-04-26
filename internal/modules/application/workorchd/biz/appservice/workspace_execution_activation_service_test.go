package appservice

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	legacycommon "github.com/muidea/workorch/internal/modules/blocks/legacy/pkg/common"
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
	runSnapshot, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, item.Run.ID)
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	if len(runSnapshot.Steps) != 1 || runSnapshot.Steps[0].Kind != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("steps = %+v, want first llm workspace execution step", runSnapshot.Steps)
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
	goal, err := DefineGoalWithBackend(context.Background(), "file", dataRoot, workspace.ID, "active activation goal")
	if err != nil {
		t.Fatalf("DefineGoalWithBackend: %v", err)
	}
	dod, err := DefineDoDWithBackend(context.Background(), "file", dataRoot, workspace.ID, []string{"active activation dod"})
	if err != nil {
		t.Fatalf("DefineDoDWithBackend: %v", err)
	}
	activeRunID := "run-" + legacycommon.RuntimeDecisionBlockRunToken
	if _, err := CreateRunWithBackend(context.Background(), "file", dataRoot, workspace.ID, activeRunID, goal.ID, dod.ID); err != nil {
		t.Fatalf("CreateRunWithBackend: %v", err)
	}
	started, err := StartRunWithBackend(context.Background(), "file", dataRoot, activeRunID)
	if err != nil {
		t.Fatalf("StartRunWithBackend: %v", err)
	}
	if started.State != "blocked_on_decision" {
		t.Fatalf("started run = %+v, want blocked_on_decision", started)
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
