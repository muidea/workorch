package readmodelports

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func TestWorkspaceSnapshotBuildsSnapshotFromReaders(t *testing.T) {
	t.Parallel()

	workspace := &workspacemodel.Workspace{ID: "ws-workspace-snapshot", Title: "workspace snapshot", State: common.WorkspaceStateActive}
	run := &runmodel.Run{ID: "run-workspace-snapshot", WorkspaceID: workspace.ID, State: common.RunStateCompleted}
	events := &fakeRunSnapshotEventReader{}

	got, err := WorkspaceSnapshot(context.Background(), WorkspaceSnapshotReaders{
		Workspaces:            fakeRunSnapshotWorkspaceReader{items: []*workspacemodel.Workspace{workspace}},
		Runs:                  fakeRunStatusRunReader{items: []*runmodel.Run{run}},
		Results:               fakeWorkspaceSnapshotResultReader{},
		Artifacts:             fakeRunSnapshotArtifactReader{},
		ArtifactContinuations: fakeRunSnapshotArtifactContinuationReader{},
		EventRecords:          events,
		Patches:               fakeRunSnapshotPatchReader{},
		Tickets:               fakeTicketReader{},
		Resolutions:           fakeResolutionReader{},
		Approvals:             fakeApprovalListReader{},
	}, workspace.ID, nil)
	if err != nil {
		t.Fatalf("WorkspaceSnapshot: %v", err)
	}
	if got.Workspace.ID != string(workspace.ID) {
		t.Fatalf("workspace ID = %q, want %q", got.Workspace.ID, workspace.ID)
	}
	if len(got.Runs) != 1 || got.Runs[0].ID != string(run.ID) {
		t.Fatalf("runs = %+v, want seeded run", got.Runs)
	}
	if events.calls != 2 {
		t.Fatalf("event reader calls = %d, want workspace events and workspace llm usage queries", events.calls)
	}
}

func TestWorkspaceSnapshotMergesRunScopedLLMUsageEvents(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-workspace-run-usage")
	firstRunID := common.RunID("run-workspace-run-usage-native")
	secondRunID := common.RunID("run-workspace-run-usage-estimated")
	reader := &workspaceEventReaderByQuery{
		workspaceRecords: []*eventstreammodel.EventRecord{
			llmUsageEventRecord("usage-native", workspaceID, firstRunID, "provider-native", "model-native", false),
		},
		runRecords: map[common.RunID][]*eventstreammodel.EventRecord{
			firstRunID: {
				llmUsageEventRecord("usage-native", workspaceID, firstRunID, "provider-native", "model-native", false),
			},
			secondRunID: {
				llmUsageEventRecord("usage-estimated", workspaceID, secondRunID, "provider-estimated", "model-estimated", true),
			},
		},
	}
	cached := &readmodelcore.RunSnapshot{
		Workspace: appmodelcore.Workspace{
			ID:          string(workspaceID),
			Title:       "workspace run usage",
			State:       string(common.WorkspaceStateRunning),
			ActiveRunID: string(secondRunID),
		},
		Run: appmodelcore.Run{
			ID:          string(secondRunID),
			WorkspaceID: string(workspaceID),
			State:       string(common.RunStateRunning),
		},
	}

	got, err := WorkspaceSnapshot(context.Background(), WorkspaceSnapshotReaders{
		Workspaces: fakeRunSnapshotWorkspaceReader{items: []*workspacemodel.Workspace{{
			ID:          workspaceID,
			Title:       "workspace run usage",
			State:       common.WorkspaceStateRunning,
			ActiveRunID: secondRunID,
		}}},
		Runs: fakeRunStatusRunReader{items: []*runmodel.Run{
			{ID: firstRunID, WorkspaceID: workspaceID, State: common.RunStateCompleted},
			{ID: secondRunID, WorkspaceID: workspaceID, State: common.RunStateRunning},
		}},
		Results:               fakeWorkspaceSnapshotResultReader{},
		Artifacts:             fakeRunSnapshotArtifactReader{},
		ArtifactContinuations: fakeRunSnapshotArtifactContinuationReader{},
		EventRecords:          reader,
		Patches:               fakeRunSnapshotPatchReader{},
		Tickets:               fakeTicketReader{},
		Resolutions:           fakeResolutionReader{},
		Approvals:             fakeApprovalListReader{},
	}, workspaceID, cached)
	if err != nil {
		t.Fatalf("WorkspaceSnapshot: %v", err)
	}
	if got.LLMUsageSummary.EntryTotal != 2 || got.LLMUsageSummary.EstimatedEntries != 1 {
		t.Fatalf("llm usage summary = %+v, want two unique entries with one estimated", got.LLMUsageSummary)
	}
	if len(got.LLMUsageSummary.ProviderModelBreakdown) != 2 {
		t.Fatalf("provider/model breakdown = %+v, want native and estimated providers", got.LLMUsageSummary.ProviderModelBreakdown)
	}
	if !llmUsageBreakdownContains(got.LLMUsageSummary.ProviderModelBreakdown, "provider-native", "model-native") ||
		!llmUsageBreakdownContains(got.LLMUsageSummary.ProviderModelBreakdown, "provider-estimated", "model-estimated") {
		t.Fatalf("provider/model breakdown = %+v, want native and estimated providers", got.LLMUsageSummary.ProviderModelBreakdown)
	}
}

