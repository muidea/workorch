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
	started := make(chan string, 3)
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
		switch {
		case request.StepID == "run-patch-graph/step-1":
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
		case boolValue(request.Input["graphFollowUp"]) && request.Input["graphPreviousNodeId"] == "verify-result":
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

func TestApplyWorkspacePatchAcceptsRangeLessUnifiedDiff(t *testing.T) {
	workingDirectory := t.TempDir()
	target := filepath.Join(workingDirectory, "docs", "sample.txt")
	original := "alpha\n\nsection one\nold value\nend section\n\nomega\n"
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create target dir: %v", err)
	}
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	err := applyWorkspacePatch(workingDirectory, runtimePatchRequest{
		TargetRef: "docs/sample.txt",
		PatchType: "unified_diff",
		Diff:      "--- a/docs/sample.txt\n+++ b/docs/sample.txt\n@@\n section one\n-old value\n+new value\n end section\n",
	})
	if err != nil {
		t.Fatalf("applyWorkspacePatch range-less diff err = %v", err)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read target: %v", readErr)
	}
	if !strings.Contains(string(content), "new value") || strings.Contains(string(content), "old value") {
		t.Fatalf("content = %q, want range-less diff applied", string(content))
	}
}

func TestApplyWorkspacePatchAcceptsUnifiedDiffWithDriftedLineNumberWhenContextIsUnique(t *testing.T) {
	workingDirectory := t.TempDir()
	target := filepath.Join(workingDirectory, "handler", "handler_test.go")
	original := "package handler\n\nimport (\n\t\"encoding/json\"\n\t\"net/http\"\n\t\"net/http/httptest\"\n\t\"os\"\n\t\"strings\"\n\t\"testing\"\n\t\"time\"\n\n\t\"ai-gateway/config\"\n)\n"
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create target dir: %v", err)
	}
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	err := applyWorkspacePatch(workingDirectory, runtimePatchRequest{
		TargetRef: "handler/handler_test.go",
		PatchType: "unified_diff",
		Diff: "--- handler/handler_test.go\n+++ handler/handler_test.go\n@@ -8,7 +8,7 @@\n" +
			" \t\"time\"\n" +
			" \n" +
			"-\t\"ai-gateway/config\"\n" +
			"+\t\"openai-gateway/config\"\n" +
			" )\n",
	})
	if err != nil {
		t.Fatalf("applyWorkspacePatch drifted line diff err = %v", err)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read target: %v", readErr)
	}
	if !strings.Contains(string(content), "\"openai-gateway/config\"") || strings.Contains(string(content), "\"ai-gateway/config\"") {
		t.Fatalf("content = %q, want drifted line diff applied", string(content))
	}
}

func TestApplyWorkspacePatchAcceptsSingleFileApplyPatchStyleDiff(t *testing.T) {
	workingDirectory := t.TempDir()
	target := filepath.Join(workingDirectory, "docs", "sample.txt")
	original := "alpha\n\nsection one\nold value\nend section\n\nomega\n"
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create target dir: %v", err)
	}
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	err := applyWorkspacePatch(workingDirectory, runtimePatchRequest{
		TargetRef: "docs/sample.txt",
		PatchType: "unified_diff",
		Diff:      "*** Begin Patch\n*** Update File: docs/sample.txt\n@@\n section one\n-old value\n+new value\n end section\n*** End Patch",
	})
	if err != nil {
		t.Fatalf("applyWorkspacePatch apply-patch style diff err = %v", err)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read target: %v", readErr)
	}
	if !strings.Contains(string(content), "new value") || strings.Contains(string(content), "old value") {
		t.Fatalf("content = %q, want apply-patch style diff applied", string(content))
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

func TestRuntimePatchRequestAllowsSourceCompositeLiteralBraces(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "main_test.go",
		"content": `package main

var response = map[string]interface{}{
	"choices": []map[string]interface{}{{
		"message": map[string]string{"content": "ok"},
	}},
}
`,
	})
	if err != nil {
		t.Fatalf("runtimePatchRequestFromMap err = %v, want Go composite literal braces allowed", err)
	}
}

func TestRuntimePatchRequestRejectsUnresolvedSymbolicPlaceholderContent(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "config.go",
		"content":   "__FROM_READ_REWRITE_CONFIG_GO_WITH_CONFIG_YAML_AND_ENV_OVERRIDE__",
	})
	if err == nil || !strings.Contains(err.Error(), "unresolved runtime template") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want unresolved symbolic placeholder error", err)
	}
}

func TestRuntimePatchRequestRejectsRedactedMarkerContent(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "main_test.go",
		"content":   `if auth != "Bearer [REDACTED]" { panic("bad auth") }`,
	})
	if err == nil || !strings.Contains(err.Error(), "redacted runtime markers") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want redacted marker error", err)
	}
}

func TestRuntimePatchRequestRejectsAnglePlaceholderDiff(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "unified_diff",
		"targetRef": "main.go",
		"diff":      "<<FROM_IMPLEMENTATION_STEP_MAIN_DIFF>>",
	})
	if err == nil || !strings.Contains(err.Error(), "unresolved runtime template") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want unresolved angle placeholder error", err)
	}
}

func TestRuntimePatchRequestRejectsEmptyFileWriteContent(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "config.json",
		"content":   "   ",
	})
	if err == nil || !strings.Contains(err.Error(), "patch content is required") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want empty content error", err)
	}
}

func TestRuntimePatchRequestRejectsNonSubstantivePatchPlaceholder(t *testing.T) {
	_, err := runtimePatchRequestFromMap(map[string]any{
		"patchType": "file_write",
		"targetRef": "logger.go",
		"summary":   "添加注释以确保补丁节点存在",
		"content": `package main

import "fmt"

// LogMessage prints a message to stdout
func LogMessage(msg string) {
	fmt.Println(msg)
}
`,
	})
	if err == nil || !strings.Contains(err.Error(), "non-substantive state-change placeholder") {
		t.Fatalf("runtimePatchRequestFromMap error = %v, want non-substantive patch error", err)
	}
}

func TestDecisionWithQuestionReferencingReadableWorkspaceFileShouldRepair(t *testing.T) {
	spec := runtimeDecisionBlockSpec{
		DecisionKind: "provide_missing_info",
		Question:     "请提供 handler/handler.go 和 config/config.go 的完整内容。",
		Reason:       "测试文件引用未定义符号，需要读取实际源文件后修复。",
		Context: map[string]any{
			"missingFields":       []any{"actualExportedSymbols"},
			"requiresHumanFields": []any{"actualExportedSymbols"},
			"readableResources":   []any{"handler/handler.go", "config/config.go"},
		},
	}

	if !knownReadableWorkspaceResourceDecisionSpec(spec) {
		t.Fatalf("knownReadableWorkspaceResourceDecisionSpec = false, want true for question-referenced workspace files")
	}
	if !continuableDecisionShouldRepair(&runmodel.Step{ID: "run-1/step-1"}, spec) {
		t.Fatalf("continuableDecisionShouldRepair = false, want true for readable workspace resources")
	}
}

func TestApplyWorkspaceUnifiedDiffReportsContextMismatch(t *testing.T) {
	workingDirectory := t.TempDir()
	targetPath := filepath.Join(workingDirectory, "main_test.go")
	if err := os.WriteFile(targetPath, []byte("package main\n\nfunc TestAuth() {\n\tif auth != \"Bearer [REDACTED]\" {\n\t\tpanic(\"bad auth\")\n\t}\n}\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	err := applyWorkspaceUnifiedDiff(workingDirectory, "main_test.go", "--- a/main_test.go\n+++ b/main_test.go\n@@ -34,7 +34,7 @@\n \tauth := r.Header.Get(\"Authorization\")\n-\tif auth != \"Bearer [REDACTED]\" {\n+\tif auth != \"Bearer test-key\" {\n \t\thttp.Error(w, \"Unauthorized\", http.StatusUnauthorized)\n", "")
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "unified diff hunk") {
		t.Fatalf("applyWorkspaceUnifiedDiff err = %v, want contextual unified diff error", err)
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
		case strings.Contains(string(request.StepID), "/repair-"):
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
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingDoDItems": []map[string]any{
					{
						"raw":                  "配置文件必须被代码真实加载并有测试断言",
						"normalized":           "配置文件必须被代码真实加载并有测试断言",
						"kind":                 "leaf",
						"requiredEvidenceHint": "Provide implementation evidence and configuration-loading verification evidence for this DoD item.",
					},
				},
			},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, common.ErrInvalidArgument)

	if manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest]); len(manifest) != 0 {
		t.Fatalf("workspaceFileManifest = %+v, want no manifest for non-path DoD repair", input[stepInputKeyWorkspaceFileManifest])
	}
	if _, ok := mapFromValue(input["runtimeDiagnostics"])["missingDoDItems"]; !ok {
		t.Fatalf("runtimeDiagnostics = %#v, want missingDoDItems diagnostics", input["runtimeDiagnostics"])
	}
	if missing := mapSliceFromValue(input["missingDoDItems"]); len(missing) != 1 || missing[0]["kind"] != "leaf" {
		t.Fatalf("missingDoDItems = %#v, want one leaf item", input["missingDoDItems"])
	}
}

