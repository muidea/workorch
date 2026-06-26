package workspaceviewcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestStoreReplaceAllListSnapshotAndDelete(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{
		{ID: "ws-b", Title: "B", State: "active"},
		{ID: "ws-a", Title: "A", State: "active"},
	}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace: readmodelcore.Workspace{
				ID:               "ws-a",
				Title:            "A+",
				State:            "running",
				ContextHints:     map[string]any{"large": "profile"},
				EvidencePolicies: []map[string]any{{"large": "policy"}},
			},
			Runs: []readmodelcore.Run{{
				ID:               "run-a",
				WorkspaceID:      "ws-a",
				State:            "running",
				ExecutionContext: map[string]any{"coverageWorkItems": []any{map[string]any{"large": strings.Repeat("x", 2048)}}},
			}},
			LLMUsageTraces: []readmodelcore.LLMUsageTrace{{
				ID:    "usage-a",
				RunID: "run-a",
				ContextBudget: map[string]any{
					"failureSignature": strings.Repeat("x", 2048),
				},
			}},
		},
	}, nil)

	items, status, err := store.List()
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if status.State != StateReady || status.GlobalRevision != 1 || status.WorkspaceTotal != 2 {
		t.Fatalf("status = %+v, want ready revision 1 total 2", status)
	}
	if len(items) != 2 || items[0].ID != "ws-a" || items[0].Title != "A+" || items[1].ID != "ws-b" {
		t.Fatalf("items = %+v, want sorted snapshot-backed workspaces", items)
	}
	if items[0].ViewModel.WorkspaceID != "ws-a" ||
		items[0].ViewModel.PromptMode != "running" ||
		items[0].ViewModel.GlobalRevision != status.GlobalRevision ||
		items[0].ViewModel.WorkspaceRevision == 0 ||
		items[0].ViewModel.Revision != items[0].ViewModel.WorkspaceRevision {
		t.Fatalf("list viewModel = %+v, status = %+v, want projection-owned running prompt", items[0].ViewModel, status)
	}
	if item, ok := items[0].ViewModel.Sections["promptMode"]; !ok || item.Revision != items[0].ViewModel.Revision {
		t.Fatalf("list promptMode section = %+v, want section revision from viewModel %+v", item, items[0].ViewModel)
	}

	snapshot, snapshotStatus, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshotStatus.GlobalRevision != status.GlobalRevision || snapshot.Workspace.State != "running" {
		t.Fatalf("snapshot/status = %+v/%+v, want running snapshot at same revision", snapshot, snapshotStatus)
	}
	if snapshot.ViewModel.WorkspaceID != "ws-a" ||
		snapshot.ViewModel.PromptMode != "running" ||
		snapshot.ViewModel.GlobalRevision != snapshotStatus.GlobalRevision ||
		snapshot.ViewModel.WorkspaceRevision != snapshotStatus.WorkspaceRevision ||
		snapshot.ViewModel.Revision != snapshotStatus.WorkspaceRevision {
		t.Fatalf("snapshot viewModel = %+v, status = %+v, want projection-owned revisions", snapshot.ViewModel, snapshotStatus)
	}
	for _, section := range []string{"list", "activeWorkspace", "runState", "promptMode", "commands", "coverage", "currentFocus", "latestDiagnostic"} {
		item, ok := snapshot.ViewModel.Sections[section]
		if !ok {
			t.Fatalf("snapshot viewModel sections = %+v, missing %s", snapshot.ViewModel.Sections, section)
		}
		if item.Revision != snapshot.ViewModel.Revision ||
			item.WorkspaceRevision != snapshot.ViewModel.WorkspaceRevision ||
			item.SourceEventRevision != snapshot.ViewModel.SourceEventRevision ||
			item.ProjectedSourceRevision != snapshot.ViewModel.ProjectedSourceRevision {
			t.Fatalf("viewModel section %s = %+v, want same revision as %+v", section, item, snapshot.ViewModel)
		}
	}
	if snapshot.Workspace.ContextHints != nil || snapshot.Workspace.EvidencePolicies != nil {
		t.Fatalf("snapshot workspace = %+v, want detail projection stored", snapshot.Workspace)
	}
	if len(snapshot.Runs) != 1 || snapshot.Runs[0].ExecutionContext != nil {
		t.Fatalf("snapshot runs = %+v, want compact detail run stored", snapshot.Runs)
	}
	if signature := snapshot.LLMUsageTraces[0].ContextBudget["failureSignature"].(string); len(signature) > 1024+len("...[truncated]") {
		t.Fatalf("stored LLM trace signature length = %d, want bounded detail projection", len(signature))
	}

	store.DeleteWorkspace("ws-a")
	items, status, err = store.List()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if status.GlobalRevision != 2 || status.WorkspaceTotal != 1 {
		t.Fatalf("status after delete = %+v, want revision 2 total 1", status)
	}
	if len(items) != 1 || items[0].ID != "ws-b" {
		t.Fatalf("items after delete = %+v, want only ws-b", items)
	}
}

func TestStoreDegradedAndDetailNotReady(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, nil, errors.New("snapshot unavailable"))

	items, status, err := store.List()
	if err != nil {
		t.Fatalf("list degraded workspaces: %v", err)
	}
	if status.State != StateDegraded || status.Error == "" || len(items) != 1 {
		t.Fatalf("status/items = %+v/%+v, want degraded with one overview", status, items)
	}
	if !status.Degraded {
		t.Fatalf("status = %+v, want degraded flag", status)
	}
	if _, detailStatus, err := store.GetSnapshot("ws-a"); err == nil {
		t.Fatal("get missing detail succeeded, want error")
	} else if detailStatus.State != StateDegraded {
		t.Fatalf("detail status = %+v, want degraded", detailStatus)
	}
}

func TestStoreUpsertWorkspaceSynchronizesDetailWorkspaceState(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "waiting_decision", ActiveRunID: "run-a", BlockingTicketID: "ticket-a"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace: readmodelcore.Workspace{ID: "ws-a", State: "waiting_decision", ActiveRunID: "run-a", BlockingTicketID: "ticket-a"},
			RunCommand: &readmodelcore.RunCommandStatus{
				WorkspaceID: "ws-a",
				RunID:       "run-a",
				Intent:      "replan",
				RequestID:   "request-a",
				Revision:    7,
			},
			GapBaseline: &readmodelcore.GapBaseline{
				WorkspaceID:      "ws-a",
				RunID:            "run-a",
				BaselineRevision: "gap-a",
			},
		},
	}, nil)

	store.UpsertWorkspace(appmodelcore.Workspace{ID: "ws-a", State: "running", ActiveRunID: "run-a"})

	items, _, err := store.List()
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(items) != 1 || items[0].State != "running" || items[0].BlockingTicketID != "" {
		t.Fatalf("list workspace = %+v, want running without blocker", items)
	}
	snapshot, _, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.Workspace.State != "running" || snapshot.Workspace.BlockingTicketID != "" {
		t.Fatalf("snapshot workspace = %+v, want synchronized running state", snapshot.Workspace)
	}
	if snapshot.RunCommand == nil || snapshot.RunCommand.RequestID != "request-a" {
		t.Fatalf("RunCommand = %+v, want retained command projection", snapshot.RunCommand)
	}
	if snapshot.GapBaseline == nil || snapshot.GapBaseline.BaselineRevision != "gap-a" {
		t.Fatalf("GapBaseline = %+v, want retained baseline projection", snapshot.GapBaseline)
	}
}

