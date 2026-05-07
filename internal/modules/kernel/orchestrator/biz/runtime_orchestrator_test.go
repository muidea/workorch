package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	mcpblockbiz "github.com/muidea/workorch/internal/modules/blocks/mcp/biz"
	mcpblockcommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	mcpbiz "github.com/muidea/workorch/internal/modules/kernel/mcp/biz"
	mcpservice "github.com/muidea/workorch/internal/modules/kernel/mcp/service"
	memorycommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
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
}

func (a *requestCapturingRuntimeAdapter) ExecuteRuntimeStep(_ context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	a.requests = append(a.requests, request)
	return runtimemodel.StepExecutionResult{Output: a.output}, nil
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

func TestRuntimeOrchestrator_StartRunToComplete_Mainline(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateCompleted {
		t.Fatalf("run state = %q, want completed", r.State)
	}

	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.ActiveRunID != "" {
		t.Fatalf("workspace active run id = %q, want empty", ws.ActiveRunID)
	}
	if ws.State != common.WorkspaceStateCompleted {
		t.Fatalf("workspace state = %q, want completed", ws.State)
	}

	stepID := common.StepID(string(runID) + "/step-1")
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateCompleted {
		t.Fatalf("step state = %q, want completed", step.State)
	}
	if step.Output["status"] != "ok" {
		t.Fatalf("step output status = %v, want ok", step.Output["status"])
	}
	if step.Input[stepInputKeySoulStage] != soulStagePlanning {
		t.Fatalf("step soulStage = %v, want planning", step.Input[stepInputKeySoulStage])
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != stepID {
		t.Fatalf("runtime calls = %#v, want [%q]", runtime.calls, stepID)
	}
}

func TestRuntimeOrchestrator_StepDispatchEvidenceWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	toolCallRepo := &blockingToolCallRepository{
		inner:              memory.NewToolCallRepository(),
		started:            started,
		release:            release,
		blockRunningCreate: true,
	}
	checkpointRepo := &blockingRuntimeCheckpointRepository{
		inner:   memory.NewRuntimeCheckpointRepository(),
		started: started,
		release: release,
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
	}
	run := &runmodel.Run{ID: "run-dispatch-evidence-parallel", WorkspaceID: "ws-dispatch-evidence-parallel", State: common.RunStateRunning}
	step := &runmodel.Step{
		ID:    "run-dispatch-evidence-parallel/step-1",
		RunID: "run-dispatch-evidence-parallel",
		Kind:  "llm/workspace_execution",
		State: common.StepStateDispatched,
		Input: map[string]any{"seed": "yes"},
	}

	done := make(chan error, 1)
	go func() {
		_, err := orch.recordStepDispatchEvidence(ctx, run, step)
		done <- err
	}()

	seen := map[string]bool{}
	for !seen["tool-call-started-create"] || !seen["step-dispatched-checkpoint-create"] {
		select {
		case name := <-started:
			seen[name] = true
		case <-ctx.Done():
			t.Fatalf("dispatch evidence writes before release = %+v, want tool call and checkpoint writes started: %v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("record dispatch evidence: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("record dispatch evidence did not finish after release: %v", ctx.Err())
	}
	toolCalls, err := toolCallRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list tool calls: %v", err)
	}
	if len(toolCalls) != 1 || toolCalls[0].State != common.ToolCallStateRunning {
		t.Fatalf("tool calls = %+v, want one running tool call", toolCalls)
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(checkpoints) != 1 || checkpoints[0].CurrentStepID != step.ID || checkpoints[0].Data["event"] != "step_dispatched" {
		t.Fatalf("checkpoints = %+v, want one step_dispatched checkpoint", checkpoints)
	}
}

func TestRuntimeAdapterRegistryDispatchesByStepKind(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	defaultRuntime := &requestCapturingRuntimeAdapter{output: map[string]any{"status": "default"}}
	externalRuntime := &requestCapturingRuntimeAdapter{output: map[string]any{"status": "external"}}
	registry := newRuntimeAdapterRegistryForTest(defaultRuntime)
	if err := registry.Register("external/mock", externalRuntime); err != nil {
		t.Fatalf("register external runtime: %v", err)
	}
	orch := NewRuntimeOrchestrator(registry, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-runtime-registry")
	runID := common.RunID("run-runtime-registry")
	stepID := common.StepID("run-runtime-registry/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "external/mock", State: common.StepStateDispatched, Input: map[string]any{"seed": "yes"}}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	if err := orch.DispatchNextStep(ctx, runID); err != nil {
		t.Fatalf("dispatch next step: %v", err)
	}

	if len(externalRuntime.requests) != 1 {
		t.Fatalf("external runtime requests = %d, want 1", len(externalRuntime.requests))
	}
	request := externalRuntime.requests[0]
	if request.WorkspaceID != workspaceID || request.RunID != runID || request.StepID != stepID || request.Kind != "external/mock" || request.Input["seed"] != "yes" {
		t.Fatalf("request = %+v, want full runtime request", request)
	}
	if len(defaultRuntime.requests) != 0 {
		t.Fatalf("default runtime requests = %d, want 0", len(defaultRuntime.requests))
	}
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.Output["status"] != "external" {
		t.Fatalf("step output = %+v, want external", step.Output)
	}
}

func TestRuntimeAdapterRegistryDoesNotFallbackForUnknownRuntimeKind(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	defaultRuntime := &requestCapturingRuntimeAdapter{output: map[string]any{"status": "default"}}
	registry := newRuntimeAdapterRegistryForTest(defaultRuntime)
	orch := NewRuntimeOrchestrator(registry, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-runtime-unknown")
	runID := common.RunID("run-runtime-unknown")
	stepID := common.StepID("run-runtime-unknown/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: "mcp/tool_call", State: common.StepStateDispatched}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	if err := orch.DispatchNextStep(ctx, runID); err != nil {
		t.Fatalf("dispatch unknown runtime should close as failed without returning error: %v", err)
	}

	if len(defaultRuntime.requests) != 0 {
		t.Fatalf("default runtime requests = %d, want 0", len(defaultRuntime.requests))
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed ||
		step.Output["stage"] != "runtime_adapter_unavailable" ||
		step.Output["kind"] != "mcp/tool_call" {
		t.Fatalf("step = %+v, want adapter unavailable failure", step)
	}
}

func TestRuntimeOrchestrator_DispatchNextStepRunsShellCommandAdapter(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	registry := newRuntimeAdapterRegistryForTest(&stubRuntimeAdapter{output: map[string]any{"status": "default"}})
	if err := registry.Register(shellcommon.StepKindCommand, shellbiz.NewRuntimeAdapter()); err != nil {
		t.Fatalf("register shell runtime: %v", err)
	}
	orch := NewRuntimeOrchestrator(registry, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workingDirectory := t.TempDir()
	workspaceID := common.WorkspaceID("ws-shell")
	runID := common.RunID("run-shell")
	stepID := common.StepID("run-shell/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateDispatched,
		Input: map[string]any{"command": "pwd"},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	if err := orch.DispatchNextStep(ctx, runID); err != nil {
		t.Fatalf("dispatch shell step: %v", err)
	}

	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.Output["exitCode"] != 0 {
		t.Fatalf("exitCode = %v, want 0", step.Output["exitCode"])
	}
	stdout, _ := step.Output["stdout"].(string)
	if strings.TrimSpace(stdout) != workingDirectory {
		t.Fatalf("stdout = %q, want %q", stdout, workingDirectory)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
}

func TestRuntimeOrchestrator_DispatchNextStepRunsMCPHTTPAdapter(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	toolCallRepo := memory.NewToolCallRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()

	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode mcp request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"summary": "mcp result"})
	}))
	defer server.Close()

	mcpService := mcpbiz.NewService(memory.NewMCPServerRepository(), memory.NewMCPCapabilityRepository())
	if err := mcpService.CreateServer(ctx, mcpservice.CreateServerCommand{
		ServerID:      common.MCPServerID("mcp-http"),
		Name:          "http tool",
		Transport:     "http",
		Endpoint:      server.URL,
		Enabled:       true,
		AllowedTools:  []string{"repo.summary"},
		TimeoutMillis: 1000,
	}); err != nil {
		t.Fatalf("create mcp server: %v", err)
	}

	registry := newRuntimeAdapterRegistryForTest(&stubRuntimeAdapter{output: map[string]any{"status": "default"}})
	if err := registry.Register(mcpblockcommon.StepKindToolCall, mcpblockbiz.NewRuntimeAdapter(mcpService)); err != nil {
		t.Fatalf("register mcp runtime: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		registry,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-mcp")
	runID := common.RunID("run-mcp")
	stepID := common.StepID("run-mcp/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  mcpblockcommon.StepKindToolCall,
		State: common.StepStateDispatched,
		Input: map[string]any{
			"serverId": "mcp-http",
			"tool":     "repo.summary",
			"input":    map[string]any{"path": "."},
		},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	if err := orch.DispatchNextStep(ctx, runID); err != nil {
		t.Fatalf("dispatch mcp step: %v", err)
	}

	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateCompleted || step.Output["stage"] != "mcp_tool_call_completed" {
		t.Fatalf("step = %+v, want completed mcp step", step)
	}
	if captured["workspaceId"] != "ws-mcp" || captured["tool"] != "repo.summary" {
		t.Fatalf("captured = %+v, want workorch mcp payload", captured)
	}
	toolCall, err := toolCallRepo.Get(ctx, common.ToolCallID("run-mcp/step-1/tool-call/runtime-execute"))
	if err != nil {
		t.Fatalf("get tool call: %v", err)
	}
	if toolCall.State != common.ToolCallStateCompleted || toolCall.ToolName != mcpblockcommon.StepKindToolCall {
		t.Fatalf("toolCall = %+v, want completed mcp evidence", toolCall)
	}
}

func TestRuntimeOrchestrator_DispatchNextStepPreservesShellFailureEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	toolCallRepo := memory.NewToolCallRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	registry := newRuntimeAdapterRegistryForTest(&stubRuntimeAdapter{output: map[string]any{"status": "default"}})
	if err := registry.Register(shellcommon.StepKindCommand, shellbiz.NewRuntimeAdapter()); err != nil {
		t.Fatalf("register shell runtime: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		registry,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-shell-failed")
	runID := common.RunID("run-shell-failed")
	stepID := common.StepID("run-shell-failed/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateDispatched,
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "echo shell-out; echo shell-err >&2; exit 7"},
		},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	if err := orch.DispatchNextStep(ctx, runID); err != nil {
		t.Fatalf("dispatch shell step should close as failed without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed || step.Output["status"] != "failed" || step.Output["exitCode"] != 7 || step.Output["reason"] == "" {
		t.Fatalf("step = %+v, want failed shell output with exit evidence", step)
	}
	stderr, _ := step.Output["stderr"].(string)
	if !strings.Contains(stderr, "shell-err") {
		t.Fatalf("step stderr = %q, want shell-err", stderr)
	}
	toolCalls, err := toolCallRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list tool calls: %v", err)
	}
	if len(toolCalls) != 1 || toolCalls[0].State != common.ToolCallStateFailed || toolCalls[0].Output["exitCode"] != 7 {
		t.Fatalf("tool calls = %+v, want failed tool call with shell output", toolCalls)
	}
	validations, err := validationRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list validations: %v", err)
	}
	if len(validations) != 1 || validations[0].State != common.ValidationResultStateFailed || validations[0].EvidenceRef != string(toolCalls[0].ID) {
		t.Fatalf("validations = %+v, want failed validation linked to tool call", validations)
	}
}

func TestRuntimeOrchestrator_DispatchNextStepPreservesShellTimeoutEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	toolCallRepo := memory.NewToolCallRepository()
	registry := newRuntimeAdapterRegistryForTest(&stubRuntimeAdapter{output: map[string]any{"status": "default"}})
	if err := registry.Register(shellcommon.StepKindCommand, shellbiz.NewRuntimeAdapterWithTimeout(10*time.Millisecond)); err != nil {
		t.Fatalf("register shell runtime: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		registry,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-shell-timeout")
	runID := common.RunID("run-shell-timeout")
	stepID := common.StepID("run-shell-timeout/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateDispatched,
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "sleep 1"},
		},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	if err := orch.DispatchNextStep(ctx, runID); err != nil {
		t.Fatalf("dispatch shell timeout should close as failed without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed || step.Output["status"] != "timeout" || step.Output["reason"] == "" {
		t.Fatalf("step = %+v, want timeout output", step)
	}
	toolCalls, err := toolCallRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list tool calls: %v", err)
	}
	if len(toolCalls) != 1 || toolCalls[0].State != common.ToolCallStateFailed || toolCalls[0].Output["status"] != "timeout" {
		t.Fatalf("tool calls = %+v, want failed timeout evidence", toolCalls)
	}
}

func TestRuntimeOrchestrator_StartRunRecordsExecutionEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	toolCallRepo := memory.NewToolCallRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-evidence")
	runID := common.RunID("run-evidence")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	toolCalls, err := toolCallRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list tool calls: %v", err)
	}
	if len(toolCalls) != 1 || toolCalls[0].State != common.ToolCallStateCompleted || toolCalls[0].Output["status"] != "ok" {
		t.Fatalf("tool calls = %+v, want one completed call with output", toolCalls)
	}
	validations, err := validationRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list validations: %v", err)
	}
	if len(validations) != 1 || validations[0].State != common.ValidationResultStatePassed || validations[0].EvidenceRef != string(toolCalls[0].ID) {
		t.Fatalf("validations = %+v, want one passed validation linked to tool call", validations)
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if !hasRunCheckpointState(checkpoints, common.RunStateRunning) ||
		!hasRunCheckpointState(checkpoints, common.RunStateValidating) ||
		!hasRunCheckpointState(checkpoints, common.RunStateCompleted) ||
		!hasCheckpointEvent(checkpoints, "step_dispatched") ||
		!hasCheckpointEvent(checkpoints, "step_completed") {
		t.Fatalf("checkpoints = %+v, want run and step execution checkpoints", checkpoints)
	}
}

func TestRuntimeOrchestrator_StartRunRecordsFailedExecutionEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	toolCallRepo := memory.NewToolCallRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	runtime := &stubRuntimeAdapter{err: errors.New("runtime failed")}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-evidence-failed")
	runID := common.RunID("run-evidence-failed")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run should close as failed without returning error: %v", err)
	}

	toolCalls, err := toolCallRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list tool calls: %v", err)
	}
	if len(toolCalls) != 1 || toolCalls[0].State != common.ToolCallStateFailed || toolCalls[0].Error != "runtime failed" {
		t.Fatalf("tool calls = %+v, want one failed call", toolCalls)
	}
	validations, err := validationRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list validations: %v", err)
	}
	if len(validations) != 1 || validations[0].State != common.ValidationResultStateFailed {
		t.Fatalf("validations = %+v, want one failed validation", validations)
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if !hasRunCheckpointState(checkpoints, common.RunStateFailed) || !hasCheckpointEvent(checkpoints, "step_failed") {
		t.Fatalf("checkpoints = %+v, want failed run and step checkpoints", checkpoints)
	}
}

func TestRuntimeOrchestrator_StartRunAppliesRuntimePatchEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	toolCallRepo := memory.NewToolCallRepository()
	patchRepo := memory.NewPatchRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "ok",
		"patches": []map[string]any{
			{
				"targetRef": "docs/result.txt",
				"content":   "runtime patch applied\n",
				"summary":   "write runtime result",
			},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			Patches:            patchRepo,
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	workingDirectory := t.TempDir()
	workspaceID := common.WorkspaceID("ws-patch")
	runID := common.RunID("run-patch")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(workingDirectory, "docs", "result.txt"))
	if err != nil {
		t.Fatalf("read applied patch file: %v", err)
	}
	if string(content) != "runtime patch applied\n" {
		t.Fatalf("applied file content = %q, want runtime patch applied", string(content))
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(patches) != 1 || patches[0].State != common.PatchStateApplied || patches[0].TargetRef != "docs/result.txt" || patches[0].ToolCallID == "" {
		t.Fatalf("patches = %+v, want one applied patch linked to tool call", patches)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchesPatchApplyGraphNode(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	patchRepo := memory.NewPatchRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	toolCallRepo := memory.NewToolCallRepository()
	workingDirectory := t.TempDir()

	llmRuntime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		switch request.StepID {
		case "run-patch-graph/step-1":
			return map[string]any{
				"status": "completed",
				"stage":  "llm_execution_graph_plan",
				"executionGraph": map[string]any{
					"nodes": []map[string]any{
						{
							"id":   "write-result",
							"kind": llmcommon.RuntimeBlockKindPatchApply,
							"input": map[string]any{
								"patchType": "file_write",
								"targetRef": "docs/result.txt",
								"content":   "graph patch applied\n",
								"summary":   "write graph result",
							},
						},
						{
							"id":        "verify-result",
							"kind":      shellcommon.StepKindCommand,
							"dependsOn": []string{"write-result"},
							"input": map[string]any{
								"command": "test",
								"args":    []string{"-f", "docs/result.txt"},
							},
						},
					},
				},
			}, nil
		case "run-patch-graph/zz-llm-after-graph-run-patch-graph-step-1-verify-result":
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "patch graph completed",
					"items": []map[string]any{
						{
							"item":         "result written",
							"state":        "satisfied",
							"evidenceRefs": []string{"run-patch-graph/block/write-result/patch/1"},
						},
					},
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected llm step %s", request.StepID)
		}
	}}
	shellRuntime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if _, err := os.Stat(filepath.Join(request.WorkspaceWorkingDirectory, "docs", "result.txt")); err != nil {
			return map[string]any{"status": "failed", "error": err.Error()}, err
		}
		return map[string]any{"status": "completed", "stage": "shell_command_completed"}, nil
	}}
	registry := NewRuntimeAdapterRegistry()
	if err := registry.Register(llmcommon.StepKindWorkspaceExecution, llmRuntime); err != nil {
		t.Fatalf("register llm runtime: %v", err)
	}
	if err := registry.Register(llmcommon.RuntimeBlockKindPatchApply, NewPatchRuntimeAdapter()); err != nil {
		t.Fatalf("register patch runtime: %v", err)
	}
	if err := registry.Register(shellcommon.StepKindCommand, shellRuntime); err != nil {
		t.Fatalf("register shell runtime: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		registry,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			Patches:            patchRepo,
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
			MemoryItems:        memory.NewMemoryItemRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-patch-graph")
	runID := common.RunID("run-patch-graph")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-patch-graph"),
		State:           common.RunStateCreated,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(workingDirectory, "docs", "result.txt"))
	if err != nil {
		t.Fatalf("read graph patch file: %v", err)
	}
	if string(content) != "graph patch applied\n" {
		t.Fatalf("graph patch file = %q, want applied content", string(content))
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(patches) != 1 || patches[0].State != common.PatchStateApplied || patches[0].TargetRef != "docs/result.txt" {
		t.Fatalf("patches = %+v, want applied graph patch", patches)
	}
}

func TestApplyWorkspaceFileWriteReportsPathConflict(t *testing.T) {
	workingDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDirectory, "llm-proxy"), []byte("binary"), 0o755); err != nil {
		t.Fatalf("seed executable file: %v", err)
	}

	err := applyWorkspaceFileWrite(workingDirectory, "llm-proxy/config.yaml", "config", "")
	if err == nil {
		t.Fatalf("applyWorkspaceFileWrite should fail for file-as-directory path conflict")
	}
	var conflict *workspacePathConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("err = %T %[1]v, want workspacePathConflictError", err)
	}
	if conflict.TargetRef != "llm-proxy/config.yaml" || conflict.ConflictRef != "llm-proxy" || conflict.ConflictKind != "file" {
		t.Fatalf("conflict = %+v, want target and conflict refs", conflict)
	}
	diagnostics := conflict.RuntimeFailureDiagnostics()
	if diagnostics["kind"] != "workspace_path_conflict" || diagnostics["conflictRef"] != "llm-proxy" {
		t.Fatalf("diagnostics = %+v, want structured path conflict diagnostics", diagnostics)
	}
}

func TestApplyWorkspacePatchRejectsBaseHashMismatch(t *testing.T) {
	workingDirectory := t.TempDir()
	target := filepath.Join(workingDirectory, "main.go")
	if err := os.WriteFile(target, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	err := applyWorkspacePatch(workingDirectory, runtimePatchRequest{
		TargetRef: "main.go",
		Content:   "package changed\n",
		PatchType: "file_write",
		BaseHash:  "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if err == nil || !strings.Contains(err.Error(), "baseHash mismatch") {
		t.Fatalf("applyWorkspacePatch err = %v, want baseHash mismatch", err)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read target: %v", readErr)
	}
	if string(content) != "package main\n" {
		t.Fatalf("content = %q, want unchanged", content)
	}
}

func TestPatchPreviewDoesNotWriteFile(t *testing.T) {
	workingDirectory := t.TempDir()
	result, err := NewPatchRuntimeAdapter().ExecuteRuntimeStep(context.Background(), runtimemodel.StepExecutionRequest{
		WorkspaceWorkingDirectory: workingDirectory,
		Kind:                      llmcommon.RuntimeBlockKindPatchPreview,
		Input: map[string]any{
			"patchType": "file_write",
			"targetRef": "preview.txt",
			"content":   "preview only\n",
		},
	})
	if err != nil {
		t.Fatalf("patch preview: %v", err)
	}
	if result.Output["stage"] != "patch_preview" || result.Output["patches"] != nil {
		t.Fatalf("patch preview output = %+v, want preview without patches side effect", result.Output)
	}
	if _, err := os.Stat(filepath.Join(workingDirectory, "preview.txt")); !os.IsNotExist(err) {
		t.Fatalf("preview wrote target file, stat err = %v", err)
	}
}

func TestRuntimeOrchestratorPublishesPatchPreviewEvent(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		memory.NewStepRepository(),
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Patches: memory.NewPatchRepository()},
		hub,
	)
	run := &runmodel.Run{ID: "run-patch-preview-event", WorkspaceID: "ws-patch-preview-event"}
	step := &runmodel.Step{ID: "run-patch-preview-event/step-1", RunID: run.ID}
	toolCall := &runmodel.ToolCall{ID: "run-patch-preview-event/step-1/tool-call"}

	err := orch.publishPatchPreviewFromOutput(ctx, run, step, toolCall, map[string]any{
		"patchPreview": map[string]any{
			"patchType":   "file_write",
			"targetRef":   "preview.txt",
			"summary":     "preview write",
			"addedLines":  1,
			"contentHash": "content-sha",
		},
	})
	if err != nil {
		t.Fatalf("publish patch preview: %v", err)
	}
	published := hub.Published()
	if len(published) != 1 || published[0].Topic != events.TopicPatchPreviewed || published[0].Payload[events.PayloadKeyTargetRef] != "preview.txt" {
		t.Fatalf("published = %+v, want patch preview event", published)
	}
}

func TestRuntimeOrchestratorApplyRuntimePatchesPublishesAppliedAndFailedEvents(t *testing.T) {
	ctx := context.Background()
	workingDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDirectory, "main.go"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("seed main.go: %v", err)
	}
	hub := events.NewMemoryHub()
	patchRepo := memory.NewPatchRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		memory.NewStepRepository(),
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Patches: patchRepo},
		hub,
	)
	run := &runmodel.Run{ID: "run-patch-event", WorkspaceID: "ws-patch-event"}
	step := &runmodel.Step{ID: "run-patch-event/step-1", RunID: run.ID}
	toolCall := &runmodel.ToolCall{ID: "run-patch-event/step-1/tool-call"}

	if err := orch.applyRuntimePatches(ctx, run, step, toolCall, map[string]any{
		stepOutputKeyPatches: []map[string]any{{
			"patchType": "file_write",
			"targetRef": "applied.txt",
			"content":   "applied\n",
			"summary":   "write applied file",
		}},
	}, workingDirectory); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if !publishedTopicForTarget(hub.Published(), events.TopicPatchApplied, "applied.txt") {
		t.Fatalf("published = %+v, want patch applied event", hub.Published())
	}

	failedStep := &runmodel.Step{ID: "run-patch-event/step-2", RunID: run.ID}
	err := orch.applyRuntimePatches(ctx, run, failedStep, toolCall, map[string]any{
		stepOutputKeyPatches: []map[string]any{{
			"patchType": "unified_diff",
			"targetRef": "main.go",
			"diff":      "--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-wrong\n+after\n",
			"summary":   "fail context",
		}},
	}, workingDirectory)
	if err == nil {
		t.Fatalf("apply invalid diff should fail")
	}
	if !publishedTopicForTarget(hub.Published(), events.TopicPatchFailed, "main.go") {
		t.Fatalf("published = %+v, want patch failed event", hub.Published())
	}
}

func TestRuntimeOrchestratorRevertsAppliedPatch(t *testing.T) {
	ctx := context.Background()
	workingDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDirectory, "main.go"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("seed main.go: %v", err)
	}
	hub := events.NewMemoryHub()
	patchRepo := memory.NewPatchRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		memory.NewStepRepository(),
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Patches: patchRepo},
		hub,
	)
	run := &runmodel.Run{ID: "run-patch-revert", WorkspaceID: "ws-patch-revert"}
	step := &runmodel.Step{ID: "run-patch-revert/step-1", RunID: run.ID}
	toolCall := &runmodel.ToolCall{ID: "run-patch-revert/step-1/tool-call"}
	if err := orch.applyRuntimePatches(ctx, run, step, toolCall, map[string]any{
		stepOutputKeyPatches: []map[string]any{{
			"patchType": "file_write",
			"targetRef": "main.go",
			"content":   "after\n",
			"summary":   "write after",
		}},
	}, workingDirectory); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	patchID := common.PatchID("run-patch-revert/step-1/patch/1")
	patch, err := patchRepo.Get(ctx, patchID)
	if err != nil {
		t.Fatalf("get patch: %v", err)
	}
	if patch.BeforeHash == "" || patch.AfterHash == "" || !patch.OriginalExists || patch.OriginalContent != "before\n" {
		t.Fatalf("patch metadata = %+v, want reversible metadata", patch)
	}
	revertStep := &runmodel.Step{ID: "run-patch-revert/step-2", RunID: run.ID}
	output := map[string]any{"patchRevert": map[string]any{"patchId": string(patchID)}}
	if err := orch.applyRuntimePatchRevert(ctx, run, revertStep, toolCall, output, workingDirectory); err != nil {
		t.Fatalf("revert patch: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(workingDirectory, "main.go"))
	if err != nil {
		t.Fatalf("read reverted file: %v", err)
	}
	if string(content) != "before\n" {
		t.Fatalf("content = %q, want reverted content", string(content))
	}
	patch, err = patchRepo.Get(ctx, patchID)
	if err != nil {
		t.Fatalf("get reverted patch: %v", err)
	}
	if patch.State != common.PatchStateReverted || patch.RevertedAt == "" {
		t.Fatalf("patch = %+v, want reverted state", patch)
	}
	if !publishedTopicForTarget(hub.Published(), events.TopicPatchReverted, "main.go") {
		t.Fatalf("published = %+v, want patch reverted event", hub.Published())
	}
}

func TestRuntimeOrchestratorRevertRejectsHashDrift(t *testing.T) {
	ctx := context.Background()
	workingDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDirectory, "main.go"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("seed main.go: %v", err)
	}
	patchRepo := memory.NewPatchRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		memory.NewStepRepository(),
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Patches: patchRepo},
		events.NewMemoryHub(),
	)
	run := &runmodel.Run{ID: "run-patch-revert-conflict", WorkspaceID: "ws-patch-revert-conflict"}
	step := &runmodel.Step{ID: "run-patch-revert-conflict/step-1", RunID: run.ID}
	toolCall := &runmodel.ToolCall{ID: "run-patch-revert-conflict/step-1/tool-call"}
	if err := orch.applyRuntimePatches(ctx, run, step, toolCall, map[string]any{
		stepOutputKeyPatches: []map[string]any{{
			"patchType": "file_write",
			"targetRef": "main.go",
			"content":   "after\n",
			"summary":   "write after",
		}},
	}, workingDirectory); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workingDirectory, "main.go"), []byte("user changed\n"), 0o644); err != nil {
		t.Fatalf("drift file: %v", err)
	}
	patchID := common.PatchID("run-patch-revert-conflict/step-1/patch/1")
	err := orch.applyRuntimePatchRevert(ctx, run, &runmodel.Step{ID: "run-patch-revert-conflict/step-2", RunID: run.ID}, toolCall, map[string]any{
		"patchRevert": map[string]any{"patchId": string(patchID)},
	}, workingDirectory)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("revert error = %v, want conflict", err)
	}
	content, err := os.ReadFile(filepath.Join(workingDirectory, "main.go"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(content) != "user changed\n" {
		t.Fatalf("content = %q, want drift preserved", string(content))
	}
	patch, err := patchRepo.Get(ctx, patchID)
	if err != nil {
		t.Fatalf("get patch: %v", err)
	}
	if patch.State != common.PatchStateApplied {
		t.Fatalf("patch state = %s, want applied after failed revert", patch.State)
	}
}

func TestRuntimeOrchestratorBlocksSensitivePatchUntilApproval(t *testing.T) {
	ctx := context.Background()
	workingDirectory := t.TempDir()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	approvalRepo := memory.NewApprovalRepository()
	patchRepo := memory.NewPatchRepository()
	workspaceID := common.WorkspaceID("ws-sensitive-patch-approval")
	runID := common.RunID("run-sensitive-patch-approval")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "sensitive patch", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "completed",
		stepOutputKeyPatches: []map[string]any{{
			"patchType": "file_write",
			"targetRef": ".env",
			"content":   "TOKEN=secret\n",
			"summary":   "write env",
		}},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		approvalRepo,
		RuntimeEvidenceRepositories{Patches: patchRepo},
		events.NewMemoryHub(),
	)

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnApproval || runItem.BlockingApprovalID == "" {
		t.Fatalf("run = %+v, want blocked on approval", runItem)
	}
	if _, err := os.Stat(filepath.Join(workingDirectory, ".env")); !os.IsNotExist(err) {
		t.Fatalf("sensitive patch wrote .env before approval, stat err = %v", err)
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(patches) != 0 {
		t.Fatalf("patches = %+v, want no patch record before approval", patches)
	}
	approval, err := approvalRepo.Get(ctx, runItem.BlockingApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if approval.Context["kind"] != "patch_approval_required" {
		t.Fatalf("approval context = %+v, want patch approval context", approval.Context)
	}
}

func TestRuntimeOrchestratorAllowsSensitivePatchWithApprovedApprovalID(t *testing.T) {
	ctx := context.Background()
	approvalRepo := memory.NewApprovalRepository()
	runID := common.RunID("run-sensitive-patch-approved")
	approvalID := common.ApprovalID("approval-sensitive-patch-approved")
	if err := approvalRepo.Create(ctx, &approvalmodel.Approval{ID: approvalID, RunID: runID, State: common.ApprovalStateApproved}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		memory.NewStepRepository(),
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewDecisionResolutionRepository(),
		approvalRepo,
		RuntimeEvidenceRepositories{Patches: memory.NewPatchRepository()},
		events.NewMemoryHub(),
	)

	spec, err := orch.patchApprovalBlockSpec(ctx, runID, "", map[string]any{
		stepOutputKeyPatches: []map[string]any{{
			"patchType":  "file_write",
			"targetRef":  ".env",
			"content":    "TOKEN=secret\n",
			"approvalId": string(approvalID),
		}},
	})
	if err != nil {
		t.Fatalf("patch approval gate: %v", err)
	}
	if spec.Reason != "" {
		t.Fatalf("approval spec = %+v, want no block with approved approval id", spec)
	}
}

func publishedTopicForTarget(items []events.Envelope, topic string, targetRef string) bool {
	for _, item := range items {
		if item.Topic == topic && item.Payload[events.PayloadKeyTargetRef] == targetRef {
			return true
		}
	}
	return false
}

func TestWorkspaceTargetPathRejectsSymlinkEscape(t *testing.T) {
	workingDirectory := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workingDirectory, "outside")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if _, err := workspaceTargetPath(workingDirectory, "outside/owned.txt"); err == nil {
		t.Fatalf("workspaceTargetPath should reject symlink escape")
	}
}

func TestRuntimePatchRequestRejectsReservedDeliverableTarget(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "delivery/weather.md",
		"content":   "weather",
	})
	if err == nil || !strings.Contains(err.Error(), "reserved for WorkOrch-managed artifacts") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want reserved target error", err)
	}
}