func TestOutputWithDoDAssessmentDiagnosticsNormalizesMissingItems(t *testing.T) {
	output := outputWithDoDAssessmentDiagnostics(map[string]any{
		"status": "completed",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"summary":   "one item missing",
			"items": []map[string]any{
				{
					"item":         "first item",
					"state":        "satisfied",
					"evidenceRefs": []string{"run/step-1"},
				},
				{
					"item":         "second item",
					"state":        "failed",
					"reason":       "missing implementation evidence",
					"impact":       "cannot accept the run",
					"evidenceRefs": []string{"run/step-2"},
					"nextActions":  []string{"implement the missing behavior", "rerun verification"},
				},
			},
		},
	})

	diagnostics := mapFromValue(output["runtimeDiagnostics"])
	if diagnostics["kind"] != failureClassDoDCoverageMissing {
		t.Fatalf("runtimeDiagnostics = %+v, want DoD coverage missing kind", diagnostics)
	}
	missing := mapSliceFromValue(output["missingDoDItems"])
	if len(missing) != 1 || missing[0]["item"] != "second item" || missing[0]["state"] != "failed" || missing[0]["source"] != "dodAssessment" {
		t.Fatalf("missingDoDItems = %+v, want normalized failed DoD item", missing)
	}
	if refs := stringSliceFromValue(missing[0]["evidenceRefs"]); len(refs) != 1 || refs[0] != "run/step-2" {
		t.Fatalf("missingDoDItems evidenceRefs = %+v, want run/step-2", missing[0]["evidenceRefs"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesPatchTargetSnapshot(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	workingDirectory := t.TempDir()
	source := "func sample(arg *Type) {\n\t_ = handler(func(v *Value) {})\n}\n"
	if err := os.WriteFile(filepath.Join(workingDirectory, "source.txt"), []byte(source), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-patch-repair-context")
	runID := common.RunID("run-patch-repair-context")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/block/apply"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{
			"targetRef": "source.txt",
			"diff":      "--- a/source.txt\n+++ b/source.txt\n@@\n-func sample(arg Type)\n+func sample(arg *Type)",
		},
		Output: map[string]any{
			"status":                  "completed",
			"stage":                   "patch_apply",
			"error":                   "invalid argument",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: "class=patch_apply_failed|targetRef=source.txt",
			"patches": []map[string]any{
				{"targetRef": "source.txt"},
			},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, common.ErrInvalidArgument)

	patchContext := mapSliceFromValue(input["patchRepairContext"])
	if len(patchContext) != 1 {
		t.Fatalf("patchRepairContext = %#v, want one snapshot", input["patchRepairContext"])
	}
	item := patchContext[0]
	if item["targetRef"] != "source.txt" || item["exists"] != true || item["source"] != "current_workspace_file" {
		t.Fatalf("patchRepairContext item = %#v, want current source.txt snapshot", item)
	}
	content, _ := item["content"].(string)
	if !strings.Contains(content, "*Type") || !strings.Contains(content, "*Value") {
		t.Fatalf("patchRepairContext content = %q, want exact current source operators", content)
	}
	if strings.TrimSpace(stringFromMap(item, "contentHash")) == "" {
		t.Fatalf("patchRepairContext contentHash missing: %#v", item)
	}
	if item["contentComplete"] != true || item["safeForFileWrite"] != true {
		t.Fatalf("patchRepairContext safety = %#v, want complete safe snapshot", item)
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	for _, want := range []string{"patchRepairContext", "failed patch text as stale", "safeForFileWrite", "minimal unified_diff", "Never copy [REDACTED]"} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("repairGuidance = %q, want %q", guidance, want)
		}
	}

	failedPreviewStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/block/preview"),
		RunID: runID,
		Kind:  "patch/preview",
		State: common.StepStateFailed,
		Input: map[string]any{
			"targetRef": "source.txt",
			"diff":      "--- a/source.txt\n+++ b/source.txt\n@@\n-func sample(arg Type)\n+func sample(arg *Type)",
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "patch_preview",
			"error":                   "conflict",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=patch/preview|targetRef=source.txt",
		},
	}

	previewInput := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedPreviewStep, 1, common.ErrConflict)
	previewPatchContext := mapSliceFromValue(previewInput["patchRepairContext"])
	if len(previewPatchContext) != 1 {
		t.Fatalf("patchRepairContext for preview = %#v, want one snapshot", previewInput["patchRepairContext"])
	}
	if previewPatchContext[0]["targetRef"] != "source.txt" {
		t.Fatalf("patchRepairContext for preview = %#v, want source.txt target", previewPatchContext[0])
	}

	failedEditStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/block/edit"),
		RunID: runID,
		Kind:  "file/edit",
		State: common.StepStateFailed,
		Input: map[string]any{
			"path":    "source.txt",
			"oldText": "func sample(arg Type)",
			"newText": "func sample(arg *Type)",
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "file_edit_conflict",
			"error":                   "conflict: file/edit oldText not found",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=file/edit|path=source.txt",
		},
	}
	editInput := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedEditStep, 1, common.ErrConflict)
	editPatchContext := mapSliceFromValue(editInput["patchRepairContext"])
	if len(editPatchContext) != 1 || editPatchContext[0]["targetRef"] != "source.txt" {
		t.Fatalf("patchRepairContext for file/edit conflict = %#v, want source.txt snapshot", editInput["patchRepairContext"])
	}
	editGuidance := strings.Join(stringSliceFromValue(editInput["repairGuidance"]), "\n")
	if !strings.Contains(editGuidance, "file/edit oldText conflict") || !strings.Contains(editGuidance, "current target snapshot") {
		t.Fatalf("repairGuidance for file/edit conflict = %q, want stale oldText guidance", editGuidance)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesPatchRepairSuggestions(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	workingDirectory := t.TempDir()
	source := "alpha\nold value\nomega\n"
	if err := os.WriteFile(filepath.Join(workingDirectory, "source.txt"), []byte(source), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-patch-repair-suggestion")
	runID := common.RunID("run-patch-repair-suggestion")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/block/apply"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{
			"targetRef": "source.txt",
			"patchType": "unified_diff",
			"diff":      "--- a/source.txt\n+++ b/source.txt\n@@ -10,3 +10,3 @@\n-old value\n+new value\n",
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "patch_apply",
			"error":                   "invalid argument: unified diff hunk context mismatch",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: "class=patch_apply_failed|targetRef=source.txt",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, common.ErrInvalidArgument)

	suggestions := mapSliceFromValue(input["patchRepairSuggestions"])
	if len(suggestions) != 1 {
		t.Fatalf("patchRepairSuggestions = %#v, want one suggestion", input["patchRepairSuggestions"])
	}
	if suggestions[0]["targetRef"] != "source.txt" || suggestions[0]["oldText"] != "old value" || suggestions[0]["newText"] != "new value" || suggestions[0]["safeForMinimal"] != true {
		t.Fatalf("patchRepairSuggestions[0] = %#v, want exact old/new replacement", suggestions[0])
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	if !strings.Contains(guidance, "patchRepairSuggestions") || !strings.Contains(guidance, "smallest current unified_diff") {
		t.Fatalf("repairGuidance = %q, want patch repair suggestion guidance", guidance)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesLineRemovalPatchSuggestion(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	workingDirectory := t.TempDir()
	source := "package main\n\nimport (\n\t\"encoding/json\"\n\t\"io\"\n\t\"net/http\"\n)\n"
	if err := os.WriteFile(filepath.Join(workingDirectory, "gateway_test.go"), []byte(source), 0o644); err != nil {
		t.Fatalf("seed source file: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-patch-repair-line-removal")
	runID := common.RunID("run-patch-repair-line-removal")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-006/001-fix-import"),
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateFailed,
		Input: map[string]any{
			"targetRef": "gateway_test.go",
		},
		Output: map[string]any{
			"status":                  "completed",
			"stage":                   "patch_apply",
			"error":                   "invalid argument: unified diff hunk context mismatch",
			failureOutputKeyClass:     failureClassPatchApply,
			failureOutputKeySignature: "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|node=fix-import",
			"patches": []map[string]any{{
				"targetRef": "gateway_test.go",
				"patchType": "unified_diff",
				"diff":      "--- a/gateway_test.go\n+++ b/gateway_test.go\n@@ -2,8 +2,7 @@\n import (\n \t\"encoding/json\"\n-\t\"io\"\n \t\"net/http\"\n )",
			}},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, common.ErrInvalidArgument)

	suggestions := mapSliceFromValue(input["patchRepairSuggestions"])
	found := false
	for _, suggestion := range suggestions {
		if suggestion["targetRef"] == "gateway_test.go" &&
			suggestion["oldText"] == "\t\"io\"" &&
			suggestion["newText"] == "" &&
			suggestion["safeForMinimal"] == true {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("patchRepairSuggestions = %#v, want exact import line removal suggestion", input["patchRepairSuggestions"])
	}
}

func TestPatchRepairTargetSnapshotMarksRedactedOrTruncatedContentUnsafeForFileWrite(t *testing.T) {
	workingDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDirectory, "secret.txt"), []byte("token [REDACTED]\n"), 0o644); err != nil {
		t.Fatalf("seed redacted file: %v", err)
	}
	large := strings.Repeat("x", maxPatchRepairSnapshotBytes+1)
	if err := os.WriteFile(filepath.Join(workingDirectory, "large.txt"), []byte(large), 0o644); err != nil {
		t.Fatalf("seed large file: %v", err)
	}

	redacted := patchRepairTargetSnapshot(workingDirectory, "secret.txt")
	if redacted["redacted"] != true || redacted["safeForFileWrite"] != false || redacted["contentComplete"] != true {
		t.Fatalf("redacted snapshot = %#v, want unsafe redacted complete snapshot", redacted)
	}
	truncated := patchRepairTargetSnapshot(workingDirectory, "large.txt")
	if truncated["truncated"] != true || truncated["contentComplete"] != false || truncated["safeForFileWrite"] != false {
		t.Fatalf("truncated snapshot = %#v, want unsafe truncated snapshot", truncated)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesFailureTargetSnapshot(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	workingDirectory := t.TempDir()
	source := "package main\n\nimport (\n\t\"fmt\"\n\t\"unused/pkg\"\n)\n"
	if err := os.WriteFile(filepath.Join(workingDirectory, "main_test.go"), []byte(source), 0o644); err != nil {
		t.Fatalf("seed failure target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workingDirectory, "support.txt"), []byte("related resource"), 0o644); err != nil {
		t.Fatalf("seed related resource: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-failure-target-repair-context")
	runID := common.RunID("run-failure-target-repair-context")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/004-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageExecution,
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			"status":                  "failed",
			"error":                   "exit status 1",
			"stderr":                  "./main_test.go:5:2: \"unused/pkg\" imported and not used\n",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|exitCode=1",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, errors.New("exit status 1"))

	targetContext := mapSliceFromValue(input["failureTargetRepairContext"])
	if len(targetContext) != 1 {
		t.Fatalf("failureTargetRepairContext = %#v, want one target snapshot", input["failureTargetRepairContext"])
	}
	item := targetContext[0]
	if item["targetRef"] != "main_test.go" || item["exists"] != true || item["source"] != "failure_target_current_workspace_file" {
		t.Fatalf("failureTargetRepairContext item = %#v, want current main_test.go target", item)
	}
	if content := stringFromMap(item, "content"); !strings.Contains(content, "unused/pkg") {
		t.Fatalf("failureTargetRepairContext content = %q, want current failing target content", content)
	}
	manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest])
	if len(manifest) != 2 || manifest[0]["path"] != "main_test.go" || manifest[1]["path"] != "support.txt" {
		t.Fatalf("workspaceFileManifest = %#v, want known workspace resources for target repair", input[stepInputKeyWorkspaceFileManifest])
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	if !strings.Contains(guidance, "failureTargetRepairContext") || !strings.Contains(guidance, "patch/apply") || !strings.Contains(guidance, "workspaceFileManifest") {
		t.Fatalf("repairGuidance = %q, want target snapshot patch guidance", guidance)
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	if len(actionPackage) == 0 {
		t.Fatalf("repairActionPackage = %#v, want actionable package", input["repairActionPackage"])
	}
	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true || !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("repairActionPackage.requiredOutput = %#v, want patch/apply state-changing contract", required)
	}
	if required["mustTargetFailureScope"] != true || required["targetRelation"] != "failure_target_or_repair_snapshot" {
		t.Fatalf("repairActionPackage.requiredOutput = %#v, want target-relevant repair contract", required)
	}
	targets := mapSliceFromValue(actionPackage["targetSnapshots"])
	if len(targets) != 1 || targets[0]["targetRef"] != "main_test.go" || strings.TrimSpace(stringFromMap(targets[0], "contentHash")) == "" {
		t.Fatalf("repairActionPackage.targetSnapshots = %#v, want current target snapshot", actionPackage["targetSnapshots"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputResolvesUniqueSuffixFailureTarget(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	workingDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workingDirectory, "nested"), 0o755); err != nil {
		t.Fatalf("seed nested dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workingDirectory, "nested", "target_test.go"), []byte("package nested\n"), 0o644); err != nil {
		t.Fatalf("seed nested target: %v", err)
	}
	workspaceID := common.WorkspaceID("ws-failure-target-suffix-context")
	runID := common.RunID("run-failure-target-suffix-context")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/004-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageExecution,
		},
		Output: map[string]any{
			"status":                  "failed",
			"error":                   "exit status 1",
			"stdout":                  "target_test.go:7: expected value\n",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=target_test.go|exitCode=1",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, errors.New("exit status 1"))

	targetContext := mapSliceFromValue(input["failureTargetRepairContext"])
	if len(targetContext) != 1 || targetContext[0]["targetRef"] != "nested/target_test.go" || targetContext[0]["exists"] != true {
		t.Fatalf("failureTargetRepairContext = %#v, want resolved nested target snapshot", input["failureTargetRepairContext"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputKeepsAmbiguousSuffixFailureTarget(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	workingDirectory := t.TempDir()
	for _, dir := range []string{"first", "second"} {
		if err := os.MkdirAll(filepath.Join(workingDirectory, dir), 0o755); err != nil {
			t.Fatalf("seed %s dir: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(workingDirectory, dir, "target_test.go"), []byte("package sample\n"), 0o644); err != nil {
			t.Fatalf("seed %s target: %v", dir, err)
		}
	}
	workspaceID := common.WorkspaceID("ws-failure-target-ambiguous-suffix")
	runID := common.RunID("run-failure-target-ambiguous-suffix")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/004-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageExecution,
		},
		Output: map[string]any{
			"status":                  "failed",
			"error":                   "exit status 1",
			"stdout":                  "target_test.go:7: expected value\n",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=target_test.go|exitCode=1",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, errors.New("exit status 1"))

	targetContext := mapSliceFromValue(input["failureTargetRepairContext"])
	if len(targetContext) != 1 || targetContext[0]["targetRef"] != "target_test.go" || targetContext[0]["exists"] != false {
		t.Fatalf("failureTargetRepairContext = %#v, want ambiguous short target preserved as missing snapshot", input["failureTargetRepairContext"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputRequiresStateChangingOutputForContractFailure(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-state-contract")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/block/implement"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphNodeID:       "unit-001",
			stepInputKeyRuntimeStage:      runtimeStageExecution,
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			"error": "state-changing execution-stage llm graph node returned no state-changing output",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-state-contract"}, failedStep, 1, common.ErrInvalidArgument)

	if input["repairObjective"] != "produce_state_changing_output" || !boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want state-changing output objective", input)
	}
	guidance := stringSliceFromValue(input["repairGuidance"])
	joined := strings.Join(guidance, "\n")
	if !strings.Contains(joined, "state-changing output") || !strings.Contains(joined, "shell-only executionGraph") || !strings.Contains(joined, "patch/apply state-change artifact") {
		t.Fatalf("repairGuidance = %#v, want state-changing output and shell-only rejection guidance", guidance)
	}
}

func TestRuntimeOrchestrator_InitialReplanStateChangingInputIncludesRepairActionPackage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-replan-action-package",
		WorkspaceID:     "ws-replan-action-package",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-before-replan-action-package",
				"reason":           "resolved_governance_replan",
				"failureContext": map[string]any{
					failureOutputKeyClass:     failureClassRuntime,
					failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|failureTargets=main.go",
					"failureTargetScope":      "failureTargets=main.go",
					"failedStepId":            "run-before-replan-action-package/repair-003",
					"failedStepKind":          llmWorkspaceExecutionStepKind,
					"failureTargetRepairContext": []map[string]any{{
						"targetRef":        "main.go",
						"exists":           true,
						"contentHash":      "hash-main",
						"contentComplete":  true,
						"safeForFileWrite": true,
						"content":          "package main\nfunc main() {}\n",
					}},
					"failedStepOutput": map[string]any{
						"error": "invalid argument: state-changing llm repair step returned executionGraph without patch/apply",
						stepOutputKeyExecutionGraph: map[string]any{"nodes": []map[string]any{{
							"id":   "test-only",
							"kind": "shell/command",
						}}},
					},
				},
			},
		},
	}

	input := initialStepInput(run)

	if input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] != true ||
		input["replanStateChangingContinuation"] != true {
		t.Fatalf("input = %+v, want state-changing replan continuation", input)
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	if len(actionPackage) == 0 {
		t.Fatalf("repairActionPackage = %#v, want package from replan continuation", input["repairActionPackage"])
	}
	targets := mapSliceFromValue(actionPackage["targetSnapshots"])
	if len(targets) != 1 || targets[0]["targetRef"] != "main.go" || targets[0]["contentHash"] != "hash-main" {
		t.Fatalf("repairActionPackage.targetSnapshots = %#v, want target snapshot", actionPackage["targetSnapshots"])
	}
	if summary := mapFromValue(actionPackage["previousRejectedOutputSummary"]); len(summary) == 0 || mapFromValue(summary["rejectedExecutionGraph"])["nodeCount"] == nil {
		t.Fatalf("repairActionPackage.previousRejectedOutputSummary = %#v, want rejected graph summary", actionPackage["previousRejectedOutputSummary"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputInheritsTargetContextAfterContractFailure(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-context-inherit")
	targetContext := []map[string]any{
		{
			"targetRef":   "main.go",
			"exists":      true,
			"source":      "failure_target_current_workspace_file",
			"contentHash": "hash-main",
			"content":     "package main\n\nimport \"old/module/config\"\n",
		},
	}
	manifest := []map[string]any{
		{"path": "main.go", "kind": "file", "size": float64(40)},
		{"path": "config.go", "kind": "file", "size": float64(80)},
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-graph-001-002-test"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"stateChangingOutputRequired":     true,
			"failureTargetRepairContext":      targetContext,
			stepInputKeyWorkspaceFileManifest: manifest,
		},
		Output: map[string]any{
			"error":                   "invalid argument: state-changing llm repair step returned executionGraph without patch/apply",
			"stage":                   "llm_execution_graph_plan",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan",
			stepOutputKeyExecutionGraph: map[string]any{"nodes": []map[string]any{{
				"id":   "test-only",
				"kind": "shell/command",
			}}},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-context-inherit"}, failedStep, 2, common.ErrInvalidArgument)

	if input["stateChangingRecoveryMode"] != "direct_state_change" {
		t.Fatalf("stateChangingRecoveryMode = %#v, want direct_state_change", input["stateChangingRecoveryMode"])
	}
	inherited := mapSliceFromValue(input["failureTargetRepairContext"])
	if len(inherited) != 1 || inherited[0]["targetRef"] != "main.go" || inherited[0]["contentHash"] != "hash-main" {
		t.Fatalf("failureTargetRepairContext = %#v, want inherited target context", input["failureTargetRepairContext"])
	}
	inheritedManifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest])
	if len(inheritedManifest) != 2 || inheritedManifest[0]["path"] != "main.go" || inheritedManifest[1]["path"] != "config.go" {
		t.Fatalf("workspaceFileManifest = %#v, want inherited manifest preserved for direct recovery", input[stepInputKeyWorkspaceFileManifest])
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	if !strings.Contains(guidance, "Preserve inherited repair context") || !strings.Contains(guidance, "patch/apply artifacts") || !strings.Contains(guidance, "workspace manifest") {
		t.Fatalf("repairGuidance = %q, want inherited repair context guidance", guidance)
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	if len(actionPackage) == 0 {
		t.Fatalf("repairActionPackage = %#v, want inherited actionable package", input["repairActionPackage"])
	}
	targets := mapSliceFromValue(actionPackage["targetSnapshots"])
	if len(targets) != 1 || targets[0]["targetRef"] != "main.go" || targets[0]["contentHash"] != "hash-main" {
		t.Fatalf("repairActionPackage.targetSnapshots = %#v, want inherited target snapshot", actionPackage["targetSnapshots"])
	}
	if summary := mapFromValue(actionPackage["previousRejectedOutputSummary"]); len(summary) == 0 || mapFromValue(summary["rejectedExecutionGraph"])["nodeCount"] == nil {
		t.Fatalf("repairActionPackage.previousRejectedOutputSummary = %#v, want rejected graph summary", actionPackage["previousRejectedOutputSummary"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesManifestForStateChangingResourceEvidence(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	workingDirectory := t.TempDir()
	for name, content := range map[string]string{
		"resource-a.txt": "primary",
		"resource-b.txt": "supporting",
	} {
		if err := os.WriteFile(filepath.Join(workingDirectory, name), []byte(content), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	workspaceID := common.WorkspaceID("ws-state-changing-resource-manifest")
	runID := common.RunID("run-state-changing-resource-manifest")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"stateChangingOutputRequired": true,
			"missingDoDItems": []map[string]any{
				{"item": "produce required artifact", "state": "failed"},
			},
			stepInputKeyGraphEvidence: []map[string]any{
				{
					"kind":   "file/list",
					"nodeId": "list_resources",
					"output": map[string]any{
						"stage": "file_list",
						"entries": []map[string]any{
							{"path": "resource-a.txt", "kind": "file"},
							{"path": "resource-b.txt", "kind": "file"},
						},
					},
				},
			},
		},
		Output: map[string]any{
			"error":                   "invalid argument: state-changing llm repair step returned executionGraph without patch/apply",
			"stage":                   "llm_execution_graph_plan",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 2, common.ErrInvalidArgument)

	if input["stateChangingRecoveryMode"] != "direct_state_change" {
		t.Fatalf("stateChangingRecoveryMode = %#v, want direct_state_change", input["stateChangingRecoveryMode"])
	}
	manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest])
	if len(manifest) != 2 || manifest[0]["path"] != "resource-a.txt" || manifest[1]["path"] != "resource-b.txt" {
		t.Fatalf("workspaceFileManifest = %#v, want manifest from current workspace", input[stepInputKeyWorkspaceFileManifest])
	}
	if _, ok := input[stepInputKeyGraphEvidence]; ok {
		t.Fatalf("graphEvidence should stay narrowed out during direct recovery: %#v", input[stepInputKeyGraphEvidence])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputIncludesManifestForOutputMissingDoD(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	workingDirectory := t.TempDir()
	for name, content := range map[string]string{
		"resource-a.txt": "primary",
		"resource-b.txt": "supporting",
	} {
		if err := os.WriteFile(filepath.Join(workingDirectory, name), []byte(content), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	workspaceID := common.WorkspaceID("ws-output-dod-manifest")
	runID := common.RunID("run-output-dod-manifest")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo, steps: memory.NewStepRepository()}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/follow-up-graph-planning-001-probe"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"stateChangingOutputRequired": true,
			stepInputKeyGraphEvidence: []map[string]any{
				{
					"kind":   "file/list",
					"nodeId": "probe",
					"output": map[string]any{
						"stage": "file_list",
						"entries": []map[string]any{
							{"path": "resource-a.txt", "kind": "file"},
							{"path": "resource-b.txt", "kind": "file"},
						},
					},
				},
			},
		},
		Output: map[string]any{
			"error":                   "invalid argument: completionState failed has no actionable continuation",
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
			"missingDoDItems": []map[string]any{
				{"item": "produce required artifact", "state": "failed", "source": "dodAssessment"},
			},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID}, failedStep, 1, common.ErrInvalidArgument)

	manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest])
	if len(manifest) != 2 || manifest[0]["path"] != "resource-a.txt" || manifest[1]["path"] != "resource-b.txt" {
		t.Fatalf("workspaceFileManifest = %#v, want manifest from output missing DoD repair", input[stepInputKeyWorkspaceFileManifest])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputDoesNotPromotePlanningReadOnlyFailureToStateChange(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-planning-state-contract")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeySoulStage: soulStagePlanning,
		},
		Output: map[string]any{
			"error": "invalid argument: state-changing planning llm step returned read-only executionGraph without patch output",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-planning-state-contract"}, failedStep, 1, common.ErrInvalidArgument)

	if input["repairObjective"] != nil || boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want planning read-only failure to stay open judgment without state-changing repair", input)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputRequiresStateChangeForMissingDoDWithStateChangePolicy(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-dod-gap-state-change")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/assessment"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
		},
		Output: map[string]any{
			"stage":               "llm_dod_assessment",
			failureOutputKeyClass: failureClassDoDCoverageMissing,
			"missingDoDItems": []map[string]any{
				{"item": "produce state-changing result", "state": "failed"},
			},
		},
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-repair-dod-gap-state-change",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, common.ErrInvalidArgument)

	if input["repairObjective"] != "produce_state_changing_output" || !boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want missing DoD repair to require state-changing output", input)
	}
	if len(mapSliceFromValue(input["missingDoDItems"])) != 1 {
		t.Fatalf("missingDoDItems = %+v, want missing DoD carried into repair input", input["missingDoDItems"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputRequiresStateChangeForValidationFollowUpReadOnlyGraph(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-validation-read-only-graph")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/follow-up-graph-validation-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
		},
		Output: map[string]any{
			"error": "invalid argument: validation follow-up llm step returned read-only executionGraph without patch output",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-validation-read-only-graph"}, failedStep, 1, common.ErrInvalidArgument)

	if input["repairObjective"] != "produce_state_changing_output" || !boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want validation follow-up read-only graph repair to require state-changing output", input)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputUsesEvidenceObjectiveForVerificationEvidenceGap(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-dod-evidence-gap")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/verify"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
			"runtimeDiagnostics": map[string]any{
				"kind": "dod_evidence_quality_gap",
			},
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied":           false,
				"summary":             "Current resources exist, but concrete verification evidence is missing.",
				"verificationSummary": "No actual verification evidence was produced.",
				"items": []map[string]any{
					{"item": "deliver verified outcome", "state": "failed"},
				},
				"knownLimitations": []string{"not verified"},
				"nextActions":      []string{"run bounded verification"},
			},
			"missingDoDItems": []map[string]any{
				{"item": "deliver verified outcome", "state": "failed"},
			},
		},
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-repair-dod-evidence-gap",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, common.ErrInvalidArgument)

	if input["repairObjective"] != repairObjectiveConcreteValidationEvidence || boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want concrete evidence objective without state-changing requirement", input)
	}
	if input["evidenceRepairRequired"] != true {
		t.Fatalf("evidenceRepairRequired = %v, want true", input["evidenceRepairRequired"])
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	if !strings.Contains(guidance, "concrete verification evidence") || !strings.Contains(guidance, "bounded executionGraph") {
		t.Fatalf("repairGuidance = %#v, want concrete evidence guidance", input["repairGuidance"])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputRequiresStateChangeForUnclassifiedDoDCoverageMissing(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-dod-coverage-unclassified-gap")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/verify"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "DoD coverage is missing.",
				"items": []map[string]any{
					{"item": "deliver accepted outcome", "state": "failed"},
				},
			},
			"missingDoDItems": []map[string]any{
				{"item": "deliver accepted outcome", "state": "failed"},
			},
		},
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-repair-dod-coverage-unclassified-gap",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, common.ErrInvalidArgument)

	if input["repairObjective"] != repairObjectiveStateChangingOutput || input["stateChangingOutputRequired"] != true {
		t.Fatalf("repair input = %+v, want state-changing objective for unclassified DoD coverage gap", input)
	}
	if input["evidenceRepairRequired"] != nil {
		t.Fatalf("repair input = %+v, want no evidence repair without structured evidence-quality diagnostics", input)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairInputInheritsStateChangingRequirement(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
	runID := common.RunID("run-repair-state-contract-inherit")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/zz-llm-repair-1"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			"stateChangingOutputRequired":     true,
			stepInputKeySoulStage:             soulStageExecution,
		},
		Output: map[string]any{
			"error": "state-changing llm repair step returned executionGraph instead of concrete patch output",
		},
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-state-contract-inherit"}, failedStep, 2, common.ErrInvalidArgument)

	if input["repairObjective"] != "produce_state_changing_output" || !boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("repair input = %+v, want inherited state-changing output objective", input)
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
	if manifest := mapSliceFromValue(pkg["workspaceFileManifest"]); len(manifest) != 1 || manifest[0]["path"] != "README.md" {
		t.Fatalf("workspaceFileManifest = %+v, want bounded current workspace manifest in planning context package", pkg["workspaceFileManifest"])
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
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}
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
		"executionContext":                  map[string]any{"skills": []any{map[string]any{"instruction": strings.Repeat("s", 256)}}},
		stepInputKeyExecutionContextPackage: map[string]any{"workspaceFileManifest": []map[string]any{{"path": ".git/object"}}},
		stepInputKeyWorkspaceFileManifest:   []map[string]any{{"path": ".git/object"}},
		"failedStepInput": map[string]any{
			"executionContext": map[string]any{"memories": []any{map[string]any{"content": strings.Repeat("m", 256)}}},
		},
		"failedStepOutput": map[string]any{"stderr": strings.Repeat("e", 256)},
		"previousStepOutput": map[string]any{
			stepOutputKeyExecutionGraph: map[string]any{"nodes": []any{map[string]any{"id": "inspect", "kind": "file/list"}}},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{"mustIncludeStateChangingNode": true, "acceptedKinds": []string{"patch/apply"}},
			"targetSnapshots": []map[string]any{{
				"targetRef":        "main.go",
				"contentHash":      "hash-main",
				"contentComplete":  true,
				"safeForFileWrite": true,
				"content":          "package main\n",
			}},
		},
		"failedStepId":  "run-1/block/test",
		"failureReason": strings.Repeat("x", maxLLMContextStringBytes+16),
	})
	if _, ok := input["executionContext"]; ok {
		t.Fatalf("executionContext should be dropped from compact input")
	}
	if _, ok := input[stepInputKeyExecutionContextPackage]; ok {
		t.Fatalf("executionContextPackage should be dropped from compact input")
	}
	if _, ok := input[stepInputKeyWorkspaceFileManifest]; ok {
		t.Fatalf("workspaceFileManifest should be dropped from compact input")
	}
	for _, key := range []string{"failedStepInput", "failedStepOutput", "previousStepOutput"} {
		if _, ok := input[key]; ok {
			t.Fatalf("%s should be dropped from compact input", key)
		}
	}
	if got := fmt.Sprint(input["failureReason"]); len(got) > maxLLMContextStringBytes+3 {
		t.Fatalf("failureReason length = %d, want truncated", len(got))
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	if len(actionPackage) == 0 {
		t.Fatalf("repairActionPackage = %#v, want retained actionable package", input["repairActionPackage"])
	}
	if targets := mapSliceFromValue(actionPackage["targetSnapshots"]); len(targets) != 1 || targets[0]["targetRef"] != "main.go" {
		t.Fatalf("repairActionPackage.targetSnapshots = %#v, want compact target snapshot", actionPackage["targetSnapshots"])
	}

	output := compactStepOutputForLLMContext(map[string]any{
		"stderr":       strings.Repeat("e", maxLLMContextStringBytes+10),
		"failureClass": failureClassShellCommand,
		"provider":     map[string]any{"id": "old-provider", "baseUrl": "http://127.0.0.1:8090/v1"},
		"contextBudget": map[string]any{
			"actualBytes": 1200,
			"providerId":  "old-provider",
			"model":       "old-model",
			"profile":     map[string]any{"maxInputTokens": 1},
		},
		"llmUsage": map[string]any{
			"inputTokens": 10,
			"providerId":  "old-provider",
			"provider":    "compatible",
			"model":       "old-model",
		},
		"nested": map[string]any{
			"executionContext":                  map[string]any{"large": "value"},
			stepInputKeyExecutionContextPackage: map[string]any{"large": "value"},
			"failedStepInput":                   map[string]any{"large": "value"},
		},
	})
	if output["failureClass"] != failureClassShellCommand {
		t.Fatalf("failureClass = %v, want preserved", output["failureClass"])
	}
	if output["provider"] != nil || output["providerId"] != nil {
		t.Fatalf("output = %+v, want provider binding stripped", output)
	}
	if budget := mapFromValue(output["contextBudget"]); budget["providerId"] != nil || budget["model"] != nil || budget["profile"] != nil {
		t.Fatalf("contextBudget = %+v, want provider binding stripped", budget)
	}
	if usage := mapFromValue(output["llmUsage"]); usage["providerId"] != nil || usage["provider"] != nil || usage["model"] != nil {
		t.Fatalf("llmUsage = %+v, want provider binding stripped", usage)
	}
	stderr := mapFromValue(output["stderr"])
	if len(stderr) == 0 ||
		stderr["truncated"] != true ||
		int64FromValue(stderr["bytes"]) <= maxLLMContextEvidenceTextBytes ||
		!strings.HasSuffix(fmt.Sprint(stderr["textPreview"]), "...") {
		t.Fatalf("stderr = %#v, want compact evidence text summary", output["stderr"])
	}
	nested := mapFromValue(output["nested"])
	if _, ok := nested[stepInputKeyExecutionContextPackage]; ok {
		t.Fatalf("nested executionContextPackage should be dropped")
	}
	if _, ok := nested["executionContext"]; ok {
		t.Fatalf("nested executionContext should be dropped")
	}
	if _, ok := nested["failedStepInput"]; ok {
		t.Fatalf("nested failedStepInput should be dropped")
	}
}

func TestRunExecutionInputKeepsOnlySlimExecutionContext(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-compact-execution-context",
		WorkspaceID: "ws-compact-execution-context",
		GoalID:      "goal-compact",
		DoDID:       "dod-compact",
		ExecutionContext: map[string]any{
			"dialogLanguage": "zh-CN",
			"llmProviderId":  "reasoning",
			"skills":         []map[string]any{{"id": "skill-compact", "instruction": strings.Repeat("x", 1024)}},
			"compressedContext": map[string]any{
				"version": "workorch.compressedContext.v1",
				"mode":    "deterministic_recovery_summary",
			},
		},
	}

	input := runExecutionInput(run, "run-compact-execution-context/step-1")

	if input["workspaceId"] != "ws-compact-execution-context" || input["goalId"] != "goal-compact" || input["doDId"] != "dod-compact" {
		t.Fatalf("input = %+v, want run identity", input)
	}
	executionContext := mapFromValue(input["executionContext"])
	if executionContext["dialogLanguage"] != "zh-CN" || executionContext["llmProviderId"] != nil {
		t.Fatalf("executionContext = %+v, want slim control fields without persisted LLM provider binding", executionContext)
	}
	if _, ok := executionContext["skills"]; ok {
		t.Fatalf("executionContext retained raw skills: %+v", executionContext["skills"])
	}
	if compressed := mapFromValue(executionContext["compressedContext"]); compressed["version"] != "workorch.compressedContext.v1" {
		t.Fatalf("compressedContext = %+v, want compact compressed context", compressed)
	}
}

func TestInitialStepInputRequiresStateChangingRepairForReplanContinuation(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-replan-state-changing-repair",
		WorkspaceID:     "ws-replan-state-changing-repair",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-replan-state-changing",
				"reason":           "previous repair returned read-only graph",
			},
		},
	}

	input := initialStepInput(run)

	if input[stepInputKeySoulStage] != soulStageExecution ||
		input[stepInputKeyRuntimeStage] != runtimeStageRepair ||
		input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] != true ||
		input["replanStateChangingContinuation"] != true {
		t.Fatalf("initial replan input = %+v, want state-changing repair constraints", input)
	}
	guidance := stringSliceFromValue(input["repairGuidance"])
	joined := strings.Join(guidance, "\n")
	if !strings.Contains(joined, "state-changing output") || !strings.Contains(joined, "shell-only executionGraph") {
		t.Fatalf("repairGuidance = %#v, want state-changing repair guidance", guidance)
	}
	executionContext := mapFromValue(input["executionContext"])
	if len(mapFromValue(executionContext["replanContinuation"])) == 0 {
		t.Fatalf("executionContext = %+v, want compact replan continuation", executionContext)
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
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			stepInputKeySoulStage:        soulStageValidation,
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
	if pkg["stepId"] != string(step.ID) || pkg["stage"] != runtimeStageVerification {
		t.Fatalf("executionContextPackage = %+v, want current step identity", pkg)
	}
	if pkg["stageMode"] != "strict_convergence" {
		t.Fatalf("executionContextPackage stageMode = %v, want strict_convergence", pkg["stageMode"])
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

func TestRuntimeOrchestrator_ExecutionContextPackageIncludesManifestForReplanStateChangingContinuation(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := newCountingWorkspaceRepository()
	workspaceID := common.WorkspaceID("ws-context-package-replan-manifest")
	workingDir := t.TempDir()
	for _, path := range []string{"README.md", "go.mod"} {
		if err := os.WriteFile(filepath.Join(workingDir, path), []byte("content"), 0o644); err != nil {
			t.Fatalf("seed workspace file %s: %v", path, err)
		}
	}
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDir, State: common.WorkspaceStateRunning}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	orch := &RuntimeOrchestrator{workspaces: workspaceRepo}
	runID := common.RunID("run-context-package-replan-manifest")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		ExecutionContext: map[string]any{
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-context-package-replan-manifest-old",
				"ticketContext": map[string]any{
					"failureSignature": "class=runtime_failed|kind=llm/workspace_execution",
				},
			},
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
		},
	}

	pkg := orch.runExecutionContextPackage(ctx, run, step, workspace)

	if got := workspaceRepo.getCount.Load(); got != 0 {
		t.Fatalf("workspace Get count = %d, want 0 when workspace is already provided", got)
	}
	manifest := mapSliceFromValue(pkg["workspaceFileManifest"])
	if len(manifest) != 2 || manifest[0]["path"] != "README.md" || manifest[1]["path"] != "go.mod" {
		t.Fatalf("workspaceFileManifest = %+v, want current workspace manifest for replan state-changing continuation", pkg["workspaceFileManifest"])
	}
	replan := mapFromValue(pkg["replanContinuation"])
	if mapFromValue(replan["ticketContext"])["failureSignature"] == "" {
		t.Fatalf("replanContinuation = %+v, want failure context retained", replan)
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
	if got := workspaceRepo.getCount.Load(); got != 2 {
		t.Fatalf("workspace Get count = %d, want initial StartRun read plus final active-run reconcile", got)
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
	if got := workspaceRepo.getCount.Load(); got != 2 {
		t.Fatalf("workspace Get count = %d, want resume state sync read plus final active-run reconcile", got)
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
		case strings.HasSuffix(string(request.StepID), "-node-a"):
			return map[string]any{"status": "completed", "node": "a"}, nil
		case strings.HasSuffix(string(request.StepID), "-node-b"):
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
	if got := workspaceRepo.getCount.Load(); got != 2 {
		t.Fatalf("workspace Get count = %d, want graph batch reuse plus final active-run reconcile", got)
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

func TestRuntimeOrchestrator_ExecutionContextPackageRefreshesCurrentRunHighValueMemory(t *testing.T) {
	ctx := context.Background()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewMemoryRecallTraceRepository()
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			MemoryItems:        memoryRepo,
			MemoryRecallTraces: traceRepo,
		},
	}
	workspaceID := common.WorkspaceID("ws-context-package-memory-refresh")
	runID := common.RunID("run-context-package-memory-refresh")
	oldRunID := common.RunID("run-context-package-memory-old")
	for idx := 1; idx <= 4; idx++ {
		if err := memoryRepo.Create(ctx, &memorymodel.MemoryItem{
			ID:          common.MemoryID(fmt.Sprintf("%s/memory/old-summary-%d", oldRunID, idx)),
			WorkspaceID: workspaceID,
			RunID:       oldRunID,
			SourceType:  memorycommon.MemorySourceRun,
			SourceRef:   string(oldRunID),
			MemoryType:  memorycommon.MemoryTypeRunSummary,
			Summary:     fmt.Sprintf("old process summary %d", idx),
			Content:     "old runtime process summary",
			Importance:  90,
			Confidence:  90,
			Visibility:  "workspace",
		}); err != nil {
			t.Fatalf("seed old memory %d: %v", idx, err)
		}
	}
	currentMemories := []*memorymodel.MemoryItem{
		{
			ID:          common.MemoryID(string(runID) + "/memory/dod-feedback"),
			WorkspaceID: workspaceID,
			RunID:       runID,
			SourceType:  memorycommon.MemorySourceRun,
			SourceRef:   string(runID) + "/assessment",
			MemoryType:  memorycommon.MemoryTypeDoDFeedback,
			Summary:     "DoD gaps require tests",
			Content:     "DoD gaps: missing verification evidence.",
			Importance:  50,
			Confidence:  50,
			Visibility:  "workspace",
		},
		{
			ID:          common.MemoryID(string(runID) + "/memory/execution-lesson"),
			WorkspaceID: workspaceID,
			RunID:       runID,
			SourceType:  memorycommon.MemorySourceRun,
			SourceRef:   string(runID) + "/repair",
			MemoryType:  memorycommon.MemoryTypeExecutionLesson,
			Summary:     "Repair must add verification",
			Content:     "Next repair should produce tests and evidence.",
			Importance:  50,
			Confidence:  50,
			Visibility:  "workspace",
		},
	}
	for _, item := range currentMemories {
		if err := memoryRepo.Create(ctx, item); err != nil {
			t.Fatalf("seed current memory %s: %v", item.ID, err)
		}
	}
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: workspaceID,
		ExecutionContext: map[string]any{
			"memories": []map[string]any{
				{
					"id":         string(oldRunID) + "/memory/activation-only",
					"runId":      string(oldRunID),
					"memoryType": memorycommon.MemoryTypeRunSummary,
					"summary":    "activation old summary",
				},
			},
			"memoryRetrieval": map[string]any{
				"policy": "activation_static",
				"status": "selected",
			},
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeySoulStage:             soulStageExecution,
			stepInputKeyRuntimeRepairFollowUp: true,
		},
	}

	orch.enrichLLMStepInputContext(ctx, run, step, nil)

	pkg := mapFromValue(step.Input[stepInputKeyExecutionContextPackage])
	selected := mapFromValue(pkg["selectedContext"])
	memories := mapSliceFromValue(selected["memories"])
	if !memoryContextContains(memories, string(runID)+"/memory/dod-feedback") ||
		!memoryContextContains(memories, string(runID)+"/memory/execution-lesson") {
		t.Fatalf("selected memories = %+v, want current run high-value memories", memories)
	}
	oldRunSummaryCount := 0
	for _, item := range memories {
		if item["runId"] == string(oldRunID) && item["memoryType"] == memorycommon.MemoryTypeRunSummary {
			oldRunSummaryCount++
		}
	}
	if oldRunSummaryCount > 1 {
		t.Fatalf("selected memories = %+v, want at most one old run_summary", memories)
	}
	retrieval := mapFromValue(selected["memoryRetrieval"])
	if retrieval["policy"] != stepInputMemorySelectionPolicy || retrieval["refreshSource"] != "repair" {
		t.Fatalf("memoryRetrieval = %+v, want dynamic repair refresh", retrieval)
	}
	trace, err := traceRepo.Get(ctx, common.MemoryRecallTraceID(string(step.ID)+"/memory-recall/step-input"))
	if err != nil {
		t.Fatalf("get recall trace: %v", err)
	}
	if !memoryIDsContain(trace.RetrievedMemoryIDs, common.MemoryID(string(runID)+"/memory/dod-feedback")) {
		t.Fatalf("trace retrieved ids = %+v, want current dod feedback", trace.RetrievedMemoryIDs)
	}
	if trace.RefreshSource != "repair" ||
		trace.SelectionPolicyVersion != stepInputMemorySelectionVersion ||
		!memoryIDsContain(trace.CandidateMemoryIDs, common.MemoryID(string(runID)+"/memory/dod-feedback")) ||
		!memoryIDsContain(trace.SelectedMemoryIDs, common.MemoryID(string(runID)+"/memory/dod-feedback")) ||
		len(trace.ExcludedMemoryIDs) == 0 ||
		trace.TypeQuota[memorycommon.MemoryTypeRunSummary] != 1 ||
		len(trace.SelectionScores) == 0 {
		t.Fatalf("trace selection fields = %+v, want structured selection evidence", trace)
	}
}

func TestStepInputMemorySelectionDowngradesSupersededRunMemory(t *testing.T) {
	currentRunID := common.RunID("run-memory-selection-current")
	completedRunID := common.RunID("run-memory-selection-completed")
	supersededRunID := common.RunID("run-memory-selection-superseded")
	selected, candidates, _ := selectStepInputMemoryItems([]*memorymodel.MemoryItem{
		{
			ID:         "memory-superseded-summary",
			RunID:      supersededRunID,
			MemoryType: memorycommon.MemoryTypeRunSummary,
			Summary:    "superseded summary",
			Importance: 90,
			Confidence: 90,
			Visibility: "workspace",
		},
		{
			ID:         "memory-completed-summary",
			RunID:      completedRunID,
			MemoryType: memorycommon.MemoryTypeRunSummary,
			Summary:    "completed summary",
			Importance: 90,
			Confidence: 90,
			Visibility: "workspace",
		},
	}, currentRunID, map[common.RunID]common.RunState{
		completedRunID:  common.RunStateCompleted,
		supersededRunID: common.RunStateSuperseded,
	}, stepInputMemoryLimit)

	if len(selected) != 1 || selected[0].RunID != completedRunID {
		t.Fatalf("selected = %+v, want completed run summary over superseded run summary", selected)
	}
	if len(candidates) != 2 || candidates[0].RunID != completedRunID {
		t.Fatalf("candidates = %+v, want completed run ranked before superseded run", candidates)
	}
	reason := stepInputMemoryScoreReason(candidates[1], currentRunID, map[common.RunID]common.RunState{supersededRunID: common.RunStateSuperseded})
	if !strings.Contains(reason, "run_state=superseded") || !strings.Contains(reason, "run_state_downgraded") {
		t.Fatalf("reason = %q, want superseded run downgrade explanation", reason)
	}
}

func TestStepInputMemorySelectionLimitsSingleHistoricalRunDominance(t *testing.T) {
	currentRunID := common.RunID("run-memory-selection-current")
	firstHistoricalRunID := common.RunID("run-memory-selection-history-a")
	secondHistoricalRunID := common.RunID("run-memory-selection-history-b")
	items := []*memorymodel.MemoryItem{
		{
			ID:         "history-a-dod",
			RunID:      firstHistoricalRunID,
			MemoryType: memorycommon.MemoryTypeDoDFeedback,
			Summary:    "history a dod gap",
			Importance: 90,
			Confidence: 90,
			Visibility: "workspace",
		},
		{
			ID:         "history-a-lesson",
			RunID:      firstHistoricalRunID,
			MemoryType: memorycommon.MemoryTypeExecutionLesson,
			Summary:    "history a repair lesson",
			Importance: 90,
			Confidence: 90,
			Visibility: "workspace",
		},
		{
			ID:         "history-a-context",
			RunID:      firstHistoricalRunID,
			MemoryType: memorycommon.MemoryTypeProjectContext,
			Summary:    "history a project context",
			Importance: 90,
			Confidence: 90,
			Visibility: "workspace",
		},
		{
			ID:         "history-a-decision",
			RunID:      firstHistoricalRunID,
			MemoryType: memorycommon.MemoryTypeDecision,
			Summary:    "history a decision",
			Importance: 90,
			Confidence: 90,
			Visibility: "workspace",
		},
		{
			ID:         "history-b-skill",
			RunID:      secondHistoricalRunID,
			MemoryType: memorycommon.MemoryTypeSkillFeedback,
			Summary:    "history b skill context",
			Importance: 80,
			Confidence: 80,
			Visibility: "workspace",
		},
		{
			ID:         "history-b-tool",
			RunID:      secondHistoricalRunID,
			MemoryType: memorycommon.MemoryTypeToolFeedback,
			Summary:    "history b tool context",
			Importance: 80,
			Confidence: 80,
			Visibility: "workspace",
		},
	}

	selected, _, excluded := selectStepInputMemoryItems(items, currentRunID, map[common.RunID]common.RunState{
		firstHistoricalRunID:  common.RunStateCompleted,
		secondHistoricalRunID: common.RunStateCompleted,
	}, stepInputMemoryLimit)

	firstHistoricalSelected := 0
	for _, item := range selected {
		if item.RunID == firstHistoricalRunID {
			firstHistoricalSelected++
		}
	}
	if firstHistoricalSelected > stepInputHistoricalRunMemoryMax {
		t.Fatalf("selected = %+v, want at most %d memories from one historical run", selected, stepInputHistoricalRunMemoryMax)
	}
	if !memoryItemsContainRun(selected, secondHistoricalRunID) {
		t.Fatalf("selected = %+v, want other historical run to remain represented", selected)
	}
	if !memoryItemsContainRun(excluded, firstHistoricalRunID) {
		t.Fatalf("excluded = %+v, want excess first historical run memories excluded", excluded)
	}
}

func TestRuntimeOrchestrator_StepInputMemoryTraceRecordsExcludedProviderRetryMemory(t *testing.T) {
	ctx := context.Background()
	memoryRepo := memory.NewMemoryItemRepository()
	traceRepo := memory.NewMemoryRecallTraceRepository()
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			MemoryItems:        memoryRepo,
			MemoryRecallTraces: traceRepo,
		},
	}
	workspaceID := common.WorkspaceID("ws-provider-retry-memory-trace")
	runID := common.RunID("run-provider-retry-memory-trace")
	memoryID := common.MemoryID(string(runID) + "/provider-retry-001/memory/1")
	if err := memoryRepo.Create(ctx, &memorymodel.MemoryItem{
		ID:          memoryID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		SourceType:  memorycommon.MemorySourceRun,
		SourceRef:   string(runID),
		MemoryType:  memorycommon.MemoryTypeToolFeedback,
		Summary:     "Provider retry outcome captured",
		Content:     "Provider retry attempt: 1\nfailureClass: provider_timeout",
		Tags:        []string{"provider_retry", "tool_context", "necessary_output"},
		Importance:  90,
		Confidence:  90,
		Visibility:  "workspace",
	}); err != nil {
		t.Fatalf("seed provider retry memory: %v", err)
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeySoulStage:             soulStageExecution,
			stepInputKeyRuntimeRepairFollowUp: true,
		},
	}

	orch.enrichLLMStepInputContext(ctx, run, step, nil)

	pkg := mapFromValue(step.Input[stepInputKeyExecutionContextPackage])
	selected := mapFromValue(pkg["selectedContext"])
	if memories := mapSliceFromValue(selected["memories"]); len(memories) != 0 {
		t.Fatalf("selected memories = %+v, want provider retry process memory excluded", memories)
	}
	retrieval := mapFromValue(selected["memoryRetrieval"])
	if retrieval["status"] != "none_selected" || retrieval["refreshSource"] != "repair" {
		t.Fatalf("memoryRetrieval = %+v, want none_selected repair trace", retrieval)
	}
	trace, err := traceRepo.Get(ctx, common.MemoryRecallTraceID(string(step.ID)+"/memory-recall/step-input"))
	if err != nil {
		t.Fatalf("get recall trace: %v", err)
	}
	if !memoryIDsContain(trace.CandidateMemoryIDs, memoryID) ||
		memoryIDsContain(trace.SelectedMemoryIDs, memoryID) ||
		!memoryIDsContain(trace.ExcludedMemoryIDs, memoryID) ||
		trace.SelectionPolicyVersion != stepInputMemorySelectionVersion ||
		len(trace.SelectionScores) == 0 {
		t.Fatalf("trace = %+v, want provider retry memory candidate excluded with scores", trace)
	}
	if !strings.Contains(trace.SelectionScores[0].Reason, "excluded_low_value_process_memory") {
		t.Fatalf("selection score reason = %q, want low-value process exclusion", trace.SelectionScores[0].Reason)
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
		case strings.HasSuffix(string(request.StepID), "-write-source"):
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
		case strings.Contains(string(request.StepID), "/graph-") && strings.HasSuffix(string(request.StepID), "-run-tests"):
			return map[string]any{
				"status":    "failed",
				"stage":     "shell_command_completed",
				"exitCode":  1,
				"stderr":    "missing go.sum entry for module providing package github.com/spf13/viper",
				"graphHint": "dependency metadata missing",
			}, errors.New("exit status 1")
		case strings.Contains(string(request.StepID), "/repair-"):
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
	failedStep, err := stepRepo.Get(ctx, graphScopedStepID(runID, 1, 2, "run-tests"))
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
		case strings.HasSuffix(stepID, "-write-buggy-source"):
			return map[string]any{
				"status": "completed",
				"patches": []map[string]any{{
					"patchType": "file_write",
					"targetRef": "calculator.go",
					"content":   "package calculator\n\nfunc Add(a, b int) int { return a - b }\n",
					"summary":   "write initial implementation",
				}},
			}, nil
		case strings.Contains(stepID, "/graph-") && strings.HasSuffix(stepID, "-run-tests-fixed"):
			return map[string]any{
				"status":   "completed",
				"stage":    "shell_command_completed",
				"command":  "go test ./...",
				"exitCode": 0,
				"stdout":   "ok calculator 0.003s",
			}, nil
		case strings.Contains(stepID, "/graph-") && strings.HasSuffix(stepID, "-run-tests"):
			return map[string]any{
				"status":   "failed",
				"stage":    "shell_command_completed",
				"command":  "go test ./...",
				"exitCode": 1,
				"stderr":   "calculator_test.go: Add(2,3)= -1, want 5",
			}, errors.New("exit status 1")
		case strings.Contains(stepID, "/repair-"):
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
		case strings.Contains(stepID, "/graph-") && strings.HasSuffix(stepID, "-fix-source"):
			return map[string]any{
				"status": "completed",
				"patches": []map[string]any{{
					"patchType": "unified_diff",
					"targetRef": "calculator.go",
					"diff":      "--- a/calculator.go\n+++ b/calculator.go\n@@ -1,3 +1,3 @@\n package calculator\n \n-func Add(a, b int) int { return a - b }\n+func Add(a, b int) int { return a + b }\n",
					"summary":   "fix Add implementation",
				}},
			}, nil
		case strings.Contains(stepID, "/follow-up-graph-"):
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
							"evidenceRefs": []string{"run-software-dev-code-agent/graph-002/001-fix-source"},
						},
						{
							"item":         "tests pass",
							"state":        "satisfied",
							"evidenceRefs": []string{"run-software-dev-code-agent/graph-002/002-run-tests-fixed"},
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
		if strings.Contains(string(request.StepID), "/repair-") {
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
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on provider decision", runItem)
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
	ticket, err := ticketRepo.Get(ctx, runItem.BlockingTicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.Context[decisionContextKeyResultPolicy] != decisionContextResultPolicyNone ||
		ticket.Context[decisionContextKeyScope] != decisionContextScopeWorkorchRuntime {
		t.Fatalf("ticket context = %+v, want provider config decision without business result", ticket.Context)
	}
	if ticket.Context["attempts"] != 0 || ticket.Context["maxAttempts"] != maxProviderRetryAttempts {
		t.Fatalf("ticket attempts = %+v/%+v, want actual attempts with max budget", ticket.Context["attempts"], ticket.Context["maxAttempts"])
	}
}

func TestRuntimeOrchestrator_RetriesLLMProviderStreamErrorBeforeConfigBlock(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-stream-error-retry")
	failedStepID := common.StepID(runID + "/repair-002-of-graph-001-004-build")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		runtime: &stubRuntimeAdapter{},
		steps:   stepRepo,
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxProviderRetryAttempts: 10}),
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-provider-stream-error-retry"),
		DoDID:           common.DoDID("dod-provider-stream-error-retry"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			"stateChangingOutputRequired":     true,
			"repairObjective":                 repairObjectiveStateChangingOutput,
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "llm_provider_stream_error",
			"error":                   "invalid argument: llm provider streamed response content is empty",
			failureOutputKeyClass:     failureClassProviderUnavailable,
			failureOutputKeySignature: "class=provider_unavailable|stage=llm_provider_stream_error",
			"streamDiagnostics": map[string]any{
				"doneReceived":       true,
				"receivedBytes":      0,
				"receivedDeltaCount": 0,
				"streamIncomplete":   false,
				"receivedEventCount": 1,
			},
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, failedStep)
	if err != nil {
		t.Fatalf("nextDispatchSpecsFromStep: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want provider retry spec", specs)
	}
	if specs[0].ID != providerRetryStepID(runID, failedStepID, 1) ||
		specs[0].Input[stepInputKeyProviderRetryFollowUp] != true ||
		specs[0].Input[stepInputKeyProviderRetryAttempt] != 1 {
		t.Fatalf("spec = %+v, want first provider retry for stream error", specs[0])
	}
	if specs[0].Input["stateChangingOutputRequired"] != true ||
		specs[0].Input["repairObjective"] != repairObjectiveStateChangingOutput {
		t.Fatalf("retry input = %+v, want state-changing repair contract preserved", specs[0].Input)
	}
}

func TestRuntimeOrchestrator_RetriesTransientProviderUnavailableBeforeConfigBlock(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-unavailable-retry")
	failedStepID := common.StepID(runID + "/step-1")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		runtime: &stubRuntimeAdapter{},
		steps:   stepRepo,
		policy:  NormalizeRuntimePolicy(RuntimePolicy{MaxProviderRetryAttempts: 10}),
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-provider-unavailable-retry"),
		DoDID:           common.DoDID("dod-provider-unavailable-retry"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "llm_provider_unavailable",
			"error":                   "local error: tls: bad record MAC",
			failureOutputKeyClass:     failureClassProviderUnavailable,
			failureOutputKeySignature: "class=provider_unavailable|stage=llm_provider_unavailable|error=local error: tls: bad record mac",
			"resolutionHint":          "check LLM provider baseURL, model, API key, protocol compatibility, and network availability before rerunning the workspace",
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, failedStep)
	if err != nil {
		t.Fatalf("nextDispatchSpecsFromStep: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want provider retry spec", specs)
	}
	if specs[0].ID != providerRetryStepID(runID, failedStepID, 1) ||
		specs[0].Input[stepInputKeyProviderRetryFollowUp] != true ||
		specs[0].Input[stepInputKeyProviderRetryAttempt] != 1 {
		t.Fatalf("spec = %+v, want first provider retry for transient provider unavailable", specs[0])
	}
}

func TestRuntimeOrchestrator_RetriesLLMProviderTimeoutBeforeFailingRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	attempts := 0
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		attempts++
		if attempts <= maxProviderRetryAttempts {
			return map[string]any{
				"status": "failed",
				"stage":  "llm_provider_timeout",
				"error":  "invalid argument: openai llm provider error: context deadline exceeded",
			}, fmt.Errorf("%w: openai llm provider error: context deadline exceeded", common.ErrInvalidArgument)
		}
		if request.Input[stepInputKeyProviderRetryFollowUp] != true || request.Input[stepInputKeyProviderRetryAttempt] != maxProviderRetryAttempts {
			t.Fatalf("provider retry input = %+v, want final retry metadata", request.Input)
		}
		return map[string]any{
			"status": "completed",
			"dodAssessment": map[string]any{
				"satisfied":   true,
				"summary":     "provider recovered and work completed",
				"goalOutcome": "provider recovered and work completed",
				"items": []map[string]any{{
					"item":         "work completes",
					"state":        "satisfied",
					"evidenceRefs": []string{string(request.StepID)},
				}},
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
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-llm-provider-timeout-retry")
	runID := common.RunID("run-llm-provider-timeout-retry")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-llm-provider-timeout-retry"),
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
		t.Fatalf("run = %+v, want completed after provider retry", runItem)
	}
	if attempts != maxProviderRetryAttempts+1 {
		t.Fatalf("attempts = %d, want initial + retries", attempts)
	}
}

func TestRuntimeOrchestrator_BlocksOnDecisionAfterProviderTimeoutRetriesExhausted(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		return map[string]any{
			"status": "failed",
			"stage":  "llm_provider_timeout",
			"error":  "invalid argument: openai llm provider error: context deadline exceeded",
		}, fmt.Errorf("%w: openai llm provider error: context deadline exceeded", common.ErrInvalidArgument)
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
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-llm-provider-timeout-block")
	runID := common.RunID("run-llm-provider-timeout-block")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-llm-provider-timeout-block"),
		State:           common.RunStateCreated,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"dialogLanguage": "zh-CN",
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
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on provider recovery decision", runItem)
	}
	ticket, err := ticketRepo.Get(ctx, runItem.BlockingTicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateOpen || ticket.Context["attempts"] != maxProviderRetryAttempts {
		t.Fatalf("ticket = %+v, want provider retry exhaustion context", ticket)
	}
	if ticket.Context[decisionContextKeyScope] != decisionContextScopeWorkorchRuntime ||
		ticket.Context[decisionContextKeyDependency] != decisionContextDependencyLLMProvider ||
		ticket.Context[decisionContextKeyRecoveryPolicy] != decisionContextRecoveryPolicyConfigOnly {
		t.Fatalf("ticket context = %+v, want WorkOrch runtime provider recovery scope", ticket.Context)
	}
	if !strings.Contains(ticket.Question, "WorkOrch runtime LLM provider") || !strings.Contains(ticket.Question, "WorkOrch 系统配置") || !strings.Contains(ticket.Question, "API Key") {
		t.Fatalf("ticket question = %q, want provider configuration recovery prompt", ticket.Question)
	}
	if _, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("provider retry exhaustion result err = %v, want no business rejected result", err)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairAttemptCountIncludesDispatchedProviderInfrastructureFailures(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-budget-provider-failures")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}

	seedSteps := []*runmodel.Step{
		{
			ID:    common.StepID(runID + "/provider-misrouted-repair"),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				failureOutputKeyClass:             failureClassProviderTimeout,
			},
			Output: map[string]any{"status": "completed"},
		},
		{
			ID:    common.StepID(runID + "/repair-provider-timeout"),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				failureOutputKeyClass:             failureClassRuntime,
			},
			Output: map[string]any{
				"stage":               "llm_provider_timeout",
				failureOutputKeyClass: failureClassProviderTimeout,
			},
		},
		{
			ID:    common.StepID(runID + "/real-runtime-repair"),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				failureOutputKeyClass:             failureClassRuntime,
			},
			Output: map[string]any{
				"stage":               "llm_execution_graph_plan",
				failureOutputKeyClass: failureClassRuntime,
			},
		},
	}
	for _, step := range seedSteps {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}

	if got := orch.runtimeRepairAttemptCount(ctx, runID); got != len(seedSteps) {
		t.Fatalf("runtimeRepairAttemptCount = %d, want every dispatched runtime repair attempt counted", got)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairAttemptCountIsScopedByFailureSignature(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-budget-scoped-signature")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-a-%d", runID, attempt)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfFailureClass:  failureClassRuntime,
				stepInputKeyRepairOfFailureSig:    "class=runtime|target=a",
			},
			Output: map[string]any{"status": "completed"},
		}); err != nil {
			t.Fatalf("seed scoped repair %d: %v", attempt, err)
		}
	}

	failedA := &runmodel.Step{
		ID:    common.StepID(runID + "/block/a"),
		RunID: runID,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime|target=a",
		},
	}
	failedB := &runmodel.Step{
		ID:    common.StepID(runID + "/block/b"),
		RunID: runID,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime|target=b",
		},
	}

	if got := orch.runtimeRepairAttemptCountForStep(ctx, runID, failedA); got != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("attempt count for A = %d, want exhausted scope", got)
	}
	if got := orch.runtimeRepairAttemptCountForStep(ctx, runID, failedB); got != 0 {
		t.Fatalf("attempt count for B = %d, want independent scope", got)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairScopeUsesCurrentFailureBeforeInheritedLineage(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-budget-current-signature")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/old-repair-%d", runID, attempt)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
				stepInputKeyRepairOfFailureSig:    "class=shell_command_failed|command=verify|failureTargets=old-target",
			},
			Output: map[string]any{"status": "completed"},
		}); err != nil {
			t.Fatalf("seed old repair %d: %v", attempt, err)
		}
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(runID + "/block/verify"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRepairOfFailureClass: failureClassShellCommand,
			stepInputKeyRepairOfFailureSig:   "class=shell_command_failed|command=verify|failureTargets=old-target",
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|command=verify|failureTargets=new-target",
		},
	}

	if got := orch.runtimeRepairAttemptCountForStep(ctx, runID, failedStep); got != 0 {
		t.Fatalf("attempt count = %d, want current output failure signature to ignore inherited lineage attempts", got)
	}
	if got := orch.runtimeRepairAttemptCountForEffectiveScope(ctx, runID, failedStep); got != 0 {
		t.Fatalf("effective attempt count = %d, want current output failure target to ignore inherited lineage attempts", got)
	}
	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{ID: runID, WorkspaceID: common.WorkspaceID("ws-current-signature")}, failedStep, 1, nil)
	if input[stepInputKeyRepairOfFailureSig] != "class=shell_command_failed|command=verify|failureTargets=new-target" {
		t.Fatalf("repair input signature = %#v, want current output failure signature", input[stepInputKeyRepairOfFailureSig])
	}
	if input[stepInputKeyRecoveryAction] != string(runtimeRecoveryActionRuntimeRepair) ||
		input[stepInputKeyRecoveryRootStepID] != string(failedStep.ID) ||
		input[stepInputKeyRecoveryLineageType] != "runtime_repair" {
		t.Fatalf("repair lineage = %+v, want explicit runtime repair lineage", input)
	}
	scope := mapFromValue(input[stepInputKeyRecoveryBudgetScope])
	if scope["lineageType"] != "runtime_repair" ||
		scope["rootStepId"] != string(failedStep.ID) ||
		scope["failureClass"] != failureClassShellCommand ||
		scope["failureSignature"] != "class=shell_command_failed|command=verify|failureTargets=new-target" {
		t.Fatalf("repair budget scope = %+v, want scoped repair lineage", scope)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairScopeUsesCurrentFailureWithoutTargetScope(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-budget-current-signature-no-target")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	oldSignature := "class=shell_command_failed|kind=shell/command|node=init|command=tool|exitCode=1|error=exit status 1"
	currentSignature := "class=shell_command_failed|kind=shell/command|node=verify|command=tool|exitCode=-1|error=context deadline exceeded"
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001-of-init"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
			stepInputKeyRepairOfFailureSig:    oldSignature,
		},
		Output: map[string]any{"status": "completed"},
	}); err != nil {
		t.Fatalf("seed old repair: %v", err)
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-002/003-verify"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:         runtimeStageExecution,
			stepInputKeyGraphNodeID:          "verify",
			stepInputKeyGraphRootStepID:      string(runID) + "/repair-001-of-init",
			stepInputKeyRepairOfStepID:       string(runID) + "/graph-001/002-init",
			stepInputKeyRepairOfRootStepID:   string(runID) + "/step-1",
			stepInputKeyRepairOfFailureClass: failureClassShellCommand,
			stepInputKeyRepairOfFailureSig:   oldSignature,
		},
		Output: map[string]any{
			"status":                  "timeout",
			"error":                   "context deadline exceeded",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: currentSignature,
		},
	}

	if got := orch.runtimeRepairAttemptCountForStep(ctx, runID, failedStep); got != 0 {
		t.Fatalf("attempt count = %d, want current output failure signature to ignore inherited repair signature", got)
	}
	input := orch.runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: common.WorkspaceID("ws-current-signature-no-target"),
		DoDID:       common.DoDID("dod-current-signature-no-target"),
	}, failedStep, 1, errors.New("context deadline exceeded"))
	if input[stepInputKeyRepairOfFailureSig] != currentSignature || input[failureOutputKeySignature] != currentSignature {
		t.Fatalf("repair input signature = repair:%#v failure:%#v, want current output signature", input[stepInputKeyRepairOfFailureSig], input[failureOutputKeySignature])
	}
	if input["repairObjective"] != repairObjectiveTransientExecutionRetry || input["stateChangingOutputRequired"] != nil {
		t.Fatalf("repair input = %+v, want transient retry without state-changing requirement", input)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairEffectiveScopeCountsFailureTargetAcrossGraphNodes(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-target-scope")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps:   stepRepo,
		tickets: memory.NewDecisionTicketRepository(),
	}
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		node := fmt.Sprintf("target-scope-repair-%02d", attempt)
		signature := fmt.Sprintf("class=shell_command_failed|kind=shell/command|node=%s|failureTargets=stream_e2e_test.go|exitCode=1", node)
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-%d", runID, attempt)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
				stepInputKeyRepairOfFailureSig:    signature,
			},
			Output: map[string]any{"status": "completed"},
		}); err != nil {
			t.Fatalf("seed target repair %d: %v", attempt, err)
		}
	}

	failedStep := &runmodel.Step{
		ID:    common.StepID(runID + "/graph-013/002-run-stream-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=run-stream-test|failureTargets=stream_e2e_test.go|exitCode=1",
		},
	}
	if got := orch.runtimeRepairAttemptCountForStep(ctx, runID, failedStep); got != 0 {
		t.Fatalf("exact attempt count = %d, want node-specific signature to remain independent", got)
	}
	if got := orch.runtimeRepairAttemptCountForEffectiveScope(ctx, runID, failedStep); got != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("effective attempt count = %d, want target scope exhausted", got)
	}
	if !orch.shouldEscalateRuntimeRepairExhaustion(ctx, &runmodel.Run{ID: runID, DoDID: common.DoDID("dod-target-scope")}, failedStep) {
		t.Fatalf("target-scoped repeated failure should escalate repair exhaustion")
	}

	otherTarget := &runmodel.Step{
		ID:    common.StepID(runID + "/graph-014/002-run-other-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=run-other-test|failureTargets=other_target_test.go|exitCode=1",
		},
	}
	if got := orch.runtimeRepairAttemptCountForEffectiveScope(ctx, runID, otherTarget); got != 0 {
		t.Fatalf("other target effective attempt count = %d, want independent target scope", got)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairTargetScopeUsesCurrentGraphWindow(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-target-window")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps:   stepRepo,
		tickets: memory.NewDecisionTicketRepository(),
	}
	signature := "class=shell_command_failed|kind=shell/command|node=build|failureTargets=main.go|exitCode=1|error=exit status 1"
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-old-%d", runID, attempt)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfRootStepID:    string(runID) + "/old-graph-root",
				stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
				stepInputKeyRepairOfFailureSig:    signature,
			},
			Output: map[string]any{"status": "completed"},
		}); err != nil {
			t.Fatalf("seed old target repair %d: %v", attempt, err)
		}
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(runID + "/graph-009/006-build"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphRootStepID: string(runID) + "/new-graph-root",
			stepInputKeyGraphNodeID:     "build",
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: signature,
			"error":                   "exit status 1",
		},
	}

	if got := orch.runtimeRepairAttemptCountForEffectiveScope(ctx, runID, failedStep); got != 0 {
		t.Fatalf("effective attempt count = %d, want old graph-root attempts excluded", got)
	}
	if orch.shouldEscalateRuntimeRepairExhaustion(ctx, &runmodel.Run{ID: runID, DoDID: common.DoDID("dod-target-window")}, failedStep) {
		t.Fatalf("new graph-root target failure should receive a fresh repair window")
	}

	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/repair-new-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfRootStepID:    string(runID) + "/new-graph-root",
			stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
			stepInputKeyRepairOfFailureSig:    signature,
		},
		Output: map[string]any{"status": "completed"},
	}); err != nil {
		t.Fatalf("seed current target repair: %v", err)
	}
	if got := orch.runtimeRepairAttemptCountForEffectiveScope(ctx, runID, failedStep); got != 1 {
		t.Fatalf("effective attempt count = %d, want current graph-root target attempt counted", got)
	}
}

