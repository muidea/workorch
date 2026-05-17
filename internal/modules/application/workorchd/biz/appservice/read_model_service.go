package appservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
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
)

const (
	readModelEventRecordLimit       = 100
	readModelLLMUsageRecordLimit    = 5000
	readModelStepPageLimit          = 100
	readModelMaxStepPageLimit       = 500
	readModelToolCallPageLimit      = 100
	readModelMaxToolCallPageLimit   = 500
	readModelCheckpointPageLimit    = 100
	readModelMaxCheckpointPageLimit = 500
	readModelResultPageLimit        = 100
	readModelMaxResultPageLimit     = 500
	readModelPatchPageLimit         = 100
	readModelMaxPatchPageLimit      = 500
	readModelValidationPageLimit    = 100
	readModelMaxValidationPageLimit = 500
	readModelArtifactPageLimit      = 100
	readModelMaxArtifactPageLimit   = 500
	readModelDecisionPageLimit      = 100
	readModelMaxDecisionPageLimit   = 500
	readModelApprovalPageLimit      = 100
	readModelMaxApprovalPageLimit   = 500
	eventPayloadStringLimit         = 512
	eventPayloadListLimit           = 16
)

type readModelDependencies struct {
	workspaces           repository.WorkspaceRepository
	goals                repository.GoalRepository
	dods                 repository.DoDRepository
	runs                 repository.RunRepository
	steps                repository.StepRepository
	results              repository.ResultRepository
	artifacts            repository.ArtifactRepository
	eventRecords         repository.EventRecordRepository
	memoryRecalls        repository.MemoryRecallTraceRepository
	toolCalls            repository.ToolCallRepository
	mcpCapabilities      repository.MCPCapabilityRepository
	patches              repository.PatchRepository
	validations          repository.ValidationResultRepository
	checkpoints          repository.RuntimeCheckpointRepository
	autoGovernanceTraces repository.AutoGovernanceTraceRepository
	tickets              repository.DecisionTicketRepository
	resolutions          repository.DecisionResolutionRepository
	approvals            repository.ApprovalRepository
	published            []events.WorkorchEvent
}

func ReadRunSnapshotWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret RunSnapshot, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return RunSnapshot{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readRunSnapshot(ctx, deps, common.RunID(runID))
}

func ReadRunSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (RunSnapshot, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return RunSnapshot{}, err
	}
	return ReadRunSnapshotWithSession(ctx, session, runID)
}

func ReadRunSnapshotFromEnv(ctx context.Context, runID string) (RunSnapshot, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return RunSnapshot{}, err
	}
	return ReadRunSnapshotWithSession(ctx, session, runID)
}

func ReadRunStatusWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret RunStatus, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return RunStatus{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readRunStatus(ctx, deps, common.RunID(runID))
}

func ReadRunStatusWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (RunStatus, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return RunStatus{}, err
	}
	return ReadRunStatusWithSession(ctx, session, runID)
}

func ReadRunStatusFromEnv(ctx context.Context, runID string) (RunStatus, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return RunStatus{}, err
	}
	return ReadRunStatusWithSession(ctx, session, runID)
}

func ReadRunObservabilityWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret RunObservability, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return RunObservability{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	snapshot, err := readRunSnapshot(ctx, deps, common.RunID(runID))
	if err != nil {
		return RunObservability{}, err
	}
	return mapRunObservability(snapshot), nil
}

func ReadRunTaskProgressWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret TaskProgress, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return TaskProgress{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	snapshot, err := readRunSnapshot(ctx, deps, common.RunID(runID))
	if err != nil {
		return TaskProgress{}, err
	}
	return runSnapshotTaskProgress(snapshot), nil
}

func ReadRunTaskProgressWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (TaskProgress, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return TaskProgress{}, err
	}
	return ReadRunTaskProgressWithSession(ctx, session, runID)
}

func ReadRunTaskProgressFromEnv(ctx context.Context, runID string) (TaskProgress, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return TaskProgress{}, err
	}
	return ReadRunTaskProgressWithSession(ctx, session, runID)
}

func ReadRunContextBundleWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret ContextBundle, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return ContextBundle{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	snapshot, err := readRunSnapshot(ctx, deps, common.RunID(runID))
	if err != nil {
		return ContextBundle{}, err
	}
	return mapRunContextBundle(snapshot), nil
}

func ReadRunContextBundleWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (ContextBundle, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return ContextBundle{}, err
	}
	return ReadRunContextBundleWithSession(ctx, session, runID)
}

func ReadRunContextBundleFromEnv(ctx context.Context, runID string) (ContextBundle, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return ContextBundle{}, err
	}
	return ReadRunContextBundleWithSession(ctx, session, runID)
}

func ListMemoryRecallTracesWithSession(ctx context.Context, session *RuntimeSession, query MemoryRecallTraceQuery) (ret []MemoryRecallTrace, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)
	items, err := deps.memoryRecalls.List(ctx, repository.MemoryRecallTraceQuery{
		WorkspaceID: common.WorkspaceID(strings.TrimSpace(query.WorkspaceID)),
		RunID:       common.RunID(strings.TrimSpace(query.RunID)),
		StepID:      common.StepID(strings.TrimSpace(query.StepID)),
		Stage:       strings.TrimSpace(query.Stage),
		Source:      strings.TrimSpace(query.Source),
		Status:      strings.TrimSpace(query.Status),
		Limit:       query.Limit,
	})
	if err != nil {
		return nil, err
	}
	return mapMemoryRecallTraces(items), nil
}

func ListMemoryRecallTracesWithBackend(ctx context.Context, backend string, dataRoot string, query MemoryRecallTraceQuery) ([]MemoryRecallTrace, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListMemoryRecallTracesWithSession(ctx, session, query)
}

func ReadRunTaskDecompositionsWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret []TaskDecomposition, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	snapshot, err := readRunSnapshot(ctx, deps, common.RunID(runID))
	if err != nil {
		return nil, err
	}
	return runSnapshotTaskDecompositions(snapshot), nil
}

func ReadRunTaskDecompositionsWithBackend(ctx context.Context, backend string, dataRoot string, runID string) ([]TaskDecomposition, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ReadRunTaskDecompositionsWithSession(ctx, session, runID)
}

func ReadRunTaskDecompositionsFromEnv(ctx context.Context, runID string) ([]TaskDecomposition, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ReadRunTaskDecompositionsWithSession(ctx, session, runID)
}

func ReadRunObservabilityWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (RunObservability, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return RunObservability{}, err
	}
	return ReadRunObservabilityWithSession(ctx, session, runID)
}

func ReadRunObservabilityFromEnv(ctx context.Context, runID string) (RunObservability, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return RunObservability{}, err
	}
	return ReadRunObservabilityWithSession(ctx, session, runID)
}

func RunStatusFromSnapshot(snapshot RunSnapshot) RunStatus {
	return mapRunStatus(snapshot)
}

func RunObservabilityFromSnapshot(snapshot RunSnapshot) RunObservability {
	return mapRunObservability(snapshot)
}

func ListEventRecordsWithSession(ctx context.Context, session *RuntimeSession, query EventRecordQuery) (ret []EventRecord, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items, err := deps.eventRecords.List(ctx, repository.EventRecordQuery{
		WorkspaceID:    common.WorkspaceID(query.WorkspaceID),
		RunID:          common.RunID(query.RunID),
		Topic:          query.Topic,
		SubjectType:    query.SubjectType,
		SubjectID:      query.SubjectID,
		CursorAfterID:  common.EventRecordID(query.CursorAfterID),
		OccurredAfter:  query.OccurredAfter,
		OccurredBefore: query.OccurredBefore,
		Limit:          query.Limit,
	})
	if err != nil {
		return nil, err
	}
	return mapEventRecords(items), nil
}

func ListRunStepsWithSession(ctx context.Context, session *RuntimeSession, query StepQuery) (ret StepPage, err error) {
	if session == nil || strings.TrimSpace(query.RunID) == "" {
		return StepPage{}, common.ErrInvalidArgument
	}
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return StepPage{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	runID := common.RunID(strings.TrimSpace(query.RunID))
	if _, err := deps.runs.Get(ctx, runID); err != nil {
		return StepPage{}, err
	}
	items, err := deps.steps.ListByRun(ctx, runID)
	if err != nil {
		return StepPage{}, err
	}
	return pageRunSteps(items, query), nil
}

func ListRunStepsWithBackend(ctx context.Context, backend string, dataRoot string, query StepQuery) (StepPage, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return StepPage{}, err
	}
	return ListRunStepsWithSession(ctx, session, query)
}

func ListRunToolCallsWithSession(ctx context.Context, session *RuntimeSession, query ToolCallQuery) (ret ToolCallPage, err error) {
	if session == nil || strings.TrimSpace(query.RunID) == "" {
		return ToolCallPage{}, common.ErrInvalidArgument
	}
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return ToolCallPage{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	runID := common.RunID(strings.TrimSpace(query.RunID))
	if _, err := deps.runs.Get(ctx, runID); err != nil {
		return ToolCallPage{}, err
	}
	items, err := deps.toolCalls.ListByRun(ctx, runID)
	if err != nil {
		return ToolCallPage{}, err
	}
	return pageRunToolCalls(items, query), nil
}

func ListRunToolCallsWithBackend(ctx context.Context, backend string, dataRoot string, query ToolCallQuery) (ToolCallPage, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return ToolCallPage{}, err
	}
	return ListRunToolCallsWithSession(ctx, session, query)
}

func ListRunRuntimeCheckpointsWithSession(ctx context.Context, session *RuntimeSession, query RuntimeCheckpointQuery) (ret RuntimeCheckpointPage, err error) {
	if session == nil || strings.TrimSpace(query.RunID) == "" {
		return RuntimeCheckpointPage{}, common.ErrInvalidArgument
	}
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return RuntimeCheckpointPage{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	runID := common.RunID(strings.TrimSpace(query.RunID))
	if _, err := deps.runs.Get(ctx, runID); err != nil {
		return RuntimeCheckpointPage{}, err
	}
	items, err := deps.checkpoints.ListByRun(ctx, runID)
	if err != nil {
		return RuntimeCheckpointPage{}, err
	}
	return pageRunRuntimeCheckpoints(items, query), nil
}

func ListRunRuntimeCheckpointsWithBackend(ctx context.Context, backend string, dataRoot string, query RuntimeCheckpointQuery) (RuntimeCheckpointPage, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return RuntimeCheckpointPage{}, err
	}
	return ListRunRuntimeCheckpointsWithSession(ctx, session, query)
}

func ListRunResultsWithSession(ctx context.Context, session *RuntimeSession, query ResultQuery) (ret ResultPage, err error) {
	if session == nil || strings.TrimSpace(query.RunID) == "" {
		return ResultPage{}, common.ErrInvalidArgument
	}
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return ResultPage{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	runID := common.RunID(strings.TrimSpace(query.RunID))
	if _, err := deps.runs.Get(ctx, runID); err != nil {
		return ResultPage{}, err
	}
	items, err := deps.results.ListByRun(ctx, runID)
	if err != nil {
		return ResultPage{}, err
	}
	return pageRunResults(items, query), nil
}

func ListRunResultsWithBackend(ctx context.Context, backend string, dataRoot string, query ResultQuery) (ResultPage, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return ResultPage{}, err
	}
	return ListRunResultsWithSession(ctx, session, query)
}

func ListRunPatchesWithSession(ctx context.Context, session *RuntimeSession, query PatchQuery) (ret PatchPage, err error) {
	if session == nil || strings.TrimSpace(query.RunID) == "" {
		return PatchPage{}, common.ErrInvalidArgument
	}
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return PatchPage{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	runID := common.RunID(strings.TrimSpace(query.RunID))
	if _, err := deps.runs.Get(ctx, runID); err != nil {
		return PatchPage{}, err
	}
	items, err := deps.patches.ListByRun(ctx, runID)
	if err != nil {
		return PatchPage{}, err
	}
	return pageRunPatches(items, query), nil
}

func ListRunPatchesWithBackend(ctx context.Context, backend string, dataRoot string, query PatchQuery) (PatchPage, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return PatchPage{}, err
	}
	return ListRunPatchesWithSession(ctx, session, query)
}

func ListRunValidationResultsWithSession(ctx context.Context, session *RuntimeSession, query ValidationResultQuery) (ret ValidationResultPage, err error) {
	if session == nil || strings.TrimSpace(query.RunID) == "" {
		return ValidationResultPage{}, common.ErrInvalidArgument
	}
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return ValidationResultPage{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	runID := common.RunID(strings.TrimSpace(query.RunID))
	if _, err := deps.runs.Get(ctx, runID); err != nil {
		return ValidationResultPage{}, err
	}
	items, err := deps.validations.ListByRun(ctx, runID)
	if err != nil {
		return ValidationResultPage{}, err
	}
	return pageRunValidationResults(items, query), nil
}

func ListRunValidationResultsWithBackend(ctx context.Context, backend string, dataRoot string, query ValidationResultQuery) (ValidationResultPage, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return ValidationResultPage{}, err
	}
	return ListRunValidationResultsWithSession(ctx, session, query)
}

func ListRunArtifactsWithSession(ctx context.Context, session *RuntimeSession, query ArtifactQuery) (ret ArtifactPage, err error) {
	if session == nil || strings.TrimSpace(query.RunID) == "" {
		return ArtifactPage{}, common.ErrInvalidArgument
	}
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return ArtifactPage{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	runID := common.RunID(strings.TrimSpace(query.RunID))
	if _, err := deps.runs.Get(ctx, runID); err != nil {
		return ArtifactPage{}, err
	}
	items, err := deps.artifacts.ListByRun(ctx, runID)
	if err != nil {
		return ArtifactPage{}, err
	}
	return pageRunArtifacts(items, query), nil
}

func ListRunArtifactsWithBackend(ctx context.Context, backend string, dataRoot string, query ArtifactQuery) (ArtifactPage, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return ArtifactPage{}, err
	}
	return ListRunArtifactsWithSession(ctx, session, query)
}

func ListEventRecordsWithBackend(ctx context.Context, backend string, dataRoot string, query EventRecordQuery) ([]EventRecord, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListEventRecordsWithSession(ctx, session, query)
}

func ListEventRecordsFromEnv(ctx context.Context, query EventRecordQuery) ([]EventRecord, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ListEventRecordsWithSession(ctx, session, query)
}

func ReadWorkspaceSnapshotWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret WorkspaceSnapshot, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readWorkspaceSnapshot(ctx, deps, common.WorkspaceID(workspaceID), nil)
}

func ReadWorkspaceSnapshotWithRunSnapshotWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, runSnapshot RunSnapshot) (ret WorkspaceSnapshot, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readWorkspaceSnapshot(ctx, deps, common.WorkspaceID(workspaceID), &runSnapshot)
}

func ReadWorkspaceTaskProgressWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret TaskProgress, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return TaskProgress{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readWorkspaceTaskProgress(ctx, deps, common.WorkspaceID(workspaceID))
}

func ReadWorkspaceTaskProgressWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (TaskProgress, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return TaskProgress{}, err
	}
	return ReadWorkspaceTaskProgressWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceTaskProgressFromEnv(ctx context.Context, workspaceID string) (TaskProgress, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return TaskProgress{}, err
	}
	return ReadWorkspaceTaskProgressWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceContextBundleWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret ContextBundle, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return ContextBundle{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readWorkspaceContextBundle(ctx, deps, common.WorkspaceID(workspaceID))
}

func ReadWorkspaceContextBundleWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (ContextBundle, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return ContextBundle{}, err
	}
	return ReadWorkspaceContextBundleWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceContextBundleFromEnv(ctx context.Context, workspaceID string) (ContextBundle, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return ContextBundle{}, err
	}
	return ReadWorkspaceContextBundleWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceTaskDecompositionsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret []TaskDecomposition, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readWorkspaceTaskDecompositions(ctx, deps, common.WorkspaceID(workspaceID))
}

func ReadWorkspaceTaskDecompositionsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]TaskDecomposition, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ReadWorkspaceTaskDecompositionsWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceTaskDecompositionsFromEnv(ctx context.Context, workspaceID string) ([]TaskDecomposition, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ReadWorkspaceTaskDecompositionsWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (WorkspaceSnapshot, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	return ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceSnapshotFromEnv(ctx context.Context, workspaceID string) (WorkspaceSnapshot, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	return ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
}

func readModelDependenciesFromSession(session *RuntimeSession) (readModelDependencies, error) {
	exports, err := session.Exports()
	if err != nil {
		return readModelDependencies{}, err
	}
	if exports.Repositories.Workspaces == nil ||
		exports.Repositories.Runs == nil ||
		exports.Repositories.Steps == nil ||
		exports.Repositories.Results == nil ||
		exports.Repositories.Artifacts == nil ||
		exports.Repositories.EventRecords == nil ||
		exports.Repositories.MemoryRecallTraces == nil ||
		exports.Repositories.ToolCalls == nil ||
		exports.Repositories.MCPCapabilities == nil ||
		exports.Repositories.Patches == nil ||
		exports.Repositories.ValidationResults == nil ||
		exports.Repositories.RuntimeCheckpoints == nil ||
		exports.Repositories.Tickets == nil ||
		exports.Repositories.Resolutions == nil ||
		exports.Repositories.Approvals == nil {
		return readModelDependencies{}, common.ErrInvalidArgument
	}

	var published []events.WorkorchEvent
	if session.Runtime != nil {
		published = session.Runtime.PublishedWorkorchEvents()
	}

	return readModelDependencies{
		workspaces:           exports.Repositories.Workspaces,
		goals:                exports.Repositories.Goals,
		dods:                 exports.Repositories.DoDs,
		runs:                 exports.Repositories.Runs,
		steps:                exports.Repositories.Steps,
		results:              exports.Repositories.Results,
		artifacts:            exports.Repositories.Artifacts,
		eventRecords:         exports.Repositories.EventRecords,
		memoryRecalls:        exports.Repositories.MemoryRecallTraces,
		toolCalls:            exports.Repositories.ToolCalls,
		mcpCapabilities:      exports.Repositories.MCPCapabilities,
		patches:              exports.Repositories.Patches,
		validations:          exports.Repositories.ValidationResults,
		checkpoints:          exports.Repositories.RuntimeCheckpoints,
		autoGovernanceTraces: exports.Repositories.AutoGovernanceTraces,
		tickets:              exports.Repositories.Tickets,
		resolutions:          exports.Repositories.Resolutions,
		approvals:            exports.Repositories.Approvals,
		published:            published,
	}, nil
}

func readRunSnapshot(ctx context.Context, deps readModelDependencies, runID common.RunID) (RunSnapshot, error) {
	item, err := deps.runs.Get(ctx, runID)
	if err != nil {
		return RunSnapshot{}, err
	}
	var workspace *workspacemodel.Workspace
	var steps []*runmodel.Step
	var toolCalls []*runmodel.ToolCall
	var patches []*runmodel.Patch
	var validations []*runmodel.ValidationResult
	var checkpoints []*runmodel.RuntimeCheckpoint
	var results []*resultmodel.Result
	var artifacts []*resultmodel.Artifact
	var eventRecords []*eventstreammodel.EventRecord
	var llmUsageEventRecords []*eventstreammodel.EventRecord
	var memoryRecallTraces []*memorymodel.MemoryRecallTrace
	var decisions []Decision
	var selectedTicket *decisionmodel.Ticket
	var selectedResolution *decisionmodel.Resolution
	var approvals []Approval
	var selectedApproval *approvalmodel.Approval
	var mcpCapabilityItems []*mcpmodel.Capability
	tasks := []func() error{
		func() error {
			var err error
			workspace, err = deps.workspaces.Get(ctx, item.WorkspaceID)
			return err
		},
		func() error {
			var err error
			steps, err = deps.steps.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			toolCalls, err = deps.toolCalls.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			patches, err = deps.patches.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			validations, err = deps.validations.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			checkpoints, err = deps.checkpoints.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			results, err = deps.results.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			artifacts, err = deps.artifacts.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			eventRecords, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{RunID: item.ID, Limit: readModelEventRecordLimit})
			return err
		},
		func() error {
			var err error
			llmUsageEventRecords, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{RunID: item.ID, Topic: events.TopicRunLLMUsageRecorded, Limit: readModelLLMUsageRecordLimit})
			return err
		},
		func() error {
			var err error
			if deps.memoryRecalls != nil {
				memoryRecallTraces, err = deps.memoryRecalls.List(ctx, repository.MemoryRecallTraceQuery{RunID: item.ID})
			}
			return err
		},
		func() error {
			var err error
			decisions, selectedTicket, selectedResolution, err = decisionsForRun(ctx, deps, item)
			return err
		},
		func() error {
			var err error
			approvals, selectedApproval, err = approvalsForRun(ctx, deps, item)
			return err
		},
	}
	if deps.mcpCapabilities != nil {
		tasks = append(tasks, func() error {
			var err error
			mcpCapabilityItems, err = deps.mcpCapabilities.List(ctx)
			return err
		})
	}
	if err := runReadModelTasks(tasks...); err != nil {
		return RunSnapshot{}, err
	}

	observedItems := observed.BuildWorkorchSnapshot(workspace, item, steps, selectedTicket, selectedResolution, selectedApproval, deps.published)
	stepViews := presenter.MapSteps(steps)
	currentStep := currentRunSnapshotStep(
		stepViews,
		decisionStepID(selectedTicket),
		approvalStepID(selectedApproval),
		mapRuntimeCheckpoints(checkpoints),
	)

	ret := RunSnapshot{
		Workspace:          mapWorkspaceWithRunProjection(workspace, item),
		Run:                mapRun(item),
		Step:               currentStep,
		Steps:              stepViews,
		ToolCalls:          mapToolCalls(toolCalls),
		Patches:            mapPatches(patches),
		WebSources:         mapWebSourceTraces(stepViews),
		LLMContextBudgets:  mapLLMContextBudgetTraces(stepViews),
		LLMUsageTraces:     mapLLMUsageTracesFromEventRecords(mapEventRecords(llmUsageEventRecords), string(item.ID)),
		ValidationResults:  mapValidationResults(validations),
		RuntimeCheckpoints: mapRuntimeCheckpoints(checkpoints),
		Results:            mapResults(results),
		Artifacts:          mapArtifacts(artifacts),
		MemoryRecallTraces: mapMemoryRecallTraces(memoryRecallTraces),
		EventRecords:       mapProjectedEventRecords(eventRecords),
		Decisions:          decisions,
		Approvals:          approvals,
		Observed:           presenter.MapObserved(observedItems),
	}
	ret.LLMUsageSummary = summarizeLLMUsage(ret.LLMUsageTraces)
	ret.Outcome = mapRunOutcome(ret)
	capabilityCatalog := runtimeCapabilityCatalogFromItems(ret.Run.ExecutionContext, ret.ToolCalls, mcpCapabilityItems)
	ret.SelectedCapabilities = selectedRuntimeCapabilities(ret.Run.ExecutionContext, capabilityCatalog)
	ret.UsedCapabilities = usedRuntimeCapabilities(ret, capabilityCatalog)
	ret.TaskProgress = mapTaskProgress(ret)
	ret.RepairLoop = ret.TaskProgress.RepairLoop
	ret.TaskDecompositions = mapTaskDecompositions(ret)
	ret.TaskPlans = mapTaskPlans(ret)
	ret.ClosureTraces = mapClosureTraces(ret)
	ret.DoDEvidence = mapDoDEvidence(ret)
	ret.ResultQuality = mapResultQuality(ret)
	ret.ExecutionSections = mapExecutionSections(ret)
	return ret, nil
}

