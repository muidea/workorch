package readmodelprojection

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestSelectDisplayRunIDPriority(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{ID: "ws-1", ActiveRunID: "run-active"},
		Outcome:   readmodelcore.WorkspaceOutcome{RunID: "run-outcome"},
		TaskProgress: readmodelcore.TaskProgress{
			RunID: "run-progress",
		},
		Runs: []appmodelcore.Run{
			{ID: "run-old", WorkspaceID: "ws-1", CreatedAt: "2026-07-01T00:00:00Z"},
			{ID: "run-latest", WorkspaceID: "ws-1", CreatedAt: "2026-07-15T00:00:00Z"},
		},
	}
	if got := SelectDisplayRunID(snapshot); got != "run-active" {
		t.Fatalf("active display run = %q, want run-active", got)
	}

	snapshot.Workspace.ActiveRunID = ""
	snapshot.Workspace.BlockingTicketID = "ticket-1"
	snapshot.Decisions = []appmodelcore.Decision{{
		ID:    "ticket-1",
		RunID: "run-blocker",
		State: "open",
	}}
	if got := SelectDisplayRunID(snapshot); got != "run-blocker" {
		t.Fatalf("blocker display run = %q, want run-blocker", got)
	}

	snapshot.Workspace.BlockingTicketID = ""
	snapshot.Decisions = nil
	if got := SelectDisplayRunID(snapshot); got != "run-outcome" {
		t.Fatalf("outcome display run = %q, want run-outcome", got)
	}

	snapshot.Outcome.RunID = ""
	if got := SelectDisplayRunID(snapshot); got != "run-progress" {
		t.Fatalf("progress display run = %q, want run-progress", got)
	}

	snapshot.TaskProgress.RunID = ""
	if got := SelectDisplayRunID(snapshot); got != "run-latest" {
		t.Fatalf("latest display run = %q, want run-latest", got)
	}
}

func TestProjectWorkspaceActivityFeedStableAndBounded(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{ID: "ws-activity", ActiveRunID: "run-1"},
		TaskProgress: readmodelcore.TaskProgress{
			RunID: "run-1",
			State: "running",
			RuntimeV2: &readmodelcore.RuntimeV2Progress{
				StateRevision:          42,
				EvidenceSequence:       7,
				CurrentFocusID:         "focus-003",
				CurrentAction:          "produce_validation",
				CompletedDispatchCount: 3,
				CandidateAttemptCount:  2,
				AcceptedEvidenceCount:  1,
				RejectedEvidenceCount:  1,
				MissingEvidenceKinds:   []string{"validation"},
			},
		},
		Results: []readmodelcore.Result{{
			ID:        "result-1",
			RunID:     "run-1",
			State:     "accepted",
			Summary:   "validation accepted",
			CreatedAt: "2026-07-15T14:33:00Z",
		}},
		Decisions: []appmodelcore.Decision{{
			ID:        "ticket-1",
			RunID:     "run-1",
			State:     "open",
			Question:  "choose recovery",
			CreatedAt: "2026-07-15T14:30:00Z",
		}},
		ViewModel: readmodelcore.WorkspaceViewModel{
			WorkspaceRevision: 12,
		},
	}

	// Transitional ProjectWorkspaceActivityFeed no longer invents fingerprint revision.
	feed := ProjectWorkspaceActivityFeed(snapshot, 2, "")
	if feed.WorkspaceID != "ws-activity" || feed.DisplayRunID != "run-1" {
		t.Fatalf("feed scope = %+v", feed)
	}
	if feed.ActivityRevision != 0 {
		t.Fatalf("transitional activity revision = %d, want 0 (store assigns transaction revision)", feed.ActivityRevision)
	}
	content := BuildWorkspaceActivityFeedContent(snapshot)
	committed := CommitActivityFeedTransaction(readmodelcore.WorkspaceActivityFeed{}, content, 17)
	if committed.ActivityRevision != 17 {
		t.Fatalf("committed activity revision = %d, want transaction 17", committed.ActivityRevision)
	}
	// Unchanged content keeps previous revision.
	again := CommitActivityFeedTransaction(committed, content, 18)
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
	// content from BuildWorkspaceActivityFeedContent is unbounded by page limit.
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