func TestRuntimePatchRequestRejectsReservedGovernanceReportTarget(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "decision_record.md",
		"content":   "decision",
	})
	if err == nil || !strings.Contains(err.Error(), "reserved for WorkOrch-managed artifacts") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want reserved governance target error", err)
	}
}

func TestRuntimePatchRequestRejectsUnresolvedTemplateContent(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "docs/weather.md",
		"content":   "{{ get_weather.stdout }}",
	})
	if err == nil || !strings.Contains(err.Error(), "unresolved runtime template") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want unresolved template error", err)
	}
}

func TestNextGraphStepInputRendersDependencyOutputTemplates(t *testing.T) {
	rootStepID := common.StepID("run-render/step-1")
	completedStep := &runmodel.Step{
		ID:    "run-render/block/get-weather",
		RunID: "run-render",
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyGraphRootStepID: string(rootStepID),
			stepInputKeyGraphNodeID:     "get_weather",
		},
		Output: map[string]any{
			"stdout": "Clear +21C",
			"nested": map[string]any{
				"value": "ok",
			},
		},
	}
	input := nextGraphStepInput(
		&runmodel.Run{ID: "run-render"},
		&runmodel.Step{ID: rootStepID},
		runtimemodel.BlockExecutionGraph{},
		runtimemodel.BlockExecutionNode{
			ID:   "write_report",
			Kind: llmcommon.RuntimeBlockKindPatchApply,
			Input: map[string]any{
				"patchType": "file_write",
				"targetRef": "docs/weather.md",
				"content":   "Weather: {{ get_weather.stdout }} / {{ get_weather.nested.value }}",
			},
		},
		map[string]bool{"get_weather": true},
		"",
		[]*runmodel.Step{completedStep},
		rootStepID,
	)
	if input["content"] != "Weather: Clear +21C / ok" {
		t.Fatalf("content = %q, want rendered dependency output", input["content"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesPatchConflictDiagnosticsAndWorkspaceManifest(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	patchRepo := memory.NewPatchRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	toolCallRepo := memory.NewToolCallRepository()
	var repairInput map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		switch {
		case request.StepID == "run-patch-conflict-repair/step-1":
			return map[string]any{
				"status": "completed",
				"patches": []map[string]any{
					{
						"patchType": "file_write",
						"targetRef": "llm-proxy/config.yaml",
						"content":   "config",
						"summary":   "write nested config",
					},
				},
			}, nil
		case strings.Contains(string(request.StepID), "/zz-llm-repair-"):
			repairInput = request.Input
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "repair received path diagnostics",
					"items": []map[string]any{
						{"item": "diagnostics available", "state": "satisfied", "evidenceRefs": []string{string(request.StepID)}},
					},
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected step %s", request.StepID)
		}
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			ToolCalls:          toolCallRepo,
			Patches:            patchRepo,
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
			MemoryItems:        memory.NewMemoryItemRepository(),
		},
		events.NewMemoryHub(),
	)

	workingDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDirectory, "llm-proxy"), []byte("binary"), 0o755); err != nil {
		t.Fatalf("seed executable file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-patch-conflict-repair")
	runID := common.RunID("run-patch-conflict-repair")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-patch-conflict-repair"),
		State:           common.RunStateCreated,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if repairInput == nil {
		t.Fatalf("repair input was not captured")
	}
	failedOutput, ok := repairInput["failedStepOutput"].(map[string]any)
	if !ok {
		t.Fatalf("failedStepOutput = %#v, want map", repairInput["failedStepOutput"])
	}
	diagnostics, ok := failedOutput["runtimeDiagnostics"].(map[string]any)
	if !ok || diagnostics["kind"] != "workspace_path_conflict" || diagnostics["conflictRef"] != "llm-proxy" {
		t.Fatalf("runtimeDiagnostics = %#v, want workspace path conflict", failedOutput["runtimeDiagnostics"])
	}
	if failedOutput["failureClass"] != failureClassWorkspacePath || !strings.Contains(fmt.Sprint(failedOutput["failureSignature"]), "conflictRef=llm-proxy") {
		t.Fatalf("failure classification = class:%#v signature:%#v, want workspace path conflict signature", failedOutput["failureClass"], failedOutput["failureSignature"])
	}
	manifest, ok := repairInput[stepInputKeyWorkspaceFileManifest].([]map[string]any)
	if !ok || len(manifest) != 1 || manifest[0]["path"] != "llm-proxy" || manifest[0]["executable"] != true {
		t.Fatalf("workspaceFileManifest = %#v, want executable llm-proxy file", repairInput[stepInputKeyWorkspaceFileManifest])
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if !hasCheckpointFailureClass(checkpoints, failureClassWorkspacePath) {
		t.Fatalf("checkpoints = %+v, want workspace path failure class checkpoint", checkpoints)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputSkipsWorkspaceManifestForDoDFailure(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	workingDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDirectory, "README.md"), []byte("workspace"), 0o644); err != nil {
		t.Fatalf("seed workspace file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-dod-repair-no-manifest")
	runID := common.RunID("run-dod-repair-no-manifest")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Output: map[string]any{
			"status": "completed",
			"dodAssessment": map[string]any{
				"satisfied": false,
				"summary":   "needs more evidence",
			},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, common.ErrInvalidArgument)

	if manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest]); len(manifest) != 0 {
		t.Fatalf("workspaceFileManifest = %+v, want no manifest for non-path DoD repair", input[stepInputKeyWorkspaceFileManifest])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputCarriesResolvedGovernanceAndFailedGraph(t *testing.T) {
	ctx := context.Background()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	runID := common.RunID("run-repair-governance-context")
	ticketID := common.TicketID("ticket-repair-governance-context")
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:           ticketID,
		WorkspaceID:  "ws-repair-governance-context",
		RunID:        runID,
		StepID:       "run-repair-governance-context/block/failed",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Provide missing direction.",
		State:        common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-repair-governance-context",
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"answer": "confirmed direction"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}
	orch := &RuntimeOrchestrator{tickets: ticketRepo, resolutions: resolutionRepo, steps: memory.NewStepRepository()}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "failed", Kind: shellcommon.StepKindCommand, Input: map[string]any{"command": "tool"}},
	}}
	failedStep := &runmodel.Step{
		ID:    "run-repair-governance-context/block/failed",
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepOutputKeyExecutionGraph: graph,
			stepInputKeyGraphNodeID:     "failed",
		},
		Output: map[string]any{"error": "failed"},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-governance-context"}, failedStep, 1, errors.New("failed"))

	if _, ok := input[stepOutputKeyExecutionGraph]; ok {
		t.Fatalf("executionGraph leaked into repair input: %+v", input[stepOutputKeyExecutionGraph])
	}
	if _, ok := input["failedExecutionGraph"].(runtimemodel.BlockExecutionGraph); !ok {
		t.Fatalf("failedExecutionGraph = %#v, want compact failed graph", input["failedExecutionGraph"])
	}
	history := mapSliceFromValue(input["governanceHistory"])
	if len(history) != 1 {
		t.Fatalf("governanceHistory = %#v, want resolved decision", input["governanceHistory"])
	}
	payload := mapFromValue(history[0]["resolutionPayload"])
	if history[0]["resolutionType"] != decisioncommon.ResolutionTypeProvideMissingInfo || payload["answer"] != "confirmed direction" {
		t.Fatalf("governanceHistory[0] = %#v, want resolution payload", history[0])
	}
}

func TestRuntimeOrchestrator_LLMInputIncludesExecutionContextPackage(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	artifactRepo := memory.NewArtifactRepository()
	var capturedInput map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		capturedInput = request.Input
		return map[string]any{
			"status": "completed",
			"dodAssessment": map[string]any{
				"satisfied": true,
				"summary":   "context package received",
				"items": []map[string]any{
					{"item": "context package available", "state": "satisfied", "evidenceRefs": []string{string(request.StepID)}},
				},
			},
		}, nil
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:            resultRepo,
			Artifacts:          artifactRepo,
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			MemoryItems:        memory.NewMemoryItemRepository(),
		},
		events.NewMemoryHub(),
	)

	workingDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDirectory, "README.md"), []byte("workspace"), 0o644); err != nil {
		t.Fatalf("seed workspace file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-context-package")
	runID := common.RunID("run-context-package")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := resultRepo.Create(ctx, &resultmodel.Result{
		ID:           "result-context-package-accepted",
		WorkspaceID:  workspaceID,
		RunID:        "run-context-package-old",
		GoalID:       "goal-context-package",
		DoDID:        "dod-context-package",
		State:        common.ResultStateAccepted,
		Title:        "Historical Accepted Result",
		Summary:      "previous accepted delivery",
		EvidenceRefs: []string{"evidence-old"},
		ArtifactRefs: []common.ArtifactID{"artifact-context-package-old"},
		UpdatedAt:    "2026-04-30T01:00:00Z",
	}); err != nil {
		t.Fatalf("seed result: %v", err)
	}
	if err := artifactRepo.Create(ctx, &resultmodel.Artifact{
		ID:          "artifact-context-package-old",
		WorkspaceID: workspaceID,
		RunID:       "run-context-package-old",
		ResultID:    "result-context-package-accepted",
		StepID:      "step-old",
		Name:        "Accepted summary",
		Summary:     "accepted artifact summary",
		URI:         "workorch://runs/run-context-package-old/results/summary",
		State:       common.ArtifactStateAvailable,
		UpdatedAt:   "2026-04-30T01:00:00Z",
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          "goal-context-package",
		DoDID:           "dod-context-package",
		State:           common.RunStateCreated,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"skills": []map[string]any{
				{"id": "skill-context", "name": "Context Skill", "instruction": "Use packaged context."},
			},
			"mcpCapabilities": []map[string]any{
				{"id": "mcp-context:tool:repo.summary", "serverId": "mcp-context", "kind": "tool", "name": "repo.summary", "usageHint": "Summarize repository."},
			},
			"memories": []map[string]any{
				{"id": "memory-context", "memoryType": "project_context", "summary": "Use context package", "content": "Package must include selected memory."},
			},
		},
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	pkg := mapFromValue(capturedInput[stepInputKeyExecutionContextPackage])
	if pkg["version"] != executionContextPackageVersion {
		t.Fatalf("executionContextPackage = %+v, want version %s", pkg, executionContextPackageVersion)
	}
	if manifest := mapSliceFromValue(pkg["workspaceFileManifest"]); len(manifest) != 0 {
		t.Fatalf("workspaceFileManifest = %+v, want no default workspace scan in LLM context package", pkg["workspaceFileManifest"])
	}
	accepted := mapFromValue(pkg["latestAcceptedResult"])
	if accepted["id"] != "result-context-package-accepted" {
		t.Fatalf("latestAcceptedResult = %+v, want historical accepted result", accepted)
	}
	if deliverables := mapSliceFromValue(pkg["currentDeliverables"]); len(deliverables) != 1 || deliverables[0]["id"] != "artifact-context-package-old" {
		t.Fatalf("currentDeliverables = %+v, want historical artifact", pkg["currentDeliverables"])
	}
	selected := mapFromValue(pkg["selectedContext"])
	if len(mapSliceFromValue(selected["skills"])) != 1 || len(mapSliceFromValue(selected["mcpCapabilities"])) != 1 || len(mapSliceFromValue(selected["memories"])) != 1 {
		t.Fatalf("selectedContext = %+v, want selected skill, mcp capability, and memory", selected)
	}
}

func TestRuntimeOrchestrator_EnrichLLMStepInputContextDoesNotPersistIntermediateStep(t *testing.T) {
	ctx := context.Background()
	stepRepo := newCountingStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-context-package-efficiency")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-context-package-efficiency",
		ExecutionContext: map[string]any{
			"skills": []map[string]any{
				{"id": "skill-efficiency", "name": "Efficiency Skill", "instruction": "Keep context available."},
			},
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{stepInputKeySoulStage: soulStageExecution},
	}

	orch.enrichLLMStepInputContext(ctx, run, step, nil)

	if got := stepRepo.updateCount.Load(); got != 0 {
		t.Fatalf("step update count = %d, want 0 redundant intermediate updates", got)
	}
	pkg := mapFromValue(step.Input[stepInputKeyExecutionContextPackage])
	if pkg["version"] != executionContextPackageVersion {
		t.Fatalf("executionContextPackage = %+v, want version %s", pkg, executionContextPackageVersion)
	}
	selected := mapFromValue(pkg["selectedContext"])
	if skills := mapSliceFromValue(selected["skills"]); len(skills) != 1 || skills[0]["id"] != "skill-efficiency" {
		t.Fatalf("selected skills = %+v, want injected skill context", selected["skills"])
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageReusesInputWorkspaceManifest(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{}
	runID := common.RunID("run-context-package-manifest-reuse")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-context-package-manifest-reuse"}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyWorkspaceFileManifest: []map[string]any{
				{"path": "README.md", "kind": "file", "size": float64(12)},
			},
		},
	}

	pkg := orch.runExecutionContextPackage(ctx, run, step, nil)

	manifest := mapSliceFromValue(pkg["workspaceFileManifest"])
	if len(manifest) != 1 || manifest[0]["path"] != "README.md" {
		t.Fatalf("workspaceFileManifest = %+v, want manifest from step input", pkg["workspaceFileManifest"])
	}
}

func TestWorkspaceFileManifestSkipsRepositoryAndRuntimeNoise(t *testing.T) {
	workingDirectory := t.TempDir()
	for _, path := range []string{
		".git/objects/aa/object",
		".workorch-data/system/event_record.json",
		"node_modules/pkg/index.js",
		"dist/bundle.js",
		"README.md",
		"cmd/workorch/main.go",
	} {
		target := filepath.Join(workingDirectory, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(target, []byte("content"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	manifest := workspaceFileManifestFromDirectory(workingDirectory)
	var paths []string
	for _, item := range manifest {
		paths = append(paths, fmt.Sprint(item["path"]))
	}
	if !reflect.DeepEqual(paths, []string{"README.md", "cmd/workorch/main.go"}) {
		t.Fatalf("manifest paths = %+v, want only source-facing files", paths)
	}
}

func TestCompactLLMContextDropsHeavyTransportFields(t *testing.T) {
	input := compactStepInputForLLMContext(map[string]any{
		stepInputKeyExecutionContextPackage: map[string]any{"workspaceFileManifest": []map[string]any{{"path": ".git/object"}}},
		stepInputKeyWorkspaceFileManifest:   []map[string]any{{"path": ".git/object"}},
		"failedStepId":                      "run-1/block/test",
		"failureReason":                     strings.Repeat("x", maxLLMContextStringBytes+16),
	})
	if _, ok := input[stepInputKeyExecutionContextPackage]; ok {
		t.Fatalf("executionContextPackage should be dropped from compact input")
	}
	if _, ok := input[stepInputKeyWorkspaceFileManifest]; ok {
		t.Fatalf("workspaceFileManifest should be dropped from compact input")
	}
	if got := fmt.Sprint(input["failureReason"]); len(got) > maxLLMContextStringBytes+3 {
		t.Fatalf("failureReason length = %d, want truncated", len(got))
	}

	output := compactStepOutputForLLMContext(map[string]any{
		"stderr":       strings.Repeat("e", maxLLMContextStringBytes+10),
		"failureClass": failureClassShellCommand,
		"nested": map[string]any{
			stepInputKeyExecutionContextPackage: map[string]any{"large": "value"},
		},
	})
	if output["failureClass"] != failureClassShellCommand {
		t.Fatalf("failureClass = %v, want preserved", output["failureClass"])
	}
	if got := fmt.Sprint(output["stderr"]); len(got) > maxLLMContextStringBytes+3 {
		t.Fatalf("stderr length = %d, want truncated", len(got))
	}
	nested := mapFromValue(output["nested"])
	if _, ok := nested[stepInputKeyExecutionContextPackage]; ok {
		t.Fatalf("nested executionContextPackage should be dropped")
	}
}

func TestRuntimeOrchestrator_FollowUpInputInheritsWorkspaceManifest(t *testing.T) {
	runID := common.RunID("run-follow-up-manifest-reuse")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-follow-up-manifest-reuse"}
	previousStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyExecutionContextPackage: map[string]any{
				stepInputKeyWorkspaceFileManifest: []map[string]any{
					{"path": "README.md", "kind": "file", "size": float64(12)},
				},
			},
		},
		Output: map[string]any{
			stepOutputKeyNextStep: "follow-up",
			stepOutputKeyNextKind: llmcommon.StepKindWorkspaceExecution,
		},
	}

	input := nextStepInputFromOutput(run, previousStep)
	manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest])
	if len(manifest) != 1 || manifest[0]["path"] != "README.md" {
		t.Fatalf("workspaceFileManifest = %+v, want inherited manifest", input[stepInputKeyWorkspaceFileManifest])
	}
	manifest[0]["path"] = "mutated.md"
	previousManifest := mapSliceFromValue(mapFromValue(previousStep.Input[stepInputKeyExecutionContextPackage])[stepInputKeyWorkspaceFileManifest])
	if previousManifest[0]["path"] != "README.md" {
		t.Fatalf("previous manifest mutated to %+v, want cloned follow-up manifest", previousManifest)
	}
}

func TestRuntimeOrchestrator_FollowUpInputInheritsStableExecutionContextPackage(t *testing.T) {
	runID := common.RunID("run-follow-up-context-package-reuse")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-follow-up-context-package-reuse"}
	previousStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyExecutionContextPackage: map[string]any{
				"version":     executionContextPackageVersion,
				"workspaceId": "ws-follow-up-context-package-reuse",
				"runId":       string(runID),
				"stepId":      string(runID) + "/step-1",
				"stage":       soulStageExecution,
				"latestAcceptedResult": map[string]any{
					"id":      "result-follow-up-reuse",
					"summary": "accepted historical result",
				},
				"currentDeliverables": []map[string]any{
					{"id": "artifact-follow-up-reuse", "summary": "available artifact"},
				},
				"selectedContext": map[string]any{
					"skills": []map[string]any{{"id": "skill-follow-up-reuse"}},
				},
				executionContextPackageHistoryKey: map[string]any{
					"latestAcceptedResult": true,
					"currentDeliverables":  true,
				},
			},
		},
		Output: map[string]any{
			stepOutputKeyNextStep: "follow-up",
			stepOutputKeyNextKind: llmcommon.StepKindWorkspaceExecution,
		},
	}

	input := nextStepInputFromOutput(run, previousStep)
	pkg := mapFromValue(input[stepInputKeyExecutionContextPackage])
	if pkg["stepId"] != nil || pkg["stage"] != nil || pkg["workspaceId"] != nil || pkg["runId"] != nil {
		t.Fatalf("inherited stable package = %+v, want no previous step identity fields", pkg)
	}
	if accepted := mapFromValue(pkg["latestAcceptedResult"]); accepted["id"] != "result-follow-up-reuse" {
		t.Fatalf("latestAcceptedResult = %+v, want inherited result", accepted)
	}
	if deliverables := mapSliceFromValue(pkg["currentDeliverables"]); len(deliverables) != 1 || deliverables[0]["id"] != "artifact-follow-up-reuse" {
		t.Fatalf("currentDeliverables = %+v, want inherited deliverable", pkg["currentDeliverables"])
	}
	if selected := mapFromValue(pkg["selectedContext"]); len(mapSliceFromValue(selected["skills"])) != 1 {
		t.Fatalf("selectedContext = %+v, want inherited selected context", selected)
	}
	historyChecked := mapFromValue(pkg[executionContextPackageHistoryKey])
	if !boolFromValue(historyChecked["latestAcceptedResult"]) || !boolFromValue(historyChecked["currentDeliverables"]) {
		t.Fatalf("workspaceHistoryChecked = %+v, want inherited history checks", historyChecked)
	}
	if _, ok := pkg["governanceHistory"]; ok {
		t.Fatalf("governanceHistory = %+v, want governance history rebuilt from current run context", pkg["governanceHistory"])
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageReusesEmptyHistoryChecks(t *testing.T) {
	ctx := context.Background()
	resultRepo := &countingResultRepository{inner: memory.NewResultRepository()}
	artifactRepo := &countingArtifactRepository{inner: memory.NewArtifactRepository()}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
		},
	}
	runID := common.RunID("run-context-package-empty-history-reuse")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-context-package-empty-history-reuse"}
	firstStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{stepInputKeySoulStage: soulStageExecution},
	}

	firstPackage := orch.runExecutionContextPackage(ctx, run, firstStep, &workspacemodel.Workspace{ID: run.WorkspaceID, WorkingDirectory: t.TempDir()})
	if got := resultRepo.listByWorkspaceCount.Load(); got != 1 {
		t.Fatalf("first result ListByWorkspace count = %d, want initial history check", got)
	}
	if got := artifactRepo.listByWorkspaceCount.Load(); got != 1 {
		t.Fatalf("first artifact ListByWorkspace count = %d, want initial history check", got)
	}
	historyChecked := mapFromValue(firstPackage[executionContextPackageHistoryKey])
	if !boolFromValue(historyChecked["latestAcceptedResult"]) || !boolFromValue(historyChecked["currentDeliverables"]) {
		t.Fatalf("first workspaceHistoryChecked = %+v, want explicit empty history check markers", historyChecked)
	}

	followUpStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/follow-up"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeySoulStage:               soulStageValidation,
			stepInputKeyExecutionContextPackage: firstPackage,
		},
	}
	secondPackage := orch.runExecutionContextPackage(ctx, run, followUpStep, &workspacemodel.Workspace{ID: run.WorkspaceID, WorkingDirectory: t.TempDir()})

	if got := resultRepo.listByWorkspaceCount.Load(); got != 1 {
		t.Fatalf("result ListByWorkspace count = %d, want follow-up to reuse empty history check", got)
	}
	if got := artifactRepo.listByWorkspaceCount.Load(); got != 1 {
		t.Fatalf("artifact ListByWorkspace count = %d, want follow-up to reuse empty history check", got)
	}
	secondHistoryChecked := mapFromValue(secondPackage[executionContextPackageHistoryKey])
	if !boolFromValue(secondHistoryChecked["latestAcceptedResult"]) || !boolFromValue(secondHistoryChecked["currentDeliverables"]) {
		t.Fatalf("second workspaceHistoryChecked = %+v, want retained empty history checks", secondHistoryChecked)
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageReusesStableFollowUpPackage(t *testing.T) {
	ctx := context.Background()
	resultRepo := &countingResultRepository{inner: memory.NewResultRepository()}
	artifactRepo := &countingArtifactRepository{inner: memory.NewArtifactRepository()}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
		},
	}
	runID := common.RunID("run-context-package-stable-reuse")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-context-package-stable-reuse",
		ExecutionContext: map[string]any{
			"governanceHistory": []map[string]any{
				{"kind": "decision", "summary": "current run context decision"},
			},
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/follow-up"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeySoulStage: soulStageValidation,
			stepInputKeyExecutionContextPackage: map[string]any{
				"stepId": "old-step-should-not-leak",
				"stage":  soulStageExecution,
				"latestAcceptedResult": map[string]any{
					"id":      "result-context-package-stable-reuse",
					"summary": "accepted reused result",
				},
				"currentDeliverables": []map[string]any{
					{"id": "artifact-context-package-stable-reuse", "summary": "reused artifact"},
				},
				"selectedContext": map[string]any{
					"skills": []map[string]any{{"id": "skill-context-package-stable-reuse"}},
				},
				"governanceHistory": []map[string]any{
					{"kind": "approval", "summary": "stale package approval"},
				},
			},
		},
	}

	pkg := orch.runExecutionContextPackage(ctx, run, step, &workspacemodel.Workspace{ID: run.WorkspaceID, WorkingDirectory: t.TempDir()})

	if got := resultRepo.listByWorkspaceCount.Load(); got != 0 {
		t.Fatalf("result ListByWorkspace count = %d, want 0 when stable result context is inherited", got)
	}
	if got := artifactRepo.listByWorkspaceCount.Load(); got != 0 {
		t.Fatalf("artifact ListByWorkspace count = %d, want 0 when stable deliverable context is inherited", got)
	}
	if pkg["stepId"] != string(step.ID) || pkg["stage"] != soulStageValidation {
		t.Fatalf("executionContextPackage = %+v, want current step identity", pkg)
	}
	if accepted := mapFromValue(pkg["latestAcceptedResult"]); accepted["id"] != "result-context-package-stable-reuse" {
		t.Fatalf("latestAcceptedResult = %+v, want inherited result", accepted)
	}
	if deliverables := mapSliceFromValue(pkg["currentDeliverables"]); len(deliverables) != 1 || deliverables[0]["id"] != "artifact-context-package-stable-reuse" {
		t.Fatalf("currentDeliverables = %+v, want inherited deliverable", pkg["currentDeliverables"])
	}
	if selected := mapFromValue(pkg["selectedContext"]); len(mapSliceFromValue(selected["skills"])) != 1 {
		t.Fatalf("selectedContext = %+v, want inherited selected context", selected)
	}
	if history := mapSliceFromValue(pkg["governanceHistory"]); len(history) != 1 || history[0]["summary"] != "current run context decision" {
		t.Fatalf("governanceHistory = %+v, want current run context governance history", pkg["governanceHistory"])
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageDoesNotScanProvidedWorkspaceByDefault(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := newCountingWorkspaceRepository()
	workspaceID := common.WorkspaceID("ws-context-package-workspace-reuse")
	workingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDir, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed workspace file: %v", err)
	}
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDir, State: common.WorkspaceStateRunning}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo}
	runID := common.RunID("run-context-package-workspace-reuse")
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{stepInputKeySoulStage: soulStageExecution},
	}

	pkg := orch.runExecutionContextPackage(ctx, run, step, workspace)

	if got := workspaceRepo.getCount.Load(); got != 0 {
		t.Fatalf("workspace Get count = %d, want 0 when workspace is already provided", got)
	}
	if manifest := mapSliceFromValue(pkg["workspaceFileManifest"]); len(manifest) != 0 {
		t.Fatalf("workspaceFileManifest = %+v, want no default workspace scan in LLM context package", pkg["workspaceFileManifest"])
	}
}