func readRunRecoverySnapshot(ctx context.Context, deps readModelDependencies, runID common.RunID) (RunSnapshot, error) {
	item, err := deps.runs.Get(ctx, runID)
	if err != nil {
		return RunSnapshot{}, err
	}
	var workspace *workspacemodel.Workspace
	var steps []*runmodel.Step
	var toolCalls []*runmodel.ToolCall
	var patches []*runmodel.Patch
	var validations []*runmodel.ValidationResult
	var checkpoints []*runmodel.RuntimeCheckpoint
	var results []*resultmodel.Result
	var artifacts []*resultmodel.Artifact
	var eventRecords []*eventstreammodel.EventRecord
	var llmUsageEventRecords []*eventstreammodel.EventRecord
	var memoryRecallTraces []*memorymodel.MemoryRecallTrace
	var decisions []Decision
	var approvals []Approval
	err = runReadModelTasks(
		func() error {
			var err error
			workspace, err = deps.workspaces.Get(ctx, item.WorkspaceID)
			return err
		},
		func() error {
			var err error
			steps, err = deps.steps.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			toolCalls, err = deps.toolCalls.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			patches, err = deps.patches.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			validations, err = deps.validations.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			checkpoints, err = deps.checkpoints.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			results, err = deps.results.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			artifacts, err = deps.artifacts.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			eventRecords, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{RunID: item.ID, Limit: readModelEventRecordLimit})
			return err
		},
		func() error {
			var err error
			llmUsageEventRecords, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{RunID: item.ID, Topic: events.TopicRunLLMUsageRecorded, Limit: readModelLLMUsageRecordLimit})
			return err
		},
		func() error {
			var err error
			if deps.memoryRecalls != nil {
				memoryRecallTraces, err = deps.memoryRecalls.List(ctx, repository.MemoryRecallTraceQuery{RunID: item.ID})
			}
			return err
		},
		func() error {
			var selectedTicket *decisionmodel.Ticket
			var selectedResolution *decisionmodel.Resolution
			var err error
			decisions, selectedTicket, selectedResolution, err = decisionsForRun(ctx, deps, item)
			_, _ = selectedTicket, selectedResolution
			return err
		},
		func() error {
			var selectedApproval *approvalmodel.Approval
			var err error
			approvals, selectedApproval, err = approvalsForRun(ctx, deps, item)
			_ = selectedApproval
			return err
		},
	)
	if err != nil {
		return RunSnapshot{}, err
	}
	stepViews := presenter.MapSteps(steps)
	currentStep := currentRunSnapshotStep(stepViews, "", "", mapRuntimeCheckpoints(checkpoints))
	ret := RunSnapshot{
		Workspace:          mapWorkspaceWithRunProjection(workspace, item),
		Run:                mapRun(item),
		Step:               currentStep,
		Steps:              stepViews,
		ToolCalls:          mapToolCalls(toolCalls),
		Patches:            mapPatches(patches),
		LLMContextBudgets:  mapLLMContextBudgetTraces(stepViews),
		LLMUsageTraces:     mapLLMUsageTracesFromEventRecords(mapEventRecords(llmUsageEventRecords), string(item.ID)),
		ValidationResults:  mapValidationResults(validations),
		RuntimeCheckpoints: mapRuntimeCheckpoints(checkpoints),
		Results:            mapResults(results),
		Artifacts:          mapArtifacts(artifacts),
		MemoryRecallTraces: mapMemoryRecallTraces(memoryRecallTraces),
		EventRecords:       mapProjectedEventRecords(eventRecords),
		Decisions:          decisions,
		Approvals:          approvals,
	}
	ret.LLMUsageSummary = summarizeLLMUsage(ret.LLMUsageTraces)
	ret.Outcome = mapRunOutcome(ret)
	ret.TaskProgress = mapTaskProgress(ret)
	ret.RepairLoop = ret.TaskProgress.RepairLoop
	ret.TaskPlans = mapTaskPlans(ret)
	ret.DoDEvidence = mapDoDEvidence(ret)
	ret.ResultQuality = mapResultQuality(ret)
	return ret, nil
}

func readRunStatus(ctx context.Context, deps readModelDependencies, runID common.RunID) (RunStatus, error) {
	item, err := deps.runs.Get(ctx, runID)
	if err != nil {
		return RunStatus{}, err
	}
	var steps []*runmodel.Step
	var checkpoints []*runmodel.RuntimeCheckpoint
	var decisions []*decisionmodel.Ticket
	var approvals []*approvalmodel.Approval
	err = runReadModelTasks(
		func() error {
			var err error
			steps, err = deps.steps.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			if deps.checkpoints == nil {
				return nil
			}
			var err error
			checkpoints, err = deps.checkpoints.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			decisions, err = deps.tickets.ListByRunID(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			approvals, err = deps.approvals.ListByRunID(ctx, item.ID)
			return err
		},
	)
	if err != nil {
		return RunStatus{}, err
	}
	ret := RunStatus{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		State:              string(item.State),
		StepCount:          len(steps),
		DecisionCount:      len(decisions),
		ApprovalCount:      len(approvals),
		BlockingTicketID:   string(item.BlockingTicketID),
		BlockingApprovalID: string(item.BlockingApprovalID),
		FailureReason:      item.FailureReason,
	}
	if currentStep := currentRunStatusStep(item, steps, checkpoints, decisions, approvals); currentStep != nil {
		ret.CurrentStepID = string(currentStep.ID)
		ret.CurrentStepKind = currentStep.Kind
		ret.CurrentStepState = string(currentStep.State)
	}
	return ret, nil
}

func runReadModelTasks(tasks ...func() error) error {
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var ret error
	recordErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		defer errMu.Unlock()
		if ret == nil {
			ret = err
		}
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		wg.Add(1)
		go func(task func() error) {
			defer wg.Done()
			recordErr(task())
		}(task)
	}
	wg.Wait()
	return ret
}

func readWorkspaceSnapshot(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID, cachedRunSnapshot *RunSnapshot) (WorkspaceSnapshot, error) {
	var item *workspacemodel.Workspace
	if cachedRunSnapshot != nil && cachedRunSnapshot.Workspace.ID == string(workspaceID) {
		item = workspaceModelFromView(cachedRunSnapshot.Workspace)
	}

	var runItems []*runmodel.Run
	var goalItems []*workspacemodel.Goal
	var dodItems []*workspacemodel.DoD
	var resultItems []*resultmodel.Result
	var eventRecordItems []*eventstreammodel.EventRecord
	var llmUsageEventRecordItems []*eventstreammodel.EventRecord
	var artifactItems []*resultmodel.Artifact
	var ticketItems []*decisionmodel.Ticket
	var approvalItems []*approvalmodel.Approval
	var patchItems []*runmodel.Patch
	tasks := []func() error{
		func() error {
			var err error
			runItems, err = deps.runs.ListByWorkspaceID(ctx, workspaceID)
			return err
		},
		func() error {
			var err error
			resultItems, err = deps.results.ListByWorkspace(ctx, workspaceID)
			return err
		},
		func() error {
			var err error
			eventRecordItems, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID, Limit: readModelEventRecordLimit})
			return err
		},
		func() error {
			var err error
			llmUsageEventRecordItems, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID, Topic: events.TopicRunLLMUsageRecorded, Limit: readModelLLMUsageRecordLimit})
			return err
		},
		func() error {
			var err error
			artifactItems, err = deps.artifacts.ListByWorkspace(ctx, workspaceID)
			return err
		},
		func() error {
			var err error
			ticketItems, err = deps.tickets.ListByWorkspaceID(ctx, workspaceID)
			return err
		},
		func() error {
			var err error
			approvalItems, err = deps.approvals.ListByWorkspaceID(ctx, workspaceID)
			return err
		},
	}
	if deps.goals != nil {
		tasks = append(tasks, func() error {
			var err error
			goalItems, err = deps.goals.ListByWorkspaceID(ctx, workspaceID)
			return err
		})
	}
	if deps.dods != nil {
		tasks = append(tasks, func() error {
			var err error
			dodItems, err = deps.dods.ListByWorkspaceID(ctx, workspaceID)
			return err
		})
	}
	if item == nil {
		tasks = append(tasks, func() error {
			var err error
			item, err = deps.workspaces.Get(ctx, workspaceID)
			return err
		})
	}
	err := runReadModelTasks(tasks...)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	runs := make([]Run, 0, len(runItems))
	projectionRun := selectWorkspaceProjectionRun(item, runItems, resultItems, eventRecordItems)
	for _, runItem := range runItems {
		if runItem.WorkspaceID != workspaceID {
			continue
		}
		runs = append(runs, mapRun(runItem))
		if deps.patches != nil {
			items, err := deps.patches.ListByRun(ctx, runItem.ID)
			if err != nil {
				return WorkspaceSnapshot{}, err
			}
			patchItems = append(patchItems, items...)
		}
	}
	decisions := make([]Decision, 0, len(ticketItems))
	resolutions, err := resolutionsForTickets(ctx, deps.resolutions, ticketItems)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	for _, ticket := range ticketItems {
		if ticket.WorkspaceID != workspaceID {
			continue
		}
		summary, err := latestAutoGovernanceSummary(ctx, deps.autoGovernanceTraces, ticket.WorkspaceID, ticket.RunID, "ticket", string(ticket.ID))
		if err != nil {
			return WorkspaceSnapshot{}, err
		}
		decisions = append(decisions, mapDecision(ticket, resolutions[ticket.ID], summary))
	}

	approvals := make([]Approval, 0, len(approvalItems))
	for _, approval := range approvalItems {
		if approval.WorkspaceID != workspaceID {
			continue
		}
		summary, err := latestAutoGovernanceSummary(ctx, deps.autoGovernanceTraces, approval.WorkspaceID, approval.RunID, "approval", string(approval.ID))
		if err != nil {
			return WorkspaceSnapshot{}, err
		}
		approvals = append(approvals, mapApproval(approval, summary))
	}

	ret := WorkspaceSnapshot{
		Workspace:    mapWorkspaceWithRunProjection(item, projectionRun),
		Goals:        mapGoals(goalItems),
		DoDs:         mapDoDs(dodItems),
		Runs:         runs,
		Results:      mapResults(resultItems),
		Artifacts:    mapArtifacts(artifactItems),
		Patches:      mapPatches(patchItems),
		EventRecords: mapProjectedEventRecords(eventRecordItems),
		Decisions:    decisions,
		Approvals:    approvals,
	}
	ret.LLMUsageTraces = mapLLMUsageTracesFromEventRecords(mapEventRecords(llmUsageEventRecordItems), "")
	ret.LLMUsageSummary = summarizeLLMUsage(ret.LLMUsageTraces)
	ret.TaskProgress = mapWorkspaceTaskProgress(ret)
	if projectionRun != nil {
		var runSnapshot RunSnapshot
		if cachedRunSnapshot != nil &&
			cachedRunSnapshot.Run.ID == string(projectionRun.ID) &&
			cachedRunSnapshot.Run.WorkspaceID == string(workspaceID) {
			runSnapshot = *cachedRunSnapshot
		} else {
			var err error
			runSnapshot, err = readRunSnapshot(ctx, deps, projectionRun.ID)
			if err != nil {
				return WorkspaceSnapshot{}, err
			}
		}
		ret.TaskProgress = runSnapshot.TaskProgress
		ret.TaskDecompositions = runSnapshot.TaskDecompositions
		ret.ExecutionSections = runSnapshot.ExecutionSections
		ret.MemoryRecallTraces = runSnapshot.MemoryRecallTraces
		ret.Patches = mergeWorkspaceSnapshotPatches(ret.Patches, runSnapshot.Patches)
		ret.WebSources = runSnapshot.WebSources
		ret.LLMContextBudgets = runSnapshot.LLMContextBudgets
		ret.LLMUsageTraces = mergeWorkspaceLLMUsageTraces(ret.LLMUsageTraces, runSnapshot.LLMUsageTraces)
		ret.LLMUsageSummary = summarizeLLMUsage(ret.LLMUsageTraces)
	}
	ret.Outcome = mapWorkspaceOutcome(ret)
	return ret, nil
}

func workspaceModelFromView(item Workspace) *workspacemodel.Workspace {
	return &workspacemodel.Workspace{
		ID:                   common.WorkspaceID(item.ID),
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		InternalDirectory:    item.InternalDirectory,
		State:                common.WorkspaceState(item.State),
		ActiveRunID:          common.RunID(item.ActiveRunID),
		DefaultSoulID:        common.SoulID(item.DefaultSoulID),
		SoulBindings:         workspaceSoulBindingsFromView(item.SoulBindings),
	}
}