func TestStoreReplaceWorkspaceListSynchronizesDetailWorkspaceState(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "waiting_decision", ActiveRunID: "run-a", BlockingTicketID: "ticket-a"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace:  readmodelcore.Workspace{ID: "ws-a", State: "waiting_decision", ActiveRunID: "run-a", BlockingTicketID: "ticket-a"},
			RunCommand: &readmodelcore.RunCommandStatus{WorkspaceID: "ws-a", RunID: "run-a", Intent: "resume", RequestID: "request-a", Revision: 9},
			GapBaseline: &readmodelcore.GapBaseline{
				WorkspaceID:      "ws-a",
				RunID:            "run-a",
				BaselineRevision: "gap-a",
			},
		},
	}, nil)

	store.ReplaceWorkspaceList([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, nil)

	snapshot, _, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.Workspace.State != "active" || snapshot.Workspace.ActiveRunID != "" || snapshot.Workspace.BlockingTicketID != "" {
		t.Fatalf("snapshot workspace = %+v, want synchronized active state", snapshot.Workspace)
	}
	if snapshot.RunCommand == nil || snapshot.RunCommand.RequestID != "request-a" {
		t.Fatalf("RunCommand = %+v, want retained command projection", snapshot.RunCommand)
	}
	if snapshot.GapBaseline == nil || snapshot.GapBaseline.BaselineRevision != "gap-a" {
		t.Fatalf("GapBaseline = %+v, want retained baseline projection", snapshot.GapBaseline)
	}
}

func TestStoreMarksStaleWithSourceEventRevision(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: readmodelcore.Workspace{ID: "ws-a", State: "active"}},
	}, nil)

	store.MarkStale("ws-a", 42)
	_, status, err := store.List()
	if err != nil {
		t.Fatalf("list stale workspaces: %v", err)
	}
	if status.State != StateStale || status.SourceEventRevision != 42 {
		t.Fatalf("status = %+v, want stale source revision 42", status)
	}
	_, detailStatus, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get stale snapshot: %v", err)
	}
	if detailStatus.State != StateStale || detailStatus.SourceEventRevision != 42 {
		t.Fatalf("detail status = %+v, want stale source revision 42", detailStatus)
	}
	workspaceStatus := store.StatusForWorkspace("ws-a")
	if workspaceStatus.State != StateStale || workspaceStatus.SourceEventRevision != 42 {
		t.Fatalf("workspace status = %+v, want stale source revision 42", workspaceStatus)
	}

	store.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{Workspace: readmodelcore.Workspace{ID: "ws-a", State: "running"}})
	_, status, err = store.List()
	if err != nil {
		t.Fatalf("list rebuilt workspaces: %v", err)
	}
	if status.State != StateReady || status.SourceEventRevision != 42 {
		t.Fatalf("rebuilt status = %+v, want ready retaining source revision 42", status)
	}
}

func TestStoreWorkspaceStatusUsesGlobalRevisionForQuietSnapshot(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{
		{ID: "ws-quiet", State: "blocked"},
		{ID: "ws-active", State: "running"},
	}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-quiet":  {Workspace: readmodelcore.Workspace{ID: "ws-quiet", State: "blocked"}},
		"ws-active": {Workspace: readmodelcore.Workspace{ID: "ws-active", State: "running"}},
	}, nil)

	_, initialStatus, err := store.GetSnapshot("ws-quiet")
	if err != nil {
		t.Fatalf("get initial quiet snapshot: %v", err)
	}
	initialProjectionBytes, err := json.Marshal(initialStatus)
	if err != nil {
		t.Fatalf("marshal initial status: %v", err)
	}
	var initialProjection map[string]any
	if err := json.Unmarshal(initialProjectionBytes, &initialProjection); err != nil {
		t.Fatalf("unmarshal initial status: %v", err)
	}
	if _, ok := initialProjection["sourceEventRevision"]; !ok {
		t.Fatalf("initial projection json = %s, want sourceEventRevision field even when zero", initialProjectionBytes)
	}
	if _, ok := initialProjection["projectedSourceRevision"]; !ok {
		t.Fatalf("initial projection json = %s, want projectedSourceRevision field even when zero", initialProjectionBytes)
	}

	store.MarkStale("ws-active", 99)

	_, quietStatus, err := store.GetSnapshot("ws-quiet")
	if err != nil {
		t.Fatalf("get quiet snapshot: %v", err)
	}
	if quietStatus.State != StateReady {
		t.Fatalf("quiet status = %+v, want ready quiet workspace status", quietStatus)
	}
	if quietStatus.SourceEventRevision != 99 || quietStatus.ProjectedSourceRevision != 99 {
		t.Fatalf("quiet status revisions = %+v, want source/projected revision 99", quietStatus)
	}
	if quietStatus.WorkspaceRevision == 0 || quietStatus.UpdatedAt == "" || quietStatus.RebuiltAt == "" {
		t.Fatalf("quiet status display metadata = %+v, want workspace revision and timestamps", quietStatus)
	}
}

func TestStoreKeepsExistingProjectionReadableWhileRebuilding(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: readmodelcore.Workspace{ID: "ws-a", State: "running"}},
	}, nil)

	store.setBuilding()

	items, status, err := store.List()
	if err != nil {
		t.Fatalf("list stale workspaces during rebuild: %v", err)
	}
	if status.State != StateStale || len(items) != 1 || items[0].ID != "ws-a" {
		t.Fatalf("status/items = %+v/%+v, want stale existing projection", status, items)
	}
	snapshot, detailStatus, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get stale snapshot during rebuild: %v", err)
	}
	if detailStatus.State != StateStale || snapshot.Workspace.State != "running" {
		t.Fatalf("detail/status = %+v/%+v, want stale running snapshot", snapshot, detailStatus)
	}
}

