package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	gitmodule "github.com/muidea/workorch/internal/modules/blocks/git"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	mcpblockmodule "github.com/muidea/workorch/internal/modules/blocks/mcp"
	shellmodule "github.com/muidea/workorch/internal/modules/blocks/shell"
	sshmodule "github.com/muidea/workorch/internal/modules/blocks/ssh"
	webmodule "github.com/muidea/workorch/internal/modules/blocks/web"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type testLifecycle struct {
	id          string
	runSeq      *[]string
	teardownSeq *[]string
	runErr      error
	teardownErr error
}

type echoRuntimeAdapter struct{}

func (a echoRuntimeAdapter) ExecuteRuntimeStep(_ context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	return runtimemodel.StepExecutionResult{
		Output: map[string]any{
			"status": "completed",
			"kind":   request.Kind,
		},
	}, nil
}

func (l testLifecycle) ID() string { return l.id }

func (l testLifecycle) Run() error {
	if l.runSeq != nil {
		*l.runSeq = append(*l.runSeq, l.id)
	}
	return l.runErr
}

func (l testLifecycle) Teardown(context.Context) error {
	if l.teardownSeq != nil {
		*l.teardownSeq = append(*l.teardownSeq, l.id)
	}
	return l.teardownErr
}

func TestNewWithBackendMemoryBuildsRuntime(t *testing.T) {
	rt := NewWithBackend("memory", "")
	if rt == nil {
		t.Fatal("expected runtime")
	}
	exports := rt.Exports()

	if err := exports.Services.WorkspaceService.CreateWorkspace(context.Background(), workspacemodule.CreateWorkspaceCommand{
		WorkspaceID:          common.WorkspaceID("ws-bootstrap-memory"),
		Title:                "bootstrap memory backend",
		DeliverableDirectory: "workspaces/ws-bootstrap-memory",
	}); err != nil {
		t.Fatalf("create workspace with memory backend: %v", err)
	}

	got, err := exports.Repositories.Workspaces.Get(context.Background(), common.WorkspaceID("ws-bootstrap-memory"))
	if err != nil {
		t.Fatalf("get workspace with memory backend: %v", err)
	}
	if got.Title != "bootstrap memory backend" {
		t.Fatalf("workspace title mismatch: %s", got.Title)
	}
}

func TestBuildRuntimeAdapterRegistryRegistersAllWorkorchBlockKinds(t *testing.T) {
	registry, err := buildRuntimeAdapterRegistry(
		echoRuntimeAdapter{},
		echoRuntimeAdapter{},
		echoRuntimeAdapter{},
		echoRuntimeAdapter{},
		echoRuntimeAdapter{},
		echoRuntimeAdapter{},
		echoRuntimeAdapter{},
	)
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}

	for _, kind := range []string{
		filemodule.StepKindList,
		filemodule.StepKindRead,
		filemodule.StepKindEdit,
		gitmodule.StepKindRead,
		gitmodule.StepKindWrite,
		shellmodule.StepKindCommand,
		sshmodule.StepKindCommand,
		webmodule.StepKindSearch,
		webmodule.StepKindFetch,
		mcpblockmodule.StepKindToolCall,
		mcpblockmodule.StepKindResourceRead,
		mcpblockmodule.StepKindPromptGet,
		llmmodule.StepKindWorkspaceExecution,
	} {
		result, err := registry.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{Kind: kind})
		if err != nil {
			t.Fatalf("execute registered kind %s: %v", kind, err)
		}
		if result.Output["kind"] != kind {
			t.Fatalf("registered kind %s output = %+v", kind, result.Output)
		}
	}

	result, err := registry.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind: llmcommon.RuntimeBlockKindPatchApply,
		Input: map[string]any{
			"patchType": "file_write",
			"targetRef": "result.txt",
			"content":   "ready\n",
		},
	})
	if err != nil {
		t.Fatalf("execute patch/apply: %v", err)
	}
	if result.Output["stage"] != "patch_apply" {
		t.Fatalf("patch/apply output = %+v, want patch_apply stage", result.Output)
	}
	result, err = registry.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind:                          llmcommon.RuntimeBlockKindPatchPreview,
		WorkspaceDeliverableDirectory: t.TempDir(),
		Input: map[string]any{
			"patchType": "file_write",
			"targetRef": "result.txt",
			"content":   "ready\n",
		},
	})
	if err != nil {
		t.Fatalf("execute patch/preview: %v", err)
	}
	if result.Output["stage"] != "patch_preview" {
		t.Fatalf("patch/preview output = %+v, want patch_preview stage", result.Output)
	}
	result, err = registry.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		Kind: llmcommon.RuntimeBlockKindPatchRevert,
		Input: map[string]any{
			"patchId": "run-1/step-1/patch/1",
		},
	})
	if err != nil {
		t.Fatalf("execute patch/revert: %v", err)
	}
	if result.Output["stage"] != "patch_revert" {
		t.Fatalf("patch/revert output = %+v, want patch_revert stage", result.Output)
	}
}