func workspaceSoulBindingsFromView(input map[string]string) map[string]common.SoulID {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]common.SoulID, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		ret[key] = common.SoulID(value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func readWorkspaceTaskProgress(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID) (TaskProgress, error) {
	snapshot, err := readWorkspaceSnapshot(ctx, deps, workspaceID, nil)
	if err != nil {
		return TaskProgress{}, err
	}
	if snapshot.TaskProgress.WorkspaceID != "" {
		return snapshot.TaskProgress, nil
	}
	return mapWorkspaceTaskProgress(snapshot), nil
}

func readRunTaskProgress(ctx context.Context, deps readModelDependencies, runID common.RunID) (TaskProgress, error) {
	snapshot, err := readRunSnapshot(ctx, deps, runID)
	if err != nil {
		return TaskProgress{}, err
	}
	return runSnapshotTaskProgress(snapshot), nil
}

func readWorkspaceContextBundle(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID) (ContextBundle, error) {
	snapshot, err := readWorkspaceSnapshot(ctx, deps, workspaceID, nil)
	if err != nil {
		return ContextBundle{}, err
	}
	if snapshot.TaskProgress.RunID != "" {
		runSnapshot, err := readRunSnapshot(ctx, deps, common.RunID(snapshot.TaskProgress.RunID))
		if err != nil {
			return ContextBundle{}, err
		}
		return mapRunContextBundle(runSnapshot), nil
	}
	return mapWorkspaceContextBundle(snapshot), nil
}

func readWorkspaceTaskDecompositions(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID) ([]TaskDecomposition, error) {
	snapshot, err := readWorkspaceSnapshot(ctx, deps, workspaceID, nil)
	if err != nil {
		return nil, err
	}
	if len(snapshot.TaskDecompositions) > 0 {
		return snapshot.TaskDecompositions, nil
	}
	if snapshot.TaskProgress.RunID == "" {
		return nil, nil
	}
	runSnapshot, err := readRunSnapshot(ctx, deps, common.RunID(snapshot.TaskProgress.RunID))
	if err != nil {
		return nil, err
	}
	return runSnapshot.TaskDecompositions, nil
}

func selectWorkspaceProjectionRun(workspace *workspacemodel.Workspace, runs []*runmodel.Run, results []*resultmodel.Result, eventRecords []*eventstreammodel.EventRecord) *runmodel.Run {
	if workspace == nil || len(runs) == 0 {
		return nil
	}
	var latest *runmodel.Run
	var active *runmodel.Run
	runsByID := make(map[common.RunID]*runmodel.Run, len(runs))
	for _, item := range runs {
		if item == nil || item.WorkspaceID != workspace.ID {
			continue
		}
		runsByID[item.ID] = item
		if workspace.ActiveRunID != "" && item.ID == workspace.ActiveRunID {
			active = item
		}
		if latest == nil || string(item.ID) > string(latest.ID) {
			latest = item
		}
	}
	if active != nil && !isTerminalProjectionRunState(active.State) {
		return active
	}
	if latestByEvent := selectLatestRunByEvent(workspace.ID, runsByID, eventRecords); latestByEvent != nil {
		return latestByEvent
	}
	if resultRun := selectAcceptedResultRun(workspace.ID, runsByID, results); resultRun != nil {
		return resultRun
	}
	if active != nil {
		return active
	}
	return latest
}

func selectLatestRunByEvent(workspaceID common.WorkspaceID, runs map[common.RunID]*runmodel.Run, eventRecords []*eventstreammodel.EventRecord) *runmodel.Run {
	var selected *eventstreammodel.EventRecord
	for _, item := range eventRecords {
		if item == nil || item.WorkspaceID != workspaceID || item.RunID == "" {
			continue
		}
		if _, ok := runs[item.RunID]; !ok {
			continue
		}
		switch item.Topic {
		case events.TopicRunCreated, events.TopicRunStateChanged:
		default:
			continue
		}
		if selected == nil || item.OccurredAt >= selected.OccurredAt {
			selected = item
		}
	}
	if selected == nil {
		return nil
	}
	return runs[selected.RunID]
}

func mapWorkspaceWithRunProjection(workspace *workspacemodel.Workspace, run *runmodel.Run) Workspace {
	ret := mapWorkspace(workspace)
	if run == nil {
		return ret
	}
	ret.State = string(workspaceProjectionState(run.State))
	if isTerminalProjectionRunState(run.State) {
		ret.ActiveRunID = ""
	} else {
		ret.ActiveRunID = string(run.ID)
	}
	return ret
}

func selectAcceptedResultRun(workspaceID common.WorkspaceID, runs map[common.RunID]*runmodel.Run, results []*resultmodel.Result) *runmodel.Run {
	var selected *resultmodel.Result
	for _, item := range results {
		if item == nil || item.WorkspaceID != workspaceID || item.State != common.ResultStateAccepted || item.RunID == "" {
			continue
		}
		run := runs[item.RunID]
		if run == nil || run.State != common.RunStateCompleted {
			continue
		}
		if selected == nil || resultProjectionTime(item) >= resultProjectionTime(selected) {
			selected = item
		}
	}
	if selected == nil {
		return nil
	}
	return runs[selected.RunID]
}

func resultProjectionTime(item *resultmodel.Result) string {
	if item == nil {
		return ""
	}
	if item.UpdatedAt != "" {
		return item.UpdatedAt
	}
	return item.CreatedAt
}

func workspaceProjectionState(runState common.RunState) common.WorkspaceState {
	switch runState {
	case common.RunStateCreated:
		return common.WorkspaceStateReady
	case common.RunStateRunning:
		return common.WorkspaceStateRunning
	case common.RunStateBlockedOnDecision:
		return common.WorkspaceStateWaitingDecision
	case common.RunStateBlockedOnApproval:
		return common.WorkspaceStateBlocked
	case common.RunStateValidating:
		return common.WorkspaceStateValidating
	case common.RunStateCompleted:
		return common.WorkspaceStateCompleted
	case common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return common.WorkspaceStateFailed
	default:
		return common.WorkspaceStateActive
	}
}

func isTerminalProjectionRunState(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func decisionsForRun(ctx context.Context, deps readModelDependencies, run *runmodel.Run) ([]Decision, *decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	items, err := deps.tickets.ListByRunID(ctx, run.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	resolutions, err := resolutionsForTickets(ctx, deps.resolutions, items)
	if err != nil {
		return nil, nil, nil, err
	}

	ret := make([]Decision, 0, len(items))
	var selectedTicket *decisionmodel.Ticket
	var selectedResolution *decisionmodel.Resolution
	for _, item := range items {
		if item.RunID != run.ID {
			continue
		}
		resolution := resolutions[item.ID]
		summary, err := latestAutoGovernanceSummary(ctx, deps.autoGovernanceTraces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
		if err != nil {
			return nil, nil, nil, err
		}
		ret = append(ret, mapDecision(item, resolution, summary))
		if run.BlockingTicketID != "" && item.ID == run.BlockingTicketID {
			selectedTicket = item
			selectedResolution = resolution
			continue
		}
		if selectedTicket == nil || run.BlockingTicketID == "" {
			selectedTicket = item
			selectedResolution = resolution
		}
	}
	return ret, selectedTicket, selectedResolution, nil
}

func resolutionsForTickets(ctx context.Context, resolutions repository.DecisionResolutionRepository, items []*decisionmodel.Ticket) (map[common.TicketID]*decisionmodel.Resolution, error) {
	ret := make(map[common.TicketID]*decisionmodel.Resolution)
	if resolutions == nil || len(items) == 0 {
		return ret, nil
	}
	var mu sync.Mutex
	err := runReadModelTasks(funcsForResolvedTickets(items, func(item *decisionmodel.Ticket) func() error {
		return func() error {
			resolution, err := resolutions.GetByTicketID(ctx, item.ID)
			if errors.Is(err, common.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			mu.Lock()
			ret[item.ID] = resolution
			mu.Unlock()
			return nil
		}
	})...)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func funcsForResolvedTickets(items []*decisionmodel.Ticket, build func(*decisionmodel.Ticket) func() error) []func() error {
	if len(items) == 0 || build == nil {
		return nil
	}
	ret := make([]func() error, 0, len(items))
	for _, item := range items {
		if item == nil || item.State != common.DecisionTicketStateResolved {
			continue
		}
		ret = append(ret, build(item))
	}
	return ret
}

func approvalsForRun(ctx context.Context, deps readModelDependencies, run *runmodel.Run) ([]Approval, *approvalmodel.Approval, error) {
	items, err := deps.approvals.ListByRunID(ctx, run.ID)
	if err != nil {
		return nil, nil, err
	}

	ret := make([]Approval, 0, len(items))
	var selectedApproval *approvalmodel.Approval
	for _, item := range items {
		if item.RunID != run.ID {
			continue
		}
		summary, err := latestAutoGovernanceSummary(ctx, deps.autoGovernanceTraces, item.WorkspaceID, item.RunID, "approval", string(item.ID))
		if err != nil {
			return nil, nil, err
		}
		ret = append(ret, mapApproval(item, summary))
		if run.BlockingApprovalID != "" && item.ID == run.BlockingApprovalID {
			selectedApproval = item
			continue
		}
		if selectedApproval == nil || run.BlockingApprovalID == "" {
			selectedApproval = item
		}
	}
	return ret, selectedApproval, nil
}

func lastStep(steps []*runmodel.Step) *runmodel.Step {
	if len(steps) == 0 {
		return nil
	}
	return steps[len(steps)-1]
}

func currentRunStatusStep(run *runmodel.Run, steps []*runmodel.Step, checkpoints []*runmodel.RuntimeCheckpoint, decisions []*decisionmodel.Ticket, approvals []*approvalmodel.Approval) *runmodel.Step {
	if len(steps) == 0 {
		return nil
	}
	byID := make(map[common.StepID]*runmodel.Step, len(steps))
	for _, step := range steps {
		if step == nil || step.ID == "" {
			continue
		}
		byID[step.ID] = step
	}
	if run != nil && run.BlockingTicketID != "" {
		for _, item := range decisions {
			if item != nil && item.ID == run.BlockingTicketID && item.StepID != "" {
				if step := byID[item.StepID]; step != nil {
					return step
				}
			}
		}
	}
	if run != nil && run.BlockingApprovalID != "" {
		for _, item := range approvals {
			if item != nil && item.ID == run.BlockingApprovalID && item.StepID != "" {
				if step := byID[item.StepID]; step != nil {
					return step
				}
			}
		}
	}
	if checkpoint := latestRuntimeCheckpointModel(checkpoints); checkpoint != nil && checkpoint.CurrentStepID != "" {
		if step := byID[checkpoint.CurrentStepID]; step != nil {
			return step
		}
	}
	if step := terminalRunStatusStep(steps); step != nil {
		return step
	}
	return lastStep(steps)
}

func currentRunSnapshotStep(steps []presenter.StepView, decisionStep string, approvalStep string, checkpoints []RuntimeCheckpoint) *presenter.StepView {
	if decisionStep != "" {
		if step := stepByID(steps, decisionStep); step != nil {
			return step
		}
	}
	if approvalStep != "" {
		if step := stepByID(steps, approvalStep); step != nil {
			return step
		}
	}
	if checkpoint := latestRuntimeCheckpoint(checkpoints); checkpoint != nil {
		if step := stepByID(steps, checkpoint.CurrentStepID); step != nil {
			return step
		}
	}
	if len(steps) == 0 {
		return nil
	}
	if step := terminalSnapshotStep(steps); step != nil {
		return step
	}
	return &steps[len(steps)-1]
}

func decisionStepID(item *decisionmodel.Ticket) string {
	if item == nil {
		return ""
	}
	return string(item.StepID)
}

func approvalStepID(item *approvalmodel.Approval) string {
	if item == nil {
		return ""
	}
	return string(item.StepID)
}

func terminalRunStatusStep(steps []*runmodel.Step) *runmodel.Step {
	referenced := map[common.StepID]bool{}
	for _, step := range steps {
		if previous := readModelPreviousStepID(step); previous != "" {
			referenced[previous] = true
		}
	}
	var terminals []*runmodel.Step
	for _, step := range steps {
		if step == nil || referenced[step.ID] {
			continue
		}
		terminals = append(terminals, step)
	}
	if len(terminals) == 0 {
		return nil
	}
	for _, step := range terminals {
		if step.State == common.StepStateDispatched {
			return step
		}
	}
	return terminals[len(terminals)-1]
}

func terminalSnapshotStep(steps []presenter.StepView) *presenter.StepView {
	referenced := map[string]bool{}
	for _, step := range steps {
		if previous := readModelPreviousStepIDSnapshot(step); previous != "" {
			referenced[previous] = true
		}
	}
	var terminals []*presenter.StepView
	for idx := range steps {
		if steps[idx].ID == "" || referenced[steps[idx].ID] {
			continue
		}
		terminals = append(terminals, &steps[idx])
	}
	if len(terminals) == 0 {
		return nil
	}
	for _, step := range terminals {
		if step.State == string(common.StepStateDispatched) {
			return step
		}
	}
	return terminals[len(terminals)-1]
}

func readModelPreviousStepID(step *runmodel.Step) common.StepID {
	if step == nil || step.Input == nil {
		return ""
	}
	for _, key := range []string{"previousStepId", "repairOfStepId", "failedStepId"} {
		value, _ := step.Input[key].(string)
		value = strings.TrimSpace(value)
		if value != "" && common.StepID(value) != step.ID {
			return common.StepID(value)
		}
	}
	return ""
}

func readModelPreviousStepIDSnapshot(step presenter.StepView) string {
	if step.Input == nil {
		return ""
	}
	value, _ := step.Input["previousStepId"].(string)
	return strings.TrimSpace(value)
}

func mapToolCalls(items []*runmodel.ToolCall) []ToolCall {
	if len(items) == 0 {
		return nil
	}
	ret := make([]ToolCall, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, ToolCall{
			ID:          string(item.ID),
			WorkspaceID: string(item.WorkspaceID),
			RunID:       string(item.RunID),
			StepID:      string(item.StepID),
			ToolName:    item.ToolName,
			Arguments:   cloneReadModelMap(item.Arguments),
			State:       string(item.State),
			StartedAt:   item.StartedAt,
			EndedAt:     item.EndedAt,
			ExitSummary: item.ExitSummary,
			Output:      cloneReadModelMap(item.Output),
			Error:       item.Error,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapWebSourceTraces(steps []presenter.StepView) []WebSourceTrace {
	if len(steps) == 0 {
		return nil
	}
	ret := make([]WebSourceTrace, 0)
	for _, step := range steps {
		if step.Output == nil {
			continue
		}
		switch step.Kind {
		case webcommon.StepKindSearch:
			trace := webSearchTrace(step)
			if len(trace.Sources) > 0 || trace.Query != "" {
				ret = append(ret, trace)
			}
		case webcommon.StepKindFetch:
			trace := webFetchTrace(step)
			if len(trace.FetchedSources) > 0 || trace.ApprovalID != "" {
				ret = append(ret, trace)
			}
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapLLMContextBudgetTraces(steps []presenter.StepView) []LLMContextBudgetTrace {
	if len(steps) == 0 {
		return nil
	}
	ret := make([]LLMContextBudgetTrace, 0)
	for _, step := range steps {
		if step.Output == nil || step.Kind != llmcommon.StepKindWorkspaceExecution {
			continue
		}
		budget := readModelMapValue(step.Output["contextBudget"])
		if len(budget) == 0 {
			continue
		}
		ret = append(ret, LLMContextBudgetTrace{
			StepID:                step.ID,
			ProviderID:            readModelStringValue(budget["providerId"]),
			Model:                 readModelStringValue(budget["model"]),
			MaxBytes:              readModelIntValue(budget["maxBytes"]),
			ActualBytes:           readModelIntValue(budget["actualBytes"]),
			MaxTokens:             readModelIntValue(budget["maxTokens"]),
			EstimatedInputTokens:  readModelIntValue(budget["estimatedInputTokens"]),
			EstimatedOutputTokens: readModelIntValue(budget["estimatedOutputTokens"]),
			ActualTokens:          readModelIntValue(budget["actualTokens"]),
			ShrinkLevel:           readModelIntValue(budget["shrinkLevel"]),
			Truncated:             readModelBoolValue(budget["truncated"]),
			Rules:                 readModelStringSliceValue(budget["rules"]),
			SourceRefs:            readModelStringSliceValue(budget["sourceRefs"]),
			IncludedSections:      readModelStringSliceValue(budget["includedSections"]),
			NormalizedSections:    readModelStringSliceValue(budget["normalizedSections"]),
			CompressedSections:    readModelStringSliceValue(budget["compressedSections"]),
			OmittedSections:       readModelStringSliceValue(budget["omittedSections"]),
			GoalDoDCompressed:     readModelBoolValue(budget["goalDoDCompressed"]),
			P0Overflow:            readModelBoolValue(budget["p0Overflow"]),
			TracePolicy:           readModelStringValue(budget["tracePolicy"]),
			PromptCaptured:        readModelBoolValue(budget["promptCaptured"]),
			Estimator:             readModelStringValue(budget["estimator"]),
			Profile:               readModelMapValue(budget["profile"]),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapLLMUsageTracesFromEventRecords(records []EventRecord, runID string) []LLMUsageTrace {
	if len(records) == 0 {
		return nil
	}
	ret := make([]LLMUsageTrace, 0)
	for _, item := range records {
		if item.Topic != events.TopicRunLLMUsageRecorded {
			continue
		}
		if runID != "" && item.RunID != runID {
			continue
		}
		payload := cloneReadModelMap(item.Payload)
		if len(payload) == 0 {
			continue
		}
		estimated := readModelBoolValue(payload[events.PayloadKeyEstimated])
		usageSource := readModelStringValue(payload[events.PayloadKeyUsageSource])
		if strings.TrimSpace(usageSource) == "" {
			if estimated {
				usageSource = llmcommon.UsageSourceEstimated
			} else {
				usageSource = llmcommon.UsageSourceProviderNative
			}
		}
		estimatedCost := readModelFloatValue(payload[events.PayloadKeyEstimatedCost])
		costEstimated := readModelBoolValue(payload[events.PayloadKeyCostEstimated])
		if !costEstimated && estimatedCost > 0 {
			costEstimated = true
		}
		costSource := readModelStringValue(payload[events.PayloadKeyCostSource])
		if costEstimated && strings.TrimSpace(costSource) == "" {
			costSource = llmcommon.CostSourceProfilePricing
		}
		trace := LLMUsageTrace{
			ID:            item.ID,
			WorkspaceID:   firstNonEmptyReadModel(item.WorkspaceID, readModelStringValue(payload[events.PayloadKeyWorkspaceID])),
			RunID:         firstNonEmptyReadModel(item.RunID, readModelStringValue(payload[events.PayloadKeyRunID])),
			StepID:        firstNonEmptyReadModel(item.StepID, readModelStringValue(payload[events.PayloadKeyStepID])),
			ProviderID:    readModelStringValue(payload[events.PayloadKeyProviderID]),
			Provider:      readModelStringValue(payload[events.PayloadKeyProvider]),
			Model:         readModelStringValue(payload[events.PayloadKeyModel]),
			Operation:     readModelStringValue(payload[events.PayloadKeyOperation]),
			InputTokens:   readModelIntValue(payload[events.PayloadKeyInputTokens]),
			OutputTokens:  readModelIntValue(payload[events.PayloadKeyOutputTokens]),
			TotalTokens:   readModelIntValue(payload[events.PayloadKeyTotalTokens]),
			Estimated:     estimated,
			UsageSource:   usageSource,
			EstimatedCost: estimatedCost,
			CostEstimated: costEstimated,
			CostSource:    costSource,
			Currency:      readModelStringValue(payload[events.PayloadKeyCurrency]),
			RequestBytes:  readModelIntValue(payload[events.PayloadKeyRequestBytes]),
			ResponseBytes: readModelIntValue(payload[events.PayloadKeyResponseBytes]),
			Streaming:     readModelBoolValue(payload[events.PayloadKeyStreaming]),
			ContextBudget: readModelMapValue(payload[events.PayloadKeyContextBudget]),
			StartedAt:     readModelStringValue(payload[events.PayloadKeyStartedAt]),
			CompletedAt:   readModelStringValue(payload[events.PayloadKeyCompletedAt]),
			DurationMs:    readModelIntValue(payload[events.PayloadKeyDurationMs]),
			Status:        readModelStringValue(payload[events.PayloadKeyStatus]),
			FailureClass:  readModelStringValue(payload[events.PayloadKeyFailureClass]),
			OccurredAt:    item.OccurredAt,
			EvidenceRef:   firstNonEmptyReadModel(item.StepID, item.SubjectID),
		}
		ret = append(ret, trace)
	}
	if len(ret) == 0 {
		return nil
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].OccurredAt == ret[j].OccurredAt {
			return ret[i].ID < ret[j].ID
		}
		return ret[i].OccurredAt < ret[j].OccurredAt
	})
	return ret
}

func summarizeLLMUsage(items []LLMUsageTrace) LLMUsageSummary {
	ret := LLMUsageSummary{}
	breakdownByKey := map[string]*LLMUsageProviderModelBreakdown{}
	costByCurrency := map[string]*LLMUsageCostBreakdown{}
	for _, item := range items {
		ret.EntryTotal++
		ret.InputTokens += item.InputTokens
		ret.OutputTokens += item.OutputTokens
		ret.TotalTokens += item.TotalTokens
		if item.Estimated {
			ret.EstimatedEntries++
		}
		if strings.TrimSpace(item.UsageSource) == llmcommon.UsageSourceProviderNative {
			ret.ProviderNativeEntries++
		}
		if item.CostEstimated {
			ret.CostEstimatedEntries++
		}
		if item.Status == "failed" {
			ret.FailedEntries++
		}
		currency := normalizedLLMUsageCurrency(item.Currency, item.EstimatedCost)
		if item.EstimatedCost > 0 && currency != "" {
			costEntry, ok := costByCurrency[currency]
			if !ok {
				costEntry = &LLMUsageCostBreakdown{Currency: currency}
				costByCurrency[currency] = costEntry
			}
			costEntry.EstimatedCost += item.EstimatedCost
			costEntry.EntryTotal++
		}

		key := strings.TrimSpace(item.ProviderID) + "|" + strings.TrimSpace(item.Provider) + "|" + strings.TrimSpace(item.Model) + "|" + currency
		entry, ok := breakdownByKey[key]
		if !ok {
			entry = &LLMUsageProviderModelBreakdown{
				ProviderID: strings.TrimSpace(item.ProviderID),
				Provider:   strings.TrimSpace(item.Provider),
				Model:      strings.TrimSpace(item.Model),
				Currency:   currency,
			}
			breakdownByKey[key] = entry
		}
		entry.EntryTotal++
		entry.InputTokens += item.InputTokens
		entry.OutputTokens += item.OutputTokens
		entry.TotalTokens += item.TotalTokens
		entry.EstimatedCost += item.EstimatedCost
		if item.Estimated {
			entry.EstimatedEntries++
		}
		if item.Status == "failed" {
			entry.FailedEntries++
		}
	}
	if len(breakdownByKey) > 0 {
		ret.ProviderModelBreakdown = make([]LLMUsageProviderModelBreakdown, 0, len(breakdownByKey))
		for _, item := range breakdownByKey {
			ret.ProviderModelBreakdown = append(ret.ProviderModelBreakdown, *item)
		}
		sort.SliceStable(ret.ProviderModelBreakdown, func(i, j int) bool {
			left := ret.ProviderModelBreakdown[i]
			right := ret.ProviderModelBreakdown[j]
			if left.ProviderID != right.ProviderID {
				return left.ProviderID < right.ProviderID
			}
			if left.Provider != right.Provider {
				return left.Provider < right.Provider
			}
			if left.Model != right.Model {
				return left.Model < right.Model
			}
			return left.Currency < right.Currency
		})
	}
	if len(costByCurrency) > 0 {
		ret.CostBreakdown = make([]LLMUsageCostBreakdown, 0, len(costByCurrency))
		for _, item := range costByCurrency {
			ret.CostBreakdown = append(ret.CostBreakdown, *item)
		}
		sort.SliceStable(ret.CostBreakdown, func(i, j int) bool {
			return ret.CostBreakdown[i].Currency < ret.CostBreakdown[j].Currency
		})
		if len(ret.CostBreakdown) == 1 {
			ret.Currency = ret.CostBreakdown[0].Currency
			ret.EstimatedCost = ret.CostBreakdown[0].EstimatedCost
		}
	}
	return ret
}

func normalizedLLMUsageCurrency(currency string, estimatedCost float64) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" && estimatedCost > 0 {
		return "USD"
	}
	return currency
}

func webSearchTrace(step presenter.StepView) WebSourceTrace {
	output := step.Output
	evidenceRef := step.ID
	results := readModelMapSliceValue(output["results"])
	sources := make([]WebSource, 0, len(results))
	for _, result := range results {
		item := WebSource{
			Title:       readModelStringValue(result["title"]),
			URL:         readModelStringValue(result["url"]),
			Snippet:     readModelStringValue(result["snippet"]),
			Source:      readModelStringValue(result["source"]),
			PublishedAt: readModelStringValue(result["publishedAt"]),
			Rank:        readModelIntValue(result["rank"]),
			EvidenceRef: evidenceRef,
		}
		if item.URL != "" {
			sources = append(sources, item)
		}
	}
	return WebSourceTrace{
		Query:          readModelStringValue(output["query"]),
		ProviderID:     readModelStringValue(output["provider"]),
		SearchedAt:     readModelStringValue(output["searchedAt"]),
		Sources:        sources,
		EvidenceRefs:   []string{evidenceRef},
		PolicyDecision: readModelPolicyDecision(output["policy"]),
	}
}

func webFetchTrace(step presenter.StepView) WebSourceTrace {
	output := step.Output
	evidenceRef := step.ID
	fetched := FetchedWebSource{
		URL:         readModelStringValue(output["url"]),
		FinalURL:    readModelStringValue(output["finalUrl"]),
		Title:       readModelStringValue(output["title"]),
		Source:      readModelStringValue(output["source"]),
		RetrievedAt: readModelStringValue(output["retrievedAt"]),
		ContentHash: readModelStringValue(output["contentHash"]),
		Truncated:   readModelBoolValue(output["truncated"]),
		EvidenceRef: evidenceRef,
	}
	fetchedSources := []FetchedWebSource(nil)
	if fetched.URL != "" {
		fetchedSources = []FetchedWebSource{fetched}
	}
	return WebSourceTrace{
		ProviderID:     readModelStringValue(output["provider"]),
		FetchedSources: fetchedSources,
		EvidenceRefs:   []string{evidenceRef},
		ApprovalID:     readModelStringValue(output["approvalId"]),
		PolicyDecision: readModelPolicyDecision(output["policy"]),
	}
}

func selectedRuntimeCapabilities(executionContext map[string]any, catalog map[string]RuntimeCapability) []RuntimeCapability {
	if len(executionContext) == 0 {
		return nil
	}
	ret := make([]RuntimeCapability, 0)
	seen := map[string]struct{}{}
	for _, raw := range readModelMapSliceValue(executionContext["mcpCapabilities"]) {
		item := runtimeCapabilityFromContext(raw)
		if item.ID == "" {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		ret = append(ret, item)
	}
	for _, id := range readModelStringSliceValue(executionContext["capabilityIds"]) {
		if _, ok := seen[id]; ok {
			continue
		}
		if item, ok := catalog[id]; ok {
			ret = append(ret, item)
		} else {
			ret = append(ret, RuntimeCapability{ID: id, DisplayName: id})
		}
		seen[id] = struct{}{}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func usedRuntimeCapabilities(snapshot RunSnapshot, catalog map[string]RuntimeCapability) []RuntimeCapability {
	if len(snapshot.ToolCalls) == 0 {
		return nil
	}
	selected := map[string]RuntimeCapability{}
	for _, item := range snapshot.SelectedCapabilities {
		selected[item.ID] = item
	}
	used := map[string]RuntimeCapability{}
	order := make([]string, 0)
	for _, toolCall := range snapshot.ToolCalls {
		identity := runtimeCapabilityIdentity(toolCall)
		if identity.ID == "" {
			continue
		}
		item, ok := used[identity.ID]
		if !ok {
			if loaded, exists := catalog[identity.ID]; exists {
				item = loaded
			} else if selectedItem, exists := selected[identity.ID]; exists {
				item = selectedItem
			} else {
				item = identity
			}
			order = append(order, identity.ID)
		}
		item.UseCount++
		item.EvidenceRefs = compactUniqueStrings(append(item.EvidenceRefs, toolCall.ID, toolCall.StepID))
		used[identity.ID] = item
	}
	ret := make([]RuntimeCapability, 0, len(order))
	for _, id := range order {
		ret = append(ret, used[id])
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runtimeCapabilityCatalog(ctx context.Context, deps readModelDependencies, executionContext map[string]any, toolCalls []ToolCall) map[string]RuntimeCapability {
	if deps.mcpCapabilities == nil || !requiresRuntimeCapabilityCatalog(executionContext, toolCalls) {
		return nil
	}
	items, err := deps.mcpCapabilities.List(ctx)
	if err != nil {
		return nil
	}
	return runtimeCapabilityCatalogFromItems(executionContext, toolCalls, items)
}

func runtimeCapabilityCatalogFromItems(executionContext map[string]any, toolCalls []ToolCall, items []*mcpmodel.Capability) map[string]RuntimeCapability {
	if items == nil || !requiresRuntimeCapabilityCatalog(executionContext, toolCalls) {
		return nil
	}
	ret := make(map[string]RuntimeCapability, len(items))
	for _, item := range items {
		capability := runtimeCapabilityFromModel(item)
		if capability.ID == "" {
			continue
		}
		ret[capability.ID] = capability
	}
	return ret
}

func requiresRuntimeCapabilityCatalog(executionContext map[string]any, toolCalls []ToolCall) bool {
	if len(readModelStringSliceValue(executionContext["capabilityIds"])) > 0 {
		return true
	}
	for _, toolCall := range toolCalls {
		if runtimeCapabilityIdentity(toolCall).ID != "" {
			return true
		}
	}
	return false
}

func runtimeCapabilityIdentity(toolCall ToolCall) RuntimeCapability {
	source := toolCall.Output
	if len(source) == 0 {
		source = toolCall.Arguments
	}
	serverID := readModelStringValue(source["serverId"])
	operation := readModelStringValue(source["operation"])
	var kind string
	var name string
	switch operation {
	case "mcp/tool_call":
		kind = mcpmodel.CapabilityKindTool
		name = readModelStringValue(source["tool"])
	case "mcp/resource_read":
		kind = mcpmodel.CapabilityKindResource
		name = readModelStringValue(source["uri"])
	case "mcp/prompt_get":
		kind = mcpmodel.CapabilityKindPrompt
		name = readModelStringValue(source["prompt"])
	}
	if serverID == "" || kind == "" || name == "" {
		return RuntimeCapability{}
	}
	id := defaultRuntimeCapabilityID(serverID, kind, name)
	return RuntimeCapability{
		ID:          id,
		ServerID:    serverID,
		Kind:        kind,
		Name:        name,
		DisplayName: name,
	}
}

func runtimeCapabilityFromModel(item *mcpmodel.Capability) RuntimeCapability {
	if item == nil {
		return RuntimeCapability{}
	}
	return RuntimeCapability{
		ID:             string(item.ID),
		ServerID:       string(item.ServerID),
		Kind:           item.Kind,
		Name:           item.Name,
		DisplayName:    firstNonEmptyReadModel(item.DisplayName, item.Name, string(item.ID)),
		Description:    item.Description,
		Category:       item.Category,
		Tags:           cloneReadModelStringSlice(item.Tags),
		RiskLevel:      item.RiskLevel,
		Enabled:        item.Enabled,
		ApprovalPolicy: item.ApprovalPolicy,
		Status:         item.Status,
	}
}

func runtimeCapabilityFromContext(raw map[string]any) RuntimeCapability {
	id := readModelStringValue(raw["id"])
	return RuntimeCapability{
		ID:             id,
		ServerID:       readModelStringValue(raw["serverId"]),
		Kind:           readModelStringValue(raw["kind"]),
		Name:           readModelStringValue(raw["name"]),
		DisplayName:    firstNonEmptyReadModel(readModelStringValue(raw["displayName"]), readModelStringValue(raw["name"]), id),
		Description:    readModelStringValue(raw["description"]),
		Category:       readModelStringValue(raw["category"]),
		Tags:           readModelStringSliceValue(raw["tags"]),
		RiskLevel:      readModelStringValue(raw["riskLevel"]),
		Enabled:        readModelBoolValue(raw["enabled"]),
		ApprovalPolicy: readModelStringValue(raw["approvalPolicy"]),
		Status:         readModelStringValue(raw["status"]),
	}
}

func defaultRuntimeCapabilityID(serverID string, kind string, name string) string {
	return strings.TrimSpace(serverID) + ":" + strings.TrimSpace(kind) + ":" + url.PathEscape(strings.TrimSpace(name))
}

func mapPatches(items []*runmodel.Patch) []Patch {
	if len(items) == 0 {
		return nil
	}
	ret := make([]Patch, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, projectPatch(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func readModelPatchLineStats(item *runmodel.Patch) (int, int) {
	if item == nil {
		return 0, 0
	}
	text := item.Diff
	if strings.TrimSpace(text) == "" {
		text = item.Content
	}
	added := 0
	deleted := 0
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			deleted++
		}
	}
	return added, deleted
}

func mergeWorkspaceSnapshotPatches(existing []Patch, latest []Patch) []Patch {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]Patch, 0, len(existing)+len(latest))
	for _, item := range existing {
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	for _, item := range latest {
		if idx, ok := seen[item.ID]; ok {
			out[idx] = item
			continue
		}
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}

func mergeWorkspaceLLMUsageTraces(existing []LLMUsageTrace, latest []LLMUsageTrace) []LLMUsageTrace {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]LLMUsageTrace, 0, len(existing)+len(latest))
	for _, item := range existing {
		key := firstNonEmptyReadModel(item.ID, item.StepID+"|"+item.OccurredAt)
		seen[key] = len(out)
		out = append(out, item)
	}
	for _, item := range latest {
		key := firstNonEmptyReadModel(item.ID, item.StepID+"|"+item.OccurredAt)
		if idx, ok := seen[key]; ok {
			out[idx] = item
			continue
		}
		seen[key] = len(out)
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].OccurredAt == out[j].OccurredAt {
			return out[i].ID < out[j].ID
		}
		return out[i].OccurredAt < out[j].OccurredAt
	})
	return out
}

func mapValidationResults(items []*runmodel.ValidationResult) []ValidationResult {
	if len(items) == 0 {
		return nil
	}
	ret := make([]ValidationResult, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, projectValidationResult(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapRuntimeCheckpoints(items []*runmodel.RuntimeCheckpoint) []RuntimeCheckpoint {
	if len(items) == 0 {
		return nil
	}
	ret := make([]RuntimeCheckpoint, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, RuntimeCheckpoint{
			ID:                 string(item.ID),
			WorkspaceID:        string(item.WorkspaceID),
			RunID:              string(item.RunID),
			CheckpointType:     string(item.CheckpointType),
			CurrentStepID:      string(item.CurrentStepID),
			RunStateSnapshot:   string(item.RunStateSnapshot),
			BlockingTicketID:   string(item.BlockingTicketID),
			BlockingApprovalID: string(item.BlockingApprovalID),
			ThreadRef:          item.ThreadRef,
			OccurredAt:         item.OccurredAt,
			RestorableUntil:    item.RestorableUntil,
			Data:               cloneReadModelMap(item.Data),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapResults(items []*resultmodel.Result) []Result {
	if len(items) == 0 {
		return nil
	}
	ret := make([]Result, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		artifactRefs := make([]string, 0, len(item.ArtifactRefs))
		for _, ref := range item.ArtifactRefs {
			if ref != "" {
				artifactRefs = append(artifactRefs, string(ref))
			}
		}
		ret = append(ret, Result{
			ID:            string(item.ID),
			WorkspaceID:   string(item.WorkspaceID),
			RunID:         string(item.RunID),
			GoalID:        string(item.GoalID),
			DoDID:         string(item.DoDID),
			State:         string(item.State),
			Title:         item.Title,
			Summary:       item.Summary,
			SourceType:    item.SourceType,
			SourceRef:     item.SourceRef,
			EvidenceRefs:  cloneReadModelStringSlice(item.EvidenceRefs),
			ArtifactRefs:  artifactRefs,
			DoDAssessment: mapDoDAssessment(item.DoDAssessment),
			SummaryDetail: mapResultSummary(item.SummaryDetail),
			CreatedAt:     item.CreatedAt,
			UpdatedAt:     item.UpdatedAt,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapResultSummary(item resultmodel.ResultSummary) *ResultSummary {
	if item.GoalOutcome == "" &&
		len(item.DoDItems) == 0 &&
		item.ImplementationSummary == "" &&
		item.VerificationSummary == "" &&
		len(item.EvidenceRefs) == 0 &&
		len(item.ArtifactRefs) == 0 &&
		len(item.KnownLimitations) == 0 &&
		len(item.ResidualRisks) == 0 &&
		len(item.NextActions) == 0 {
		return nil
	}
	artifactRefs := make([]string, 0, len(item.ArtifactRefs))
	for _, ref := range item.ArtifactRefs {
		if ref != "" {
			artifactRefs = append(artifactRefs, string(ref))
		}
	}
	ret := &ResultSummary{
		GoalOutcome:           item.GoalOutcome,
		ImplementationSummary: item.ImplementationSummary,
		VerificationSummary:   item.VerificationSummary,
		EvidenceRefs:          cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs:          artifactRefs,
		KnownLimitations:      cloneReadModelStringSlice(item.KnownLimitations),
		ResidualRisks:         cloneReadModelStringSlice(item.ResidualRisks),
		NextActions:           cloneReadModelStringSlice(item.NextActions),
	}
	if len(item.DoDItems) > 0 {
		ret.DoDItems = make([]DoDAssessmentItem, 0, len(item.DoDItems))
		for _, assessmentItem := range item.DoDItems {
			ret.DoDItems = append(ret.DoDItems, DoDAssessmentItem{
				Item:          assessmentItem.Item,
				State:         assessmentItem.State,
				EvidenceRefs:  cloneReadModelStringSlice(assessmentItem.EvidenceRefs),
				FailureReason: assessmentItem.FailureReason,
				Confidence:    assessmentItem.Confidence,
				SourceNote:    assessmentItem.SourceNote,
			})
		}
	}
	return ret
}

func mapDoDAssessment(item resultmodel.DoDAssessment) *DoDAssessment {
	if item.Summary == "" && len(item.Items) == 0 && !item.Satisfied {
		return nil
	}
	ret := &DoDAssessment{
		Satisfied: item.Satisfied,
		Summary:   item.Summary,
	}
	if len(item.Items) > 0 {
		ret.Items = make([]DoDAssessmentItem, 0, len(item.Items))
		for _, assessmentItem := range item.Items {
			ret.Items = append(ret.Items, DoDAssessmentItem{
				Item:          assessmentItem.Item,
				State:         assessmentItem.State,
				EvidenceRefs:  cloneReadModelStringSlice(assessmentItem.EvidenceRefs),
				FailureReason: assessmentItem.FailureReason,
				Confidence:    assessmentItem.Confidence,
				SourceNote:    assessmentItem.SourceNote,
			})
		}
	}
	return ret
}

func mapDoDEvidence(snapshot RunSnapshot) []DoDEvidenceTrace {
	evidenceIndex := runEvidenceRefIndex(snapshot)
	results := runScopedResults(snapshot)
	if len(results) == 0 {
		return nil
	}
	ret := make([]DoDEvidenceTrace, 0, len(results))
	for _, item := range results {
		if item.DoDAssessment == nil {
			continue
		}
		if snapshot.Run.DoDID != "" && item.DoDID != snapshot.Run.DoDID {
			continue
		}
		trace := DoDEvidenceTrace{
			ResultID:  item.ID,
			DoDID:     item.DoDID,
			Satisfied: item.DoDAssessment.Satisfied,
			Summary:   item.DoDAssessment.Summary,
		}
		assessmentItems := item.DoDAssessment.Items
		if len(assessmentItems) == 0 && item.SummaryDetail != nil && len(item.SummaryDetail.DoDItems) > 0 {
			assessmentItems = item.SummaryDetail.DoDItems
		}
		if len(assessmentItems) > 0 {
			trace.Items = make([]DoDEvidenceItem, 0, len(assessmentItems))
			for _, assessmentItem := range assessmentItems {
				refs := compactUniqueStrings(cloneReadModelStringSlice(assessmentItem.EvidenceRefs))
				missing := make([]string, 0)
				for _, ref := range refs {
					if _, ok := evidenceIndex[ref]; !ok {
						missing = append(missing, ref)
					}
				}
				status := "missing"
				if len(refs) > 0 && len(missing) == 0 {
					status = "linked"
				} else if len(refs) > 0 && len(missing) < len(refs) {
					status = "partial"
				}
				trace.Items = append(trace.Items, DoDEvidenceItem{
					Item:                assessmentItem.Item,
					State:               assessmentItem.State,
					FailureReason:       assessmentItem.FailureReason,
					Confidence:          assessmentItem.Confidence,
					SourceNote:          assessmentItem.SourceNote,
					EvidenceRefs:        refs,
					EvidenceStatus:      status,
					MissingEvidenceRefs: missing,
				})
			}
		}
		ret = append(ret, trace)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapResultQuality(snapshot RunSnapshot) []ResultQualityTrace {
	results := runScopedResults(snapshot)
	if len(results) == 0 {
		return nil
	}
	evidenceIndex := runEvidenceRefIndex(snapshot)
	weakValidationRefs := weakValidationEvidenceRefIndex(snapshot)
	requiredDoDLeafItems := runRequiredDoDLeafItems(snapshot)
	ret := make([]ResultQualityTrace, 0, len(results))
	for _, item := range results {
		quality := ResultQualityTrace{
			ResultID:     item.ID,
			DoDID:        item.DoDID,
			State:        "passed",
			Summary:      strings.TrimSpace(item.Summary),
			EvidenceRefs: compactUniqueStrings(cloneReadModelStringSlice(item.EvidenceRefs)),
		}
		addIssue := func(code string, severity string, message string, suggestion string, refs ...string) {
			issue := ResultQualityIssue{
				Code:         code,
				Severity:     severity,
				Message:      message,
				Suggestion:   suggestion,
				EvidenceRefs: compactUniqueStrings(refs),
			}
			quality.Issues = append(quality.Issues, issue)
			if severity == "error" {
				quality.State = "failed"
			} else if quality.State != "failed" {
				quality.State = "warning"
			}
		}
		var evidenceRefs []string
		var goalOutcome string
		var verificationSummary string
		var knownLimitations []string
		var residualRisks []string
		var nextActions []string
		if item.SummaryDetail != nil {
			goalOutcome = strings.TrimSpace(item.SummaryDetail.GoalOutcome)
			verificationSummary = strings.TrimSpace(item.SummaryDetail.VerificationSummary)
			knownLimitations = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.KnownLimitations))
			residualRisks = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.ResidualRisks))
			nextActions = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.NextActions))
			evidenceRefs = compactUniqueStrings(cloneReadModelStringSlice(item.SummaryDetail.EvidenceRefs))
		}
		if len(evidenceRefs) == 0 {
			evidenceRefs = compactUniqueStrings(cloneReadModelStringSlice(item.EvidenceRefs))
		}
		quality.EvidenceRefs = evidenceRefs
		if goalOutcome == "" {
			addIssue("missing_goal_outcome", "error", "result is missing a concrete goal outcome summary", "populate summaryDetail.goalOutcome with the delivered outcome", evidenceRefs...)
		}
		if verificationSummary == "" {
			addIssue("missing_verification_summary", "error", "result is missing verification summary", "populate summaryDetail.verificationSummary with verification scope and conclusions", evidenceRefs...)
		}
		if len(evidenceRefs) == 0 {
			addIssue("missing_evidence_refs", "error", "result is missing evidence references", "attach validation, patch, tool call or artifact evidence refs")
		}
		if len(knownLimitations) == 0 && len(residualRisks) == 0 {
			addIssue("missing_risk_disclosure", "error", "result is missing known limitations and residual risks", "populate summaryDetail.knownLimitations or summaryDetail.residualRisks", evidenceRefs...)
		}
		if len(nextActions) == 0 {
			addIssue("missing_next_actions", "warning", "result is missing next actions", "populate summaryDetail.nextActions for follow-up guidance", evidenceRefs...)
		}
		if item.DoDAssessment == nil || len(item.DoDAssessment.Items) == 0 {
			addIssue("missing_dod_item_assessment", "warning", "result is missing DoD item-level assessment", "attach DoD item states and evidence refs", evidenceRefs...)
		} else {
			if missingLeaf := missingRequiredDoDLeafItems(requiredDoDLeafItems, item.DoDAssessment.Items); len(missingLeaf) > 0 {
				addIssue(
					"missing_dod_leaf_coverage",
					"error",
					fmt.Sprintf("result dodAssessment does not cover required DoD leaf items: %s", strings.Join(missingLeaf, "; ")),
					"include each missing DoD leaf item in dodAssessment.items with concrete evidenceRefs",
					evidenceRefs...,
				)
			}
			for _, assessmentItem := range item.DoDAssessment.Items {
				if strings.TrimSpace(assessmentItem.State) != "satisfied" {
					continue
				}
				itemName := strings.TrimSpace(assessmentItem.Item)
				refs := compactUniqueStrings(cloneReadModelStringSlice(assessmentItem.EvidenceRefs))
				if len(refs) == 0 {
					addIssue(
						"dod_item_missing_evidence_refs",
						"error",
						fmt.Sprintf("satisfied DoD item %q has no evidence refs", itemName),
						"attach concrete validation/patch/tool evidence refs for every satisfied DoD item",
					)
					continue
				}
				missingRefs := make([]string, 0, len(refs))
				for _, ref := range refs {
					if _, ok := evidenceIndex[ref]; !ok {
						missingRefs = append(missingRefs, ref)
					}
				}
				if len(missingRefs) > 0 {
					addIssue(
						"dod_item_unlinked_evidence_refs",
						"error",
						fmt.Sprintf("satisfied DoD item %q references unlinked evidence: %s", itemName, strings.Join(missingRefs, ", ")),
						"use evidence refs that exist in runtime steps, validations, patches, tool calls, checkpoints, or artifacts",
						refs...,
					)
				}
				if allEvidenceRefsWeakValidation(refs, weakValidationRefs) {
					addIssue(
						"dod_item_weak_verification_evidence",
						"error",
						fmt.Sprintf("satisfied DoD item %q relies only on weak verification evidence", itemName),
						"add effective assertions/tests or mark the item failed/skipped with impact and residual risk",
						refs...,
					)
				}
			}
		}
		ret = append(ret, quality)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runRequiredDoDLeafItems(snapshot RunSnapshot) []string {
	dod := readModelMapValue(snapshot.Run.ExecutionContext["dod"])
	items := readModelStringSliceValue(dod["items"])
	if len(items) == 0 {
		for _, item := range readModelMapSliceValue(snapshot.Run.ExecutionContext["dods"]) {
			items = append(items, readModelStringSliceValue(item["items"])...)
		}
	}
	if len(items) == 0 {
		return nil
	}
	ret := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, raw := range items {
		raw = strings.TrimSpace(raw)
		if raw == "" || !readModelDoDItemIsLeaf(raw) {
			continue
		}
		normalized := normalizeReadModelDoDText(raw)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		ret = append(ret, raw)
	}
	return ret
}

func missingRequiredDoDLeafItems(required []string, assessed []DoDAssessmentItem) []string {
	if len(required) == 0 {
		return nil
	}
	covered := make([]string, 0, len(assessed))
	for _, item := range assessed {
		if strings.TrimSpace(item.State) != "satisfied" {
			continue
		}
		normalized := normalizeReadModelDoDText(item.Item)
		if normalized != "" {
			covered = append(covered, normalized)
		}
	}
	missing := make([]string, 0)
	for _, raw := range required {
		requiredNormalized := normalizeReadModelDoDText(raw)
		if requiredNormalized == "" {
			continue
		}
		matched := false
		for _, actual := range covered {
			if actual == "" {
				continue
			}
			if strings.Contains(actual, requiredNormalized) || strings.Contains(requiredNormalized, actual) {
				matched = true
				break
			}
		}
		if !matched {
			missing = append(missing, raw)
		}
	}
	return compactUniqueStrings(missing)
}

func weakValidationEvidenceRefIndex(snapshot RunSnapshot) map[string]bool {
	ret := map[string]bool{}
	for _, item := range snapshot.ValidationResults {
		quality := strings.TrimSpace(item.EvidenceQuality)
		if !item.ZeroWork && (quality == "" || quality == string(common.EvidenceQualityValid)) {
			continue
		}
		if ref := strings.TrimSpace(item.ID); ref != "" {
			ret[ref] = true
		}
		if ref := strings.TrimSpace(item.EvidenceRef); ref != "" {
			ret[ref] = true
		}
		if ref := strings.TrimSpace(item.StepID); ref != "" {
			ret[ref] = true
		}
	}
	return ret
}

func allEvidenceRefsWeakValidation(refs []string, weakRefIndex map[string]bool) bool {
	if len(refs) == 0 || len(weakRefIndex) == 0 {
		return false
	}
	seen := false
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		seen = true
		if !weakRefIndex[ref] {
			return false
		}
	}
	return seen
}

func readModelDoDItemIsLeaf(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "#") {
		return false
	}
	content := trimReadModelDoDItemText(trimmed)
	return !(content == "" || strings.HasSuffix(content, ":") || strings.HasSuffix(content, "："))
}

func normalizeReadModelDoDText(value string) string {
	value = trimReadModelDoDItemText(value)
	value = strings.NewReplacer("**", "", "__", "", "`", "", "（", "(", "）", ")").Replace(value)
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func trimReadModelDoDItemText(value string) string {
	value = strings.TrimSpace(value)
	for {
		next := strings.TrimSpace(value)
		lower := strings.ToLower(next)
		switch {
		case strings.HasPrefix(lower, "- [ ]"):
			value = strings.TrimSpace(next[len("- [ ]"):])
		case strings.HasPrefix(lower, "- [x]"):
			value = strings.TrimSpace(next[len("- [x]"):])
		case strings.HasPrefix(lower, "* [ ]"):
			value = strings.TrimSpace(next[len("* [ ]"):])
		case strings.HasPrefix(lower, "* [x]"):
			value = strings.TrimSpace(next[len("* [x]"):])
		case strings.HasPrefix(next, "-"), strings.HasPrefix(next, "*"), strings.HasPrefix(next, "•"):
			value = strings.TrimSpace(next[1:])
		default:
			return strings.TrimSpace(value)
		}
	}
}

func runEvidenceRefIndex(snapshot RunSnapshot) map[string]struct{} {
	ret := map[string]struct{}{}
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return
		}
		ret[ref] = struct{}{}
	}
	add(snapshot.Run.ID)
	for _, item := range snapshot.Steps {
		add(item.ID)
	}
	for _, item := range snapshot.ToolCalls {
		add(item.ID)
		add(item.StepID)
	}
	for _, item := range snapshot.Patches {
		add(item.ID)
		add(item.StepID)
	}
	for _, item := range snapshot.ValidationResults {
		add(item.ID)
		add(item.StepID)
		add(item.EvidenceRef)
	}
	for _, item := range snapshot.RuntimeCheckpoints {
		add(item.ID)
		add(item.CurrentStepID)
	}
	for _, item := range snapshot.Results {
		add(item.ID)
		for _, ref := range item.EvidenceRefs {
			add(ref)
		}
	}
	for _, item := range snapshot.Artifacts {
		add(item.ID)
		add(item.StepID)
		add(item.ResultID)
		add(item.ToolCallID)
		add(item.PatchID)
		add(item.ValidationResultID)
		for _, ref := range item.EvidenceRefs {
			add(ref)
		}
	}
	for _, item := range snapshot.Decisions {
		add(item.ID)
		add(item.StepID)
	}
	for _, item := range snapshot.Approvals {
		add(item.ID)
		add(item.StepID)
	}
	for _, item := range snapshot.MemoryRecallTraces {
		add(item.ID)
		add(item.StepID)
		add(item.EvidenceRef)
	}
	return ret
}

func mapArtifacts(items []*resultmodel.Artifact) []Artifact {
	if len(items) == 0 {
		return nil
	}
	ret := make([]Artifact, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, Artifact{
			ID:                 string(item.ID),
			WorkspaceID:        string(item.WorkspaceID),
			RunID:              string(item.RunID),
			ResultID:           string(item.ResultID),
			StepID:             string(item.StepID),
			ToolCallID:         string(item.ToolCallID),
			PatchID:            string(item.PatchID),
			ValidationResultID: string(item.ValidationResultID),
			ArtifactType:       item.ArtifactType,
			Category:           item.Category,
			Name:               item.Name,
			Summary:            item.Summary,
			URI:                item.URI,
			ContentType:        item.ContentType,
			State:              string(item.State),
			EvidenceRefs:       cloneReadModelStringSlice(item.EvidenceRefs),
			Metadata:           cloneReadModelMap(item.Metadata),
			CreatedAt:          item.CreatedAt,
			UpdatedAt:          item.UpdatedAt,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapEventRecords(items []*eventstreammodel.EventRecord) []EventRecord {
	if len(items) == 0 {
		return nil
	}
	ret := make([]EventRecord, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, EventRecord{
			ID:           string(item.ID),
			Version:      item.Version,
			Topic:        item.Topic,
			OccurredAt:   item.OccurredAt,
			SourceModule: item.SourceModule,
			SubjectType:  item.SubjectType,
			SubjectID:    item.SubjectID,
			WorkspaceID:  string(item.WorkspaceID),
			RunID:        string(item.RunID),
			StepID:       string(item.StepID),
			Payload:      cloneReadModelMap(item.Payload),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func pageRunSteps(items []*runmodel.Step, query StepQuery) StepPage {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelStepPageLimit
	}
	if limit > readModelMaxStepPageLimit {
		limit = readModelMaxStepPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]presenter.StepView, 0, min(limit, len(items)))
	hasMore := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			hasMore = true
			break
		}
		mapped := presenter.MapStep(item)
		if mapped == nil {
			continue
		}
		selected = append(selected, projectStepView(*mapped))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return StepPage{
		RunID:             strings.TrimSpace(query.RunID),
		Steps:             selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}

func projectStepView(item presenter.StepView) presenter.StepView {
	return presenter.StepView{
		ID:     item.ID,
		Kind:   item.Kind,
		State:  item.State,
		Output: projectStepOutput(item.Output),
	}
}

func projectStepOutput(output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"approvalId":                   {},
		"assessmentOnly":               {},
		"assessmentOnlyReason":         {},
		"blockingApprovalId":           {},
		"blockingTicketId":             {},
		"capabilityId":                 {},
		"category":                     {},
		"completionState":              {},
		"contextBudget":                {},
		"decisionId":                   {},
		"dodAssessment":                {},
		"error":                        {},
		"failureClass":                 {},
		"failureReason":                {},
		"llmCallRefs":                  {},
		"llmUsage":                     {},
		"memoryRefs":                   {},
		"nextKind":                     {},
		"operation":                    {},
		"patchId":                      {},
		"prompt":                       {},
		"reason":                       {},
		"repairAttempt":                {},
		"resultId":                     {},
		"serverId":                     {},
		"stage":                        {},
		"state":                        {},
		"status":                       {},
		"streamDiagnostics":            {},
		"summary":                      {},
		"target":                       {},
		"targetRef":                    {},
		"tool":                         {},
		"toolCallId":                   {},
		"uri":                          {},
		"validationId":                 {},
		"workspaceExecutionSummary":    {},
		"workspaceExecutionSummaryRef": {},
	}
	ret := map[string]any{}
	for key, value := range output {
		if _, ok := allowed[key]; !ok {
			continue
		}
		ret[key] = value
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func pageRunToolCalls(items []*runmodel.ToolCall, query ToolCallQuery) ToolCallPage {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelToolCallPageLimit
	}
	if limit > readModelMaxToolCallPageLimit {
		limit = readModelMaxToolCallPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]ToolCall, 0, min(limit, len(items)))
	hasMore := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			hasMore = true
			break
		}
		selected = append(selected, projectToolCall(item))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return ToolCallPage{
		RunID:             strings.TrimSpace(query.RunID),
		ToolCalls:         selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}

func projectToolCall(item *runmodel.ToolCall) ToolCall {
	if item == nil {
		return ToolCall{}
	}
	return ToolCall{
		ID:          string(item.ID),
		WorkspaceID: string(item.WorkspaceID),
		RunID:       string(item.RunID),
		StepID:      string(item.StepID),
		ToolName:    item.ToolName,
		State:       string(item.State),
		StartedAt:   item.StartedAt,
		EndedAt:     item.EndedAt,
		ExitSummary: item.ExitSummary,
		Error:       item.Error,
	}
}

func ProjectRunSnapshotDetail(snapshot RunSnapshot) RunSnapshot {
	ret := snapshot
	if ret.Step != nil {
		projected := projectStepView(*ret.Step)
		ret.Step = &projected
	}
	if len(ret.Steps) > 0 {
		limit := min(len(ret.Steps), readModelStepPageLimit)
		projected := make([]presenter.StepView, 0, limit)
		for _, item := range ret.Steps[:limit] {
			projected = append(projected, projectStepView(item))
		}
		ret.Steps = projected
	}
	if len(ret.ToolCalls) > 0 {
		limit := min(len(ret.ToolCalls), readModelToolCallPageLimit)
		projected := make([]ToolCall, 0, limit)
		for _, item := range ret.ToolCalls[:limit] {
			projected = append(projected, projectToolCallView(item))
		}
		ret.ToolCalls = projected
	}
	if len(ret.RuntimeCheckpoints) > 0 {
		limit := min(len(ret.RuntimeCheckpoints), readModelCheckpointPageLimit)
		projected := make([]RuntimeCheckpoint, 0, limit)
		for _, item := range ret.RuntimeCheckpoints[:limit] {
			projected = append(projected, projectRuntimeCheckpointView(item))
		}
		ret.RuntimeCheckpoints = projected
	}
	if len(ret.Results) > 0 {
		limit := min(len(ret.Results), readModelResultPageLimit)
		projected := make([]Result, 0, limit)
		for _, item := range ret.Results[:limit] {
			projected = append(projected, projectResultView(item))
		}
		ret.Results = projected
	}
	if len(ret.Patches) > 0 {
		limit := min(len(ret.Patches), readModelPatchPageLimit)
		projected := make([]Patch, 0, limit)
		for _, item := range ret.Patches[:limit] {
			projected = append(projected, projectPatchView(item))
		}
		ret.Patches = projected
	}
	if len(ret.ValidationResults) > 0 {
		limit := min(len(ret.ValidationResults), readModelValidationPageLimit)
		projected := make([]ValidationResult, 0, limit)
		for _, item := range ret.ValidationResults[:limit] {
			projected = append(projected, projectValidationResultView(item))
		}
		ret.ValidationResults = projected
	}
	if len(ret.Artifacts) > 0 {
		limit := min(len(ret.Artifacts), readModelArtifactPageLimit)
		projected := make([]Artifact, 0, limit)
		for _, item := range ret.Artifacts[:limit] {
			projected = append(projected, projectArtifactView(item))
		}
		ret.Artifacts = projected
	}
	if len(ret.Decisions) > 0 {
		limit := min(len(ret.Decisions), readModelDecisionPageLimit)
		projected := make([]Decision, 0, limit)
		for _, item := range ret.Decisions[:limit] {
			projected = append(projected, projectDecisionView(item))
		}
		ret.Decisions = projected
	}
	if len(ret.Approvals) > 0 {
		limit := min(len(ret.Approvals), readModelApprovalPageLimit)
		projected := make([]Approval, 0, limit)
		for _, item := range ret.Approvals[:limit] {
			projected = append(projected, projectApprovalView(item))
		}
		ret.Approvals = projected
	}
	return ret
}

func ProjectWorkspaceSnapshotDetail(snapshot WorkspaceSnapshot) WorkspaceSnapshot {
	ret := snapshot
	if len(ret.Decisions) > 0 {
		limit := min(len(ret.Decisions), readModelDecisionPageLimit)
		projected := make([]Decision, 0, limit)
		for _, item := range ret.Decisions[:limit] {
			projected = append(projected, projectDecisionView(item))
		}
		ret.Decisions = projected
	}
	if len(ret.Approvals) > 0 {
		limit := min(len(ret.Approvals), readModelApprovalPageLimit)
		projected := make([]Approval, 0, limit)
		for _, item := range ret.Approvals[:limit] {
			projected = append(projected, projectApprovalView(item))
		}
		ret.Approvals = projected
	}
	return ret
}

func projectToolCallView(item ToolCall) ToolCall {
	return ToolCall{
		ID:          item.ID,
		WorkspaceID: item.WorkspaceID,
		RunID:       item.RunID,
		StepID:      item.StepID,
		ToolName:    item.ToolName,
		State:       item.State,
		StartedAt:   item.StartedAt,
		EndedAt:     item.EndedAt,
		ExitSummary: item.ExitSummary,
		Error:       item.Error,
	}
}

func pageRunRuntimeCheckpoints(items []*runmodel.RuntimeCheckpoint, query RuntimeCheckpointQuery) RuntimeCheckpointPage {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelCheckpointPageLimit
	}
	if limit > readModelMaxCheckpointPageLimit {
		limit = readModelMaxCheckpointPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]RuntimeCheckpoint, 0, min(limit, len(items)))
	hasMore := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			hasMore = true
			break
		}
		selected = append(selected, projectRuntimeCheckpoint(item))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return RuntimeCheckpointPage{
		RunID:              strings.TrimSpace(query.RunID),
		RuntimeCheckpoints: selected,
		Limit:              limit,
		CursorAfterID:      cursorAfterID,
		NextCursorAfterID:  nextCursor,
		HasMore:            hasMore,
	}
}

func projectRuntimeCheckpoint(item *runmodel.RuntimeCheckpoint) RuntimeCheckpoint {
	if item == nil {
		return RuntimeCheckpoint{}
	}
	return RuntimeCheckpoint{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		RunID:              string(item.RunID),
		CheckpointType:     string(item.CheckpointType),
		CurrentStepID:      string(item.CurrentStepID),
		RunStateSnapshot:   string(item.RunStateSnapshot),
		BlockingTicketID:   string(item.BlockingTicketID),
		BlockingApprovalID: string(item.BlockingApprovalID),
		ThreadRef:          item.ThreadRef,
		OccurredAt:         item.OccurredAt,
		RestorableUntil:    item.RestorableUntil,
	}
}

func pageRunResults(items []*resultmodel.Result, query ResultQuery) ResultPage {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelResultPageLimit
	}
	if limit > readModelMaxResultPageLimit {
		limit = readModelMaxResultPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]Result, 0, min(limit, len(items)))
	hasMore := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			hasMore = true
			break
		}
		selected = append(selected, projectResult(item))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return ResultPage{
		RunID:             strings.TrimSpace(query.RunID),
		Results:           selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}

func projectResult(item *resultmodel.Result) Result {
	if item == nil {
		return Result{}
	}
	artifactRefs := make([]string, 0, len(item.ArtifactRefs))
	for _, ref := range item.ArtifactRefs {
		if ref != "" {
			artifactRefs = append(artifactRefs, string(ref))
		}
	}
	return Result{
		ID:           string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		GoalID:       string(item.GoalID),
		DoDID:        string(item.DoDID),
		State:        string(item.State),
		Title:        item.Title,
		Summary:      item.Summary,
		SourceType:   item.SourceType,
		SourceRef:    item.SourceRef,
		EvidenceRefs: cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs: artifactRefs,
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}
}

func projectRuntimeCheckpointView(item RuntimeCheckpoint) RuntimeCheckpoint {
	return RuntimeCheckpoint{
		ID:                 item.ID,
		WorkspaceID:        item.WorkspaceID,
		RunID:              item.RunID,
		CheckpointType:     item.CheckpointType,
		CurrentStepID:      item.CurrentStepID,
		RunStateSnapshot:   item.RunStateSnapshot,
		BlockingTicketID:   item.BlockingTicketID,
		BlockingApprovalID: item.BlockingApprovalID,
		ThreadRef:          item.ThreadRef,
		OccurredAt:         item.OccurredAt,
		RestorableUntil:    item.RestorableUntil,
	}
}

func projectResultView(item Result) Result {
	return Result{
		ID:           item.ID,
		WorkspaceID:  item.WorkspaceID,
		RunID:        item.RunID,
		GoalID:       item.GoalID,
		DoDID:        item.DoDID,
		State:        item.State,
		Title:        item.Title,
		Summary:      item.Summary,
		SourceType:   item.SourceType,
		SourceRef:    item.SourceRef,
		EvidenceRefs: cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs: cloneReadModelStringSlice(item.ArtifactRefs),
		CreatedAt:    item.CreatedAt,
		UpdatedAt:    item.UpdatedAt,
	}
}

func pageRunPatches(items []*runmodel.Patch, query PatchQuery) PatchPage {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelPatchPageLimit
	}
	if limit > readModelMaxPatchPageLimit {
		limit = readModelMaxPatchPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]Patch, 0, min(limit, len(items)))
	hasMore := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			hasMore = true
			break
		}
		selected = append(selected, projectPatch(item))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return PatchPage{
		RunID:             strings.TrimSpace(query.RunID),
		Patches:           selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}

func projectPatch(item *runmodel.Patch) Patch {
	if item == nil {
		return Patch{}
	}
	addedLines, deletedLines := readModelPatchLineStats(item)
	return Patch{
		ID:               string(item.ID),
		WorkspaceID:      string(item.WorkspaceID),
		RunID:            string(item.RunID),
		StepID:           string(item.StepID),
		ToolCallID:       string(item.ToolCallID),
		ApprovalID:       string(item.ApprovalID),
		RevertApprovalID: string(item.RevertApprovalID),
		PatchType:        item.PatchType,
		TargetRef:        item.TargetRef,
		Summary:          item.Summary,
		State:            string(item.State),
		AddedLines:       addedLines,
		DeletedLines:     deletedLines,
		BeforeHash:       item.BeforeHash,
		AfterHash:        item.AfterHash,
		CreatedAt:        item.CreatedAt,
		AppliedAt:        item.AppliedAt,
		RevertedAt:       item.RevertedAt,
	}
}

func projectPatchView(item Patch) Patch {
	return Patch{
		ID:               item.ID,
		WorkspaceID:      item.WorkspaceID,
		RunID:            item.RunID,
		StepID:           item.StepID,
		ToolCallID:       item.ToolCallID,
		ApprovalID:       item.ApprovalID,
		RevertApprovalID: item.RevertApprovalID,
		PatchType:        item.PatchType,
		TargetRef:        item.TargetRef,
		Summary:          item.Summary,
		State:            item.State,
		AddedLines:       item.AddedLines,
		DeletedLines:     item.DeletedLines,
		BeforeHash:       item.BeforeHash,
		AfterHash:        item.AfterHash,
		CreatedAt:        item.CreatedAt,
		AppliedAt:        item.AppliedAt,
		RevertedAt:       item.RevertedAt,
	}
}

func pageRunValidationResults(items []*runmodel.ValidationResult, query ValidationResultQuery) ValidationResultPage {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelValidationPageLimit
	}
	if limit > readModelMaxValidationPageLimit {
		limit = readModelMaxValidationPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]ValidationResult, 0, min(limit, len(items)))
	hasMore := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			hasMore = true
			break
		}
		selected = append(selected, projectValidationResult(item))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return ValidationResultPage{
		RunID:             strings.TrimSpace(query.RunID),
		ValidationResults: selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}

func projectValidationResult(item *runmodel.ValidationResult) ValidationResult {
	if item == nil {
		return ValidationResult{}
	}
	return ValidationResult{
		ID:              string(item.ID),
		WorkspaceID:     string(item.WorkspaceID),
		RunID:           string(item.RunID),
		StepID:          string(item.StepID),
		ValidationKind:  item.ValidationKind,
		TargetRef:       item.TargetRef,
		State:           string(item.State),
		Summary:         item.Summary,
		EvidenceRef:     item.EvidenceRef,
		EvidenceQuality: string(item.EvidenceQuality),
		EvidenceReason:  item.EvidenceReason,
		ZeroWork:        item.ZeroWork,
		EvidenceTags:    cloneReadModelStringSlice(item.EvidenceTags),
		StartedAt:       item.StartedAt,
		EndedAt:         item.EndedAt,
	}
}

func projectValidationResultView(item ValidationResult) ValidationResult {
	return ValidationResult{
		ID:              item.ID,
		WorkspaceID:     item.WorkspaceID,
		RunID:           item.RunID,
		StepID:          item.StepID,
		ValidationKind:  item.ValidationKind,
		TargetRef:       item.TargetRef,
		State:           item.State,
		Summary:         item.Summary,
		EvidenceRef:     item.EvidenceRef,
		EvidenceQuality: item.EvidenceQuality,
		EvidenceReason:  item.EvidenceReason,
		ZeroWork:        item.ZeroWork,
		EvidenceTags:    cloneReadModelStringSlice(item.EvidenceTags),
		StartedAt:       item.StartedAt,
		EndedAt:         item.EndedAt,
	}
}

func pageRunArtifacts(items []*resultmodel.Artifact, query ArtifactQuery) ArtifactPage {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelArtifactPageLimit
	}
	if limit > readModelMaxArtifactPageLimit {
		limit = readModelMaxArtifactPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]Artifact, 0, min(limit, len(items)))
	hasMore := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			hasMore = true
			break
		}
		selected = append(selected, projectArtifact(item))
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = selected[len(selected)-1].ID
	}
	return ArtifactPage{
		RunID:             strings.TrimSpace(query.RunID),
		Artifacts:         selected,
		Limit:             limit,
		CursorAfterID:     cursorAfterID,
		NextCursorAfterID: nextCursor,
		HasMore:           hasMore,
	}
}

func projectArtifact(item *resultmodel.Artifact) Artifact {
	if item == nil {
		return Artifact{}
	}
	return Artifact{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		RunID:              string(item.RunID),
		ResultID:           string(item.ResultID),
		StepID:             string(item.StepID),
		ToolCallID:         string(item.ToolCallID),
		PatchID:            string(item.PatchID),
		ValidationResultID: string(item.ValidationResultID),
		ArtifactType:       item.ArtifactType,
		Category:           item.Category,
		Name:               item.Name,
		Summary:            item.Summary,
		URI:                item.URI,
		ContentType:        item.ContentType,
		State:              string(item.State),
		EvidenceRefs:       cloneReadModelStringSlice(item.EvidenceRefs),
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}

func projectArtifactView(item Artifact) Artifact {
	return Artifact{
		ID:                 item.ID,
		WorkspaceID:        item.WorkspaceID,
		RunID:              item.RunID,
		ResultID:           item.ResultID,
		StepID:             item.StepID,
		ToolCallID:         item.ToolCallID,
		PatchID:            item.PatchID,
		ValidationResultID: item.ValidationResultID,
		ArtifactType:       item.ArtifactType,
		Category:           item.Category,
		Name:               item.Name,
		Summary:            item.Summary,
		URI:                item.URI,
		ContentType:        item.ContentType,
		State:              item.State,
		EvidenceRefs:       cloneReadModelStringSlice(item.EvidenceRefs),
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}

func pageDecisionTickets(items []*decisionmodel.Ticket, query DecisionQuery) ([]*decisionmodel.Ticket, int, string, string, bool) {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelDecisionPageLimit
	}
	if limit > readModelMaxDecisionPageLimit {
		limit = readModelMaxDecisionPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]*decisionmodel.Ticket, 0, min(limit, len(items)))
	hasMore := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			hasMore = true
			break
		}
		selected = append(selected, item)
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = string(selected[len(selected)-1].ID)
	}
	return selected, limit, cursorAfterID, nextCursor, hasMore
}

func projectDecisionView(item Decision) Decision {
	return Decision{
		ID:             item.ID,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         item.StepID,
		Reason:         item.Reason,
		DecisionKind:   item.DecisionKind,
		Question:       item.Question,
		State:          item.State,
		ResolutionID:   item.ResolutionID,
		ResolutionType: item.ResolutionType,
		AutoGovernance: item.AutoGovernance,
	}
}

func pageApprovals(items []*approvalmodel.Approval, query ApprovalQuery) ([]*approvalmodel.Approval, int, string, string, bool) {
	limit := query.Limit
	if limit <= 0 {
		limit = readModelApprovalPageLimit
	}
	if limit > readModelMaxApprovalPageLimit {
		limit = readModelMaxApprovalPageLimit
	}
	cursorAfterID := strings.TrimSpace(query.CursorAfterID)
	started := cursorAfterID == ""
	selected := make([]*approvalmodel.Approval, 0, min(limit, len(items)))
	hasMore := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			hasMore = true
			break
		}
		selected = append(selected, item)
	}
	nextCursor := ""
	if len(selected) > 0 {
		nextCursor = string(selected[len(selected)-1].ID)
	}
	return selected, limit, cursorAfterID, nextCursor, hasMore
}

func projectApprovalView(item Approval) Approval {
	return Approval{
		ID:             item.ID,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		StepID:         item.StepID,
		Reason:         item.Reason,
		Action:         item.Action,
		State:          item.State,
		AutoGovernance: item.AutoGovernance,
	}
}

func mapProjectedEventRecords(items []*eventstreammodel.EventRecord) []EventRecord {
	if len(items) == 0 {
		return nil
	}
	ret := make([]EventRecord, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, EventRecord{
			ID:           string(item.ID),
			Version:      item.Version,
			Topic:        item.Topic,
			OccurredAt:   item.OccurredAt,
			SourceModule: item.SourceModule,
			SubjectType:  item.SubjectType,
			SubjectID:    item.SubjectID,
			WorkspaceID:  string(item.WorkspaceID),
			RunID:        string(item.RunID),
			StepID:       string(item.StepID),
			Payload:      projectEventPayload(item.Payload),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func projectEventPayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		events.PayloadKeyAction:             {},
		events.PayloadKeyActiveRunID:        {},
		events.PayloadKeyAddedLines:         {},
		events.PayloadKeyApprovalID:         {},
		events.PayloadKeyArtifactRefs:       {},
		events.PayloadKeyArtifactType:       {},
		events.PayloadKeyBlockingApprovalID: {},
		events.PayloadKeyBlockingTicketID:   {},
		events.PayloadKeyChunkCount:         {},
		events.PayloadKeyCompletedAt:        {},
		events.PayloadKeyConfidenceScore:    {},
		events.PayloadKeyContentHash:        {},
		events.PayloadKeyContentType:        {},
		events.PayloadKeyCostEstimated:      {},
		events.PayloadKeyCostSource:         {},
		events.PayloadKeyCurrency:           {},
		events.PayloadKeyDeletedLines:       {},
		events.PayloadKeyDiffHash:           {},
		events.PayloadKeyDoDID:              {},
		events.PayloadKeyDurationMs:         {},
		events.PayloadKeyElapsedMs:          {},
		events.PayloadKeyError:              {},
		events.PayloadKeyEstimated:          {},
		events.PayloadKeyEstimatedCost:      {},
		events.PayloadKeyEvidenceRefs:       {},
		events.PayloadKeyFailureClass:       {},
		events.PayloadKeyFailureReason:      {},
		events.PayloadKeyGoalID:             {},
		events.PayloadKeyItemID:             {},
		events.PayloadKeyItemType:           {},
		events.PayloadKeyKind:               {},
		events.PayloadKeyLLMDecisionPreview: {},
		events.PayloadKeyLLMModel:           {},
		events.PayloadKeyLLMProviderID:      {},
		events.PayloadKeyModel:              {},
		events.PayloadKeyName:               {},
		events.PayloadKeyOperation:          {},
		events.PayloadKeyOutputTokens:       {},
		events.PayloadKeyPatchID:            {},
		events.PayloadKeyPatchType:          {},
		events.PayloadKeyPolicy:             {},
		events.PayloadKeyPolicyBlocked:      {},
		events.PayloadKeyPolicyMatched:      {},
		events.PayloadKeyPromptFingerprint:  {},
		events.PayloadKeyProvider:           {},
		events.PayloadKeyProviderID:         {},
		events.PayloadKeyReason:             {},
		events.PayloadKeyReceivedBytes:      {},
		events.PayloadKeyRequestBytes:       {},
		events.PayloadKeyResponseBytes:      {},
		events.PayloadKeyResultID:           {},
		events.PayloadKeyRisk:               {},
		events.PayloadKeyRunID:              {},
		events.PayloadKeySourceRef:          {},
		events.PayloadKeyStage:              {},
		events.PayloadKeyStartedAt:          {},
		events.PayloadKeyState:              {},
		events.PayloadKeyStatus:             {},
		events.PayloadKeyStepID:             {},
		events.PayloadKeyStreaming:          {},
		events.PayloadKeySummary:            {},
		events.PayloadKeyTargetRef:          {},
		events.PayloadKeyToolCallID:         {},
		events.PayloadKeyTotalTokens:        {},
		events.PayloadKeyTraceID:            {},
		events.PayloadKeyType:               {},
		events.PayloadKeyURI:                {},
		events.PayloadKeyUsageSource:        {},
		events.PayloadKeyWorkspaceID:        {},
	}
	ret := map[string]any{}
	for key, value := range payload {
		if _, ok := allowed[key]; !ok {
			continue
		}
		if projected, ok := projectEventPayloadValue(value); ok {
			ret[key] = projected
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func projectEventPayloadValue(value any) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case string:
		if len(typed) <= eventPayloadStringLimit {
			return typed, true
		}
		return typed[:eventPayloadStringLimit] + "...", true
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed, true
	case []string:
		limit := min(len(typed), eventPayloadListLimit)
		ret := make([]string, 0, limit)
		for _, item := range typed[:limit] {
			if len(item) > eventPayloadStringLimit {
				item = item[:eventPayloadStringLimit] + "..."
			}
			ret = append(ret, item)
		}
		return ret, true
	case []any:
		limit := min(len(typed), eventPayloadListLimit)
		ret := make([]any, 0, limit)
		for _, item := range typed[:limit] {
			if projected, ok := projectEventPayloadValue(item); ok {
				ret = append(ret, projected)
			}
		}
		if len(ret) == 0 {
			return nil, false
		}
		return ret, true
	default:
		return nil, false
	}
}

func mapMemoryRecallTraces(items []*memorymodel.MemoryRecallTrace) []MemoryRecallTrace {
	if len(items) == 0 {
		return nil
	}
	ret := make([]MemoryRecallTrace, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, MemoryRecallTrace{
			ID:                     string(item.ID),
			WorkspaceID:            string(item.WorkspaceID),
			RunID:                  string(item.RunID),
			StepID:                 string(item.StepID),
			GraphNodeID:            item.GraphNodeID,
			Stage:                  item.Stage,
			Source:                 item.Source,
			Policy:                 item.Policy,
			Status:                 item.Status,
			ProviderID:             item.ProviderID,
			Operation:              item.Operation,
			QueryTextPreview:       item.QueryTextPreview,
			QueryTextSha256:        item.QueryTextSha256,
			MinScore:               item.MinScore,
			Limit:                  item.Limit,
			EmbeddingModel:         item.EmbeddingModel,
			VectorDimension:        item.VectorDimension,
			RefreshSource:          item.RefreshSource,
			SelectionPolicyVersion: item.SelectionPolicyVersion,
			CandidateMemoryIDs:     memoryIDsToReadModelStrings(item.CandidateMemoryIDs),
			SelectedMemoryIDs:      memoryIDsToReadModelStrings(item.SelectedMemoryIDs),
			ExcludedMemoryIDs:      memoryIDsToReadModelStrings(item.ExcludedMemoryIDs),
			TypeQuota:              cloneReadModelIntMap(item.TypeQuota),
			SelectionScores:        mapMemoryRecallSelectionScores(item.SelectionScores),
			RetrievedMemoryIDs:     memoryIDsToReadModelStrings(item.RetrievedMemoryIDs),
			RetrievedMemoryHits:    mapMemoryRecallHits(item.RetrievedMemoryHits),
			Reason:                 item.Reason,
			OccurredAt:             item.OccurredAt,
			EvidenceRef:            item.EvidenceRef,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapMemoryRecallSelectionScores(items []memorymodel.MemoryRecallSelectionScore) []MemoryRecallSelectionScore {
	if len(items) == 0 {
		return nil
	}
	ret := make([]MemoryRecallSelectionScore, 0, len(items))
	for _, item := range items {
		ret = append(ret, MemoryRecallSelectionScore{
			ID:     string(item.ID),
			Score:  item.Score,
			Reason: item.Reason,
		})
	}
	return ret
}

func cloneReadModelIntMap(input map[string]int) map[string]int {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]int, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func mapMemoryRecallHits(items []memorymodel.MemoryRecallHit) []MemoryRecallHit {
	if len(items) == 0 {
		return nil
	}
	ret := make([]MemoryRecallHit, 0, len(items))
	for _, item := range items {
		ret = append(ret, MemoryRecallHit{
			ID:              string(item.ID),
			Summary:         item.Summary,
			VectorScore:     item.VectorScore,
			VectorDimension: item.VectorDimension,
		})
	}
	return ret
}

func memoryIDsToReadModelStrings(input []common.MemoryID) []string {
	if len(input) == 0 {
		return nil
	}
	ret := make([]string, 0, len(input))
	for _, item := range input {
		ret = append(ret, string(item))
	}
	return ret
}

func cloneReadModelMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneReadModelStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	return append([]string(nil), input...)
}

func readModelStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func readModelBoolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		return false
	}
}

func readModelIntValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		if typed == float64(int(typed)) {
			return int(typed)
		}
	default:
		return 0
	}
	return 0
}

func readModelFloatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func readModelMapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneReadModelMap(typed)
	default:
		return nil
	}
}

func readModelMapSliceValue(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneReadModelMap(item))
		}
		return ret
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				ret = append(ret, cloneReadModelMap(mapped))
			}
		}
		return ret
	default:
		return nil
	}
}

func readModelStringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func readModelPolicyDecision(value any) string {
	policy := readModelMapValue(value)
	if len(policy) == 0 {
		return ""
	}
	return readModelStringValue(policy["decision"])
}

func mapRunStatus(snapshot RunSnapshot) RunStatus {
	ret := RunStatus{
		ID:                 snapshot.Run.ID,
		WorkspaceID:        snapshot.Run.WorkspaceID,
		State:              snapshot.Run.State,
		StepCount:          len(snapshot.Steps),
		DecisionCount:      len(snapshot.Decisions),
		ApprovalCount:      len(snapshot.Approvals),
		BlockingTicketID:   snapshot.Run.BlockingTicketID,
		BlockingApprovalID: snapshot.Run.BlockingApprovalID,
		FailureReason:      snapshot.Run.FailureReason,
	}
	if snapshot.Step != nil {
		ret.CurrentStepID = snapshot.Step.ID
		ret.CurrentStepKind = snapshot.Step.Kind
		ret.CurrentStepState = snapshot.Step.State
	}
	return ret
}

func mapRunObservability(snapshot RunSnapshot) RunObservability {
	timeline := runtimeTimeline(snapshot)
	return RunObservability{
		Run:                  snapshot.Run,
		Outcome:              snapshot.Outcome,
		Progress:             runtimeProgress(snapshot),
		TaskProgress:         snapshot.TaskProgress,
		RepairLoop:           snapshot.RepairLoop,
		TaskDecompositions:   snapshot.TaskDecompositions,
		TaskPlans:            snapshot.TaskPlans,
		ExecutionSections:    snapshot.ExecutionSections,
		LLMContextBudgets:    snapshot.LLMContextBudgets,
		LLMUsageTraces:       snapshot.LLMUsageTraces,
		LLMUsageSummary:      snapshot.LLMUsageSummary,
		MemoryRecallTraces:   snapshot.MemoryRecallTraces,
		WebSources:           snapshot.WebSources,
		ClosureTraces:        snapshot.ClosureTraces,
		DoDEvidence:          snapshot.DoDEvidence,
		ResultQuality:        snapshot.ResultQuality,
		Evidence:             runtimeEvidenceSummary(snapshot, len(timelineEvents(timeline))),
		LatestCheckpoint:     latestRuntimeCheckpoint(snapshot.RuntimeCheckpoints),
		SelectedCapabilities: snapshot.SelectedCapabilities,
		UsedCapabilities:     snapshot.UsedCapabilities,
		Timeline:             timeline,
	}
}

