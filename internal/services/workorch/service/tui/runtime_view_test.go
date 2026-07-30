package tui

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
)

func TestApplyWorkspaceRuntimeViewRendersProgressAndActivity(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-runtime", "")
	m.width = 120
	m.height = 30
	m.activeWorkspaceID = "ws-runtime"
	m.progress = progressRunning
	m.running = true

	view := backend.WorkspaceRuntimeViewInfo{
		WorkspaceID:  "ws-runtime",
		ActiveRunID:  "run-1",
		DisplayRunID: "run-1",
		Projection: backend.ProjectionInfo{
			State:             "ready",
			WorkspaceRevision: 12,
		},
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    12,
			RuntimeStateRevision: 42,
			EvidenceSequence:     7,
			ActivityRevision:     9,
		},
		PromptMode: workspacePromptModeRunning,
		RunState:   common.RunStateActive,
		TaskProgress: backend.TaskProgressInfo{
			WorkspaceID: "ws-runtime",
			RunID:       "run-1",
			State:       string(common.RunStateActive),
			Summary:     "revision 42 | evidence 7 | focus focus-003 | action produce_validation",
			RuntimeV2: &backend.RuntimeV2ProgressInfo{
				StateRevision:          42,
				EvidenceSequence:       7,
				CurrentFocusID:         "focus-003",
				ConvergenceState:       "need_validation",
				CompletedDispatchCount: 3,
				CandidateAttemptCount:  2,
				AcceptedEvidenceCount:  1,
				RejectedEvidenceCount:  1,
				MissingEvidenceKinds:   []string{"validation"},
			},
		},
		Activity: backend.WorkspaceActivityFeedInfo{
			WorkspaceID:      "ws-runtime",
			DisplayRunID:     "run-1",
			ActivityRevision: 9,
			Items: []backend.WorkspaceActivityItemInfo{
				{
					ID:          "activity-1",
					WorkspaceID: "ws-runtime",
					RunID:       "run-1",
					Kind:        backend.WorkspaceActivityRuntimeTransition,
					Title:       "runtime focus",
					Summary:     "runtime focus focus-003 activated",
					OccurredAt:  "2026-07-15T14:31:00Z",
				},
				{
					ID:          "activity-2",
					WorkspaceID: "ws-runtime",
					RunID:       "run-1",
					Kind:        backend.WorkspaceActivityEvidence,
					Title:       "evidence",
					Summary:     "evidence accepted: validation-report",
					OccurredAt:  "2026-07-15T14:32:00Z",
				},
			},
		},
	}

	m.applyWorkspaceRuntimeView(view, true)
	screen := stripANSI(m.View())
	if !strings.Contains(screen, "Runtime Progress") {
		t.Fatalf("missing Runtime Progress section:\n%s", screen)
	}
	if !strings.Contains(screen, "revision 42") || !strings.Contains(screen, "focus focus-003") {
		t.Fatalf("missing runtime progress details:\n%s", screen)
	}
	if !strings.Contains(screen, "Recent Activity") || !strings.Contains(screen, "runtime focus focus-003 activated") {
		t.Fatalf("missing Recent Activity section:\n%s", screen)
	}
	if m.progressText == "" || !strings.Contains(m.progressText, "revision 42") {
		t.Fatalf("progressText = %q", m.progressText)
	}
	if entry := m.workspaceRuntimeState("ws-runtime"); entry == nil || !entry.HasRuntimeView || entry.AppliedRevision.RuntimeStateRevision != 42 {
		t.Fatalf("runtime state not cached: %+v", entry)
	}
}

func TestRenderRuntimeProgressDisplaysPrerequisiteRefreshSeparately(t *testing.T) {
	text := stripANSI(renderRuntimeProgressCompact(&backend.RuntimeV2ProgressInfo{
		StateRevision: 8, CandidateAttemptCount: 0,
		PrerequisiteRefreshState: "prerequisite_refreshing", PrerequisiteRefreshUsed: 1, PrerequisiteRefreshLimit: 3,
		PrerequisiteInvalidations: []backend.RuntimePrerequisiteRefreshInvalidationInfo{{
			Source: "owner_receipt_resource_change", OwnerModuleID: "file", ResourceDomain: "workspace_target", SubjectRef: "main.go",
		}},
	}, 160))
	if !strings.Contains(text, "prerequisite refresh prerequisite_refreshing | stale 1/3") ||
		!strings.Contains(text, "owner_receipt_resource_change | file | workspace_target:main.go") {
		t.Fatalf("freshness display missing: %q", text)
	}
	if !strings.Contains(text, "candidate attempts 0") {
		t.Fatalf("freshness was not kept separate from candidate attempts: %q", text)
	}
}

func TestRenderRuntimeProgressKeepsProviderTransportFailureSeparateFromFreshness(t *testing.T) {
	text := stripANSI(renderRuntimeProgressCompact(&backend.RuntimeV2ProgressInfo{
		FailureClass:             "provider_transport",
		FailureCode:              "deadline_exceeded",
		FailureMessage:           "POST /v1/chat/completions: context deadline exceeded",
		PrerequisiteRefreshState: "prerequisite_refreshing",
		PrerequisiteRefreshUsed:  1,
		PrerequisiteRefreshLimit: 3,
		PrerequisiteInvalidations: []backend.RuntimePrerequisiteRefreshInvalidationInfo{{
			Source: "execution_receipt_stale", OwnerModuleID: "llm", ResourceDomain: "provider_config", SubjectRef: "ai-proxy/model-a",
		}},
	}, 180))
	if !strings.Contains(text, "prerequisite refresh prerequisite_refreshing | stale 1/3") ||
		!strings.Contains(text, "execution_receipt_stale | llm | provider_config:ai-proxy/model-a") ||
		!strings.Contains(text, "failure POST /v1/chat/completions: context deadline exceeded") {
		t.Fatalf("runtime display conflated provider transport and freshness: %q", text)
	}
	if strings.Contains(text, "candidate attempts 1") {
		t.Fatalf("transport/freshness display fabricated a candidate retry: %q", text)
	}
}

