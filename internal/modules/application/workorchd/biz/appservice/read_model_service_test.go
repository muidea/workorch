package appservice

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	resultcommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type countingStepRepository struct {
	base           readmodelcore.StepReader
	listByRunCalls int
}

type eventRecordRepositoryTestReader struct {
	repository.EventRecordRepository
}

func (r eventRecordRepositoryTestReader) List(ctx context.Context, query eventstreamevents.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	return r.EventRecordRepository.List(ctx, repository.EventRecordQuery{WorkspaceID: query.WorkspaceID, RunID: query.RunID, Topic: query.Topic, ExcludeTopics: append([]string(nil), query.ExcludeTopics...), SubjectType: query.SubjectType, SubjectID: query.SubjectID, CursorAfterID: query.CursorAfterID, OccurredAfter: query.OccurredAfter, OccurredBefore: query.OccurredBefore, Limit: query.Limit, IncludePayload: query.IncludePayload})
}

type memoryRecallTraceRepositoryTestReader struct {
	repository.MemoryRecallTraceRepository
}

func (r memoryRecallTraceRepositoryTestReader) List(ctx context.Context, query memoryevents.MemoryRecallTraceQuery) ([]*memorymodel.MemoryRecallTrace, error) {
	return r.MemoryRecallTraceRepository.List(ctx, repository.MemoryRecallTraceQuery{WorkspaceID: query.WorkspaceID, RunID: query.RunID, StepID: query.StepID, Stage: query.Stage, Source: query.Source, Status: query.Status, Limit: query.Limit})
}

func (r *countingStepRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.Step, error) {
	r.listByRunCalls++
	return r.base.ListByRun(ctx, runID)
}

func TestProjectStepOutputKeepsMCPStructuredEvidence(t *testing.T) {
	output := readmodelprojection.ProjectStepOutput("mcp/tool_call", map[string]any{
		"operation": "mcp/tool_call",
		"serverId":  "mcp-stdio",
		"tool":      "repo.summary",
		"transport": "stdio",
		"status":    "completed",
		"result": map[string]any{
			"structuredContent": map[string]any{
				"transport": "stdio",
				"tool":      "repo.summary",
				"arguments": map[string]any{
					"path": ".",
				},
				"ok":          true,
				"largeNoise":  "should be removed",
				"nestedNoise": []any{"should be removed"},
			},
			"isError": false,
			"content": []any{
				map[string]any{"type": "text", "text": strings.Repeat("x", 4096)},
			},
		},
		"rawNoise": "should be removed",
	})

	expected := map[string]any{
		"operation": "mcp/tool_call",
		"serverId":  "mcp-stdio",
		"tool":      "repo.summary",
		"transport": "stdio",
		"status":    "completed",
		"result": map[string]any{
			"structuredContent": map[string]any{
				"transport": "stdio",
				"tool":      "repo.summary",
				"arguments": map[string]any{
					"path": ".",
				},
				"ok": true,
			},
			"isError": false,
		},
	}
	if !reflect.DeepEqual(output, expected) {
		t.Fatalf("projected output = %#v, want %#v", output, expected)
	}
}

type blockingWorkspaceRepository struct {
	base    repository.WorkspaceRepository
	started chan string
	release chan struct{}
}

func (r *blockingWorkspaceRepository) Create(ctx context.Context, item *workspacemodel.Workspace) error {
	return r.base.Create(ctx, item)
}

func (r *blockingWorkspaceRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	select {
	case r.started <- string(id):
	default:
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.release:
	}
	return r.base.Get(ctx, id)
}

func (r *blockingWorkspaceRepository) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	return r.base.List(ctx)
}

func (r *blockingWorkspaceRepository) Update(ctx context.Context, item *workspacemodel.Workspace) error {
	return r.base.Update(ctx, item)
}

func (r *blockingWorkspaceRepository) CompareAndSwap(ctx context.Context, item *workspacemodel.Workspace, expectedRevision uint64) error {
	return r.base.CompareAndSwap(ctx, item, expectedRevision)
}

func (r *blockingWorkspaceRepository) Delete(ctx context.Context, id common.WorkspaceID) error {
	return r.base.Delete(ctx, id)
}

var _ repository.WorkspaceRepository = (*blockingWorkspaceRepository)(nil)

type blockingStepRepository struct {
	base    repository.StepRepository
	started chan string
	release chan struct{}
}

func (r *blockingStepRepository) Create(ctx context.Context, item *runmodel.Step) error {
	return r.base.Create(ctx, item)
}

func (r *blockingStepRepository) Get(ctx context.Context, id common.StepID) (*runmodel.Step, error) {
	return r.base.Get(ctx, id)
}

func (r *blockingStepRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.Step, error) {
	select {
	case r.started <- string(runID):
	default:
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.release:
	}
	return r.base.ListByRun(ctx, runID)
}

func (r *blockingStepRepository) ListByRunSummary(ctx context.Context, runID common.RunID) ([]*runmodel.Step, error) {
	return r.base.ListByRunSummary(ctx, runID)
}

func (r *blockingStepRepository) Update(ctx context.Context, item *runmodel.Step) error {
	return r.base.Update(ctx, item)
}

func (r *blockingStepRepository) Delete(ctx context.Context, id common.StepID) error {
	return r.base.Delete(ctx, id)
}

var _ repository.StepRepository = (*blockingStepRepository)(nil)

type blockingRunListRepository struct {
	base    repository.RunRepository
	started chan string
	release chan struct{}
}

func (r *blockingRunListRepository) Create(ctx context.Context, item *runmodel.Run) error {
	return r.base.Create(ctx, item)
}

func (r *blockingRunListRepository) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	return r.base.Get(ctx, id)
}

func (r *blockingRunListRepository) GetByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID, id common.RunID) (*runmodel.Run, error) {
	return r.base.GetByWorkspaceID(ctx, workspaceID, id)
}

func (r *blockingRunListRepository) List(ctx context.Context) ([]*runmodel.Run, error) {
	return r.base.List(ctx)
}

func (r *blockingRunListRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	select {
	case r.started <- string(workspaceID):
	default:
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.release:
	}
	return r.base.ListByWorkspaceID(ctx, workspaceID)
}

func (r *blockingRunListRepository) Update(ctx context.Context, item *runmodel.Run) error {
	return r.base.Update(ctx, item)
}

func (r *blockingRunListRepository) CompareAndSwap(ctx context.Context, item *runmodel.Run, expectedRevision uint64) error {
	return r.base.CompareAndSwap(ctx, item, expectedRevision)
}

func (r *blockingRunListRepository) Delete(ctx context.Context, id common.RunID) error {
	return r.base.Delete(ctx, id)
}

var _ repository.RunRepository = (*blockingRunListRepository)(nil)

func TestGetRunFromRepositoryUsesScopedGetterWithoutWorkspaceList(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-run-helper-scoped")
	baseRuns := memory.NewRunRepository()
	runID := common.RunID("run-helper-scoped")
	if err := baseRuns.Create(context.Background(), &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-run-helper-scoped",
		State:       common.RunStatePending,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	started := make(chan string, 1)
	repo := &blockingRunListRepository{
		base:    baseRuns,
		started: started,
		release: make(chan struct{}),
	}

	got, err := getRunFromRepository(ctx, repo, runID)
	if err != nil {
		t.Fatalf("getRunFromRepository: %v", err)
	}
	if got.ID != runID || got.WorkspaceID != "ws-run-helper-scoped" {
		t.Fatalf("run = %+v, want scoped run", got)
	}
	select {
	case workspaceID := <-started:
		t.Fatalf("ListByWorkspaceID was called for %s; want scoped getter only", workspaceID)
	default:
	}
}

type countingMCPCapabilityRepository struct {
	base      repository.MCPCapabilityRepository
	getCalls  int
	listCalls int
}

func (r *countingMCPCapabilityRepository) Create(ctx context.Context, item *mcpmodel.Capability) error {
	return r.base.Create(ctx, item)
}

func (r *countingMCPCapabilityRepository) Get(ctx context.Context, id common.MCPCapabilityID) (*mcpmodel.Capability, error) {
	r.getCalls++
	return r.base.Get(ctx, id)
}

func (r *countingMCPCapabilityRepository) List(ctx context.Context) ([]*mcpmodel.Capability, error) {
	r.listCalls++
	return r.base.List(ctx)
}

func (r *countingMCPCapabilityRepository) Update(ctx context.Context, item *mcpmodel.Capability) error {
	return r.base.Update(ctx, item)
}

var _ repository.MCPCapabilityRepository = (*countingMCPCapabilityRepository)(nil)

type blockingDecisionResolutionRepository struct {
	base    readmodelcore.DecisionResolutionReader
	needed  int
	started chan common.TicketID
	release chan struct{}
	mu      sync.Mutex
	current int
}

func (r *blockingDecisionResolutionRepository) GetByTicketID(ctx context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	r.mu.Lock()
	r.current++
	current := r.current
	r.mu.Unlock()
	select {
	case r.started <- ticketID:
	default:
	}
	if current <= r.needed {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-r.release:
		}
	}
	return r.base.GetByTicketID(ctx, ticketID)
}

func TestReadRunSnapshotWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := ReadRunSnapshotWithSession(context.Background(), nil, "run-1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
	}
}

func TestMapGitOperationTracesProjectsStructuredOutput(t *testing.T) {
	traces := readmodelprojection.MapGitOperationTraces([]presenter.StepView{
		{
			ID:   "step-git-commit",
			Kind: gitcommon.StepKindWrite,
			Output: map[string]any{
				"status":           "completed",
				"operation":        "commit",
				"branch":           "main",
				"headCommitBefore": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"headCommitAfter":  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				"commit": map[string]any{
					"hash":    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					"subject": "docs: update",
				},
				"changedFiles": []any{
					map[string]any{"path": "docs/design.md", "status": "modified", "staged": true},
				},
				"summary": "git commit completed",
			},
		},
	})
	if len(traces) != 1 {
		t.Fatalf("git traces = %+v, want one trace", traces)
	}
	trace := traces[0]
	if trace.Operation != "commit" || trace.CommitHash == "" || trace.ChangedFiles[0].Path != "docs/design.md" {
		t.Fatalf("git trace = %+v, want commit projection with changed file", trace)
	}
	if len(trace.EvidenceRefs) != 1 || trace.EvidenceRefs[0] != "step-git-commit" {
		t.Fatalf("evidence refs = %+v, want step ref", trace.EvidenceRefs)
	}
}

func TestReadRunSnapshotUsesBoundedProjectedEventRecords(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)
	workspaceID := common.WorkspaceID("ws-bounded-events")
	runID := common.RunID("run-bounded-events")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "bounded events",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for i := 0; i < readModelEventRecordLimit+25; i++ {
		payload := map[string]any{
			recordpayload.PayloadKeyState:  "running",
			recordpayload.PayloadKeyOutput: map[string]any{"raw": strings.Repeat("x", eventPayloadStringLimit*8)},
		}
		topic := runevents.TopicRunStateChanged
		if i == readModelEventRecordLimit+20 {
			topic = "/run/llm_usage/recorded"
			payload = map[string]any{
				recordpayload.PayloadKeyWorkspaceID:  string(workspaceID),
				recordpayload.PayloadKeyRunID:        string(runID),
				recordpayload.PayloadKeyStepID:       "run-bounded-events/step-usage",
				recordpayload.PayloadKeyProviderID:   "provider-a",
				recordpayload.PayloadKeyModel:        "model-a",
				recordpayload.PayloadKeyOperation:    "chat_completions",
				recordpayload.PayloadKeyInputTokens:  30,
				recordpayload.PayloadKeyOutputTokens: 14,
				recordpayload.PayloadKeyTotalTokens:  44,
				recordpayload.PayloadKeyStatus:       "completed",
			}
		}
		if err := exports.Repositories.EventRecords.Append(ctx, &eventstreammodel.EventRecord{
			ID:          common.EventRecordID("event-bounded-" + strconv.Itoa(i)),
			Version:     "v1",
			Topic:       topic,
			OccurredAt:  fmt.Sprintf("2026-05-16T%02d:%02d:00Z", 10+i/60, i%60),
			WorkspaceID: workspaceID,
			RunID:       runID,
			Payload:     mustAppserviceGovernancePayload(t, payload),
		}); err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}

	snapshot, err := ReadRunSnapshotWithSession(ctx, session, string(runID))
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithSession: %v", err)
	}
	if len(snapshot.EventRecords) != readModelEventRecordLimit {
		t.Fatalf("event record count = %d, want %d", len(snapshot.EventRecords), readModelEventRecordLimit)
	}
	if snapshot.EventRecords[0].ID != "event-bounded-25" || snapshot.EventRecords[len(snapshot.EventRecords)-1].ID != "event-bounded-124" {
		t.Fatalf("bounded event window = first %q last %q, want latest 100 records", snapshot.EventRecords[0].ID, snapshot.EventRecords[len(snapshot.EventRecords)-1].ID)
	}
	for _, item := range snapshot.EventRecords {
		if _, ok := item.Payload.Field(recordpayload.PayloadKeyOutput); ok {
			t.Fatalf("snapshot event contains raw output payload: %+v", item.Payload)
		}
	}
	// RVC-14: formal Run snapshot hydrates topic-scoped usage payloads into summary
	// even on the compact path. General event timeline remains metadata-only above.
	if snapshot.LLMUsageSummary.EntryTotal == 0 || snapshot.LLMUsageSummary.TotalTokens == 0 {
		t.Fatalf("compact llm usage summary = %+v, want non-zero usage from /run/llm_usage/recorded", snapshot.LLMUsageSummary)
	}
	if len(snapshot.LLMUsageTraces) == 0 {
		t.Fatalf("compact llm usage traces empty, want hydrated run-scoped traces")
	}
}

func TestProjectRunSnapshotDetailBoundsLargeCollectionsAndOmitsRawPayloads(t *testing.T) {
	rawPayload := map[string]any{"content": strings.Repeat("x", 4096)}
	governancePayload := mustAppserviceGovernancePayload(t, rawPayload)
	rawOutput := map[string]any{"rawOutput": strings.Repeat("o", 4096), "summary": "kept"}
	count := readModelStepPageLimit + 7
	snapshot := RunSnapshot{
		Step: &presenter.StepView{
			ID:     "run-large/step-current",
			Kind:   "llm/workspace_execution",
			State:  "completed",
			Input:  rawPayload,
			Output: rawOutput,
		},
	}
	for idx := 0; idx < count; idx++ {
		suffix := fmt.Sprintf("%03d", idx)
		snapshot.Steps = append(snapshot.Steps, presenter.StepView{
			ID:     "run-large/step-" + suffix,
			Kind:   "llm/workspace_execution",
			State:  "completed",
			Input:  rawPayload,
			Output: rawOutput,
		})
		snapshot.ToolCalls = append(snapshot.ToolCalls, ToolCall{
			ID:        "tool-call-" + suffix,
			RunID:     "run-large",
			ToolName:  "neutral-tool",
			Arguments: rawPayload,
			State:     "completed",
			Output:    rawPayload,
		})
		snapshot.RuntimeCheckpoints = append(snapshot.RuntimeCheckpoints, RuntimeCheckpoint{
			ID:               "checkpoint-" + suffix,
			RunID:            "run-large",
			CheckpointType:   "runtime",
			RunStateSnapshot: "running",
			Data:             rawPayload,
		})
		snapshot.Results = append(snapshot.Results, Result{
			ID:      "result-" + suffix,
			RunID:   "run-large",
			State:   "completed",
			Summary: "summary",
			DoDAssessment: &DoDAssessment{
				Satisfied: true,
				Summary:   strings.Repeat("d", 4096),
			},
			SummaryDetail: &ResultSummary{
				ImplementationSummary: strings.Repeat("s", 4096),
			},
		})
		snapshot.Patches = append(snapshot.Patches, Patch{
			ID:         "patch-" + suffix,
			RunID:      "run-large",
			PatchType:  "unified_diff",
			TargetRef:  "workorch://target/" + suffix,
			State:      "applied",
			AddedLines: 3,
		})
		snapshot.ValidationResults = append(snapshot.ValidationResults, ValidationResult{
			ID:             "validation-" + suffix,
			RunID:          "run-large",
			ValidationKind: "generic",
			State:          "passed",
			Summary:        "validation summary",
			EvidenceRef:    "workorch://evidence/" + suffix,
		})
		snapshot.Artifacts = append(snapshot.Artifacts, Artifact{
			ID:           "artifact-" + suffix,
			RunID:        "run-large",
			ArtifactType: "file",
			State:        "created",
			Summary:      "artifact summary",
			Metadata:     rawPayload,
		})
		snapshot.Decisions = append(snapshot.Decisions, Decision{
			ID:     "decision-" + suffix,
			RunID:  "run-large",
			Reason: "needs input",
			State:  "resolved",
			Context: mustAppserviceGovernancePayload(t, map[string]any{
				"allowedRecoveryActions": []string{"replan", "terminate"},
				"blockerKind":            "runtime_ineffective_convergence",
				"failedStepInput":        rawPayload,
			}),
			ResolutionID:      "resolution-" + suffix,
			ResolutionType:    "resume",
			ResolutionPayload: governancePayload,
		})
		snapshot.Approvals = append(snapshot.Approvals, Approval{
			ID:      "approval-" + suffix,
			RunID:   "run-large",
			Reason:  "needs approval",
			Action:  "resume",
			State:   "approved",
			Context: governancePayload,
		})
	}

	projected := ProjectRunSnapshotDetail(snapshot)

	if projected.Step == nil || projected.Step.Input != nil || projected.Step.Output["rawOutput"] != nil || projected.Step.Output["summary"] != "kept" {
		t.Fatalf("current step projection = %+v, want raw input/output omitted and summary kept", projected.Step)
	}
	assertProjectedCollectionLength(t, "steps", len(projected.Steps), readModelStepPageLimit)
	assertProjectedCollectionLength(t, "tool calls", len(projected.ToolCalls), readModelToolCallPageLimit)
	assertProjectedCollectionLength(t, "checkpoints", len(projected.RuntimeCheckpoints), readModelCheckpointPageLimit)
	assertProjectedCollectionLength(t, "results", len(projected.Results), readModelResultPageLimit)
	assertProjectedCollectionLength(t, "patches", len(projected.Patches), readModelPatchPageLimit)
	assertProjectedCollectionLength(t, "validations", len(projected.ValidationResults), readModelValidationPageLimit)
	assertProjectedCollectionLength(t, "artifacts", len(projected.Artifacts), readModelArtifactPageLimit)
	assertProjectedCollectionLength(t, "decisions", len(projected.Decisions), readModelDecisionPageLimit)
	assertProjectedCollectionLength(t, "approvals", len(projected.Approvals), readModelApprovalPageLimit)

	if projected.Steps[0].Input != nil || projected.Steps[0].Output["rawOutput"] != nil || projected.Steps[0].Output["summary"] != "kept" {
		t.Fatalf("step projection = %+v, want raw input/output omitted and summary kept", projected.Steps[0])
	}
	if projected.ToolCalls[0].Arguments != nil || projected.ToolCalls[0].Output != nil {
		t.Fatalf("tool call projection = %+v, want raw arguments/output omitted", projected.ToolCalls[0])
	}
	if projected.RuntimeCheckpoints[0].Data != nil {
		t.Fatalf("checkpoint projection = %+v, want raw data omitted", projected.RuntimeCheckpoints[0])
	}
	if projected.Results[0].DoDAssessment == nil || projected.Results[0].DoDAssessment.Summary == "" ||
		projected.Results[0].SummaryDetail == nil || projected.Results[0].SummaryDetail.ImplementationSummary == "" {
		t.Fatalf("result projection = %+v, want structured assessment/detail retained", projected.Results[0])
	}
	if projected.Artifacts[0].Metadata != nil {
		t.Fatalf("artifact projection = %+v, want metadata omitted", projected.Artifacts[0])
	}
	blockerKind, _ := projected.Decisions[0].Context.String("blockerKind")
	_, hasFailedStepInput := projected.Decisions[0].Context.Field("failedStepInput")
	content, hasContent := projected.Decisions[0].ResolutionPayload.String("content")
	if blockerKind != "runtime_ineffective_convergence" || hasFailedStepInput || !hasContent {
		t.Fatalf("decision projection = %+v, want bounded context and bounded resolution payload", projected.Decisions[0])
	}
	if len(content) > 600 {
		t.Fatalf("decision resolution payload content length = %d, want bounded", len(content))
	}
	if len(projected.Approvals[0].Context.Fields) != 0 {
		t.Fatalf("approval projection = %+v, want context omitted", projected.Approvals[0])
	}
}

func assertProjectedCollectionLength(t *testing.T, name string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("%s count = %d, want %d", name, got, want)
	}
}

func TestListRunStepsWithSessionPaginatesProjectedSteps(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)
	workspaceID := common.WorkspaceID("ws-step-page")
	runID := common.RunID("run-step-page")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "step page", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for idx := 1; idx <= 3; idx++ {
		stepID := common.StepID(fmt.Sprintf("%s/step-%d", runID, idx))
		if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
			ID:    stepID,
			RunID: runID,
			Kind:  "llm/workspace_execution",
			State: common.StepStateSucceeded,
			Input: mustAppserviceGovernancePayload(t, map[string]any{"rawInput": strings.Repeat("i", 1024)}),
			Output: mustAppserviceGovernancePayload(t, map[string]any{
				"stage":       "stage-" + strconv.Itoa(idx),
				"summary":     "summary-" + strconv.Itoa(idx),
				"llmCallRefs": map[string]any{"requestRef": "workorch://request"},
				"rawOutput":   strings.Repeat("o", 1024),
			}),
		}); err != nil {
			t.Fatalf("create step %d: %v", idx, err)
		}
	}

	first, err := ListRunStepsWithBackend(ctx, "file", dataRoot, StepQuery{RunID: string(runID), Limit: 2})
	if err != nil {
		t.Fatalf("ListRunStepsWithSession first page: %v", err)
	}
	if len(first.Steps) != 2 || !first.HasMore || first.NextCursorAfterID != "run-step-page/step-2" {
		t.Fatalf("first page = %+v, want two steps and cursor step-2", first)
	}
	if first.Steps[0].Input != nil {
		t.Fatalf("first step input = %+v, want omitted projection", first.Steps[0].Input)
	}
	if first.Steps[0].Output["rawOutput"] != nil || first.Steps[0].Output["stage"] != "stage-1" {
		t.Fatalf("first step output = %+v, want projected output", first.Steps[0].Output)
	}

	second, err := ListRunStepsWithBackend(ctx, "file", dataRoot, StepQuery{RunID: string(runID), CursorAfterID: first.NextCursorAfterID, Limit: 2})
	if err != nil {
		t.Fatalf("ListRunStepsWithSession second page: %v", err)
	}
	if len(second.Steps) != 1 || second.HasMore || second.Steps[0].ID != "run-step-page/step-3" {
		t.Fatalf("second page = %+v, want final step only", second)
	}
}

func TestListRunToolCallsWithSessionPaginatesProjectedToolCalls(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)
	workspaceID := common.WorkspaceID("ws-tool-page")
	runID := common.RunID("run-tool-page")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "tool page", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for idx := 1; idx <= 3; idx++ {
		toolCallID := common.ToolCallID(fmt.Sprintf("%s/tool-%d", runID, idx))
		if err := exports.Repositories.ToolCalls.Create(ctx, &runmodel.ToolCall{
			ID:          toolCallID,
			WorkspaceID: workspaceID,
			RunID:       runID,
			StepID:      common.StepID(fmt.Sprintf("%s/step-%d", runID, idx)),
			ToolName:    "tool-" + strconv.Itoa(idx),
			Arguments:   mustAppserviceGovernancePayload(t, map[string]any{"rawArguments": strings.Repeat("a", 1024)}),
			State:       common.ToolCallStateCompleted,
			ExitSummary: "summary-" + strconv.Itoa(idx),
			Output:      mustAppserviceGovernancePayload(t, map[string]any{"rawOutput": strings.Repeat("o", 1024)}),
		}); err != nil {
			t.Fatalf("create tool call %d: %v", idx, err)
		}
	}

	first, err := ListRunToolCallsWithBackend(ctx, "file", dataRoot, ToolCallQuery{RunID: string(runID), Limit: 2})
	if err != nil {
		t.Fatalf("ListRunToolCallsWithSession first page: %v", err)
	}
	if len(first.ToolCalls) != 2 || !first.HasMore || first.NextCursorAfterID != "run-tool-page/tool-2" {
		t.Fatalf("first page = %+v, want two tool calls and cursor tool-2", first)
	}
	if first.ToolCalls[0].Arguments != nil || first.ToolCalls[0].Output != nil || first.ToolCalls[0].ExitSummary != "summary-1" {
		t.Fatalf("first tool call = %+v, want projected metadata without raw payloads", first.ToolCalls[0])
	}

	second, err := ListRunToolCallsWithBackend(ctx, "file", dataRoot, ToolCallQuery{RunID: string(runID), CursorAfterID: first.NextCursorAfterID, Limit: 2})
	if err != nil {
		t.Fatalf("ListRunToolCallsWithSession second page: %v", err)
	}
	if len(second.ToolCalls) != 1 || second.HasMore || second.ToolCalls[0].ID != "run-tool-page/tool-3" {
		t.Fatalf("second page = %+v, want final tool call only", second)
	}
}