func mapWorkspaceTaskProgress(snapshot WorkspaceSnapshot) TaskProgress {
	language := workspaceSnapshotLanguage(snapshot)
	ret := TaskProgress{
		WorkspaceID: snapshot.Workspace.ID,
		State:       snapshot.Workspace.State,
		Recovery: RecoveryPlan{
			Mode:       "idle",
			NextAction: "start_workspace_execution",
			Reason:     localizedWorkspaceProgressText(language, "no_active_run"),
		},
		RepairLoop: RepairLoop{
			State: "inactive",
		},
	}
	if snapshot.Workspace.ActiveRunID != "" {
		ret.RunID = snapshot.Workspace.ActiveRunID
		ret.Recovery.RunID = snapshot.Workspace.ActiveRunID
		ret.Recovery.Mode = "continue"
		ret.Recovery.NextAction = "inspect_active_run"
		ret.Recovery.Reason = localizedWorkspaceProgressText(language, "active_run")
	}
	for _, item := range snapshot.Decisions {
		if isResolvedDecisionState(item.State) {
			continue
		}
		ret.Blockers = append(ret.Blockers, ProgressBlocker{
			ID:          item.ID,
			Kind:        "decision",
			State:       item.State,
			Reason:      firstNonEmptyReadModel(item.Question, item.Reason),
			EvidenceRef: item.StepID,
		})
		if ret.Recovery.BlockingTicketID == "" {
			ret.Recovery.Mode = "wait_decision"
			ret.Recovery.NextAction = "resolve_decision_ticket"
			ret.Recovery.BlockingTicketID = item.ID
			ret.Recovery.StepID = item.StepID
			ret.Recovery.Reason = firstNonEmptyReadModel(item.Question, item.Reason)
		}
	}
	for _, item := range snapshot.Approvals {
		if isResolvedApprovalState(item.State) {
			continue
		}
		ret.Blockers = append(ret.Blockers, ProgressBlocker{
			ID:          item.ID,
			Kind:        "approval",
			State:       item.State,
			Reason:      item.Reason,
			EvidenceRef: item.StepID,
		})
		if ret.Recovery.BlockingApprovalID == "" && ret.Recovery.BlockingTicketID == "" {
			ret.Recovery.Mode = "wait_approval"
			ret.Recovery.NextAction = "resolve_approval"
			ret.Recovery.BlockingApprovalID = item.ID
			ret.Recovery.StepID = item.StepID
			ret.Recovery.Reason = item.Reason
		}
	}
	ret.Milestones = []ProgressMilestone{
		{ID: "workspace_defined", Title: localizedWorkspaceProgressText(language, "workspace_defined"), State: "completed", EvidenceRefs: []string{snapshot.Workspace.ID}},
		{ID: "formal_result", Title: localizedMilestoneTitle(language, "formal_result"), State: progressStateFromCount(len(snapshot.Results), false, snapshot.Workspace.State == string(common.WorkspaceStateCompleted)), EvidenceRefs: idsFromResults(snapshot.Results)},
	}
	ret.Items = []ProgressItem{
		{ID: "runs", Kind: "run", Title: localizedWorkspaceProgressText(language, "runs"), State: progressStateFromCount(len(snapshot.Runs), false, false)},
		{ID: "results", Kind: "result", Title: localizedWorkspaceProgressText(language, "results"), State: progressStateFromCount(len(snapshot.Results), false, false), EvidenceRefs: idsFromResults(snapshot.Results)},
		{ID: "artifacts", Kind: "artifact", Title: localizedWorkspaceProgressText(language, "artifacts"), State: progressStateFromCount(len(snapshot.Artifacts), false, false), EvidenceRefs: idsFromArtifacts(snapshot.Artifacts)},
	}
	ret.CurrentMilestone = currentProgressMilestone(ret.Milestones)
	ret.EvidenceRefs = compactUniqueStrings(append(idsFromResults(snapshot.Results), idsFromArtifacts(snapshot.Artifacts)...))
	ret.Summary = taskProgressSummary(ret, nil)
	return ret
}