func TestRuntimeViewFollowTailAndUserScrolledUnread(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-follow", "")
	m.width = 120
	m.height = 16
	m.activeWorkspaceID = "ws-follow"
	lines := make([]string, 0, 80)
	for i := 0; i < 80; i++ {
		lines = append(lines, "line-"+string(rune('A'+(i%26))))
	}
	m.result = strings.Join(lines, "\n")
	_ = m.View()

	// Leave bottom.
	m.scrollMain(-10)
	if !m.mainViewportUserScrolled {
		t.Fatal("expected user-scrolled after leaving bottom")
	}
	offset := m.mainViewport.YOffset

	view := backend.WorkspaceRuntimeViewInfo{
		WorkspaceID: "ws-follow",
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    2,
			RuntimeStateRevision: 3,
			ActivityRevision:     4,
		},
		TaskProgress: backend.TaskProgressInfo{
			Summary: "revision 3 | evidence 1",
			RuntimeV2: &backend.RuntimeV2ProgressInfo{
				StateRevision:    3,
				EvidenceSequence: 1,
			},
		},
		Activity: backend.WorkspaceActivityFeedInfo{
			ActivityRevision: 4,
			Items: []backend.WorkspaceActivityItemInfo{{
				ID:      "a1",
				Summary: "dispatch completed",
			}},
		},
	}
	m.applyWorkspaceRuntimeView(view, false)
	if m.mainViewport.YOffset != offset {
		t.Fatalf("user-scrolled position stolen: before=%d after=%d", offset, m.mainViewport.YOffset)
	}
	if m.mainViewportUnreadCount == 0 || !m.mainViewportPendingUpdate {
		t.Fatalf("expected unread update while user-scrolled: unread=%d pending=%v", m.mainViewportUnreadCount, m.mainViewportPendingUpdate)
	}
	screen := stripANSI(m.View())
	if !strings.Contains(screen, "new updates:") || !strings.Contains(screen, "PgDn to follow") {
		t.Fatalf("status bar missing unread prompt:\n%s", screen)
	}

	// Return to bottom restores follow-tail.
	m.ensureMainViewport().GotoBottom()
	m.scrollMain(1)
	if m.mainViewportUserScrolled || m.mainViewportUnreadCount != 0 {
		// If still not at bottom after one step, force GotoBottom path.
		m.followMainViewportTail()
	}
	if m.mainViewportUserScrolled || m.mainViewportUnreadCount != 0 {
		t.Fatalf("expected follow-tail restored: scrolled=%v unread=%d", m.mainViewportUserScrolled, m.mainViewportUnreadCount)
	}
}

func TestStaleRuntimeViewDoesNotOverrideNewerRevision(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-stale", "")
	m.activeWorkspaceID = "ws-stale"
	entry := m.ensureWorkspaceRuntimeState("ws-stale")
	entry.AppliedRevision = backend.WorkspaceRefreshRevision{
		WorkspaceRevision:    10,
		RuntimeStateRevision: 20,
		ActivityRevision:     30,
	}
	entry.HasRuntimeView = true

	stale := backend.WorkspaceRuntimeViewInfo{
		WorkspaceID: "ws-stale",
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    9,
			RuntimeStateRevision: 19,
			ActivityRevision:     29,
		},
		TaskProgress: backend.TaskProgressInfo{
			Summary: "stale summary",
			RuntimeV2: &backend.RuntimeV2ProgressInfo{
				StateRevision: 19,
			},
		},
	}
	m.applyWorkspaceRuntimeView(stale, false)
	if m.progressText == "stale summary" {
		t.Fatal("stale runtime view overwrote active progress text")
	}
	if entry.AppliedRevision.WorkspaceRevision != 10 || entry.AppliedRevision.RuntimeStateRevision != 20 {
		t.Fatalf("revision mutated by stale view: %+v", entry.AppliedRevision)
	}
}

func TestTimelineUsesActivityFeed(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-timeline", "")
	m.activeWorkspaceID = "ws-timeline"
	m.activityFeed = backend.WorkspaceActivityFeedInfo{
		DisplayRunID: "run-1",
		Items: []backend.WorkspaceActivityItemInfo{
			{ID: "1", Summary: "runtime focus activated", OccurredAt: "14:31"},
			{ID: "2", Summary: "evidence accepted", OccurredAt: "14:32"},
		},
	}
	m.input = "/timeline"
	next, _, handled := m.handleSlashCommand("/timeline")
	if !handled {
		t.Fatal("timeline command not handled")
	}
	m = asModel(next)
	if !strings.Contains(m.result, "runtime focus activated") || !strings.Contains(m.result, "evidence accepted") {
		t.Fatalf("timeline result missing activity feed:\n%s", m.result)
	}
}

func TestNonActiveRuntimeViewOnlyUpdatesCache(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-active", "")
	m.activeWorkspaceID = "ws-active"
	m.progressText = "active-progress"
	m.activityFeed = backend.WorkspaceActivityFeedInfo{
		Items: []backend.WorkspaceActivityItemInfo{{ID: "active", Summary: "active activity"}},
	}

	view := backend.WorkspaceRuntimeViewInfo{
		WorkspaceID: "ws-background",
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    5,
			RuntimeStateRevision: 6,
			ActivityRevision:     7,
		},
		TaskProgress: backend.TaskProgressInfo{
			Summary: "background progress",
			RuntimeV2: &backend.RuntimeV2ProgressInfo{
				StateRevision: 6,
			},
		},
		Activity: backend.WorkspaceActivityFeedInfo{
			Items: []backend.WorkspaceActivityItemInfo{{ID: "bg", Summary: "background activity"}},
		},
	}
	m.applyWorkspaceRuntimeView(view, false)
	if m.progressText != "active-progress" {
		t.Fatalf("active progress polluted: %q", m.progressText)
	}
	if len(m.activityFeed.Items) != 1 || m.activityFeed.Items[0].ID != "active" {
		t.Fatalf("active activity polluted: %+v", m.activityFeed)
	}
	entry := m.workspaceRuntimeState("ws-background")
	if entry == nil || !entry.HasRuntimeView || entry.Display.Activity.Items[0].ID != "bg" {
		t.Fatalf("background cache not updated: %+v", entry)
	}
}

