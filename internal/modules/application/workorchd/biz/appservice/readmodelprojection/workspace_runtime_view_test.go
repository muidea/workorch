package readmodelprojection

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchresultcontract"
)

func TestSelectDisplayRunIDPriority(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshotState{
		Workspace:      appmodelcore.Workspace{ID: "ws-1", ActiveRunID: "run-active"},
		DisplayOutcome: projectioncontract.WorkspaceDisplayOutcome{RunID: "run-outcome"},
		TaskProgress: projectioncontract.TaskProgress{
			RunID: "run-progress",
		},
		Runs: []appmodelcore.Run{
			{ID: "run-old", WorkspaceID: "ws-1", CreatedAt: "2026-07-01T00:00:00Z"},
			{ID: "run-latest", WorkspaceID: "ws-1", CreatedAt: "2026-07-15T00:00:00Z"},
		},
	}
	if got := selectDisplayRunID(snapshot); got != "run-active" {
		t.Fatalf("active display run = %q, want run-active", got)
	}

	snapshot.Workspace.ActiveRunID = ""
	snapshot.GovernanceDisplay = projectioncontract.GovernanceDisplay{PromptMode: "decision", RunID: "run-blocker", DecisionID: "ticket-1"}
	snapshot.Decisions = []projectioncontract.Decision{{
		ID:    "ticket-1",
		RunID: "run-blocker",
		State: "open",
	}}
	if got := selectDisplayRunID(snapshot); got != "run-blocker" {
		t.Fatalf("blocker display run = %q, want run-blocker", got)
	}

	snapshot.GovernanceDisplay = projectioncontract.GovernanceDisplay{}
	snapshot.Decisions = nil
	if got := selectDisplayRunID(snapshot); got != "run-outcome" {
		t.Fatalf("outcome display run = %q, want run-outcome", got)
	}

	snapshot.DisplayOutcome.RunID = ""
	if got := selectDisplayRunID(snapshot); got != "run-progress" {
		t.Fatalf("progress display run = %q, want run-progress", got)
	}

	snapshot.TaskProgress.RunID = ""
	if got := selectDisplayRunID(snapshot); got != "run-latest" {
		t.Fatalf("latest display run = %q, want run-latest", got)
	}
}

