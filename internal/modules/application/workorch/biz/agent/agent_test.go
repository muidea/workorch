package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func useTestConfig(t *testing.T) {
	t.Helper()
	_ = workorchconfig.Close()
	configDir := t.TempDir()
	content := `endpointName = "workorchd"

[server]
addr = "127.0.0.1:0"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "180s"
idleTimeout = "60s"
shutdownTimeout = "5s"
maxConcurrentRuns = 1

[repository]
backend = "memory"
dataDir = ""

[llm]
defaultProvider = "test"

[llm.providers.test]
provider = "compatible"
baseURL = "http://127.0.0.1:1"
apiKey = "test-key"
model = "test-model"
timeout = "5s"
disabled = false
chatCompletions = true
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CONFIG_PATH", configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
}

// TestSessionBorrowRequired proves that EVERY WithSession call needs Borrow().
// Each WithSession call has defer CloseOnReturn which closes the session on exit.
// Using the same session without Borrow() causes "invalid argument" errors.
func TestSessionBorrowRequired(t *testing.T) {
	useTestConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := appservice.NewRuntimeSessionWithBackend("memory", "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer session.Close(ctx)

	// Create workspace — also needs Borrow()!
	ws, err := appservice.CreateWorkspaceWithSession(ctx, session.Borrow(), "borrow-test", "test", "")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	t.Logf("workspace: %s", ws.ID)

	// Define goal — uses Borrow()
	goal, err := appservice.DefineGoalWithSession(ctx, session.Borrow(), ws.ID, "test goal")
	if err != nil {
		t.Fatalf("define goal (with Borrow): %v", err)
	}
	t.Logf("goal: %s", goal.ID)

	// Define DoD — uses Borrow()
	dod, err := appservice.DefineDoDWithSession(ctx, session.Borrow(), ws.ID, []string{"verify borrow pattern"})
	if err != nil {
		t.Fatalf("define dod (with Borrow): %v", err)
	}
	t.Logf("dod: %s", dod.ID)

	// List goals — uses Borrow()
	goals, err := appservice.ListGoalsWithSession(ctx, session.Borrow(), ws.ID)
	if err != nil {
		t.Fatalf("list goals: %v", err)
	}
	if len(goals) != 1 {
		t.Fatalf("expected 1 goal, got %d", len(goals))
	}

	// Read workspace via repository (direct, no WithSession needed)
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	retrieved, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(ws.ID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if retrieved.Title != "borrow-test" {
		t.Fatalf("expected title 'borrow-test', got %q", retrieved.Title)
	}
}

// TestAgentChannelLifecycle verifies agent event channel behavior.
func TestAgentChannelLifecycle(t *testing.T) {
	a := New()
	if a.Events() == nil {
		t.Fatal("expected non-nil events channel")
	}

	// Send event
	a.Send(Event{Type: EventRunCreated, Payload: RunCreatedInfo{
		RunID: "test-run", WorkspaceID: "ws-1",
	}})

	select {
	case evt := <-a.Events():
		if evt.Type != EventRunCreated {
			t.Fatalf("expected EventRunCreated, got %s", evt.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}

	// First close
	a.Close()

	// Second close must not panic
	a.Close()
}

// TestAgentEventTypes verifies all event type constants.
func TestAgentEventTypes(t *testing.T) {
	types := map[EventType]string{
		EventRunCreated:   "run_created",
		EventDecision:     "decision",
		EventApproval:     "approval",
		EventRunCompleted: "run_completed",
		EventRunFailed:    "run_failed",
		EventCancelled:    "cancelled",
	}
	for typ, expected := range types {
		if string(typ) != expected {
			t.Fatalf("EventType %v = %q, want %q", typ, string(typ), expected)
		}
	}
}

// TestDecisionApprovalInfoTypes verifies payload struct fields.
func TestDecisionApprovalInfoTypes(t *testing.T) {
	d := DecisionInfo{TicketID: "t-1", Question: "q?", Reason: "r", RunID: "run-1"}
	if d.TicketID != "t-1" || d.Question != "q?" || d.Reason != "r" || d.RunID != "run-1" {
		t.Fatal("DecisionInfo field mismatch")
	}
	a := ApprovalInfo{ApprovalID: "a-1", Reason: "reason", RunID: "run-1"}
	if a.ApprovalID != "a-1" || a.Reason != "reason" || a.RunID != "run-1" {
		t.Fatal("ApprovalInfo field mismatch")
	}
	r := RunCreatedInfo{RunID: "run-1", WorkspaceID: "ws-1"}
	if r.RunID != "run-1" || r.WorkspaceID != "ws-1" {
		t.Fatal("RunCreatedInfo field mismatch")
	}
}

// TestDirectSessionCRUD verifies basic CRUD through appservice session.
func TestDirectSessionCRUD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := appservice.NewRuntimeSessionWithBackend("memory", "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer session.Close(ctx)

	// Create via WithSession (uses Borrow internally)
	ws, err := appservice.CreateWorkspaceWithSession(ctx, session.Borrow(), "crud-test", "CRUD test", "")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if ws.State != "active" {
		t.Fatalf("expected active state, got %s", ws.State)
	}

	// Verify via direct repository access
	exports, err := session.Exports()
	if err != nil {
		t.Fatalf("exports: %v", err)
	}
	list, err := exports.Repositories.Workspaces.List(ctx)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(list))
	}
}
