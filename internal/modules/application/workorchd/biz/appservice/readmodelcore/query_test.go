package readmodelcore

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func TestMemoryRecallTraceOwnerQueryNormalizesFields(t *testing.T) {
	query := workorchquerycontract.MemoryRecallTraceQuery{
		WorkspaceID: " ws-001 ",
		RunID:       " run-001 ",
		StepID:      " step-001 ",
		Stage:       " execution ",
		Source:      " recovery ",
		Status:      " selected ",
		Limit:       25,
	}

	got := MemoryRecallTraceOwnerQuery(query)

	if got.WorkspaceID != common.WorkspaceID("ws-001") {
		t.Fatalf("workspace id mismatch: %q", got.WorkspaceID)
	}
	if got.RunID != common.RunID("run-001") {
		t.Fatalf("run id mismatch: %q", got.RunID)
	}
	if got.StepID != common.StepID("step-001") {
		t.Fatalf("step id mismatch: %q", got.StepID)
	}
	if got.Stage != "execution" || got.Source != "recovery" || got.Status != "selected" {
		t.Fatalf("string filters mismatch: stage=%q source=%q status=%q", got.Stage, got.Source, got.Status)
	}
	if got.Limit != 25 {
		t.Fatalf("limit mismatch: %d", got.Limit)
	}
}

func TestEventRecordOwnerQueryNormalizesFields(t *testing.T) {
	query := workorchquerycontract.EventRecordQuery{
		WorkspaceID:    " ws-001 ",
		RunID:          " run-001 ",
		Topic:          " /topic ",
		ExcludeTopics:  []string{" /exclude-a ", " /exclude-a ", "", "/exclude-b"},
		SubjectType:    " run ",
		SubjectID:      " subject-001 ",
		CursorAfterID:  " evt-001 ",
		OccurredAfter:  " 2026-01-01T00:00:00Z ",
		OccurredBefore: " 2026-01-02T00:00:00Z ",
		Limit:          50,
		IncludePayload: true,
	}

	got := EventRecordOwnerQuery(query)

	if got.WorkspaceID != common.WorkspaceID("ws-001") {
		t.Fatalf("workspace id mismatch: %q", got.WorkspaceID)
	}
	if got.RunID != common.RunID("run-001") {
		t.Fatalf("run id mismatch: %q", got.RunID)
	}
	if got.CursorAfterID != common.EventRecordID("evt-001") {
		t.Fatalf("cursor id mismatch: %q", got.CursorAfterID)
	}
	if got.Topic != "/topic" || got.SubjectType != "run" || got.SubjectID != "subject-001" {
		t.Fatalf("event filters mismatch: topic=%q subjectType=%q subjectID=%q", got.Topic, got.SubjectType, got.SubjectID)
	}
	if len(got.ExcludeTopics) != 2 || got.ExcludeTopics[0] != "/exclude-a" || got.ExcludeTopics[1] != "/exclude-b" {
		t.Fatalf("exclude topics = %+v, want normalized unique topics", got.ExcludeTopics)
	}
	if got.OccurredAfter != "2026-01-01T00:00:00Z" || got.OccurredBefore != "2026-01-02T00:00:00Z" {
		t.Fatalf("time filters mismatch: after=%q before=%q", got.OccurredAfter, got.OccurredBefore)
	}
	if got.Limit != 50 {
		t.Fatalf("limit mismatch: %d", got.Limit)
	}
	if !got.IncludePayload {
		t.Fatalf("include payload mismatch: %v", got.IncludePayload)
	}
}