func TestStoredWorkspaceActivityFeedStableAndBounded(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-activity", ActiveRunID: "run-1"},
		TaskProgress: projectioncontract.TaskProgress{
			RunID: "run-1",
			State: "running",
			Runtime: &projectioncontract.RuntimeProgress{
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
		Results: []projectioncontract.Result{{
			ID:        "result-1",
			RunID:     "run-1",
			State:     "accepted",
			Summary:   "validation accepted",
			CreatedAt: "2026-07-15T14:33:00Z",
		}},
		Decisions: []projectioncontract.Decision{{
			ID:        "ticket-1",
			RunID:     "run-1",
			State:     "open",
			Question:  "choose recovery",
			CreatedAt: "2026-07-15T14:30:00Z",
		}},
		ViewModel: projectioncontract.WorkspaceViewModel{
			WorkspaceRevision: 12,
		},
	}

	content := buildWorkspaceActivityFeedContent(snapshot)
	committed := commitActivityFeedTransaction(projectioncontract.WorkspaceActivityFeed{}, content, 17)
	feed := pageWorkspaceActivityFeed(committed, 2, "")
	if feed.WorkspaceID != "ws-activity" || feed.DisplayRunID != "run-1" {
		t.Fatalf("feed scope = %+v", feed)
	}
	if feed.ActivityRevision != 17 {
		t.Fatalf("activity revision = %d, want store transaction 17", feed.ActivityRevision)
	}
	if committed.ActivityRevision != 17 {
		t.Fatalf("committed activity revision = %d, want transaction 17", committed.ActivityRevision)
	}
	// Unchanged content keeps previous revision.
	again := commitActivityFeedTransaction(committed, content, 18)
	if again.ActivityRevision != 17 {
		t.Fatalf("unchanged activity revision = %d, want keep 17", again.ActivityRevision)
	}
	// limit=2 is read-page truncation (not store-cap): Truncated + non-empty NextCursor.
	if !feed.Truncated || strings.TrimSpace(feed.NextCursor) == "" {
		t.Fatalf("expected page-truncated feed with next cursor, got %+v", feed)
	}
	if len(feed.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(feed.Items))
	}
	// FR03: store-cap content itself uses Truncated=true with empty NextCursor.
	// Content from buildWorkspaceActivityFeedContent is unbounded by page limit.
	if content.Truncated {
		t.Fatalf("small content must not be store-cap truncated: %+v", content)
	}
	seen := map[string]struct{}{}
	for _, item := range feed.Items {
		if item.ID == "" || item.Summary == "" {
			t.Fatalf("item missing id/summary: %+v", item)
		}
		if item.WorkspaceID != "ws-activity" {
			t.Fatalf("item workspace = %q", item.WorkspaceID)
		}
		if _, ok := seen[item.ID]; ok {
			t.Fatalf("duplicate activity id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
}

func TestCommitActivityFeedTransactionAppendsSameRunRuntimeProgress(t *testing.T) {
	previous := projectioncontract.WorkspaceActivityFeed{
		WorkspaceID:      "ws-stream",
		DisplayRunID:     "run-1",
		ActivityRevision: 4,
		Items: []projectioncontract.WorkspaceActivityItem{{
			ID:                   "runtime-run-1-rev-1",
			WorkspaceID:          "ws-stream",
			RunID:                "run-1",
			Kind:                 projectioncontract.WorkspaceActivityRuntimeTransition,
			Summary:              "runtime focus focus-1 activated",
			RuntimeStateRevision: 1,
		}},
	}
	candidate := projectioncontract.WorkspaceActivityFeed{
		WorkspaceID:  "ws-stream",
		DisplayRunID: "run-1",
		Items: []projectioncontract.WorkspaceActivityItem{{
			ID:                   "runtime-run-1-rev-2",
			WorkspaceID:          "ws-stream",
			RunID:                "run-1",
			Kind:                 projectioncontract.WorkspaceActivityRuntimeTransition,
			Summary:              "runtime convergence need_validation",
			RuntimeStateRevision: 2,
		}},
	}
	merged := commitActivityFeedTransaction(previous, candidate, 5)
	if len(merged.Items) != 2 {
		t.Fatalf("merged activity items = %d, want 2: %+v", len(merged.Items), merged.Items)
	}
	if merged.Items[0].ID != "runtime-run-1-rev-1" || merged.Items[1].ID != "runtime-run-1-rev-2" {
		t.Fatalf("merged activity order = %+v", merged.Items)
	}
	if merged.ActivityRevision != 5 {
		t.Fatalf("activity revision = %d, want 5", merged.ActivityRevision)
	}
	newRun := candidate
	newRun.DisplayRunID = "run-2"
	newRun.Items[0].RunID = "run-2"
	reset := commitActivityFeedTransaction(merged, newRun, 6)
	if len(reset.Items) != 1 || reset.Items[0].ID != "runtime-run-1-rev-2" {
		t.Fatalf("new run should reset stream, got %+v", reset.Items)
	}
}

func TestProjectWorkspaceRuntimeViewUsesRuntimeSummaryAndDisplayRun(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:          "ws-runtime",
			ActiveRunID: "run-runtime",
			State:       "active",
		},
		ViewModel: projectioncontract.WorkspaceViewModel{
			WorkspaceID:       "ws-runtime",
			PromptMode:        "running",
			WorkspaceRevision: 21,
		},
		TaskProgress: projectioncontract.TaskProgress{
			WorkspaceID: "ws-runtime",
			RunID:       "run-runtime",
			State:       "running",
			Summary:     "stale step summary",
			Runtime: &projectioncontract.RuntimeProgress{
				StateRevision:    9,
				EvidenceSequence: 4,
				CurrentFocusID:   "focus-1",
				ConvergenceState: "need_validation",
			},
		},
		Runs: []appmodelcore.Run{{
			ID:          "run-runtime",
			WorkspaceID: "ws-runtime",
			State:       "running",
		}},
		DisplayOutcome: projectioncontract.WorkspaceDisplayOutcome{
			WorkspaceID: "ws-runtime",
			State:       "running",
			RunID:       "run-runtime",
		},
	}

	view := projectWorkspaceRuntimeView(snapshot, 10, "")
	if view.DisplayRunID != "run-runtime" || view.ActiveRunID != "run-runtime" {
		t.Fatalf("ids = active=%q display=%q", view.ActiveRunID, view.DisplayRunID)
	}
	if view.Revision.RuntimeStateRevision != 9 || view.Revision.EvidenceSequence != 4 {
		t.Fatalf("revision = %+v", view.Revision)
	}
	if !strings.Contains(view.TaskProgress.Summary, "revision 9") {
		t.Fatalf("summary = %q, want runtime summary", view.TaskProgress.Summary)
	}
	// Transitional RuntimeView path keeps activity revision 0 until store-backed read is wired.
	if view.Activity.ActivityRevision != 0 {
		t.Fatalf("activity revision = %d, want 0 without store activity authority", view.Activity.ActivityRevision)
	}
	if view.PromptMode != "running" {
		t.Fatalf("prompt mode = %q", view.PromptMode)
	}
}

func TestWorkspaceRuntimeViewBlockersRequireCanonicalRunReference(t *testing.T) {
	t.Parallel()

	t.Run("decision", func(t *testing.T) {
		snapshot := readmodelcore.WorkspaceSnapshotState{
			Workspace: appmodelcore.Workspace{ID: "ws-owner", State: "active", ActiveRunID: "run-owner"},
			Run: &appmodelcore.Run{
				ID:               "run-owner",
				WorkspaceID:      "ws-owner",
				State:            "blocked_on_decision",
				BlockingTicketID: "ticket-owner",
			},
			GovernanceDisplay: projectioncontract.GovernanceDisplay{
				PromptMode: "decision", RunID: "run-owner", DecisionID: "ticket-display",
			},
			Decisions: []projectioncontract.Decision{
				{ID: "ticket-display", WorkspaceID: "ws-owner", RunID: "run-owner", State: "open"},
				{ID: "ticket-owner", WorkspaceID: "ws-owner", RunID: "run-owner", State: "open"},
			},
		}

		view := projectWorkspaceRuntimeView(snapshot, 10, "")
		if view.BlockingDecision == nil || view.BlockingDecision.ID != "ticket-owner" {
			t.Fatalf("blocking decision = %+v, want exact canonical Run blocker ticket-owner", view.BlockingDecision)
		}

		snapshot.Run.BlockingTicketID = ""
		if got := projectWorkspaceRuntimeView(snapshot, 10, "").BlockingDecision; got != nil {
			t.Fatalf("display-only decision selected mutation resource: %+v", got)
		}
		snapshot.Run.BlockingTicketID = "ticket-owner"
		snapshot.Run.State = "active"
		if got := projectWorkspaceRuntimeView(snapshot, 10, "").BlockingDecision; got != nil {
			t.Fatalf("non-blocked canonical Run selected decision resource: %+v", got)
		}
	})

	t.Run("approval", func(t *testing.T) {
		snapshot := readmodelcore.WorkspaceSnapshotState{
			Workspace: appmodelcore.Workspace{ID: "ws-owner", State: "blocked_on_approval", ActiveRunID: "run-owner"},
			Run: &appmodelcore.Run{
				ID:                 "run-owner",
				WorkspaceID:        "ws-owner",
				State:              "blocked_on_approval",
				BlockingApprovalID: "approval-owner",
			},
			GovernanceDisplay: projectioncontract.GovernanceDisplay{
				PromptMode: "approval", RunID: "run-owner", ApprovalID: "approval-display",
			},
			Approvals: []projectioncontract.Approval{
				{ID: "approval-display", WorkspaceID: "ws-owner", RunID: "run-owner", State: "pending"},
				{ID: "approval-owner", WorkspaceID: "ws-owner", RunID: "run-owner", State: "pending"},
			},
		}

		view := projectWorkspaceRuntimeView(snapshot, 10, "")
		if view.BlockingApproval == nil || view.BlockingApproval.ID != "approval-owner" {
			t.Fatalf("blocking approval = %+v, want exact canonical Run blocker approval-owner", view.BlockingApproval)
		}

		snapshot.Run.BlockingApprovalID = ""
		if got := projectWorkspaceRuntimeView(snapshot, 10, "").BlockingApproval; got != nil {
			t.Fatalf("display-only approval selected mutation resource: %+v", got)
		}
		snapshot.Run.BlockingApprovalID = "approval-owner"
		snapshot.Run.State = "active"
		if got := projectWorkspaceRuntimeView(snapshot, 10, "").BlockingApproval; got != nil {
			t.Fatalf("non-blocked canonical Run selected approval resource: %+v", got)
		}
	})
}

func TestProjectWorkspaceRuntimeViewKeepsOnlyDisplayRunSemanticPlanningAtRuntimeRevision(t *testing.T) {
	t.Parallel()

	base := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-semantic", ActiveRunID: "run-current", State: "active"},
		TaskProgress: projectioncontract.TaskProgress{RunID: "run-current", Runtime: &projectioncontract.RuntimeProgress{
			StateRevision: 12,
		}},
		Runs: []appmodelcore.Run{{ID: "run-current", WorkspaceID: "ws-semantic", State: "active"}},
		Acceptance: projectioncontract.AcceptanceProjection{SemanticPlanning: &projectioncontract.RuntimeSemanticPlanning{
			WorkspaceID: "ws-semantic", RunID: "run-current", StateRevision: 12, State: "planning",
		}},
	}

	view := projectWorkspaceRuntimeView(base, 10, "")
	if view.Acceptance.SemanticPlanning == nil || view.Acceptance.SemanticPlanning.RunID != "run-current" ||
		view.Revision.RuntimeStateRevision != 12 {
		t.Fatalf("current semantic planning/runtime revision = %+v/%+v", view.Acceptance.SemanticPlanning, view.Revision)
	}

	staleRun := base
	staleRun.Acceptance.SemanticPlanning = &projectioncontract.RuntimeSemanticPlanning{
		WorkspaceID: "ws-semantic", RunID: "run-stale", StateRevision: 12, State: "planning",
	}
	if got := projectWorkspaceRuntimeView(staleRun, 10, ""); got.Acceptance.SemanticPlanning != nil {
		t.Fatalf("cross-run semantic planning survived projection: %+v", got.Acceptance.SemanticPlanning)
	}

	staleRevision := base
	staleRevision.Acceptance.SemanticPlanning = &projectioncontract.RuntimeSemanticPlanning{
		WorkspaceID: "ws-semantic", RunID: "run-current", StateRevision: 11, State: "planning",
	}
	if got := projectWorkspaceRuntimeView(staleRevision, 10, ""); got.Acceptance.SemanticPlanning != nil {
		t.Fatalf("cross-revision semantic planning survived projection: %+v", got.Acceptance.SemanticPlanning)
	}

	withoutProgress := base
	withoutProgress.TaskProgress.Runtime = nil
	withoutProgress.Acceptance.SemanticPlanning = &projectioncontract.RuntimeSemanticPlanning{
		WorkspaceID: "ws-semantic", RunID: "run-current", StateRevision: 15, State: "planning",
	}
	got := projectWorkspaceRuntimeView(withoutProgress, 10, "")
	if got.Acceptance.SemanticPlanning == nil || got.Revision.RuntimeStateRevision != 15 {
		t.Fatalf("semantic planning did not establish missing runtime high-watermark: %+v/%+v", got.Acceptance.SemanticPlanning, got.Revision)
	}
}

