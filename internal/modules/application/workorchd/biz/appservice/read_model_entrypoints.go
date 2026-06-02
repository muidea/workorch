package appservice

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

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
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(query.WorkspaceID))

	runID := common.RunID(strings.TrimSpace(query.RunID))
	runItem, err := getRunFromRepository(ctx, deps.runs, runID)
	if err != nil {
		return StepPage{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, runItem.WorkspaceID)
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
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(query.WorkspaceID))

	runID := common.RunID(strings.TrimSpace(query.RunID))
	runItem, err := getRunFromRepository(ctx, deps.runs, runID)
	if err != nil {
		return ToolCallPage{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, runItem.WorkspaceID)
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
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(query.WorkspaceID))

	runID := common.RunID(strings.TrimSpace(query.RunID))
	runItem, err := getRunFromRepository(ctx, deps.runs, runID)
	if err != nil {
		return RuntimeCheckpointPage{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, runItem.WorkspaceID)
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
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(query.WorkspaceID))

	runID := common.RunID(strings.TrimSpace(query.RunID))
	runItem, err := getRunFromRepository(ctx, deps.runs, runID)
	if err != nil {
		return ResultPage{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, runItem.WorkspaceID)
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
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(query.WorkspaceID))

	runID := common.RunID(strings.TrimSpace(query.RunID))
	runItem, err := getRunFromRepository(ctx, deps.runs, runID)
	if err != nil {
		return PatchPage{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, runItem.WorkspaceID)
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
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(query.WorkspaceID))

	runID := common.RunID(strings.TrimSpace(query.RunID))
	runItem, err := getRunFromRepository(ctx, deps.runs, runID)
	if err != nil {
		return ValidationResultPage{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, runItem.WorkspaceID)
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
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(query.WorkspaceID))

	runID := common.RunID(strings.TrimSpace(query.RunID))
	runItem, err := getRunFromRepository(ctx, deps.runs, runID)
	if err != nil {
		return ArtifactPage{}, err
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, runItem.WorkspaceID)
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