func TestRunEventRecordRepositoryQueries(t *testing.T) {
	got := RunEventRecordOwnerQuery(common.RunID(" run-001 "), 100)
	if got.RunID != common.RunID("run-001") {
		t.Fatalf("run id mismatch: %q", got.RunID)
	}
	if got.Topic != "" {
		t.Fatalf("topic mismatch: %q", got.Topic)
	}
	if got.Limit != 100 {
		t.Fatalf("limit mismatch: %d", got.Limit)
	}
	if got.IncludePayload {
		t.Fatalf("run event query include payload = true, want false")
	}

	observability := RunObservabilityEventRecordOwnerQuery(common.RunID(" run-001 "), 100)
	if observability.RunID != common.RunID("run-001") {
		t.Fatalf("observability run id mismatch: %q", observability.RunID)
	}
	if observability.Topic != "" {
		t.Fatalf("observability topic mismatch: %q", observability.Topic)
	}
	if observability.Limit != 100 {
		t.Fatalf("observability limit mismatch: %d", observability.Limit)
	}
	if !observability.IncludePayload {
		t.Fatalf("observability include payload = false, want true")
	}
	if len(observability.ExcludeTopics) != 0 {
		t.Fatalf("observability exclude topics = %+v, want complete event timeline", observability.ExcludeTopics)
	}

	projection := RunProjectionEventRecordOwnerQuery(common.RunID(" run-001 "), 100)
	if projection.RunID != common.RunID("run-001") || projection.IncludePayload {
		t.Fatalf("projection query = %+v, want run scoped compact projection query", projection)
	}
	if len(projection.ExcludeTopics) != 1 || projection.ExcludeTopics[0] != "/run/llm_stream/progress" {
		t.Fatalf("projection exclude topics = %+v, want stream progress excluded", projection.ExcludeTopics)
	}

	llm := RunLLMUsageEventRecordOwnerQuery(common.RunID(" run-001 "), 5000)
	if llm.RunID != common.RunID("run-001") {
		t.Fatalf("llm run id mismatch: %q", llm.RunID)
	}
	if llm.Topic != "/llm/usage/recorded" {
		t.Fatalf("llm topic mismatch: %q", llm.Topic)
	}
	if llm.Limit != 5000 {
		t.Fatalf("llm limit mismatch: %d", llm.Limit)
	}
	if !llm.IncludePayload {
		t.Fatalf("llm include payload = false, want true")
	}
}

func TestWorkspaceEventRecordRepositoryQueries(t *testing.T) {
	got := WorkspaceEventRecordOwnerQuery(common.WorkspaceID(" ws-001 "), 100)
	if got.WorkspaceID != common.WorkspaceID("ws-001") {
		t.Fatalf("workspace id mismatch: %q", got.WorkspaceID)
	}
	if got.Topic != "" {
		t.Fatalf("topic mismatch: %q", got.Topic)
	}
	if got.Limit != 100 {
		t.Fatalf("limit mismatch: %d", got.Limit)
	}
	if got.IncludePayload {
		t.Fatalf("workspace event query include payload = true, want false")
	}

	llm := WorkspaceLLMUsageEventRecordOwnerQuery(common.WorkspaceID(" ws-001 "), 5000)
	if llm.WorkspaceID != common.WorkspaceID("ws-001") {
		t.Fatalf("llm workspace id mismatch: %q", llm.WorkspaceID)
	}
	if llm.Topic != "/llm/usage/recorded" {
		t.Fatalf("llm topic mismatch: %q", llm.Topic)
	}
	if llm.Limit != 5000 {
		t.Fatalf("llm limit mismatch: %d", llm.Limit)
	}
	if !llm.IncludePayload {
		t.Fatalf("llm include payload = false, want true")
	}
}

func TestRunMemoryRecallTraceOwnerQuery(t *testing.T) {
	got := RunMemoryRecallTraceOwnerQuery(common.WorkspaceID(" ws-001 "), common.RunID(" run-001 "))
	if got.WorkspaceID != common.WorkspaceID("ws-001") {
		t.Fatalf("workspace id mismatch: %q", got.WorkspaceID)
	}
	if got.RunID != common.RunID("run-001") {
		t.Fatalf("run id mismatch: %q", got.RunID)
	}
}
