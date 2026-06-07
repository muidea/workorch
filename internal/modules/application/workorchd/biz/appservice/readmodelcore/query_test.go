package readmodelcore

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestMemoryRecallTraceRepositoryQueryNormalizesFields(t *testing.T) {
	query := MemoryRecallTraceQuery{
		WorkspaceID: " ws-001 ",
		RunID:       " run-001 ",
		StepID:      " step-001 ",
		Stage:       " execution ",
		Source:      " recovery ",
		Status:      " selected ",
		Limit:       25,
	}

	got := MemoryRecallTraceRepositoryQuery(query)

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

func TestEventRecordRepositoryQueryNormalizesFields(t *testing.T) {
	query := EventRecordQuery{
		WorkspaceID:    " ws-001 ",
		RunID:          " run-001 ",
		Topic:          " /topic ",
		SubjectType:    " run ",
		SubjectID:      " subject-001 ",
		CursorAfterID:  " evt-001 ",
		OccurredAfter:  " 2026-01-01T00:00:00Z ",
		OccurredBefore: " 2026-01-02T00:00:00Z ",
		Limit:          50,
	}

	got := EventRecordRepositoryQuery(query)

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
	if got.OccurredAfter != "2026-01-01T00:00:00Z" || got.OccurredBefore != "2026-01-02T00:00:00Z" {
		t.Fatalf("time filters mismatch: after=%q before=%q", got.OccurredAfter, got.OccurredBefore)
	}
	if got.Limit != 50 {
		t.Fatalf("limit mismatch: %d", got.Limit)
	}
}

func TestRunEventRecordRepositoryQueries(t *testing.T) {
	got := RunEventRecordRepositoryQuery(common.RunID(" run-001 "), 100)
	if got.RunID != common.RunID("run-001") {
		t.Fatalf("run id mismatch: %q", got.RunID)
	}
	if got.Topic != "" {
		t.Fatalf("topic mismatch: %q", got.Topic)
	}
	if got.Limit != 100 {
		t.Fatalf("limit mismatch: %d", got.Limit)
	}

	llm := RunLLMUsageEventRecordRepositoryQuery(common.RunID(" run-001 "), 5000)
	if llm.RunID != common.RunID("run-001") {
		t.Fatalf("llm run id mismatch: %q", llm.RunID)
	}
	if llm.Topic != events.TopicRunLLMUsageRecorded {
		t.Fatalf("llm topic mismatch: %q", llm.Topic)
	}
	if llm.Limit != 5000 {
		t.Fatalf("llm limit mismatch: %d", llm.Limit)
	}
}

func TestWorkspaceEventRecordRepositoryQueries(t *testing.T) {
	got := WorkspaceEventRecordRepositoryQuery(common.WorkspaceID(" ws-001 "), 100)
	if got.WorkspaceID != common.WorkspaceID("ws-001") {
		t.Fatalf("workspace id mismatch: %q", got.WorkspaceID)
	}
	if got.Topic != "" {
		t.Fatalf("topic mismatch: %q", got.Topic)
	}
	if got.Limit != 100 {
		t.Fatalf("limit mismatch: %d", got.Limit)
	}

	llm := WorkspaceLLMUsageEventRecordRepositoryQuery(common.WorkspaceID(" ws-001 "), 5000)
	if llm.WorkspaceID != common.WorkspaceID("ws-001") {
		t.Fatalf("llm workspace id mismatch: %q", llm.WorkspaceID)
	}
	if llm.Topic != events.TopicRunLLMUsageRecorded {
		t.Fatalf("llm topic mismatch: %q", llm.Topic)
	}
	if llm.Limit != 5000 {
		t.Fatalf("llm limit mismatch: %d", llm.Limit)
	}
}

func TestRunMemoryRecallTraceRepositoryQuery(t *testing.T) {
	got := RunMemoryRecallTraceRepositoryQuery(common.WorkspaceID(" ws-001 "), common.RunID(" run-001 "))
	if got.WorkspaceID != common.WorkspaceID("ws-001") {
		t.Fatalf("workspace id mismatch: %q", got.WorkspaceID)
	}
	if got.RunID != common.RunID("run-001") {
		t.Fatalf("run id mismatch: %q", got.RunID)
	}
}