func TestListRunRuntimeCheckpointsWithSessionPaginatesProjectedCheckpoints(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)
	workspaceID := common.WorkspaceID("ws-checkpoint-page")
	runID := common.RunID("run-checkpoint-page")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "checkpoint page", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for idx := 1; idx <= 3; idx++ {
		checkpointID := common.CheckpointID(fmt.Sprintf("%s/checkpoint-%d", runID, idx))
		if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
			ID:               checkpointID,
			WorkspaceID:      workspaceID,
			RunID:            runID,
			CheckpointType:   common.CheckpointTypeRunState,
			CurrentStepID:    common.StepID(fmt.Sprintf("%s/step-%d", runID, idx)),
			RunStateSnapshot: common.RunStateActive,
			OccurredAt:       fmt.Sprintf("2026-05-16T10:0%d:00Z", idx),
			Data:             mustAppserviceGovernancePayload(t, map[string]any{"rawData": strings.Repeat("d", 1024)}),
		}); err != nil {
			t.Fatalf("create checkpoint %d: %v", idx, err)
		}
	}

	first, err := ListRunRuntimeCheckpointsWithBackend(ctx, "file", dataRoot, RuntimeCheckpointQuery{RunID: string(runID), Limit: 2})
	if err != nil {
		t.Fatalf("ListRunRuntimeCheckpointsWithSession first page: %v", err)
	}
	if len(first.RuntimeCheckpoints) != 2 || !first.HasMore || first.NextCursorAfterID != "run-checkpoint-page/checkpoint-2" {
		t.Fatalf("first page = %+v, want two checkpoints and cursor checkpoint-2", first)
	}
	if first.RuntimeCheckpoints[0].Data != nil || first.RuntimeCheckpoints[0].CurrentStepID != "run-checkpoint-page/step-1" {
		t.Fatalf("first checkpoint = %+v, want projected metadata without raw data", first.RuntimeCheckpoints[0])
	}

	second, err := ListRunRuntimeCheckpointsWithBackend(ctx, "file", dataRoot, RuntimeCheckpointQuery{RunID: string(runID), CursorAfterID: first.NextCursorAfterID, Limit: 2})
	if err != nil {
		t.Fatalf("ListRunRuntimeCheckpointsWithSession second page: %v", err)
	}
	if len(second.RuntimeCheckpoints) != 1 || second.HasMore || second.RuntimeCheckpoints[0].ID != "run-checkpoint-page/checkpoint-3" {
		t.Fatalf("second page = %+v, want final checkpoint only", second)
	}
}

func TestListRunResultsWithSessionPaginatesProjectedResults(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)
	workspaceID := common.WorkspaceID("ws-result-page")
	runID := common.RunID("run-result-page")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "result page", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for idx := 1; idx <= 3; idx++ {
		resultID := common.ResultID(fmt.Sprintf("%s/result-%d", runID, idx))
		if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
			ID:           resultID,
			WorkspaceID:  workspaceID,
			RunID:        runID,
			State:        common.ResultStateAccepted,
			Title:        "result-" + strconv.Itoa(idx),
			Summary:      "summary-" + strconv.Itoa(idx),
			SourceType:   "runtime",
			SourceRef:    fmt.Sprintf("%s/step-%d", runID, idx),
			EvidenceRefs: []string{fmt.Sprintf("evidence-%d", idx)},
			DoDAssessment: resultmodel.DoDAssessment{
				Satisfied: true,
				Items:     []resultmodel.DoDAssessmentItem{{Item: strings.Repeat("dod", 512), State: "satisfied"}},
			},
			SummaryDetail: resultmodel.ResultSummary{
				GoalOutcome: strings.Repeat("goal", 512),
			},
		}); err != nil {
			t.Fatalf("create result %d: %v", idx, err)
		}
	}

	first, err := ListRunResultsWithBackend(ctx, "file", dataRoot, ResultQuery{RunID: string(runID), Limit: 2})
	if err != nil {
		t.Fatalf("ListRunResultsWithSession first page: %v", err)
	}
	if len(first.Results) != 2 || !first.HasMore || first.NextCursorAfterID != "run-result-page/result-2" {
		t.Fatalf("first page = %+v, want two results and cursor result-2", first)
	}
	if first.Results[0].DoDAssessment != nil || first.Results[0].SummaryDetail != nil || first.Results[0].Summary != "summary-1" {
		t.Fatalf("first result = %+v, want projected metadata without raw assessment detail", first.Results[0])
	}

	second, err := ListRunResultsWithBackend(ctx, "file", dataRoot, ResultQuery{RunID: string(runID), CursorAfterID: first.NextCursorAfterID, Limit: 2})
	if err != nil {
		t.Fatalf("ListRunResultsWithSession second page: %v", err)
	}
	if len(second.Results) != 1 || second.HasMore || second.Results[0].ID != "run-result-page/result-3" {
		t.Fatalf("second page = %+v, want final result only", second)
	}
}

func TestListRunPatchesWithSessionPaginatesProjectedPatches(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)
	workspaceID := common.WorkspaceID("ws-patch-page")
	runID := common.RunID("run-patch-page")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "patch page", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for idx := 1; idx <= 3; idx++ {
		patchID := common.PatchID(fmt.Sprintf("%s/patch-%d", runID, idx))
		if err := exports.Repositories.Patches.Create(ctx, &runmodel.Patch{
			ID:          patchID,
			WorkspaceID: workspaceID,
			RunID:       runID,
			StepID:      common.StepID(fmt.Sprintf("%s/step-%d", runID, idx)),
			PatchType:   "unified_diff",
			TargetRef:   fmt.Sprintf("target-%d", idx),
			Diff:        "+added\n-deleted\n" + strings.Repeat("x", 1024),
			Summary:     "summary-" + strconv.Itoa(idx),
			State:       common.PatchStateApplied,
		}); err != nil {
			t.Fatalf("create patch %d: %v", idx, err)
		}
	}

	first, err := ListRunPatchesWithBackend(ctx, "file", dataRoot, PatchQuery{RunID: string(runID), Limit: 2})
	if err != nil {
		t.Fatalf("ListRunPatchesWithSession first page: %v", err)
	}
	if len(first.Patches) != 2 || !first.HasMore || first.NextCursorAfterID != "run-patch-page/patch-2" {
		t.Fatalf("first page = %+v, want two patches and cursor patch-2", first)
	}
	if first.Patches[0].AddedLines != 1 || first.Patches[0].DeletedLines != 1 || first.Patches[0].Summary != "summary-1" {
		t.Fatalf("first patch = %+v, want projected metadata and line stats", first.Patches[0])
	}

	second, err := ListRunPatchesWithBackend(ctx, "file", dataRoot, PatchQuery{RunID: string(runID), CursorAfterID: first.NextCursorAfterID, Limit: 2})
	if err != nil {
		t.Fatalf("ListRunPatchesWithSession second page: %v", err)
	}
	if len(second.Patches) != 1 || second.HasMore || second.Patches[0].ID != "run-patch-page/patch-3" {
		t.Fatalf("second page = %+v, want final patch only", second)
	}
}

func TestListRunValidationResultsWithSessionPaginatesProjectedValidationResults(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)
	workspaceID := common.WorkspaceID("ws-validation-page")
	runID := common.RunID("run-validation-page")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "validation page", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for idx := 1; idx <= 3; idx++ {
		validationID := common.ValidationResultID(fmt.Sprintf("%s/validation-%d", runID, idx))
		if err := exports.Repositories.ValidationResults.Create(ctx, &runmodel.ValidationResult{
			ID:              validationID,
			WorkspaceID:     workspaceID,
			RunID:           runID,
			StepID:          common.StepID(fmt.Sprintf("%s/step-%d", runID, idx)),
			ValidationKind:  "dod",
			TargetRef:       fmt.Sprintf("target-%d", idx),
			State:           common.ValidationResultStatePassed,
			Summary:         "summary-" + strconv.Itoa(idx),
			EvidenceRef:     fmt.Sprintf("evidence-%d", idx),
			EvidenceQuality: common.EvidenceQualityValid,
			EvidenceReason:  strings.Repeat("reason", 256),
			EvidenceTags:    []string{"tag"},
		}); err != nil {
			t.Fatalf("create validation %d: %v", idx, err)
		}
	}

	first, err := ListRunValidationResultsWithBackend(ctx, "file", dataRoot, ValidationResultQuery{RunID: string(runID), Limit: 2})
	if err != nil {
		t.Fatalf("ListRunValidationResultsWithSession first page: %v", err)
	}
	if len(first.ValidationResults) != 2 || !first.HasMore || first.NextCursorAfterID != "run-validation-page/validation-2" {
		t.Fatalf("first page = %+v, want two validation results and cursor validation-2", first)
	}
	if first.ValidationResults[0].EvidenceRef != "evidence-1" || first.ValidationResults[0].Summary != "summary-1" {
		t.Fatalf("first validation = %+v, want projected validation metadata", first.ValidationResults[0])
	}

	second, err := ListRunValidationResultsWithBackend(ctx, "file", dataRoot, ValidationResultQuery{RunID: string(runID), CursorAfterID: first.NextCursorAfterID, Limit: 2})
	if err != nil {
		t.Fatalf("ListRunValidationResultsWithSession second page: %v", err)
	}
	if len(second.ValidationResults) != 1 || second.HasMore || second.ValidationResults[0].ID != "run-validation-page/validation-3" {
		t.Fatalf("second page = %+v, want final validation only", second)
	}
}

func TestListRunArtifactsWithSessionPaginatesProjectedArtifacts(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)
	workspaceID := common.WorkspaceID("ws-artifact-page")
	runID := common.RunID("run-artifact-page")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "artifact page", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for idx := 1; idx <= 3; idx++ {
		artifactID := common.ArtifactID(fmt.Sprintf("%s/artifact-%d", runID, idx))
		if err := exports.Repositories.Artifacts.Create(ctx, &resultmodel.Artifact{
			ID:           artifactID,
			WorkspaceID:  workspaceID,
			RunID:        runID,
			ResultID:     common.ResultID(fmt.Sprintf("%s/result-%d", runID, idx)),
			ArtifactType: "file",
			Category:     "deliverable",
			Name:         "artifact-" + strconv.Itoa(idx),
			Summary:      "summary-" + strconv.Itoa(idx),
			URI:          fmt.Sprintf("workorch://workspaces/%s/artifacts/%s/artifact-%d.txt", workspaceID, runID, idx),
			ContentType:  "text/plain",
			State:        common.ArtifactStateAvailable,
			EvidenceRefs: []string{fmt.Sprintf("evidence-%d", idx)},
			Metadata:     mustAppserviceEventPayload(t, map[string]any{"raw": strings.Repeat("m", 1024)}),
		}); err != nil {
			t.Fatalf("create artifact %d: %v", idx, err)
		}
	}

	first, err := ListRunArtifactsWithBackend(ctx, "file", dataRoot, ArtifactQuery{RunID: string(runID), Limit: 2})
	if err != nil {
		t.Fatalf("ListRunArtifactsWithSession first page: %v", err)
	}
	if len(first.Artifacts) != 2 || !first.HasMore || first.NextCursorAfterID != "run-artifact-page/artifact-2" {
		t.Fatalf("first page = %+v, want two artifacts and cursor artifact-2", first)
	}
	if first.Artifacts[0].Metadata != nil || first.Artifacts[0].URI == "" || first.Artifacts[0].Summary != "summary-1" {
		t.Fatalf("first artifact = %+v, want projected metadata without raw metadata map", first.Artifacts[0])
	}

	second, err := ListRunArtifactsWithBackend(ctx, "file", dataRoot, ArtifactQuery{RunID: string(runID), CursorAfterID: first.NextCursorAfterID, Limit: 2})
	if err != nil {
		t.Fatalf("ListRunArtifactsWithSession second page: %v", err)
	}
	if len(second.Artifacts) != 1 || second.HasMore || second.Artifacts[0].ID != "run-artifact-page/artifact-3" {
		t.Fatalf("second page = %+v, want final artifact only", second)
	}
}

func TestReadRunSnapshotWithBackendReturnsBlockedDecisionReadModel(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-decision-snapshot"
	started := prepareDecisionRun(t, dataRoot, "ws-run-snapshot", runID)

	item, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	if item.Workspace.ID != "ws-run-snapshot" {
		t.Fatalf("workspace = %+v, want ws-run-snapshot", item.Workspace)
	}
	if item.Run.ID != runID || item.Run.State != "blocked_on_decision" {
		t.Fatalf("run = %+v, want blocked decision run", item.Run)
	}
	if item.Step == nil || item.Step.ID == "" || item.Step.State == "" {
		t.Fatalf("step = %+v, want current step", item.Step)
	}
	if len(item.Steps) == 0 {
		t.Fatalf("steps = %+v, want non-empty", item.Steps)
	}
	if len(item.Decisions) != 1 || item.Decisions[0].ID != started.BlockingTicketID {
		t.Fatalf("decisions = %+v, want decision %s", item.Decisions, started.BlockingTicketID)
	}
}

func TestReadRunSnapshotReadsDecisionResolutionsConcurrently(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-concurrent-resolution-read")
	runID := common.RunID("run-concurrent-resolution-read")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "concurrent resolution read",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateBlockedOnDecision,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for i := 1; i <= 3; i++ {
		ticketID := common.TicketID("ticket-concurrent-resolution-" + strconv.Itoa(i))
		if err := exports.Repositories.Tickets.Create(ctx, &decisionmodel.Ticket{
			ID:          ticketID,
			WorkspaceID: workspaceID,
			RunID:       runID,
			Question:    "question " + strconv.Itoa(i),
			State:       common.DecisionTicketStateResolved,
		}); err != nil {
			t.Fatalf("create ticket %d: %v", i, err)
		}
		if err := exports.Repositories.Resolutions.Create(ctx, &decisionmodel.Resolution{
			ID:       common.ResolutionID("resolution-concurrent-" + strconv.Itoa(i)),
			TicketID: ticketID,
			Type:     "provide_missing_info",
			Payload:  mustAppserviceGovernancePayload(t, map[string]any{"answer": "answer " + strconv.Itoa(i)}),
		}); err != nil {
			t.Fatalf("create resolution %d: %v", i, err)
		}
	}

	release := make(chan struct{})
	started := make(chan common.TicketID, 3)
	deps, err := readModelDependenciesFromSession(session.Borrow())
	if err != nil {
		t.Fatalf("readModelDependenciesFromSession: %v", err)
	}
	deps.resolutions = &blockingDecisionResolutionRepository{
		base:    deps.resolutions,
		needed:  3,
		started: started,
		release: release,
	}

	errCh := make(chan error, 1)
	var snapshot RunSnapshot
	go func() {
		var err error
		snapshot, err = readRunSnapshot(ctx, deps, runID)
		errCh <- err
	}()

	seen := map[common.TicketID]struct{}{}
	for len(seen) < 3 {
		select {
		case ticketID := <-started:
			seen[ticketID] = struct{}{}
		case err := <-errCh:
			t.Fatalf("readRunSnapshot returned before all resolution reads started: %v", err)
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for concurrent resolution reads, saw %d", len(seen))
		}
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatalf("readRunSnapshot: %v", err)
	}
	if len(snapshot.Decisions) != 3 {
		t.Fatalf("decisions = %+v, want 3 resolved decisions", snapshot.Decisions)
	}
	for _, item := range snapshot.Decisions {
		if item.ResolutionID == "" || item.ResolutionType != "provide_missing_info" {
			t.Fatalf("decision = %+v, want mapped resolution", item)
		}
	}
}

func TestReadRunSnapshotReadsWorkspaceAndRunScopedEvidenceConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	workspaceID := common.WorkspaceID("ws-run-snapshot-concurrent")
	runID := common.RunID("run-snapshot-concurrent")
	baseWorkspaces := memory.NewWorkspaceRepository()
	baseSteps := memory.NewStepRepository()
	if err := baseWorkspaces.Create(context.Background(), &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "run snapshot concurrent",
		State:                common.WorkspaceStateActive,
		DeliverableDirectory: "workspaces/run-snapshot-concurrent",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := baseSteps.Create(context.Background(), &runmodel.Step{
		ID:     common.StepID("run-snapshot-concurrent/step-1"),
		RunID:  runID,
		Kind:   "llm/workspace_execution",
		State:  common.StepStateSucceeded,
		Output: mustAppserviceGovernancePayload(t, map[string]any{"summary": "done"}),
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	workspaceStarted := make(chan string, 1)
	stepStarted := make(chan string, 1)
	release := make(chan struct{})
	deps := readModelDependencies{
		workspaces: &blockingWorkspaceRepository{base: baseWorkspaces, started: workspaceStarted, release: release},
		runs: func() repository.RunRepository {
			repo := memory.NewRunRepository()
			if err := repo.Create(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
				t.Fatalf("create run: %v", err)
			}
			return repo
		}(),
		steps:                 &blockingStepRepository{base: baseSteps, started: stepStarted, release: release},
		results:               memory.NewResultRepository(),
		artifacts:             memory.NewArtifactRepository(),
		artifactContinuations: memory.NewArtifactContinuationRepository(),
		eventRecords:          eventRecordRepositoryTestReader{memory.NewEventRecordRepository()},
		memoryRecalls:         memoryRecallTraceRepositoryTestReader{memory.NewMemoryRecallTraceRepository()},
		toolCalls:             memory.NewToolCallRepository(),
		mcpCapabilities:       memory.NewMCPCapabilityRepository(),
		patches:               memory.NewPatchRepository(),
		validations:           memory.NewValidationResultRepository(),
		checkpoints:           memory.NewRuntimeCheckpointRepository(),
		tickets:               memory.NewDecisionTicketRepository(),
		resolutions:           memory.NewDecisionResolutionRepository(),
		approvals:             memory.NewApprovalRepository(),
	}

	done := make(chan error, 1)
	go func() {
		_, err := readRunSnapshot(ctx, deps, runID)
		done <- err
	}()

	for _, ch := range []<-chan string{workspaceStarted, stepStarted} {
		select {
		case <-ch:
		case err := <-done:
			t.Fatalf("readRunSnapshot returned before both reads started: %v", err)
		case <-ctx.Done():
			t.Fatal("timed out waiting for concurrent workspace and step reads")
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("readRunSnapshot: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for readRunSnapshot")
	}
}

func TestReadRunSnapshotScopesMemoryRecallTracesByWorkspace(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-memory-snapshot")
	otherWorkspaceID := common.WorkspaceID("ws-memory-snapshot-other")
	runID := common.RunID("run-memory-snapshot")
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	memoryRecallRepo := memory.NewMemoryRecallTraceRepository()

	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "memory snapshot", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := memoryRecallRepo.Create(ctx, &memorymodel.MemoryRecallTrace{
		ID:                 "trace-memory-snapshot-current",
		WorkspaceID:        workspaceID,
		RunID:              runID,
		Source:             "activation",
		Status:             "selected",
		RetrievedMemoryIDs: []common.MemoryID{"memory-current"},
	}); err != nil {
		t.Fatalf("create current memory recall trace: %v", err)
	}
	if err := memoryRecallRepo.Create(ctx, &memorymodel.MemoryRecallTrace{
		ID:                 "trace-memory-snapshot-other",
		WorkspaceID:        otherWorkspaceID,
		RunID:              runID,
		Source:             "activation",
		Status:             "selected",
		RetrievedMemoryIDs: []common.MemoryID{"memory-other"},
	}); err != nil {
		t.Fatalf("create other memory recall trace: %v", err)
	}

	snapshot, err := readRunSnapshot(ctx, readModelDependencies{
		workspaces:            workspaceRepo,
		runs:                  runRepo,
		steps:                 memory.NewStepRepository(),
		results:               memory.NewResultRepository(),
		artifacts:             memory.NewArtifactRepository(),
		artifactContinuations: memory.NewArtifactContinuationRepository(),
		eventRecords:          eventRecordRepositoryTestReader{memory.NewEventRecordRepository()},
		memoryRecalls:         memoryRecallTraceRepositoryTestReader{memoryRecallRepo},
		toolCalls:             memory.NewToolCallRepository(),
		mcpCapabilities:       memory.NewMCPCapabilityRepository(),
		patches:               memory.NewPatchRepository(),
		validations:           memory.NewValidationResultRepository(),
		checkpoints:           memory.NewRuntimeCheckpointRepository(),
		tickets:               memory.NewDecisionTicketRepository(),
		resolutions:           memory.NewDecisionResolutionRepository(),
		approvals:             memory.NewApprovalRepository(),
	}, runID)
	if err != nil {
		t.Fatalf("readRunSnapshot: %v", err)
	}
	if len(snapshot.MemoryRecallTraces) != 1 || snapshot.MemoryRecallTraces[0].ID != "trace-memory-snapshot-current" {
		t.Fatalf("memory recall traces = %+v, want only current workspace trace", snapshot.MemoryRecallTraces)
	}
}

func TestReadRunSnapshotWithBackendReturnsExecutionEvidence(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-run-evidence-snapshot")
	runID := common.RunID("run-evidence-snapshot")
	stepID := common.StepID("run-evidence-snapshot/step-1")
	toolCallID := common.ToolCallID("tool-call-evidence-snapshot")

	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "run evidence snapshot",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  runID,
		Kind:   "validation",
		State:  common.StepStateSucceeded,
		Output: mustAppserviceGovernancePayload(t, map[string]any{"result": "done"}),
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := exports.Repositories.ToolCalls.Create(ctx, &runmodel.ToolCall{
		ID:          toolCallID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolName:    "shell",
		Arguments:   mustAppserviceGovernancePayload(t, map[string]any{"command": "go test ./..."}),
		State:       common.ToolCallStateCompleted,
		Output:      mustAppserviceGovernancePayload(t, map[string]any{"exitCode": 0}),
	}); err != nil {
		t.Fatalf("create tool call: %v", err)
	}
	if err := exports.Repositories.Patches.Create(ctx, &runmodel.Patch{
		ID:          common.PatchID("patch-evidence-snapshot"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolCallID:  toolCallID,
		PatchType:   "unified_diff",
		TargetRef:   "internal/example.go",
		State:       common.PatchStateApplied,
	}); err != nil {
		t.Fatalf("create patch: %v", err)
	}
	if err := exports.Repositories.ValidationResults.Create(ctx, &runmodel.ValidationResult{
		ID:             common.ValidationResultID("validation-evidence-snapshot"),
		WorkspaceID:    workspaceID,
		RunID:          runID,
		StepID:         stepID,
		ValidationKind: "go_test",
		State:          common.ValidationResultStatePassed,
		Summary:        "passed",
	}); err != nil {
		t.Fatalf("create validation result: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:               common.CheckpointID("checkpoint-evidence-snapshot"),
		WorkspaceID:      workspaceID,
		RunID:            runID,
		CheckpointType:   common.CheckpointTypeRunState,
		CurrentStepID:    stepID,
		RunStateSnapshot: common.RunStateCompleted,
		Data:             mustAppserviceGovernancePayload(t, map[string]any{"thread": "main"}),
	}); err != nil {
		t.Fatalf("create runtime checkpoint: %v", err)
	}
	resultID := common.ResultID("result-evidence-snapshot")
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:           resultID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		DoDID:        common.DoDID("dod-evidence-snapshot"),
		State:        common.ResultStateAccepted,
		Title:        "Accepted Result",
		Summary:      "formal outcome",
		SourceType:   "dod_assessment",
		SourceRef:    string(stepID),
		EvidenceRefs: []string{string(stepID), "validation-evidence-snapshot"},
		DoDAssessment: resultmodel.DoDAssessment{
			Satisfied: true,
			Summary:   "formal outcome",
			Items: []resultmodel.DoDAssessmentItem{
				{
					Item:         "go test passes",
					State:        "satisfied",
					EvidenceRefs: []string{"validation-evidence-snapshot"},
				},
			},
		},
		SummaryDetail: resultmodel.ResultSummary{
			GoalOutcome:           "formal outcome",
			ImplementationSummary: "implemented requested changes",
			VerificationSummary:   "validation evidence passed",
			EvidenceRefs:          []string{string(stepID), "validation-evidence-snapshot"},
			ArtifactRefs:          []common.ArtifactID{"artifact-evidence-snapshot"},
			ResidualRisks:         []string{"none reported"},
			NextActions:           []string{"review artifact"},
			Diagnostics:           mustAppserviceEventPayload(t, map[string]any{"resultScope": "acceptance_slice", "sliceId": "slice-001"}),
		},
	}); err != nil {
		t.Fatalf("create result: %v", err)
	}
	if err := exports.Repositories.Artifacts.Create(ctx, &resultmodel.Artifact{
		ID:           common.ArtifactID("artifact-evidence-snapshot"),
		WorkspaceID:  workspaceID,
		RunID:        runID,
		ResultID:     resultID,
		StepID:       stepID,
		ArtifactType: "report",
		Category:     "final_summary",
		Name:         "outcome.md",
		Summary:      "formal artifact",
		State:        common.ArtifactStateAvailable,
		EvidenceRefs: []string{string(resultID)},
		Metadata:     mustAppserviceEventPayload(t, map[string]any{"format": "markdown"}),
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	first, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("first ReadRunSnapshotWithBackend: %v", err)
	}
	second, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("second ReadRunSnapshotWithBackend: %v", err)
	}

	if len(first.Steps) != 1 || first.Steps[0].ID != string(stepID) {
		t.Fatalf("steps = %+v, want %s", first.Steps, stepID)
	}
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].ID != string(toolCallID) {
		t.Fatalf("toolCalls = %+v, want %s", first.ToolCalls, toolCallID)
	}
	if len(first.Patches) != 1 || first.Patches[0].ToolCallID != string(toolCallID) {
		t.Fatalf("patches = %+v, want tool call ref %s", first.Patches, toolCallID)
	}
	if len(first.ValidationResults) != 1 || first.ValidationResults[0].State != "passed" {
		t.Fatalf("validationResults = %+v, want passed", first.ValidationResults)
	}
	if len(first.RuntimeCheckpoints) != 1 || first.RuntimeCheckpoints[0].Data["thread"] != "main" {
		t.Fatalf("runtimeCheckpoints = %+v, want checkpoint data", first.RuntimeCheckpoints)
	}
	if len(first.Results) != 1 || first.Results[0].Summary != "formal outcome" {
		t.Fatalf("results = %+v, want formal outcome", first.Results)
	}
	if first.Results[0].DoDAssessment == nil || !first.Results[0].DoDAssessment.Satisfied || len(first.Results[0].DoDAssessment.Items) != 1 {
		t.Fatalf("result DoD assessment = %+v, want item-level assessment", first.Results[0].DoDAssessment)
	}
	if first.Results[0].DoDAssessment.Items[0].Item != "go test passes" ||
		first.Results[0].DoDAssessment.Items[0].State != "satisfied" ||
		!reflect.DeepEqual(first.Results[0].DoDAssessment.Items[0].EvidenceRefs, []string{"validation-evidence-snapshot"}) {
		t.Fatalf("result DoD assessment items = %+v, want satisfied evidence item", first.Results[0].DoDAssessment.Items)
	}
	if len(first.DoDEvidence) != 1 ||
		first.DoDEvidence[0].ResultID != string(resultID) ||
		first.DoDEvidence[0].DoDID != "dod-evidence-snapshot" ||
		!first.DoDEvidence[0].Satisfied ||
		len(first.DoDEvidence[0].Items) != 1 ||
		first.DoDEvidence[0].Items[0].EvidenceStatus != "linked" ||
		len(first.DoDEvidence[0].Items[0].MissingEvidenceRefs) != 0 {
		t.Fatalf("dod evidence = %+v, want item-level trace with linked evidence refs", first.DoDEvidence)
	}
	if first.Results[0].SummaryDetail == nil ||
		first.Results[0].SummaryDetail.ImplementationSummary != "implemented requested changes" ||
		!reflect.DeepEqual(first.Results[0].SummaryDetail.ArtifactRefs, []string{"artifact-evidence-snapshot"}) ||
		first.Results[0].SummaryDetail.Diagnostics["resultScope"] != "acceptance_slice" ||
		first.Results[0].SummaryDetail.Diagnostics["sliceId"] != "slice-001" {
		t.Fatalf("result summary detail = %+v, want structured summary detail", first.Results[0].SummaryDetail)
	}
	if len(first.Artifacts) != 1 || first.Artifacts[0].ResultID != string(resultID) || first.Artifacts[0].Category != "final_summary" || first.Artifacts[0].Metadata["format"] != "markdown" {
		t.Fatalf("artifacts = %+v, want result-linked artifact", first.Artifacts)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("snapshot mismatch after reload: first=%+v second=%+v", first, second)
	}
	if len(first.ResultQuality) != 1 ||
		first.ResultQuality[0].ResultID != string(resultID) ||
		first.ResultQuality[0].State != "passed" {
		t.Fatalf("result quality = %+v, want one passed trace", first.ResultQuality)
	}
	if len(first.ResultQuality[0].Issues) != 0 {
		t.Fatalf("result quality issues = %+v, want no quality issues", first.ResultQuality[0].Issues)
	}
	observability, err := ReadRunObservabilityWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunObservabilityWithBackend: %v", err)
	}
	if len(observability.DoDEvidence) != 1 || len(observability.DoDEvidence[0].Items) != 1 || observability.DoDEvidence[0].Items[0].EvidenceStatus != "linked" {
		t.Fatalf("observability dod evidence = %+v, want dod evidence trace", observability.DoDEvidence)
	}
	if len(observability.ResultQuality) != 1 || observability.ResultQuality[0].State != "passed" {
		t.Fatalf("observability result quality = %+v, want passed result quality trace", observability.ResultQuality)
	}
}