func TestDeriveChangedSectionsForDetail(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshotState{
		Workspace:         appmodelcore.Workspace{ID: "ws-sections"},
		GovernanceDisplay: projectioncontract.GovernanceDisplay{PromptMode: "decision", DecisionID: "ticket-1"},
		TaskProgress: projectioncontract.TaskProgress{
			Runtime: &projectioncontract.RuntimeProgress{StateRevision: 1},
		},
		Results:   []projectioncontract.Result{{ID: "result-1"}},
		Artifacts: []projectioncontract.Artifact{{ID: "artifact-1"}},
		MemoryRecallTraces: []projectioncontract.MemoryRecallTrace{{
			ID: "memory-1",
		}},
		LLMUsageSummary: projectioncontract.LLMUsageSummary{EntryTotal: 2},
	}
	sections := DeriveChangedSections("detail", snapshot)
	want := map[string]bool{
		"overview":         true,
		"runtime_progress": true,
		"activity":         true,
		"evidence":         true,
		"governance":       true,
		"result":           true,
		"artifacts":        true,
		"memory":           true,
		"llm_usage":        true,
	}
	for _, section := range sections {
		if !want[section] {
			t.Fatalf("unexpected section %q in %v", section, sections)
		}
		delete(want, section)
	}
	if len(want) != 0 {
		t.Fatalf("missing sections: %v", want)
	}
}

