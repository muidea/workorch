package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// runProjector is a typed, stateless read-only facade table.
type runProjector struct {
	Snapshot            func(RunSnapshotModelInput) readmodelcore.RunSnapshotState
	RecoverySnapshot    func(RunSnapshotModelInput) readmodelcore.RunSnapshotState
	Compact             func(readmodelcore.RunSnapshotState) readmodelcore.RunSnapshotState
	Detail              func(readmodelcore.RunSnapshotState) readmodelcore.RunSnapshotState
	TaskProgress        func(readmodelcore.RunSnapshotState) projectioncontract.TaskProgress
	TaskDecompositions  func(readmodelcore.RunSnapshotState) []projectioncontract.TaskDecomposition
	Observability       func(readmodelcore.RunSnapshotState) readmodelcore.RunObservabilityState
	Status              func(readmodelcore.RunSnapshotState) projectioncontract.RunStatus
	Acceptance          func(appmodelcore.Run) projectioncontract.AcceptanceProjection
	Convergence         func(readmodelcore.RunSnapshotState) *projectioncontract.ConvergenceStatus
	CurrentStatusStep   func(*runmodel.Run, []*runmodel.Step, []*runmodel.RuntimeCheckpoint, []*decisionmodel.Ticket, []*approvalmodel.Approval) *runmodel.Step
	Steps               func([]*runmodel.Step) []projectioncontract.StepView
	CanonicalStepKind   func(string) string
	StepOutput          func(string, map[string]any) map[string]any
	ResultQuality       func(readmodelcore.RunSnapshotState) []projectioncontract.ResultQualityTrace
	Outcome             func(readmodelcore.RunSnapshotState) projectioncontract.RunOutcome
	ResultProgressState func(readmodelcore.RunSnapshotState) string
	HasAcceptedResult   func(readmodelcore.RunSnapshotState) bool
	PublicSnapshot      func(readmodelcore.RunSnapshotState) projectioncontract.RunSnapshot
	PublicObservability func(readmodelcore.RunObservabilityState) projectioncontract.RunObservability
}

// RunProjection returns the one typed application-facing run projection
// facade. It has no lifecycle, persistence, or owner command capability.
func RunProjection() runProjector {
	return runProjector{
		Snapshot:            runSnapshotFromModels,
		RecoverySnapshot:    runRecoverySnapshotFromModels,
		Compact:             projectRunSnapshotCompact,
		Detail:              projectRunSnapshotDetail,
		TaskProgress:        runSnapshotTaskProgress,
		TaskDecompositions:  runSnapshotTaskDecompositions,
		Observability:       mapRunObservability,
		Status:              mapRunStatus,
		Acceptance:          mapAcceptanceProjection,
		Convergence:         mapConvergenceStatus,
		CurrentStatusStep:   currentRunStatusStep,
		Steps:               mapSteps,
		CanonicalStepKind:   canonicalStepKind,
		StepOutput:          projectStepOutput,
		ResultQuality:       mapResultQuality,
		Outcome:             mapRunOutcome,
		ResultProgressState: resultProgressState,
		HasAcceptedResult:   hasAcceptedRunResult,
		PublicSnapshot:      publicRunSnapshot,
		PublicObservability: publicRunObservability,
	}
}
