package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	magictask "github.com/muidea/magicCommon/task"
	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
	gitmodule "github.com/muidea/workorch/internal/modules/blocks/git"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	mcpblockmodule "github.com/muidea/workorch/internal/modules/blocks/mcp"
	"github.com/muidea/workorch/internal/modules/blocks/pkg/runtimeevent"
	shellmodule "github.com/muidea/workorch/internal/modules/blocks/shell"
	sshmodule "github.com/muidea/workorch/internal/modules/blocks/ssh"
	webmodule "github.com/muidea/workorch/internal/modules/blocks/web"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	orchestratormodule "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type testLifecycle struct {
	id          string
	runSeq      *[]string
	teardownSeq *[]string
	runErr      error
	teardownErr error
}

type backgroundAwareLifecycle struct {
	id      string
	stopped *bool
}

type echoRuntimeAdapter struct{}

type terminatingEventHub struct {
	terminated bool
}

func (h *terminatingEventHub) Publish(context.Context, events.Envelope) error {
	return nil
}

func (h *terminatingEventHub) Subscribe(string, events.Handler) func() {
	return func() {}
}

func (h *terminatingEventHub) Terminate(context.Context) {
	h.terminated = true
}

func (a echoRuntimeAdapter) ExecuteRuntimeStep(_ context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	return runtimemodel.StepExecutionResult{
		Output: map[string]any{
			"status": "completed",
			"kind":   request.Kind,
		},
	}, nil
}

func (l testLifecycle) ID() string { return l.id }

func (l testLifecycle) Start() error {
	if l.runSeq != nil {
		*l.runSeq = append(*l.runSeq, l.id)
	}
	return l.runErr
}

func (l testLifecycle) Stop(context.Context) error {
	if l.teardownSeq != nil {
		*l.teardownSeq = append(*l.teardownSeq, l.id)
	}
	return l.teardownErr
}

func (l backgroundAwareLifecycle) ID() string { return l.id }

func (l backgroundAwareLifecycle) Start() error { return nil }

func (l backgroundAwareLifecycle) Stop(context.Context) error {
	if l.stopped != nil {
		*l.stopped = true
	}
	return nil
}

func publishTestCommand(ctx context.Context, hub events.Hub, envelope events.Envelope) error {
	err := hub.Publish(ctx, envelope)
	if _, ok := events.ResponseEnvelope(err); ok {
		return nil
	}
	return err
}

func createTestWorkspace(ctx context.Context, exports Exports, workspaceID common.WorkspaceID, title string, internalDirectory string) error {
	return publishTestCommand(ctx, exports.Infrastructure.EventHub, events.Envelope{
		Topic:        workspaceevents.TopicWorkspaceCommandCreate,
		SourceModule: "workorch/bootstrap-test",
		SubjectType:  "workspace",
		SubjectID:    string(workspaceID),
		WorkspaceID:  string(workspaceID),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID:          string(workspaceID),
			events.PayloadKeyTitle:                title,
			events.PayloadKeyDeliverableDirectory: "workspaces/" + string(workspaceID),
			events.PayloadKeyInternalDirectory:    internalDirectory,
		},
	})
}

func createTestRun(ctx context.Context, exports Exports, workspaceID common.WorkspaceID, runID common.RunID) error {
	return publishTestCommand(ctx, exports.Infrastructure.EventHub, events.Envelope{
		Topic:        runevents.TopicRunCommandEnsure,
		SourceModule: "workorch/bootstrap-test",
		SubjectType:  "run",
		SubjectID:    string(runID),
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: string(workspaceID),
			events.PayloadKeyRunID:       string(runID),
		},
	})
}

func createTestApproval(ctx context.Context, exports Exports, workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, approvalID common.ApprovalID) error {
	return publishTestCommand(ctx, exports.Infrastructure.EventHub, events.Envelope{
		Topic:        approvalevents.TopicApprovalCommandEnsure,
		SourceModule: "workorch/bootstrap-test",
		SubjectType:  "approval",
		SubjectID:    string(approvalID),
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		StepID:       string(stepID),
		Payload: map[string]any{
			events.PayloadKeyApprovalID:  string(approvalID),
			events.PayloadKeyWorkspaceID: string(workspaceID),
			events.PayloadKeyRunID:       string(runID),
			events.PayloadKeyStepID:      string(stepID),
			events.PayloadKeyReason:      "test approval gate",
		},
	})
}