func TestWorkspaceSnapshotUsesCachedRunSnapshotForProjectionRun(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-workspace-cached-run")
	runID := common.RunID("run-workspace-cached")
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	cached := &readmodelcore.RunSnapshot{
		Workspace: appmodelcore.Workspace{
			ID:          string(workspaceID),
			Title:       "cached workspace",
			State:       string(common.WorkspaceStateRunning),
			ActiveRunID: string(runID),
		},
		Run: appmodelcore.Run{
			ID:          string(runID),
			WorkspaceID: string(workspaceID),
			State:       string(common.RunStateRunning),
		},
		TaskProgress: readmodelcore.TaskProgress{
			RunID:   string(runID),
			Summary: "cached task progress",
		},
	}

	got, err := WorkspaceSnapshot(context.Background(), WorkspaceSnapshotReaders{
		Workspaces:            failGetWorkspaceReader{},
		Runs:                  fakeRunStatusRunReader{items: []*runmodel.Run{run}},
		Results:               fakeWorkspaceSnapshotResultReader{},
		Artifacts:             fakeRunSnapshotArtifactReader{},
		ArtifactContinuations: fakeRunSnapshotArtifactContinuationReader{},
		EventRecords:          &fakeRunSnapshotEventReader{},
		Patches:               fakeRunSnapshotPatchReader{},
		Tickets:               fakeTicketReader{},
		Resolutions:           fakeResolutionReader{},
		Approvals:             fakeApprovalListReader{},
	}, workspaceID, cached)
	if err != nil {
		t.Fatalf("WorkspaceSnapshot: %v", err)
	}
	if got.Run == nil || got.Run.ID != string(runID) {
		t.Fatalf("projected run = %+v, want cached run", got.Run)
	}
	if got.TaskProgress.Summary != "cached task progress" {
		t.Fatalf("task progress summary = %q, want cached task progress", got.TaskProgress.Summary)
	}
}