func TestRuntimeOrchestrator_ResumeClearsStaleRuntimeRepairExhaustionBlocker(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "completed"}}
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
	workspaceID := common.WorkspaceID("ws-stale-repair-blocker")
	runID := common.RunID("run-stale-repair-blocker")
	ticketID := common.TicketID("ticket-stale-repair-blocker")
	signature := "class=runtime|target=stale"
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateWaitingDecision, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID(runID + "/block/original"),
		State:       common.DecisionTicketStateOpen,
		Context: map[string]any{
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
			"failedStepId":                   string(runID + "/block/original"),
			"failureClass":                   failureClassRuntime,
			"failureSignature":               signature,
		},
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/block/repair-success"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateCompleted,
		Input: map[string]any{
			"previousStepId":                 string(runID + "/block/original"),
			stepInputKeyGraphNodeID:          "repair-success",
			stepInputKeyRepairOfStepID:       string(runID + "/block/original"),
			stepInputKeyRepairOfFailureClass: failureClassRuntime,
			stepInputKeyRepairOfFailureSig:   signature,
		},
		Output: map[string]any{
			"status":               "completed",
			stepOutputKeyNextStep:  string(runID + "/step-after-stale-repair"),
			stepOutputKeyNextKind:  llmcommon.StepKindWorkspaceExecution,
			stepOutputKeyNextInput: map[string]any{"fromStaleRepairEvidence": true},
		},
	}); err != nil {
		t.Fatalf("seed repair success evidence: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume stale repair blocker: %v", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.BlockingTicketID != "" || runItem.State != common.RunStateCompleted {
		t.Fatalf("run = %+v, want stale blocker cleared and completed", runItem)
	}
}

func TestRuntimeOrchestrator_ResumeClearsRepairExhaustionWhenTargetWindowHasBudget(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	runtime := &requestCapturingRuntimeAdapter{output: map[string]any{
		"status": "completed",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "repair completed in current graph window",
			"items": []map[string]any{{
				"item":         "current graph repair",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-repair-window-resume/repair-001-of-graph-009-006-build"},
			}},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Results: memory.NewResultRepository()},
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-repair-window-resume")
	runID := common.RunID("run-repair-window-resume")
	failedStepID := common.StepID(runID + "/graph-009/006-build")
	ticketID := common.TicketID(string(failedStepID) + "-repair-exhausted-decision")
	signature := "class=shell_command_failed|kind=shell/command|node=build|failureTargets=main.go|exitCode=1|error=exit status 1"
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateWaitingDecision, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		DoDID:            common.DoDID("dod-repair-window-resume"),
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-old-%d", runID, attempt)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfRootStepID:    string(runID) + "/old-graph-root",
				stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
				stepInputKeyRepairOfFailureSig:    signature,
			},
			Output: map[string]any{"status": "completed"},
		}); err != nil {
			t.Fatalf("seed old repair %d: %v", attempt, err)
		}
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphRootStepID: string(runID) + "/new-graph-root",
			stepInputKeyGraphNodeID:     "build",
		},
		Output: map[string]any{
			"status":                  "failed",
			"error":                   "exit status 1",
			"stderr":                  "./main.go:177:10: declared and not used: choice",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: signature,
		},
	}); err != nil {
		t.Fatalf("seed failed build step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:           ticketID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		StepID:       failedStepID,
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Reason:       "runtime repair exhausted",
		Question:     "provide repair strategy",
		Context: map[string]any{
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
			"failedStepId":                   string(failedStepID),
			"failureClass":                   failureClassShellCommand,
			"failureSignature":               signature,
			"failureTargetScope":             "failureTargets=main.go",
			"attempts":                       defaultMaxRuntimeRepairAttempts,
		},
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume repair exhaustion with fresh target window: %v", err)
	}
	ticket, err := ticketRepo.Get(ctx, ticketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateDismissed {
		t.Fatalf("ticket state = %s, want dismissed stale repair exhaustion blocker", ticket.State)
	}
	if len(runtime.requests) != 1 || !boolValue(runtime.requests[0].Input[stepInputKeyRuntimeRepairFollowUp]) {
		t.Fatalf("runtime requests = %+v, want one runtime repair dispatch", runtime.requests)
	}
	if runtime.requests[0].Input["targetRepairAttempts"] != 0 {
		t.Fatalf("targetRepairAttempts = %v, want old graph attempts excluded", runtime.requests[0].Input["targetRepairAttempts"])
	}
}

func TestRuntimeRepairMCPFailureTargetUsesCompositeRunBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-mcp-target-budget")
	signature := "class=mcp_runtime_failed|kind=mcp/tool_call|stage=mcp_tool_call_server_unavailable|serverId=external-capability|tool=lookup.current|error=not found"
	if got := runtimeFailureTargetScope(signature); got != "serverId=external-capability|tool=lookup.current" {
		t.Fatalf("runtimeFailureTargetScope = %q, want composite server/tool target", got)
	}
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo, tickets: memory.NewDecisionTicketRepository()}
	for idx, root := range []string{"graph-001", "graph-002"} {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-%03d", runID, idx+1)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyGraphRootStepID:       string(runID) + "/" + root,
				stepInputKeyRepairOfFailureClass:  failureClassMCPRuntime,
				stepInputKeyRepairOfFailureSig:    signature,
			},
		}); err != nil {
			t.Fatalf("seed repair step: %v", err)
		}
	}
	priorFailed := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-002/001-capability-call"),
		RunID: runID,
		Kind:  "mcp/tool_call",
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassMCPRuntime,
			failureOutputKeySignature: signature,
		},
	}
	if err := stepRepo.Create(ctx, priorFailed); err != nil {
		t.Fatalf("seed prior failed step: %v", err)
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-003/001-capability-call"),
		RunID: runID,
		Kind:  "mcp/tool_call",
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphRootStepID: string(runID) + "/graph-003",
		},
		Output: map[string]any{
			"stage":                   "mcp_tool_call_server_unavailable",
			"serverId":                "external-capability",
			"tool":                    "lookup.current",
			"error":                   "not found",
			failureOutputKeyClass:     failureClassMCPRuntime,
			failureOutputKeySignature: signature,
		},
	}

	if attempts := orch.runtimeRepairAttemptCountForEffectiveScope(ctx, runID, failedStep); attempts != 2 {
		t.Fatalf("effective attempts = %d, want same external capability target counted across graph roots", attempts)
	}
	if !orch.shouldBlockOnDuplicateRuntimeFailure(ctx, &runmodel.Run{ID: runID, DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}, failedStep) {
		t.Fatalf("duplicate external capability target should block before repeating the same call again")
	}
}

func TestRuntimeOrchestrator_ResumeClearsDuplicateFailureBlockerBelowScopedBudget(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	runtime := &requestCapturingRuntimeAdapter{output: map[string]any{"status": "completed"}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-stale-duplicate-blocker")
	runID := common.RunID("run-stale-duplicate-blocker")
	failedStepID := common.StepID(runID + "/block/verify")
	ticketID := common.TicketID(string(failedStepID) + "-duplicate-failure-decision")
	signature := "class=shell_command_failed|kind=shell/command|node=verify|failureTargets=config/settings.yaml|exitCode=1"
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateWaitingDecision, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		DoDID:            common.DoDID("dod-stale-duplicate-blocker"),
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/repair-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
			stepInputKeyRepairOfFailureSig:    signature,
		},
		Output: map[string]any{"status": "completed"},
	}); err != nil {
		t.Fatalf("seed repair step: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: signature,
			"error":                   "exit status 1",
		},
	}); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      failedStepID,
		State:       common.DecisionTicketStateOpen,
		Context: map[string]any{
			"failedStepId":     string(failedStepID),
			"failureClass":     failureClassShellCommand,
			"failureSignature": signature,
		},
	}); err != nil {
		t.Fatalf("seed duplicate ticket: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume duplicate blocker below scoped budget: %v", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.BlockingTicketID != "" {
		t.Fatalf("run = %+v, want duplicate blocker cleared", runItem)
	}
	if len(runtime.requests) == 0 || !boolValue(runtime.requests[0].Input[stepInputKeyRuntimeRepairFollowUp]) {
		t.Fatalf("runtime requests = %+v, want runtime repair dispatch", runtime.requests)
	}
	if runtime.requests[0].Input[stepInputKeyRepairOfFailureSig] != signature {
		t.Fatalf("repair signature = %#v, want %q", runtime.requests[0].Input[stepInputKeyRepairOfFailureSig], signature)
	}
}

func TestRuntimeOrchestrator_BlockerAutoResumePolicyMatrix(t *testing.T) {
	type expected struct {
		err              error
		state            common.RunState
		blockingTicket   bool
		blockingApproval bool
		runtimeCalls     int
	}
	tests := []struct {
		name string
		seed func(ctx context.Context, t *testing.T, orch *RuntimeOrchestrator, repos blockerPolicyRepos) common.RunID
		want expected
	}{
		{
			name: "duplicate failure below scoped repair budget can resume",
			seed: func(ctx context.Context, t *testing.T, _ *RuntimeOrchestrator, repos blockerPolicyRepos) common.RunID {
				t.Helper()
				workspaceID := common.WorkspaceID("ws-policy-duplicate")
				runID := common.RunID("run-policy-duplicate")
				failedStepID := common.StepID(runID + "/block/verify")
				ticketID := common.TicketID(string(failedStepID) + "-duplicate-failure-decision")
				signature := "class=shell_command_failed|kind=shell/command|node=verify|failureTargets=config/settings.yaml|exitCode=1"
				seedBlockerPolicyWorkspaceAndRun(ctx, t, repos, workspaceID, runID, common.RunStateBlockedOnDecision, ticketID, "")
				if err := repos.steps.Create(ctx, &runmodel.Step{
					ID:    common.StepID(runID + "/repair-1"),
					RunID: runID,
					Kind:  llmcommon.StepKindWorkspaceExecution,
					State: common.StepStateCompleted,
					Input: map[string]any{
						stepInputKeyRuntimeRepairFollowUp: true,
						stepInputKeyRuntimeRepairAttempt:  1,
						stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
						stepInputKeyRepairOfFailureSig:    signature,
					},
					Output: map[string]any{"status": "completed"},
				}); err != nil {
					t.Fatalf("seed repair step: %v", err)
				}
				if err := repos.steps.Create(ctx, &runmodel.Step{
					ID:    failedStepID,
					RunID: runID,
					Kind:  shellcommon.StepKindCommand,
					State: common.StepStateFailed,
					Output: map[string]any{
						failureOutputKeyClass:     failureClassShellCommand,
						failureOutputKeySignature: signature,
					},
				}); err != nil {
					t.Fatalf("seed failed step: %v", err)
				}
				if err := repos.tickets.Create(ctx, &decisionmodel.Ticket{
					ID:          ticketID,
					WorkspaceID: workspaceID,
					RunID:       runID,
					StepID:      failedStepID,
					State:       common.DecisionTicketStateOpen,
					Context: map[string]any{
						"failureClass":     failureClassShellCommand,
						"failureSignature": signature,
						"failedStepId":     string(failedStepID),
					},
				}); err != nil {
					t.Fatalf("seed duplicate ticket: %v", err)
				}
				return runID
			},
			want: expected{state: common.RunStateCompleted, runtimeCalls: 1},
		},
		{
			name: "runtime repair exhaustion without success evidence stays blocked",
			seed: func(ctx context.Context, t *testing.T, _ *RuntimeOrchestrator, repos blockerPolicyRepos) common.RunID {
				t.Helper()
				workspaceID := common.WorkspaceID("ws-policy-repair-exhausted")
				runID := common.RunID("run-policy-repair-exhausted")
				failedStepID := common.StepID(runID + "/block/verify")
				ticketID := common.TicketID(string(failedStepID) + "-repair-exhausted-decision")
				signature := "class=runtime_failed|target=verification"
				seedBlockerPolicyWorkspaceAndRun(ctx, t, repos, workspaceID, runID, common.RunStateBlockedOnDecision, ticketID, "")
				if err := repos.steps.Create(ctx, &runmodel.Step{
					ID:    failedStepID,
					RunID: runID,
					Kind:  shellcommon.StepKindCommand,
					State: common.StepStateFailed,
					Output: map[string]any{
						failureOutputKeyClass:     failureClassRuntime,
						failureOutputKeySignature: signature,
					},
				}); err != nil {
					t.Fatalf("seed failed step: %v", err)
				}
				if err := repos.tickets.Create(ctx, &decisionmodel.Ticket{
					ID:          ticketID,
					WorkspaceID: workspaceID,
					RunID:       runID,
					StepID:      failedStepID,
					State:       common.DecisionTicketStateOpen,
					Context: map[string]any{
						decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
						"failureClass":                   failureClassRuntime,
						"failureSignature":               signature,
						"failedStepId":                   string(failedStepID),
					},
				}); err != nil {
					t.Fatalf("seed repair exhaustion ticket: %v", err)
				}
				return runID
			},
			want: expected{err: common.ErrBlocked, state: common.RunStateBlockedOnDecision, blockingTicket: true},
		},
		{
			name: "provider retry exhaustion stays blocked for explicit runtime provider recovery",
			seed: func(ctx context.Context, t *testing.T, _ *RuntimeOrchestrator, repos blockerPolicyRepos) common.RunID {
				t.Helper()
				workspaceID := common.WorkspaceID("ws-policy-provider-exhausted")
				runID := common.RunID("run-policy-provider-exhausted")
				failedStepID := common.StepID(runID + "/block/provider")
				ticketID := common.TicketID(string(failedStepID) + "-provider-retry-exhausted-decision")
				seedBlockerPolicyWorkspaceAndRun(ctx, t, repos, workspaceID, runID, common.RunStateBlockedOnDecision, ticketID, "")
				if err := repos.steps.Create(ctx, &runmodel.Step{
					ID:    failedStepID,
					RunID: runID,
					Kind:  llmcommon.StepKindWorkspaceExecution,
					State: common.StepStateFailed,
					Output: map[string]any{
						"stage":                   "llm_provider_timeout",
						failureOutputKeyClass:     failureClassProviderTimeout,
						failureOutputKeySignature: "class=provider_timeout|stage=llm_provider_timeout",
					},
				}); err != nil {
					t.Fatalf("seed provider failed step: %v", err)
				}
				if err := repos.tickets.Create(ctx, &decisionmodel.Ticket{
					ID:          ticketID,
					WorkspaceID: workspaceID,
					RunID:       runID,
					StepID:      failedStepID,
					State:       common.DecisionTicketStateOpen,
					Context: map[string]any{
						decisionContextKeyScope:          decisionContextScopeWorkorchRuntime,
						decisionContextKeyDependency:     decisionContextDependencyLLMProvider,
						decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyConfigOnly,
					},
				}); err != nil {
					t.Fatalf("seed provider exhaustion ticket: %v", err)
				}
				return runID
			},
			want: expected{err: common.ErrBlocked, state: common.RunStateBlockedOnDecision, blockingTicket: true},
		},
		{
			name: "pending approval stays blocked",
			seed: func(ctx context.Context, t *testing.T, _ *RuntimeOrchestrator, repos blockerPolicyRepos) common.RunID {
				t.Helper()
				workspaceID := common.WorkspaceID("ws-policy-approval")
				runID := common.RunID("run-policy-approval")
				approvalID := common.ApprovalID("approval-policy-pending")
				seedBlockerPolicyWorkspaceAndRun(ctx, t, repos, workspaceID, runID, common.RunStateBlockedOnApproval, "", approvalID)
				if err := repos.approvals.Create(ctx, &approvalmodel.Approval{
					ID:          approvalID,
					WorkspaceID: workspaceID,
					RunID:       runID,
					State:       common.ApprovalStatePending,
				}); err != nil {
					t.Fatalf("seed approval: %v", err)
				}
				return runID
			},
			want: expected{err: common.ErrBlocked, state: common.RunStateBlockedOnApproval, blockingApproval: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repos := newBlockerPolicyRepos()
			runtime := &requestCapturingRuntimeAdapter{output: map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied":   true,
					"summary":     "blocker policy resumed and completed",
					"goalOutcome": "blocker policy resumed and completed",
					"items": []map[string]any{{
						"item":         "blocker policy verification",
						"state":        "satisfied",
						"evidenceRefs": []string{"policy-step"},
					}},
				},
			}}
			orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
				runtime,
				repos.runs,
				repos.steps,
				repos.workspaces,
				repos.tickets,
				repos.resolutions,
				repos.approvals,
				RuntimeEvidenceRepositories{
					ToolCalls:          memory.NewToolCallRepository(),
					ValidationResults:  memory.NewValidationResultRepository(),
					RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
				},
				events.NewMemoryHub(),
			)
			runID := tt.seed(ctx, t, orch, repos)

			err := orch.ResumeRun(ctx, runID)
			if tt.want.err != nil {
				if !errors.Is(err, tt.want.err) {
					t.Fatalf("ResumeRun err = %v, want %v", err, tt.want.err)
				}
			} else if err != nil {
				t.Fatalf("ResumeRun err = %v", err)
			}
			runItem, err := repos.runs.Get(ctx, runID)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			if runItem.State != tt.want.state {
				t.Fatalf("run state = %q, want %q", runItem.State, tt.want.state)
			}
			if (runItem.BlockingTicketID != "") != tt.want.blockingTicket {
				t.Fatalf("blocking ticket = %q, want present=%v", runItem.BlockingTicketID, tt.want.blockingTicket)
			}
			if (runItem.BlockingApprovalID != "") != tt.want.blockingApproval {
				t.Fatalf("blocking approval = %q, want present=%v", runItem.BlockingApprovalID, tt.want.blockingApproval)
			}
			if len(runtime.requests) != tt.want.runtimeCalls {
				t.Fatalf("runtime calls = %d, want %d", len(runtime.requests), tt.want.runtimeCalls)
			}
		})
	}
}

type blockerPolicyRepos struct {
	workspaces  *memory.WorkspaceRepository
	runs        *memory.RunRepository
	steps       *memory.StepRepository
	tickets     *memory.DecisionTicketRepository
	resolutions *memory.DecisionResolutionRepository
	approvals   *memory.ApprovalRepository
}

func newBlockerPolicyRepos() blockerPolicyRepos {
	return blockerPolicyRepos{
		workspaces:  memory.NewWorkspaceRepository(),
		runs:        memory.NewRunRepository(),
		steps:       memory.NewStepRepository(),
		tickets:     memory.NewDecisionTicketRepository(),
		resolutions: memory.NewDecisionResolutionRepository(),
		approvals:   memory.NewApprovalRepository(),
	}
}

func seedBlockerPolicyWorkspaceAndRun(
	ctx context.Context,
	t *testing.T,
	repos blockerPolicyRepos,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	state common.RunState,
	ticketID common.TicketID,
	approvalID common.ApprovalID,
) {
	t.Helper()
	if err := repos.workspaces.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateWaitingDecision,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:                 runID,
		WorkspaceID:        workspaceID,
		DoDID:              common.DoDID("dod-" + string(runID)),
		State:              state,
		InitialStepKind:    llmcommon.StepKindWorkspaceExecution,
		BlockingTicketID:   ticketID,
		BlockingApprovalID: approvalID,
	}
	if err := repos.runs.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
}

func TestRuntimeOrchestrator_ResumeDispatchForProviderFailureUsesProviderRetryWithinBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-resume-provider-retry")
	failedStepID := common.StepID(runID + "/llm-provider-step")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{runtime: &stubRuntimeAdapter{}, steps: stepRepo}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-resume-provider-retry"),
		DoDID:           common.DoDID("dod-resume-provider-retry"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, failedStep)
	if err != nil {
		t.Fatalf("nextDispatchSpecsFromStep: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want one provider retry spec", specs)
	}
	if specs[0].Input[stepInputKeyProviderRetryFollowUp] != true ||
		specs[0].Input[stepInputKeyProviderRetryAttempt] != 1 ||
		specs[0].Input[stepInputKeyRuntimeRepairFollowUp] == true {
		t.Fatalf("spec input = %+v, want provider retry within budget", specs[0].Input)
	}
}

func TestRuntimeOrchestrator_ResumeDispatchForProviderFailureStopsAfterRetryBudget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-resume-provider-retry-exhausted")
	failedStepID := common.StepID(runID + "/llm-provider-step")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{runtime: &stubRuntimeAdapter{}, steps: stepRepo}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-resume-provider-retry-exhausted"),
		DoDID:           common.DoDID("dod-resume-provider-retry-exhausted"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}
	for attempt := 1; attempt <= maxProviderRetryAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    providerRetryStepID(runID, failedStepID, attempt),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyProviderRetryFollowUp: true,
				stepInputKeyProviderRetryAttempt:  attempt,
				stepInputKeyProviderRetryRootStep: string(failedStepID),
				"failedStepId":                    string(failedStepID),
			},
			Output: map[string]any{
				"stage":               "llm_provider_timeout",
				failureOutputKeyClass: failureClassProviderTimeout,
			},
		}); err != nil {
			t.Fatalf("seed provider retry %d: %v", attempt, err)
		}
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, failedStep)
	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("nextDispatchSpecsFromStep err = %v, want blocked after provider retry budget", err)
	}
	if len(specs) != 0 {
		t.Fatalf("specs = %+v, want no max+1 provider retry or runtime repair", specs)
	}
}

func TestRuntimeOrchestrator_DispatchNextBlocksProviderFailureAfterRetryBudget(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-dispatch-provider-retry-exhausted")
	runID := common.RunID("run-dispatch-provider-retry-exhausted")
	rootStepID := common.StepID(runID + "/llm-provider-step")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-dispatch-provider-retry-exhausted"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    rootStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}); err != nil {
		t.Fatalf("seed root step: %v", err)
	}
	for attempt := 1; attempt <= maxProviderRetryAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    providerRetryStepID(runID, rootStepID, attempt),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyProviderRetryFollowUp: true,
				stepInputKeyProviderRetryAttempt:  attempt,
				stepInputKeyProviderRetryRootStep: string(rootStepID),
				"failedStepId":                    string(rootStepID),
			},
			Output: map[string]any{
				"stage":               "llm_provider_timeout",
				failureOutputKeyClass: failureClassProviderTimeout,
			},
		}); err != nil {
			t.Fatalf("seed provider retry %d: %v", attempt, err)
		}
	}

	if err := orch.DispatchNextStep(ctx, runID); !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("DispatchNextStep err = %v, want blocked", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on provider retry exhaustion decision", runItem)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	for _, step := range steps {
		if step != nil && strings.Contains(string(step.ID), "provider-retry-3") {
			t.Fatalf("steps = %+v, want no provider retry beyond budget", steps)
		}
	}
}