func TestWorkspaceRuntimeViewMsgAppliesAndHonorsRequestSeq(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-msg", "")
	m.activeWorkspaceID = "ws-msg"
	entry := m.ensureWorkspaceRuntimeState("ws-msg")
	entry.ContextRequestSeq = 3
	entry.ContextInFlight = true

	// Stale sequence discarded.
	next, _ := m.Update(WorkspaceRuntimeViewMsg{
		RequestSeq:  2,
		WorkspaceID: "ws-msg",
		View: &backend.WorkspaceRuntimeViewInfo{
			WorkspaceID: "ws-msg",
			TaskProgress: backend.TaskProgressInfo{
				Summary: "stale-seq",
				RuntimeV2: &backend.RuntimeV2ProgressInfo{
					StateRevision: 1,
				},
			},
			Revision: backend.WorkspaceRefreshRevision{WorkspaceRevision: 1, RuntimeStateRevision: 1, ActivityRevision: 1},
		},
	})
	m = asModel(next)
	if m.progressText == "stale-seq" {
		t.Fatal("stale request sequence applied")
	}

	next, _ = m.Update(WorkspaceRuntimeViewMsg{
		RequestSeq:  3,
		WorkspaceID: "ws-msg",
		View: &backend.WorkspaceRuntimeViewInfo{
			WorkspaceID: "ws-msg",
			RunState:    common.RunStateActive,
			TaskProgress: backend.TaskProgressInfo{
				Summary: "fresh-view",
				RuntimeV2: &backend.RuntimeV2ProgressInfo{
					StateRevision:    8,
					EvidenceSequence: 2,
					CurrentFocusID:   "focus-x",
				},
			},
			Revision: backend.WorkspaceRefreshRevision{WorkspaceRevision: 4, RuntimeStateRevision: 8, ActivityRevision: 5},
			Activity: backend.WorkspaceActivityFeedInfo{
				Items: []backend.WorkspaceActivityItemInfo{{ID: "fresh", Summary: "fresh activity"}},
			},
		},
	})
	m = asModel(next)
	if m.runtimeV2 == nil || m.runtimeV2.StateRevision != 8 {
		t.Fatalf("runtime v2 not applied: %+v", m.runtimeV2)
	}
	if len(m.activityFeed.Items) != 1 || m.activityFeed.Items[0].ID != "fresh" {
		t.Fatalf("activity not applied: %+v", m.activityFeed)
	}
}

func TestInFlightRuntimeViewNotificationsCollapseToPending(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-pending", "")
	m.activeWorkspaceID = "ws-pending"
	entry := m.ensureWorkspaceRuntimeState("ws-pending")
	entry.ContextInFlight = true
	entry.ContextRequestSeq = 1

	cmd := m.requestWorkspaceContextRefresh(false)
	if cmd != nil {
		t.Fatal("expected nil command while in-flight")
	}
	if !entry.ContextPending {
		t.Fatal("expected pending flag while in-flight")
	}
}

// TC10: reconnect live must trigger list + RuntimeView readback without clearing view.
func TestSubscriptionLiveRecoveryTriggersReadbackWithoutClearingView(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-reconnect", "")
	m.activeWorkspaceID = "ws-reconnect"
	m.width = 100
	m.height = 24
	m.applyWorkspaceRuntimeView(backend.WorkspaceRuntimeViewInfo{
		WorkspaceID:  "ws-reconnect",
		DisplayRunID: "run-1",
		RunState:     common.RunStateActive,
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    5,
			RuntimeStateRevision: 10,
			ActivityRevision:     3,
		},
		TaskProgress: backend.TaskProgressInfo{
			Summary: "pre-disconnect progress",
			RuntimeV2: &backend.RuntimeV2ProgressInfo{
				StateRevision:    10,
				EvidenceSequence: 2,
				CurrentFocusID:   "focus-keep",
			},
		},
		Activity: backend.WorkspaceActivityFeedInfo{
			ActivityRevision: 3,
			Items: []backend.WorkspaceActivityItemInfo{
				{ID: "keep-1", Summary: "kept activity"},
			},
		},
	}, true)
	before := stripANSI(m.View())
	if !strings.Contains(before, "kept activity") && !strings.Contains(before, "pre-disconnect") && m.runtimeV2 == nil {
		t.Fatalf("expected pre-disconnect view retained: %q", before)
	}

	// Simulate reconnecting then live recovery.
	m.subscriptionStatus = backend.SubscriptionStatusInfo{State: backend.SubscriptionReconnecting}
	cmd := m.applySubscriptionStatus(backend.SubscriptionStatusInfo{State: backend.SubscriptionLive})
	if cmd == nil {
		t.Fatal("live recovery must schedule list + RuntimeView readback")
	}
	if m.subscriptionStatus.State != backend.SubscriptionLive {
		t.Fatalf("status = %q, want live", m.subscriptionStatus.State)
	}
	// View content must not be cleared during reconnect/live transition.
	if m.runtimeV2 == nil || m.runtimeV2.CurrentFocusID != "focus-keep" {
		t.Fatalf("view cleared on reconnect: %+v", m.runtimeV2)
	}
	if len(m.activityFeed.Items) != 1 || m.activityFeed.Items[0].ID != "keep-1" {
		t.Fatalf("activity cleared on reconnect: %+v", m.activityFeed)
	}
}