func TestNewWithBackendFilePersistsWorkspaceData(t *testing.T) {
	dataDir := t.TempDir()
	rt := NewWithBackend("file", dataDir)
	exports := rt.Exports()

	err := exports.Services.WorkspaceService.CreateWorkspace(context.Background(), workspacemodule.CreateWorkspaceCommand{
		WorkspaceID:          common.WorkspaceID("ws-bootstrap-file"),
		Title:                "bootstrap file backend",
		DeliverableDirectory: "workspaces/ws-bootstrap-file",
		InternalDirectory:    filepath.Join(dataDir, "internal", "workspaces", "ws-bootstrap-file"),
	})
	if err != nil {
		t.Fatalf("create workspace with file backend: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dataDir, "internal", "workspaces", "ws-bootstrap-file", "workspace.json")); err != nil {
		t.Fatalf("expected workspace persistence file: %v", err)
	}
}

func TestNewWithBackendFileStoresAvailableArtifactContent(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	rt := NewWithBackend("file", dataDir)
	exports := rt.Exports()

	item := &resultmodel.Artifact{
		ID:           common.ArtifactID("run-bootstrap-artifact/artifact/result-summary"),
		WorkspaceID:  common.WorkspaceID("ws-bootstrap-artifact"),
		RunID:        common.RunID("run-bootstrap-artifact"),
		ResultID:     common.ResultID("run-bootstrap-artifact/result/dod-assessment"),
		ArtifactType: resultcommon.ArtifactTypeText,
		Name:         "Accepted Result Summary",
		Summary:      "Bootstrap artifact summary",
		State:        common.ArtifactStateAvailable,
	}
	if err := exports.Repositories.Artifacts.Create(ctx, item); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	stored, err := exports.Repositories.Artifacts.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("get artifact: %v", err)
	}
	wantURI := "workorch://workspaces/ws-bootstrap-artifact/artifacts/run-bootstrap-artifact/result-summary.txt"
	if stored.URI != wantURI {
		t.Fatalf("artifact uri = %q, want %q", stored.URI, wantURI)
	}
	payload, err := os.ReadFile(filepath.Join(dataDir, "internal", "workspaces", "ws-bootstrap-artifact", "artifacts", "run-bootstrap-artifact", "result-summary.txt"))
	if err != nil {
		t.Fatalf("read stored artifact: %v", err)
	}
	if string(payload) != "Bootstrap artifact summary\n" {
		t.Fatalf("stored artifact payload = %q, want summary", string(payload))
	}
}