func TestRuntimeOrchestrator_ResumeResolvedProviderRetryExhaustionReblocksWithoutFailing(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	runtime := &requestCapturingRuntimeAdapter{output: map[string]any{"status": "completed"}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{Results: resultRepo},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-resolved-provider-exhaustion-reblock")
	runID := common.RunID("run-resolved-provider-exhaustion-reblock")
	rootStepID := common.StepID(runID + "/repair-001-of-planning-001")
	failedStepID := providerRetryStepID(runID, rootStepID, maxProviderRetryAttempts)
	ticketID := common.TicketID(runtimeChildID(failedStepID, "provider-retry-exhausted-decision"))
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
		DoDID:            "dod-resolved-provider-exhaustion-reblock",
		State:            common.RunStateBlockedOnDecision,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	for attempt := 1; attempt <= maxProviderRetryAttempts; attempt++ {
		stepID := providerRetryStepID(runID, rootStepID, attempt)
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    stepID,
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyProviderRetryFollowUp: true,
				stepInputKeyProviderRetryAttempt:  attempt,
				stepInputKeyProviderRetryRootStep: string(rootStepID),
				stepInputKeyRecoveryAction:        string(runtimeRecoveryActionProviderRetry),
				stepInputKeyRecoveryRootStepID:    string(rootStepID),
				stepInputKeyRecoveryLineageType:   "provider_retry",
			},
			Output: map[string]any{
				"stage":                   "llm_provider_timeout",
				"error":                   "context deadline exceeded",
				failureOutputKeyClass:     failureClassProviderTimeout,
				failureOutputKeySignature: "class=provider_timeout|stage=llm_provider_timeout",
			},
		}); err != nil {
			t.Fatalf("seed provider retry %d: %v", attempt, err)
		}
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:           ticketID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		StepID:       failedStepID,
		State:        common.DecisionTicketStateResolved,
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Context: map[string]any{
			decisionContextKeyScope:          decisionContextScopeWorkorchRuntime,
			decisionContextKeyDependency:     decisionContextDependencyLLMProvider,
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyConfigOnly,
			decisionContextKeyResultPolicy:   decisionContextResultPolicyNone,
		},
	}); err != nil {
		t.Fatalf("seed resolved ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-provider-exhaustion-continue",
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"answer": "continue"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	err := orch.ResumeRun(ctx, runID)
	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("ResumeRun err = %v, want blocked on renewed provider decision", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" || runItem.BlockingTicketID == ticketID || runItem.FailureReason != "" {
		t.Fatalf("run = %+v, want reblocked with a new provider ticket and no failure", runItem)
	}
	newTicket, err := ticketRepo.Get(ctx, runItem.BlockingTicketID)
	if err != nil {
		t.Fatalf("get renewed ticket: %v", err)
	}
	if newTicket.Context[decisionContextKeyResultPolicy] != decisionContextResultPolicyNone ||
		newTicket.Context["previousTicketId"] != string(ticketID) ||
		newTicket.Context["repeatBlocker"] != true {
		t.Fatalf("renewed ticket context = %+v, want repeated provider blocker metadata", newTicket.Context)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("results = %+v, want no business rejected result for provider blocker", results)
	}
	if len(runtime.requests) != 0 {
		t.Fatalf("runtime calls = %d, want no LLM dispatch while provider retry budget remains exhausted", len(runtime.requests))
	}
}

func TestRuntimeOrchestrator_ResumeDispatchForContractFailureUsesRuntimeRepair(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-resume-contract-failure")
	failedStepID := common.StepID(runID + "/step-1")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{runtime: &stubRuntimeAdapter{}, steps: stepRepo}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-resume-contract-failure"),
		DoDID:           common.DoDID("dod-resume-contract-failure"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{},
		Output: map[string]any{
			"stage":               "llm_execution_graph_plan",
			"error":               "state-changing planning llm step returned read-only executionGraph without patch output",
			failureOutputKeyClass: failureClassRuntime,
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, failedStep)
	if err != nil {
		t.Fatalf("nextDispatchSpecsFromStep: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want runtime repair spec", specs)
	}
	if specs[0].Input[stepInputKeyProviderRetryFollowUp] == true ||
		specs[0].Input[stepInputKeyRuntimeRepairFollowUp] != true {
		t.Fatalf("spec input = %+v, want runtime repair without provider retry", specs[0].Input)
	}
}

func TestRuntimeOrchestrator_ResumeDispatchForLLMDecisionInvalidUsesRuntimeRepair(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-resume-llm-decision-invalid")
	failedStepID := common.StepID(runID + "/step-1")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{runtime: &stubRuntimeAdapter{}, steps: stepRepo}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-resume-llm-decision-invalid"),
		DoDID:           common.DoDID("dod-resume-llm-decision-invalid"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{},
		Output: map[string]any{
			"stage":               "llm_schema_invalid",
			"error":               "invalid argument: response must be valid LLM decision JSON",
			failureOutputKeyClass: failureClassLLMDecisionInvalid,
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, failedStep)
	if err != nil {
		t.Fatalf("nextDispatchSpecsFromStep: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want runtime repair spec", specs)
	}
	if specs[0].Input[stepInputKeyProviderRetryFollowUp] == true ||
		specs[0].Input[stepInputKeyRuntimeRepairFollowUp] != true {
		t.Fatalf("spec input = %+v, want LLM decision contract repair without provider retry", specs[0].Input)
	}
}

func TestRuntimeOrchestrator_ProviderRetryAttemptCountIgnoresSuccessfulProviderRetries(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-retry-budget-successful-retry")
	rootStepID := common.StepID(runID + "/llm-provider-root")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    providerRetryStepID(runID, rootStepID, 1),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryAttempt:  1,
			"failedStepId":                    string(rootStepID),
		},
		Output: map[string]any{"stage": "llm_execution_graph_plan"},
	}); err != nil {
		t.Fatalf("seed successful retry: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    providerRetryStepID(runID, rootStepID, 2),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryAttempt:  2,
			"failedStepId":                    string(rootStepID),
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}); err != nil {
		t.Fatalf("seed failed retry: %v", err)
	}

	if got := orch.providerRetryAttemptCount(ctx, runID, rootStepID); got != 1 {
		t.Fatalf("providerRetryAttemptCount = %d, want only failed provider retry attempts", got)
	}
}

func TestRuntimeOrchestrator_ProviderRetryInputDoesNotCarryRuntimeRepairMarkers(t *testing.T) {
	rootStepID := common.StepID("run-provider-retry-input/root")
	input := providerRetryStepInput(&runmodel.Step{
		ID:    rootStepID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  3,
			"previousStepId":                  "previous",
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}, 1, errors.New("provider timeout"))

	if input[stepInputKeyProviderRetryFollowUp] != true ||
		input[stepInputKeyRuntimeRepairFollowUp] != nil ||
		input[stepInputKeyRuntimeRepairAttempt] != nil {
		t.Fatalf("provider retry input = %+v, want provider markers without runtime repair markers", input)
	}
	if input[stepInputKeyRuntimeStage] != runtimeStageRepair {
		t.Fatalf("provider retry runtimeStage = %v, want repair", input[stepInputKeyRuntimeStage])
	}
	if input[stepInputKeyRecoveryAction] != string(runtimeRecoveryActionProviderRetry) ||
		input[stepInputKeyRecoveryRootStepID] != string(rootStepID) ||
		input[stepInputKeyRecoveryLineageType] != "provider_retry" {
		t.Fatalf("provider retry lineage = %+v, want explicit recovery lineage", input)
	}
	scope := mapFromValue(input[stepInputKeyRecoveryBudgetScope])
	if scope["lineageType"] != "provider_retry" || scope["rootStepId"] != string(rootStepID) || scope["failureClass"] != failureClassProviderTimeout {
		t.Fatalf("provider retry budget scope = %+v, want provider retry root scope", scope)
	}
}

func TestProviderRetryStepInputKeepsStateChangingRepairContract(t *testing.T) {
	failedStep := &runmodel.Step{
		ID:    "run-provider-retry-state-changing-repair/repair-001-of-planning-001",
		RunID: "run-provider-retry-state-changing-repair",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfRootStepID:    "run-provider-retry-state-changing-repair/step-1",
			stepInputKeyRepairOfStepID:        "run-provider-retry-state-changing-repair/step-1",
			"repairObjective":                 "produce_state_changing_output",
			"stateChangingOutputRequired":     true,
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
			"error":               "context deadline exceeded",
		},
	}

	input := providerRetryStepInput(failedStep, 1, errors.New("context deadline exceeded"))
	if input[stepInputKeyRuntimeRepairFollowUp] != nil || input[stepInputKeyRuntimeRepairAttempt] != nil {
		t.Fatalf("provider retry input = %+v, want retry markers without runtime repair dispatch markers", input)
	}
	if input["repairObjective"] != "produce_state_changing_output" ||
		input["stateChangingOutputRequired"] != true ||
		len(stringSliceFromValue(input["repairGuidance"])) == 0 {
		t.Fatalf("provider retry input = %+v, want state-changing repair contract preserved", input)
	}
	if input[stepInputKeyRuntimeStage] != runtimeStageRepair {
		t.Fatalf("provider retry runtimeStage = %v, want repair", input[stepInputKeyRuntimeStage])
	}
	if input[stepInputKeyRepairOfRootStepID] != "run-provider-retry-state-changing-repair/step-1" ||
		input[stepInputKeyRepairOfStepID] != "run-provider-retry-state-changing-repair/step-1" {
		t.Fatalf("provider retry repair lineage = %+v, want repair scope preserved", input)
	}
}

func TestRuntimePolicyConfiguresProviderRetryAttempts(t *testing.T) {
	if got := NormalizeRuntimePolicy(RuntimePolicy{}).MaxRuntimeRepairAttempts; got != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("default max runtime repair attempts = %d, want %d", got, defaultMaxRuntimeRepairAttempts)
	}
	if got := NormalizeRuntimePolicy(RuntimePolicy{}).MaxProviderRetryAttempts; got != maxProviderRetryAttempts {
		t.Fatalf("default max provider retry attempts = %d, want %d", got, maxProviderRetryAttempts)
	}
	orch := &RuntimeOrchestrator{
		policy: NormalizeRuntimePolicy(RuntimePolicy{MaxRuntimeRepairAttempts: 9, MaxProviderRetryAttempts: 7}),
	}
	repairInput := orch.runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{
		ID:          "run-policy",
		WorkspaceID: "ws-policy",
		DoDID:       "dod-policy",
	}, &runmodel.Step{
		ID:    "run-policy/failed-step",
		RunID: "run-policy",
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     "shell_command_failed",
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|command=go|exitCode=1|error=exit status 1",
		},
	}, 1, errors.New("exit status 1"))
	if got := repairInput["maxRuntimeRepairAttempts"]; got != 9 {
		t.Fatalf("maxRuntimeRepairAttempts = %v, want configured 9", got)
	}
	input := orch.providerRetryStepInput(&runmodel.Step{
		ID:    "run-policy/step-1",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage":                   "llm_provider_timeout",
			failureOutputKeyClass:     failureClassProviderTimeout,
			failureOutputKeySignature: "provider-timeout",
		},
	}, 1, errors.New("context deadline exceeded"))
	if got := input["maxProviderRetryAttempts"]; got != 7 {
		t.Fatalf("maxProviderRetryAttempts = %v, want configured 7", got)
	}
}

func TestProviderRetryStepInputNarrowsStateChangingProviderTimeout(t *testing.T) {
	failedStep := &runmodel.Step{
		ID:    "run-provider-retry-state-changing-timeout/step-1",
		RunID: "run-provider-retry-state-changing-timeout",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failedExecutionGraph":            map[string]any{"nodes": []any{map[string]any{"kind": "shell/command"}}},
			stepInputKeyGraphEvidence:         []map[string]any{{"nodeId": "inspect", "output": map[string]any{"stdout": strings.Repeat("x", 2048)}}},
			stepInputKeyWorkspaceFileManifest: []map[string]any{{"path": "README.md"}},
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			"status":              "failed",
			"error":               "context deadline exceeded",
			failureOutputKeyClass: failureClassProviderTimeout,
			"contextBudget":       map[string]any{"actualBytes": 9999},
			"llmUsage":            map[string]any{"inputTokens": 1000},
			"provider":            map[string]any{"baseUrl": "http://127.0.0.1:8090/v1"},
			"streamDiagnostics":   map[string]any{"streamIncomplete": true, "doneReceived": false},
			"streamEvents": []map[string]any{
				{"type": "delta", "delta": "{\"completionState\":\"continue\",\"executionGraph\":{\"nodes\":[{\"id\":\"write_go_mod\""},
			},
		},
	}

	input := providerRetryStepInput(failedStep, 1, errors.New("context deadline exceeded"))

	if input["stateChangingRecoveryMode"] != "direct_state_change" ||
		input["providerTimeoutStateChangingRetry"] != true {
		t.Fatalf("provider retry input = %+v, want direct state-changing recovery mode", input)
	}
	for _, key := range []string{"failedExecutionGraph", stepInputKeyGraphEvidence, stepInputKeyWorkspaceFileManifest, "previousStepOutput"} {
		if _, ok := input[key]; ok {
			t.Fatalf("provider retry input retained %s: %+v", key, input[key])
		}
	}
	failedOutput := mapFromValue(input["failedStepOutput"])
	if failedOutput["contextBudget"] != nil || failedOutput["llmUsage"] != nil || failedOutput["provider"] != nil {
		t.Fatalf("failedStepOutput = %+v, want provider timeout summary without heavy provider payload", failedOutput)
	}
	if !strings.Contains(fmt.Sprint(failedOutput["partialOutputPreview"]), "write_go_mod") {
		t.Fatalf("partialOutputPreview = %q, want partial state-changing output hint", failedOutput["partialOutputPreview"])
	}
	forbidden := stringSliceFromValue(input["forbiddenRecoveryOutputs"])
	if !containsString(forbidden, "run_snapshot_graph") || !containsString(forbidden, "context_rehydration_graph") {
		t.Fatalf("forbiddenRecoveryOutputs = %+v, want self-inspection outputs forbidden", forbidden)
	}
}

func TestProviderRetryStepInputRestoresStateChangingContractFromMissingDoDRepair(t *testing.T) {
	failedStep := &runmodel.Step{
		ID:    "run-provider-retry-dod-gap-repair/repair-001-of-planning-001",
		RunID: "run-provider-retry-dod-gap-repair",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfRootStepID:    "run-provider-retry-dod-gap-repair/step-1",
			stepInputKeyRepairOfStepID:        "run-provider-retry-dod-gap-repair/assessment",
			"missingDoDItems": []map[string]any{
				{"item": "produce state-changing result", "state": "failed"},
			},
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
			"error":               "context deadline exceeded",
		},
	}

	input := providerRetryStepInput(failedStep, 1, errors.New("context deadline exceeded"))
	if input["repairObjective"] != "produce_state_changing_output" ||
		input["stateChangingOutputRequired"] != true ||
		len(stringSliceFromValue(input["repairGuidance"])) == 0 {
		t.Fatalf("provider retry input = %+v, want state-changing contract restored from missing DoD repair", input)
	}
	if len(mapSliceFromValue(input["missingDoDItems"])) != 1 {
		t.Fatalf("missingDoDItems = %+v, want missing DoD carried into provider retry", input["missingDoDItems"])
	}
}

func TestProviderRetryStepInputRestoresStateChangingContractFromGatedGraphFailure(t *testing.T) {
	failedStep := &runmodel.Step{
		ID:    "run-provider-retry-gated-repair/repair-001-of-planning-001",
		RunID: "run-provider-retry-gated-repair",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfRootStepID:    "run-provider-retry-gated-repair/step-1",
			stepInputKeyRepairOfStepID:        "run-provider-retry-gated-repair/step-1",
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
			"error":               "invalid argument: state-changing executionGraph patch/apply depends on llm/workspace_execution before patch",
		},
	}

	input := providerRetryStepInput(failedStep, 1, errors.New("invalid argument: state-changing executionGraph patch/apply depends on llm/workspace_execution before patch"))
	if input["repairObjective"] != "produce_state_changing_output" || input["stateChangingOutputRequired"] != true {
		t.Fatalf("provider retry input = %+v, want state-changing contract restored from gated graph failure", input)
	}
	if input[stepInputKeyRuntimeStage] != runtimeStageRepair {
		t.Fatalf("provider retry runtimeStage = %v, want repair", input[stepInputKeyRuntimeStage])
	}
}

func TestRuntimeOrchestrator_RuntimeRepairBudgetCountsProviderTimeoutRepairDispatch(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-repair-budget-provider-timeout")
	failedStepID := common.StepID(runID + "/graph-001/005-implement")
	signature := "class=runtime_failed|targetRef=main.go"
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}

	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    runtimeRepairFollowUpStepID(runID, &runmodel.Step{ID: failedStepID}, 1),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			stepInputKeyRepairOfStepID:        string(failedStepID),
			stepInputKeyRepairOfRootStepID:    string(failedStepID),
			stepInputKeyRepairOfFailureClass:  failureClassRuntime,
			stepInputKeyRepairOfFailureSig:    signature,
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
			"error":               "context deadline exceeded",
		},
	}); err != nil {
		t.Fatalf("seed provider-timeout repair step: %v", err)
	}

	failedStep := &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: signature,
		},
	}
	if got := orch.runtimeRepairAttemptCountForStep(ctx, runID, failedStep); got != 1 {
		t.Fatalf("runtimeRepairAttemptCountForStep = %d, want provider-timeout repair dispatch counted", got)
	}
}

func TestProviderRetryRootStepIDDoesNotUseRepairFailedStepIDForNonRetryStep(t *testing.T) {
	repairStepID := common.StepID("run-provider-retry-root/zz-llm-repair-root-repair-1")
	step := &runmodel.Step{
		ID:   repairStepID,
		Kind: llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			"failedStepId":                    "run-provider-retry-root/step-1",
		},
	}

	if got := providerRetryRootStepID(step); got != repairStepID {
		t.Fatalf("providerRetryRootStepID = %q, want repair step id %q", got, repairStepID)
	}

	input := providerRetryStepInput(step, 1, errors.New("provider timeout"))
	if got := input[stepInputKeyProviderRetryRootStep]; got != string(repairStepID) {
		t.Fatalf("providerRetryRootStepId = %v, want %s", got, repairStepID)
	}
}

func TestRuntimeOrchestrator_ProviderRetryKeepsRootAcrossRetryFailure(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-retry-root-normalized")
	rootStepID := common.StepID("run-provider-retry-root-normalized/zz-llm-repair-root-repair-1")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    providerRetryStepID(runID, rootStepID, 1),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryAttempt:  1,
			stepInputKeyProviderRetryRootStep: string(rootStepID),
			"failedStepId":                    string(rootStepID),
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}); err != nil {
		t.Fatalf("seed retry step: %v", err)
	}
	failedRetry := &runmodel.Step{
		ID:    providerRetryStepID(runID, rootStepID, 1),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryAttempt:  1,
			stepInputKeyProviderRetryRootStep: string(rootStepID),
			"failedStepId":                    string(rootStepID),
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}

	if got := providerRetryRootStepID(failedRetry); got != rootStepID {
		t.Fatalf("providerRetryRootStepID = %q, want root %q", got, rootStepID)
	}
	if got := orch.providerRetryAttemptCount(ctx, runID, providerRetryRootStepID(failedRetry)); got != 1 {
		t.Fatalf("providerRetryAttemptCount = %d, want prior retry counted for same root", got)
	}
	nextID := providerRetryStepID(runID, providerRetryRootStepID(failedRetry), 2)
	if strings.Contains(string(nextID), "provider-retry-1-provider-retry-2") {
		t.Fatalf("provider retry step id = %q, want non-nested retry id", nextID)
	}
	if want := providerRetryStepID(runID, rootStepID, 2); nextID != want {
		t.Fatalf("provider retry step id = %q, want %q", nextID, want)
	}
}

func TestProviderRetryRootStepIDNormalizesRepairOfProviderRetryShell(t *testing.T) {
	runID := common.RunID("run-provider-retry-repair-shell-root")
	businessRootID := common.StepID(runID + "/step-1")
	retryStepID := providerRetryStepID(runID, common.StepID(runID+"/repair-001-of-planning-001"), 1)
	repairShellID := common.StepID(runID + "/repair-001-of-provider-retry-001")
	step := &runmodel.Step{
		ID:    repairShellID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfStepID:        string(retryStepID),
			stepInputKeyRepairOfRootStepID:    string(businessRootID),
			stepInputKeyRecoveryRootStepID:    string(businessRootID),
			"failedStepId":                    string(retryStepID),
		},
		Output: map[string]any{
			"stage":               "llm_provider_timeout",
			failureOutputKeyClass: failureClassProviderTimeout,
		},
	}

	if got := providerRetryRootStepID(step); got != businessRootID {
		t.Fatalf("providerRetryRootStepID = %q, want business root %q", got, businessRootID)
	}
	if got := runtimeRepairRootStepID(step); got != businessRootID {
		t.Fatalf("runtimeRepairRootStepID = %q, want business root %q", got, businessRootID)
	}
	nextID := providerRetryStepID(runID, providerRetryRootStepID(step), 1)
	if strings.Contains(string(nextID), "of-provider-retry-") {
		t.Fatalf("provider retry id = %q, want normalized readable root without provider retry shell", nextID)
	}
}

func TestRuntimeRepairFollowUpStepIDUsesBusinessRootForProviderRetryContractViolation(t *testing.T) {
	runID := common.RunID("run-repair-id-contract")
	businessRootID := common.StepID(runID + "/step-1")
	retryStepID := providerRetryStepID(runID, businessRootID, 1)
	failedRetry := &runmodel.Step{
		ID:    retryStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryRootStep: string(businessRootID),
			stepInputKeyRepairOfRootStepID:    string(businessRootID),
		},
		Output: map[string]any{
			"stage":               "llm_execution_graph_plan",
			failureOutputKeyClass: failureClassRuntime,
		},
	}

	stepID := runtimeRepairFollowUpStepID(runID, failedRetry, 1)
	if strings.Contains(strings.TrimPrefix(string(stepID), string(runID)+"/"), "provider-retry") {
		t.Fatalf("runtime repair step id = %q, want business root label without provider retry shell", stepID)
	}
	if !strings.Contains(string(stepID), "planning-001") {
		t.Fatalf("runtime repair step id = %q, want business root label", stepID)
	}
}

func TestRuntimeRepairFollowUpStepIDUsesGraphNodeForExecutionFailure(t *testing.T) {
	runID := common.RunID("run-repair-id-graph-failure")
	failedStep := &runmodel.Step{
		ID:    common.StepID(runID + "/graph-003/003-run-go-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:         runtimeStageExecution,
			stepInputKeyGraphNodeID:          "run_go_test",
			stepInputKeyRepairOfRootStepID:   string(runID) + "/step-1",
			stepInputKeyRepairOfFailureSig:   "class=dod_coverage_missing|stage=llm_dod_assessment",
			stepInputKeyRepairOfFailureClass: failureClassDoDCoverageMissing,
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=run_go_test|exitCode=1",
			"error":                   "exit status 1",
		},
	}

	stepID := runtimeRepairFollowUpStepID(runID, failedStep, 1)
	if !strings.Contains(string(stepID), "graph-003-003-run-go-test") {
		t.Fatalf("runtime repair step id = %q, want failed graph node label", stepID)
	}
	if strings.Contains(string(stepID), "planning-001-2") || strings.HasSuffix(string(stepID), "planning-001") {
		t.Fatalf("runtime repair step id = %q, want graph failure label instead of planning root label", stepID)
	}
}

func TestRuntimeRepairFollowUpStepIDKeepsGraphNodeForSecondOrderRepair(t *testing.T) {
	runID := common.RunID("run-repair-id-second-order")
	originalGraphStepID := common.StepID(runID + "/graph-004/001-run-stream-test-baseline")
	failedRepair := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-graph-004-001-run-stream-test-baseline"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfStepID:        string(originalGraphStepID),
			stepInputKeyRepairOfRootStepID:    string(runID) + "/step-1",
		},
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=run-stream-test|failureTargets=stream_e2e_test.go|exitCode=1",
		},
	}

	stepID := runtimeRepairFollowUpStepID(runID, failedRepair, 2)
	if !strings.Contains(string(stepID), "graph-004-001-run-stream-test-baseline") {
		t.Fatalf("runtime repair step id = %q, want original graph node label", stepID)
	}
	if strings.Contains(string(stepID), "planning-001") {
		t.Fatalf("runtime repair step id = %q, want graph repair label instead of planning root label", stepID)
	}
}

func TestRuntimeOrchestrator_ProviderUnavailableBlocksOnProviderDecisionWithoutRejectedResult(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-unavailable-decision")
	workspaceID := common.WorkspaceID("ws-provider-unavailable-decision")
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resultRepo := memory.NewResultRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		nil,
		RuntimeEvidenceRepositories{Results: resultRepo},
		nil,
	)
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-provider-unavailable-decision",
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(runID + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage":               "llm_provider_unavailable",
			"error":               "auth_unavailable: no auth available",
			failureOutputKeyClass: failureClassProviderUnavailable,
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("create failed step: %v", err)
	}

	err := orch.DispatchNextStep(ctx, runID)
	if !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("DispatchNextStep err = %v, want blocked", err)
	}
	updated, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if updated.State != common.RunStateBlockedOnDecision || updated.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on provider decision", updated)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("results = %+v, want no business rejected result for provider config blocker", results)
	}
	ticket, err := ticketRepo.Get(ctx, updated.BlockingTicketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.Context[decisionContextKeyScope] != decisionContextScopeWorkorchRuntime ||
		ticket.Context[decisionContextKeyResultPolicy] != decisionContextResultPolicyNone {
		t.Fatalf("ticket context = %+v, want provider config resultPolicy none", ticket.Context)
	}
}

func TestRuntimeOrchestrator_ProviderRetryContractInvalidRoutesToRuntimeRepair(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-provider-retry-contract-invalid")
	rootStepID := common.StepID(runID + "/follow-up-001/dod-gate")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{runtime: &stubRuntimeAdapter{}, steps: stepRepo}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-provider-retry-contract-invalid"),
		DoDID:           common.DoDID("dod-provider-retry-contract-invalid"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	failedRetry := &runmodel.Step{
		ID:    providerRetryStepID(runID, rootStepID, 1),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			"graphFollowUp":                   true,
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryAttempt:  1,
			stepInputKeyProviderRetryRootStep: string(rootStepID),
			stepInputKeyRecoveryAction:        string(runtimeRecoveryActionProviderRetry),
			stepInputKeyRecoveryRootStepID:    string(rootStepID),
			stepInputKeyRecoveryLineageType:   "provider_retry",
			stepInputKeyRecoveryBudgetScope: map[string]any{
				"lineageType": "provider_retry",
				"rootStepId":  string(rootStepID),
			},
		},
		Output: map[string]any{
			"stage":  "llm_schema_invalid",
			"status": "failed",
			"error":  "invalid argument: executionGraph patch/apply targetRef is required",
		},
	}
	attachRuntimeFailureClassification(failedRetry.Output, classifyRuntimeFailure(failedRetry, failedRetry.Output, "runtime step failed", errors.New("invalid argument: executionGraph patch/apply targetRef is required")))
	if err := stepRepo.Create(ctx, failedRetry); err != nil {
		t.Fatalf("seed failed retry: %v", err)
	}

	specs, err := orch.nextDispatchSpecsFromStep(ctx, run, failedRetry)
	if err != nil {
		t.Fatalf("nextDispatchSpecsFromStep: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs = %+v, want runtime repair spec", specs)
	}
	if specs[0].Input[stepInputKeyRuntimeRepairFollowUp] != true ||
		specs[0].Input[stepInputKeyProviderRetryFollowUp] == true {
		t.Fatalf("spec input = %+v, want runtime repair without provider retry", specs[0].Input)
	}
	if specs[0].Input[stepInputKeyRepairOfRootStepID] != string(rootStepID) {
		t.Fatalf("repair root = %v, want provider retry root %s", specs[0].Input[stepInputKeyRepairOfRootStepID], rootStepID)
	}
	if specs[0].Input["repairObjective"] != "produce_state_changing_output" ||
		specs[0].Input["stateChangingOutputRequired"] != true {
		t.Fatalf("repair input = %+v, want targetRef contract failure to require state-changing output", specs[0].Input)
	}
}

func TestRuntimeFollowUpIDsUseReadableLabelsInsteadOfFullLineage(t *testing.T) {
	runID := common.RunID("run-stable-lineage-id")
	lineageStepID := common.StepID("run-stable-lineage-id/follow-up-graph-repair-001-of-provider-retry-001-of-planning-001-build-check")
	step := &runmodel.Step{
		ID: lineageStepID,
		Input: map[string]any{
			stepInputKeyGraphNodeID: "build_check",
			"graphRootStepId":       "run-stable-lineage-id/repair-001-of-provider-retry-001-of-planning-001",
		},
	}

	ids := []common.StepID{
		providerRetryStepID(runID, lineageStepID, 1),
		runtimeRepairFollowUpStepID(runID, step, 1),
		graphFollowUpStepID(runID, step),
		assessmentOnlyContractCorrectionStepID(runID, step),
		llmCompletionFollowUpStepID(runID, step),
	}
	for _, id := range ids {
		if !strings.HasPrefix(string(id), string(runID)+"/") {
			t.Fatalf("step id %q is not run-scoped", id)
		}
		if strings.Count(string(id), "provider-retry-") > 1 {
			t.Fatalf("step id %q retained full provider retry lineage", id)
		}
		if strings.Contains(string(id), "provider-retry-001-of-planning-001-build-check") {
			t.Fatalf("step id %q retained full provider retry ancestor context", id)
		}
		if strings.Contains(string(id), "step-") && strings.Contains(string(id), "provider-retry") {
			t.Fatalf("step id %q retained hash-like provider retry id", id)
		}
	}
}

func TestRuntimeOrchestrator_DuplicateRuntimeFailureBelowScopedBudgetDispatchesRepair(t *testing.T) {
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
		t.Fatalf("duplicate failure below scoped repair budget should dispatch repair: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.BlockingTicketID != "" {
		t.Fatalf("run = %+v, want duplicate failure to continue repair before scoped budget is exhausted", runItem)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	foundRepair := false
	for _, step := range steps {
		if step != nil && boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) && step.ID != common.StepID(string(runID)+"/repair-1") {
			foundRepair = true
			break
		}
	}
	if !foundRepair {
		t.Fatalf("steps = %+v, want runtime repair follow-up", steps)
	}
}

func TestRuntimeOrchestrator_DuplicateRuntimeFailureExhaustedScopeUsesRepairExhaustion(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:            resultRepo,
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-duplicate-failure-exhausted")
	runID := common.RunID("run-duplicate-failure-exhausted")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-duplicate-failure-exhausted"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	previousFailed := duplicateShellFailureStep(runID, "previous-shell-failure")
	if err := stepRepo.Create(ctx, previousFailed); err != nil {
		t.Fatalf("seed previous failed step: %v", err)
	}
	currentFailed := duplicateShellFailureStep(runID, "current-shell-failure")
	signature := stringFromMap(currentFailed.Output, failureOutputKeySignature)
	class := stringFromMap(currentFailed.Output, failureOutputKeyClass)
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/scoped-repair-%d", runID, attempt)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfFailureClass:  class,
				stepInputKeyRepairOfFailureSig:    signature,
			},
			Output: map[string]any{"status": "completed"},
		}); err != nil {
			t.Fatalf("seed scoped repair %d: %v", attempt, err)
		}
	}

	if err := orch.dispatchRuntimeRepairFollowUp(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-duplicate-failure-exhausted"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}, currentFailed, errors.New("exit status 1")); err != nil {
		t.Fatalf("duplicate failure with exhausted scoped repair budget should block on repair exhaustion: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on repair exhaustion decision", runItem)
	}
	ticket, err := ticketRepo.Get(ctx, runItem.BlockingTicketID)
	if err != nil {
		t.Fatalf("get repair exhaustion ticket: %v", err)
	}
	if !strings.HasSuffix(string(ticket.ID), "repair-exhausted-decision") || ticket.Context["attempts"] != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("ticket = %+v, want repair exhaustion ticket", ticket)
	}
	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get repair exhaustion rejected result: %v", err)
	}
	if result.State != common.ResultStateRejected {
		t.Fatalf("result = %+v, want rejected result for business repair exhaustion", result)
	}
}

func TestRuntimeOrchestrator_DuplicateRuntimeFailureUsesTargetScopeAcrossGraphNodes(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-duplicate-failure-target-scope")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps:   stepRepo,
		tickets: memory.NewDecisionTicketRepository(),
	}
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		node := fmt.Sprintf("duplicate-scope-repair-%02d", attempt)
		signature := fmt.Sprintf("class=shell_command_failed|kind=shell/command|node=%s|failureTargets=stream_e2e_test.go|exitCode=1", node)
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/target-repair-%d", runID, attempt)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfFailureClass:  failureClassShellCommand,
				stepInputKeyRepairOfFailureSig:    signature,
			},
			Output: map[string]any{"status": "completed"},
		}); err != nil {
			t.Fatalf("seed target repair %d: %v", attempt, err)
		}
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/graph-010/001-baseline"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=baseline|failureTargets=stream_e2e_test.go|exitCode=1",
		},
	}); err != nil {
		t.Fatalf("seed previous target failure: %v", err)
	}
	currentFailed := &runmodel.Step{
		ID:    common.StepID(runID + "/graph-011/002-targeted"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=targeted|failureTargets=stream_e2e_test.go|exitCode=1",
		},
	}

	if !orch.shouldBlockOnDuplicateRuntimeFailure(ctx, &runmodel.Run{ID: runID, DoDID: common.DoDID("dod-target-duplicate")}, currentFailed) {
		t.Fatalf("duplicate failure should block after target-scoped budget is exhausted")
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
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
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
		ticket.Context["attempts"] != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("ticket = %+v, want repair exhaustion decision context", ticket)
	}
	if ticket.Context[decisionContextKeyRecoveryPolicy] != decisionContextRecoveryPolicyRepairPlan {
		t.Fatalf("ticket context = %+v, want explicit repair recovery policy", ticket.Context)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairExhaustionWritesRejectedResultBeforeTicketEvent(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resultRepo := memory.NewResultRepository()
	hub := events.NewMemoryHub()
	runID := common.RunID("run-repair-exhaustion-result-before-ticket")
	resultID := common.ResultID(string(runID) + "/result/dod-assessment")
	resultVisibleAtTicketEvent := false
	hub.Subscribe(events.TopicTicketCreated, func(ctx context.Context, _ events.Envelope) error {
		result, err := resultRepo.Get(ctx, resultID)
		resultVisibleAtTicketEvent = err == nil && result != nil && result.State == common.ResultStateRejected
		return nil
	})
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Results: resultRepo},
		hub,
	)
	workspaceID := common.WorkspaceID("ws-repair-exhaustion-result-before-ticket")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-repair-exhaustion-result-before-ticket",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	signature := "class=runtime_failed|target=planning"
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-%03d-of-planning-001", runID, attempt)),
			RunID: runID,
			Kind:  llmWorkspaceExecutionStepKind,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfRootStepID:    string(runID) + "/step-1",
				stepInputKeyRepairOfFailureClass:  failureClassRuntime,
				stepInputKeyRepairOfFailureSig:    signature,
			},
			Output: map[string]any{
				"stage":                   "llm_execution_graph_plan",
				"error":                   "invalid argument: state-changing llm repair step returned read-only executionGraph without patch/apply",
				failureOutputKeyClass:     failureClassRuntime,
				failureOutputKeySignature: signature,
			},
		}); err != nil {
			t.Fatalf("seed repair attempt %d: %v", attempt, err)
		}
	}
	failedStep, err := stepRepo.Get(ctx, common.StepID(fmt.Sprintf("%s/repair-%03d-of-planning-001", runID, defaultMaxRuntimeRepairAttempts)))
	if err != nil {
		t.Fatalf("get failed repair: %v", err)
	}

	if err := orch.dispatchRuntimeRepairFollowUp(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-repair-exhaustion-result-before-ticket",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}, failedStep, errors.New("invalid argument: state-changing llm repair step returned read-only executionGraph without patch/apply")); err != nil {
		t.Fatalf("repair exhaustion: %v", err)
	}
	if !resultVisibleAtTicketEvent {
		t.Fatalf("rejected result was not visible when repair exhausted ticket event was published")
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
	if len(items) != 1 {
		t.Fatalf("memory items = %+v, want explicit user preference only", items)
	}
	if !hasMemoryItem(items, runID, "user_preference", "Default test command preference: go test ./...") {
		t.Fatalf("memory items = %+v, want persisted user preference", items)
	}
	step, err := stepRepo.Get(ctx, common.StepID("run-memory-write/step-1"))
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	refs, ok := step.Output["memoryRefs"].([]string)
	if !ok || len(refs) != 1 {
		t.Fatalf("memoryRefs = %+v, want created memory ids", step.Output["memoryRefs"])
	}
}