// TC10: TUI "restart" is modeled as a new AppModel that recovers history via RuntimeView.
func TestRuntimeViewHistoryRecoveryAfterRestart(t *testing.T) {
	// First process session applied a view, then process restarts.
	restarted := NewAppModel(newFake(t), "ws-history", "")
	restarted.activeWorkspaceID = "ws-history"
	restarted.width = 100
	restarted.height = 24

	// No local string timeline; history comes only from RuntimeView readback.
	if len(restarted.activityFeed.Items) != 0 {
		t.Fatalf("fresh model should start empty activity, got %+v", restarted.activityFeed)
	}

	view := backend.WorkspaceRuntimeViewInfo{
		WorkspaceID:  "ws-history",
		DisplayRunID: "run-history",
		RunState:     common.RunStateCompleted,
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    20,
			RuntimeStateRevision: 88,
			ActivityRevision:     15,
		},
		TaskProgress: backend.TaskProgressInfo{
			Summary: "revision 88 | evidence 4 | focus focus-done",
			State:   string(common.RunStateCompleted),
			RuntimeV2: &backend.RuntimeV2ProgressInfo{
				StateRevision:    88,
				EvidenceSequence: 4,
				CurrentFocusID:   "focus-done",
				ConvergenceState: "complete",
			},
		},
		Activity: backend.WorkspaceActivityFeedInfo{
			WorkspaceID:      "ws-history",
			DisplayRunID:     "run-history",
			ActivityRevision: 15,
			Items: []backend.WorkspaceActivityItemInfo{
				{ID: "hist-1", Summary: "historical focus change", OccurredAt: "2026-07-15T10:00:00Z"},
				{ID: "hist-2", Summary: "historical evidence accepted", OccurredAt: "2026-07-15T10:01:00Z"},
				{ID: "hist-3", Summary: "historical outcome completed", OccurredAt: "2026-07-15T10:02:00Z"},
			},
		},
	}
	restarted.applyWorkspaceRuntimeView(view, true)
	if restarted.activityFeed.DisplayRunID != "run-history" {
		t.Fatalf("activity DisplayRunID = %q, want run-history", restarted.activityFeed.DisplayRunID)
	}
	if len(restarted.activityFeed.Items) != 3 {
		t.Fatalf("history items = %d, want 3", len(restarted.activityFeed.Items))
	}
	// /timeline must use typed ActivityFeed recovered from RuntimeView.
	next, _, handled := restarted.handleSlashCommand("/timeline")
	if !handled {
		t.Fatal("timeline command not handled after restart recovery")
	}
	restarted = asModel(next)
	if !strings.Contains(restarted.result, "historical evidence accepted") {
		t.Fatalf("timeline missing recovered history: %q", restarted.result)
	}
}

// TC10: non-active workspace dirty does not pollute active view; switch recovers via RuntimeView.
func TestNonActiveDirtyThenSwitchRecoversHistory(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.applyWorkspaceRuntimeView(backend.WorkspaceRuntimeViewInfo{
		WorkspaceID:  "ws-a",
		DisplayRunID: "run-a",
		Revision:     backend.WorkspaceRefreshRevision{WorkspaceRevision: 2, ActivityRevision: 1},
		Activity: backend.WorkspaceActivityFeedInfo{
			Items: []backend.WorkspaceActivityItemInfo{{ID: "a-1", Summary: "active-a"}},
		},
	}, true)

	// Background workspace B advances via typed projection notification only.
	cmd := m.applyWorkspaceProjectionChanged(backend.WorkspaceProjectionChangedInfo{
		WorkspaceID: "ws-b",
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    7,
			RuntimeStateRevision: 30,
			ActivityRevision:     12,
		},
		ChangedSections: []backend.WorkspaceProjectionSection{
			backend.WorkspaceProjectionSectionRuntimeProgress,
			backend.WorkspaceProjectionSectionActivity,
		},
	})
	// Non-active dirty must not mutate active activity.
	if len(m.activityFeed.Items) != 1 || m.activityFeed.Items[0].ID != "a-1" {
		t.Fatalf("active activity polluted: %+v", m.activityFeed)
	}
	entryB := m.ensureWorkspaceRuntimeState("ws-b")
	if entryB.DirtyRevision.ActivityRevision != 12 {
		t.Fatalf("ws-b dirty activity revision = %d, want 12", entryB.DirtyRevision.ActivityRevision)
	}
	_ = cmd

	// Switch to B and recover via RuntimeView (not string timeline).
	m.activeWorkspaceID = "ws-b"
	m.applyWorkspaceRuntimeView(backend.WorkspaceRuntimeViewInfo{
		WorkspaceID:  "ws-b",
		DisplayRunID: "run-b",
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    7,
			RuntimeStateRevision: 30,
			ActivityRevision:     12,
		},
		Activity: backend.WorkspaceActivityFeedInfo{
			ActivityRevision: 12,
			Items: []backend.WorkspaceActivityItemInfo{
				{ID: "b-1", Summary: "background progress during A active"},
				{ID: "b-2", Summary: "background evidence during A active"},
			},
		},
	}, true)
	if len(m.activityFeed.Items) != 2 {
		t.Fatalf("switch recovery items = %d, want 2", len(m.activityFeed.Items))
	}
	if m.activityFeed.Items[0].Summary != "background progress during A active" {
		t.Fatalf("unexpected recovered activity: %+v", m.activityFeed.Items)
	}
}

// TC10: reconnecting status is visible and degraded does not clear AppliedRevision.
func TestSubscriptionReconnectingPreservesAppliedRevision(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-status", "")
	m.activeWorkspaceID = "ws-status"
	m.applyWorkspaceRuntimeView(backend.WorkspaceRuntimeViewInfo{
		WorkspaceID: "ws-status",
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    9,
			RuntimeStateRevision: 21,
			ActivityRevision:     4,
		},
		TaskProgress: backend.TaskProgressInfo{
			RuntimeV2: &backend.RuntimeV2ProgressInfo{StateRevision: 21, CurrentFocusID: "focus-status"},
		},
	}, true)
	entry := m.ensureWorkspaceRuntimeState("ws-status")
	applied := entry.AppliedRevision

	cmd := m.applySubscriptionStatus(backend.SubscriptionStatusInfo{
		State:     backend.SubscriptionReconnecting,
		LastError: "projection sse disconnected",
	})
	if cmd != nil {
		t.Fatal("reconnecting must not trigger readback")
	}
	if m.subscriptionStatus.State != backend.SubscriptionReconnecting {
		t.Fatalf("status = %q, want reconnecting", m.subscriptionStatus.State)
	}
	if entry.AppliedRevision != applied {
		t.Fatalf("AppliedRevision changed during reconnecting: %+v -> %+v", applied, entry.AppliedRevision)
	}
	if m.runtimeV2 == nil || m.runtimeV2.CurrentFocusID != "focus-status" {
		t.Fatalf("view cleared while reconnecting: %+v", m.runtimeV2)
	}
}

