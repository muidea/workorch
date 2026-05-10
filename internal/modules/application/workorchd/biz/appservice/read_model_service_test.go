package appservice

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

type countingStepRepository struct {
	base           repository.StepRepository
	listByRunCalls int
}

func (r *countingStepRepository) Create(ctx context.Context, item *runmodel.Step) error {
	return r.base.Create(ctx, item)
}

func (r *countingStepRepository) Get(ctx context.Context, id common.StepID) (*runmodel.Step, error) {
	return r.base.Get(ctx, id)
}

func (r *countingStepRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.Step, error) {
	r.listByRunCalls++
	return r.base.ListByRun(ctx, runID)
}

func (r *countingStepRepository) Update(ctx context.Context, item *runmodel.Step) error {
	return r.base.Update(ctx, item)
}

func (r *countingStepRepository) Delete(ctx context.Context, id common.StepID) error {
	return r.base.Delete(ctx, id)
}

var _ repository.StepRepository = (*countingStepRepository)(nil)

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

func (r *blockingRunListRepository) Delete(ctx context.Context, id common.RunID) error {
	return r.base.Delete(ctx, id)
}

var _ repository.RunRepository = (*blockingRunListRepository)(nil)

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
	base    repository.DecisionResolutionRepository
	needed  int
	started chan common.TicketID
	release chan struct{}
	mu      sync.Mutex
	current int
}