func TestRuntimeOrchestrator_DoesNotCreateImplicitRunSummaryMemoryFromDoDAssessment(t *testing.T) {
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
	if len(requests) != 0 {
		t.Fatalf("memory requests = %+v, want no implicit run_summary duplicate", requests)
	}
}

func TestRuntimeOrchestrator_DoDFeedbackMemoryCapturesAssessmentGaps(t *testing.T) {
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_dod_assessment",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"summary":   "DoD still lacks verification evidence",
			"items": []map[string]any{
				{
					"item":   "streaming usage is verified",
					"state":  "failed",
					"reason": "no stream test evidence",
				},
				{
					"item":         "non-streaming usage is verified",
					"state":        "satisfied",
					"evidenceRefs": []string{"run/verify"},
				},
			},
		},
	}

	requests := runtimeMemoryWriteRequestsMust(t, output)
	if !memoryWriteRequestContains(requests, memorycommon.MemoryTypeDoDFeedback, "streaming usage is verified") {
		t.Fatalf("memory requests = %+v, want dod_feedback gap", requests)
	}
}

func TestRuntimeOrchestrator_AcceptedInputMemoryWritesCaptureSkillSoulToolContext(t *testing.T) {
	runID := common.RunID("run-accepted-input-memory")
	run := &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-accepted-input-memory",
		ExecutionContext: map[string]any{
			"soul": map[string]any{
				"id":               "soul-software-dev-execution",
				"name":             "software execution",
				"description":      "execute implementation safely",
				"allowedToolKinds": []string{"shell/command", "patch/apply"},
			},
			"soulRouting": map[string]any{
				"policy": "workspace_stage_soul_routing",
			},
			"mcpSelection": map[string]any{
				"autoBoundMCPServerIds": []string{"mcp-code"},
			},
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/step-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeySoulStage: soulStageExecution,
			stepInputKeyExecutionContextPackage: map[string]any{
				"selectedContext": map[string]any{
					"skills": []map[string]any{
						{
							"id":                "skill-go",
							"description":       "Go implementation workflow",
							"requiredToolKinds": []string{"shell/command"},
							"constraints":       []string{"run targeted tests"},
						},
					},
					"mcpServers": []map[string]any{
						{"id": "mcp-code", "transport": "stdio"},
					},
					"mcpCapabilities": []map[string]any{
						{"id": "cap-read", "kind": "resource"},
					},
				},
			},
		},
	}

	requests := acceptedInputMemoryWriteRequests(run, step)
	if !memoryWriteRequestContains(requests, memorycommon.MemoryTypeSkillFeedback, "skill-go") ||
		!memoryWriteRequestContains(requests, memorycommon.MemoryTypeToolFeedback, "mcp-code") ||
		!memoryWriteRequestContains(requests, memorycommon.MemoryTypeProjectContext, "soul-software-dev-execution") {
		t.Fatalf("accepted input memory requests = %+v, want skill/tool/soul context", requests)
	}
}

func TestRuntimeOrchestrator_RepairOutputMemoryWritesCaptureExecutionLesson(t *testing.T) {
	runID := common.RunID("run-repair-lesson-memory")
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/zz-llm-repair-failed-1"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  2,
			"failedStepId":                    string(runID) + "/block/test",
			"failureReason":                   "go test failed",
			failureOutputKeyClass:             failureClassShellCommand,
			failureOutputKeySignature:         "kind=shell/command|cmd=go test",
			"missingDoDItems": []map[string]any{
				{"item": "streaming usage verified"},
			},
		},
	}
	output := map[string]any{
		"status":  "completed",
		"stage":   "llm_execution_graph_repair",
		"summary": "Repair will add missing stream tests",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{"id": "patch-tests"}},
		},
	}

	requests := appendNecessaryOutputMemoryWriteRequests(nil, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-lesson-memory"}, step, output)
	if !memoryWriteRequestContains(requests, memorycommon.MemoryTypeExecutionLesson, "streaming usage verified") ||
		!memoryWriteRequestContains(requests, memorycommon.MemoryTypeExecutionLesson, "go test failed") {
		t.Fatalf("repair memory requests = %+v, want execution lesson with failure and missing DoD context", requests)
	}
}

func TestRuntimeOrchestrator_RepairOutputMemoryWritesDistillMissingFileLesson(t *testing.T) {
	runID := common.RunID("run-repair-missing-file-lesson")
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-missing-file"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  1,
			"failedStepId":                    string(runID) + "/block/read_test_file",
			"failedStepKind":                  "file/read",
			"failureReason":                   "open main_test.go: no such file or directory",
			failureOutputKeySignature:         "kind=file/read|path=main_test.go",
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "file_read_invalid_path",
	}

	requests := appendNecessaryOutputMemoryWriteRequests(nil, &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-missing-file-lesson"}, step, output)
	if len(requests) != 1 ||
		requests[0].MemoryType != memorycommon.MemoryTypeExecutionLesson ||
		requests[0].Summary != "Missing file reads require optional probes or listing before repair" ||
		!strings.Contains(requests[0].Content, "lesson: Missing file reads require optional probes or listing before repair") {
		t.Fatalf("repair memory requests = %+v, want distilled missing-file lesson", requests)
	}
}

func TestRuntimeOrchestrator_OptionalRunSummaryMemoryDoesNotRequireRepository(t *testing.T) {
	ctx := context.Background()
	orch := &RuntimeOrchestrator{}
	refs, err := orch.recordMemoryWrites(ctx,
		&runmodel.Run{ID: "run-memory-optional", WorkspaceID: "ws-memory-optional"},
		&runmodel.Step{ID: "run-memory-optional/step-1", RunID: "run-memory-optional"},
		map[string]any{
			"status": "completed",
			"stage":  "llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "DoD satisfied",
			},
		},
	)
	if err != nil || len(refs) != 0 {
		t.Fatalf("recordMemoryWrites refs=%+v err=%v, want optional run summary skipped without repository", refs, err)
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
	if len(items) != 2 {
		t.Fatalf("memory items = %+v, want 2", items)
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
	if len(items) != 2 {
		t.Fatalf("memory items = %+v, want decision and project context writes", items)
	}
	if !hasMemoryItem(items, runID, memorycommon.MemoryTypeDecision, "approval transcript") ||
		!hasMemoryItem(items, runID, memorycommon.MemoryTypeProjectContext, "confirmed workspace convention") {
		t.Fatalf("memory items = %+v, want decision and project context only", items)
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

func TestRuntimeOrchestrator_StartRunPublishesLLMStreamProgressEvent(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "ok",
		"stage":  "llm_execution_graph_plan",
		"contextBudget": map[string]any{
			"providerId": "provider-a",
			"model":      "model-a",
		},
		"llmUsage": map[string]any{
			"providerId":    "provider-a",
			"provider":      "compatible",
			"model":         "model-a",
			"operation":     "chat_completions",
			"inputTokens":   100,
			"outputTokens":  20,
			"totalTokens":   120,
			"estimated":     true,
			"requestBytes":  1024,
			"responseBytes": 256,
			"streaming":     true,
			"status":        "completed",
		},
		"streamEvents": []map[string]any{
			{"type": "delta", "delta": "abc"},
			{"type": "delta", "delta": "def"},
			{"type": "done"},
		},
	}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-evt-stream")
	runID := common.RunID("run-evt-stream")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		State:           common.RunStateCreated,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	var streamProgress events.Envelope
	found := false
	for _, item := range hub.Published() {
		if item.Topic != events.TopicRunLLMStreamProgress {
			continue
		}
		streamProgress = item
		found = true
		break
	}
	if !found {
		t.Fatalf("published events missing %s", events.TopicRunLLMStreamProgress)
	}
	if streamProgress.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) ||
		streamProgress.Payload[events.PayloadKeyRunID] != string(runID) {
		t.Fatalf("stream payload workspace/run = %+v, want %s/%s", streamProgress.Payload, workspaceID, runID)
	}
	if streamProgress.Payload[events.PayloadKeyChunkCount] != 3 ||
		streamProgress.Payload[events.PayloadKeyReceivedBytes] != 6 {
		t.Fatalf("stream payload chunk/bytes = %+v, want 3/6", streamProgress.Payload)
	}
	if streamProgress.Payload[events.PayloadKeyProviderID] != "provider-a" ||
		streamProgress.Payload[events.PayloadKeyModel] != "model-a" ||
		streamProgress.Payload[events.PayloadKeyStatus] != string(common.StepStateCompleted) {
		t.Fatalf("stream payload provider/model/status = %+v", streamProgress.Payload)
	}

	var usageRecorded events.Envelope
	usageFound := false
	for _, item := range hub.Published() {
		if item.Topic != events.TopicRunLLMUsageRecorded {
			continue
		}
		usageRecorded = item
		usageFound = true
		break
	}
	if !usageFound {
		t.Fatalf("published events missing %s", events.TopicRunLLMUsageRecorded)
	}
	if usageRecorded.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) ||
		usageRecorded.Payload[events.PayloadKeyRunID] != string(runID) ||
		usageRecorded.Payload[events.PayloadKeyStepID] == "" {
		t.Fatalf("usage payload workspace/run/step = %+v", usageRecorded.Payload)
	}
	if usageRecorded.Payload[events.PayloadKeyProviderID] != "provider-a" ||
		usageRecorded.Payload[events.PayloadKeyModel] != "model-a" ||
		usageRecorded.Payload[events.PayloadKeyStreaming] != true {
		t.Fatalf("usage payload provider/model/streaming = %+v", usageRecorded.Payload)
	}
	if usageRecorded.Payload[events.PayloadKeyStatus] != "completed" || usageRecorded.Payload[events.PayloadKeyTotalTokens] == nil {
		t.Fatalf("usage payload status/tokens = %+v", usageRecorded.Payload)
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
			case "run-graph-branch/graph-001/002-build":
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
	if len(runtime.calls) != 2 || runtime.calls[0] != "run-graph-branch/block/prepare" || runtime.calls[1] != "run-graph-branch/graph-001/002-build" {
		t.Fatalf("runtime calls = %#v, want prepare then build only", runtime.calls)
	}
	if _, err := stepRepo.Get(ctx, "run-graph-branch/graph-001/003-reject"); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("reject branch step err = %v, want not found", err)
	}
	buildStep, err := stepRepo.Get(ctx, "run-graph-branch/graph-001/002-build")
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
			case "run-graph-concurrent/graph-001/001-plan-a", "run-graph-concurrent/graph-001/002-plan-b":
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
			case "run-graph-concurrent/graph-001/003-merge":
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
	mergeStep, err := stepRepo.Get(ctx, "run-graph-concurrent/graph-001/003-merge")
	if err != nil {
		t.Fatalf("get merge step: %v", err)
	}
	completed, _ := mergeStep.Input["graphCompletedNodes"].([]string)
	if !reflect.DeepEqual(completed, []string{"plan-a", "plan-b"}) {
		t.Fatalf("merge completed nodes = %#v, want both parallel nodes", mergeStep.Input["graphCompletedNodes"])
	}
}

func TestRuntimeOrchestrator_GraphBatchContinuationUsesCurrentBatchRoot(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-graph-batch-current-root")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-graph-batch-current-root"}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "patch-a", Kind: "graph/mock"},
		{ID: "patch-b", Kind: "graph/mock"},
		{ID: "verify", Kind: "graph/mock", DependsOn: []string{"patch-a", "patch-b"}},
	}}
	root := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/provider-retry-001-of-repair-001"),
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateCompleted,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	steps := []*runmodel.Step{
		root,
		{
			ID:    graphScopedStepID(runID, 1, 1, "patch-a"),
			RunID: runID,
			Kind:  "graph/mock",
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyGraphRootStepID: string(root.ID),
				stepInputKeyGraphNodeID:     "patch-a",
				stepOutputKeyExecutionGraph: graph,
				"previousStepId":            string(root.ID),
			},
		},
		{
			ID:    graphScopedStepID(runID, 1, 2, "patch-b"),
			RunID: runID,
			Kind:  "graph/mock",
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyGraphRootStepID: string(root.ID),
				stepInputKeyGraphNodeID:     "patch-b",
				stepOutputKeyExecutionGraph: graph,
				"previousStepId":            string(root.ID),
			},
		},
		{
			ID:    common.StepID(string(runID) + "/graph-099/001-unrelated"),
			RunID: runID,
			Kind:  "graph/mock",
			State: common.StepStateCompleted,
			Input: map[string]any{
				stepInputKeyGraphRootStepID: string(runID) + "/unrelated-root",
				stepInputKeyGraphNodeID:     "unrelated",
				stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{{ID: "unrelated", Kind: "graph/mock"}}},
			},
		},
	}
	specs := []stepDispatchSpec{
		{ID: graphScopedStepID(runID, 1, 1, "patch-a"), Kind: "graph/mock", Input: map[string]any{stepInputKeyGraphRootStepID: string(root.ID)}},
		{ID: graphScopedStepID(runID, 1, 2, "patch-b"), Kind: "graph/mock", Input: map[string]any{stepInputKeyGraphRootStepID: string(root.ID)}},
	}

	last := latestCompletedGraphBatchStep(steps, specs)
	if last == nil || graphRootStepID(last) != root.ID {
		t.Fatalf("last = %+v, want completed step from current graph root", last)
	}
	continuedGraph, ok := runtimeGraphSpecForContinuation(steps, last)
	if !ok || len(continuedGraph.Nodes) != 3 {
		t.Fatalf("continued graph = %+v/%v, want root execution graph", continuedGraph, ok)
	}
	nextSpecs := (&RuntimeOrchestrator{}).readyGraphStepSpecsFromSteps(ctx, run, last, continuedGraph, steps)
	if len(nextSpecs) != 1 || nextSpecs[0].ID != graphScopedStepID(runID, 1, 3, "verify") {
		t.Fatalf("next specs = %+v, want dependent verify node from current graph", nextSpecs)
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
			case "run-graph-decision/graph-001/002-review":
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
	reviewStep, err := stepRepo.Get(ctx, "run-graph-decision/graph-001/002-review")
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

func TestRuntimeOrchestrator_ConvertsKnownReadableResourceDecisionToRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	workingDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDirectory, "main_test.go"), []byte("package main\n\nfunc TestKnownResource() {}\n"), 0o644); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}

	var repairInput map[string]any
	call := 0
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		call++
		if call == 1 {
			return map[string]any{
				"status":                "completed",
				"blockDecisionReason":   "need main_test.go content",
				"blockDecisionKind":     "provide_missing_info",
				"blockDecisionQuestion": "Please provide main_test.go.",
				"blockDecisionContext": map[string]any{
					"knownValues": map[string]any{
						"existingFiles": []string{"main.go", "main_test.go"},
					},
					"missingFields":       []string{"main_test.go content"},
					"requiresHumanFields": []string{"main_test.go content"},
				},
				"nextStep": "run-known-resource-decision/llm-after-decision",
				"nextKind": "llm/workspace_execution",
			}, nil
		}
		repairInput = cloneStepPayload(request.Input)
		return map[string]any{
			"status": "completed",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "completed after readable resource repair",
				"items": []map[string]any{
					{
						"item":         "runtime repair",
						"state":        "satisfied",
						"evidenceRefs": []string{string(request.StepID)},
					},
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
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-known-resource-decision")
	runID := common.RunID("run-known-resource-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-known-resource-decision"),
		State:           common.RunStateCreated,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if call != 2 {
		t.Fatalf("runtime calls = %d, want initial decision plus repair", call)
	}
	tickets, err := ticketRepo.ListByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if len(tickets) != 0 {
		t.Fatalf("tickets = %+v, want no human decision for known readable resource", tickets)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	if repairInput == nil || !boolValue(repairInput[stepInputKeyRuntimeRepairFollowUp]) {
		t.Fatalf("repair input = %+v, want runtime repair follow-up", repairInput)
	}
	resourceContext := mapSliceFromValue(repairInput["readableResourceRepairContext"])
	if len(resourceContext) != 1 ||
		resourceContext[0]["targetRef"] != "main_test.go" ||
		!strings.Contains(stringFromMap(resourceContext[0], "content"), "TestKnownResource") {
		t.Fatalf("readableResourceRepairContext = %+v, want main_test.go content", resourceContext)
	}
}

func TestRuntimeOrchestrator_ConvertsQuestionOnlyReadableResourceDecisionToRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	workingDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workingDirectory, "config"), 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workingDirectory, "handler"), 0o755); err != nil {
		t.Fatalf("create handler dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workingDirectory, "config", "config.go"), []byte("package config\n\nconst AppName = \"workorch-test\"\n"), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workingDirectory, "handler", "handler.go"), []byte("package handler\n\nfunc Route() string { return \"ok\" }\n"), 0o644); err != nil {
		t.Fatalf("write handler file: %v", err)
	}

	var repairInput map[string]any
	call := 0
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		call++
		if call == 1 {
			return map[string]any{
				"status":                "completed",
				"blockDecisionReason":   "tests need actual file contents before patching",
				"blockDecisionKind":     "provide_missing_info",
				"blockDecisionQuestion": "请提供 config/config.go 和 handler/handler.go 的完整内容。",
				"nextStep":              "run-question-readable-decision/llm-after-decision",
				"nextKind":              "llm/workspace_execution",
			}, nil
		}
		repairInput = cloneStepPayload(request.Input)
		return map[string]any{
			"status": "completed",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "completed after question-only readable resource repair",
				"items": []map[string]any{{
					"item":         "runtime repair",
					"state":        "satisfied",
					"evidenceRefs": []string{string(request.StepID)},
				}},
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
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-question-readable-decision")
	runID := common.RunID("run-question-readable-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-question-readable-decision"),
		State:           common.RunStateCreated,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if call != 2 {
		t.Fatalf("runtime calls = %d, want initial decision plus repair", call)
	}
	tickets, err := ticketRepo.ListByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if len(tickets) != 0 {
		t.Fatalf("tickets = %+v, want no human decision for workspace-readable files", tickets)
	}
	resourceContext := mapSliceFromValue(repairInput["readableResourceRepairContext"])
	if len(resourceContext) != 2 {
		t.Fatalf("readableResourceRepairContext = %+v, want two file snapshots", resourceContext)
	}
	contentsByTarget := map[string]string{}
	for _, item := range resourceContext {
		contentsByTarget[stringFromMap(item, "targetRef")] = stringFromMap(item, "content")
	}
	if !strings.Contains(contentsByTarget["config/config.go"], "AppName") ||
		!strings.Contains(contentsByTarget["handler/handler.go"], "Route") {
		t.Fatalf("readableResourceRepairContext = %+v, want config and handler snapshots", resourceContext)
	}
}

func TestRuntimeOrchestrator_ResumeRunAutoRepairsOpenKnownReadableResourceDecision(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	workingDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workingDirectory, "main_test.go"), []byte("package main\n\nfunc TestReadableTicketResource(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}

	var repairInput map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		repairInput = cloneStepPayload(request.Input)
		return map[string]any{
			"status": "completed",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "completed after auto readable-resource repair",
				"items": []map[string]any{{
					"item":         "readable resource used",
					"state":        "satisfied",
					"evidenceRefs": []string{string(request.StepID)},
				}},
			},
		}, nil
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{Results: memory.NewResultRepository()},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-open-known-resource-decision")
	runID := common.RunID("run-open-known-resource-decision")
	stepID := common.StepID("run-open-known-resource-decision/follow-up-graph-001")
	ticketID := common.TicketID("ticket-open-known-resource-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateWaitingDecision, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		DoDID:            common.DoDID("dod-open-known-resource-decision"),
		State:            common.RunStateBlockedOnDecision,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			stepInputKeySoulStage:        soulStageValidation,
			"graphFollowUp":              true,
			"completionFollowUp":         true,
		},
		Output: map[string]any{
			"status":                "completed",
			"stage":                 "llm_decision_required",
			"blockDecisionReason":   "need main_test.go content",
			"blockDecisionKind":     decisioncommon.ResolutionTypeProvideMissingInfo,
			"blockDecisionQuestion": "Please provide main_test.go.",
			"blockDecisionContext": map[string]any{
				"knownValues": map[string]any{
					"existingFiles": []string{"go.mod", "config.yaml", "main.go"},
				},
				"missingFields":       []string{"main_test.go content"},
				"requiresHumanFields": []string{"main_test.go content"},
			},
		},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:           ticketID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		StepID:       stepID,
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Reason:       "need main_test.go content",
		Question:     "Please provide main_test.go.",
		Context: map[string]any{
			"knownValues": map[string]any{
				"existingFiles": []string{"go.mod", "config.yaml", "main.go"},
			},
			"missingFields":       []string{"main_test.go content"},
			"requiresHumanFields": []string{"main_test.go content"},
		},
		State: common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume run: %v", err)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted || runItem.BlockingTicketID != "" {
		t.Fatalf("run = %+v, want completed without blocker", runItem)
	}
	ticket, err := ticketRepo.Get(ctx, ticketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.State != common.DecisionTicketStateDismissed {
		t.Fatalf("ticket state = %s, want dismissed auto-repairable decision", ticket.State)
	}
	if repairInput == nil || !boolValue(repairInput[stepInputKeyRuntimeRepairFollowUp]) {
		t.Fatalf("repair input = %+v, want runtime repair", repairInput)
	}
	resourceContext := mapSliceFromValue(repairInput["readableResourceRepairContext"])
	if len(resourceContext) != 1 ||
		resourceContext[0]["targetRef"] != "main_test.go" ||
		!strings.Contains(stringFromMap(resourceContext[0], "content"), "TestReadableTicketResource") {
		t.Fatalf("readableResourceRepairContext = %+v, want current main_test.go content", resourceContext)
	}
}

func TestRuntimeOrchestrator_DecisionBlockResultPolicyRejectedWritesResult(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status":                "completed",
		"summary":               "cannot continue without a concrete target",
		"blockDecisionReason":   "missing implementation target",
		"blockDecisionKind":     "provide_missing_info",
		"blockDecisionQuestion": "Please provide the implementation target.",
		"blockDecisionContext": map[string]any{
			decisionContextKeyResultPolicy: decisionContextResultPolicyRejected,
			"missingFields":                []string{"target"},
		},
		"nextStep": "run-decision-result-policy/llm-after-decision",
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
			Results:            resultRepo,
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-decision-result-policy")
	runID := common.RunID("run-decision-result-policy")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          common.GoalID("goal-decision-result-policy"),
		DoDID:           common.DoDID("dod-decision-result-policy"),
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
	if runItem.State != common.RunStateBlockedOnDecision || runItem.BlockingTicketID == "" {
		t.Fatalf("run = %+v, want blocked on decision", runItem)
	}
	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get rejected result: %v", err)
	}
	if result.State != common.ResultStateRejected || result.DoDAssessment.Satisfied {
		t.Fatalf("result = %+v, want rejected result for decision result policy", result)
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

func TestRuntimeOrchestrator_FailsContinueWithoutActionableContinuationWithoutDoDGate(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{
		output: map[string]any{
			"status": "completed",
			"llmDecision": map[string]any{
				"completionState": "continue",
			},
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-continue-no-action")
	runID := common.RunID("run-continue-no-action")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
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
	if runItem.State != common.RunStateFailed || !strings.Contains(runItem.FailureReason, "completionState continue has no actionable continuation") {
		t.Fatalf("run = %+v, want failed for continue without actionable continuation", runItem)
	}
	step, err := stepRepo.Get(ctx, common.StepID("run-continue-no-action/step-1"))
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateFailed || !strings.Contains(step.Output["error"].(string), "completionState continue has no actionable continuation") {
		t.Fatalf("step = %+v, want failed completion continuation gate", step)
	}
}

func TestRuntimeOrchestrator_DispatchesValidationFollowUpForContinueWithoutActionableContinuation(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var followUpInput map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if boolValue(request.Input[stepInputKeyNoActionableFollowUp]) {
			followUpInput = request.Input
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "current evidence satisfies the DoD",
					"items": []map[string]any{
						{
							"item":         "goal completed",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						},
					},
				},
			}, nil
		}
		return map[string]any{
			"status": "completed",
			"llmDecision": map[string]any{
				"completionState": "continue",
			},
		}, nil
	}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-continue-follow-up")
	runID := common.RunID("run-continue-follow-up")
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
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run = %+v, want completed after validation follow-up", runItem)
	}
	if !boolValue(followUpInput[stepInputKeyNoActionableFollowUp]) {
		t.Fatalf("followUpInput = %+v, want no-actionable continuation marker", followUpInput)
	}
	if followUpInput["previousCompletionState"] != "continue" {
		t.Fatalf("followUpInput previousCompletionState = %#v, want continue", followUpInput["previousCompletionState"])
	}
	if evidence := mapSliceFromValue(followUpInput[stepInputKeyRunEvidenceSummary]); len(evidence) == 0 {
		t.Fatalf("followUpInput runEvidenceSummary = %#v, want prior evidence", followUpInput[stepInputKeyRunEvidenceSummary])
	}
}

func TestRuntimeOrchestrator_DispatchStepDoesNotOverwriteTerminalStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "completed"}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, memory.NewDecisionTicketRepository(), memory.NewApprovalRepository(), events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-terminal-step-dispatch")
	runID := common.RunID("run-terminal-step-dispatch")
	stepID := common.StepID("run-terminal-step-dispatch/step-1")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateRunning, InitialStepKind: llmWorkspaceExecutionStepKind}
	if err := workspaceRepo.Create(ctx, workspace); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateCompleted,
		Input:  map[string]any{"original": true},
		Output: map[string]any{"status": "completed"},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	err := orch.dispatchStepWithWorkspace(ctx, run, stepID, llmWorkspaceExecutionStepKind, map[string]any{"replacement": true}, workspace)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("dispatch terminal step err = %v, want conflict", err)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime calls = %+v, want no terminal step re-execution", runtime.calls)
	}
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if step.State != common.StepStateCompleted || step.Input["original"] != true || step.Input["replacement"] == true {
		t.Fatalf("step = %+v, want original completed step preserved", step)
	}
}

func TestRuntimeOrchestrator_RepairsRepeatedContinueWithoutActionableContinuation(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var repairSeen bool
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
			repairSeen = true
			return map[string]any{
				"status": "completed",
				"dodAssessment": map[string]any{
					"satisfied": true,
					"summary":   "repair completed validation",
					"items": []map[string]any{
						{
							"item":         "goal completed",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						},
					},
				},
			}, nil
		}
		return map[string]any{
			"status": "completed",
			"llmDecision": map[string]any{
				"completionState": "continue",
			},
		}, nil
	}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-repeated-continue-repair")
	runID := common.RunID("run-repeated-continue-repair")
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
		t.Fatalf("start run: %v", err)
	}

	if !repairSeen {
		t.Fatalf("runtime repair follow-up was not dispatched after repeated no-actionable continuation")
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run = %+v, want completed after repair", runItem)
	}
}

func TestRuntimeOrchestrator_RepairsInconclusiveSatisfiedDoDAssessmentBeforeEscalation(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	validationRepo := memory.NewValidationResultRepository()
	var repairInputs []map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
			repairInputs = append(repairInputs, request.Input)
		}
		return map[string]any{
			"status":          "completed",
			"evidenceQuality": string(common.EvidenceQualityInconclusive),
			"evidenceReason":  "runtime adapter reported inconclusive evidence",
			"zeroWork":        true,
			"evidenceTags":    []string{"validation", "zero_work"},
			"dodAssessment": map[string]any{
				"satisfied":           true,
				"summary":             "verification was inconclusive",
				"goalOutcome":         "done",
				"verificationSummary": "runtime adapter reported inconclusive evidence",
				"items": []map[string]any{
					{
						"item":         "verification passes",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-dod-weak-gate/step-1"},
					},
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
		RuntimeEvidenceRepositories{Results: resultRepo, ValidationResults: validationRepo},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-dod-weak-gate")
	runID := common.RunID("run-dod-weak-gate")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-weak"),
		State:           common.RunStateCreated,
		InitialStepKind: "llm/workspace_execution",
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run should repair and escalate without returning error: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed ||
		!strings.Contains(runItem.FailureReason, "satisfied dodAssessment item") {
		t.Fatalf("run = %+v, want failed after repair returned non-actionable weak assessment", runItem)
	}
	if len(repairInputs) != 1 {
		t.Fatalf("repair inputs = %d, want one non-actionable repair assessment", len(repairInputs))
	}
	if repairInputs[0]["failedStepId"] == "" || repairInputs[0]["failedStepOutput"] == nil {
		t.Fatalf("first repair input = %+v, want failed DoD context", repairInputs[0])
	}
	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get rejected result: %v", err)
	}
	if result.State != common.ResultStateRejected || result.DoDAssessment.Satisfied {
		t.Fatalf("result = %+v, want rejected unsatisfied DoD result", result)
	}
	if tickets, err := ticketRepo.ListByRunID(ctx, runID); err != nil || len(tickets) != 0 {
		t.Fatalf("tickets = %+v err=%v, want no repair exhaustion ticket for non-actionable repair assessment", tickets, err)
	}
}

func TestGraphLLMNodeInputRequiresStateChangeWhenRunContractRequiresIt(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-graph-llm-state-change",
		WorkspaceID:     "ws-graph-llm-state-change",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "plan-implementation", Kind: llmWorkspaceExecutionStepKind, Input: map[string]any{"task": "produce implementation"}},
	}}
	last := &runmodel.Step{
		ID:     common.StepID(string(run.ID) + "/step-1"),
		RunID:  run.ID,
		Kind:   llmWorkspaceExecutionStepKind,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}

	input := nextGraphStepInput(run, last, graph, graph.Nodes[0], nil, "", []*runmodel.Step{last}, last.ID)

	if input["stateChangingOutputRequired"] != true ||
		input["repairObjective"] != "produce_state_changing_output" ||
		len(stringSliceFromValue(input["repairGuidance"])) == 0 {
		t.Fatalf("graph llm input = %+v, want state-changing contract injected", input)
	}
	if input[stepInputKeyRuntimeStage] != runtimeStageExecution || input[stepInputKeyRuntimeStageMode] != "strict_convergence" {
		t.Fatalf("graph llm runtime stage = %v/%v, want execution/strict", input[stepInputKeyRuntimeStage], input[stepInputKeyRuntimeStageMode])
	}
}

func TestGraphReadNodeInputDoesNotRequireStateChangeWhenRunContractRequiresIt(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-graph-read-state-change",
		WorkspaceID:     "ws-graph-read-state-change",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "read-main", Kind: "file/read", Input: map[string]any{"path": "main.go", "optional": true}},
	}}
	last := &runmodel.Step{
		ID:     common.StepID(string(run.ID) + "/step-1"),
		RunID:  run.ID,
		Kind:   llmWorkspaceExecutionStepKind,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}

	input := nextGraphStepInput(run, last, graph, graph.Nodes[0], nil, "", []*runmodel.Step{last}, last.ID)

	if input["stateChangingOutputRequired"] != nil || input["repairObjective"] != nil {
		t.Fatalf("graph read input = %+v, want read node to stay observation-only", input)
	}
}