func TestRuntimeOrchestrator_ResultManifestReusesProvidedWorkspace(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := newCountingWorkspaceRepository()
	workspaceID := common.WorkspaceID("ws-result-manifest-workspace-reuse")
	workingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDir, "README.md"), []byte("accepted"), 0o644); err != nil {
		t.Fatalf("seed workspace file: %v", err)
	}
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDir, State: common.WorkspaceStateRunning}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	runID := common.RunID("run-result-manifest-workspace-reuse")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          "goal-result-manifest-workspace-reuse",
		DoDID:           "dod-result-manifest-workspace-reuse",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	orch := &RuntimeOrchestrator{
		workspaces: workspaceRepo,
		evidence: RuntimeEvidenceRepositories{
			Results:   memory.NewResultRepository(),
			Artifacts: memory.NewArtifactRepository(),
		},
	}

	err := orch.recordRunResultFromDoDAssessment(ctx, run, step, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "accepted with reusable workspace",
			"items": []map[string]any{
				{"item": "workspace reused", "state": "satisfied", "evidenceRefs": []string{string(step.ID)}},
			},
		},
	}, workspace)
	if err != nil {
		t.Fatalf("record result: %v", err)
	}

	if got := workspaceRepo.getCount.Load(); got != 0 {
		t.Fatalf("workspace Get count = %d, want 0 when result manifest receives workspace", got)
	}
	artifacts, err := orch.evidence.Artifacts.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %+v, want result summary and deliverable manifest", artifacts)
	}
	for _, artifact := range artifacts {
		if !reflect.DeepEqual(artifact.EvidenceRefs, []string{string(step.ID)}) {
			t.Fatalf("artifact %s evidence refs = %+v, want assessment refs", artifact.ID, artifact.EvidenceRefs)
		}
		if artifact.Category == resultcommon.ArtifactCategoryRuntimeEvidence && !strings.Contains(artifact.Summary, fmt.Sprintf("- `%s`", step.ID)) {
			t.Fatalf("manifest summary = %q, want acceptance evidence ref", artifact.Summary)
		}
	}
}

func TestRuntimeOrchestrator_ResultManifestDoesNotListWorkspaceFilesWhenNoPatch(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-result-manifest-step-context-reuse")
	workingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDir, "from-disk.md"), []byte("stale disk content"), 0o644); err != nil {
		t.Fatalf("seed workspace file: %v", err)
	}
	runID := common.RunID("run-result-manifest-step-context-reuse")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          "goal-result-manifest-step-context-reuse",
		DoDID:           "dod-result-manifest-step-context-reuse",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyExecutionContextPackage: map[string]any{
				stepInputKeyWorkspaceFileManifest: []map[string]any{
					{"path": "from-context.md", "kind": "file", "size": float64(12)},
				},
			},
		},
	}
	orch := &RuntimeOrchestrator{}

	manifest := orch.workspaceDeliverableManifestContent(ctx, run, step, map[string]any{
		"summary": "accepted with context manifest",
	}, []string{string(step.ID)}, &workspacemodel.Workspace{ID: workspaceID, WorkingDirectory: workingDir}, nil)

	if !strings.Contains(manifest, "No workspace file deliverables were produced in this run.") ||
		strings.Contains(manifest, "`from-context.md`") ||
		strings.Contains(manifest, "`from-disk.md`") {
		t.Fatalf("manifest = %q, want no workspace file deliverables without explicit patches", manifest)
	}
}

func TestRuntimeOrchestrator_ResultManifestListsOnlyPatchTargets(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-result-manifest-patch-rescan")
	workingDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDir, "from-disk.md"), []byte("fresh disk content"), 0o644); err != nil {
		t.Fatalf("seed workspace file: %v", err)
	}
	runID := common.RunID("run-result-manifest-patch-rescan")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          "goal-result-manifest-patch-rescan",
		DoDID:           "dod-result-manifest-patch-rescan",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyExecutionContextPackage: map[string]any{
				stepInputKeyWorkspaceFileManifest: []map[string]any{
					{"path": "from-context.md", "kind": "file", "size": float64(12)},
				},
			},
		},
		Output: map[string]any{
			stepOutputKeyPatches: []map[string]any{
				{"targetRef": "from-disk.md", "content": "fresh disk content"},
			},
		},
	}
	orch := &RuntimeOrchestrator{}

	manifest := orch.workspaceDeliverableManifestContent(ctx, run, step, map[string]any{
		"summary": "accepted after patch",
	}, []string{string(step.ID)}, &workspacemodel.Workspace{ID: workspaceID, WorkingDirectory: workingDir}, []resultPatchDetail{
		{TargetRef: "from-disk.md", PatchType: "file_write", Content: "fresh disk content", State: common.PatchStateApplied},
		{TargetRef: "from-disk.md", PatchType: "file_write", Content: "fresh disk content", State: common.PatchStateApplied},
	})

	if !strings.Contains(manifest, "`from-disk.md`") || strings.Contains(manifest, "`from-context.md`") {
		t.Fatalf("manifest = %q, want only explicit patch target deliverables", manifest)
	}
	deliverableSection := strings.Split(manifest, "## Code Change Evidence")[0]
	if got := strings.Count(deliverableSection, "`from-disk.md`"); got != 1 {
		t.Fatalf("manifest = %q, deliverable file row count = %d, want deduplicated row", manifest, got)
	}
}

func TestRuntimeOrchestrator_RunStateTransitionsReuseProvidedWorkspace(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := newCountingWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	workspaceID := common.WorkspaceID("ws-state-transition-workspace-reuse")
	runID := common.RunID("run-state-transition-workspace-reuse")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runs:       runRepo,
		workspaces: workspaceRepo,
	}

	if err := orch.blockOnDecision(ctx, run, workspace, common.TicketID("ticket-state-transition-workspace-reuse")); err != nil {
		t.Fatalf("block on decision: %v", err)
	}
	if got := workspaceRepo.getCount.Load(); got != 0 {
		t.Fatalf("workspace Get count after decision block = %d, want 0 when workspace is already provided", got)
	}
	if workspace.State != common.WorkspaceStateWaitingDecision {
		t.Fatalf("workspace state = %s, want waiting decision", workspace.State)
	}

	run.State = common.RunStateRunning
	run.BlockingTicketID = ""
	workspace.State = common.WorkspaceStateRunning
	if err := runRepo.Update(ctx, run); err != nil {
		t.Fatalf("reset run: %v", err)
	}
	if err := workspaceRepo.Update(ctx, workspace); err != nil {
		t.Fatalf("reset workspace: %v", err)
	}
	if err := orch.blockOnApproval(ctx, run, workspace, common.ApprovalID("approval-state-transition-workspace-reuse")); err != nil {
		t.Fatalf("block on approval: %v", err)
	}
	if got := workspaceRepo.getCount.Load(); got != 0 {
		t.Fatalf("workspace Get count after approval block = %d, want 0 when workspace is already provided", got)
	}
	if workspace.State != common.WorkspaceStateBlocked {
		t.Fatalf("workspace state = %s, want blocked", workspace.State)
	}
}

func TestRuntimeOrchestrator_ValidateCompleteReusesProvidedWorkspace(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := newCountingWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	workspaceID := common.WorkspaceID("ws-complete-workspace-reuse")
	runID := common.RunID("run-complete-workspace-reuse")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runs:       runRepo,
		workspaces: workspaceRepo,
	}

	if err := orch.validateAndCompleteRunWithWorkspace(ctx, run, common.StepID("run-complete-workspace-reuse/step-1"), workspace); err != nil {
		t.Fatalf("validate and complete: %v", err)
	}
	if got := workspaceRepo.getCount.Load(); got != 0 {
		t.Fatalf("workspace Get count = %d, want 0 when workspace is already provided", got)
	}
	if run.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", run.State)
	}
	if workspace.State != common.WorkspaceStateCompleted || workspace.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want completed and released", workspace)
	}
}

func TestRuntimeOrchestrator_ValidationRunAndWorkspaceWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	runRepo := &blockingValidatingRunRepository{
		inner:   memory.NewRunRepository(),
		started: started,
		release: release,
	}
	workspaceRepo := &blockingValidatingWorkspaceRepository{
		inner:   memory.NewWorkspaceRepository(),
		started: started,
		release: release,
	}
	workspaceID := common.WorkspaceID("ws-validating-write-parallel")
	runID := common.RunID("run-validating-write-parallel")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runs:       runRepo,
		workspaces: workspaceRepo,
	}

	done := make(chan error, 1)
	go func() {
		_, err := orch.syncRunAndWorkspaceForValidation(ctx, run, workspace)
		done <- err
	}()

	seen := map[string]bool{}
	for !seen["run-validating-update"] || !seen["workspace-validating-update"] {
		select {
		case name := <-started:
			seen[name] = true
		case <-ctx.Done():
			t.Fatalf("validation updates before release = %+v, want run and workspace writes started: %v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("sync validation state: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("sync validation state did not finish after release: %v", ctx.Err())
	}
	if run.State != common.RunStateValidating {
		t.Fatalf("run state = %s, want validating", run.State)
	}
	if workspace.State != common.WorkspaceStateValidating || workspace.ActiveRunID != runID {
		t.Fatalf("workspace = %+v, want validating with active run", workspace)
	}
}

func TestRuntimeOrchestrator_ActiveRunAndWorkspaceWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	runRepo := &blockingRunningRunRepository{
		inner:   memory.NewRunRepository(),
		started: started,
		release: release,
	}
	workspaceRepo := &blockingRunningWorkspaceRepository{
		inner:   memory.NewWorkspaceRepository(),
		started: started,
		release: release,
	}
	workspaceID := common.WorkspaceID("ws-active-write-parallel")
	runID := common.RunID("run-active-write-parallel")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateWaitingDecision, ActiveRunID: runID}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runs:       runRepo,
		workspaces: workspaceRepo,
	}

	done := make(chan error, 1)
	go func() {
		_, err := orch.syncRunAndWorkspaceForActiveState(ctx, run, common.WorkspaceStateRunning, workspace)
		done <- err
	}()

	seen := map[string]bool{}
	for !seen["run-running-update"] || !seen["workspace-running-update"] {
		select {
		case name := <-started:
			seen[name] = true
		case <-ctx.Done():
			t.Fatalf("active updates before release = %+v, want run and workspace writes started: %v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("sync active state: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("sync active state did not finish after release: %v", ctx.Err())
	}
	if run.State != common.RunStateRunning {
		t.Fatalf("run state = %s, want running", run.State)
	}
	if workspace.State != common.WorkspaceStateRunning || workspace.ActiveRunID != runID {
		t.Fatalf("workspace = %+v, want running with active run", workspace)
	}
}

func TestRuntimeOrchestrator_StartRunActiveWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	runRepo := &blockingRunningRunRepository{
		inner:   memory.NewRunRepository(),
		started: started,
		release: release,
	}
	workspaceRepo := &blockingRunningWorkspaceRepository{
		inner:   memory.NewWorkspaceRepository(),
		started: started,
		release: release,
	}
	stepRepo := memory.NewStepRepository()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-start-active-write-parallel")
	runID := common.RunID("run-start-active-write-parallel")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runtime:    &stubRuntimeAdapter{output: map[string]any{"status": "completed"}},
		runs:       runRepo,
		steps:      stepRepo,
		workspaces: workspaceRepo,
		eventHub:   hub,
	}

	done := make(chan error, 1)
	go func() {
		done <- orch.StartRun(ctx, runID)
	}()

	seen := map[string]bool{}
	for !seen["run-running-update"] || !seen["workspace-running-update"] {
		select {
		case name := <-started:
			seen[name] = true
		case err := <-done:
			t.Fatalf("StartRun returned before both active writes started: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for concurrent StartRun active writes, saw %+v: %v", seen, ctx.Err())
		}
	}
	for _, item := range hub.Published() {
		if item.Topic == events.TopicRunStateChanged && item.Payload[events.PayloadKeyState] == string(common.RunStateRunning) {
			t.Fatalf("published running run event before active writes were released: %+v", hub.Published())
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("StartRun did not complete after releasing active writes: %v", ctx.Err())
	}
	published := hub.Published()
	var runningRunIndex = -1
	var runningWorkspaceIndex = -1
	for idx, item := range published {
		if item.Topic == events.TopicRunStateChanged && item.Payload[events.PayloadKeyState] == string(common.RunStateRunning) {
			runningRunIndex = idx
		}
		if item.Topic == events.TopicWorkspaceStateChanged && item.Payload[events.PayloadKeyState] == string(common.WorkspaceStateRunning) {
			runningWorkspaceIndex = idx
		}
	}
	if runningRunIndex < 0 || runningWorkspaceIndex < 0 || runningRunIndex > runningWorkspaceIndex {
		t.Fatalf("published events = %+v, want running run event before running workspace event", published)
	}
	updatedRun, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if updatedRun.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", updatedRun.State)
	}
	updatedWorkspace, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if updatedWorkspace.State != common.WorkspaceStateCompleted || updatedWorkspace.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want completed and released", updatedWorkspace)
	}
}

func TestRuntimeOrchestrator_BlockedRunAndWorkspaceWritesConcurrently(t *testing.T) {
	cases := []struct {
		name           string
		runState       common.RunState
		workspaceState common.WorkspaceState
		configureRun   func(*runmodel.Run)
	}{
		{
			name:           "decision",
			runState:       common.RunStateBlockedOnDecision,
			workspaceState: common.WorkspaceStateWaitingDecision,
			configureRun: func(run *runmodel.Run) {
				run.BlockingTicketID = common.TicketID("ticket-blocked-write-parallel")
			},
		},
		{
			name:           "approval",
			runState:       common.RunStateBlockedOnApproval,
			workspaceState: common.WorkspaceStateBlocked,
			configureRun: func(run *runmodel.Run) {
				run.BlockingApprovalID = common.ApprovalID("approval-blocked-write-parallel")
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			started := make(chan string, 4)
			release := make(chan struct{})
			runRepo := &blockingBlockedRunRepository{
				inner:   memory.NewRunRepository(),
				started: started,
				release: release,
			}
			workspaceRepo := &blockingBlockedWorkspaceRepository{
				inner:   memory.NewWorkspaceRepository(),
				started: started,
				release: release,
			}
			workspaceID := common.WorkspaceID("ws-blocked-write-parallel-" + tt.name)
			runID := common.RunID("run-blocked-write-parallel-" + tt.name)
			workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}
			run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: tt.runState}
			tt.configureRun(run)
			if err := workspaceRepo.Create(ctx, workspace); err != nil {
				t.Fatalf("seed workspace: %v", err)
			}
			if err := runRepo.Create(ctx, run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			orch := &RuntimeOrchestrator{
				runs:       runRepo,
				workspaces: workspaceRepo,
			}

			done := make(chan error, 1)
			go func() {
				_, err := orch.syncRunAndWorkspaceForActiveState(ctx, run, tt.workspaceState, workspace)
				done <- err
			}()

			seen := map[string]bool{}
			for !seen["run-blocked-update"] || !seen["workspace-blocked-update"] {
				select {
				case name := <-started:
					seen[name] = true
				case <-ctx.Done():
					t.Fatalf("blocked updates before release = %+v, want run and workspace writes started: %v", seen, ctx.Err())
				}
			}
			close(release)
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("sync blocked state: %v", err)
				}
			case <-ctx.Done():
				t.Fatalf("sync blocked state did not finish after release: %v", ctx.Err())
			}
			if run.State != tt.runState {
				t.Fatalf("run state = %s, want %s", run.State, tt.runState)
			}
			if workspace.State != tt.workspaceState || workspace.ActiveRunID != runID {
				t.Fatalf("workspace = %+v, want %s with active run", workspace, tt.workspaceState)
			}
		})
	}
}

func TestRuntimeOrchestrator_BlockCheckpointUsesProvidedStepID(t *testing.T) {
	ctx := context.Background()
	runRepo := memory.NewRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	stepRepo := newCountingStepRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	workspaceID := common.WorkspaceID("ws-block-checkpoint-step-id")
	runID := common.RunID("run-block-checkpoint-step-id")
	stepID := common.StepID("run-block-checkpoint-step-id/step-1")
	ticketID := common.TicketID("run-block-checkpoint-step-id/step-1-decision")
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runs:       runRepo,
		steps:      stepRepo,
		workspaces: workspaceRepo,
		evidence: RuntimeEvidenceRepositories{
			RuntimeCheckpoints: checkpointRepo,
		},
		eventHub: events.NewMemoryHub(),
	}

	if err := orch.blockOnDecisionAtStep(ctx, run, workspace, ticketID, stepID); err != nil {
		t.Fatalf("block on decision: %v", err)
	}
	if got := stepRepo.listCount.Load(); got != 0 {
		t.Fatalf("step ListByRun count = %d, want 0 when current step id is provided", got)
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(checkpoints) != 1 {
		t.Fatalf("checkpoints = %+v, want one blocked checkpoint", checkpoints)
	}
	if checkpoints[0].CurrentStepID != stepID || checkpoints[0].CheckpointType != common.CheckpointTypeBlocked {
		t.Fatalf("checkpoint = %+v, want blocked checkpoint at %s", checkpoints[0], stepID)
	}
}

func TestRuntimeOrchestrator_CompletedStepEvidenceWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	toolCallRepo := &blockingToolCallRepository{
		inner:   memory.NewToolCallRepository(),
		started: started,
		release: release,
	}
	validationRepo := &blockingValidationResultRepository{
		inner:   memory.NewValidationResultRepository(),
		started: started,
		release: release,
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			ToolCalls:         toolCallRepo,
			ValidationResults: validationRepo,
		},
	}
	workspaceID := common.WorkspaceID("ws-completed-step-evidence-parallel")
	runID := common.RunID("run-completed-step-evidence-parallel")
	stepID := common.StepID("run-completed-step-evidence-parallel/step-1")
	toolCallID := common.ToolCallID("run-completed-step-evidence-parallel/step-1/tool-call")
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	step := &runmodel.Step{ID: stepID, RunID: runID, State: common.StepStateCompleted}
	toolCall := &runmodel.ToolCall{
		ID:          toolCallID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		State:       common.ToolCallStateRunning,
	}
	if err := toolCallRepo.Create(ctx, toolCall); err != nil {
		t.Fatalf("seed tool call: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- orch.recordCompletedRuntimeStepEvidence(ctx, run, step, toolCall, map[string]any{"status": "ok"})
	}()

	seen := map[string]bool{}
	for !seen["tool-call-completed-update"] || !seen["validation-passed-create"] {
		select {
		case name := <-started:
			seen[name] = true
		case <-ctx.Done():
			t.Fatalf("evidence writes before release = %+v, want tool call and validation writes started: %v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("record completed evidence: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("record completed evidence did not finish after release: %v", ctx.Err())
	}
	updatedToolCall, err := toolCallRepo.Get(ctx, toolCallID)
	if err != nil {
		t.Fatalf("get tool call: %v", err)
	}
	if updatedToolCall.State != common.ToolCallStateCompleted {
		t.Fatalf("tool call state = %s, want completed", updatedToolCall.State)
	}
	validations, err := validationRepo.ListByStep(ctx, stepID)
	if err != nil {
		t.Fatalf("list validations: %v", err)
	}
	if len(validations) != 1 || validations[0].State != common.ValidationResultStatePassed || validations[0].EvidenceRef != string(toolCallID) {
		t.Fatalf("validations = %+v, want passed validation with tool call evidence", validations)
	}
}

func TestRuntimeOrchestrator_CompletedStepStateAndCheckpointWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	stepRepo := &blockingCompletedStepRepository{
		inner:   memory.NewStepRepository(),
		started: started,
		release: release,
	}
	checkpointRepo := &blockingRuntimeCheckpointRepository{
		inner:   memory.NewRuntimeCheckpointRepository(),
		started: started,
		release: release,
	}
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			RuntimeCheckpoints: checkpointRepo,
		},
	}
	workspaceID := common.WorkspaceID("ws-completed-step-state-parallel")
	runID := common.RunID("run-completed-step-state-parallel")
	stepID := common.StepID("run-completed-step-state-parallel/step-1")
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}
	step := &runmodel.Step{
		ID:     stepID,
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateCompleted,
		Output: map[string]any{"status": "completed"},
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateDispatched,
		Output: map[string]any{"status": "running"},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- orch.persistCompletedStepState(ctx, run, step)
	}()

	seen := map[string]bool{}
	for !seen["step-completed-update"] || !seen["step_completed-checkpoint-create"] {
		select {
		case name := <-started:
			seen[name] = true
		case err := <-done:
			t.Fatalf("persistCompletedStepState returned before both writes started: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for completed step state writes, saw %+v: %v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("persistCompletedStepState: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("persistCompletedStepState did not finish after release: %v", ctx.Err())
	}
	updatedStep, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if updatedStep.State != common.StepStateCompleted {
		t.Fatalf("step state = %s, want completed", updatedStep.State)
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(checkpoints) != 1 || checkpoints[0].CurrentStepID != stepID || checkpoints[0].Data["event"] != "step_completed" {
		t.Fatalf("checkpoints = %+v, want completed step checkpoint", checkpoints)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchReusesLoadedRun(t *testing.T) {
	ctx := context.Background()
	runRepo := newCountingRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	stepRepo := memory.NewStepRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestrator(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewApprovalRepository(),
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-start-run-loaded-run-reuse")
	runID := common.RunID("run-start-run-loaded-run-reuse")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if got := runRepo.getCount.Load(); got != 1 {
		t.Fatalf("run Get count = %d, want only initial StartRun read", got)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchReusesLoadedWorkspace(t *testing.T) {
	ctx := context.Background()
	runRepo := memory.NewRunRepository()
	workspaceRepo := newCountingWorkspaceRepository()
	stepRepo := memory.NewStepRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestrator(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewApprovalRepository(),
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-start-run-loaded-workspace-reuse")
	runID := common.RunID("run-start-run-loaded-workspace-reuse")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if got := workspaceRepo.getCount.Load(); got != 1 {
		t.Fatalf("workspace Get count = %d, want only initial StartRun read", got)
	}
}

func TestRuntimeOrchestrator_ResumeRunDispatchReusesSyncedWorkspace(t *testing.T) {
	ctx := context.Background()
	runRepo := memory.NewRunRepository()
	workspaceRepo := newCountingWorkspaceRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	workspaceID := common.WorkspaceID("ws-resume-run-loaded-workspace-reuse")
	runID := common.RunID("run-resume-run-loaded-workspace-reuse")
	blockStepID := common.StepID(string(runID) + "/step-1")
	nextStepID := common.StepID(string(runID) + "/step-2")
	ticketID := common.TicketID(string(blockStepID) + "-decision")
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"completionState": "completed",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "accepted after resume",
			"items": []map[string]any{
				{"item": "resume workspace reused", "state": "satisfied", "evidenceRefs": []string{string(nextStepID)}},
			},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:               workspaceID,
		Title:            "ws",
		WorkingDirectory: t.TempDir(),
		State:            common.WorkspaceStateWaitingDecision,
		ActiveRunID:      runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		DoDID:            "dod-resume-run-loaded-workspace-reuse",
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    blockStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateCompleted,
		Output: map[string]any{
			stepOutputKeyNextStep: string(nextStepID),
			stepOutputKeyNextKind: llmcommon.StepKindWorkspaceExecution,
		},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      blockStepID,
		Reason:      "resume",
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-resume-run-loaded-workspace-reuse",
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"answer": "continue"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if got := workspaceRepo.getCount.Load(); got != 1 {
		t.Fatalf("workspace Get count = %d, want only resume state sync read", got)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
}

func TestRuntimeOrchestrator_GraphBatchDispatchReusesLoadedWorkspace(t *testing.T) {
	ctx := context.Background()
	runRepo := memory.NewRunRepository()
	workspaceRepo := newCountingWorkspaceRepository()
	stepRepo := memory.NewStepRepository()
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		switch {
		case request.StepID == "run-graph-batch-workspace-reuse/step-1":
			return map[string]any{
				"status": "completed",
				"executionGraph": map[string]any{
					"nodes": []map[string]any{
						{"id": "node-a", "kind": "test/node"},
						{"id": "node-b", "kind": "test/node"},
					},
				},
			}, nil
		case strings.Contains(string(request.StepID), "/block/node-a"):
			return map[string]any{"status": "completed", "node": "a"}, nil
		case strings.Contains(string(request.StepID), "/block/node-b"):
			return map[string]any{"status": "completed", "node": "b"}, nil
		default:
			return nil, fmt.Errorf("unexpected step %s", request.StepID)
		}
	}}
	orch := NewRuntimeOrchestrator(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		memory.NewApprovalRepository(),
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-graph-batch-workspace-reuse")
	runID := common.RunID("run-graph-batch-workspace-reuse")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if got := workspaceRepo.getCount.Load(); got != 1 {
		t.Fatalf("workspace Get count = %d, want graph batch to reuse the initial StartRun workspace", got)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("step count = %d, want initial graph step plus two batch nodes", len(steps))
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageIncludesRecentFailureSummary(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-context-package-failure")
	runID := common.RunID("run-context-package-failure")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-context-package-failure",
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateRunning}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	failed := duplicateShellFailureStep(runID, "failed-before-context")
	if err := stepRepo.Create(ctx, failed); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	pkg := orch.runExecutionContextPackage(ctx, run, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-context"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeySoulStage:             soulStageExecution,
			stepInputKeyRuntimeRepairFollowUp: true,
			"failedStepId":                    string(failed.ID),
		},
	}, nil)
	failures := mapSliceFromValue(pkg["recentFailureSummary"])
	if len(failures) != 1 || failures[0]["failureClass"] != failureClassShellCommand || failures[0]["failureSignature"] == "" {
		t.Fatalf("recentFailureSummary = %+v, want shell failure signature", failures)
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageSkipsInitialStepFailureHistoryRead(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := newCountingStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-context-package-initial")
	runID := common.RunID("run-context-package-initial")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-context-package-initial",
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateRunning}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	pkg := orch.runExecutionContextPackage(ctx, run, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{stepInputKeySoulStage: soulStagePlanning},
	}, nil)
	if got := stepRepo.listCount.Load(); got != 0 {
		t.Fatalf("step ListByRun count = %d, want no failure-history read for initial step", got)
	}
	if failures := mapSliceFromValue(pkg["recentFailureSummary"]); len(failures) != 0 {
		t.Fatalf("recentFailureSummary = %+v, want empty for initial step", failures)
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageSkipsFailureHistoryForNormalFollowUp(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := newCountingStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-context-package-normal-follow-up")
	runID := common.RunID("run-context-package-normal-follow-up")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-context-package-normal-follow-up",
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateRunning}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, duplicateShellFailureStep(runID, "failed-before-normal-follow-up")); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	pkg := orch.runExecutionContextPackage(ctx, run, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/llm-after-decision"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{stepInputKeySoulStage: soulStageDecision},
	}, nil)
	if got := stepRepo.listCount.Load(); got != 0 {
		t.Fatalf("step ListByRun count = %d, want no failure-history read for normal follow-up", got)
	}
	if failures := mapSliceFromValue(pkg["recentFailureSummary"]); len(failures) != 0 {
		t.Fatalf("recentFailureSummary = %+v, want empty for normal follow-up", failures)
	}
}

func TestRuntimeOrchestrator_ExecutionContextPackageBuildsIndependentContextReadsConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	started := make(chan string, 2)
	release := make(chan struct{})
	resultRepo := &blockingResultRepository{inner: memory.NewResultRepository(), started: started, release: release}
	artifactRepo := &blockingArtifactRepository{inner: memory.NewArtifactRepository(), started: started, release: release}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
		},
	}
	run := &runmodel.Run{
		ID:          "run-context-package-concurrent",
		WorkspaceID: "ws-context-package-concurrent",
		State:       common.RunStateRunning,
	}
	step := &runmodel.Step{
		ID:    "run-context-package-concurrent/step-1",
		RunID: run.ID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}

	done := make(chan map[string]any, 1)
	go func() {
		done <- orch.runExecutionContextPackage(ctx, run, step, &workspacemodel.Workspace{ID: run.WorkspaceID, WorkingDirectory: t.TempDir()})
	}()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case name := <-started:
			seen[name] = true
		case <-ctx.Done():
			t.Fatalf("context package reads did not start concurrently before release: seen=%v err=%v", seen, ctx.Err())
		}
	}
	close(release)
	select {
	case pkg := <-done:
		if pkg["version"] != executionContextPackageVersion {
			t.Fatalf("executionContextPackage = %+v, want version %s", pkg, executionContextPackageVersion)
		}
	case <-ctx.Done():
		t.Fatalf("context package did not finish after release: %v", ctx.Err())
	}
}

func TestRuntimeOrchestrator_DispatchesLLMRepairAfterGraphNodeFailure(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var repairInput map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		switch {
		case request.StepID == "run-runtime-repair/step-1":
			return map[string]any{
				"status": "completed",
				"stage":  "llm_execution_graph_plan",
				"executionGraph": map[string]any{
					"nodes": []map[string]any{
						{
							"id":   "write-source",
							"kind": llmcommon.RuntimeBlockKindPatchApply,
							"input": map[string]any{
								"patchType": "file_write",
								"targetRef": "main.go",
								"content":   "package main\n",
							},
						},
						{
							"id":        "run-tests",
							"kind":      shellcommon.StepKindCommand,
							"dependsOn": []string{"write-source"},
							"input": map[string]any{
								"command": "go",
								"args":    []string{"test", "./..."},
							},
						},
					},
				},
			}, nil
		case strings.Contains(string(request.StepID), "/block/write-source"):
			return map[string]any{
				"status": "completed",
				"patches": []map[string]any{
					{
						"patchType": "file_write",
						"targetRef": "main.go",
						"content":   "package main\n",
						"summary":   "write source",
					},
				},
			}, nil
		case strings.Contains(string(request.StepID), "/block/run-tests"):
			return map[string]any{
				"status":    "failed",
				"stage":     "shell_command_completed",
				"exitCode":  1,
				"stderr":    "missing go.sum entry for module providing package github.com/spf13/viper",
				"graphHint": "dependency metadata missing",
			}, errors.New("exit status 1")
		case strings.Contains(string(request.StepID), "/zz-llm-repair-"):
			repairInput = request.Input
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "repair evidence handled and verification passed",
					"items": []map[string]any{
						{
							"item":         "tests pass",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						},
					},
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected step %s", request.StepID)
		}
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			MemoryItems:        memory.NewMemoryItemRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-runtime-repair")
	runID := common.RunID("run-runtime-repair")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-runtime-repair"),
		State:           common.RunStateCreated,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s failureReason=%q, want completed", runItem.State, runItem.FailureReason)
	}
	if repairInput == nil || repairInput[stepInputKeyRuntimeRepairFollowUp] != true || repairInput[stepInputKeyRuntimeRepairAttempt] != 1 {
		t.Fatalf("repair input = %+v, want runtime repair follow-up attempt 1", repairInput)
	}
	failedOutput, ok := repairInput["failedStepOutput"].(map[string]any)
	if !ok || !strings.Contains(fmt.Sprint(failedOutput["stderr"]), "missing go.sum") {
		t.Fatalf("failedStepOutput = %+v, want stderr evidence", repairInput["failedStepOutput"])
	}
	if repairInput["failureClass"] != failureClassShellCommand || !strings.Contains(fmt.Sprint(repairInput["failureSignature"]), "kind=shell/command") {
		t.Fatalf("repair failure classification = class:%#v signature:%#v, want shell command signature", repairInput["failureClass"], repairInput["failureSignature"])
	}
	failedStep, err := stepRepo.Get(ctx, "run-runtime-repair/block/run-tests")
	if err != nil {
		t.Fatalf("get failed step: %v", err)
	}
	if failedStep.State != common.StepStateFailed {
		t.Fatalf("failed step state = %s, want failed evidence retained", failedStep.State)
	}
}