func TestWorkspaceSnapshotLoadsRunScopedLLMUsageOnlyForProjectionRun(t *testing.T) {
	t.Parallel()

	workspaceID := common.WorkspaceID("ws-workspace-usage-projection-run")
	historicalRunID := common.RunID("run-workspace-usage-history")
	activeRunID := common.RunID("run-workspace-usage-active")
	reader := &recordingWorkspaceEventReader{}
	cached := &readmodelcore.RunSnapshot{
		Workspace: appmodelcore.Workspace{
			ID:          string(workspaceID),
			Title:       "cached workspace",
			State:       string(common.WorkspaceStateRunning),
			ActiveRunID: string(activeRunID),
		},
		Run: appmodelcore.Run{
			ID:          string(activeRunID),
			WorkspaceID: string(workspaceID),
			State:       string(common.RunStateRunning),
		},
	}

	_, err := WorkspaceSnapshot(context.Background(), WorkspaceSnapshotReaders{
		Workspaces: fakeRunSnapshotWorkspaceReader{items: []*workspacemodel.Workspace{{
			ID:          workspaceID,
			Title:       "workspace usage projection run",
			State:       common.WorkspaceStateRunning,
			ActiveRunID: activeRunID,
		}}},
		Runs: fakeRunStatusRunReader{items: []*runmodel.Run{
			{ID: historicalRunID, WorkspaceID: workspaceID, State: common.RunStateCompleted},
			{ID: activeRunID, WorkspaceID: workspaceID, State: common.RunStateRunning},
		}},
		Results:               fakeWorkspaceSnapshotResultReader{},
		Artifacts:             fakeRunSnapshotArtifactReader{},
		ArtifactContinuations: fakeRunSnapshotArtifactContinuationReader{},
		EventRecords:          reader,
		Patches:               fakeRunSnapshotPatchReader{},
		Tickets:               fakeTicketReader{},
		Resolutions:           fakeResolutionReader{},
		Approvals:             fakeApprovalListReader{},
	}, workspaceID, cached)
	if err != nil {
		t.Fatalf("WorkspaceSnapshot: %v", err)
	}

	var runScoped []common.RunID
	for _, query := range reader.queries {
		if query.RunID != "" && query.Topic == runevents.TopicRunLLMUsageRecorded {
			runScoped = append(runScoped, query.RunID)
		}
	}
	if len(runScoped) != 1 || runScoped[0] != activeRunID {
		t.Fatalf("run-scoped llm usage queries = %+v, want only active projection run %s", runScoped, activeRunID)
	}
}

type failGetWorkspaceReader struct{}

func (failGetWorkspaceReader) Get(context.Context, common.WorkspaceID) (*workspacemodel.Workspace, error) {
	return nil, common.ErrNotFound
}

func (failGetWorkspaceReader) List(context.Context) ([]*workspacemodel.Workspace, error) {
	return nil, nil
}

type fakeWorkspaceSnapshotResultReader struct {
	items []*resultmodel.Result
}

func (f fakeWorkspaceSnapshotResultReader) ListByWorkspace(_ context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	ret := make([]*resultmodel.Result, 0, len(f.items))
	for _, item := range f.items {
		if item != nil && item.WorkspaceID == workspaceID {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

func (f fakeWorkspaceSnapshotResultReader) ListByRun(_ context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	ret := make([]*resultmodel.Result, 0, len(f.items))
	for _, item := range f.items {
		if item != nil && item.RunID == runID {
			ret = append(ret, item)
		}
	}
	return ret, nil
}

type recordingWorkspaceEventReader struct {
	queries []repository.EventRecordQuery
}

func (r *recordingWorkspaceEventReader) List(_ context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	r.queries = append(r.queries, query)
	return nil, nil
}

type workspaceEventReaderByQuery struct {
	workspaceRecords []*eventstreammodel.EventRecord
	runRecords       map[common.RunID][]*eventstreammodel.EventRecord
}

func (r *workspaceEventReaderByQuery) List(_ context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	if query.RunID != "" {
		return r.runRecords[query.RunID], nil
	}
	if query.WorkspaceID != "" && query.Topic == runevents.TopicRunLLMUsageRecorded {
		return r.workspaceRecords, nil
	}
	return nil, nil
}

func llmUsageEventRecord(id string, workspaceID common.WorkspaceID, runID common.RunID, providerID string, model string, estimated bool) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		ID:          common.EventRecordID(id),
		Version:     "workorch.event.v1",
		Topic:       runevents.TopicRunLLMUsageRecorded,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID(string(runID) + "/step-1"),
		Payload: map[string]any{
			"workspaceId":  string(workspaceID),
			"runId":        string(runID),
			"stepId":       string(runID) + "/step-1",
			"providerId":   providerID,
			"provider":     "compatible",
			"model":        model,
			"operation":    "chat_completions",
			"inputTokens":  10,
			"outputTokens": 5,
			"totalTokens":  15,
			"estimated":    estimated,
			"usageSource":  "provider_native",
			"status":       "completed",
		},
	}
}

func llmUsageBreakdownContains(items []readmodelcore.LLMUsageProviderModelBreakdown, providerID string, model string) bool {
	for _, item := range items {
		if item.ProviderID == providerID && item.Model == model && item.EntryTotal == 1 {
			return true
		}
	}
	return false
}