func TestStoreRunHeartbeatMetricsCountsActiveProblemSignals(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	store.ReplaceAll([]appmodelcore.Workspace{
		{ID: "ws-active", State: "running"},
		{ID: "ws-terminal", State: "completed"},
	}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-active": {
			Workspace: readmodelcore.Workspace{ID: "ws-active", State: "running"},
			RunHeartbeat: readmodelcore.RunHeartbeat{
				WorkspaceID:               "ws-active",
				RunID:                     "run-active",
				State:                     string(common.RunStateRunning),
				ActivityState:             "stalled",
				LatestEffectiveProgressAt: now.Add(-12 * time.Minute).Format(time.RFC3339Nano),
				Stalled:                   true,
				IneffectiveRetry:          true,
			},
			Acceptance: readmodelcore.AcceptanceProjection{
				Summary: readmodelcore.AcceptanceSummary{ActiveSlices: 1},
				CurrentFocus: &readmodelcore.CoverageFocus{
					ID:      "focus-active",
					State:   "active",
					Attempt: 2,
				},
				Budget: &readmodelcore.ConvergenceBudget{
					Decision:              "decision_required",
					RepairAttempts:        3,
					NoCoverageDeltaRounds: 1,
				},
			},
		},
		"ws-terminal": {
			Workspace: readmodelcore.Workspace{ID: "ws-terminal", State: "completed"},
			RunHeartbeat: readmodelcore.RunHeartbeat{
				WorkspaceID:               "ws-terminal",
				RunID:                     "run-terminal",
				State:                     string(common.RunStateCompleted),
				ActivityState:             "stalled",
				LatestEffectiveProgressAt: now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
				Stalled:                   true,
				IneffectiveRetry:          true,
			},
		},
	}, nil)

	metrics := store.RunHeartbeatMetrics(now)
	if metrics.ActiveRunHeartbeatTotal != 1 {
		t.Fatalf("active heartbeat total = %d, want 1", metrics.ActiveRunHeartbeatTotal)
	}
	if metrics.ActiveRunHeartbeatAgeSecondsMax != int64((12 * time.Minute).Seconds()) {
		t.Fatalf("active heartbeat age max = %d, want 720", metrics.ActiveRunHeartbeatAgeSecondsMax)
	}
	if metrics.StalledRunTotal != 1 || metrics.IneffectiveRetryRunTotal != 1 {
		t.Fatalf("problem signal totals = stalled %d ineffective %d, want 1/1", metrics.StalledRunTotal, metrics.IneffectiveRetryRunTotal)
	}
	if metrics.ConvergenceActiveSlices != 1 ||
		metrics.ConvergenceOpenFocuses != 1 ||
		metrics.ConvergenceRepairAttemptsTotal != 3 ||
		metrics.ConvergenceNoDeltaRoundsTotal != 1 ||
		metrics.ConvergenceBudgetEscalations != 1 {
		t.Fatalf("convergence metrics = %+v, want active/open/repair/no-delta/escalation 1/1/3/1/1", metrics)
	}
}

func TestManagerPublishesProjectionChangedAndIgnoresOwnEvent(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	scheduled := map[string]int{}
	scheduledFuncs := map[string][]func(context.Context){}
	manager := NewManager(ManagerOptions{
		Loader: fakeProjectionLoader{
			workspaces: []appmodelcore.Workspace{{ID: "ws-a", State: "active"}},
			snapshots: map[string]readmodelcore.WorkspaceSnapshot{
				"ws-a": {Workspace: readmodelcore.Workspace{ID: "ws-a", State: "running"}},
			},
		},
		Hub: hub,
		Schedule: func(name string, fn func(context.Context)) bool {
			scheduled[name]++
			scheduledFuncs[name] = append(scheduledFuncs[name], fn)
			return true
		},
	})
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	if scheduled["workspace_projection_rebuild_all"] != 1 {
		t.Fatalf("startup full rebuild scheduled %d times, want 1", scheduled["workspace_projection_rebuild_all"])
	}
	scheduledFuncs["workspace_projection_rebuild_all"][0](ctx)
	baselineWorkspaceRebuilds := scheduled["workspace_projection_rebuild"]
	if baselineWorkspaceRebuilds != 0 {
		t.Fatalf("baseline workspace rebuilds = %d, want no detail fan-out", baselineWorkspaceRebuilds)
	}
	if err := manager.RebuildWorkspace(ctx, "ws-a"); err != nil {
		t.Fatalf("rebuild workspace: %v", err)
	}
	if scheduled["workspace_projection_rebuild"] != baselineWorkspaceRebuilds {
		t.Fatalf("scheduled own projection event rebuild count = %d, want unchanged %d", scheduled["workspace_projection_rebuild"], baselineWorkspaceRebuilds)
	}
	var found events.Envelope
	for _, item := range hub.Published() {
		if item.Topic == workorchdevents.TopicWorkspaceProjectionChanged && item.WorkspaceID == "ws-a" {
			found = item
		}
	}
	if found.Topic == "" {
		t.Fatalf("published events = %+v, want workspace projection changed for ws-a", hub.Published())
	}
	if found.Payload["changeKind"] != "detail" || found.Payload["projectionState"] != StateReady {
		t.Fatalf("projection payload = %+v, want detail ready", found.Payload)
	}

	if err := hub.Publish(ctx, events.Envelope{
		ID:          "run-a/state",
		Version:     events.WorkorchEventVersion,
		Topic:       runevents.TopicRunStateChanged,
		WorkspaceID: "ws-a",
		RunID:       "run-a",
		SubjectType: "run",
		SubjectID:   "run-a",
		Payload:     events.RunPayload("", "", common.RunStateCompleted, "", "", ""),
	}); err != nil {
		t.Fatalf("publish run state event: %v", err)
	}
	if scheduled["workspace_projection_rebuild"] != baselineWorkspaceRebuilds+1 {
		t.Fatalf("scheduled rebuild count after run event = %d, want %d", scheduled["workspace_projection_rebuild"], baselineWorkspaceRebuilds+1)
	}
	status := manager.Status()
	if status.State != StateStale || status.SourceEventRevision == 0 {
		t.Fatalf("manager status after run event = %+v, want stale with source revision", status)
	}
}