func mapTaskProgress(snapshot RunSnapshot) TaskProgress {
	language := snapshotLanguage(snapshot)
	latestCheckpoint := latestRuntimeCheckpoint(snapshot.RuntimeCheckpoints)
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	ret := TaskProgress{
		WorkspaceID: snapshot.Run.WorkspaceID,
		RunID:       snapshot.Run.ID,
		State:       snapshot.Run.State,
		Recovery: RecoveryPlan{
			Mode:       "continue",
			NextAction: localizedRecoveryNextAction(language),
			RunID:      snapshot.Run.ID,
		},
	}
	if snapshot.Step != nil {
		ret.Recovery.StepID = snapshot.Step.ID
	}
	if latestCheckpoint != nil {
		ret.Recovery.CheckpointID = latestCheckpoint.ID
	}

	ret.Blockers = taskProgressBlockers(snapshot)
	ret.Milestones = []ProgressMilestone{
		{ID: "run_created", Title: localizedMilestoneTitle(language, "run_created"), State: "completed", EvidenceRefs: []string{snapshot.Run.ID}},
		{ID: "execution_steps", Title: localizedMilestoneTitle(language, "execution_steps"), State: stepProgressState(snapshot), EvidenceRefs: idsFromSteps(snapshot.Steps)},
		{ID: "governance", Title: localizedMilestoneTitle(language, "governance"), State: governanceProgressState(snapshot), EvidenceRefs: governanceEvidenceRefs(snapshot)},
		{ID: "validation", Title: localizedMilestoneTitle(language, "validation"), State: validationProgressState(snapshot), EvidenceRefs: idsFromValidations(snapshot.ValidationResults)},
		{ID: "formal_result", Title: localizedMilestoneTitle(language, "formal_result"), State: resultProgressState(snapshot), EvidenceRefs: append(idsFromResults(results), idsFromArtifacts(artifacts)...)},
	}
	ret.Items = taskProgressItems(snapshot)
	ret.CurrentMilestone = currentProgressMilestone(ret.Milestones)
	ret.EvidenceRefs = taskProgressEvidenceRefs(snapshot)
	ret.Recovery = taskProgressRecovery(snapshot, latestCheckpoint, ret.Blockers, ret.Recovery)
	ret.BlockingTicketID = ret.Recovery.BlockingTicketID
	ret.BlockingApprovalID = ret.Recovery.BlockingApprovalID
	ret.RepairLoop = mapTaskProgressRepairLoop(snapshot, ret.Recovery)
	ret.Summary = taskProgressSummary(ret, &snapshot)
	return ret
}

func mapWorkspaceContextBundle(snapshot WorkspaceSnapshot) ContextBundle {
	language := workspaceSnapshotLanguage(snapshot)
	progress := snapshot.TaskProgress
	ret := ContextBundle{
		WorkspaceID:  snapshot.Workspace.ID,
		Mode:         "workspace",
		Summary:      localizedWorkspaceProgressText(language, "context_summary"),
		TaskProgress: progress,
		Recovery:     progress.Recovery,
		Sections: []ContextSection{
			{
				ID:       "workspace",
				Kind:     "workspace",
				Title:    localizedWorkspaceProgressText(language, "workspace"),
				Priority: 100,
				State:    snapshot.Workspace.State,
				Summary:  snapshot.Workspace.Title,
				Data: map[string]any{
					"id":                   snapshot.Workspace.ID,
					"title":                snapshot.Workspace.Title,
					"description":          snapshot.Workspace.Description,
					"deliverableDirectory": snapshot.Workspace.DeliverableDirectory,
				},
			},
			{
				ID:           "task_progress",
				Kind:         "task_progress",
				Title:        localizedWorkspaceProgressText(language, "task_progress"),
				Priority:     90,
				State:        progress.State,
				Summary:      progress.Summary,
				EvidenceRefs: progress.EvidenceRefs,
			},
		},
	}
	ret.CompressionCandidates = contextCompressionCandidates(ret)
	ret.CompressedContext = compressedContextFromBundle(ret, language)
	return ret
}

func mapRunContextBundle(snapshot RunSnapshot) ContextBundle {
	progress := runSnapshotTaskProgress(snapshot)
	timeline := runtimeTimeline(snapshot)
	recentEvidence := contextRecentEvidence(timeline, 12)
	ret := ContextBundle{
		WorkspaceID:      snapshot.Run.WorkspaceID,
		RunID:            snapshot.Run.ID,
		Mode:             "run_recovery",
		Summary:          contextBundleSummary(snapshot, progress),
		TaskProgress:     progress,
		Recovery:         progress.Recovery,
		LatestCheckpoint: latestRuntimeCheckpoint(snapshot.RuntimeCheckpoints),
		RecentEvidence:   recentEvidence,
	}
	ret.Sections = []ContextSection{
		contextWorkspaceSection(snapshot),
		contextRunSection(snapshot),
		contextGoalSection(snapshot),
		contextDoDSection(snapshot),
		contextExecutionContextSection(snapshot),
		contextTaskProgressSection(progress),
		contextTaskDecompositionSection(snapshot.TaskDecompositions),
		contextExecutionSectionsSection(snapshot.ExecutionSections),
		contextMemoryRecallSection(snapshot.MemoryRecallTraces),
		contextGovernanceSection(snapshot),
		contextFormalOutputSection(snapshot),
		contextRecentEvidenceSection(recentEvidence),
		contextRecoverySection(progress.Recovery),
	}
	ret.Sections = compactContextSections(ret.Sections)
	ret.CompressionCandidates = contextCompressionCandidates(ret)
	ret.CompressedContext = compressedContextFromBundle(ret, snapshotLanguage(snapshot))
	return ret
}

func compressedContextFromBundle(bundle ContextBundle, language string) CompressedContext {
	sectionRefs := make([]string, 0, len(bundle.Sections))
	for _, section := range bundle.Sections {
		sectionRefs = append(sectionRefs, section.ID)
	}
	sourceEvidenceRefs := compactUniqueStrings(append(bundle.TaskProgress.EvidenceRefs, contextEvidenceRefs(bundle.RecentEvidence)...))
	return CompressedContext{
		Version:            "workorch.compressedContext.v1",
		Mode:               "deterministic_recovery_summary",
		WorkspaceID:        bundle.WorkspaceID,
		RunID:              bundle.RunID,
		Summary:            bundle.Summary,
		CurrentMilestone:   bundle.TaskProgress.CurrentMilestone,
		Milestones:         bundle.TaskProgress.Milestones,
		Blockers:           bundle.TaskProgress.Blockers,
		Recovery:           bundle.Recovery,
		RecentEvidence:     lastContextEvidence(bundle.RecentEvidence, 6),
		ResultRefs:         resultRefsFromTaskProgress(bundle.TaskProgress),
		SourceSectionRefs:  compactUniqueStrings(sectionRefs),
		SourceEvidenceRefs: sourceEvidenceRefs,
		RetentionPolicy:    localizedRetentionPolicy(language),
	}
}

func compressedContextFromRunSnapshot(snapshot RunSnapshot) CompressedContext {
	language := snapshotLanguage(snapshot)
	progress := runSnapshotTaskProgress(snapshot)
	timeline := runtimeTimeline(snapshot)
	recentEvidence := contextRecentEvidence(timeline, 12)
	sourceEvidenceRefs := compactUniqueStrings(append(progress.EvidenceRefs, contextEvidenceRefs(recentEvidence)...))
	return CompressedContext{
		Version:            "workorch.compressedContext.v1",
		Mode:               "deterministic_recovery_summary",
		WorkspaceID:        snapshot.Run.WorkspaceID,
		RunID:              snapshot.Run.ID,
		Summary:            contextBundleSummary(snapshot, progress),
		CurrentMilestone:   progress.CurrentMilestone,
		Milestones:         progress.Milestones,
		Blockers:           progress.Blockers,
		Recovery:           progress.Recovery,
		RecentEvidence:     lastContextEvidence(recentEvidence, 6),
		ResultRefs:         resultRefsFromTaskProgress(progress),
		SourceSectionRefs:  compressedContextSourceSectionRefs(snapshot, recentEvidence),
		SourceEvidenceRefs: sourceEvidenceRefs,
		RetentionPolicy:    localizedRetentionPolicy(language),
	}
}

func runSnapshotTaskProgress(snapshot RunSnapshot) TaskProgress {
	if snapshot.TaskProgress.RunID == snapshot.Run.ID && snapshot.TaskProgress.WorkspaceID == snapshot.Run.WorkspaceID {
		return snapshot.TaskProgress
	}
	return mapTaskProgress(snapshot)
}

func runSnapshotTaskDecompositions(snapshot RunSnapshot) []TaskDecomposition {
	if len(snapshot.TaskDecompositions) > 0 {
		return snapshot.TaskDecompositions
	}
	return mapTaskDecompositions(snapshot)
}

func compressedContextSourceSectionRefs(snapshot RunSnapshot, recentEvidence []ContextEvidence) []string {
	sections := []ContextSection{
		{ID: "workspace", Priority: 100},
		{ID: "run", Priority: 100},
	}
	if len(readModelMapValue(snapshot.Run.ExecutionContext["goal"])) > 0 {
		sections = append(sections, ContextSection{ID: "goal", Priority: 95})
	}
	if len(readModelMapValue(snapshot.Run.ExecutionContext["dod"])) > 0 {
		sections = append(sections, ContextSection{ID: "dod", Priority: 95})
	}
	if hasNonDefinitionExecutionContext(snapshot.Run.ExecutionContext) {
		sections = append(sections, ContextSection{ID: "execution_context", Priority: 85})
	}
	sections = append(sections, ContextSection{ID: "task_progress", Priority: 90})
	if len(snapshot.TaskDecompositions) > 0 {
		sections = append(sections, ContextSection{ID: "task_decomposition", Priority: 85})
	}
	if len(snapshot.ExecutionSections) > 0 {
		sections = append(sections, ContextSection{ID: "execution_sections", Priority: 84})
	}
	if len(snapshot.MemoryRecallTraces) > 0 {
		sections = append(sections, ContextSection{ID: "memory_recall", Priority: 83})
	}
	if len(snapshot.Decisions) > 0 || len(snapshot.Approvals) > 0 {
		sections = append(sections, ContextSection{ID: "governance", Priority: 80})
	}
	if len(runScopedResults(snapshot)) > 0 || len(runScopedArtifacts(snapshot)) > 0 {
		sections = append(sections, ContextSection{ID: "formal_outputs", Priority: 90})
	}
	if len(recentEvidence) > 0 {
		sections = append(sections, ContextSection{ID: "recent_evidence", Priority: 70})
	}
	sections = append(sections, ContextSection{ID: "recovery", Priority: 100})
	sections = compactContextSections(sections)
	refs := make([]string, 0, len(sections))
	for _, section := range sections {
		refs = append(refs, section.ID)
	}
	return compactUniqueStrings(refs)
}

func hasNonDefinitionExecutionContext(executionContext map[string]any) bool {
	for key := range executionContext {
		if key != "goal" && key != "dod" && key != "compressedContext" {
			return true
		}
	}
	return false
}