func TestReadRunSnapshotWithBackendReturnsSelectedAndUsedCapabilities(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-run-capability-snapshot")
	runID := common.RunID("run-capability-snapshot")
	stepID := common.StepID("run-capability-snapshot/block/repo-summary")
	capabilityID := common.MCPCapabilityID("mcp-capability-read-model:tool:repo.summary")

	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "capability snapshot", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.MCPCapabilities.Create(ctx, &mcpmodel.Capability{
		ID:             capabilityID,
		ServerID:       common.MCPServerID("mcp-capability-read-model"),
		Kind:           mcpmodel.CapabilityKindTool,
		Name:           "repo.summary",
		DisplayName:    "仓库摘要能力",
		Category:       "代码仓库",
		RiskLevel:      "read_only",
		Enabled:        true,
		ApprovalPolicy: "operator",
		Status:         "active",
	}); err != nil {
		t.Fatalf("create capability: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateCompleted,
		RuntimeBootstrap: &orchestratormodel.RuntimeBootstrap{Activation: orchestratormodel.RuntimeActivationContext{MCPCapabilities: []orchestratormodel.RuntimeMCPCapabilityContext{{
			ID: capabilityID, ServerID: "mcp-capability-read-model", Kind: mcpmodel.CapabilityKindTool, Name: "repo.summary", Enabled: true,
		}}}},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "mcp/tool_call",
		State: common.StepStateSucceeded,
		Output: mustAppserviceGovernancePayload(t, map[string]any{
			"operation": "mcp/tool_call",
			"serverId":  "mcp-capability-read-model",
			"tool":      "repo.summary",
			"status":    "completed",
		}),
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := exports.Repositories.ToolCalls.Create(ctx, &runmodel.ToolCall{
		ID:          common.ToolCallID("tool-call-capability-read-model"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolName:    "mcp/tool_call",
		State:       common.ToolCallStateCompleted,
		Output: mustAppserviceGovernancePayload(t, map[string]any{
			"operation": "mcp/tool_call",
			"serverId":  "mcp-capability-read-model",
			"tool":      "repo.summary",
			"status":    "completed",
		}),
	}); err != nil {
		t.Fatalf("create tool call: %v", err)
	}

	snapshot, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	if len(snapshot.SelectedCapabilities) != 1 || snapshot.SelectedCapabilities[0].DisplayName != "仓库摘要能力" {
		t.Fatalf("selected capabilities = %+v, want display metadata", snapshot.SelectedCapabilities)
	}
	if len(snapshot.UsedCapabilities) != 1 || snapshot.UsedCapabilities[0].ID != string(capabilityID) || snapshot.UsedCapabilities[0].UseCount != 1 {
		t.Fatalf("used capabilities = %+v, want used capability evidence", snapshot.UsedCapabilities)
	}
	observability, err := ReadRunObservabilityWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunObservabilityWithBackend: %v", err)
	}
	if len(observability.SelectedCapabilities) != 1 || len(observability.UsedCapabilities) != 1 {
		t.Fatalf("observability capabilities = selected:%+v used:%+v, want capability refs", observability.SelectedCapabilities, observability.UsedCapabilities)
	}
}

func TestMapResultQualityFlagsWeakResult(t *testing.T) {
	snapshot := RunSnapshot{
		Run: Run{
			ID:              "run-quality-weak",
			WorkspaceID:     "ws-quality-weak",
			DoDID:           "dod-quality-weak",
			State:           "completed",
			InitialStepKind: "llm/workspace_execution",
		},
		Results: []Result{
			{
				ID:          "result-quality-weak",
				WorkspaceID: "ws-quality-weak",
				RunID:       "run-quality-weak",
				DoDID:       "dod-quality-weak",
				State:       "accepted",
				Summary:     "too thin",
				DoDAssessment: &DoDAssessment{
					Satisfied: true,
					Summary:   "too thin",
					Items: []DoDAssessmentItem{
						{Item: "tests pass", State: "satisfied", EvidenceRefs: []string{"step-1"}},
					},
				},
				SummaryDetail: &ResultSummary{
					DoDItems: []DoDAssessmentItem{
						{Item: "tests pass", State: "satisfied", EvidenceRefs: []string{"step-1"}},
					},
				},
			},
		},
	}
	quality := readmodelprojection.MapResultQuality(snapshot)
	if len(quality) != 1 {
		t.Fatalf("result quality = %+v, want one trace", quality)
	}
	if quality[0].State != "failed" {
		t.Fatalf("result quality state = %q, want failed", quality[0].State)
	}
	codes := map[string]bool{}
	for _, issue := range quality[0].Issues {
		codes[issue.Code] = true
	}
	for _, want := range []string{"missing_goal_outcome", "missing_verification_summary", "missing_evidence_refs", "missing_risk_disclosure"} {
		if !codes[want] {
			t.Fatalf("result quality issues = %+v, missing %s", quality[0].Issues, want)
		}
	}
}

func TestMapResultQualityFlagsWeakDoDItemEvidence(t *testing.T) {
	snapshot := RunSnapshot{
		Run: Run{
			ID:          "run-quality-weak-evidence",
			WorkspaceID: "ws-quality-weak-evidence",
			DoDID:       "dod-quality-weak-evidence",
		},
		ValidationResults: []ValidationResult{
			{
				ID:              "validation-1",
				RunID:           "run-quality-weak-evidence",
				State:           "passed",
				Summary:         "validation completed without producing matching work",
				EvidenceQuality: string(common.EvidenceQualityInconclusive),
				ZeroWork:        true,
			},
		},
		Results: []Result{
			{
				ID:          "result-quality-weak-evidence",
				WorkspaceID: "ws-quality-weak-evidence",
				RunID:       "run-quality-weak-evidence",
				DoDID:       "dod-quality-weak-evidence",
				State:       "accepted",
				Summary:     "weak evidence",
				DoDAssessment: &DoDAssessment{
					Satisfied: true,
					Summary:   "weak evidence",
					Items: []DoDAssessmentItem{
						{Item: "validation evidence is produced by real executed work", State: "satisfied", EvidenceRefs: []string{"validation-1"}},
					},
				},
				SummaryDetail: &ResultSummary{
					GoalOutcome:         "validation completed",
					VerificationSummary: "validation produced inconclusive evidence",
					EvidenceRefs:        []string{"validation-1"},
					KnownLimitations:    []string{"none"},
					ResidualRisks:       []string{"assertion strength uncertain"},
					NextActions:         []string{"add assertion coverage"},
				},
				EvidenceRefs: []string{"validation-1"},
			},
		},
	}
	quality := readmodelprojection.MapResultQuality(snapshot)
	if len(quality) != 1 {
		t.Fatalf("result quality = %+v, want one trace", quality)
	}
	if quality[0].State != "failed" {
		t.Fatalf("result quality state = %q, want failed", quality[0].State)
	}
	if !containsResultQualityIssueCode(quality[0].Issues, "dod_item_weak_verification_evidence") {
		t.Fatalf("result quality issues = %+v, want dod_item_weak_verification_evidence", quality[0].Issues)
	}
}

func TestReadRunSnapshotLoadsCapabilityCatalogOnce(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-run-capability-catalog")
	runID := common.RunID("run-capability-catalog")
	stepID := common.StepID("run-capability-catalog/block/repo-summary")
	capabilityID := common.MCPCapabilityID("mcp-capability-catalog:tool:repo.summary")
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	toolCallRepo := memory.NewToolCallRepository()
	capabilityRepo := &countingMCPCapabilityRepository{base: memory.NewMCPCapabilityRepository()}

	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "capability catalog", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := capabilityRepo.Create(ctx, &mcpmodel.Capability{
		ID:             capabilityID,
		ServerID:       common.MCPServerID("mcp-capability-catalog"),
		Kind:           mcpmodel.CapabilityKindTool,
		Name:           "repo.summary",
		DisplayName:    "仓库摘要能力",
		Category:       "代码仓库",
		RiskLevel:      "read_only",
		Enabled:        true,
		ApprovalPolicy: "operator",
		Status:         "active",
	}); err != nil {
		t.Fatalf("create capability: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateCompleted,
		RuntimeBootstrap: &orchestratormodel.RuntimeBootstrap{Activation: orchestratormodel.RuntimeActivationContext{MCPCapabilities: []orchestratormodel.RuntimeMCPCapabilityContext{{
			ID: capabilityID, ServerID: "mcp-capability-catalog", Kind: mcpmodel.CapabilityKindTool, Name: "repo.summary", Enabled: true,
		}}}},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "mcp/tool_call",
		State: common.StepStateSucceeded,
		Output: mustAppserviceGovernancePayload(t, map[string]any{
			"operation": "mcp/tool_call",
			"serverId":  "mcp-capability-catalog",
			"tool":      "repo.summary",
			"status":    "completed",
		}),
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := toolCallRepo.Create(ctx, &runmodel.ToolCall{
		ID:          common.ToolCallID("tool-call-capability-catalog"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolName:    "mcp/tool_call",
		State:       common.ToolCallStateCompleted,
		Output: mustAppserviceGovernancePayload(t, map[string]any{
			"operation": "mcp/tool_call",
			"serverId":  "mcp-capability-catalog",
			"tool":      "repo.summary",
			"status":    "completed",
		}),
	}); err != nil {
		t.Fatalf("create tool call: %v", err)
	}

	snapshot, err := readRunSnapshot(ctx, readModelDependencies{
		workspaces:            workspaceRepo,
		runs:                  runRepo,
		steps:                 stepRepo,
		results:               memory.NewResultRepository(),
		artifacts:             memory.NewArtifactRepository(),
		artifactContinuations: memory.NewArtifactContinuationRepository(),
		eventRecords:          eventRecordRepositoryTestReader{memory.NewEventRecordRepository()},
		memoryRecalls:         memoryRecallTraceRepositoryTestReader{memory.NewMemoryRecallTraceRepository()},
		toolCalls:             toolCallRepo,
		mcpCapabilities:       capabilityRepo,
		patches:               memory.NewPatchRepository(),
		validations:           memory.NewValidationResultRepository(),
		checkpoints:           memory.NewRuntimeCheckpointRepository(),
		tickets:               memory.NewDecisionTicketRepository(),
		resolutions:           memory.NewDecisionResolutionRepository(),
		approvals:             memory.NewApprovalRepository(),
	}, runID)
	if err != nil {
		t.Fatalf("readRunSnapshot: %v", err)
	}
	if capabilityRepo.listCalls != 1 || capabilityRepo.getCalls != 0 {
		t.Fatalf("capability reads = list:%d get:%d, want one catalog list and no per-id get", capabilityRepo.listCalls, capabilityRepo.getCalls)
	}
	if len(snapshot.SelectedCapabilities) != 1 || snapshot.SelectedCapabilities[0].DisplayName != "仓库摘要能力" {
		t.Fatalf("selected capabilities = %+v, want catalog metadata", snapshot.SelectedCapabilities)
	}
	if len(snapshot.UsedCapabilities) != 1 || snapshot.UsedCapabilities[0].ID != string(capabilityID) || snapshot.UsedCapabilities[0].UseCount != 1 {
		t.Fatalf("used capabilities = %+v, want catalog metadata with use count", snapshot.UsedCapabilities)
	}
}

func TestMapRunObservabilityReusesSnapshotProjection(t *testing.T) {
	snapshot := RunSnapshot{
		Workspace: Workspace{ID: "ws-observability-projection"},
		Run:       Run{ID: "run-observability-projection", WorkspaceID: "ws-observability-projection", State: string(common.RunStateCompleted)},
		TaskProgress: TaskProgress{
			WorkspaceID: "ws-observability-projection",
			RunID:       "run-observability-projection",
			State:       string(common.RunStateCompleted),
			Summary:     "cached task progress",
		},
		TaskDecompositions: []TaskDecomposition{{
			ID:      "cached-decomposition",
			RunID:   "run-observability-projection",
			State:   "completed",
			Summary: "cached decomposition",
		}},
		TaskPlans: []TaskPlan{{
			ID:             "cached-root/plan/1",
			RunID:          "run-observability-projection",
			AnchorStepID:   "cached-root",
			RevisionStepID: "cached-root",
			Revision:       1,
			State:          "accepted",
			Summary:        "cached plan",
			NodeTotal:      1,
		}},
		ExecutionSections: []ExecutionSection{{
			ID:      "cached-section",
			RunID:   "run-observability-projection",
			Title:   "cached section",
			State:   "completed",
			Summary: "cached section summary",
		}},
	}

	observability := readmodelprojection.MapRunObservability(snapshot)

	if observability.TaskProgress.Summary != "cached task progress" {
		t.Fatalf("task progress = %+v, want cached projection", observability.TaskProgress)
	}
	if len(observability.TaskDecompositions) != 1 || observability.TaskDecompositions[0].ID != "cached-decomposition" {
		t.Fatalf("task decompositions = %+v, want cached projection", observability.TaskDecompositions)
	}
	if len(observability.TaskPlans) != 1 || observability.TaskPlans[0].ID != "cached-root/plan/1" {
		t.Fatalf("task plans = %+v, want cached projection", observability.TaskPlans)
	}
	if len(observability.ExecutionSections) != 1 || observability.ExecutionSections[0].ID != "cached-section" {
		t.Fatalf("execution sections = %+v, want cached projection", observability.ExecutionSections)
	}
}

func TestMapWebSourceTracesProjectsSearchAndFetchSteps(t *testing.T) {
	items := readmodelprojection.MapWebSourceTraces([]presenter.StepView{
		{
			ID:    "run-web/step-search",
			Kind:  webcommon.StepKindSearch,
			State: string(common.StepStateSucceeded),
			Output: map[string]any{
				"provider":   "default-http",
				"query":      "golang latest",
				"searchedAt": "2026-05-05T00:00:00Z",
				"policy":     map[string]any{"decision": "allowed"},
				"results": []map[string]any{{
					"title":       "Go Downloads",
					"url":         "https://go.dev/dl/",
					"snippet":     "Download Go",
					"source":      "go.dev",
					"publishedAt": "2026-05-01",
					"rank":        1,
				}},
			},
		},
		{
			ID:    "run-web/step-fetch",
			Kind:  webcommon.StepKindFetch,
			State: string(common.StepStateSucceeded),
			Output: map[string]any{
				"provider":    "default-http",
				"url":         "https://go.dev/dl/",
				"finalUrl":    "https://go.dev/dl/",
				"title":       "Downloads",
				"source":      "go.dev",
				"retrievedAt": "2026-05-05T00:00:01Z",
				"contentHash": "hash",
				"truncated":   true,
				"approvalId":  "approval-web",
				"policy":      map[string]any{"decision": "allowed"},
			},
		},
	})
	if len(items) != 2 {
		t.Fatalf("web sources = %+v, want search and fetch traces", items)
	}
	if items[0].Query != "golang latest" ||
		len(items[0].Sources) != 1 ||
		items[0].Sources[0].EvidenceRef != "run-web/step-search" ||
		items[0].PolicyDecision != "allowed" {
		t.Fatalf("search trace = %+v, want normalized source", items[0])
	}
	if len(items[1].FetchedSources) != 1 ||
		items[1].FetchedSources[0].ContentHash != "hash" ||
		!items[1].FetchedSources[0].Truncated ||
		items[1].ApprovalID != "approval-web" {
		t.Fatalf("fetch trace = %+v, want normalized fetched source", items[1])
	}
	observability := readmodelprojection.MapRunObservability(RunSnapshot{
		Run:        Run{ID: "run-web", WorkspaceID: "ws-web"},
		WebSources: items,
	})
	if len(observability.WebSources) != 2 || observability.WebSources[0].Sources[0].URL != "https://go.dev/dl/" {
		t.Fatalf("observability web sources = %+v, want projected web sources", observability.WebSources)
	}
}

func TestRuntimeTimelineProjectsPersistedWebStepEvents(t *testing.T) {
	runID := "run-web-event"
	stepID := "run-web-event/step-search"
	snapshot := RunSnapshot{
		Run: Run{ID: runID, WorkspaceID: "ws-web-event", State: string(common.RunStateCompleted)},
		Steps: []presenter.StepView{{
			ID:    stepID,
			Kind:  webcommon.StepKindSearch,
			State: string(common.StepStateSucceeded),
			Output: map[string]any{
				"status":     "completed",
				"stage":      "web_search",
				"provider":   "default-http",
				"query":      "golang latest",
				"searchedAt": "2026-05-05T00:00:00Z",
				"results": []map[string]any{{
					"title": "Go Downloads",
					"url":   "https://go.dev/dl/",
				}},
			},
		}},
		EventRecords: []EventRecord{{
			ID:          "event-web-step-completed",
			Topic:       "/step/state/changed",
			SubjectType: "step",
			SubjectID:   stepID,
			WorkspaceID: "ws-web-event",
			RunID:       runID,
			StepID:      stepID,
			OccurredAt:  "2026-05-05T00:00:02Z",
			Payload: mustReadModelEventPayload(t, map[string]any{
				recordpayload.PayloadKeyKind:  webcommon.StepKindSearch,
				recordpayload.PayloadKeyState: string(common.StepStateSucceeded),
				recordpayload.PayloadKeyOutput: map[string]any{
					"status":     "completed",
					"stage":      "web_search",
					"provider":   "default-http",
					"query":      "golang latest",
					"searchedAt": "2026-05-05T00:00:00Z",
					"results": []any{map[string]any{
						"title": "Go Downloads",
						"url":   "https://go.dev/dl/",
					}},
				},
			}),
		}},
		WebSources: []WebSourceTrace{{
			Query:        "golang latest",
			ProviderID:   "default-http",
			EvidenceRefs: []string{stepID},
			Sources:      []WebSource{{Title: "Go Downloads", URL: "https://go.dev/dl/", EvidenceRef: stepID}},
		}},
	}

	observability := readmodelprojection.MapRunObservability(snapshot)

	if observability.Evidence.EventTotal != 1 || len(observability.WebSources) != 1 {
		t.Fatalf("observability = %+v, want event count and web source projection", observability)
	}
	if !timelineContainsPersistedEvent(observability.Timeline, "/step/state/changed") ||
		!timelineContainsStepOccurredAt(observability.Timeline, "step", stepID, "2026-05-05T00:00:02Z") {
		t.Fatalf("timeline = %+v, want persisted web step event and step occurrence", observability.Timeline)
	}
}

func TestMapLLMContextBudgetTracesProjectsStructuredBudgets(t *testing.T) {
	steps := []presenter.StepView{
		{
			ID:    "run-ctx/step-1",
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: string(common.StepStateSucceeded),
			Output: map[string]any{
				"contextBudget": map[string]any{
					"maxBytes":              98304,
					"actualBytes":           72440,
					"maxTokens":             16000,
					"estimatedInputTokens":  12000,
					"estimatedOutputTokens": 2048,
					"actualTokens":          14048,
					"shrinkLevel":           3,
					"truncated":             true,
					"rules":                 []string{"compact provider input", "apply priority compaction (P0/P1/P2)"},
					"sourceRefs":            []string{"runtime.definition.goal", "runtime.graphEvidence"},
					"includedSections":      []string{"p0"},
					"normalizedSections":    []string{"request-intent"},
					"compressedSections":    []string{"p1", "p2"},
					"omittedSections":       []string{"older_p1"},
					"goalDoDCompressed":     false,
					"p0Overflow":            true,
					"tracePolicy":           "summary_refs_only",
					"promptCaptured":        false,
					"estimator":             "bytes_ratio",
					"cachePlan": map[string]any{
						"stablePrefixHash":        "sha256:c0",
						"workspaceDefinitionHash": "sha256:c1",
						"volatileTailHash":        "sha256:c4",
						"cacheBreakers": []map[string]any{
							{"sectionId": "C4.current-progress", "reason": "volatile_execution_tail"},
						},
					},
					"profile": map[string]any{
						"contextWindowTokens":    200000,
						"maxInputTokens":         160000,
						"reservedOutputTokens":   2048,
						"safetyRatio":            0.8,
						"bytesHardLimit":         98304,
						"estimatedTokensPerByte": 0.25,
					},
				},
				"llmUsage": map[string]any{
					"providerId": "reasoning",
					"provider":   "compatible",
					"model":      "gpt-5",
					"estimated":  true,
				},
			},
		},
		{
			ID:    "run-ctx/step-shell",
			Kind:  shellcommon.StepKindCommand,
			State: string(common.StepStateSucceeded),
			Output: map[string]any{
				"status": "completed",
			},
		},
	}
	traces := readmodelprojection.MapLLMContextBudgetTraces(steps)
	if len(traces) != 1 {
		t.Fatalf("traces = %+v, want one llm context budget trace", traces)
	}
	cachePrefix, _ := traces[0].CachePlan.String("stablePrefixHash")
	contextWindowTokens, _ := traces[0].Profile.Int64("contextWindowTokens")
	if traces[0].StepID != "run-ctx/step-1" ||
		traces[0].ProviderID != "reasoning" ||
		traces[0].Model != "gpt-5" ||
		!traces[0].Truncated ||
		traces[0].ShrinkLevel != 3 ||
		len(traces[0].SourceRefs) != 2 ||
		len(traces[0].IncludedSections) != 1 ||
		len(traces[0].CompressedSections) != 2 ||
		traces[0].GoalDoDCompressed ||
		!traces[0].P0Overflow ||
		traces[0].TracePolicy != "summary_refs_only" ||
		traces[0].PromptCaptured ||
		cachePrefix != "sha256:c0" ||
		contextWindowTokens != 200000 {
		t.Fatalf("trace = %+v, want projected context budget fields", traces[0])
	}
	observability := readmodelprojection.MapRunObservability(RunSnapshot{
		Run:               Run{ID: "run-ctx", WorkspaceID: "ws-ctx"},
		LLMContextBudgets: traces,
	})
	if len(observability.LLMContextBudgets) != 1 || observability.LLMContextBudgets[0].StepID != "run-ctx/step-1" {
		t.Fatalf("observability budgets = %+v, want projected context budget traces", observability.LLMContextBudgets)
	}
}

func TestRunContextBundleReusesSnapshotTaskProgress(t *testing.T) {
	snapshot := RunSnapshot{
		Workspace: Workspace{ID: "ws-context-progress-reuse"},
		Run:       Run{ID: "run-context-progress-reuse", WorkspaceID: "ws-context-progress-reuse", State: string(common.RunStateCompleted)},
		TaskProgress: TaskProgress{
			WorkspaceID:      "ws-context-progress-reuse",
			RunID:            "run-context-progress-reuse",
			State:            string(common.RunStateCompleted),
			Summary:          "cached progress reused",
			CurrentMilestone: "formal_result",
			EvidenceRefs:     []string{"cached-progress-evidence"},
			Recovery: RecoveryPlan{
				Mode:       "complete",
				NextAction: "inspect_result",
				RunID:      "run-context-progress-reuse",
			},
		},
	}

	bundle := readmodelprojection.MapRunContextBundle(snapshot)
	compressed := readmodelprojection.CompressedContextFromLocalizedRunSnapshot(snapshot)
	progress := readmodelprojection.RunSnapshotTaskProgress(snapshot)

	if bundle.TaskProgress.Summary != "cached progress reused" || bundle.Recovery.Mode != "complete" {
		t.Fatalf("bundle progress/recovery = %+v/%+v, want cached task progress", bundle.TaskProgress, bundle.Recovery)
	}
	if compressed.Summary != "cached progress reused" || compressed.CurrentMilestone != "formal_result" {
		t.Fatalf("compressed context = %+v, want cached progress-derived summary and milestone", compressed)
	}
	if progress.Summary != "cached progress reused" || progress.EvidenceRefs[0] != "cached-progress-evidence" {
		t.Fatalf("run snapshot task progress = %+v, want cached projection", progress)
	}
}

func TestRunProjectionHelpersReuseSnapshotDerivedViews(t *testing.T) {
	snapshot := RunSnapshot{
		Workspace: Workspace{ID: "ws-derived-view-reuse"},
		Run:       Run{ID: "run-derived-view-reuse", WorkspaceID: "ws-derived-view-reuse", State: string(common.RunStateActive)},
		TaskProgress: TaskProgress{
			WorkspaceID: "ws-derived-view-reuse",
			RunID:       "run-derived-view-reuse",
			State:       string(common.RunStateActive),
			Summary:     "cached derived progress",
		},
		TaskDecompositions: []TaskDecomposition{{
			ID:    "cached-derived-decomposition",
			RunID: "run-derived-view-reuse",
			State: "running",
		}},
	}

	progress := readmodelprojection.RunSnapshotTaskProgress(snapshot)
	decompositions := readmodelprojection.RunSnapshotTaskDecompositions(snapshot)

	if progress.Summary != "cached derived progress" {
		t.Fatalf("progress = %+v, want cached snapshot progress", progress)
	}
	if len(decompositions) != 1 || decompositions[0].ID != "cached-derived-decomposition" {
		t.Fatalf("decompositions = %+v, want cached snapshot decomposition", decompositions)
	}
}

func TestReadRunStatusWithBackendReturnsBlockedApprovalSummary(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-approval-status"
	started := prepareApprovalRun(t, dataRoot, "ws-run-status", runID)

	item, err := ReadRunStatusWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("ReadRunStatusWithBackend: %v", err)
	}
	if item.ID != runID || item.WorkspaceID != "ws-run-status" || item.State != "blocked_on_approval" {
		t.Fatalf("status = %+v, want blocked approval status", item)
	}
	if item.StepCount == 0 || item.CurrentStepID == "" || item.CurrentStepState == "" {
		t.Fatalf("status = %+v, want populated step summary", item)
	}
	if item.ApprovalCount != 1 || item.BlockingApprovalID != started.BlockingApprovalID {
		t.Fatalf("status = %+v, want approval count 1 and blocking approval %s", item, started.BlockingApprovalID)
	}
}

func TestReadRunStatusUsesLatestCheckpointStep(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "memory", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-status-current-step")
	runID := common.RunID("run-status-current-step")
	initialStepID := common.StepID("run-status-current-step/step-1")
	latestStepID := common.StepID("run-status-current-step/provider-retry-002-of-repair-001-of-planning-001")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{ID: latestStepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateFailed}); err != nil {
		t.Fatalf("create latest step: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{ID: initialStepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateFailed}); err != nil {
		t.Fatalf("create initial step: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:               "run-status-current-step/checkpoint/1",
		WorkspaceID:      workspaceID,
		RunID:            runID,
		CurrentStepID:    latestStepID,
		RunStateSnapshot: common.RunStateActive,
		OccurredAt:       "2026-05-12T11:20:53Z",
		Data:             mustAppserviceGovernancePayload(t, map[string]any{"event": "step_failed"}),
	}); err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}

	status, err := ReadRunStatusWithSession(ctx, session, string(runID))
	if err != nil {
		t.Fatalf("ReadRunStatusWithSession: %v", err)
	}
	if status.CurrentStepID != string(latestStepID) || status.CurrentStepState != string(common.StepStateFailed) {
		t.Fatalf("status = %+v, want current latest checkpoint step %s", status, latestStepID)
	}
}

func TestReadRunStatusPrefersBlockingDecisionStepOverStaleCheckpoint(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-status-blocking-step")
	runID := common.RunID("run-status-blocking-step")
	staleStepID := common.StepID("run-status-blocking-step/graph-001/001-stale")
	blockingStepID := common.StepID("run-status-blocking-step/provider-retry-010-of-repair-001")
	ticketID := common.TicketID("ticket-run-status-blocking-step")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{ID: staleStepID, RunID: runID, Kind: "patch/apply", State: common.StepStateSucceeded}); err != nil {
		t.Fatalf("create stale step: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{ID: blockingStepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateFailed}); err != nil {
		t.Fatalf("create blocking step: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:               "run-status-blocking-step/checkpoint/stale",
		WorkspaceID:      workspaceID,
		RunID:            runID,
		CurrentStepID:    staleStepID,
		RunStateSnapshot: common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
		OccurredAt:       "2026-05-12T11:20:53Z",
	}); err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	if err := exports.Repositories.Tickets.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      blockingStepID,
		Reason:      "provider retry exhausted",
		State:       common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	status, err := ReadRunStatusWithSession(ctx, session, string(runID))
	if err != nil {
		t.Fatalf("ReadRunStatusWithSession: %v", err)
	}
	if status.CurrentStepID != string(blockingStepID) || status.CurrentStepKind != "llm/workspace_execution" || status.CurrentStepState != string(common.StepStateFailed) {
		t.Fatalf("status = %+v, want blocking decision step %s", status, blockingStepID)
	}

	snapshot, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	if snapshot.Step == nil || snapshot.Step.ID != string(blockingStepID) {
		t.Fatalf("snapshot step = %+v, want blocking decision step %s", snapshot.Step, blockingStepID)
	}
	if snapshot.TaskProgress.Recovery.StepID != string(blockingStepID) {
		t.Fatalf("recovery = %+v, want blocking decision step %s", snapshot.TaskProgress.Recovery, blockingStepID)
	}
}

func TestRuntimeTimelineUsesStepStateForStreamItemsWhenRunBlockedOnDecision(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-stream-blocked-timeline")
	runID := common.RunID("run-stream-blocked-timeline")
	stepID := "run-stream-blocked-timeline/zz-llm-repair-step"
	timeline := readmodelprojection.RuntimeTimeline(RunSnapshot{
		Run: Run{
			ID:          string(runID),
			WorkspaceID: string(workspaceID),
			State:       string(common.RunStateBlockedOnDecision),
		},
		Steps: []presenter.StepView{{
			ID:    stepID,
			Kind:  "llm/workspace_execution",
			State: string(common.StepStateSucceeded),
			Output: map[string]any{
				"stage": "llm_execution_graph_plan",
				"streamEvents": []map[string]any{
					{"type": "delta", "delta": `{"completionState":"continue"`},
					{"type": "done"},
				},
			},
		}},
		EventRecords: []EventRecord{{
			ID:          "evt-stream-progress",
			Topic:       "/run/llm_stream/progress",
			SubjectType: "llm_stream_progress",
			SubjectID:   stepID,
			WorkspaceID: string(workspaceID),
			RunID:       string(runID),
			StepID:      stepID,
			Payload: mustReadModelEventPayload(t, map[string]any{
				recordpayload.PayloadKeyWorkspaceID:   string(workspaceID),
				recordpayload.PayloadKeyRunID:         string(runID),
				recordpayload.PayloadKeyStepID:        stepID,
				recordpayload.PayloadKeyProviderID:    "provider-a",
				recordpayload.PayloadKeyModel:         "model-a",
				recordpayload.PayloadKeyChunkCount:    2,
				recordpayload.PayloadKeyReceivedBytes: 28,
				recordpayload.PayloadKeyElapsedMs:     0,
				recordpayload.PayloadKeyStatus:        "completed",
				recordpayload.PayloadKeySummary:       "llm_execution_graph_plan - completed",
			}),
		}},
	})

	var checked int
	for _, item := range timeline {
		switch item.Source {
		case "step", "step_stream", "llm_stream_progress_event":
			checked++
			if item.RunState == string(common.RunStateBlockedOnDecision) {
				t.Fatalf("timeline item = %+v, want step/stream item not to inherit final blocked run state", item)
			}
			if item.RunState != string(common.StepStateSucceeded) {
				t.Fatalf("timeline item = %+v, want succeeded display state", item)
			}
		}
	}
	if checked != 4 {
		t.Fatalf("checked timeline items = %d, want step + two stream chunks + progress event", checked)
	}
}

func TestRuntimeTimelineUsesStreamDiagnosticsWhenRawEventsAreRefOnly(t *testing.T) {
	stepID := "run-stream-diagnostics/llm-1"
	timeline := readmodelprojection.RuntimeTimeline(RunSnapshot{
		Run: Run{ID: "run-stream-diagnostics", WorkspaceID: "ws-stream-diagnostics", State: string(common.RunStateActive)},
		Steps: []presenter.StepView{{
			ID:    stepID,
			Kind:  "llm/workspace_execution",
			State: string(common.StepStateSucceeded),
			Output: map[string]any{
				"stage": "llm_execution_graph_plan",
				"llmCallRefs": map[string]any{
					"streamRef": "workorch://workspaces/ws-stream-diagnostics/runs/run-stream-diagnostics/llm-calls/step/response.sse",
				},
				"streamDiagnostics": map[string]any{
					"streaming":          true,
					"receivedEventCount": 3,
					"receivedDeltaCount": 2,
					"receivedBytes":      28,
					"doneReceived":       true,
				},
			},
		}},
	})

	var found *RuntimeTimelineItem
	for idx := range timeline {
		if timeline[idx].Source == "step_stream_summary" {
			found = &timeline[idx]
			break
		}
	}
	if found == nil || found.Summary != "received 28 stream bytes" || found.Data["receivedEventCount"] != 3 {
		t.Fatalf("stream diagnostics timeline item = %+v, want bounded diagnostics summary", found)
	}
}

func TestRuntimeTimelineCompactsLargeStreamDeltaOutput(t *testing.T) {
	stepID := "run-large-stream/step-1"
	streamEvents := make([]map[string]any, 0, 80)
	for idx := 0; idx < 80; idx++ {
		streamEvents = append(streamEvents, map[string]any{
			"type":  "delta",
			"delta": "chunk-" + strconv.Itoa(idx+1),
		})
	}
	timeline := readmodelprojection.RuntimeTimeline(RunSnapshot{
		Run: Run{ID: "run-large-stream", WorkspaceID: "ws-large-stream", State: string(common.RunStateActive)},
		Steps: []presenter.StepView{{
			ID:    stepID,
			Kind:  "llm/workspace_execution",
			State: string(common.StepStateSucceeded),
			Output: map[string]any{
				"stage":        "llm_execution_graph_plan",
				"streamEvents": streamEvents,
			},
		}},
	})

	var streamCount int
	var omitted *RuntimeTimelineItem
	for idx := range timeline {
		item := timeline[idx]
		if item.Source != "step_stream" {
			continue
		}
		streamCount++
		if item.Type == "omitted" {
			omitted = &timeline[idx]
		}
		if item.RunState == string(common.RunStateActive) {
			t.Fatalf("stream timeline item = %+v, want step display state", item)
		}
	}
	want := 25
	if streamCount != want {
		t.Fatalf("stream timeline count = %d, want compact count %d", streamCount, want)
	}
	if omitted == nil || omitted.Data["omitted"] != 56 || omitted.Data["total"] != 80 {
		t.Fatalf("omitted item = %+v, want compact omission metadata", omitted)
	}
}

func TestReadRunTaskProgressWithBackendReturnsFormalResultProgress(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-task-progress-result")
	runID := common.RunID("run-task-progress-result")
	stepID := common.StepID("run-task-progress-result/step-1")
	failedStepID := common.StepID("run-task-progress-result/step-failed-before-repair")
	resultID := common.ResultID("result-task-progress")
	artifactID := common.ArtifactID("artifact-task-progress")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "task progress", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateSucceeded}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{ID: failedStepID, RunID: runID, Kind: "patch/apply", State: common.StepStateFailed}); err != nil {
		t.Fatalf("create failed historical step: %v", err)
	}
	if err := exports.Repositories.ValidationResults.Create(ctx, &runmodel.ValidationResult{
		ID:          common.ValidationResultID("validation-task-progress"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		State:       common.ValidationResultStatePassed,
		Summary:     "validation passed",
	}); err != nil {
		t.Fatalf("create validation: %v", err)
	}
	if err := exports.Repositories.ValidationResults.Create(ctx, &runmodel.ValidationResult{
		ID:          common.ValidationResultID("validation-task-progress-failed-before-repair"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      failedStepID,
		State:       common.ValidationResultStateFailed,
		Summary:     "historical repair validation failed",
	}); err != nil {
		t.Fatalf("create historical failed validation: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:           resultID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		State:        common.ResultStateAccepted,
		Title:        "Accepted Result",
		Summary:      "formal task outcome",
		EvidenceRefs: []string{string(stepID), "validation-task-progress"},
		ArtifactRefs: []common.ArtifactID{artifactID},
	}); err != nil {
		t.Fatalf("create result: %v", err)
	}
	if err := exports.Repositories.Artifacts.Create(ctx, &resultmodel.Artifact{
		ID:           artifactID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		ResultID:     resultID,
		ArtifactType: "report",
		State:        common.ArtifactStateAvailable,
		Summary:      "formal artifact",
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}

	progress, err := ReadRunTaskProgressWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunTaskProgressWithBackend: %v", err)
	}
	if progress.WorkspaceID != string(workspaceID) || progress.RunID != string(runID) || progress.State != "completed" {
		t.Fatalf("progress identity = %+v, want completed run progress", progress)
	}
	if progress.Recovery.Mode != "none" || progress.Recovery.NextAction != "inspect_result" {
		t.Fatalf("recovery = %+v, want inspect completed result", progress.Recovery)
	}
	if progress.Summary != "formal task outcome" {
		t.Fatalf("summary = %q, want formal task outcome", progress.Summary)
	}
	snapshot, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	if snapshot.Outcome.State != "accepted" || !snapshot.Outcome.Accepted || snapshot.Outcome.ResultID != string(resultID) {
		t.Fatalf("outcome = %+v, want accepted result outcome", snapshot.Outcome)
	}
	if !progressContainsMilestone(progress, "formal_result", "completed") {
		t.Fatalf("milestones = %+v, want completed formal result", progress.Milestones)
	}
	if !progressContainsMilestone(progress, "execution_steps", "completed") || !progressContainsMilestone(progress, "validation", "completed") {
		t.Fatalf("milestones = %+v, want accepted result to complete execution and validation milestones", progress.Milestones)
	}
	if !progressContainsMilestone(progress, "governance", "completed") {
		t.Fatalf("milestones = %+v, want accepted result to complete governance milestone", progress.Milestones)
	}
	if !progressContainsItem(progress, string(failedStepID), "step", "recovered") ||
		!progressContainsItem(progress, "validation-task-progress-failed-before-repair", "validation_result", "recovered") {
		t.Fatalf("items = %+v, want historical failed step and validation marked recovered", progress.Items)
	}
	if !progressContainsItem(progress, string(resultID), "result", "accepted") || !progressContainsItem(progress, string(artifactID), "artifact", "available") {
		t.Fatalf("items = %+v, want result and artifact items", progress.Items)
	}
}

func TestMapLLMUsageTracesFromEventRecordsNormalizesUsageAndCostSemantics(t *testing.T) {
	records := []EventRecord{
		{
			ID:         "evt-1",
			Topic:      "/run/llm_usage/recorded",
			OccurredAt: "2026-05-07T10:00:00Z",
			RunID:      "run-1",
			StepID:     "run-1/step-1",
			Payload: mustReadModelEventPayload(t, map[string]any{
				recordpayload.PayloadKeyWorkspaceID:  "ws-1",
				recordpayload.PayloadKeyRunID:        "run-1",
				recordpayload.PayloadKeyStepID:       "run-1/step-1",
				recordpayload.PayloadKeyProviderID:   "provider-a",
				recordpayload.PayloadKeyProvider:     "compatible",
				recordpayload.PayloadKeyModel:        "model-a",
				recordpayload.PayloadKeyOperation:    "chat_completions",
				recordpayload.PayloadKeyInputTokens:  120,
				recordpayload.PayloadKeyOutputTokens: 30,
				recordpayload.PayloadKeyTotalTokens:  150,
				recordpayload.PayloadKeyEstimated:    true,
				recordpayload.PayloadKeyCacheUsage: map[string]any{
					"estimated":               true,
					"source":                  "cache_plan_estimated",
					"cacheReadTokens":         0,
					"cacheWriteTokens":        0,
					"cacheCreationTokens":     0,
					"cacheableInputTokens":    90,
					"nonCacheableInputTokens": 30,
				},
				recordpayload.PayloadKeyStatus: "completed",
			}),
		},
		{
			ID:         "evt-2",
			Topic:      "/run/llm_usage/recorded",
			OccurredAt: "2026-05-07T10:00:01Z",
			RunID:      "run-1",
			StepID:     "run-1/step-2",
			Payload: mustReadModelEventPayload(t, map[string]any{
				recordpayload.PayloadKeyWorkspaceID:  "ws-1",
				recordpayload.PayloadKeyRunID:        "run-1",
				recordpayload.PayloadKeyStepID:       "run-1/step-2",
				recordpayload.PayloadKeyProviderID:   "provider-b",
				recordpayload.PayloadKeyProvider:     "compatible",
				recordpayload.PayloadKeyModel:        "model-b",
				recordpayload.PayloadKeyOperation:    "chat_completions",
				recordpayload.PayloadKeyInputTokens:  60,
				recordpayload.PayloadKeyOutputTokens: 10,
				recordpayload.PayloadKeyTotalTokens:  70,
				recordpayload.PayloadKeyEstimated:    false,
				recordpayload.PayloadKeyCacheUsage: map[string]any{
					"estimated":           false,
					"source":              llmcommon.UsageSourceProviderNative,
					"cacheReadTokens":     20,
					"cacheWriteTokens":    8,
					"cacheCreationTokens": 8,
				},
				recordpayload.PayloadKeyStatus: "completed",
			}),
		},
	}

	traces := readmodelprojection.MapLLMUsageTracesFromEventRecords(records, "run-1")
	if len(traces) != 2 {
		t.Fatalf("llm usage traces length = %d, want 2", len(traces))
	}
	if traces[0].UsageSource != llmcommon.UsageSourceEstimated ||
		!traces[0].Estimated {
		t.Fatalf("trace[0] = %+v, want normalized estimated usage semantics", traces[0])
	}
	if traces[1].UsageSource != llmcommon.UsageSourceProviderNative ||
		traces[1].Estimated {
		t.Fatalf("trace[1] = %+v, want normalized provider_native usage semantics", traces[1])
	}
	cacheReadTokens, _ := traces[1].CacheUsage.Int64("cacheReadTokens")
	if cacheReadTokens != 20 {
		t.Fatalf("trace[1].CacheUsage = %+v, want provider cache usage", traces[1].CacheUsage)
	}

	summary := readmodelprojection.SummarizeLLMUsage(traces)
	if summary.EntryTotal != 2 ||
		summary.EstimatedEntries != 1 ||
		summary.ProviderNativeEntries != 1 {
		t.Fatalf("summary = %+v, want entry=2 estimated=1 native=1", summary)
	}
	if summary.CacheReadTokens != 20 ||
		summary.CacheWriteTokens != 8 ||
		summary.CacheCreationTokens != 8 ||
		summary.CacheableInputTokens != 90 ||
		summary.NonCacheableInputTokens != 30 ||
		summary.CacheEstimatedEntries != 1 ||
		summary.CacheProviderEntries != 1 {
		t.Fatalf("summary = %+v, want aggregated cache usage", summary)
	}
	if summary.CacheReadInputRatio < 0.11 || summary.CacheReadInputRatio > 0.12 ||
		summary.CacheableInputRatio != 0.5 ||
		summary.NonCacheableInputRatio < 0.16 || summary.NonCacheableInputRatio > 0.17 {
		t.Fatalf("summary cache ratios = read:%v cacheable:%v nonCacheable:%v, want aggregated ratios", summary.CacheReadInputRatio, summary.CacheableInputRatio, summary.NonCacheableInputRatio)
	}
	if len(summary.ProviderModelBreakdown) != 2 {
		t.Fatalf("summary breakdown = %+v, want 2 provider/model entries", summary.ProviderModelBreakdown)
	}
	if summary.ProviderModelBreakdown[0].ProviderID != "provider-a" ||
		summary.ProviderModelBreakdown[0].Model != "model-a" ||
		summary.ProviderModelBreakdown[0].EntryTotal != 1 ||
		summary.ProviderModelBreakdown[1].ProviderID != "provider-b" ||
		summary.ProviderModelBreakdown[1].Model != "model-b" ||
		summary.ProviderModelBreakdown[1].EntryTotal != 1 ||
		summary.ProviderModelBreakdown[1].CacheReadTokens != 20 ||
		summary.ProviderModelBreakdown[1].CacheWriteTokens != 8 ||
		summary.ProviderModelBreakdown[1].CacheReadInputRatio < 0.33 ||
		summary.ProviderModelBreakdown[1].CacheReadInputRatio > 0.34 {
		t.Fatalf("summary breakdown = %+v, want provider-a/model-a and provider-b/model-b", summary.ProviderModelBreakdown)
	}
}

func TestMapProjectedEventRecordsKeepsBoundedMetadataPayload(t *testing.T) {
	items := []*eventstreammodel.EventRecord{{
		ID:          common.EventRecordID("event-projection"),
		Topic:       runevents.TopicRunStateChanged,
		OccurredAt:  "2026-05-16T10:00:00Z",
		WorkspaceID: common.WorkspaceID("ws-projection"),
		RunID:       common.RunID("run-projection"),
		StepID:      common.StepID("run-projection/step-1"),
		Payload: mustAppserviceGovernancePayload(t, map[string]any{
			recordpayload.PayloadKeyState:        "running",
			recordpayload.PayloadKeySummary:      strings.Repeat("s", eventPayloadStringLimit+20),
			recordpayload.PayloadKeyEvidenceRefs: []any{"ref-1", "ref-2"},
			recordpayload.PayloadKeyOutput:       map[string]any{"raw": strings.Repeat("x", eventPayloadStringLimit*4)},
			recordpayload.PayloadKeyContext:      map[string]any{"raw": "hidden"},
		}),
	}}

	projected := readmodelprojection.MapProjectedEventRecords(items)

	if len(projected) != 1 {
		t.Fatalf("projected = %+v, want one event", projected)
	}
	payload := projected[0].Payload.Map()
	if payload[recordpayload.PayloadKeyState] != "running" {
		t.Fatalf("payload state = %v, want running", payload[recordpayload.PayloadKeyState])
	}
	summary, ok := payload[recordpayload.PayloadKeySummary].(string)
	if !ok || len(summary) > eventPayloadStringLimit+3 || !strings.HasSuffix(summary, "...") {
		t.Fatalf("summary projection = %q, want bounded string with suffix", summary)
	}
	if _, ok := payload[recordpayload.PayloadKeyOutput]; ok {
		t.Fatalf("projected payload includes raw output: %+v", payload)
	}
	if _, ok := payload[recordpayload.PayloadKeyContext]; ok {
		t.Fatalf("projected payload includes raw context: %+v", payload)
	}
	refs, ok := payload[recordpayload.PayloadKeyEvidenceRefs].([]any)
	if !ok || len(refs) != 2 {
		t.Fatalf("evidence refs = %#v, want projected refs", payload[recordpayload.PayloadKeyEvidenceRefs])
	}
}

func TestMapEventRecordsPreservesWorkspacePlanStepPayload(t *testing.T) {
	items := []*eventstreammodel.EventRecord{{
		ID:          common.EventRecordID("event-step-alias"),
		Topic:       "/step/state/changed",
		OccurredAt:  "2026-05-16T10:00:00Z",
		WorkspaceID: common.WorkspaceID("ws-step-alias"),
		RunID:       common.RunID("run-step-alias"),
		StepID:      common.StepID("run-step-alias/step-1"),
		SubjectType: "step",
		Payload: mustAppserviceGovernancePayload(t, map[string]any{
			recordpayload.PayloadKeyKind:  llmcommon.StepKindWorkspacePlan,
			recordpayload.PayloadKeyState: string(common.StepStateSucceeded),
			recordpayload.PayloadKeyOutput: map[string]any{
				"executionGraph": map[string]any{
					"nodes": []any{map[string]any{
						"id":   "plan-next",
						"kind": llmcommon.StepKindWorkspacePlan,
					}},
				},
			},
		}),
	}}

	mapped := readmodelprojection.MapEventRecords(items)

	if len(mapped) != 1 {
		t.Fatalf("mapped = %+v, want one event", mapped)
	}
	payload := mapped[0].Payload.Map()
	if payload[recordpayload.PayloadKeyKind] != llmcommon.StepKindWorkspacePlan {
		t.Fatalf("payload kind = %v, want %s", payload[recordpayload.PayloadKeyKind], llmcommon.StepKindWorkspacePlan)
	}
	output := readModelMapValue(payload[recordpayload.PayloadKeyOutput])
	graph := readModelMapValue(output["executionGraph"])
	nodes := readModelMapSliceValue(graph["nodes"])
	if len(nodes) != 1 || nodes[0]["kind"] != llmcommon.StepKindWorkspacePlan {
		t.Fatalf("executionGraph nodes = %+v, want original workspace plan kind", nodes)
	}
}

func TestSummarizeLLMUsageMergesProviderModelBreakdownWithoutCost(t *testing.T) {
	summary := readmodelprojection.SummarizeLLMUsage([]LLMUsageTrace{
		{
			ProviderID:   "provider-a",
			Provider:     "compatible",
			APISurface:   llmcommon.ProviderSurfaceOpenAICompatibleHTTP,
			Model:        "model-a",
			InputTokens:  100,
			OutputTokens: 10,
			TotalTokens:  110,
			Status:       "completed",
			UsageSource:  llmcommon.UsageSourceEstimated,
			Estimated:    true,
			CacheMarkers: mustReadModelEventPayload(t, map[string]any{
				"providerMode":              llmcommon.ProviderCacheModeAutoPrefix,
				"autoPrefixCacheExpected":   true,
				"renderedStablePrefixBytes": 1024,
			}),
		},
		{
			ProviderID:   "provider-a",
			Provider:     "compatible",
			APISurface:   llmcommon.ProviderSurfaceOpenAICompatibleHTTP,
			Model:        "model-a",
			InputTokens:  200,
			OutputTokens: 20,
			TotalTokens:  220,
			Status:       "completed",
			UsageSource:  llmcommon.UsageSourceProviderNative,
			CacheUsage:   mustReadModelEventPayload(t, map[string]any{"cacheReadTokens": 20, "cacheCreationTokens": 8}),
			CacheMarkers: mustReadModelEventPayload(t, map[string]any{
				"providerMode":              llmcommon.ProviderCacheModeAutoPrefix,
				"autoPrefixCacheExpected":   true,
				"renderedStablePrefixBytes": 1024,
			}),
		},
	})

	if len(summary.ProviderModelBreakdown) != 1 {
		t.Fatalf("provider/model breakdown = %+v, want merged provider/model entry", summary.ProviderModelBreakdown)
	}
	item := summary.ProviderModelBreakdown[0]
	if item.APISurface != llmcommon.ProviderSurfaceOpenAICompatibleHTTP ||
		item.ProviderMode != llmcommon.ProviderCacheModeAutoPrefix ||
		item.EntryTotal != 2 ||
		item.InputTokens != 300 ||
		item.OutputTokens != 30 ||
		item.TotalTokens != 330 ||
		item.EstimatedEntries != 1 ||
		item.CacheReadTokens != 20 ||
		item.CacheCreationTokens != 8 ||
		item.CacheablePromptCount != 2 ||
		item.ActualCacheHitCount != 1 ||
		item.CacheCreationCount != 1 ||
		item.AutoPrefixExpectedCount != 2 {
		t.Fatalf("provider/model breakdown = %+v, want merged token counters", summary.ProviderModelBreakdown)
	}
}

func TestSummarizeLLMUsageDoesNotMergeDifferentSurfacesOrProviderModes(t *testing.T) {
	summary := readmodelprojection.SummarizeLLMUsage([]LLMUsageTrace{
		{
			ProviderID: "provider-a",
			Provider:   "compatible",
			APISurface: llmcommon.ProviderSurfaceOpenAICompatibleHTTP,
			Model:      "model-a",
			CacheMarkers: mustReadModelEventPayload(t, map[string]any{
				"providerMode": llmcommon.ProviderCacheModeAutoPrefix,
			}),
		},
		{
			ProviderID: "provider-a",
			Provider:   "compatible",
			APISurface: llmcommon.ProviderSurfaceAnthropicCompatibleHTTP,
			Model:      "model-a",
			CacheMarkers: mustReadModelEventPayload(t, map[string]any{
				"providerMode": llmcommon.ProviderCacheModeNone,
			}),
		},
	})

	if len(summary.ProviderModelBreakdown) != 2 {
		t.Fatalf("provider/model breakdown = %+v, want distinct surface/mode entries", summary.ProviderModelBreakdown)
	}
	if summary.ProviderModelBreakdown[0].APISurface == summary.ProviderModelBreakdown[1].APISurface {
		t.Fatalf("provider/model breakdown = %+v, want surface-specific entries", summary.ProviderModelBreakdown)
	}
}

func TestReadRunSnapshotWithBackendProjectsCompletedRunWithoutResultAsFailedOutcome(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-completed-without-result")
	runID := common.RunID("run-completed-without-result")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "completed without result",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	snapshot, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	if snapshot.Outcome.State != "failed" || snapshot.Outcome.Accepted || !snapshot.Outcome.Terminal {
		t.Fatalf("outcome = %+v, want failed terminal outcome without accepted result", snapshot.Outcome)
	}
	if snapshot.Outcome.Reason != "run completed without accepted result" {
		t.Fatalf("outcome reason = %q, want missing accepted result reason", snapshot.Outcome.Reason)
	}
	if !progressContainsMilestone(snapshot.TaskProgress, "formal_result", "failed") {
		t.Fatalf("milestones = %+v, want failed formal result milestone", snapshot.TaskProgress.Milestones)
	}
}

func TestReadRunTaskProgressWithBackendReturnsDecisionRecovery(t *testing.T) {
	dataRoot := t.TempDir()
	runID := "run-task-progress-decision"
	started := prepareDecisionRun(t, dataRoot, "ws-task-progress-decision", runID)

	progress, err := ReadRunTaskProgressWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("ReadRunTaskProgressWithBackend: %v", err)
	}
	if progress.Recovery.Mode != "wait_decision" || progress.Recovery.BlockingTicketID != started.BlockingTicketID {
		t.Fatalf("recovery = %+v, want decision blocker %s", progress.Recovery, started.BlockingTicketID)
	}
	if len(progress.Blockers) != 1 || progress.Blockers[0].Kind != "decision" || progress.Blockers[0].ID != started.BlockingTicketID {
		t.Fatalf("blockers = %+v, want decision blocker %s", progress.Blockers, started.BlockingTicketID)
	}
	if !progressContainsMilestone(progress, "governance", "blocked") {
		t.Fatalf("milestones = %+v, want blocked governance", progress.Milestones)
	}
}

func TestReadRunTaskProgressSummaryCountsAppliedPatchesAsChangedFiles(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)
	workspaceID := common.WorkspaceID("ws-task-progress-patches")
	runID := common.RunID("run-task-progress-patches")
	stepID := common.StepID("run-task-progress-patches/graph-001/001-apply-main")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "patch/apply", State: common.StepStateSucceeded}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := exports.Repositories.Patches.Create(ctx, &runmodel.Patch{
		ID:          common.PatchID("run-task-progress-patches/patch/1"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		PatchType:   "unified_diff",
		TargetRef:   "main.go",
		State:       common.PatchStateApplied,
	}); err != nil {
		t.Fatalf("create patch: %v", err)
	}

	progress, err := ReadRunTaskProgressWithSession(ctx, session, string(runID))
	if err != nil {
		t.Fatalf("ReadRunTaskProgressWithSession: %v", err)
	}
	if !strings.Contains(progress.Summary, "1 changed files") {
		t.Fatalf("summary = %q, want changed file count from applied patches", progress.Summary)
	}
	if !progressContainsItem(progress, "run-task-progress-patches/patch/1", "patch", string(common.PatchStateApplied)) {
		t.Fatalf("items = %+v, want patch progress item", progress.Items)
	}
}

func TestMapTaskProgressProjectsRepairLoopState(t *testing.T) {
	snapshot := RunSnapshot{
		Workspace: Workspace{ID: "ws-repair-loop"},
		Run: Run{
			ID:          "run-repair-loop",
			WorkspaceID: "ws-repair-loop",
			State:       string(common.RunStateActive),
		},
		Steps: []presenter.StepView{
			{
				ID:    "run-repair-loop/block/run-tests",
				Kind:  "shell/command",
				State: string(common.StepStateFailed),
				Output: map[string]any{
					"failureClass":     "shell_command_failed",
					"failureSignature": "kind=shell/command",
				},
			},
			{
				ID:    "run-repair-loop/zz-llm-repair-run-tests-1",
				Kind:  "llm/workspace_execution",
				State: string(common.StepStateSucceeded),
				Input: map[string]any{
					"runtimeRepairFollowUp": true,
					"runtimeRepairAttempt":  1,
				},
			},
			{
				ID:    "run-repair-loop/zz-llm-repair-run-tests-2",
				Kind:  "llm/workspace_execution",
				State: string(common.StepStateExecuting),
				Input: map[string]any{
					"runtimeRepairFollowUp": true,
					"runtimeRepairAttempt":  2,
					"failureTargetScope":    "target.txt",
					"structuredFailureSignals": map[string]any{
						"targetRefs":  []string{"target.txt"},
						"identifiers": []string{"MissingThing"},
					},
					"repairInputCompaction": map[string]any{
						"mode":          "repair_action_package_primary",
						"omittedFields": []string{"failedStepOutput", "previousStepOutput"},
					},
					"repairActionPackage": map[string]any{
						"targetRelevanceDiagnostics": map[string]any{
							"failureTargetScope": "target.txt",
						},
						"repeatedTargetRepairMode": map[string]any{
							"enabled": true,
							"reason":  "same_target_scope_repeated_failure",
						},
					},
				},
			},
		},
		RuntimeCheckpoints: []RuntimeCheckpoint{
			{
				ID:            "checkpoint-repair-failure-2",
				RunID:         "run-repair-loop",
				CurrentStepID: "run-repair-loop/block/run-tests",
				OccurredAt:    "2026-05-06T10:00:02Z",
				Data: map[string]any{
					"event":            "step_failed",
					"failureClass":     "shell_command_failed",
					"failureSignature": "kind=shell/command",
				},
			},
		},
	}

	progress := readmodelprojection.MapLocalizedTaskProgress(snapshot)
	if progress.RepairLoop.State != "repairing" ||
		progress.RepairLoop.AttemptCount != 2 ||
		progress.RepairLoop.LastAttempt != 2 ||
		progress.RepairLoop.LastRepairStepID != "run-repair-loop/zz-llm-repair-run-tests-2" ||
		progress.RepairLoop.LastFailureClass != "shell_command_failed" ||
		progress.RepairLoop.LastFailureSignature != "kind=shell/command" ||
		progress.RepairLoop.LastFailureStepID != "run-repair-loop/block/run-tests" ||
		progress.RepairLoop.NextAction != "resume_run" ||
		progress.RepairLoop.LastFailureTargetScope != "target.txt" ||
		progress.RepairLoop.RepeatedFailureReason != "same_target_scope_repeated_failure" ||
		progress.RepairLoop.LastRepairInputBytes == 0 {
		t.Fatalf("repairLoop = %+v, want repairing projection with attempts/failure/next action", progress.RepairLoop)
	}
	if len(progress.RepairLoop.StructuredFailureSignals) == 0 || len(progress.RepairLoop.RepairInputCompaction) == 0 {
		t.Fatalf("repairLoop = %+v, want repair grounding observability", progress.RepairLoop)
	}
}

func TestMapTaskProgressRepairLoopSignalsPreferCurrentFailureStep(t *testing.T) {
	snapshot := RunSnapshot{
		Workspace: Workspace{ID: "ws-current-failure-signals"},
		Run: Run{
			ID:          "run-current-failure-signals",
			WorkspaceID: "ws-current-failure-signals",
			State:       string(common.RunStateActive),
		},
		Steps: []presenter.StepView{
			{
				ID:    "run-current-failure-signals/graph-002/patch",
				Kind:  "patch/apply",
				State: string(common.StepStateFailed),
				Output: map[string]any{
					"stage":            "patch_apply",
					"error":            "invalid argument: unified diff hunk context mismatch",
					"targetRef":        "target.txt",
					"failureClass":     "patch_apply_failed",
					"failureSignature": "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|targetRef=target.txt|error=context mismatch",
				},
			},
			{
				ID:    "run-current-failure-signals/repair-003",
				Kind:  "llm/workspace_execution",
				State: string(common.StepStateExecuting),
				Input: map[string]any{
					"runtimeRepairFollowUp": true,
					"runtimeRepairAttempt":  3,
					"structuredFailureSignals": map[string]any{
						"messages":    []string{"state-changing llm repair step returned executionGraph without patch/apply"},
						"identifiers": []string{"llm_execution_graph_plan"},
					},
					"repairActionPackage": map[string]any{
						"structuredFailureSignals": map[string]any{
							"messages": []string{"historical graph contract violation"},
						},
					},
				},
			},
		},
		RuntimeCheckpoints: []RuntimeCheckpoint{
			{
				ID:            "checkpoint-current-patch-failure",
				RunID:         "run-current-failure-signals",
				CurrentStepID: "run-current-failure-signals/graph-002/patch",
				OccurredAt:    "2026-05-06T10:00:03Z",
				Data: map[string]any{
					"event":            "step_failed",
					"failureClass":     "patch_apply_failed",
					"failureSignature": "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|targetRef=target.txt|error=context mismatch",
				},
			},
		},
	}

	progress := readmodelprojection.MapLocalizedTaskProgress(snapshot)
	signals := progress.RepairLoop.StructuredFailureSignals
	messages := readModelStringSliceValue(signals["messages"])
	foundCurrentPatchMessage := false
	for _, message := range messages {
		if strings.Contains(message, "invalid argument: unified diff hunk context mismatch") {
			foundCurrentPatchMessage = true
			break
		}
	}
	if progress.RepairLoop.LastFailureClass != "patch_apply_failed" ||
		progress.RepairLoop.LastFailureTargetScope != "target.txt" ||
		!foundCurrentPatchMessage {
		t.Fatalf("repairLoop = %+v signals=%+v, want current patch failure signals", progress.RepairLoop, signals)
	}
	for _, message := range messages {
		if strings.Contains(message, "executionGraph without patch/apply") || strings.Contains(message, "historical graph") {
			t.Fatalf("signals = %+v, should not expose stale repair-step graph violation", signals)
		}
	}
}

func TestMapTaskProgressProjectsRepairLoopEscalation(t *testing.T) {
	snapshot := RunSnapshot{
		Workspace: Workspace{ID: "ws-repair-escalation"},
		Run: Run{
			ID:               "run-repair-escalation",
			WorkspaceID:      "ws-repair-escalation",
			State:            string(common.RunStateBlockedOnDecision),
			BlockingTicketID: "ticket-repair-exhausted",
		},
		Steps: []presenter.StepView{
			{
				ID:    "run-repair-escalation/zz-llm-repair-run-tests-1",
				Kind:  "llm/workspace_execution",
				State: string(common.StepStateSucceeded),
				Input: map[string]any{
					"runtimeRepairFollowUp": true,
					"runtimeRepairAttempt":  1,
				},
			},
			{
				ID:    "run-repair-escalation/zz-llm-repair-run-tests-2",
				Kind:  "llm/workspace_execution",
				State: string(common.StepStateSucceeded),
				Input: map[string]any{
					"runtimeRepairFollowUp": true,
					"runtimeRepairAttempt":  2,
				},
			},
		},
		Decisions: []Decision{
			{
				ID:          "ticket-repair-exhausted",
				WorkspaceID: "ws-repair-escalation",
				RunID:       "run-repair-escalation",
				Reason:      "runtime repair attempts exhausted after 2 attempts",
				State:       string(common.DecisionTicketStateOpen),
				Context: mustAppserviceGovernancePayload(t, map[string]any{
					"attempts":         2,
					"failureClass":     "shell_command_failed",
					"failureSignature": "kind=shell/command",
				}),
			},
		},
	}

	progress := readmodelprojection.MapLocalizedTaskProgress(snapshot)
	if progress.RepairLoop.State != "escalated_decision" ||
		!progress.RepairLoop.Escalated ||
		progress.RepairLoop.EscalationTicketID != "ticket-repair-exhausted" ||
		progress.RepairLoop.EscalationReason == "" ||
		progress.RepairLoop.AttemptCount != 2 ||
		progress.RepairLoop.NextAction != "resolve_decision_ticket" {
		t.Fatalf("repairLoop = %+v, want escalated decision projection", progress.RepairLoop)
	}
}

func TestMapClosureTracesProjectsGraphLimitAssessmentAndMissingDoD(t *testing.T) {
	snapshot := RunSnapshot{
		Workspace: Workspace{ID: "ws-closure"},
		Run: Run{
			ID:          "run-closure",
			WorkspaceID: "ws-closure",
			State:       string(common.RunStateActive),
		},
		Steps: []presenter.StepView{
			{
				ID:    "run-closure/zz-llm-after-graph-node",
				Kind:  "llm/workspace_execution",
				State: string(common.StepStateSucceeded),
				Input: map[string]any{
					"graphFollowUp": true,
				},
				Output: map[string]any{
					"graphFollowUpExpansionLimitExceeded": true,
					"graphFollowUpExpansionCount":         3,
					"graphFollowUpExpansionLimit":         2,
					"ignoredExecutionGraph":               true,
				},
			},
			{
				ID:    "run-closure/zz-llm-after-graph-limit-node-assessment",
				Kind:  "llm/workspace_execution",
				State: string(common.StepStateFailed),
				Input: map[string]any{
					"assessmentOnly":              true,
					"graphExpansionLimitFollowUp": true,
					"assessmentOnlyReason":        "graph_follow_up_expansion_limit_exceeded",
					"graphFollowUpExpansionCount": 3,
					"graphFollowUpExpansionLimit": 2,
				},
				Output: map[string]any{
					"runtimeDiagnostics": map[string]any{
						"kind": "dod_coverage_missing",
						"missingDoDItems": []map[string]any{
							{
								"kind":   "leaf",
								"item":   "missing item",
								"state":  "failed",
								"source": "dodAssessment",
							},
						},
					},
				},
			},
		},
	}

	snapshot.ClosureTraces = readmodelprojection.MapClosureTraces(snapshot)
	observability := readmodelprojection.MapRunObservability(snapshot)
	if len(snapshot.ClosureTraces) != 2 {
		t.Fatalf("closureTraces = %+v, want two traces", snapshot.ClosureTraces)
	}
	first := snapshot.ClosureTraces[0]
	if !first.GraphFollowUp || !first.GraphFollowUpExpansionLimitExceeded || first.GraphFollowUpExpansionCount != 3 || first.GraphFollowUpExpansionLimit != 2 || !first.IgnoredExecutionGraph {
		t.Fatalf("first closure trace = %+v, want graph expansion limit projection", first)
	}
	second := snapshot.ClosureTraces[1]
	if !second.AssessmentOnly || !second.GraphExpansionLimitFollowUp || second.Reason != "graph_follow_up_expansion_limit_exceeded" || len(second.MissingDoDItems) != 1 {
		t.Fatalf("second closure trace = %+v, want assessment-only missing DoD projection", second)
	}
	if len(observability.ClosureTraces) != len(snapshot.ClosureTraces) {
		t.Fatalf("observability closureTraces = %+v, want snapshot projection reused", observability.ClosureTraces)
	}
}

func TestReadWorkspaceTaskProgressWithBackendReturnsActiveRunProgress(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := "ws-task-progress-approval"
	runID := "run-task-progress-approval"
	started := prepareApprovalRun(t, dataRoot, workspaceID, runID)

	progress, err := ReadWorkspaceTaskProgressWithBackend(context.Background(), "file", dataRoot, workspaceID)
	if err != nil {
		t.Fatalf("ReadWorkspaceTaskProgressWithBackend: %v", err)
	}
	if progress.WorkspaceID != workspaceID || progress.RunID != runID {
		t.Fatalf("progress identity = %+v, want workspace active run progress", progress)
	}
	if progress.Recovery.Mode != "wait_approval" || progress.Recovery.BlockingApprovalID != started.BlockingApprovalID {
		t.Fatalf("recovery = %+v, want approval blocker %s", progress.Recovery, started.BlockingApprovalID)
	}
}

func TestReadRunContextBundleWithBackendReturnsRecoveryContextAndCompressionCandidates(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-context-bundle")
	runID := common.RunID("run-context-bundle")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "context bundle workspace",
		State:                common.WorkspaceStateActive,
		DeliverableDirectory: "workspaces/context-bundle",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
		RuntimeBootstrap: &orchestratormodel.RuntimeBootstrap{Activation: orchestratormodel.RuntimeActivationContext{
			Skills:       []orchestratormodel.RuntimeSkillContext{{ID: "skill-context", Name: "context", Instruction: "context"}},
			MCPServerIDs: []common.MCPServerID{"mcp-context"},
			Memories:     []orchestratormodel.RuntimeMemoryContext{{ID: "memory-context", Summary: "prior context"}},
		}},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for idx := 0; idx < 11; idx++ {
		stepID := common.StepID("run-context-bundle/step-" + strconv.Itoa(idx+1))
		if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
			ID:     stepID,
			RunID:  runID,
			Kind:   "llm/workspace_execution",
			State:  common.StepStateSucceeded,
			Output: mustAppserviceGovernancePayload(t, map[string]any{"stage": "context_bundle_stage"}),
		}); err != nil {
			t.Fatalf("create step: %v", err)
		}
		if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
			ID:               common.CheckpointID("checkpoint-context-bundle-" + strconv.Itoa(idx+1)),
			WorkspaceID:      workspaceID,
			RunID:            runID,
			CheckpointType:   common.CheckpointTypeRunState,
			CurrentStepID:    stepID,
			RunStateSnapshot: common.RunStateActive,
			OccurredAt:       "2026-04-29T10:00:" + twoDigitSecond(idx) + "Z",
			Data:             mustAppserviceGovernancePayload(t, map[string]any{"event": "step_completed"}),
		}); err != nil {
			t.Fatalf("create checkpoint: %v", err)
		}
	}

	bundle, err := ReadRunContextBundleWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunContextBundleWithBackend: %v", err)
	}
	if bundle.WorkspaceID != string(workspaceID) || bundle.RunID != string(runID) || bundle.Mode != "run_recovery" {
		t.Fatalf("bundle identity = %+v, want run recovery bundle", bundle)
	}
	if bundle.TaskProgress.RunID != string(runID) || bundle.Recovery.Mode != "resume" {
		t.Fatalf("bundle progress/recovery = %+v/%+v, want resume run progress", bundle.TaskProgress, bundle.Recovery)
	}
	if bundle.CompressedContext.Version != "workorch.compressedContext.v1" ||
		bundle.CompressedContext.Mode != "deterministic_recovery_summary" ||
		bundle.CompressedContext.RunID != string(runID) {
		t.Fatalf("compressedContext = %+v, want deterministic compressed context for run", bundle.CompressedContext)
	}
	if bundle.LatestCheckpoint == nil || bundle.LatestCheckpoint.ID != "checkpoint-context-bundle-11" {
		t.Fatalf("latest checkpoint = %+v, want checkpoint-context-bundle-11", bundle.LatestCheckpoint)
	}
	if !contextBundleContainsSection(bundle, "goal") || !contextBundleContainsSection(bundle, "dod") || !contextBundleContainsSection(bundle, "task_progress") {
		t.Fatalf("sections = %+v, want goal/dod/task_progress", bundle.Sections)
	}
	if len(bundle.RecentEvidence) != 12 {
		t.Fatalf("recentEvidence = %d, want 12", len(bundle.RecentEvidence))
	}
	if len(bundle.CompressedContext.RecentEvidence) != 6 {
		t.Fatalf("compressed recentEvidence = %d, want 6", len(bundle.CompressedContext.RecentEvidence))
	}
	snapshot, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	lightweight := readmodelprojection.CompressedContextFromLocalizedRunSnapshot(snapshot)
	wantLightweight := bundle.CompressedContext
	lightweight.SourceSectionRefs = nil
	wantLightweight.SourceSectionRefs = nil
	if !reflect.DeepEqual(lightweight, wantLightweight) {
		t.Fatalf("lightweight compressedContext = %+v, want full bundle compressed context %+v", lightweight, bundle.CompressedContext)
	}
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		t.Fatalf("readModelDependenciesFromSession: %v", err)
	}
	recoverySnapshot, err := readRunRecoverySnapshot(context.Background(), deps, runID)
	if err != nil {
		t.Fatalf("readRunRecoverySnapshot: %v", err)
	}
	recoveryCompressed := readmodelprojection.CompressedContextFromLocalizedRunSnapshot(recoverySnapshot)
	wantRecoveryCompressed := bundle.CompressedContext
	recoveryCompressed.SourceSectionRefs = nil
	wantRecoveryCompressed.SourceSectionRefs = nil
	if !reflect.DeepEqual(recoveryCompressed, wantRecoveryCompressed) {
		t.Fatalf("recovery compressedContext = %+v, want full bundle compressed context %+v", recoveryCompressed, bundle.CompressedContext)
	}
	if len(recoverySnapshot.TaskDecompositions) != 0 || len(recoverySnapshot.ExecutionSections) != 0 || len(recoverySnapshot.SelectedCapabilities) != 0 || len(recoverySnapshot.UsedCapabilities) != 0 {
		t.Fatalf("recovery snapshot loaded display projections: taskDecompositions=%d executionSections=%d selected=%d used=%d", len(recoverySnapshot.TaskDecompositions), len(recoverySnapshot.ExecutionSections), len(recoverySnapshot.SelectedCapabilities), len(recoverySnapshot.UsedCapabilities))
	}
	if !contextBundleContainsCompressionCandidate(bundle, "recent_evidence") || !contextBundleContainsCompressionCandidate(bundle, "task_progress") {
		t.Fatalf("compressionCandidates = %+v, want recent evidence and task progress candidates", bundle.CompressionCandidates)
	}
}