func TestRuntimeProgressSummaryChangesWithRevision(t *testing.T) {
	t.Parallel()

	first := runtimeProgressSummary(&projectioncontract.RuntimeProgress{
		StateRevision: 1, EvidenceSequence: 1, CurrentFocusID: "focus-a", ConvergenceState: "need_validation",
	})
	second := runtimeProgressSummary(&projectioncontract.RuntimeProgress{
		StateRevision: 2, EvidenceSequence: 1, CurrentFocusID: "focus-a", ConvergenceState: "need_validation",
	})
	if first == "" || second == "" || first == second {
		t.Fatalf("summary must change with revision: first=%q second=%q", first, second)
	}
}

func TestDiffWorkspaceProjectionSectionsDetectsSameLengthContentMutation(t *testing.T) {
	previous := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:          "ws-diff",
			Title:       "diff",
			State:       "active",
			ActiveRunID: "run-1",
		},
		GovernanceDisplay: projectioncontract.GovernanceDisplay{PromptMode: "decision", RunID: "run-1", DecisionID: "ticket-1"},
		Decisions: []projectioncontract.Decision{{
			ID: "ticket-1", WorkspaceID: "ws-diff", RunID: "run-1", State: "open", Question: "choose path?",
		}},
		Approvals: []projectioncontract.Approval{{
			ID: "approval-1", WorkspaceID: "ws-diff", RunID: "run-1", State: "pending", Reason: "apply patch",
		}},
		Results: []projectioncontract.Result{{
			ID: "result-1", WorkspaceID: "ws-diff", RunID: "run-1", State: "accepted", Summary: "ok",
		}},
		Artifacts: []projectioncontract.Artifact{{
			ID: "artifact-1", WorkspaceID: "ws-diff", RunID: "run-1", State: "ready", Name: "report.md",
		}},
		Patches: []projectioncontract.Patch{{
			ID: "patch-1", WorkspaceID: "ws-diff", RunID: "run-1", State: "previewed", Summary: "edit",
		}},
		MemoryRecallTraces: []projectioncontract.MemoryRecallTrace{{
			ID: "memory-1", WorkspaceID: "ws-diff", RunID: "run-1", Status: "selected", Policy: "semantic",
		}},
		LLMUsageTraces: []projectioncontract.LLMUsageTrace{{
			ID: "llm-1", WorkspaceID: "ws-diff", RunID: "run-1", Status: "ok", Model: "m1", TotalTokens: 10,
		}},
		LLMUsageSummary: projectioncontract.LLMUsageSummary{EntryTotal: 1},
		TaskProgress: projectioncontract.TaskProgress{
			State:   "running",
			Summary: "same",
			Runtime: &projectioncontract.RuntimeProgress{StateRevision: 1, EvidenceSequence: 1, CurrentFocusID: "f1"},
		},
	}
	candidate := previous
	// Same lengths, mutated identity/state fields.
	candidate.Decisions = append([]projectioncontract.Decision(nil), previous.Decisions...)
	candidate.Decisions[0].State = "resolved"
	candidate.Approvals = append([]projectioncontract.Approval(nil), previous.Approvals...)
	candidate.Approvals[0].State = "approved"
	candidate.Results = append([]projectioncontract.Result(nil), previous.Results...)
	candidate.Results[0].Summary = "updated"
	candidate.Artifacts = append([]projectioncontract.Artifact(nil), previous.Artifacts...)
	candidate.Artifacts[0].Name = "report-v2.md"
	candidate.Patches = append([]projectioncontract.Patch(nil), previous.Patches...)
	candidate.Patches[0].State = "applied"
	candidate.MemoryRecallTraces = append([]projectioncontract.MemoryRecallTrace(nil), previous.MemoryRecallTraces...)
	candidate.MemoryRecallTraces[0].Status = "none_selected"
	candidate.LLMUsageTraces = append([]projectioncontract.LLMUsageTrace(nil), previous.LLMUsageTraces...)
	candidate.LLMUsageTraces[0].TotalTokens = 42

	sections := diffWorkspaceProjectionSections(previous, candidate, "detail")
	want := map[string]bool{
		"governance": true,
		"result":     true,
		"artifacts":  true,
		"memory":     true,
		"llm_usage":  true,
		"activity":   true,
	}
	for _, section := range sections {
		delete(want, section)
	}
	if len(want) != 0 {
		t.Fatalf("missing sections for same-length content mutations: missing=%v got=%v", want, sections)
	}

	// Unchanged content must not mark sections via ChangeKind fallback.
	unchanged := diffWorkspaceProjectionSections(previous, previous, "detail")
	if len(unchanged) != 0 {
		t.Fatalf("unchanged detail diff = %v, want empty (no ChangeKind fallback)", unchanged)
	}
}