func TestRuntimeOrchestrator_SoftwareDevCodeAgentRepairsTestFailureAndDeliversDiff(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	artifactRepo := memory.NewArtifactRepository()
	patchRepo := memory.NewPatchRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	workingDirectory := t.TempDir()
	targetPath := filepath.Join(workingDirectory, "calculator.go")
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		stepID := string(request.StepID)
		switch {
		case stepID == "run-software-dev-code-agent/step-1":
			return map[string]any{
				"status": "completed",
				"stage":  "llm_execution_graph_plan",
				"executionGraph": map[string]any{
					"nodes": []map[string]any{
						{
							"id":   "write-buggy-source",
							"kind": llmcommon.RuntimeBlockKindPatchApply,
							"input": map[string]any{
								"patchType": "file_write",
								"targetRef": "calculator.go",
								"content":   "package calculator\n\nfunc Add(a, b int) int { return a - b }\n",
							},
						},
						{
							"id":        "run-tests",
							"kind":      shellcommon.StepKindCommand,
							"dependsOn": []string{"write-buggy-source"},
							"input": map[string]any{
								"command": "go",
								"args":    []string{"test", "./..."},
							},
						},
					},
				},
			}, nil
		case strings.Contains(stepID, "/block/write-buggy-source"):
			return map[string]any{
				"status": "completed",
				"patches": []map[string]any{{
					"patchType": "file_write",
					"targetRef": "calculator.go",
					"content":   "package calculator\n\nfunc Add(a, b int) int { return a - b }\n",
					"summary":   "write initial implementation",
				}},
			}, nil
		case strings.Contains(stepID, "/block/run-tests-fixed"):
			return map[string]any{
				"status":   "completed",
				"stage":    "shell_command_completed",
				"command":  "go test ./...",
				"exitCode": 0,
				"stdout":   "ok calculator 0.003s",
			}, nil
		case strings.Contains(stepID, "/block/run-tests"):
			return map[string]any{
				"status":   "failed",
				"stage":    "shell_command_completed",
				"command":  "go test ./...",
				"exitCode": 1,
				"stderr":   "calculator_test.go: Add(2,3)= -1, want 5",
			}, errors.New("exit status 1")
		case strings.Contains(stepID, "/zz-llm-repair-"):
			return map[string]any{
				"status": "completed",
				"stage":  "llm_execution_graph_repair",
				"executionGraph": map[string]any{
					"nodes": []map[string]any{
						{
							"id":   "fix-source",
							"kind": llmcommon.RuntimeBlockKindPatchApply,
							"input": map[string]any{
								"patchType": "unified_diff",
								"targetRef": "calculator.go",
								"diff":      "--- a/calculator.go\n+++ b/calculator.go\n@@ -1,3 +1,3 @@\n package calculator\n \n-func Add(a, b int) int { return a - b }\n+func Add(a, b int) int { return a + b }\n",
							},
						},
						{
							"id":        "run-tests-fixed",
							"kind":      shellcommon.StepKindCommand,
							"dependsOn": []string{"fix-source"},
							"input": map[string]any{
								"command": "go",
								"args":    []string{"test", "./..."},
							},
						},
					},
				},
			}, nil
		case strings.Contains(stepID, "/block/fix-source"):
			return map[string]any{
				"status": "completed",
				"patches": []map[string]any{{
					"patchType": "unified_diff",
					"targetRef": "calculator.go",
					"diff":      "--- a/calculator.go\n+++ b/calculator.go\n@@ -1,3 +1,3 @@\n package calculator\n \n-func Add(a, b int) int { return a - b }\n+func Add(a, b int) int { return a + b }\n",
					"summary":   "fix Add implementation",
				}},
			}, nil
		case strings.Contains(stepID, "/zz-llm-after-graph-"):
			return map[string]any{
				"status": "completed",
				"stage":  "llm_dod_assessment",
				"dodAssessment": map[string]any{
					"satisfied":             true,
					"summary":               "Software development DoD satisfied after automatic repair.",
					"goalOutcome":           "Add now returns the sum of two integers.",
					"implementationSummary": "Fixed calculator.go with a unified diff.",
					"verificationSummary":   "go test ./... passed after the repair.",
					"items": []map[string]any{
						{
							"item":         "implementation fixed",
							"state":        "satisfied",
							"evidenceRefs": []string{"run-software-dev-code-agent/block/fix-source"},
						},
						{
							"item":         "tests pass",
							"state":        "satisfied",
							"evidenceRefs": []string{"run-software-dev-code-agent/block/run-tests-fixed"},
						},
					},
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected software-dev step %s", request.StepID)
		}
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:            resultRepo,
			Artifacts:          artifactRepo,
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            patchRepo,
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			MemoryItems:        memoryRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-software-dev-code-agent")
	runID := common.RunID("run-software-dev-code-agent")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "software-dev", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-software-dev-code-agent"),
		State:           common.RunStateCreated,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s failureReason=%q, want completed", runItem.State, runItem.FailureReason)
	}
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !strings.Contains(string(content), "return a + b") {
		t.Fatalf("target content = %q, want repaired implementation", string(content))
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	var repairPatch *runmodel.Patch
	for _, patch := range patches {
		if patch.PatchType == "unified_diff" {
			repairPatch = patch
			break
		}
	}
	if len(patches) != 2 || repairPatch == nil || !strings.Contains(repairPatch.Diff, "+func Add(a, b int) int { return a + b }") {
		t.Fatalf("patches = %+v, want initial write and repair unified diff", patches)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 || results[0].State != common.ResultStateAccepted {
		t.Fatalf("results = %+v, want accepted result", results)
	}
	artifacts, err := artifactRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	var diffArtifact *resultmodel.Artifact
	for _, artifact := range artifacts {
		if artifact.ID == common.ArtifactID("run-software-dev-code-agent/artifact/code-change-diff") {
			diffArtifact = artifact
			break
		}
	}
	if diffArtifact == nil || !strings.Contains(diffArtifact.Summary, "```diff") || !strings.Contains(diffArtifact.Summary, "+func Add(a, b int) int { return a + b }") {
		t.Fatalf("diff artifact = %+v, want full repair diff", diffArtifact)
	}
	memories, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if !hasRunSummaryContent(memories, runID, "Patch targets:", "go test ./...") {
		t.Fatalf("memories = %+v, want run summary memory with patch and verification context", memories)
	}
}

func TestRuntimeOrchestrator_DoesNotRepairLLMProviderError(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if strings.Contains(string(request.StepID), "/zz-llm-repair-") {
			t.Fatalf("unexpected runtime repair step for provider failure: %s", request.StepID)
		}
		return map[string]any{
			"status": "failed",
			"stage":  "llm_provider_error",
			"error":  "invalid argument: llm provider http status 404",
		}, fmt.Errorf("%w: llm provider http status 404", common.ErrInvalidArgument)
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-llm-provider-error")
	runID := common.RunID("run-llm-provider-error")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-llm-provider-error"),
		State:           common.RunStateCreated,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed || !strings.Contains(runItem.FailureReason, "llm provider http status 404") {
		t.Fatalf("run = %+v, want failed with provider reason", runItem)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if len(steps) != 1 || steps[0].ID != common.StepID("run-llm-provider-error/step-1") {
		t.Fatalf("steps = %+v, want only initial failed provider step", steps)
	}
	if steps[0].Output["failureClass"] != failureClassProviderUnavailable {
		t.Fatalf("failureClass = %#v, want provider unavailable", steps[0].Output["failureClass"])
	}
}

func TestRuntimeOrchestrator_BlocksOnDuplicateRuntimeFailureSignature(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-duplicate-failure")
	runID := common.RunID("run-duplicate-failure")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-duplicate-failure"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateCompleted,
		Input: map[string]any{stepInputKeyRuntimeRepairFollowUp: true},
	}); err != nil {
		t.Fatalf("seed repair step: %v", err)
	}
	previousFailed := duplicateShellFailureStep(runID, "previous-shell-failure")
	if err := stepRepo.Create(ctx, previousFailed); err != nil {
		t.Fatalf("seed previous failed step: %v", err)
	}
	currentFailed := duplicateShellFailureStep(runID, "current-shell-failure")

	if err := orch.dispatchRuntimeRepairFollowUp(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-duplicate-failure"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}, currentFailed, errors.New("exit status 1")); err != nil {
		t.Fatalf("duplicate failure should block on decision without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on duplicate failure decision", runItem)
	}
	ticket, err := ticketRepo.Get(ctx, runItem.BlockingTicketID)
	if err != nil {
		t.Fatalf("get duplicate failure ticket: %v", err)
	}
	if ticket.Context["failureClass"] != failureClassShellCommand || ticket.Context["failureSignature"] == "" {
		t.Fatalf("ticket context = %+v, want shell failure signature", ticket.Context)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairExhaustionFailsRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-runtime-repair-exhausted")
	runID := common.RunID("run-runtime-repair-exhausted")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-runtime-repair-exhausted"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	for attempt := 1; attempt <= maxRuntimeRepairAttempts; attempt++ {
		stepID := common.StepID(fmt.Sprintf("%s/seed-repair-%d", runID, attempt))
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    stepID,
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateCompleted,
			Input: map[string]any{stepInputKeyRuntimeRepairFollowUp: true},
		}); err != nil {
			t.Fatalf("seed repair step: %v", err)
		}
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(fmt.Sprintf("%s/block/run-tests", runID)),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphNodeID: "run-tests",
		},
		Output: map[string]any{"status": "failed", "stderr": "still failing"},
	}

	if err := orch.dispatchRuntimeRepairFollowUp(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-runtime-repair-exhausted"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}, failedStep, errors.New("exit status 1")); err != nil {
		t.Fatalf("repair exhaustion should close run without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on decision after repair exhaustion", runItem)
	}
	ticket, err := ticketRepo.Get(ctx, runItem.BlockingTicketID)
	if err != nil {
		t.Fatalf("get repair exhaustion ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateOpen ||
		!strings.Contains(ticket.Reason, "runtime repair attempts exhausted") ||
		ticket.Context["attempts"] != maxRuntimeRepairAttempts {
		t.Fatalf("ticket = %+v, want repair exhaustion decision context", ticket)
	}
}

func TestRuntimeOrchestrator_StartRunPersistsRuntimeMemoryWrites(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status":  "completed",
		"stage":   "llm_dod_assessment",
		"summary": "Run completed with targeted tests",
		"memoryWrites": []map[string]any{
			{
				"memoryType": "user_preference",
				"summary":    "Default test command preference: go test ./...",
				"content":    "Prefer go test ./... for final verification.",
				"tags":       []string{"testing", "workflow"},
			},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			MemoryItems:        memoryRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-memory-write")
	runID := common.RunID("run-memory-write")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	items, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memory items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("memory items = %+v, want user preference and run summary", items)
	}
	if !hasMemoryItem(items, runID, "user_preference", "Default test command preference: go test ./...") ||
		!hasMemoryItem(items, runID, "run_summary", "Run completed with targeted tests") {
		t.Fatalf("memory items = %+v, want persisted user preference and run summary", items)
	}
	step, err := stepRepo.Get(ctx, common.StepID("run-memory-write/step-1"))
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	refs, ok := step.Output["memoryRefs"].([]string)
	if !ok || len(refs) != 2 {
		t.Fatalf("memoryRefs = %+v, want created memory ids", step.Output["memoryRefs"])
	}
}

func TestRuntimeOrchestrator_RunSummaryMemoryUsesDoDAssessmentSummary(t *testing.T) {
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_dod_assessment",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied":             true,
			"summary":               "目标已完成，成功查询到服务配置信息",
			"goalOutcome":           "已成功查询远程服务器配置信息。",
			"implementationSummary": "通过 SSH 获取 CPU、内存、磁盘和 Docker 版本。",
			"verificationSummary":   "命令执行成功，exit code 0。",
		},
	}

	requests := runtimeRunSummaryMemoryWrite(output)
	if len(requests) != 1 {
		t.Fatalf("memory requests = %+v, want one run summary", requests)
	}
	got := requests[0]
	if got.MemoryType != memorycommon.MemoryTypeRunSummary ||
		got.Summary != "目标已完成，成功查询到服务配置信息" ||
		!strings.Contains(got.Content, "通过 SSH 获取 CPU、内存、磁盘和 Docker 版本。") {
		t.Fatalf("run summary memory = %+v, want DoD assessment summary/content", got)
	}
}

func TestRuntimeOrchestrator_RuntimeMemoryWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	memoryRepo := &blockingMemoryItemRepository{
		inner:   memory.NewMemoryItemRepository(),
		started: started,
		release: release,
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			MemoryItems: memoryRepo,
		},
	}
	workspaceID := common.WorkspaceID("ws-memory-write-parallel")
	runID := common.RunID("run-memory-write-parallel")
	stepID := common.StepID("run-memory-write-parallel/step-1")
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID}
	step := &runmodel.Step{ID: stepID, RunID: runID}
	output := map[string]any{
		"status":  "completed",
		"stage":   "llm_dod_assessment",
		"summary": "parallel memory write summary",
		stepOutputKeyMemoryWrites: []map[string]any{
			{
				"memoryType": "user_preference",
				"summary":    "first default preference",
				"content":    "first content",
			},
			{
				"memoryType": "project_context",
				"summary":    "second confirmed project convention",
				"content":    "second content",
				"tags":       []string{"confirmed"},
			},
		},
	}

	done := make(chan struct {
		refs []string
		err  error
	}, 1)
	go func() {
		refs, err := orch.recordMemoryWrites(ctx, run, step, output)
		done <- struct {
			refs []string
			err  error
		}{refs: refs, err: err}
	}()

	wantStarted := map[string]bool{
		"memory-create:run-memory-write-parallel/step-1/memory/1": false,
		"memory-create:run-memory-write-parallel/step-1/memory/2": false,
		"memory-create:run-memory-write-parallel/step-1/memory/3": false,
	}
	for !allSeen(wantStarted) {
		select {
		case name := <-started:
			if _, ok := wantStarted[name]; ok {
				wantStarted[name] = true
			}
		case result := <-done:
			t.Fatalf("recordMemoryWrites returned before all memory writes started: refs=%+v err=%v seen=%+v", result.refs, result.err, wantStarted)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for concurrent memory writes, saw %+v: %v", wantStarted, ctx.Err())
		}
	}
	close(release)
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("recordMemoryWrites: %v", result.err)
		}
		wantRefs := []string{
			"run-memory-write-parallel/step-1/memory/1",
			"run-memory-write-parallel/step-1/memory/2",
			"run-memory-write-parallel/step-1/memory/3",
		}
		if !reflect.DeepEqual(result.refs, wantRefs) {
			t.Fatalf("refs = %+v, want %+v", result.refs, wantRefs)
		}
	case <-ctx.Done():
		t.Fatalf("recordMemoryWrites did not finish after release: %v", ctx.Err())
	}
	items, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memory items: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("memory items = %+v, want 3", items)
	}
}

func TestRuntimeOrchestrator_FiltersDisallowedRuntimeMemoryWrites(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "completed",
		"stage":  "runtime_step",
		"memoryWrites": []map[string]any{
			{
				"memoryType": "decision",
				"summary":    "approval transcript",
				"content":    "single approval raw text",
			},
			{
				"memoryType": "project_context",
				"summary":    "confirmed workspace convention",
				"content":    "Prefer workspace-local commands.",
				"tags":       []string{"confirmed"},
			},
			{
				"memoryType": "user_preference",
				"summary":    "deployment secret",
				"content":    "api key: should not persist",
			},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			MemoryItems:        memoryRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-memory-filter")
	runID := common.RunID("run-memory-filter")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	items, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memory items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("memory items = %+v, want only one allowed write", items)
	}
	if items[0].MemoryType != memorycommon.MemoryTypeProjectContext ||
		items[0].Summary != "confirmed workspace convention" {
		t.Fatalf("memory item = %+v, want persisted project context only", items[0])
	}
}

func allSeen(items map[string]bool) bool {
	for _, seen := range items {
		if !seen {
			return false
		}
	}
	return true
}

func TestRuntimeOrchestrator_StartRunAppliesUnifiedDiffPatchEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	patchRepo := memory.NewPatchRepository()
	workingDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workingDirectory, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	targetPath := filepath.Join(workingDirectory, "docs", "result.txt")
	if err := os.WriteFile(targetPath, []byte("before\nkeep\n"), 0o644); err != nil {
		t.Fatalf("seed target file: %v", err)
	}
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "ok",
		"patches": []map[string]any{
			{
				"patchType": "unified_diff",
				"targetRef": "docs/result.txt",
				"diff":      "--- a/docs/result.txt\n+++ b/docs/result.txt\n@@ -1,2 +1,2 @@\n-before\n+after\n keep\n",
				"summary":   "apply unified diff",
			},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            patchRepo,
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-unified-diff")
	runID := common.RunID("run-unified-diff")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target file: %v", err)
	}
	if string(content) != "after\nkeep\n" {
		t.Fatalf("target content = %q, want unified diff result", string(content))
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(patches) != 1 || patches[0].PatchType != "unified_diff" || patches[0].State != common.PatchStateApplied {
		t.Fatalf("patches = %+v, want applied unified diff patch", patches)
	}
	if !strings.Contains(patches[0].Diff, "-before\n+after") {
		t.Fatalf("patch diff = %q, want persisted unified diff", patches[0].Diff)
	}
}

func TestRuntimeOrchestrator_StartRunPersistsUnifiedDiffPatchEvidenceWithFileBackend(t *testing.T) {
	ctx := context.Background()
	dataRoot := t.TempDir()
	workspaceRepo := filerepo.NewWorkspaceRepository(filepath.Join(dataRoot, "workspaces.json"))
	runRepo := filerepo.NewRunRepository(filepath.Join(dataRoot, "runs.json"))
	stepRepo := filerepo.NewStepRepository(filepath.Join(dataRoot, "steps.json"))
	ticketRepo := filerepo.NewDecisionTicketRepository(filepath.Join(dataRoot, "decision_tickets.json"))
	resolutionRepo := filerepo.NewDecisionResolutionRepository(filepath.Join(dataRoot, "decision_resolutions.json"))
	approvalRepo := filerepo.NewApprovalRepository(filepath.Join(dataRoot, "approvals.json"))
	patchPath := filepath.Join(dataRoot, "patches.json")
	patchRepo := filerepo.NewPatchRepository(patchPath)
	workingDirectory := filepath.Join(dataRoot, "workspace")
	if err := os.MkdirAll(filepath.Join(workingDirectory, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workingDirectory, "docs", "result.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("seed target file: %v", err)
	}
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "ok",
		"patches": []map[string]any{
			{
				"patchType": "unified_diff",
				"targetRef": "docs/result.txt",
				"diff":      "--- a/docs/result.txt\n+++ b/docs/result.txt\n@@ -1 +1 @@\n-before\n+after\n",
			},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          filerepo.NewToolCallRepository(filepath.Join(dataRoot, "tool_calls.json")),
			Patches:            patchRepo,
			ValidationResults:  filerepo.NewValidationResultRepository(filepath.Join(dataRoot, "validation_results.json")),
			RuntimeCheckpoints: filerepo.NewRuntimeCheckpointRepository(filepath.Join(dataRoot, "runtime_checkpoints.json")),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-unified-diff-file")
	runID := common.RunID("run-unified-diff-file")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	reloadedPatchRepo := filerepo.NewPatchRepository(patchPath)
	patches, err := reloadedPatchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list reloaded patches: %v", err)
	}
	if len(patches) != 1 || patches[0].PatchType != "unified_diff" || patches[0].State != common.PatchStateApplied || patches[0].TargetRef != "docs/result.txt" {
		t.Fatalf("reloaded patches = %+v, want persisted applied unified diff patch", patches)
	}
	if !strings.Contains(patches[0].Diff, "-before\n+after") {
		t.Fatalf("reloaded patch diff = %q, want persisted unified diff", patches[0].Diff)
	}
}

func TestRuntimeOrchestrator_StartRunFailsInvalidUnifiedDiffPatch(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	patchRepo := memory.NewPatchRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "ok",
		"patches": []map[string]any{
			{
				"patchType": "unified_diff",
				"targetRef": "docs/result.txt",
				"diff":      "--- a/docs/result.txt\n+++ b/docs/result.txt\n@@ -1 +1 @@\n-missing\n+after\n",
			},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            patchRepo,
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workingDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workingDirectory, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workingDirectory, "docs", "result.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("seed target file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-unified-diff-invalid")
	runID := common.RunID("run-unified-diff-invalid")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run should close as failed without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(patches) != 1 || patches[0].State != common.PatchStateFailed {
		t.Fatalf("patches = %+v, want failed unified diff patch", patches)
	}
	content, err := os.ReadFile(filepath.Join(workingDirectory, "docs", "result.txt"))
	if err != nil {
		t.Fatalf("read target file: %v", err)
	}
	if string(content) != "before\n" {
		t.Fatalf("target content = %q, want unchanged content", string(content))
	}
}

func TestRuntimeOrchestrator_StartRunFailsInvalidRuntimePatchTarget(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	patchRepo := memory.NewPatchRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "ok",
		"patches": []map[string]any{
			{
				"targetRef": "../escape.txt",
				"content":   "invalid\n",
			},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            patchRepo,
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-patch-invalid")
	runID := common.RunID("run-patch-invalid")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run should close as failed without returning error: %v", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	patches, err := patchRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list patches: %v", err)
	}
	if len(patches) != 1 || patches[0].State != common.PatchStateFailed {
		t.Fatalf("patches = %+v, want one failed patch", patches)
	}
}

func TestRuntimeOrchestrator_StartRunPublishesRunAndStepEvents(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-evt")
	runID := common.RunID("run-evt")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	published := hub.Published()
	if len(published) < 7 {
		t.Fatalf("published events = %d, want at least 7", len(published))
	}
	if published[0].Version != events.WorkorchEventVersion {
		t.Fatalf("first event version = %s, want %s", published[0].Version, events.WorkorchEventVersion)
	}
	if published[0].Topic != events.TopicRunStateChanged || published[0].Payload[events.PayloadKeyState] != string(common.RunStateRunning) {
		t.Fatalf("first event = %#v, want run running", published[0])
	}
	if published[1].Topic != events.TopicWorkspaceStateChanged || published[1].Payload[events.PayloadKeyActiveRunID] != string(runID) {
		t.Fatalf("second event = %#v, want workspace active run bound", published[1])
	}
	if published[1].Payload[events.PayloadKeyState] != string(common.WorkspaceStateRunning) {
		t.Fatalf("second event state = %v, want workspace running", published[1].Payload[events.PayloadKeyState])
	}
	if published[2].Topic != events.TopicStepDispatched || published[2].Payload[events.PayloadKeyState] != string(common.StepStateDispatched) {
		t.Fatalf("third event = %#v, want step dispatched", published[2])
	}
	if published[3].Topic != events.TopicStepStateChanged || published[3].Payload[events.PayloadKeyState] != string(common.StepStateCompleted) {
		t.Fatalf("fourth event = %#v, want step completed", published[3])
	}
	if published[4].Topic != events.TopicRunStateChanged || published[4].Payload[events.PayloadKeyState] != string(common.RunStateValidating) {
		t.Fatalf("fifth event = %#v, want run validating", published[4])
	}
	if published[5].Topic != events.TopicWorkspaceStateChanged || published[5].Payload[events.PayloadKeyState] != string(common.WorkspaceStateValidating) {
		t.Fatalf("sixth event = %#v, want workspace validating", published[5])
	}
	if published[6].Topic != events.TopicRunStateChanged || published[6].Payload[events.PayloadKeyState] != string(common.RunStateCompleted) {
		t.Fatalf("seventh event = %#v, want run completed", published[6])
	}
	last := published[len(published)-1]
	if last.Topic != events.TopicWorkspaceStateChanged || last.Payload[events.PayloadKeyActiveRunID] != "" {
		t.Fatalf("last event = %#v, want workspace active run cleared", last)
	}
	if last.Payload[events.PayloadKeyState] != string(common.WorkspaceStateCompleted) {
		t.Fatalf("last event state = %v, want workspace completed", last.Payload[events.PayloadKeyState])
	}
}

func TestRuntimeOrchestrator_CompleteRunClearsStaleTerminalActiveRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-stale-active-complete")
	staleRunID := common.RunID("run-stale-failed")
	completedRunID := common.RunID("run-new-completed")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateFailed,
		ActiveRunID: staleRunID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: staleRunID, WorkspaceID: workspaceID, State: common.RunStateFailed}); err != nil {
		t.Fatalf("seed stale run: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: completedRunID, WorkspaceID: workspaceID, State: common.RunStateValidating}); err != nil {
		t.Fatalf("seed completed run: %v", err)
	}

	if err := orch.CompleteRun(ctx, completedRunID); err != nil {
		t.Fatalf("complete run: %v", err)
	}

	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.State != common.WorkspaceStateCompleted || ws.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want completed with no active run", ws)
	}
	last := hub.Published()[len(hub.Published())-1]
	if last.Topic != events.TopicWorkspaceStateChanged ||
		last.RunID != string(completedRunID) ||
		last.Payload[events.PayloadKeyState] != string(common.WorkspaceStateCompleted) ||
		last.Payload[events.PayloadKeyActiveRunID] != "" {
		t.Fatalf("last event = %#v, want completed workspace release for new run", last)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchesMultipleStepsWhenRuntimeRequestsNextStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-multi/step-1":
				return map[string]any{
					"status":   "completed",
					"nextStep": "run-multi/step-2",
					"nextKind": "runtime/test",
					"stage":    "prepare",
				}, nil
			case "run-multi/step-2":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-multi")
	runID := common.RunID("run-multi")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start multi-step run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		steps, _ := stepRepo.ListByRun(ctx, runID)
		t.Fatalf("run state = %s failureReason=%q calls=%#v steps=%+v, want completed", runItem.State, runItem.FailureReason, runtime.calls, steps)
	}

	step1, err := stepRepo.Get(ctx, "run-multi/step-1")
	if err != nil {
		t.Fatalf("get step1: %v", err)
	}
	if step1.State != common.StepStateCompleted {
		t.Fatalf("step1 state = %s, want completed", step1.State)
	}
	if step1.Output["nextStep"] != "run-multi/step-2" {
		t.Fatalf("step1 nextStep = %v, want run-multi/step-2", step1.Output["nextStep"])
	}

	step2, err := stepRepo.Get(ctx, "run-multi/step-2")
	if err != nil {
		t.Fatalf("get step2: %v", err)
	}
	if step2.State != common.StepStateCompleted {
		t.Fatalf("step2 state = %s, want completed", step2.State)
	}
	if step2.Input["previousStepId"] != "run-multi/step-1" {
		t.Fatalf("step2 previousStepId = %v, want run-multi/step-1", step2.Input["previousStepId"])
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-multi/step-1" || runtime.calls[1] != "run-multi/step-2" {
		t.Fatalf("runtime calls = %#v, want [run-multi/step-1 run-multi/step-2]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_StartRunExecutesBlockExecutionGraphBranch(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "prepare", Kind: "graph/mock"},
		{ID: "build", Kind: "graph/mock", DependsOn: []string{"prepare"}, Branch: "ship", Input: map[string]any{stepInputKeySoulStage: "artifact"}},
		{ID: "reject", Kind: "graph/mock", DependsOn: []string{"prepare"}, Branch: "reject"},
	}}
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-graph-branch/block/prepare":
				return map[string]any{
					"status":         "completed",
					"executionGraph": graph,
					"graphBranch":    "ship",
				}, nil
			case "run-graph-branch/block/build":
				return map[string]any{"status": "completed"}, nil
			default:
				return nil, errors.New("unexpected graph step")
			}
		},
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-graph-branch")
	runID := common.RunID("run-graph-branch")
	prepareStepID := common.StepID("run-graph-branch/block/prepare")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    prepareStepID,
		RunID: runID,
		Kind:  "graph/mock",
		State: common.StepStateDispatched,
		Input: map[string]any{
			"executionGraph": graph,
			"graphNodeId":    "prepare",
		},
	}); err != nil {
		t.Fatalf("seed graph start step: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start graph run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-graph-branch/block/prepare" || runtime.calls[1] != "run-graph-branch/block/build" {
		t.Fatalf("runtime calls = %#v, want prepare then build only", runtime.calls)
	}
	if _, err := stepRepo.Get(ctx, "run-graph-branch/block/reject"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("reject branch step err = %v, want not found", err)
	}
	buildStep, err := stepRepo.Get(ctx, "run-graph-branch/block/build")
	if err != nil {
		t.Fatalf("get build step: %v", err)
	}
	if buildStep.Input["graphBranch"] != "ship" || buildStep.Input["graphPreviousNodeId"] != "prepare" {
		t.Fatalf("build input = %+v, want graph branch and previous node", buildStep.Input)
	}
	if buildStep.Input[stepInputKeySoulStage] != soulStageExecution {
		t.Fatalf("build soulStage = %v, want execution", buildStep.Input[stepInputKeySoulStage])
	}
	checkpoints, err := checkpointRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if !hasCheckpointGraphBranch(checkpoints, "ship") {
		t.Fatalf("checkpoints = %+v, want graph branch evidence", checkpoints)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchesIndependentGraphNodesConcurrently(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "plan-a", Kind: "graph/mock"},
		{ID: "plan-b", Kind: "graph/mock"},
		{ID: "merge", Kind: "graph/mock", DependsOn: []string{"plan-a", "plan-b"}},
	}}
	var active atomic.Int32
	var maxActive atomic.Int32
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			switch request.StepID {
			case "run-graph-concurrent/step-1":
				return map[string]any{
					"status":         "completed",
					"executionGraph": graph,
				}, nil
			case "run-graph-concurrent/block/plan-a", "run-graph-concurrent/block/plan-b":
				current := active.Add(1)
				for {
					observed := maxActive.Load()
					if current <= observed || maxActive.CompareAndSwap(observed, current) {
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
				active.Add(-1)
				return map[string]any{"status": "completed"}, nil
			case "run-graph-concurrent/block/merge":
				return map[string]any{"status": "completed"}, nil
			default:
				return nil, errors.New("unexpected graph concurrent step")
			}
		},
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			MemoryItems:        memory.NewMemoryItemRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-graph-concurrent")
	runID := common.RunID("run-graph-concurrent")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated, InitialStepKind: "llm/workspace_execution"}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start graph run: %v", err)
	}

	if maxActive.Load() < 2 {
		t.Fatalf("maxActive = %d, want at least 2 concurrent ready graph nodes", maxActive.Load())
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	mergeStep, err := stepRepo.Get(ctx, "run-graph-concurrent/block/merge")
	if err != nil {
		t.Fatalf("get merge step: %v", err)
	}
	completed, _ := mergeStep.Input["graphCompletedNodes"].([]string)
	if !reflect.DeepEqual(completed, []string{"plan-a", "plan-b"}) {
		t.Fatalf("merge completed nodes = %#v, want both parallel nodes", mergeStep.Input["graphCompletedNodes"])
	}
}