func TestReadRunTaskDecompositionsWithBackendReturnsExecutionGraphProjection(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-task-decomposition")
	runID := common.RunID("run-task-decomposition")
	rootStepID := common.StepID("run-task-decomposition/step-1")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "task decomposition", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    rootStepID,
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateSucceeded,
		Output: mustAppserviceGovernancePayload(t, map[string]any{
			"stage": "llm_execution_graph_plan",
			"executionGraph": map[string]any{
				"nodes": []map[string]any{
					{"id": "inspect", "kind": "shell/command", "input": map[string]any{"command": "hostname"}},
					{"id": "verify", "kind": "shell/command", "dependsOn": []string{"inspect"}, "input": map[string]any{"command": "true"}},
				},
			},
		}),
	}); err != nil {
		t.Fatalf("create root step: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    common.StepID("run-task-decomposition/block/inspect"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateSucceeded,
		Input: mustAppserviceGovernancePayload(t, map[string]any{
			"graphNodeId":     "inspect",
			"graphRootStepId": string(rootStepID),
		}),
		Output: mustAppserviceGovernancePayload(t, map[string]any{"exitCode": 0}),
	}); err != nil {
		t.Fatalf("create inspect step: %v", err)
	}
	if err := exports.Repositories.ToolCalls.Create(ctx, &runmodel.ToolCall{
		ID:          common.ToolCallID("tool-task-decomposition-inspect"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID("run-task-decomposition/block/inspect"),
		ToolName:    "shell/command",
		State:       common.ToolCallStateCompleted,
	}); err != nil {
		t.Fatalf("create tool call: %v", err)
	}
	if err := exports.Repositories.Patches.Create(ctx, &runmodel.Patch{
		ID:          common.PatchID("patch-task-decomposition-inspect"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID("run-task-decomposition/block/inspect"),
		ToolCallID:  common.ToolCallID("tool-task-decomposition-inspect"),
		PatchType:   "file_write",
		TargetRef:   "report.md",
		State:       common.PatchStateApplied,
	}); err != nil {
		t.Fatalf("create patch: %v", err)
	}
	if err := exports.Repositories.ValidationResults.Create(ctx, &runmodel.ValidationResult{
		ID:             common.ValidationResultID("validation-task-decomposition-inspect"),
		WorkspaceID:    workspaceID,
		RunID:          runID,
		StepID:         common.StepID("run-task-decomposition/block/inspect"),
		ValidationKind: "runtime_step",
		State:          common.ValidationResultStatePassed,
		EvidenceRef:    "tool-task-decomposition-inspect",
	}); err != nil {
		t.Fatalf("create validation: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:           common.ResultID("result-task-decomposition-inspect"),
		WorkspaceID:  workspaceID,
		RunID:        runID,
		DoDID:        "dod-task-decomposition",
		State:        common.ResultStateAccepted,
		SourceType:   "dod_assessment",
		SourceRef:    "run-task-decomposition/block/inspect",
		Summary:      "inspect result accepted",
		EvidenceRefs: []string{"run-task-decomposition/block/inspect"},
	}); err != nil {
		t.Fatalf("create result: %v", err)
	}
	if err := exports.Repositories.Artifacts.Create(ctx, &resultmodel.Artifact{
		ID:          common.ArtifactID("artifact-task-decomposition-inspect"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		ResultID:    common.ResultID("result-task-decomposition-inspect"),
		StepID:      common.StepID("run-task-decomposition/block/inspect"),
		Name:        "Inspect report",
		State:       common.ArtifactStateAvailable,
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if err := exports.Repositories.MemoryRecallTraces.Create(ctx, &memorymodel.MemoryRecallTrace{
		ID:                 common.MemoryRecallTraceID("memory-recall-task-decomposition-inspect"),
		WorkspaceID:        workspaceID,
		RunID:              runID,
		StepID:             common.StepID("run-task-decomposition/block/inspect"),
		Stage:              "execution",
		Source:             "step_input",
		Status:             "selected",
		RetrievedMemoryIDs: []common.MemoryID{"memory-task-decomposition-inspect"},
		EvidenceRef:        "memory-recall-task-decomposition-inspect",
	}); err != nil {
		t.Fatalf("create memory recall trace: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    common.StepID("run-task-decomposition/block/verify"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateExecuting,
		Input: mustAppserviceGovernancePayload(t, map[string]any{
			"graphNodeId":     "verify",
			"graphRootStepId": string(rootStepID),
		}),
	}); err != nil {
		t.Fatalf("create verify step: %v", err)
	}

	items, err := ReadRunTaskDecompositionsWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunTaskDecompositionsWithBackend: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("task decompositions = %+v, want one graph projection", items)
	}
	item := items[0]
	if item.RootStepID != string(rootStepID) || item.State != "running" || item.NodeTotal != 2 || item.NodeCompleted != 1 || item.NodeRunning != 1 {
		t.Fatalf("task decomposition = %+v, want running 1/2 completed", item)
	}
	if len(item.Nodes) != 2 || item.Nodes[0].ID != "inspect" || item.Nodes[0].State != "completed" || item.Nodes[1].ID != "verify" || item.Nodes[1].State != "running" {
		t.Fatalf("nodes = %+v, want inspect completed and verify running", item.Nodes)
	}
	if len(item.Nodes[1].DependsOn) != 1 || item.Nodes[1].DependsOn[0] != "inspect" {
		t.Fatalf("verify dependsOn = %+v, want inspect", item.Nodes[1].DependsOn)
	}
	snapshot, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithBackend: %v", err)
	}
	if len(snapshot.ExecutionSections) != 2 {
		t.Fatalf("executionSections = %+v, want two sections", snapshot.ExecutionSections)
	}
	if len(snapshot.TaskPlans) != 1 ||
		snapshot.TaskPlans[0].AnchorStepID != string(rootStepID) ||
		snapshot.TaskPlans[0].Revision != 1 ||
		snapshot.TaskPlans[0].State != "accepted" ||
		snapshot.TaskPlans[0].NodeTotal != 2 {
		t.Fatalf("taskPlans = %+v, want one accepted plan revision", snapshot.TaskPlans)
	}
	inspectSection := snapshot.ExecutionSections[0]
	if inspectSection.GraphNodeID != "inspect" || inspectSection.State != "completed" || inspectSection.StepID != "run-task-decomposition/block/inspect" {
		t.Fatalf("inspect section = %+v, want linked completed inspect step", inspectSection)
	}
	if !stringSliceContains(inspectSection.ToolCallRefs, "tool-task-decomposition-inspect") ||
		!stringSliceContains(inspectSection.PatchRefs, "patch-task-decomposition-inspect") ||
		!stringSliceContains(inspectSection.ValidationRefs, "validation-task-decomposition-inspect") ||
		!stringSliceContains(inspectSection.ResultRefs, "result-task-decomposition-inspect") ||
		!stringSliceContains(inspectSection.ArtifactRefs, "artifact-task-decomposition-inspect") ||
		!stringSliceContains(inspectSection.MemoryRecallRefs, "memory-recall-task-decomposition-inspect") ||
		!stringSliceContains(inspectSection.MemoryIDs, "memory-task-decomposition-inspect") {
		t.Fatalf("inspect section = %+v, want linked evidence refs", inspectSection)
	}

	bundle, err := ReadRunContextBundleWithBackend(context.Background(), "file", dataRoot, string(runID))
	if err != nil {
		t.Fatalf("ReadRunContextBundleWithBackend: %v", err)
	}
	if !contextBundleContainsSection(bundle, "task_decomposition") {
		t.Fatalf("context bundle sections = %+v, want task_decomposition", bundle.Sections)
	}
	if !contextBundleContainsSection(bundle, "execution_sections") {
		t.Fatalf("context bundle sections = %+v, want execution_sections", bundle.Sections)
	}
}

func progressContainsMilestone(progress TaskProgress, id string, state string) bool {
	for _, item := range progress.Milestones {
		if item.ID == id && item.State == state {
			return true
		}
	}
	return false
}

func contextBundleContainsSection(bundle ContextBundle, id string) bool {
	for _, item := range bundle.Sections {
		if item.ID == id {
			return true
		}
	}
	return false
}

func stringSliceContains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func contextBundleContainsCompressionCandidate(bundle ContextBundle, sectionID string) bool {
	for _, item := range bundle.CompressionCandidates {
		if item.SectionID == sectionID {
			return true
		}
	}
	return false
}

func twoDigitSecond(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func progressContainsItem(progress TaskProgress, id string, kind string, state string) bool {
	for _, item := range progress.Items {
		if item.ID == id && item.Kind == kind && item.State == state {
			return true
		}
	}
	return false
}

func timelineContainsStepOutput(items []RuntimeTimelineItem, stepID string, stage string, runState string) bool {
	for _, item := range items {
		if item.Source != "step" || item.StepID != stepID || item.Output == nil {
			continue
		}
		if item.Output["stage"] == stage && item.RunState == runState {
			return true
		}
	}
	return false
}

func timelineContainsStreamDelta(items []RuntimeTimelineItem, stepID string, delta string, runState string) bool {
	for _, item := range items {
		if item.Source != "step_stream" || item.StepID != stepID {
			continue
		}
		if item.Type == "delta" && item.Data["delta"] == delta && item.RunState == runState {
			return true
		}
	}
	return false
}

func timelineContainsStepOccurredAt(items []RuntimeTimelineItem, source string, stepID string, occurredAt string) bool {
	for _, item := range items {
		if item.Source == source && item.StepID == stepID && item.OccurredAt == occurredAt {
			return true
		}
	}
	return false
}

func timelineContainsPersistedEvent(items []RuntimeTimelineItem, topic string) bool {
	for _, item := range items {
		if item.Source == "persisted_event" && item.Topic == topic {
			return true
		}
	}
	return false
}

func timelineContainsLLMStreamProgressEvent(items []RuntimeTimelineItem, stepID string, status string, chunkCount int, receivedBytes int) bool {
	for _, item := range items {
		if item.Source != "llm_stream_progress_event" || item.Topic != "/run/llm_stream/progress" || item.StepID != stepID {
			continue
		}
		if item.Type != status {
			continue
		}
		if readModelIntValue(item.Data[recordpayload.PayloadKeyChunkCount]) != chunkCount ||
			readModelIntValue(item.Data[recordpayload.PayloadKeyReceivedBytes]) != receivedBytes {
			continue
		}
		return true
	}
	return false
}

func TestReadWorkspaceSnapshotWithBackendReturnsRelatedRunsAndBlockingItems(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := "ws-workspace-snapshot"
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, workspaceID, "workspace snapshot", "", "workspaces/workspace snapshot"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports, err := testSessionExports(session)
	if err != nil {
		t.Fatalf("runtime exports: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          "run-workspace-snapshot-completed",
		WorkspaceID: common.WorkspaceID(workspaceID),
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create completed run: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:          "result-workspace-snapshot",
		WorkspaceID: common.WorkspaceID(workspaceID),
		RunID:       "run-workspace-snapshot-completed",
		State:       common.ResultStateAccepted,
		Summary:     "workspace formal outcome",
	}); err != nil {
		t.Fatalf("create workspace result: %v", err)
	}
	if err := exports.Repositories.Artifacts.Create(ctx, &resultmodel.Artifact{
		ID:           "artifact-workspace-snapshot",
		WorkspaceID:  common.WorkspaceID(workspaceID),
		RunID:        "run-workspace-snapshot-completed",
		ResultID:     "result-workspace-snapshot",
		ArtifactType: "report",
		State:        common.ArtifactStateAvailable,
	}); err != nil {
		t.Fatalf("create workspace artifact: %v", err)
	}

	decisionRunID := "run-decision-workspace-snapshot"
	decisionRun := prepareDecisionRun(t, dataRoot, workspaceID, decisionRunID)
	if _, err := ResolveDecisionWithBackend(context.Background(), "file", dataRoot, decisionRun.BlockingTicketID, ResolveDecisionCommand{
		ResolutionType: decisioncommon.ResolutionTypeProvideMissingInfo,
	}); err != nil {
		t.Fatalf("ResolveDecisionWithBackend: %v", err)
	}
	workspaceAfterDecision, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		t.Fatalf("get workspace after decision: %v", err)
	}
	workspaceAfterDecision.ActiveRunID = ""
	workspaceAfterDecision.State = common.WorkspaceStateActive
	if err := exports.Repositories.Workspaces.Update(ctx, workspaceAfterDecision); err != nil {
		t.Fatalf("clear decision active run: %v", err)
	}

	approvalRunID := "run-approval-workspace-snapshot"
	approvalRun := prepareApprovalRun(t, dataRoot, workspaceID, approvalRunID)
	workspaceItem, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspaceItem.ActiveRunID = common.RunID(approvalRunID)
	if err := exports.Repositories.Workspaces.Update(ctx, workspaceItem); err != nil {
		t.Fatalf("update workspace active run: %v", err)
	}

	item, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, workspaceID)
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithBackend: %v", err)
	}
	if item.Workspace.ID != workspaceID {
		t.Fatalf("workspace = %+v, want %s", item.Workspace, workspaceID)
	}
	if item.Workspace.State != string(common.WorkspaceStateBlockedOnApproval) || item.Workspace.ActiveRunID != approvalRunID {
		t.Fatalf("workspace projection = %+v, want blocked_on_approval active run %s", item.Workspace, approvalRunID)
	}
	if len(item.Runs) != 3 {
		t.Fatalf("runs = %+v, want 3 runs", item.Runs)
	}
	if len(item.Results) != 1 || item.Results[0].Summary != "workspace formal outcome" {
		t.Fatalf("results = %+v, want workspace formal outcome", item.Results)
	}
	if len(item.Artifacts) != 1 || item.Artifacts[0].ResultID != "result-workspace-snapshot" {
		t.Fatalf("artifacts = %+v, want workspace artifact", item.Artifacts)
	}
	if len(item.Decisions) != 1 || item.Decisions[0].ID != decisionRun.BlockingTicketID {
		t.Fatalf("decisions = %+v, want decision %s", item.Decisions, decisionRun.BlockingTicketID)
	}
	if len(item.Approvals) != 1 || item.Approvals[0].ID != approvalRun.BlockingApprovalID {
		t.Fatalf("approvals = %+v, want approval %s", item.Approvals, approvalRun.BlockingApprovalID)
	}
}