func blockTestRunOnApproval(ctx context.Context, exports Exports, runID common.RunID, approvalID common.ApprovalID) error {
	return publishTestCommand(ctx, exports.Infrastructure.EventHub, events.Envelope{
		Topic:        orchestratorevents.TopicOrchestratorCommandBlockApproval,
		SourceModule: "workorch/bootstrap-test",
		SubjectType:  "run",
		SubjectID:    string(runID),
		RunID:        string(runID),
		Payload: map[string]any{
			events.PayloadKeyRunID:              string(runID),
			events.PayloadKeyBlockingApprovalID: string(approvalID),
		},
	})
}

func TestNewWithBackendMemoryBuildsRuntime(t *testing.T) {
	rt := NewWithBackend("memory", "")
	if rt == nil {
		t.Fatal("expected runtime")
	}
	exports := rt.Exports()

	if err := createTestWorkspace(context.Background(), exports, "ws-bootstrap-memory", "bootstrap memory backend", ""); err != nil {
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

func TestNewWithBackendMemoryStartsSystemDefaultsModule(t *testing.T) {
	ctx := context.Background()
	rt := NewWithBackend("memory", "")
	if rt == nil {
		t.Fatal("expected runtime")
	}
	exports := rt.Exports()

	server, err := exports.Repositories.MCPServers.Get(ctx, common.MCPServerID("mcp-lsp-bridge"))
	if err != nil {
		t.Fatalf("get mcp-lsp-bridge server: %v", err)
	}
	for _, tool := range []string{"lsp_status", "lsp_shutdown", "lsp_repair"} {
		assertBootstrapStringSliceContains(t, server.AllowedTools, tool)
	}

	capability, err := exports.Repositories.MCPCapabilities.Get(ctx, common.MCPCapabilityID("mcp-lsp-bridge:tool:lsp_status"))
	if err != nil {
		t.Fatalf("get lsp_status capability: %v", err)
	}
	if capability.Category != "code_semantics" || capability.RiskLevel != "read_only" {
		t.Fatalf("lsp_status capability = %+v, want code_semantics read_only", capability)
	}

	template, err := exports.Repositories.WorkspaceTemplates.Get(ctx, "software-dev")
	if err != nil {
		t.Fatalf("get software-dev template: %v", err)
	}
	semanticNavigation, ok := template.ContextHints["semanticCodeNavigation"].(map[string]any)
	if !ok {
		t.Fatalf("software-dev semanticCodeNavigation = %#v, want object", template.ContextHints["semanticCodeNavigation"])
	}
	defaultCapabilityIDs, ok := semanticNavigation["defaultCapabilityIds"].([]any)
	if !ok {
		t.Fatalf("semanticCodeNavigation.defaultCapabilityIds = %#v, want list", semanticNavigation["defaultCapabilityIds"])
	}
	assertBootstrapAnySliceContainsString(t, defaultCapabilityIDs, "mcp-lsp-bridge:tool:lsp_status")
}

func TestBuildRuntimeAdapterRegistryRegistersAllWorkorchBlockKinds(t *testing.T) {
	hub := events.NewMemoryHub()
	background := magictask.NewBackgroundRoutine(32)
	defer background.Shutdown(context.Background())
	executor := runtimeevent.NewExecutor("test/block", hub, background, echoRuntimeAdapter{},
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
	)
	if err := executor.Run(); err != nil {
		t.Fatalf("run echo executor: %v", err)
	}
	defer executor.Teardown()
	patchExecutor := runtimeevent.NewExecutor("test/patch", hub, background, orchestratormodule.NewPatchRuntimeAdapter(),
		llmcommon.RuntimeBlockKindPatchApply,
		llmcommon.RuntimeBlockKindPatchPreview,
		llmcommon.RuntimeBlockKindPatchRevert,
	)
	if err := patchExecutor.Run(); err != nil {
		t.Fatalf("run patch executor: %v", err)
	}
	defer patchExecutor.Teardown()
	registry, err := buildRuntimeAdapterRegistry(hub)
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
		result, err := registry.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
			WorkspaceID: "ws-runtime-test",
			RunID:       "run-001",
			StepID:      common.StepID("run-001-step-" + kind),
			Kind:        kind,
		})
		if err != nil {
			t.Fatalf("execute registered kind %s: %v", kind, err)
		}
		if result.Output["kind"] != kind {
			t.Fatalf("registered kind %s output = %+v", kind, result.Output)
		}
	}

	result, err := registry.ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceID: "ws-runtime-test",
		RunID:       "run-001",
		StepID:      "run-001-step-patch-apply",
		Kind:        llmcommon.RuntimeBlockKindPatchApply,
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
		WorkspaceID:                   "ws-runtime-test",
		RunID:                         "run-001",
		StepID:                        "run-001-step-patch-preview",
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
		WorkspaceID: "ws-runtime-test",
		RunID:       "run-001",
		StepID:      "run-001-step-patch-revert",
		Kind:        llmcommon.RuntimeBlockKindPatchRevert,
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