func TestRuntimeOrchestrator_ResumeRunContinuesBlockExecutionGraphAfterDecision(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "prepare", Kind: "graph/mock"},
		{ID: "review", Kind: "graph/mock", DependsOn: []string{"prepare"}},
	}}
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-graph-decision/block/prepare":
				return map[string]any{
					"status":              "completed",
					"executionGraph":      graph,
					"blockDecisionReason": "need operator input",
				}, nil
			case "run-graph-decision/block/review":
				return map[string]any{"status": "completed"}, nil
			default:
				return nil, errors.New("unexpected graph step")
			}
		},
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-graph-decision")
	runID := common.RunID("run-graph-decision")
	prepareStepID := common.StepID("run-graph-decision/block/prepare")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    prepareStepID,
		RunID: runID,
		Kind:  "graph/mock",
		State: common.StepStateDispatched,
		Input: map[string]any{
			"executionGraph": graph,
			"graphNodeId":    "prepare",
		},
	}); err != nil {
		t.Fatalf("seed graph start step: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start graph run: %v", err)
	}
	blockedRun, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run: %v", err)
	}
	if blockedRun.State != common.RunStateBlockedOnDecision || blockedRun.BlockingTicketID == "" {
		t.Fatalf("blocked run = %+v, want decision block", blockedRun)
	}
	ticket, err := ticketRepo.Get(ctx, blockedRun.BlockingTicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("resolve ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-graph-decision"),
		TicketID: ticket.ID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume graph run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	reviewStep, err := stepRepo.Get(ctx, "run-graph-decision/block/review")
	if err != nil {
		t.Fatalf("get review step: %v", err)
	}
	if reviewStep.State != common.StepStateCompleted || reviewStep.Input["graphPreviousNodeId"] != "prepare" {
		t.Fatalf("review step = %+v, want resumed graph node after prepare", reviewStep)
	}
}

func TestRuntimeOrchestrator_ResumeRunFailsResolvedDecisionWithoutContinuation(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestratorWithResolutions(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-decision-no-continuation")
	runID := common.RunID("run-decision-no-continuation")
	stepID := common.StepID("run-decision-no-continuation/step-1")
	ticketID := common.TicketID("ticket-decision-no-continuation")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateCompleted,
		Output: map[string]any{"stage": "blocked_on_decision"},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-decision-no-continuation"),
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed terminal state", runItem.State)
	}
	if runItem.BlockingTicketID != "" || !strings.Contains(runItem.FailureReason, common.ErrConflict.Error()) {
		t.Fatalf("run terminal details = %+v, want cleared blocker and conflict reason", runItem)
	}
}

func TestRuntimeOrchestrator_CreatesDecisionTicketWithStructuredFields(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status":                "completed",
		"blockDecisionReason":   "need implementation direction",
		"blockDecisionKind":     "provide_missing_info",
		"blockDecisionQuestion": "Should the run call an external MCP tool?",
		"blockDecisionContext": map[string]any{
			"candidate": "mcp/tool_call",
		},
		"nextStep": "run-decision-structured/llm-after-decision",
		"nextKind": "llm/workspace_execution",
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			MemoryItems:        memory.NewMemoryItemRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-decision-structured")
	runID := common.RunID("run-decision-structured")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on decision", runItem)
	}
	ticket, err := ticketRepo.Get(ctx, runItem.BlockingTicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.Reason != "need implementation direction" ||
		ticket.DecisionKind != "provide_missing_info" ||
		ticket.Question != "Should the run call an external MCP tool?" ||
		ticket.Context["candidate"] != "mcp/tool_call" {
		t.Fatalf("ticket = %+v, want structured decision fields", ticket)
	}
}

func TestRuntimeOrchestrator_FailsLLMRunCompletionWithoutDoDAssessment(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "completed"}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-dod-gate-missing")
	runID := common.RunID("run-dod-gate-missing")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-1"),
		State:           common.RunStateCreated,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run should close as failed without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed || !strings.Contains(runItem.FailureReason, "dodAssessment is required") {
		t.Fatalf("run = %+v, want failed DoD assessment gate", runItem)
	}
	step, err := stepRepo.Get(ctx, common.StepID("run-dod-gate-missing/step-1"))
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed || !strings.Contains(step.Output["error"].(string), "dodAssessment is required") {
		t.Fatalf("step = %+v, want failed DoD assessment gate", step)
	}
}

func TestRuntimeOrchestrator_DispatchesLLMFollowUpAfterExecutionGraphCompletes(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "inspect", Kind: "graph/mock"},
	}}
	var followUpInput map[string]any
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			switch request.StepID {
			case "run-graph-follow-up/step-1":
				return map[string]any{
					"status":         "completed",
					"executionGraph": graph,
				}, nil
			case "run-graph-follow-up/block/inspect":
				return map[string]any{
					"status": "completed",
					"stdout": "workspace inspected",
				}, nil
			case "run-graph-follow-up/zz-llm-after-graph-run-graph-follow-up-step-1-inspect":
				followUpInput = request.Input
				return map[string]any{
					"status": "completed",
					"dodAssessment": map[string]any{
						"satisfied": true,
						"summary":   "graph completed",
						"items": []map[string]any{
							{
								"item":         "graph completed",
								"state":        "satisfied",
								"evidenceRefs": []string{"run-graph-follow-up/block/inspect"},
							},
						},
					},
				}, nil
			default:
				return nil, errors.New("unexpected graph follow-up step")
			}
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-graph-follow-up")
	runID := common.RunID("run-graph-follow-up")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-graph-follow-up"),
		State:           common.RunStateCreated,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	if !reflect.DeepEqual(runtime.calls, []common.StepID{
		"run-graph-follow-up/step-1",
		"run-graph-follow-up/block/inspect",
		"run-graph-follow-up/zz-llm-after-graph-run-graph-follow-up-step-1-inspect",
	}) {
		t.Fatalf("runtime calls = %#v, want graph then llm follow-up", runtime.calls)
	}
	if followUpInput["graphFollowUp"] != true ||
		followUpInput["graphPreviousNodeId"] != "inspect" ||
		followUpInput["previousStepId"] != "run-graph-follow-up/block/inspect" ||
		followUpInput[stepInputKeySoulStage] != soulStageValidation {
		t.Fatalf("followUpInput = %+v, want graph follow-up context", followUpInput)
	}
	if _, ok := followUpInput["previousStepOutput"]; ok {
		t.Fatalf("previousStepOutput = %+v, want graph follow-up to use graphEvidence only", followUpInput["previousStepOutput"])
	}
	evidence := mapSliceFromValue(followUpInput[stepInputKeyGraphEvidence])
	if len(evidence) != 1 || evidence[0]["nodeId"] != "inspect" {
		t.Fatalf("graphEvidence = %+v, want graph node evidence", followUpInput[stepInputKeyGraphEvidence])
	}
	output, _ := evidence[0]["output"].(map[string]any)
	if output["stdout"] != "workspace inspected" {
		t.Fatalf("graphEvidence output = %+v, want graph node output", output)
	}
}

func TestRuntimeOrchestrator_DispatchesLLMFollowUpAfterLLMPlannedShellLeaf(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var followUpInput map[string]any
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			switch request.StepID {
			case "run-shell-follow-up/step-1":
				return map[string]any{
					"status":   "completed",
					"nextStep": "run-shell-follow-up/shell-1",
					"nextKind": "shell/command",
					"nextInput": map[string]any{
						"command": "cat",
						"args":    []string{"result.txt"},
					},
				}, nil
			case "run-shell-follow-up/shell-1":
				return map[string]any{
					"status": "completed",
					"stdout": "result ready",
				}, nil
			case "run-shell-follow-up/zz-llm-after-step-run-shell-follow-up-shell-1-completion":
				followUpInput = request.Input
				return map[string]any{
					"status": "completed",
					"dodAssessment": map[string]any{
						"satisfied": true,
						"summary":   "shell evidence assessed",
						"items": []map[string]any{
							{
								"item":         "result ready",
								"state":        "satisfied",
								"evidenceRefs": []string{"run-shell-follow-up/shell-1"},
							},
						},
					},
				}, nil
			default:
				return nil, fmt.Errorf("unexpected shell follow-up step %s", request.StepID)
			}
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-shell-follow-up")
	runID := common.RunID("run-shell-follow-up")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-shell-follow-up"),
		State:           common.RunStateCreated,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	if followUpInput["completionFollowUp"] != true ||
		followUpInput["previousStepKind"] != "shell/command" ||
		followUpInput["previousStepId"] != "run-shell-follow-up/shell-1" ||
		followUpInput[stepInputKeySoulStage] != soulStageValidation {
		t.Fatalf("followUpInput = %+v, want completion follow-up context", followUpInput)
	}
	previousOutput, ok := followUpInput["previousStepOutput"].(map[string]any)
	if !ok || previousOutput["stdout"] != "result ready" {
		t.Fatalf("previousStepOutput = %+v, want shell output", followUpInput["previousStepOutput"])
	}
}

func TestRuntimeOrchestrator_DispatchesLLMFollowUpAfterLLMPatchSideEffect(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var followUpInput map[string]any
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			switch request.StepID {
			case "run-patch-follow-up/step-1":
				return map[string]any{
					"status": "completed",
					"patches": []map[string]any{
						{
							"patchType": "file_write",
							"targetRef": "result.txt",
							"content":   "result ready",
						},
					},
				}, nil
			case "run-patch-follow-up/zz-llm-after-step-run-patch-follow-up-step-1-completion":
				followUpInput = request.Input
				return map[string]any{
					"status": "completed",
					"dodAssessment": map[string]any{
						"satisfied": true,
						"summary":   "patch evidence assessed",
						"items": []map[string]any{
							{
								"item":         "result ready",
								"state":        "satisfied",
								"evidenceRefs": []string{"run-patch-follow-up/step-1/patch/1"},
							},
						},
					},
				}, nil
			default:
				return nil, fmt.Errorf("unexpected patch follow-up step %s", request.StepID)
			}
		},
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			MemoryItems:        memory.NewMemoryItemRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-patch-follow-up")
	runID := common.RunID("run-patch-follow-up")
	workingDirectory := t.TempDir()
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-patch-follow-up"),
		State:           common.RunStateCreated,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	if followUpInput["completionFollowUp"] != true ||
		followUpInput["previousStepKind"] != "llm/workspace_execution" ||
		followUpInput["previousStepId"] != "run-patch-follow-up/step-1" ||
		followUpInput[stepInputKeySoulStage] != soulStageValidation {
		t.Fatalf("followUpInput = %+v, want completion follow-up context", followUpInput)
	}
}

func TestRuntimeOrchestrator_GraphFollowUpIncludesAllCompletedParallelEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "cpu", Kind: "graph/mock"},
		{ID: "memory", Kind: "graph/mock"},
	}}
	var followUpInput map[string]any
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			switch {
			case request.StepID == "run-graph-parallel-follow-up/step-1":
				return map[string]any{
					"status":         "completed",
					"executionGraph": graph,
				}, nil
			case request.StepID == "run-graph-parallel-follow-up/block/cpu":
				return map[string]any{
					"status": "completed",
					"stdout": "cpu: 4 cores",
				}, nil
			case request.StepID == "run-graph-parallel-follow-up/block/memory":
				return map[string]any{
					"status": "completed",
					"stdout": "memory: 15Gi",
				}, nil
			case strings.HasPrefix(string(request.StepID), "run-graph-parallel-follow-up/zz-llm-after-graph-"):
				followUpInput = request.Input
				return map[string]any{
					"status": "completed",
					"dodAssessment": map[string]any{
						"satisfied": true,
						"summary":   "parallel graph evidence assessed",
						"items": []map[string]any{
							{
								"item":         "parallel graph completed",
								"state":        "satisfied",
								"evidenceRefs": []string{"run-graph-parallel-follow-up/block/cpu", "run-graph-parallel-follow-up/block/memory"},
							},
						},
					},
				}, nil
			default:
				return nil, fmt.Errorf("unexpected graph parallel follow-up step %s", request.StepID)
			}
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-graph-parallel-follow-up")
	runID := common.RunID("run-graph-parallel-follow-up")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-graph-parallel-follow-up"),
		State:           common.RunStateCreated,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	evidence, ok := followUpInput["graphEvidence"].([]map[string]any)
	if !ok || len(evidence) != 2 {
		t.Fatalf("graphEvidence = %#v, want two completed graph node evidence entries", followUpInput["graphEvidence"])
	}
	if evidence[0]["nodeId"] != "cpu" || evidence[1]["nodeId"] != "memory" {
		t.Fatalf("graphEvidence = %#v, want graph node order", evidence)
	}
	output0, _ := evidence[0]["output"].(map[string]any)
	output1, _ := evidence[1]["output"].(map[string]any)
	if output0["stdout"] != "cpu: 4 cores" || output1["stdout"] != "memory: 15Gi" {
		t.Fatalf("graphEvidence outputs = %#v, want both parallel outputs", evidence)
	}
	if !reflect.DeepEqual(followUpInput["graphCompletedNodes"], []string{"cpu", "memory"}) {
		t.Fatalf("graphCompletedNodes = %#v, want both completed nodes", followUpInput["graphCompletedNodes"])
	}
}

func TestRuntimeOrchestrator_GraphFollowUpReusesLoadedStepsForEvidence(t *testing.T) {
	ctx := context.Background()
	stepRepo := newCountingStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-graph-follow-up-efficiency")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-graph-follow-up-efficiency"}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "cpu", Kind: "graph/mock"},
		{ID: "memory", Kind: "graph/mock"},
	}}
	anchor := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/step-1"),
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateCompleted,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	for _, step := range []*runmodel.Step{
		anchor,
		{
			ID:     common.StepID(string(runID) + "/block/cpu"),
			RunID:  runID,
			Kind:   "graph/mock",
			State:  common.StepStateCompleted,
			Input:  map[string]any{stepInputKeyGraphNodeID: "cpu", stepInputKeyGraphRootStepID: string(anchor.ID)},
			Output: map[string]any{"stdout": "cpu ok"},
		},
		{
			ID:     common.StepID(string(runID) + "/block/memory"),
			RunID:  runID,
			Kind:   "graph/mock",
			State:  common.StepStateCompleted,
			Input:  map[string]any{stepInputKeyGraphNodeID: "memory", stepInputKeyGraphRootStepID: string(anchor.ID)},
			Output: map[string]any{"stdout": "memory ok"},
		},
	} {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}

	specs, steps := orch.readyGraphStepSpecsWithSteps(ctx, run, anchor)
	if len(specs) != 0 {
		t.Fatalf("ready specs = %+v, want no remaining graph node", specs)
	}
	input := orch.graphFollowUpStepInputFromSteps(run, anchor, steps)

	if got := stepRepo.listCount.Load(); got != 1 {
		t.Fatalf("step ListByRun count = %d, want 1 reused read", got)
	}
	if evidence := mapSliceFromValue(input[stepInputKeyGraphEvidence]); len(evidence) != 2 {
		t.Fatalf("graphEvidence = %+v, want two evidence items", input[stepInputKeyGraphEvidence])
	}
	if !reflect.DeepEqual(input[stepInputKeyGraphCompletedNodes], []string{"cpu", "memory"}) {
		t.Fatalf("graphCompletedNodes = %#v, want cpu and memory", input[stepInputKeyGraphCompletedNodes])
	}
}

func TestRuntimeOrchestrator_ReadyGraphStepSpecsReusesLoadedStepsForStepID(t *testing.T) {
	ctx := context.Background()
	stepRepo := newCountingStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-graph-step-id-efficiency")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-graph-step-id-efficiency"}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "verify-file", Kind: "graph/mock"},
	}}
	root := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/zz-llm-after-graph-root"),
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateCompleted,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	conflictingBaseStep := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/block/verify-file"),
		RunID:  runID,
		Kind:   "graph/mock",
		State:  common.StepStateCompleted,
		Input:  map[string]any{stepInputKeyGraphNodeID: "verify-file", stepInputKeyGraphRootStepID: string(runID) + "/previous-root"},
		Output: map[string]any{"stdout": "previous graph scope"},
	}
	for _, step := range []*runmodel.Step{root, conflictingBaseStep} {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}

	specs, _ := orch.readyGraphStepSpecsWithSteps(ctx, run, root)

	if got := stepRepo.listCount.Load(); got != 1 {
		t.Fatalf("step ListByRun count = %d, want one graph step scan", got)
	}
	if got := stepRepo.getCount.Load(); got != 0 {
		t.Fatalf("step Get count = %d, want step ID resolution to reuse loaded steps", got)
	}
	if len(specs) != 1 {
		t.Fatalf("ready specs = %+v, want one graph node spec", specs)
	}
	wantID := common.StepID(fmt.Sprintf("%s/block/%s", runID, runtimeChildID(root.ID, "verify-file")))
	if specs[0].ID != wantID {
		t.Fatalf("spec ID = %s, want scoped ID %s", specs[0].ID, wantID)
	}
}

func TestRuntimeOrchestrator_DispatchStepGraphFollowUpReusesLoadedStepsForEvidence(t *testing.T) {
	ctx := context.Background()
	stepRepo := newCountingStepRepository()
	runRepo := memory.NewRunRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	resultRepo := memory.NewResultRepository()
	artifactRepo := memory.NewArtifactRepository()
	workspaceID := common.WorkspaceID("ws-graph-follow-up-dispatch-efficiency")
	runID := common.RunID("run-graph-follow-up-dispatch-efficiency")
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "loadtest-tool", Kind: "mcp/tool_call"},
	}}
	var followUpInput map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		switch {
		case request.StepID == common.StepID(string(runID)+"/block/loadtest-tool"):
			return map[string]any{
				"status": "completed",
				"stdout": "loadtest evidence",
			}, nil
		case strings.HasPrefix(string(request.StepID), string(runID)+"/zz-llm-after-graph-"):
			followUpInput = request.Input
			return map[string]any{
				"status": "completed",
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied": true,
					"summary":   "graph follow-up accepted",
					"items": []map[string]any{
						{
							"item":         "graph evidence reused",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						},
					},
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected step %s", request.StepID)
		}
	}}
	orch := &RuntimeOrchestrator{
		runtime:    runtime,
		runs:       runRepo,
		steps:      stepRepo,
		workspaces: workspaceRepo,
		evidence: RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
			Results:            resultRepo,
			Artifacts:          artifactRepo,
		},
		eventHub: events.NewMemoryHub(),
	}
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateRunning, ActiveRunID: runID}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-graph-follow-up-dispatch-efficiency"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	root := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/llm-after-approval"),
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateCompleted,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	graphStep := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/block/loadtest-tool"),
		RunID:  runID,
		Kind:   "mcp/tool_call",
		State:  common.StepStateDispatched,
		Input:  map[string]any{stepInputKeyGraphNodeID: "loadtest-tool", stepInputKeyGraphRootStepID: string(root.ID), stepOutputKeyExecutionGraph: graph, "previousStepId": string(root.ID)},
		Output: map[string]any{"stdout": "loadtest evidence"},
	}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, root); err != nil {
		t.Fatalf("seed root step: %v", err)
	}
	if err := stepRepo.Create(ctx, graphStep); err != nil {
		t.Fatalf("seed graph step: %v", err)
	}

	if err := orch.dispatchStepWithWorkspace(ctx, run, graphStep.ID, graphStep.Kind, graphStep.Input, workspace); err != nil {
		t.Fatalf("dispatch graph step: %v", err)
	}

	if got := stepRepo.graphEvidenceListCount.Load(); got != 0 {
		t.Fatalf("graph evidence ListByRun count = %d, want reused loaded steps without an extra evidence scan", got)
	}
	if evidence := mapSliceFromValue(followUpInput[stepInputKeyGraphEvidence]); len(evidence) != 1 || evidence[0]["nodeId"] != "loadtest-tool" {
		t.Fatalf("graphEvidence = %+v, want loadtest-tool evidence", followUpInput[stepInputKeyGraphEvidence])
	}
	if _, ok := followUpInput["previousStepOutput"]; ok {
		t.Fatalf("previousStepOutput = %+v, want graph follow-up payload to avoid duplicate evidence", followUpInput["previousStepOutput"])
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
}

func TestRuntimeOrchestrator_DispatchesRepeatedGraphNodeIDInNewGraphScope(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	firstGraph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "verify-file", Kind: "graph/mock"},
	}}
	secondGraph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "verify-file", Kind: "graph/mock"},
	}}
	var secondGraphStepID common.StepID
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			switch {
			case request.StepID == "run-graph-repeat/step-1":
				return map[string]any{
					"status":         "completed",
					"executionGraph": firstGraph,
				}, nil
			case request.StepID == "run-graph-repeat/block/verify-file":
				return map[string]any{
					"status": "completed",
					"stdout": "first verify",
				}, nil
			case request.StepID == "run-graph-repeat/zz-llm-after-graph-run-graph-repeat-step-1-verify-file":
				return map[string]any{
					"status":         "completed",
					"executionGraph": secondGraph,
				}, nil
			case strings.HasPrefix(string(request.StepID), "run-graph-repeat/block/run-graph-repeat-zz-llm-after-graph-run-graph-repeat-step-1-verify-file-verify-file"):
				secondGraphStepID = request.StepID
				return map[string]any{
					"status": "completed",
					"stdout": "second verify",
				}, nil
			case strings.HasPrefix(string(request.StepID), "run-graph-repeat/zz-llm-after-graph-run-graph-repeat-zz-llm-after-graph-run-graph-repeat-step-1-verify-file-verify-file"):
				return map[string]any{
					"status": "completed",
					"dodAssessment": map[string]any{
						"satisfied": true,
						"summary":   "second graph evidence assessed",
						"items": []map[string]any{
							{
								"item":         "repeated graph node completed",
								"state":        "satisfied",
								"evidenceRefs": []string{string(secondGraphStepID)},
							},
						},
					},
				}, nil
			default:
				return nil, fmt.Errorf("unexpected repeated graph step %s", request.StepID)
			}
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-graph-repeat")
	runID := common.RunID("run-graph-repeat")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-graph-repeat"),
		State:           common.RunStateCreated,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		steps, _ := stepRepo.ListByRun(ctx, runID)
		t.Fatalf("run state = %s failureReason=%q calls=%#v steps=%+v, want completed", runItem.State, runItem.FailureReason, runtime.calls, steps)
	}
	if secondGraphStepID == "" || secondGraphStepID == "run-graph-repeat/block/verify-file" {
		t.Fatalf("secondGraphStepID = %q, want graph-root-scoped unique step id", secondGraphStepID)
	}
}