func TestReadWorkspaceSnapshotWithBackendReturnsPersistedEventRecords(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := "ws-event-record-snapshot"
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, workspaceID, "event record snapshot", "", "workspaces/event record snapshot"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	item, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, workspaceID)
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithBackend: %v", err)
	}
	if len(item.EventRecords) == 0 {
		t.Fatalf("eventRecords = %+v, want persisted workspace event", item.EventRecords)
	}
	if item.EventRecords[0].WorkspaceID != workspaceID || item.EventRecords[0].Topic != workspaceevents.TopicWorkspaceCreated {
		t.Fatalf("eventRecords = %+v, want workspace-created event", item.EventRecords)
	}
}

func TestReadWorkspaceSnapshotWithBackendSummarizesRunScopedLLMUsageRecords(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := common.WorkspaceID("ws-run-scoped-llm-usage")
	runID := common.RunID("run-scoped-llm-usage")
	if _, err := createWorkspaceWithIDWithBackend(context.Background(), "file", dataRoot, string(workspaceID), "run scoped llm usage", "", "workspaces/run scoped llm usage"); err != nil {
		t.Fatalf("CreateWorkspaceWithBackend: %v", err)
	}

	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.EventRecords.Append(ctx, &eventstreammodel.EventRecord{
		ID:          "run-scoped-llm-usage/event/llm-usage-recorded",
		Version:     recordpayload.WorkorchEventVersion,
		Topic:       "/run/llm_usage/recorded",
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      "run-scoped-llm-usage/step-1",
		Payload: mustAppserviceGovernancePayload(t, map[string]any{
			recordpayload.PayloadKeyWorkspaceID:  string(workspaceID),
			recordpayload.PayloadKeyRunID:        string(runID),
			recordpayload.PayloadKeyStepID:       "run-scoped-llm-usage/step-1",
			recordpayload.PayloadKeyProviderID:   "provider-run-scoped",
			recordpayload.PayloadKeyProvider:     "compatible",
			recordpayload.PayloadKeyModel:        "model-run-scoped",
			recordpayload.PayloadKeyOperation:    "chat_completions",
			recordpayload.PayloadKeyInputTokens:  32,
			recordpayload.PayloadKeyOutputTokens: 8,
			recordpayload.PayloadKeyTotalTokens:  40,
			recordpayload.PayloadKeyEstimated:    true,
			recordpayload.PayloadKeyUsageSource:  "stream_estimated",
			recordpayload.PayloadKeyStatus:       "completed",
		}),
	}); err != nil {
		t.Fatalf("append run scoped llm usage event: %v", err)
	}

	item, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, string(workspaceID))
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithBackend: %v", err)
	}
	if item.LLMUsageSummary.EntryTotal != 1 || item.LLMUsageSummary.EstimatedEntries != 1 {
		t.Fatalf("llm usage summary = %+v, want one estimated run-scoped entry", item.LLMUsageSummary)
	}
	if len(item.LLMUsageSummary.ProviderModelBreakdown) != 1 ||
		item.LLMUsageSummary.ProviderModelBreakdown[0].ProviderID != "provider-run-scoped" ||
		item.LLMUsageSummary.ProviderModelBreakdown[0].Model != "model-run-scoped" {
		t.Fatalf("provider/model breakdown = %+v, want run scoped provider/model", item.LLMUsageSummary.ProviderModelBreakdown)
	}
}