func TestNewWithBackendRegistersOrchestratorCommandHandlers(t *testing.T) {
	rt := NewWithBackend("file", t.TempDir())
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := rt.Shutdown(ctx); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	}()
	exports := rt.Exports()
	if err := createTestWorkspace(context.Background(), exports, "ws-command", "command", ""); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := createTestRun(context.Background(), exports, "ws-command", "run-command"); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := exports.Repositories.Steps.Create(context.Background(), &runmodel.Step{
		ID:     "run-command/approval",
		RunID:  "run-command",
		Kind:   "llm/workspace_execution",
		State:  common.StepStateCompleted,
		Output: map[string]any{"stage": "blocked_on_approval"},
	}); err != nil {
		t.Fatalf("CreateStep: %v", err)
	}
	if err := createTestApproval(context.Background(), exports, "ws-command", "run-command", "run-command/approval", "approval-command"); err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}
	if err := blockTestRunOnApproval(context.Background(), exports, "run-command", "approval-command"); err != nil {
		t.Fatalf("BlockOnApproval: %v", err)
	}
	err := exports.Infrastructure.EventHub.Publish(context.Background(), events.Envelope{
		Topic: orchestratorevents.TopicOrchestratorCommandBlockApproval,
		Payload: map[string]any{
			events.PayloadKeyRunID:              "run-missing",
			events.PayloadKeyBlockingApprovalID: "approval-missing",
		},
	})
	if err == nil {
		t.Fatalf("Publish returned nil, want orchestrator handler error for missing run")
	}
	if _, ok := events.ResponseEnvelope(err); ok {
		t.Fatalf("Publish returned response for missing run, want handler error")
	}
}

func TestNewWithBackendFilePersistsWorkspaceData(t *testing.T) {
	dataDir := t.TempDir()
	rt := NewWithBackend("file", dataDir)
	exports := rt.Exports()

	err := createTestWorkspace(context.Background(), exports, "ws-bootstrap-file", "bootstrap file backend", filepath.Join(dataDir, "internal", "workspaces", "ws-bootstrap-file"))
	if err != nil {
		t.Fatalf("create workspace with file backend: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dataDir, "internal", "workspaces", "ws-bootstrap-file", "workspace.json")); err != nil {
		t.Fatalf("expected workspace persistence file: %v", err)
	}
}

func TestWorkspaceScopedStepRepositoryUsesRuntimeWorkspaceForDuplicateRunIDs(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	rt := NewWithBackend("file", dataDir)
	exports := rt.Exports()
	runID := common.RunID("run-001")
	stepID := common.StepID("run-001/step-1")
	ctxA := common.ContextWithRuntimeWorkspaceID(ctx, "ws-step-scope-a")
	ctxB := common.ContextWithRuntimeWorkspaceID(ctx, "ws-step-scope-b")

	for _, workspaceID := range []common.WorkspaceID{"ws-step-scope-a", "ws-step-scope-b"} {
		if err := exports.Repositories.Runs.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &runmodel.Run{
			ID:          runID,
			WorkspaceID: workspaceID,
			State:       common.RunStateRunning,
		}); err != nil {
			t.Fatalf("create run %s: %v", workspaceID, err)
		}
	}
	if err := exports.Repositories.Steps.Create(ctxA, &runmodel.Step{
		ID:     stepID,
		RunID:  runID,
		Kind:   "llm/workspace_execution",
		State:  common.StepStateCompleted,
		Output: map[string]any{"status": "completed"},
	}); err != nil {
		t.Fatalf("create step in workspace A: %v", err)
	}
	if item, err := exports.Repositories.Steps.Get(ctxA, stepID); err != nil || item.State != common.StepStateCompleted {
		t.Fatalf("get step in workspace A = %+v, %v; want completed", item, err)
	}
	if _, err := exports.Repositories.Steps.Get(ctxB, stepID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get step in workspace B err = %v, want not found", err)
	}
	if err := exports.Repositories.Steps.Create(ctxB, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "mcp/tool_call",
		State: common.StepStateDispatched,
	}); err != nil {
		t.Fatalf("create duplicate step in workspace B: %v", err)
	}
	item, err := exports.Repositories.Steps.Get(ctxB, stepID)
	if err != nil {
		t.Fatalf("get step in workspace B after create: %v", err)
	}
	if item.Kind != "mcp/tool_call" || item.State != common.StepStateDispatched {
		t.Fatalf("workspace B step = %+v, want dispatched mcp/tool_call", item)
	}
}