func TestRuntimeOrchestrator_FailsLLMRunCompletionWithoutDoDSummary(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "completed",
		"dodAssessment": map[string]any{
			"satisfied": true,
			"items": []map[string]any{
				{
					"item":         "tests pass",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-dod-summary-missing/validation/runtime-step"},
				},
			},
		},
	}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-dod-summary-missing")
	runID := common.RunID("run-dod-summary-missing")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-1"),
		State:           common.RunStateCreated,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run should close as failed without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed || !strings.Contains(runItem.FailureReason, "dodAssessment.summary is required") {
		t.Fatalf("run = %+v, want failed DoD summary gate", runItem)
	}
}

func TestRuntimeOrchestrator_CompletesLLMRunWithSatisfiedDoDAssessment(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	artifactRepo := memory.NewArtifactRepository()
	validationRepo := memory.NewValidationResultRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "completed",
		"dodAssessment": map[string]any{
			"satisfied": true,
			"items": []map[string]any{
				{
					"item":         "tests pass",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-dod-gate-complete/step-1/validation/runtime-step"},
				},
			},
			"summary": "DoD satisfied",
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:           resultRepo,
			Artifacts:         artifactRepo,
			ValidationResults: validationRepo,
			MemoryItems:       memoryRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-dod-gate-complete")
	runID := common.RunID("run-dod-gate-complete")
	workingDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDirectory, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write deliverable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workingDirectory, "README.md"), []byte("# Delivery\n"), 0o644); err != nil {
		t.Fatalf("write deliverable readme: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-1"),
		State:           common.RunStateCreated,
		InitialStepKind: "llm/workspace_execution",
		ExecutionContext: map[string]any{
			"skillIds": []string{"skill-dod-gate"},
		},
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want one accepted result", results)
	}
	result := results[0]
	if result.State != common.ResultStateAccepted || result.Summary != "DoD satisfied" {
		t.Fatalf("result = %+v, want accepted DoD result", result)
	}
	if result.WorkspaceID != workspaceID || result.RunID != runID || result.DoDID != "dod-1" {
		t.Fatalf("result refs = %+v, want workspace/run/dod refs", result)
	}
	if result.SourceType != "dod_assessment" || result.SourceRef != "run-dod-gate-complete/step-1" {
		t.Fatalf("result source = %+v, want DoD assessment step source", result)
	}
	if !reflect.DeepEqual(result.EvidenceRefs, []string{"run-dod-gate-complete/step-1", "run-dod-gate-complete/step-1/validation/runtime-step"}) {
		t.Fatalf("result evidence = %+v, want step and validation refs", result.EvidenceRefs)
	}
	if !result.DoDAssessment.Satisfied || result.DoDAssessment.Summary != "DoD satisfied" || len(result.DoDAssessment.Items) != 1 {
		t.Fatalf("result DoD assessment = %+v, want satisfied item-level assessment", result.DoDAssessment)
	}
	if result.SummaryDetail.GoalOutcome != "DoD satisfied" ||
		result.SummaryDetail.ImplementationSummary != "DoD satisfied" ||
		!reflect.DeepEqual(result.SummaryDetail.ArtifactRefs, []common.ArtifactID{
			"run-dod-gate-complete/artifact/result-summary",
			"run-dod-gate-complete/artifact/deliverable-manifest",
		}) ||
		!reflect.DeepEqual(result.SummaryDetail.EvidenceRefs, []string{"run-dod-gate-complete/step-1", "run-dod-gate-complete/step-1/validation/runtime-step"}) {
		t.Fatalf("result summary detail = %+v, want structured accepted result summary", result.SummaryDetail)
	}
	if result.DoDAssessment.Items[0].Item != "tests pass" ||
		result.DoDAssessment.Items[0].State != "satisfied" ||
		!reflect.DeepEqual(result.DoDAssessment.Items[0].EvidenceRefs, []string{"run-dod-gate-complete/step-1/validation/runtime-step"}) {
		t.Fatalf("result DoD assessment items = %+v, want item state with evidence", result.DoDAssessment.Items)
	}
	if !reflect.DeepEqual(result.ArtifactRefs, []common.ArtifactID{
		"run-dod-gate-complete/artifact/result-summary",
		"run-dod-gate-complete/artifact/deliverable-manifest",
	}) {
		t.Fatalf("result artifacts = %+v, want summary and deliverable manifest artifact refs", result.ArtifactRefs)
	}
	artifacts, err := artifactRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %+v, want summary and deliverable manifest artifacts", artifacts)
	}
	var summaryArtifact *resultmodel.Artifact
	var manifestArtifact *resultmodel.Artifact
	for _, artifact := range artifacts {
		switch artifact.ID {
		case "run-dod-gate-complete/artifact/result-summary":
			summaryArtifact = artifact
		case "run-dod-gate-complete/artifact/deliverable-manifest":
			manifestArtifact = artifact
		}
	}
	if summaryArtifact == nil || summaryArtifact.ResultID != result.ID || summaryArtifact.ArtifactType != "text" || summaryArtifact.State != common.ArtifactStateAvailable {
		t.Fatalf("summary artifact = %+v, want available text artifact linked to result", summaryArtifact)
	}
	if summaryArtifact.Category != resultcommon.ArtifactCategoryFinalSummary ||
		!strings.Contains(summaryArtifact.Summary, "Accepted Result Summary") ||
		!strings.Contains(summaryArtifact.Summary, "DoD satisfied") ||
		summaryArtifact.ContentType != "text/markdown; charset=utf-8" {
		t.Fatalf("summary artifact content = %+v, want DoD summary text artifact", summaryArtifact)
	}
	if manifestArtifact == nil || manifestArtifact.ResultID != result.ID || manifestArtifact.ArtifactType != "report" || manifestArtifact.State != common.ArtifactStateAvailable {
		t.Fatalf("manifest artifact = %+v, want available report artifact linked to result", manifestArtifact)
	}
	if manifestArtifact.Category != resultcommon.ArtifactCategoryRuntimeEvidence {
		t.Fatalf("manifest artifact category = %q, want runtime evidence", manifestArtifact.Category)
	}
	if !strings.Contains(manifestArtifact.Summary, "Workspace Deliverable Manifest") ||
		!strings.Contains(manifestArtifact.Summary, "No workspace file deliverables were produced in this run.") ||
		strings.Contains(manifestArtifact.Summary, "| `main.go` | source |") ||
		strings.Contains(manifestArtifact.Summary, "| `README.md` | documentation |") ||
		!strings.Contains(manifestArtifact.Summary, "## DoD Relation") ||
		!strings.Contains(manifestArtifact.Summary, "run-dod-gate-complete/step-1/validation/runtime-step") {
		t.Fatalf("manifest summary = %q, want scoped manifest and evidence refs", manifestArtifact.Summary)
	}
	memories, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if !hasSkillFeedbackMemory(memories, runID, "skill-dod-gate") {
		t.Fatalf("memories = %+v, want skill feedback memory", memories)
	}
}

func TestRuntimeOrchestrator_ReusesValidatedDoDAssessmentForCompletionEvidence(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-dod-assessment-reuse")
	workspaceID := common.WorkspaceID("ws-dod-assessment-reuse")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-dod-assessment-reuse",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"skillIds": []string{"skill-dod-assessment-reuse"},
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	assessment := map[string]any{
		"satisfied": true,
		"items": []map[string]any{
			{
				"item":         "validated assessment reused",
				"state":        "satisfied",
				"evidenceRefs": []string{string(step.ID)},
			},
		},
		"summary": "  validated DoD summary  ",
	}
	output := map[string]any{stepOutputKeyDoDAssessment: assessment}
	validated, summary, err := validatedCompletionDoDAssessment(run, output)
	if err != nil {
		t.Fatalf("validated DoD assessment: %v", err)
	}
	validated["summary"] = "validated DoD summary"
	if summary != "validated DoD summary" {
		t.Fatalf("summary=%q, want trimmed validated summary", summary)
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Results:     memory.NewResultRepository(),
			Artifacts:   memory.NewArtifactRepository(),
			MemoryItems: memory.NewMemoryItemRepository(),
		},
	}

	if err := orch.recordRunResultFromValidatedDoDAssessment(ctx, run, step, validated, summary, nil); err != nil {
		t.Fatalf("record result from validated assessment: %v", err)
	}
	if err := orch.recordSkillFeedbackMemoryFromValidatedDoDAssessment(ctx, run, step, validated, summary); err != nil {
		t.Fatalf("record skill feedback from validated assessment: %v", err)
	}
	if err := orch.recordRunSummaryMemoryFromValidatedDoDAssessment(ctx, run, step, validated, summary); err != nil {
		t.Fatalf("record run summary memory from validated assessment: %v", err)
	}
	results, err := orch.evidence.Results.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 || results[0].Summary != "validated DoD summary" {
		t.Fatalf("results = %+v, want accepted result from validated assessment", results)
	}
	memories, err := orch.evidence.MemoryItems.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if !hasSkillFeedbackMemory(memories, runID, "skill-dod-assessment-reuse") {
		t.Fatalf("memories = %+v, want skill feedback from same validated assessment", memories)
	}
	if !hasCompletionRunSummaryMemory(memories, runID, step.ID, "validated DoD summary") {
		t.Fatalf("memories = %+v, want completion run summary memory from same validated assessment", memories)
	}
}

func TestRuntimeOrchestrator_RecordsValidationCommandMemoriesFromDoDAssessment(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-validation-command-memory")
	workspaceID := common.WorkspaceID("ws-validation-command-memory")
	evidenceStepID := common.StepID(string(runID) + "/block/verify")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-validation-command-memory",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	stepRepo := memory.NewStepRepository()
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    evidenceStepID,
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"command":  "go test ./...",
			"exitCode": 1,
			"stderr":   "compile error in internal/app.go",
		},
	}); err != nil {
		t.Fatalf("seed evidence step: %v", err)
	}
	assessment := map[string]any{
		"satisfied": true,
		"summary":   "DoD satisfied after repair",
		"items": []map[string]any{
			{
				"item":         "verification command tracked",
				"state":        "satisfied",
				"evidenceRefs": []string{string(evidenceStepID)},
			},
		},
	}
	memoryRepo := memory.NewMemoryItemRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			MemoryItems: memoryRepo,
		},
	}
	if err := orch.recordValidationCommandMemoriesFromValidatedDoDAssessment(ctx, run, step, assessment); err != nil {
		t.Fatalf("recordValidationCommandMemoriesFromValidatedDoDAssessment: %v", err)
	}
	memories, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	var commandMemory *memorymodel.MemoryItem
	var lessonMemory *memorymodel.MemoryItem
	for _, item := range memories {
		if item == nil || item.RunID != runID {
			continue
		}
		switch item.MemoryType {
		case memorycommon.MemoryTypeProjectContext:
			if item.Summary == "Validation command baseline captured" {
				commandMemory = item
			}
		case memorycommon.MemoryTypeExecutionLesson:
			if item.Summary == "Validation failure signals captured for repair" {
				lessonMemory = item
			}
		}
	}
	if commandMemory == nil ||
		!strings.Contains(commandMemory.Content, "go test ./...") ||
		!strings.Contains(commandMemory.Content, string(evidenceStepID)) {
		t.Fatalf("command memory = %+v, want command with provenance evidence", commandMemory)
	}
	if lessonMemory == nil ||
		!strings.Contains(lessonMemory.Content, "compile error in internal/app.go") ||
		!strings.Contains(lessonMemory.Content, "exitCode=1") {
		t.Fatalf("lesson memory = %+v, want failure lesson content", lessonMemory)
	}
}

func TestRuntimeOrchestrator_ResultSummaryIncludesRuntimeEvidenceOutput(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-result-summary-evidence-output")
	workspaceID := common.WorkspaceID("ws-result-summary-evidence-output")
	evidenceStepID := common.StepID("run-result-summary-evidence-output/block/query_server_config")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-result-summary-evidence-output",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"dialogLanguage": "zh-CN",
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	stepRepo := memory.NewStepRepository()
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    evidenceStepID,
		RunID: runID,
		Kind:  "ssh/command",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"command":  "lscpu && free -h && df -h && docker --version",
			"exitCode": 0,
			"stdout": strings.Join([]string{
				"Model name:                              Intel(R) Core(TM) i7-10750H CPU @ 2.60GHz",
				"CPU(s):                                  12",
				"Mem:            15Gi       1.5Gi        11Gi       1.8Mi       3.0Gi        14Gi",
				"/dev/mapper/ubuntu--vg-ubuntu--lv  466G   23G  419G   6% /",
				"Docker version 29.2.1, build a5c7197",
			}, "\n"),
		},
	}); err != nil {
		t.Fatalf("seed evidence step: %v", err)
	}
	assessment := map[string]any{
		"satisfied": true,
		"items": []map[string]any{
			{
				"item":         "输出包含服务器配置",
				"state":        "satisfied",
				"evidenceRefs": []string{string(evidenceStepID)},
			},
		},
		"summary":               "目标已完成，成功查询到服务配置信息（CPU、内存、磁盘、Docker版本）。",
		"goalOutcome":           "已成功查询远程服务器配置信息。",
		"implementationSummary": "通过 SSH 获取 CPU、内存、磁盘和 Docker 版本。",
		"verificationSummary":   "命令执行成功，exit code 0。",
	}
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			Results:   memory.NewResultRepository(),
			Artifacts: memory.NewArtifactRepository(),
		},
	}

	if err := orch.recordRunResultFromValidatedDoDAssessment(ctx, run, step, assessment, "目标已完成，成功查询到服务配置信息（CPU、内存、磁盘、Docker版本）。", nil); err != nil {
		t.Fatalf("record result from validated assessment: %v", err)
	}
	artifacts, err := orch.evidence.Artifacts.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	var summaryArtifact *resultmodel.Artifact
	for _, artifact := range artifacts {
		if artifact.ID == common.ArtifactID(string(runID)+"/artifact/result-summary") {
			summaryArtifact = artifact
			break
		}
	}
	if summaryArtifact == nil {
		t.Fatalf("artifacts = %+v, want result summary artifact", artifacts)
	}
	for _, want := range []string{
		"## 证据输出",
		"Intel(R) Core(TM) i7-10750H CPU @ 2.60GHz",
		"Mem:            15Gi",
		"/dev/mapper/ubuntu--vg-ubuntu--lv  466G",
		"Docker version 29.2.1",
	} {
		if !strings.Contains(summaryArtifact.Summary, want) {
			t.Fatalf("summary artifact missing %q:\n%s", want, summaryArtifact.Summary)
		}
	}
}

func TestRuntimeOrchestrator_ResultArtifactsIncludePatchDiff(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-result-patch-diff")
	workspaceID := common.WorkspaceID("ws-result-patch-diff")
	stepID := common.StepID(string(runID) + "/validation")
	patchID := common.PatchID(string(runID) + "/step-1/patch/1")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-result-patch-diff",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	step := &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	patchRepo := memory.NewPatchRepository()
	if err := patchRepo.Create(ctx, &runmodel.Patch{
		ID:          patchID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID(string(runID) + "/step-1"),
		PatchType:   "unified_diff",
		TargetRef:   "internal/example.go",
		Diff:        "--- a/internal/example.go\n+++ b/internal/example.go\n@@ -1 +1 @@\n-before\n+after\n",
		Summary:     "update example",
		State:       common.PatchStateApplied,
		CreatedAt:   "2026-05-04T00:00:00Z",
		AppliedAt:   "2026-05-04T00:00:01Z",
	}); err != nil {
		t.Fatalf("seed patch: %v", err)
	}
	assessment := map[string]any{
		"satisfied": true,
		"items": []map[string]any{
			{
				"item":         "代码变更可追溯",
				"state":        "satisfied",
				"evidenceRefs": []string{string(stepID)},
			},
		},
		"summary":               "DoD satisfied with patch diff",
		"implementationSummary": "Updated internal/example.go.",
		"verificationSummary":   "Patch was applied and captured as evidence.",
	}
	artifactRepo := memory.NewArtifactRepository()
	resultRepo := memory.NewResultRepository()
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
			Patches:   patchRepo,
		},
	}

	if err := orch.recordRunResultFromValidatedDoDAssessment(ctx, run, step, assessment, "DoD satisfied with patch diff", nil); err != nil {
		t.Fatalf("record result from validated assessment: %v", err)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want one accepted result", results)
	}
	if !reflect.DeepEqual(results[0].ArtifactRefs, []common.ArtifactID{
		common.ArtifactID(string(runID) + "/artifact/result-summary"),
		common.ArtifactID(string(runID) + "/artifact/deliverable-manifest"),
		common.ArtifactID(string(runID) + "/artifact/code-change-diff"),
	}) {
		t.Fatalf("result artifact refs = %+v, want summary, manifest and patch diff", results[0].ArtifactRefs)
	}
	artifacts, err := artifactRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	var summaryArtifact *resultmodel.Artifact
	var manifestArtifact *resultmodel.Artifact
	var diffArtifact *resultmodel.Artifact
	for _, artifact := range artifacts {
		switch artifact.ID {
		case common.ArtifactID(string(runID) + "/artifact/result-summary"):
			summaryArtifact = artifact
		case common.ArtifactID(string(runID) + "/artifact/deliverable-manifest"):
			manifestArtifact = artifact
		case common.ArtifactID(string(runID) + "/artifact/code-change-diff"):
			diffArtifact = artifact
		}
	}
	if diffArtifact == nil || diffArtifact.ArtifactType != resultcommon.ArtifactTypePatch || diffArtifact.PatchID != patchID {
		t.Fatalf("diff artifact = %+v, want patch artifact linked to patch", diffArtifact)
	}
	for _, want := range []string{"```diff", "--- a/internal/example.go", "+after"} {
		if !strings.Contains(diffArtifact.Summary, want) {
			t.Fatalf("diff artifact missing %q:\n%s", want, diffArtifact.Summary)
		}
	}
	if summaryArtifact == nil || !strings.Contains(summaryArtifact.Summary, string(diffArtifact.ID)) {
		t.Fatalf("summary artifact = %+v, want patch diff artifact ref", summaryArtifact)
	}
	if manifestArtifact == nil || !strings.Contains(manifestArtifact.Summary, "## Code Change Evidence") || !strings.Contains(manifestArtifact.Summary, string(patchID)) {
		t.Fatalf("manifest artifact = %+v, want patch evidence section", manifestArtifact)
	}
}

func TestRuntimeOrchestrator_ResultArtifactsIncludeWebSourceIndex(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-result-web-source-index")
	workspaceID := common.WorkspaceID("ws-result-web-source-index")
	stepID := common.StepID(string(runID) + "/validation")
	searchStepID := common.StepID(string(runID) + "/step-search")
	fetchStepID := common.StepID(string(runID) + "/step-fetch")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-result-web-source-index",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	step := &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	stepRepo := memory.NewStepRepository()
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    searchStepID,
		RunID: runID,
		Kind:  webcommon.StepKindSearch,
		State: common.StepStateCompleted,
		Output: map[string]any{
			"provider":   "default-http",
			"query":      "reference latest",
			"searchedAt": "2026-05-05T00:00:00Z",
			"policy":     map[string]any{"decision": "allowed"},
			"results": []map[string]any{{
				"title":       "Reference Downloads",
				"url":         "https://example.com/reference",
				"snippet":     "Reference data",
				"source":      "example.com",
				"publishedAt": "2026-05-01",
				"rank":        1,
				"contentHash": "search-hash",
			}},
		},
	}); err != nil {
		t.Fatalf("seed search step: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    fetchStepID,
		RunID: runID,
		Kind:  webcommon.StepKindFetch,
		State: common.StepStateCompleted,
		Output: map[string]any{
			"provider":    "default-http",
			"url":         "https://go.dev/dl/",
			"finalUrl":    "https://go.dev/dl/",
			"title":       "Downloads",
			"source":      "go.dev",
			"retrievedAt": "2026-05-05T00:00:01Z",
			"contentHash": "fetch-hash",
			"policy":      map[string]any{"decision": "allowed"},
		},
	}); err != nil {
		t.Fatalf("seed fetch step: %v", err)
	}
	assessment := map[string]any{
		"satisfied": true,
		"items": []map[string]any{
			{
				"item":         "web source is cited",
				"state":        "satisfied",
				"evidenceRefs": []string{string(fetchStepID)},
			},
		},
		"summary":               "DoD satisfied with web source",
		"implementationSummary": "Verified the external source.",
		"verificationSummary":   "Source was fetched and indexed.",
	}
	artifactRepo := memory.NewArtifactRepository()
	resultRepo := memory.NewResultRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
		},
	}

	if err := orch.recordRunResultFromValidatedDoDAssessment(ctx, run, step, assessment, "DoD satisfied with web source", nil); err != nil {
		t.Fatalf("record result from validated assessment: %v", err)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want one accepted result", results)
	}
	if !reflect.DeepEqual(results[0].ArtifactRefs, []common.ArtifactID{
		common.ArtifactID(string(runID) + "/artifact/result-summary"),
		common.ArtifactID(string(runID) + "/artifact/deliverable-manifest"),
		common.ArtifactID(string(runID) + "/artifact/web-source-index"),
	}) {
		t.Fatalf("result artifact refs = %+v, want summary, manifest and web source index", results[0].ArtifactRefs)
	}
	artifacts, err := artifactRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	var sourceArtifact *resultmodel.Artifact
	var summaryArtifact *resultmodel.Artifact
	for _, artifact := range artifacts {
		switch artifact.ID {
		case common.ArtifactID(string(runID) + "/artifact/web-source-index"):
			sourceArtifact = artifact
		case common.ArtifactID(string(runID) + "/artifact/result-summary"):
			summaryArtifact = artifact
		}
	}
	if sourceArtifact == nil ||
		sourceArtifact.Category != resultcommon.ArtifactCategorySource ||
		sourceArtifact.ArtifactType != resultcommon.ArtifactTypeReport ||
		!reflect.DeepEqual(sourceArtifact.EvidenceRefs, []string{string(fetchStepID), string(searchStepID)}) {
		t.Fatalf("source artifact = %+v, want web source report linked to web evidence", sourceArtifact)
	}
	for _, want := range []string{"Web Source Index", "https://go.dev/dl/", string(searchStepID), string(fetchStepID)} {
		if !strings.Contains(sourceArtifact.Summary, want) {
			t.Fatalf("source artifact missing %q:\n%s", want, sourceArtifact.Summary)
		}
	}
	if summaryArtifact == nil || !strings.Contains(summaryArtifact.Summary, string(sourceArtifact.ID)) {
		t.Fatalf("summary artifact = %+v, want web source artifact ref", summaryArtifact)
	}

	memoryRepo := memory.NewMemoryItemRepository()
	orch.evidence.MemoryItems = memoryRepo
	if err := orch.recordRunSummaryMemoryFromValidatedDoDAssessment(ctx, run, step, assessment, "DoD satisfied with web source"); err != nil {
		t.Fatalf("record run summary memory: %v", err)
	}
	memories, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(memories) != 1 ||
		!strings.Contains(memories[0].Content, "Web sources:") ||
		!strings.Contains(memories[0].Content, "https://go.dev/dl/") ||
		!strings.Contains(memories[0].Content, "fetch-hash") {
		t.Fatalf("memory = %+v, want source summary refs", memories)
	}
	if strings.Contains(memories[0].Content, "Download Go") {
		t.Fatalf("memory content = %q, must not store raw search snippet/page body", memories[0].Content)
	}
}

func TestRuntimeOrchestrator_CompletionEvidenceWritesResultAndSkillFeedbackConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	artifactRelease := make(chan struct{})
	memoryRelease := make(chan struct{})
	resultRepo := memory.NewResultRepository()
	artifactRepo := &blockingArtifactRepository{inner: memory.NewArtifactRepository(), started: started, release: artifactRelease}
	memoryRepo := &blockingMemoryItemRepository{inner: memory.NewMemoryItemRepository(), started: started, release: memoryRelease}
	orch := &RuntimeOrchestrator{
		workspaces: memory.NewWorkspaceRepository(),
		evidence: RuntimeEvidenceRepositories{
			Results:     resultRepo,
			Artifacts:   artifactRepo,
			MemoryItems: memoryRepo,
		},
	}

	workspaceID := common.WorkspaceID("ws-completion-evidence-concurrent")
	runID := common.RunID("run-completion-evidence-concurrent")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateRunning}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-completion-evidence-concurrent"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"skillIds": []string{"skill-completion-evidence"},
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	assessment := map[string]any{
		"satisfied": true,
		"items": []map[string]any{
			{
				"item":         "completion evidence writes are concurrent",
				"state":        "satisfied",
				"evidenceRefs": []string{string(step.ID)},
			},
		},
		"summary": "DoD satisfied",
	}

	done := make(chan error, 1)
	go func() {
		done <- orch.recordCompletionEvidenceFromValidatedDoDAssessment(ctx, run, step, assessment, "DoD satisfied", workspace)
	}()

	seen := map[string]bool{}
	for !seen["artifact-create:run-completion-evidence-concurrent/artifact/result-summary"] ||
		!seen["artifact-create:run-completion-evidence-concurrent/artifact/deliverable-manifest"] ||
		!seen["memory-create:run-completion-evidence-concurrent/memory/skill-feedback"] {
		select {
		case name := <-started:
			seen[name] = true
		case err := <-done:
			t.Fatalf("recordCompletionEvidenceFromValidatedDoDAssessment returned before all writes started: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for concurrent completion evidence writes, saw %+v: %v", seen, ctx.Err())
		}
	}
	close(artifactRelease)
	close(memoryRelease)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("recordCompletionEvidenceFromValidatedDoDAssessment: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("recordCompletionEvidenceFromValidatedDoDAssessment did not complete after release: %v", ctx.Err())
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 || len(results[0].ArtifactRefs) != 2 {
		t.Fatalf("results = %+v, want accepted result with both artifacts", results)
	}
	memories, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if !hasSkillFeedbackMemory(memories, runID, "skill-completion-evidence") {
		t.Fatalf("memories = %+v, want skill feedback memory", memories)
	}
}

func TestRuntimeOrchestrator_FinalRunAndWorkspaceCompletionWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	workspaceInner := memory.NewWorkspaceRepository()
	runInner := memory.NewRunRepository()
	workspaceRepo := &blockingWorkspaceRepository{inner: workspaceInner, started: started, updateRelease: release}
	runRepo := &blockingCompletedRunRepository{inner: runInner, started: started, release: release}
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	artifactRepo := memory.NewArtifactRepository()
	hub := events.NewMemoryHub()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "completed",
		"dodAssessment": map[string]any{
			"satisfied": true,
			"items": []map[string]any{
				{
					"item":         "completion writes are concurrent",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-terminal-write-concurrent/step-1/validation/runtime-step"},
				},
			},
			"summary": "DoD satisfied",
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
		},
		hub,
	)

	workspaceID := common.WorkspaceID("ws-terminal-write-concurrent")
	runID := common.RunID("run-terminal-write-concurrent")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-terminal-write-concurrent"),
		State:           common.RunStateCreated,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- orch.StartRun(ctx, runID)
	}()

	seen := map[string]bool{}
	for !seen["run-completed-update"] || !seen["workspace-completed-update"] {
		select {
		case name := <-started:
			seen[name] = true
		case err := <-done:
			t.Fatalf("StartRun returned before both terminal writes started: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for concurrent terminal writes, saw %+v: %v", seen, ctx.Err())
		}
	}
	publishedBeforeRelease := hub.Published()
	for _, item := range publishedBeforeRelease {
		if item.Topic == events.TopicRunStateChanged && item.Payload[events.PayloadKeyState] == string(common.RunStateCompleted) {
			t.Fatalf("published completed run event before terminal writes were released: %+v", publishedBeforeRelease)
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("StartRun did not complete after releasing terminal writes: %v", ctx.Err())
	}
	published := hub.Published()
	var completedRunIndex = -1
	var completedWorkspaceIndex = -1
	for idx, item := range published {
		if item.Topic == events.TopicRunStateChanged && item.Payload[events.PayloadKeyState] == string(common.RunStateCompleted) {
			completedRunIndex = idx
		}
		if item.Topic == events.TopicWorkspaceStateChanged && item.Payload[events.PayloadKeyState] == string(common.WorkspaceStateCompleted) {
			completedWorkspaceIndex = idx
		}
	}
	if completedRunIndex < 0 || completedWorkspaceIndex < 0 || completedRunIndex > completedWorkspaceIndex {
		t.Fatalf("published events = %+v, want run completed before workspace completed", published)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	workspaceItem, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if workspaceItem.State != common.WorkspaceStateCompleted || workspaceItem.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want completed with active run cleared", workspaceItem)
	}
}