func TestManagerPublishesProjectionChangedThroughAsyncHub(t *testing.T) {
	ctx := context.Background()
	hub := &recordingAsyncProjectionHub{}
	manager := NewManager(ManagerOptions{Hub: hub})
	manager.store.UpsertWorkspace(appmodelcore.Workspace{ID: "ws-a", State: "active"})

	manager.publishProjectionChanged(ctx, "ws-a", "detail")

	if hub.asyncCalls != 1 || hub.syncCalls != 0 {
		t.Fatalf("publish calls = async:%d sync:%d, want projection notification through async hub", hub.asyncCalls, hub.syncCalls)
	}
	if hub.last.Topic != workorchdevents.TopicWorkspaceProjectionChanged || hub.last.WorkspaceID != "ws-a" || hub.last.Payload["changeKind"] != "detail" {
		t.Fatalf("projection event = %+v, want workspace projection changed detail", hub.last)
	}
}

func TestManagerAppliesLLMProgressWithoutFullWorkspaceRebuildOrProjectionFanout(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	scheduled := map[string]int{}
	scheduledFuncs := map[string][]func(context.Context){}
	manager := NewManager(ManagerOptions{
		Loader: fakeProjectionLoader{
			workspaces: []appmodelcore.Workspace{{ID: "ws-a", State: "running", ActiveRunID: "run-a"}},
			snapshots: map[string]readmodelcore.WorkspaceSnapshot{
				"ws-a": {
					Workspace: readmodelcore.Workspace{ID: "ws-a", State: "running", ActiveRunID: "run-a"},
					Run:       &readmodelcore.Run{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateRunning)},
				},
			},
		},
		Hub: hub,
		Schedule: func(name string, fn func(context.Context)) bool {
			scheduled[name]++
			scheduledFuncs[name] = append(scheduledFuncs[name], fn)
			return true
		},
	})
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	scheduledFuncs["workspace_projection_rebuild_all"][0](ctx)
	if err := manager.RebuildWorkspace(ctx, "ws-a"); err != nil {
		t.Fatalf("rebuild workspace detail: %v", err)
	}
	baselineWorkspaceRebuilds := scheduled["workspace_projection_rebuild"]

	if err := hub.Publish(ctx, events.Envelope{
		Version:     events.WorkorchEventVersion,
		Topic:       runevents.TopicRunLLMStreamProgress,
		WorkspaceID: "ws-a",
		RunID:       "run-a",
		StepID:      "run-a/step-1",
		Payload: events.LLMStreamProgressPayload(
			"ws-a",
			"run-a",
			"run-a/step-1",
			"provider-a",
			"model-a",
			2,
			128,
			250,
			string(common.StepStateDispatched),
			"provider call waiting",
		),
	}); err != nil {
		t.Fatalf("publish llm progress event: %v", err)
	}
	if scheduled["workspace_projection_rebuild"] != baselineWorkspaceRebuilds {
		t.Fatalf("workspace rebuilds after progress = %d, want unchanged %d", scheduled["workspace_projection_rebuild"], baselineWorkspaceRebuilds)
	}
	snapshot, status, err := manager.ReadWorkspaceSnapshot("ws-a")
	if err != nil {
		t.Fatalf("read snapshot after progress: %v", err)
	}
	if status.State != StateReady || status.SourceEventRevision == 0 || status.ProjectedSourceRevision != status.SourceEventRevision {
		t.Fatalf("status after progress = %+v, want ready projected progress revision", status)
	}
	if snapshot.RunHeartbeat.LatestEventTopic != runevents.TopicRunLLMStreamProgress ||
		snapshot.RunHeartbeat.LatestStepID != "run-a/step-1" ||
		snapshot.RunHeartbeat.LatestStepState != string(common.StepStateDispatched) ||
		snapshot.RunHeartbeat.ActivityState != "running" {
		t.Fatalf("heartbeat after progress = %+v", snapshot.RunHeartbeat)
	}
	if snapshot.RuntimeExecution == nil ||
		snapshot.RuntimeExecution.StepID != "run-a/step-1" ||
		snapshot.RuntimeExecution.LastProgressSummary != "provider call waiting" ||
		!snapshot.RuntimeExecution.Pending {
		t.Fatalf("runtime execution after progress = %+v", snapshot.RuntimeExecution)
	}
	var progressProjectionCount int
	for _, item := range hub.Published() {
		if item.Topic == workorchdevents.TopicWorkspaceProjectionChanged && item.Payload["changeKind"] == "runtime_progress" {
			progressProjectionCount++
		}
	}
	if progressProjectionCount != 0 {
		t.Fatalf("published runtime_progress projection events = %d, want none; events=%+v", progressProjectionCount, hub.Published())
	}
}

func TestStoreRejectsRuntimeProgressForDifferentRun(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "running", ActiveRunID: "run-a"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace: readmodelcore.Workspace{ID: "ws-a", State: "running", ActiveRunID: "run-a"},
			Run:       &readmodelcore.Run{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateRunning)},
		},
	}, nil)

	if store.ApplyRuntimeProgress(RuntimeProgressUpdate{
		WorkspaceID: "ws-a",
		RunID:       "run-b",
		StepID:      "run-b/step-1",
		StepState:   string(common.StepStateDispatched),
		Topic:       runevents.TopicRunLLMStreamProgress,
		Revision:    10,
		Summary:     "wrong run progress",
	}) {
		t.Fatalf("ApplyRuntimeProgress returned true for different run")
	}
	snapshot, status, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot after rejected progress: %v", err)
	}
	if status.SourceEventRevision != 0 || status.ProjectedSourceRevision != 0 {
		t.Fatalf("status after rejected progress = %+v, want unchanged source revisions", status)
	}
	if snapshot.RunHeartbeat.LatestStepID != "" || snapshot.RuntimeExecution != nil {
		t.Fatalf("snapshot after rejected progress = %+v, want unchanged runtime progress fields", snapshot)
	}
}