func TestWorkspaceScopedGovernanceRepositoriesUseRuntimeWorkspaceForDuplicateIDs(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	rt := NewWithBackend("file", dataDir)
	exports := rt.Exports()
	runID := common.RunID("run-001")
	ticketID := common.TicketID("run-001-step-1-decision")
	approvalID := common.ApprovalID("run-001-step-1-approval")
	ctxA := common.ContextWithRuntimeWorkspaceID(ctx, "ws-governance-scope-a")
	ctxB := common.ContextWithRuntimeWorkspaceID(ctx, "ws-governance-scope-b")

	for _, workspaceID := range []common.WorkspaceID{"ws-governance-scope-a", "ws-governance-scope-b"} {
		if err := exports.Repositories.Runs.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &runmodel.Run{
			ID:          runID,
			WorkspaceID: workspaceID,
			State:       common.RunStateBlockedOnDecision,
		}); err != nil {
			t.Fatalf("create run %s: %v", workspaceID, err)
		}
		if err := exports.Repositories.Tickets.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &decisionmodel.Ticket{
			ID:           ticketID,
			WorkspaceID:  workspaceID,
			RunID:        runID,
			StepID:       "run-001/step-1",
			Reason:       "decision " + string(workspaceID),
			DecisionKind: "provide_missing_info",
			State:        common.DecisionTicketStateOpen,
		}); err != nil {
			t.Fatalf("create ticket %s: %v", workspaceID, err)
		}
		if err := exports.Repositories.Approvals.Create(common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), &approvalmodel.Approval{
			ID:          approvalID,
			WorkspaceID: workspaceID,
			RunID:       runID,
			StepID:      "run-001/step-1",
			Reason:      "approval " + string(workspaceID),
			Action:      "resume",
			State:       common.ApprovalStatePending,
		}); err != nil {
			t.Fatalf("create approval %s: %v", workspaceID, err)
		}
	}

	ticketA, err := exports.Repositories.Tickets.Get(ctxA, ticketID)
	if err != nil {
		t.Fatalf("get ticket A: %v", err)
	}
	if ticketA.WorkspaceID != "ws-governance-scope-a" || ticketA.Reason != "decision ws-governance-scope-a" {
		t.Fatalf("ticket A = %+v, want workspace A", ticketA)
	}
	ticketB, err := exports.Repositories.Tickets.Get(ctxB, ticketID)
	if err != nil {
		t.Fatalf("get ticket B: %v", err)
	}
	if ticketB.WorkspaceID != "ws-governance-scope-b" || ticketB.Reason != "decision ws-governance-scope-b" {
		t.Fatalf("ticket B = %+v, want workspace B", ticketB)
	}
	approvalA, err := exports.Repositories.Approvals.Get(ctxA, approvalID)
	if err != nil {
		t.Fatalf("get approval A: %v", err)
	}
	if approvalA.WorkspaceID != "ws-governance-scope-a" || approvalA.Reason != "approval ws-governance-scope-a" {
		t.Fatalf("approval A = %+v, want workspace A", approvalA)
	}
	approvalB, err := exports.Repositories.Approvals.Get(ctxB, approvalID)
	if err != nil {
		t.Fatalf("get approval B: %v", err)
	}
	if approvalB.WorkspaceID != "ws-governance-scope-b" || approvalB.Reason != "approval ws-governance-scope-b" {
		t.Fatalf("approval B = %+v, want workspace B", approvalB)
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
		if err := exports.Repositories.Artifacts.Create(ctx, item); err != nil {
			t.Fatalf("create artifact %s: %v", item.ID, err)
		}
	}
	artifactDir := filepath.Join(dataDir, "internal", "workspaces", "ws-bootstrap-artifact-cache", "artifacts", "run-bootstrap-artifact-cache")
	for _, fileName := range []string{"result-summary.txt", "workspace-deliverable-manifest.txt"} {
		if _, err := os.Stat(filepath.Join(artifactDir, fileName)); err != nil {
			t.Fatalf("expected stored artifact %s: %v", fileName, err)
		}
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
	var initialIndex testWorkspaceIndex
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
	var updatedIndex testWorkspaceIndex
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

func TestWorkspaceScopedRunRepositoryResolvesDuplicateRunIDByWorkspaceContext(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	rt := NewWithBackend("file", dataDir)
	exports := rt.Exports()

	runID := common.RunID("run-001")
	for _, workspaceID := range []common.WorkspaceID{"ws-run-scope-a", "ws-run-scope-b"} {
		if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
			ID:                   workspaceID,
			Title:                string(workspaceID),
			DeliverableDirectory: filepath.Join(filepath.Dir(dataDir), ".workorch-workspaces", string(workspaceID)),
			InternalDirectory:    filepath.Join(dataDir, "internal", "workspaces", string(workspaceID)),
			State:                common.WorkspaceStateActive,
		}); err != nil {
			t.Fatalf("create workspace %s: %v", workspaceID, err)
		}
		scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
		if err := exports.Repositories.Runs.Create(scopedCtx, &runmodel.Run{
			ID:          runID,
			WorkspaceID: workspaceID,
			State:       common.RunStateRunning,
		}); err != nil {
			t.Fatalf("create run %s/%s: %v", workspaceID, runID, err)
		}
		if err := exports.Repositories.Steps.Create(scopedCtx, &runmodel.Step{
			ID:    common.StepID(string(runID) + "/step-" + string(workspaceID)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateCompleted,
		}); err != nil {
			t.Fatalf("create step %s/%s: %v", workspaceID, runID, err)
		}
	}

	wsBCtx := common.ContextWithRuntimeWorkspaceID(ctx, "ws-run-scope-b")
	run, err := exports.Repositories.Runs.Get(wsBCtx, runID)
	if err != nil {
		t.Fatalf("get scoped run: %v", err)
	}
	if run.WorkspaceID != "ws-run-scope-b" {
		t.Fatalf("scoped run workspace = %s, want ws-run-scope-b", run.WorkspaceID)
	}
	steps, err := exports.Repositories.Steps.ListByRun(wsBCtx, runID)
	if err != nil {
		t.Fatalf("list scoped steps: %v", err)
	}
	if len(steps) != 1 || !strings.Contains(string(steps[0].ID), "ws-run-scope-b") {
		t.Fatalf("scoped steps = %+v, want only ws-run-scope-b", steps)
	}
	if _, err := exports.Repositories.Runs.Get(ctx, runID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unscoped duplicate run get err = %v, want conflict", err)
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
	var bindings testWorkspaceSoulBindingsFile
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

type testWorkspaceSoulBindingsFile struct {
	DefaultSoulID common.SoulID            `json:"defaultSoulId,omitempty"`
	Bindings      map[string]common.SoulID `json:"bindings,omitempty"`
}

type testWorkspaceIndex struct {
	Workspaces []testWorkspaceIndexEntry `json:"workspaces"`
}

type testWorkspaceIndexEntry struct {
	WorkspaceID  common.WorkspaceID    `json:"workspaceId"`
	InternalRoot string                `json:"internalRoot"`
	State        common.WorkspaceState `json:"state"`
	ActiveRunID  common.RunID          `json:"activeRunId,omitempty"`
}

func workspaceIndexContains(index testWorkspaceIndex, workspaceID common.WorkspaceID, state common.WorkspaceState, activeRunID common.RunID) bool {
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

	err := createTestWorkspace(context.Background(), exports, "ws-bootstrap-file-default", "bootstrap file backend default data root", filepath.Join(defaultDataDir, "internal", "workspaces", "ws-bootstrap-file-default"))
	if err != nil {
		t.Fatalf("create workspace with default file backend: %v", err)
	}

	if _, err := os.Stat(filepath.Join(defaultDataDir, "internal", "workspaces", "ws-bootstrap-file-default", "workspace.json")); err != nil {
		t.Fatalf("expected workspace persistence file in default data dir: %v", err)
	}
}

func TestRuntimeModuleLifecycleRunsAndShutsDownInOrder(t *testing.T) {
	var runSeq []string
	var teardownSeq []string
	lifecycle, err := newRuntimeModuleLifecycle([]lifecycle{
		testLifecycle{id: "first", runSeq: &runSeq, teardownSeq: &teardownSeq},
		testLifecycle{id: "second", runSeq: &runSeq, teardownSeq: &teardownSeq},
	})
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}

	if err := lifecycle.Start(); err != nil {
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

func TestRuntimeModuleLifecycleReturnsRunAndShutdownErrors(t *testing.T) {
	errRun := errors.New("run failed")
	runtimeLifecycle, err := newRuntimeModuleLifecycle([]lifecycle{
		testLifecycle{id: "broken-run", runErr: errRun},
	})
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}
	if err := runtimeLifecycle.Start(); err == nil || !strings.Contains(err.Error(), errRun.Error()) {
		t.Fatalf("run err = %v, want %v", err, errRun)
	}

	errShutdown := errors.New("shutdown failed")
	runtimeLifecycle, err = newRuntimeModuleLifecycle([]lifecycle{
		testLifecycle{id: "broken-shutdown", teardownErr: errShutdown},
	})
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}
	if err := runtimeLifecycle.Shutdown(context.Background()); !errors.Is(err, errShutdown) {
		t.Fatalf("shutdown err = %v, want %v", err, errShutdown)
	}
}

func TestRuntimeModuleLifecycleContinuesShutdownAfterError(t *testing.T) {
	errShutdown := errors.New("shutdown failed")
	var teardownSeq []string
	runtimeLifecycle, err := newRuntimeModuleLifecycle([]lifecycle{
		testLifecycle{id: "first", teardownSeq: &teardownSeq},
		testLifecycle{id: "broken", teardownSeq: &teardownSeq, teardownErr: errShutdown},
		testLifecycle{id: "last", teardownSeq: &teardownSeq},
	})
	if err != nil {
		t.Fatalf("new lifecycle: %v", err)
	}

	err = runtimeLifecycle.Shutdown(context.Background())
	if !errors.Is(err, errShutdown) {
		t.Fatalf("shutdown err = %v, want %v", err, errShutdown)
	}
	if got, want := strings.Join(teardownSeq, ","), "last,broken,first"; got != want {
		t.Fatalf("teardown seq = %s, want %s", got, want)
	}
	if runtimeLifecycle.modules != nil {
		t.Fatal("shutdown did not clear lifecycle modules")
	}
}

func TestRuntimeShutdownTerminatesHubAfterTeardownError(t *testing.T) {
	errShutdown := errors.New("shutdown failed")
	var teardownSeq []string
	hub := &terminatingEventHub{}
	rt := &Runtime{
		modules: []lifecycle{
			testLifecycle{id: "first", teardownSeq: &teardownSeq},
			testLifecycle{id: "broken", teardownSeq: &teardownSeq, teardownErr: errShutdown},
			testLifecycle{id: "last", teardownSeq: &teardownSeq},
		},
		exports: Exports{Infrastructure: InfrastructureExports{EventHub: hub}},
	}

	err := rt.Shutdown(context.Background())
	if !errors.Is(err, errShutdown) {
		t.Fatalf("shutdown err = %v, want %v", err, errShutdown)
	}
	if got, want := strings.Join(teardownSeq, ","), "last,broken,first"; got != want {
		t.Fatalf("teardown seq = %s, want %s", got, want)
	}
	if rt.modules != nil {
		t.Fatal("runtime shutdown did not clear modules")
	}
	if !hub.terminated {
		t.Fatal("runtime shutdown did not terminate event hub after teardown error")
	}
}

func TestRuntimeShutdownDrainsBackgroundBeforeModuleTeardown(t *testing.T) {
	background := magictask.NewBackgroundRoutine(4)
	stopped := false
	done := make(chan bool, 1)
	rt := &Runtime{
		modules: []lifecycle{
			backgroundAwareLifecycle{id: "background-aware", stopped: &stopped},
		},
		background: background,
	}
	if err := background.AsyncFunction(func() {
		time.Sleep(10 * time.Millisecond)
		done <- stopped
	}); err != nil {
		t.Fatalf("schedule background task: %v", err)
	}

	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatalf("runtime shutdown: %v", err)
	}
	select {
	case sawStopped := <-done:
		if sawStopped {
			t.Fatal("background task observed module teardown before background drain")
		}
	default:
		t.Fatal("runtime shutdown returned before background task completed")
	}
	if !stopped {
		t.Fatal("runtime shutdown did not teardown module after background drain")
	}
}

func assertBootstrapStringSliceContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("values = %+v, want %q", values, want)
}

func assertBootstrapAnySliceContainsString(t *testing.T, values []any, want string) {
	t.Helper()
	for _, value := range values {
		if got, ok := value.(string); ok && got == want {
			return
		}
	}
	t.Fatalf("values = %+v, want %q", values, want)
}
