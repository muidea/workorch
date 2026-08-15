package server

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/content"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

// Run/read-model adapters are query and content concerns only. They do not
// issue Run/Runtime commands.
func (s *serverState) configureRunOperations(session *runtimecore.Session) {
	s.readRun = func(ctx context.Context, _, _, runID string) (appmodelcore.Run, error) {
		var ret appmodelcore.Run
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRunWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.readRunSnapshot = func(ctx context.Context, _, _, runID string) (readmodelcore.RunSnapshotState, error) {
		var ret readmodelcore.RunSnapshotState
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRunSnapshotWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.listRunSteps = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.StepPage, error) {
		var ret readmodelcore.StepPage
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunStepsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listRunToolCalls = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.ToolCallPage, error) {
		var ret readmodelcore.ToolCallPage
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunToolCallsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listRunRuntimeCheckpoints = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.RuntimeCheckpointPage, error) {
		var ret readmodelcore.RuntimeCheckpointPage
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunRuntimeCheckpointsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listRunResults = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.ResultPage, error) {
		var ret readmodelcore.ResultPage
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunResultsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listRunPatches = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.PatchPage, error) {
		var ret readmodelcore.PatchPage
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunPatchesWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listRunValidationResults = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.ValidationResultPage, error) {
		var ret readmodelcore.ValidationResultPage
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunValidationResultsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listRunArtifacts = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.ArtifactPage, error) {
		var ret readmodelcore.ArtifactPage
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunArtifactsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.readRunStatus = func(ctx context.Context, _, _, runID string) (projectioncontract.RunStatus, error) {
		var ret projectioncontract.RunStatus
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRunStatusWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.readRunObservability = func(ctx context.Context, _, _, runID string) (readmodelcore.RunObservabilityState, error) {
		var ret readmodelcore.RunObservabilityState
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRunObservabilityWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.readArtifactContent = func(ctx context.Context, _, _, artifactURI string) (content.ArtifactContent, error) {
		var ret content.ArtifactContent
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadArtifactContentWithSession(ctx, session, artifactURI)
			return err
		})
		return ret, err
	}
	s.readRuntimeRawContent = func(ctx context.Context, _, _, ref string) (content.RuntimeRawContent, error) {
		var ret content.RuntimeRawContent
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRuntimeRawContentWithSession(ctx, session, ref)
			return err
		})
		return ret, err
	}
	s.readRunTaskProgress = func(ctx context.Context, _, _, runID string) (projectioncontract.TaskProgress, error) {
		var ret projectioncontract.TaskProgress
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRunTaskProgressWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.readRunContextBundle = func(ctx context.Context, _, _, runID string) (projectioncontract.ContextBundle, error) {
		var ret projectioncontract.ContextBundle
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRunContextBundleWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.readRunTaskDecompositions = func(ctx context.Context, _, _, runID string) ([]projectioncontract.TaskDecomposition, error) {
		var ret []projectioncontract.TaskDecomposition
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRunTaskDecompositionsWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.listEventRecords = func(ctx context.Context, _, _ string, query workorchquerycontract.EventRecordQuery) ([]projectioncontract.EventRecord, error) {
		var ret []projectioncontract.EventRecord
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListEventRecordsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listRuns = func(ctx context.Context, _, _, workspaceID string) ([]appmodelcore.Run, error) {
		var ret []appmodelcore.Run
		err := withBorrowedTestSession(ctx, session, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunsWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
}
