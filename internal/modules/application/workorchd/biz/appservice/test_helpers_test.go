package appservice

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

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

func publishAppserviceTestCommand(ctx context.Context, hub events.Hub, envelope events.Envelope) error {
	err := hub.Publish(ctx, envelope)
	if _, ok := events.ResponseEnvelope(err); ok {
		return nil
	}
	return err
}

func createTestMCPServerWithExports(ctx context.Context, exports bootstrap.Exports, cmd mcpservice.CreateServerCommand) error {
	return publishAppserviceTestCommand(ctx, exports.Infrastructure.EventHub, events.Envelope{
		Topic:        events.TopicMCPCommandCreateServer,
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
		Topic:        events.TopicMCPCommandUpdateCapability,
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

func prepareDecisionRun(t *testing.T, dataRoot string, workspaceID string, runID string) Run {
	t.Helper()

	ticketID := common.TicketID("ticket-" + runID + "-decision")
	stepID := common.StepID(runID + "/decision")
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("runtime exports: %v", err)
	}
	actions := newEventHubActionPort(exports.Infrastructure.EventHub)
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
		Topic:        events.TopicDecisionCommandEnsureTicket,
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
	return mapRun(item)
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
	actions := newEventHubActionPort(exports.Infrastructure.EventHub)
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
	return mapRun(item)
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
