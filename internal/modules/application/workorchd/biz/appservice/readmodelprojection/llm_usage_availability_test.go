package readmodelprojection

import (
	"slices"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestLLMUsageQueryAvailability(t *testing.T) {
	for _, test := range []struct {
		name    string
		loaded  bool
		limited bool
		records []projectioncontract.EventRecord
		want    string
	}{
		{name: "not_loaded", want: "not_loaded"},
		{name: "queried_empty", loaded: true, want: "complete"},
		{name: "query_limit_reached", loaded: true, limited: true, want: "partial"},
		{name: "nil_owner_record", loaded: true, records: []projectioncontract.EventRecord{{}}, want: "partial"},
		{name: "payload_missing", loaded: true, records: []projectioncontract.EventRecord{{ID: "usage", Topic: "/llm/usage/recorded"}}, want: "partial"},
	} {
		t.Run(test.name, func(t *testing.T) {
			summary := summarizeQueriedLLMUsage(nil, len(test.records), test.loaded, test.limited, 200, "workspace")
			if summary.Availability != test.want || summary.Scope != "workspace" || summary.RecordLimit != 200 || summary.ObservedRecords != len(test.records) || summary.EntryTotal != 0 {
				t.Fatalf("summary=%+v", summary)
			}
		})
	}
}

func TestLLMUsageQueryBoundCannotBecomeCompleteWithoutLimitedFlag(t *testing.T) {
	const limit = 200
	traces := make([]projectioncontract.LLMUsageTrace, limit)
	summary := summarizeQueriedLLMUsage(traces, limit, true, false, limit, "workspace")
	if summary.Availability != "partial" || summary.ObservedRecords != limit || summary.RecordLimit != limit {
		t.Fatalf("触及上限的查询不能因 limited 标志缺失而变完整: %+v", summary)
	}
	summary = summarizeQueriedLLMUsage(traces[:limit-1], limit-1, true, false, limit, "workspace")
	if summary.Availability != "complete" || summary.ObservedRecords != limit-1 {
		t.Fatalf("上限内全部可解析的窗口应为 complete: %+v", summary)
	}
}

func TestLLMUsageAvailabilityParticipatesInProjectionDiff(t *testing.T) {
	left := projectioncontract.LLMUsageSummary{Availability: "not_loaded", Scope: "workspace", RecordLimit: 200}
	right := left
	right.Availability = "complete"
	if llmUsageSummaryEqual(left, right) {
		t.Fatal("加载状态变化被零 token 汇总隐藏")
	}
	previous := readmodelcore.WorkspaceSnapshotState{Workspace: appmodelcore.Workspace{ID: "ws"}, LLMUsageSummary: left}
	candidate := previous
	candidate.LLMUsageSummary = right
	sections := diffWorkspaceProjectionSections(previous, candidate, "")
	if !slices.Contains(sections, "llm_usage") {
		t.Fatalf("加载状态变化未通知: %v", sections)
	}
	previous = readmodelcore.WorkspaceSnapshotState{}
	sections = diffWorkspaceProjectionSections(previous, candidate, "")
	if !slices.Contains(sections, "llm_usage") {
		t.Fatalf("初次已加载空窗口遗漏 usage section: %v", sections)
	}
	clone := projectioncontract.CloneLLMUsageSummary(right)
	if !llmUsageSummaryEqual(right, clone) {
		t.Fatal("clone 丢失查询范围")
	}
}

func TestLLMUsageUnprojectableOwnerRecordRemainsPartial(t *testing.T) {
	got := workspaceSnapshotFromModels(WorkspaceSnapshotModelInput{
		Workspace:            &workspacemodel.Workspace{ID: "ws"},
		LLMUsageLoaded:       true,
		LLMUsageEventRecords: []*eventstreammodel.EventRecord{nil},
	})
	if got.LLMUsageSummary.Availability != "partial" || got.LLMUsageSummary.ObservedRecords != 1 || got.LLMUsageSummary.EntryTotal != 0 {
		t.Fatalf("不可投影的返回值被冒充为完整空查询: %+v", got.LLMUsageSummary)
	}
}

func TestWorkspaceUsageBoundedUnionStaysPartial(t *testing.T) {
	workspace := projectioncontract.LLMUsageSummary{Availability: "complete", Scope: "workspace", EntryTotal: 1, RecordLimit: 200, ObservedRecords: 1}
	run := projectioncontract.LLMUsageSummary{Availability: "complete", Scope: "run", EntryTotal: 1, RecordLimit: 200, ObservedRecords: 1}
	merged := mergeWorkspaceLLMUsageSummary([]projectioncontract.LLMUsageTrace{{ID: "authoring"}, {ID: "run"}}, workspace, run)
	if merged.Availability != "partial" || merged.ObservedRecords != 1 || merged.EntryTotal != 2 {
		t.Fatalf("两个不同范围不能冒完整 Workspace 查询: %+v", merged)
	}
}

func TestRunOnlyUsageDoesNotCompleteWorkspaceQuery(t *testing.T) {
	workspace := projectioncontract.LLMUsageSummary{Availability: "not_loaded", Scope: "workspace", RecordLimit: 200}
	run := projectioncontract.LLMUsageSummary{Availability: "complete", Scope: "run", RecordLimit: 200}
	merged := mergeWorkspaceLLMUsageSummary(nil, workspace, run)
	if merged.Availability != "partial" || merged.Scope != "workspace" {
		t.Fatalf("Run 查询冒充包含创建期的 Workspace 完整查询: %+v", merged)
	}
}