// CR02: same-length objects that only mutate context/detail/evidence/metadata/
// cache/usage/provider fields must still mark the corresponding ChangedSections.
func TestDiffWorkspaceProjectionSectionsDetectsFullDisplayFieldMutations(t *testing.T) {
	mustPayload := func(input map[string]any) workorcheventpayload.Payload {
		t.Helper()
		payload, err := workorcheventpayload.FromMap(input)
		if err != nil {
			t.Fatalf("FromMap: %v", err)
		}
		return payload
	}
	previous := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID: "ws-diff-full", Title: "diff", State: "active", ActiveRunID: "run-1",
		},
		GovernanceDisplay: projectioncontract.GovernanceDisplay{PromptMode: "decision", RunID: "run-1", DecisionID: "ticket-1"},
		Decisions: []projectioncontract.Decision{{
			ID: "ticket-1", WorkspaceID: "ws-diff-full", RunID: "run-1", State: "open",
			Question: "choose path?", Reason: "need choice",
			Context: mustPayload(map[string]any{"hint": "old"}),
		}},
		Approvals: []projectioncontract.Approval{{
			ID: "approval-1", WorkspaceID: "ws-diff-full", RunID: "run-1", State: "pending",
			Reason: "apply patch", Action: "apply",
			Context: mustPayload(map[string]any{"risk": "low"}),
		}},
		Results: []projectioncontract.Result{{
			ID: "result-1", WorkspaceID: "ws-diff-full", RunID: "run-1", State: "accepted", Summary: "ok",
			EvidenceRefs: []string{"ev-1"},
			SummaryDetail: &workorchresultcontract.ResultSummary{
				GoalOutcome: "done", ImplementationSummary: "impl-old", EvidenceRefs: []string{"ev-1"},
			},
		}},
		Artifacts: []projectioncontract.Artifact{{
			ID: "artifact-1", WorkspaceID: "ws-diff-full", RunID: "run-1", State: "ready",
			Name: "report.md", Summary: "report", URI: "file://report.md",
			Metadata: map[string]any{"version": "1"},
		}},
		Patches: []projectioncontract.Patch{{
			ID: "patch-1", WorkspaceID: "ws-diff-full", RunID: "run-1", State: "previewed",
			Summary: "edit", TargetRef: "main.go", RiskLevel: "low",
			AffectedFiles: []string{"main.go"}, RiskReasons: []string{"touch core"},
		}},
		MemoryRecallTraces: []projectioncontract.MemoryRecallTrace{{
			ID: "memory-1", WorkspaceID: "ws-diff-full", RunID: "run-1", Status: "selected",
			Policy: "semantic", Source: "vector",
			SelectedMemoryIDs: []string{"m1"}, RetrievedMemoryIDs: []string{"m1"},
		}},
		LLMUsageTraces: []projectioncontract.LLMUsageTrace{{
			ID: "llm-1", WorkspaceID: "ws-diff-full", RunID: "run-1", Status: "ok",
			Model: "m1", Provider: "p1", ProviderID: "pid-1", TotalTokens: 10, InputTokens: 6, OutputTokens: 4,
			CacheUsage: mustPayload(map[string]any{"read": 1}),
		}},
		LLMUsageSummary: projectioncontract.LLMUsageSummary{
			EntryTotal: 1, TotalTokens: 10, CacheReadTokens: 1,
			ProviderModelBreakdown: []projectioncontract.LLMUsageProviderModelBreakdown{{
				Provider: "p1", Model: "m1", EntryTotal: 1, TotalTokens: 10,
			}},
		},
		TaskProgress: projectioncontract.TaskProgress{
			State: "running", Summary: "same",
			Runtime: &projectioncontract.RuntimeProgress{StateRevision: 1, EvidenceSequence: 1, CurrentFocusID: "f1"},
		},
	}
	candidate := previous
	candidate.Decisions = append([]projectioncontract.Decision(nil), previous.Decisions...)
	candidate.Decisions[0].Context = mustPayload(map[string]any{"hint": "new"})
	candidate.Approvals = append([]projectioncontract.Approval(nil), previous.Approvals...)
	candidate.Approvals[0].Context = mustPayload(map[string]any{"risk": "high"})
	candidate.Results = append([]projectioncontract.Result(nil), previous.Results...)
	candidate.Results[0].EvidenceRefs = []string{"ev-2"}
	candidate.Results[0].SummaryDetail = &workorchresultcontract.ResultSummary{
		GoalOutcome: "done", ImplementationSummary: "impl-new", EvidenceRefs: []string{"ev-2"},
	}
	candidate.Artifacts = append([]projectioncontract.Artifact(nil), previous.Artifacts...)
	candidate.Artifacts[0].Metadata = map[string]any{"version": "2"}
	candidate.Artifacts[0].Summary = "report v2"
	candidate.Patches = append([]projectioncontract.Patch(nil), previous.Patches...)
	candidate.Patches[0].RiskLevel = "high"
	candidate.Patches[0].AffectedFiles = []string{"main.go", "util.go"}
	candidate.MemoryRecallTraces = append([]projectioncontract.MemoryRecallTrace(nil), previous.MemoryRecallTraces...)
	candidate.MemoryRecallTraces[0].SelectedMemoryIDs = []string{"m1", "m2"}
	candidate.MemoryRecallTraces[0].RetrievedMemoryHits = []workorchmemorycontract.RecallHit{{ID: "m2", Summary: "hit"}}
	candidate.LLMUsageTraces = append([]projectioncontract.LLMUsageTrace(nil), previous.LLMUsageTraces...)
	candidate.LLMUsageTraces[0].Provider = "p2"
	candidate.LLMUsageTraces[0].CacheUsage = mustPayload(map[string]any{"read": 3})
	candidate.LLMUsageSummary = previous.LLMUsageSummary
	candidate.LLMUsageSummary.CacheReadTokens = 3
	candidate.LLMUsageSummary.ProviderModelBreakdown = []projectioncontract.LLMUsageProviderModelBreakdown{{
		Provider: "p2", Model: "m1", EntryTotal: 1, TotalTokens: 10,
	}}

	sections := diffWorkspaceProjectionSections(previous, candidate, "detail")
	want := map[string]bool{
		"governance": true,
		"result":     true,
		"artifacts":  true,
		"memory":     true,
		"llm_usage":  true,
		"activity":   true,
	}
	for _, section := range sections {
		delete(want, section)
	}
	if len(want) != 0 {
		t.Fatalf("missing sections for full display-field mutations: missing=%v got=%v", want, sections)
	}
}