func TestGraphLLMNodeInputIncludesDependencyOutput(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-graph-dependency-output",
		WorkspaceID:     "ws-graph-dependency-output",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "scan_workspace", Kind: "file/list", Input: map[string]any{"path": ".", "recursive": true}},
		{ID: "refine_plan_from_manifest", Kind: llmWorkspaceExecutionStepKind, DependsOn: []string{"scan_workspace"}},
	}}
	root := &runmodel.Step{
		ID:     common.StepID(string(run.ID) + "/step-1"),
		RunID:  run.ID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateCompleted,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	scan := &runmodel.Step{
		ID:    common.StepID(string(run.ID) + "/graph-001/001-scan-workspace"),
		RunID: run.ID,
		Kind:  "file/list",
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "scan_workspace",
			stepInputKeyGraphRootStepID: string(root.ID),
			stepOutputKeyExecutionGraph: graph,
		},
		Output: map[string]any{
			"status":    "completed",
			"stage":     "file_list",
			"path":      ".",
			"entries":   []map[string]any{},
			"truncated": false,
		},
	}
	completed := map[string]bool{"scan_workspace": true}

	input := nextGraphStepInput(run, scan, graph, graph.Nodes[1], completed, "", []*runmodel.Step{root, scan}, root.ID)

	if input["previousStepKind"] != "file/list" {
		t.Fatalf("previousStepKind = %v, want file/list", input["previousStepKind"])
	}
	previousOutput := mapFromValue(input["previousStepOutput"])
	if previousOutput["status"] != "completed" || previousOutput["truncated"] != false {
		t.Fatalf("previousStepOutput = %+v, want compact scan output", input["previousStepOutput"])
	}
	if _, ok := previousOutput["entries"]; !ok {
		t.Fatalf("previousStepOutput = %+v, want explicit empty entries from scan output", previousOutput)
	}
	evidence := mapSliceFromValue(input[stepInputKeyGraphEvidence])
	if len(evidence) != 1 || evidence[0]["nodeId"] != "scan_workspace" || evidence[0]["kind"] != "file/list" {
		t.Fatalf("graphEvidence = %+v, want dependency evidence for scan_workspace", input[stepInputKeyGraphEvidence])
	}
	evidenceOutput := mapFromValue(evidence[0]["output"])
	if evidenceOutput["truncated"] != false {
		t.Fatalf("graphEvidence output = %+v, want scan output", evidenceOutput)
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
			switch {
			case request.StepID == "run-graph-follow-up/step-1":
				return map[string]any{
					"status":         "completed",
					"executionGraph": graph,
				}, nil
			case request.StepID == "run-graph-follow-up/graph-001/001-inspect":
				return map[string]any{
					"status": "completed",
					"stdout": "workspace inspected",
				}, nil
			case boolValue(request.Input["graphFollowUp"]) && request.Input["graphPreviousNodeId"] == "inspect":
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
								"evidenceRefs": []string{"run-graph-follow-up/graph-001/001-inspect"},
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
	if len(runtime.calls) != 3 ||
		runtime.calls[0] != "run-graph-follow-up/step-1" ||
		runtime.calls[1] != "run-graph-follow-up/graph-001/001-inspect" ||
		!strings.HasPrefix(string(runtime.calls[2]), "run-graph-follow-up/follow-up-graph-") ||
		strings.Contains(string(runtime.calls[2]), "run-graph-follow-up-step-1-inspect") {
		t.Fatalf("runtime calls = %#v, want graph then llm follow-up", runtime.calls)
	}
	if followUpInput["graphFollowUp"] != true ||
		followUpInput["graphPreviousNodeId"] != "inspect" ||
		followUpInput["previousStepId"] != "run-graph-follow-up/graph-001/001-inspect" ||
		followUpInput[stepInputKeySoulStage] != soulStageValidation {
		t.Fatalf("followUpInput = %+v, want graph follow-up context", followUpInput)
	}
	if followUpInput[stepInputKeyRuntimeStage] != runtimeStageVerification {
		t.Fatalf("graph follow-up runtimeStage = %v, want verification", followUpInput[stepInputKeyRuntimeStage])
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
			switch {
			case request.StepID == "run-shell-follow-up/step-1":
				return map[string]any{
					"status":   "completed",
					"nextStep": "run-shell-follow-up/shell-1",
					"nextKind": "shell/command",
					"nextInput": map[string]any{
						"command": "cat",
						"args":    []string{"result.txt"},
					},
				}, nil
			case request.StepID == "run-shell-follow-up/shell-1":
				return map[string]any{
					"status": "completed",
					"stdout": "result ready",
				}, nil
			case boolValue(request.Input["completionFollowUp"]):
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
	if followUpInput[stepInputKeyRuntimeStage] != runtimeStageVerification {
		t.Fatalf("completion follow-up runtimeStage = %v, want verification", followUpInput[stepInputKeyRuntimeStage])
	}
	previousOutput, ok := followUpInput["previousStepOutput"].(map[string]any)
	if !ok || previousOutput["stdout"] != "result ready" {
		t.Fatalf("previousStepOutput = %+v, want shell output", followUpInput["previousStepOutput"])
	}
	runEvidence := mapSliceFromValue(followUpInput[stepInputKeyRunEvidenceSummary])
	if len(runEvidence) == 0 {
		t.Fatalf("runEvidenceSummary = %+v, want compact run evidence", followUpInput[stepInputKeyRunEvidenceSummary])
	}
	foundShellEvidence := false
	for _, item := range runEvidence {
		if item["stepId"] != "run-shell-follow-up/shell-1" {
			continue
		}
		output, _ := item["output"].(map[string]any)
		foundShellEvidence = output["stdout"] == "result ready"
	}
	if !foundShellEvidence {
		t.Fatalf("runEvidenceSummary = %+v, want shell output evidence", runEvidence)
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
			switch {
			case request.StepID == "run-patch-follow-up/step-1":
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
			case boolValue(request.Input["completionFollowUp"]):
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
	if followUpInput[stepInputKeyRuntimeStage] != runtimeStageVerification {
		t.Fatalf("patch follow-up runtimeStage = %v, want verification", followUpInput[stepInputKeyRuntimeStage])
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
			case request.StepID == "run-graph-parallel-follow-up/graph-001/001-cpu":
				return map[string]any{
					"status": "completed",
					"stdout": "cpu: 4 cores",
				}, nil
			case request.StepID == "run-graph-parallel-follow-up/graph-001/002-memory":
				return map[string]any{
					"status": "completed",
					"stdout": "memory: 15Gi",
				}, nil
			case strings.HasPrefix(string(request.StepID), "run-graph-parallel-follow-up/follow-up-graph-"):
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
								"evidenceRefs": []string{"run-graph-parallel-follow-up/graph-001/001-cpu", "run-graph-parallel-follow-up/graph-001/002-memory"},
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
	runEvidence := mapSliceFromValue(input[stepInputKeyRunEvidenceSummary])
	if len(runEvidence) != 3 {
		t.Fatalf("runEvidenceSummary = %+v, want root and graph node evidence", input[stepInputKeyRunEvidenceSummary])
	}
	foundCPUEvidence := false
	for _, item := range runEvidence {
		if item["stepId"] != string(runID)+"/block/cpu" {
			continue
		}
		output, _ := item["output"].(map[string]any)
		foundCPUEvidence = item["graphNodeId"] == "cpu" && output["stdout"] == "cpu ok"
	}
	if !foundCPUEvidence {
		t.Fatalf("runEvidenceSummary = %+v, want graph node evidence", runEvidence)
	}
}

func TestRuntimeOrchestrator_GraphFollowUpRunEvidenceIncludesPriorCompletedSteps(t *testing.T) {
	runID := common.RunID("run-graph-follow-up-run-evidence")
	orch := &RuntimeOrchestrator{}
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-graph-follow-up-run-evidence"}
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "verify", Kind: "shell/command"},
	}}
	priorStep := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/previous-validation"),
		RunID:  runID,
		Kind:   "shell/command",
		State:  common.StepStateCompleted,
		Output: map[string]any{"stdout": "previous validation passed"},
	}
	anchor := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/step-1"),
		RunID:  runID,
		Kind:   llmcommon.StepKindWorkspaceExecution,
		State:  common.StepStateCompleted,
		Output: map[string]any{stepOutputKeyExecutionGraph: graph},
	}
	graphStep := &runmodel.Step{
		ID:     common.StepID(string(runID) + "/block/verify"),
		RunID:  runID,
		Kind:   "shell/command",
		State:  common.StepStateCompleted,
		Input:  map[string]any{stepInputKeyGraphNodeID: "verify", stepInputKeyGraphRootStepID: string(anchor.ID)},
		Output: map[string]any{"stdout": "current graph validation passed"},
	}

	input := orch.graphFollowUpStepInputFromSteps(run, anchor, []*runmodel.Step{priorStep, anchor, graphStep})

	graphEvidence := mapSliceFromValue(input[stepInputKeyGraphEvidence])
	if len(graphEvidence) != 1 || graphEvidence[0]["nodeId"] != "verify" {
		t.Fatalf("graphEvidence = %+v, want current graph evidence only", input[stepInputKeyGraphEvidence])
	}
	runEvidence := mapSliceFromValue(input[stepInputKeyRunEvidenceSummary])
	if len(runEvidence) != 3 {
		t.Fatalf("runEvidenceSummary = %+v, want prior, root, and current graph evidence", input[stepInputKeyRunEvidenceSummary])
	}
	foundPriorEvidence := false
	for _, item := range runEvidence {
		if item["stepId"] != string(priorStep.ID) {
			continue
		}
		output, _ := item["output"].(map[string]any)
		foundPriorEvidence = output["stdout"] == "previous validation passed"
	}
	if !foundPriorEvidence {
		t.Fatalf("runEvidenceSummary = %+v, want prior completed evidence", runEvidence)
	}
}

func TestMergeRunExecutionContextPatchRefreshesEffectiveDefinitionIDs(t *testing.T) {
	run := &runmodel.Run{
		ID:               "run-definition-patch",
		WorkspaceID:      "ws-definition-patch",
		GoalID:           "goal-old",
		DoDID:            "dod-old",
		ExecutionContext: map[string]any{"existing": "kept"},
	}

	mergeRunExecutionContextPatch(run, map[string]any{
		"goalId": "goal-new",
		"doDId":  "dod-new",
		"goal":   map[string]any{"id": "goal-new"},
		"dod":    map[string]any{"id": "dod-new"},
	})

	if run.GoalID != "goal-new" || run.DoDID != "dod-new" {
		t.Fatalf("run definitions = %s/%s, want refreshed IDs", run.GoalID, run.DoDID)
	}
	if run.ExecutionContext["existing"] != "kept" || run.ExecutionContext["goalId"] != "goal-new" || run.ExecutionContext["doDId"] != "dod-new" {
		t.Fatalf("executionContext = %+v, want merged definition patch", run.ExecutionContext)
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
	wantID := graphScopedStepID(runID, 2, 1, "verify-file")
	if specs[0].ID != wantID {
		t.Fatalf("spec ID = %s, want scoped ID %s", specs[0].ID, wantID)
	}
	if strings.Contains(string(specs[0].ID), "zz-llm-after-graph") {
		t.Fatalf("spec ID = %s, want readable scoped ID without embedded root lineage", specs[0].ID)
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
		case strings.HasPrefix(string(request.StepID), string(runID)+"/follow-up-graph-"):
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
		{
			ID:   "verify-file",
			Kind: "patch/apply",
			Input: map[string]any{
				"patchType": "file_write",
				"targetRef": "main.go",
				"content":   "package main\n",
			},
		},
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
			case request.StepID == "run-graph-repeat/graph-001/001-verify-file":
				return map[string]any{
					"status": "completed",
					"stdout": "first verify",
				}, nil
			case boolValue(request.Input["graphFollowUp"]) && request.Input["previousStepId"] == "run-graph-repeat/graph-001/001-verify-file":
				return map[string]any{
					"status":         "completed",
					"executionGraph": secondGraph,
				}, nil
			case strings.HasPrefix(string(request.StepID), "run-graph-repeat/graph-002/") && strings.HasSuffix(string(request.StepID), "-verify-file"):
				secondGraphStepID = request.StepID
				return map[string]any{
					"status": "completed",
					"stdout": "second verify",
				}, nil
			case boolValue(request.Input["graphFollowUp"]) && secondGraphStepID != "" && request.Input["previousStepId"] == string(secondGraphStepID):
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
	if secondGraphStepID == "" || secondGraphStepID == "run-graph-repeat/graph-001/001-verify-file" {
		t.Fatalf("secondGraphStepID = %q, want graph-root-scoped unique step id", secondGraphStepID)
	}
	if strings.Contains(string(secondGraphStepID), "zz-llm-after-graph") || strings.Contains(string(secondGraphStepID), "run-graph-repeat-step-1") {
		t.Fatalf("secondGraphStepID = %q, want readable scoped id without embedded ancestor lineage", secondGraphStepID)
	}
}

func TestRuntimeOrchestrator_GraphFollowUpExpansionLimitRoutesToAssessmentOnly(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var graphExpansions int
	var assessmentOnlySeen bool
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if boolValue(request.Input[stepInputKeyAssessmentOnly]) {
				assessmentOnlySeen = true
				if boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
					t.Fatalf("assessment-only input unexpectedly uses runtime repair follow-up: %+v", request.Input)
				}
				if _, ok := request.Input[stepOutputKeyExecutionGraph]; ok {
					t.Fatalf("assessment-only input retained executionGraph: %+v", request.Input[stepOutputKeyExecutionGraph])
				}
				return map[string]any{
					"status": "completed",
					stepOutputKeyDoDAssessment: map[string]any{
						"satisfied":   true,
						"summary":     "graph expansion bounded into final assessment",
						"goalOutcome": "The run completed after graph expansion assessment.",
						"items": []map[string]any{
							{
								"item":         "graph expansion bounded",
								"state":        "satisfied",
								"evidenceRefs": []string{string(request.StepID)},
							},
						},
					},
				}, nil
			}
			if boolValue(request.Input["graphFollowUp"]) {
				graphExpansions++
				return map[string]any{
					"status": "completed",
					stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
						{
							ID:   fmt.Sprintf("verify_%d", graphExpansions),
							Kind: "patch/apply",
							Input: map[string]any{
								"patchType": "file_write",
								"targetRef": fmt.Sprintf("graph-%d.txt", graphExpansions),
								"content":   fmt.Sprintf("graph expansion %d\n", graphExpansions),
							},
						},
					}},
				}, nil
			}
			if request.Kind == "graph/mock" || request.Kind == "patch/apply" {
				return map[string]any{"status": "completed", "stdout": "graph node completed"}, nil
			}
			return map[string]any{
				"status": "completed",
				stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
					{ID: "inspect", Kind: "graph/mock"},
				}},
			}, nil
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-graph-expansion-limit")
	runID := common.RunID("run-graph-expansion-limit")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-graph-expansion-limit"),
		State:           common.RunStateCreated,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	if !assessmentOnlySeen {
		t.Fatalf("assessment-only follow-up was not dispatched after graph expansion limit")
	}
	if graphExpansions != maxGraphFollowUpExpansionAttempts+1 {
		t.Fatalf("graphExpansions = %d, want one bounded overflow beyond limit %d", graphExpansions, maxGraphFollowUpExpansionAttempts)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		steps, _ := stepRepo.ListByRun(ctx, runID)
		t.Fatalf("run = %+v steps=%+v, want completed after assessment-only follow-up", runItem, steps)
	}
}

func TestRuntimeOrchestrator_AssessmentOnlyContractViolationDispatchesOneCorrection(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var assessmentOnlySeen bool
	var correctionSeen bool
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if boolValue(request.Input[stepInputKeyAssessmentOnly]) {
				assessmentOnlySeen = true
				if boolValue(request.Input["assessmentOnlyContractCorrection"]) {
					correctionSeen = true
					if _, ok := request.Input[stepOutputKeyExecutionGraph]; ok {
						t.Fatalf("correction input retained executionGraph: %+v", request.Input)
					}
					if previous := mapFromValue(request.Input["previousInvalidAssessmentOutput"]); len(previous) == 0 || previous[stepOutputKeyExecutionGraph] != nil {
						t.Fatalf("previousInvalidAssessmentOutput = %+v, want sanitized contract violation output", previous)
					}
					return map[string]any{
						"status": "completed",
						stepOutputKeyDoDAssessment: map[string]any{
							"satisfied":   true,
							"summary":     "assessment-only correction completed",
							"goalOutcome": "assessment-only correction completed",
							"items": []map[string]any{{
								"item":         "contract corrected",
								"state":        "satisfied",
								"evidenceRefs": []string{string(request.StepID)},
							}},
						},
					}, nil
				}
				return map[string]any{
					"status": "completed",
					stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
						{ID: "must_not_execute", Kind: "graph/mock"},
					}},
				}, nil
			}
			if boolValue(request.Input["graphFollowUp"]) {
				return map[string]any{
					"status": "completed",
					stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
						{
							ID:   runtimeChildID(request.StepID, "verify"),
							Kind: "patch/apply",
							Input: map[string]any{
								"patchType": "file_write",
								"targetRef": "assessment-follow-up.txt",
								"content":   "assessment follow-up\n",
							},
						},
					}},
				}, nil
			}
			if request.Kind == "graph/mock" || request.Kind == "patch/apply" {
				return map[string]any{"status": "completed", "stdout": "graph node completed"}, nil
			}
			return map[string]any{
				"status": "completed",
				stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
					{ID: "inspect", Kind: "graph/mock"},
				}},
			}, nil
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-assessment-only-rejects-graph")
	runID := common.RunID("run-assessment-only-rejects-graph")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-assessment-only-rejects-graph"),
		State:           common.RunStateCreated,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	if !assessmentOnlySeen {
		t.Fatalf("assessment-only follow-up was not reached")
	}
	if !correctionSeen {
		t.Fatalf("assessment-only contract correction follow-up was not reached")
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		steps, _ := stepRepo.ListByRun(ctx, runID)
		t.Fatalf("run = %+v steps=%+v, want completed after contract correction", runItem, steps)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	for _, step := range steps {
		if strings.Contains(string(step.ID), "must_not_execute") {
			t.Fatalf("steps = %+v, assessment-only executionGraph must not execute", steps)
		}
	}
}

func TestAssessmentOnlyContractCorrectionInputStripsRecoveryMarkersAndNestedExecutableFields(t *testing.T) {
	step := &runmodel.Step{
		ID:   "run-correction/zz-llm-after-graph-step-provider-retry-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyAssessmentOnly:        true,
			"providerRetryAssessmentOnly":     true,
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryAttempt:  1,
			stepInputKeyProviderRetryRootStep: "run-correction/zz-llm-after-graph-step-provider-retry-1",
			stepInputKeyRecoveryAction:        string(runtimeRecoveryActionProviderRetry),
			stepInputKeyRecoveryRootStepID:    "run-correction/zz-llm-after-graph-step-provider-retry-1",
			stepInputKeyRecoveryLineageType:   "provider_retry",
			stepInputKeyRecoveryBudgetScope: map[string]any{
				"lineageType": "provider_retry",
				"rootStepId":  "run-correction/zz-llm-after-graph-step-provider-retry-1",
			},
			stepInputKeyRuntimeRepairFollowUp: true,
			"stateChangingOutputRequired":     true,
			"repairObjective":                 "produce_state_changing_output",
			"previousStepOutput": map[string]any{
				"llmDecision": map[string]any{
					stepOutputKeyExecutionGraph: map[string]any{"nodes": []any{map[string]any{"id": "inspect", "kind": "file/list"}}},
					"patchRequests":             []any{map[string]any{"targetRef": "x"}},
				},
				"items": []any{map[string]any{
					"shellCommands": []any{map[string]any{"command": "echo"}},
				}},
			},
		},
	}
	output := map[string]any{
		"llmDecision": map[string]any{
			stepOutputKeyExecutionGraph: map[string]any{"nodes": []any{map[string]any{"id": "inspect", "kind": "file/list"}}},
		},
		stepOutputKeyExecutionGraph: map[string]any{"nodes": []any{map[string]any{"id": "inspect", "kind": "file/list"}}},
	}

	input := assessmentOnlyContractCorrectionStepInput(step, output, errors.New("assessment-only step returned executionGraph"))
	for _, key := range []string{
		"providerRetryAssessmentOnly",
		stepInputKeyProviderRetryFollowUp,
		stepInputKeyProviderRetryAttempt,
		stepInputKeyProviderRetryRootStep,
		stepInputKeyRecoveryAction,
		stepInputKeyRecoveryRootStepID,
		stepInputKeyRecoveryLineageType,
		stepInputKeyRecoveryBudgetScope,
		stepInputKeyRuntimeRepairFollowUp,
		"stateChangingOutputRequired",
		"repairObjective",
	} {
		if _, ok := input[key]; ok {
			t.Fatalf("correction input retained %s: %+v", key, input)
		}
	}
	if input[stepInputKeyAssessmentOnly] != true || input["assessmentOnlyContractCorrection"] != true {
		t.Fatalf("correction input = %+v, want assessment-only correction markers", input)
	}
	previous := mapFromValue(input["previousStepOutput"])
	if nested := mapFromValue(previous["llmDecision"]); nested[stepOutputKeyExecutionGraph] != nil || nested["patchRequests"] != nil {
		t.Fatalf("previousStepOutput retained executable nested fields: %+v", previous)
	}
	items := previous["items"].([]any)
	if item := mapFromValue(items[0]); item["shellCommands"] != nil {
		t.Fatalf("previousStepOutput list retained shellCommands: %+v", previous)
	}
	invalid := mapFromValue(input["previousInvalidAssessmentOutput"])
	if nested := mapFromValue(invalid["llmDecision"]); nested[stepOutputKeyExecutionGraph] != nil {
		t.Fatalf("previousInvalidAssessmentOutput retained nested graph: %+v", invalid)
	}
}

func TestRuntimeOrchestrator_AssessmentOnlyIgnoresGraphWhenAssessmentIsPresent(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var assessmentOnlySeen bool
	var correctionSeen bool
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if boolValue(request.Input[stepInputKeyAssessmentOnly]) {
				assessmentOnlySeen = true
				if boolValue(request.Input["assessmentOnlyContractCorrection"]) {
					correctionSeen = true
				}
				return map[string]any{
					"status": "completed",
					stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
						{ID: "must_not_execute", Kind: "graph/mock"},
					}},
					stepOutputKeyDoDAssessment: map[string]any{
						"satisfied":   true,
						"summary":     "assessment-only completed despite ignored graph",
						"goalOutcome": "assessment-only completed despite ignored graph",
						"items": []map[string]any{{
							"item":         "contract corrected",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						}},
					},
				}, nil
			}
			if boolValue(request.Input["graphFollowUp"]) {
				return map[string]any{
					"status": "completed",
					stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
						{
							ID:   runtimeChildID(request.StepID, "verify"),
							Kind: "patch/apply",
							Input: map[string]any{
								"patchType": "file_write",
								"targetRef": "assessment-repeat.txt",
								"content":   "assessment repeat\n",
							},
						},
					}},
				}, nil
			}
			if request.Kind == "graph/mock" || request.Kind == "patch/apply" {
				return map[string]any{"status": "completed", "stdout": "graph node completed"}, nil
			}
			return map[string]any{
				"status": "completed",
				stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
					{ID: "inspect", Kind: "graph/mock"},
				}},
			}, nil
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-assessment-only-ignores-graph-with-assessment")
	runID := common.RunID("run-assessment-only-ignores-graph-with-assessment")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-assessment-only-ignores-graph-with-assessment"),
		State:           common.RunStateCreated,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if !assessmentOnlySeen {
		t.Fatalf("assessment-only follow-up was not reached")
	}
	if correctionSeen {
		t.Fatalf("assessment-only correction should not be dispatched when a dodAssessment is present")
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		steps, _ := stepRepo.ListByRun(ctx, runID)
		t.Fatalf("run = %+v steps=%+v, want completed after ignoring assessment-only graph", runItem, steps)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	var violationMarked bool
	for _, step := range steps {
		if strings.Contains(string(step.ID), "must_not_execute") {
			t.Fatalf("steps = %+v, assessment-only executionGraph must not execute", steps)
		}
		if boolValue(step.Output["assessmentOnlyContractViolation"]) && boolValue(step.Output["ignoredExecutionGraph"]) {
			violationMarked = true
		}
	}
	if !violationMarked {
		t.Fatalf("steps = %+v, want ignored graph contract marker on assessment step", steps)
	}
}

func TestRuntimeOrchestrator_AssessmentOnlyContractViolationFailsRepeatedGraph(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	var correctionSeen bool
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			if boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
				t.Fatalf("runtime repair follow-up should not be dispatched for repeated assessment-only graph violation")
			}
			if boolValue(request.Input[stepInputKeyAssessmentOnly]) {
				if boolValue(request.Input["assessmentOnlyContractCorrection"]) {
					correctionSeen = true
				}
				return map[string]any{
					"status": "completed",
					stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
						{ID: "must_not_execute", Kind: "graph/mock"},
					}},
				}, nil
			}
			if boolValue(request.Input["graphFollowUp"]) {
				return map[string]any{
					"status": "completed",
					stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
						{
							ID:   runtimeChildID(request.StepID, "verify"),
							Kind: "patch/apply",
							Input: map[string]any{
								"patchType": "file_write",
								"targetRef": "assessment-repeated-graph.txt",
								"content":   "assessment repeated graph\n",
							},
						},
					}},
				}, nil
			}
			if request.Kind == "graph/mock" || request.Kind == "patch/apply" {
				return map[string]any{"status": "completed", "stdout": "graph node completed"}, nil
			}
			return map[string]any{
				"status": "completed",
				stepOutputKeyExecutionGraph: runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
					{ID: "inspect", Kind: "graph/mock"},
				}},
			}, nil
		},
	}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-assessment-only-rejects-repeated-graph")
	runID := common.RunID("run-assessment-only-rejects-repeated-graph")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-assessment-only-rejects-repeated-graph"),
		State:           common.RunStateCreated,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if !correctionSeen {
		t.Fatalf("assessment-only contract correction follow-up was not reached")
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed ||
		!strings.Contains(runItem.FailureReason, "assessment-only step returned executionGraph") {
		steps, _ := stepRepo.ListByRun(ctx, runID)
		t.Fatalf("run = %+v steps=%+v, want failed after repeated contract violation", runItem, steps)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	for _, step := range steps {
		if strings.Contains(string(step.ID), "must_not_execute") {
			t.Fatalf("steps = %+v, assessment-only executionGraph must not execute", steps)
		}
	}
}

func TestRuntimeOrchestrator_AssessmentOnlyDecisionForContinuableCandidateRoutesToRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
			return map[string]any{
				"status": "completed",
				"stage":  "llm_dod_assessment",
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied":   true,
					"summary":     "repair produced implementation evidence",
					"goalOutcome": "implementation evidence is now available",
					"items": []map[string]any{{
						"item":         "produce implementation",
						"state":        "satisfied",
						"evidenceRefs": []string{string(request.StepID)},
					}},
				},
			}, nil
		}
		return map[string]any{
			"status":                           "completed",
			"stage":                            "llm_decision_required",
			stepOutputKeyBlockDecisionReason:   "empty workspace requires execution target confirmation",
			stepOutputKeyBlockDecisionKind:     decisioncommon.ResolutionTypeProvideMissingInfo,
			stepOutputKeyBlockDecisionQuestion: "Initialize the current workspace or provide existing project content?",
			stepOutputKeyBlockDecisionContext: map[string]any{
				"candidateValues": map[string]any{
					"executionTargetMode": []any{"init_from_scratch", "use_existing_project"},
				},
				"missingFields":       []any{"executionTargetMode"},
				"requiresHumanFields": []any{"executionTargetMode"},
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
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-assessment-only-decision-repair")
	runID := common.RunID("run-assessment-only-decision-repair")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          common.GoalID("goal-assessment-only-decision-repair"),
		DoDID:           common.DoDID("dod-assessment-only-decision-repair"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []any{"produce implementation"}},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.dispatchStep(ctx, run, "run-assessment-only-decision-repair/provider-retry", llmcommon.StepKindWorkspaceExecution, map[string]any{
		stepInputKeySoulStage:         soulStageValidation,
		"providerRetryAssessmentOnly": true,
		"providerRetryMode":           "completion_assessment_only",
	}); err != nil {
		t.Fatalf("dispatch assessment-only step: %v", err)
	}

	tickets, err := ticketRepo.ListByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if len(tickets) != 0 {
		ids := make([]common.TicketID, 0, len(tickets))
		for _, ticket := range tickets {
			ids = append(ids, ticket.ID)
		}
		steps, _ := stepRepo.ListByRun(ctx, runID)
		stepSummaries := make([]map[string]any, 0, len(steps))
		for _, step := range steps {
			stepSummaries = append(stepSummaries, map[string]any{
				"id":    step.ID,
				"state": step.State,
				"input": step.Input,
				"stage": stringFromMap(step.Output, "stage"),
				"error": firstNonEmptyString(stringFromMap(step.Output, "error"), stringFromMap(step.Output, "reason")),
			})
		}
		t.Fatalf("tickets = %+v ids=%v steps=%+v, want no decision ticket for continuable assessment-only candidate", tickets, ids, stepSummaries)
	}
	failed, err := stepRepo.Get(ctx, "run-assessment-only-decision-repair/provider-retry")
	if err != nil {
		t.Fatalf("get assessment step: %v", err)
	}
	if failed.State != common.StepStateFailed || failed.Output["assessmentOnlyDecisionConvertedToRepair"] != true {
		t.Fatalf("assessment step = %+v, want failed converted to repair", failed)
	}
	steps, err := stepRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	var repairSeen bool
	for _, step := range steps {
		if boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) {
			repairSeen = true
			break
		}
	}
	if !repairSeen {
		t.Fatalf("steps = %+v, want runtime repair follow-up", steps)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run = %+v, want completed after repair", runItem)
	}
}

func TestRuntimeOrchestrator_ContinuableDecisionRoutesToRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
			return map[string]any{
				"status": "completed",
				"stage":  "llm_dod_assessment",
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied":   true,
					"summary":     "initialized project from scratch",
					"goalOutcome": "project bootstrap evidence is available",
					"items": []map[string]any{{
						"item":         "produce implementation",
						"state":        "satisfied",
						"evidenceRefs": []string{string(request.StepID)},
					}},
				},
			}, nil
		}
		return map[string]any{
			"status":                           "completed",
			"stage":                            "llm_decision_required",
			stepOutputKeyBlockDecisionReason:   "empty workspace requires execution target confirmation",
			stepOutputKeyBlockDecisionKind:     decisioncommon.ResolutionTypeProvideMissingInfo,
			stepOutputKeyBlockDecisionQuestion: "Sync existing code or initialize a new project?",
			stepOutputKeyBlockDecisionContext: map[string]any{
				"candidateValues": map[string]any{
					"initializationPermission": []any{"允许在空目录从零初始化 Go 网关项目", "不允许初始化，仅等待现有代码同步"},
					"workspaceSource":          []any{"提供现有项目文件到当前 workspace"},
				},
				"missingFields":       []any{"workspaceSource", "initializationPermission"},
				"requiresHumanFields": []any{"workspaceSource", "initializationPermission"},
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
			Results:            memory.NewResultRepository(),
			Artifacts:          memory.NewArtifactRepository(),
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-continuable-decision-repair")
	runID := common.RunID("run-continuable-decision-repair")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: t.TempDir(), State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          common.GoalID("goal-continuable-decision-repair"),
		DoDID:           common.DoDID("dod-continuable-decision-repair"),
		State:           common.RunStateRunning,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []any{"produce implementation"}},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.dispatchStep(ctx, run, "run-continuable-decision-repair/step-1", llmcommon.StepKindWorkspaceExecution, map[string]any{
		stepInputKeySoulStage: soulStageExecution,
	}); err != nil {
		t.Fatalf("dispatch step: %v", err)
	}
	tickets, err := ticketRepo.ListByRunID(ctx, runID)
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if len(tickets) != 0 {
		t.Fatalf("tickets = %+v, want no decision ticket for continuable candidate", tickets)
	}
	failed, err := stepRepo.Get(ctx, "run-continuable-decision-repair/step-1")
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	if failed.State != common.StepStateFailed || failed.Output["continuableDecisionConvertedToRepair"] != true {
		t.Fatalf("step = %+v, want continuable decision converted to repair", failed)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run = %+v, want completed after repair", runItem)
	}
}

func TestProviderRetryStepInputUsesAssessmentOnlyModeForCompletionGateFollowUp(t *testing.T) {
	graph := runtimemodel.BlockExecutionGraph{Nodes: []runtimemodel.BlockExecutionNode{
		{ID: "verify", Kind: "graph/mock"},
	}}
	failedStep := &runmodel.Step{
		ID:    "run-provider-retry-assessment/zz-llm-after-graph-verify",
		RunID: "run-provider-retry-assessment",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"graphFollowUp":             true,
			stepOutputKeyExecutionGraph: graph,
			"previousStepOutput":        map[string]any{"large": "payload"},
			"failedStepInput":           map[string]any{"large": strings.Repeat("x", 1024)},
			"failedStepOutput":          map[string]any{"large": strings.Repeat("x", 1024)},
			"executionContext": map[string]any{
				"dialogLanguage": "zh-CN",
				"skills":         []map[string]any{{"id": "skill-provider-retry", "instruction": strings.Repeat("x", 1024)}},
			},
			stepInputKeyExecutionContextPackage: map[string]any{"selectedContext": map[string]any{"skills": []any{map[string]any{"id": "skill-provider-retry"}}}},
		},
		Output: map[string]any{
			"stage":                     "llm_provider_timeout",
			failureOutputKeyClass:       failureClassProviderTimeout,
			failureOutputKeySignature:   "class=provider_timeout|stage=llm_provider_timeout",
			stepOutputKeyExecutionGraph: graph,
		},
	}

	input := providerRetryStepInput(failedStep, 1, errors.New("context deadline exceeded"))

	if input["providerRetryMode"] != "completion_assessment_only" || input["providerRetryAssessmentOnly"] != true || input["completionFollowUp"] != true {
		t.Fatalf("input = %+v, want assessment-only provider retry markers", input)
	}
	if _, ok := input[stepOutputKeyExecutionGraph]; ok {
		t.Fatalf("input retained executionGraph: %+v", input[stepOutputKeyExecutionGraph])
	}
	if _, ok := input["previousStepOutput"]; ok {
		t.Fatalf("input retained previousStepOutput: %+v", input["previousStepOutput"])
	}
	for _, key := range []string{"failedStepInput", stepInputKeyExecutionContextPackage} {
		if _, ok := input[key]; ok {
			t.Fatalf("input retained %s: %+v", key, input[key])
		}
	}
	executionContext := mapFromValue(input["executionContext"])
	if executionContext["dialogLanguage"] != "zh-CN" {
		t.Fatalf("executionContext = %+v, want slim dialog language", executionContext)
	}
	if _, ok := executionContext["skills"]; ok {
		t.Fatalf("executionContext retained raw skills: %+v", executionContext["skills"])
	}
	if input["failedStepOutput"] == nil || input[failureOutputKeyClass] != failureClassProviderTimeout {
		t.Fatalf("input = %+v, want compact failure context", input)
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

func TestValidatedCompletionDoDAssessmentRejectsMissingCurrentDoDCoverage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-dod-coverage-validate",
		WorkspaceID:     "ws-dod-coverage-validate",
		DoDID:           "dod-coverage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{
					"### Acceptance",
					"- [ ] **build passes**",
					"- [ ] **error path covered**",
				},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "build passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-dod-coverage-validate/step-1"},
				},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "does not cover all current DoD items") || !strings.Contains(err.Error(), "error path covered") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want missing DoD coverage", err)
	}
}

func TestValidatedCompletionDoDAssessmentRejectsMissingGoalCoverage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-validate",
		WorkspaceID:     "ws-goal-coverage-validate",
		DoDID:           "dod-goal-coverage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "支持同时代理不同类型的 LLM\n支持通过配置单独停用某个 LLM",
			},
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "base DoD passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-goal-coverage-validate/block/build"},
				},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "dodAssessment.goalCoverage is required") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want missing Goal coverage", err)
	}
}

func TestNormalizedRunGoalCoverageItemsIncludeStableRequirementIDs(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-goal-coverage-stable-id",
		WorkspaceID: "ws-goal-coverage-stable-id",
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "支持流式响应 Token 统计\n支持配置化停用能力",
			},
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}

	items := normalizedRunGoalCoverageItems(run)
	if len(items) != 2 {
		t.Fatalf("goal coverage items = %+v, want two uncovered Goal leaves", items)
	}
	for _, item := range items {
		if !strings.HasPrefix(item.id, "goal-req-") {
			t.Fatalf("goal item id = %q, want stable goal requirement id", item.id)
		}
	}
	again := normalizedRunGoalCoverageItems(run)
	if len(again) != len(items) || again[0].id != items[0].id || again[1].id != items[1].id {
		t.Fatalf("goal item ids = %+v then %+v, want deterministic ids", items, again)
	}
}

func TestValidatedCompletionDoDAssessmentRejectsImplicitGoalCoverageWithoutGoalCoverageMap(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-implicit",
		WorkspaceID:     "ws-goal-coverage-implicit",
		DoDID:           "dod-goal-coverage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "支持通过配置单独停用某个 LLM",
			},
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "goal text appears in a DoD item but no goalCoverage map exists",
			"items": []map[string]any{
				{
					"item":         "支持通过配置单独停用某个 LLM",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-goal-coverage-implicit/block/build"},
				},
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-goal-coverage-implicit/block/build"},
				},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "dodAssessment.goalCoverage is required") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want explicit goalCoverage requirement", err)
	}
}

func TestValidatedCompletionDoDAssessmentRejectsGoalCoverageWithoutEvidenceRefs(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-no-evidence",
		WorkspaceID:     "ws-goal-coverage-no-evidence",
		DoDID:           "dod-goal-coverage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "支持通过配置单独停用某个 LLM",
			},
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "goal coverage lacks evidence",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-goal-coverage-no-evidence/block/build"},
				},
			},
			"goalCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{
					{
						"goal":  "支持通过配置单独停用某个 LLM",
						"state": "satisfied",
					},
				},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "satisfied dodAssessment.goalCoverage item requires evidenceRefs") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want goalCoverage evidenceRefs requirement", err)
	}
}

func TestValidatedCompletionDoDAssessmentAcceptsExplicitGoalCoverage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-coverage-complete",
		WorkspaceID:     "ws-goal-coverage-complete",
		DoDID:           "dod-goal-coverage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "支持同时代理不同类型的 LLM\n支持通过配置单独停用某个 LLM",
			},
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}
	_, summary, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "DoD and Goal coverage passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-goal-coverage-complete/block/build"},
				},
			},
			"goalCoverage": map[string]any{
				"satisfied": true,
				"items": []map[string]any{
					{
						"goal":         "支持同时代理不同类型的 LLM",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-goal-coverage-complete/block/patch"},
					},
					{
						"goal":         "支持通过配置单独停用某个 LLM",
						"state":        "satisfied",
						"evidenceRefs": []string{"run-goal-coverage-complete/block/test-disabled"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	if summary != "DoD and Goal coverage passed" {
		t.Fatalf("summary = %q, want Goal-aware summary", summary)
	}
}

func TestValidatedCompletionDoDAssessmentRejectsMissingRequiredAcceptanceDimension(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-required-dimension-missing",
		WorkspaceID:     "ws-required-dimension-missing",
		DoDID:           "dod-required-dimension",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"acceptanceDimensions": []map[string]any{
							{"id": "runtime_entrypoint", "required": true},
						},
					},
				},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "base DoD passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-required-dimension-missing/block/build"},
				},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "acceptanceDimensions missing required dimensions: runtime_entrypoint") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want missing acceptance dimension", err)
	}
}