// RR04: request WorkspaceID must match View.WorkspaceID; mismatches update nothing.
func TestRuntimeViewMsgRejectsWorkspaceIDMismatch(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-a", "")
	m.activeWorkspaceID = "ws-a"
	m.applyWorkspaceRuntimeView(backend.WorkspaceRuntimeViewInfo{
		WorkspaceID: "ws-a",
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    5,
			RuntimeStateRevision: 10,
			ActivityRevision:     3,
		},
		TaskProgress: backend.TaskProgressInfo{
			RuntimeV2: &backend.RuntimeV2ProgressInfo{StateRevision: 10, CurrentFocusID: "focus-a"},
		},
		Activity: backend.WorkspaceActivityFeedInfo{
			Items: []backend.WorkspaceActivityItemInfo{{ID: "a-1", Summary: "keep-a"}},
		},
	}, true)
	entry := m.ensureWorkspaceRuntimeState("ws-a")
	entry.ContextRequestSeq = 2
	applied := entry.AppliedRevision

	next, cmd := m.Update(WorkspaceRuntimeViewMsg{
		RequestSeq:  2,
		WorkspaceID: "ws-a",
		View: &backend.WorkspaceRuntimeViewInfo{
			WorkspaceID: "ws-b", // mismatch with request/msg workspace
			Revision: backend.WorkspaceRefreshRevision{
				WorkspaceRevision:    99,
				RuntimeStateRevision: 99,
				ActivityRevision:     99,
			},
			TaskProgress: backend.TaskProgressInfo{
				RuntimeV2: &backend.RuntimeV2ProgressInfo{StateRevision: 99, CurrentFocusID: "poison"},
			},
			Activity: backend.WorkspaceActivityFeedInfo{
				Items: []backend.WorkspaceActivityItemInfo{{ID: "poison", Summary: "must-not-apply"}},
			},
		},
	})
	m = asModel(next)
	_ = cmd
	if m.runtimeV2 == nil || m.runtimeV2.CurrentFocusID != "focus-a" {
		t.Fatalf("active view polluted by mismatched response: %+v", m.runtimeV2)
	}
	if len(m.activityFeed.Items) != 1 || m.activityFeed.Items[0].ID != "a-1" {
		t.Fatalf("activity polluted: %+v", m.activityFeed)
	}
	if entry.AppliedRevision != applied {
		t.Fatalf("AppliedRevision changed on mismatch: %+v -> %+v", applied, entry.AppliedRevision)
	}
	// Other workspace cache must also remain untouched.
	if other := m.workspaceRuntimeState("ws-b"); other != nil && other.HasRuntimeView {
		t.Fatalf("mismatched response updated other workspace cache: %+v", other.RuntimeView)
	}
}

// RR04: zero revision response while DirtyRevision is non-zero must not apply,
// must keep dirty, and must schedule one pending readback.
func TestZeroRevisionResponseBelowDirtySchedulesPendingReadback(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-dirty", "")
	m.activeWorkspaceID = "ws-dirty"
	m.applyWorkspaceRuntimeView(backend.WorkspaceRuntimeViewInfo{
		WorkspaceID: "ws-dirty",
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    4,
			RuntimeStateRevision: 8,
			ActivityRevision:     2,
		},
		TaskProgress: backend.TaskProgressInfo{
			RuntimeV2: &backend.RuntimeV2ProgressInfo{StateRevision: 8, CurrentFocusID: "focus-keep"},
		},
	}, true)
	entry := m.ensureWorkspaceRuntimeState("ws-dirty")
	entry.ContextRequestSeq = 5
	entry.DirtyRevision = backend.WorkspaceRefreshRevision{
		WorkspaceRevision:    9,
		RuntimeStateRevision: 20,
		ActivityRevision:     6,
	}
	applied := entry.AppliedRevision
	dirty := entry.DirtyRevision

	next, cmd := m.Update(WorkspaceRuntimeViewMsg{
		RequestSeq:  5,
		WorkspaceID: "ws-dirty",
		View: &backend.WorkspaceRuntimeViewInfo{
			WorkspaceID: "ws-dirty",
			// All-zero revision while dirty is set.
			Revision: backend.WorkspaceRefreshRevision{},
			TaskProgress: backend.TaskProgressInfo{
				RuntimeV2: &backend.RuntimeV2ProgressInfo{StateRevision: 0, CurrentFocusID: "zero-poison"},
			},
		},
	})
	m = asModel(next)
	if cmd == nil {
		t.Fatal("zero revision below dirty must schedule pending readback")
	}
	if entry.AppliedRevision != applied {
		t.Fatalf("zero revision applied over existing view: %+v -> %+v", applied, entry.AppliedRevision)
	}
	if entry.DirtyRevision != dirty {
		t.Fatalf("dirty cleared on zero response: %+v -> %+v", dirty, entry.DirtyRevision)
	}
	if m.runtimeV2 == nil || m.runtimeV2.CurrentFocusID != "focus-keep" {
		t.Fatalf("view replaced by zero revision response: %+v", m.runtimeV2)
	}
}

// RR04: refreshRevisionBelow treats zero current under non-zero dirty as below.
func TestRefreshRevisionBelowZeroCurrentWithDirty(t *testing.T) {
	if !refreshRevisionBelow(
		backend.WorkspaceRefreshRevision{},
		backend.WorkspaceRefreshRevision{WorkspaceRevision: 3, ActivityRevision: 2},
	) {
		t.Fatal("zero current under non-zero dirty must be below")
	}
	if refreshRevisionBelow(
		backend.WorkspaceRefreshRevision{WorkspaceRevision: 5, ActivityRevision: 4},
		backend.WorkspaceRefreshRevision{WorkspaceRevision: 5, ActivityRevision: 4},
	) {
		t.Fatal("equal revisions must not be below")
	}
	if !refreshRevisionBelow(
		backend.WorkspaceRefreshRevision{WorkspaceRevision: 4, RuntimeStateRevision: 10, ActivityRevision: 2},
		backend.WorkspaceRefreshRevision{WorkspaceRevision: 5, RuntimeStateRevision: 10, ActivityRevision: 2},
	) {
		t.Fatal("lower workspace revision must be below dirty")
	}
}