func DeriveChangedSections(changeKind string, snapshot readmodelcore.WorkspaceSnapshotState) []string {
	changeKind = strings.ToLower(strings.TrimSpace(changeKind))
	sections := make([]string, 0, 8)
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		for _, existing := range sections {
			if existing == name {
				return
			}
		}
		sections = append(sections, name)
	}
	switch changeKind {
	case "overview", "baseline", "deleted":
		add("overview")
	case "run_command_accepted", "run_command_rejected":
		add("overview")
		add("runtime_progress")
	case "detail", "detail_failed", "":
		add("overview")
		add("runtime_progress")
		add("activity")
		if snapshot.TaskProgress.Runtime != nil {
			add("evidence")
		}
		if len(snapshot.Decisions) > 0 || len(snapshot.Approvals) > 0 ||
			strings.TrimSpace(snapshot.GovernanceDisplay.DecisionID) != "" ||
			strings.TrimSpace(snapshot.GovernanceDisplay.ApprovalID) != "" {
			add("governance")
		}
		if strings.TrimSpace(snapshot.DisplayOutcome.State) != "" || len(snapshot.Results) > 0 {
			add("result")
		}
		if len(snapshot.Artifacts) > 0 || len(snapshot.Patches) > 0 {
			add("artifacts")
		}
		if len(snapshot.MemoryRecallTraces) > 0 {
			add("memory")
		}
		if len(snapshot.LLMUsageTraces) > 0 || snapshot.LLMUsageSummary.EntryTotal > 0 {
			add("llm_usage")
		}
	default:
		add("overview")
		add("runtime_progress")
		add("activity")
	}
	return sections
}