func TestReadWorkspaceSnapshotProjectsWorkspaceStateFromRun(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-projection")
	runID := common.RunID("run-projection")
	workspace := &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "projection",
		State:       common.WorkspaceStateActive,
		ActiveRunID: runID,
	}
	if err := exports.Repositories.Workspaces.Create(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
	}
	if err := exports.Repositories.Runs.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	tests := []struct {
		runState       common.RunState
		workspaceState string
	}{
		{runState: common.RunStateActive, workspaceState: string(common.WorkspaceStateActive)},
		{runState: common.RunStateBlockedOnDecision, workspaceState: string(common.WorkspaceStateActive)},
		{runState: common.RunStateBlockedOnApproval, workspaceState: string(common.WorkspaceStateBlockedOnApproval)},
		{runState: common.RunStateActive, workspaceState: string(common.WorkspaceStateActive)},
		{runState: common.RunStateCompleted, workspaceState: string(common.WorkspaceStateActive)},
		{runState: common.RunStateFailed, workspaceState: string(common.WorkspaceStateActive)},
		{runState: common.RunStateCancelled, workspaceState: string(common.WorkspaceStateActive)},
		{runState: common.RunStateSuperseded, workspaceState: string(common.WorkspaceStateSuperseded)},
	}
	for _, tc := range tests {
		t.Run(string(tc.runState), func(t *testing.T) {
			run.State = tc.runState
			if err := exports.Repositories.Runs.Update(ctx, run); err != nil {
				t.Fatalf("update run: %v", err)
			}
			snapshot, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, string(workspaceID))
			if err != nil {
				t.Fatalf("ReadWorkspaceSnapshotWithBackend: %v", err)
			}
			if snapshot.Workspace.State != tc.workspaceState {
				t.Fatalf("workspace state = %q, want %q", snapshot.Workspace.State, tc.workspaceState)
			}
			expectedActiveRunID := string(runID)
			if tc.runState == common.RunStateCompleted ||
				tc.runState == common.RunStateFailed ||
				tc.runState == common.RunStateCancelled ||
				tc.runState == common.RunStateSuperseded {
				expectedActiveRunID = ""
			}
			if snapshot.Workspace.ActiveRunID != expectedActiveRunID {
				t.Fatalf("activeRunId = %q, want %q", snapshot.Workspace.ActiveRunID, expectedActiveRunID)
			}
		})
	}
}