func TestStoreRuntimeProgressResetsExecutionStateForNewStep(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "running", ActiveRunID: "run-a"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace: readmodelcore.Workspace{ID: "ws-a", State: "running", ActiveRunID: "run-a"},
			Run:       &readmodelcore.Run{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateRunning)},
			RuntimeExecution: &readmodelcore.RuntimeExecutionState{
				StepID:          "run-a/old-step",
				RequestID:       "old-request",
				QueueKey:        "old-queue",
				WorkerStoppedAt: "2026-06-20T00:00:10Z",
				DoneAt:          "2026-06-20T00:00:09Z",
				Pending:         true,
				Gap:             "runtime_terminal_not_finalized",
			},
		},
	}, nil)

	if !store.ApplyRuntimeProgress(RuntimeProgressUpdate{
		WorkspaceID: "ws-a",
		RunID:       "run-a",
		StepID:      "run-a/new-step",
		StepKind:    "llm/workspace_execution",
		StepState:   string(common.StepStateDispatched),
		Topic:       runevents.TopicRunLLMStreamProgress,
		Revision:    10,
		Summary:     "provider call waiting",
	}) {
		t.Fatalf("ApplyRuntimeProgress returned false for new step progress")
	}
	snapshot, _, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot after progress: %v", err)
	}
	if snapshot.RuntimeExecution == nil {
		t.Fatal("RuntimeExecution is nil, want new step progress state")
	}
	if snapshot.RuntimeExecution.StepID != "run-a/new-step" ||
		snapshot.RuntimeExecution.StepKind != "llm/workspace_execution" ||
		snapshot.RuntimeExecution.LastProgressSummary != "provider call waiting" ||
		!snapshot.RuntimeExecution.Pending ||
		snapshot.RuntimeExecution.Gap != "accepted_in_progress" {
		t.Fatalf("RuntimeExecution = %+v, want new step progress state", snapshot.RuntimeExecution)
	}
	if snapshot.RuntimeExecution.RequestID != "" ||
		snapshot.RuntimeExecution.QueueKey != "" ||
		snapshot.RuntimeExecution.WorkerStoppedAt != "" ||
		snapshot.RuntimeExecution.DoneAt != "" {
		t.Fatalf("RuntimeExecution retained old step fields: %+v", snapshot.RuntimeExecution)
	}
}

func TestStoreRunCommandAcceptedProjectsCommandRevision(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace: readmodelcore.Workspace{ID: "ws-a", State: "active"},
		},
	}, nil)

	if !store.ApplyRunCommandAccepted(RunCommandAcceptedUpdate{
		WorkspaceID:        "ws-a",
		RunID:              "run-a",
		Intent:             "resume",
		Reason:             "user_requested",
		RequestID:          "request-a",
		IdempotencyKey:     "resume/ws-a/run-a",
		Source:             "tui",
		DefinitionRevision: "rev-a",
		DecisionID:         "ticket-a",
		Revision:           42,
		AcceptedAt:         "2026-06-21T00:00:00Z",
	}) {
		t.Fatalf("ApplyRunCommandAccepted returned false")
	}
	snapshot, status, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.WorkspaceID != "ws-a" ||
		snapshot.RunCommand.RunID != "run-a" ||
		snapshot.RunCommand.State != "accepted" ||
		snapshot.RunCommand.Intent != "resume" ||
		snapshot.RunCommand.RequestID != "request-a" ||
		snapshot.RunCommand.IdempotencyKey != "resume/ws-a/run-a" ||
		snapshot.RunCommand.DefinitionRevision != "rev-a" ||
		snapshot.RunCommand.DecisionID != "ticket-a" ||
		snapshot.RunCommand.Revision != 42 {
		t.Fatalf("RunCommand = %+v, want accepted command projection", snapshot.RunCommand)
	}
	if status.SourceEventRevision != 42 || status.ProjectedSourceRevision != 42 {
		t.Fatalf("status revisions = %+v, want accepted command revision", status)
	}
}

func TestStoreRunCommandAcceptedCreatesDetailPlaceholderForKnownWorkspace(t *testing.T) {
	store := NewStore()
	store.ReplaceWorkspaceList([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, nil)

	if !store.ApplyRunCommandAccepted(RunCommandAcceptedUpdate{
		WorkspaceID: "ws-a",
		RunID:       "run-a",
		Intent:      "replan",
		RequestID:   "request-a",
		Revision:    42,
	}) {
		t.Fatalf("ApplyRunCommandAccepted returned false")
	}
	snapshot, status, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.Workspace.ID != "ws-a" || snapshot.Workspace.State != "active" {
		t.Fatalf("Workspace = %+v, want placeholder workspace detail", snapshot.Workspace)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.WorkspaceID != "ws-a" ||
		snapshot.RunCommand.RunID != "run-a" ||
		snapshot.RunCommand.State != "accepted" ||
		snapshot.RunCommand.Intent != "replan" ||
		snapshot.RunCommand.RequestID != "request-a" ||
		snapshot.RunCommand.Revision != 42 {
		t.Fatalf("RunCommand = %+v, want accepted command projection", snapshot.RunCommand)
	}
	if status.SourceEventRevision != 42 || status.ProjectedSourceRevision != 42 {
		t.Fatalf("status revisions = %+v, want accepted command revision", status)
	}
}

func TestStoreUpsertSnapshotPreservesAcceptedRunCommand(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: readmodelcore.Workspace{ID: "ws-a", State: "active"}},
	}, nil)
	if !store.ApplyRunCommandAccepted(RunCommandAcceptedUpdate{
		WorkspaceID: "ws-a",
		Intent:      "resume",
		RequestID:   "request-a",
		Revision:    42,
	}) {
		t.Fatalf("ApplyRunCommandAccepted returned false")
	}

	store.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-a", State: "running"},
	})

	snapshot, status, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.Intent != "resume" ||
		snapshot.RunCommand.RequestID != "request-a" ||
		snapshot.RunCommand.Revision != 42 {
		t.Fatalf("RunCommand = %+v, want preserved accepted command", snapshot.RunCommand)
	}
	if status.SourceEventRevision != 42 {
		t.Fatalf("status = %+v, want source revision retained", status)
	}
}

func TestStoreRunCommandAcceptedRejectsUnknownWorkspace(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: readmodelcore.Workspace{ID: "ws-a", State: "active"}},
	}, nil)
	if store.ApplyRunCommandAccepted(RunCommandAcceptedUpdate{WorkspaceID: "ws-b", Intent: "resume", Revision: 2}) {
		t.Fatal("ApplyRunCommandAccepted returned true for unknown workspace")
	}
	snapshot, status, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.RunCommand != nil {
		t.Fatalf("RunCommand = %+v, want no cross-workspace command projection", snapshot.RunCommand)
	}
	if status.SourceEventRevision != 0 || status.ProjectedSourceRevision != 0 {
		t.Fatalf("status after rejected command = %+v, want unchanged revisions", status)
	}
}