// FR06: delimiter-bearing strings must not collide with multi-element joins.
func TestStableAnyFactDelimiterCollisionFree(t *testing.T) {
	// ["a,b"] vs ["a","b"] previously both encoded as as:[a,b].
	left := stableAnyFact([]string{"a,b"})
	right := stableAnyFact([]string{"a", "b"})
	if left == right {
		t.Fatalf("delimiter collision: %q == %q", left, right)
	}
	// Nested map keys/values with '=' and ',' must not collide.
	a := map[string]any{"k=v": "1,2"}
	b := map[string]any{"k": "v=1,2"}
	if stableMapFactsEqual(a, b) {
		t.Fatalf("map fact collision: a=%v b=%v", stableMapFactPairs(a), stableMapFactPairs(b))
	}
	// Equal content still equal.
	if !stableMapFactsEqual(a, map[string]any{"k=v": "1,2"}) {
		t.Fatal("equal maps reported unequal")
	}
	// Nested slice of maps
	nestedA := []map[string]any{{"x": []string{"a,b"}}}
	nestedB := []map[string]any{{"x": []string{"a", "b"}}}
	if stableMapSliceFactsEqual(nestedA, nestedB) {
		t.Fatal("nested slice map collision")
	}
	// Unsupported Go values must conservatively force a changed section instead
	// of falling back to a potentially colliding fmt.Sprint representation.
	unsupported := map[string]any{"value": uint(1)}
	if stableMapFactsEqual(unsupported, unsupported) {
		t.Fatal("unsupported comparison value must not be silently considered equal")
	}
}

// FR03: Truncated transition alone advances content inequality.
func TestActivityFeedContentEqualIncludesTruncationBoundary(t *testing.T) {
	base := projectioncontract.WorkspaceActivityFeed{
		WorkspaceID: "ws-1", DisplayRunID: "run-1",
		Items: []projectioncontract.WorkspaceActivityItem{{ID: "a", Summary: "s"}},
	}
	capped := base
	capped.Truncated = true
	if activityFeedContentEqual(base, capped) {
		t.Fatal("Truncated change must break content equality")
	}
	// Empty NextCursor on both still differs only by Truncated.
	capped.NextCursor = ""
	if activityFeedContentEqual(base, capped) {
		t.Fatal("Truncated=true with empty cursor must still differ")
	}
}
