package appservice

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	governancemodule "github.com/muidea/workorch/internal/modules/kernel/governance"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func mustAppserviceEventPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatalf("workorcheventpayload.FromMap: %v", err)
	}
	return payload
}

func mustReadModelEventPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatalf("event payload: %v", err)
	}
	return payload
}

func mustAppserviceGovernancePayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// runtimeV2TestTemplate seals the same explicit policy required by production
// activation. Test fixtures may add their own contracts, but must not rely on a
// removed implicit acceptance-slice default.
func runtimeV2TestTemplate(template WorkspaceTemplate) WorkspaceTemplate {
	if template.ContextHints == nil {
		template.ContextHints = map[string]any{}
	}
	if _, ok := template.ContextHints["acceptanceSlicePolicy"]; ok {
		return template
	}
	template.ContextHints["acceptanceSlicePolicy"] = map[string]any{
		"schemaVersion": "workorch.acceptance_slice_policy.v2", "maxWorkItems": 4, "maxFocuses": 8, "maxCandidateContextItems": 4,
		"completionPolicy": "all_required_items_accepted", "resultPolicy": "slice_result_allowed",
		"groups": []any{map[string]any{"id": "test-all", "title": "Test acceptance", "scope": "test_fixture", "priority": 10, "maxWorkItems": 4, "match": map[string]any{"all": true}}},
	}
	return template
}

func runtimeV2TestContextHints(t *testing.T) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(runtimeV2TestTemplate(WorkspaceTemplate{ContextHints: map[string]any{}}).ContextHints)
	if err != nil {
		t.Fatalf("Runtime V2 test context hints: %v", err)
	}
	return payload
}

func createRuntimeV2TestTemplateWithBackend(ctx context.Context, backend string, dataRoot string, template WorkspaceTemplate) (WorkspaceTemplate, error) {
	return CreateWorkspaceTemplateWithBackend(ctx, backend, dataRoot, runtimeV2TestTemplate(template))
}

func TestMain(m *testing.M) {
	skillHubRunner = testNoopSkillHubRunner{}
	restoreBootstrapFactories := installAppserviceOwnerModuleTestFactories()
	defer restoreBootstrapFactories()
	originalWD, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "get test working directory: %v\n", err)
		os.Exit(1)
	}
	testWD, err := os.MkdirTemp("", "workorch-appservice-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create isolated test working directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.Chdir(testWD); err != nil {
		fmt.Fprintf(os.Stderr, "switch to isolated test working directory: %v\n", err)
		_ = os.RemoveAll(testWD)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.Chdir(originalWD)
	_ = os.RemoveAll(testWD)
	os.Exit(code)
}

type testNoopSkillHubRunner struct{}

func (testNoopSkillHubRunner) RunSkillHub(context.Context, string, ...string) ([]byte, error) {
	return []byte(`{"items":[]}`), nil
}

var testSharedBackendSessionCleanup sync.Map

func completeAppserviceTestExports(value any) bootstrap.Exports {
	var infrastructure bootstrap.InfrastructureExports
	switch exports := value.(type) {
	case bootstrap.Exports:
		infrastructure = exports.Infrastructure
	case testkit.Exports:
		infrastructure = exports.Infrastructure
	}
	if infrastructure.CommandHub == nil {
		infrastructure.CommandHub = magicevent.NewHub(16)
	}
	return bootstrap.Exports{Infrastructure: infrastructure}
}

func startFocusedGovernanceModule(t *testing.T, exports bootstrap.Exports, rawHub magicevent.Hub) {
	t.Helper()
	module := governancemodule.New()
	if err := module.Configure(governancemodule.Dependencies{
		AutoGovernanceTraces: memory.NewAutoGovernanceTraceRepository(),
		EventHub:             rawHub,
		LoadConfig: func() (workorchconfig.Config, error) {
			return workorchconfig.Config{}, nil
		},
	}); err != nil {
		t.Fatalf("configure focused governance module: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("start focused governance module: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(context.Background()); err != nil {
			t.Errorf("stop focused governance module: %v", err)
		}
	})
}

func testRuntimeExports(runtime *bootstrap.Runtime) testkit.Exports {
	return testkit.RuntimeExports(runtime)
}

func testSessionExports(session *RuntimeSession) (testkit.Exports, error) {
	return testkit.SessionExports(session)
}