func TestValidatedCompletionDoDAssessmentAcceptsRequiredAcceptanceDimension(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-required-dimension-complete",
		WorkspaceID:     "ws-required-dimension-complete",
		DoDID:           "dod-required-dimension",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"acceptanceDimensions": []map[string]any{
							{"id": "runtime_entrypoint", "required": true},
						},
					},
				},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "base DoD and required dimension passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-required-dimension-complete/block/build"},
				},
			},
			"acceptanceDimensions": []map[string]any{
				{
					"id":           "runtime_entrypoint",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-required-dimension-complete/block/entrypoint"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
}

func TestValidatedCompletionDoDAssessmentRejectsMissingRequiredDeliverableCoverage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-required-deliverable-missing",
		WorkspaceID:     "ws-required-deliverable-missing",
		DoDID:           "dod-required-deliverable",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "single_binary", "required": true},
						},
					},
				},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "base DoD passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-required-deliverable-missing/block/build"},
				},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "deliverableCoverage missing required deliverables: single_binary") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want missing deliverable coverage", err)
	}
}

func TestValidatedCompletionDoDAssessmentAcceptsRequiredDeliverableCoverage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-required-deliverable-complete",
		WorkspaceID:     "ws-required-deliverable-complete",
		DoDID:           "dod-required-deliverable",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "single_binary", "required": true},
						},
					},
				},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "base DoD and required deliverable passed",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-required-deliverable-complete/block/build"},
				},
			},
			"deliverableCoverage": map[string]any{
				"items": []map[string]any{
					{
						"id":           "single_binary",
						"state":        "satisfied",
						"artifactRefs": []string{"run-required-deliverable-complete/artifact/result-summary"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
}

func TestValidatedCompletionDoDAssessmentRejectsOpenEvidenceGaps(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-open-evidence-gap",
		WorkspaceID:     "ws-open-evidence-gap",
		DoDID:           "dod-open-evidence-gap",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "base DoD passed but evidence gap remains",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-open-evidence-gap/block/build"},
				},
			},
			"evidenceGaps": []map[string]any{
				{"id": "missing-error-path", "reason": "error branch not verified"},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "dodAssessment.evidenceGaps must be empty") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want open evidence gap rejection", err)
	}
}

func TestValidatedCompletionDoDAssessmentRejectsRequiredBranchCoverageWithoutEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-branch-coverage-no-evidence",
		WorkspaceID:     "ws-branch-coverage-no-evidence",
		DoDID:           "dod-branch-coverage-no-evidence",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "base DoD passed but branch evidence missing",
			"items": []map[string]any{
				{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-branch-coverage-no-evidence/block/build"},
				},
			},
			"branchCoverage": map[string]any{
				"items": []map[string]any{
					{
						"id":       "error_path",
						"required": true,
						"state":    "satisfied",
					},
				},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "satisfied dodAssessment.branchCoverage item") {
		t.Fatalf("validatedCompletionDoDAssessment err = %v, want branch coverage evidence rejection", err)
	}
}

func TestValidatedCompletionDoDAssessmentIgnoresGoalParentItems(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-goal-parent-coverage",
		WorkspaceID:     "ws-goal-parent-coverage",
		DoDID:           "dod-goal-parent-coverage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"text": "### Goal\n- Token 自动审计：\n  - 非流式 usage 字段解析",
			},
			"dod": map[string]any{
				"items": []string{"非流式 usage 字段解析"},
			},
		},
	}
	_, _, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "Goal parent is a grouping label; leaf DoD is covered.",
			"items": []map[string]any{{
				"item":         "非流式 usage 字段解析",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-goal-parent-coverage/block/test"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
}

func TestLLMExecutionGraphNodeReturnedOnlyGraphViolatesContract(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-execution-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-execution-contract/block/implement",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyGraphNodeID: "implement",
			stepInputKeySoulStage:   soulStageExecution,
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "nested-plan",
				"kind":  "llm/workspace_execution",
				"input": map[string]any{"task": "plan again"},
			}},
		},
	}
	if !llmExecutionGraphNodeReturnedOnlyGraph(run, step, output) {
		t.Fatalf("llmExecutionGraphNodeReturnedOnlyGraph = false, want contract violation")
	}
	output[stepOutputKeyPatches] = []map[string]any{{
		"patchType": "file_write",
		"targetRef": "main.go",
		"content":   "package main\n",
	}}
	if llmExecutionGraphNodeReturnedOnlyGraph(run, step, output) {
		t.Fatalf("llmExecutionGraphNodeReturnedOnlyGraph = true, want concrete patch output allowed")
	}
}

func TestLLMExecutionGraphNodeReturnedOnlyGraphAllowsConcretePatchGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-execution-concrete-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-execution-concrete-graph/graph-001/006-plan-impl",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyGraphNodeID:       "plan-impl",
			stepInputKeyRuntimeStage:      runtimeStageExecution,
			"stateChangingOutputRequired": true,
		},
	}
	output := map[string]any{
		"completionState": "continue",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "fix-config",
					"kind": "patch/apply",
					"input": map[string]any{
						"patchType": "file_write",
						"targetRef": "config.yaml",
						"content":   "enabled: true\n",
					},
				},
				{
					"id":        "verify",
					"kind":      "shell/command",
					"dependsOn": []string{"fix-config"},
					"input": map[string]any{
						"command": "go",
						"args":    []string{"test", "./..."},
					},
				},
			},
		},
	}

	if llmExecutionGraphNodeReturnedOnlyGraph(run, step, output) {
		t.Fatalf("llmExecutionGraphNodeReturnedOnlyGraph = true, want concrete patch graph allowed")
	}
	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract: %v, want concrete patch graph allowed", err)
	}
}

func TestRuntimeOrchestrator_DispatchesConcreteGraphReturnedByExecutionLLMNode(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	workspaceID := common.WorkspaceID("ws-execution-concrete-graph")
	runID := common.RunID("run-execution-concrete-graph-dispatch")
	rootStepID := common.StepID(string(runID) + "/step-1")
	llmStepID := common.StepID(string(runID) + "/graph-001/006-plan-impl")
	workingDirectory := t.TempDir()
	nestedGraph := map[string]any{
		"nodes": []map[string]any{
			{
				"id":   "fix-config",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "config.yaml",
					"content":   "enabled: true\n",
				},
			},
			{
				"id":        "verify",
				"kind":      "shell/command",
				"dependsOn": []string{"fix-config"},
				"input": map[string]any{
					"command": "go",
					"args":    []string{"test", "./..."},
				},
			},
		},
	}
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		switch request.StepID {
		case llmStepID:
			return map[string]any{
				"status":          "completed",
				"completionState": "continue",
				"executionGraph":  nestedGraph,
			}, nil
		case common.StepID(string(runID) + "/graph-002/001-fix-config"):
			return map[string]any{"status": "completed"}, nil
		case common.StepID(string(runID) + "/graph-002/002-verify"):
			return map[string]any{"status": "completed"}, nil
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
			ToolCalls:          memory.NewToolCallRepository(),
			Patches:            memory.NewPatchRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", WorkingDirectory: workingDirectory, State: common.WorkspaceStateRunning}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     rootStepID,
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateCompleted,
		Output: map[string]any{"executionGraph": map[string]any{"nodes": []map[string]any{{"id": "plan-impl", "kind": llmWorkspaceExecutionStepKind}}}},
	}); err != nil {
		t.Fatalf("seed root step: %v", err)
	}

	if err := orch.dispatchStepWithWorkspace(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}, llmStepID, llmWorkspaceExecutionStepKind, map[string]any{
		stepInputKeyGraphNodeID:       "plan-impl",
		stepInputKeyGraphRootStepID:   string(rootStepID),
		stepInputKeyRuntimeStage:      runtimeStageExecution,
		"stateChangingOutputRequired": true,
	}, nil); err != nil {
		t.Fatalf("dispatch execution llm node: %v", err)
	}

	for _, id := range []common.StepID{
		llmStepID,
		common.StepID(string(runID) + "/graph-002/001-fix-config"),
		common.StepID(string(runID) + "/graph-002/002-verify"),
	} {
		step, err := stepRepo.Get(ctx, id)
		if err != nil {
			t.Fatalf("get step %s: %v", id, err)
		}
		if step.State != common.StepStateCompleted {
			t.Fatalf("step %s state = %s, want completed", id, step.State)
		}
		if id != llmStepID && step.Input[stepInputKeyGraphRootStepID] != string(llmStepID) {
			t.Fatalf("step %s graph root = %v, want nested root %s", id, step.Input[stepInputKeyGraphRootStepID], llmStepID)
		}
	}
}

func TestValidateLLMExecutionOutputContractRejectsImplementationShellOnly(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-implementation-state-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-implementation-state-contract/block/implement",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageExecution,
			stepInputKeyGraphNodeID:       "unit-001",
			"stateChangingOutputRequired": true,
		},
	}
	output := map[string]any{
		"completionState": "continue",
		"shellCommands": []map[string]any{{
			"command": "go",
			"args":    []string{"test", "./..."},
		}},
		"nextStep": "run-implementation-state-contract/block/test",
		"nextKind": shellcommon.StepKindCommand,
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "no state-changing output") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want state-changing output rejection", err)
	}

	output["patchRequests"] = []map[string]any{{
		"patchType": "file_write",
		"targetRef": "config.yaml",
		"content":   "providers: []\n",
	}}
	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract with patch err = %v, want allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsStateChangingRepairReadOnlyGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-contract/zz-llm-repair-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			stepInputKeySoulStage:         soulStageExecution,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "read_main",
				"kind":  "file/read",
				"input": map[string]any{"path": "main.go"},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "executionGraph without patch/apply") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want state-changing repair read-only graph rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsStateChangingRepairPatchGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-patch-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-patch-graph/zz-llm-repair-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			stepInputKeySoulStage:         soulStageExecution,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "write_config",
					"kind": "patch/apply",
					"input": map[string]any{
						"patchType": "file_write",
						"targetRef": "config.yaml",
						"content":   "enabled: true\n",
					},
				},
				{
					"id":        "inspect_after_patch",
					"kind":      "file/read",
					"dependsOn": []string{"write_config"},
					"input":     map[string]any{"path": "config.yaml"},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want state-changing patch graph allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsStateChangingRepairUnrelatedPatchTarget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-unrelated-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-unrelated-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=main_test.go",
			stepInputKeyRepairOfFailureSig:    "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|exitCode=1",
			"failureTargetRepairContext": []map[string]any{
				{"targetRef": "main_test.go", "exists": true, "contentHash": "hash-main-test"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "write-marker",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "fix-marker.txt",
					"content":   "fixed\n",
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "not related to failure target") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want unrelated repair target rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsStateChangingRepairRelevantPatchTarget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-relevant-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-relevant-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=main_test.go",
			stepInputKeyRepairOfFailureSig:    "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|exitCode=1",
			"failureTargetRepairContext": []map[string]any{
				{"targetRef": "main_test.go", "exists": true, "contentHash": "hash-main-test"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-main-test",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "unified_diff",
					"targetRef": "main_test.go",
					"diff":      "--- a/main_test.go\n+++ b/main_test.go\n@@ -1 +1 @@\n-old\n+new\n",
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want relevant repair patch allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsStateChangingRepairPatchWithoutExplicitTarget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-no-explicit-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-no-explicit-target/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "write-config",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "file_write",
					"targetRef": "config.yaml",
					"content":   "enabled: true\n",
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want repair patch allowed without explicit target scope", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsImplementationPatchGraphGatedByLLMNode(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-gated-patch-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
		},
	}
	step := &runmodel.Step{
		ID:   "run-gated-patch-graph/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStagePlanning,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "analysis",
					"kind": llmWorkspaceExecutionStepKind,
				},
				{
					"id":        "write_main",
					"kind":      "patch/apply",
					"dependsOn": []string{"analysis"},
					"input": map[string]any{
						"patchType": "file_write",
						"targetRef": "main.go",
						"content":   "package main\n",
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "patch/apply depends on llm/workspace_execution") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want gated patch graph rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsImplementationPatchGraphAfterReadOnlyNodes(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-read-then-patch-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
		},
	}
	step := &runmodel.Step{
		ID:   "run-read-then-patch-graph/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStagePlanning,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":    "read_go_mod",
					"kind":  "file/read",
					"input": map[string]any{"path": "go.mod"},
				},
				{
					"id":        "write_main",
					"kind":      "patch/apply",
					"dependsOn": []string{"read_go_mod"},
					"input": map[string]any{
						"patchType": "file_write",
						"targetRef": "main.go",
						"content":   "package main\n",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want read-only prerequisites before patch allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsStateChangingRepairPatchGraphGatedByLLMNode(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-state-changing-repair-gated-patch",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-state-changing-repair-gated-patch/repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			stepInputKeySoulStage:         soulStageExecution,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "analysis",
					"kind": llmWorkspaceExecutionStepKind,
				},
				{
					"id":        "write_config",
					"kind":      "patch/apply",
					"dependsOn": []string{"analysis"},
					"input": map[string]any{
						"patchType": "file_write",
						"targetRef": "config.yaml",
						"content":   "enabled: true\n",
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "patch/apply depends on llm/workspace_execution") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want gated repair patch graph rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsInvalidPatchGraphNode(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-invalid-patch-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-invalid-patch-graph/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeySoulStage: soulStagePlanning,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "write_config",
				"kind":  "patch/apply",
				"input": map[string]any{"patchType": "file_write"},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "executionGraph patch/apply input invalid") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want patch graph input rejection", err)
	}
	if !strings.Contains(err.Error(), "patch targetRef is required") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want targetRef-specific contract reason", err)
	}
}

func TestRuntimeOrchestrator_InvalidPatchGraphTargetRefRoutesToRuntimeRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	var repairInput map[string]any
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		switch {
		case request.StepID == "run-invalid-patch-targetref-repair/step-1":
			return map[string]any{
				"status": "completed",
				"stage":  "llm_execution_graph_plan",
				stepOutputKeyExecutionGraph: map[string]any{
					"nodes": []map[string]any{{
						"id":    "write_config",
						"kind":  "patch/apply",
						"input": map[string]any{"patchType": "file_write", "content": "enabled: true\n"},
					}},
				},
			}, nil
		case boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]):
			repairInput = request.Input
			return map[string]any{
				"status": "completed",
				"stage":  "llm_dod_assessment",
				stepOutputKeyDoDAssessment: map[string]any{
					"satisfied": true,
					"summary":   "contract repair selected a concrete targetRef",
					"items": []map[string]any{{
						"item":         "targetRef repaired",
						"state":        "satisfied",
						"evidenceRefs": []string{string(request.StepID)},
					}},
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
			Results:            resultRepo,
			ToolCalls:          memory.NewToolCallRepository(),
			ValidationResults:  memory.NewValidationResultRepository(),
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)
	workspaceID := common.WorkspaceID("ws-invalid-patch-targetref-repair")
	runID := common.RunID("run-invalid-patch-targetref-repair")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          "goal-invalid-patch-targetref-repair",
		DoDID:           "dod-invalid-patch-targetref-repair",
		State:           common.RunStateCreated,
		InitialStepKind: llmWorkspaceExecutionStepKind,
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
	if repairInput == nil ||
		repairInput[stepInputKeyRuntimeRepairFollowUp] != true ||
		repairInput["repairObjective"] != "produce_state_changing_output" ||
		repairInput["stateChangingOutputRequired"] != true {
		t.Fatalf("repairInput = %+v, want state-changing runtime repair", repairInput)
	}
	if !strings.Contains(fmt.Sprint(repairInput["failureReason"]), "patch targetRef is required") {
		t.Fatalf("repair failureReason = %v, want targetRef-specific reason", repairInput["failureReason"])
	}
}

func TestValidateLLMExecutionOutputContractRejectsFollowUpReadOnlyGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-follow-up-read-only-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-follow-up-read-only-graph/zz-graph-follow-up-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"graphFollowUp":         true,
			stepInputKeySoulStage:   soulStageValidation,
			stepInputKeyGraphNodeID: "validate_implementation",
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "read_main",
				"kind":  "file/read",
				"input": map[string]any{"path": "main.go"},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "read-only executionGraph without patch output") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want follow-up read-only graph rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractUsesRuntimeStageForVerificationFollowUp(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-runtime-stage-follow-up-read-only-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-runtime-stage-follow-up-read-only-graph/follow-up-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageVerification,
			stepInputKeyGraphNodeID:  "validate_implementation",
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "read_main",
				"kind":  "file/read",
				"input": map[string]any{"path": "main.go"},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "read-only executionGraph without patch output") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want runtimeStage-based verification rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsReadOnlyPlanningGraphWithStateChangeContract(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-planning-read-only-with-state-change-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
		},
	}
	step := &runmodel.Step{
		ID:   "run-planning-read-only-with-state-change-contract/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStagePlanning,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "read_config",
				"kind":  "file/read",
				"input": map[string]any{"path": "config.yaml"},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want planning read-only graph allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsReadOnlyPlanningGraphWithoutStateChangeContract(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-planning-read-only-allowed",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "allowed_but_not_required",
			},
		},
	}
	step := &runmodel.Step{
		ID:   "run-planning-read-only-allowed/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStagePlanning,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "read_config",
				"kind":  "file/read",
				"input": map[string]any{"path": "config.yaml"},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want allowed without state-change contract", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsEvidenceRepairShellGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-evidence-repair-shell-graph",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-evidence-repair-shell-graph/repair-001-of-verify",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageRepair,
			"repairObjective":        repairObjectiveConcreteValidationEvidence,
			"evidenceRepairRequired": true,
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":    "verify",
					"kind":  "shell/command",
					"input": map[string]any{"command": "verify", "args": []string{"--bounded"}},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want evidence repair shell graph allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsPatchRepairNoOpValidationGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-patch-repair-noop-validation",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-patch-repair-noop-validation/repair-001-of-graph-001-003-patch",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"failedStepKind":              "patch/preview",
			"failedStepOutput": map[string]any{
				"stage":                   "patch_preview",
				"error":                   "invalid argument: unified diff hunk context mismatch",
				failureOutputKeySignature: "class=runtime_failed|kind=patch/preview|stage=patch_preview",
			},
			"patchRepairContext": []map[string]any{
				{"targetRef": "main.go", "exists": true, "contentHash": "hash-main"},
			},
		},
	}
	output := map[string]any{
		"status":  "completed",
		"stage":   "llm_execution_graph_plan",
		"summary": "Current snapshot already satisfies the intended patch; run bounded verification.",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":    "read-go-mod",
					"kind":  "file/read",
					"input": map[string]any{"path": "go.mod"},
				},
				{
					"id":        "build",
					"kind":      "shell/command",
					"dependsOn": []string{"read-go-mod"},
					"input":     map[string]any{"command": "go", "args": []string{"build"}},
				},
				{
					"id":        "test",
					"kind":      "shell/command",
					"dependsOn": []string{"build"},
					"input":     map[string]any{"command": "go", "args": []string{"test", "./..."}},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want patch no-op validation graph allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsRepairFileWriteForExistingContextTarget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-repair-file-write-existing-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-repair-file-write-existing-target/repair-001-of-graph-001-003-patch",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failedStepKind":                  "patch/apply",
			"patchRepairContext": []map[string]any{
				{"targetRef": "main.go", "exists": true, "contentHash": "hash-main"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":    "rewrite-main",
					"kind":  "patch/apply",
					"input": map[string]any{"targetRef": "main.go", "patchType": "file_write", "content": "package main\n"},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "file_write cannot replace existing target") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want existing target file_write rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractRejectsPatchRepairReadOnlyNoOpGraph(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-patch-repair-readonly-noop",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:   "run-patch-repair-readonly-noop/repair-001-of-graph-001-003-patch",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"failedStepKind":              "patch/apply",
			"failedStepOutput": map[string]any{
				"stage":                   "patch_apply",
				"error":                   "invalid argument: unified diff hunk context mismatch",
				failureOutputKeySignature: "class=patch_apply_failed|kind=patch/apply|stage=patch_apply",
			},
			"patchRepairContext": []map[string]any{
				{"targetRef": "main.go", "exists": true, "contentHash": "hash-main"},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":    "read-main",
					"kind":  "file/read",
					"input": map[string]any{"path": "main.go"},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "executionGraph without patch/apply") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want read-only patch no-op graph rejected", err)
	}
}

func TestValidateLLMExecutionOutputContractRequiresGraphNodeStateChangeOnlyByExplicitInput(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-graph-node-explicit-state-change",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	output := map[string]any{
		"status":  "completed",
		"stage":   "llm_workspace_execution",
		"summary": "read-only node completed",
	}
	readOnlyStep := &runmodel.Step{
		ID:   "run-graph-node-explicit-state-change/graph-001/001-implement-looking-name",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage: runtimeStageExecution,
			stepInputKeyGraphNodeID:  "implement_feature",
		},
	}
	if err := validateLLMExecutionOutputContract(run, readOnlyStep, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want graph node id text ignored without explicit contract", err)
	}

	stateChangingStep := &runmodel.Step{
		ID:   "run-graph-node-explicit-state-change/graph-001/002-unit",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageExecution,
			stepInputKeyGraphNodeID:       "unit-002",
			"stateChangingOutputRequired": true,
		},
	}
	err := validateLLMExecutionOutputContract(run, stateChangingStep, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "no state-changing output") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want explicit state-changing graph node rejection", err)
	}
}

func TestValidatePatchStepInputBeforeRuntimeRejectsPlaceholder(t *testing.T) {
	err := validatePatchStepInputBeforeRuntime("patch/apply", map[string]any{
		"patchType": "file_write",
		"targetRef": "config.yaml",
		"content":   "__FROM_PREVIOUS_STEP_OUTPUT__",
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "unresolved runtime template") {
		t.Fatalf("validatePatchStepInputBeforeRuntime err = %v, want unresolved placeholder rejection", err)
	}
}

func TestValidatePatchStepInputBeforeRuntimeRejectsPlaceholderBaseHash(t *testing.T) {
	err := validatePatchStepInputBeforeRuntime("patch/apply", map[string]any{
		"patchType": "unified_diff",
		"targetRef": "main_test.go",
		"baseHash":  "${read-main-test.output.contentHash}",
		"diff":      "--- a/main_test.go\n+++ b/main_test.go\n@@ -1 +1 @@\n-old\n+new\n",
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "baseHash contains unresolved runtime template") {
		t.Fatalf("validatePatchStepInputBeforeRuntime err = %v, want unresolved baseHash placeholder rejection", err)
	}
}

func TestValidatePatchStepInputBeforeRuntimeRejectsEllipsisUnifiedDiffHunk(t *testing.T) {
	err := validatePatchStepInputBeforeRuntime("patch/apply", map[string]any{
		"patchType": "unified_diff",
		"targetRef": "main_test.go",
		"diff":      "--- a/main_test.go\n+++ b/main_test.go\n@@ ... @@\n-old\n+new\n",
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "diff contains unresolved runtime template") {
		t.Fatalf("validatePatchStepInputBeforeRuntime err = %v, want ellipsis hunk placeholder rejection", err)
	}
}

func TestValidatedCompletionDoDAssessmentCoversCurrentDoDWithMarkdownNormalization(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-dod-coverage-complete",
		WorkspaceID:     "ws-dod-coverage-complete",
		DoDID:           "dod-coverage",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{
					"### Acceptance",
					"- [ ] **build passes**",
					"- [ ] **error path covered**",
				},
			},
		},
	}
	_, summary, err := validatedCompletionDoDAssessment(run, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "all current DoD items passed",
			"items": []map[string]any{
				{
					"item":         "Build passes with go test",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-dod-coverage-complete/step-1"},
				},
				{
					"item":         "Error path covered by regression test",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-dod-coverage-complete/step-2"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("validatedCompletionDoDAssessment: %v", err)
	}
	if summary != "all current DoD items passed" {
		t.Fatalf("summary = %q, want normalized summary", summary)
	}
}

func TestRuntimeStepEvidenceQualityUsesRuntimeOutputMetadata(t *testing.T) {
	run := &runmodel.Run{ID: "run-output-quality", WorkspaceID: "ws-output-quality"}
	step := &runmodel.Step{ID: "run-output-quality/block/verify", RunID: run.ID, Kind: shellcommon.StepKindCommand}
	output := map[string]any{
		"evidenceQuality": string(common.EvidenceQualityInconclusive),
		"evidenceReason":  "runtime adapter reported inconclusive evidence",
		"zeroWork":        true,
		"evidenceTags":    []any{"validation", "zero_work"},
	}
	quality := runtimeStepEvidenceQuality(run, step, common.ValidationResultStatePassed, output)
	if quality.Quality != common.EvidenceQualityInconclusive || !quality.ZeroWork || !strings.Contains(strings.Join(quality.Tags, ","), "zero_work") {
		t.Fatalf("quality = %+v, want inconclusive zero-work metadata", quality)
	}
}

func TestDoDAssessmentEvidenceQualityRejectsInconclusiveStepEvidence(t *testing.T) {
	ctx := context.Background()
	validationRepo := memory.NewValidationResultRepository()
	orch := &RuntimeOrchestrator{evidence: RuntimeEvidenceRepositories{ValidationResults: validationRepo}}
	runID := common.RunID("run-dod-evidence-quality")
	stepID := common.StepID("run-dod-evidence-quality/block/verify")
	if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
		ID:              common.ValidationResultID(string(stepID) + "/validation/runtime-step"),
		WorkspaceID:     "ws-dod-evidence-quality",
		RunID:           runID,
		StepID:          stepID,
		ValidationKind:  "runtime_step",
		State:           common.ValidationResultStatePassed,
		Summary:         "runtime step completed",
		EvidenceQuality: common.EvidenceQualityInconclusive,
		EvidenceReason:  "runtime adapter reported inconclusive evidence",
		ZeroWork:        true,
		EvidenceTags:    []string{"validation", "zero_work"},
	}); err != nil {
		t.Fatalf("seed validation result: %v", err)
	}
	err := orch.validateDoDAssessmentEvidenceQuality(ctx, &runmodel.Run{ID: runID}, map[string]any{
		"items": []map[string]any{
			{
				"item":         "429 passthrough is covered",
				"state":        "satisfied",
				"evidenceRefs": []string{string(stepID)},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "inconclusive") || !strings.Contains(err.Error(), string(stepID)) {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %v, want inconclusive evidence rejection", err)
	}
	var diagnostic runtimeFailureDiagnosticError
	if !errors.As(err, &diagnostic) {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %T, want runtime diagnostics", err)
	}
	diagnostics := diagnostic.RuntimeFailureDiagnostics()
	if stringFromMap(diagnostics, "kind") != "dod_evidence_quality_gap" {
		t.Fatalf("diagnostics = %+v, want dod_evidence_quality_gap", diagnostics)
	}
	missing := mapSliceFromValue(diagnostics["missingDoDItems"])
	if len(missing) != 1 || missing[0]["source"] != "evidenceQuality" || missing[0]["state"] != "failed" {
		t.Fatalf("missingDoDItems = %+v, want failed evidence quality gap", missing)
	}
}

func TestDoDAssessmentEvidenceKindRequirementRejectsReadOnlySideEffectEvidence(t *testing.T) {
	ctx := context.Background()
	validationRepo := memory.NewValidationResultRepository()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			ValidationResults: validationRepo,
		},
	}
	runID := common.RunID("run-dod-evidence-kind")
	readStepID := common.StepID("run-dod-evidence-kind/block/read-usage")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    readStepID,
		RunID: runID,
		Kind:  "file/read",
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("seed read step: %v", err)
	}
	if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
		ID:              common.ValidationResultID(string(readStepID) + "/validation/runtime-step"),
		WorkspaceID:     "ws-dod-evidence-kind",
		RunID:           runID,
		StepID:          readStepID,
		ValidationKind:  "runtime_step",
		State:           common.ValidationResultStatePassed,
		EvidenceQuality: common.EvidenceQualityValid,
	}); err != nil {
		t.Fatalf("seed validation result: %v", err)
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{
							{
								"id":                "persistence_or_side_effect",
								"scope":             "acceptanceDimension",
								"acceptedStepKinds": []string{shellcommon.StepKindCommand, "patch/apply", "mcp/tool_call"},
								"rejectedStepKinds": []string{"file/read", "file/list"},
								"description":       "Read-only evidence cannot prove side effects.",
							},
						},
					},
				},
			},
		},
	}
	err := orch.validateDoDAssessmentEvidenceQuality(ctx, run, map[string]any{
		"items": []map[string]any{{
			"item":         "side effect recorded",
			"state":        "satisfied",
			"evidenceRefs": []string{string(readStepID)},
		}},
		"acceptanceDimensions": []map[string]any{{
			"id":           "persistence_or_side_effect",
			"state":        "satisfied",
			"evidenceRefs": []string{string(readStepID)},
		}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "persistence_or_side_effect") ||
		!strings.Contains(err.Error(), "file/read") {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %v, want read-only side-effect evidence rejection", err)
	}
	var diagnostic runtimeFailureDiagnosticError
	if !errors.As(err, &diagnostic) {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %T, want runtime diagnostics", err)
	}
	missing := mapSliceFromValue(diagnostic.RuntimeFailureDiagnostics()["missingDoDItems"])
	if len(missing) != 1 || missing[0]["source"] != "evidenceKind" {
		t.Fatalf("missingDoDItems = %+v, want evidenceKind diagnostic", missing)
	}
}

func TestDoDAssessmentEvidenceKindRequirementAcceptsActionEvidence(t *testing.T) {
	ctx := context.Background()
	validationRepo := memory.NewValidationResultRepository()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			ValidationResults: validationRepo,
		},
	}
	runID := common.RunID("run-dod-evidence-kind-valid")
	testStepID := common.StepID("run-dod-evidence-kind-valid/block/test")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    testStepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("seed shell step: %v", err)
	}
	if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
		ID:              common.ValidationResultID(string(testStepID) + "/validation/runtime-step"),
		WorkspaceID:     "ws-dod-evidence-kind-valid",
		RunID:           runID,
		StepID:          testStepID,
		ValidationKind:  "runtime_step",
		State:           common.ValidationResultStatePassed,
		EvidenceQuality: common.EvidenceQualityValid,
	}); err != nil {
		t.Fatalf("seed validation result: %v", err)
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{
							{
								"id":                "persistence_or_side_effect",
								"scope":             "acceptanceDimension",
								"acceptedStepKinds": []string{shellcommon.StepKindCommand, "patch/apply", "mcp/tool_call"},
								"rejectedStepKinds": []string{"file/read", "file/list"},
							},
						},
					},
				},
			},
		},
	}
	err := orch.validateDoDAssessmentEvidenceQuality(ctx, run, map[string]any{
		"items": []map[string]any{{
			"item":         "side effect recorded",
			"state":        "satisfied",
			"evidenceRefs": []string{string(testStepID)},
		}},
		"acceptanceDimensions": []map[string]any{{
			"id":           "persistence_or_side_effect",
			"state":        "satisfied",
			"evidenceRefs": []string{string(testStepID)},
		}},
	})
	if err != nil {
		t.Fatalf("validateDoDAssessmentEvidenceQuality: %v", err)
	}
}

func TestDoDAssessmentEvidenceKindRequirementAllowsReadEvidenceWithActionEvidence(t *testing.T) {
	ctx := context.Background()
	validationRepo := memory.NewValidationResultRepository()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			ValidationResults: validationRepo,
		},
	}
	runID := common.RunID("run-dod-evidence-kind-mixed")
	readStepID := common.StepID("run-dod-evidence-kind-mixed/block/read")
	actionStepID := common.StepID("run-dod-evidence-kind-mixed/block/action")
	for _, step := range []*runmodel.Step{
		{ID: readStepID, RunID: runID, Kind: "file/read", State: common.StepStateCompleted},
		{ID: actionStepID, RunID: runID, Kind: shellcommon.StepKindCommand, State: common.StepStateCompleted},
	} {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
		if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
			ID:              common.ValidationResultID(string(step.ID) + "/validation/runtime-step"),
			WorkspaceID:     "ws-dod-evidence-kind-mixed",
			RunID:           runID,
			StepID:          step.ID,
			ValidationKind:  "runtime_step",
			State:           common.ValidationResultStatePassed,
			EvidenceQuality: common.EvidenceQualityValid,
		}); err != nil {
			t.Fatalf("seed validation result for %s: %v", step.ID, err)
		}
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{
							{
								"id":                "persistence_or_side_effect",
								"scope":             "acceptanceDimension",
								"acceptedStepKinds": []string{shellcommon.StepKindCommand, "patch/apply", "mcp/tool_call"},
								"rejectedStepKinds": []string{"file/read", "file/list"},
							},
						},
					},
				},
			},
		},
	}
	err := orch.validateDoDAssessmentEvidenceQuality(ctx, run, map[string]any{
		"acceptanceDimensions": []map[string]any{{
			"id":           "persistence_or_side_effect",
			"state":        "satisfied",
			"evidenceRefs": []string{string(readStepID), string(actionStepID)},
		}},
	})
	if err != nil {
		t.Fatalf("validateDoDAssessmentEvidenceQuality: %v", err)
	}
}

func TestDoDAssessmentEvidenceKindRequirementRejectsRejectedOnlyRequirement(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-dod-evidence-kind-rejected-only")
	readStepID := common.StepID("run-dod-evidence-kind-rejected-only/block/read")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    readStepID,
		RunID: runID,
		Kind:  "file/read",
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("seed read step: %v", err)
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{
							{
								"id":                "runtime_entrypoint",
								"scope":             "acceptanceDimension",
								"rejectedStepKinds": []string{"file/read"},
							},
						},
					},
				},
			},
		},
	}
	err := orch.validateDoDAssessmentEvidenceQuality(ctx, run, map[string]any{
		"acceptanceDimensions": []map[string]any{{
			"id":           "runtime_entrypoint",
			"state":        "satisfied",
			"evidenceRefs": []string{string(readStepID)},
		}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "file/read") {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %v, want rejected-only requirement failure", err)
	}
}

func TestRuntimeOrchestrator_RecordsRejectedResultForUnsatisfiedDoDAssessment(t *testing.T) {
	ctx := context.Background()
	resultRepo := memory.NewResultRepository()
	orch := &RuntimeOrchestrator{evidence: RuntimeEvidenceRepositories{Results: resultRepo}}
	runID := common.RunID("run-rejected-dod")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-rejected-dod",
		GoalID:          "goal-rejected-dod",
		DoDID:           "dod-rejected-dod",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{ID: common.StepID(string(runID) + "/step-1"), RunID: runID}

	if err := orch.recordRejectedCompletionDoDAssessment(ctx, run, step, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"summary":   "DoD not satisfied; missing effective tests.",
			"items": []map[string]any{
				{
					"item":          "effective tests",
					"state":         "failed",
					"failureReason": "no effective assertions",
				},
			},
		},
	}); err != nil {
		t.Fatalf("record rejected result: %v", err)
	}

	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get rejected result: %v", err)
	}
	if result.State != common.ResultStateRejected || result.Summary != "DoD not satisfied; missing effective tests." {
		t.Fatalf("result = %+v, want rejected DoD result", result)
	}
	if result.DoDAssessment.Satisfied {
		t.Fatalf("DoDAssessment = %+v, want unsatisfied", result.DoDAssessment)
	}
	if len(result.DoDAssessment.Items) != 1 ||
		!reflect.DeepEqual(result.DoDAssessment.Items[0].EvidenceRefs, []string{string(step.ID)}) {
		t.Fatalf("DoDAssessment items = %+v, want failed item to cite fallback step evidence", result.DoDAssessment.Items)
	}
}

