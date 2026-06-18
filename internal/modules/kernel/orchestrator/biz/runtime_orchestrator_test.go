package orchestrator

import (
	"context"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"

	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type stubRuntimeAdapter struct {
	mu        sync.Mutex
	output    map[string]any
	err       error
	calls     []common.StepID
	fn        func(common.StepID) (map[string]any, error)
	fnRequest func(runtimemodel.StepExecutionRequest) (map[string]any, error)
}

func (s *stubRuntimeAdapter) ExecuteRuntimeStep(_ context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	stepID := request.StepID
	s.mu.Lock()
	s.calls = append(s.calls, stepID)
	s.mu.Unlock()
	if s.fnRequest != nil {
		output, err := s.fnRequest(request)
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	if s.fn != nil {
		output, err := s.fn(stepID)
		return runtimemodel.StepExecutionResult{Output: output}, err
	}
	if s.err != nil {
		return runtimemodel.StepExecutionResult{}, s.err
	}
	return runtimemodel.StepExecutionResult{Output: s.output}, nil
}

type requestCapturingRuntimeAdapter struct {
	requests []runtimemodel.StepExecutionRequest
	output   map[string]any
	fn       func(runtimemodel.StepExecutionRequest) map[string]any
}

func (a *requestCapturingRuntimeAdapter) ExecuteRuntimeStep(_ context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	a.requests = append(a.requests, request)
	if a.fn != nil {
		return runtimemodel.StepExecutionResult{Output: a.fn(request)}, nil
	}
	return runtimemodel.StepExecutionResult{Output: a.output}, nil
}

type requestOnlyRuntimeAdapter struct {
	requests  []runtimemodel.StepExecutionRequest
	accepted  []runtimemodel.RuntimeStepExecutionAccepted
	execCalls int
}

func (a *requestOnlyRuntimeAdapter) RequestRuntimeStepExecution(_ context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.RuntimeStepExecutionAccepted, error) {
	a.requests = append(a.requests, request)
	accepted := runtimemodel.RuntimeStepExecutionAccepted{
		WorkspaceID: request.WorkspaceID,
		RunID:       request.RunID,
		StepID:      request.StepID,
		StepKind:    request.Kind,
		RequestID:   "request-only",
		QueueKey:    "queue-only",
		AcceptedAt:  "2026-06-17T00:00:00Z",
	}
	a.accepted = append(a.accepted, accepted)
	return accepted, nil
}

func (a *requestOnlyRuntimeAdapter) ExecuteRuntimeStep(context.Context, runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	a.execCalls++
	return runtimemodel.StepExecutionResult{}, common.ErrConflict
}

type countingStepRepository struct {
	inner                  *memory.StepRepository
	getCount               atomic.Int64
	listCount              atomic.Int64
	graphEvidenceListCount atomic.Int64
	updateCount            atomic.Int64
}

func newCountingStepRepository() *countingStepRepository {
	return &countingStepRepository{inner: memory.NewStepRepository()}
}

func (r *countingStepRepository) Create(ctx context.Context, item *runmodel.Step) error {
	return r.inner.Create(ctx, item)
}

func (r *countingStepRepository) Get(ctx context.Context, id common.StepID) (*runmodel.Step, error) {
	r.getCount.Add(1)
	return r.inner.Get(ctx, id)
}

func (r *countingStepRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.Step, error) {
	r.listCount.Add(1)
	for depth := 1; depth < 16; depth++ {
		pc, _, _, ok := goruntime.Caller(depth)
		if !ok {
			break
		}
		fn := goruntime.FuncForPC(pc)
		if fn != nil && strings.Contains(fn.Name(), "completedGraphEvidence") {
			r.graphEvidenceListCount.Add(1)
			break
		}
	}
	return r.inner.ListByRun(ctx, runID)
}

func (r *countingStepRepository) ListByRunSummary(ctx context.Context, runID common.RunID) ([]*runmodel.Step, error) {
	return r.inner.ListByRunSummary(ctx, runID)
}

func (r *countingStepRepository) Update(ctx context.Context, item *runmodel.Step) error {
	r.updateCount.Add(1)
	return r.inner.Update(ctx, item)
}

func (r *countingStepRepository) Delete(ctx context.Context, id common.StepID) error {
	return r.inner.Delete(ctx, id)
}

type blockingCompletedStepRepository struct {
	inner   *memory.StepRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingCompletedStepRepository) Create(ctx context.Context, item *runmodel.Step) error {
	return r.inner.Create(ctx, item)
}

func (r *blockingCompletedStepRepository) Get(ctx context.Context, id common.StepID) (*runmodel.Step, error) {
	return r.inner.Get(ctx, id)
}

func (r *blockingCompletedStepRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.Step, error) {
	return r.inner.ListByRun(ctx, runID)
}

func (r *blockingCompletedStepRepository) ListByRunSummary(ctx context.Context, runID common.RunID) ([]*runmodel.Step, error) {
	return r.inner.ListByRunSummary(ctx, runID)
}

func (r *blockingCompletedStepRepository) Update(ctx context.Context, item *runmodel.Step) error {
	if item != nil && item.State == common.StepStateCompleted {
		if r.started != nil {
			r.started <- "step-completed-update"
		}
		if r.release != nil {
			select {
			case <-r.release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return r.inner.Update(ctx, item)
}

func (r *blockingCompletedStepRepository) Delete(ctx context.Context, id common.StepID) error {
	return r.inner.Delete(ctx, id)
}

type countingRunRepository struct {
	inner    *memory.RunRepository
	getCount atomic.Int64
}

func newCountingRunRepository() *countingRunRepository {
	return &countingRunRepository{inner: memory.NewRunRepository()}
}

func (r *countingRunRepository) Create(ctx context.Context, item *runmodel.Run) error {
	return r.inner.Create(ctx, item)
}

func (r *countingRunRepository) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	r.getCount.Add(1)
	return r.inner.Get(ctx, id)
}

func (r *countingRunRepository) List(ctx context.Context) ([]*runmodel.Run, error) {
	return r.inner.List(ctx)
}

func (r *countingRunRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	return r.inner.ListByWorkspaceID(ctx, workspaceID)
}

func (r *countingRunRepository) Update(ctx context.Context, item *runmodel.Run) error {
	return r.inner.Update(ctx, item)
}

func (r *countingRunRepository) Delete(ctx context.Context, id common.RunID) error {
	return r.inner.Delete(ctx, id)
}

type blockingCompletedRunRepository struct {
	inner   *memory.RunRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingCompletedRunRepository) Create(ctx context.Context, item *runmodel.Run) error {
	return r.inner.Create(ctx, item)
}

func (r *blockingCompletedRunRepository) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	return r.inner.Get(ctx, id)
}

func (r *blockingCompletedRunRepository) List(ctx context.Context) ([]*runmodel.Run, error) {
	return r.inner.List(ctx)
}

func (r *blockingCompletedRunRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	return r.inner.ListByWorkspaceID(ctx, workspaceID)
}

func (r *blockingCompletedRunRepository) Update(ctx context.Context, item *runmodel.Run) error {
	if item != nil && item.State == common.RunStateCompleted {
		if r.started != nil {
			r.started <- "run-completed-update"
		}
		if r.release != nil {
			select {
			case <-r.release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return r.inner.Update(ctx, item)
}

func (r *blockingCompletedRunRepository) Delete(ctx context.Context, id common.RunID) error {
	return r.inner.Delete(ctx, id)
}

type blockingValidatingRunRepository struct {
	inner   *memory.RunRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingValidatingRunRepository) Create(ctx context.Context, item *runmodel.Run) error {
	return r.inner.Create(ctx, item)
}

func (r *blockingValidatingRunRepository) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	return r.inner.Get(ctx, id)
}

func (r *blockingValidatingRunRepository) List(ctx context.Context) ([]*runmodel.Run, error) {
	return r.inner.List(ctx)
}

func (r *blockingValidatingRunRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	return r.inner.ListByWorkspaceID(ctx, workspaceID)
}

func (r *blockingValidatingRunRepository) Update(ctx context.Context, item *runmodel.Run) error {
	if item != nil && item.State == common.RunStateValidating {
		if r.started != nil {
			r.started <- "run-validating-update"
		}
		if r.release != nil {
			select {
			case <-r.release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return r.inner.Update(ctx, item)
}

func (r *blockingValidatingRunRepository) Delete(ctx context.Context, id common.RunID) error {
	return r.inner.Delete(ctx, id)
}

type blockingRunningRunRepository struct {
	inner   *memory.RunRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingRunningRunRepository) Create(ctx context.Context, item *runmodel.Run) error {
	return r.inner.Create(ctx, item)
}

func (r *blockingRunningRunRepository) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	return r.inner.Get(ctx, id)
}

func (r *blockingRunningRunRepository) List(ctx context.Context) ([]*runmodel.Run, error) {
	return r.inner.List(ctx)
}

func (r *blockingRunningRunRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	return r.inner.ListByWorkspaceID(ctx, workspaceID)
}

func (r *blockingRunningRunRepository) Update(ctx context.Context, item *runmodel.Run) error {
	if item != nil && item.State == common.RunStateRunning {
		if r.started != nil {
			r.started <- "run-running-update"
		}
		if r.release != nil {
			select {
			case <-r.release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return r.inner.Update(ctx, item)
}

func (r *blockingRunningRunRepository) Delete(ctx context.Context, id common.RunID) error {
	return r.inner.Delete(ctx, id)
}

type blockingBlockedRunRepository struct {
	inner   *memory.RunRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingBlockedRunRepository) Create(ctx context.Context, item *runmodel.Run) error {
	return r.inner.Create(ctx, item)
}

func (r *blockingBlockedRunRepository) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	return r.inner.Get(ctx, id)
}

func (r *blockingBlockedRunRepository) List(ctx context.Context) ([]*runmodel.Run, error) {
	return r.inner.List(ctx)
}

func (r *blockingBlockedRunRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	return r.inner.ListByWorkspaceID(ctx, workspaceID)
}

func (r *blockingBlockedRunRepository) Update(ctx context.Context, item *runmodel.Run) error {
	if item != nil && (item.State == common.RunStateBlockedOnDecision || item.State == common.RunStateBlockedOnApproval) {
		if r.started != nil {
			r.started <- "run-blocked-update"
		}
		if r.release != nil {
			select {
			case <-r.release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return r.inner.Update(ctx, item)
}

func (r *blockingBlockedRunRepository) Delete(ctx context.Context, id common.RunID) error {
	return r.inner.Delete(ctx, id)
}

type countingDecisionTicketRepository struct {
	inner            *memory.DecisionTicketRepository
	listCount        atomic.Int64
	listByRunIDCount atomic.Int64
}

func newCountingDecisionTicketRepository() *countingDecisionTicketRepository {
	return &countingDecisionTicketRepository{inner: memory.NewDecisionTicketRepository()}
}

func (r *countingDecisionTicketRepository) Create(ctx context.Context, item *decisionmodel.Ticket) error {
	return r.inner.Create(ctx, item)
}

func (r *countingDecisionTicketRepository) Get(ctx context.Context, id common.TicketID) (*decisionmodel.Ticket, error) {
	return r.inner.Get(ctx, id)
}

func (r *countingDecisionTicketRepository) List(ctx context.Context) ([]*decisionmodel.Ticket, error) {
	r.listCount.Add(1)
	return r.inner.List(ctx)
}

func (r *countingDecisionTicketRepository) ListByRunID(ctx context.Context, runID common.RunID) ([]*decisionmodel.Ticket, error) {
	r.listByRunIDCount.Add(1)
	return r.inner.ListByRunID(ctx, runID)
}

func (r *countingDecisionTicketRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*decisionmodel.Ticket, error) {
	return r.inner.ListByWorkspaceID(ctx, workspaceID)
}

func (r *countingDecisionTicketRepository) Update(ctx context.Context, item *decisionmodel.Ticket) error {
	return r.inner.Update(ctx, item)
}

func (r *countingDecisionTicketRepository) Delete(ctx context.Context, id common.TicketID) error {
	return r.inner.Delete(ctx, id)
}

type countingWorkspaceRepository struct {
	inner    *memory.WorkspaceRepository
	getCount atomic.Int64
}

func newCountingWorkspaceRepository() *countingWorkspaceRepository {
	return &countingWorkspaceRepository{inner: memory.NewWorkspaceRepository()}
}

func (r *countingWorkspaceRepository) Create(ctx context.Context, item *workspacemodel.Workspace) error {
	return r.inner.Create(ctx, item)
}

func (r *countingWorkspaceRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	r.getCount.Add(1)
	return r.inner.Get(ctx, id)
}

func (r *countingWorkspaceRepository) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	return r.inner.List(ctx)
}

func (r *countingWorkspaceRepository) Update(ctx context.Context, item *workspacemodel.Workspace) error {
	return r.inner.Update(ctx, item)
}

func (r *countingWorkspaceRepository) Delete(ctx context.Context, id common.WorkspaceID) error {
	return r.inner.Delete(ctx, id)
}

type blockingWorkspaceRepository struct {
	inner         *memory.WorkspaceRepository
	started       chan<- string
	release       <-chan struct{}
	updateRelease <-chan struct{}
}

func (r *blockingWorkspaceRepository) Create(ctx context.Context, item *workspacemodel.Workspace) error {
	return r.inner.Create(ctx, item)
}

func (r *blockingWorkspaceRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if r.started != nil {
		r.started <- "workspace-get"
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return r.inner.Get(ctx, id)
}

func (r *blockingWorkspaceRepository) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	return r.inner.List(ctx)
}

func (r *blockingWorkspaceRepository) Update(ctx context.Context, item *workspacemodel.Workspace) error {
	if item != nil && item.State == common.WorkspaceStateCompleted && item.ActiveRunID == "" {
		if r.started != nil {
			r.started <- "workspace-completed-update"
		}
		if r.updateRelease != nil {
			select {
			case <-r.updateRelease:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return r.inner.Update(ctx, item)
}

func (r *blockingWorkspaceRepository) Delete(ctx context.Context, id common.WorkspaceID) error {
	return r.inner.Delete(ctx, id)
}

type blockingValidatingWorkspaceRepository struct {
	inner   *memory.WorkspaceRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingValidatingWorkspaceRepository) Create(ctx context.Context, item *workspacemodel.Workspace) error {
	return r.inner.Create(ctx, item)
}

func (r *blockingValidatingWorkspaceRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	return r.inner.Get(ctx, id)
}

func (r *blockingValidatingWorkspaceRepository) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	return r.inner.List(ctx)
}

func (r *blockingValidatingWorkspaceRepository) Update(ctx context.Context, item *workspacemodel.Workspace) error {
	if item != nil && item.State == common.WorkspaceStateValidating {
		if r.started != nil {
			r.started <- "workspace-validating-update"
		}
		if r.release != nil {
			select {
			case <-r.release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return r.inner.Update(ctx, item)
}

func (r *blockingValidatingWorkspaceRepository) Delete(ctx context.Context, id common.WorkspaceID) error {
	return r.inner.Delete(ctx, id)
}

type blockingRunningWorkspaceRepository struct {
	inner   *memory.WorkspaceRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingRunningWorkspaceRepository) Create(ctx context.Context, item *workspacemodel.Workspace) error {
	return r.inner.Create(ctx, item)
}

func (r *blockingRunningWorkspaceRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	return r.inner.Get(ctx, id)
}

func (r *blockingRunningWorkspaceRepository) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	return r.inner.List(ctx)
}

func (r *blockingRunningWorkspaceRepository) Update(ctx context.Context, item *workspacemodel.Workspace) error {
	if item != nil && item.State == common.WorkspaceStateRunning {
		if r.started != nil {
			r.started <- "workspace-running-update"
		}
		if r.release != nil {
			select {
			case <-r.release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return r.inner.Update(ctx, item)
}

func (r *blockingRunningWorkspaceRepository) Delete(ctx context.Context, id common.WorkspaceID) error {
	return r.inner.Delete(ctx, id)
}

type blockingBlockedWorkspaceRepository struct {
	inner   *memory.WorkspaceRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingBlockedWorkspaceRepository) Create(ctx context.Context, item *workspacemodel.Workspace) error {
	return r.inner.Create(ctx, item)
}

func (r *blockingBlockedWorkspaceRepository) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	return r.inner.Get(ctx, id)
}

func (r *blockingBlockedWorkspaceRepository) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	return r.inner.List(ctx)
}

func (r *blockingBlockedWorkspaceRepository) Update(ctx context.Context, item *workspacemodel.Workspace) error {
	if item != nil && (item.State == common.WorkspaceStateWaitingDecision || item.State == common.WorkspaceStateBlocked) {
		if r.started != nil {
			r.started <- "workspace-blocked-update"
		}
		if r.release != nil {
			select {
			case <-r.release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return r.inner.Update(ctx, item)
}

func (r *blockingBlockedWorkspaceRepository) Delete(ctx context.Context, id common.WorkspaceID) error {
	return r.inner.Delete(ctx, id)
}

type blockingResultRepository struct {
	inner   *memory.ResultRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingResultRepository) Create(ctx context.Context, item *resultmodel.Result) error {
	return r.inner.Create(ctx, item)
}

func (r *blockingResultRepository) Get(ctx context.Context, id common.ResultID) (*resultmodel.Result, error) {
	return r.inner.Get(ctx, id)
}

func (r *blockingResultRepository) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	r.started <- "results"
	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return r.inner.ListByWorkspace(ctx, workspaceID)
}

func (r *blockingResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	return r.inner.ListByRun(ctx, runID)
}

func (r *blockingResultRepository) Update(ctx context.Context, item *resultmodel.Result) error {
	return r.inner.Update(ctx, item)
}

func (r *blockingResultRepository) Delete(ctx context.Context, id common.ResultID) error {
	return r.inner.Delete(ctx, id)
}

type countingResultRepository struct {
	inner                  *memory.ResultRepository
	listByWorkspaceCount   atomic.Int64
	listByWorkspaceStarted chan<- string
}

func (r *countingResultRepository) Create(ctx context.Context, item *resultmodel.Result) error {
	return r.inner.Create(ctx, item)
}

func (r *countingResultRepository) Get(ctx context.Context, id common.ResultID) (*resultmodel.Result, error) {
	return r.inner.Get(ctx, id)
}

func (r *countingResultRepository) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Result, error) {
	r.listByWorkspaceCount.Add(1)
	if r.listByWorkspaceStarted != nil {
		r.listByWorkspaceStarted <- "results"
	}
	return r.inner.ListByWorkspace(ctx, workspaceID)
}

func (r *countingResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Result, error) {
	return r.inner.ListByRun(ctx, runID)
}

func (r *countingResultRepository) Update(ctx context.Context, item *resultmodel.Result) error {
	return r.inner.Update(ctx, item)
}

func (r *countingResultRepository) Delete(ctx context.Context, id common.ResultID) error {
	return r.inner.Delete(ctx, id)
}

type blockingArtifactRepository struct {
	inner   *memory.ArtifactRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingArtifactRepository) Create(ctx context.Context, item *resultmodel.Artifact) error {
	if r.started != nil {
		r.started <- "artifact-create:" + string(item.ID)
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return r.inner.Create(ctx, item)
}

func (r *blockingArtifactRepository) Get(ctx context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	return r.inner.Get(ctx, id)
}

func (r *blockingArtifactRepository) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	if r.started != nil {
		r.started <- "artifacts"
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return r.inner.ListByWorkspace(ctx, workspaceID)
}

func (r *blockingArtifactRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	return r.inner.ListByRun(ctx, runID)
}

func (r *blockingArtifactRepository) ListByResult(ctx context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	return r.inner.ListByResult(ctx, resultID)
}

func (r *blockingArtifactRepository) Update(ctx context.Context, item *resultmodel.Artifact) error {
	return r.inner.Update(ctx, item)
}

func (r *blockingArtifactRepository) Delete(ctx context.Context, id common.ArtifactID) error {
	return r.inner.Delete(ctx, id)
}

type countingArtifactRepository struct {
	inner                  *memory.ArtifactRepository
	listByWorkspaceCount   atomic.Int64
	listByWorkspaceStarted chan<- string
}

func (r *countingArtifactRepository) Create(ctx context.Context, item *resultmodel.Artifact) error {
	return r.inner.Create(ctx, item)
}

func (r *countingArtifactRepository) Get(ctx context.Context, id common.ArtifactID) (*resultmodel.Artifact, error) {
	return r.inner.Get(ctx, id)
}

func (r *countingArtifactRepository) ListByWorkspace(ctx context.Context, workspaceID common.WorkspaceID) ([]*resultmodel.Artifact, error) {
	r.listByWorkspaceCount.Add(1)
	if r.listByWorkspaceStarted != nil {
		r.listByWorkspaceStarted <- "artifacts"
	}
	return r.inner.ListByWorkspace(ctx, workspaceID)
}

func (r *countingArtifactRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*resultmodel.Artifact, error) {
	return r.inner.ListByRun(ctx, runID)
}

func (r *countingArtifactRepository) ListByResult(ctx context.Context, resultID common.ResultID) ([]*resultmodel.Artifact, error) {
	return r.inner.ListByResult(ctx, resultID)
}

func (r *countingArtifactRepository) Update(ctx context.Context, item *resultmodel.Artifact) error {
	return r.inner.Update(ctx, item)
}

func (r *countingArtifactRepository) Delete(ctx context.Context, id common.ArtifactID) error {
	return r.inner.Delete(ctx, id)
}

type blockingMemoryItemRepository struct {
	inner   *memory.MemoryItemRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingMemoryItemRepository) Create(ctx context.Context, item *memorymodel.MemoryItem) error {
	if r.started != nil {
		r.started <- "memory-create:" + string(item.ID)
	}
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return r.inner.Create(ctx, item)
}

func (r *blockingMemoryItemRepository) Get(ctx context.Context, id common.MemoryID) (*memorymodel.MemoryItem, error) {
	return r.inner.Get(ctx, id)
}

func (r *blockingMemoryItemRepository) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*memorymodel.MemoryItem, error) {
	return r.inner.ListByWorkspaceID(ctx, workspaceID)
}

func (r *blockingMemoryItemRepository) Search(ctx context.Context, query repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	return r.inner.Search(ctx, query)
}

func (r *blockingMemoryItemRepository) Update(ctx context.Context, item *memorymodel.MemoryItem) error {
	return r.inner.Update(ctx, item)
}

func (r *blockingMemoryItemRepository) Delete(ctx context.Context, id common.MemoryID) error {
	return r.inner.Delete(ctx, id)
}

type blockingToolCallRepository struct {
	inner              *memory.ToolCallRepository
	started            chan<- string
	release            <-chan struct{}
	blockRunningCreate bool
}

func (r *blockingToolCallRepository) Create(ctx context.Context, item *runmodel.ToolCall) error {
	if r.blockRunningCreate && item != nil && item.State == common.ToolCallStateRunning {
		if r.started != nil {
			r.started <- "tool-call-started-create"
		}
		if r.release != nil {
			select {
			case <-r.release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return r.inner.Create(ctx, item)
}

func (r *blockingToolCallRepository) Get(ctx context.Context, id common.ToolCallID) (*runmodel.ToolCall, error) {
	return r.inner.Get(ctx, id)
}

func (r *blockingToolCallRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.ToolCall, error) {
	return r.inner.ListByRun(ctx, runID)
}

func (r *blockingToolCallRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*runmodel.ToolCall, error) {
	return r.inner.ListByStep(ctx, stepID)
}

func (r *blockingToolCallRepository) Update(ctx context.Context, item *runmodel.ToolCall) error {
	if item != nil && item.State == common.ToolCallStateCompleted {
		if r.started != nil {
			r.started <- "tool-call-completed-update"
		}
		if r.release != nil {
			select {
			case <-r.release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return r.inner.Update(ctx, item)
}

func (r *blockingToolCallRepository) Delete(ctx context.Context, id common.ToolCallID) error {
	return r.inner.Delete(ctx, id)
}

type blockingValidationResultRepository struct {
	inner   *memory.ValidationResultRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingValidationResultRepository) Create(ctx context.Context, item *runmodel.ValidationResult) error {
	if item != nil && item.State == common.ValidationResultStatePassed {
		if r.started != nil {
			r.started <- "validation-passed-create"
		}
		if r.release != nil {
			select {
			case <-r.release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return r.inner.Create(ctx, item)
}

func (r *blockingValidationResultRepository) Get(ctx context.Context, id common.ValidationResultID) (*runmodel.ValidationResult, error) {
	return r.inner.Get(ctx, id)
}

func (r *blockingValidationResultRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.ValidationResult, error) {
	return r.inner.ListByRun(ctx, runID)
}

func (r *blockingValidationResultRepository) ListByStep(ctx context.Context, stepID common.StepID) ([]*runmodel.ValidationResult, error) {
	return r.inner.ListByStep(ctx, stepID)
}

func (r *blockingValidationResultRepository) Update(ctx context.Context, item *runmodel.ValidationResult) error {
	return r.inner.Update(ctx, item)
}

func (r *blockingValidationResultRepository) Delete(ctx context.Context, id common.ValidationResultID) error {
	return r.inner.Delete(ctx, id)
}

type blockingRuntimeCheckpointRepository struct {
	inner   *memory.RuntimeCheckpointRepository
	started chan<- string
	release <-chan struct{}
}

func (r *blockingRuntimeCheckpointRepository) Create(ctx context.Context, item *runmodel.RuntimeCheckpoint) error {
	if item != nil && item.Data != nil {
		eventName, _ := item.Data["event"].(string)
		startedName := ""
		switch eventName {
		case "step_dispatched":
			startedName = "step-dispatched-checkpoint-create"
		case "step_completed":
			startedName = "step_completed-checkpoint-create"
		}
		if startedName != "" {
			if r.started != nil {
				r.started <- startedName
			}
			if r.release != nil {
				select {
				case <-r.release:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
	return r.inner.Create(ctx, item)
}

func (r *blockingRuntimeCheckpointRepository) Get(ctx context.Context, id common.CheckpointID) (*runmodel.RuntimeCheckpoint, error) {
	return r.inner.Get(ctx, id)
}

func (r *blockingRuntimeCheckpointRepository) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.RuntimeCheckpoint, error) {
	return r.inner.ListByRun(ctx, runID)
}

func (r *blockingRuntimeCheckpointRepository) Update(ctx context.Context, item *runmodel.RuntimeCheckpoint) error {
	return r.inner.Update(ctx, item)
}

func (r *blockingRuntimeCheckpointRepository) Delete(ctx context.Context, id common.CheckpointID) error {
	return r.inner.Delete(ctx, id)
}

func newRuntimeAdapterRegistryForTest(defaultAdapter RuntimeAdapter) *RuntimeAdapterRegistry {
	registry := NewRuntimeAdapterRegistry()
	if defaultAdapter != nil {
		if err := registry.Register(defaultInitialStepKind, defaultAdapter); err != nil {
			panic(err)
		}
	}
	return registry
}