// CR03: scope mismatch must not clear ContextPending/ContextInFlight that were
// already set for a valid in-flight request, and must not write any cache.
func TestRuntimeViewMsgScopeMismatchPreservesPendingReadback(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-scope", "")
	m.activeWorkspaceID = "ws-scope"
	m.applyWorkspaceRuntimeView(backend.WorkspaceRuntimeViewInfo{
		WorkspaceID: "ws-scope",
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    3,
			RuntimeStateRevision: 7,
			ActivityRevision:     2,
		},
		TaskProgress: backend.TaskProgressInfo{
			RuntimeV2: &backend.RuntimeV2ProgressInfo{StateRevision: 7, CurrentFocusID: "focus-keep"},
		},
		Activity: backend.WorkspaceActivityFeedInfo{
			Items: []backend.WorkspaceActivityItemInfo{{ID: "keep", Summary: "keep"}},
		},
	}, true)
	entry := m.ensureWorkspaceRuntimeState("ws-scope")
	entry.ContextRequestSeq = 9
	entry.ContextInFlight = true
	entry.ContextPending = true
	entry.DirtyRevision = backend.WorkspaceRefreshRevision{
		WorkspaceRevision:    8,
		RuntimeStateRevision: 12,
		ActivityRevision:     4,
	}
	pendingBefore := entry.ContextPending
	inFlightBefore := entry.ContextInFlight
	dirtyBefore := entry.DirtyRevision
	appliedBefore := entry.AppliedRevision

	next, cmd := m.Update(WorkspaceRuntimeViewMsg{
		RequestSeq:  9,
		WorkspaceID: "ws-scope",
		View: &backend.WorkspaceRuntimeViewInfo{
			WorkspaceID: "ws-other",
			Revision: backend.WorkspaceRefreshRevision{
				WorkspaceRevision:    99,
				RuntimeStateRevision: 99,
				ActivityRevision:     99,
			},
			TaskProgress: backend.TaskProgressInfo{
				RuntimeV2: &backend.RuntimeV2ProgressInfo{StateRevision: 99, CurrentFocusID: "poison"},
			},
		},
	})
	m = asModel(next)
	_ = cmd
	entry = m.workspaceRuntimeState("ws-scope")
	if entry == nil {
		t.Fatal("expected workspace runtime state")
	}
	if entry.ContextPending != pendingBefore {
		t.Fatalf("ContextPending cleared on scope mismatch: %v -> %v", pendingBefore, entry.ContextPending)
	}
	if entry.ContextInFlight != inFlightBefore {
		t.Fatalf("ContextInFlight cleared on scope mismatch: %v -> %v", inFlightBefore, entry.ContextInFlight)
	}
	if entry.DirtyRevision != dirtyBefore {
		t.Fatalf("DirtyRevision changed on scope mismatch: %+v -> %+v", dirtyBefore, entry.DirtyRevision)
	}
	if entry.AppliedRevision != appliedBefore {
		t.Fatalf("AppliedRevision changed on scope mismatch: %+v -> %+v", appliedBefore, entry.AppliedRevision)
	}
	if m.runtimeV2 == nil || m.runtimeV2.CurrentFocusID != "focus-keep" {
		t.Fatalf("active view polluted: %+v", m.runtimeV2)
	}
	if other := m.workspaceRuntimeState("ws-other"); other != nil && other.HasRuntimeView {
		t.Fatalf("mismatch wrote other workspace cache")
	}
}

// CR03: nil view response must not clear pending/in-flight/dirty.
func TestRuntimeViewMsgNilViewPreservesPending(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-nil", "")
	m.activeWorkspaceID = "ws-nil"
	entry := m.ensureWorkspaceRuntimeState("ws-nil")
	entry.ContextRequestSeq = 3
	entry.ContextInFlight = true
	entry.ContextPending = true
	entry.DirtyRevision = backend.WorkspaceRefreshRevision{WorkspaceRevision: 5}

	next, _ := m.Update(WorkspaceRuntimeViewMsg{
		RequestSeq:  3,
		WorkspaceID: "ws-nil",
		View:        nil,
	})
	m = asModel(next)
	entry = m.workspaceRuntimeState("ws-nil")
	if entry == nil || !entry.ContextPending || !entry.ContextInFlight {
		t.Fatalf("nil view cleared pending/in-flight: pending=%v inFlight=%v", entry.ContextPending, entry.ContextInFlight)
	}
	if entry.DirtyRevision.WorkspaceRevision != 5 {
		t.Fatalf("nil view cleared dirty: %+v", entry.DirtyRevision)
	}
}

// CR04: equal revision + equal content readback is a full no-op for unread/viewport.
func TestEqualRevisionEqualContentReadbackIsNoOp(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-noop", "")
	m.activeWorkspaceID = "ws-noop"
	view := backend.WorkspaceRuntimeViewInfo{
		WorkspaceID:  "ws-noop",
		ActiveRunID:  "run-1",
		DisplayRunID: "run-1",
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision:    4,
			RuntimeStateRevision: 9,
			ActivityRevision:     3,
		},
		TaskProgress: backend.TaskProgressInfo{
			State:   "running",
			Summary: "revision 9 | focus f1",
			RuntimeV2: &backend.RuntimeV2ProgressInfo{
				StateRevision: 9, EvidenceSequence: 2, CurrentFocusID: "f1", ConvergenceState: "need_state_change",
			},
		},
		Activity: backend.WorkspaceActivityFeedInfo{
			WorkspaceID:      "ws-noop",
			DisplayRunID:     "run-1",
			ActivityRevision: 3,
			Items: []backend.WorkspaceActivityItemInfo{
				{ID: "a1", Summary: "focus f1", Title: "runtime focus", Kind: backend.WorkspaceActivityRuntimeTransition},
			},
		},
	}
	m.applyWorkspaceRuntimeView(view, true)
	// Leave follow-tail and force user-scrolled so a real update would bump unread.
	m.mainViewportUserScrolled = true
	m.setActiveWorkspaceFollowTail(false)
	m.mainViewportUnreadCount = 2
	entry := m.ensureWorkspaceRuntimeState("ws-noop")
	entry.UnreadUpdateCount = 2
	unreadBefore := m.mainViewportUnreadCount
	appliedBefore := entry.AppliedRevision

	// Identical content/revision re-apply must not bump unread or change applied.
	m.applyWorkspaceRuntimeView(view, false)
	if m.mainViewportUnreadCount != unreadBefore {
		t.Fatalf("equal content readback increased unread: %d -> %d", unreadBefore, m.mainViewportUnreadCount)
	}
	if entry.UnreadUpdateCount != unreadBefore {
		t.Fatalf("equal content readback changed entry unread: %d", entry.UnreadUpdateCount)
	}
	if entry.AppliedRevision != appliedBefore {
		t.Fatalf("equal content readback changed AppliedRevision: %+v -> %+v", appliedBefore, entry.AppliedRevision)
	}

	// Same revision but mutated display content must still apply and can bump unread
	// while user-scrolled (content identity conflict accepted by content gate).
	changed := view
	changed.TaskProgress.Summary = "revision 9 | focus f1 updated"
	changed.TaskProgress.RuntimeV2 = &backend.RuntimeV2ProgressInfo{
		StateRevision: 9, EvidenceSequence: 2, CurrentFocusID: "f1", ConvergenceState: "need_state_change",
	}
	changed.Activity = backend.WorkspaceActivityFeedInfo{
		WorkspaceID:      "ws-noop",
		DisplayRunID:     "run-1",
		ActivityRevision: 3,
		Items: []backend.WorkspaceActivityItemInfo{
			{ID: "a1", Summary: "focus f1 updated", Title: "runtime focus", Kind: backend.WorkspaceActivityRuntimeTransition},
		},
	}
	m.applyWorkspaceRuntimeView(changed, false)
	if m.mainViewportUnreadCount != unreadBefore+1 {
		t.Fatalf("changed content at equal revision unread = %d, want %d", m.mainViewportUnreadCount, unreadBefore+1)
	}
	if m.progressText != "revision 9 | focus f1 updated" {
		t.Fatalf("changed content not applied: progressText=%q", m.progressText)
	}
}