func mapTaskDecompositions(snapshot RunSnapshot) []TaskDecomposition {
	if len(snapshot.Steps) == 0 {
		return nil
	}
	stepByNodeRoot := taskExecutionStepsByRoot(snapshot.Steps)
	ret := make([]TaskDecomposition, 0)
	for _, step := range snapshot.Steps {
		graph := taskGraphFromValue(step.Output["executionGraph"])
		nodes := readModelMapSliceValue(graph["nodes"])
		if len(nodes) == 0 {
			continue
		}
		rootStepID := step.ID
		executedByNode := stepByNodeRoot[rootStepID]
		item := TaskDecomposition{
			ID:          rootStepID + "/task-decomposition",
			WorkspaceID: snapshot.Run.WorkspaceID,
			RunID:       snapshot.Run.ID,
			RootStepID:  rootStepID,
			State:       "pending",
			Summary:     timelineStepSummary(step.Output),
			GraphBranch: readModelStringValue(step.Output["graphBranch"]),
			NodeTotal:   len(nodes),
			EvidenceRefs: compactUniqueStrings([]string{
				rootStepID,
			}),
		}
		for _, node := range nodes {
			taskNode := taskDecompositionNode(node, executedByNode)
			item.Nodes = append(item.Nodes, taskNode)
			item.EvidenceRefs = append(item.EvidenceRefs, taskNode.EvidenceRefs...)
			switch taskNode.State {
			case "completed":
				item.NodeCompleted++
			case "failed":
				item.NodeFailed++
			case "running":
				item.NodeRunning++
			}
		}
		item.State = taskDecompositionState(item)
		item.EvidenceRefs = compactUniqueStrings(item.EvidenceRefs)
		ret = append(ret, item)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapTaskPlans(snapshot RunSnapshot) []TaskPlan {
	decompositions := runSnapshotTaskDecompositions(snapshot)
	if len(decompositions) == 0 {
		return nil
	}
	stepByID := map[string]presenter.StepView{}
	for _, step := range snapshot.Steps {
		stepByID[step.ID] = step
	}
	occurrences := timelineStepOccurrences(snapshot)
	grouped := map[string][]TaskPlan{}
	for _, decomposition := range decompositions {
		if strings.TrimSpace(decomposition.RootStepID) == "" {
			continue
		}
		step, hasStep := stepByID[decomposition.RootStepID]
		anchorStepID := strings.TrimSpace(decomposition.RootStepID)
		stage := ""
		state := taskPlanStateFromDecomposition(decomposition.State)
		occurredAt := strings.TrimSpace(occurrences[decomposition.RootStepID])
		if hasStep {
			if root := strings.TrimSpace(readModelStringValue(step.Input["graphRootStepId"])); root != "" {
				anchorStepID = root
			}
			stage = strings.TrimSpace(readModelStringValue(step.Output["stage"]))
			if stage == "" {
				stage = strings.TrimSpace(readModelStringValue(step.Input["stage"]))
			}
			state = taskPlanStateFromStep(step.State)
		}
		plan := TaskPlan{
			WorkspaceID:      snapshot.Run.WorkspaceID,
			RunID:            snapshot.Run.ID,
			AnchorStepID:     anchorStepID,
			RevisionStepID:   decomposition.RootStepID,
			State:            state,
			Stage:            stage,
			Summary:          decomposition.Summary,
			GraphBranch:      decomposition.GraphBranch,
			NodeTotal:        decomposition.NodeTotal,
			NodeCompleted:    decomposition.NodeCompleted,
			NodeFailed:       decomposition.NodeFailed,
			NodeRunning:      decomposition.NodeRunning,
			EvidenceRefs:     compactUniqueStrings(append([]string{decomposition.RootStepID}, decomposition.EvidenceRefs...)),
			OccurredAt:       occurredAt,
			SourceProjection: "task_decomposition",
		}
		grouped[anchorStepID] = append(grouped[anchorStepID], plan)
	}
	if len(grouped) == 0 {
		return nil
	}
	ret := make([]TaskPlan, 0)
	anchors := make([]string, 0, len(grouped))
	for anchor := range grouped {
		anchors = append(anchors, anchor)
	}
	sort.Strings(anchors)
	for _, anchor := range anchors {
		plans := grouped[anchor]
		sort.SliceStable(plans, func(i, j int) bool {
			leftAt := strings.TrimSpace(plans[i].OccurredAt)
			rightAt := strings.TrimSpace(plans[j].OccurredAt)
			if leftAt == rightAt {
				return plans[i].RevisionStepID < plans[j].RevisionStepID
			}
			if leftAt == "" {
				return false
			}
			if rightAt == "" {
				return true
			}
			return leftAt < rightAt
		})
		for idx := range plans {
			plans[idx].Revision = idx + 1
			plans[idx].ID = fmt.Sprintf("%s/plan/%d", plans[idx].AnchorStepID, plans[idx].Revision)
			if plans[idx].Revision > 1 && plans[idx].State == "accepted" {
				plans[idx].State = "revised"
			}
			ret = append(ret, plans[idx])
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapClosureTraces(snapshot RunSnapshot) []ClosureTrace {
	ret := make([]ClosureTrace, 0)
	for _, step := range snapshot.Steps {
		trace, ok := closureTraceFromStep(step)
		if ok {
			ret = append(ret, trace)
		}
	}
	return ret
}

func closureTraceFromStep(step presenter.StepView) (ClosureTrace, bool) {
	input := step.Input
	output := step.Output
	trace := ClosureTrace{
		StepID: step.ID,
		Kind:   step.Kind,
		State:  step.State,
	}
	trace.GraphFollowUp = readModelBoolValue(input["graphFollowUp"])
	trace.AssessmentOnly = readModelBoolValue(input["assessmentOnly"]) || readModelBoolValue(input["providerRetryAssessmentOnly"])
	trace.GraphExpansionLimitFollowUp = readModelBoolValue(input["graphExpansionLimitFollowUp"])
	trace.GraphFollowUpExpansionLimitExceeded = readModelBoolValue(output["graphFollowUpExpansionLimitExceeded"])
	trace.GraphFollowUpExpansionCount = firstPositiveInt(
		readModelIntValue(output["graphFollowUpExpansionCount"]),
		readModelIntValue(input["graphFollowUpExpansionCount"]),
	)
	trace.GraphFollowUpExpansionLimit = firstPositiveInt(
		readModelIntValue(output["graphFollowUpExpansionLimit"]),
		readModelIntValue(input["graphFollowUpExpansionLimit"]),
	)
	trace.IgnoredExecutionGraph = readModelBoolValue(output["ignoredExecutionGraph"])
	trace.Reason = strings.TrimSpace(firstNonEmptyReadModel(
		readModelStringValue(input["assessmentOnlyReason"]),
		readModelStringValue(output["error"]),
		readModelStringValue(output["reason"]),
	))
	trace.MissingDoDItems = readModelMissingDoDItemsFromStepOutput(output)
	trace.EvidenceRefs = compactUniqueStrings([]string{step.ID})
	ok := trace.GraphFollowUp ||
		trace.AssessmentOnly ||
		trace.GraphExpansionLimitFollowUp ||
		trace.GraphFollowUpExpansionLimitExceeded ||
		trace.IgnoredExecutionGraph ||
		len(trace.MissingDoDItems) > 0
	return trace, ok
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func taskPlanStateFromStep(state string) string {
	switch strings.TrimSpace(state) {
	case string(common.StepStateCompleted):
		return "accepted"
	case string(common.StepStateFailed):
		return "rejected"
	case string(common.StepStateDispatched):
		return "proposed"
	default:
		return "proposed"
	}
}

func taskPlanStateFromDecomposition(state string) string {
	switch strings.TrimSpace(state) {
	case "completed":
		return "accepted"
	case "failed":
		return "rejected"
	default:
		return "proposed"
	}
}

func taskExecutionStepsByRoot(steps []presenter.StepView) map[string]map[string]presenter.StepView {
	ret := map[string]map[string]presenter.StepView{}
	for _, step := range steps {
		nodeID := readModelStringValue(step.Input["graphNodeId"])
		if nodeID == "" {
			continue
		}
		rootStepID := readModelStringValue(step.Input["graphRootStepId"])
		if rootStepID == "" {
			rootStepID = graphRootStepIDFromStepID(step.ID)
		}
		if rootStepID == "" {
			continue
		}
		if ret[rootStepID] == nil {
			ret[rootStepID] = map[string]presenter.StepView{}
		}
		ret[rootStepID][nodeID] = step
	}
	return ret
}

func taskGraphFromValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneReadModelMap(typed)
	default:
		return nil
	}
}

func taskDecompositionNode(raw map[string]any, executedByNode map[string]presenter.StepView) TaskDecompositionNode {
	nodeID := readModelStringValue(raw["id"])
	input, _ := raw["input"].(map[string]any)
	ret := TaskDecompositionNode{
		ID:        nodeID,
		Kind:      readModelStringValue(raw["kind"]),
		Title:     taskNodeTitle(raw),
		State:     "pending",
		DependsOn: readModelStringSliceValue(raw["dependsOn"]),
		Branch:    readModelStringValue(raw["branch"]),
		Input:     cloneReadModelMap(input),
	}
	if executedByNode != nil {
		if step, ok := executedByNode[nodeID]; ok {
			ret.StepID = step.ID
			ret.StepState = step.State
			ret.State = taskNodeStateFromStep(step.State)
			ret.EvidenceRefs = compactUniqueStrings([]string{nodeID, step.ID})
			return ret
		}
	}
	ret.EvidenceRefs = compactUniqueStrings([]string{nodeID})
	return ret
}

func taskNodeTitle(raw map[string]any) string {
	if title := readModelStringValue(raw["summary"]); title != "" {
		return title
	}
	input, _ := raw["input"].(map[string]any)
	if command := readModelStringValue(input["command"]); command != "" {
		return command
	}
	if tool := readModelStringValue(input["tool"]); tool != "" {
		return tool
	}
	if operation := readModelStringValue(input["operation"]); operation != "" {
		return operation
	}
	return readModelStringValue(raw["kind"])
}

func taskNodeStateFromStep(state string) string {
	switch state {
	case string(common.StepStateCompleted):
		return "completed"
	case string(common.StepStateFailed):
		return "failed"
	case string(common.StepStateDispatched):
		return "running"
	default:
		if state != "" {
			return state
		}
		return "pending"
	}
}

func taskDecompositionState(item TaskDecomposition) string {
	if item.NodeFailed > 0 {
		return "failed"
	}
	if item.NodeTotal > 0 && item.NodeCompleted == item.NodeTotal {
		return "completed"
	}
	if item.NodeCompleted > 0 || item.NodeRunning > 0 {
		return "running"
	}
	return "pending"
}

func mapExecutionSections(snapshot RunSnapshot) []ExecutionSection {
	if len(snapshot.TaskDecompositions) == 0 {
		return nil
	}
	dodItems := runDoDItems(snapshot)
	refs := executionEvidenceRefsByStep(snapshot)
	ret := make([]ExecutionSection, 0)
	for _, decomposition := range snapshot.TaskDecompositions {
		for _, node := range decomposition.Nodes {
			section := ExecutionSection{
				ID:              decomposition.ID + "/section/" + node.ID,
				WorkspaceID:     snapshot.Run.WorkspaceID,
				RunID:           snapshot.Run.ID,
				Title:           firstNonEmptyReadModel(node.Title, node.ID, node.Kind),
				State:           node.State,
				GraphRootStepID: decomposition.RootStepID,
				GraphNodeID:     node.ID,
				StepID:          node.StepID,
				StepKind:        node.Kind,
				RelatedDoDItems: relatedDoDItemsForSection(node, dodItems),
				PlannedStepRefs: compactUniqueStrings([]string{decomposition.RootStepID, node.ID}),
				Summary:         executionSectionSummary(node),
			}
			section.ActualEvidenceRefs = compactUniqueStrings(append(cloneReadModelStringSlice(node.EvidenceRefs), refs.evidenceByStep[node.StepID]...))
			section.DecisionRefs = compactUniqueStrings(refs.decisionsByStep[node.StepID])
			section.ApprovalRefs = compactUniqueStrings(refs.approvalsByStep[node.StepID])
			section.ToolCallRefs = compactUniqueStrings(refs.toolCallsByStep[node.StepID])
			section.PatchRefs = compactUniqueStrings(refs.patchesByStep[node.StepID])
			section.ValidationRefs = compactUniqueStrings(refs.validationsByStep[node.StepID])
			section.ResultRefs = compactUniqueStrings(refs.resultsByStep[node.StepID])
			section.ArtifactRefs = compactUniqueStrings(refs.artifactsByStep[node.StepID])
			section.MemoryRecallRefs = compactUniqueStrings(refs.memoryRecallsByStep[node.StepID])
			section.MemoryIDs = compactUniqueStrings(refs.memoryIDsByStep[node.StepID])
			ret = append(ret, section)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

type executionEvidenceRefs struct {
	evidenceByStep      map[string][]string
	decisionsByStep     map[string][]string
	approvalsByStep     map[string][]string
	toolCallsByStep     map[string][]string
	patchesByStep       map[string][]string
	validationsByStep   map[string][]string
	resultsByStep       map[string][]string
	artifactsByStep     map[string][]string
	memoryRecallsByStep map[string][]string
	memoryIDsByStep     map[string][]string
}

func executionEvidenceRefsByStep(snapshot RunSnapshot) executionEvidenceRefs {
	ret := executionEvidenceRefs{
		evidenceByStep:      map[string][]string{},
		decisionsByStep:     map[string][]string{},
		approvalsByStep:     map[string][]string{},
		toolCallsByStep:     map[string][]string{},
		patchesByStep:       map[string][]string{},
		validationsByStep:   map[string][]string{},
		resultsByStep:       map[string][]string{},
		artifactsByStep:     map[string][]string{},
		memoryRecallsByStep: map[string][]string{},
		memoryIDsByStep:     map[string][]string{},
	}
	add := func(target map[string][]string, stepID string, refs ...string) {
		stepID = strings.TrimSpace(stepID)
		if stepID == "" {
			return
		}
		for _, ref := range refs {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			target[stepID] = append(target[stepID], ref)
			ret.evidenceByStep[stepID] = append(ret.evidenceByStep[stepID], ref)
		}
	}
	for _, item := range snapshot.Decisions {
		add(ret.decisionsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.Approvals {
		add(ret.approvalsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.ToolCalls {
		add(ret.toolCallsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.Patches {
		add(ret.patchesByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.ValidationResults {
		add(ret.validationsByStep, item.StepID, item.ID, item.EvidenceRef)
	}
	for _, item := range snapshot.Results {
		if item.SourceType == "dod_assessment" {
			add(ret.resultsByStep, item.SourceRef, append([]string{item.ID}, item.EvidenceRefs...)...)
		}
	}
	for _, item := range snapshot.Artifacts {
		add(ret.artifactsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.MemoryRecallTraces {
		add(ret.memoryRecallsByStep, item.StepID, item.ID, item.EvidenceRef)
		add(ret.memoryIDsByStep, item.StepID, item.RetrievedMemoryIDs...)
	}
	for stepID, refs := range ret.evidenceByStep {
		ret.evidenceByStep[stepID] = compactUniqueStrings(refs)
	}
	return ret
}

func runDoDItems(snapshot RunSnapshot) []string {
	if snapshot.Run.DoDID == "" {
		return nil
	}
	for _, item := range snapshot.Results {
		if item.DoDID != snapshot.Run.DoDID || item.DoDAssessment == nil {
			continue
		}
		ret := make([]string, 0, len(item.DoDAssessment.Items))
		for _, assessmentItem := range item.DoDAssessment.Items {
			if strings.TrimSpace(assessmentItem.Item) != "" {
				ret = append(ret, assessmentItem.Item)
			}
		}
		if len(ret) > 0 {
			return ret
		}
	}
	return nil
}

func relatedDoDItemsForSection(node TaskDecompositionNode, dodItems []string) []string {
	if len(dodItems) == 0 {
		return nil
	}
	haystack := strings.ToLower(strings.Join([]string{node.ID, node.Title, node.Kind}, " "))
	ret := make([]string, 0)
	for _, item := range dodItems {
		if strings.Contains(haystack, strings.ToLower(strings.TrimSpace(item))) {
			ret = append(ret, item)
		}
	}
	return ret
}

func executionSectionSummary(node TaskDecompositionNode) string {
	switch node.State {
	case "completed":
		return "section completed with linked runtime evidence"
	case "failed":
		return "section failed; inspect linked evidence and governance records"
	case "running":
		return "section is running"
	default:
		return "section is planned"
	}
}

func graphRootStepIDFromStepID(stepID string) string {
	idx := strings.Index(stepID, "/block/")
	if idx <= 0 {
		return ""
	}
	return stepID[:idx]
}

func lastContextEvidence(items []ContextEvidence, limit int) []ContextEvidence {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	start := len(items) - limit
	if start < 0 {
		start = 0
	}
	return append([]ContextEvidence(nil), items[start:]...)
}

func resultRefsFromTaskProgress(progress TaskProgress) []string {
	var refs []string
	for _, item := range progress.Items {
		if item.Kind == "result" || item.Kind == "artifact" {
			refs = append(refs, item.ID)
			refs = append(refs, item.EvidenceRefs...)
		}
	}
	return compactUniqueStrings(refs)
}

func contextBundleSummary(snapshot RunSnapshot, progress TaskProgress) string {
	if progress.Summary != "" {
		return progress.Summary
	}
	return "run " + snapshot.Run.State + " context bundle for recovery and compression"
}

func contextWorkspaceSection(snapshot RunSnapshot) ContextSection {
	return ContextSection{
		ID:       "workspace",
		Kind:     "workspace",
		Title:    "Workspace",
		Priority: 100,
		State:    snapshot.Workspace.State,
		Summary:  snapshot.Workspace.Title,
		Data: map[string]any{
			"id":                   snapshot.Workspace.ID,
			"title":                snapshot.Workspace.Title,
			"description":          snapshot.Workspace.Description,
			"deliverableDirectory": snapshot.Workspace.DeliverableDirectory,
		},
	}
}

func contextRunSection(snapshot RunSnapshot) ContextSection {
	return ContextSection{
		ID:       "run",
		Kind:     "run",
		Title:    "Run",
		Priority: 100,
		State:    snapshot.Run.State,
		Summary:  snapshot.Run.FailureReason,
		EvidenceRefs: compactUniqueStrings([]string{
			snapshot.Run.ID,
			snapshot.Run.BlockingTicketID,
			snapshot.Run.BlockingApprovalID,
		}),
		Data: map[string]any{
			"id":                 snapshot.Run.ID,
			"workspaceId":        snapshot.Run.WorkspaceID,
			"goalId":             snapshot.Run.GoalID,
			"doDId":              snapshot.Run.DoDID,
			"state":              snapshot.Run.State,
			"currentStepId":      currentStepID(snapshot),
			"blockingTicketId":   snapshot.Run.BlockingTicketID,
			"blockingApprovalId": snapshot.Run.BlockingApprovalID,
			"failureReason":      snapshot.Run.FailureReason,
		},
	}
}

func contextGoalSection(snapshot RunSnapshot) ContextSection {
	goal, _ := snapshot.Run.ExecutionContext["goal"].(map[string]any)
	return ContextSection{
		ID:       "goal",
		Kind:     "goal",
		Title:    "Goal",
		Priority: 95,
		Summary:  readModelStringValue(goal["text"]),
		Data:     cloneReadModelMap(goal),
	}
}

func contextDoDSection(snapshot RunSnapshot) ContextSection {
	dod, _ := snapshot.Run.ExecutionContext["dod"].(map[string]any)
	return ContextSection{
		ID:       "dod",
		Kind:     "dod",
		Title:    "Definition of Done",
		Priority: 95,
		State:    readModelStringValue(dod["state"]),
		Summary:  contextDoDSummary(dod),
		Data:     cloneReadModelMap(dod),
	}
}

func contextExecutionContextSection(snapshot RunSnapshot) ContextSection {
	data := cloneReadModelMap(snapshot.Run.ExecutionContext)
	delete(data, "goal")
	delete(data, "dod")
	delete(data, "compressedContext")
	return ContextSection{
		ID:       "execution_context",
		Kind:     "execution_context",
		Title:    "Execution context",
		Priority: 85,
		Summary:  "selected Soul, Skill, MCP, Memory, and provider context",
		Data:     data,
	}
}

func contextTaskProgressSection(progress TaskProgress) ContextSection {
	return ContextSection{
		ID:           "task_progress",
		Kind:         "task_progress",
		Title:        "Task progress",
		Priority:     90,
		State:        progress.State,
		Summary:      progress.Summary,
		EvidenceRefs: progress.EvidenceRefs,
		Data: map[string]any{
			"currentMilestone": progress.CurrentMilestone,
			"milestones":       progress.Milestones,
			"blockers":         progress.Blockers,
			"recovery":         progress.Recovery,
		},
	}
}

func contextTaskDecompositionSection(items []TaskDecomposition) ContextSection {
	if len(items) == 0 {
		return ContextSection{}
	}
	refs := make([]string, 0, len(items))
	state := "completed"
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
		if item.State == "failed" {
			state = "failed"
			continue
		}
		if item.State != "completed" && state != "failed" {
			state = "running"
		}
	}
	return ContextSection{
		ID:           "task_decomposition",
		Kind:         "task_decomposition",
		Title:        "Task decomposition",
		Priority:     85,
		State:        state,
		Summary:      strconv.Itoa(len(items)) + " decomposition graphs",
		EvidenceRefs: refs,
		Data: map[string]any{
			"taskDecompositions": items,
		},
	}
}

func contextExecutionSectionsSection(items []ExecutionSection) ContextSection {
	if len(items) == 0 {
		return ContextSection{}
	}
	refs := make([]string, 0)
	state := "completed"
	for _, item := range items {
		refs = append(refs, item.ActualEvidenceRefs...)
		if item.State == "failed" {
			state = "failed"
			continue
		}
		if item.State != "completed" && state != "failed" {
			state = "running"
		}
	}
	return ContextSection{
		ID:           "execution_sections",
		Kind:         "execution_sections",
		Title:        "Execution sections",
		Priority:     84,
		State:        state,
		Summary:      strconv.Itoa(len(items)) + " execution sections",
		EvidenceRefs: compactUniqueStrings(refs),
		Data: map[string]any{
			"executionSections": items,
		},
	}
}

func contextMemoryRecallSection(items []MemoryRecallTrace) ContextSection {
	if len(items) == 0 {
		return ContextSection{}
	}
	refs := make([]string, 0)
	memoryIDs := make([]string, 0)
	status := "selected"
	for _, item := range items {
		refs = append(refs, item.ID, item.EvidenceRef)
		memoryIDs = append(memoryIDs, item.RetrievedMemoryIDs...)
		if item.Status == "degraded" {
			status = "degraded"
		}
	}
	return ContextSection{
		ID:           "memory_recall",
		Kind:         "memory_recall",
		Title:        "Memory recall",
		Priority:     83,
		State:        status,
		Summary:      strconv.Itoa(len(items)) + " memory recall traces",
		EvidenceRefs: compactUniqueStrings(append(refs, memoryIDs...)),
		Data: map[string]any{
			"memoryRecallTraces": items,
			"memoryIds":          compactUniqueStrings(memoryIDs),
		},
	}
}

func contextGovernanceSection(snapshot RunSnapshot) ContextSection {
	language := snapshotLanguage(snapshot)
	if len(snapshot.Decisions) == 0 && len(snapshot.Approvals) == 0 {
		return ContextSection{}
	}
	return ContextSection{
		ID:           "governance",
		Kind:         "governance",
		Title:        localizedMilestoneTitle(language, "governance"),
		Priority:     80,
		State:        governanceProgressState(snapshot),
		Summary:      "decision tickets and approvals that affected run progress",
		EvidenceRefs: governanceEvidenceRefs(snapshot),
		Data: map[string]any{
			"decisions": snapshot.Decisions,
			"approvals": snapshot.Approvals,
		},
	}
}

func contextFormalOutputSection(snapshot RunSnapshot) ContextSection {
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	if len(results) == 0 && len(artifacts) == 0 {
		return ContextSection{}
	}
	return ContextSection{
		ID:           "formal_outputs",
		Kind:         "formal_outputs",
		Title:        "Formal outputs",
		Priority:     90,
		State:        resultProgressState(snapshot),
		Summary:      formalOutputSummary(results),
		EvidenceRefs: append(idsFromResults(results), idsFromArtifacts(artifacts)...),
		Data: map[string]any{
			"results":   results,
			"artifacts": artifacts,
		},
	}
}

func contextRecentEvidenceSection(items []ContextEvidence) ContextSection {
	if len(items) == 0 {
		return ContextSection{}
	}
	refs := make([]string, 0, len(items))
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
	}
	return ContextSection{
		ID:           "recent_evidence",
		Kind:         "recent_evidence",
		Title:        "Recent evidence",
		Priority:     70,
		Summary:      "latest runtime timeline items retained for recovery",
		EvidenceRefs: compactUniqueStrings(refs),
		Data: map[string]any{
			"items": items,
		},
	}
}

func contextRecoverySection(recovery RecoveryPlan) ContextSection {
	return ContextSection{
		ID:       "recovery",
		Kind:     "recovery",
		Title:    "Recovery recommendation",
		Priority: 100,
		State:    recovery.Mode,
		Summary:  recovery.NextAction,
		EvidenceRefs: compactUniqueStrings([]string{
			recovery.StepID,
			recovery.BlockingTicketID,
			recovery.BlockingApprovalID,
			recovery.CheckpointID,
		}),
		Data: map[string]any{
			"mode":               recovery.Mode,
			"nextAction":         recovery.NextAction,
			"runId":              recovery.RunID,
			"stepId":             recovery.StepID,
			"blockingTicketId":   recovery.BlockingTicketID,
			"blockingApprovalId": recovery.BlockingApprovalID,
			"checkpointId":       recovery.CheckpointID,
			"reason":             recovery.Reason,
		},
	}
}

func contextRecentEvidence(timeline []RuntimeTimelineItem, limit int) []ContextEvidence {
	if len(timeline) == 0 || limit <= 0 {
		return nil
	}
	start := len(timeline) - limit
	if start < 0 {
		start = 0
	}
	ret := make([]ContextEvidence, 0, len(timeline)-start)
	for _, item := range timeline[start:] {
		ret = append(ret, ContextEvidence{
			ID:         firstNonEmptyReadModel(item.ID, item.Topic, item.StepID),
			Kind:       firstNonEmptyReadModel(item.Source+"/"+item.Type, item.Source),
			State:      firstNonEmptyReadModel(item.StepState, item.RunState),
			Summary:    item.Summary,
			OccurredAt: item.OccurredAt,
			EvidenceRefs: compactUniqueStrings([]string{
				item.ID,
				item.StepID,
				item.Topic,
			}),
			Data: compactContextEvidenceData(item),
		})
	}
	return ret
}

func compactContextEvidenceData(item RuntimeTimelineItem) map[string]any {
	data := map[string]any{}
	if item.Topic != "" {
		data["topic"] = item.Topic
	}
	if item.RunState != "" {
		data["runState"] = item.RunState
	}
	if item.StepState != "" {
		data["stepState"] = item.StepState
	}
	if item.Summary != "" {
		data["summary"] = item.Summary
	}
	if len(data) == 0 {
		return nil
	}
	return data
}

func contextCompressionCandidates(bundle ContextBundle) []ContextCompressionCandidate {
	var ret []ContextCompressionCandidate
	if len(bundle.RecentEvidence) > 8 {
		ret = append(ret, ContextCompressionCandidate{
			SectionID:     "recent_evidence",
			Reason:        "recent evidence exceeds compact recovery window",
			Retention:     "retain latest evidence ids, states, summaries, and timestamps",
			SourceRefs:    contextEvidenceRefs(bundle.RecentEvidence),
			SuggestedMode: "summarize",
		})
	}
	if len(bundle.TaskProgress.Items) > 10 {
		ret = append(ret, ContextCompressionCandidate{
			SectionID:     "task_progress",
			Reason:        "task progress contains many low-level items",
			Retention:     "retain milestones, blockers, recovery, and accepted result refs",
			SourceRefs:    bundle.TaskProgress.EvidenceRefs,
			SuggestedMode: "milestone_summary",
		})
	}
	for _, section := range bundle.Sections {
		if section.ID == "execution_context" && len(section.Data) > 4 {
			ret = append(ret, ContextCompressionCandidate{
				SectionID:     section.ID,
				Reason:        "execution context catalog can be summarized after activation",
				Retention:     "retain selected ids and policy evidence, summarize verbose catalog entries",
				SuggestedMode: "catalog_summary",
			})
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func contextEvidenceRefs(items []ContextEvidence) []string {
	var refs []string
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
	}
	return compactUniqueStrings(refs)
}

func compactContextSections(items []ContextSection) []ContextSection {
	ret := make([]ContextSection, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		item.EvidenceRefs = compactUniqueStrings(item.EvidenceRefs)
		ret = append(ret, item)
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].Priority == ret[j].Priority {
			return ret[i].ID < ret[j].ID
		}
		return ret[i].Priority > ret[j].Priority
	})
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func currentStepID(snapshot RunSnapshot) string {
	if snapshot.Step == nil {
		return ""
	}
	return snapshot.Step.ID
}

func contextDoDSummary(dod map[string]any) string {
	if len(dod) == 0 {
		return ""
	}
	switch items := dod["items"].(type) {
	case []string:
		return strconv.Itoa(len(items)) + " DoD items"
	case []any:
		return strconv.Itoa(len(items)) + " DoD items"
	default:
		return ""
	}
}

func formalOutputSummary(items []Result) string {
	if result := primaryTaskProgressResult(items); result != nil {
		return result.Summary
	}
	return ""
}

func taskProgressBlockers(snapshot RunSnapshot) []ProgressBlocker {
	var ret []ProgressBlocker
	for _, item := range snapshot.Decisions {
		if isResolvedDecisionState(item.State) {
			continue
		}
		ret = append(ret, ProgressBlocker{
			ID:          item.ID,
			Kind:        "decision",
			State:       item.State,
			Reason:      firstNonEmptyReadModel(item.Question, item.Reason),
			EvidenceRef: item.StepID,
		})
	}
	for _, item := range snapshot.Approvals {
		if isResolvedApprovalState(item.State) {
			continue
		}
		ret = append(ret, ProgressBlocker{
			ID:          item.ID,
			Kind:        "approval",
			State:       item.State,
			Reason:      item.Reason,
			EvidenceRef: item.StepID,
		})
	}
	return ret
}

func taskProgressItems(snapshot RunSnapshot) []ProgressItem {
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	ret := make([]ProgressItem, 0, len(snapshot.Steps)+len(snapshot.ToolCalls)+len(snapshot.ValidationResults)+len(results)+len(artifacts)+len(snapshot.Decisions)+len(snapshot.Approvals))
	for _, item := range snapshot.Steps {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "step", Title: firstNonEmptyReadModel(item.Kind, item.ID), State: item.State, EvidenceRefs: []string{item.ID}})
	}
	for _, item := range snapshot.ToolCalls {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "tool_call", Title: firstNonEmptyReadModel(item.ToolName, item.ID), State: item.State, EvidenceRefs: []string{item.ID, item.StepID}})
	}
	for _, item := range snapshot.Patches {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "patch", Title: firstNonEmptyReadModel(item.TargetRef, item.PatchType, item.ID), State: item.State, EvidenceRefs: []string{item.ID, item.ToolCallID, item.StepID}})
	}
	for _, item := range snapshot.ValidationResults {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "validation_result", Title: firstNonEmptyReadModel(item.ValidationKind, item.TargetRef, item.ID), State: item.State, EvidenceRefs: []string{item.ID, item.EvidenceRef, item.StepID}})
	}
	for _, item := range results {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "result", Title: firstNonEmptyReadModel(item.Title, item.SourceType, item.ID), State: item.State, EvidenceRefs: compactUniqueStrings(append([]string{item.ID, item.SourceRef}, item.EvidenceRefs...))})
	}
	for _, item := range artifacts {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "artifact", Title: firstNonEmptyReadModel(item.Name, item.URI, item.ArtifactType, item.ID), State: item.State, EvidenceRefs: compactUniqueStrings(append([]string{item.ID, item.ResultID, item.StepID, item.ToolCallID, item.PatchID, item.ValidationResultID}, item.EvidenceRefs...))})
	}
	for _, item := range snapshot.Decisions {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "decision", Title: firstNonEmptyReadModel(item.Question, item.DecisionKind, item.ID), State: item.State, EvidenceRefs: []string{item.ID, item.StepID}})
	}
	for _, item := range snapshot.Approvals {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "approval", Title: firstNonEmptyReadModel(item.Reason, item.ID), State: item.State, EvidenceRefs: []string{item.ID, item.StepID}})
	}
	for idx := range ret {
		ret[idx].EvidenceRefs = compactUniqueStrings(ret[idx].EvidenceRefs)
	}
	return ret
}

func taskProgressRecovery(snapshot RunSnapshot, latestCheckpoint *RuntimeCheckpoint, blockers []ProgressBlocker, fallback RecoveryPlan) RecoveryPlan {
	ret := fallback
	ret.RunID = snapshot.Run.ID
	ret.BlockingTicketID = snapshot.Run.BlockingTicketID
	ret.BlockingApprovalID = snapshot.Run.BlockingApprovalID
	if latestCheckpoint != nil {
		ret.CheckpointID = latestCheckpoint.ID
		if ret.BlockingTicketID == "" && ret.BlockingApprovalID == "" && latestCheckpoint.CurrentStepID != "" {
			ret.StepID = latestCheckpoint.CurrentStepID
		} else if ret.StepID == "" {
			ret.StepID = latestCheckpoint.CurrentStepID
		}
		if ret.BlockingTicketID == "" {
			ret.BlockingTicketID = latestCheckpoint.BlockingTicketID
		}
		if ret.BlockingApprovalID == "" {
			ret.BlockingApprovalID = latestCheckpoint.BlockingApprovalID
		}
		if ret.FailureClass == "" {
			ret.FailureClass = strings.TrimSpace(readModelStringValue(latestCheckpoint.Data["failureClass"]))
		}
		if ret.FailureSignature == "" {
			ret.FailureSignature = strings.TrimSpace(readModelStringValue(latestCheckpoint.Data["failureSignature"]))
		}
	}
	if ret.BlockingTicketID != "" {
		ret.Mode = "wait_decision"
		ret.NextAction = "resolve_decision_ticket"
		ret.Reason = blockerReason(blockers, "decision", ret.BlockingTicketID)
		return ret
	}
	if ret.BlockingApprovalID != "" {
		ret.Mode = "wait_approval"
		ret.NextAction = "resolve_approval"
		ret.Reason = blockerReason(blockers, "approval", ret.BlockingApprovalID)
		return ret
	}
	switch common.RunState(snapshot.Run.State) {
	case common.RunStateCompleted:
		ret.Mode = "none"
		ret.NextAction = "inspect_result"
		ret.Reason = "run completed"
	case common.RunStateFailed:
		ret.Mode = "inspect_failure"
		ret.NextAction = "inspect_failure_reason_and_evidence"
		ret.Reason = snapshot.Run.FailureReason
	case common.RunStateCancelled, common.RunStateSuperseded:
		ret.Mode = "restart"
		ret.NextAction = "start_new_run"
		ret.Reason = "run is terminal but not completed"
	case common.RunStateValidating:
		ret.Mode = "continue"
		ret.NextAction = "continue_validation"
		ret.Reason = "run is validating"
	default:
		ret.Mode = "continue"
		ret.NextAction = "continue_run"
		ret.Reason = "run is not terminal"
	}
	return ret
}