func TestArtifactStorageRepositoryCachesRunArtifactDirectory(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	rt := NewWithBackend("file", dataDir)
	exports := rt.Exports()
	artifactRepo, ok := exports.Repositories.Artifacts.(*artifactStorageRepository)
	if !ok {
		t.Fatalf("artifact repository = %T, want artifactStorageRepository", exports.Repositories.Artifacts)
	}
	for _, item := range []*resultmodel.Artifact{
		{
			ID:           common.ArtifactID("run-bootstrap-artifact-cache/artifact/result-summary"),
			WorkspaceID:  common.WorkspaceID("ws-bootstrap-artifact-cache"),
			RunID:        common.RunID("run-bootstrap-artifact-cache"),
			ResultID:     common.ResultID("run-bootstrap-artifact-cache/result/dod-assessment"),
			ArtifactType: resultcommon.ArtifactTypeText,
			Name:         "Accepted Result Summary",
			Summary:      "Cached summary artifact",
			State:        common.ArtifactStateAvailable,
		},
		{
			ID:           common.ArtifactID("run-bootstrap-artifact-cache/artifact/deliverable-manifest"),
			WorkspaceID:  common.WorkspaceID("ws-bootstrap-artifact-cache"),
			RunID:        common.RunID("run-bootstrap-artifact-cache"),
			ResultID:     common.ResultID("run-bootstrap-artifact-cache/result/dod-assessment"),
			ArtifactType: resultcommon.ArtifactTypeReport,
			Name:         "Workspace Deliverable Manifest",
			Summary:      "Cached manifest artifact",
			State:        common.ArtifactStateAvailable,
		},
	} {
		if err := artifactRepo.Create(ctx, item); err != nil {
			t.Fatalf("create artifact %s: %v", item.ID, err)
		}
	}
	artifactRepo.dirCacheMu.Lock()
	defer artifactRepo.dirCacheMu.Unlock()
	if len(artifactRepo.dirCache) != 1 {
		t.Fatalf("artifact dir cache = %+v, want one cached run artifact directory", artifactRepo.dirCache)
	}
}