func TestStoreRunCommandRejectedProjectsCommandRevision(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace: readmodelcore.Workspace{ID: "ws-a", State: "active"},
		},
	}, nil)

	if !store.ApplyRunCommandRejected(RunCommandRejectedUpdate{
		WorkspaceID:        "ws-a",
		RunID:              "run-a",
		Intent:             "resume",
		Reason:             "user_requested",
		RequestID:          "request-a",
		IdempotencyKey:     "resume/ws-a/run-a",
		Source:             "tui",
		DefinitionRevision: "rev-a",
		DecisionID:         "ticket-a",
		Revision:           43,
		RejectedAt:         "2026-06-21T00:01:00Z",
		Error:              "scheduler command was not queued",
	}) {
		t.Fatalf("ApplyRunCommandRejected returned false")
	}
	snapshot, status, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.RunCommand == nil ||
		snapshot.RunCommand.WorkspaceID != "ws-a" ||
		snapshot.RunCommand.RunID != "run-a" ||
		snapshot.RunCommand.State != "rejected" ||
		snapshot.RunCommand.Intent != "resume" ||
		snapshot.RunCommand.RequestID != "request-a" ||
		snapshot.RunCommand.IdempotencyKey != "resume/ws-a/run-a" ||
		snapshot.RunCommand.DefinitionRevision != "rev-a" ||
		snapshot.RunCommand.DecisionID != "ticket-a" ||
		snapshot.RunCommand.RejectedAt != "2026-06-21T00:01:00Z" ||
		snapshot.RunCommand.Error != "scheduler command was not queued" ||
		snapshot.RunCommand.Revision != 43 {
		t.Fatalf("RunCommand = %+v, want rejected command projection", snapshot.RunCommand)
	}
	if status.SourceEventRevision != 43 || status.ProjectedSourceRevision != 43 {
		t.Fatalf("status revisions = %+v, want rejected command revision", status)
	}
}

func TestStoreRunCommandRejectedRejectsUnknownWorkspace(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: readmodelcore.Workspace{ID: "ws-a", State: "active"}},
	}, nil)
	if store.ApplyRunCommandRejected(RunCommandRejectedUpdate{WorkspaceID: "ws-b", Intent: "resume", Revision: 2}) {
		t.Fatal("ApplyRunCommandRejected returned true for unknown workspace")
	}
	snapshot, status, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshot.RunCommand != nil {
		t.Fatalf("RunCommand = %+v, want no cross-workspace command projection", snapshot.RunCommand)
	}
	if status.SourceEventRevision != 0 || status.ProjectedSourceRevision != 0 {
		t.Fatalf("status after rejected command = %+v, want unchanged revisions", status)
	}
}

func TestStoreRuntimeProgressClearsGapForTerminalStep(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "running", ActiveRunID: "run-a"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {
			Workspace: readmodelcore.Workspace{ID: "ws-a", State: "running", ActiveRunID: "run-a"},
			Run:       &readmodelcore.Run{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateRunning)},
			RuntimeExecution: &readmodelcore.RuntimeExecutionState{
				StepID:  "run-a/step-1",
				Pending: true,
				Gap:     "accepted_in_progress",
			},
		},
	}, nil)

	if !store.ApplyRuntimeProgress(RuntimeProgressUpdate{
		WorkspaceID: "ws-a",
		RunID:       "run-a",
		StepID:      "run-a/step-1",
		StepState:   string(common.StepStateCompleted),
		Topic:       runevents.TopicRunLLMStreamProgress,
		Revision:    10,
		Summary:     "step completed",
	}) {
		t.Fatalf("ApplyRuntimeProgress returned false for terminal progress")
	}
	snapshot, _, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot after terminal progress: %v", err)
	}
	if snapshot.RuntimeExecution == nil || snapshot.RuntimeExecution.Pending || snapshot.RuntimeExecution.Gap != "" {
		t.Fatalf("RuntimeExecution = %+v, want terminal progress without pending gap", snapshot.RuntimeExecution)
	}
}

func TestManagerStartRequiresScheduler(t *testing.T) {
	manager := NewManager(ManagerOptions{
		Loader: fakeProjectionLoader{},
	})
	if err := manager.Start(context.Background()); err == nil {
		t.Fatal("Start returned nil error without scheduler")
	}
}

func TestManagerReschedulesWorkspaceRebuildWhenEventArrivesInFlight(t *testing.T) {
	ctx := context.Background()
	loader := &countingProjectionLoader{}
	var scheduled []func(context.Context)
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(name string, fn func(context.Context)) bool {
			if name != "workspace_projection_rebuild" {
				t.Fatalf("scheduled task = %q, want workspace_projection_rebuild", name)
			}
			scheduled = append(scheduled, fn)
			return true
		},
	})

	manager.ScheduleRebuildWorkspace("ws-a")
	manager.ScheduleRebuildWorkspace("ws-a")
	if len(scheduled) != 1 {
		t.Fatalf("scheduled rebuilds = %d, want one in-flight rebuild before pending rerun", len(scheduled))
	}
	if metrics := manager.Metrics(); metrics.WorkspaceRebuildDebouncedTotal != 1 || metrics.PendingWorkspaceRebuilds != 1 {
		t.Fatalf("debounce metrics = %+v, want one debounced pending workspace rebuild", metrics)
	}

	scheduled[0](ctx)
	if len(scheduled) != 2 {
		t.Fatalf("scheduled rebuilds after first run = %d, want pending rerun", len(scheduled))
	}
	scheduled[1](ctx)

	snapshot, _, err := manager.ReadWorkspaceSnapshot("ws-a")
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if snapshot.Workspace.Title != "snapshot-2" || loader.calls != 2 {
		t.Fatalf("snapshot title/calls = %q/%d, want second rebuild to win", snapshot.Workspace.Title, loader.calls)
	}
}

func TestManagerFullRebuildDoesNotLoadWorkspaceDetails(t *testing.T) {
	ctx := context.Background()
	loader := &countingProjectionLoader{}
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(string, func(context.Context)) bool {
			return false
		},
	})

	if err := manager.RebuildAll(ctx); err != nil {
		t.Fatalf("rebuild all: %v", err)
	}
	if loader.calls != 0 {
		t.Fatalf("detail snapshot calls after full rebuild = %d, want 0", loader.calls)
	}
	items, status, err := manager.ListWorkspaces()
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if status.State != StateReady || len(items) != 1 || items[0].ID != "ws-a" {
		t.Fatalf("items/status = %+v/%+v, want ready overview projection", items, status)
	}
	if _, _, err := manager.ReadWorkspaceSnapshot("ws-a"); err == nil {
		t.Fatal("memory snapshot read succeeded before detail load, want detail not ready")
	}
	snapshot, _, err := manager.ReadWorkspaceSnapshotWithContext(ctx, "ws-a")
	if err != nil {
		t.Fatalf("on-demand snapshot read: %v", err)
	}
	if loader.calls != 1 || snapshot.Workspace.Title != "snapshot-1" {
		t.Fatalf("snapshot title/calls = %q/%d, want single on-demand detail load", snapshot.Workspace.Title, loader.calls)
	}
}

