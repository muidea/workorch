package appservice

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimeports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	mcpservice "github.com/muidea/workorch/internal/modules/kernel/mcp/service"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func newTestRuntimeSession(t *testing.T, backend string, dataRoot string) (*RuntimeSession, context.Context) {
	t.Helper()

	ctx := context.Background()
	session, err := NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(ctx); err != nil {
			t.Errorf("close runtime session: %v", err)
		}
	})
	return session, ctx
}

func useWorkorchTestConfig(t *testing.T, backend string, dataRoot string) {
	t.Helper()

	_ = workorchconfig.Close()
	configDir := t.TempDir()
	content := fmt.Sprintf(`endpointName = "workorch"

[server]
addr = ":8080"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "180s"
idleTimeout = "60s"
shutdownTimeout = "5s"
maxConcurrentRuns = 1

[repository]
backend = %q
dataDir = %q

[llm]
timeout = "60s"
disabled = false
`, backend, dataRoot)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CONFIG_PATH", configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
}

func createWorkspaceWithIDWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, title string, description string, deliverableDirectory string) (ret Workspace, err error) {
	hub, err := session.RequiredEventHub()
	if err != nil {
		return Workspace{}, err
	}

	id := common.WorkspaceID(workspaceID)
	layout, err := workspacecore.ResolveDirectoryLayout(session.RepositoryBackend(), session.RepositoryDataDir(), string(id), deliverableDirectory)
	if err != nil {
		return Workspace{}, err
	}
	if err := workspacecore.EnsureDirectoryLayout(layout); err != nil {
		return Workspace{}, err
	}
	if err := ownercommandports.Publish(ctx, hub, workspaceevents.TopicWorkspaceCommandCreate, "workspace", string(id), id, "", map[string]any{
		events.PayloadKeyWorkspaceID:          string(id),
		events.PayloadKeyTitle:                title,
		events.PayloadKeyDescription:          description,
		events.PayloadKeyDeliverableDirectory: layout.DeliverableDirectory,
		events.PayloadKeyInternalDirectory:    layout.InternalDirectory,
	}); err != nil {
		return Workspace{}, err
	}

	item, err := ownerreadports.ReadWorkspaceByOwnerCommand(ctx, hub, id)
	if err != nil {
		return Workspace{}, err
	}
	return workspacecore.MapWorkspace(item), nil
}

func createWorkspaceWithIDWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, title string, description string, deliverableDirectory string) (Workspace, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Workspace{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	return createWorkspaceWithIDWithSession(ctx, session, workspaceID, title, description, deliverableDirectory)
}

func publishAppserviceTestCommand(ctx context.Context, hub events.Hub, envelope events.Envelope) error {
	err := hub.Publish(ctx, envelope)
	if _, ok := events.ResponseEnvelope(err); ok {
		return nil
	}
	return err
}

func createTestMCPServerWithExports(ctx context.Context, exports bootstrap.Exports, cmd mcpservice.CreateServerCommand) error {
	return publishAppserviceTestCommand(ctx, exports.Infrastructure.EventHub, events.Envelope{
		Topic:        mcpevents.TopicMCPCommandCreateServer,
		SourceModule: "workorchd/appservice/test",
		SubjectType:  "mcp_server",
		SubjectID:    string(cmd.ServerID),
		Payload: map[string]any{
			events.PayloadKeyCommand:  cmd,
			events.PayloadKeyServerID: string(cmd.ServerID),
		},
	})
}

func updateTestMCPCapabilityWithExports(ctx context.Context, exports bootstrap.Exports, cmd mcpservice.UpdateCapabilityCommand) error {
	return publishAppserviceTestCommand(ctx, exports.Infrastructure.EventHub, events.Envelope{
		Topic:        mcpevents.TopicMCPCommandUpdateCapability,
		SourceModule: "workorchd/appservice/test",
		SubjectType:  "mcp_capability",
		SubjectID:    string(cmd.ID),
		Payload: map[string]any{
			events.PayloadKeyCommand:      cmd,
			events.PayloadKeyCapabilityID: string(cmd.ID),
			events.PayloadKeyServerID:     string(cmd.ServerID),
		},
	})
}

func readModelIntValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		if typed == float64(int(typed)) {
			return int(typed)
		}
	default:
		return 0
	}
	return 0
}

func readModelStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func readModelStringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func prepareDecisionRun(t *testing.T, dataRoot string, workspaceID string, runID string) Run {
	t.Helper()

	ticketID := common.TicketID("ticket-" + runID + "-decision")
	stepID := common.StepID(runID + "/decision")
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("runtime exports: %v", err)
	}
	actions := runtimeports.NewEventHubActionPort(exports.Infrastructure.EventHub)
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceID))
	ensureTestWorkspace(t, ctx, session, workspaceID)
	if err := actions.EnsureRun(ctx, common.RunID(runID), common.WorkspaceID(workspaceID)); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  common.RunID(runID),
		Kind:   "llm/workspace_execution",
		State:  common.StepStateCompleted,
		Output: map[string]any{"stage": "blocked_on_decision"},
	}); err != nil {
		t.Fatalf("Create decision step: %v", err)
	}
	if err := publishAppserviceTestCommand(ctx, exports.Infrastructure.EventHub, events.Envelope{
		Topic:        decisionevents.TopicDecisionCommandEnsureTicket,
		SourceModule: "workorchd/appservice/test",
		SubjectType:  "decision_ticket",
		SubjectID:    string(ticketID),
		WorkspaceID:  workspaceID,
		RunID:        runID,
		StepID:       string(stepID),
		Payload: map[string]any{
			events.PayloadKeyBlockingTicketID: string(ticketID),
			events.PayloadKeyWorkspaceID:      workspaceID,
			events.PayloadKeyRunID:            runID,
			events.PayloadKeyStepID:           string(stepID),
			events.PayloadKeyReason:           "test decision gate",
			events.PayloadKeyKind:             decisioncommon.ResolutionTypeProvideMissingInfo,
			events.PayloadKeyQuery:            "provide test decision input",
			events.PayloadKeyContext:          map[string]any{"requiredFields": []any{"input"}},
		},
	}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if err := actions.EnsureRunBlockedOnDecision(ctx, common.RunID(runID), ticketID); err != nil {
		t.Fatalf("BlockOnDecision: %v", err)
	}
	item, err := exports.Repositories.Runs.Get(ctx, common.RunID(runID))
	if err != nil {
		t.Fatalf("Get run: %v", err)
	}
	return runtimecore.MapRun(item)
}

func prepareApprovalRun(t *testing.T, dataRoot string, workspaceID string, runID string) Run {
	t.Helper()

	approvalID := common.ApprovalID("approval-" + runID + "-approval")
	stepID := common.StepID(runID + "/approval")
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("runtime exports: %v", err)
	}
	actions := runtimeports.NewEventHubActionPort(exports.Infrastructure.EventHub)
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceID))
	ensureTestWorkspace(t, ctx, session, workspaceID)
	if err := actions.EnsureRun(ctx, common.RunID(runID), common.WorkspaceID(workspaceID)); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  common.RunID(runID),
		Kind:   "llm/workspace_execution",
		State:  common.StepStateCompleted,
		Output: map[string]any{"stage": "blocked_on_approval"},
	}); err != nil {
		t.Fatalf("Create approval step: %v", err)
	}
	if err := actions.EnsureApproval(ctx, approvalID, common.WorkspaceID(workspaceID), common.RunID(runID), stepID, "test approval gate"); err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}
	if err := actions.EnsureRunBlockedOnApproval(ctx, common.RunID(runID), approvalID); err != nil {
		t.Fatalf("BlockOnApproval: %v", err)
	}
	item, err := exports.Repositories.Runs.Get(ctx, common.RunID(runID))
	if err != nil {
		t.Fatalf("Get run: %v", err)
	}
	return runtimecore.MapRun(item)
}

func ensureTestWorkspace(t *testing.T, ctx context.Context, session *RuntimeSession, workspaceID string) {
	t.Helper()
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("runtime exports: %v", err)
	}
	if _, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspaceID)); err == nil {
		return
	}
	if _, err := createWorkspaceWithIDWithSession(ctx, session, workspaceID, workspaceID+" title", "", "workspaces/"+workspaceID); err != nil {
		t.Fatalf("CreateWorkspaceWithSession: %v", err)
	}
}