func TestNewWithBackendFilePartitionsWorkspaceScopedRepositories(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	rt := NewWithBackend("file", dataDir)
	exports := rt.Exports()

	workspaceID := common.WorkspaceID("ws-bootstrap-workspace-partition")
	goalID := common.GoalID("goal-bootstrap-workspace-partition")
	dodID := common.DoDID("dod-bootstrap-workspace-partition")
	memoryID := common.MemoryID("memory-bootstrap-workspace-partition")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "workspace partition",
		DeliverableDirectory: filepath.Join(filepath.Dir(dataDir), ".workorch-workspaces", string(workspaceID)),
		InternalDirectory:    filepath.Join(dataDir, "internal", "workspaces", string(workspaceID)),
		State:                common.WorkspaceStateActive,
		DefaultSoulID:        common.SoulID("soul-default"),
		SoulBindings: map[string]common.SoulID{
			"execution": common.SoulID("soul-execution"),
		},
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Goals.Create(ctx, &workspacemodel.Goal{
		ID:          goalID,
		WorkspaceID: workspaceID,
		Text:        "partitioned goal",
	}); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if err := exports.Repositories.DoDs.Create(ctx, &workspacemodel.DoD{
		ID:          dodID,
		WorkspaceID: workspaceID,
		Items:       []string{"partitioned dod"},
		State:       common.DoDStateReady,
	}); err != nil {
		t.Fatalf("create dod: %v", err)
	}
	if err := exports.Repositories.MemoryItems.Create(ctx, &memorymodel.MemoryItem{
		ID:          memoryID,
		WorkspaceID: workspaceID,
		MemoryType:  "workspace_note",
		Content:     "partitioned memory",
		Importance:  7,
	}); err != nil {
		t.Fatalf("create memory item: %v", err)
	}

	workspaceRoot := filepath.Join(dataDir, "internal", "workspaces", string(workspaceID))
	for _, fileName := range []string{"workspace.json", "goals.json", "dods.json", "memory_item.json"} {
		if _, err := os.Stat(filepath.Join(workspaceRoot, fileName)); err != nil {
			t.Fatalf("expected workspace-scoped %s: %v", fileName, err)
		}
		if _, err := os.Stat(filepath.Join(dataDir, fileName)); !os.IsNotExist(err) {
			t.Fatalf("unexpected root-scoped %s err=%v", fileName, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dataDir, "system", "workspace_index.json")); err != nil {
		t.Fatalf("expected workspace index: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "config", "soul_bindings.json")); err != nil {
		t.Fatalf("expected workspace soul bindings config: %v", err)
	}
	assertWorkspaceSoulBindingsStoredOutsideWorkspaceJSON(t, workspaceRoot, workspaceID, "soul-default", "soul-execution")
	indexPayload, err := os.ReadFile(filepath.Join(dataDir, "system", "workspace_index.json"))
	if err != nil {
		t.Fatalf("read workspace index: %v", err)
	}
	var initialIndex workspaceIndex
	if err := json.Unmarshal(indexPayload, &initialIndex); err != nil {
		t.Fatalf("decode workspace index: %v", err)
	}
	if !workspaceIndexContains(initialIndex, workspaceID, common.WorkspaceStateActive, "") {
		t.Fatalf("workspace index = %+v, want active workspace entry", initialIndex)
	}
	workspaces, err := exports.Repositories.Workspaces.List(ctx)
	if err != nil || len(workspaces) != 1 || workspaces[0].ID != workspaceID {
		t.Fatalf("workspaces = %+v, err=%v; want %s", workspaces, err, workspaceID)
	}
	if workspaces[0].DefaultSoulID != "soul-default" || workspaces[0].SoulBindings["execution"] != "soul-execution" {
		t.Fatalf("workspace soul bindings = %+v/%+v, want persisted config values", workspaces[0].DefaultSoulID, workspaces[0].SoulBindings)
	}
	updated := *workspaces[0]
	updated.State = common.WorkspaceStateRunning
	updated.ActiveRunID = common.RunID("run-bootstrap-index-active")
	updated.DefaultSoulID = common.SoulID("soul-updated")
	updated.SoulBindings = map[string]common.SoulID{
		"validation": common.SoulID("soul-validation"),
	}
	if err := exports.Repositories.Workspaces.Update(ctx, &updated); err != nil {
		t.Fatalf("update workspace: %v", err)
	}
	assertWorkspaceSoulBindingsStoredOutsideWorkspaceJSON(t, workspaceRoot, workspaceID, "soul-updated", "soul-validation")
	indexPayload, err = os.ReadFile(filepath.Join(dataDir, "system", "workspace_index.json"))
	if err != nil {
		t.Fatalf("read updated workspace index: %v", err)
	}
	var updatedIndex workspaceIndex
	if err := json.Unmarshal(indexPayload, &updatedIndex); err != nil {
		t.Fatalf("decode updated workspace index: %v", err)
	}
	if !workspaceIndexContains(updatedIndex, workspaceID, common.WorkspaceStateRunning, common.RunID("run-bootstrap-index-active")) {
		t.Fatalf("updated workspace index = %+v, want running workspace with active run", updatedIndex)
	}
	goals, err := exports.Repositories.Goals.ListByWorkspaceID(ctx, workspaceID)
	if err != nil || len(goals) != 1 || goals[0].ID != goalID {
		t.Fatalf("goals = %+v, err=%v; want %s", goals, err, goalID)
	}
	memories, err := exports.Repositories.MemoryItems.Search(ctx, repository.MemorySearchQuery{Query: "partitioned", Limit: 1})
	if err != nil || len(memories) != 1 || memories[0].ID != memoryID {
		t.Fatalf("memories = %+v, err=%v; want %s", memories, err, memoryID)
	}
}

func assertWorkspaceSoulBindingsStoredOutsideWorkspaceJSON(t *testing.T, workspaceRoot string, workspaceID common.WorkspaceID, wantDefault common.SoulID, wantBinding common.SoulID) {
	t.Helper()

	workspacePayload, err := os.ReadFile(filepath.Join(workspaceRoot, "workspace.json"))
	if err != nil {
		t.Fatalf("read workspace metadata: %v", err)
	}
	var workspaces map[common.WorkspaceID]workspacemodel.Workspace
	if err := json.Unmarshal(workspacePayload, &workspaces); err != nil {
		t.Fatalf("decode workspace metadata: %v", err)
	}
	workspace := workspaces[workspaceID]
	if workspace.DefaultSoulID != "" || len(workspace.SoulBindings) != 0 {
		t.Fatalf("workspace metadata contains soul bindings: %+v", workspace)
	}

	bindingsPayload, err := os.ReadFile(filepath.Join(workspaceRoot, "config", "soul_bindings.json"))
	if err != nil {
		t.Fatalf("read workspace soul bindings: %v", err)
	}
	var bindings workspaceSoulBindingsFile
	if err := json.Unmarshal(bindingsPayload, &bindings); err != nil {
		t.Fatalf("decode workspace soul bindings: %v", err)
	}
	if bindings.DefaultSoulID != wantDefault {
		t.Fatalf("defaultSoulId = %s, want %s", bindings.DefaultSoulID, wantDefault)
	}
	found := false
	for _, value := range bindings.Bindings {
		if value == wantBinding {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("bindings = %+v, want value %s", bindings.Bindings, wantBinding)
	}
}

func workspaceIndexContains(index workspaceIndex, workspaceID common.WorkspaceID, state common.WorkspaceState, activeRunID common.RunID) bool {
	for _, item := range index.Workspaces {
		if item.WorkspaceID == workspaceID && item.State == state && item.ActiveRunID == activeRunID && item.InternalRoot == filepath.ToSlash(filepath.Join("internal", "workspaces", string(workspaceID))) {
			return true
		}
	}
	return false
}

func TestNewWithBackendFileStoresSystemRepositoriesUnderSystemDirectory(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	rt := NewWithBackend("file", dataDir)
	exports := rt.Exports()

	if err := exports.Repositories.Souls.Create(ctx, &soulmodel.Soul{ID: common.SoulID("soul-bootstrap-system"), Name: "system soul", SoulMD: "# System Soul\n\nUse system soul.\n"}); err != nil {
		t.Fatalf("create soul: %v", err)
	}
	if err := exports.Repositories.Skills.Create(ctx, &skillmodel.Skill{ID: common.SkillID("skill-bootstrap-system"), Name: "system skill"}); err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if err := exports.Repositories.MCPServers.Create(ctx, &mcpmodel.Server{ID: common.MCPServerID("mcp-bootstrap-system"), Name: "system mcp"}); err != nil {
		t.Fatalf("create mcp server: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dataDir, "system", "souls", "soul-bootstrap-system", "soul.md")); err != nil {
		t.Fatalf("expected soul.md definition: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "system", "souls", "soul-bootstrap-system", "metadata.json")); err != nil {
		t.Fatalf("expected soul metadata: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "system", "soul.json")); !os.IsNotExist(err) {
		t.Fatalf("unexpected root soul.json err=%v", err)
	}
	for _, fileName := range []string{"skill.json", "mcp_server.json"} {
		if _, err := os.Stat(filepath.Join(dataDir, "system", fileName)); err != nil {
			t.Fatalf("expected system-scoped %s: %v", fileName, err)
		}
		if _, err := os.Stat(filepath.Join(dataDir, fileName)); !os.IsNotExist(err) {
			t.Fatalf("unexpected root-scoped %s err=%v", fileName, err)
		}
	}
}

func TestNewWithBackendFilePartitionsRunScopedRepositories(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	rt := NewWithBackend("file", dataDir)
	exports := rt.Exports()

	workspaceID := common.WorkspaceID("ws-bootstrap-partition")
	runID := common.RunID("run-bootstrap-partition")
	stepID := common.StepID("step-bootstrap-partition")
	toolCallID := common.ToolCallID("tool-bootstrap-partition")
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, State: common.StepStateDispatched}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := exports.Repositories.ToolCalls.Create(ctx, &runmodel.ToolCall{
		ID:          toolCallID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		State:       common.ToolCallStateCompleted,
	}); err != nil {
		t.Fatalf("create tool call: %v", err)
	}
	if err := exports.Repositories.Patches.Create(ctx, &runmodel.Patch{
		ID:          common.PatchID("patch-bootstrap-partition"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		State:       common.PatchStateApplied,
	}); err != nil {
		t.Fatalf("create patch: %v", err)
	}
	if err := exports.Repositories.ValidationResults.Create(ctx, &runmodel.ValidationResult{
		ID:          common.ValidationResultID("validation-bootstrap-partition"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		State:       common.ValidationResultStatePassed,
	}); err != nil {
		t.Fatalf("create validation result: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:          common.CheckpointID("checkpoint-bootstrap-partition"),
		WorkspaceID: workspaceID,
		RunID:       runID,
	}); err != nil {
		t.Fatalf("create runtime checkpoint: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:          common.ResultID("result-bootstrap-partition"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		State:       common.ResultStateAccepted,
	}); err != nil {
		t.Fatalf("create result: %v", err)
	}
	if err := exports.Repositories.Artifacts.Create(ctx, &resultmodel.Artifact{
		ID:           common.ArtifactID("artifact-bootstrap-partition"),
		WorkspaceID:  workspaceID,
		RunID:        runID,
		ResultID:     common.ResultID("result-bootstrap-partition"),
		ArtifactType: resultcommon.ArtifactTypeText,
		Summary:      "partition artifact",
		State:        common.ArtifactStateAvailable,
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if err := exports.Repositories.Tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          common.TicketID("ticket-bootstrap-partition"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		State:       common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if err := exports.Repositories.Resolutions.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-bootstrap-partition"),
		TicketID: common.TicketID("ticket-bootstrap-partition"),
		Type:     "provide_missing_info",
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}
	if err := exports.Repositories.Approvals.Create(ctx, &approvalmodel.Approval{
		ID:          common.ApprovalID("approval-bootstrap-partition"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		State:       common.ApprovalStatePending,
	}); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	if err := exports.Repositories.EventRecords.Append(ctx, &eventstreammodel.EventRecord{
		ID:          common.EventRecordID("event-bootstrap-partition"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		OccurredAt:  "2026-04-29T00:00:00Z",
	}); err != nil {
		t.Fatalf("append event record: %v", err)
	}

	runRoot := filepath.Join(dataDir, "internal", "workspaces", string(workspaceID), "runs", string(runID))
	for _, fileName := range []string{
		"run.json",
	} {
		if _, err := os.Stat(filepath.Join(runRoot, fileName)); err != nil {
			t.Fatalf("expected run-scoped %s: %v", fileName, err)
		}
		if _, err := os.Stat(filepath.Join(dataDir, fileName)); !os.IsNotExist(err) {
			t.Fatalf("unexpected root-scoped %s err=%v", fileName, err)
		}
	}
	if _, err := os.Stat(filepath.Join(runRoot, "steps", string(runID), "001-step-bootstrap-partition", "metadata.json")); err != nil {
		t.Fatalf("expected item-scoped step metadata: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runRoot, "tool-calls", string(runID), "001-tool-bootstrap-partition", "metadata.json")); err != nil {
		t.Fatalf("expected item-scoped tool call metadata: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runRoot, "validations", string(runID), "001-validation-bootstrap-partition", "metadata.json")); err != nil {
		t.Fatalf("expected item-scoped validation metadata: %v", err)
	}
	for _, item := range []struct {
		dir string
		id  string
	}{
		{dir: "patches", id: "patch-bootstrap-partition"},
		{dir: "checkpoints", id: "checkpoint-bootstrap-partition"},
		{dir: "results", id: "result-bootstrap-partition"},
		{dir: "artifacts", id: "artifact-bootstrap-partition"},
		{dir: filepath.Join("decisions", "tickets"), id: "ticket-bootstrap-partition"},
		{dir: filepath.Join("decisions", "resolutions"), id: "resolution-bootstrap-partition"},
		{dir: "approvals", id: "approval-bootstrap-partition"},
	} {
		if _, err := os.Stat(filepath.Join(runRoot, item.dir, filepath.FromSlash(item.id)+".json")); err != nil {
			t.Fatalf("expected item-scoped %s %s: %v", item.dir, item.id, err)
		}
	}
	for _, legacyFileName := range []string{"step.json", "tool_call.json", "patch.json", "validation_result.json", "runtime_checkpoint.json", "result.json", "artifact.json", "ticket.json", "resolution.json", "approval.json"} {
		if _, err := os.Stat(filepath.Join(runRoot, legacyFileName)); !os.IsNotExist(err) {
			t.Fatalf("unexpected legacy run-scoped %s err=%v", legacyFileName, err)
		}
	}
	if _, err := os.Stat(filepath.Join(runRoot, "events", "events-000001.jsonl")); err != nil {
		t.Fatalf("expected run-scoped event JSONL segment: %v", err)
	}
	if steps, err := exports.Repositories.Steps.ListByRun(ctx, runID); err != nil || len(steps) != 1 {
		t.Fatalf("list run steps = %v, %v; want one", steps, err)
	}
	if events, err := exports.Repositories.EventRecords.List(ctx, repository.EventRecordQuery{RunID: runID}); err != nil || len(events) != 1 {
		t.Fatalf("list run events = %v, %v; want one", events, err)
	}
}

func TestNewWithBackendFileUsesDefaultDataRootWhenUnset(t *testing.T) {
	tempRoot := t.TempDir()
	defaultDataDir := filepath.Join(tempRoot, "workorch-data")
	t.Setenv("TMPDIR", tempRoot)

	rt := NewWithBackend("file", "")
	exports := rt.Exports()

	err := exports.Services.WorkspaceService.CreateWorkspace(context.Background(), workspacemodule.CreateWorkspaceCommand{
		WorkspaceID:          common.WorkspaceID("ws-bootstrap-file-default"),
		Title:                "bootstrap file backend default data root",
		DeliverableDirectory: "workspaces/ws-bootstrap-file-default",
		InternalDirectory:    filepath.Join(defaultDataDir, "internal", "workspaces", "ws-bootstrap-file-default"),
	})
	if err != nil {
		t.Fatalf("create workspace with default file backend: %v", err)
	}

	if _, err := os.Stat(filepath.Join(defaultDataDir, "internal", "workspaces", "ws-bootstrap-file-default", "workspace.json")); err != nil {
		t.Fatalf("expected workspace persistence file in default data dir: %v", err)
	}
}

func TestModulePluginLifecycleRunsAndShutsDownInOrder(t *testing.T) {
	var runSeq []string
	var teardownSeq []string
	lifecycle, err := newModulePluginLifecycle([]lifecycle{
		testLifecycle{id: "first", runSeq: &runSeq, teardownSeq: &teardownSeq},
		testLifecycle{id: "second", runSeq: &runSeq, teardownSeq: &teardownSeq},
	})
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}

	if err := lifecycle.Run(); err != nil {
		t.Fatalf("run lifecycle: %v", err)
	}
	if got, want := runSeq, []string{"first", "second"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("run seq = %v, want %v", got, want)
	}

	if err := lifecycle.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown lifecycle: %v", err)
	}
	if got, want := teardownSeq, []string{"second", "first"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("teardown seq = %v, want %v", got, want)
	}
}

func TestModulePluginLifecycleReturnsRunAndShutdownErrors(t *testing.T) {
	errRun := errors.New("run failed")
	pluginLifecycle, err := newModulePluginLifecycle([]lifecycle{
		testLifecycle{id: "broken-run", runErr: errRun},
	})
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}
	if err := pluginLifecycle.Run(); err == nil || !strings.Contains(err.Error(), errRun.Error()) {
		t.Fatalf("run err = %v, want %v", err, errRun)
	}

	errShutdown := errors.New("shutdown failed")
	pluginLifecycle, err = newModulePluginLifecycle([]lifecycle{
		testLifecycle{id: "broken-shutdown", teardownErr: errShutdown},
	})
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}
	if err := pluginLifecycle.Shutdown(context.Background()); !errors.Is(err, errShutdown) {
		t.Fatalf("shutdown err = %v, want %v", err, errShutdown)
	}
}