func taskProgressSummary(progress TaskProgress, snapshot *RunSnapshot) string {
	if snapshot != nil {
		results := runScopedResults(*snapshot)
		artifacts := runScopedArtifacts(*snapshot)
		if result := primaryTaskProgressResult(results); result != nil && result.Summary != "" {
			return result.Summary
		}
		if snapshot.Run.FailureReason != "" {
			return snapshot.Run.FailureReason
		}
		changedFiles := appliedPatchTargetCount(snapshot.Patches)
		summary := "run " + snapshot.Run.State + " with " + strconv.Itoa(len(snapshot.Steps)) + " steps, " + strconv.Itoa(len(results)) + " results, " + strconv.Itoa(len(artifacts)) + " artifacts"
		if changedFiles > 0 {
			summary += ", " + strconv.Itoa(changedFiles) + " changed files"
		}
		return summary
	}
	return "workspace " + progress.State + " with " + strconv.Itoa(len(progress.Blockers)) + " open blockers"
}

func appliedPatchTargetCount(items []Patch) int {
	if len(items) == 0 {
		return 0
	}
	targets := map[string]struct{}{}
	for _, item := range items {
		if item.State != string(common.PatchStateApplied) {
			continue
		}
		target := strings.TrimSpace(item.TargetRef)
		if target == "" {
			target = strings.TrimSpace(item.ID)
		}
		if target == "" {
			continue
		}
		targets[target] = struct{}{}
	}
	return len(targets)
}

func mapTaskProgressRepairLoop(snapshot RunSnapshot, recovery RecoveryPlan) RepairLoop {
	ret := RepairLoop{
		State:      "inactive",
		NextAction: recovery.NextAction,
	}
	attemptCount, lastAttempt, lastRepairStepID := repairAttemptProjection(snapshot.Steps)
	ret.AttemptCount = attemptCount
	ret.LastAttempt = lastAttempt
	ret.LastRepairStepID = lastRepairStepID

	lastFailure := latestRepairFailureProjection(snapshot)
	if lastFailure != nil {
		ret.LastFailureStepID = lastFailure.stepID
		ret.LastFailureClass = lastFailure.class
		ret.LastFailureSignature = lastFailure.signature
		ret.LastFailureAt = lastFailure.occurredAt
		ret.EvidenceRefs = append(ret.EvidenceRefs, lastFailure.evidenceRef)
		if failedStep := stepByID(snapshot.Steps, lastFailure.stepID); failedStep != nil {
			ret.MissingDoDItems = readModelMissingDoDItemsFromStepOutput(failedStep.Output)
		}
	}
	projectRepairGroundingObservability(&ret, snapshot)

	if ticketID, reason, escalationContext, ok := repairEscalationProjection(snapshot, recovery); ok {
		ret.Escalated = true
		ret.EscalationTicketID = ticketID
		ret.EscalationReason = reason
		ret.EvidenceRefs = append(ret.EvidenceRefs, ticketID)
		ret.State = "escalated_decision"
		if items := readModelMapSliceValue(escalationContext["missingDoDItems"]); len(items) > 0 {
			ret.MissingDoDItems = items
		}
		if attempts := readModelMapSliceValue(escalationContext["recentRepairAttempts"]); len(attempts) > 0 {
			ret.RecentAttempts = attempts
		}
		if suggestions := readModelStringSliceValue(escalationContext["recommendedNextActions"]); len(suggestions) > 0 {
			ret.RecommendedNextSteps = suggestions
		}
	} else {
		switch recovery.Mode {
		case "wait_decision":
			ret.State = "awaiting_decision"
		case "wait_approval":
			ret.State = "awaiting_approval"
		case "none":
			if attemptCount > 0 {
				ret.State = "completed_after_repair"
			}
		case "inspect_failure", "restart":
			ret.State = "terminated"
		default:
			if attemptCount > 0 || lastFailure != nil {
				ret.State = "repairing"
			}
		}
	}

	ret.EvidenceRefs = compactUniqueStrings(append(ret.EvidenceRefs, ret.LastRepairStepID, ret.LastFailureStepID))
	return ret
}

func projectRepairGroundingObservability(ret *RepairLoop, snapshot RunSnapshot) {
	if ret == nil || strings.TrimSpace(ret.LastRepairStepID) == "" {
		return
	}
	repairStep := stepByID(snapshot.Steps, ret.LastRepairStepID)
	if repairStep == nil {
		return
	}
	actionPackage := readModelMapValue(repairStep.Input["repairActionPackage"])
	targetDiagnostics := readModelMapValue(actionPackage["targetRelevanceDiagnostics"])
	ret.LastFailureTargetScope = strings.TrimSpace(readModelFirstNonEmptyString(
		readModelStringValue(repairStep.Input["failureTargetScope"]),
		readModelStringValue(actionPackage["failureTargetScope"]),
		readModelStringValue(targetDiagnostics["failureTargetScope"]),
	))
	if signals := readModelMapValue(repairStep.Input["structuredFailureSignals"]); len(signals) > 0 {
		ret.StructuredFailureSignals = signals
	} else if signals := readModelMapValue(actionPackage["structuredFailureSignals"]); len(signals) > 0 {
		ret.StructuredFailureSignals = signals
	}
	if compaction := readModelMapValue(repairStep.Input["repairInputCompaction"]); len(compaction) > 0 {
		ret.RepairInputCompaction = compaction
	}
	if repeated := readModelMapValue(actionPackage["repeatedTargetRepairMode"]); len(repeated) > 0 {
		ret.RepeatedFailureReason = strings.TrimSpace(readModelStringValue(repeated["reason"]))
	}
	ret.LastRepairInputBytes = readModelJSONSize(repairStep.Input)
}

func readModelJSONSize(value any) int {
	if value == nil {
		return 0
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return len(encoded)
}

func readModelFirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

type repairFailureProjection struct {
	stepID      string
	class       string
	signature   string
	occurredAt  string
	evidenceRef string
}

func repairAttemptProjection(steps []presenter.StepView) (count int, lastAttempt int, lastStepID string) {
	for _, step := range steps {
		if !readModelBoolValue(step.Input["runtimeRepairFollowUp"]) {
			continue
		}
		count++
		attempt := readModelIntValue(step.Input["runtimeRepairAttempt"])
		if attempt <= 0 {
			attempt = 1
		}
		if attempt > lastAttempt || (attempt == lastAttempt && step.ID > lastStepID) {
			lastAttempt = attempt
			lastStepID = step.ID
		}
	}
	if count > 0 && lastAttempt == 0 {
		lastAttempt = count
	}
	return count, lastAttempt, lastStepID
}

func latestRepairFailureProjection(snapshot RunSnapshot) *repairFailureProjection {
	var latest *repairFailureProjection
	for _, checkpoint := range snapshot.RuntimeCheckpoints {
		class := strings.TrimSpace(readModelStringValue(checkpoint.Data["failureClass"]))
		signature := strings.TrimSpace(readModelStringValue(checkpoint.Data["failureSignature"]))
		event := strings.TrimSpace(readModelStringValue(checkpoint.Data["event"]))
		if class == "" && signature == "" && event != "step_failed" {
			continue
		}
		candidate := &repairFailureProjection{
			stepID:      checkpoint.CurrentStepID,
			class:       class,
			signature:   signature,
			occurredAt:  checkpoint.OccurredAt,
			evidenceRef: checkpoint.ID,
		}
		if latest == nil || candidate.occurredAt > latest.occurredAt || (candidate.occurredAt == latest.occurredAt && candidate.evidenceRef > latest.evidenceRef) {
			latest = candidate
		}
	}
	if latest != nil {
		return latest
	}
	for _, step := range snapshot.Steps {
		class := strings.TrimSpace(readModelStringValue(step.Output["failureClass"]))
		signature := strings.TrimSpace(readModelStringValue(step.Output["failureSignature"]))
		if class == "" && signature == "" {
			continue
		}
		latest = &repairFailureProjection{
			stepID:      step.ID,
			class:       class,
			signature:   signature,
			evidenceRef: step.ID,
		}
	}
	return latest
}

func stepByID(steps []presenter.StepView, stepID string) *presenter.StepView {
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return nil
	}
	for idx := range steps {
		if steps[idx].ID == stepID {
			return &steps[idx]
		}
	}
	return nil
}

func readModelMissingDoDItemsFromStepOutput(output map[string]any) []map[string]any {
	if len(output) == 0 {
		return nil
	}
	diagnostics := readModelMapValue(output["runtimeDiagnostics"])
	if items := readModelMapSliceValue(diagnostics["missingDoDItems"]); len(items) > 0 {
		return items
	}
	if failedOutput := readModelMapValue(output["failedStepOutput"]); len(failedOutput) > 0 {
		if items := readModelMissingDoDItemsFromStepOutput(failedOutput); len(items) > 0 {
			return items
		}
	}
	return nil
}

func repairEscalationProjection(snapshot RunSnapshot, recovery RecoveryPlan) (ticketID string, reason string, context map[string]any, ok bool) {
	if recovery.BlockingTicketID == "" {
		return "", "", nil, false
	}
	for _, item := range snapshot.Decisions {
		if item.ID != recovery.BlockingTicketID {
			continue
		}
		context := readModelMapValue(item.Context)
		if readModelIntValue(context["attempts"]) > 0 ||
			strings.TrimSpace(readModelStringValue(context["failureClass"])) != "" ||
			strings.TrimSpace(readModelStringValue(context["failureSignature"])) != "" {
			return item.ID, firstNonEmptyReadModel(item.Reason, item.Question), context, true
		}
		return "", "", nil, false
	}
	return "", "", nil, false
}

func primaryTaskProgressResult(items []Result) *Result {
	for idx := range items {
		if items[idx].State == string(common.ResultStateAccepted) {
			return &items[idx]
		}
	}
	if len(items) == 0 {
		return nil
	}
	return &items[len(items)-1]
}

func stepProgressState(snapshot RunSnapshot) string {
	if len(snapshot.Steps) == 0 {
		return "pending"
	}
	if hasAcceptedRunResult(snapshot) && snapshot.Run.State == string(common.RunStateCompleted) {
		return "completed"
	}
	for _, item := range snapshot.Steps {
		if item.State == string(common.StepStateFailed) {
			return "failed"
		}
	}
	if runtimeProgress(snapshot).StepCompleted == len(snapshot.Steps) {
		return "completed"
	}
	return "running"
}

func governanceProgressState(snapshot RunSnapshot) string {
	if len(snapshot.Decisions) == 0 && len(snapshot.Approvals) == 0 {
		return "pending"
	}
	if snapshot.Run.BlockingTicketID != "" || snapshot.Run.BlockingApprovalID != "" {
		return "blocked"
	}
	for _, item := range snapshot.Decisions {
		if !isResolvedDecisionState(item.State) {
			return "blocked"
		}
	}
	for _, item := range snapshot.Approvals {
		if !isResolvedApprovalState(item.State) {
			return "blocked"
		}
	}
	return "completed"
}

func validationProgressState(snapshot RunSnapshot) string {
	if len(snapshot.ValidationResults) == 0 {
		if snapshot.Run.State == string(common.RunStateValidating) {
			return "running"
		}
		return "pending"
	}
	if hasAcceptedRunResult(snapshot) && snapshot.Run.State == string(common.RunStateCompleted) {
		return "completed"
	}
	for _, item := range snapshot.ValidationResults {
		if item.State == string(common.ValidationResultStateFailed) {
			return "failed"
		}
	}
	return "completed"
}

func resultProgressState(snapshot RunSnapshot) string {
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	if len(results) == 0 && len(artifacts) == 0 {
		if isTerminalRunState(snapshot.Run.State) {
			return "failed"
		}
		return "pending"
	}
	for _, item := range results {
		if item.State == string(common.ResultStateAccepted) {
			return "completed"
		}
	}
	return progressStateFromCount(len(results)+len(artifacts), false, false)
}

func mapRunOutcome(snapshot RunSnapshot) RunOutcome {
	ret := RunOutcome{
		RunID:    snapshot.Run.ID,
		State:    snapshot.Run.State,
		RawState: snapshot.Run.State,
		Terminal: isTerminalRunState(snapshot.Run.State),
	}
	if result := primaryAcceptedResult(runScopedResults(snapshot)); result != nil {
		ret.State = "accepted"
		ret.Accepted = true
		ret.Terminal = true
		ret.ResultID = result.ID
		ret.Summary = result.Summary
		ret.EvidenceRefs = compactUniqueStrings(result.EvidenceRefs)
		ret.ArtifactRefs = compactUniqueStrings(result.ArtifactRefs)
		return ret
	}
	switch common.RunState(snapshot.Run.State) {
	case common.RunStateCompleted:
		ret.State = "failed"
		ret.Terminal = true
		ret.Reason = "run completed without accepted result"
	case common.RunStateFailed:
		ret.State = "failed"
		ret.Terminal = true
		ret.Reason = snapshot.Run.FailureReason
	case common.RunStateCancelled:
		ret.State = string(common.RunStateCancelled)
		ret.Terminal = true
		ret.Reason = "run cancelled"
	case common.RunStateSuperseded:
		ret.State = string(common.RunStateSuperseded)
		ret.Terminal = true
		ret.Reason = "run superseded"
	}
	if ret.Summary == "" {
		ret.Summary = ret.Reason
	}
	return ret
}

func mapWorkspaceOutcome(snapshot WorkspaceSnapshot) WorkspaceOutcome {
	ret := WorkspaceOutcome{
		WorkspaceID:       snapshot.Workspace.ID,
		State:             snapshot.Workspace.State,
		RawWorkspaceState: snapshot.Workspace.State,
	}
	var selectedRun *Run
	if isTerminalProjectionState(snapshot.Workspace.State) {
		selectedRun = latestWorkspaceRunForOutcome(snapshot)
	}
	if selectedRun == nil && snapshot.TaskProgress.RunID != "" {
		for idx := range snapshot.Runs {
			if snapshot.Runs[idx].ID == snapshot.TaskProgress.RunID {
				selectedRun = &snapshot.Runs[idx]
				break
			}
		}
	}
	if selectedRun == nil && snapshot.Workspace.ActiveRunID != "" {
		for idx := range snapshot.Runs {
			if snapshot.Runs[idx].ID == snapshot.Workspace.ActiveRunID {
				selectedRun = &snapshot.Runs[idx]
				break
			}
		}
	}
	if selectedRun == nil && len(snapshot.Runs) > 0 {
		selectedRun = &snapshot.Runs[len(snapshot.Runs)-1]
	}
	if selectedRun == nil {
		return ret
	}
	runSnapshot := RunSnapshot{
		Workspace: snapshot.Workspace,
		Run:       *selectedRun,
		Results:   snapshot.Results,
		Artifacts: snapshot.Artifacts,
	}
	outcome := mapRunOutcome(runSnapshot)
	ret.State = outcome.State
	ret.RunID = outcome.RunID
	ret.ResultID = outcome.ResultID
	ret.Accepted = outcome.Accepted
	ret.Terminal = outcome.Terminal
	ret.Summary = outcome.Summary
	ret.Reason = outcome.Reason
	ret.EvidenceRefs = outcome.EvidenceRefs
	ret.ArtifactRefs = outcome.ArtifactRefs
	return ret
}

func isTerminalProjectionState(state string) bool {
	switch common.WorkspaceState(state) {
	case common.WorkspaceStateCompleted, common.WorkspaceStateFailed, common.WorkspaceStateArchived:
		return true
	default:
		return false
	}
}

func latestWorkspaceRunForOutcome(snapshot WorkspaceSnapshot) *Run {
	if len(snapshot.Runs) == 0 {
		return nil
	}
	runsByID := make(map[string]*Run, len(snapshot.Runs))
	for idx := range snapshot.Runs {
		runsByID[snapshot.Runs[idx].ID] = &snapshot.Runs[idx]
	}
	var selectedEvent *EventRecord
	for idx := range snapshot.EventRecords {
		item := &snapshot.EventRecords[idx]
		if item.WorkspaceID != snapshot.Workspace.ID || item.RunID == "" {
			continue
		}
		if _, ok := runsByID[item.RunID]; !ok {
			continue
		}
		switch item.Topic {
		case events.TopicRunCreated, events.TopicRunStateChanged:
		default:
			continue
		}
		if selectedEvent == nil || item.OccurredAt >= selectedEvent.OccurredAt {
			selectedEvent = item
		}
	}
	if selectedEvent != nil {
		return runsByID[selectedEvent.RunID]
	}
	if result := primaryAcceptedResult(snapshot.Results); result != nil {
		if run := runsByID[result.RunID]; run != nil {
			return run
		}
	}
	return &snapshot.Runs[len(snapshot.Runs)-1]
}

func primaryAcceptedResult(items []Result) *Result {
	var selected *Result
	for idx := range items {
		if items[idx].State != string(common.ResultStateAccepted) {
			continue
		}
		if selected == nil || resultTimeValue(items[idx]) >= resultTimeValue(*selected) {
			selected = &items[idx]
		}
	}
	return selected
}

func resultTimeValue(item Result) string {
	if item.UpdatedAt != "" {
		return item.UpdatedAt
	}
	if item.CreatedAt != "" {
		return item.CreatedAt
	}
	return item.ID
}

func hasAcceptedRunResult(snapshot RunSnapshot) bool {
	for _, item := range runScopedResults(snapshot) {
		if item.State == string(common.ResultStateAccepted) {
			return true
		}
	}
	return false
}

func progressStateFromCount(count int, failed bool, completed bool) string {
	if failed {
		return "failed"
	}
	if completed || count > 0 {
		return "completed"
	}
	return "pending"
}

func currentProgressMilestone(items []ProgressMilestone) string {
	for _, item := range items {
		if item.State != "completed" {
			return item.ID
		}
	}
	if len(items) == 0 {
		return ""
	}
	return items[len(items)-1].ID
}

func taskProgressEvidenceRefs(snapshot RunSnapshot) []string {
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	refs := []string{snapshot.Run.ID}
	if snapshot.Step != nil {
		refs = append(refs, snapshot.Step.ID)
	}
	refs = append(refs, idsFromSteps(snapshot.Steps)...)
	refs = append(refs, idsFromToolCalls(snapshot.ToolCalls)...)
	refs = append(refs, idsFromPatches(snapshot.Patches)...)
	refs = append(refs, idsFromValidations(snapshot.ValidationResults)...)
	refs = append(refs, idsFromCheckpoints(snapshot.RuntimeCheckpoints)...)
	refs = append(refs, idsFromResults(results)...)
	refs = append(refs, idsFromArtifacts(artifacts)...)
	refs = append(refs, governanceEvidenceRefs(snapshot)...)
	return compactUniqueStrings(refs)
}

func runScopedResults(snapshot RunSnapshot) []Result {
	if snapshot.Run.ID == "" {
		return snapshot.Results
	}
	ret := make([]Result, 0, len(snapshot.Results))
	for _, item := range snapshot.Results {
		if item.RunID == snapshot.Run.ID {
			ret = append(ret, item)
		}
	}
	return ret
}

func runScopedArtifacts(snapshot RunSnapshot) []Artifact {
	if snapshot.Run.ID == "" {
		return snapshot.Artifacts
	}
	ret := make([]Artifact, 0, len(snapshot.Artifacts))
	for _, item := range snapshot.Artifacts {
		if item.RunID == snapshot.Run.ID {
			ret = append(ret, item)
		}
	}
	return ret
}

func governanceEvidenceRefs(snapshot RunSnapshot) []string {
	refs := make([]string, 0, len(snapshot.Decisions)+len(snapshot.Approvals))
	for _, item := range snapshot.Decisions {
		refs = append(refs, item.ID, item.StepID, item.ResolutionID)
	}
	for _, item := range snapshot.Approvals {
		refs = append(refs, item.ID, item.StepID)
	}
	return compactUniqueStrings(refs)
}

func idsFromSteps(items []presenter.StepView) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func idsFromToolCalls(items []ToolCall) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func idsFromPatches(items []Patch) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func idsFromValidations(items []ValidationResult) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID, item.EvidenceRef)
	}
	return compactUniqueStrings(ret)
}

func idsFromCheckpoints(items []RuntimeCheckpoint) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func idsFromResults(items []Result) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
		ret = append(ret, item.EvidenceRefs...)
		ret = append(ret, item.ArtifactRefs...)
	}
	return compactUniqueStrings(ret)
}

func idsFromArtifacts(items []Artifact) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
		ret = append(ret, item.EvidenceRefs...)
	}
	return compactUniqueStrings(ret)
}

func blockerReason(items []ProgressBlocker, kind string, id string) string {
	for _, item := range items {
		if item.Kind == kind && item.ID == id {
			return item.Reason
		}
	}
	return ""
}

func isResolvedDecisionState(state string) bool {
	return state == string(common.DecisionTicketStateResolved)
}

func isResolvedApprovalState(state string) bool {
	return state == string(common.ApprovalStateApproved) || state == string(common.ApprovalStateRejected) || state == string(common.ApprovalStateResolved)
}

func firstNonEmptyReadModel(values ...string) string {
	for _, item := range values {
		if item != "" {
			return item
		}
	}
	return ""
}

func compactUniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runtimeProgress(snapshot RunSnapshot) RunProgress {
	ret := RunProgress{
		State:     snapshot.Run.State,
		StepTotal: len(snapshot.Steps),
		Blocked:   snapshot.Run.BlockingTicketID != "" || snapshot.Run.BlockingApprovalID != "",
		Terminal:  isTerminalRunState(snapshot.Run.State),
	}
	if snapshot.Step != nil {
		ret.CurrentStepID = snapshot.Step.ID
		ret.CurrentStepKind = snapshot.Step.Kind
		ret.CurrentStepState = snapshot.Step.State
	}
	for _, step := range snapshot.Steps {
		switch step.State {
		case string(common.StepStateDispatched):
			ret.StepDispatched++
		case string(common.StepStateCompleted):
			ret.StepCompleted++
		case string(common.StepStateFailed):
			ret.StepFailed++
		}
	}
	return ret
}

func isTerminalRunState(state string) bool {
	switch common.RunState(state) {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func snapshotLanguage(snapshot RunSnapshot) string {
	if snapshot.Run.ExecutionContext == nil {
		return defaultDialogLanguage()
	}
	if lang, ok := snapshot.Run.ExecutionContext["dialogLanguage"].(string); ok {
		if normalized := languageNameFromLocale(lang); normalized != "" {
			return normalized
		}
	}
	return defaultDialogLanguage()
}

func workspaceSnapshotLanguage(snapshot WorkspaceSnapshot) string {
	if snapshot.Workspace.ActiveRunID != "" {
		for _, item := range snapshot.Runs {
			if item.ID == snapshot.Workspace.ActiveRunID {
				return languageFromExecutionContext(item.ExecutionContext)
			}
		}
	}
	for idx := len(snapshot.Runs) - 1; idx >= 0; idx-- {
		if lang := languageFromExecutionContext(snapshot.Runs[idx].ExecutionContext); lang != "" {
			return lang
		}
	}
	return defaultDialogLanguage()
}

func languageFromExecutionContext(executionContext map[string]any) string {
	if executionContext == nil {
		return ""
	}
	if lang, ok := executionContext["dialogLanguage"].(string); ok {
		return languageNameFromLocale(lang)
	}
	return ""
}

func localizedWorkspaceProgressText(language string, key string) string {
	if prefersChinese(language) {
		switch key {
		case "no_active_run":
			return "工作空间尚无活跃运行（Run）"
		case "active_run":
			return "工作空间存在活跃运行（Run）投影"
		case "workspace_defined":
			return "工作空间（Workspace）已定义"
		case "runs":
			return "运行（Run）"
		case "results":
			return "结果（Result）"
		case "artifacts":
			return "交付物（Artifact）"
		case "context_summary":
			return "无活跃运行（Run）的工作空间上下文包"
		case "workspace":
			return "工作空间（Workspace）"
		case "task_progress":
			return "任务进度"
		}
	}
	switch key {
	case "no_active_run":
		return "workspace has no active Run"
	case "active_run":
		return "workspace has an active Run projection"
	case "workspace_defined":
		return "Workspace defined"
	case "runs":
		return "Runs"
	case "results":
		return "Results"
	case "artifacts":
		return "Artifacts"
	case "context_summary":
		return "workspace context bundle without active Run"
	case "workspace":
		return "Workspace"
	case "task_progress":
		return "Task progress"
	default:
		return key
	}
}