func TestRuntimeOrchestrator_FailRunPrefersUnsatisfiedDoDSummary(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	resultRepo := memory.NewResultRepository()
	runID := common.RunID("run-unsatisfied-dod-summary-failure")
	workspaceID := common.WorkspaceID("ws-unsatisfied-dod-summary-failure")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-unsatisfied-dod-summary-failure",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runs:       runRepo,
		workspaces: workspaceRepo,
		evidence:   RuntimeEvidenceRepositories{Results: resultRepo},
	}
	summary := "无法完成目标，因为所需外部能力当前不可用。"
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Output: map[string]any{
			"error": "invalid argument: completionState failed has no actionable continuation",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   summary,
				"items": []map[string]any{{
					"item":          "produce requested result",
					"state":         "failed",
					"failureReason": "external capability unavailable",
				}},
			},
		},
	}

	if err := orch.failRunWithRejectedCompletionResult(ctx, run, step, "invalid argument: completionState failed has no actionable continuation"); err != nil {
		t.Fatalf("failRunWithRejectedCompletionResult: %v", err)
	}
	closed, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if closed.State != common.RunStateFailed || closed.FailureReason != summary {
		t.Fatalf("run = %+v, want failed with DoD summary reason %q", closed, summary)
	}
}

func TestRuntimeOrchestrator_RecordsRejectedResultForWeakSatisfiedDoDAssessment(t *testing.T) {
	ctx := context.Background()
	resultRepo := memory.NewResultRepository()
	orch := &RuntimeOrchestrator{evidence: RuntimeEvidenceRepositories{Results: resultRepo}}
	runID := common.RunID("run-rejected-weak-dod")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-rejected-weak-dod",
		GoalID:          "goal-rejected-weak-dod",
		DoDID:           "dod-rejected-weak-dod",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{ID: common.StepID(string(runID) + "/step-1"), RunID: runID}

	if err := orch.recordRejectedCompletionDoDAssessment(ctx, run, step, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied":           true,
			"summary":             "validation completed with inconclusive evidence",
			"goalOutcome":         "done",
			"verificationSummary": "runtime adapter reported inconclusive evidence",
			"items": []map[string]any{
				{
					"item":         "tests pass",
					"state":        "satisfied",
					"evidenceRefs": []string{string(step.ID)},
				},
			},
		},
	}); err != nil {
		t.Fatalf("record rejected result: %v", err)
	}

	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get rejected result: %v", err)
	}
	if result.State != common.ResultStateRejected || result.DoDAssessment.Satisfied {
		t.Fatalf("result = %+v, want rejected unsatisfied DoD result", result)
	}
	if !strings.Contains(result.Summary, "DoD assessment rejected") {
		t.Fatalf("result summary = %q, want rejected weak evidence summary", result.Summary)
	}
}

func TestRuntimeOrchestrator_RecordsSyntheticRejectedResultWithoutDoDAssessment(t *testing.T) {
	ctx := context.Background()
	resultRepo := memory.NewResultRepository()
	orch := &RuntimeOrchestrator{evidence: RuntimeEvidenceRepositories{Results: resultRepo}}
	runID := common.RunID("run-rejected-synthetic-dod")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-rejected-synthetic-dod",
		GoalID:          "goal-rejected-synthetic-dod",
		DoDID:           "dod-rejected-synthetic-dod",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{ID: common.StepID(string(runID) + "/block/implement"), RunID: runID}

	if err := orch.recordRejectedCompletionDoDAssessment(ctx, run, step, map[string]any{
		"error": "state-changing execution-stage llm graph node returned no state-changing output",
	}); err != nil {
		t.Fatalf("record synthetic rejected result: %v", err)
	}

	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get synthetic rejected result: %v", err)
	}
	if result.State != common.ResultStateRejected || result.DoDAssessment.Satisfied {
		t.Fatalf("result = %+v, want rejected unsatisfied synthetic DoD result", result)
	}
	if !strings.Contains(result.Summary, "no state-changing output") {
		t.Fatalf("result summary = %q, want contract failure reason", result.Summary)
	}
	if len(result.DoDAssessment.Items) == 0 || !strings.Contains(result.DoDAssessment.Items[0].Item, "runtime execution contract") {
		t.Fatalf("DoDAssessment = %+v, want synthetic runtime contract item", result.DoDAssessment)
	}
}

func TestRuntimeOrchestrator_FailRunWritesSyntheticRejectedResultForLLMDecisionContractTerminalFailure(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	resultRepo := memory.NewResultRepository()
	runID := common.RunID("run-llm-decision-contract-terminal-result")
	workspaceID := common.WorkspaceID("ws-llm-decision-contract-terminal-result")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          "goal-llm-decision-contract-terminal-result",
		DoDID:           "dod-llm-decision-contract-terminal-result",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runtime:    &stubRuntimeAdapter{},
		runs:       runRepo,
		steps:      stepRepo,
		workspaces: workspaceRepo,
		evidence:   RuntimeEvidenceRepositories{Results: resultRepo},
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/provider-retry-1"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{"providerRetryAssessmentOnly": true},
		Output: map[string]any{
			"status": "failed",
			"stage":  "llm_schema_invalid",
			"error":  "invalid argument: executionGraph patch/apply targetRef is required",
		},
	}

	if err := orch.dispatchRuntimeRepairFollowUp(ctx, run, failedStep, errors.New("invalid argument: executionGraph patch/apply targetRef is required")); err != nil {
		t.Fatalf("dispatchRuntimeRepairFollowUp: %v", err)
	}

	closed, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if closed.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", closed.State)
	}
	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get synthetic rejected result: %v", err)
	}
	if result.State != common.ResultStateRejected || !strings.Contains(result.Summary, "patch/apply targetRef is required") {
		t.Fatalf("result = %+v, want rejected result with schema failure reason", result)
	}
}

func TestRuntimeOrchestrator_RejectedResultAttributesProviderRetryToTerminalStep(t *testing.T) {
	ctx := context.Background()
	resultRepo := memory.NewResultRepository()
	orch := &RuntimeOrchestrator{evidence: RuntimeEvidenceRepositories{Results: resultRepo}}
	runID := common.RunID("run-provider-retry-rejected-source")
	rootStepID := common.StepID(runID + "/follow-up-001/dod-gate")
	retryStepID := providerRetryStepID(runID, rootStepID, 1)
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-provider-retry-rejected-source",
		GoalID:          "goal-provider-retry-rejected-source",
		DoDID:           "dod-provider-retry-rejected-source",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:    retryStepID,
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryRootStep: string(rootStepID),
		},
		Output: map[string]any{
			"stage": "llm_schema_invalid",
			"error": "invalid argument: executionGraph patch/apply targetRef is required",
		},
	}

	if err := orch.recordRejectedCompletionDoDAssessment(ctx, run, step, step.Output); err != nil {
		t.Fatalf("record rejected result: %v", err)
	}

	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get rejected result: %v", err)
	}
	if result.SourceRef != string(retryStepID) {
		t.Fatalf("result sourceRef = %q, want terminal step %q", result.SourceRef, retryStepID)
	}
	if !containsString(result.EvidenceRefs, string(rootStepID)) || !containsString(result.EvidenceRefs, string(retryStepID)) {
		t.Fatalf("result evidenceRefs = %+v, want root and provider retry evidence", result.EvidenceRefs)
	}
}

func TestRuntimeOrchestrator_DispatchNextClosesRunningRunWithRejectedResult(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	resultRepo := memory.NewResultRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "completed"}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Results: resultRepo},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-rejected-result-boundary")
	runID := common.RunID("run-rejected-result-boundary")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-rejected-result-boundary",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     common.StepID(string(runID) + "/step-1"),
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateCompleted,
		Output: map[string]any{"status": "completed"},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := resultRepo.Create(ctx, &resultmodel.Result{
		ID:          common.ResultID(string(runID) + "/result/dod-assessment"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		DoDID:       "dod-rejected-result-boundary",
		State:       common.ResultStateRejected,
		Title:       "Rejected Result",
		Summary:     "DoD not satisfied; stop execution.",
		SourceType:  resultcommon.SourceTypeDoDAssessment,
		SourceRef:   string(runID) + "/step-1",
	}); err != nil {
		t.Fatalf("seed rejected result: %v", err)
	}

	err := orch.DispatchNextStep(ctx, runID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("DispatchNextStep err = %v, want conflict after closing rejected run", err)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime calls = %+v, want no dispatch after rejected result", runtime.calls)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed ||
		runItem.FailureReason != "DoD not satisfied; stop execution." {
		t.Fatalf("run = %+v, want failed with rejected result summary", runItem)
	}
}

func TestRuntimeOrchestrator_RejectedResultSupersededByRecoveryProgress(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-rejected-result-superseded")
	workspaceID := common.WorkspaceID("ws-rejected-result-superseded")
	sourceStepID := common.StepID(runID + "/graph-001/003-build")
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	resultRepo := memory.NewResultRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Results: resultRepo},
		events.NewMemoryHub(),
	)
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, DoDID: "dod", State: common.RunStateRunning, InitialStepKind: llmWorkspaceExecutionStepKind}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: sourceStepID, RunID: runID, Kind: shellcommon.StepKindCommand, State: common.StepStateFailed, Output: map[string]any{"error": "exit status 1"}}); err != nil {
		t.Fatalf("seed source step: %v", err)
	}
	resultID := common.ResultID(runID + "/result/dod-assessment")
	if err := resultRepo.Create(ctx, &resultmodel.Result{
		ID:          resultID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		DoDID:       "dod",
		State:       common.ResultStateRejected,
		Title:       "Rejected Result",
		Summary:     "exit status 1",
		SourceType:  resultcommon.SourceTypeDoDAssessment,
		SourceRef:   string(sourceStepID),
	}); err != nil {
		t.Fatalf("seed rejected result: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-graph-001-003-build"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			"failedStepId":                    string(sourceStepID),
		},
		Output: map[string]any{"status": "completed"},
	}); err != nil {
		t.Fatalf("seed repair step: %v", err)
	}

	if summary, ok := orch.rejectedRunResultSummary(ctx, runID); ok {
		t.Fatalf("rejectedRunResultSummary = %q, true; want superseded stale result", summary)
	}
	result, err := resultRepo.Get(ctx, resultID)
	if err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result.State != common.ResultStateSuperseded {
		t.Fatalf("result state = %s, want superseded", result.State)
	}
}

func TestRuntimeRepairInputForSatisfiedDoDCoverageMissingIsAssessmentOnly(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-dod-coverage-assessment-repair")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-dod-coverage-assessment-repair", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}
	stepRepo := memory.NewStepRepository()
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/graph-001/003-test"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"stdout": "TestStream passed; token statistics include prompt tokens and completion tokens.",
		},
	}); err != nil {
		t.Fatalf("seed evidence step: %v", err)
	}
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/follow-up-graph-test"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			"graphFollowUp": true,
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "DoD passed but goal coverage is missing.",
				"items": []map[string]any{{
					"item":         "tests pass",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-dod-coverage-assessment-repair/graph-001/003-test"},
				}},
			},
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingGoalCoverage": []map[string]any{{
					"raw":   "streaming token accounting",
					"kind":  "leaf",
					"state": "missing",
				}},
			},
		},
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		stepRepo,
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewApprovalRepository(),
		events.NewMemoryHub(),
	)

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, step, 1, errors.New("missing goal coverage"))

	if input[stepInputKeyAssessmentOnly] != true || input[stepInputKeyRuntimeStage] != runtimeStageVerification || input[stepInputKeySoulStage] != soulStageValidation {
		t.Fatalf("input = %+v, want assessment-only verification repair", input)
	}
	if boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("input = %+v, want no state-changing requirement for coverage correction", input)
	}
	if input["repairObjective"] != "complete_dod_goal_coverage_assessment" {
		t.Fatalf("repairObjective = %v, want coverage assessment correction", input["repairObjective"])
	}
	if missing := mapSliceFromValue(input["missingGoalCoverage"]); len(missing) != 1 {
		t.Fatalf("missingGoalCoverage = %+v, want one item", input["missingGoalCoverage"])
	} else if strings.TrimSpace(stringFromMap(missing[0], "goalRequirementId")) == "" {
		t.Fatalf("missingGoalCoverage = %+v, want stable goalRequirementId", input["missingGoalCoverage"])
	} else if candidates := mapSliceFromValue(missing[0]["evidenceCandidates"]); len(candidates) == 0 {
		t.Fatalf("missingGoalCoverage = %+v, want neutral evidence candidates", input["missingGoalCoverage"])
	}
}

func TestCompletionGateCoverageRepairFollowUpCanContinue(t *testing.T) {
	runID := common.RunID("run-dod-coverage-assessment-repair-continues")
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyAssessmentOnly:        true,
			"repairObjective":                 "complete_dod_goal_coverage_assessment",
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "DoD passed but goal coverage is still incomplete.",
				"items": []map[string]any{{
					"item":         "tests pass",
					"state":        "satisfied",
					"evidenceRefs": []string{string(runID) + "/graph-001/003-test"},
				}},
				"goalCoverage": map[string]any{
					"satisfied": true,
					"items": []map[string]any{{
						"item":         "covered goal",
						"state":        "satisfied",
						"evidenceRefs": []string{string(runID) + "/graph-001/003-test"},
					}},
				},
			},
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingGoalCoverage": []map[string]any{{
					"raw":   "streaming token accounting",
					"kind":  "leaf",
					"state": "missing",
				}},
			},
		},
	}

	if shouldCloseCompletionGateFailureWithoutRepair("runtime completion gate failed", step, step.Output) {
		t.Fatalf("coverage repair follow-up should continue through runtime repair budget")
	}
	if input := (&RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}).runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: "ws-dod-coverage-assessment-repair-continues", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}, step, 2, errors.New("missing goal coverage")); input[stepInputKeyAssessmentOnly] != true || input["repairObjective"] != "complete_dod_goal_coverage_assessment" {
		t.Fatalf("repair input = %+v, want continued assessment-only coverage correction", input)
	} else if missing := mapSliceFromValue(input["missingGoalCoverage"]); len(missing) != 1 || missing[0]["raw"] != "streaming token accounting" {
		t.Fatalf("missingGoalCoverage = %+v, want carried forward missing goal coverage", input["missingGoalCoverage"])
	}
}

func TestCompletionGateAssessmentOnlyContradictionWithEvidenceCandidatesContinues(t *testing.T) {
	runID := common.RunID("run-dod-coverage-assessment-contradiction")
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyAssessmentOnly:        true,
			"repairObjective":                 "complete_dod_goal_coverage_assessment",
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-req-001-abcdef12",
				"raw":               "streaming token accounting",
				"evidenceCandidates": []map[string]any{{
					"stepId":     string(runID) + "/graph-001/003-test",
					"kind":       "shell/command",
					"matchScore": 2,
				}},
			}},
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "Goal coverage marked failed despite candidate evidence.",
				"items": []map[string]any{{
					"item":         "tests pass",
					"state":        "satisfied",
					"evidenceRefs": []string{string(runID) + "/graph-001/003-test"},
				}},
				"goalCoverage": map[string]any{
					"satisfied": false,
					"items": []map[string]any{{
						"goalRequirementId": "goal-req-001-abcdef12",
						"item":              "streaming token accounting",
						"state":             "failed",
						"reason":            "not enough evidence",
					}},
				},
			},
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingGoalCoverage": []map[string]any{{
					"goalRequirementId": "goal-req-001-abcdef12",
					"raw":               "streaming token accounting",
					"kind":              "leaf",
					"state":             "missing",
				}},
			},
		},
	}

	if shouldCloseCompletionGateFailureWithoutRepair("runtime completion gate failed", step, step.Output) {
		t.Fatalf("assessment-only contradiction with evidence candidates should get another coverage correction attempt")
	}
	input := (&RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}).runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: "ws-dod-coverage-assessment-contradiction", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}, step, 2, errors.New("missing goal coverage"))
	if input[stepInputKeyAssessmentOnly] != true || input["repairObjective"] != "complete_dod_goal_coverage_assessment" {
		t.Fatalf("repair input = %+v, want continued assessment-only coverage correction", input)
	}
	if missing := mapSliceFromValue(input["missingGoalCoverage"]); len(missing) != 1 || len(mapSliceFromValue(missing[0]["evidenceCandidates"])) == 0 {
		t.Fatalf("missingGoalCoverage = %+v, want inherited evidence candidates", input["missingGoalCoverage"])
	}
}

func TestRuntimeOrchestrator_StateChangingProviderRetryViolationContinuesRecovery(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			stepID := string(request.StepID)
			if strings.Contains(stepID, "/repair-001-of-planning-001-2") {
				return map[string]any{
					"status": "completed",
					stepOutputKeyDoDAssessment: map[string]any{
						"satisfied": false,
						"summary":   "state-changing repair could not produce a valid patch",
						"items": []map[string]any{{
							"item":         "runtime contract repair",
							"state":        "failed",
							"reason":       "LLM kept returning read-only executionGraph",
							"evidenceRefs": []string{stepID},
						}},
					},
				}, nil
			}
			return nil, fmt.Errorf("unexpected step %s", stepID)
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
			Results:            resultRepo,
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-state-changing-retry-closure")
	runID := common.RunID("run-state-changing-retry-closure")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-state-changing-retry-closure",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID("run-state-changing-retry-closure/provider-retry-002-of-repair-001-of-planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryAttempt:  2,
			stepInputKeyProviderRetryRootStep: "run-state-changing-retry-closure/repair-001-of-planning-001",
			stepInputKeyRepairOfStepID:        "run-state-changing-retry-closure/provider-retry-001-of-planning-001",
			stepInputKeyRepairOfRootStepID:    "run-state-changing-retry-closure/step-1",
			"stateChangingOutputRequired":     true,
			"repairObjective":                 "produce_state_changing_output",
		},
		Output: map[string]any{
			"status":                  "completed",
			"stage":                   "llm_execution_graph_plan",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|error=invalid argument: state-changing llm repair step returned read-only executiongraph without patch/apply",
			"error":                   "invalid argument: state-changing llm repair step returned read-only executionGraph without patch/apply",
			stepOutputKeyExecutionGraph: map[string]any{"nodes": []any{
				map[string]any{"id": "inspect", "kind": "file/list"},
			}},
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID("run-state-changing-retry-closure/repair-001-of-planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfStepID:        "run-state-changing-retry-closure/provider-retry-001-of-planning-001",
			stepInputKeyRepairOfRootStepID:    "run-state-changing-retry-closure/step-1",
		},
		Output: map[string]any{
			"status": "failed",
			"stage":  "llm_provider_timeout",
			"error":  "context deadline exceeded",
		},
	}); err != nil {
		t.Fatalf("seed existing repair step: %v", err)
	}

	if err := orch.dispatchRuntimeRepairFollowUp(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-state-changing-retry-closure",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}, failedStep, errors.New("invalid argument: state-changing llm repair step returned read-only executionGraph without patch/apply")); err != nil {
		t.Fatalf("dispatchRuntimeRepairFollowUp: %v", err)
	}

	sawFollowUpRepair := false
	for _, call := range runtime.calls {
		if call == common.StepID("run-state-changing-retry-closure/repair-001-of-planning-001-2") {
			sawFollowUpRepair = true
			break
		}
	}
	if !sawFollowUpRepair {
		t.Fatalf("runtime calls = %+v, want follow-up repair after provider retry contract violation", runtime.calls)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State == common.RunStateRunning {
		t.Fatalf("run state = running, want recovery closure to terminal/blocking state")
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("results empty, want rejected result from failed repair assessment")
	}
}

func TestRuntimeRepairInputRequiresStateChangingForMissingWorkspaceFile(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-missing-file-state-changing-repair")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-missing-file-state-changing-repair",
		DoDID:           "dod-missing-file-state-changing-repair",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/002-inspect-main"),
		RunID: runID,
		Kind:  "file/read",
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "inspect_main",
			stepInputKeyGraphRootStepID: string(runID) + "/step-1",
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "file_read_invalid_path",
			"error":                   "lstat /workspace/main.go: no such file or directory",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=file/read|stage=file_read_invalid_path|node=inspect_main|error=lstat /workspace/main.go: no such file or directory",
		},
	}
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, errors.New("lstat /workspace/main.go: no such file or directory"))
	if input["stateChangingOutputRequired"] != true ||
		input["repairObjective"] != "produce_state_changing_output" {
		t.Fatalf("repair input = %+v, want state-changing repair for missing workspace file", input)
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	if !strings.Contains(guidance, "create it with patch/apply file_write") {
		t.Fatalf("repair guidance = %q, want missing file creation guidance", guidance)
	}
}

func TestRuntimeRepairInputRequiresStateChangingForExecutionShellFailure(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-shell-failure-state-changing-repair")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-shell-failure-state-changing-repair",
		DoDID:           "dod-shell-failure-state-changing-repair",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-003/003-run-go-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:       runtimeStageExecution,
			stepInputKeyGraphNodeID:        "run_go_test",
			stepInputKeyGraphRootStepID:    string(runID) + "/step-1",
			stepInputKeyRepairOfRootStepID: string(runID) + "/step-1",
		},
		Output: map[string]any{
			"status":                  "failed",
			"error":                   "exit status 1",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=run_go_test|exitCode=1",
		},
	}
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, errors.New("exit status 1"))
	if input["stateChangingOutputRequired"] != true ||
		input["repairObjective"] != "produce_state_changing_output" {
		t.Fatalf("repair input = %+v, want execution shell failure to require state-changing repair", input)
	}
	if input[stepInputKeyRepairOfStepID] != string(failedStep.ID) ||
		input["failedGraphNodeId"] != "run_go_test" {
		t.Fatalf("repair input = %+v, want failed graph node lineage", input)
	}
}

func TestRuntimeRepairInputAllowsBoundedRetryForExecutionTimeout(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-shell-timeout-retry")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-shell-timeout-retry",
		DoDID:           "dod-shell-timeout-retry",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-003/003-verify"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageExecution,
			stepInputKeyGraphNodeID:     "verify",
			stepInputKeyGraphRootStepID: string(runID) + "/step-1",
		},
		Output: map[string]any{
			"status":                  "timeout",
			"error":                   "context deadline exceeded",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=verify|exitCode=-1|error=context deadline exceeded",
		},
	}
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, errors.New("context deadline exceeded"))
	if input["stateChangingOutputRequired"] != nil {
		t.Fatalf("repair input = %+v, want timeout repair not to require state-changing output", input)
	}
	if input["repairObjective"] != repairObjectiveTransientExecutionRetry || input["executionTimeoutRetryAllowed"] != true {
		t.Fatalf("repair input = %+v, want transient execution retry objective", input)
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
			"stdout":   "E2E assertions passed\ncoverage ok",
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
		!strings.Contains(commandMemory.Content, string(evidenceStepID)) ||
		!strings.Contains(commandMemory.Content, "signal=E2E assertions passed") {
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
			case "run-graph-approval/graph-001/002-apply":
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
	applyStep, err := stepRepo.Get(ctx, "run-graph-approval/graph-001/002-apply")
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
			if !strings.Contains(string(request.StepID), "/repair-") {
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
			if !strings.Contains(string(request.StepID), "/repair-") {
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

func TestRuntimeOrchestrator_StartRunAllowsStaleTerminalActiveRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-stale-terminal-active")
	staleRunID := common.RunID("run-stale-terminal-active")
	runID := common.RunID("run-start-after-stale-terminal")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateFailed,
		ActiveRunID: staleRunID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: staleRunID, WorkspaceID: workspaceID, State: common.RunStateSuperseded}); err != nil {
		t.Fatalf("seed stale run: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateCreated}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run with stale terminal active run: %v", err)
	}
	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.ActiveRunID == staleRunID {
		t.Fatalf("workspace active run id = %q, want stale terminal run replaced", ws.ActiveRunID)
	}
	if ws.State != common.WorkspaceStateCompleted || ws.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want completed with no stale active run", ws)
	}
}

func TestRuntimeOrchestrator_ReconcileWorkspaceActiveRunClearsTerminalActiveRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-terminal-active-run")
	runID := common.RunID("run-terminal-active-run")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateRunning,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateSuperseded}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.reconcileWorkspaceActiveRun(ctx, workspaceID); err != nil {
		t.Fatalf("reconcile workspace active run: %v", err)
	}
	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.State != common.WorkspaceStateFailed || ws.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want failed with no terminal active run", ws)
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

func TestRuntimeOrchestrator_ResumeProviderConfigRecoveryContinueDispatchesExplicitRetry(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status":          "completed",
		"completionState": "completed",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "provider retry resumed",
			"items": []map[string]any{{
				"item":         "provider retry explicit continue",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-provider-retry-continue/provider-retry-003-of-repair-001"},
			}},
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
		RuntimeEvidenceRepositories{Results: resultRepo},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-provider-retry-continue")
	runID := common.RunID("run-provider-retry-continue")
	rootStepID := common.StepID(string(runID) + "/repair-001")
	failedStepID := common.StepID(string(runID) + "/provider-retry-002-of-repair-001")
	ticketID := common.TicketID(string(failedStepID) + "-provider-retry-exhausted-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateWaitingDecision, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		DoDID:            "dod-provider-retry-continue",
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		stepID := common.StepID(fmt.Sprintf("%s/provider-retry-%03d-of-repair-001", runID, attempt))
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    stepID,
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyProviderRetryFollowUp: true,
				stepInputKeyProviderRetryAttempt:  attempt,
				stepInputKeyProviderRetryRootStep: string(rootStepID),
				"stateChangingOutputRequired":     true,
				"repairObjective":                 repairObjectiveStateChangingOutput,
				stepInputKeySoulStage:             soulStageExecution,
				stepInputKeyRuntimeStage:          runtimeStageRepair,
				stepInputKeyRuntimeStageMode:      "strict_convergence",
			},
			Output: map[string]any{
				"status":                  "failed",
				"stage":                   "llm_provider_unavailable",
				"error":                   "provider unavailable",
				failureOutputKeyClass:     failureClassProviderUnavailable,
				failureOutputKeySignature: "class=provider_unavailable|stage=llm_provider_unavailable|error=provider unavailable",
			},
		}); err != nil {
			t.Fatalf("seed provider retry step %d: %v", attempt, err)
		}
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      failedStepID,
		State:       common.DecisionTicketStateResolved,
		Context: map[string]any{
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyConfigOnly,
		},
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-provider-retry-continue",
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"answer": "继续处理", "recoveryAction": "retry_same_step"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume provider recovery continue: %v", err)
	}
	if len(runtime.calls) != 1 || !strings.Contains(string(runtime.calls[0]), "provider-retry-003-of-repair-001") {
		t.Fatalf("runtime calls = %#v, want explicit provider retry attempt 3", runtime.calls)
	}
	retry, err := stepRepo.Get(ctx, runtime.calls[0])
	if err != nil {
		t.Fatalf("get retry step: %v", err)
	}
	if retry.Input["decisionAuthorizedRetry"] != true {
		t.Fatalf("retry input = %+v, want decisionAuthorizedRetry", retry.Input)
	}
	if retry.Input[stepInputKeyRuntimeStage] != runtimeStageRepair ||
		retry.Input[stepInputKeyRuntimeStageMode] != "strict_convergence" ||
		retry.Input[stepInputKeySoulStage] != soulStageExecution {
		t.Fatalf("retry stage input = %+v, want source repair stage contract preserved", retry.Input)
	}
	if retry.Input["repairObjective"] != repairObjectiveStateChangingOutput || retry.Input["stateChangingOutputRequired"] != true {
		t.Fatalf("retry repair input = %+v, want state-changing repair contract preserved", retry.Input)
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
	resultRepo := memory.NewResultRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "completed",
		"dodAssessment": map[string]any{
			"satisfied": true,
			"summary":   "replan successor completed",
			"items": []map[string]any{{
				"item":         "successor run",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-decision-replan-replan-001/step-1"},
			}},
		},
	}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{Results: resultRepo},
		hub,
	)

	workspaceID := common.WorkspaceID("ws-decision-replan")
	runID := common.RunID("run-decision-replan")
	ticketID := common.TicketID("ticket-decision-replan")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		GoalID:           "goal-decision-replan",
		DoDID:            "dod-decision-replan",
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
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
		t.Fatalf("workspace active run = %q, want empty after completed successor", ws.ActiveRunID)
	}
	successor, err := runRepo.Get(ctx, "run-decision-replan-replan-001")
	if err != nil {
		t.Fatalf("get successor run: %v", err)
	}
	if successor.State != common.RunStateCompleted ||
		successor.WorkspaceID != workspaceID ||
		successor.GoalID != "goal-decision-replan" ||
		successor.DoDID != "dod-decision-replan" ||
		mapFromValue(successor.ExecutionContext["replanContinuation"])["predecessorRunId"] != string(runID) {
		t.Fatalf("successor = %+v, want completed replan continuation with inherited definitions/context", successor)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "run-decision-replan-replan-001/step-1" {
		t.Fatalf("runtime calls = %#v, want successor step dispatched", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunLegacyRepairResolutionPayloadReplans(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "completed",
		"dodAssessment": map[string]any{
			"satisfied": true,
			"summary":   "legacy repair replan successor completed",
			"items": []map[string]any{{
				"item":         "legacy successor run",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-legacy-repair-replan-replan-001/step-1"},
			}},
		},
	}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{Results: resultRepo},
		hub,
	)

	workspaceID := common.WorkspaceID("ws-legacy-repair-replan")
	runID := common.RunID("run-legacy-repair-replan")
	ticketID := common.TicketID("ticket-legacy-repair-replan")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		GoalID:           "goal-legacy-repair-replan",
		DoDID:            "dod-legacy-repair-replan",
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		State:       common.DecisionTicketStateResolved,
		Context: map[string]any{
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
			"failureClass":                   failureClassShellCommand,
			"failureSignature":               "class=shell_command_failed|kind=shell/command|failureTargets=handler.go|exitCode=1",
			"failedStepOutput": map[string]any{
				"error": "exit status 1",
			},
		},
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-legacy-repair-replan"),
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload: map[string]any{
			"answer":         "replan",
			"recoveryAction": "replan",
		},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume legacy repair replan decision: %v", err)
	}

	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateSuperseded {
		t.Fatalf("run state = %q, want superseded", r.State)
	}
	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.ActiveRunID != "" {
		t.Fatalf("workspace active run = %q, want empty after completed successor", ws.ActiveRunID)
	}
	successor, err := runRepo.Get(ctx, "run-legacy-repair-replan-replan-001")
	if err != nil {
		t.Fatalf("get successor run: %v", err)
	}
	if successor.State != common.RunStateCompleted ||
		successor.WorkspaceID != workspaceID ||
		successor.GoalID != "goal-legacy-repair-replan" ||
		successor.DoDID != "dod-legacy-repair-replan" ||
		mapFromValue(successor.ExecutionContext["replanContinuation"])["predecessorRunId"] != string(runID) {
		t.Fatalf("successor = %+v, want completed replan continuation with inherited definitions/context", successor)
	}
	replanContext := mapFromValue(successor.ExecutionContext["replanContinuation"])
	ticketContext := mapFromValue(replanContext["ticketContext"])
	if ticketContext["failureSignature"] != "class=shell_command_failed|kind=shell/command|failureTargets=handler.go|exitCode=1" ||
		mapFromValue(ticketContext["failedStepOutput"])["error"] != "exit status 1" {
		t.Fatalf("replan ticketContext = %+v, want failure evidence carried to successor", ticketContext)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "run-legacy-repair-replan-replan-001/step-1" {
		t.Fatalf("runtime calls = %#v, want successor step dispatched", runtime.calls)
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

func TestRuntimeFailureSignatureSeparatesRepeatedFailuresByTargetObject(t *testing.T) {
	runID := common.RunID("run-target-object-signature")
	stepFor := func(id string, stderr string) *runmodel.Step {
		step := &runmodel.Step{
			ID:    common.StepID(string(runID) + "/" + id),
			RunID: runID,
			Kind:  shellcommon.StepKindCommand,
			State: common.StepStateFailed,
			Input: map[string]any{stepInputKeyGraphNodeID: "run_tests"},
			Output: map[string]any{
				"status":   "failed",
				"command":  "go",
				"args":     []string{"test", "./..."},
				"exitCode": 1,
				"stderr":   stderr,
				"error":    "exit status 1",
			},
		}
		classification := classifyRuntimeFailure(step, step.Output, "runtime step failed", errors.New("exit status 1"))
		attachRuntimeFailureClassification(step.Output, classification)
		return step
	}

	configFailure := stepFor("config", "config/settings.yaml:10:2: missing required value")
	serverFailure := stepFor("policy", "policies/routing.rules:1:1: invalid rule expression")
	configAgain := stepFor("config-again", "./config/settings.yaml:99:1: different line, same target object")

	configSig := stringFromMap(configFailure.Output, failureOutputKeySignature)
	serverSig := stringFromMap(serverFailure.Output, failureOutputKeySignature)
	configAgainSig := stringFromMap(configAgain.Output, failureOutputKeySignature)
	if !strings.Contains(configSig, "failureTargets=config/settings.yaml") || !strings.Contains(serverSig, "failureTargets=policies/routing.rules") {
		t.Fatalf("signatures = config:%q policy:%q, want target-object failure signatures", configSig, serverSig)
	}
	if configSig == serverSig {
		t.Fatalf("signature = %q, want different signatures for different target objects", configSig)
	}
	if configSig != configAgainSig {
		t.Fatalf("same target object signatures differ: %q vs %q", configSig, configAgainSig)
	}
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

func memoryContextContains(items []map[string]any, id string) bool {
	for _, item := range items {
		if item != nil && item["id"] == id {
			return true
		}
	}
	return false
}

func memoryIDsContain(items []common.MemoryID, id common.MemoryID) bool {
	for _, item := range items {
		if item == id {
			return true
		}
	}
	return false
}

func memoryItemsContainRun(items []*memorymodel.MemoryItem, runID common.RunID) bool {
	for _, item := range items {
		if item != nil && item.RunID == runID {
			return true
		}
	}
	return false
}

func runtimeMemoryWriteRequestsMust(t *testing.T, output map[string]any) []runtimeMemoryWriteRequest {
	t.Helper()
	requests, err := runtimeMemoryWriteRequests(output)
	if err != nil {
		t.Fatalf("runtimeMemoryWriteRequests: %v", err)
	}
	return requests
}

func memoryWriteRequestContains(items []runtimeMemoryWriteRequest, memoryType string, contentPart string) bool {
	for _, item := range items {
		if item.MemoryType == memoryType && strings.Contains(item.Content, contentPart) {
			return true
		}
	}
	return false
}