func newTestRuntimeSession(t *testing.T, backend string, dataRoot string) (*RuntimeSession, context.Context) {
	t.Helper()
	useWorkorchTestConfig(t, backend, dataRoot)

	ctx := context.Background()
	session, err := NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		t.Fatalf("new runtime session: %v", err)
	}
	t.Cleanup(func() {
		// FR01: bound cleanup so leaked workers surface as test failures instead of
		// hanging the full recursive package for the default 10m timeout.
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := session.Close(closeCtx); err != nil {
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

[llm.aiProxy]
baseURL = "http://127.0.0.1:9"
timeout = "180s"
catalogRequestTimeout = "1s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "custom-test"
modelID = "model-1"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25

[llm.customProviders.custom-test]
provider = "compatible"
apiSurface = "openai_compatible_http"
baseURL = "http://127.0.0.1:9"
timeout = "30s"

[llm.customProviders.custom-test.models.model-1]
contextWindowTokens = 128000
maxOutputTokens = 8192
operations = ["chat_completions"]


`, backend, dataRoot)
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CONFIG_PATH", configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
	if _, err := workorchconfig.Current(); err != nil {
		t.Fatalf("current config: %v", err)
	}
}

func createWorkspaceWithIDWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, title string, description string, deliverableDirectory string) (ret Workspace, err error) {
	hub, err := session.RequiredCommandHub()
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
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandCreate, "application/workorchd/appservice/test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.CreateWorkspaceCommand{
		WorkspaceID:          id,
		Title:                title,
		Description:          description,
		DeliverableDirectory: layout.DeliverableDirectory,
		InternalDirectory:    layout.InternalDirectory,
	})
	ev.BindLaneKey(events.EventHubLaneKeyFor(workspaceevents.TopicWorkspaceCommandCreate, events.EventHubLaneScope{
		SubjectType: "workspace",
		SubjectID:   string(id),
		WorkspaceID: string(id),
	}))
	result := hub.Send(ev)
	if result == nil {
		return Workspace{}, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return Workspace{}, err
	}
	if _, err := result.Get(); err != nil {
		return Workspace{}, err
	}

	item, err := ownerreadports.ReadWorkspaceByOwnerCommand(ctx, hub, id)
	if err != nil {
		return Workspace{}, err
	}
	// This low-level test helper deliberately bypasses the product workspace
	// creation view to retain caller-provided IDs. Bind an explicit current
	// policy through the same owner-template path before any test later creates
	// or activates a Runtime V2 Run; production must not regain an implicit
	// acceptance-slice default for this fixture convenience.
	templateID := "runtime-v2-test-policy-" + string(id)
	template := WorkspaceTemplate{
		ID:   templateID,
		Name: "Runtime V2 test acceptance policy",
		ContextHints: map[string]any{
			"acceptanceSlicePolicy": map[string]any{
				"schemaVersion":            "workorch.acceptance_slice_policy.v2",
				"maxWorkItems":             4,
				"maxFocuses":               8,
				"maxCandidateContextItems": 4,
				"completionPolicy":         "all_required_items_accepted",
				"resultPolicy":             "slice_result_allowed",
				"groups": []any{map[string]any{
					"id": "test-all", "title": "Test acceptance", "scope": "test_fixture", "priority": 10,
					"maxWorkItems": 4, "match": map[string]any{"all": true},
				}},
			},
		},
	}
	if _, err := CreateWorkspaceTemplateWithSession(ctx, session, template); err != nil {
		return Workspace{}, fmt.Errorf("create Runtime V2 test acceptance template: %w", err)
	}
	item, err = workspacecore.ApplyTemplateToWorkspace(item, template)
	if err != nil {
		return Workspace{}, fmt.Errorf("apply Runtime V2 test acceptance template: %w", err)
	}
	// File-backed fixtures can use the test-only repository seed surface without
	// introducing a second RuntimeSession. Memory-only tests using this helper
	// do not create Runs and retain their intentionally bare setup.
	if session.RepositoryBackend() == "file" {
		repositories := testkit.OpenRepositories(session.RepositoryBackend(), session.RepositoryDataDir())
		if repositories.Workspaces == nil {
			return Workspace{}, fmt.Errorf("load Runtime V2 test workspace fixture repository: %w", common.ErrInvalidArgument)
		}
		if err := repositories.Workspaces.Update(ctx, item); err != nil {
			return Workspace{}, fmt.Errorf("bind Runtime V2 test acceptance template: %w", err)
		}
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

func cleanupSharedBackendSession(t *testing.T, backend string, dataRoot string) {
	t.Helper()
	key := fmt.Sprintf("%s|%s", backend, dataRoot)
	if _, loaded := testSharedBackendSessionCleanup.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := closeSharedBackendSession(ctx, backend, dataRoot); err != nil {
			t.Errorf("close shared backend session: %v", err)
		}
		// Drain residual background file writers so t.TempDir cleanup does not
		// fail with "directory not empty" after async Runtime V2 drives finish.
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if err := removeIfEmptyTree(dataRoot); err == nil {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
}

// openFileRepositories opens file-backed repositories for test seeding without
// starting a second RuntimeSession (avoids dual-session TempDir races).
func openFileRepositories(t *testing.T, dataRoot string) testkit.RepositoryExports {
	t.Helper()
	return testkit.OpenRepositories("file", dataRoot)
}

func removeIfEmptyTree(root string) error {
	// Best-effort: if residual writers still hold files, return error so caller retries.
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	// Do not delete content; only signal residual activity for wait loop.
	return fmt.Errorf("data root still has %d entries", len(entries))
}

func waitRunWithBackend(t *testing.T, backend string, dataRoot string, runID string, timeout time.Duration, accept func(Run) bool) Run {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last Run
	var lastErr error
	for {
		item, err := ReadRunWithBackend(context.Background(), backend, dataRoot, runID)
		if err == nil {
			last = item
			if accept == nil || accept(item) {
				return item
			}
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				t.Fatalf("run %s did not reach expected state before timeout; last err = %v", runID, lastErr)
			}
			t.Fatalf("run %s did not reach expected state before timeout; last = %+v", runID, last)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitRunStateWithBackend(t *testing.T, backend string, dataRoot string, runID string, state string) Run {
	t.Helper()
	return waitRunWithBackend(t, backend, dataRoot, runID, 5*time.Second, func(item Run) bool {
		return item.State == state
	})
}

func waitRunSnapshotWithBackend(t *testing.T, backend string, dataRoot string, runID string, timeout time.Duration, accept func(RunSnapshot) bool) RunSnapshot {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last RunSnapshot
	var lastErr error
	for {
		item, err := ReadRunSnapshotWithBackend(context.Background(), backend, dataRoot, runID)
		if err == nil {
			last = item
			if accept == nil || accept(item) {
				return item
			}
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				t.Fatalf("run snapshot %s did not reach expected state before timeout; last err = %v", runID, lastErr)
			}
			t.Fatalf("run snapshot %s did not reach expected state before timeout; last = %+v", runID, last)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitRunListWithBackend(t *testing.T, backend string, dataRoot string, workspaceID string, timeout time.Duration, accept func([]Run) bool) []Run {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last []Run
	var lastErr error
	for {
		items, err := ListRunsWithBackend(context.Background(), backend, dataRoot, workspaceID)
		if err == nil {
			last = items
			if accept == nil || accept(items) {
				return items
			}
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				t.Fatalf("run list for workspace %s did not reach expected state before timeout; last err = %v", workspaceID, lastErr)
			}
			t.Fatalf("run list for workspace %s did not reach expected state before timeout; last = %+v", workspaceID, last)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func sendAppserviceTestCommand[Command any, Result any](ctx context.Context, hub magicevent.Hub, topic string, destination string, command Command) (Result, error) {
	var zero Result
	if hub == nil {
		return zero, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(topic, "application/workorchd/appservice/test", destination, magicevent.NewHeader(), ctx, command)
	response := hub.Send(event)
	if response == nil {
		return zero, common.ErrInvalidArgument
	}
	if err := response.Error(); err != nil {
		return zero, err
	}
	data, err := response.Get()
	if err != nil {
		return zero, err
	}
	result, ok := data.(Result)
	if !ok {
		return zero, fmt.Errorf("%w: event %s result type %T", common.ErrInvalidArgument, topic, data)
	}
	return result, nil
}

func createTestMCPServerWithExports(ctx context.Context, exports testkit.Exports, cmd mcpevents.CreateServerCommand) error {
	_, err := sendAppserviceTestCommand[mcpevents.CreateServerCommand, mcpevents.MCPCommandResult](ctx, exports.Infrastructure.CommandHub, mcpevents.TopicMCPCommandCreateServer, mcpevents.CommandDestination, cmd)
	return err
}

func updateTestMCPCapabilityWithExports(ctx context.Context, exports testkit.Exports, cmd mcpevents.UpdateCapabilityCommand) error {
	_, err := sendAppserviceTestCommand[mcpevents.UpdateCapabilityCommand, mcpevents.MCPCommandResult](ctx, exports.Infrastructure.CommandHub, mcpevents.TopicMCPCommandUpdateCapability, mcpevents.CommandDestination, cmd)
	return err
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
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("runtime exports: %v", err)
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceID))
	ensureTestWorkspace(t, ctx, session, workspaceID)
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: common.RunID(runID), WorkspaceID: common.WorkspaceID(workspaceID), State: common.RunStateActive}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  common.RunID(runID),
		Kind:   "llm/workspace_execution",
		State:  common.StepStateSucceeded,
		Output: mustAppserviceGovernancePayload(t, map[string]any{"stage": "blocked_on_decision"}),
	}); err != nil {
		t.Fatalf("Create decision step: %v", err)
	}
	if err := exports.Repositories.Tickets.Create(ctx, &decisionmodel.Ticket{
		ID: ticketID, WorkspaceID: common.WorkspaceID(workspaceID), RunID: common.RunID(runID), StepID: stepID,
		Reason: "test decision gate", DecisionKind: "provide_missing_info", Question: "provide test decision input",
		Context: mustAppserviceGovernancePayload(t, map[string]any{"requiredFields": []any{"input"}}), State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	// This read-model fixture intentionally has no Runtime V2 aggregate.  It is
	// therefore projected directly as blocked instead of invoking the production
	// SM-1 command, which correctly accepts only aggregate-backed Runs.
	seeded, err := exports.Repositories.Runs.Get(ctx, common.RunID(runID))
	if err != nil {
		t.Fatalf("Get decision fixture run: %v", err)
	}
	seeded.State = common.RunStateBlockedOnDecision
	seeded.BlockingTicketID = ticketID
	if err := exports.Repositories.Runs.Update(ctx, seeded); err != nil {
		t.Fatalf("Block decision fixture run: %v", err)
	}
	workspace, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		t.Fatalf("Get decision fixture workspace: %v", err)
	}
	workspace.ActiveRunID = common.RunID(runID)
	if err := exports.Repositories.Workspaces.Update(ctx, workspace); err != nil {
		t.Fatalf("Activate decision fixture workspace: %v", err)
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
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("runtime exports: %v", err)
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(workspaceID))
	ensureTestWorkspace(t, ctx, session, workspaceID)
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: common.RunID(runID), WorkspaceID: common.WorkspaceID(workspaceID), State: common.RunStateActive}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  common.RunID(runID),
		Kind:   "llm/workspace_execution",
		State:  common.StepStateSucceeded,
		Output: mustAppserviceGovernancePayload(t, map[string]any{"stage": "blocked_on_approval"}),
	}); err != nil {
		t.Fatalf("Create approval step: %v", err)
	}
	if err := exports.Repositories.Approvals.Create(ctx, &approvalmodel.Approval{ID: approvalID, WorkspaceID: common.WorkspaceID(workspaceID), RunID: common.RunID(runID), StepID: stepID, Reason: "test approval gate", State: common.ApprovalStatePending}); err != nil {
		t.Fatalf("CreateApproval: %v", err)
	}
	// See prepareDecisionRun: this is a read-model fixture, not an authority
	// transition for a synthetic aggregate-free Run.
	seeded, err := exports.Repositories.Runs.Get(ctx, common.RunID(runID))
	if err != nil {
		t.Fatalf("Get approval fixture run: %v", err)
	}
	seeded.State = common.RunStateBlockedOnApproval
	seeded.BlockingApprovalID = approvalID
	if err := exports.Repositories.Runs.Update(ctx, seeded); err != nil {
		t.Fatalf("Block approval fixture run: %v", err)
	}
	workspace, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		t.Fatalf("Get approval fixture workspace: %v", err)
	}
	workspace.ActiveRunID = common.RunID(runID)
	if err := exports.Repositories.Workspaces.Update(ctx, workspace); err != nil {
		t.Fatalf("Activate approval fixture workspace: %v", err)
	}
	item, err := exports.Repositories.Runs.Get(ctx, common.RunID(runID))
	if err != nil {
		t.Fatalf("Get run: %v", err)
	}
	return runtimecore.MapRun(item)
}

func ensureTestWorkspace(t *testing.T, ctx context.Context, session *RuntimeSession, workspaceID string) {
	t.Helper()
	exports, err := testSessionExports(session)
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