func TestProjectWorkspaceRuntimeViewUsesRuntimeSummaryAndDisplayRun(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{
			ID:          "ws-runtime",
			ActiveRunID: "run-runtime",
			State:       "active",
		},
		ViewModel: readmodelcore.WorkspaceViewModel{
			WorkspaceID:       "ws-runtime",
			PromptMode:        "running",
			WorkspaceRevision: 21,
		},
		TaskProgress: readmodelcore.TaskProgress{
			RunID:   "run-runtime",
			State:   "running",
			Summary: "stale step summary",
			RuntimeV2: &readmodelcore.RuntimeV2Progress{
				StateRevision:    9,
				EvidenceSequence: 4,
				CurrentFocusID:   "focus-1",
				CurrentAction:    "produce_validation",
			},
		},
		Runs: []appmodelcore.Run{{
			ID:    "run-runtime",
			State: "running",
		}},
		Outcome: readmodelcore.WorkspaceOutcome{
			WorkspaceID: "ws-runtime",
			State:       "running",
			RunID:       "run-runtime",
		},
	}

	view := ProjectWorkspaceRuntimeView(snapshot, 10, "")
	if view.DisplayRunID != "run-runtime" || view.ActiveRunID != "run-runtime" {
		t.Fatalf("ids = active=%q display=%q", view.ActiveRunID, view.DisplayRunID)
	}
	if view.Revision.RuntimeStateRevision != 9 || view.Revision.EvidenceSequence != 4 {
		t.Fatalf("revision = %+v", view.Revision)
	}
	if !strings.Contains(view.TaskProgress.Summary, "revision 9") {
		t.Fatalf("summary = %q, want runtime v2 summary", view.TaskProgress.Summary)
	}
	// Transitional RuntimeView path keeps activity revision 0 until store-backed read is wired.
	if view.Activity.ActivityRevision != 0 {
		t.Fatalf("activity revision = %d, want 0 on transitional path", view.Activity.ActivityRevision)
	}
	if view.PromptMode != "running" {
		t.Fatalf("prompt mode = %q", view.PromptMode)
	}
}

