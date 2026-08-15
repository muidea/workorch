package biz

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

func (p *runtimeApplicationPorts) configureRunPorts() {
	ports := &p.ports
	ports.Run.ReadRun = func(ctx context.Context, _, _, runID string) (appmodelcore.Run, error) {
		var ret appmodelcore.Run
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRunWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	ports.Run.ReadRunSnapshot = func(ctx context.Context, _, _, runID string) (readmodelcore.RunSnapshotState, error) {
		var ret readmodelcore.RunSnapshotState
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRunSnapshotWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	ports.Run.ListRunSteps = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.StepPage, error) {
		var ret readmodelcore.StepPage
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunStepsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	ports.Run.ListRunToolCalls = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.ToolCallPage, error) {
		var ret readmodelcore.ToolCallPage
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunToolCallsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	ports.Run.ListRunRuntimeCheckpoints = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.RuntimeCheckpointPage, error) {
		var ret readmodelcore.RuntimeCheckpointPage
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunRuntimeCheckpointsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	ports.Run.ListRunResults = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.ResultPage, error) {
		var ret readmodelcore.ResultPage
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunResultsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	ports.Run.ListRunPatches = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.PatchPage, error) {
		var ret readmodelcore.PatchPage
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunPatchesWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	ports.Run.ListRunValidationResults = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.ValidationResultPage, error) {
		var ret readmodelcore.ValidationResultPage
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunValidationResultsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	ports.Run.ListRunArtifacts = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (readmodelcore.ArtifactPage, error) {
		var ret readmodelcore.ArtifactPage
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunArtifactsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	ports.Run.ReadRunStatus = func(ctx context.Context, _, _, runID string) (projectioncontract.RunStatus, error) {
		var ret projectioncontract.RunStatus
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRunStatusWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	ports.Run.ReadRunObservability = func(ctx context.Context, _, _, runID string) (readmodelcore.RunObservabilityState, error) {
		var ret readmodelcore.RunObservabilityState
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRunObservabilityWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	ports.Run.ReadArtifactContent = func(ctx context.Context, _, _, artifactURI string) (content.ArtifactContent, error) {
		var ret content.ArtifactContent
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadArtifactContentWithSession(ctx, session, artifactURI)
			return err
		})
		return ret, err
	}
	ports.Run.ReadRuntimeRawContent = func(ctx context.Context, _, _, ref string) (content.RuntimeRawContent, error) {
		var ret content.RuntimeRawContent
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRuntimeRawContentWithSession(ctx, session, ref)
			return err
		})
		return ret, err
	}
	ports.Run.ReadRunTaskProgress = func(ctx context.Context, _, _, runID string) (projectioncontract.TaskProgress, error) {
		var ret projectioncontract.TaskProgress
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRunTaskProgressWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	ports.Run.ReadRunContextBundle = func(ctx context.Context, _, _, runID string) (projectioncontract.ContextBundle, error) {
		var ret projectioncontract.ContextBundle
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRunContextBundleWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	ports.Run.ReadRunTaskDecompositions = func(ctx context.Context, _, _, runID string) ([]projectioncontract.TaskDecomposition, error) {
		var ret []projectioncontract.TaskDecomposition
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadRunTaskDecompositionsWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	ports.Run.ListEventRecords = func(ctx context.Context, _, _ string, query workorchquerycontract.EventRecordQuery) ([]projectioncontract.EventRecord, error) {
		var ret []projectioncontract.EventRecord
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListEventRecordsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	ports.Run.ListRuns = func(ctx context.Context, _, _, workspaceID string) ([]appmodelcore.Run, error) {
		var ret []appmodelcore.Run
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListRunsWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
}
