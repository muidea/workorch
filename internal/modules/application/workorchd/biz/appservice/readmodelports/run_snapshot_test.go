package readmodelports

import (
	"context"
	"strconv"
	"sync"
	"testing"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRunSnapshotBuildsSnapshotFromReaders(t *testing.T) {
	t.Parallel()

	workspace := &workspacemodel.Workspace{ID: "ws-run-snapshot", Title: "run snapshot", State: common.WorkspaceStateActive}
	run := &runmodel.Run{ID: "run-snapshot", WorkspaceID: workspace.ID, State: common.RunStateActive}
	steps := []*runmodel.Step{{ID: "step-snapshot", RunID: run.ID, Kind: "llm/workspace_execution", State: common.StepStateExecuting}}
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
		got.Run.CurrentStepState != string(common.StepStateExecuting) {
		t.Fatalf("snapshot run current step = %+v, want %s/llm/workspace_execution/dispatched", got.Run, steps[0].ID)
	}
	if got.TaskProgress.CurrentStepID != got.Run.CurrentStepID {
		t.Fatalf("task progress current step = %q, want snapshot run current step %q", got.TaskProgress.CurrentStepID, got.Run.CurrentStepID)
	}
	if status := RunStatusFromSnapshot(got); status.CurrentStepID != got.Run.CurrentStepID {
		t.Fatalf("status current step = %q, want snapshot run current step %q", status.CurrentStepID, got.Run.CurrentStepID)
	}
	// RVC-14: compact snapshot issues metadata event timeline + topic-scoped usage payload query.
	if events.calls != 2 {
		t.Fatalf("event reader calls = %d, want compact metadata + llm usage queries", events.calls)
	}
	if len(events.queries) != 2 {
		t.Fatalf("event reader queries = %d, want compact metadata + llm usage queries", len(events.queries))
	}
	var sawMetadata, sawUsage bool
	for _, query := range events.queries {
		if query.Topic == "/llm/usage/recorded" {
			sawUsage = true
			if !query.IncludePayload {
				t.Fatalf("usage query = %+v, want IncludePayload", query)
			}
			continue
		}
		sawMetadata = true
		if query.IncludePayload {
			t.Fatalf("metadata event query = %+v, want metadata-only", query)
		}
		if len(query.ExcludeTopics) != 1 || query.ExcludeTopics[0] != "/run/llm_stream/progress" {
			t.Fatalf("snapshot event exclude topics = %+v, want stream progress excluded", query.ExcludeTopics)
		}
	}
	if !sawMetadata || !sawUsage {
		t.Fatalf("queries = %+v, want both metadata timeline and llm usage payload query", events.queries)
	}
}

func TestRunObservabilityHydratesLLMUsagePayloads(t *testing.T) {
	t.Parallel()

	workspace := &workspacemodel.Workspace{ID: "ws-run-observability-usage", Title: "run observability usage", State: common.WorkspaceStateActive}
	run := &runmodel.Run{ID: "run-observability-usage", WorkspaceID: workspace.ID, State: common.RunStateActive}
	usage := &eventstreammodel.EventRecord{
		ID:          "usage-run-observability",
		Topic:       "/llm/usage/recorded",
		WorkspaceID: workspace.ID,
		RunID:       run.ID,
		Payload: mustPortEventPayload(t, map[string]any{
			recordpayload.PayloadKeyWorkspaceID:          string(workspace.ID),
			recordpayload.PayloadKeyRunID:                string(run.ID),
			recordpayload.PayloadKeyStepID:               "run-observability-usage/step-usage",
			recordpayload.PayloadKeyProviderID:           "provider-observability",
			recordpayload.PayloadKeyModel:                "model-observability",
			recordpayload.PayloadKeyOperation:            "responses",
			recordpayload.PayloadKeyInputTokens:          30,
			recordpayload.PayloadKeyOutputTokens:         14,
			recordpayload.PayloadKeyTotalTokens:          44,
			recordpayload.PayloadKeyPhysicalResponseMode: string(llmaudit.PhysicalResponseModeNonStreaming),
			recordpayload.PayloadKeyProviderSubmitted:    true,
			recordpayload.PayloadKeyStatus:               "completed",
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
	if got.LLMUsageSummary.Availability != "complete" || got.LLMUsageSummary.Scope != "run" || got.LLMUsageSummary.ObservedRecords != 1 {
		t.Fatalf("usage query availability = %+v", got.LLMUsageSummary)
	}
	var usageQuerySeen bool
	for _, query := range eventReader.queries {
		if query.Topic == "/llm/usage/recorded" && query.IncludePayload {
			usageQuerySeen = true
		}
	}
	if !usageQuerySeen {
		t.Fatalf("event queries = %+v, want explicit llm usage payload query", eventReader.queries)
	}
}

func TestRunSnapshotViewCompactBoundsStepsAndProjectsStepPayloads(t *testing.T) {
	t.Parallel()

	workspace := &workspacemodel.Workspace{ID: "ws-run-view-compact", Title: "run view compact", State: common.WorkspaceStateActive}
	run := &runmodel.Run{ID: "run-view-compact", WorkspaceID: workspace.ID, State: common.RunStateFailed}
	steps := make([]*runmodel.Step, 0, runSnapshotViewCompactStepLimit+5)
	for idx := 0; idx < runSnapshotViewCompactStepLimit+5; idx++ {
		steps = append(steps, &runmodel.Step{
			ID:    common.StepID("step-view-compact-" + string(rune('a'+idx%26)) + "-" + strconv.Itoa(idx)),
			RunID: run.ID,
			Kind:  "llm/workspace_execution",
			State: common.StepStateSucceeded,
			Input: mustPortEventPayload(t, map[string]any{"large": "input"}),
			Output: mustPortEventPayload(t, map[string]any{
				"stage":     "llm_execution_graph_plan",
				"rawOutput": "large output",
			}),
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

func (fakeRunSnapshotToolCallReader) ListByRun(context.Context, common.RunID) ([]*runtimeevidencemodel.ToolCall, error) {
	return nil, nil
}

type fakeRunSnapshotPatchReader struct{}

func (fakeRunSnapshotPatchReader) ListByRun(context.Context, common.RunID) ([]*runtimeevidencemodel.Patch, error) {
	return nil, nil
}

type fakeRunSnapshotValidationReader struct{}

func (fakeRunSnapshotValidationReader) ListByRun(context.Context, common.RunID) ([]*runtimeevidencemodel.ValidationResult, error) {
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
	mu      sync.Mutex
	calls   int
	queries []workorchquerycontract.EventRecordQuery
	records []*eventstreammodel.EventRecord
}

func (f *fakeRunSnapshotEventReader) List(_ context.Context, query workorchquerycontract.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	f.mu.Lock()
	f.calls++
	f.queries = append(f.queries, query)
	f.mu.Unlock()
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

func (fakeRunSnapshotMemoryRecallReader) List(context.Context, workorchquerycontract.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	return nil, nil
}

type fakeRunSnapshotMCPCapabilityReader struct{}

func (fakeRunSnapshotMCPCapabilityReader) List(context.Context) ([]*mcpmodel.Capability, error) {
	return nil, nil
}