func (r *blockingDecisionResolutionRepository) Create(ctx context.Context, item *decisionmodel.Resolution) error {
	return r.base.Create(ctx, item)
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

func (r *blockingDecisionResolutionRepository) DeleteByTicketID(ctx context.Context, ticketID common.TicketID) error {
	return r.base.DeleteByTicketID(ctx, ticketID)
}

var _ repository.DecisionResolutionRepository = (*blockingDecisionResolutionRepository)(nil)

func TestReadRunSnapshotWithSessionReturnsInvalidArgumentWhenSessionMissing(t *testing.T) {
	if _, err := ReadRunSnapshotWithSession(context.Background(), nil, "run-1"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("err = %v, want invalid argument", err)
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
	exports := session.Bootstrap.Exports()

	workspaceID := common.WorkspaceID("ws-concurrent-resolution-read")
	runID := common.RunID("run-concurrent-resolution-read")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "concurrent resolution read",
		State: common.WorkspaceStateWaitingDecision,
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
			Payload:  map[string]any{"answer": "answer " + strconv.Itoa(i)},
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
		ID:               workspaceID,
		Title:            "run snapshot concurrent",
		State:            common.WorkspaceStateActive,
		WorkingDirectory: "workspaces/run-snapshot-concurrent",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := baseSteps.Create(context.Background(), &runmodel.Step{
		ID:     common.StepID("run-snapshot-concurrent/step-1"),
		RunID:  runID,
		Kind:   "llm/workspace_execution",
		State:  common.StepStateCompleted,
		Output: map[string]any{"summary": "done"},
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
		steps:           &blockingStepRepository{base: baseSteps, started: stepStarted, release: release},
		results:         memory.NewResultRepository(),
		artifacts:       memory.NewArtifactRepository(),
		eventRecords:    memory.NewEventRecordRepository(),
		toolCalls:       memory.NewToolCallRepository(),
		mcpCapabilities: memory.NewMCPCapabilityRepository(),
		patches:         memory.NewPatchRepository(),
		validations:     memory.NewValidationResultRepository(),
		checkpoints:     memory.NewRuntimeCheckpointRepository(),
		tickets:         memory.NewDecisionTicketRepository(),
		resolutions:     memory.NewDecisionResolutionRepository(),
		approvals:       memory.NewApprovalRepository(),
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

func TestReadRunSnapshotWithBackendReturnsExecutionEvidence(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := session.Bootstrap.Exports()

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
		State:  common.StepStateCompleted,
		Output: map[string]any{"result": "done"},
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := exports.Repositories.ToolCalls.Create(ctx, &runmodel.ToolCall{
		ID:          toolCallID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolName:    "shell",
		Arguments:   map[string]any{"command": "go test ./..."},
		State:       common.ToolCallStateCompleted,
		Output:      map[string]any{"exitCode": 0},
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
		Data:             map[string]any{"thread": "main"},
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
		Metadata:     map[string]any{"format": "markdown"},
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
		!reflect.DeepEqual(first.Results[0].SummaryDetail.ArtifactRefs, []string{"artifact-evidence-snapshot"}) {
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
	exports := session.Bootstrap.Exports()

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
		ExecutionContext: map[string]any{
			"capabilityIds": []string{string(capabilityID)},
		},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "mcp/tool_call",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"operation": "mcp/tool_call",
			"serverId":  "mcp-capability-read-model",
			"tool":      "repo.summary",
			"status":    "completed",
		},
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
		Output: map[string]any{
			"operation": "mcp/tool_call",
			"serverId":  "mcp-capability-read-model",
			"tool":      "repo.summary",
			"status":    "completed",
		},
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
	quality := mapResultQuality(snapshot)
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

func TestMapResultQualityFlagsMissingDoDLeafCoverage(t *testing.T) {
	snapshot := RunSnapshot{
		Run: Run{
			ID:          "run-quality-dod-leaf",
			WorkspaceID: "ws-quality-dod-leaf",
			DoDID:       "dod-quality-dod-leaf",
			ExecutionContext: map[string]any{
				"dod": map[string]any{
					"items": []string{
						"### 验收",
						"- [ ] 结果质量：",
						"- [ ] 覆盖主路径并可验证",
						"- [ ] 覆盖错误路径并可验证",
					},
				},
			},
		},
		Steps: []presenter.StepView{
			{ID: "step-1"},
		},
		Results: []Result{
			{
				ID:          "result-quality-dod-leaf",
				WorkspaceID: "ws-quality-dod-leaf",
				RunID:       "run-quality-dod-leaf",
				DoDID:       "dod-quality-dod-leaf",
				State:       "accepted",
				Summary:     "partial dod coverage",
				DoDAssessment: &DoDAssessment{
					Satisfied: true,
					Summary:   "partial dod coverage",
					Items: []DoDAssessmentItem{
						{Item: "覆盖主路径并可验证", State: "satisfied", EvidenceRefs: []string{"step-1"}},
					},
				},
				SummaryDetail: &ResultSummary{
					GoalOutcome:         "main path covered",
					VerificationSummary: "main path verified",
					EvidenceRefs:        []string{"step-1"},
					KnownLimitations:    []string{"error path not covered"},
					NextActions:         []string{"cover error path"},
				},
				EvidenceRefs: []string{"step-1"},
			},
		},
	}
	quality := mapResultQuality(snapshot)
	if len(quality) != 1 {
		t.Fatalf("result quality = %+v, want one trace", quality)
	}
	if quality[0].State != "failed" {
		t.Fatalf("result quality state = %q, want failed", quality[0].State)
	}
	if !containsResultQualityIssueCode(quality[0].Issues, "missing_dod_leaf_coverage") {
		t.Fatalf("result quality issues = %+v, want missing_dod_leaf_coverage", quality[0].Issues)
	}
}

func TestMapResultQualityFlagsWeakDoDItemEvidence(t *testing.T) {
	snapshot := RunSnapshot{
		Run: Run{
			ID:          "run-quality-weak-evidence",
			WorkspaceID: "ws-quality-weak-evidence",
			DoDID:       "dod-quality-weak-evidence",
			ExecutionContext: map[string]any{
				"dod": map[string]any{
					"items": []string{"- [ ] validation evidence is produced by real executed work"},
				},
			},
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
	quality := mapResultQuality(snapshot)
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
		ExecutionContext: map[string]any{
			"capabilityIds": []string{string(capabilityID)},
		},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "mcp/tool_call",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"operation": "mcp/tool_call",
			"serverId":  "mcp-capability-catalog",
			"tool":      "repo.summary",
			"status":    "completed",
		},
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
		Output: map[string]any{
			"operation": "mcp/tool_call",
			"serverId":  "mcp-capability-catalog",
			"tool":      "repo.summary",
			"status":    "completed",
		},
	}); err != nil {
		t.Fatalf("create tool call: %v", err)
	}

	snapshot, err := readRunSnapshot(ctx, readModelDependencies{
		workspaces:      workspaceRepo,
		runs:            runRepo,
		steps:           stepRepo,
		results:         memory.NewResultRepository(),
		artifacts:       memory.NewArtifactRepository(),
		eventRecords:    memory.NewEventRecordRepository(),
		toolCalls:       toolCallRepo,
		mcpCapabilities: capabilityRepo,
		patches:         memory.NewPatchRepository(),
		validations:     memory.NewValidationResultRepository(),
		checkpoints:     memory.NewRuntimeCheckpointRepository(),
		tickets:         memory.NewDecisionTicketRepository(),
		resolutions:     memory.NewDecisionResolutionRepository(),
		approvals:       memory.NewApprovalRepository(),
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

	observability := mapRunObservability(snapshot)

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

func TestMapRunObservabilityProjectsPatchEventsIntoTimeline(t *testing.T) {
	snapshot := RunSnapshot{
		Workspace: Workspace{ID: "ws-observability-patch-event"},
		Run:       Run{ID: "run-observability-patch-event", WorkspaceID: "ws-observability-patch-event", State: string(common.RunStateCompleted)},
		EventRecords: []EventRecord{{
			ID:          "event-patch-applied",
			Topic:       events.TopicPatchApplied,
			SubjectType: "patch",
			SubjectID:   "patch-observability-event",
			WorkspaceID: "ws-observability-patch-event",
			RunID:       "run-observability-patch-event",
			StepID:      "run-observability-patch-event/step-1",
			OccurredAt:  "2026-04-25T10:00:00Z",
			Payload: events.PatchPayload(
				common.PatchID("patch-observability-event"),
				common.StepID("run-observability-patch-event/step-1"),
				common.ToolCallID("run-observability-patch-event/step-1/tool-call"),
				"file_write",
				"result.txt",
				"write result",
				common.PatchStateApplied,
				1,
				0,
				"content-sha",
				"",
				"",
				"",
				"",
			),
		}},
	}

	observability := mapRunObservability(snapshot)

	if observability.Evidence.EventTotal != 1 || !timelineContainsPersistedEvent(observability.Timeline, events.TopicPatchApplied) {
		t.Fatalf("observability = %+v, want patch event in timeline", observability)
	}
}

func TestMapWebSourceTracesProjectsSearchAndFetchSteps(t *testing.T) {
	items := mapWebSourceTraces([]presenter.StepView{
		{
			ID:    "run-web/step-search",
			Kind:  webcommon.StepKindSearch,
			State: string(common.StepStateCompleted),
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
			State: string(common.StepStateCompleted),
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
	observability := mapRunObservability(RunSnapshot{
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
			State: string(common.StepStateCompleted),
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
			Topic:       events.TopicStepStateChanged,
			SubjectType: "step",
			SubjectID:   stepID,
			WorkspaceID: "ws-web-event",
			RunID:       runID,
			StepID:      stepID,
			OccurredAt:  "2026-05-05T00:00:02Z",
			Payload: events.StepPayload(webcommon.StepKindSearch, common.StepStateCompleted, nil, map[string]any{
				"status":     "completed",
				"stage":      "web_search",
				"provider":   "default-http",
				"query":      "golang latest",
				"searchedAt": "2026-05-05T00:00:00Z",
				"results": []map[string]any{{
					"title": "Go Downloads",
					"url":   "https://go.dev/dl/",
				}},
			}),
		}},
		WebSources: []WebSourceTrace{{
			Query:        "golang latest",
			ProviderID:   "default-http",
			EvidenceRefs: []string{stepID},
			Sources:      []WebSource{{Title: "Go Downloads", URL: "https://go.dev/dl/", EvidenceRef: stepID}},
		}},
	}

	observability := mapRunObservability(snapshot)

	if observability.Evidence.EventTotal != 1 || len(observability.WebSources) != 1 {
		t.Fatalf("observability = %+v, want event count and web source projection", observability)
	}
	if !timelineContainsPersistedEvent(observability.Timeline, events.TopicStepStateChanged) ||
		!timelineContainsStepOccurredAt(observability.Timeline, "step", stepID, "2026-05-05T00:00:02Z") {
		t.Fatalf("timeline = %+v, want persisted web step event and step occurrence", observability.Timeline)
	}
}

func TestMapLLMContextBudgetTracesProjectsStructuredBudgets(t *testing.T) {
	steps := []presenter.StepView{
		{
			ID:    "run-ctx/step-1",
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: string(common.StepStateCompleted),
			Output: map[string]any{
				"contextBudget": map[string]any{
					"providerId":            "reasoning",
					"model":                 "gpt-5",
					"maxBytes":              98304,
					"actualBytes":           72440,
					"maxTokens":             16000,
					"estimatedInputTokens":  12000,
					"estimatedOutputTokens": 2048,
					"actualTokens":          14048,
					"shrinkLevel":           3,
					"truncated":             true,
					"rules":                 []string{"compact provider input", "apply priority compaction (P0/P1/P2)"},
					"sourceRefs":            []string{"input.executionContext.goal", "input.graphEvidence"},
					"includedSections":      []string{"p0"},
					"normalizedSections":    []string{"request-intent"},
					"compressedSections":    []string{"p1", "p2"},
					"omittedSections":       []string{"older_p1"},
					"goalDoDCompressed":     false,
					"p0Overflow":            true,
					"tracePolicy":           "summary_refs_only",
					"promptCaptured":        false,
					"estimator":             "bytes_ratio",
					"profile": map[string]any{
						"contextWindowTokens":    200000,
						"maxInputTokens":         160000,
						"reservedOutputTokens":   2048,
						"safetyRatio":            0.8,
						"bytesHardLimit":         98304,
						"estimatedTokensPerByte": 0.25,
						"inputTokenPrice":        0.0,
						"outputTokenPrice":       0.0,
						"currency":               "USD",
					},
				},
			},
		},
		{
			ID:    "run-ctx/step-shell",
			Kind:  shellcommon.StepKindCommand,
			State: string(common.StepStateCompleted),
			Output: map[string]any{
				"status": "completed",
			},
		},
	}
	traces := mapLLMContextBudgetTraces(steps)
	if len(traces) != 1 {
		t.Fatalf("traces = %+v, want one llm context budget trace", traces)
	}
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
		traces[0].Profile["currency"] != "USD" {
		t.Fatalf("trace = %+v, want projected context budget fields", traces[0])
	}
	observability := mapRunObservability(RunSnapshot{
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

	bundle := mapRunContextBundle(snapshot)
	compressed := compressedContextFromRunSnapshot(snapshot)
	progress := runSnapshotTaskProgress(snapshot)

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
		Run:       Run{ID: "run-derived-view-reuse", WorkspaceID: "ws-derived-view-reuse", State: string(common.RunStateRunning)},
		TaskProgress: TaskProgress{
			WorkspaceID: "ws-derived-view-reuse",
			RunID:       "run-derived-view-reuse",
			State:       string(common.RunStateRunning),
			Summary:     "cached derived progress",
		},
		TaskDecompositions: []TaskDecomposition{{
			ID:    "cached-derived-decomposition",
			RunID: "run-derived-view-reuse",
			State: "running",
		}},
	}

	progress := runSnapshotTaskProgress(snapshot)
	decompositions := runSnapshotTaskDecompositions(snapshot)

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

func TestReadRunObservabilityWithSessionReturnsDerivedRuntimeProgress(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "memory", dataRoot)
	exports := session.Bootstrap.Exports()

	workspaceID := common.WorkspaceID("ws-run-observability")
	runID := common.RunID("run-observability")
	stepID := common.StepID("run-observability/step-1")
	toolCallID := common.ToolCallID("tool-call-observability")

	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "observability", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"stage": "llm_execution_graph_plan",
			"streamEvents": []map[string]any{
				{"type": "delta", "delta": `{"completionState":"continue"`},
				{"type": "done"},
			},
		},
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	if err := exports.Repositories.ToolCalls.Create(ctx, &runmodel.ToolCall{
		ID:          toolCallID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolName:    "graph/mock",
		State:       common.ToolCallStateCompleted,
	}); err != nil {
		t.Fatalf("create tool call: %v", err)
	}
	if err := exports.Repositories.Patches.Create(ctx, &runmodel.Patch{
		ID:          common.PatchID("patch-observability"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		ToolCallID:  toolCallID,
		PatchType:   "unified_diff",
		TargetRef:   "README.md",
		State:       common.PatchStateApplied,
	}); err != nil {
		t.Fatalf("create patch: %v", err)
	}
	if err := exports.Repositories.ValidationResults.Create(ctx, &runmodel.ValidationResult{
		ID:             common.ValidationResultID("validation-observability"),
		WorkspaceID:    workspaceID,
		RunID:          runID,
		StepID:         stepID,
		ValidationKind: "runtime_step",
		State:          common.ValidationResultStatePassed,
	}); err != nil {
		t.Fatalf("create validation result: %v", err)
	}
	if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:               common.CheckpointID("checkpoint-observability"),
		WorkspaceID:      workspaceID,
		RunID:            runID,
		CheckpointType:   common.CheckpointTypeRunState,
		CurrentStepID:    stepID,
		RunStateSnapshot: common.RunStateCompleted,
		OccurredAt:       "2026-04-25T10:00:00Z",
		Data:             map[string]any{"event": "step_completed"},
	}); err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	if err := exports.Infrastructure.EventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicRunStateChanged,
		SourceModule: "test",
		SubjectType:  "run",
		SubjectID:    string(runID),
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		Payload:      events.RunPayload("", "", common.RunStateCompleted, "", "", ""),
	}); err != nil {
		t.Fatalf("publish event: %v", err)
	}
	if err := exports.Infrastructure.EventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicRunLLMStreamProgress,
		SourceModule: "test",
		SubjectType:  "llm_stream_progress",
		SubjectID:    string(stepID),
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		StepID:       string(stepID),
		Payload: events.LLMStreamProgressPayload(
			workspaceID,
			runID,
			stepID,
			"provider-a",
			"model-a",
			2,
			28,
			0,
			"completed",
			"llm_execution_graph_plan - completed",
		),
	}); err != nil {
		t.Fatalf("publish llm stream progress event: %v", err)
	}
	if err := exports.Infrastructure.EventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        events.TopicRunLLMUsageRecorded,
		SourceModule: "test",
		SubjectType:  "llm_usage",
		SubjectID:    string(stepID),
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		StepID:       string(stepID),
		Payload: events.LLMUsagePayload(events.LLMUsagePayloadData{
			WorkspaceID:   workspaceID,
			RunID:         runID,
			StepID:        stepID,
			ProviderID:    "provider-a",
			Provider:      "compatible",
			Model:         "model-a",
			Operation:     "chat_completions",
			InputTokens:   128,
			OutputTokens:  16,
			TotalTokens:   144,
			Estimated:     true,
			EstimatedCost: 0.000288,
			Currency:      "USD",
			RequestBytes:  3000,
			ResponseBytes: 400,
			Streaming:     true,
			StartedAt:     "2026-04-25T10:00:01Z",
			CompletedAt:   "2026-04-25T10:00:02Z",
			DurationMs:    1000,
			Status:        "completed",
		}),
	}); err != nil {
		t.Fatalf("publish llm usage event: %v", err)
	}

	item, err := ReadRunObservabilityWithSession(ctx, session, string(runID))
	if err != nil {
		t.Fatalf("ReadRunObservabilityWithSession: %v", err)
	}
	if item.Progress.StepTotal != 1 || item.Progress.StepCompleted != 1 || !item.Progress.Terminal {
		t.Fatalf("progress = %+v, want terminal completed progress", item.Progress)
	}
	if item.Evidence.ToolCallCompleted != 1 || item.Evidence.PatchApplied != 1 || item.Evidence.ValidationPassed != 1 || item.Evidence.RuntimeCheckpointTotal != 1 || item.Evidence.EventTotal == 0 {
		t.Fatalf("evidence = %+v, want derived evidence counters", item.Evidence)
	}
	if item.LatestCheckpoint == nil || item.LatestCheckpoint.ID != "checkpoint-observability" {
		t.Fatalf("latest checkpoint = %+v, want checkpoint-observability", item.LatestCheckpoint)
	}
	if len(item.Timeline) < 2 {
		t.Fatalf("timeline = %+v, want checkpoint and events", item.Timeline)
	}
	if !timelineContainsStepOutput(item.Timeline, string(stepID), "llm_execution_graph_plan", "completed") {
		t.Fatalf("timeline = %+v, want step output timeline item", item.Timeline)
	}
	if !timelineContainsStreamDelta(item.Timeline, string(stepID), `{"completionState":"continue"`, "completed") {
		t.Fatalf("timeline = %+v, want stream delta timeline item", item.Timeline)
	}
	if !timelineContainsStepOccurredAt(item.Timeline, "step", string(stepID), "2026-04-25T10:00:00Z") {
		t.Fatalf("timeline = %+v, want step occurrence derived from checkpoint", item.Timeline)
	}
	if !timelineContainsStepOccurredAt(item.Timeline, "step_stream", string(stepID), "2026-04-25T10:00:00Z") {
		t.Fatalf("timeline = %+v, want stream occurrence derived from step checkpoint", item.Timeline)
	}
	if !timelineContainsLLMStreamProgressEvent(item.Timeline, string(stepID), "completed", 2, 28) {
		t.Fatalf("timeline = %+v, want llm stream progress projected from persisted event", item.Timeline)
	}
	if len(item.LLMUsageTraces) != 1 || item.LLMUsageSummary.EntryTotal != 1 || item.LLMUsageSummary.TotalTokens != 144 {
		t.Fatalf("llm usage traces/summary = %+v / %+v, want one usage trace and totalTokens=144", item.LLMUsageTraces, item.LLMUsageSummary)
	}
	if item.LLMUsageTraces[0].UsageSource != llmcommon.UsageSourceEstimated ||
		!item.LLMUsageTraces[0].CostEstimated ||
		item.LLMUsageTraces[0].CostSource != llmcommon.CostSourceProfilePricing {
		t.Fatalf("llm usage trace = %+v, want normalized usage/cost semantics", item.LLMUsageTraces[0])
	}
	if item.LLMUsageSummary.EstimatedEntries != 1 ||
		item.LLMUsageSummary.ProviderNativeEntries != 0 ||
		item.LLMUsageSummary.CostEstimatedEntries != 1 {
		t.Fatalf("llm usage summary = %+v, want estimated=1 native=0 costEstimated=1", item.LLMUsageSummary)
	}
	if item.LLMUsageSummary.Currency != "USD" ||
		item.LLMUsageSummary.EstimatedCost != 0.000288 ||
		len(item.LLMUsageSummary.CostBreakdown) != 1 ||
		item.LLMUsageSummary.CostBreakdown[0].Currency != "USD" ||
		item.LLMUsageSummary.CostBreakdown[0].EstimatedCost != 0.000288 ||
		item.LLMUsageSummary.CostBreakdown[0].EntryTotal != 1 {
		t.Fatalf("llm usage summary = %+v, want single-currency estimated cost breakdown", item.LLMUsageSummary)
	}
	if len(item.LLMUsageSummary.ProviderModelBreakdown) != 1 ||
		item.LLMUsageSummary.ProviderModelBreakdown[0].ProviderID != "provider-a" ||
		item.LLMUsageSummary.ProviderModelBreakdown[0].Model != "model-a" ||
		item.LLMUsageSummary.ProviderModelBreakdown[0].Currency != "USD" ||
		item.LLMUsageSummary.ProviderModelBreakdown[0].EntryTotal != 1 {
		t.Fatalf("llm usage provider/model breakdown = %+v, want provider-a/model-a one entry", item.LLMUsageSummary.ProviderModelBreakdown)
	}
}

func TestRuntimeTimelineUsesStepStateForStreamItemsWhenRunBlockedOnDecision(t *testing.T) {
	workspaceID := common.WorkspaceID("ws-stream-blocked-timeline")
	runID := common.RunID("run-stream-blocked-timeline")
	stepID := "run-stream-blocked-timeline/zz-llm-repair-step"
	timeline := runtimeTimeline(RunSnapshot{
		Run: Run{
			ID:          string(runID),
			WorkspaceID: string(workspaceID),
			State:       string(common.RunStateBlockedOnDecision),
		},
		Steps: []presenter.StepView{{
			ID:    stepID,
			Kind:  "llm/workspace_execution",
			State: string(common.StepStateCompleted),
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
			Topic:       events.TopicRunLLMStreamProgress,
			SubjectType: "llm_stream_progress",
			SubjectID:   stepID,
			WorkspaceID: string(workspaceID),
			RunID:       string(runID),
			StepID:      stepID,
			Payload: events.LLMStreamProgressPayload(
				workspaceID,
				runID,
				common.StepID(stepID),
				"provider-a",
				"model-a",
				2,
				28,
				0,
				"completed",
				"llm_execution_graph_plan - completed",
			),
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
			if item.RunState != string(common.StepStateCompleted) {
				t.Fatalf("timeline item = %+v, want completed display state", item)
			}
		}
	}
	if checked != 4 {
		t.Fatalf("checked timeline items = %d, want step + two stream chunks + progress event", checked)
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
	timeline := runtimeTimeline(RunSnapshot{
		Run: Run{ID: "run-large-stream", WorkspaceID: "ws-large-stream", State: string(common.RunStateRunning)},
		Steps: []presenter.StepView{{
			ID:    stepID,
			Kind:  "llm/workspace_execution",
			State: string(common.StepStateCompleted),
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
		if item.RunState == string(common.RunStateRunning) {
			t.Fatalf("stream timeline item = %+v, want step display state", item)
		}
	}
	want := streamTimelineHeadItemKeep + streamTimelineTailItemKeep + 1
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
	exports := session.Bootstrap.Exports()

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
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "llm/workspace_execution", State: common.StepStateCompleted}); err != nil {
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
	if !progressContainsItem(progress, string(resultID), "result", "accepted") || !progressContainsItem(progress, string(artifactID), "artifact", "available") {
		t.Fatalf("items = %+v, want result and artifact items", progress.Items)
	}
}

func TestMapLLMUsageTracesFromEventRecordsNormalizesUsageAndCostSemantics(t *testing.T) {
	records := []EventRecord{
		{
			ID:         "evt-1",
			Topic:      events.TopicRunLLMUsageRecorded,
			OccurredAt: "2026-05-07T10:00:00Z",
			RunID:      "run-1",
			StepID:     "run-1/step-1",
			Payload: events.LLMUsagePayload(events.LLMUsagePayloadData{
				WorkspaceID:   "ws-1",
				RunID:         "run-1",
				StepID:        "run-1/step-1",
				ProviderID:    "provider-a",
				Provider:      "compatible",
				Model:         "model-a",
				Operation:     "chat_completions",
				InputTokens:   120,
				OutputTokens:  30,
				TotalTokens:   150,
				Estimated:     true,
				EstimatedCost: 0.0002,
				Currency:      "USD",
				Status:        "completed",
			}),
		},
		{
			ID:         "evt-2",
			Topic:      events.TopicRunLLMUsageRecorded,
			OccurredAt: "2026-05-07T10:00:01Z",
			RunID:      "run-1",
			StepID:     "run-1/step-2",
			Payload: events.LLMUsagePayload(events.LLMUsagePayloadData{
				WorkspaceID:  "ws-1",
				RunID:        "run-1",
				StepID:       "run-1/step-2",
				ProviderID:   "provider-b",
				Provider:     "compatible",
				Model:        "model-b",
				Operation:    "chat_completions",
				InputTokens:  60,
				OutputTokens: 10,
				TotalTokens:  70,
				Estimated:    false,
				Status:       "completed",
			}),
		},
	}

	traces := mapLLMUsageTracesFromEventRecords(records, "run-1")
	if len(traces) != 2 {
		t.Fatalf("llm usage traces length = %d, want 2", len(traces))
	}
	if traces[0].UsageSource != llmcommon.UsageSourceEstimated ||
		!traces[0].CostEstimated ||
		traces[0].CostSource != llmcommon.CostSourceProfilePricing {
		t.Fatalf("trace[0] = %+v, want normalized estimated usage/cost semantics", traces[0])
	}
	if traces[1].UsageSource != llmcommon.UsageSourceProviderNative ||
		traces[1].CostEstimated {
		t.Fatalf("trace[1] = %+v, want normalized provider_native usage semantics", traces[1])
	}

	summary := summarizeLLMUsage(traces)
	if summary.EntryTotal != 2 ||
		summary.EstimatedEntries != 1 ||
		summary.ProviderNativeEntries != 1 ||
		summary.CostEstimatedEntries != 1 {
		t.Fatalf("summary = %+v, want entry=2 estimated=1 native=1 costEstimated=1", summary)
	}
	if len(summary.ProviderModelBreakdown) != 2 {
		t.Fatalf("summary breakdown = %+v, want 2 provider/model entries", summary.ProviderModelBreakdown)
	}
	if summary.ProviderModelBreakdown[0].ProviderID != "provider-a" ||
		summary.ProviderModelBreakdown[0].Model != "model-a" ||
		summary.ProviderModelBreakdown[0].EntryTotal != 1 ||
		summary.ProviderModelBreakdown[1].ProviderID != "provider-b" ||
		summary.ProviderModelBreakdown[1].Model != "model-b" ||
		summary.ProviderModelBreakdown[1].EntryTotal != 1 {
		t.Fatalf("summary breakdown = %+v, want provider-a/model-a and provider-b/model-b", summary.ProviderModelBreakdown)
	}
}

func TestSummarizeLLMUsageKeepsCostCurrencyAware(t *testing.T) {
	summary := summarizeLLMUsage([]LLMUsageTrace{
		{
			ProviderID:    "provider-a",
			Provider:      "compatible",
			Model:         "model-a",
			InputTokens:   100,
			OutputTokens:  10,
			TotalTokens:   110,
			EstimatedCost: 0.10,
			CostEstimated: true,
			CostSource:    llmcommon.CostSourceProfilePricing,
			Currency:      "usd",
			Status:        "completed",
			UsageSource:   llmcommon.UsageSourceEstimated,
			Estimated:     true,
		},
		{
			ProviderID:    "provider-a",
			Provider:      "compatible",
			Model:         "model-a",
			InputTokens:   200,
			OutputTokens:  20,
			TotalTokens:   220,
			EstimatedCost: 0.20,
			CostEstimated: true,
			CostSource:    llmcommon.CostSourceProfilePricing,
			Currency:      "CNY",
			Status:        "completed",
			UsageSource:   llmcommon.UsageSourceProviderNative,
		},
	})

	if summary.EstimatedCost != 0 || summary.Currency != "" {
		t.Fatalf("summary = %+v, want no top-level cross-currency estimated cost", summary)
	}
	if len(summary.CostBreakdown) != 2 {
		t.Fatalf("summary cost breakdown = %+v, want two currencies", summary.CostBreakdown)
	}
	if summary.CostBreakdown[0].Currency != "CNY" ||
		summary.CostBreakdown[0].EstimatedCost != 0.20 ||
		summary.CostBreakdown[0].EntryTotal != 1 ||
		summary.CostBreakdown[1].Currency != "USD" ||
		summary.CostBreakdown[1].EstimatedCost != 0.10 ||
		summary.CostBreakdown[1].EntryTotal != 1 {
		t.Fatalf("summary cost breakdown = %+v, want currency-separated costs", summary.CostBreakdown)
	}
	if len(summary.ProviderModelBreakdown) != 2 {
		t.Fatalf("provider/model breakdown = %+v, want currency-separated entries", summary.ProviderModelBreakdown)
	}
	if summary.ProviderModelBreakdown[0].Currency != "CNY" ||
		summary.ProviderModelBreakdown[0].EstimatedCost != 0.20 ||
		summary.ProviderModelBreakdown[1].Currency != "USD" ||
		summary.ProviderModelBreakdown[1].EstimatedCost != 0.10 {
		t.Fatalf("provider/model breakdown = %+v, want same provider/model separated by currency", summary.ProviderModelBreakdown)
	}
}

func TestReadRunSnapshotWithBackendProjectsCompletedRunWithoutResultAsFailedOutcome(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := session.Bootstrap.Exports()

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

func TestMapTaskProgressProjectsRepairLoopState(t *testing.T) {
	snapshot := RunSnapshot{
		Workspace: Workspace{ID: "ws-repair-loop"},
		Run: Run{
			ID:          "run-repair-loop",
			WorkspaceID: "ws-repair-loop",
			State:       string(common.RunStateRunning),
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
				State: string(common.StepStateCompleted),
				Input: map[string]any{
					"runtimeRepairFollowUp": true,
					"runtimeRepairAttempt":  1,
				},
			},
			{
				ID:    "run-repair-loop/zz-llm-repair-run-tests-2",
				Kind:  "llm/workspace_execution",
				State: string(common.StepStateDispatched),
				Input: map[string]any{
					"runtimeRepairFollowUp": true,
					"runtimeRepairAttempt":  2,
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

	progress := mapTaskProgress(snapshot)
	if progress.RepairLoop.State != "repairing" ||
		progress.RepairLoop.AttemptCount != 2 ||
		progress.RepairLoop.LastAttempt != 2 ||
		progress.RepairLoop.LastRepairStepID != "run-repair-loop/zz-llm-repair-run-tests-2" ||
		progress.RepairLoop.LastFailureClass != "shell_command_failed" ||
		progress.RepairLoop.LastFailureSignature != "kind=shell/command" ||
		progress.RepairLoop.LastFailureStepID != "run-repair-loop/block/run-tests" ||
		progress.RepairLoop.NextAction != "continue_run" {
		t.Fatalf("repairLoop = %+v, want repairing projection with attempts/failure/next action", progress.RepairLoop)
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
				State: string(common.StepStateCompleted),
				Input: map[string]any{
					"runtimeRepairFollowUp": true,
					"runtimeRepairAttempt":  1,
				},
			},
			{
				ID:    "run-repair-escalation/zz-llm-repair-run-tests-2",
				Kind:  "llm/workspace_execution",
				State: string(common.StepStateCompleted),
				Input: map[string]any{
					"runtimeRepairFollowUp": true,
					"runtimeRepairAttempt":  2,
				},
			},
		},
		Decisions: []Decision{
			{
				ID:     "ticket-repair-exhausted",
				RunID:  "run-repair-escalation",
				Reason: "runtime repair attempts exhausted after 2 attempts",
				State:  string(common.DecisionTicketStateOpen),
				Context: map[string]any{
					"attempts":         2,
					"failureClass":     "shell_command_failed",
					"failureSignature": "kind=shell/command",
				},
			},
		},
	}

	progress := mapTaskProgress(snapshot)
	if progress.RepairLoop.State != "escalated_decision" ||
		!progress.RepairLoop.Escalated ||
		progress.RepairLoop.EscalationTicketID != "ticket-repair-exhausted" ||
		progress.RepairLoop.EscalationReason == "" ||
		progress.RepairLoop.AttemptCount != 2 ||
		progress.RepairLoop.NextAction != "resolve_decision_ticket" {
		t.Fatalf("repairLoop = %+v, want escalated decision projection", progress.RepairLoop)
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
	exports := session.Bootstrap.Exports()

	workspaceID := common.WorkspaceID("ws-context-bundle")
	runID := common.RunID("run-context-bundle")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:               workspaceID,
		Title:            "context bundle workspace",
		State:            common.WorkspaceStateActive,
		WorkingDirectory: "workspaces/context-bundle",
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		State:       common.RunStateRunning,
		ExecutionContext: map[string]any{
			"goal": map[string]any{"id": "goal-context-bundle", "text": "deliver context bundle"},
			"dod":  map[string]any{"id": "dod-context-bundle", "state": "ready", "items": []any{"bundle exposes recovery context"}},
			"skills": []any{
				map[string]any{"id": "skill-context", "name": "context"},
			},
			"mcpServers": []any{
				map[string]any{"id": "mcp-context", "name": "context mcp"},
			},
			"memories": []any{
				map[string]any{"id": "memory-context", "summary": "prior context"},
			},
		},
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	for idx := 0; idx < 11; idx++ {
		stepID := common.StepID("run-context-bundle/step-" + strconv.Itoa(idx+1))
		if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
			ID:     stepID,
			RunID:  runID,
			Kind:   "llm/workspace_execution",
			State:  common.StepStateCompleted,
			Output: map[string]any{"stage": "context_bundle_stage"},
		}); err != nil {
			t.Fatalf("create step: %v", err)
		}
		if err := exports.Repositories.RuntimeCheckpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
			ID:               common.CheckpointID("checkpoint-context-bundle-" + strconv.Itoa(idx+1)),
			WorkspaceID:      workspaceID,
			RunID:            runID,
			CheckpointType:   common.CheckpointTypeRunState,
			CurrentStepID:    stepID,
			RunStateSnapshot: common.RunStateRunning,
			OccurredAt:       "2026-04-29T10:00:" + twoDigitSecond(idx) + "Z",
			Data:             map[string]any{"event": "step_completed"},
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
	if bundle.TaskProgress.RunID != string(runID) || bundle.Recovery.Mode != "continue" {
		t.Fatalf("bundle progress/recovery = %+v/%+v, want continue run progress", bundle.TaskProgress, bundle.Recovery)
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
	lightweight := compressedContextFromRunSnapshot(snapshot)
	if !reflect.DeepEqual(lightweight, bundle.CompressedContext) {
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
	recoveryCompressed := compressedContextFromRunSnapshot(recoverySnapshot)
	if !reflect.DeepEqual(recoveryCompressed, bundle.CompressedContext) {
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
	exports := session.Bootstrap.Exports()

	workspaceID := common.WorkspaceID("ws-task-decomposition")
	runID := common.RunID("run-task-decomposition")
	rootStepID := common.StepID("run-task-decomposition/step-1")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "task decomposition", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := exports.Repositories.Runs.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    rootStepID,
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"stage": "llm_execution_graph_plan",
			"executionGraph": map[string]any{
				"nodes": []map[string]any{
					{"id": "inspect", "kind": "shell/command", "input": map[string]any{"command": "hostname"}},
					{"id": "verify", "kind": "shell/command", "dependsOn": []string{"inspect"}, "input": map[string]any{"command": "true"}},
				},
			},
		},
	}); err != nil {
		t.Fatalf("create root step: %v", err)
	}
	if err := exports.Repositories.Steps.Create(ctx, &runmodel.Step{
		ID:    common.StepID("run-task-decomposition/block/inspect"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Input: map[string]any{
			"graphNodeId":     "inspect",
			"graphRootStepId": string(rootStepID),
		},
		Output: map[string]any{"exitCode": 0},
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
		State: common.StepStateDispatched,
		Input: map[string]any{
			"graphNodeId":     "verify",
			"graphRootStepId": string(rootStepID),
		},
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
		if item.Source != "llm_stream_progress_event" || item.Topic != events.TopicRunLLMStreamProgress || item.StepID != stepID {
			continue
		}
		if item.Type != status {
			continue
		}
		if readModelIntValue(item.Data[events.PayloadKeyChunkCount]) != chunkCount ||
			readModelIntValue(item.Data[events.PayloadKeyReceivedBytes]) != receivedBytes {
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
	exports, err := session.Exports()
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
	if item.Workspace.State != "blocked" || item.Workspace.ActiveRunID != approvalRunID {
		t.Fatalf("workspace projection = %+v, want blocked active run %s", item.Workspace, approvalRunID)
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
	if item.EventRecords[0].WorkspaceID != workspaceID || item.EventRecords[0].Topic != events.TopicWorkspaceCreated {
		t.Fatalf("eventRecords = %+v, want workspace-created event", item.EventRecords)
	}
}

func TestReadWorkspaceSnapshotProjectsWorkspaceStateFromRun(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := session.Bootstrap.Exports()

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
		State:       common.RunStateRunning,
	}
	if err := exports.Repositories.Runs.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	tests := []struct {
		runState       common.RunState
		workspaceState string
	}{
		{runState: common.RunStateRunning, workspaceState: "running"},
		{runState: common.RunStateBlockedOnDecision, workspaceState: "waiting_decision"},
		{runState: common.RunStateBlockedOnApproval, workspaceState: "blocked"},
		{runState: common.RunStateValidating, workspaceState: "validating"},
		{runState: common.RunStateCompleted, workspaceState: "completed"},
		{runState: common.RunStateFailed, workspaceState: "failed"},
		{runState: common.RunStateCancelled, workspaceState: "failed"},
		{runState: common.RunStateSuperseded, workspaceState: "failed"},
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
	exports := session.Bootstrap.Exports()

	workspaceID := common.WorkspaceID("ws-cached-run-projection")
	runID := common.RunID("run-cached-run-projection")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "cached run projection",
		State:       common.WorkspaceStateCompleted,
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
		Workspace: Workspace{ID: string(workspaceID), State: string(common.WorkspaceStateCompleted)},
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
	exports := session.Bootstrap.Exports()

	workspaceID := common.WorkspaceID("ws-cached-run-no-step-read")
	runID := common.RunID("run-cached-run-no-step-read")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "cached run no step read",
		State:       common.WorkspaceStateRunning,
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
		Workspace: Workspace{ID: string(workspaceID), State: string(common.WorkspaceStateRunning), ActiveRunID: string(runID)},
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
		workspaces:      workspaceRepo,
		goals:           goalRepo,
		dods:            dodRepo,
		runs:            memory.NewRunRepository(),
		steps:           memory.NewStepRepository(),
		results:         memory.NewResultRepository(),
		artifacts:       memory.NewArtifactRepository(),
		eventRecords:    memory.NewEventRecordRepository(),
		toolCalls:       memory.NewToolCallRepository(),
		mcpCapabilities: memory.NewMCPCapabilityRepository(),
		patches:         memory.NewPatchRepository(),
		validations:     memory.NewValidationResultRepository(),
		checkpoints:     memory.NewRuntimeCheckpointRepository(),
		tickets:         memory.NewDecisionTicketRepository(),
		resolutions:     memory.NewDecisionResolutionRepository(),
		approvals:       memory.NewApprovalRepository(),
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
		ID:               workspaceID,
		Title:            "workspace snapshot concurrent",
		State:            common.WorkspaceStateActive,
		WorkingDirectory: "workspaces/workspace-snapshot-concurrent",
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
		workspaces:      &blockingWorkspaceRepository{base: baseWorkspaces, started: workspaceStarted, release: release},
		runs:            &blockingRunListRepository{base: baseRuns, started: runListStarted, release: release},
		steps:           memory.NewStepRepository(),
		results:         memory.NewResultRepository(),
		artifacts:       memory.NewArtifactRepository(),
		eventRecords:    memory.NewEventRecordRepository(),
		toolCalls:       memory.NewToolCallRepository(),
		mcpCapabilities: memory.NewMCPCapabilityRepository(),
		patches:         memory.NewPatchRepository(),
		validations:     memory.NewValidationResultRepository(),
		checkpoints:     memory.NewRuntimeCheckpointRepository(),
		tickets:         memory.NewDecisionTicketRepository(),
		resolutions:     memory.NewDecisionResolutionRepository(),
		approvals:       memory.NewApprovalRepository(),
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

func TestReadWorkspaceSnapshotProjectsAcceptedResultOverStaleTerminalActiveRun(t *testing.T) {
	dataRoot := t.TempDir()
	session, ctx := newTestRuntimeSession(t, "file", dataRoot)
	exports := session.Bootstrap.Exports()

	workspaceID := common.WorkspaceID("ws-stale-active-result")
	staleRunID := common.RunID("run-stale-failed")
	completedRunID := common.RunID("run-result-completed")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "stale projection",
		State:       common.WorkspaceStateFailed,
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
	if snapshot.Workspace.State != string(common.WorkspaceStateCompleted) {
		t.Fatalf("workspace state = %q, want completed", snapshot.Workspace.State)
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
	exports := session.Bootstrap.Exports()

	workspaceID := common.WorkspaceID("ws-latest-failure-projection")
	completedRunID := common.RunID("run-completed-with-result")
	failedRunID := common.RunID("run-latest-provider-failed")
	if err := exports.Repositories.Workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:    workspaceID,
		Title: "latest failure projection",
		State: common.WorkspaceStateCompleted,
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
		Version:     events.WorkorchEventVersion,
		Topic:       events.TopicRunCreated,
		OccurredAt:  "2026-04-29T10:00:00Z",
		SubjectType: "run",
		SubjectID:   string(completedRunID),
		WorkspaceID: workspaceID,
		RunID:       completedRunID,
		Payload:     map[string]any{events.PayloadKeyState: string(common.RunStateCompleted)},
	}); err != nil {
		t.Fatalf("append completed event: %v", err)
	}
	if err := exports.Repositories.EventRecords.Append(ctx, &eventstreammodel.EventRecord{
		ID:          common.EventRecordID("evt-run-latest-failed"),
		Version:     events.WorkorchEventVersion,
		Topic:       events.TopicRunStateChanged,
		OccurredAt:  "2026-04-30T10:00:00Z",
		SubjectType: "run",
		SubjectID:   string(failedRunID),
		WorkspaceID: workspaceID,
		RunID:       failedRunID,
		Payload:     map[string]any{events.PayloadKeyState: string(common.RunStateFailed), events.PayloadKeyFailureReason: "llm provider returned html"},
	}); err != nil {
		t.Fatalf("append failed event: %v", err)
	}

	snapshot, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, string(workspaceID))
	if err != nil {
		t.Fatalf("ReadWorkspaceSnapshotWithBackend: %v", err)
	}
	if snapshot.Workspace.State != string(common.WorkspaceStateFailed) {
		t.Fatalf("workspace state = %q, want failed from latest run", snapshot.Workspace.State)
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

func TestMapWorkspaceOutcomePrefersLatestTerminalRunOverStaleTaskProgress(t *testing.T) {
	olderRunID := "run-older-accepted"
	latestRunID := "run-latest-accepted"
	snapshot := WorkspaceSnapshot{
		Workspace: Workspace{
			ID:    "ws-latest-outcome",
			State: string(common.WorkspaceStateCompleted),
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
				Topic:       events.TopicRunStateChanged,
				OccurredAt:  "2026-05-08T10:00:00Z",
				WorkspaceID: "ws-latest-outcome",
				RunID:       olderRunID,
			},
			{
				ID:          "evt-latest",
				Topic:       events.TopicRunStateChanged,
				OccurredAt:  "2026-05-08T11:00:00Z",
				WorkspaceID: "ws-latest-outcome",
				RunID:       latestRunID,
			},
		},
	}

	outcome := mapWorkspaceOutcome(snapshot)
	if outcome.State != "accepted" || !outcome.Accepted || outcome.RunID != latestRunID || outcome.Summary != "latest accepted result" {
		t.Fatalf("outcome = %+v, want latest accepted run despite stale task progress", outcome)
	}
}

func TestReadTerminalRunModelsWithBackendRemainStableAcrossReloads(t *testing.T) {
	dataRoot := t.TempDir()
	workspaceID := "ws-run-terminal-reload"
	runID := "run-approval-terminal-reload"
	started := prepareApprovalRun(t, dataRoot, workspaceID, runID)
	if _, err := RejectApprovalWithBackend(context.Background(), "file", dataRoot, started.BlockingApprovalID); err != nil {
		t.Fatalf("RejectApprovalWithBackend: %v", err)
	}
	if _, err := ResumeRunWithBackend(context.Background(), "file", dataRoot, runID); err != nil {
		t.Fatalf("ResumeRunWithBackend: %v", err)
	}

	firstRun, err := ReadRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("first ReadRunWithBackend: %v", err)
	}
	firstSnapshot, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("first ReadRunSnapshotWithBackend: %v", err)
	}
	firstStatus, err := ReadRunStatusWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("first ReadRunStatusWithBackend: %v", err)
	}
	firstWorkspace, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, workspaceID)
	if err != nil {
		t.Fatalf("first ReadWorkspaceSnapshotWithBackend: %v", err)
	}

	secondRun, err := ReadRunWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("second ReadRunWithBackend: %v", err)
	}
	secondSnapshot, err := ReadRunSnapshotWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("second ReadRunSnapshotWithBackend: %v", err)
	}
	secondStatus, err := ReadRunStatusWithBackend(context.Background(), "file", dataRoot, runID)
	if err != nil {
		t.Fatalf("second ReadRunStatusWithBackend: %v", err)
	}
	secondWorkspace, err := ReadWorkspaceSnapshotWithBackend(context.Background(), "file", dataRoot, workspaceID)
	if err != nil {
		t.Fatalf("second ReadWorkspaceSnapshotWithBackend: %v", err)
	}

	if firstRun.State != "failed" || firstRun.FailureReason == "" {
		t.Fatalf("first run = %+v, want failed with failure reason", firstRun)
	}
	if !reflect.DeepEqual(firstRun, secondRun) {
		t.Fatalf("run mismatch after reload: first=%+v second=%+v", firstRun, secondRun)
	}
	if !reflect.DeepEqual(firstSnapshot, secondSnapshot) {
		t.Fatalf("snapshot mismatch after reload: first=%+v second=%+v", firstSnapshot, secondSnapshot)
	}
	if !reflect.DeepEqual(firstStatus, secondStatus) {
		t.Fatalf("status mismatch after reload: first=%+v second=%+v", firstStatus, secondStatus)
	}
	if !reflect.DeepEqual(firstWorkspace, secondWorkspace) {
		t.Fatalf("workspace snapshot mismatch after reload: first=%+v second=%+v", firstWorkspace, secondWorkspace)
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