func TestManagerSchedulesIndependentWorkspaceRebuildsWithPerWorkspaceDebounce(t *testing.T) {
	ctx := context.Background()
	loader := &workspaceCountingProjectionLoader{}
	var scheduled []func(context.Context)
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(name string, fn func(context.Context)) bool {
			if name != "workspace_projection_rebuild" {
				t.Fatalf("scheduled task = %q, want workspace_projection_rebuild", name)
			}
			scheduled = append(scheduled, fn)
			return true
		},
	})

	manager.ScheduleRebuildWorkspace("ws-a")
	manager.ScheduleRebuildWorkspace("ws-b")
	manager.ScheduleRebuildWorkspace("ws-a")
	if len(scheduled) != 2 {
		t.Fatalf("scheduled rebuilds = %d, want one task per distinct workspace before pending rerun", len(scheduled))
	}
	if metrics := manager.Metrics(); metrics.WorkspaceRebuildDebouncedTotal != 1 || metrics.PendingWorkspaceRebuilds != 1 {
		t.Fatalf("debounce metrics = %+v, want only ws-a pending rerun", metrics)
	}

	scheduled[1](ctx)
	if got := loader.callsFor("ws-b"); got != 1 {
		t.Fatalf("ws-b rebuild calls = %d, want independent rebuild while ws-a is pending", got)
	}
	if len(scheduled) != 2 {
		t.Fatalf("scheduled rebuilds after ws-b run = %d, want no cross-workspace rerun", len(scheduled))
	}

	scheduled[0](ctx)
	if len(scheduled) != 3 {
		t.Fatalf("scheduled rebuilds after first ws-a run = %d, want pending ws-a rerun", len(scheduled))
	}
	scheduled[2](ctx)
	if got := loader.callsFor("ws-a"); got != 2 {
		t.Fatalf("ws-a rebuild calls = %d, want pending rerun", got)
	}
	if metrics := manager.Metrics(); metrics.PendingWorkspaceRebuilds != 0 {
		t.Fatalf("pending workspace rebuilds = %d, want drained", metrics.PendingWorkspaceRebuilds)
	}
	snapshotA, _, err := manager.ReadWorkspaceSnapshot("ws-a")
	if err != nil {
		t.Fatalf("read ws-a snapshot: %v", err)
	}
	snapshotB, _, err := manager.ReadWorkspaceSnapshot("ws-b")
	if err != nil {
		t.Fatalf("read ws-b snapshot: %v", err)
	}
	if snapshotA.Workspace.Title != "ws-a-snapshot-2" || snapshotB.Workspace.Title != "ws-b-snapshot-1" {
		t.Fatalf("snapshots = %q/%q, want per-workspace rebuild results", snapshotA.Workspace.Title, snapshotB.Workspace.Title)
	}
}

func TestManagerSchedulesWorkspaceRebuildAfterWorkspaceEventDuringFullRebuild(t *testing.T) {
	ctx := context.Background()
	loader := &blockingProjectionLoader{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	var (
		scheduledMu sync.Mutex
		scheduled   []func(context.Context)
	)
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(name string, fn func(context.Context)) bool {
			scheduledMu.Lock()
			scheduled = append(scheduled, fn)
			scheduledMu.Unlock()
			return true
		},
	})

	done := make(chan error, 1)
	go func() {
		done <- manager.RebuildAll(ctx)
	}()
	<-loader.started

	if err := manager.handleEvent(ctx, events.Envelope{
		Topic:       runevents.TopicRunStateChanged,
		WorkspaceID: "ws-a",
		SubjectType: "run",
		SubjectID:   "run-a",
		Revision:    7,
	}); err != nil {
		t.Fatalf("handle workspace event: %v", err)
	}
	close(loader.release)
	if err := <-done; err != nil {
		t.Fatalf("full rebuild: %v", err)
	}

	scheduledMu.Lock()
	if len(scheduled) != 1 {
		t.Fatalf("scheduled rebuilds = %d, want one workspace rebuild", len(scheduled))
	}
	workspaceRebuild := scheduled[0]
	scheduledMu.Unlock()
	workspaceRebuild(ctx)
	if loader.calls != 1 {
		t.Fatalf("loader list calls = %d, want only initial full rebuild", loader.calls)
	}
	snapshot, _, err := manager.ReadWorkspaceSnapshot("ws-a")
	if err != nil {
		t.Fatalf("read workspace snapshot: %v", err)
	}
	if snapshot.Workspace.Title != "snapshot-1" {
		t.Fatalf("snapshot title = %q, want snapshot-1", snapshot.Workspace.Title)
	}
}

func TestManagerSchedulesFullRebuildAfterGlobalWorkspaceEventDuringFullRebuild(t *testing.T) {
	ctx := context.Background()
	loader := &blockingProjectionLoader{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	var (
		scheduledMu sync.Mutex
		scheduled   []scheduledProjectionTask
	)
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(name string, fn func(context.Context)) bool {
			scheduledMu.Lock()
			scheduled = append(scheduled, scheduledProjectionTask{name: name, fn: fn})
			scheduledMu.Unlock()
			return true
		},
	})

	done := make(chan error, 1)
	go func() {
		done <- manager.RebuildAll(ctx)
	}()
	<-loader.started

	if err := manager.handleEvent(ctx, events.Envelope{
		Topic:       runevents.TopicRunStateChanged,
		SubjectType: "workspace",
		SubjectID:   "ws-a",
		Revision:    8,
	}); err != nil {
		t.Fatalf("handle global workspace event: %v", err)
	}
	close(loader.release)
	if err := <-done; err != nil {
		t.Fatalf("full rebuild: %v", err)
	}

	scheduledMu.Lock()
	if len(scheduled) != 1 {
		t.Fatalf("scheduled rebuilds = %d, want follow-up full rebuild only", len(scheduled))
	}
	if scheduled[0].name != "workspace_projection_rebuild_all" {
		t.Fatalf("scheduled task name = %q, want workspace_projection_rebuild_all", scheduled[0].name)
	}
	followup := scheduled[0].fn
	scheduledMu.Unlock()
	followup(ctx)
	if loader.calls != 2 {
		t.Fatalf("loader calls = %d, want initial and follow-up full rebuild", loader.calls)
	}
}