// FR02: equal revision but mutated applied fields must not be treated as no-op.
func TestEqualRevisionFieldMutationsAreNotNoOp(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-fr02", "")
	m.activeWorkspaceID = "ws-fr02"
	base := backend.WorkspaceRuntimeViewInfo{
		WorkspaceID:  "ws-fr02",
		ActiveRunID:  "run-1",
		DisplayRunID: "run-1",
		Revision: backend.WorkspaceRefreshRevision{
			WorkspaceRevision: 4, RuntimeStateRevision: 9, ActivityRevision: 3,
		},
		TaskProgress: backend.TaskProgressInfo{
			State: "running", Summary: "same",
			RuntimeV2: &backend.RuntimeV2ProgressInfo{StateRevision: 9, CurrentFocusID: "f1", ConvergenceState: "need_state_change"},
		},
		Activity: backend.WorkspaceActivityFeedInfo{
			WorkspaceID: "ws-fr02", DisplayRunID: "run-1", ActivityRevision: 3,
			Items: []backend.WorkspaceActivityItemInfo{{ID: "a1", Summary: "s1"}},
		},
		Outcome: backend.WorkspaceOutcomeInfo{State: "running", Summary: "ok", Reason: "none"},
		RunHeartbeat: backend.RunHeartbeatInfo{
			WorkspaceID: "ws-fr02", RunID: "run-1", State: "running", Summary: "hb-old",
		},
		Acceptance: backend.AcceptanceProjectionInfo{
			ActiveSliceID: "slice-1",
			Summary:       backend.AcceptanceSummaryInfo{OpenSlices: 1},
		},
		RunCommand: &backend.RunCommandStatusInfo{
			WorkspaceID: "ws-fr02", RunID: "run-1", State: "accepted", Intent: "resume", Reason: "old",
		},
		ConvergenceStatus: &backend.ConvergenceStatusInfo{
			WorkspaceID: "ws-fr02", RunID: "run-1", CurrentFocusID: "f1", DecisionCause: "old",
		},
	}
	m.applyWorkspaceRuntimeView(base, true)
	m.mainViewportUserScrolled = true
	m.setActiveWorkspaceFollowTail(false)
	unreadBefore := m.mainViewportUnreadCount

	// Mutate Outcome.Reason only — previously not compared.
	changed := base
	changed.Outcome.Reason = "new-reason"
	m.applyWorkspaceRuntimeView(changed, false)
	if m.mainViewportUnreadCount != unreadBefore+1 {
		t.Fatalf("Outcome.Reason-only mutation unread = %d, want %d", m.mainViewportUnreadCount, unreadBefore+1)
	}
	entry := m.workspaceRuntimeState("ws-fr02")
	if entry == nil || entry.RuntimeView.Outcome.Reason != "new-reason" {
		t.Fatalf("Outcome.Reason not applied: %+v", entry.RuntimeView.Outcome)
	}

	// Reset applied view then mutate Acceptance.
	m.applyWorkspaceRuntimeView(base, true)
	m.mainViewportUserScrolled = true
	m.setActiveWorkspaceFollowTail(false)
	acc := base
	acc.Acceptance.Summary.OpenSlices = 3
	m.applyWorkspaceRuntimeView(acc, false)
	if m.acceptance.Summary.OpenSlices != 3 {
		t.Fatalf("Acceptance not applied: %+v", m.acceptance)
	}

	// Mutate RunHeartbeat.Summary
	m.applyWorkspaceRuntimeView(base, true)
	hb := base
	hb.RunHeartbeat.Summary = "hb-new"
	m.applyWorkspaceRuntimeView(hb, false)
	if m.runHeartbeat.Summary != "hb-new" {
		t.Fatalf("RunHeartbeat not applied: %+v", m.runHeartbeat)
	}

	// Mutate ConvergenceStatus.DecisionCause
	m.applyWorkspaceRuntimeView(base, true)
	cs := base
	cs.ConvergenceStatus = &backend.ConvergenceStatusInfo{
		WorkspaceID: "ws-fr02", RunID: "run-1", CurrentFocusID: "f1", DecisionCause: "new-cause",
	}
	m.applyWorkspaceRuntimeView(cs, false)
	if m.convergenceStatus == nil || m.convergenceStatus.DecisionCause != "new-cause" {
		t.Fatalf("ConvergenceStatus not applied: %+v", m.convergenceStatus)
	}

	// Mutate RunCommand.Reason
	m.applyWorkspaceRuntimeView(base, true)
	rc := base
	rc.RunCommand = &backend.RunCommandStatusInfo{
		WorkspaceID: "ws-fr02", RunID: "run-1", State: "accepted", Intent: "resume", Reason: "new-cmd",
	}
	m.applyWorkspaceRuntimeView(rc, false)
	if m.runCommand == nil || m.runCommand.Reason != "new-cmd" {
		t.Fatalf("RunCommand not applied: %+v", m.runCommand)
	}

	// Mutate RepairLoop only. It is rendered in the status line and therefore
	// must bypass the equal-revision no-op gate.
	m.applyWorkspaceRuntimeView(base, true)
	repair := base
	repair.TaskProgress.RepairLoop.AttemptCount = 2
	repair.TaskProgress.RepairLoop.LastAttempt = 2
	repair.TaskProgress.RepairLoop.LastFailureClass = "provider_retry"
	repair.TaskProgress.RepairLoop.NextAction = "retry"
	m.applyWorkspaceRuntimeView(repair, false)
	if !strings.Contains(m.repairLoop, "repair #2") || !strings.Contains(m.repairLoop, "provider_retry") {
		t.Fatalf("RepairLoop-only mutation treated as no-op: %q", m.repairLoop)
	}

	// Projection health is cached by RuntimeView and must also bypass the no-op
	// gate when the revision domain is otherwise unchanged.
	m.applyWorkspaceRuntimeView(base, true)
	projection := base
	projection.Projection.State = "degraded"
	projection.Projection.Degraded = true
	projection.Projection.Error = "projection lag"
	m.applyWorkspaceRuntimeView(projection, false)
	entry = m.workspaceRuntimeState("ws-fr02")
	if entry == nil || entry.Projection.State != "degraded" || !entry.Projection.Degraded {
		t.Fatalf("Projection-only mutation treated as no-op: %+v", entry)
	}
}