func TestReadWorkspaceSnapshotWithRunSnapshotReusesProjectionSections(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-cached-run-projection")
	runID := common.RunID("run-cached-run-projection")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "cached run projection",
		State:       common.WorkspaceStateActive,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	cached := RunSnapshot{
		Workspace: Workspace{ID: string(workspaceID), State: string(common.WorkspaceStateActive)},
		Run:       Run{ID: string(runID), WorkspaceID: string(workspaceID), State: string(common.RunStateCompleted)},
		TaskProgress: TaskProgress{
			WorkspaceID: string(workspaceID),
			RunID:       string(runID),
			State:       string(common.RunStateCompleted),
			Summary:     "cached run task progress",
		},
		TaskDecompositions: []TaskDecomposition{{ID: "cached-decomposition", Summary: "cached decomposition"}},
		ExecutionSections:  []ExecutionSection{{ID: "cached-section", Title: "cached section"}},
		Patches:            []Patch{{ID: "patch-cached", WorkspaceID: string(workspaceID), RunID: string(runID), TargetRef: "main.go", State: "applied", AddedLines: 2, DeletedLines: 1}},
	}
	snapshot, err := ReadWorkspaceSnapshotWithRunSnapshotWithSession(ctx, session.Borrow(), string(workspaceID), cached)
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithRunSnapshotWithSession: %v", err)
	}
	if snapshot.TaskProgress.Summary != cached.TaskProgress.Summary {
		t.Fatalf("task progress summary = %q, want cached summary", snapshot.TaskProgress.Summary)
	}
	if len(snapshot.TaskDecompositions) != 1 || snapshot.TaskDecompositions[0].ID != "cached-decomposition" {
		t.Fatalf("task decompositions = %+v, want cached decomposition", snapshot.TaskDecompositions)
	}
	if len(snapshot.ExecutionSections) != 1 || snapshot.ExecutionSections[0].ID != "cached-section" {
		t.Fatalf("execution sections = %+v, want cached section", snapshot.ExecutionSections)
	}
	if len(snapshot.Patches) != 1 || snapshot.Patches[0].ID != "patch-cached" || snapshot.Patches[0].AddedLines != 2 {
		t.Fatalf("patches = %+v, want cached patch projection", snapshot.Patches)
	}
}

func TestReadWorkspaceSnapshotWithRunSnapshotAvoidsProjectionRunStepRead(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-cached-run-no-step-read")
	runID := common.RunID("run-cached-run-no-step-read")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "cached run no step read",
		State:       common.WorkspaceStateActive,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateBlockedOnDecision,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateFailed,
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}

	deps, err := readModelDependenciesFromSession(session.Borrow())
	if err != nil {
		t.Fatalf("readModelDependenciesFromSession: %v", err)
	}
	countingSteps := &countingStepRepository{base: deps.steps}
	deps.steps = countingSteps
	cached := RunSnapshot{
		Workspace: Workspace{ID: string(workspaceID), State: string(common.WorkspaceStateActive), ActiveRunID: string(runID)},
		Run:       Run{ID: string(runID), WorkspaceID: string(workspaceID), State: string(common.RunStateBlockedOnDecision)},
		TaskProgress: TaskProgress{
			WorkspaceID: string(workspaceID),
			RunID:       string(runID),
			State:       string(common.RunStateBlockedOnDecision),
			Summary:     "cached activation progress",
		},
	}

	snapshot, err := readWorkspaceSnapshot(ctx, deps, workspaceID, &cached)
	if err != nil {
		t.Fatalf("readWorkspaceSnapshot: %v", err)
	}
	if countingSteps.listByRunCalls != 0 {
		t.Fatalf("ListByRun calls = %d, want 0 with cached projection RunSnapshot", countingSteps.listByRunCalls)
	}
	if snapshot.TaskProgress.Summary != "cached activation progress" {
		t.Fatalf("task progress summary = %q, want cached progress", snapshot.TaskProgress.Summary)
	}
}

func TestReadWorkspaceSnapshotProjectsActiveRunAndCurrentStep(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-active-run-current-step")
	runID := common.RunID("run-active-run-current-step")
	stepID := common.StepID(string(runID) + "/graph-001/002-repair")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "active run current step",
		State:       common.WorkspaceStateActive,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/001-read"),
		RunID: runID,
		Kind:  "file/read",
		State: common.StepStateSucceeded,
	}); err != nil {
		t.Fatalf("create completed step: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateExecuting,
	}); err != nil {
		t.Fatalf("create current step: %v", err)
	}

	snapshot, err := ReadWorkspaceSnapshotWithSession(ctx, session.Borrow(), string(workspaceID))
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithSession: %v", err)
	}
	if snapshot.Run == nil || snapshot.Run.ID != string(runID) {
		t.Fatalf("snapshot.Run = %+v, want active run", snapshot.Run)
	}
	if snapshot.Step == nil || snapshot.Step.ID != string(stepID) || snapshot.Step.State != string(common.StepStateExecuting) {
		t.Fatalf("snapshot.Step = %+v, want dispatched current step", snapshot.Step)
	}
	if snapshot.TaskProgress.CurrentStepID != string(stepID) ||
		snapshot.TaskProgress.CurrentStepKind != "llm/workspace_execution" ||
		snapshot.TaskProgress.CurrentStepState != string(common.StepStateExecuting) {
		t.Fatalf("taskProgress current step = %+v, want active run current step", snapshot.TaskProgress)
	}
}

func TestReadWorkspaceSnapshotCanonicalizesWorkspacePlanCurrentStepKind(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-active-run-current-step-alias")
	runID := common.RunID("run-active-run-current-step-alias")
	stepID := common.StepID(string(runID) + "/graph-001/002-repair")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "active run current step alias",
		State:       common.WorkspaceStateActive,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		State:           common.RunStateActive,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspacePlan,
		State: common.StepStateExecuting,
	}); err != nil {
		t.Fatalf("create current step: %v", err)
	}

	snapshot, err := ReadWorkspaceSnapshotWithSession(ctx, session.Borrow(), string(workspaceID))
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithSession: %v", err)
	}
	if snapshot.TaskProgress.CurrentStepKind != llmcommon.StepKindWorkspaceExecution {
		t.Fatalf("taskProgress current step kind = %q, want %q", snapshot.TaskProgress.CurrentStepKind, llmcommon.StepKindWorkspaceExecution)
	}
}

func TestReadRunSnapshotIncludesArtifactContinuationMetadata(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)
	workspaceID := common.WorkspaceID("ws-artifact-continuation-read-model")
	runID := common.RunID("run-artifact-continuation-read-model")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "artifact continuation read model",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.ArtifactContinuations.Create(ctx, &orchestratormodel.ArtifactContinuation{
		ID:                "artifact-continuation-read-model-001",
		WorkspaceID:       workspaceID,
		RunID:             runID,
		ArtifactID:        "artifact-001",
		TargetRef:         "deliverable.txt",
		Operation:         "replace",
		State:             common.ArtifactContinuationStateChunking,
		ExpectedSequence:  3,
		ReceivedSequences: []int{1, 2},
		BytesWritten:      256,
		Metadata:          mustAppserviceEventPayload(t, map[string]any{"content": "must not project"}),
	}); err != nil {
		t.Fatalf("Create artifact continuation: %v", err)
	}

	snapshot, err := ReadRunSnapshotWithSession(ctx, session, string(runID))
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithSession: %v", err)
	}
	if len(snapshot.ArtifactContinuations) != 1 {
		t.Fatalf("artifact continuations = %+v, want one", snapshot.ArtifactContinuations)
	}
	item := snapshot.ArtifactContinuations[0]
	if item.TargetRef != "deliverable.txt" || item.State != string(common.ArtifactContinuationStateChunking) || item.ExpectedSequence != 3 || len(item.ReceivedSequences) != 2 {
		t.Fatalf("artifact continuation = %+v, want metadata projection", item)
	}
	if item.Progress == nil || item.Progress["receivedCount"] != 2 {
		t.Fatalf("progress = %+v, want received count", item.Progress)
	}
}

func TestReadRunSnapshotReadsCoverageWorkItemsFromOwnerRepository(t *testing.T) {
	session, ctx := newTestRuntimeSession(t, "memory", "")
	exports := testRuntimeExports(session.Bootstrap)
	workspaceID := common.WorkspaceID("ws-coverage-work-item-read-model")
	runID := common.RunID("run-coverage-work-item-read-model")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "coverage work item read model",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.CoverageWorkItems.Create(ctx, &orchestratormodel.CoverageWorkItem{
		ID:                    "run-coverage-work-item-read-model-cwi-001",
		WorkspaceID:           workspaceID,
		RunID:                 runID,
		DefinitionRevision:    "rev-coverage-read-model",
		NormalizedSourceKey:   "source:coverage-read-model",
		TargetText:            "owner-backed coverage item",
		RequiredEvidenceKinds: []string{"validation"},
		Status:                "open",
		CurrentGap:            "missing accepted evidence binding for required evidence kind: validation",
		Metadata: mustAppserviceEventPayload(t, map[string]any{
			"id":                     "cwi-001",
			"source":                 "goal",
			"acceptedEvidenceByKind": map[string]any{"validation": []string{"evidence-ref-001"}},
		}),
	}); err != nil {
		t.Fatalf("Create coverage work item: %v", err)
	}

	snapshot, err := ReadRunSnapshotWithSession(ctx, session, string(runID))
	if err != nil {
		t.Fatalf("ReadRunSnapshotWithSession: %v", err)
	}
	if len(snapshot.CoverageWorkItems) != 1 {
		t.Fatalf("coverage work items = %+v, want one owner item", snapshot.CoverageWorkItems)
	}
	item := snapshot.CoverageWorkItems[0]
	if item["id"] != "cwi-001" || item["repositoryId"] != "run-coverage-work-item-read-model-cwi-001" {
		t.Fatalf("coverage item ids = %+v, want context id plus repository id", item)
	}
	if item["targetText"] != "owner-backed coverage item" || item["status"] != "open" {
		t.Fatalf("coverage item = %+v, want owner-backed fields", item)
	}
}

func TestReadWorkspaceSnapshotIncludesWorkspaceGoalsAndDoDs(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-snapshot-goal-dod")
	goalID := common.GoalID("goal-snapshot")
	dodID := common.DoDID("dod-snapshot")
	workspaceRepo := memory.NewWorkspaceRepository()
	goalRepo := memory.NewGoalRepository()
	dodRepo := memory.NewDoDRepository()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "snapshot", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := goalRepo.Create(ctx, &workspacemodel.Goal{ID: goalID, WorkspaceID: workspaceID, Text: "goal text"}); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	if err := dodRepo.Create(ctx, &workspacemodel.DoD{ID: dodID, WorkspaceID: workspaceID, Items: []string{"dod item"}, State: common.DoDStateReady}); err != nil {
		t.Fatalf("create dod: %v", err)
	}
	deps := readModelDependencies{
		workspaces:            workspaceRepo,
		goals:                 goalRepo,
		dods:                  dodRepo,
		runs:                  memory.NewRunRepository(),
		steps:                 memory.NewStepRepository(),
		results:               memory.NewResultRepository(),
		artifacts:             memory.NewArtifactRepository(),
		artifactContinuations: memory.NewArtifactContinuationRepository(),
		eventRecords:          eventRecordRepositoryTestReader{memory.NewEventRecordRepository()},
		memoryRecalls:         memoryRecallTraceRepositoryTestReader{memory.NewMemoryRecallTraceRepository()},
		toolCalls:             memory.NewToolCallRepository(),
		mcpCapabilities:       memory.NewMCPCapabilityRepository(),
		patches:               memory.NewPatchRepository(),
		validations:           memory.NewValidationResultRepository(),
		checkpoints:           memory.NewRuntimeCheckpointRepository(),
		tickets:               memory.NewDecisionTicketRepository(),
		resolutions:           memory.NewDecisionResolutionRepository(),
		approvals:             memory.NewApprovalRepository(),
	}

	snapshot, err := readWorkspaceSnapshot(ctx, deps, workspaceID, nil)
	if err != nil {
		t.Fatalf("readWorkspaceSnapshot: %v", err)
	}
	if len(snapshot.Goals) != 1 || snapshot.Goals[0].ID != string(goalID) || snapshot.Goals[0].Text != "goal text" {
		t.Fatalf("goals = %+v, want workspace goal", snapshot.Goals)
	}
	if len(snapshot.DoDs) != 1 || snapshot.DoDs[0].ID != string(dodID) || !reflect.DeepEqual(snapshot.DoDs[0].Items, []string{"dod item"}) {
		t.Fatalf("dods = %+v, want workspace DoD", snapshot.DoDs)
	}
}

func TestReadWorkspaceSnapshotReadsWorkspaceAndWorkspaceScopedItemsConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	workspaceID := common.WorkspaceID("ws-workspace-snapshot-concurrent")
	runID := common.RunID("run-workspace-snapshot-concurrent")
	baseWorkspaces := memory.NewWorkspaceRepository()
	baseRuns := memory.NewRunRepository()
	if err := baseWorkspaces.Create(context.Background(), &workspacemodel.Workspace{
		ID:                   workspaceID,
		Title:                "workspace snapshot concurrent",
		State:                common.WorkspaceStateActive,
		DeliverableDirectory: "workspaces/workspace-snapshot-concurrent",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := baseRuns.Create(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	workspaceStarted := make(chan string, 1)
	runListStarted := make(chan string, 1)
	release := make(chan struct{})
	deps := readModelDependencies{
		workspaces:            &blockingWorkspaceRepository{base: baseWorkspaces, started: workspaceStarted, release: release},
		runs:                  &blockingRunListRepository{base: baseRuns, started: runListStarted, release: release},
		steps:                 memory.NewStepRepository(),
		results:               memory.NewResultRepository(),
		artifacts:             memory.NewArtifactRepository(),
		artifactContinuations: memory.NewArtifactContinuationRepository(),
		eventRecords:          eventRecordRepositoryTestReader{memory.NewEventRecordRepository()},
		memoryRecalls:         memoryRecallTraceRepositoryTestReader{memory.NewMemoryRecallTraceRepository()},
		toolCalls:             memory.NewToolCallRepository(),
		mcpCapabilities:       memory.NewMCPCapabilityRepository(),
		patches:               memory.NewPatchRepository(),
		validations:           memory.NewValidationResultRepository(),
		checkpoints:           memory.NewRuntimeCheckpointRepository(),
		tickets:               memory.NewDecisionTicketRepository(),
		resolutions:           memory.NewDecisionResolutionRepository(),
		approvals:             memory.NewApprovalRepository(),
	}

	done := make(chan error, 1)
	go func() {
		_, err := readWorkspaceSnapshot(ctx, deps, workspaceID, nil)
		done <- err
	}()

	for _, ch := range []<-chan string{workspaceStarted, runListStarted} {
		select {
		case <-ch:
		case err := <-done:
			t.Fatalf("readWorkspaceSnapshot returned before both reads started: %v", err)
		case <-ctx.Done():
			t.Fatal("timed out waiting for concurrent workspace and run list reads")
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("readWorkspaceSnapshot: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for readWorkspaceSnapshot")
	}
}

func TestReadWorkspaceSnapshotBindsWorkspaceScopeForWorkspaceScopedRunIDs(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)

	firstWorkspaceID := common.WorkspaceID("ws-snapshot-run-scope-a")
	secondWorkspaceID := common.WorkspaceID("ws-snapshot-run-scope-b")
	runID := common.RunID("run-002")
	for _, workspaceID := range []common.WorkspaceID{firstWorkspaceID, secondWorkspaceID} {
		if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
			ID:          workspaceID,
			Title:       string(workspaceID),
			State:       common.WorkspaceStateActive,
			ActiveRunID: runID,
		}); err != nil {
			t.Fatalf("create workspace %s: %v", workspaceID, err)
		}
		if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
			ID:          runID,
			WorkspaceID: workspaceID,
			State:       common.RunStateActive,
		}); err != nil {
			t.Fatalf("create run %s/%s: %v", workspaceID, runID, err)
		}
	}

	snapshot, err := ReadWorkspaceSnapshotWithSession(ctx, session.Borrow(), string(secondWorkspaceID))
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithSession: %v", err)
	}
	if snapshot.Workspace.ID != string(secondWorkspaceID) || snapshot.Run == nil || snapshot.Run.WorkspaceID != string(secondWorkspaceID) || snapshot.Run.ID != string(runID) {
		t.Fatalf("snapshot workspace/run = %+v/%+v, want scoped %s/%s", snapshot.Workspace, snapshot.Run, secondWorkspaceID, runID)
	}
	bundle, err := ReadWorkspaceContextBundleWithSession(ctx, session.Borrow(), string(secondWorkspaceID))
	if err != nil {
		t.Fatalf("ReadWorkspaceContextBundleWithSession: %v", err)
	}
	if bundle.RunID != string(runID) || bundle.WorkspaceID != string(secondWorkspaceID) {
		t.Fatalf("context bundle = %+v, want scoped run %s/%s", bundle, secondWorkspaceID, runID)
	}
}

func TestReadWorkspaceSnapshotProjectsAcceptedResultOverStaleTerminalActiveRun(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-stale-active-result")
	staleRunID := common.RunID("run-stale-failed")
	completedRunID := common.RunID("run-result-completed")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "stale projection",
		State:       common.WorkspaceStateActive,
		ActiveRunID: staleRunID,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:            staleRunID,
		WorkspaceID:   workspaceID,
		State:         common.RunStateFailed,
		FailureReason: "old provider failure",
	}); err != nil {
		t.Fatalf("create stale run: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          completedRunID,
		WorkspaceID: workspaceID,
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create completed run: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:          common.ResultID(string(completedRunID) + "/result/dod-assessment"),
		WorkspaceID: workspaceID,
		RunID:       completedRunID,
		State:       common.ResultStateAccepted,
		Summary:     "formal result",
		CreatedAt:   "2026-04-29T10:00:00Z",
	}); err != nil {
		t.Fatalf("create result: %v", err)
	}

	snapshot, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, string(workspaceID))
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithBackend: %v", err)
	}
	if snapshot.Workspace.State != string(common.WorkspaceStateActive) {
		t.Fatalf("workspace state = %q, want active", snapshot.Workspace.State)
	}
	if snapshot.Outcome.State != "accepted" || !snapshot.Outcome.Accepted || snapshot.Outcome.RunID != string(completedRunID) {
		t.Fatalf("workspace outcome = %+v, want accepted completed run", snapshot.Outcome)
	}
	if snapshot.Workspace.ActiveRunID != "" {
		t.Fatalf("activeRunId = %q, want empty for terminal projection", snapshot.Workspace.ActiveRunID)
	}
	if len(snapshot.Results) != 1 || snapshot.Results[0].RunID != string(completedRunID) || snapshot.Results[0].Summary != "formal result" {
		t.Fatalf("results = %+v, want accepted formal result for completed run", snapshot.Results)
	}
}

func TestReadWorkspaceSnapshotProjectsLatestRunFailureOverHistoricalAcceptedResult(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-latest-failure-projection")
	completedRunID := common.RunID("run-completed-with-result")
	failedRunID := common.RunID("run-latest-provider-failed")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "latest failure projection",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          completedRunID,
		WorkspaceID: workspaceID,
		State:       common.RunStateCompleted,
	}); err != nil {
		t.Fatalf("create completed run: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:          common.ResultID(string(completedRunID) + "/result/dod-assessment"),
		WorkspaceID: workspaceID,
		RunID:       completedRunID,
		State:       common.ResultStateAccepted,
		Summary:     "historical formal result",
		CreatedAt:   "2026-04-29T10:00:00Z",
	}); err != nil {
		t.Fatalf("create historical result: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:            failedRunID,
		WorkspaceID:   workspaceID,
		State:         common.RunStateFailed,
		FailureReason: "llm provider returned html",
	}); err != nil {
		t.Fatalf("create failed run: %v", err)
	}
	if err := exports.Repositories.EventRecords.Append(ctx, &eventstreammodel.EventRecord{
		ID:          common.EventRecordID("evt-run-completed-created"),
		Version:     recordpayload.WorkorchEventVersion,
		Topic:       runevents.TopicRunCreated,
		OccurredAt:  "2026-04-29T10:00:00Z",
		SubjectType: "run",
		SubjectID:   string(completedRunID),
		WorkspaceID: workspaceID,
		RunID:       completedRunID,
		Payload:     mustAppserviceGovernancePayload(t, map[string]any{recordpayload.PayloadKeyState: string(common.RunStateCompleted)}),
	}); err != nil {
		t.Fatalf("append completed event: %v", err)
	}
	if err := exports.Repositories.EventRecords.Append(ctx, &eventstreammodel.EventRecord{
		ID:          common.EventRecordID("evt-run-latest-failed"),
		Version:     recordpayload.WorkorchEventVersion,
		Topic:       runevents.TopicRunStateChanged,
		OccurredAt:  "2026-04-30T10:00:00Z",
		SubjectType: "run",
		SubjectID:   string(failedRunID),
		WorkspaceID: workspaceID,
		RunID:       failedRunID,
		Payload:     mustAppserviceGovernancePayload(t, map[string]any{recordpayload.PayloadKeyState: string(common.RunStateFailed), recordpayload.PayloadKeyFailureReason: "llm provider returned html"}),
	}); err != nil {
		t.Fatalf("append failed event: %v", err)
	}

	snapshot, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, string(workspaceID))
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithBackend: %v", err)
	}
	if snapshot.Workspace.State != string(common.WorkspaceStateActive) {
		t.Fatalf("workspace state = %q, want active from latest run", snapshot.Workspace.State)
	}
	if snapshot.TaskProgress.RunID != string(failedRunID) || snapshot.TaskProgress.State != string(common.RunStateFailed) {
		t.Fatalf("task progress = %+v, want latest failed run", snapshot.TaskProgress)
	}
	if snapshot.Outcome.State != "failed" || snapshot.Outcome.RunID != string(failedRunID) || snapshot.Outcome.Accepted {
		t.Fatalf("workspace outcome = %+v, want failed latest run outcome", snapshot.Outcome)
	}
	if !progressContainsMilestone(snapshot.TaskProgress, "formal_result", "failed") {
		t.Fatalf("milestones = %+v, want formal_result failed for latest run without result", snapshot.TaskProgress.Milestones)
	}
	if len(snapshot.Results) != 1 || snapshot.Results[0].RunID != string(completedRunID) {
		t.Fatalf("results = %+v, want historical result still visible", snapshot.Results)
	}
}

func TestReadWorkspaceSnapshotTaskProgressUsesLatestTerminalRejectedResult(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := testRuntimeExports(session.Bootstrap)

	workspaceID := common.WorkspaceID("ws-terminal-rejected-progress")
	oldRunID := common.RunID("run-old-failed")
	currentRunID := common.RunID("run-current-rejected")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "terminal rejected progress",
		State: common.WorkspaceStateActive,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	for _, item := range []struct {
		id     common.RunID
		reason string
	}{
		{oldRunID, "old failed run"},
		{currentRunID, "current rejected run"},
	} {
		if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
			ID:            item.id,
			WorkspaceID:   workspaceID,
			State:         common.RunStateFailed,
			FailureReason: item.reason,
		}); err != nil {
			t.Fatalf("create run %s: %v", item.id, err)
		}
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:          common.ResultID(string(oldRunID) + "/result/dod-assessment"),
		WorkspaceID: workspaceID,
		RunID:       oldRunID,
		State:       common.ResultStateRejected,
		Summary:     "old rejected result",
		CreatedAt:   "2026-05-17T07:03:52Z",
		UpdatedAt:   "2026-05-17T07:03:52Z",
	}); err != nil {
		t.Fatalf("create old result: %v", err)
	}
	if err := exports.Repositories.Results.Create(ctx, &resultmodel.Result{
		ID:          common.ResultID(string(currentRunID) + "/result/dod-assessment"),
		WorkspaceID: workspaceID,
		RunID:       currentRunID,
		State:       common.ResultStateRejected,
		Summary:     "current rejected result",
		SourceType:  resultcommon.SourceTypeDoDAssessment,
		SourceRef:   string(currentRunID) + "/graph-002/assessment",
		CreatedAt:   "2026-05-17T12:31:45Z",
		UpdatedAt:   "2026-05-17T12:31:45Z",
	}); err != nil {
		t.Fatalf("create current result: %v", err)
	}
	if err := exports.Repositories.EventRecords.Append(ctx, &eventstreammodel.EventRecord{
		ID:          common.EventRecordID("evt-old-failed-before-current-result"),
		Version:     recordpayload.WorkorchEventVersion,
		Topic:       runevents.TopicRunStateChanged,
		OccurredAt:  "2026-05-17T12:30:00Z",
		SubjectType: "run",
		SubjectID:   string(oldRunID),
		WorkspaceID: workspaceID,
		RunID:       oldRunID,
		Payload:     mustAppserviceGovernancePayload(t, map[string]any{recordpayload.PayloadKeyState: string(common.RunStateFailed)}),
	}); err != nil {
		t.Fatalf("append stale event: %v", err)
	}

	snapshot, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, string(workspaceID))
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithBackend: %v", err)
	}
	if snapshot.Outcome.RunID != string(currentRunID) || snapshot.Outcome.ResultID != string(currentRunID)+"/result/dod-assessment" {
		t.Fatalf("outcome = %+v, want current rejected result", snapshot.Outcome)
	}
	if snapshot.TaskProgress.RunID != string(currentRunID) || snapshot.TaskProgress.State != string(common.RunStateFailed) {
		t.Fatalf("task progress = %+v, want same current rejected run as outcome", snapshot.TaskProgress)
	}
	if snapshot.Result == nil || snapshot.Result.ID != string(currentRunID)+"/result/dod-assessment" || snapshot.Result.Summary != "current rejected result" {
		t.Fatalf("snapshot result = %+v, want current rejected result", snapshot.Result)
	}
	if snapshot.TaskProgress.CurrentStepID != string(currentRunID)+"/graph-002/assessment" ||
		snapshot.TaskProgress.CurrentStepKind != "result/"+resultcommon.SourceTypeDoDAssessment ||
		snapshot.TaskProgress.CurrentStepState != string(common.ResultStateRejected) {
		t.Fatalf("task progress current = %s/%s/%s, want terminal rejected result source", snapshot.TaskProgress.CurrentStepID, snapshot.TaskProgress.CurrentStepKind, snapshot.TaskProgress.CurrentStepState)
	}
}