func TestManagerMetricsTracksProjectionRebuilds(t *testing.T) {
	ctx := context.Background()
	loader := &countingProjectionLoader{}
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(string, func(context.Context)) bool {
			return false
		},
	})

	if err := manager.RebuildAll(ctx); err != nil {
		t.Fatalf("rebuild all: %v", err)
	}
	if err := manager.RebuildWorkspace(ctx, "ws-a"); err != nil {
		t.Fatalf("rebuild workspace: %v", err)
	}
	metrics := manager.Metrics()
	if metrics.FullRebuildTotal != 1 || metrics.FullRebuildFailedTotal != 0 {
		t.Fatalf("full rebuild metrics = %+v, want one successful full rebuild", metrics)
	}
	if len(metrics.WorkspaceRebuilds) != 1 || metrics.WorkspaceRebuilds[0].WorkspaceID != "ws-a" || metrics.WorkspaceRebuilds[0].RebuildTotal != 1 || metrics.WorkspaceRebuilds[0].FailedTotal != 0 {
		t.Fatalf("workspace rebuild metrics = %+v, want one successful ws-a rebuild", metrics.WorkspaceRebuilds)
	}

	droppedBefore := metrics.ScheduleDroppedTotal
	manager.ScheduleRebuildWorkspace("ws-a")
	metrics = manager.Metrics()
	if metrics.ScheduleDroppedTotal != droppedBefore+1 {
		t.Fatalf("schedule dropped total = %d, want one additional failed scheduler counted from %d", metrics.ScheduleDroppedTotal, droppedBefore)
	}
	if metrics.WorkspaceRebuildDebouncedTotal != 0 || metrics.PendingWorkspaceRebuilds != 0 {
		t.Fatalf("debounce/pending metrics = %d/%d, want no pending after schedule rejection", metrics.WorkspaceRebuildDebouncedTotal, metrics.PendingWorkspaceRebuilds)
	}
}

func TestManagerMetricsTracksPendingWorkspaceRebuilds(t *testing.T) {
	ctx := context.Background()
	loader := &blockingProjectionLoader{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(string, func(context.Context)) bool {
			return true
		},
	})

	done := make(chan error, 1)
	go func() {
		done <- manager.RebuildAll(ctx)
	}()
	<-loader.started
	if err := manager.handleEvent(ctx, events.Envelope{
		Topic:       runevents.TopicRunStateChanged,
		WorkspaceID: "ws-a",
		SubjectType: "run",
		SubjectID:   "run-a",
		Revision:    9,
	}); err != nil {
		t.Fatalf("handle workspace event: %v", err)
	}
	if metrics := manager.Metrics(); metrics.PendingWorkspaceRebuilds != 1 {
		t.Fatalf("pending workspace rebuilds = %d, want 1", metrics.PendingWorkspaceRebuilds)
	}
	close(loader.release)
	if err := <-done; err != nil {
		t.Fatalf("full rebuild: %v", err)
	}
}

type scheduledProjectionTask struct {
	name string
	fn   func(context.Context)
}

type recordingAsyncProjectionHub struct {
	syncCalls  int
	asyncCalls int
	last       events.Envelope
}

func (h *recordingAsyncProjectionHub) Publish(_ context.Context, envelope events.Envelope) error {
	h.syncCalls++
	h.last = envelope
	return nil
}

func (h *recordingAsyncProjectionHub) PublishAsync(_ context.Context, envelope events.Envelope) error {
	h.asyncCalls++
	h.last = envelope
	return nil
}

func (h *recordingAsyncProjectionHub) Subscribe(string, events.Handler) func() {
	return func() {}
}

type fakeProjectionLoader struct {
	workspaces []appmodelcore.Workspace
	snapshots  map[string]readmodelcore.WorkspaceSnapshot
}

func (f fakeProjectionLoader) ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error) {
	return append([]appmodelcore.Workspace(nil), f.workspaces...), nil
}

func (f fakeProjectionLoader) ReadWorkspace(_ context.Context, workspaceID string) (appmodelcore.Workspace, error) {
	for _, item := range f.workspaces {
		if item.ID == workspaceID {
			return item, nil
		}
	}
	return appmodelcore.Workspace{}, errors.New("workspace not found")
}

func (f fakeProjectionLoader) ReadWorkspaceSnapshot(_ context.Context, workspaceID string) (readmodelcore.WorkspaceSnapshot, error) {
	if item, ok := f.snapshots[workspaceID]; ok {
		return item, nil
	}
	return readmodelcore.WorkspaceSnapshot{}, errors.New("snapshot not found")
}

type countingProjectionLoader struct {
	calls int
}

func (f *countingProjectionLoader) ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error) {
	return []appmodelcore.Workspace{{ID: "ws-a", Title: "workspace"}}, nil
}

func (f *countingProjectionLoader) ReadWorkspace(context.Context, string) (appmodelcore.Workspace, error) {
	return appmodelcore.Workspace{ID: "ws-a", Title: "workspace"}, nil
}

func (f *countingProjectionLoader) ReadWorkspaceSnapshot(context.Context, string) (readmodelcore.WorkspaceSnapshot, error) {
	f.calls++
	return readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-a", Title: fmt.Sprintf("snapshot-%d", f.calls)},
	}, nil
}

type workspaceCountingProjectionLoader struct {
	mu    sync.Mutex
	calls map[string]int
}

func (f *workspaceCountingProjectionLoader) ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error) {
	return []appmodelcore.Workspace{
		{ID: "ws-a", Title: "workspace-a"},
		{ID: "ws-b", Title: "workspace-b"},
	}, nil
}

func (f *workspaceCountingProjectionLoader) ReadWorkspace(_ context.Context, workspaceID string) (appmodelcore.Workspace, error) {
	return appmodelcore.Workspace{ID: workspaceID, Title: workspaceID + "-workspace"}, nil
}

func (f *workspaceCountingProjectionLoader) ReadWorkspaceSnapshot(_ context.Context, workspaceID string) (readmodelcore.WorkspaceSnapshot, error) {
	f.mu.Lock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[workspaceID]++
	call := f.calls[workspaceID]
	f.mu.Unlock()
	return readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: workspaceID, Title: fmt.Sprintf("%s-snapshot-%d", workspaceID, call)},
	}, nil
}

func (f *workspaceCountingProjectionLoader) callsFor(workspaceID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[workspaceID]
}

type blockingProjectionLoader struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (f *blockingProjectionLoader) ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error) {
	f.mu.Lock()
	f.calls++
	call := f.calls
	f.mu.Unlock()
	if call == 1 {
		close(f.started)
		<-f.release
	}
	return []appmodelcore.Workspace{{ID: "ws-a", Title: fmt.Sprintf("workspace-%d", call)}}, nil
}

func (f *blockingProjectionLoader) ReadWorkspace(context.Context, string) (appmodelcore.Workspace, error) {
	return appmodelcore.Workspace{ID: "ws-a", Title: "workspace"}, nil
}

func (f *blockingProjectionLoader) ReadWorkspaceSnapshot(context.Context, string) (readmodelcore.WorkspaceSnapshot, error) {
	f.mu.Lock()
	call := f.calls
	f.mu.Unlock()
	return readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-a", Title: fmt.Sprintf("snapshot-%d", call)},
	}, nil
}