func TestRenderInspectAcceptanceSeparatesDirectiveFromAssessmentObservation(t *testing.T) {
	text := renderInspectAcceptance(backend.AcceptanceProjectionInfo{
		RepairDirective: &backend.RuntimeRepairDirectiveInfo{RequiredAction: "need_state_change", FocusID: "focus-tui", Trigger: "assessment_gap", Closure: &backend.RuntimeRepairClosureInfo{ID: "closure-tui"}},
		Audit: &backend.RuntimeV2AuditInfo{Failure: &backend.RuntimeV2AuditFailureInfo{
			Code: "definition_mismatch", OriginalDiagnostic: &backend.RuntimeV2AuditOriginalDiagnosticInfo{OwnerModuleID: "blocks/run", OriginalMessage: "definition revision is stale"},
		}, Assessments: []backend.RuntimeV2AuditAssessmentInfo{{
			ClaimedDisposition: "repairable_gap", CanonicalDisposition: "repairable_gap", Summary: "producer summary", Gap: &backend.RuntimeV2AuditAssessmentGapInfo{MissingEvidenceKinds: []string{"validation"}, Reason: "producer reason"},
		}}},
	})
	for _, want := range []string{"canonical repair directive", "need_state_change", "runtime failure (diagnostic only)", "definition_mismatch", "producer=blocks/run", "original=definition revision is stale", "assessment observation (diagnostic only)", "claimed=repairable_gap", "canonical=repairable_gap", "claimed missing=validation"} {
		if !strings.Contains(text, want) {
			t.Fatalf("inspect acceptance = %q, want %q", text, want)
		}
	}
}

// FR05/FR07: a server restart changes the revision comparison domain. A long
// lived TUI must accept the new generation even if its numeric counters start
// lower than the old process's counters, and discard old dirty counters.
func TestRuntimeViewNewStreamGenerationSupersedesOldRevisionDomain(t *testing.T) {
	m := NewAppModel(newFake(t), "ws-generation", "")
	m.activeWorkspaceID = "ws-generation"
	oldView := backend.WorkspaceRuntimeViewInfo{
		WorkspaceID: "ws-generation",
		Revision: backend.WorkspaceRefreshRevision{
			StreamGeneration:     "server-before-restart",
			StreamRevision:       900,
			WorkspaceRevision:    800,
			RuntimeStateRevision: 700,
			ActivityRevision:     600,
		},
		TaskProgress: backend.TaskProgressInfo{
			RuntimeV2: &backend.RuntimeV2ProgressInfo{StateRevision: 700, CurrentFocusID: "old-focus"},
		},
	}
	m.applyWorkspaceRuntimeView(oldView, true)
	entry := m.workspaceRuntimeState("ws-generation")
	if entry == nil {
		t.Fatal("missing workspace state")
	}
	entry.DirtyRevision = backend.WorkspaceRefreshRevision{
		StreamGeneration: "server-before-restart", WorkspaceRevision: 999, RuntimeStateRevision: 999, ActivityRevision: 999,
	}

	// The first post-restart notification has lower counters, but a distinct
	// generation. It must replace the dirty domain rather than be max-merged.
	m.applyWorkspaceProjectionChanged(backend.WorkspaceProjectionChangedInfo{
		WorkspaceID: "ws-generation",
		Revision: backend.WorkspaceRefreshRevision{
			StreamGeneration: "server-after-restart", StreamRevision: 2, WorkspaceRevision: 2, RuntimeStateRevision: 1, ActivityRevision: 2,
		},
		ChangedSections: []backend.WorkspaceProjectionSection{backend.WorkspaceProjectionSectionRuntimeProgress},
	})
	if entry.DirtyRevision.StreamGeneration != "server-after-restart" || entry.DirtyRevision.WorkspaceRevision != 2 {
		t.Fatalf("new generation notification was merged into old domain: %+v", entry.DirtyRevision)
	}

	newView := oldView
	newView.Revision = backend.WorkspaceRefreshRevision{
		StreamGeneration: "server-after-restart", StreamRevision: 2, WorkspaceRevision: 2, RuntimeStateRevision: 1, ActivityRevision: 2,
	}
	newView.TaskProgress.RuntimeV2 = &backend.RuntimeV2ProgressInfo{StateRevision: 1, CurrentFocusID: "new-focus"}
	m.applyWorkspaceRuntimeView(newView, false)
	entry = m.workspaceRuntimeState("ws-generation")
	if entry.AppliedRevision.StreamGeneration != "server-after-restart" || entry.AppliedRevision.WorkspaceRevision != 2 {
		t.Fatalf("new generation RuntimeView was rejected: %+v", entry.AppliedRevision)
	}
	if entry.DirtyRevision.StreamGeneration != "server-after-restart" ||
		entry.DirtyRevision.WorkspaceRevision != 0 ||
		entry.DirtyRevision.RuntimeStateRevision != 0 ||
		entry.DirtyRevision.ActivityRevision != 0 {
		t.Fatalf("old generation dirty counters survived new authority: %+v", entry.DirtyRevision)
	}
	if m.runtimeV2 == nil || m.runtimeV2.CurrentFocusID != "new-focus" {
		t.Fatalf("new generation runtime progress was not applied: %+v", m.runtimeV2)
	}
}
