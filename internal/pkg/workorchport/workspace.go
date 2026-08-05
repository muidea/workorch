package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type WorkspaceSnapshotInfo struct {
	Workspace               projectioncontract.Workspace
	ViewModel               projectioncontract.WorkspaceViewModel
	Projection              projectioncontract.ProjectionStatus
	Run                     *projectioncontract.Run
	Step                    *projectioncontract.StepView
	Result                  *projectioncontract.Result
	Outcome                 projectioncontract.WorkspaceOutcome
	TaskProgress            projectioncontract.TaskProgress
	RunHeartbeat            projectioncontract.RunHeartbeat
	ConvergenceStatus       *projectioncontract.ConvergenceStatus
	RunCommand              *projectioncontract.RunCommandStatus
	SchedulerDiagnostics    *projectioncontract.SchedulerDiagnostics
	ContextBundle           *projectioncontract.ContextBundle
	CoverageSummary         workorcheventpayload.Payload
	GapBaseline             *projectioncontract.GapBaseline
	AuthorityDiagnostics    []projectioncontract.GapDiagnostic
	Acceptance              projectioncontract.AcceptanceProjection
	TaskDecompositions      []projectioncontract.TaskDecomposition
	TaskPlans               []projectioncontract.TaskPlan
	ExecutionSections       []projectioncontract.ExecutionSection
	Goals                   []workorchmanagementcontract.Goal
	DoDs                    []workorchmanagementcontract.DoD
	Runs                    []projectioncontract.Run
	Results                 []projectioncontract.Result
	Artifacts               []projectioncontract.Artifact
	ArtifactContinuations   []projectioncontract.ArtifactContinuation
	Patches                 []projectioncontract.Patch
	MemoryRecallTraces      []projectioncontract.MemoryRecallTrace
	WebSources              []projectioncontract.WebSourceTrace
	LLMContextBudgets       []projectioncontract.LLMContextBudgetTrace
	LLMContextBudgetSummary projectioncontract.LLMContextBudgetSummary
	LLMUsageTraces          []projectioncontract.LLMUsageTrace
	LLMUsageSummary         projectioncontract.LLMUsageSummary
	EventRecords            []projectioncontract.EventRecord
	Decisions               []projectioncontract.Decision
	Approvals               []projectioncontract.Approval
}

// CloneWorkspaceSnapshotInfo detaches the process-facing aggregate read from
// owner and adapter storage. It intentionally remains a port aggregate because
// it also carries the projection revision surrounding the canonical snapshot.
func CloneWorkspaceSnapshotInfo(item WorkspaceSnapshotInfo) WorkspaceSnapshotInfo {
	item.Workspace = projectioncontract.CloneWorkspace(item.Workspace)
	item.ViewModel = projectioncontract.CloneWorkspaceViewModel(item.ViewModel)
	item.Projection = CloneProjectionStatus(item.Projection)
	item.Run = cloneRunPtr(item.Run)
	item.Step = projectioncontract.CloneStepView(item.Step)
	item.Result = cloneResultPtr(item.Result)
	item.Outcome = projectioncontract.CloneWorkspaceOutcome(item.Outcome)
	item.TaskProgress = projectioncontract.CloneTaskProgress(item.TaskProgress)
	item.ConvergenceStatus = projectioncontract.CloneConvergenceStatus(item.ConvergenceStatus)
	item.RunCommand = projectioncontract.CloneRunCommandStatus(item.RunCommand)
	item.SchedulerDiagnostics = projectioncontract.CloneSchedulerDiagnostics(item.SchedulerDiagnostics)
	item.ContextBundle = projectioncontract.CloneContextBundle(item.ContextBundle)
	item.CoverageSummary = item.CoverageSummary.Clone()
	item.GapBaseline = projectioncontract.CloneGapBaseline(item.GapBaseline)
	item.AuthorityDiagnostics = projectioncontract.CloneGapDiagnostics(item.AuthorityDiagnostics)
	item.Acceptance = projectioncontract.CloneAcceptanceProjection(item.Acceptance)
	item.TaskDecompositions = projectioncontract.CloneTaskDecompositions(item.TaskDecompositions)
	item.TaskPlans = projectioncontract.CloneTaskPlans(item.TaskPlans)
	item.ExecutionSections = ExecutionSectionsFromProjection(item.ExecutionSections)
	item.Goals = workorchmanagementcontract.CloneGoals(item.Goals)
	item.DoDs = workorchmanagementcontract.CloneDoDs(item.DoDs)
	item.Runs = projectioncontract.CloneRuns(item.Runs)
	item.Results = projectioncontract.CloneResults(item.Results)
	item.Artifacts = projectioncontract.CloneArtifacts(item.Artifacts)
	item.ArtifactContinuations = projectioncontract.CloneArtifactContinuations(item.ArtifactContinuations)
	item.Patches = projectioncontract.ClonePatches(item.Patches)
	item.MemoryRecallTraces = projectioncontract.CloneMemoryRecallTraces(item.MemoryRecallTraces)
	item.WebSources = projectioncontract.CloneWebSourceTraces(item.WebSources)
	item.LLMContextBudgets = projectioncontract.CloneLLMContextBudgetTraces(item.LLMContextBudgets)
	item.LLMUsageTraces = projectioncontract.CloneLLMUsageTraces(item.LLMUsageTraces)
	item.LLMUsageSummary = projectioncontract.CloneLLMUsageSummary(item.LLMUsageSummary)
	item.EventRecords = projectioncontract.CloneEventRecords(item.EventRecords)
	item.Decisions = projectioncontract.CloneDecisions(item.Decisions)
	item.Approvals = projectioncontract.CloneApprovals(item.Approvals)
	return item
}

func cloneRunPtr(item *projectioncontract.Run) *projectioncontract.Run {
	if item == nil {
		return nil
	}
	ret := projectioncontract.CloneRun(*item)
	return &ret
}

func cloneResultPtr(item *projectioncontract.Result) *projectioncontract.Result {
	if item == nil {
		return nil
	}
	ret := projectioncontract.CloneResult(*item)
	return &ret
}

type WorkspaceBackend interface {
	Create(ctx context.Context, req workorchmanagementcontract.WorkspaceCreateRequest) (*projectioncontract.Workspace, error)
	Get(ctx context.Context, id common.WorkspaceID) (*projectioncontract.Workspace, error)
	List(ctx context.Context) ([]projectioncontract.Workspace, error)
	Snapshot(ctx context.Context, id common.WorkspaceID) (*WorkspaceSnapshotInfo, error)
	RuntimeView(ctx context.Context, req ReadWorkspaceRuntimeViewRequest) (*WorkspaceRuntimeViewInfo, error)
	UpdateLLMTarget(ctx context.Context, id common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (*projectioncontract.Workspace, error)
	Delete(ctx context.Context, id common.WorkspaceID, purge bool) error
}