func TestRuntimeOrchestrator_ResultArtifactsWriteConcurrentlyBeforeAcceptedResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	artifactRelease := make(chan struct{})
	resultRepo := memory.NewResultRepository()
	artifactRepo := &blockingArtifactRepository{inner: memory.NewArtifactRepository(), started: started, release: artifactRelease}
	orch := &RuntimeOrchestrator{
		workspaces: memory.NewWorkspaceRepository(),
		evidence: RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
		},
	}

	workspaceID := common.WorkspaceID("ws-result-manifest-prefetch")
	runID := common.RunID("run-result-manifest-prefetch")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-result-manifest-prefetch"),
		State:           common.RunStateRunning,
		InitialStepKind: "llm/workspace_execution",
	}
	step := &runmodel.Step{
		ID:    "run-result-manifest-prefetch/step-1",
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyExecutionContextPackage: map[string]any{
				stepInputKeyWorkspaceFileManifest: []map[string]any{
					{"path": "README.md", "size": 32},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"dodAssessment": map[string]any{
			"satisfied": true,
			"items": []map[string]any{
				{
					"item":         "manifest generated while summary artifact is writing",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-result-manifest-prefetch/step-1/validation/runtime-step"},
				},
			},
			"summary": "DoD satisfied",
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- orch.recordRunResultFromDoDAssessment(ctx, run, step, output, workspace)
	}()

	seen := map[string]bool{}
	for !seen["artifact-create:run-result-manifest-prefetch/artifact/result-summary"] ||
		!seen["artifact-create:run-result-manifest-prefetch/artifact/deliverable-manifest"] {
		select {
		case name := <-started:
			seen[name] = true
		case <-ctx.Done():
			t.Fatalf("events before release = %+v, want both artifact writes: %v", seen, ctx.Err())
		}
	}
	close(artifactRelease)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("recordRunResultFromDoDAssessment: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("recordRunResultFromDoDAssessment did not complete after release: %v", ctx.Err())
	}
	artifacts, err := artifactRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %+v, want summary and manifest artifacts", artifacts)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 || results[0].State != common.ResultStateAccepted || len(results[0].ArtifactRefs) != 2 {
		t.Fatalf("results = %+v, want one accepted result with both artifact refs", results)
	}
}

func TestRuntimeOrchestrator_ResumeRunContinuesBlockExecutionGraphAfterApproval(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "prepare", Kind: "graph/mock"},
		{ID: "apply", Kind: "graph/mock", DependsOn: []string{"prepare"}},
	}}
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-graph-approval/block/prepare":
				return map[string]any{
					"status":              "completed",
					"executionGraph":      graph,
					"blockApprovalReason": "requires approval",
				}, nil
			case "run-graph-approval/block/apply":
				return map[string]any{"status": "completed"}, nil
			default:
				return nil, errors.New("unexpected graph step")
			}
		},
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-graph-approval")
	runID := common.RunID("run-graph-approval")
	prepareStepID := common.StepID("run-graph-approval/block/prepare")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    prepareStepID,
		RunID: runID,
		Kind:  "graph/mock",
		State: common.StepStateDispatched,
		Input: map[string]any{
			"executionGraph": graph,
			"graphNodeId":    "prepare",
		},
	}); err != nil {
		t.Fatalf("seed graph start step: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start graph run: %v", err)
	}
	blockedRun, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run: %v", err)
	}
	if blockedRun.State != common.RunStateBlockedOnApproval || blockedRun.BlockingApprovalID == "" {
		t.Fatalf("blocked run = %+v, want approval block", blockedRun)
	}
	approval, err := approvalRepo.Get(ctx, blockedRun.BlockingApprovalID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	approval.State = common.ApprovalStateApproved
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("approve item: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume graph run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	applyStep, err := stepRepo.Get(ctx, "run-graph-approval/block/apply")
	if err != nil {
		t.Fatalf("get apply step: %v", err)
	}
	if applyStep.State != common.StepStateCompleted || applyStep.Input["graphPreviousNodeId"] != "prepare" {
		t.Fatalf("apply step = %+v, want resumed graph node after prepare", applyStep)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchesConditionalBranchStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-branch/step-1":
				return map[string]any{
					"status":   "completed",
					"stage":    "route",
					"branch":   "fast-track",
					"nextStep": "run-branch/step-fast",
					"nextKind": "runtime/test",
				}, nil
			case "run-branch/step-fast":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-branch")
	runID := common.RunID("run-branch")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start branch run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step1, err := stepRepo.Get(ctx, "run-branch/step-1")
	if err != nil {
		t.Fatalf("get step1: %v", err)
	}
	if step1.Output["branch"] != "fast-track" {
		t.Fatalf("step1 branch = %v, want fast-track", step1.Output["branch"])
	}

	stepFast, err := stepRepo.Get(ctx, "run-branch/step-fast")
	if err != nil {
		t.Fatalf("get branch step: %v", err)
	}
	if stepFast.State != common.StepStateCompleted {
		t.Fatalf("branch step state = %s, want completed", stepFast.State)
	}
	if stepFast.Input["previousStepId"] != "run-branch/step-1" {
		t.Fatalf("branch step previousStepId = %v, want run-branch/step-1", stepFast.Input["previousStepId"])
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-branch/step-1" || runtime.calls[1] != "run-branch/step-fast" {
		t.Fatalf("runtime calls = %#v, want [run-branch/step-1 run-branch/step-fast]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_StartRunDispatchesConditionalReviewStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-branch-review/step-1":
				return map[string]any{
					"status":   "completed",
					"stage":    "route",
					"branch":   "review-path",
					"nextStep": "run-branch-review/step-review",
					"nextKind": "runtime/test",
				}, nil
			case "run-branch-review/step-review":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-branch-review")
	runID := common.RunID("run-branch-review")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start branch review run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step1, err := stepRepo.Get(ctx, "run-branch-review/step-1")
	if err != nil {
		t.Fatalf("get step1: %v", err)
	}
	if step1.Output["branch"] != "review-path" {
		t.Fatalf("step1 branch = %v, want review-path", step1.Output["branch"])
	}

	stepReview, err := stepRepo.Get(ctx, "run-branch-review/step-review")
	if err != nil {
		t.Fatalf("get review step: %v", err)
	}
	if stepReview.State != common.StepStateCompleted {
		t.Fatalf("review step state = %s, want completed", stepReview.State)
	}
	if stepReview.Input["previousStepId"] != "run-branch-review/step-1" {
		t.Fatalf("review step previousStepId = %v, want run-branch-review/step-1", stepReview.Input["previousStepId"])
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-branch-review/step-1" || runtime.calls[1] != "run-branch-review/step-review" {
		t.Fatalf("runtime calls = %#v, want [run-branch-review/step-1 run-branch-review/step-review]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_StartRunBlocksOnDecisionWhenRuntimeRequestsDecisionGate(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		output: map[string]any{
			"status":                           "completed",
			stepOutputKeyBlockDecisionReason:   "runtime decision gate",
			stepOutputKeyBlockDecisionTicketID: "ticket-runtime-block",
			stepOutputKeyNextStep:              "run-runtime-block/step-2",
			stepOutputKeyNextKind:              "runtime/test",
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-runtime-block")
	runID := common.RunID("run-runtime-block")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start runtime-decision run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision {
		t.Fatalf("run state = %s, want blocked_on_decision", runItem.State)
	}
	if runItem.BlockingTicketID != "ticket-runtime-block" {
		t.Fatalf("blocking ticket = %s, want ticket-runtime-block", runItem.BlockingTicketID)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-runtime-block")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateOpen {
		t.Fatalf("ticket state = %s, want open", ticket.State)
	}
	if ticket.StepID != "run-runtime-block/step-1" {
		t.Fatalf("ticket step id = %s, want run-runtime-block/step-1", ticket.StepID)
	}

	step1, err := stepRepo.Get(ctx, "run-runtime-block/step-1")
	if err != nil {
		t.Fatalf("get step1: %v", err)
	}
	if step1.State != common.StepStateCompleted {
		t.Fatalf("step1 state = %s, want completed", step1.State)
	}
	if _, err := stepRepo.Get(ctx, "run-runtime-block/step-2"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("step2 err = %v, want not found", err)
	}

	published := hub.Published()
	foundTicketCreated := false
	for _, envelope := range published {
		if envelope.Topic == events.TopicTicketCreated && envelope.SubjectID == "ticket-runtime-block" {
			foundTicketCreated = true
			break
		}
	}
	if !foundTicketCreated {
		t.Fatalf("published events = %#v, want ticket created event", published)
	}
}

func TestRuntimeOrchestrator_ResumeRunContinuesFromNextStepAfterRuntimeDecisionResolved(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-runtime-resume/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "runtime decision gate",
					stepOutputKeyBlockDecisionTicketID: "ticket-runtime-resume",
					stepOutputKeyNextStep:              "run-runtime-resume/step-2",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			case "run-runtime-resume/step-2":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-runtime-resume")
	runID := common.RunID("run-runtime-resume")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start runtime-decision run: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-runtime-resume")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-runtime-resume"),
		TicketID: ticket.ID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step2, err := stepRepo.Get(ctx, "run-runtime-resume/step-2")
	if err != nil {
		t.Fatalf("get step2: %v", err)
	}
	if step2.State != common.StepStateCompleted {
		t.Fatalf("step2 state = %s, want completed", step2.State)
	}
	if step2.Input["previousStepId"] != "run-runtime-resume/step-1" {
		t.Fatalf("step2 previousStepId = %v, want run-runtime-resume/step-1", step2.Input["previousStepId"])
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-runtime-resume/step-1" || runtime.calls[1] != "run-runtime-resume/step-2" {
		t.Fatalf("runtime calls = %#v, want [run-runtime-resume/step-1 run-runtime-resume/step-2]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunResolvedDecisionUsesRunScopedTicketRead(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := newCountingDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if !strings.Contains(string(request.StepID), "zz-llm-repair-") {
				return nil, fmt.Errorf("unexpected step %s", request.StepID)
			}
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "repair completed after operator guidance",
					"items": []map[string]any{
						{
							"item":         "repair completed",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						},
					},
				},
			}, nil
		},
	}
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-runtime-ticket-scope")
	runID := common.RunID("run-runtime-ticket-scope")
	failedStepID := common.StepID("run-runtime-ticket-scope/block/write_config")
	ticketID := common.TicketID("ticket-runtime-ticket-scope")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          common.GoalID("goal-runtime-ticket-scope"),
		DoDID:           common.DoDID("dod-runtime-ticket-scope"),
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		State:           common.RunStateRunning,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{"targetRef": "llm-proxy/config.yaml"},
		Output: map[string]any{
			"error": "workspace patch path conflict",
		},
	}); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      failedStepID,
		Reason:      "operator guidance required",
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          common.TicketID("ticket-other-run"),
		WorkspaceID: workspaceID,
		RunID:       common.RunID("run-other"),
		StepID:      common.StepID("run-other/step-1"),
		Reason:      "other run ticket",
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed unrelated ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-runtime-ticket-scope"),
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if got := ticketRepo.listCount.Load(); got != 0 {
		t.Fatalf("ticket List count = %d, want no full ticket scan", got)
	}
	if got := ticketRepo.listByRunIDCount.Load(); got < 1 {
		t.Fatalf("ticket ListByRunID count = %d, want at least one run scoped ticket read", got)
	}
}

func TestRuntimeOrchestrator_ResumeRunContinuesFromFailedStepAfterRepairDecisionResolved(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	var capturedInput map[string]any
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if !strings.Contains(string(request.StepID), "zz-llm-repair-") {
				return nil, fmt.Errorf("unexpected step %s", request.StepID)
			}
			capturedInput = request.Input
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "repair completed after operator guidance",
					"items": []map[string]any{
						{
							"item":         "repair completed",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						},
					},
				},
			}, nil
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-repair-decision-resume")
	runID := common.RunID("run-repair-decision-resume")
	failedStepID := common.StepID("run-repair-decision-resume/block/write_config")
	ticketID := common.TicketID("ticket-repair-decision-resume")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		GoalID:           common.GoalID("goal-repair-decision-resume"),
		DoDID:            common.DoDID("dod-repair-decision-resume"),
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{"targetRef": "llm-proxy/config.yaml"},
		Output: map[string]any{
			"error": "workspace patch path conflict",
			"runtimeDiagnostics": map[string]any{
				"kind":        "workspace_path_conflict",
				"targetRef":   "llm-proxy/config.yaml",
				"conflictRef": "llm-proxy",
			},
		},
	}); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      failedStepID,
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-repair-decision-resume"),
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"resumeInstruction": "continue from the root project"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	if capturedInput == nil {
		t.Fatalf("captured input is nil")
	}
	if capturedInput["failedStepId"] != string(failedStepID) {
		t.Fatalf("failedStepId = %v, want %s", capturedInput["failedStepId"], failedStepID)
	}
	if _, ok := capturedInput["decisionResolution"].(map[string]any); !ok {
		t.Fatalf("decisionResolution missing from resume input: %#v", capturedInput)
	}
}

func TestRuntimeOrchestrator_ResumeRunRecoversRunningRunWithResolvedFailedStepDecision(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if request.Input["decisionResolution"] == nil {
				return nil, errors.New("decision resolution input is required")
			}
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "stale running run recovered",
					"items": []map[string]any{
						{
							"item":         "stale running run recovered",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						},
					},
				},
			}, nil
		},
	}
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-running-recovery")
	runID := common.RunID("run-running-recovery")
	failedStepID := common.StepID("run-running-recovery/block/write_config")
	ticketID := common.TicketID("ticket-running-recovery")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          common.GoalID("goal-running-recovery"),
		DoDID:           common.DoDID("dod-running-recovery"),
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		State:           common.RunStateRunning,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     failedStepID,
		RunID:  runID,
		Kind:   "patch/apply",
		State:  common.StepStateFailed,
		Output: map[string]any{"error": "workspace patch path conflict"},
	}); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      failedStepID,
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-running-recovery"),
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"resumeInstruction": "continue from existing files"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume running run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
}

func TestRuntimeOrchestrator_ResumeRunCreatesUniqueStepForMultiRoundDecision(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runID := common.RunID("run-runtime-multi-decision")
	firstResumeStepID := common.StepID("run-runtime-multi-decision/llm-after-decision")
	secondResumeStepID := common.StepID("run-runtime-multi-decision/llm-after-decision-2")
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			switch request.StepID {
			case "run-runtime-multi-decision/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "first decision gate",
					stepOutputKeyBlockDecisionTicketID: "ticket-runtime-multi-decision-first",
					stepOutputKeyNextStep:              string(firstResumeStepID),
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			case firstResumeStepID:
				resolution := mapFromValue(request.Input["decisionResolution"])
				payload := mapFromValue(resolution["resolutionPayload"])
				if payload["answer"] != "first answer is incomplete" {
					return nil, fmt.Errorf("first resume payload = %+v, want first answer", payload)
				}
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockDecisionReason: "second decision gate",
					stepOutputKeyNextStep:            string(firstResumeStepID),
					stepOutputKeyNextKind:            "runtime/test",
				}, nil
			case secondResumeStepID:
				resolution := mapFromValue(request.Input["decisionResolution"])
				payload := mapFromValue(resolution["resolutionPayload"])
				if payload["answer"] != "second answer is enough" {
					return nil, fmt.Errorf("second resume payload = %+v, want second answer", payload)
				}
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, fmt.Errorf("unexpected step %s", request.StepID)
			}
		},
	}
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-runtime-multi-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	resolveRuntimeTicket(t, ctx, ticketRepo, resolutionRepo, "ticket-runtime-multi-decision-first", "resolution-runtime-multi-decision-first", map[string]any{"answer": "first answer is incomplete"})
	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("first resume run: %v", err)
	}
	secondTicketID := common.TicketID("run-runtime-multi-decision-llm-after-decision-decision")
	resolveRuntimeTicket(t, ctx, ticketRepo, resolutionRepo, secondTicketID, "resolution-runtime-multi-decision-second", map[string]any{"answer": "second answer is enough"})
	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("second resume run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	if _, err := stepRepo.Get(ctx, firstResumeStepID); err != nil {
		t.Fatalf("get first resume step: %v", err)
	}
	secondStep, err := stepRepo.Get(ctx, secondResumeStepID)
	if err != nil {
		t.Fatalf("get second resume step: %v", err)
	}
	if secondStep.Input["previousStepId"] != string(firstResumeStepID) {
		t.Fatalf("second previousStepId = %v, want %s", secondStep.Input["previousStepId"], firstResumeStepID)
	}
	if len(runtime.calls) != 3 ||
		runtime.calls[0] != "run-runtime-multi-decision/step-1" ||
		runtime.calls[1] != firstResumeStepID ||
		runtime.calls[2] != secondResumeStepID {
		t.Fatalf("runtime calls = %#v, want unique multi-round decision calls", runtime.calls)
	}
}

func resolveRuntimeTicket(
	t *testing.T,
	ctx context.Context,
	ticketRepo *memory.DecisionTicketRepository,
	resolutionRepo *memory.DecisionResolutionRepository,
	ticketID common.TicketID,
	resolutionID common.ResolutionID,
	payload map[string]any,
) {
	t.Helper()
	ticket, err := ticketRepo.Get(ctx, ticketID)
	if err != nil {
		t.Fatalf("get ticket %s: %v", ticketID, err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket %s: %v", ticketID, err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       resolutionID,
		TicketID: ticket.ID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  payload,
	}); err != nil {
		t.Fatalf("create resolution %s: %v", resolutionID, err)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectsRuntimeDecisionWithoutDispatchingNextStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-runtime-decision-reject/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "runtime decision gate",
					stepOutputKeyBlockDecisionTicketID: "ticket-runtime-decision-reject",
					stepOutputKeyNextStep:              "run-runtime-decision-reject/step-2",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-runtime-decision-reject")
	runID := common.RunID("run-runtime-decision-reject")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start runtime-decision reject run: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-runtime-decision-reject")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-runtime-decision-reject"),
		TicketID: ticket.ID,
		Type:     "reject_current_path",
		Payload:  map[string]any{"operator": "tester", "outcome": "reject"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume rejected runtime decision: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	if runItem.BlockingTicketID != "" {
		t.Fatalf("blocking ticket = %q, want empty", runItem.BlockingTicketID)
	}
	if _, err := stepRepo.Get(ctx, "run-runtime-decision-reject/step-2"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no dispatched step-2, got %v", err)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "run-runtime-decision-reject/step-1" {
		t.Fatalf("runtime calls = %#v, want [run-runtime-decision-reject/step-1]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunContinuesFromNextStepAfterRuntimeApprovalApproved(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-runtime-approval/step-1":
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockApprovalReason: "risky operation requires approval",
					stepOutputKeyBlockApprovalID:     "approval-runtime-approval",
					stepOutputKeyBlockApprovalAction: "remove public API",
					stepOutputKeyBlockApprovalContext: map[string]any{
						"risk":       "breaking_change",
						"targetFile": "internal/api.go",
					},
					stepOutputKeyNextStep: "run-runtime-approval/step-2",
					stepOutputKeyNextKind: "runtime/test",
				}, nil
			case "run-runtime-approval/step-2":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-runtime-approval")
	runID := common.RunID("run-runtime-approval")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start runtime-approval run: %v", err)
	}

	approval, err := approvalRepo.Get(ctx, "approval-runtime-approval")
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if approval.Reason != "risky operation requires approval" ||
		approval.Action != "remove public API" ||
		approval.Context["risk"] != "breaking_change" {
		t.Fatalf("approval = %+v, want persisted risk operation metadata", approval)
	}
	if _, err := stepRepo.Get(ctx, "run-runtime-approval/step-2"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("step2 before approval err = %v, want not found", err)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "run-runtime-approval/step-1" {
		t.Fatalf("runtime calls before approval = %#v, want only step-1", runtime.calls)
	}
	approval.State = common.ApprovalStateApproved
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step2, err := stepRepo.Get(ctx, "run-runtime-approval/step-2")
	if err != nil {
		t.Fatalf("get step2: %v", err)
	}
	if step2.State != common.StepStateCompleted {
		t.Fatalf("step2 state = %s, want completed", step2.State)
	}
	if step2.Input["previousStepId"] != "run-runtime-approval/step-1" {
		t.Fatalf("step2 previousStepId = %v, want run-runtime-approval/step-1", step2.Input["previousStepId"])
	}
	approvalResolution, ok := step2.Input["approvalResolution"].(map[string]any)
	if !ok {
		t.Fatalf("step2 approvalResolution = %#v, want map", step2.Input["approvalResolution"])
	}
	if approvalResolution["action"] != "remove public API" {
		t.Fatalf("approvalResolution action = %v, want remove public API", approvalResolution["action"])
	}
	approvalContext, ok := approvalResolution["context"].(map[string]any)
	if !ok || approvalContext["risk"] != "breaking_change" || approvalContext["targetFile"] != "internal/api.go" {
		t.Fatalf("approvalResolution context = %#v, want risk metadata", approvalResolution["context"])
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-runtime-approval/step-1" || runtime.calls[1] != "run-runtime-approval/step-2" {
		t.Fatalf("runtime calls = %#v, want [run-runtime-approval/step-1 run-runtime-approval/step-2]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectsRuntimeApprovalWithoutDispatchingNextStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-runtime-approval-reject/step-1":
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockApprovalReason: "risky operation requires approval",
					stepOutputKeyBlockApprovalID:     "approval-runtime-approval-reject",
					stepOutputKeyBlockApprovalAction: "delete production data",
					stepOutputKeyBlockApprovalContext: map[string]any{
						"risk": "data_loss",
					},
					stepOutputKeyNextStep: "run-runtime-approval-reject/step-2",
					stepOutputKeyNextKind: "runtime/test",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-runtime-approval-reject")
	runID := common.RunID("run-runtime-approval-reject")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start runtime-approval reject run: %v", err)
	}

	approval, err := approvalRepo.Get(ctx, "approval-runtime-approval-reject")
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if approval.Action != "delete production data" || approval.Context["risk"] != "data_loss" {
		t.Fatalf("approval = %+v, want persisted risky action metadata", approval)
	}
	approval.State = common.ApprovalStateRejected
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume rejected runtime approval: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	if runItem.BlockingApprovalID != "" {
		t.Fatalf("blocking approval = %q, want empty", runItem.BlockingApprovalID)
	}
	if _, err := stepRepo.Get(ctx, "run-runtime-approval-reject/step-2"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no dispatched step-2, got %v", err)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "run-runtime-approval-reject/step-1" {
		t.Fatalf("runtime calls = %#v, want [run-runtime-approval-reject/step-1]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunSupportsMultipleDecisionGates(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-multi-gate/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "gate 1",
					stepOutputKeyBlockDecisionTicketID: "ticket-multi-gate-1",
					stepOutputKeyNextStep:              "run-multi-gate/step-2",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			case "run-multi-gate/step-2":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "gate 2",
					stepOutputKeyBlockDecisionTicketID: "ticket-multi-gate-2",
					stepOutputKeyNextStep:              "run-multi-gate/step-3",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			case "run-multi-gate/step-3":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-multi-gate")
	runID := common.RunID("run-multi-gate")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	resolveTicket := func(ticketID common.TicketID, resolutionID common.ResolutionID) {
		t.Helper()
		ticket, err := ticketRepo.Get(ctx, ticketID)
		if err != nil {
			t.Fatalf("get ticket %s: %v", ticketID, err)
		}
		ticket.State = common.DecisionTicketStateResolved
		if err := ticketRepo.Update(ctx, ticket); err != nil {
			t.Fatalf("update ticket %s: %v", ticketID, err)
		}
		if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
			ID:       resolutionID,
			TicketID: ticketID,
			Type:     "provide_missing_info",
			Payload:  map[string]any{"operator": "tester"},
		}); err != nil {
			t.Fatalf("create resolution for %s: %v", ticketID, err)
		}
	}

	resolveTicket("ticket-multi-gate-1", "resolution-multi-gate-1")
	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after first gate: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run after first resume: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID != "ticket-multi-gate-2" {
		t.Fatalf("run after first resume = %+v, want blocked on second gate", runItem)
	}

	resolveTicket("ticket-multi-gate-2", "resolution-multi-gate-2")
	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after second gate: %v", err)
	}

	runItem, err = runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get final run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step3, err := stepRepo.Get(ctx, "run-multi-gate/step-3")
	if err != nil {
		t.Fatalf("get step3: %v", err)
	}
	if step3.State != common.StepStateCompleted {
		t.Fatalf("step3 state = %s, want completed", step3.State)
	}
	if step3.Input["previousStepId"] != "run-multi-gate/step-2" {
		t.Fatalf("step3 previousStepId = %v, want run-multi-gate/step-2", step3.Input["previousStepId"])
	}
	if len(runtime.calls) != 3 || runtime.calls[0] != "run-multi-gate/step-1" || runtime.calls[1] != "run-multi-gate/step-2" || runtime.calls[2] != "run-multi-gate/step-3" {
		t.Fatalf("runtime calls = %#v, want [run-multi-gate/step-1 run-multi-gate/step-2 run-multi-gate/step-3]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectsSecondDecisionGateWithoutDispatchingNextStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-multi-gate-reject/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "gate 1",
					stepOutputKeyBlockDecisionTicketID: "ticket-multi-gate-reject-1",
					stepOutputKeyNextStep:              "run-multi-gate-reject/step-2",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			case "run-multi-gate-reject/step-2":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "gate 2",
					stepOutputKeyBlockDecisionTicketID: "ticket-multi-gate-reject-2",
					stepOutputKeyNextStep:              "run-multi-gate-reject/step-3",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-multi-gate-reject")
	runID := common.RunID("run-multi-gate-reject")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	ticket1, err := ticketRepo.Get(ctx, "ticket-multi-gate-reject-1")
	if err != nil {
		t.Fatalf("get ticket1: %v", err)
	}
	ticket1.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket1); err != nil {
		t.Fatalf("update ticket1: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-multi-gate-reject-1"),
		TicketID: ticket1.ID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution1: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after first gate: %v", err)
	}

	ticket2, err := ticketRepo.Get(ctx, "ticket-multi-gate-reject-2")
	if err != nil {
		t.Fatalf("get ticket2: %v", err)
	}
	ticket2.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket2); err != nil {
		t.Fatalf("update ticket2: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-multi-gate-reject-2"),
		TicketID: ticket2.ID,
		Type:     "reject_current_path",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution2: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume rejected second gate: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get final run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	if runItem.BlockingTicketID != "" || runItem.BlockingApprovalID != "" {
		t.Fatalf("blocking refs = ticket=%q approval=%q, want empty", runItem.BlockingTicketID, runItem.BlockingApprovalID)
	}
	if _, err := stepRepo.Get(ctx, "run-multi-gate-reject/step-3"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no dispatched step-3, got %v", err)
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-multi-gate-reject/step-1" || runtime.calls[1] != "run-multi-gate-reject/step-2" {
		t.Fatalf("runtime calls = %#v, want [run-multi-gate-reject/step-1 run-multi-gate-reject/step-2]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunSupportsDecisionThenApprovalGates(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-mixed-gate/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "mixed decision",
					stepOutputKeyBlockDecisionTicketID: "ticket-mixed-gate-1",
					stepOutputKeyNextStep:              "run-mixed-gate/step-2",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			case "run-mixed-gate/step-2":
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockApprovalReason: "mixed approval",
					stepOutputKeyBlockApprovalID:     "approval-mixed-gate-1",
					stepOutputKeyNextStep:            "run-mixed-gate/step-3",
					stepOutputKeyNextKind:            "runtime/test",
				}, nil
			case "run-mixed-gate/step-3":
				return map[string]any{
					"status": "completed",
					"stage":  "finalize",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-mixed-gate")
	runID := common.RunID("run-mixed-gate")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-mixed-gate-1")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-mixed-gate-1"),
		TicketID: ticket.ID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after decision: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run after decision: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnApproval || runItem.BlockingApprovalID != "approval-mixed-gate-1" {
		t.Fatalf("run after decision resume = %+v, want blocked on approval", runItem)
	}

	approval, err := approvalRepo.Get(ctx, "approval-mixed-gate-1")
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	approval.State = common.ApprovalStateApproved
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after approval: %v", err)
	}

	runItem, err = runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get final run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}

	step3, err := stepRepo.Get(ctx, "run-mixed-gate/step-3")
	if err != nil {
		t.Fatalf("get step3: %v", err)
	}
	if step3.State != common.StepStateCompleted {
		t.Fatalf("step3 state = %s, want completed", step3.State)
	}
	if step3.Input["previousStepId"] != "run-mixed-gate/step-2" {
		t.Fatalf("step3 previousStepId = %v, want run-mixed-gate/step-2", step3.Input["previousStepId"])
	}
	if len(runtime.calls) != 3 || runtime.calls[0] != "run-mixed-gate/step-1" || runtime.calls[1] != "run-mixed-gate/step-2" || runtime.calls[2] != "run-mixed-gate/step-3" {
		t.Fatalf("runtime calls = %#v, want [run-mixed-gate/step-1 run-mixed-gate/step-2 run-mixed-gate/step-3]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunSupportsLLMNamedDecisionThenApprovalGates(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-llm-mixed/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "llm decision",
					stepOutputKeyBlockDecisionTicketID: "ticket-llm-mixed-1",
					stepOutputKeyNextStep:              "run-llm-mixed/llm-after-decision",
					stepOutputKeyNextKind:              "llm/workspace-plan",
				}, nil
			case "run-llm-mixed/llm-after-decision":
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockApprovalReason: "llm approval",
					stepOutputKeyBlockApprovalID:     "approval-llm-mixed-1",
					stepOutputKeyNextStep:            "run-llm-mixed/llm-after-approval",
					stepOutputKeyNextKind:            "llm/workspace-plan",
				}, nil
			case "run-llm-mixed/llm-after-approval":
				return map[string]any{
					"status": "completed",
					"dodAssessment": map[string]any{
						"satisfied": true,
						"summary":   "multi gate completed",
						"items": []map[string]any{{
							"item":         "multi gate completed",
							"state":        "satisfied",
							"evidenceRefs": []string{"run-llm-mixed/llm-after-approval"},
						}},
					},
				}, nil
			default:
				return nil, fmt.Errorf("unexpected step %s", stepID)
			}
		},
	}
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-llm-mixed")
	runID := common.RunID("run-llm-mixed")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated, DoDID: "dod-llm-mixed"}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-llm-mixed-1")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-llm-mixed-1"),
		TicketID: ticket.ID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after decision: %v", err)
	}
	afterDecision, err := stepRepo.Get(ctx, "run-llm-mixed/llm-after-decision")
	if err != nil {
		t.Fatalf("get after decision step: %v", err)
	}
	if afterDecision.Input[stepInputKeySoulStage] != soulStageDecision {
		t.Fatalf("after decision soulStage = %v, want decision", afterDecision.Input[stepInputKeySoulStage])
	}
	decisionResolution, ok := afterDecision.Input["decisionResolution"].(map[string]any)
	if !ok {
		t.Fatalf("decisionResolution input = %#v, want map", afterDecision.Input["decisionResolution"])
	}
	if decisionResolution["ticketId"] != string(ticket.ID) || decisionResolution["resolutionType"] != "provide_missing_info" {
		t.Fatalf("decisionResolution = %#v, want ticket and provide_missing_info", decisionResolution)
	}
	resolutionPayload, ok := decisionResolution["resolutionPayload"].(map[string]any)
	if !ok || resolutionPayload["operator"] != "tester" {
		t.Fatalf("decisionResolution payload = %#v, want operator tester", decisionResolution["resolutionPayload"])
	}

	approval, err := approvalRepo.Get(ctx, "approval-llm-mixed-1")
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	approval.State = common.ApprovalStateApproved
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after approval: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get final run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	afterApproval, err := stepRepo.Get(ctx, "run-llm-mixed/llm-after-approval")
	if err != nil {
		t.Fatalf("get after approval step: %v", err)
	}
	if afterApproval.Input["previousStepId"] != "run-llm-mixed/llm-after-decision" {
		t.Fatalf("after approval previousStepId = %v, want run-llm-mixed/llm-after-decision", afterApproval.Input["previousStepId"])
	}
	if afterApproval.Input[stepInputKeySoulStage] != soulStageApproval {
		t.Fatalf("after approval soulStage = %v, want approval", afterApproval.Input[stepInputKeySoulStage])
	}
	approvalResolution, ok := afterApproval.Input["approvalResolution"].(map[string]any)
	if !ok {
		t.Fatalf("approvalResolution input = %#v, want map", afterApproval.Input["approvalResolution"])
	}
	if approvalResolution["approvalId"] != string(approval.ID) || approvalResolution["state"] != string(common.ApprovalStateApproved) {
		t.Fatalf("approvalResolution = %#v, want approved approval", approvalResolution)
	}
	if len(runtime.calls) != 3 ||
		runtime.calls[0] != "run-llm-mixed/step-1" ||
		runtime.calls[1] != "run-llm-mixed/llm-after-decision" ||
		runtime.calls[2] != "run-llm-mixed/llm-after-approval" {
		t.Fatalf("runtime calls = %#v, want llm named decision then approval flow", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunSupportsDecisionThenRejectedApproval(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		fn: func(stepID common.StepID) (map[string]any, error) {
			switch stepID {
			case "run-mixed-gate-reject/step-1":
				return map[string]any{
					"status":                           "completed",
					stepOutputKeyBlockDecisionReason:   "mixed decision",
					stepOutputKeyBlockDecisionTicketID: "ticket-mixed-gate-reject-1",
					stepOutputKeyNextStep:              "run-mixed-gate-reject/step-2",
					stepOutputKeyNextKind:              "runtime/test",
				}, nil
			case "run-mixed-gate-reject/step-2":
				return map[string]any{
					"status":                         "completed",
					stepOutputKeyBlockApprovalReason: "mixed approval",
					stepOutputKeyBlockApprovalID:     "approval-mixed-gate-reject-1",
					stepOutputKeyNextStep:            "run-mixed-gate-reject/step-3",
					stepOutputKeyNextKind:            "runtime/test",
				}, nil
			default:
				return nil, errors.New("unexpected step")
			}
		},
	}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-mixed-gate-reject")
	runID := common.RunID("run-mixed-gate-reject")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	ticket, err := ticketRepo.Get(ctx, "ticket-mixed-gate-reject-1")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	ticket.State = common.DecisionTicketStateResolved
	if err := ticketRepo.Update(ctx, ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-mixed-gate-reject-1"),
		TicketID: ticket.ID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after decision: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get blocked run after decision: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnApproval || runItem.BlockingApprovalID != "approval-mixed-gate-reject-1" {
		t.Fatalf("run after decision resume = %+v, want blocked on approval", runItem)
	}

	approval, err := approvalRepo.Get(ctx, "approval-mixed-gate-reject-1")
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	approval.State = common.ApprovalStateRejected
	if err := approvalRepo.Update(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after rejected approval: %v", err)
	}

	runItem, err = runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get final run: %v", err)
	}
	if runItem.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", runItem.State)
	}
	if runItem.BlockingTicketID != "" || runItem.BlockingApprovalID != "" {
		t.Fatalf("blocking refs = ticket=%q approval=%q, want empty", runItem.BlockingTicketID, runItem.BlockingApprovalID)
	}

	step2, err := stepRepo.Get(ctx, "run-mixed-gate-reject/step-2")
	if err != nil {
		t.Fatalf("get step2: %v", err)
	}
	if step2.State != common.StepStateCompleted {
		t.Fatalf("step2 state = %s, want completed", step2.State)
	}
	if step2.Input["previousStepId"] != "run-mixed-gate-reject/step-1" {
		t.Fatalf("step2 previousStepId = %v, want run-mixed-gate-reject/step-1", step2.Input["previousStepId"])
	}
	if _, err := stepRepo.Get(ctx, "run-mixed-gate-reject/step-3"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no dispatched step-3, got %v", err)
	}
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-mixed-gate-reject/step-1" || runtime.calls[1] != "run-mixed-gate-reject/step-2" {
		t.Fatalf("runtime calls = %#v, want [run-mixed-gate-reject/step-1 run-mixed-gate-reject/step-2]", runtime.calls)
	}
}