func TestMapWorkspaceOutcomePrefersLatestTerminalRunOverStaleTaskProgress(t *testing.T) {
	olderRunID := "run-older-accepted"
	latestRunID := "run-latest-accepted"
	snapshot := WorkspaceSnapshot{
		Workspace: Workspace{
			ID:    "ws-latest-outcome",
			State: string(common.WorkspaceStateActive),
		},
		TaskProgress: TaskProgress{
			RunID: olderRunID,
			State: string(common.RunStateCompleted),
		},
		Runs: []Run{
			{ID: olderRunID, WorkspaceID: "ws-latest-outcome", State: string(common.RunStateCompleted)},
			{ID: latestRunID, WorkspaceID: "ws-latest-outcome", State: string(common.RunStateCompleted)},
		},
		Results: []Result{
			{
				ID:          olderRunID + "/result/dod-assessment",
				WorkspaceID: "ws-latest-outcome",
				RunID:       olderRunID,
				State:       string(common.ResultStateAccepted),
				Summary:     "older accepted result",
				CreatedAt:   "2026-05-08T10:00:00Z",
			},
			{
				ID:          latestRunID + "/result/dod-assessment",
				WorkspaceID: "ws-latest-outcome",
				RunID:       latestRunID,
				State:       string(common.ResultStateAccepted),
				Summary:     "latest accepted result",
				CreatedAt:   "2026-05-08T11:00:00Z",
			},
		},
		EventRecords: []EventRecord{
			{
				ID:          "evt-older",
				Topic:       runevents.TopicRunStateChanged,
				OccurredAt:  "2026-05-08T10:00:00Z",
				WorkspaceID: "ws-latest-outcome",
				RunID:       olderRunID,
			},
			{
				ID:          "evt-latest",
				Topic:       runevents.TopicRunStateChanged,
				OccurredAt:  "2026-05-08T11:00:00Z",
				WorkspaceID: "ws-latest-outcome",
				RunID:       latestRunID,
			},
		},
	}

	outcome := readmodelprojection.MapWorkspaceOutcome(snapshot)
	if outcome.State != "accepted" || !outcome.Accepted || outcome.RunID != latestRunID || outcome.Summary != "latest accepted result" {
		t.Fatalf("outcome = %+v, want latest accepted run despite stale task progress", outcome)
	}
}

func TestMapWorkspaceOutcomePrefersActiveEffectiveRunOverHistoricalTerminalOutcome(t *testing.T) {
	snapshot := WorkspaceSnapshot{
		Workspace: Workspace{
			ID:          "ws-active-effective-outcome",
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-active",
		},
		Run: &Run{
			ID:          "run-active",
			WorkspaceID: "ws-active-effective-outcome",
			State:       string(common.RunStateActive),
		},
		Runs: []Run{
			{ID: "run-historical", WorkspaceID: "ws-active-effective-outcome", State: string(common.RunStateFailed), FailureReason: "historical failure"},
			{ID: "run-active", WorkspaceID: "ws-active-effective-outcome", State: string(common.RunStateActive)},
		},
		Results: []Result{
			{
				ID:          "run-historical/result/dod-assessment",
				WorkspaceID: "ws-active-effective-outcome",
				RunID:       "run-historical",
				State:       string(common.ResultStateRejected),
				Summary:     "historical rejected result",
				CreatedAt:   "2026-07-11T08:00:00Z",
			},
		},
		EventRecords: []EventRecord{
			{
				ID:          "evt-historical",
				Topic:       runevents.TopicRunStateChanged,
				OccurredAt:  "2026-07-11T08:00:00Z",
				WorkspaceID: "ws-active-effective-outcome",
				RunID:       "run-historical",
			},
		},
	}

	outcome := readmodelprojection.MapWorkspaceOutcome(snapshot)
	if outcome.RunID != "run-active" || outcome.State != string(common.RunStateActive) || outcome.Terminal || outcome.Accepted {
		t.Fatalf("outcome = %+v, want active effective run instead of historical terminal outcome", outcome)
	}
	if outcome.ResultID != "" || outcome.Reason == "historical failure" || outcome.Summary == "historical rejected result" {
		t.Fatalf("outcome = %+v, want no historical terminal result leakage", outcome)
	}
}

func TestMapWorkspaceOutcomeDoesNotReuseAcceptedResultAfterDefinitionsReopenWorkspace(t *testing.T) {
	snapshot := WorkspaceSnapshot{
		Workspace: Workspace{
			ID:    "ws-definition-reopened",
			State: string(common.WorkspaceStateActive),
		},
		Goals: []Goal{{ID: "goal-current", WorkspaceID: "ws-definition-reopened"}},
		DoDs:  []DoD{{ID: "dod-current", WorkspaceID: "ws-definition-reopened"}},
		Runs: []Run{
			{
				ID:          "run-old-accepted",
				WorkspaceID: "ws-definition-reopened",
				GoalID:      "goal-old",
				DoDID:       "dod-old",
				State:       string(common.RunStateCompleted),
			},
		},
		Results: []Result{
			{
				ID:          "run-old-accepted/result/dod-assessment",
				WorkspaceID: "ws-definition-reopened",
				RunID:       "run-old-accepted",
				GoalID:      "goal-old",
				DoDID:       "dod-old",
				State:       string(common.ResultStateAccepted),
				Summary:     "old accepted result",
				CreatedAt:   "2026-05-18T10:00:00Z",
			},
		},
	}

	outcome := readmodelprojection.MapWorkspaceOutcome(snapshot)
	if outcome.Accepted || outcome.Terminal || outcome.ResultID != "" || outcome.RunID != "" {
		t.Fatalf("outcome = %+v, want no accepted terminal outcome after definition change", outcome)
	}
	if outcome.State != string(common.WorkspaceStateActive) || !strings.Contains(outcome.Reason, "definitions changed") {
		t.Fatalf("outcome = %+v, want active definition-change rerun reason", outcome)
	}
}

func TestMapWorkspaceOutcomeIgnoresAcceptedSliceForRunningRun(t *testing.T) {
	snapshot := WorkspaceSnapshot{
		Workspace: Workspace{
			ID:          "ws-running-slice",
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-running-slice",
		},
		Run: &Run{
			ID:          "run-running-slice",
			WorkspaceID: "ws-running-slice",
			State:       string(common.RunStateActive),
		},
		Runs: []Run{
			{ID: "run-running-slice", WorkspaceID: "ws-running-slice", State: string(common.RunStateActive)},
		},
		Results: []Result{
			{
				ID:          "run-running-slice/result/acceptance-slice/slice-001",
				WorkspaceID: "ws-running-slice",
				RunID:       "run-running-slice",
				State:       string(common.ResultStateAccepted),
				SourceType:  resultcommon.SourceTypeAcceptanceSlice,
				Summary:     "slice accepted",
				SummaryDetail: &readmodelcore.ResultSummary{
					Diagnostics: map[string]any{
						"resultScope":              resultcommon.SourceTypeAcceptanceSlice,
						"finalCompletionGateState": "not_final_result",
					},
				},
				CreatedAt: "2026-06-28T05:55:05Z",
				UpdatedAt: "2026-06-28T05:55:05Z",
			},
		},
		CoverageSummary: map[string]any{"open": 1, "failed": 1},
	}

	outcome := readmodelprojection.MapWorkspaceOutcome(snapshot)
	if outcome.Accepted || outcome.Terminal || outcome.ResultID != "" {
		t.Fatalf("outcome = %+v, want running non-terminal outcome without accepted slice promotion", outcome)
	}
	if outcome.State != string(common.RunStateActive) || outcome.RawWorkspaceState != string(common.WorkspaceStateActive) {
		t.Fatalf("outcome = %+v, want running outcome with raw workspace active state retained", outcome)
	}
}

func TestMapRunOutcomeIgnoresAcceptedSliceForRunningRun(t *testing.T) {
	snapshot := RunSnapshot{
		Run: Run{
			ID:          "run-slice-running",
			WorkspaceID: "ws-slice-running",
			State:       string(common.RunStateActive),
		},
		Results: []Result{
			{
				ID:          "run-slice-running/result/acceptance-slice/slice-001",
				WorkspaceID: "ws-slice-running",
				RunID:       "run-slice-running",
				State:       string(common.ResultStateAccepted),
				SourceType:  resultcommon.SourceTypeAcceptanceSlice,
				SummaryDetail: &readmodelcore.ResultSummary{
					Diagnostics: map[string]any{
						"resultScope":              resultcommon.SourceTypeAcceptanceSlice,
						"finalCompletionGateState": "not_final_result",
					},
				},
				CreatedAt: "2026-06-28T05:55:05Z",
				UpdatedAt: "2026-06-28T05:55:05Z",
			},
		},
	}

	outcome := readmodelprojection.MapRunOutcome(snapshot)
	if outcome.Accepted || outcome.Terminal || outcome.ResultID != "" {
		t.Fatalf("outcome = %+v, want running non-terminal run outcome without accepted slice promotion", outcome)
	}
	if outcome.State != string(common.RunStateActive) || outcome.RawState != string(common.RunStateActive) {
		t.Fatalf("outcome = %+v, want running state retained", outcome)
	}
}

func TestResultProgressDoesNotCompleteFromAcceptedSlice(t *testing.T) {
	snapshot := RunSnapshot{
		Run: Run{
			ID:          "run-progress-slice",
			WorkspaceID: "ws-progress-slice",
			State:       string(common.RunStateActive),
		},
		Results: []Result{
			{
				ID:          "run-progress-slice/result/acceptance-slice/slice-001",
				WorkspaceID: "ws-progress-slice",
				RunID:       "run-progress-slice",
				State:       string(common.ResultStateAccepted),
				SourceType:  resultcommon.SourceTypeAcceptanceSlice,
				SummaryDetail: &readmodelcore.ResultSummary{
					Diagnostics: map[string]any{
						"resultScope":              resultcommon.SourceTypeAcceptanceSlice,
						"finalCompletionGateState": "not_final_result",
					},
				},
			},
		},
	}

	if got := readmodelprojection.ResultProgressState(snapshot); got == "completed" {
		t.Fatalf("ResultProgressState = %q, want non-completed for accepted slice on running run", got)
	}
	if readmodelprojection.HasAcceptedRunResult(snapshot) {
		t.Fatal("HasAcceptedRunResult = true, want false for accepted slice on running run")
	}
}

func TestFinalAcceptedResultStillProducesTerminalOutcome(t *testing.T) {
	snapshot := WorkspaceSnapshot{
		Workspace: Workspace{
			ID:    "ws-final-accepted",
			State: string(common.WorkspaceStateActive),
		},
		Runs: []Run{
			{ID: "run-final-accepted", WorkspaceID: "ws-final-accepted", State: string(common.RunStateCompleted)},
		},
		Results: []Result{
			{
				ID:          "run-final-accepted/result/dod-assessment",
				WorkspaceID: "ws-final-accepted",
				RunID:       "run-final-accepted",
				State:       string(common.ResultStateAccepted),
				SourceType:  resultcommon.SourceTypeDoDAssessment,
				Summary:     "final accepted result",
				CreatedAt:   "2026-06-28T05:55:05Z",
				UpdatedAt:   "2026-06-28T05:55:05Z",
			},
		},
	}

	outcome := readmodelprojection.MapWorkspaceOutcome(snapshot)
	if !outcome.Accepted || !outcome.Terminal || outcome.State != "accepted" || outcome.ResultID != "run-final-accepted/result/dod-assessment" {
		t.Fatalf("outcome = %+v, want final accepted terminal outcome", outcome)
	}
}

func TestTerminalRejectedResultStillProducesFailedOutcome(t *testing.T) {
	snapshot := WorkspaceSnapshot{
		Workspace: Workspace{
			ID:    "ws-final-rejected",
			State: string(common.WorkspaceStateActive),
		},
		Runs: []Run{
			{ID: "run-final-rejected", WorkspaceID: "ws-final-rejected", State: string(common.RunStateFailed)},
		},
		Results: []Result{
			{
				ID:          "run-final-rejected/result/dod-assessment",
				WorkspaceID: "ws-final-rejected",
				RunID:       "run-final-rejected",
				State:       string(common.ResultStateRejected),
				SourceType:  resultcommon.SourceTypeDoDAssessment,
				Summary:     "final rejected result",
				CreatedAt:   "2026-06-28T05:55:05Z",
				UpdatedAt:   "2026-06-28T05:55:05Z",
			},
		},
	}

	outcome := readmodelprojection.MapWorkspaceOutcome(snapshot)
	if outcome.Accepted || !outcome.Terminal || outcome.State != "failed" || outcome.ResultID != "run-final-rejected/result/dod-assessment" {
		t.Fatalf("outcome = %+v, want final rejected terminal outcome", outcome)
	}
}

func containsResultQualityIssueCode(items []ResultQualityIssue, code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item.Code) == code {
			return true
		}
	}
	return false
}

func TestMapWorkspaceOutcomeUsesLatestTerminalResult(t *testing.T) {
	snapshot := WorkspaceSnapshot{
		Workspace: Workspace{
			ID:    "ws-outcome-latest-result",
			State: string(common.WorkspaceStateActive),
		},
		Runs: []Run{
			{
				ID:            "run-old",
				WorkspaceID:   "ws-outcome-latest-result",
				State:         string(common.RunStateFailed),
				FailureReason: "old failure",
			},
			{
				ID:            "run-new",
				WorkspaceID:   "ws-outcome-latest-result",
				State:         string(common.RunStateFailed),
				FailureReason: "new failure",
			},
		},
		Results: []Result{
			{
				ID:           "run-old/result/dod-assessment",
				WorkspaceID:  "ws-outcome-latest-result",
				RunID:        "run-old",
				State:        string(common.ResultStateRejected),
				Summary:      "old rejected result",
				EvidenceRefs: []string{"run-old/step-1"},
				CreatedAt:    "2026-05-17T07:03:52Z",
				UpdatedAt:    "2026-05-17T07:03:52Z",
			},
			{
				ID:           "run-new/result/dod-assessment",
				WorkspaceID:  "ws-outcome-latest-result",
				RunID:        "run-new",
				State:        string(common.ResultStateRejected),
				Summary:      "new schema rejected result",
				EvidenceRefs: []string{"run-new/assessment"},
				CreatedAt:    "2026-05-17T07:12:30Z",
				UpdatedAt:    "2026-05-17T07:12:30Z",
			},
		},
	}

	outcome := readmodelprojection.MapWorkspaceOutcome(snapshot)
	if outcome.RunID != "run-new" {
		t.Fatalf("outcome runId = %q, want latest terminal result run", outcome.RunID)
	}
	if outcome.ResultID != "run-new/result/dod-assessment" || outcome.Summary != "new schema rejected result" {
		t.Fatalf("outcome = %+v, want latest rejected result summary", outcome)
	}
	if outcome.State != "failed" || outcome.Accepted {
		t.Fatalf("outcome state/accepted = %q/%v, want failed false", outcome.State, outcome.Accepted)
	}
}

func TestMapRunOutcomeKeepsSupersededRunFromStaleRejectedResult(t *testing.T) {
	snapshot := RunSnapshot{
		Run: Run{
			ID:          "run-superseded",
			WorkspaceID: "ws-superseded-outcome",
			State:       string(common.RunStateSuperseded),
		},
		Results: []Result{
			{
				ID:          "run-superseded/result/dod-assessment",
				WorkspaceID: "ws-superseded-outcome",
				RunID:       "run-superseded",
				State:       string(common.ResultStateRejected),
				Summary:     "stale rejected result",
				CreatedAt:   "2026-05-17T07:03:52Z",
				UpdatedAt:   "2026-05-17T07:03:52Z",
			},
		},
	}

	outcome := readmodelprojection.MapRunOutcome(snapshot)
	if outcome.State != string(common.RunStateSuperseded) || outcome.Accepted || outcome.ResultID != "" {
		t.Fatalf("outcome = %+v, want superseded without stale rejected result", outcome)
	}
}

func TestMapWorkspaceOutcomeUsesLatestRunEventOverHistoricalAcceptedResult(t *testing.T) {
	snapshot := WorkspaceSnapshot{
		Workspace: Workspace{
			ID:    "ws-latest-run-event",
			State: string(common.WorkspaceStateActive),
		},
		Runs: []Run{
			{
				ID:          "run-accepted",
				WorkspaceID: "ws-latest-run-event",
				State:       string(common.RunStateCompleted),
			},
			{
				ID:            "run-latest-failed",
				WorkspaceID:   "ws-latest-run-event",
				State:         string(common.RunStateFailed),
				FailureReason: "latest verification failed",
			},
		},
		Results: []Result{
			{
				ID:          "run-accepted/result/dod-assessment",
				WorkspaceID: "ws-latest-run-event",
				RunID:       "run-accepted",
				State:       string(common.ResultStateAccepted),
				Summary:     "historical accepted result",
				CreatedAt:   "2026-05-17T07:00:00Z",
				UpdatedAt:   "2026-05-17T07:00:00Z",
			},
		},
		EventRecords: []EventRecord{
			{
				ID:          "evt-accepted",
				Topic:       runevents.TopicRunStateChanged,
				OccurredAt:  "2026-05-17T07:00:00Z",
				WorkspaceID: "ws-latest-run-event",
				RunID:       "run-accepted",
			},
			{
				ID:          "evt-latest-failed",
				Topic:       runevents.TopicRunStateChanged,
				OccurredAt:  "2026-05-17T08:00:00Z",
				WorkspaceID: "ws-latest-run-event",
				RunID:       "run-latest-failed",
			},
		},
	}

	outcome := readmodelprojection.MapWorkspaceOutcome(snapshot)
	if outcome.RunID != "run-latest-failed" || outcome.State != "failed" || outcome.Accepted {
		t.Fatalf("outcome = %+v, want latest failed run event over historical accepted result", outcome)
	}
	if outcome.ResultID != "" || outcome.Reason != "latest verification failed" {
		t.Fatalf("outcome = %+v, want failed run reason without historical accepted result", outcome)
	}
}

func TestMapWorkspaceOutcomeIgnoresSupersededEventWhenReplacementRunFailed(t *testing.T) {
	snapshot := WorkspaceSnapshot{
		Workspace: Workspace{
			ID:    "ws-replan-failed",
			State: string(common.WorkspaceStateActive),
		},
		Runs: []Run{
			{
				ID:          "run-001",
				WorkspaceID: "ws-replan-failed",
				State:       string(common.RunStateSuperseded),
			},
			{
				ID:            "run-001-replan-001",
				WorkspaceID:   "ws-replan-failed",
				State:         string(common.RunStateFailed),
				FailureReason: "stuck running run recovered",
			},
		},
		EventRecords: []EventRecord{
			{
				ID:          "evt-replan-failed",
				Topic:       runevents.TopicRunStateChanged,
				OccurredAt:  "2026-06-09T14:50:05Z",
				WorkspaceID: "ws-replan-failed",
				RunID:       "run-001-replan-001",
				Payload:     mustReadModelEventPayload(t, map[string]any{recordpayload.PayloadKeyState: string(common.RunStateFailed)}),
			},
			{
				ID:          "evt-old-superseded",
				Topic:       runevents.TopicRunStateChanged,
				OccurredAt:  "2026-06-09T14:50:06Z",
				WorkspaceID: "ws-replan-failed",
				RunID:       "run-001",
				Payload:     mustReadModelEventPayload(t, map[string]any{recordpayload.PayloadKeyState: string(common.RunStateFailed)}),
			},
		},
	}

	outcome := readmodelprojection.MapWorkspaceOutcome(snapshot)
	if outcome.RunID != "run-001-replan-001" || outcome.State != "failed" || outcome.Reason != "stuck running run recovered" {
		t.Fatalf("outcome = %+v, want failed replacement run over newer superseded marker", outcome)
	}
}

func TestMapWorkspaceOutcomeFallsBackToReplacementRunWhenEventsOnlyContainSupersededRun(t *testing.T) {
	snapshot := WorkspaceSnapshot{
		Workspace: Workspace{
			ID:    "ws-replan-failed-with-stale-events",
			State: string(common.WorkspaceStateActive),
		},
		Runs: []Run{
			{
				ID:          "run-001",
				WorkspaceID: "ws-replan-failed-with-stale-events",
				State:       string(common.RunStateSuperseded),
			},
			{
				ID:            "run-001-replan-001",
				WorkspaceID:   "ws-replan-failed-with-stale-events",
				State:         string(common.RunStateFailed),
				FailureReason: "replacement failed",
			},
		},
		EventRecords: []EventRecord{
			{
				ID:          "evt-old-failed-before-superseded",
				Topic:       runevents.TopicRunStateChanged,
				OccurredAt:  "2026-06-09T14:50:06Z",
				WorkspaceID: "ws-replan-failed-with-stale-events",
				RunID:       "run-001",
				Payload:     mustReadModelEventPayload(t, map[string]any{recordpayload.PayloadKeyState: string(common.RunStateFailed)}),
			},
		},
	}

	outcome := readmodelprojection.MapWorkspaceOutcome(snapshot)
	if outcome.RunID != "run-001-replan-001" || outcome.State != "failed" || outcome.Reason != "replacement failed" {
		t.Fatalf("outcome = %+v, want replacement failed run when event page only has superseded run", outcome)
	}
}

func TestMapWorkspaceOutcomeUsesNewerTerminalResultOverStaleRunEvents(t *testing.T) {
	snapshot := WorkspaceSnapshot{
		Workspace: Workspace{
			ID:    "ws-newer-terminal-result",
			State: string(common.WorkspaceStateActive),
		},
		Runs: []Run{
			{
				ID:            "run-old-failed",
				WorkspaceID:   "ws-newer-terminal-result",
				State:         string(common.RunStateFailed),
				FailureReason: "old failed run",
			},
			{
				ID:            "run-current-failed",
				WorkspaceID:   "ws-newer-terminal-result",
				State:         string(common.RunStateFailed),
				FailureReason: "current failed run",
			},
		},
		Results: []Result{
			{
				ID:          "run-old-failed/result/dod-assessment",
				WorkspaceID: "ws-newer-terminal-result",
				RunID:       "run-old-failed",
				State:       string(common.ResultStateRejected),
				Summary:     "old rejected result",
				CreatedAt:   "2026-05-17T07:03:52Z",
				UpdatedAt:   "2026-05-17T07:03:52Z",
			},
			{
				ID:          "run-current-failed/result/dod-assessment",
				WorkspaceID: "ws-newer-terminal-result",
				RunID:       "run-current-failed",
				State:       string(common.ResultStateRejected),
				Summary:     "current rejected result",
				CreatedAt:   "2026-05-17T11:48:05Z",
				UpdatedAt:   "2026-05-17T11:48:05Z",
			},
		},
		EventRecords: []EventRecord{
			{
				ID:          "evt-old-run-state",
				Topic:       runevents.TopicRunStateChanged,
				OccurredAt:  "2026-05-17T07:01:03Z",
				WorkspaceID: "ws-newer-terminal-result",
				RunID:       "run-old-failed",
			},
		},
	}

	outcome := readmodelprojection.MapWorkspaceOutcome(snapshot)
	if outcome.RunID != "run-current-failed" || outcome.ResultID != "run-current-failed/result/dod-assessment" {
		t.Fatalf("outcome = %+v, want newer terminal result over stale run event", outcome)
	}
	if outcome.Summary != "current rejected result" || outcome.State != "failed" || outcome.Accepted {
		t.Fatalf("outcome = %+v, want current failed rejected result", outcome)
	}
}
