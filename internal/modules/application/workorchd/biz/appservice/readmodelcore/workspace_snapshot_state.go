package readmodelcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type WorkspaceSnapshotState struct {
	Workspace               appmodelcore.Workspace                     `json:"workspace"`
	ViewModel               projectioncontract.WorkspaceViewModel      `json:"viewModel"`
	Run                     *appmodelcore.Run                          `json:"run,omitempty"`
	Step                    *projectioncontract.StepView               `json:"step,omitempty"`
	Result                  *projectioncontract.Result                 `json:"result,omitempty"`
	Outcome                 projectioncontract.WorkspaceOutcome        `json:"outcome"`
	TaskProgress            projectioncontract.TaskProgress            `json:"taskProgress"`
	RunHeartbeat            projectioncontract.RunHeartbeat            `json:"runHeartbeat,omitempty"`
	ConvergenceStatus       *projectioncontract.ConvergenceStatus      `json:"convergenceStatus,omitempty"`
	RunCommand              *projectioncontract.RunCommandStatus       `json:"runCommand,omitempty"`
	SchedulerDiagnostics    *projectioncontract.SchedulerDiagnostics   `json:"schedulerDiagnostics,omitempty"`
	ContextBundle           *projectioncontract.ContextBundle          `json:"contextBundle,omitempty"`
	CoverageSummary         map[string]any                             `json:"coverageSummary,omitempty"`
	GapBaseline             *projectioncontract.GapBaseline            `json:"gapBaseline,omitempty"`
	AuthorityDiagnostics    []projectioncontract.GapDiagnostic         `json:"authorityDiagnostics,omitempty"`
	Acceptance              projectioncontract.AcceptanceProjection    `json:"acceptance"`
	TaskDecompositions      []projectioncontract.TaskDecomposition     `json:"taskDecompositions,omitempty"`
	TaskPlans               []projectioncontract.TaskPlan              `json:"taskPlans,omitempty"`
	ExecutionSections       []projectioncontract.ExecutionSection      `json:"executionSections,omitempty"`
	MemoryRecallTraces      []projectioncontract.MemoryRecallTrace     `json:"memoryRecallTraces,omitempty"`
	Goals                   []workorchmanagementcontract.Goal          `json:"goals,omitempty"`
	DoDs                    []workorchmanagementcontract.DoD           `json:"dods,omitempty"`
	Runs                    []appmodelcore.Run                         `json:"runs,omitempty"`
	Results                 []projectioncontract.Result                `json:"results,omitempty"`
	Artifacts               []projectioncontract.Artifact              `json:"artifacts,omitempty"`
	ArtifactContinuations   []projectioncontract.ArtifactContinuation  `json:"artifactContinuations,omitempty"`
	Patches                 []projectioncontract.Patch                 `json:"patches,omitempty"`
	WebSources              []projectioncontract.WebSourceTrace        `json:"webSources,omitempty"`
	LLMContextBudgets       []projectioncontract.LLMContextBudgetTrace `json:"llmContextBudgets,omitempty"`
	LLMContextBudgetSummary projectioncontract.LLMContextBudgetSummary `json:"llmContextBudgetSummary"`
	LLMUsageTraces          []projectioncontract.LLMUsageTrace         `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary         projectioncontract.LLMUsageSummary         `json:"llmUsageSummary"`
	EventRecords            []projectioncontract.EventRecord           `json:"eventRecords,omitempty"`
	Decisions               []projectioncontract.Decision              `json:"decisions,omitempty"`
	Approvals               []projectioncontract.Approval              `json:"approvals,omitempty"`
}

type WorkspaceExecutionActivationState struct {
	Workspace appmodelcore.Workspace `json:"workspace"`
	Run       appmodelcore.Run       `json:"run"`
	Snapshot  WorkspaceSnapshotState `json:"snapshot"`
}