func TestDeriveChangedSectionsForDetail(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshot{
		Workspace: appmodelcore.Workspace{
			ID:               "ws-sections",
			BlockingTicketID: "ticket-1",
		},
		TaskProgress: readmodelcore.TaskProgress{
			RuntimeV2: &readmodelcore.RuntimeV2Progress{StateRevision: 1},
		},
		Results:   []readmodelcore.Result{{ID: "result-1"}},
		Artifacts: []readmodelcore.Artifact{{ID: "artifact-1"}},
		MemoryRecallTraces: []readmodelcore.MemoryRecallTrace{{
			ID: "memory-1",
		}},
		LLMUsageSummary: readmodelcore.LLMUsageSummary{EntryTotal: 2},
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

func TestRuntimeV2ProgressSummaryChangesWithRevision(t *testing.T) {
	t.Parallel()

	first := RuntimeV2ProgressSummary(&readmodelcore.RuntimeV2Progress{
		StateRevision: 1, EvidenceSequence: 1, CurrentFocusID: "focus-a", CurrentAction: "act-a",
	})
	second := RuntimeV2ProgressSummary(&readmodelcore.RuntimeV2Progress{
		StateRevision: 2, EvidenceSequence: 1, CurrentFocusID: "focus-a", CurrentAction: "act-a",
	})
	if first == "" || second == "" || first == second {
		t.Fatalf("summary must change with revision: first=%q second=%q", first, second)
	}
}

func TestDiffWorkspaceProjectionSectionsDetectsSameLengthContentMutation(t *testing.T) {
	previous := readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{
			ID:                 "ws-diff",
			Title:              "diff",
			State:              "active",
			ActiveRunID:        "run-1",
			BlockingTicketID:   "ticket-1",
			BlockingApprovalID: "",
		},
		Decisions: []readmodelcore.Decision{{
			ID: "ticket-1", WorkspaceID: "ws-diff", RunID: "run-1", State: "open", Question: "choose path?",
		}},
		Approvals: []readmodelcore.Approval{{
			ID: "approval-1", WorkspaceID: "ws-diff", RunID: "run-1", State: "pending", Reason: "apply patch",
		}},
		Results: []readmodelcore.Result{{
			ID: "result-1", WorkspaceID: "ws-diff", RunID: "run-1", State: "accepted", Summary: "ok",
		}},
		Artifacts: []readmodelcore.Artifact{{
			ID: "artifact-1", WorkspaceID: "ws-diff", RunID: "run-1", State: "ready", Name: "report.md",
		}},
		Patches: []readmodelcore.Patch{{
			ID: "patch-1", WorkspaceID: "ws-diff", RunID: "run-1", State: "previewed", Summary: "edit",
		}},
		MemoryRecallTraces: []readmodelcore.MemoryRecallTrace{{
			ID: "memory-1", WorkspaceID: "ws-diff", RunID: "run-1", Status: "selected", Policy: "semantic",
		}},
		LLMUsageTraces: []readmodelcore.LLMUsageTrace{{
			ID: "llm-1", WorkspaceID: "ws-diff", RunID: "run-1", Status: "ok", Model: "m1", TotalTokens: 10,
		}},
		LLMUsageSummary: readmodelcore.LLMUsageSummary{EntryTotal: 1},
		TaskProgress: readmodelcore.TaskProgress{
			State:     "running",
			Summary:   "same",
			RuntimeV2: &readmodelcore.RuntimeV2Progress{StateRevision: 1, EvidenceSequence: 1, CurrentFocusID: "f1"},
		},
	}
	candidate := previous
	// Same lengths, mutated identity/state fields.
	candidate.Decisions = append([]readmodelcore.Decision(nil), previous.Decisions...)
	candidate.Decisions[0].State = "resolved"
	candidate.Approvals = append([]readmodelcore.Approval(nil), previous.Approvals...)
	candidate.Approvals[0].State = "approved"
	candidate.Results = append([]readmodelcore.Result(nil), previous.Results...)
	candidate.Results[0].Summary = "updated"
	candidate.Artifacts = append([]readmodelcore.Artifact(nil), previous.Artifacts...)
	candidate.Artifacts[0].Name = "report-v2.md"
	candidate.Patches = append([]readmodelcore.Patch(nil), previous.Patches...)
	candidate.Patches[0].State = "applied"
	candidate.MemoryRecallTraces = append([]readmodelcore.MemoryRecallTrace(nil), previous.MemoryRecallTraces...)
	candidate.MemoryRecallTraces[0].Status = "none_selected"
	candidate.LLMUsageTraces = append([]readmodelcore.LLMUsageTrace(nil), previous.LLMUsageTraces...)
	candidate.LLMUsageTraces[0].TotalTokens = 42

	sections := DiffWorkspaceProjectionSections(previous, candidate, "detail")
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
	unchanged := DiffWorkspaceProjectionSections(previous, previous, "detail")
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
	previous := readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{
			ID: "ws-diff-full", Title: "diff", State: "active", ActiveRunID: "run-1", BlockingTicketID: "ticket-1",
		},
		Decisions: []readmodelcore.Decision{{
			ID: "ticket-1", WorkspaceID: "ws-diff-full", RunID: "run-1", State: "open",
			Question: "choose path?", Reason: "need choice",
			Context: mustPayload(map[string]any{"hint": "old"}),
		}},
		Approvals: []readmodelcore.Approval{{
			ID: "approval-1", WorkspaceID: "ws-diff-full", RunID: "run-1", State: "pending",
			Reason: "apply patch", Action: "apply",
			Context: mustPayload(map[string]any{"risk": "low"}),
		}},
		Results: []readmodelcore.Result{{
			ID: "result-1", WorkspaceID: "ws-diff-full", RunID: "run-1", State: "accepted", Summary: "ok",
			EvidenceRefs: []string{"ev-1"},
			SummaryDetail: &readmodelcore.ResultSummary{
				GoalOutcome: "done", ImplementationSummary: "impl-old", EvidenceRefs: []string{"ev-1"},
			},
		}},
		Artifacts: []readmodelcore.Artifact{{
			ID: "artifact-1", WorkspaceID: "ws-diff-full", RunID: "run-1", State: "ready",
			Name: "report.md", Summary: "report", URI: "file://report.md",
			Metadata: map[string]any{"version": "1"},
		}},
		Patches: []readmodelcore.Patch{{
			ID: "patch-1", WorkspaceID: "ws-diff-full", RunID: "run-1", State: "previewed",
			Summary: "edit", TargetRef: "main.go", RiskLevel: "low",
			AffectedFiles: []string{"main.go"}, RiskReasons: []string{"touch core"},
		}},
		MemoryRecallTraces: []readmodelcore.MemoryRecallTrace{{
			ID: "memory-1", WorkspaceID: "ws-diff-full", RunID: "run-1", Status: "selected",
			Policy: "semantic", Source: "vector",
			SelectedMemoryIDs: []string{"m1"}, RetrievedMemoryIDs: []string{"m1"},
		}},
		LLMUsageTraces: []readmodelcore.LLMUsageTrace{{
			ID: "llm-1", WorkspaceID: "ws-diff-full", RunID: "run-1", Status: "ok",
			Model: "m1", Provider: "p1", ProviderID: "pid-1", TotalTokens: 10, InputTokens: 6, OutputTokens: 4,
			CacheUsage: mustPayload(map[string]any{"read": 1}),
		}},
		LLMUsageSummary: readmodelcore.LLMUsageSummary{
			EntryTotal: 1, TotalTokens: 10, CacheReadTokens: 1,
			ProviderModelBreakdown: []readmodelcore.LLMUsageProviderModelBreakdown{{
				Provider: "p1", Model: "m1", EntryTotal: 1, TotalTokens: 10,
			}},
		},
		TaskProgress: readmodelcore.TaskProgress{
			State: "running", Summary: "same",
			RuntimeV2: &readmodelcore.RuntimeV2Progress{StateRevision: 1, EvidenceSequence: 1, CurrentFocusID: "f1"},
		},
	}
	candidate := previous
	candidate.Decisions = append([]readmodelcore.Decision(nil), previous.Decisions...)
	candidate.Decisions[0].Context = mustPayload(map[string]any{"hint": "new"})
	candidate.Approvals = append([]readmodelcore.Approval(nil), previous.Approvals...)
	candidate.Approvals[0].Context = mustPayload(map[string]any{"risk": "high"})
	candidate.Results = append([]readmodelcore.Result(nil), previous.Results...)
	candidate.Results[0].EvidenceRefs = []string{"ev-2"}
	candidate.Results[0].SummaryDetail = &readmodelcore.ResultSummary{
		GoalOutcome: "done", ImplementationSummary: "impl-new", EvidenceRefs: []string{"ev-2"},
	}
	candidate.Artifacts = append([]readmodelcore.Artifact(nil), previous.Artifacts...)
	candidate.Artifacts[0].Metadata = map[string]any{"version": "2"}
	candidate.Artifacts[0].Summary = "report v2"
	candidate.Patches = append([]readmodelcore.Patch(nil), previous.Patches...)
	candidate.Patches[0].RiskLevel = "high"
	candidate.Patches[0].AffectedFiles = []string{"main.go", "util.go"}
	candidate.MemoryRecallTraces = append([]readmodelcore.MemoryRecallTrace(nil), previous.MemoryRecallTraces...)
	candidate.MemoryRecallTraces[0].SelectedMemoryIDs = []string{"m1", "m2"}
	candidate.MemoryRecallTraces[0].RetrievedMemoryHits = []readmodelcore.MemoryRecallHit{{ID: "m2", Summary: "hit"}}
	candidate.LLMUsageTraces = append([]readmodelcore.LLMUsageTrace(nil), previous.LLMUsageTraces...)
	candidate.LLMUsageTraces[0].Provider = "p2"
	candidate.LLMUsageTraces[0].CacheUsage = mustPayload(map[string]any{"read": 3})
	candidate.LLMUsageSummary = previous.LLMUsageSummary
	candidate.LLMUsageSummary.CacheReadTokens = 3
	candidate.LLMUsageSummary.ProviderModelBreakdown = []readmodelcore.LLMUsageProviderModelBreakdown{{
		Provider: "p2", Model: "m1", EntryTotal: 1, TotalTokens: 10,
	}}

	sections := DiffWorkspaceProjectionSections(previous, candidate, "detail")
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

func DeriveChangedSections(changeKind string, snapshot readmodelcore.WorkspaceSnapshot) []string {
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
		if snapshot.TaskProgress.RuntimeV2 != nil {
			add("evidence")
		}
		if len(snapshot.Decisions) > 0 || len(snapshot.Approvals) > 0 ||
			strings.TrimSpace(snapshot.Workspace.BlockingTicketID) != "" ||
			strings.TrimSpace(snapshot.Workspace.BlockingApprovalID) != "" {
			add("governance")
		}
		if strings.TrimSpace(snapshot.Outcome.State) != "" || len(snapshot.Results) > 0 {
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
	base := readmodelcore.WorkspaceActivityFeed{
		WorkspaceID: "ws-1", DisplayRunID: "run-1",
		Items: []readmodelcore.WorkspaceActivityItem{{ID: "a", Summary: "s"}},
	}
	capped := base
	capped.Truncated = true
	if ActivityFeedContentEqual(base, capped) {
		t.Fatal("Truncated change must break content equality")
	}
	// Empty NextCursor on both still differs only by Truncated.
	capped.NextCursor = ""
	if ActivityFeedContentEqual(base, capped) {
		t.Fatal("Truncated=true with empty cursor must still differ")
	}
}
