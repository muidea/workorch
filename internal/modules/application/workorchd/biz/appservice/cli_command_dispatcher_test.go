package appservice

import (
	"context"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/events"
)

func TestCLICommandDispatcherRunIsIdempotent(t *testing.T) {
	hub := &countingCLIHub{}
	dispatcher := NewCLICommandDispatcher(&RuntimeSession{}, hub, nil)
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if hub.subscribeCount != 1 {
		t.Fatalf("subscribe count = %d, want 1", hub.subscribeCount)
	}
	dispatcher.Close()
	if hub.unsubscribeCount != 1 {
		t.Fatalf("unsubscribe count = %d, want 1", hub.unsubscribeCount)
	}
	if err := dispatcher.Run(); err != nil {
		t.Fatalf("Run after Close: %v", err)
	}
	if hub.subscribeCount != 2 {
		t.Fatalf("subscribe count after rerun = %d, want 2", hub.subscribeCount)
	}
}

func TestCLIWorkspaceSnapshotCommandProjectsDetailView(t *testing.T) {
	dispatcher := &CLICommandDispatcher{
		viewReader: staticCLIWorkspaceViewReader{
			snapshot: WorkspaceSnapshot{
				Workspace: Workspace{
					ID:               "ws-cli-detail",
					ActiveRunID:      "run-cli-active",
					ContextHints:     map[string]any{"large": "profile"},
					EvidencePolicies: []map[string]any{{"large": "policy"}},
				},
				Runs: []Run{{
					ID:               "run-cli-active",
					WorkspaceID:      "ws-cli-detail",
					State:            "running",
					ExecutionContext: map[string]any{"coverageWorkItems": []any{map[string]any{"large": strings.Repeat("x", 2048)}}},
				}},
				LLMUsageTraces: []LLMUsageTrace{{
					ID:    "usage-cli",
					RunID: "run-cli-active",
					ContextBudget: map[string]any{
						"failureSignature": strings.Repeat("x", 2048),
					},
				}},
			},
		},
	}

	resource, _, err := dispatcher.handleWorkspaceCommand(context.Background(), events.CLICommandPayload{
		Command:   events.CommandWorkspaceSnapshot,
		SubjectID: "ws-cli-detail",
	})
	if err != nil {
		t.Fatalf("handle workspace snapshot: %v", err)
	}
	workspace := resource["workspace"].(Workspace)
	if workspace.ContextHints != nil || workspace.EvidencePolicies != nil {
		t.Fatalf("workspace = %+v, want detail projection without heavy template fields", workspace)
	}
	runs := resource["runs"].([]map[string]any)
	if len(runs) != 1 || runs[0]["executionContext"] != nil {
		t.Fatalf("runs = %+v, want compact detail run projection", runs)
	}
	traces := resource["llmUsageTraces"].([]LLMUsageTrace)
	signature := traces[0].ContextBudget["failureSignature"].(string)
	if len(signature) > workspaceSnapshotTraceProjectionStringLimitForTest() {
		t.Fatalf("failureSignature length = %d, want detail projection bounded", len(signature))
	}
}

func workspaceSnapshotTraceProjectionStringLimitForTest() int {
	return 1024 + len("...[truncated]")
}

type staticCLIWorkspaceViewReader struct {
	snapshot WorkspaceSnapshot
}

func (r staticCLIWorkspaceViewReader) ListWorkspaceView(context.Context) ([]Workspace, error) {
	return nil, nil
}

func (r staticCLIWorkspaceViewReader) ReadWorkspaceViewSnapshot(context.Context, string) (WorkspaceSnapshot, error) {
	return r.snapshot, nil
}

type countingCLIHub struct {
	subscribeCount   int
	unsubscribeCount int
}

func (h *countingCLIHub) Publish(context.Context, events.Envelope) error {
	return nil
}

func (h *countingCLIHub) Subscribe(string, events.Handler) func() {
	h.subscribeCount++
	return func() {
		h.unsubscribeCount++
	}
}
