package readmodelports

import (
	"context"
	"strconv"
	"testing"

	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRunSnapshotBuildsSnapshotFromReaders(t *testing.T) {
	t.Parallel()

	workspace := &workspacemodel.Workspace{ID: "ws-run-snapshot", Title: "run snapshot", State: common.WorkspaceStateRunning}
	run := &runmodel.Run{ID: "run-snapshot", WorkspaceID: workspace.ID, State: common.RunStateRunning}
	steps := []*runmodel.Step{{ID: "step-snapshot", RunID: run.ID, Kind: "llm/workspace_execution", State: common.StepStateDispatched}}
	events := &fakeRunSnapshotEventReader{}

	got, err := RunSnapshot(context.Background(), RunSnapshotReaders{
		Workspaces:            fakeRunSnapshotWorkspaceReader{items: []*workspacemodel.Workspace{workspace}},
		Runs:                  fakeRunStatusRunReader{items: []*runmodel.Run{run}},
		Steps:                 fakeRunStatusStepReader{items: steps},
		ToolCalls:             fakeRunSnapshotToolCallReader{},
		Patches:               fakeRunSnapshotPatchReader{},
		Validations:           fakeRunSnapshotValidationReader{},
		Checkpoints:           fakeRunStatusCheckpointReader{},
		Results:               fakeRunSnapshotResultReader{},
		Artifacts:             fakeRunSnapshotArtifactReader{},
		ArtifactContinuations: fakeRunSnapshotArtifactContinuationReader{},
		EventRecords:          events,
		MemoryRecalls:         fakeRunSnapshotMemoryRecallReader{},
		Tickets:               fakeTicketReader{},
		Resolutions:           fakeResolutionReader{},
		Approvals:             fakeApprovalListReader{},
		MCPCapabilities:       fakeRunSnapshotMCPCapabilityReader{},
	}, run.ID)
	if err != nil {
		t.Fatalf("RunSnapshot: %v", err)
	}
	if got.Run.ID != string(run.ID) || got.Workspace.ID != string(workspace.ID) {
		t.Fatalf("snapshot identity = run:%s workspace:%s, want seeded IDs", got.Run.ID, got.Workspace.ID)
	}
	if len(got.Steps) != 1 || got.Steps[0].ID != string(steps[0].ID) {
		t.Fatalf("steps = %+v, want seeded step", got.Steps)
	}
	if got.Step == nil || got.Step.ID != string(steps[0].ID) {
		t.Fatalf("current step = %+v, want seeded step %s", got.Step, steps[0].ID)
	}
	if got.Run.CurrentStepID != string(steps[0].ID) ||
		got.Run.CurrentStepKind != "llm/workspace_execution" ||
		got.Run.CurrentStepState != string(common.StepStateDispatched) {
		t.Fatalf("snapshot run current step = %+v, want %s/llm/workspace_execution/dispatched", got.Run, steps[0].ID)
	}
	if got.TaskProgress.CurrentStepID != got.Run.CurrentStepID {
		t.Fatalf("task progress current step = %q, want snapshot run current step %q", got.TaskProgress.CurrentStepID, got.Run.CurrentStepID)
	}
	if status := RunStatusFromSnapshot(got); status.CurrentStepID != got.Run.CurrentStepID {
		t.Fatalf("status current step = %q, want snapshot run current step %q", status.CurrentStepID, got.Run.CurrentStepID)
	}
	if events.calls != 1 {
		t.Fatalf("event reader calls = %d, want compact run event metadata query", events.calls)
	}
	if len(events.queries) != 1 {
		t.Fatalf("event reader queries = %d, want compact run event metadata query", len(events.queries))
	}
	if len(events.queries[0].ExcludeTopics) != 1 || events.queries[0].ExcludeTopics[0] != runevents.TopicRunLLMStreamProgress {
		t.Fatalf("snapshot event exclude topics = %+v, want stream progress excluded", events.queries[0].ExcludeTopics)
	}
}

func TestRunObservabilityHydratesLLMUsagePayloads(t *testing.T) {
	t.Parallel()

	workspace := &workspacemodel.Workspace{ID: "ws-run-observability-usage", Title: "run observability usage", State: common.WorkspaceStateRunning}
	run := &runmodel.Run{ID: "run-observability-usage", WorkspaceID: workspace.ID, State: common.RunStateRunning}
	usage := &eventstreammodel.EventRecord{
		ID:          "usage-run-observability",
		Topic:       runevents.TopicRunLLMUsageRecorded,
		WorkspaceID: workspace.ID,
		RunID:       run.ID,
		Payload: events.LLMUsagePayload(events.LLMUsagePayloadData{
			WorkspaceID:  workspace.ID,
			RunID:        run.ID,
			StepID:       "run-observability-usage/step-usage",
			ProviderID:   "provider-observability",
			Model:        "model-observability",
			Operation:    "chat_completions",
			InputTokens:  30,
			OutputTokens: 14,
			TotalTokens:  44,
			Status:       "completed",
		}),
	}
	eventReader := &fakeRunSnapshotEventReader{records: []*eventstreammodel.EventRecord{usage}}

	got, err := RunObservability(context.Background(), RunSnapshotReaders{
		Workspaces:            fakeRunSnapshotWorkspaceReader{items: []*workspacemodel.Workspace{workspace}},
		Runs:                  fakeRunStatusRunReader{items: []*runmodel.Run{run}},
		Steps:                 fakeRunStatusStepReader{},
		ToolCalls:             fakeRunSnapshotToolCallReader{},
		Patches:               fakeRunSnapshotPatchReader{},
		Validations:           fakeRunSnapshotValidationReader{},
		Checkpoints:           fakeRunStatusCheckpointReader{},
		Results:               fakeRunSnapshotResultReader{},
		Artifacts:             fakeRunSnapshotArtifactReader{},
		ArtifactContinuations: fakeRunSnapshotArtifactContinuationReader{},
		EventRecords:          eventReader,
		MemoryRecalls:         fakeRunSnapshotMemoryRecallReader{},
		Tickets:               fakeTicketReader{},
		Resolutions:           fakeResolutionReader{},
		Approvals:             fakeApprovalListReader{},
	}, run.ID)
	if err != nil {
		t.Fatalf("RunObservability: %v", err)
	}
	if got.LLMUsageSummary.EntryTotal != 1 || got.LLMUsageSummary.TotalTokens != 44 {
		t.Fatalf("llm usage summary = %+v, want one explicit usage payload", got.LLMUsageSummary)
	}
	var usageQuerySeen bool
	for _, query := range eventReader.queries {
		if query.Topic == runevents.TopicRunLLMUsageRecorded && query.IncludePayload {
			usageQuerySeen = true
		}
	}
	if !usageQuerySeen {
		t.Fatalf("event queries = %+v, want explicit llm usage payload query", eventReader.queries)
	}
}

func TestRunSnapshotViewCompactBoundsStepsAndProjectsStepPayloads(t *testing.T) {
	t.Parallel()

	workspace := &workspacemodel.Workspace{ID: "ws-run-view-compact", Title: "run view compact", State: common.WorkspaceStateRunning}
	run := &runmodel.Run{ID: "run-view-compact", WorkspaceID: workspace.ID, State: common.RunStateFailed}
	steps := make([]*runmodel.Step, 0, runSnapshotViewCompactStepLimit+5)
	for idx := 0; idx < runSnapshotViewCompactStepLimit+5; idx++ {
		steps = append(steps, &runmodel.Step{
			ID:    common.StepID("step-view-compact-" + string(rune('a'+idx%26)) + "-" + strconv.Itoa(idx)),
			RunID: run.ID,
			Kind:  "llm/workspace_execution",
			State: common.StepStateCompleted,
			Input: map[string]any{"large": "input"},
			Output: map[string]any{
				"stage":     "llm_execution_graph_plan",
				"rawOutput": "large output",
			},
		})
	}
	currentStepID := steps[1].ID
	steps[1].State = common.StepStateFailed

	got, err := runSnapshot(context.Background(), RunSnapshotReaders{
		Workspaces:            fakeRunSnapshotWorkspaceReader{items: []*workspacemodel.Workspace{workspace}},
		Runs:                  fakeRunStatusRunReader{items: []*runmodel.Run{run}},
		Steps:                 fakeRunStatusStepReader{items: steps},
		ToolCalls:             fakeRunSnapshotToolCallReader{},
		Patches:               fakeRunSnapshotPatchReader{},
		Validations:           fakeRunSnapshotValidationReader{},
		Checkpoints:           fakeRunStatusCheckpointReader{items: []*runmodel.RuntimeCheckpoint{{RunID: run.ID, CurrentStepID: currentStepID}}},
		Results:               fakeRunSnapshotResultReader{},
		Artifacts:             fakeRunSnapshotArtifactReader{},
		ArtifactContinuations: fakeRunSnapshotArtifactContinuationReader{},
		EventRecords:          &fakeRunSnapshotEventReader{},
		MemoryRecalls:         fakeRunSnapshotMemoryRecallReader{},
		Tickets:               fakeTicketReader{},
		Resolutions:           fakeResolutionReader{},
		Approvals:             fakeApprovalListReader{},
	}, run.ID, runSnapshotLoadViewCompact, false)
	if err != nil {
		t.Fatalf("runSnapshot view compact: %v", err)
	}
	if len(got.Steps) != runSnapshotViewCompactStepLimit {
		t.Fatalf("steps = %d, want bounded %d", len(got.Steps), runSnapshotViewCompactStepLimit)
	}
	if got.Step == nil || got.Step.ID != string(currentStepID) {
		t.Fatalf("current step = %+v, want checkpoint step %s retained", got.Step, currentStepID)
	}
	if got.Step.Input != nil || got.Step.Output["rawOutput"] != nil {
		t.Fatalf("current step payload = input:%+v output:%+v, want compact projected view", got.Step.Input, got.Step.Output)
	}
}

type fakeRunSnapshotWorkspaceReader struct {
	items []*workspacemodel.Workspace
}

func (f fakeRunSnapshotWorkspaceReader) Get(_ context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	for _, item := range f.items {
		if item != nil && item.ID == id {
			return item, nil
		}
	}
	return nil, common.ErrNotFound
}

func (f fakeRunSnapshotWorkspaceReader) List(context.Context) ([]*workspacemodel.Workspace, error) {
	return append([]*workspacemodel.Workspace(nil), f.items...), nil
}

type fakeRunSnapshotToolCallReader struct{}

func (fakeRunSnapshotToolCallReader) ListByRun(context.Context, common.RunID) ([]*runmodel.ToolCall, error) {
	return nil, nil
}

type fakeRunSnapshotPatchReader struct{}

func (fakeRunSnapshotPatchReader) ListByRun(context.Context, common.RunID) ([]*runmodel.Patch, error) {
	return nil, nil
}

type fakeRunSnapshotValidationReader struct{}

func (fakeRunSnapshotValidationReader) ListByRun(context.Context, common.RunID) ([]*runmodel.ValidationResult, error) {
	return nil, nil
}

type fakeRunSnapshotResultReader struct{}

func (fakeRunSnapshotResultReader) ListByWorkspace(context.Context, common.WorkspaceID) ([]*resultmodel.Result, error) {
	return nil, nil
}

func (fakeRunSnapshotResultReader) ListByRun(context.Context, common.RunID) ([]*resultmodel.Result, error) {
	return nil, nil
}

type fakeRunSnapshotArtifactReader struct{}

func (fakeRunSnapshotArtifactReader) ListByWorkspace(context.Context, common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	return nil, nil
}

func (fakeRunSnapshotArtifactReader) ListByRun(context.Context, common.RunID) ([]*resultmodel.Artifact, error) {
	return nil, nil
}

func (fakeRunSnapshotArtifactReader) ListByResult(context.Context, common.ResultID) ([]*resultmodel.Artifact, error) {
	return nil, nil
}

type fakeRunSnapshotArtifactContinuationReader struct{}

func (fakeRunSnapshotArtifactContinuationReader) ListByRun(context.Context, common.RunID) ([]*orchestratormodel.ArtifactContinuation, error) {
	return nil, nil
}

type fakeRunSnapshotEventReader struct {
	calls   int
	queries []repository.EventRecordQuery
	records []*eventstreammodel.EventRecord
}

func (f *fakeRunSnapshotEventReader) List(_ context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	f.calls++
	f.queries = append(f.queries, query)
	if len(f.records) == 0 {
		return nil, nil
	}
	ret := make([]*eventstreammodel.EventRecord, 0, len(f.records))
	excluded := map[string]bool{}
	for _, topic := range query.ExcludeTopics {
		excluded[topic] = true
	}
	for _, item := range f.records {
		if item == nil {
			continue
		}
		if query.RunID != "" && item.RunID != query.RunID {
			continue
		}
		if query.WorkspaceID != "" && item.WorkspaceID != query.WorkspaceID {
			continue
		}
		if query.Topic != "" && item.Topic != query.Topic {
			continue
		}
		if excluded[item.Topic] {
			continue
		}
		ret = append(ret, item)
	}
	return ret, nil
}

type fakeRunSnapshotMemoryRecallReader struct{}

func (fakeRunSnapshotMemoryRecallReader) List(context.Context, repository.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	return nil, nil
}

type fakeRunSnapshotMCPCapabilityReader struct{}

func (fakeRunSnapshotMCPCapabilityReader) List(context.Context) ([]*mcpmodel.Capability, error) {
	return nil, nil
}