func TestRuntimeOrchestrator_RuntimeFailurePublishesWorkspaceReleaseEvent(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{err: errors.New("runtime exploded")}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-fail-evt")
	runID := common.RunID("run-fail-evt")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	published := hub.Published()
	if len(published) < 5 {
		t.Fatalf("published events = %d, want at least 5", len(published))
	}
	last := published[len(published)-1]
	if last.Topic != events.TopicWorkspaceStateChanged || last.Payload["activeRunId"] != "" {
		t.Fatalf("last event = %#v, want workspace release event", last)
	}
}

func TestRuntimeOrchestrator_StartRunRejectsDifferentActiveRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-active-conflict")
	runID := common.RunID("run-active-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateActive,
		ActiveRunID: common.RunID("other-run"),
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("start run err = %v, want conflict", err)
	}
}

func TestRuntimeOrchestrator_StartRunRejectsNonCreatedState(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-start-state-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name  string
		state common.RunState
	}{
		{name: "running", state: common.RunStateRunning},
		{name: "blocked decision", state: common.RunStateBlockedOnDecision},
		{name: "blocked approval", state: common.RunStateBlockedOnApproval},
		{name: "completed", state: common.RunStateCompleted},
		{name: "failed", state: common.RunStateFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runID := common.RunID("run-start-state-conflict-" + tc.name)
			if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: tc.state}); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := orch.StartRun(ctx, runID); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("start run err = %v, want conflict", err)
			}
		})
	}
}

func TestRuntimeOrchestrator_StartRun_RuntimeFailureMarksRunAndStepFailed(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{err: errors.New("runtime exploded")}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run should close failure state in-repo, got err: %v", err)
	}

	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateFailed {
		t.Fatalf("run state = %q, want failed", r.State)
	}

	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.ActiveRunID != "" {
		t.Fatalf("workspace active run id = %q, want empty", ws.ActiveRunID)
	}

	step, err := stepRepo.Get(ctx, common.StepID(string(runID)+"/step-1"))
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed {
		t.Fatalf("step state = %q, want failed", step.State)
	}
	if step.Output["reason"] != "runtime exploded" {
		t.Fatalf("step output reason = %v, want runtime exploded", step.Output["reason"])
	}
}

func TestRuntimeOrchestrator_DispatchNextStep_ValidateArguments(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()

	orch := NewRuntimeOrchestrator(nil, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)
	if err := orch.DispatchNextStep(ctx, ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty runID err = %v, want invalid argument", err)
	}

	runID := common.RunID("run-no-runtime")
	workspaceID := common.WorkspaceID("ws-no-runtime")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := orch.DispatchNextStep(ctx, runID); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("nil runtime err = %v, want invalid argument", err)
	}
}

func TestRuntimeOrchestrator_BlockCommands_ValidateArgumentsAndSetExpectedRefs(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	if err := orch.BlockOnDecision(ctx, "", common.TicketID("ticket-1")); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on decision empty run err = %v, want invalid argument", err)
	}
	if err := orch.BlockOnDecision(ctx, common.RunID("run-1"), ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on decision empty ticket err = %v, want invalid argument", err)
	}
	if err := orch.BlockOnApproval(ctx, "", common.ApprovalID("approval-1")); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on approval empty run err = %v, want invalid argument", err)
	}
	if err := orch.BlockOnApproval(ctx, common.RunID("run-1"), ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("block on approval empty approval err = %v, want invalid argument", err)
	}

	decisionWorkspaceID := common.WorkspaceID("ws-block-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: decisionWorkspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	decisionRunID := common.RunID("run-block-decision")
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          decisionRunID,
		WorkspaceID: decisionWorkspaceID,
		State:       common.RunStateCreated,
	}); err != nil {
		t.Fatalf("seed decision run: %v", err)
	}
	if err := orch.BlockOnDecision(ctx, decisionRunID, common.TicketID("ticket-1")); err != nil {
		t.Fatalf("block on decision: %v", err)
	}
	blockedDecision, err := runRepo.Get(ctx, decisionRunID)
	if err != nil {
		t.Fatalf("get decision-blocked run: %v", err)
	}
	if blockedDecision.BlockingTicketID != common.TicketID("ticket-1") || blockedDecision.BlockingApprovalID != "" {
		t.Fatalf("unexpected decision block refs: %+v", blockedDecision)
	}
	decisionWorkspace, err := workspaceRepo.Get(ctx, decisionWorkspaceID)
	if err != nil {
		t.Fatalf("get decision-blocked workspace: %v", err)
	}
	if decisionWorkspace.State != common.WorkspaceStateWaitingDecision || decisionWorkspace.ActiveRunID != decisionRunID {
		t.Fatalf("decision workspace = %+v, want waiting_decision with active run", decisionWorkspace)
	}

	approvalWorkspaceID := common.WorkspaceID("ws-block-approval")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: approvalWorkspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed approval workspace: %v", err)
	}
	approvalRunID := common.RunID("run-block-approval")
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          approvalRunID,
		WorkspaceID: approvalWorkspaceID,
		State:       common.RunStateRunning,
	}); err != nil {
		t.Fatalf("seed approval run: %v", err)
	}
	if err := orch.BlockOnApproval(ctx, approvalRunID, common.ApprovalID("approval-1")); err != nil {
		t.Fatalf("block on approval: %v", err)
	}
	blockedApproval, err := runRepo.Get(ctx, approvalRunID)
	if err != nil {
		t.Fatalf("get approval-blocked run: %v", err)
	}
	if blockedApproval.BlockingApprovalID != common.ApprovalID("approval-1") || blockedApproval.BlockingTicketID != "" {
		t.Fatalf("unexpected approval block refs: %+v", blockedApproval)
	}
	approvalWorkspace, err := workspaceRepo.Get(ctx, approvalWorkspaceID)
	if err != nil {
		t.Fatalf("get approval-blocked workspace: %v", err)
	}
	if approvalWorkspace.State != common.WorkspaceStateBlocked || approvalWorkspace.ActiveRunID != approvalRunID {
		t.Fatalf("approval workspace = %+v, want blocked with active run", approvalWorkspace)
	}
}

func TestRuntimeOrchestrator_BlockCommandsRejectBlockedOrTerminalRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-block-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name string
		run  *runmodel.Run
	}{
		{
			name: "already blocked on decision",
			run: &runmodel.Run{
				ID:               common.RunID("run-block-conflict-decision"),
				WorkspaceID:      workspaceID,
				State:            common.RunStateBlockedOnDecision,
				BlockingTicketID: common.TicketID("ticket-existing"),
			},
		},
		{
			name: "already blocked on approval",
			run: &runmodel.Run{
				ID:                 common.RunID("run-block-conflict-approval"),
				WorkspaceID:        workspaceID,
				State:              common.RunStateBlockedOnApproval,
				BlockingApprovalID: common.ApprovalID("approval-existing"),
			},
		},
		{
			name: "completed run",
			run: &runmodel.Run{
				ID:          common.RunID("run-block-conflict-completed"),
				WorkspaceID: workspaceID,
				State:       common.RunStateCompleted,
			},
		},
		{
			name: "failed run",
			run: &runmodel.Run{
				ID:          common.RunID("run-block-conflict-failed"),
				WorkspaceID: workspaceID,
				State:       common.RunStateFailed,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := runRepo.Create(ctx, tc.run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := orch.BlockOnDecision(ctx, tc.run.ID, common.TicketID("ticket-next")); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("block on decision err = %v, want conflict", err)
			}
			if err := orch.BlockOnApproval(ctx, tc.run.ID, common.ApprovalID("approval-next")); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("block on approval err = %v, want conflict", err)
			}
		})
	}
}

func TestRuntimeOrchestrator_ValidateRunProjectsWorkspaceState(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-validating")
	runID := common.RunID("run-validating")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.ValidateRun(ctx, runID); err != nil {
		t.Fatalf("validate run: %v", err)
	}

	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.State != common.WorkspaceStateValidating || ws.ActiveRunID != runID {
		t.Fatalf("workspace = %+v, want validating with active run", ws)
	}
	published := hub.Published()
	if len(published) < 2 {
		t.Fatalf("published events = %d, want run and workspace state events", len(published))
	}
	last := published[len(published)-1]
	if last.Topic != events.TopicWorkspaceStateChanged || last.Payload[events.PayloadKeyState] != string(common.WorkspaceStateValidating) {
		t.Fatalf("last event = %#v, want workspace validating event", last)
	}
}

func TestRuntimeOrchestrator_BlockRunCanReplaceStaleTerminalActiveRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-stale-terminal-active")
	staleRunID := common.RunID("run-stale-terminal-active")
	currentRunID := common.RunID("run-current-block")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: staleRunID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: staleRunID, WorkspaceID: workspaceID, State: common.RunStateCompleted}); err != nil {
		t.Fatalf("seed stale run: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: currentRunID, WorkspaceID: workspaceID, State: common.RunStateRunning}); err != nil {
		t.Fatalf("seed current run: %v", err)
	}

	if err := orch.BlockOnApproval(ctx, currentRunID, common.ApprovalID("approval-current-block")); err != nil {
		t.Fatalf("block on approval with stale terminal active run: %v", err)
	}

	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.State != common.WorkspaceStateBlocked || ws.ActiveRunID != currentRunID {
		t.Fatalf("workspace = %+v, want current blocked run projection", ws)
	}
}

func TestRuntimeOrchestrator_ResumeRunRequiresResolvedTicket(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	ticketID := common.TicketID("ticket-1")
	_ = workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID})
	_ = runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision, BlockingTicketID: ticketID})
	_ = ticketRepo.Create(ctx, &decisionmodel.Ticket{ID: ticketID, WorkspaceID: workspaceID, RunID: runID, State: common.DecisionTicketStateOpen})

	if err := orch.ResumeRun(ctx, runID); !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("resume blocked by open ticket err = %v, want blocked", err)
	}

	ticket, _ := ticketRepo.Get(ctx, ticketID)
	ticket.State = common.DecisionTicketStateResolved
	_ = ticketRepo.Update(ctx, ticket)
	_ = resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-1"),
		TicketID: ticketID,
		Type:     "provide_missing_info",
		Payload:  map[string]any{"operator": "tester"},
	})

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after ticket resolved: %v", err)
	}

	r, _ := runRepo.Get(ctx, runID)
	if r.State != common.RunStateCompleted {
		t.Fatalf("run state = %q, want completed", r.State)
	}
	if r.BlockingTicketID != "" {
		t.Fatalf("blocking ticket = %q, want empty", r.BlockingTicketID)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectsDecisionWithoutDispatchingStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-decision-reject")
	runID := common.RunID("run-decision-reject")
	ticketID := common.TicketID("ticket-decision-reject")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision, BlockingTicketID: ticketID}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{ID: ticketID, WorkspaceID: workspaceID, RunID: runID, State: common.DecisionTicketStateResolved}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-decision-reject"),
		TicketID: ticketID,
		Type:     "reject_current_path",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume rejected decision: %v", err)
	}

	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateFailed {
		t.Fatalf("run state = %q, want failed", r.State)
	}
	if r.BlockingTicketID != "" || r.BlockingApprovalID != "" {
		t.Fatalf("blocking refs = ticket=%q approval=%q, want empty", r.BlockingTicketID, r.BlockingApprovalID)
	}
	if _, err := stepRepo.Get(ctx, common.StepID(string(runID)+"/step-1")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no dispatched step, got %v", err)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime calls = %#v, want none", runtime.calls)
	}
	last := hub.Published()[len(hub.Published())-1]
	if last.Topic != events.TopicWorkspaceStateChanged || last.Payload[events.PayloadKeyActiveRunID] != "" {
		t.Fatalf("last event = %#v, want workspace release event", last)
	}
}

func TestRuntimeOrchestrator_ResumeRunReplanSupersedesRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-decision-replan")
	runID := common.RunID("run-decision-replan")
	ticketID := common.TicketID("ticket-decision-replan")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision, BlockingTicketID: ticketID}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{ID: ticketID, WorkspaceID: workspaceID, RunID: runID, State: common.DecisionTicketStateResolved}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-decision-replan"),
		TicketID: ticketID,
		Type:     "replan",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume replan decision: %v", err)
	}

	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateSuperseded {
		t.Fatalf("run state = %q, want superseded", r.State)
	}
	if r.BlockingTicketID != "" || r.BlockingApprovalID != "" {
		t.Fatalf("blocking refs = ticket=%q approval=%q, want empty", r.BlockingTicketID, r.BlockingApprovalID)
	}
	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.ActiveRunID != "" {
		t.Fatalf("workspace active run = %q, want empty", ws.ActiveRunID)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime calls = %#v, want none", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunRequiresResolvedApproval(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	approvalID := common.ApprovalID("approval-1")
	_ = workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID})
	_ = runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnApproval, BlockingApprovalID: approvalID})
	_ = approvalRepo.Create(ctx, &approvalmodel.Approval{ID: approvalID, WorkspaceID: workspaceID, RunID: runID, State: common.ApprovalStatePending})

	if err := orch.ResumeRun(ctx, runID); !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("resume blocked by pending approval err = %v, want blocked", err)
	}

	approval, _ := approvalRepo.Get(ctx, approvalID)
	approval.State = common.ApprovalStateApproved
	_ = approvalRepo.Update(ctx, approval)

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after approval resolved: %v", err)
	}

	r, _ := runRepo.Get(ctx, runID)
	if r.State != common.RunStateCompleted {
		t.Fatalf("run state = %q, want completed", r.State)
	}
	if r.BlockingApprovalID != "" {
		t.Fatalf("blocking approval = %q, want empty", r.BlockingApprovalID)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectedApprovalFailsRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	approvalID := common.ApprovalID("approval-1")
	_ = workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID})
	_ = runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnApproval, BlockingApprovalID: approvalID})
	_ = approvalRepo.Create(ctx, &approvalmodel.Approval{ID: approvalID, WorkspaceID: workspaceID, RunID: runID, State: common.ApprovalStateRejected})

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after approval rejection should fail run in-repo, got err: %v", err)
	}

	r, _ := runRepo.Get(ctx, runID)
	if r.State != common.RunStateFailed {
		t.Fatalf("run state = %q, want failed", r.State)
	}
	if r.BlockingApprovalID != "" {
		t.Fatalf("blocking approval = %q, want empty", r.BlockingApprovalID)
	}

	ws, _ := workspaceRepo.Get(ctx, workspaceID)
	if ws.ActiveRunID != "" {
		t.Fatalf("workspace active run id = %q, want empty", ws.ActiveRunID)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime calls = %#v, want no execution after rejection", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectsMissingBlockingReference(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-missing-ref")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name string
		run  *runmodel.Run
	}{
		{
			name: "decision missing ticket",
			run:  &runmodel.Run{ID: common.RunID("run-missing-ticket"), WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision},
		},
		{
			name: "approval missing approval",
			run:  &runmodel.Run{ID: common.RunID("run-missing-approval"), WorkspaceID: workspaceID, State: common.RunStateBlockedOnApproval},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := runRepo.Create(ctx, tc.run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := orch.ResumeRun(ctx, tc.run.ID); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("resume err = %v, want conflict", err)
			}
		})
	}
}

func TestRuntimeOrchestrator_CompleteRunAndFailRun_ValidateArguments(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	if err := orch.CompleteRun(ctx, ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("complete empty run err = %v, want invalid argument", err)
	}
	if err := orch.FailRun(ctx, "", "reason"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("fail empty run err = %v, want invalid argument", err)
	}
	if err := orch.FailRun(ctx, common.RunID("run-1"), ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("fail empty reason err = %v, want invalid argument", err)
	}
}

func TestRuntimeOrchestrator_CompleteRunRejectsNonRunningState(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-complete-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name string
		run  *runmodel.Run
	}{
		{name: "created", run: &runmodel.Run{ID: common.RunID("run-complete-created"), WorkspaceID: workspaceID, State: common.RunStateCreated}},
		{name: "blocked decision", run: &runmodel.Run{ID: common.RunID("run-complete-blocked-decision"), WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision, BlockingTicketID: common.TicketID("ticket-1")}},
		{name: "blocked approval", run: &runmodel.Run{ID: common.RunID("run-complete-blocked-approval"), WorkspaceID: workspaceID, State: common.RunStateBlockedOnApproval, BlockingApprovalID: common.ApprovalID("approval-1")}},
		{name: "completed", run: &runmodel.Run{ID: common.RunID("run-complete-completed"), WorkspaceID: workspaceID, State: common.RunStateCompleted}},
		{name: "failed", run: &runmodel.Run{ID: common.RunID("run-complete-failed"), WorkspaceID: workspaceID, State: common.RunStateFailed}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := runRepo.Create(ctx, tc.run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := orch.CompleteRun(ctx, tc.run.ID); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("complete run err = %v, want conflict", err)
			}
			got, err := runRepo.Get(ctx, tc.run.ID)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			if !reflect.DeepEqual(got, tc.run) {
				t.Fatalf("run mutated after conflict: got=%+v want=%+v", *got, *tc.run)
			}
		})
	}
}

func TestRuntimeOrchestrator_FailRunRejectsTerminalState(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-fail-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name string
		run  *runmodel.Run
	}{
		{name: "completed", run: &runmodel.Run{ID: common.RunID("run-fail-completed"), WorkspaceID: workspaceID, State: common.RunStateCompleted}},
		{name: "failed", run: &runmodel.Run{ID: common.RunID("run-fail-failed"), WorkspaceID: workspaceID, State: common.RunStateFailed}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := runRepo.Create(ctx, tc.run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := orch.FailRun(ctx, tc.run.ID, "manual fail"); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("fail run err = %v, want conflict", err)
			}
			got, err := runRepo.Get(ctx, tc.run.ID)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			if !reflect.DeepEqual(got, tc.run) {
				t.Fatalf("run mutated after conflict: got=%+v want=%+v", *got, *tc.run)
			}
		})
	}
}

func hasRunCheckpointState(items []*runmodel.RuntimeCheckpoint, state common.RunState) bool {
	for _, item := range items {
		if item != nil && item.RunStateSnapshot == state {
			return true
		}
	}
	return false
}

func hasCheckpointEvent(items []*runmodel.RuntimeCheckpoint, event string) bool {
	for _, item := range items {
		if item == nil || item.Data == nil {
			continue
		}
		if item.Data["event"] == event {
			return true
		}
	}
	return false
}

func hasCheckpointGraphBranch(items []*runmodel.RuntimeCheckpoint, branch string) bool {
	for _, item := range items {
		if item == nil || item.Data == nil {
			continue
		}
		if item.Data["graphBranch"] == branch {
			return true
		}
	}
	return false
}

func hasCheckpointFailureClass(items []*runmodel.RuntimeCheckpoint, class string) bool {
	for _, item := range items {
		if item == nil || item.Data == nil {
			continue
		}
		if item.Data["failureClass"] == class {
			return true
		}
	}
	return false
}

func duplicateShellFailureStep(runID common.RunID, suffix string) *runmodel.Step {
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/" + suffix),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{stepInputKeyGraphNodeID: "verify"},
		Output: map[string]any{
			"status":   "failed",
			"command":  "go",
			"args":     []string{"test", "./..."},
			"exitCode": 1,
			"stderr":   "same compiler error",
			"error":    "exit status 1",
		},
	}
	classification := classifyRuntimeFailure(step, step.Output, "runtime step failed", errors.New("exit status 1"))
	attachRuntimeFailureClassification(step.Output, classification)
	return step
}

func hasMemoryItem(items []*memorymodel.MemoryItem, runID common.RunID, memoryType string, summary string) bool {
	for _, item := range items {
		if item != nil &&
			item.RunID == runID &&
			item.SourceType == "run" &&
			item.SourceRef == string(runID) &&
			item.MemoryType == memoryType &&
			item.Summary == summary {
			return true
		}
	}
	return false
}

func hasSkillFeedbackMemory(items []*memorymodel.MemoryItem, runID common.RunID, skillID string) bool {
	for _, item := range items {
		if item != nil &&
			item.RunID == runID &&
			item.SourceType == "run" &&
			item.MemoryType == memorycommon.MemoryTypeSkillFeedback &&
			strings.Contains(item.Summary, skillID) {
			return true
		}
	}
	return false
}

func hasCompletionRunSummaryMemory(items []*memorymodel.MemoryItem, runID common.RunID, sourceRef common.StepID, summary string) bool {
	for _, item := range items {
		if item != nil &&
			item.RunID == runID &&
			item.SourceType == "run" &&
			item.SourceRef == string(sourceRef) &&
			item.MemoryType == memorycommon.MemoryTypeRunSummary &&
			item.Summary == summary &&
			strings.Contains(item.Content, "DoD items:") {
			return true
		}
	}
	return false
}

func hasRunSummaryContent(items []*memorymodel.MemoryItem, runID common.RunID, parts ...string) bool {
	for _, item := range items {
		if item == nil || item.RunID != runID || item.MemoryType != memorycommon.MemoryTypeRunSummary {
			continue
		}
		missing := false
		for _, part := range parts {
			if !strings.Contains(item.Content, part) {
				missing = true
				break
			}
		}
		if !missing {
			return true
		}
	}
	return false
}
